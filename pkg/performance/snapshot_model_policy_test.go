package performance

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hyscale-lab/aries/pkg/core"
)

func policyBoundaryFixture(t *testing.T, root, policy string) ([]map[string]any, map[string]any) {
	t.Helper()
	events, index := boundaryFixture(t, root)
	start, finish := maps.Clone(events[5]), maps.Clone(events[5])
	start["event"], finish["event"] = "request_match_started", "request_matched"
	finish["tool_content_differences"] = []any{map[string]any{
		"schema_version": 1, "path": "/messages/2/content", "encoding": "utf-8",
		"recorded": map[string]any{"bytes": 12, "sha256": strings.Repeat("a", 64)},
		"actual":   map[string]any{"bytes": 16, "sha256": strings.Repeat("b", 64)},
		"content":  "SECRET_TOOL_OUTPUT",
	}}
	events = append(events[:6:6], append([]map[string]any{start, finish}, events[6:]...)...)
	for i, event := range events {
		event["sequence"], event["replay_policy"] = i+1, policy
	}
	index["attempted"], index["written"], index["replay_policy"] = len(events), len(events), policy
	return events, index
}

func TestSnapshotWorkloadPolicyAndRedactedDifferencesReachBothTraces(t *testing.T) {
	root, directory := traceFixture(t)
	events, index := policyBoundaryFixture(t, root, "workload")
	writeBoundaryFixture(t, root, events, index)
	snapshot, err := BuildSnapshot(directory)
	if err != nil {
		t.Fatalf("%v: %v", err, snapshot.Diagnostics)
	}
	// Exercise the frozen-file path used by the CLI, not just in-memory export.
	content, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "snapshot.json")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSnapshot(path)
	if err != nil || loaded.SnapshotID != snapshot.SnapshotID {
		t.Fatal("workload snapshot lost frozen identity", err)
	}
	snapshot = loaded
	matched := false
	for _, interval := range snapshot.Intervals {
		if interval.Category != "model" {
			continue
		}
		if interval.Attributes["replay_policy"] != "workload" {
			t.Fatal("model span lost workload policy")
		}
		if interval.Name == "model.match" {
			matched = true
			if len(interval.Attributes["tool_content_differences"].([]modelToolContentDifference)) != 1 {
				t.Fatal("difference lost")
			}
		}
	}
	if !matched {
		t.Fatal("no measured matching interval")
	}
	trace, err := ProjectTrace(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	otlp, err := ProjectOTLP(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for name, projection := range map[string]any{"snapshot": snapshot, "perfetto": trace, "otlp": otlp} {
		data, err := json.Marshal(projection)
		if err != nil || !strings.Contains(string(data), "replay_policy") || !strings.Contains(string(data), "workload") || !strings.Contains(string(data), "tool_content_differences") || !strings.Contains(string(data), strings.Repeat("a", 64)) || strings.Contains(string(data), "SECRET_") {
			t.Fatalf("%s lost policy/evidence or leaked payload: %v", name, err)
		}
	}
	for name, changed := range map[string]string{
		"changed_digest": strings.Replace(string(content), strings.Repeat("a", 64), strings.Repeat("c", 64), 1),
		"unknown_field":  strings.Replace(string(content), `"tool_content_differences":[{`, `"tool_content_differences":[{"extra":true,`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if changed == string(content) {
				t.Fatal("mutation did not touch fixture")
			}
			if err := os.WriteFile(path, []byte(changed), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadSnapshot(path); err == nil {
				t.Fatal("changed workload snapshot accepted")
			}
		})
	}
}

func TestSnapshotRejectsInconsistentReplayPolicyAndDifferenceEvidence(t *testing.T) {
	for _, mutation := range []string{"index_mismatch", "unknown_policy", "record_with_policy", "strict_with_difference", "bad_path", "missing_bytes", "bad_digest", "wrong_event", "failed_match"} {
		t.Run(mutation, func(t *testing.T) {
			root, directory := traceFixture(t)
			events, index := policyBoundaryFixture(t, root, "workload")
			difference := events[7]["tool_content_differences"].([]any)[0].(map[string]any)
			switch mutation {
			case "index_mismatch":
				index["replay_policy"] = "strict"
			case "unknown_policy":
				for _, e := range events {
					e["replay_policy"] = "other"
				}
				index["replay_policy"] = "other"
			case "record_with_policy":
				events[7]["mode"] = "record"
			case "strict_with_difference":
				for _, e := range events {
					e["replay_policy"] = "strict"
				}
				index["replay_policy"] = "strict"
			case "bad_path":
				difference["path"] = "/messages/SECRET/content"
			case "missing_bytes":
				delete(difference["actual"].(map[string]any), "bytes")
			case "bad_digest":
				difference["recorded"].(map[string]any)["sha256"] = "SECRET_TOOL_OUTPUT"
			case "wrong_event":
				events[6]["tool_content_differences"] = events[7]["tool_content_differences"]
				delete(events[7], "tool_content_differences")
			case "failed_match":
				events[7]["status"] = "failed"
			}
			writeBoundaryFixture(t, root, events, index)
			snapshot, err := BuildSnapshot(directory)
			if err == nil || snapshot.IntegrityStatus != "incomplete" {
				t.Fatal("invalid model evidence accepted")
			}
			for _, interval := range snapshot.Intervals {
				if interval.Category == "model" {
					t.Fatal("invalid model spans retained")
				}
			}
		})
	}
}

func TestSnapshotReplayPolicyChecksRunSessionAgainstBoundaryEvidence(t *testing.T) {
	for _, mutation := range []string{"matching", "policy", "mode", "missing_attempt", "duplicate", "session_id"} {
		t.Run(mutation, func(t *testing.T) {
			root, directory := traceFixture(t)
			events, index := policyBoundaryFixture(t, root, "workload")
			writeBoundaryFixture(t, root, events, index)
			session := core.ModelSessionResult{AttemptID: "run/occ", SessionID: "model", Mode: "replay", ReplayPolicy: "workload"}
			switch mutation {
			case "policy":
				session.ReplayPolicy = "strict"
			case "mode":
				session.Mode, session.ReplayPolicy = "record", ""
			case "missing_attempt":
				session.AttemptID = "run/other"
			case "session_id":
				session.SessionID = "other"
			}
			sessions := []core.ModelSessionResult{session}
			if mutation == "duplicate" {
				sessions = append(sessions, session)
			}
			writeSummaryJSON(t, root, "run-result.json", map[string]any{"run_id": "run", "attempts": []traceAttempt{{AttemptID: "run/occ", OccurrenceID: "occ", ModelSessionID: "model"}}, "model_sessions": sessions})
			snapshot, err := BuildSnapshot(directory)
			if mutation == "matching" {
				if err != nil || len(snapshot.Attempts) != 1 || snapshot.Attempts[0].ReplayPolicy != "workload" || snapshot.Attempts[0].ModelMode != "replay" {
					t.Fatal("valid session policy lost", err)
				}
			} else if err == nil || snapshot.IntegrityStatus != "incomplete" {
				t.Fatal("session policy contradiction accepted")
			}
		})
	}
}

func TestSnapshotOldPolicyRemainsUnknownAndLegacyPolicyIsPreserved(t *testing.T) {
	for _, source := range []string{"boundary", "legacy"} {
		for _, policy := range []string{"", "strict", "workload"} {
			t.Run(source+"/"+policy, func(t *testing.T) {
				root, directory := traceFixture(t)
				if source == "boundary" {
					events, index := boundaryFixture(t, root)
					for _, e := range events {
						e["replay_policy"] = policy
					}
					index["replay_policy"] = policy
					writeBoundaryFixture(t, root, events, index)
				} else {
					events := modelTraceFixture(t, root)
					for _, event := range events {
						event["replay_policy"] = policy
					}
					writeTraceEvents(t, filepath.Join(root, "occ", "model", "events.jsonl"), events)
					toolTraceFixture(t, root)
				}
				snapshot, err := BuildSnapshot(directory)
				if err != nil {
					t.Fatal(err)
				}
				want := policy
				if want == "" {
					want = "unknown"
				}
				count := 0
				for _, interval := range snapshot.Intervals {
					if interval.Category == "model" {
						count++
						if interval.Attributes["replay_policy"] != want {
							t.Fatal("policy inferred or lost")
						}
					}
				}
				if count == 0 {
					t.Fatal("no model intervals")
				}
			})
		}
	}
}

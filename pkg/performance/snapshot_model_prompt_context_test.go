package performance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func promptContextBoundaryFixture(t *testing.T, root string) ([]map[string]any, map[string]any) {
	t.Helper()
	events, index := policyBoundaryFixture(t, root, "workload")
	digest := func(text string) map[string]any {
		sum := sha256.Sum256([]byte(text))
		return map[string]any{"bytes": len([]byte(text)), "sha256": hex.EncodeToString(sum[:])}
	}
	events[7]["prompt_context_differences"] = []any{
		map[string]any{
			"schema_version": 1, "path": "/messages/0/content", "encoding": "utf-8", "category": "jiuwen_system_context",
			"recorded": digest("SECRET_SYSTEM_CONTEXT 原生路径 /run/record"), "actual": digest("SECRET_SYSTEM_CONTEXT 原生路径 /run/replay"),
		},
		map[string]any{
			"schema_version": 1, "path": "/messages/1/content", "encoding": "utf-8", "category": "jiuwen_user_timestamp",
			"recorded": digest("SECRET_USER_TASK 2026-10-27 12:00:00"), "actual": digest("SECRET_USER_TASK 2026-10-27 12:00:01"),
		},
	}
	return events, index
}

func TestSnapshotPromptContextDifferencesSurviveFrozenDualTraceExport(t *testing.T) {
	root, directory := traceFixture(t)
	events, index := promptContextBoundaryFixture(t, root)
	writeBoundaryFixture(t, root, events, index)
	snapshot, err := BuildSnapshot(directory)
	if err != nil {
		t.Fatalf("%v: %v", err, snapshot.Diagnostics)
	}
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
		t.Fatal("prompt context receipts changed frozen snapshot identity", err)
	}
	matched := 0
	for _, interval := range loaded.Intervals {
		if interval.Name != "model.match" {
			continue
		}
		matched++
		tool, toolOK := interval.Attributes["tool_content_differences"].([]modelToolContentDifference)
		prompt, promptOK := interval.Attributes["prompt_context_differences"].([]modelPromptContextDifference)
		if !toolOK || len(tool) != 1 || !promptOK || len(prompt) != 2 || prompt[0].Category != "jiuwen_system_context" || prompt[1].Category != "jiuwen_user_timestamp" {
			t.Fatal("tool and prompt context differences were conflated or lost typed receipts")
		}
		if interval.Attributes["mode"] != "replay" || interval.Attributes["replay_policy"] != "workload" || interval.Outcome != "succeeded" {
			t.Fatal("prompt context evidence lost its accepted workload match boundary")
		}
	}
	if matched != 1 {
		t.Fatalf("expected one real match interval, got %d", matched)
	}
	trace, err := ProjectTrace(loaded)
	if err != nil {
		t.Fatal(err)
	}
	otlp, err := ProjectOTLP(loaded)
	if err != nil {
		t.Fatal(err)
	}
	prompt := events[7]["prompt_context_differences"].([]any)
	for name, projection := range map[string]any{"snapshot": loaded, "perfetto": trace, "otlp": otlp} {
		data, err := json.Marshal(projection)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"tool_content_differences", "prompt_context_differences", "jiuwen_system_context", "jiuwen_user_timestamp", strings.Repeat("a", 64)} {
			if !bytes.Contains(data, []byte(want)) {
				t.Fatalf("%s lost difference metadata %s", name, want)
			}
		}
		for _, difference := range prompt {
			for _, side := range []string{"recorded", "actual"} {
				want := difference.(map[string]any)[side].(map[string]any)["sha256"].(string)
				if !bytes.Contains(data, []byte(want)) {
					t.Fatalf("%s lost a full-string prompt digest", name)
				}
			}
		}
		if bytes.Contains(data, []byte("SECRET_")) || bytes.Contains(data, []byte("/run/record")) || bytes.Contains(data, []byte("/run/replay")) {
			t.Fatalf("%s contains original prompt or tool text", name)
		}
	}
	originalTrace, err := ProjectTrace(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	originalOTLP, err := ProjectOTLP(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]any{{originalTrace, trace}, {originalOTLP, otlp}} {
		before, _ := json.Marshal(pair[0])
		after, _ := json.Marshal(pair[1])
		if !bytes.Equal(before, after) {
			t.Fatal("frozen reload changed a dual Trace projection")
		}
	}
	for name, changed := range map[string]string{
		"category":   strings.Replace(string(content), "jiuwen_system_context", "unreviewed_context", 1),
		"body_field": strings.Replace(string(content), `"prompt_context_differences":[{`, `"prompt_context_differences":[{"content":"SECRET_PROMPT_BODY",`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if changed == string(content) {
				t.Fatal("mutation did not change the frozen snapshot")
			}
			if err := os.WriteFile(path, []byte(changed), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadSnapshot(path); err == nil {
				t.Fatal("modified prompt context snapshot accepted")
			}
		})
	}
}

func TestSnapshotRejectsInvalidPromptContextDifferenceEvidence(t *testing.T) {
	for _, mutation := range []string{"category", "schema", "path", "encoding", "missing_bytes", "negative_bytes", "digest", "same_digest", "body_field", "digest_body_field", "duplicate_path", "strict", "record", "wrong_event", "failed_match"} {
		t.Run(mutation, func(t *testing.T) {
			root, directory := traceFixture(t)
			events, index := promptContextBoundaryFixture(t, root)
			delete(events[7], "tool_content_differences")
			differences := events[7]["prompt_context_differences"].([]any)
			difference := differences[0].(map[string]any)
			switch mutation {
			case "category":
				difference["category"] = "unreviewed_context"
			case "schema":
				difference["schema_version"] = 2
			case "path":
				difference["path"] = "/messages/0/SECRET_PROMPT_BODY"
			case "encoding":
				difference["encoding"] = "base64"
			case "missing_bytes":
				delete(difference["actual"].(map[string]any), "bytes")
			case "negative_bytes":
				difference["actual"].(map[string]any)["bytes"] = -1
			case "digest":
				difference["actual"].(map[string]any)["sha256"] = "SECRET_PROMPT_BODY"
			case "same_digest":
				difference["actual"] = difference["recorded"]
			case "body_field":
				difference["content"] = "SECRET_PROMPT_BODY"
			case "digest_body_field":
				difference["recorded"].(map[string]any)["content"] = "SECRET_PROMPT_BODY"
			case "duplicate_path":
				differences[1].(map[string]any)["path"] = difference["path"]
			case "strict":
				for _, event := range events {
					event["replay_policy"] = "strict"
				}
				index["replay_policy"] = "strict"
			case "record":
				for _, event := range events {
					event["mode"], event["replay_policy"] = "record", ""
				}
				index["replay_policy"] = ""
			case "wrong_event":
				events[6]["prompt_context_differences"] = differences
				delete(events[7], "prompt_context_differences")
			case "failed_match":
				events[7]["status"] = "failed"
			}
			writeBoundaryFixture(t, root, events, index)
			snapshot, err := BuildSnapshot(directory)
			if err == nil || snapshot.IntegrityStatus != "incomplete" {
				t.Fatal("invalid prompt context evidence accepted")
			}
			for _, interval := range snapshot.Intervals {
				if interval.Category == "model" {
					t.Fatal("invalid model intervals retained")
				}
			}
			for _, event := range snapshot.Events {
				if event.Category == "model" {
					t.Fatal("invalid model events retained")
				}
			}
			data, _ := json.Marshal(snapshot)
			if bytes.Contains(data, []byte("SECRET_")) {
				t.Fatal("rejected evidence leaked prompt text into snapshot diagnostics")
			}
		})
	}
}

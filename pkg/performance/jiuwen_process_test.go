package performance

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func jiuwenProcessFixture(t *testing.T, owner string) (string, string, map[string]any, []map[string]any) {
	t.Helper()
	var root, directory string
	if owner == "bridge" {
		root, directory, _ = jiuwenProviderFixture(t)
	} else {
		root, directory, _ = jiuwenSnapshotFixture(t)
	}
	declaration := "native-spans-index.json"
	if owner == "bridge" {
		declaration = "remote-bridge-index.json"
	}
	base := filepath.Join(root, "occ", owner)
	raw, err := os.ReadFile(filepath.Join(base, declaration))
	if err != nil {
		t.Fatal(err)
	}
	var declared map[string]any
	if err := json.Unmarshal(raw, &declared); err != nil {
		t.Fatal(err)
	}
	declared["process_observation_expected"] = true
	writeSummaryJSON(t, base, declaration, declared)
	index := map[string]any{
		"schema_version": 1, "source": "jiuwen_native_process", "binding": map[string]any{"run_id": "run", "task_id": "occ"},
		"clock":           map[string]any{"emitter_id": strings.Repeat("c", 32), "clock_domain": "jiuwen-process/" + strings.Repeat("c", 32), "origin_monotonic_ns": 1000, "origin_unix_ns": summaryAnchor.UnixNano(), "anchor_before_monotonic_ns": 998, "anchor_after_monotonic_ns": 1002},
		"counters":        map[string]any{"spawn_attempted": 1, "started": 1, "exited": 1, "events_attempted": 8, "events_written": 8, "dropped": 0, "write_failed": 0, "index_write_failed": 0, "late_events": 0, "identity_failed": 0},
		"observer_closed": true, "pending_processes": 0,
	}
	rows := jiuwenProcessRows(owner, []string{"spawn_requested", "connection_made", "spawn_return", "stdout_first", "stderr_first", "child_exit", "stdout_closed", "stderr_closed"})
	writeJiuwenProcessFixture(t, base, index, rows)
	return base, directory, index, rows
}

func jiuwenProcessRows(owner string, names []string) []map[string]any {
	rows := make([]map[string]any, 0, len(names))
	var pid any
	for i, name := range names {
		var invocation any
		if owner == "bridge" {
			invocation = "inv-a"
		}
		if name == "connection_made" {
			pid = 4321
		}
		row := map[string]any{
			"schema_version": 1, "source": "jiuwen_native_process", "sequence": i + 1, "clock_domain": "jiuwen-process/" + strings.Repeat("c", 32),
			"monotonic_ns": 1100 + i*100, "event": name, "process_id": strings.Repeat("d", 32), "pid": pid,
			"identity": map[string]any{"run_id": "run", "task_id": "occ", "session_id": "jiuwen-session", "request_id": "request-run", "tool_call_id": "call-1", "invocation_id": invocation, "identity_status": "known"},
		}
		if name == "child_exit" {
			row["returncode"] = 7
		}
		if name == "spawn_failed" {
			row["error_type"] = "FileNotFoundError"
		}
		rows = append(rows, row)
	}
	return rows
}

func writeJiuwenProcessFixture(t *testing.T, base string, index map[string]any, rows []map[string]any) {
	t.Helper()
	path := filepath.Join(base, "jiuwen-process-events.jsonl")
	writeTraceEvents(t, path, rows)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	index["ledger_sha256"], index["written_bytes"] = fmt.Sprintf("%x", sha256.Sum256(data)), len(data)
	writeSummaryJSON(t, base, "jiuwen-process-index.json", index)
}

func jiuwenProcessIntervals(snapshot AnalysisSnapshot) map[string]AnalysisInterval {
	intervals := map[string]AnalysisInterval{}
	for _, interval := range snapshot.Intervals {
		if interval.Category == "jiuwen_process" {
			intervals[interval.Name] = interval
		}
	}
	return intervals
}

func TestJiuwenProcessABExactIdentityTimingAndExports(t *testing.T) {
	for _, owner := range []string{"bridge", "harness"} {
		t.Run(owner, func(t *testing.T) {
			_, directory, _, rows := jiuwenProcessFixture(t, owner)
			snapshot, err := BuildSnapshot(directory)
			if err != nil {
				t.Fatal(err, snapshot.Diagnostics)
			}
			intervals := jiuwenProcessIntervals(snapshot)
			observation := intervals["jiuwen.process.observation"]
			if len(intervals) != 4 || observation.StartNS != 100 || observation.EndNS != 800 || observation.Attributes["observation_status"] != "complete" {
				t.Fatal("process interval boundaries or denominator", intervals)
			}
			for name, duration := range map[string]int64{"jiuwen.process.creation": 100, "jiuwen.process.command": 400, "jiuwen.process.exit_to_pipe_closed": 200} {
				if intervals[name].DurationNS != duration {
					t.Fatal("process duration changed", name, intervals[name])
				}
			}
			if observation.Attributes["returncode"] != 7 || observation.Attributes["process_completion"] != "direct_child_exit_observed" || observation.Attributes["pipe_closure"] != "pipe_closed_observed" || observation.Attributes["natural_pipe_eof"] != "unproven" || observation.Attributes["all_descendants_completion"] != "unknown" {
				t.Fatal("nonzero child exit or closure meaning changed", observation.Attributes)
			}
			if observation.AttemptID != "run/occ" || observation.AgentSessionID != "jiuwen-session" || observation.RequestInstanceID != "request-run" || observation.ToolCallID != "call-1" {
				t.Fatal("background process lost the spawn-time identity", observation.AnalysisIdentity)
			}
			markers := map[string]AnalysisEvent{}
			for _, event := range snapshot.Events {
				if event.Category != "jiuwen_process" {
					continue
				}
				if event.AnalysisIdentity != observation.AnalysisIdentity || event.Attributes["parent_analysis_operation_id"] != observation.ID || event.ClockDomain != observation.ClockDomain {
					t.Fatal("process callback identity changed", event)
				}
				markers[event.Name] = event
			}
			if len(markers) != len(rows) {
				t.Fatal("raw callback denominator", len(markers), len(rows))
			}
			for _, row := range rows {
				name := "jiuwen.process." + row["event"].(string)
				if markers[name].OffsetNS != int64(row["monotonic_ns"].(int)-1000) {
					t.Fatal("callback clock offset changed", name, markers[name])
				}
			}
			primary, err := ProjectTrace(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			for _, event := range primary.Events {
				if event.Category == "jiuwen_process" {
					t.Fatal("process clock silently aligned to Runner")
				}
			}
			trace, err := ProjectTraceDomain(snapshot, observation.ClockDomain)
			if err != nil {
				t.Fatal(err)
			}
			instantCount, intervalCount := 0, 0
			for _, event := range trace.Events {
				switch event.Phase {
				case "i":
					instantCount++
					if event.Args["event_id"] != markers[event.Name].ID || event.Args["parent_analysis_operation_id"] != observation.ID || event.Timestamp != float64(markers[event.Name].OffsetNS)/1000 {
						t.Fatal("Chrome callback projection changed", event)
					}
				case "X":
					intervalCount++
				case "s", "f":
					t.Fatal("process identity link became a timing flow", event)
				}
			}
			if instantCount != len(rows) || intervalCount != len(intervals) {
				t.Fatal("Chrome process denominator", instantCount, intervalCount)
			}
			out, err := ProjectOTLP(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			spans := map[string]OTLPSpan{}
			for _, resource := range out.Request.ResourceSpans {
				for _, scope := range resource.ScopeSpans {
					for _, span := range scope.Spans {
						id, ok := otlpAttribute(span.Attributes, "aries.operation_id")
						if ok && id.StringValue != nil {
							spans[*id.StringValue] = span
						}
					}
				}
			}
			container := spans[observation.ID]
			if len(container.Events) != len(rows) || container.ParentSpanID != "" || container.StartTimeUnixNano != strconv.FormatInt(summaryAnchor.UnixNano()+100, 10) || container.EndTimeUnixNano != strconv.FormatInt(summaryAnchor.UnixNano()+800, 10) {
				t.Fatal("OTLP process container identity/time/events changed", container)
			}
			seen := map[string]bool{}
			for _, event := range container.Events {
				source, ok := markers[event.Name]
				if !ok || seen[event.Name] || event.TimeUnixNano != strconv.FormatInt(summaryAnchor.UnixNano()+source.OffsetNS, 10) {
					t.Fatal("OTLP callback raw epoch or denominator changed", event)
				}
				seen[event.Name] = true
				for key, want := range map[string]string{"event_id": source.ID, "parent_analysis_operation_id": observation.ID, "clock_domain": source.ClockDomain, "attempt_id": source.AttemptID, "occurrence_id": source.OccurrenceID, "logical_task_id": source.LogicalTaskID, "arrival_id": source.ArrivalID, "purpose": source.Purpose, "agent_session_id": source.AgentSessionID, "request_instance_id": source.RequestInstanceID, "tool_call_id": source.ToolCallID, "process_id": strings.Repeat("d", 32), "invocation_id": observation.Attributes["invocation_id"].(string)} {
					value, exists := otlpAttribute(event.Attributes, "aries."+key)
					if !exists || value.StringValue == nil || *value.StringValue != want {
						t.Fatal("OTLP callback identity changed", key, event)
					}
				}
				if event.Name == "jiuwen.process.child_exit" {
					code, _ := otlpAttribute(event.Attributes, "aries.returncode")
					if code.IntValue == nil || *code.IntValue != "7" {
						t.Fatal("OTLP lost the actual nonzero child exit", event)
					}
				}
			}
			wantLinks := 1
			if owner == "bridge" {
				wantLinks = 2
			}
			links := 0
			for _, edge := range snapshot.Edges {
				if edge.Kind != "process_tool_identity" && edge.Kind != "process_invocation_identity" {
					continue
				}
				links++
				if edge.To != observation.ID || edge.Correlation != "exact_id" {
					t.Fatal("process identity edge direction changed", edge)
				}
				source, ok := spans[edge.From]
				matched := 0
				for _, link := range container.Links {
					kind, _ := otlpAttribute(link.Attributes, "aries.edge_kind")
					if kind.StringValue != nil && *kind.StringValue == edge.Kind && link.TraceID == source.TraceID && link.SpanID == source.SpanID {
						matched++
					}
				}
				if !ok || matched != 1 {
					t.Fatal("OTLP lost native Tool or Provider identity", edge, container.Links)
				}
			}
			if links != wantLinks || len(container.Links) != wantLinks || len(trace.Metadata.IdentityLinks) != wantLinks {
				t.Fatal("process identity link denominator", links, container.Links, trace.Metadata.IdentityLinks)
			}
			writeSummaryJSON(t, filepath.Dir(directory), "process-snapshot.json", snapshot)
			loaded, err := LoadSnapshot(filepath.Join(filepath.Dir(directory), "process-snapshot.json"))
			if err != nil {
				t.Fatal(err)
			}
			reloaded, err := ProjectOTLP(loaded)
			if err != nil || !reflect.DeepEqual(out, reloaded) {
				t.Fatal("frozen process snapshot changed OTLP projection", err)
			}
		})
	}
}

func TestJiuwenProcessPartialAndCallbackOrder(t *testing.T) {
	for _, tc := range []struct {
		name            string
		events          []string
		started, exited int
		partial         bool
		firstOutput     string
		intervalCount   int
	}{
		{"callbacks_before_spawn_return", []string{"spawn_requested", "connection_made", "stdout_first", "stderr_first", "child_exit", "stdout_closed", "stderr_closed", "spawn_return"}, 1, 1, false, "observed", 4},
		{"no_output", []string{"spawn_requested", "connection_made", "spawn_return", "child_exit", "stdout_closed", "stderr_closed"}, 1, 1, false, "no_nonempty_chunk_observed", 4},
		{"pipes_closed_before_exit", []string{"spawn_requested", "connection_made", "spawn_return", "stdout_closed", "stderr_closed", "child_exit"}, 1, 1, false, "no_nonempty_chunk_observed", 3},
		{"spawn_failed_without_pid", []string{"spawn_requested", "spawn_failed"}, 0, 0, false, "unknown", 1},
		{"cancelled_after_connection", []string{"spawn_requested", "connection_made", "spawn_failed"}, 1, 0, true, "unknown", 2},
		{"background_still_running", []string{"spawn_requested", "connection_made", "spawn_return"}, 1, 0, true, "unknown", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base, directory, index, _ := jiuwenProcessFixture(t, "harness")
			rows := jiuwenProcessRows("harness", tc.events)
			counters := index["counters"].(map[string]any)
			counters["started"], counters["exited"] = tc.started, tc.exited
			counters["events_attempted"], counters["events_written"] = len(rows), len(rows)
			if tc.partial {
				index["pending_processes"] = 1
			}
			if tc.name == "cancelled_after_connection" {
				rows[len(rows)-1]["error_type"] = "CancelledError"
			}
			writeJiuwenProcessFixture(t, base, index, rows)
			snapshot, err := BuildSnapshot(directory)
			if (err != nil) != tc.partial || tc.partial && snapshot.IntegrityStatus != "incomplete" {
				t.Fatal("wrong partial observation integrity", err, snapshot.Diagnostics)
			}
			intervals := jiuwenProcessIntervals(snapshot)
			observation := intervals["jiuwen.process.observation"]
			status := "complete"
			if tc.partial {
				status = "partial"
			}
			if len(intervals) != tc.intervalCount || observation.EndNS != int64(len(rows)*100) || observation.Attributes["observation_status"] != status || observation.Attributes["first_output"] != tc.firstOutput {
				t.Fatal("missing callbacks became complete or zero-duration measurements", intervals)
			}
			if tc.firstOutput != "observed" {
				for _, key := range []string{"stdout_first_offset_ns", "stderr_first_offset_ns"} {
					if _, exists := observation.Attributes[key]; exists {
						t.Fatal("absent output became a zero observation", observation.Attributes)
					}
				}
			}
			if tc.started == 0 {
				if _, exists := observation.Attributes["pid"]; exists {
					t.Fatal("failed spawn invented a PID", observation.Attributes)
				}
			}
			if tc.exited == 0 {
				if _, exists := observation.Attributes["returncode"]; exists {
					t.Fatal("missing exit invented a returncode", observation.Attributes)
				}
			}
		})
	}
}

func TestJiuwenProcessRejectsMissingEvidenceCountersAndIdentity(t *testing.T) {
	for _, name := range []string{"missing_pair", "declared_files_absent", "declared_harness_files_absent", "source_hash", "sequence", "clock", "row_clock", "fixed_identity", "foreign_binding", "wrong_invocation", "wrong_harness_session", "late_events", "identity_failed", "pending_processes"} {
		t.Run(name, func(t *testing.T) {
			owner := "bridge"
			if name == "declared_harness_files_absent" || name == "wrong_harness_session" {
				owner = "harness"
			}
			base, directory, index, rows := jiuwenProcessFixture(t, owner)
			counters := index["counters"].(map[string]any)
			switch name {
			case "missing_pair":
				counters["spawn_attempted"], counters["started"], counters["exited"] = 2, 2, 2
			case "sequence":
				rows[2]["sequence"] = 4
			case "clock":
				index["clock"].(map[string]any)["origin_monotonic_ns"] = 999
			case "row_clock":
				rows[2]["clock_domain"] = "jiuwen-process/" + strings.Repeat("e", 32)
			case "fixed_identity":
				rows[3]["identity"].(map[string]any)["tool_call_id"] = "another-tool"
			case "foreign_binding":
				index["binding"].(map[string]any)["task_id"] = "another-occurrence"
			case "wrong_invocation":
				for _, row := range rows {
					row["identity"].(map[string]any)["invocation_id"] = "unobserved-invocation"
				}
			case "wrong_harness_session":
				for _, row := range rows {
					row["identity"].(map[string]any)["session_id"] = "other-session"
				}
			case "late_events", "identity_failed":
				counters[name] = 1
			case "pending_processes":
				index[name] = 1
			}
			writeJiuwenProcessFixture(t, base, index, rows)
			if name == "source_hash" {
				index["ledger_sha256"] = strings.Repeat("0", 64)
				writeSummaryJSON(t, base, "jiuwen-process-index.json", index)
			}
			if name == "declared_files_absent" || name == "declared_harness_files_absent" {
				for _, file := range []string{"jiuwen-process-index.json", "jiuwen-process-events.jsonl"} {
					if err := os.Remove(filepath.Join(base, file)); err != nil {
						t.Fatal(err)
					}
				}
			}
			snapshot, err := BuildSnapshot(directory)
			if err == nil || snapshot.IntegrityStatus != "incomplete" {
				t.Fatal("accepted invalid process evidence", snapshot.Diagnostics)
			}
			if name == "wrong_invocation" || name == "wrong_harness_session" || name == "late_events" || name == "identity_failed" || name == "pending_processes" {
				intervals := jiuwenProcessIntervals(snapshot)
				if len(intervals) != 4 || intervals["jiuwen.process.command"].DurationNS != 400 {
					t.Fatal("incomplete producer or identity evidence erased observed process timing", intervals)
				}
				if intervals["jiuwen.process.observation"].Attributes["observation_status"] != "partial" {
					t.Fatal("producer evidence gap became a complete observation", intervals)
				}
			}
			if name == "missing_pair" {
				found := false
				for _, coverage := range snapshot.Coverage {
					if coverage.Source == "jiuwen_process_bridge" {
						found = true
						if coverage.ExpectedOperations == nil || *coverage.ExpectedOperations != 2 || coverage.Paired != 1 || coverage.Integrity != "incomplete" {
							t.Fatal("missing process lost independent producer denominator", coverage)
						}
					}
				}
				if !found {
					t.Fatal("missing process coverage")
				}
			}
		})
	}
}

func TestJiuwenProcessCallbackStampsAreIndependentOfLedgerOrder(t *testing.T) {
	base, directory, index, rows := jiuwenProcessFixture(t, "harness")
	// Callbacks stamp their entry before the writer serializes their evidence.
	rows[3]["monotonic_ns"] = 1550
	rows[7]["monotonic_ns"] = 1650
	writeJiuwenProcessFixture(t, base, index, rows)
	snapshot, err := BuildSnapshot(directory)
	if err != nil {
		t.Fatal(err, snapshot.Diagnostics)
	}
	intervals := jiuwenProcessIntervals(snapshot)
	observation := intervals["jiuwen.process.observation"]
	if observation.EndNS != 700 || intervals["jiuwen.process.command"].DurationNS != 400 || intervals["jiuwen.process.exit_to_pipe_closed"].DurationNS != 100 {
		t.Fatal("ledger order replaced actual callback times", intervals)
	}
	out, err := ProjectOTLP(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, resource := range out.Request.ResourceSpans {
		for _, scope := range resource.ScopeSpans {
			for _, span := range scope.Spans {
				if span.Name == "jiuwen.process.observation" {
					if len(span.Events) != len(rows) {
						t.Fatal("callback outside incorrectly shortened container", span)
					}
					return
				}
			}
		}
	}
	t.Fatal("missing process observation span")
}

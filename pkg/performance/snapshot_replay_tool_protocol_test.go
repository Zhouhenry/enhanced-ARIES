package performance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// Synthetic parser inputs exercise identity and clock contracts, not a cloud run.
func replayProtocolFixture(t *testing.T) (*identitySnapshotFixture, []map[string]any) {
	t.Helper()
	f := newIdentitySnapshotFixture(t)
	h := filepath.Join(f.root, "occ", "harness")
	for _, id := range []string{"wait-one", "wait-two"} {
		f.transcript = append(f.transcript, map[string]any{"type": "message", "message": map[string]any{"role": "assistant", "content": []map[string]any{{"type": "toolCall", "name": "process", "id": id, "arguments": map[string]any{"action": "wait", "sessionId": "job-one"}}}}})
	}
	f.transcript = append(f.transcript, map[string]any{"type": "message", "message": map[string]any{"role": "toolResult", "toolName": "exec", "toolCallId": "tool-one", "content": []map[string]any{{"type": "text", "text": "SECRET_ACCEPTED_TEXT"}}, "details": map[string]any{"status": "accepted", "protocol": "terminal-result-v1", "sessionId": "job-one"}}})
	f.write(t)
	var tools []map[string]any
	for _, id := range []string{"tool-one", "tool-two", "wait-one", "wait-two"} {
		name := "exec"
		if strings.HasPrefix(id, "wait-") {
			name = "process"
		}
		for _, phase := range []string{"start", "result"} {
			tools = append(tools, map[string]any{"run_id": "native-run", "session_key": "session-key", "stream": "tool", "phase": phase, "tool_call_id": id, "name": name, "native_sequence": len(tools) + 1, "host_observed_offset_ns": len(tools) * 100})
		}
	}
	writeTraceEvents(t, filepath.Join(h, "native-tool-events.jsonl"), tools)
	writeSummaryJSON(t, h, "native-tool-events.index.json", map[string]any{"schema_version": 1, "run_id": "native-run", "session_key": "session-key", "complete": true, "terminal_observed": true, "events": len(tools), "clock": map[string]any{"emitter_id": "emitter", "clock_domain": "clock", "started_at_utc": summaryAnchor}, "host_origin_offset_ns": 0})
	writeSummaryJSON(t, h, "agent-result.json", map[string]any{"run_id": "gateway-run", "requested_native_session_id": "native-session", "native_run_id": "native-run", "session_key": "session-key"})
	var rows []map[string]any
	for i, event := range []string{"accepted", "wait_started", "terminal_result", "wait_finished", "wait_started", "wait_finished"} {
		row := map[string]any{"schema_version": 1, "protocol": "terminal-result-v1", "sequence": i + 1, "event": event,
			"identity": f.producer[1], "logical_handle": "job-one", "native_session_id": "real-session", "native_pid": 123,
			"emitter_id": "producer", "emitter_pid": 99, "clock_id": "native-clock", "clock_domain": "native_replay_tool_monotonic",
			"clock_origin_monotonic_ns": "1000", "observed_monotonic_ns": strconv.Itoa(1010 + i*10), "previous_emit_failed": false}
		if strings.HasPrefix(event, "wait_") {
			row["wait_tool_call_id"], row["wait_invocation_id"] = "wait-one", "wait-inv-one"
			if i >= 4 {
				row["wait_tool_call_id"], row["wait_invocation_id"] = "wait-two", "wait-inv-two"
			}
			if event == "wait_finished" {
				row["outcome"] = "returned"
			}
		}
		if event == "terminal_result" {
			row["status"], row["exit_code"], row["output_renderer"] = "completed", 7, "stdout-stderr-v1"
		}
		rows = append(rows, row)
	}
	writeReplayProtocolFixture(t, f, rows)
	return f, rows
}

func writeReplayProtocolFixture(t *testing.T, f *identitySnapshotFixture, rows []map[string]any) {
	t.Helper()
	h := filepath.Join(f.root, "occ", "harness")
	file := filepath.Join(h, "replay-tool-protocol.jsonl")
	writeTraceEvents(t, file, rows)
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	writeSummaryJSON(t, h, "replay-tool-protocol.index.json", map[string]any{"schema_version": 1, "protocol": "terminal-result-v1", "file_integrity": "retained", "sha256": hex.EncodeToString(digest[:])})
}

func replayCoverage(t *testing.T, snapshot AnalysisSnapshot) SourceCoverage {
	t.Helper()
	for _, coverage := range snapshot.Coverage {
		if coverage.Source == "native_replay_tool_protocol" {
			return coverage
		}
	}
	t.Fatal("protocol coverage missing")
	return SourceCoverage{}
}

func TestReplayProtocolSnapshotOwnWaitIdentityAndClock(t *testing.T) {
	f, _ := replayProtocolFixture(t)
	s, err := BuildSnapshot(f.directory)
	if err != nil {
		t.Fatal(err, s.Diagnostics)
	}
	c := replayCoverage(t, s)
	if c.Integrity != "complete" || c.Status != "unknown" || c.Started != 2 || c.Paired != 2 || c.ExpectedEvents != nil || c.ExpectedOperations != nil || len(c.Missing) != 3 {
		t.Fatalf("coverage=%+v", c)
	}
	var native []AnalysisInterval
	for _, interval := range s.Intervals {
		if interval.Category != "native_replay_tool" {
			continue
		}
		native = append(native, interval)
		if interval.ClockDomain == s.Summary.ClockDomain {
			t.Fatal("invented controller clock alignment")
		}
		if interval.Name == "replay.process.wait" && (!strings.HasPrefix(interval.ToolCallID, "wait-") || interval.Attributes["exec_tool_call_id"] != "tool-one") {
			t.Fatal("wait used exec identity")
		}
	}
	if len(native) != 3 || native[0].DurationNS != 20 || native[1].DurationNS != 20 || native[2].DurationNS != 10 {
		t.Fatalf("native intervals=%+v", native)
	}
	for _, clock := range s.Clocks {
		if clock.ID == native[0].ClockDomain && (clock.OriginUTC != nil || clock.Alignment != "unaligned_native_monotonic") {
			t.Fatal("invented UTC")
		}
	}
	trace, err := ProjectTraceDomain(s, native[0].ClockDomain)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range trace.Events {
		if event.Phase == "X" {
			count++
		}
	}
	if count != 3 {
		t.Fatal("native clock view omitted measured intervals")
	}
	otlp, err := ProjectOTLP(s)
	if err != nil {
		t.Fatal(err)
	}
	excluded := 0
	for _, interval := range otlp.Manifest.ExcludedIntervals {
		if interval.ClockDomain == native[0].ClockDomain {
			excluded++
		}
	}
	if excluded != 3 {
		t.Fatal("OTLP synthesized unanchored spans")
	}
	links := 0
	for _, resource := range otlp.Request.ResourceSpans {
		for _, scope := range resource.ScopeSpans {
			for _, span := range scope.Spans {
				links += len(span.Links)
			}
		}
	}
	if links != 2 {
		t.Fatalf("controller exec/wait links=%d", links)
	}
	second, err := BuildSnapshot(f.directory)
	if err != nil || !reflect.DeepEqual(s, second) {
		t.Fatal("unstable snapshot")
	}
	file := filepath.Join(t.TempDir(), "snapshot.json")
	encoded, _ := json.Marshal(s)
	if err := os.WriteFile(file, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSnapshot(file)
	if err != nil || loaded.SnapshotID != s.SnapshotID {
		t.Fatal("frozen snapshot did not retain identity", err)
	}
	if strings.Contains(string(encoded), "SECRET_") {
		t.Fatal("private ToolResult entered projection")
	}
	beforeIdentity := s.NativeToolIdentities
	for _, name := range []string{"replay-tool-protocol.jsonl", "replay-tool-protocol.index.json"} {
		if err := os.Remove(filepath.Join(f.root, "occ", "harness", name)); err != nil {
			t.Fatal(err)
		}
	}
	legacy, err := BuildSnapshot(f.directory)
	if err != nil || !reflect.DeepEqual(beforeIdentity, legacy.NativeToolIdentities) {
		t.Fatal("exec SSH identity changed")
	}
	for _, point := range legacy.Events {
		if point.Category == "native_replay_tool" {
			t.Fatal("absent protocol synthesized events")
		}
	}
}

func TestReplayProtocolSnapshotRejectsForeignAmbiguousOrCorruptEvidence(t *testing.T) {
	for _, kind := range []string{"foreign-exec", "wrong-wait-call", "wrong-handle", "wrong-session", "duplicate-wait", "missing-invocation", "sequence-gap", "clock-regression", "clock-restart", "wrong-terminal-order", "sha-mismatch"} {
		t.Run(kind, func(t *testing.T) {
			f, rows := replayProtocolFixture(t)
			switch kind {
			case "foreign-exec":
				rows[0]["identity"] = map[string]any{"schema_version": 1, "scope_id": "foreign", "agent_session_id": "session-key", "tool_call_id": "tool-one", "native_invocation_id": "inv-b"}
			case "wrong-wait-call":
				rows[1]["wait_tool_call_id"] = "tool-one"
			case "wrong-handle":
				rows[1]["logical_handle"] = "wrong-job"
			case "wrong-session":
				rows[1]["native_session_id"] = "other-process"
			case "duplicate-wait":
				rows[4]["wait_invocation_id"] = "wait-inv-one"
			case "missing-invocation":
				delete(rows[1], "wait_invocation_id")
			case "sequence-gap":
				rows[2]["sequence"] = 99
			case "clock-regression":
				rows[2]["observed_monotonic_ns"] = "1009"
			case "clock-restart":
				rows[2]["clock_id"] = "other-clock"
			case "wrong-terminal-order":
				rows[2], rows[3] = rows[3], rows[2]
				for i, row := range rows {
					row["sequence"], row["observed_monotonic_ns"] = i+1, strconv.Itoa(1010+i*10)
				}
			}
			writeReplayProtocolFixture(t, f, rows)
			if kind == "sha-mismatch" {
				if err := os.WriteFile(filepath.Join(f.root, "occ", "harness", "replay-tool-protocol.jsonl"), []byte("{}\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			s, err := BuildSnapshot(f.directory)
			if err == nil || replayCoverage(t, s).Integrity != "incomplete" {
				t.Fatal("invalid evidence accepted")
			}
			for _, interval := range s.Intervals {
				if interval.Category == "native_replay_tool" {
					t.Fatal("invalid source projected intervals")
				}
			}
		})
	}
}

func TestReplayProtocolSnapshotRepeatedWaitCallKeepsInvocationPairs(t *testing.T) {
	f, rows := replayProtocolFixture(t)
	for _, original := range []map[string]any{rows[1], rows[3]} {
		row := map[string]any{}
		for key, value := range original {
			row[key] = value
		}
		row["wait_invocation_id"] = "wait-inv-three"
		row["sequence"], row["observed_monotonic_ns"] = len(rows)+1, strconv.Itoa(1010+len(rows)*10)
		rows = append(rows, row)
	}
	writeReplayProtocolFixture(t, f, rows)
	s, err := BuildSnapshot(f.directory)
	if err != nil {
		t.Fatal(err, s.Diagnostics)
	}
	c := replayCoverage(t, s)
	if c.Started != 3 || c.Paired != 3 || c.Status != "unknown" {
		t.Fatalf("wait invocations collapsed: %+v", c)
	}
	var repeated []AnalysisInterval
	for _, interval := range s.Intervals {
		if interval.Name == "replay.process.wait" && interval.ToolCallID == "wait-one" {
			repeated = append(repeated, interval)
		}
	}
	if len(repeated) != 2 || repeated[0].ID == repeated[1].ID {
		t.Fatal("wait caller ID replaced invocation ID")
	}
}

func TestReplayProtocolSnapshotIncompleteLifecycleKeepsOnlyRealBoundaries(t *testing.T) {
	for _, kind := range []string{"missing-terminal", "missing-wait-end", "missing-whole-wait", "emission-failed", "interrupted", "terminal-error"} {
		t.Run(kind, func(t *testing.T) {
			f, rows := replayProtocolFixture(t)
			complete := false
			switch kind {
			case "missing-terminal":
				rows = rows[:2]
			case "missing-wait-end":
				rows = rows[:5]
			case "missing-whole-wait":
				rows = rows[:4]
			case "emission-failed":
				rows[len(rows)-1]["previous_emit_failed"] = true
			case "interrupted":
				rows[3]["outcome"] = "interrupted"
				complete = true
			case "terminal-error":
				rows[2]["event"] = "terminal_error"
				delete(rows[2], "status")
				rows[3]["outcome"], rows[5]["outcome"] = "interrupted", "interrupted"
				complete = true
			}
			writeReplayProtocolFixture(t, f, rows)
			s, err := BuildSnapshot(f.directory)
			if (err == nil) != complete {
				t.Fatalf("complete=%v err=%v diagnostics=%v", complete, err, s.Diagnostics)
			}
			c := replayCoverage(t, s)
			if complete && c.Status != "unknown" || !complete && c.Status != "incomplete" {
				t.Fatalf("coverage=%+v", c)
			}
			if kind == "missing-terminal" {
				found := 0
				for _, point := range s.Events {
					if point.Category == "native_replay_tool" {
						found++
					}
				}
				if found != 2 {
					t.Fatal("real start evidence was dropped")
				}
				for _, interval := range s.Intervals {
					if interval.Category == "native_replay_tool" {
						t.Fatal("missing terminal invented duration")
					}
				}
			}
		})
	}
}

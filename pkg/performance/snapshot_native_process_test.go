package performance

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// These 27 unchanged metadata lines come from the real ECS native factory fixture,
// not a real Agent run. Owner evidence: b6628f73f9fc6629af27fbf653cb400061599e8d7ce7ba49969b4ef31b21f398.
// The native start/result records below are synthetic parser inputs only.
func nativeProcessFixture(t *testing.T) (string, string, []map[string]any) {
	t.Helper()
	root, directory := traceFixture(t)
	data, err := os.ReadFile("testdata/native-process-gateway.txt")
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
		var row map[string]any
		if !bytes.HasPrefix(line, []byte(nativeProcessPrefix)) || json.Unmarshal(line[len(nativeProcessPrefix):], &row) != nil {
			t.Fatal("invalid archived fixture")
		}
		rows = append(rows, row)
	}
	nativeRun, session := rows[0]["native_run_id"].(string), rows[0]["native_session_key"].(string)
	writeSummaryJSON(t, root, "cloud-profile.json", map[string]any{"placement": "embedded", "harness": map[string]any{"native_observation_protocol": "aries-native-process-v1"}})
	writeSummaryJSON(t, root, "run-result.json", map[string]any{"run_id": "run", "attempts": []traceAttempt{{AttemptID: "run/occ", OccurrenceID: "occ", AgentSessionID: session, Status: "failed"}}})
	var native []map[string]any
	for _, tool := range []string{"foreground", "descendant", "cancel"} {
		for _, phase := range []string{"start", "result"} {
			native = append(native, map[string]any{"run_id": nativeRun, "session_key": session, "stream": "tool", "phase": phase, "tool_call_id": tool, "name": "exec", "native_sequence": len(native) + 1, "host_observed_offset_ns": len(native) * 1000, "command": "SECRET_COMMAND"})
		}
	}
	dir := filepath.Join(root, "occ", "harness")
	writeTraceEvents(t, filepath.Join(dir, "native-tool-events.jsonl"), native)
	writeSummaryJSON(t, dir, "native-tool-events.index.json", map[string]any{"schema_version": 1, "run_id": nativeRun, "session_key": session, "complete": true, "terminal_observed": true, "events": len(native)})
	writeSummaryJSON(t, dir, "agent-result.json", map[string]any{"native_run_id": nativeRun, "session_key": session, "requested_native_session_id": rows[0]["native_agent_session_id"]})
	if err := os.WriteFile(filepath.Join(dir, "gateway.log"), data, 0600); err != nil {
		t.Fatal(err)
	}
	return root, directory, rows
}

func writeNativeProcessRows(t *testing.T, root string, rows []map[string]any) {
	t.Helper()
	var data bytes.Buffer
	for _, row := range rows {
		value, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		data.WriteString(nativeProcessPrefix)
		data.Write(value)
		data.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(root, "occ/harness/gateway.log"), data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
}

func nativeProcessCoverage(t *testing.T, snapshot AnalysisSnapshot) SourceCoverage {
	t.Helper()
	for _, coverage := range snapshot.Coverage {
		if coverage.Source == "native_process" {
			return coverage
		}
	}
	t.Fatal("missing native process coverage")
	return SourceCoverage{}
}

func TestSnapshotNativeProcessRealCallbacksKeepSeparateClockAndEOFOrder(t *testing.T) {
	_, directory, _ := nativeProcessFixture(t)
	snapshot, err := BuildSnapshot(directory)
	if err != nil {
		t.Fatalf("%v: %v", err, snapshot.Diagnostics)
	}
	coverage := nativeProcessCoverage(t, snapshot)
	if coverage.Integrity != "complete" || coverage.Status != "unknown" || coverage.Started != 3 || coverage.Paired != 3 || coverage.ObservedEvents != 27 || coverage.ExpectedEvents != nil || coverage.ExpectedOperations != nil || coverage.DroppedEvents != nil {
		t.Fatalf("invented completeness denominator: %+v", coverage)
	}
	lifetimes, drains, consumers := 0, 0, 0
	var domain string
	for _, interval := range snapshot.Intervals {
		if interval.Category == "native_tool" {
			consumers++
		}
		if interval.Category != "native_process" {
			continue
		}
		domain = interval.ClockDomain
		if interval.ClockDomain == snapshot.Summary.ClockDomain || interval.Attributes["all_descendants_completion"] != "unknown" {
			t.Fatal("callback became controller or containment evidence")
		}
		if interval.Attributes["first_output"] != "not_collected" {
			t.Fatal("schema v1 absence misreported as no output")
		}
		switch interval.Name {
		case "process.spawn_to_exit":
			lifetimes++
			if interval.Outcome != "exited" {
				t.Fatal("child exit treated as task success")
			}
			if interval.ToolCallID == "foreground" {
				if code := interval.Attributes["exit_code"].(*int); code == nil || *code != 7 || interval.Attributes["pipe_drain"] != "both_natural_eof_before_or_at_child_exit" {
					t.Fatal("exit code or earlier EOF lost")
				}
			}
		case "process.exit_to_pipe_eof":
			drains++
			if interval.ToolCallID != "descendant" || interval.DurationNS != 313661219 || interval.Outcome != "observed" {
				t.Fatal("recorded callback drain gap changed")
			}
		}
	}
	if lifetimes != 3 || drains != 1 || consumers != 3 || snapshot.Attempts[0].Status != "failed" {
		t.Fatalf("native observations changed business/consumer evidence: %d/%d/%d", lifetimes, drains, consumers)
	}
	for _, clock := range snapshot.Clocks {
		if clock.ID == domain && (clock.OriginUTC != nil || clock.Alignment != "unaligned_native_monotonic") {
			t.Fatal("invented epoch anchor")
		}
	}
	primary, err := ProjectTrace(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, point := range primary.Events {
		if point.Category == "native_process" {
			t.Fatal("unaligned process data in primary clock view")
		}
	}
	view, err := ProjectTraceDomain(snapshot, domain)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, point := range view.Events {
		if point.Category == "native_process" && point.Phase == "X" {
			count++
		}
	}
	if count != 4 {
		t.Fatal("native clock view missing measured intervals")
	}
	encoded, _ := json.Marshal(snapshot)
	if strings.Contains(string(encoded), "SECRET_COMMAND") {
		t.Fatal("private command leaked")
	}
}

// Synthetic v2 callback sequences exercise ordering independently of the unchanged
// archived v1 factory evidence; they are not evidence of a real v2 Agent run.
func nativeProcessFirstOutputFixture(t *testing.T, events []string) (string, string, []map[string]any) {
	t.Helper()
	root, directory, archived := nativeProcessFixture(t)
	base := archived[0]
	observed, err := strconv.ParseInt(base["observed_monotonic_ns"].(string), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	rows := make([]map[string]any, 0, len(events))
	for i, event := range events {
		row := make(map[string]any, len(base)+3)
		for key, value := range base {
			row[key] = value
		}
		row["schema_version"], row["event"], row["sequence"] = 2, event, i+1
		row["observed_monotonic_ns"] = strconv.FormatInt(observed+int64(i)*100, 10)
		if event == "child_exit" || event == "child_close" || event == "wait_settle" {
			row["code"], row["signal"] = 7, nil
		}
		if event == "wait_settle" {
			row["source"] = "child_close"
		}
		rows = append(rows, row)
	}
	writeNativeProcessRows(t, root, rows)
	return root, directory, rows
}

func TestSnapshotNativeProcessFirstOutputUsesEarliestRawCallback(t *testing.T) {
	for _, tc := range []struct {
		name       string
		outputs    []string
		wantStream string
		afterExit  bool
	}{
		{name: "stdout_only", outputs: []string{"stdout_first_nonempty"}, wantStream: "stdout"},
		{name: "stderr_only", outputs: []string{"stderr_first_nonempty"}, wantStream: "stderr"},
		{name: "stdout_then_stderr", outputs: []string{"stdout_first_nonempty", "stderr_first_nonempty"}, wantStream: "stdout"},
		{name: "stderr_then_stdout", outputs: []string{"stderr_first_nonempty", "stdout_first_nonempty"}, wantStream: "stderr"},
		{name: "descendant_after_child_exit", outputs: []string{"stdout_first_nonempty"}, wantStream: "stdout", afterExit: true},
		{name: "no_output"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events := []string{"child_spawn_return"}
			if tc.afterExit {
				events = append(events, "child_exit")
			}
			events = append(events, tc.outputs...)
			events = append(events, "stdout_end", "stderr_end")
			if !tc.afterExit {
				events = append(events, "child_exit")
			}
			events = append(events, "child_close", "wait_settle", "observer_dispose")
			_, directory, _ := nativeProcessFirstOutputFixture(t, events)
			snapshot, err := BuildSnapshot(directory)
			if err != nil {
				t.Fatalf("%v: %v", err, snapshot.Diagnostics)
			}
			points := make(map[string]AnalysisEvent)
			for _, point := range snapshot.Events {
				if point.Category == "native_process" {
					points[point.Name] = point
				}
			}
			lifetimes, outputs := 0, 0
			for _, interval := range snapshot.Intervals {
				if interval.Category != "native_process" {
					continue
				}
				if interval.Name == "process.spawn_to_exit" {
					lifetimes++
					if tc.wantStream == "" && interval.Attributes["first_output"] != "no_nonempty_chunk_observed" {
						t.Fatal("observed no-output pipes confused with uncollected v1")
					}
				}
				if interval.Name != "process.spawn_to_first_output" {
					continue
				}
				outputs++
				first, start := points[tc.wantStream+"_first_nonempty"], points["child_spawn_return"]
				if interval.StartEventID != start.ID || interval.EndEventID != first.ID || interval.DurationNS != first.OffsetNS-start.OffsetNS || interval.DurationNS <= 0 || interval.ClockDomain != first.ClockDomain || interval.ClockDomain == snapshot.Summary.ClockDomain || interval.Attributes["first_output_stream"] != tc.wantStream || interval.Attributes["first_output"] != "observed" || interval.Outcome != "observed" {
					t.Fatalf("first output lost raw identity, clock or boundaries: %+v", interval)
				}
				if tc.afterExit && first.OffsetNS <= points["child_exit"].OffsetNS {
					t.Fatal("descendant output moved before direct-child exit")
				}
			}
			wantOutputs := 1
			if tc.wantStream == "" {
				wantOutputs = 0
			}
			if lifetimes != 1 || outputs != wantOutputs {
				t.Fatalf("missing or invented intervals: lifetime=%d output=%d", lifetimes, outputs)
			}
			for _, name := range tc.outputs {
				if points[name].ID == "" || points[name].ToolCallID != "foreground" || points[name].SourceLine == 0 {
					t.Fatal("first-output instant lost its source or exact tool identity")
				}
			}
		})
	}
}

func TestSnapshotNativeProcessRejectsInvalidFirstOutput(t *testing.T) {
	for _, mode := range []string{"after_stdout_eof", "after_stderr_eof", "duplicate", "schema_v1", "mixed_schema"} {
		t.Run(mode, func(t *testing.T) {
			events := []string{"child_spawn_return", "stdout_first_nonempty", "stdout_end", "stderr_end", "child_exit", "child_close", "wait_settle"}
			switch mode {
			case "after_stdout_eof":
				events[1], events[2] = events[2], events[1]
			case "after_stderr_eof":
				events[1], events[2], events[3] = "stderr_end", "stderr_first_nonempty", "stdout_end"
			case "duplicate":
				events = append(events[:2], append([]string{"stdout_first_nonempty"}, events[2:]...)...)
			}
			root, directory, rows := nativeProcessFirstOutputFixture(t, events)
			if mode == "schema_v1" {
				for _, row := range rows {
					row["schema_version"] = 1
				}
			} else if mode == "mixed_schema" {
				rows[len(rows)-1]["schema_version"] = 1
			}
			writeNativeProcessRows(t, root, rows)
			snapshot, err := BuildSnapshot(directory)
			if err == nil || nativeProcessCoverage(t, snapshot).Integrity != "incomplete" {
				t.Fatal("invalid first-output source accepted")
			}
			for _, point := range snapshot.Events {
				if point.Category == "native_process" {
					t.Fatal("invalid first-output source projected")
				}
			}
		})
	}
}

func TestSnapshotNativeProcessMissingEvidenceIsNotNoOutput(t *testing.T) {
	for _, mode := range []string{"missing_eof", "emit_failed"} {
		t.Run(mode, func(t *testing.T) {
			events := []string{"child_spawn_return", "stdout_end", "stderr_end", "child_exit", "child_close", "wait_settle"}
			if mode == "missing_eof" {
				events = append(events[:2], events[3:]...)
			}
			root, directory, rows := nativeProcessFirstOutputFixture(t, events)
			if mode == "emit_failed" {
				rows[len(rows)-1]["previous_emit_failed"] = true
				writeNativeProcessRows(t, root, rows)
			}
			snapshot, err := BuildSnapshot(directory)
			if err == nil {
				t.Fatal("incomplete callback source accepted")
			}
			for _, interval := range snapshot.Intervals {
				if interval.Category == "native_process" && (interval.Name != "process.spawn_to_exit" || interval.Attributes["first_output"] != "unknown") {
					t.Fatal("missing output evidence became a duration or confirmed no-output result")
				}
			}
		})
	}
}

func TestSnapshotNativeProcessRejectsUntrustedOrBrokenRecords(t *testing.T) {
	for _, mode := range []string{"sequence_gap", "sequence_duplicate", "clock_backwards", "foreign_tool", "foreign_run", "foreign_session", "wrong_agent_session", "pid_changed", "untrusted_native", "wrong_prefix"} {
		t.Run(mode, func(t *testing.T) {
			root, directory, rows := nativeProcessFixture(t)
			switch mode {
			case "sequence_gap":
				rows[1]["sequence"] = 3
			case "sequence_duplicate":
				rows[1]["sequence"] = 1
			case "clock_backwards":
				rows[1]["observed_monotonic_ns"] = rows[0]["clock_origin_monotonic_ns"]
			case "foreign_tool":
				rows[0]["tool_call_id"] = "foreign"
			case "foreign_run":
				rows[0]["native_run_id"] = "foreign"
			case "foreign_session":
				rows[0]["native_session_key"] = "foreign"
			case "wrong_agent_session":
				rows[0]["native_agent_session_id"] = "foreign-agent-session"
			case "pid_changed":
				rows[1]["pid"] = 999
			case "untrusted_native":
				if err := os.Remove(filepath.Join(root, "occ/harness/native-tool-events.index.json")); err != nil {
					t.Fatal(err)
				}
			}
			writeNativeProcessRows(t, root, rows)
			if mode == "wrong_prefix" {
				path := filepath.Join(root, "occ/harness/gateway.log")
				data, _ := os.ReadFile(path)
				if err := os.WriteFile(path, bytes.Replace(data, []byte("[aries/native-process]"), []byte("[tools]"), 1), 0600); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, err := BuildSnapshot(directory)
			if err == nil || nativeProcessCoverage(t, snapshot).Integrity != "incomplete" {
				t.Fatal("broken source accepted")
			}
			for _, point := range snapshot.Events {
				if point.Category == "native_process" {
					t.Fatal("untrusted source projected")
				}
			}
		})
	}
}

func TestSnapshotNativeProcessDoesNotHideUnexpectedSSHIdentityEvidence(t *testing.T) {
	root, directory, _ := nativeProcessFixture(t)
	if err := os.WriteFile(filepath.Join(root, "occ/harness/native-tool-identities.jsonl"), []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := BuildSnapshot(directory)
	if err == nil || snapshot.NativeToolIdentities == nil || len(snapshot.NativeToolIdentities.Attempts) != 1 {
		t.Fatal("unexpected external identity evidence ignored")
	}
	identity := snapshot.NativeToolIdentities.Attempts[0]
	if identity.Status != "incomplete" || !reflect.DeepEqual(identity.Diagnostics, []string{"identity_opt_in_or_placement_invalid"}) {
		t.Fatalf("unexpected identity scope accepted: %+v", identity)
	}
	if nativeProcessCoverage(t, snapshot).Integrity != "complete" {
		t.Fatal("external identity fault changed valid process callbacks")
	}
}

func TestSnapshotNativeProcessMissingExitAndCloseDoNotInventTermination(t *testing.T) {
	for _, mode := range []string{"no_exit", "fallback_close_only", "missing_stdout_end"} {
		t.Run(mode, func(t *testing.T) {
			root, directory, rows := nativeProcessFixture(t)
			kept := []map[string]any{}
			for _, row := range rows[:9] {
				event := row["event"]
				if mode != "missing_stdout_end" && event == "child_exit" || mode == "missing_stdout_end" && event == "stdout_end" || mode == "fallback_close_only" && (event == "stdout_end" || event == "stderr_end") {
					continue
				}
				if mode == "fallback_close_only" && event == "wait_settle" {
					row["source"], row["code"], row["signal"] = "force_kill_fallback", nil, "SIGKILL"
				}
				row["sequence"] = len(kept) + 1 // Model an un-emitted callback, not a lost log line.
				kept = append(kept, row)
			}
			writeNativeProcessRows(t, root, kept)
			snapshot, err := BuildSnapshot(directory)
			if err == nil || nativeProcessCoverage(t, snapshot).Status != "incomplete" {
				t.Fatal("missing lifecycle boundary accepted")
			}
			lifetimes := 0
			for _, interval := range snapshot.Intervals {
				if interval.Category != "native_process" {
					continue
				}
				if mode != "missing_stdout_end" || interval.Name != "process.spawn_to_exit" {
					t.Fatal("close/fallback used as an exit or EOF")
				}
				lifetimes++
			}
			if mode == "missing_stdout_end" && lifetimes != 1 {
				t.Fatal("independently observed child exit discarded")
			}
		})
	}
}

func TestSnapshotNativeProcessAbsentSourcePreservesLegacyProjection(t *testing.T) {
	root, directory, _ := nativeProcessFixture(t)
	path := filepath.Join(root, "occ/harness/gateway.log")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	before, err := BuildSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("ordinary legacy Gateway log\n"), 0600); err != nil {
		t.Fatal(err)
	}
	after, err := BuildSnapshot(directory)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("unobserved process source changed legacy projection")
	}
}

func TestSnapshotNativeProcessGatewayTimestampIsEnvelopeOnly(t *testing.T) {
	root, directory, _ := nativeProcessFixture(t)
	before, err := BuildSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "occ/harness/gateway.log")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Exact compact prefix observed in the ECS Gateway CLI envelope probe. The
	// archived callback metadata is unchanged; these combined lines are a fixture.
	stamped := bytes.ReplaceAll(data, []byte(nativeProcessPrefix), []byte("2026-10-25T07:32:24.660+00:00 "+nativeProcessPrefix))
	if err := os.WriteFile(path, stamped, 0600); err != nil {
		t.Fatal(err)
	}
	after, err := BuildSnapshot(directory)
	if err != nil {
		t.Fatalf("%v: %v", err, after.Diagnostics)
	}
	if len(before.Clocks) != len(after.Clocks) || !reflect.DeepEqual(before.Coverage, after.Coverage) || len(before.Intervals) != len(after.Intervals) {
		t.Fatal("console timestamp changed clock or completeness")
	}
	for i, clock := range before.Clocks {
		other := after.Clocks[i]
		// Adding an envelope changes the source hash, but cannot create an anchor.
		if clock.ID != other.ID || clock.EmitterID != other.EmitterID || clock.Alignment != other.Alignment || !reflect.DeepEqual(clock.OriginUTC, other.OriginUTC) {
			t.Fatal("console timestamp changed process clock")
		}
	}
	for i, interval := range before.Intervals {
		other := after.Intervals[i]
		if interval.ClockDomain != other.ClockDomain || interval.StartNS != other.StartNS || interval.EndNS != other.EndNS || interval.DurationNS != other.DurationNS {
			t.Fatal("console timestamp changed measured interval")
		}
	}
	for _, line := range []string{
		"2026-99-25T07:32:24.660+00:00 " + nativeProcessPrefix + "{}",
		"2026-10-25T07:32:24.660+00:00 [other] " + nativeProcessMarker + "{}",
		"extra 2026-10-25T07:32:24.660+00:00 " + nativeProcessPrefix + "{}",
	} {
		if _, accepted := nativeProcessPayload([]byte(line)); accepted {
			t.Fatalf("unverified console envelope accepted: %s", line)
		}
	}
}

package performance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func sdkSnapshotFixture(t *testing.T) (string, string, []map[string]any, map[string]any) {
	t.Helper()
	root, directory := traceFixture(t)
	writeSummaryJSON(t, root, "run-result.json", map[string]any{"run_id": "run", "attempts": []traceAttempt{{AttemptID: "run/occ", OccurrenceID: "occ", Status: "failed"}}})
	rows := []map[string]any{}
	for i, input := range []struct {
		id                       int
		event, operation, status string
		offset, duration         int64
	}{
		{1, "start", "exec.wait", "", 10, 0}, {2, "start", "adapter_rpc.stdin", "", 20, 0},
		{2, "end", "adapter_rpc.stdin", "failed", 25, 5}, {1, "end", "exec.wait", "succeeded", 40, 30},
	} {
		row := map[string]any{"schema_version": 1, "sequence": i + 1, "event": input.event, "operation_id": input.id, "operation": input.operation, "run_id": "run", "task_id": "occ", "purpose": "task", "sandbox_handle": "owned-handle", "execution_id": "same-execution", "process_id": "same-process", "observed_offset_ns": input.offset, "started_at_utc": "2035-01-01T00:00:00Z"}
		if input.event == "end" {
			row["status"], row["client_duration_ns"] = input.status, input.duration
		}
		if input.operation == "exec.wait" && input.event == "end" {
			row["exit_code"], row["guest_duration_ns"] = 0, 12
		}
		if input.operation == "adapter_rpc.stdin" && input.event == "end" {
			row["sdk_calls"] = []AnalysisSDKCall{{Operation: "commands.close_stdin", DurationNS: 3, Status: "failed"}}
		}
		rows = append(rows, row)
	}
	index := map[string]any{"schema_version": 1, "status": "complete", "records": 4, "started": 2, "ended": 2, "clock": map[string]any{"emitter_id": "emitter", "clock_domain": "clock", "started_at_utc": summaryAnchor}}
	writeSDKSnapshot(t, root, rows, index)
	return root, directory, rows, index
}

func writeSDKSnapshot(t *testing.T, root string, rows []map[string]any, index map[string]any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "occ"), 0700); err != nil {
		t.Fatal(err)
	}
	writeTraceEvents(t, filepath.Join(root, "occ/e2b-operations.jsonl"), rows)
	writeSummaryJSON(t, filepath.Join(root, "occ"), "e2b-operations-index.json", index)
}

func TestSnapshotSDKClientClockKeepsFailedStdinAndSuccessfulWaitSeparate(t *testing.T) {
	root, directory, rows, index := sdkSnapshotFixture(t)
	snapshot, err := BuildSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	operations := map[string]AnalysisInterval{}
	for _, interval := range snapshot.Intervals {
		if interval.Category == "sandbox_client" {
			operations[interval.Name] = interval
		}
	}
	wait, stdin := operations["exec.wait"], operations["adapter_rpc.stdin"]
	if len(operations) != 2 || wait.Outcome != "succeeded" || wait.StartNS != 10 || wait.EndNS != 40 || wait.DurationNS != 30 || stdin.Outcome != "failed" || stdin.DurationNS != 5 || stdin.StartNS != 20 || stdin.EndNS != 25 {
		t.Fatalf("distinct SDK outcomes lost: %+v", operations)
	}
	if wait.Scope != "controller_process_submission_to_wait_return" || stdin.Scope != "controller_adapter_rpc" || wait.Attributes["guest_duration_ns"].(*int64) == nil || *wait.Attributes["guest_duration_ns"].(*int64) != 12 {
		t.Fatal("client/guest scopes conflated")
	}
	if got := stdin.Attributes["sdk_calls_duration_only"].([]AnalysisSDKCall); len(got) != 1 || got[0].DurationNS != 3 || got[0].Status != "failed" {
		t.Fatal("SDK duration-only outcome lost")
	}
	for _, edge := range snapshot.Edges {
		if edge.From == wait.ID || edge.To == wait.ID || edge.From == stdin.ID || edge.To == stdin.ID {
			t.Fatal("invented operation parent or flow")
		}
	}
	trace, err := ProjectTrace(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, event := range trace.Events {
		if event.Category == "sandbox_client" && event.Phase == "X" {
			found++
			if event.Args["status"] != operations[event.Name].Outcome {
				t.Fatal("trace changed SDK outcome")
			}
		}
	}
	if found != 2 {
		t.Fatal("SDK intervals absent from trace")
	}
	// Clockless archived rows keep outcomes and durations but gain no origin.
	delete(index, "clock")
	for _, row := range rows {
		delete(row, "observed_offset_ns")
	}
	writeSDKSnapshot(t, root, rows, index)
	legacy, err := BuildSnapshot(directory)
	if err != nil || legacy.IntegrityStatus != "complete" || legacy.CoverageStatus != "incomplete" || len(legacy.UntimedOperations) != 2 {
		t.Fatalf("legacy duration-only evidence missing: %+v %v", legacy.UntimedOperations, err)
	}
	for _, interval := range legacy.Intervals {
		if interval.Category == "sandbox_client" {
			t.Fatal("legacy UTC was used to invent a timeline")
		}
	}
	if legacy.UntimedOperations[0].Outcome != "failed" || *legacy.UntimedOperations[0].DurationNS != 5 || legacy.UntimedOperations[1].Outcome != "succeeded" {
		t.Fatal("legacy outcomes changed")
	}
	// One raw operation source, reused by both timeline and identity analysis.
	sources := 0
	for _, source := range snapshot.Sources {
		if source.File == "occ/e2b-operations.jsonl" {
			sources++
		}
	}
	if sources != 1 {
		t.Fatal("operation source parsed twice")
	}
}

func TestSnapshotSDKRejectsWrongClockMissingBoundaryAndDurationMismatch(t *testing.T) {
	for _, mode := range []string{"foreign-clock", "duration-mismatch", "missing-end-offset", "missing-end", "duplicate-end", "clockless-offset"} {
		t.Run(mode, func(t *testing.T) {
			root, directory, rows, index := sdkSnapshotFixture(t)
			switch mode {
			case "foreign-clock":
				index["clock"].(map[string]any)["clock_domain"] = "different"
			case "duration-mismatch":
				rows[3]["client_duration_ns"] = 31
			case "missing-end-offset":
				delete(rows[3], "observed_offset_ns")
			case "missing-end":
				rows = rows[:3]
				index["records"] = 3
			case "duplicate-end":
				rows[3]["operation_id"] = 2
			case "clockless-offset":
				delete(index, "clock")
			}
			writeSDKSnapshot(t, root, rows, index)
			snapshot, err := BuildSnapshot(directory)
			if err == nil || snapshot.IntegrityStatus != "incomplete" {
				t.Fatal("invalid source became complete")
			}
			for _, interval := range snapshot.Intervals {
				if interval.Category == "sandbox_client" {
					t.Fatal("invalid client interval invented")
				}
			}
		})
	}
}

func TestSnapshotSDKIdentityAndTimingReuseSameRawSource(t *testing.T) {
	f := newIdentitySnapshotFixture(t)
	before, group := identityGroup(t, f)
	index := f.operationIndex
	index["clock"] = map[string]any{"emitter_id": "emitter", "clock_domain": "clock", "started_at_utc": summaryAnchor}
	for i, row := range f.operations {
		row["observed_offset_ns"] = int64(i + 1)
		if row["event"] == "end" {
			row["client_duration_ns"], row["status"] = int64(1), "succeeded"
		}
	}
	f.write(t)
	after, next := identityGroup(t, f)
	if next.Status != group.Status || next.CoveredExecCalls != group.CoveredExecCalls || len(after.Intervals) != len(before.Intervals)+6 {
		t.Fatalf("identity/timing were conflated: %+v", next)
	}
	counts := map[string]int{}
	for _, source := range after.Sources {
		counts[source.File]++
	}
	if counts["occ/e2b-operations.jsonl"] != 1 || counts["occ/e2b-operations-index.json"] != 1 {
		t.Fatal("SDK and identity reread sources")
	}
	first, _ := json.Marshal(after)
	again, _ := BuildSnapshot(f.directory)
	second, _ := json.Marshal(again)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("snapshot is not deterministic")
	}
}

package performance

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var summaryAnchor = time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)

func summaryPair(id, task, purpose, status string, duration int64) []Event {
	started := summaryAnchor.Add(10 * time.Nanosecond)
	start := Event{Identity: Identity{LogicalTaskID: task, Purpose: purpose}, EventType: "operation_started", Operation: "sandbox.start", OperationID: id, ObservedOffsetNS: 10, StartedAtUTC: &started}
	end := start
	end.EventType, end.Status, end.DurationNS, end.ObservedOffsetNS = "operation_finished", status, &duration, 10+duration
	return []Event{start, end}
}

func summaryArrival(id, task, status string) []Event {
	planned, anchor := int64(20), int64(5)
	base := Event{Identity: Identity{LogicalTaskID: task, OccurrenceID: "occ-" + id, ArrivalID: id, Purpose: "task"}, Operation: "arrival", PlannedOffsetNS: &planned, ScheduleAnchorOffsetNS: &anchor, Status: "succeeded"}
	stages := []string{"arrival_planned", "arrival_admitted", "arrival_dispatched", "arrival_finished"}
	if status == "not_started" {
		stages = []string{"arrival_planned", "arrival_not_started"}
	}
	var events []Event
	for i, stage := range stages {
		event := base
		event.EventType, event.ObservedOffsetNS = stage, int64(30+i*10)
		switch stage {
		case "arrival_admitted":
			duration := int64(15)
			event.DurationNS = &duration
		case "arrival_finished":
			duration := int64(10)
			event.Status, event.DurationNS = status, &duration
		case "arrival_not_started":
			event.Status, event.Reason = status, "context_canceled"
		}
		events = append(events, event)
	}
	return events
}

func summaryRecording(t *testing.T, events []Event) string {
	t.Helper()
	directory := t.TempDir()
	manifest := Manifest{SchemaVersion: SchemaVersion, RunID: "run", EmitterID: "emitter", ClockDomain: "clock", StartedAtUTC: summaryAnchor, EventsFile: "runner-events.jsonl", Scope: "scheduler_and_runner"}
	writeSummaryJSON(t, directory, "manifest.json", manifest)
	index := Index{SchemaVersion: SchemaVersion, RunID: "run", EmitterID: "emitter", Status: "complete", Attempted: uint64(len(events)), Written: uint64(len(events))}
	writeSummaryJSON(t, directory, "index.json", index)
	file, err := os.Create(filepath.Join(directory, "runner-events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	encoder := json.NewEncoder(file)
	for i, event := range events {
		event.SchemaVersion, event.RunID, event.EmitterID, event.ClockDomain = SchemaVersion, "run", "emitter", "clock"
		if event.Sequence == 0 {
			event.Sequence = uint64(i + 1)
		}
		event.ObservedAtUTC = summaryAnchor.Add(time.Duration(event.ObservedOffsetNS))
		if err := encoder.Encode(event); err != nil {
			t.Fatal(err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return directory
}

func writeSummaryJSON(t *testing.T, directory, name string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, name), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestAnalyzeSeparatesTaskPurposeAndFailureDurations(t *testing.T) {
	var events []Event
	for i, duration := range []int64{0, 10, 20, 30} {
		events = append(events, summaryPair(string(rune('a'+i)), "task-a", "task", "succeeded", duration)...)
	}
	for _, status := range []string{"failed", "canceled", "timeout"} {
		events = append(events, summaryPair(status, "task-a", "task", status, 9999)...)
	}
	events = append(events, summaryPair("eval", "task-a", "evaluation", "succeeded", 100)...)
	events = append(events, summaryPair("other-task", "task-b", "task", "succeeded", 200)...)
	summary, err := Analyze(summaryRecording(t, events))
	if err != nil || summary.Status != "complete" || len(summary.Operations) != 3 {
		t.Fatalf("Analyze = %+v, %v", summary, err)
	}
	group := summary.Operations[1]
	if group.LogicalTaskID != "task-a" || group.Purpose != "task" || group.Started != 7 || group.Succeeded != 4 || group.Failed != 1 || group.Canceled != 1 || group.Timeout != 1 || group.Incomplete != 0 {
		t.Fatalf("wrong groups/counts: %+v", summary.Operations)
	}
	stats := group.SuccessfulDurationNS
	if stats.Samples != 4 || *stats.MinNS != 0 || *stats.P50NS != 10 || *stats.P95NS != 30 || *stats.MaxNS != 30 || stats.P99NS != nil {
		t.Fatalf("wrong success-only statistics: %+v", stats)
	}
}

func TestAnalyzeP99Threshold(t *testing.T) {
	for _, count := range []int{999, 1000} {
		values := make([]int64, count)
		for i := range values {
			values[i] = int64(i + 1)
		}
		stats := durationStats(values)
		if count < 1000 && stats.P99NS != nil || count == 1000 && (stats.P99NS == nil || *stats.P99NS != 990) {
			t.Fatalf("count=%d: %+v", count, stats)
		}
	}
}

func TestAnalyzeEmptyRecordingAndMissingCounts(t *testing.T) {
	directory := summaryRecording(t, nil)
	summary, err := Analyze(directory)
	if err != nil || summary.EventCount != 0 || len(summary.Operations) != 0 || len(summary.Arrivals) != 0 {
		t.Fatalf("empty finalized recording: %+v, %v", summary, err)
	}
	writeSummaryJSON(t, directory, "index.json", map[string]any{"schema_version": SchemaVersion, "run_id": "run", "emitter_id": "emitter", "status": "complete"})
	if summary, err := Analyze(directory); err == nil || summary.Status != "incomplete" {
		t.Fatalf("missing counts accepted as zero: %+v, %v", summary, err)
	}
}

func TestAnalyzeNoSuccessfulSamples(t *testing.T) {
	summary, err := Analyze(summaryRecording(t, summaryPair("timeout", "task", "task", "timeout", 1000)))
	if err != nil {
		t.Fatal(err)
	}
	group := summary.Operations[0]
	stats := group.SuccessfulDurationNS
	if group.Timeout != 1 || stats.Samples != 0 || stats.MinNS != nil || stats.P50NS != nil || stats.P95NS != nil || stats.P99NS != nil || stats.MaxNS != nil {
		t.Fatalf("missing statistics were fabricated: %+v", group)
	}
}

func TestAnalyzeRejectsDamagedOperations(t *testing.T) {
	tests := map[string]func([]Event) []Event{
		"missing terminal": func(events []Event) []Event { return events[:1] },
		"missing start":    func(events []Event) []Event { return events[1:] },
		"duplicate terminal": func(events []Event) []Event {
			return append(events, events[1])
		},
		"duplicate sequence terminal": func(events []Event) []Event {
			events[1].Sequence = 2
			return append(events, events[1])
		},
		"negative duration": func(events []Event) []Event {
			*events[1].DurationNS = -1
			return events
		},
		"duration inconsistent": func(events []Event) []Event {
			*events[1].DurationNS = 200
			return events
		},
		"unknown status": func(events []Event) []Event {
			events[1].Status = "done"
			return events
		},
		"wrong metadata": func(events []Event) []Event {
			events[1].LogicalTaskID = "other"
			return events
		},
		"terminal before start": func(events []Event) []Event {
			return []Event{events[1], events[0]}
		},
		"absent duration": func(events []Event) []Event {
			events[1].DurationNS = nil
			return events
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			events := mutate(summaryPair("op", "task", "task", "succeeded", 100))
			summary, err := Analyze(summaryRecording(t, events))
			if err == nil || summary.Status != "incomplete" || len(summary.Problems) == 0 || len(summary.Operations) != 1 || summary.Operations[0].Incomplete != 1 || summary.Operations[0].Succeeded != 0 || summary.Operations[0].SuccessfulDurationNS.Samples != 0 {
				t.Fatalf("bad recording accepted: %+v, %v", summary, err)
			}
		})
	}
}

func TestAnalyzeArrivalsAndQueue(t *testing.T) {
	var events []Event
	events = append(events, summaryArrival("a", "task-a", "succeeded")...)
	events = append(events, summaryArrival("b", "task-a", "failed")...)
	events = append(events, summaryArrival("c", "task-a", "not_started")...)
	events = append(events, summaryArrival("d", "task-b", "timeout")...)
	summary, err := Analyze(summaryRecording(t, events))
	if err != nil || len(summary.Arrivals) != 2 {
		t.Fatalf("Analyze = %+v, %v", summary, err)
	}
	a := summary.Arrivals[0]
	if a.Planned != 3 || a.Admitted != 2 || a.Dispatched != 2 || a.Finished != 2 || a.NotStarted != 1 || a.Succeeded != 1 || a.Failed != 1 || a.Incomplete != 0 || a.AdmissionQueueNS.Samples != 2 || *a.AdmissionQueueNS.P50NS != 15 {
		t.Fatalf("arrivals do not reconcile: %+v", a)
	}
}

func TestAnalyzeRejectsDamagedArrivals(t *testing.T) {
	tests := map[string]func([]Event) []Event{
		"missing plan":     func(e []Event) []Event { return e[1:] },
		"missing dispatch": func(e []Event) []Event { return append(e[:2], e[3]) },
		"unfinished":       func(e []Event) []Event { return e[:3] },
		"duplicate finish": func(e []Event) []Event { return append(e, e[3]) },
		"both terminals": func(e []Event) []Event {
			return append(e, summaryArrival("a", "task", "not_started")[1])
		},
		"identity changed": func(e []Event) []Event { e[1].OccurrenceID = "other"; return e },
		"negative queue":   func(e []Event) []Event { *e[1].DurationNS = -1; return e },
		"missing offset":   func(e []Event) []Event { e[0].PlannedOffsetNS = nil; return e },
		"time regressed":   func(e []Event) []Event { e[2].ObservedOffsetNS = 1; return e },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			summary, err := Analyze(summaryRecording(t, mutate(summaryArrival("a", "task", "succeeded"))))
			if err == nil || summary.Status != "incomplete" || len(summary.Arrivals) != 1 || summary.Arrivals[0].Incomplete != 1 || summary.Arrivals[0].Succeeded != 0 || summary.Arrivals[0].AdmissionQueueNS.Samples != 0 {
				t.Fatalf("bad arrival accepted: %+v, %v", summary, err)
			}
		})
	}
}

func TestAnalyzePartialFilesAndIdentities(t *testing.T) {
	for _, damage := range []string{"missing index", "truncated JSON", "missing newline", "wrong count", "dropped", "run", "emitter", "clock", "schema", "sequence gap", "missing offset"} {
		t.Run(damage, func(t *testing.T) {
			directory := summaryRecording(t, summaryPair("op", "task", "task", "succeeded", 100))
			path := filepath.Join(directory, "runner-events.jsonl")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			switch damage {
			case "missing index":
				err = os.Remove(filepath.Join(directory, "index.json"))
			case "truncated JSON":
				err = os.WriteFile(path, data[:len(data)-20], 0o600)
			case "missing newline":
				err = os.WriteFile(path, data[:len(data)-1], 0o600)
			case "wrong count", "dropped":
				index := Index{SchemaVersion: SchemaVersion, RunID: "run", EmitterID: "emitter", Status: "complete", Attempted: 3, Written: 3}
				if damage == "dropped" {
					index.Written, index.Dropped, index.Status = 2, 1, "incomplete"
				}
				writeSummaryJSON(t, directory, "index.json", index)
			default:
				old, replacement := `"run_id":"run"`, `"run_id":"other"`
				switch damage {
				case "emitter":
					old, replacement = `"emitter_id":"emitter"`, `"emitter_id":"other"`
				case "clock":
					old, replacement = `"clock_domain":"clock"`, `"clock_domain":"other"`
				case "schema":
					old, replacement = fmt.Sprintf(`"schema_version":%d`, SchemaVersion), fmt.Sprintf(`"schema_version":%d`, SchemaVersion+1)
				case "sequence gap":
					old, replacement = `"sequence":2`, `"sequence":4`
				case "missing offset":
					old, replacement = `"observed_offset_ns":10,`, ``
				}
				err = os.WriteFile(path, []byte(strings.Replace(string(data), old, replacement, 1)), 0o600)
			}
			if err != nil {
				t.Fatal(err)
			}
			summary, err := Analyze(directory)
			if err == nil || summary.Status != "incomplete" || len(summary.Problems) == 0 {
				t.Fatalf("damage accepted: %+v, %v", summary, err)
			}
			if damage == "missing index" && (len(summary.Operations) != 1 || summary.Operations[0].Succeeded != 1) {
				t.Fatal("valid partial result was hidden")
			}
		})
	}
}

func TestAnalyzeUsesOffsetsDespiteWallClockChange(t *testing.T) {
	directory := summaryRecording(t, summaryPair("op", "task", "task", "succeeded", 100))
	path := filepath.Join(directory, "runner-events.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	old := `"observed_at_utc":"2026-10-22T00:00:00.00000011Z"`
	replacement := `"observed_at_utc":"2026-10-21T00:00:00Z"`
	if !strings.Contains(string(data), old) {
		t.Fatalf("fixture does not contain terminal timestamp: %s", data)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(data), old, replacement, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	summary, err := Analyze(directory)
	if err != nil || *summary.Operations[0].SuccessfulDurationNS.P50NS != 100 {
		t.Fatalf("wall clock changed duration: %+v, %v", summary, err)
	}
}

func TestAnalyzeParentTimingAndIdentity(t *testing.T) {
	for _, damage := range []string{"none", "missing", "too late", "ends early", "identity"} {
		t.Run(damage, func(t *testing.T) {
			parent := summaryPair("parent", "task", "task", "succeeded", 100)
			child := summaryPair("child", "task", "evaluation", "succeeded", 10)
			child[0].ParentOperationID, child[1].ParentOperationID = "parent", "parent"
			events := []Event{parent[0], child[0], child[1], parent[1]}
			switch damage {
			case "missing":
				events = child
			case "too late":
				events = []Event{child[0], parent[0], child[1], parent[1]}
			case "ends early":
				events = []Event{parent[0], child[0], parent[1], child[1]}
			case "identity":
				events[0].OccurrenceID, events[3].OccurrenceID = "parent-occurrence", "parent-occurrence"
			}
			summary, err := Analyze(summaryRecording(t, events))
			if damage == "none" && err != nil || damage != "none" && err == nil {
				t.Fatalf("parent validation: %+v, %v", summary, err)
			}
		})
	}
}

func TestAnalyzeActualRecorder(t *testing.T) {
	root := t.TempDir()
	recorder, err := Open(root, "real-run")
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithIdentity(WithRecorder(context.Background(), recorder), Identity{LogicalTaskID: "task", OccurrenceID: "task-1", Purpose: "task"})
	parentCtx, parent := Begin(ctx, "task.e2e")
	_, child := Begin(WithIdentity(parentCtx, Identity{Purpose: "evaluation"}), "sandbox.start")
	child.End(context.DeadlineExceeded)
	parent.End(nil)
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	summary, err := Analyze(filepath.Join(root, "performance"))
	if err != nil || len(summary.Operations) != 2 || summary.Operations[0].Timeout != 1 || summary.Operations[1].Succeeded != 1 {
		t.Fatalf("real recorder incompatible: %+v, %v", summary, err)
	}
}

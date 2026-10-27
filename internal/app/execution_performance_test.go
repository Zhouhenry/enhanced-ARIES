package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/performance"
)

func TestRunProfileRecordsEntireFinitePlanAfterCancellation(t *testing.T) {
	for _, withArrivals := range []bool{false, true} {
		t.Run(fmt.Sprintf("arrivals=%t", withArrivals), func(t *testing.T) {
			ctx, readEvents := performanceRecording(t)
			ctx, cancel := context.WithCancel(ctx)
			cancel()
			var schedule []arrival
			if withArrivals {
				schedule = []arrival{{"a", 0}, {"b", time.Hour}, {"a", 2 * time.Hour}}
			}
			result, err := runProfile(ctx, "name", "run", []string{"a", "b", "a"}, 1, 0, schedule,
				func(context.Context, taskOccurrence) (core.RunResult, error) {
					t.Error("canceled occurrence was dispatched")
					return core.RunResult{}, nil
				})
			if !errors.Is(err, context.Canceled) || len(result.Tasks) != 0 || result.Summary.Tasks != 0 {
				t.Fatalf("result=%+v error=%v", result, err)
			}
			events := readEvents()
			if len(events) != 6 {
				t.Fatalf("events=%+v", events)
			}
			for i, logicalID := range []string{"a", "b", "a"} {
				planned, terminal := events[i], events[i+3]
				var wantOffset int64
				if withArrivals {
					wantOffset = int64(schedule[i].at)
				}
				for _, event := range []performance.Event{planned, terminal} {
					if event.LogicalTaskID != logicalID || event.OccurrenceID != fmt.Sprintf("%s-%03d", logicalID, i+1) || event.ArrivalID != fmt.Sprintf("arrival-%06d", i+1) || event.Purpose != "task" {
						t.Fatalf("identity=%+v", event)
					}
					if event.PlannedOffsetNS == nil || *event.PlannedOffsetNS != wantOffset || event.ScheduleAnchorOffsetNS == nil || *event.ScheduleAnchorOffsetNS < 0 || event.DurationNS != nil {
						t.Fatalf("plan timing=%+v", event)
					}
				}
				if planned.EventType != "arrival_planned" || terminal.EventType != "arrival_not_started" || terminal.Status != "not_started" || terminal.Reason != "context_canceled" {
					t.Fatalf("planned=%+v terminal=%+v", planned, terminal)
				}
			}
		})
	}
}

func TestRunProfileCancellationAccountsForQueuedAndFutureArrivals(t *testing.T) {
	ctx, readEvents := performanceRecording(t)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	started := make(chan struct{})
	type outcome struct {
		result core.RunResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := runProfile(ctx, "name", "run", []string{"a", "b", "c"}, 1, 0,
			[]arrival{{"a", 0}, {"b", 0}, {"c", time.Hour}},
			func(taskCtx context.Context, occurrence taskOccurrence) (core.RunResult, error) {
				close(started)
				<-taskCtx.Done()
				return core.RunResult{Tasks: []core.TaskResult{{TaskID: occurrence.executionID}}, Summary: core.RunSummary{Tasks: 1}}, taskCtx.Err()
			})
		done <- outcome{result, err}
	}()
	<-started
	cancel()
	got := <-done
	if !errors.Is(got.err, context.Canceled) || len(got.result.Tasks) != 1 || got.result.Summary.Tasks != 1 {
		t.Fatalf("result=%+v error=%v", got.result, got.err)
	}
	byOccurrence := map[string][]string{}
	for _, event := range readEvents() {
		byOccurrence[event.OccurrenceID] = append(byOccurrence[event.OccurrenceID], event.EventType)
		if event.EventType == "arrival_finished" && (event.Status != "canceled" || event.Reason != "context_canceled") {
			t.Fatalf("terminal=%+v", event)
		}
	}
	want := map[string][]string{
		"a-001": {"arrival_planned", "arrival_admitted", "arrival_dispatched", "arrival_finished"},
		"b-002": {"arrival_planned", "arrival_not_started"},
		"c-003": {"arrival_planned", "arrival_not_started"},
	}
	if !reflect.DeepEqual(byOccurrence, want) {
		t.Fatalf("events=%v, want %v", byOccurrence, want)
	}
}

func TestRunProfileRecordsQueueRelativeToSchedulerAnchor(t *testing.T) {
	ctx, readEvents := performanceRecording(t)
	// Deliberately separate recorder startup from the scheduling anchor.
	time.Sleep(15 * time.Millisecond)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	release := make(chan struct{})
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := runProfile(ctx, "name", "run", []string{"a", "b"}, 1, 0, []arrival{{"a", 0}, {"b", 5 * time.Millisecond}},
			func(taskCtx context.Context, occurrence taskOccurrence) (core.RunResult, error) {
				if occurrence.logicalID == "a" {
					close(started)
					select {
					case <-release:
					case <-taskCtx.Done():
						return core.RunResult{}, taskCtx.Err()
					}
				}
				return core.RunResult{}, nil
			})
		done <- err
	}()
	<-started
	time.Sleep(25 * time.Millisecond)
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	var admission, dispatch *performance.Event
	for _, event := range readEvents() {
		if event.OccurrenceID != "b-002" {
			continue
		}
		switch event.EventType {
		case "arrival_admitted":
			admission = &event
		case "arrival_dispatched":
			dispatch = &event
		}
	}
	if admission == nil || dispatch == nil || admission.DurationNS == nil || admission.PlannedOffsetNS == nil || admission.ScheduleAnchorOffsetNS == nil {
		t.Fatalf("admission=%+v dispatch=%+v", admission, dispatch)
	}
	if *admission.ScheduleAnchorOffsetNS < int64(15*time.Millisecond) || *admission.PlannedOffsetNS != int64(5*time.Millisecond) || *admission.DurationNS < int64(20*time.Millisecond) {
		t.Fatalf("anchor=%d planned=%d queue=%d", *admission.ScheduleAnchorOffsetNS, *admission.PlannedOffsetNS, *admission.DurationNS)
	}
	// The recorded event occurs after the queue duration is measured; it must
	// use the scheduler anchor, rather than subtracting a recorder-relative offset.
	if admission.ObservedOffsetNS < *admission.ScheduleAnchorOffsetNS+*admission.PlannedOffsetNS+*admission.DurationNS || dispatch.ObservedOffsetNS < admission.ObservedOffsetNS {
		t.Fatalf("inconsistent timing: admission=%+v dispatch=%+v", admission, dispatch)
	}
}

func TestRunProfilePropagatesIsolatedConcurrentIdentities(t *testing.T) {
	ctx, readEvents := performanceRecording(t)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	ctx = performance.WithIdentity(ctx, performance.Identity{LogicalTaskID: "parent", OccurrenceID: "parent", ArrivalID: "parent", Purpose: "parent"})
	ready := make(chan struct{}, 4)
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := runProfile(ctx, "name", "run", []string{"same", "same", "same", "same"}, 4, 0, nil,
			func(taskCtx context.Context, occurrence taskOccurrence) (core.RunResult, error) {
				ready <- struct{}{}
				select {
				case <-release:
				case <-taskCtx.Done():
					return core.RunResult{}, taskCtx.Err()
				}
				performance.Record(taskCtx, performance.Event{EventType: "identity_probe", Operation: occurrence.executionID, Status: "succeeded"})
				return core.RunResult{}, nil
			})
		done <- err
	}()
	for range 4 {
		<-ready
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, event := range readEvents() {
		if event.EventType != "identity_probe" {
			continue
		}
		if event.OccurrenceID != event.Operation || event.LogicalTaskID != "same" || event.Purpose != "task" || event.ArrivalID == "parent" {
			t.Fatalf("crossed identity=%+v", event)
		}
		seen[event.OccurrenceID] = event.ArrivalID
	}
	want := map[string]string{"same-001": "arrival-000001", "same-002": "arrival-000002", "same-003": "arrival-000003", "same-004": "arrival-000004"}
	if !reflect.DeepEqual(seen, want) {
		t.Fatalf("identities=%v, want %v", seen, want)
	}
}

func TestRunProfileLoopOnlyPlansGeneratedCandidates(t *testing.T) {
	ctx, readEvents := performanceRecording(t)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	result, err := runProfile(ctx, "name", "run", []string{"a", "b", "c"}, 1, time.Second, nil,
		func(context.Context, taskOccurrence) (core.RunResult, error) {
			cancel()
			return core.RunResult{Summary: core.RunSummary{Tasks: 1}}, nil
		})
	if !errors.Is(err, context.Canceled) || result.Summary.Tasks != 1 {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	planCount, terminalCount := 0, 0
	for _, event := range readEvents() {
		switch event.EventType {
		case "arrival_planned":
			planCount++
			if event.PlannedOffsetNS == nil || *event.PlannedOffsetNS < 0 {
				t.Fatalf("loop plan=%+v", event)
			}
		case "arrival_finished", "arrival_not_started":
			terminalCount++
		}
	}
	// Depending on scheduling, cancellation happens before or after the next
	// candidate is generated. No candidates beyond that blocked one exist.
	if planCount < 1 || planCount > 2 || terminalCount != planCount {
		t.Fatalf("planned=%d terminal=%d", planCount, terminalCount)
	}
}

func performanceRecording(t *testing.T) (context.Context, func() []performance.Event) {
	t.Helper()
	root := t.TempDir()
	recorder, err := performance.Open(root, "run")
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	t.Cleanup(func() {
		if !closed {
			if err := recorder.Close(); err != nil {
				t.Error(err)
			}
		}
	})
	return performance.WithRecorder(context.Background(), recorder), func() []performance.Event {
		t.Helper()
		closed = true
		if err := recorder.Close(); err != nil {
			t.Fatal(err)
		}
		file, err := os.Open(filepath.Join(root, "performance", "runner-events.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		var events []performance.Event
		decoder := json.NewDecoder(file)
		for {
			var event performance.Event
			if err := decoder.Decode(&event); err != nil {
				if err != io.EOF {
					t.Fatal(err)
				}
				break
			}
			events = append(events, event)
		}
		return events
	}
}

package runner

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/performance"
)

type lifecycleEvent struct {
	EventType         string `json:"event_type"`
	Operation         string `json:"operation"`
	OperationID       string `json:"operation_id"`
	ParentOperationID string `json:"parent_operation_id"`
	LogicalTaskID     string `json:"logical_task_id"`
	OccurrenceID      string `json:"occurrence_id"`
	ArrivalID         string `json:"arrival_id"`
	Purpose           string `json:"purpose"`
	Status            string `json:"status"`
	ErrorClass        string `json:"error_class"`
	DurationNS        *int64 `json:"duration_ns"`
}

func recordLifecycle(t *testing.T) (context.Context, func() []lifecycleEvent) {
	t.Helper()
	root := t.TempDir()
	recorder, err := performance.Open(root, "test-run")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = recorder.Close() })
	ctx := performance.WithRecorder(context.Background(), recorder)
	ctx = performance.WithIdentity(ctx, performance.Identity{LogicalTaskID: "logical-a", ArrivalID: "arrival-7"})
	return ctx, func() []lifecycleEvent {
		t.Helper()
		if err := recorder.Close(); err != nil {
			t.Fatal(err)
		}
		if summary, err := performance.Analyze(filepath.Join(root, "performance")); err != nil {
			t.Fatalf("lifecycle cannot be recomputed: %v; problems=%v", err, summary.Problems)
		}
		file, err := os.Open(filepath.Join(root, "performance", "runner-events.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		decoder := json.NewDecoder(file)
		active := make(map[string]lifecycleEvent)
		var finished []lifecycleEvent
		for {
			var event lifecycleEvent
			if err := decoder.Decode(&event); errors.Is(err, io.EOF) {
				break
			} else if err != nil {
				t.Fatal(err)
			}
			switch event.EventType {
			case "operation_started":
				if event.OperationID == "" || active[event.OperationID].Operation != "" {
					t.Fatalf("invalid or duplicate operation start: %+v", event)
				}
				if event.ParentOperationID != "" && active[event.ParentOperationID].Operation == "" {
					t.Fatalf("operation began outside its parent: %+v", event)
				}
				active[event.OperationID] = event
			case "operation_finished":
				start, ok := active[event.OperationID]
				if !ok || start.Operation != event.Operation || start.ParentOperationID != event.ParentOperationID || event.DurationNS == nil || *event.DurationNS < 0 {
					t.Fatalf("unmatched or invalid operation end: %+v", event)
				}
				delete(active, event.OperationID)
				finished = append(finished, event)
			}
		}
		if len(active) != 0 {
			t.Fatalf("unfinished lifecycle operations: %+v", active)
		}
		return finished
	}
}

func findLifecycle(t *testing.T, events []lifecycleEvent, operation, purpose string) []lifecycleEvent {
	t.Helper()
	var matching []lifecycleEvent
	for _, event := range events {
		if event.Operation == operation && event.Purpose == purpose {
			matching = append(matching, event)
		}
	}
	if len(matching) == 0 {
		t.Fatalf("missing %s (%s) in %+v", operation, purpose, events)
	}
	return matching
}

func TestRunnerLifecycleEventsPreserveParentsAndEvaluationPurpose(t *testing.T) {
	rig := newRig(t, 1)
	rig.benchmark.freshStarts = 2
	ctx, read := recordLifecycle(t)
	ctx, parent := performance.Begin(ctx, "occurrence.parent")
	result, err := rig.runner.Run(ctx)
	parent.End(err)
	if err != nil || result.Tasks[0].Cleanup.Status != core.StatusSucceeded {
		t.Fatalf("Run = %+v, %v", result, err)
	}
	events := read()
	task := findLifecycle(t, events, "task.e2e", "task")[0]
	evaluation := findLifecycle(t, events, "benchmark.evaluate", "task")[0]
	if task.ParentOperationID != findLifecycle(t, events, "occurrence.parent", "")[0].OperationID {
		t.Fatal("task did not retain its caller's parent")
	}
	for _, event := range events {
		if event.Operation == "occurrence.parent" {
			continue
		}
		if event.LogicalTaskID != "logical-a" || event.OccurrenceID != "a" || event.ArrivalID != "arrival-7" || event.Status != "succeeded" {
			t.Fatalf("lifecycle identity or outcome changed: %+v", event)
		}
		if event.Operation == "task.e2e" {
			continue
		}
		wantParent := task.OperationID
		if event.Operation == "sandbox.start" && event.Purpose == core.SandboxPurposeEvaluation {
			wantParent = evaluation.OperationID
		}
		if event.ParentOperationID != wantParent {
			t.Fatalf("%s parent = %q, want %q", event.Operation, event.ParentOperationID, wantParent)
		}
	}
	for _, operation := range []string{"sandbox.start", "sandbox.prepare", "bridge.register", "harness.start", "agent.run", "harness.stop", "bridge.revoke", "benchmark.evaluate", "sandbox.stop", "task.e2e"} {
		if got := len(findLifecycle(t, events, operation, "task")); got != 1 {
			t.Errorf("%s task operations = %d, want 1", operation, got)
		}
	}
	for _, operation := range []string{"sandbox.start", "sandbox.stop"} {
		if got := len(findLifecycle(t, events, operation, core.SandboxPurposeEvaluation)); got != 2 {
			t.Errorf("%s evaluation operations = %d, want 2", operation, got)
		}
	}
}

type nilLifecycleSandbox struct {
	ToolSandbox
	purpose string
}

func (s nilLifecycleSandbox) Start(ctx context.Context, request core.SandboxRequest) (Sandbox, error) {
	if request.Purpose == s.purpose {
		return nil, nil
	}
	return s.ToolSandbox.Start(ctx, request)
}

func TestRunnerLifecycleEventsRecordFailuresAndCleanupAttempts(t *testing.T) {
	for _, tc := range []struct {
		name, operation, purpose string
		configure                func(*rig)
		statuses                 []string
	}{
		{"sandbox start", "sandbox.start", "task", func(r *rig) { r.factory.startErr = errInjected }, []string{"failed"}},
		{"nil sandbox", "sandbox.start", "task", func(r *rig) { r.runner.toolSandbox = nilLifecycleSandbox{ToolSandbox: r.factory} }, []string{"failed"}},
		{"prepare", "sandbox.prepare", "task", func(r *rig) { r.benchmark.prepareErr = errInjected }, []string{"failed"}},
		{"bridge register", "bridge.register", "task", func(r *rig) { r.bridge.startErr = errInjected }, []string{"failed"}},
		{"harness start", "harness.start", "task", func(r *rig) { r.harness.startErr = errInjected }, []string{"failed"}},
		{"isolation retry", "harness.stop", "task", func(r *rig) { r.harness.stopErrors = []error{errInjected, nil} }, []string{"failed", "succeeded"}},
		{"sandbox cleanup", "sandbox.stop", "task", func(r *rig) { r.factory.stopErrors = []error{errInjected} }, []string{"failed"}},
		{"nil evaluation sandbox", "sandbox.start", core.SandboxPurposeEvaluation, func(r *rig) {
			r.benchmark.freshStarts = 1
			r.runner.toolSandbox = nilLifecycleSandbox{ToolSandbox: r.factory, purpose: core.SandboxPurposeEvaluation}
		}, []string{"failed"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rig := newRig(t, 1)
			tc.configure(rig)
			ctx, read := recordLifecycle(t)
			result, err := rig.runner.Run(ctx)
			if err == nil || len(result.Tasks) != 1 || result.Tasks[0].Error == "" {
				t.Fatalf("missing task failure: %+v, %v", result, err)
			}
			events := read()
			task := findLifecycle(t, events, "task.e2e", "task")[0]
			if task.Status != "failed" || task.ErrorClass != "operation_error" {
				t.Fatalf("task span hid task error: %+v", task)
			}
			var statuses []string
			for _, event := range findLifecycle(t, events, tc.operation, tc.purpose) {
				statuses = append(statuses, event.Status)
			}
			if !reflect.DeepEqual(statuses, tc.statuses) {
				t.Fatalf("%s outcomes = %v, want %v", tc.operation, statuses, tc.statuses)
			}
		})
	}
}

func TestRunnerLifecycleCancellationRetainsCleanupIdentity(t *testing.T) {
	rig := newRig(t, 1)
	ctx, read := recordLifecycle(t)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	rig.log.setHook("harness.run", cancel)
	result, err := rig.runner.Run(ctx)
	if !errors.Is(err, context.Canceled) || result.Tasks[0].Cleanup.Status != core.StatusSucceeded {
		t.Fatalf("canceled Run = %+v, %v", result, err)
	}
	events := read()
	task := findLifecycle(t, events, "task.e2e", "task")[0]
	for _, operation := range []string{"task.e2e", "agent.run"} {
		event := findLifecycle(t, events, operation, "task")[0]
		if event.Status != "canceled" || event.ErrorClass != "canceled" {
			t.Fatalf("cancellation recorded incorrectly: %+v", event)
		}
	}
	for _, operation := range []string{"harness.stop", "bridge.revoke", "sandbox.stop"} {
		event := findLifecycle(t, events, operation, "task")[0]
		if event.Status != "succeeded" || event.ParentOperationID != task.OperationID || event.LogicalTaskID != "logical-a" || event.ArrivalID != "arrival-7" || event.OccurrenceID != "a" {
			t.Fatalf("cleanup lost outcome or identity: %+v", event)
		}
	}
}

func TestRunnerLifecycleWithoutRecorderDoesNotCreateArtifacts(t *testing.T) {
	rig := newRig(t, 1)
	rig.runner.outputDir = t.TempDir()
	result, err := rig.runner.Run(context.Background())
	if err != nil || result.Tasks[0].Harness.Status != core.StatusSucceeded || result.Tasks[0].Cleanup.Status != core.StatusSucceeded {
		t.Fatalf("unobserved Run = %+v, %v", result, err)
	}
	files, err := os.ReadDir(rig.runner.outputDir)
	if err != nil || len(files) != 0 {
		t.Fatalf("unobserved Runner wrote files: %v, %v", files, err)
	}
}

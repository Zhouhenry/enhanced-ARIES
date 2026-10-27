package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/performance"
)

func TestPerformanceFinalizesBeforeRunResultWithoutChangingTaskOutcome(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	runErr := errors.New("task failed")
	var stdout bytes.Buffer
	err := executeAndRecord(context.Background(), func(ctx context.Context) (core.RunResult, error) {
		return runWithPerformance(ctx, root, "run", func(ctx context.Context) (core.RunResult, error) {
			_, stage := performance.Begin(ctx, "sandbox.start")
			stage.End(runErr)
			return core.RunResult{RunID: "run", Tasks: []core.TaskResult{{TaskID: "task", Error: runErr.Error()}}, Summary: core.RunSummary{Tasks: 1}}, runErr
		})
	}, root, &stdout)
	if !errors.Is(err, runErr) {
		t.Fatalf("run error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, runResultName))
	if err != nil {
		t.Fatal(err)
	}
	var result core.RunResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if result.Performance == nil || result.Performance.Status != core.StatusSucceeded || result.Summary.Tasks != 1 || result.Tasks[0].Error != runErr.Error() {
		t.Fatalf("result = %+v", result)
	}
	data, err = os.ReadFile(filepath.Join(root, "performance", "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var index performance.Index
	if err := json.Unmarshal(data, &index); err != nil || index.Status != "complete" || index.Written != 2 {
		t.Fatalf("index = %+v, err = %v", index, err)
	}
}

func TestPerformanceOpenFailureDoesNotStartInfrastructure(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "performance"), 0o700); err != nil {
		t.Fatal(err)
	}
	result, err := runWithPerformance(context.Background(), root, "run", func(context.Context) (core.RunResult, error) {
		t.Fatal("execution started without a recorder")
		return core.RunResult{}, nil
	})
	if err == nil || result.Performance == nil || result.Performance.Status != core.StatusFailed || result.Performance.Error == "" {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
}

func TestBaselineSkipsOptionalWriterButRunsExecution(t *testing.T) {
	t.Setenv("ARIES_OBSERVATION_LEVEL", "baseline")
	root := t.TempDir()
	called := false
	result, err := runWithPerformance(context.Background(), root, "baseline", func(ctx context.Context) (core.RunResult, error) {
		called = performance.ObservationLevelFromContext(ctx) == performance.ObservationBaseline
		return core.RunResult{RunID: "baseline", Summary: core.RunSummary{Tasks: 1}}, nil
	})
	if err != nil || !called || result.Performance == nil || result.Performance.Status != core.StatusNotRun || result.Performance.ObservationLevel != "baseline" {
		t.Fatalf("baseline result=%+v err=%v called=%v", result, err, called)
	}
}

func TestPerformanceFinalizationFailureIsSeparateFromTaskSuccess(t *testing.T) {
	root := t.TempDir()
	result, err := runWithPerformance(context.Background(), root, "run", func(ctx context.Context) (core.RunResult, error) {
		_, stage := performance.Begin(ctx, "sandbox.start")
		stage.End(nil)
		// An existing artifact must not be replaced during finalization.
		if err := os.WriteFile(filepath.Join(root, "performance", "index.json"), []byte("existing evidence"), 0o600); err != nil {
			t.Fatal(err)
		}
		return core.RunResult{Tasks: []core.TaskResult{{TaskID: "task"}}, Summary: core.RunSummary{Tasks: 1}}, nil
	})
	if err == nil || result.Performance.Status != core.StatusFailed || result.Tasks[0].Error != "" || result.Summary.Tasks != 1 {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	data, readErr := os.ReadFile(filepath.Join(root, "performance", "index.json"))
	if readErr != nil || string(data) != "existing evidence" {
		t.Fatalf("existing evidence changed: %q, %v", data, readErr)
	}
}

func TestScheduledPerformanceCanBeRecomputedOffline(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(map[bool]string{false: "completed with task failure", true: "canceled before admission"}[canceled], func(t *testing.T) {
			root := t.TempDir()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if canceled {
				cancel()
			}
			taskErr := errors.New("execution failed")
			result, err := runWithPerformance(ctx, root, "run", func(ctx context.Context) (core.RunResult, error) {
				return runProfile(ctx, "test", "run", []string{"a", "b", "a"}, 2, 0, nil,
					func(ctx context.Context, occurrence taskOccurrence) (core.RunResult, error) {
						_, stage := performance.Begin(ctx, "task.e2e")
						var outcome error
						if occurrence.logicalID == "b" {
							outcome = taskErr
						}
						stage.End(outcome)
						return core.RunResult{Tasks: []core.TaskResult{{TaskID: occurrence.executionID}}, Summary: core.RunSummary{Tasks: 1}}, outcome
					})
			})
			if err == nil || result.Performance.Status != core.StatusSucceeded {
				t.Fatalf("expected task failure with successful recording: %+v, %v", result, err)
			}
			summary, err := performance.Analyze(filepath.Join(root, "performance"))
			if err != nil || summary.Status != "complete" || len(summary.Arrivals) != 2 {
				t.Fatalf("summary = %+v, err = %v", summary, err)
			}
			var planned, completed, unstarted uint64
			for _, group := range summary.Arrivals {
				planned += group.Planned
				completed += group.Finished
				unstarted += group.NotStarted
			}
			if planned != 3 || canceled && (unstarted != 3 || completed != 0) || !canceled && (completed != 3 || unstarted != 0) {
				t.Fatalf("planned=%d completed=%d unstarted=%d", planned, completed, unstarted)
			}
			if !canceled {
				if len(summary.Operations) != 2 || summary.Operations[0].Succeeded != 2 || summary.Operations[1].Failed != 1 || summary.Operations[1].SuccessfulDurationNS.Samples != 0 {
					t.Fatalf("operations = %+v", summary.Operations)
				}
			}
		})
	}
}

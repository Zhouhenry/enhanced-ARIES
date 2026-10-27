package app

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/config"
	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/runner"
	"github.com/sirupsen/logrus"
)

func TestAttemptLedgerHundredOccurrencesIncludingRealConstructionFailure(t *testing.T) {
	ids := make([]string, 100)
	for i := range ids {
		ids[i] = "repeat"
	}
	root := t.TempDir()
	buildErr := errors.New("harness construction canary")
	catalog := PreparedBenchmark{NewOccurrence: func(string, string) (runner.Benchmark, error) {
		return &stubBenchmark{}, nil
	}}
	wiring := Wiring{
		NewHarness: func(config.Config, string, func(string) ([]byte, bool), *logrus.Logger) (HarnessInstance, error) {
			return HarnessInstance{}, buildErr
		},
		NewSandbox: func(config.Config, string, string, string, []int, *logrus.Logger) (SandboxInstance, error) {
			return SandboxInstance{}, errors.New("unexpected sandbox construction")
		},
		NewBridge: func(config.Config, string, *logrus.Logger) (runner.ToolBridge, error) {
			return nil, errors.New("unexpected bridge construction")
		},
	}
	result, err := runProfile(context.Background(), "faults", "run-faults", ids, 7, 0, nil,
		func(_ context.Context, occurrence taskOccurrence) (core.RunResult, error) {
			n, _ := strconv.Atoi(strings.TrimPrefix(occurrence.executionID, "repeat-"))
			kind := (n - 1) % 10
			if kind == 0 {
				_, err := buildTaskExperiment(config.Config{Name: "faults"}, core.ModelConfig{}, nil,
					"run-faults", root, occurrence.logicalID, occurrence.executionID, nil, nil, catalog, wiring)
				return core.RunResult{}, err
			}
			now := time.Now()
			task := passedAttemptTask(occurrence.executionID, now)
			var failure error
			switch kind {
			case 1, 2:
				task.Harness.Status = core.StatusNotStarted
				task.Timing.HarnessRunInvokedAt = nil
				task.Evaluation.Status, task.Evaluation.VerifierStatus = core.StatusNotRun, core.StatusNotRun
				failure = errors.New("sandbox create or harness start failed")
			case 3:
				task.Harness.Status = core.StatusFailed
				task.Evaluation.Status = core.StatusFailed
				failure = errors.New("agent failed")
			case 4:
				task.Harness.Status, task.Evaluation.Status = core.StatusFailed, core.StatusNotRun
				failure = context.DeadlineExceeded
			case 5:
				task.Harness.Status, task.Evaluation.Status = core.StatusCanceled, core.StatusNotRun
				failure = context.Canceled
			case 6:
				task.Isolation = core.IsolationResult{Status: core.StatusBlockedIsolation}
				task.Evaluation.Status = core.StatusBlockedIsolation
				failure = errors.New("gateway or in-flight tool did not stop")
			case 7:
				task.Evaluation.Status, task.Evaluation.VerifierStatus = core.StatusFailed, core.StatusFailed
			case 8:
				task.Cleanup.Status = core.StatusFailed
				failure = errors.New("cleanup failed after independent PASS")
			}
			return core.RunResult{Tasks: []core.TaskResult{task}, Summary: core.RunSummary{Tasks: 1}}, failure
		}, time.Minute)
	if !errors.Is(err, buildErr) || !errors.Is(err, context.Canceled) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("construction/cancellation errors disappeared: %v", err)
	}
	s := result.AttemptSummary
	if len(result.Attempts) != 100 || len(result.Tasks) != 90 || result.Summary.Tasks != 90 || s == nil {
		t.Fatalf("legacy tasks or planned denominator changed: attempts=%d tasks=%d summary=%+v", len(result.Attempts), len(result.Tasks), s)
	}
	if s.Planned != 100 || s.Admitted != 100 || s.Started != 100 || s.Executed != 70 || s.Completed != 40 ||
		s.ValidatedSucceeded != 20 || s.Succeeded != 10 || s.Failed != 60 || s.Blocked != 10 || s.Canceled != 10 || s.TimedOut != 10 ||
		s.Unknown != 0 || s.CleanupFailed != 10 || s.GoodputCount == nil || *s.GoodputCount != 20 {
		t.Fatalf("fault matrix accounting: %+v", s)
	}
	seen := make(map[string]bool)
	for _, attempt := range result.Attempts {
		if seen[attempt.AttemptID] || attempt.LogicalTaskID != "repeat" || attempt.FinishedAt == nil || attempt.StartedAt == nil {
			t.Fatalf("missing/duplicate identity or terminal observation: %+v", attempt)
		}
		seen[attempt.AttemptID] = true
	}
}

func passedAttemptTask(id string, at time.Time) core.TaskResult {
	return core.TaskResult{
		TaskID: id, Harness: core.HarnessResult{Status: core.StatusSucceeded, SessionID: "native-session-" + id},
		Isolation: core.IsolationResult{Status: core.StatusConfirmed, HarnessStopped: true, BridgeRevoked: true},
		Evaluation: core.Evaluation{Status: core.StatusSucceeded, VerifierStatus: core.StatusSucceeded,
			Artifact: &core.ArtifactEvidence{ObservedAt: at, Path: "private/candidate.py", SHA256: strings.Repeat("a", 64)}},
		Cleanup: core.CleanupResult{Status: core.StatusSucceeded},
		Timing:  &core.TaskTiming{HarnessRunInvokedAt: &at, ArtifactReadyAt: &at, VerifierFinishedAt: &at},
	}
}

func TestAttemptLedgerPreAdmissionCancellationRetainsFuturePlan(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := runProfile(ctx, "canceled", "run", []string{"a", "a", "b"}, 1, 0, nil,
		func(context.Context, taskOccurrence) (core.RunResult, error) {
			t.Error("canceled task admitted")
			return core.RunResult{}, nil
		})
	if !errors.Is(err, context.Canceled) || result.AttemptSummary.Planned != 3 || result.AttemptSummary.NotStarted != 3 || len(result.Tasks) != 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	for _, attempt := range result.Attempts {
		if attempt.AdmittedAt != nil || attempt.StartedAt != nil || attempt.FinishedAt == nil || attempt.Reason != "context_canceled" {
			t.Fatalf("invented startup timing: %+v", attempt)
		}
	}
}

func TestGoodputUsesArtifactDeadlineAndIndependentVerification(t *testing.T) {
	start := time.Now()
	admitted, ready, verified, finished := start.Add(time.Second), start.Add(2*time.Second), start.Add(8*time.Second), start.Add(10*time.Second)
	for _, name := range []string{"late_verifier_cleanup_failure", "no_slo", "no_artifact", "bad_digest", "unproven_isolation", "skipped_verifier", "late_artifact", "missing_ready", "different_ready", "blank_path", "verifier_error"} {
		t.Run(name, func(t *testing.T) {
			a := core.AttemptResult{OccurrenceID: "task", ScheduledAt: start, AdmittedAt: &admitted, StartedAt: &admitted, FinishedAt: &finished}
			task := passedAttemptTask("task", ready)
			task.Timing.VerifierFinishedAt = &verified
			deadline := 3 * time.Second
			switch name {
			case "late_verifier_cleanup_failure":
				task.Cleanup.Status = core.StatusFailed
			case "no_slo":
				deadline = 0
			case "no_artifact":
				task.Evaluation.Artifact = nil
			case "bad_digest":
				task.Evaluation.Artifact.SHA256 = "bad"
			case "unproven_isolation":
				task.Isolation.BridgeRevoked = false
			case "skipped_verifier":
				task.Evaluation.VerifierStatus = core.StatusNotEnabled
			case "late_artifact":
				late := start.Add(4 * time.Second)
				task.Evaluation.Artifact.ObservedAt = late
				task.Timing.ArtifactReadyAt = &late
			case "missing_ready":
				task.Timing.ArtifactReadyAt = nil
			case "different_ready":
				task.Timing.ArtifactReadyAt = &verified
			case "blank_path":
				task.Evaluation.Artifact.Path = "  "
			case "verifier_error":
				task.Evaluation.Status = core.StatusFailed
				task.Timing.VerifierFinishedAt = nil
			}
			finishAttempt(&a, core.RunResult{Tasks: []core.TaskResult{task}}, nil)
			if name == "verifier_error" && (!a.ArtifactProven || a.VerifiedLatency != nil) {
				t.Fatal("verifier error erased captured artifact or fabricated verification time")
			}
			s := summarizeAttempts([]core.AttemptResult{a}, start, admitted, finished, deadline)
			if name == "no_slo" {
				if s.Goodput != nil || s.GoodputCount != nil || s.GoodputDeadline != nil {
					t.Fatal("implicit SLO invented")
				}
				return
			}
			want := 0
			if name == "late_verifier_cleanup_failure" {
				want = 1
			}
			if s.GoodputCount == nil || *s.GoodputCount != want || s.Goodput == nil || *s.Goodput != float64(want)/10 {
				t.Fatalf("goodput includes verifier/cleanup delay or lacks proof: %+v", s)
			}
			if name != "verifier_error" && (a.VerifiedLatency == nil || *a.VerifiedLatency != 8*time.Second) || s.ObservationDuration != 10*time.Second || s.DrainDuration != 9*time.Second {
				t.Fatalf("incorrect clocks/windows: attempt=%+v summary=%+v", a, s)
			}
		})
	}
}

func TestLegacyResultOmitsNewAccountingFields(t *testing.T) {
	encoded, err := json.Marshal(core.RunResult{Tasks: []core.TaskResult{{TaskID: "legacy"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"attempts"`, `"attempt_summary"`, `"timing"`, `"runtime_started_at"`} {
		if strings.Contains(string(encoded), key) {
			t.Fatalf("legacy JSON acquired %s", key)
		}
	}
}

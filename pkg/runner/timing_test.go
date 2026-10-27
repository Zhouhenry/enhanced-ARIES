package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
)

type timedHarness struct {
	*fakeHarness
	nativeStart time.Time
	nativeEnd   time.Time
}

func (h *timedHarness) Run(ctx context.Context, instruction string) (core.HarnessResult, error) {
	h.nativeStart = time.Now()
	result, err := h.fakeHarness.Run(ctx, instruction)
	h.nativeEnd = time.Now()
	duration := h.nativeEnd.Sub(h.nativeStart)
	result.RuntimeStartedAt = &h.nativeStart
	result.RuntimeFinishedAt = &h.nativeEnd
	result.RuntimeExecutionDuration = &duration
	result.SessionID = "native-session"
	return result, err
}

func TestTaskTimingUsesNativeWindowAndIndependentArtifactEvidence(t *testing.T) {
	rig := newRig(t, 1)
	harness := &timedHarness{fakeHarness: rig.harness}
	rig.runner.harness = harness
	rig.benchmark.evaluation.VerifierStatus = core.StatusSucceeded
	rig.log.setHook("benchmark.evaluate", func() {
		rig.benchmark.evaluation.Artifact = &core.ArtifactEvidence{ObservedAt: time.Now(), Path: "/private/candidate.py", SHA256: strings.Repeat("a", 64)}
	})
	run, err := rig.runner.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	result := run.Tasks[0]
	timing := result.Timing
	ordered := []*time.Time{timing.SandboxCreateStartedAt, timing.SandboxReadyAt, timing.SandboxPreparedAt, timing.BridgePrepareStartedAt, timing.BridgeReadyAt, timing.HarnessReadyAt, timing.HarnessRunInvokedAt, timing.AgentStartedAt, timing.AgentFinishedAt, timing.HarnessRunReturnedAt, timing.IsolationFinishedAt, timing.ArtifactReadyAt, timing.VerifierFinishedAt, timing.CleanupFinishedAt}
	for i, stamp := range ordered {
		if stamp == nil || stamp.IsZero() {
			t.Fatalf("missing timing boundary %d: %+v", i, timing)
		}
		if i > 0 && stamp.Before(*ordered[i-1]) {
			t.Fatalf("reversed timing boundary %d", i)
		}
	}
	if *timing.RuntimeExecutionDuration != harness.nativeEnd.Sub(harness.nativeStart) {
		t.Fatal("native duration changed")
	}
	if result.Duration < *timing.RuntimeExecutionDuration {
		t.Fatal("task duration excludes runtime")
	}
	oldStart := *timing.AgentStartedAt
	harness.nativeStart = time.Time{}
	if !timing.AgentStartedAt.Equal(oldStart) {
		t.Fatal("timing aliases mutable harness state")
	}
}

func TestBridgePreparationTimingIncludesFailedAndCanceledStarts(t *testing.T) {
	for _, scenario := range []string{"succeeded", "failed", "canceled", "create_failed", "prepare_failed"} {
		t.Run(scenario, func(t *testing.T) {
			rig := newRig(t, 1)
			ctx, readEvents := recordLifecycle(t)
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			var entered time.Time
			rig.log.setHook("bridge.start", func() {
				entered = time.Now()
				if scenario == "canceled" {
					cancel()
				}
			})
			switch scenario {
			case "failed":
				rig.bridge.startErr = errInjected
			case "create_failed":
				rig.factory.startErr = errInjected
			case "prepare_failed":
				rig.benchmark.prepareErr = errInjected
			}
			run, err := rig.runner.Run(ctx)
			if (err == nil) != (scenario == "succeeded") || len(run.Tasks) != 1 {
				t.Fatalf("unexpected task outcome: %+v, %v", run, err)
			}
			result := run.Tasks[0]
			timing := result.Timing
			events := readEvents()
			if entered.IsZero() {
				if timing.BridgePrepareStartedAt != nil || timing.BridgeReadyAt != nil || timing.BridgePrepareDuration != nil {
					t.Fatal("unreached bridge preparation fabricated")
				}
				for _, event := range events {
					if event.Operation == "bridge.register" {
						t.Fatal("unreached bridge operation fabricated")
					}
				}
				return
			}
			if timing.BridgePrepareStartedAt == nil || timing.BridgePrepareDuration == nil || *timing.BridgePrepareDuration < 0 {
				t.Fatal("attempted bridge preparation missing")
			}
			finished := timing.BridgePrepareStartedAt.Add(*timing.BridgePrepareDuration)
			if entered.Before(*timing.BridgePrepareStartedAt) || entered.After(finished) || result.Duration < *timing.BridgePrepareDuration {
				t.Fatal("bridge duration does not cover the actual Start call")
			}
			bridgeEvent := findLifecycle(t, events, "bridge.register", "task")[0]
			if bridgeEvent.Status != scenario || bridgeEvent.DurationNS == nil || *bridgeEvent.DurationNS < int64(*timing.BridgePrepareDuration) {
				t.Fatal("bridge duration/outcome does not reconcile with its enclosing raw span")
			}
			if scenario == "succeeded" {
				if timing.BridgeReadyAt == nil || !timing.BridgeReadyAt.Equal(finished) {
					t.Fatal("successful readiness does not match monotonic Start duration")
				}
			} else if timing.BridgeReadyAt != nil || timing.HarnessStartDuration != nil || timing.VerifierDuration != nil {
				t.Fatal("failed or canceled bridge start fabricated readiness or later work")
			}
			if result.Cleanup.Status != core.StatusSucceeded || rig.bridge.stopCount() != 1 {
				t.Fatal("timing changed bridge revocation/cleanup ownership")
			}
		})
	}
}

func TestTaskTimingDoesNotInventReadinessOrVerification(t *testing.T) {
	for _, scenario := range []string{"create", "prepare", "harness_start", "timeout", "isolation", "verifier_error", "verifier_failed", "skip_verifier", "cleanup"} {
		t.Run(scenario, func(t *testing.T) {
			rig := newRig(t, 1)
			rig.benchmark.evaluation.VerifierStatus = core.StatusSucceeded
			switch scenario {
			case "create":
				rig.factory.startErr = errInjected
			case "prepare":
				rig.benchmark.prepareErr = errInjected
			case "harness_start":
				rig.harness.startErr = errInjected
			case "timeout":
				rig.harness.runErrors = []error{context.DeadlineExceeded}
			case "isolation":
				rig.bridge.stopErrors = []error{errInjected, nil}
			case "verifier_error":
				rig.benchmark.evaluationError = errInjected
				rig.benchmark.evaluation.VerifierStatus = core.StatusFailed
			case "verifier_failed":
				rig.benchmark.evaluation.Status = core.StatusFailed
				rig.benchmark.evaluation.VerifierStatus = core.StatusFailed
			case "skip_verifier":
				rig.benchmark.evaluation.VerifierStatus = "skipped"
			case "cleanup":
				rig.factory.stopErrors = []error{errInjected}
			}
			rig.log.setHook("benchmark.evaluate", func() {
				rig.benchmark.evaluation.Artifact = &core.ArtifactEvidence{ObservedAt: time.Now(), Path: "/private/candidate.py", SHA256: strings.Repeat("a", 64)}
			})
			run, _ := rig.runner.Run(context.Background())
			if len(run.Tasks) != 1 {
				t.Fatal("failed task disappeared")
			}
			result := run.Tasks[0]
			timing := result.Timing
			if timing == nil || timing.SandboxCreateStartedAt == nil || timing.SandboxCreateDuration == nil {
				t.Fatal("create attempt lost")
			}
			if timing.AgentStartedAt != nil || timing.AgentFinishedAt != nil || timing.RuntimeExecutionDuration != nil {
				t.Fatal("Run invocation substituted for unknown native timing")
			}
			if scenario == "create" {
				if timing.SandboxReadyAt != nil || timing.CleanupFinishedAt != nil {
					t.Fatal("failed create fabricated readiness/cleanup")
				}
				return
			}
			if timing.SandboxReadyAt == nil || timing.CleanupFinishedAt == nil || timing.CleanupDuration == nil {
				t.Fatal("successful create/cleanup attempt missing")
			}
			if scenario == "prepare" {
				if timing.SandboxPreparedAt != nil || timing.HarnessReadyAt != nil || timing.HarnessRunInvokedAt != nil {
					t.Fatal("failed preparation fabricated later boundaries")
				}
				return
			}
			if scenario == "harness_start" {
				if timing.HarnessReadyAt != nil || timing.HarnessRunInvokedAt != nil {
					t.Fatal("failed harness start fabricated execution")
				}
			} else if timing.HarnessRunInvokedAt == nil || timing.HarnessRunReturnedAt == nil {
				t.Fatal("actual Run call missing")
			}
			if scenario == "isolation" {
				if timing.IsolationFinishedAt != nil || timing.VerifierFinishedAt != nil || timing.VerifierDuration != nil || timing.ArtifactReadyAt != nil {
					t.Fatal("blocked isolation fabricated evaluation")
				}
				if timing.StopRevokeDuration == nil || result.Evaluation.Status != core.StatusBlockedIsolation {
					t.Fatal("isolation attempt not recorded")
				}
				return
			}
			if timing.IsolationFinishedAt == nil || timing.VerifierDuration == nil || timing.ArtifactReadyAt == nil {
				t.Fatal("evaluation/artifact evidence lost")
			}
			if scenario == "verifier_error" || scenario == "skip_verifier" {
				if timing.VerifierFinishedAt != nil {
					t.Fatal("unproven verifier completion")
				}
			} else if timing.VerifierFinishedAt == nil {
				t.Fatal("terminal verifier result not timed")
			}
			if scenario == "cleanup" && result.Cleanup.Status != core.StatusFailed {
				t.Fatal("cleanup error hidden")
			}
		})
	}
}

func TestInvalidArtifactEvidenceDoesNotBecomeGoodputTimestamp(t *testing.T) {
	for _, scenario := range []string{"missing", "future", "stale", "path", "digest"} {
		t.Run(scenario, func(t *testing.T) {
			rig := newRig(t, 1)
			rig.log.setHook("benchmark.evaluate", func() {
				evidence := &core.ArtifactEvidence{ObservedAt: time.Now(), Path: "/candidate.py", SHA256: strings.Repeat("b", 64)}
				switch scenario {
				case "missing":
					evidence = nil
				case "future":
					evidence.ObservedAt = time.Now().Add(time.Hour)
				case "stale":
					evidence.ObservedAt = time.Now().Add(-time.Hour)
				case "path":
					evidence.Path = " "
				case "digest":
					evidence.SHA256 = "not-a-digest"
				}
				rig.benchmark.evaluation.Artifact = evidence
			})
			run, err := rig.runner.Run(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if run.Tasks[0].Timing.ArtifactReadyAt != nil {
				t.Fatal("unproven artifact readiness accepted")
			}
		})
	}
}

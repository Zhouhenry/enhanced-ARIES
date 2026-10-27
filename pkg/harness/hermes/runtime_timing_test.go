package hermes

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/deployment"
)

type timingDeployment struct {
	deployment.Deployment
	artifactStarted time.Time
}

func (d *timingDeployment) Logs(ctx context.Context, id string, limit int) ([]byte, error) {
	d.artifactStarted = time.Now()
	return d.Deployment.Logs(ctx, id, limit)
}

func TestNativeGatewayTimingExcludesArtifactsAndSurvivesFailure(t *testing.T) {
	for _, scenario := range []string{"success", "native_failure", "artifact_failure"} {
		t.Run(scenario, func(t *testing.T) {
			fake := newFakeDeployment(t)
			if scenario == "native_failure" {
				fake.gatewayStatus = "failed"
			}
			if scenario == "artifact_failure" {
				fake.logsErr = errors.New("artifact collection failed")
			}
			manager := newTestManager(t, fake, []byte("model-secret"))
			observed := &timingDeployment{Deployment: fake}
			manager.runtime.Options.Deployment = observed
			if err := manager.Start(context.Background(), testRequest(t)); err != nil {
				t.Fatal(err)
			}
			defer manager.Stop(context.Background())
			result, err := manager.Run(context.Background(), "repair")
			if (err != nil) != (scenario != "success") {
				t.Fatalf("Run error = %v", err)
			}
			if result.RuntimeStartedAt == nil || result.RuntimeFinishedAt == nil || result.RuntimeExecutionDuration == nil {
				t.Fatal("native timing missing")
			}
			if result.SessionID != "aries-attempt" {
				t.Fatalf("wrong native session: %q", result.SessionID)
			}
			if observed.artifactStarted.IsZero() || result.RuntimeFinishedAt.After(observed.artifactStarted) {
				t.Fatal("native timing includes artifact collection")
			}
			if *result.RuntimeExecutionDuration != result.RuntimeFinishedAt.Sub(*result.RuntimeStartedAt) || result.Duration < *result.RuntimeExecutionDuration {
				t.Fatal("native duration inconsistent")
			}
		})
	}
}

func TestStoppedGatewayDoesNotInventNativeWindow(t *testing.T) {
	var result core.HarnessResult
	applyRuntimeTiming(&result, runOutcome{SessionID: "known-session"})
	if result.SessionID != "known-session" || result.RuntimeStartedAt != nil || result.RuntimeFinishedAt != nil || result.RuntimeExecutionDuration != nil {
		t.Fatal("unknown execution window fabricated")
	}
}

package openclaw

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
	gatewayclient "github.com/hyscale-lab/aries/pkg/harness/openclaw/gateway"
)

func TestNativeExecutionTimingEndsBeforeArtifactCollectionIncludingFailure(t *testing.T) {
	for _, failed := range []bool{false, true} {
		name := "success"
		if failed {
			name = "failure"
		}
		t.Run(name, func(t *testing.T) {
			fake := newFakeDeployment()
			var artifactsStarted time.Time
			fake.onLogs = func() { artifactsStarted = time.Now() }
			manager := newTestManager(t, fake, []byte("model-secret"))
			var nativeErr error
			if failed {
				nativeErr = errors.New("native agent failed")
			}
			manager.newGateway = func(string, []byte) (gatewayConnection, error) {
				return &secretAgentGateway{summary: gatewayclient.ConnectSummary{Scopes: []string{"operator.write"}}, result: gatewayclient.AgentResult{Text: "done"}, err: nativeErr}, nil
			}
			request := core.HarnessRequest{Connectivity: core.HarnessConnectivity{Placement: core.RuntimePlacement{AttachmentID: "network"}}, RunID: "run", TaskID: "task", Endpoint: endpointFiles(t), Model: testModel()}
			if err := manager.Start(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			defer manager.Stop(context.Background())
			result, err := manager.Run(context.Background(), "repair")
			if (err != nil) != failed {
				t.Fatalf("run error = %v", err)
			}
			if result.RuntimeStartedAt == nil || result.RuntimeFinishedAt == nil || result.RuntimeExecutionDuration == nil {
				t.Fatal("native timing lost")
			}
			if artifactsStarted.IsZero() || result.RuntimeFinishedAt.After(artifactsStarted) || *result.RuntimeExecutionDuration != result.RuntimeFinishedAt.Sub(*result.RuntimeStartedAt) || result.Duration < *result.RuntimeExecutionDuration {
				t.Fatalf("native timing includes artifacts or is inconsistent: %+v", result)
			}
		})
	}
}

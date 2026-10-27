package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/config"
	"github.com/hyscale-lab/aries/pkg/core"
)

func TestCloudHarnessReturnCallbackKeepsLateRequestsOutOfTape(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer upstream.Close()
	cfg := config.CloudConfig{Placement: "external", Harness: config.CloudHarnessConfig{OpenClawVersion: "test-pin"},
		Model:     config.CloudModelConfig{Mode: "record", ID: "test-model", BaseURL: upstream.URL, APIKeyEnv: "TEST_KEY", ProxyPublicURL: "https://proxy.example.test", MaxTurns: 2},
		Execution: config.CloudExecutionConfig{TaskTimeout: time.Second}}
	proxy := &cloudModelProxy{sessions: make(map[[32]byte]http.Handler)}
	cloud := CloudRun{NewAttempt: func(_ context.Context, request CloudAttempt) (CloudExperiment, error) {
		if request.OnHarnessRunReturned == nil {
			t.Fatal("actual model admission callback missing")
		}
		return CloudExperiment{Run: func(ctx context.Context) (core.RunResult, error) {
			post := func(message string) *httptest.ResponseRecorder {
				r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(fmt.Sprintf(`{"model":"test-model","stream":true,"messages":[{"role":"user","content":%q}]}`, message))).WithContext(ctx)
				r.Header.Set("Authorization", "Bearer "+string(request.AccessToken))
				w := httptest.NewRecorder()
				proxy.ServeHTTP(w, r)
				return w
			}
			if post("task").Code != http.StatusOK {
				t.Fatal("fixture original request failed")
			}
			request.OnHarnessRunReturned()
			late := post("new request during Stop")
			if late.Code != http.StatusConflict || !strings.Contains(late.Body.String(), "ADMISSION_CLOSED") {
				t.Fatal("late request reached active trajectory matching")
			}
			return core.RunResult{}, nil
		}}, nil
	}}
	result, err := runCloudAttempt(context.Background(), cfg, "run", t.TempDir(), taskOccurrence{logicalID: "case", executionID: "case-001"}, nil, proxy, cloud,
		func(string) ([]byte, bool) { return []byte("fixture-key"), true })
	if err != nil || len(result.ModelSessions) != 1 || result.ModelSessions[0].Status != core.StatusSucceeded || upstreamCalls.Load() != 1 {
		t.Fatalf("late request poisoned completed model session: %+v %v", result.ModelSessions, err)
	}
}

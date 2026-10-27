package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/internal/modelreplay"
	"github.com/hyscale-lab/aries/pkg/config"
	"github.com/hyscale-lab/aries/pkg/core"
)

func TestCloudReplayPolicyReachesModelWithoutChangingTaskOutcome(t *testing.T) {
	requests := [][]byte{
		[]byte(`{"model":"test-model","stream":true,"messages":[{"role":"user","content":"task"}]}`),
		[]byte(`{"model":"test-model","stream":true,"messages":[{"role":"user","content":"task"},{"role":"assistant","content":null,"tool_calls":[{"id":"call_a","type":"function","function":{"name":"exec","arguments":"{\"command\":\"test\"}"}}]},{"role":"tool","tool_call_id":"call_a","content":"original output"}]}`),
	}
	frames := []string{
		"data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"tool_calls\":[{\"index\":0,\"id\":\"call_a\",\"type\":\"function\",\"function\":{\"name\":\"exec\",\"arguments\":\"{\\\"command\\\":\\\"test\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n",
		"data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n",
	}
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		turn := int(calls.Add(1)) - 1
		if turn >= len(frames) {
			t.Error("unexpected upstream call")
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		fmt.Fprint(w, frames[turn])
	}))
	defer upstream.Close()
	const token = "fixture-record-access-token"
	recordDir := filepath.Join(t.TempDir(), "model")
	recorder, err := modelreplay.New(modelreplay.Options{Mode: "record", Model: "test-model",
		Identity:    modelreplay.Identity{RunID: "record", AttemptID: "record/task", SessionID: "record-model", Placement: "external", HarnessVersion: "test-pin"},
		AccessToken: []byte(token), UpstreamURL: upstream.URL, UpstreamKey: []byte("fixture-upstream-key"), OutputDir: recordDir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = recorder.Close() })
	for _, body := range requests {
		r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		recorder.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatal("fixture record failed")
		}
	}
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	tape, err := modelreplay.Load(filepath.Join(recordDir, "tape.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, policy := range []string{"", "strict", "workload"} {
		t.Run("policy-"+policy, func(t *testing.T) {
			cfg := config.CloudConfig{Placement: "external", Harness: config.CloudHarnessConfig{OpenClawVersion: "test-pin"},
				Model:     config.CloudModelConfig{Mode: "replay", ReplayPolicy: policy, Timing: "zero", ID: "test-model", ProxyPublicURL: "https://proxy.example.test"},
				Execution: config.CloudExecutionConfig{TaskTimeout: time.Second}}
			proxy := &cloudModelProxy{sessions: make(map[[32]byte]http.Handler)}
			businessFailure := errors.New("independent task verification failed")
			cloud := CloudRun{NewAttempt: func(_ context.Context, attempt CloudAttempt) (CloudExperiment, error) {
				return CloudExperiment{Run: func(ctx context.Context) (core.RunResult, error) {
					for i, body := range requests {
						actual := bytes.ReplaceAll(body, []byte("original output"), []byte("real command failed; exit 127"))
						r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(actual)).WithContext(ctx)
						r.Header.Set("Authorization", "Bearer "+string(attempt.AccessToken))
						w := httptest.NewRecorder()
						proxy.ServeHTTP(w, r)
						want := http.StatusOK
						if i == 1 && policy != "workload" {
							want = http.StatusConflict
						}
						if w.Code != want {
							t.Errorf("policy handoff: got %d want %d", w.Code, want)
						}
					}
					return core.RunResult{Tasks: []core.TaskResult{{TaskID: "task", Evaluation: core.Evaluation{Status: core.StatusFailed, Score: 0, Reward: 0, Error: businessFailure.Error()}, Error: businessFailure.Error()}}}, businessFailure
				}}, nil
			}}
			result, runErr := runCloudAttempt(context.Background(), cfg, "run", t.TempDir(), taskOccurrence{logicalID: "task", executionID: "task-001"}, tape, proxy, cloud,
				func(string) ([]byte, bool) { t.Error("replay requested upstream credentials"); return nil, false })
			if !errors.Is(runErr, businessFailure) || len(result.Tasks) != 1 || result.Tasks[0].Evaluation.Status != core.StatusFailed || result.Tasks[0].Evaluation.Score != 0 || result.Tasks[0].Error != businessFailure.Error() {
				t.Fatal("model policy replaced the original business/verification outcome")
			}
			wantPolicy := policy
			if wantPolicy == "" {
				wantPolicy = "strict"
			}
			wantStatus := core.StatusFailed
			if policy == "workload" {
				wantStatus = core.StatusSucceeded
			}
			if len(result.ModelSessions) != 1 || result.ModelSessions[0].ReplayPolicy != wantPolicy || result.ModelSessions[0].Status != wantStatus || len(proxy.sessions) != 0 {
				t.Fatal("model result lost explicit policy, actual completion, or session cleanup")
			}
		})
	}
}

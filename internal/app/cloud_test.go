package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/config"
	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/sirupsen/logrus"
)

func TestCloudProxyRoutesByExplicitCredentialAndRevokes(t *testing.T) {
	p := &cloudModelProxy{sessions: make(map[[32]byte]http.Handler)}
	for _, id := range []string{"attempt-a", "attempt-b"} {
		id := id
		key := sha256.Sum256([]byte("private-" + id))
		p.sessions[key] = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, id) })
	}
	for _, id := range []string{"attempt-a", "attempt-b", "missing"} {
		r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"messages":[{"content":"attempt-b"}]}`))
		r.Header.Set("Authorization", "Bearer private-"+id)
		w := httptest.NewRecorder()
		p.ServeHTTP(w, r)
		if id == "missing" {
			if w.Code != 401 {
				t.Fatal("unbound credential accepted")
			}
			continue
		}
		if w.Code != 200 || w.Body.String() != id {
			t.Fatal("prompt inferred the wrong session")
		}
	}
	delete(p.sessions, sha256.Sum256([]byte("private-attempt-a")))
	r := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	r.Header.Set("Authorization", "Bearer private-attempt-a")
	w := httptest.NewRecorder()
	p.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("revoked session still reachable")
	}
}

func TestCloudBuildFailureRetainsBoundSessionAndAttempt(t *testing.T) {
	root := t.TempDir()
	cfg := config.CloudConfig{Placement: "embedded", Harness: config.CloudHarnessConfig{OpenClawVersion: "2026.7.1"},
		Model:     config.CloudModelConfig{Mode: "record", ToolCallIDDialect: "openclaw-2026.7.1", ID: "test-model", BaseURL: "https://api.example.test", APIKeyEnv: "TEST_KEY", ProxyPublicURL: "https://proxy.example.test", MaxTurns: 2},
		Execution: config.CloudExecutionConfig{TaskTimeout: time.Second}}
	proxy := &cloudModelProxy{sessions: make(map[[32]byte]http.Handler)}
	canary := errors.New("constructor failed")
	cloud := CloudRun{NewAttempt: func(_ context.Context, request CloudAttempt) (CloudExperiment, error) {
		if request.OutputDir != root || request.Model.Provider != "deepseek" || request.Model.BaseURL != "https://proxy.example.test/v1" || request.Model.APIKeyEnv != "ARIES_MODEL_PROXY_ACCESS_TOKEN" || len(request.AccessToken) < 32 {
			t.Error("cloud identity/root/native endpoint handoff is inconsistent")
		}
		return CloudExperiment{}, canary
	}}
	result, err := runProfile(context.Background(), "cloud", "run", []string{"a", "a"}, 2, 0, nil,
		func(ctx context.Context, occurrence taskOccurrence) (core.RunResult, error) {
			return runCloudAttempt(ctx, cfg, "run", root, occurrence, nil, proxy, cloud, func(name string) ([]byte, bool) {
				if name != "TEST_KEY" {
					t.Error("unexpected credential requested")
				}
				return []byte("fixture-provider-secret"), true
			})
		})
	if !errors.Is(err, canary) || len(result.Attempts) != 2 || len(result.ModelSessions) != 2 || result.AttemptSummary.Failed != 2 {
		t.Fatalf("construction failure disappeared: result=%+v err=%v", result, err)
	}
	if result.ModelSessions[0].SessionID == result.ModelSessions[1].SessionID {
		t.Fatal("sessions reused")
	}
	for i, a := range result.Attempts {
		if a.ModelSessionID != result.ModelSessions[i].SessionID || a.ModelSessionStatus != core.StatusFailed || a.Validated {
			t.Fatal("incomplete model session misreported")
		}
		data, err := os.ReadFile(filepath.Join(result.ModelSessions[i].ArtifactDir, "tape.json"))
		if err != nil {
			t.Fatal(err)
		}
		var tape struct {
			ToolCallIDDialect string `json:"tool_call_id_dialect"`
		}
		if err := json.Unmarshal(data, &tape); err != nil || tape.ToolCallIDDialect != cfg.Model.ToolCallIDDialect {
			t.Fatalf("cloud omitted declared dialect: %v", err)
		}
	}
	if len(proxy.sessions) != 0 {
		t.Fatal("failed attempt retained routing credential")
	}
}

func TestCloudOriginalTaskBudgetReachesRecordProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(10 * time.Millisecond):
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer upstream.Close()
	cfg := config.CloudConfig{Placement: "external", Benchmark: &config.CloudBenchmarkConfig{Type: "terminalbench2", TaskID: "fix-git"},
		Harness:   config.CloudHarnessConfig{OpenClawVersion: "test-pin"},
		Model:     config.CloudModelConfig{Mode: "record", ID: "test-model", BaseURL: upstream.URL, APIKeyEnv: "TEST_KEY", ProxyPublicURL: "https://proxy.example.test", MaxTurns: 2},
		Execution: config.CloudExecutionConfig{TaskTimeout: time.Nanosecond}}
	proxy := &cloudModelProxy{sessions: make(map[[32]byte]http.Handler)}
	cloud := CloudRun{TaskTimeout: time.Second, NewAttempt: func(_ context.Context, request CloudAttempt) (CloudExperiment, error) {
		if request.LogicalID != "fix-git" || request.OccurrenceID != "fix-git-001" {
			t.Fatal("lost original task identity")
		}
		return CloudExperiment{Run: func(ctx context.Context) (core.RunResult, error) {
			r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"test-model","stream":true,"messages":[{"role":"user","content":"original task"}]}`)).WithContext(ctx)
			r.Header.Set("Authorization", "Bearer "+string(request.AccessToken))
			w := httptest.NewRecorder()
			proxy.ServeHTTP(w, r)
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "[DONE]") {
				t.Fatalf("proxy used codingcase budget: %d %s", w.Code, w.Body.String())
			}
			return core.RunResult{}, nil
		}}, nil
	}}
	result, err := runCloudAttempt(context.Background(), cfg, "run", t.TempDir(), taskOccurrence{logicalID: "fix-git", executionID: "fix-git-001"}, nil, proxy, cloud,
		func(string) ([]byte, bool) { return []byte("fixture-key"), true })
	if err != nil || len(result.ModelSessions) != 1 || result.ModelSessions[0].Status != core.StatusSucceeded {
		t.Fatalf("original task record failed: %+v %v", result.ModelSessions, err)
	}
}

func writeCloudRecordProfile(t *testing.T, output, adapterURL, baseURL string) string {
	t.Helper()
	// Reuse the existing complete pin fixture; only this private profile changes.
	path := writeCommandProfile(t, output)
	profile := config.CloudConfig{Name: "cloud-test", AgentType: "openclaw", Placement: "embedded", SandboxBackend: "e2b-cloud", VersionsFile: "versions.json", OutputDir: output,
		Sandbox: config.CloudSandboxConfig{AdapterURL: adapterURL},
		Harness: config.CloudHarnessConfig{OpenClawPath: "/usr/local/bin/openclaw", OpenClawVersion: "2026.7.1"},
		Model:   config.CloudModelConfig{Mode: "record", BaseURL: baseURL}}
	data, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func cloudRecordLookup(name string) ([]byte, bool) {
	values := map[string]string{"E2B_TEMPLATE_ID": "template", "DEEPSEEK_MODEL": "explicit-catalog-model", "DEEPSEEK_API_KEY": "upstream-secret-canary", "ARIES_MODEL_PROXY_PUBLIC_URL": "https://proxy.example.test"}
	value, found := values[name]
	return []byte(value), found
}

func forbidCloudPrepare(t *testing.T) CloudWiring {
	t.Helper()
	return CloudWiring{Prepare: func(context.Context, config.CloudConfig, string, string, func(string) ([]byte, bool), *logrus.Logger) (CloudRun, error) {
		t.Error("invalid configuration or rejected preflight reached cloud effects")
		return CloudRun{}, errors.New("unexpected cloud preparation")
	}}
}

func TestCloudDefaultOriginReachesVersionedCatalogWithoutChangingRecordRoot(t *testing.T) {
	for _, baseURL := range []string{"", "https://api.deepseek.com/", "https://api.deepseek.com/v1"} {
		t.Run(baseURL, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "runs")
			profile := writeCloudRecordProfile(t, output, "", baseURL)
			// Stop at a real parsed preflight outcome before binding any listener.
			doer := &sglangPreflightDoer{t: t, wantURL: "https://api.deepseek.com/v1/models", status: http.StatusUnauthorized, body: `{}`}
			err := RunCloud(context.Background(), profile, io.Discard, Dependencies{PreflightClient: doer}, forbidCloudPrepare(t), cloudRecordLookup, "")
			var failure *liveValidationError
			if !errors.As(err, &failure) || failure.category != liveValidationUnauthorized || doer.requests != 1 {
				t.Fatalf("default origin failed before reaching catalog: %v; requests=%d", err, doer.requests)
			}
			paths, err := filepath.Glob(filepath.Join(output, "*", "cloud-profile.json"))
			if err != nil || len(paths) != 1 {
				t.Fatal("effective profile missing")
			}
			content, err := os.ReadFile(paths[0])
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(content, []byte("upstream-secret-canary")) {
				t.Fatal("credential persisted in effective profile")
			}
			var effective config.CloudConfig
			if err := json.Unmarshal(content, &effective); err != nil {
				t.Fatal(err)
			}
			want := baseURL
			if want == "" {
				want = "https://api.deepseek.com"
			}
			if effective.Model.BaseURL != want {
				t.Fatal("catalog normalization changed the recorded upstream root")
			}
		})
	}
}

func TestCloudUnsafeAdapterOriginCreatesNoRunArtifactsOrPreflight(t *testing.T) {
	for _, endpoint := range []string{
		"https://user:credential-canary@adapter.example.test",
		"https://adapter.example.test?token=credential-canary",
		"https://adapter.example.test#credential-canary",
	} {
		for _, viaEnvironment := range []bool{false, true} {
			output := filepath.Join(t.TempDir(), "runs")
			configured := endpoint
			if viaEnvironment {
				configured = ""
			}
			profile := writeCloudRecordProfile(t, output, configured, "")
			lookup := func(name string) ([]byte, bool) {
				if viaEnvironment && name == "ARIES_E2B_ADAPTER_URL" {
					return []byte(endpoint), true
				}
				return cloudRecordLookup(name)
			}
			doer := &sglangPreflightDoer{t: t, wantURL: "https://api.deepseek.com/v1/models", status: http.StatusUnauthorized, body: `{}`}
			var stdout bytes.Buffer
			err := RunCloud(context.Background(), profile, &stdout, Dependencies{PreflightClient: doer}, forbidCloudPrepare(t), lookup, "")
			if err == nil || strings.Contains(err.Error(), "credential-canary") || stdout.Len() != 0 || doer.requests != 0 {
				t.Fatal("unsafe adapter reached output or preflight")
			}
			if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("persisted run artifacts before validating adapter credentials")
			}
		}
	}
}

func TestCloudObservationFollowsOwnerCleanupWithoutChangingCleanupOutcome(t *testing.T) {
	root := t.TempDir()
	cfg := config.CloudConfig{Placement: "embedded", Harness: config.CloudHarnessConfig{OpenClawVersion: "test-pin"},
		Model:     config.CloudModelConfig{Mode: "record", ID: "test-model", BaseURL: "https://api.example.test", APIKeyEnv: "TEST_KEY", ProxyPublicURL: "https://proxy.example.test", MaxTurns: 2},
		Execution: config.CloudExecutionConfig{TaskTimeout: time.Second}}
	proxy := &cloudModelProxy{sessions: make(map[[32]byte]http.Handler)}
	closed := false
	observationErr := errors.New("operation evidence incomplete")
	cloud := CloudRun{NewAttempt: func(context.Context, CloudAttempt) (CloudExperiment, error) {
		return CloudExperiment{
			Run: func(context.Context) (core.RunResult, error) {
				return core.RunResult{Tasks: []core.TaskResult{{TaskID: "case-001", Cleanup: core.CleanupResult{Status: core.StatusSucceeded}}}}, nil
			},
			Close: func() error { closed = true; return nil },
			Observation: func() (core.ObserverResult, error) {
				if !closed {
					t.Error("observation finalized before owner cleanup")
				}
				return core.ObserverResult{Status: core.StatusFailed, Error: observationErr.Error()}, observationErr
			},
		}, nil
	}}
	result, err := runCloudAttempt(context.Background(), cfg, "run", root, taskOccurrence{logicalID: "case", executionID: "case-001"}, nil, proxy, cloud,
		func(string) ([]byte, bool) { return []byte("fixture-provider-secret"), true })
	if errors.Is(err, observationErr) || !closed || len(result.Tasks) != 1 {
		t.Fatal("cleanup/observation result was lost")
	}
	if err == nil || len(result.ModelSessions) != 1 || result.ModelSessions[0].Status != core.StatusFailed {
		t.Fatal("incomplete model session no longer failed")
	}
	if result.Tasks[0].Cleanup.Status != core.StatusSucceeded || result.Tasks[0].Observer.Status != core.StatusFailed || result.Tasks[0].Error != "" || !strings.Contains(result.Tasks[0].Observer.Error, observationErr.Error()) {
		t.Fatal("measurement failure was hidden or changed the cleanup outcome")
	}
}

func TestCloudObserverFailureDoesNotChangeSuccessfulAttempt(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer upstream.Close()
	for _, failure := range []string{"observer_only", "task", "cleanup"} {
		t.Run(failure, func(t *testing.T) {
			root := t.TempDir()
			cfg := config.CloudConfig{Placement: "embedded", Harness: config.CloudHarnessConfig{OpenClawVersion: "test-pin"},
				Model:     config.CloudModelConfig{Mode: "record", ID: "test-model", BaseURL: upstream.URL, APIKeyEnv: "TEST_KEY", ProxyPublicURL: "https://proxy.example.test", MaxTurns: 2},
				Execution: config.CloudExecutionConfig{TaskTimeout: time.Second}}
			proxy := &cloudModelProxy{sessions: make(map[[32]byte]http.Handler)}
			observationErr := errors.New("metrics file failed")
			businessErr := errors.New("real " + failure + " failure")
			closed := false
			cloud := CloudRun{NewAttempt: func(_ context.Context, request CloudAttempt) (CloudExperiment, error) {
				return CloudExperiment{
					Run: func(ctx context.Context) (core.RunResult, error) {
						r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"test-model","stream":true,"messages":[{"role":"user","content":"finish"}]}`)).WithContext(ctx)
						r.Header.Set("Authorization", "Bearer "+string(request.AccessToken))
						w := httptest.NewRecorder()
						proxy.ServeHTTP(w, r)
						if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "[DONE]") {
							t.Error("fixture model session did not complete")
						}
						now := time.Now()
						task := core.TaskResult{TaskID: request.OccurrenceID, Timing: &core.TaskTiming{HarnessRunInvokedAt: &now},
							Harness: core.HarnessResult{Status: core.StatusSucceeded}, Evaluation: core.Evaluation{Status: core.StatusNotEnabled},
							Cleanup: core.CleanupResult{Status: core.StatusSucceeded}}
						var err error
						if failure == "task" {
							task.Error, task.Harness.Status, err = businessErr.Error(), core.StatusFailed, businessErr
						}
						return core.RunResult{Tasks: []core.TaskResult{task}}, err
					},
					Close: func() error {
						closed = true
						if failure == "cleanup" {
							return businessErr
						}
						return nil
					},
					Observation: func() (core.ObserverResult, error) {
						if !closed {
							t.Error("observer preceded cleanup")
						}
						// The returned error must be retained even when the report omits it.
						return core.ObserverResult{}, observationErr
					},
				}, nil
			}}
			result, err := runProfile(context.Background(), "cloud", "run", []string{"case"}, 1, 0, nil,
				func(ctx context.Context, occurrence taskOccurrence) (core.RunResult, error) {
					return runCloudAttempt(ctx, cfg, "run", root, occurrence, nil, proxy, cloud,
						func(string) ([]byte, bool) { return []byte("fixture-provider-secret"), true })
				})
			if !closed || len(result.Tasks) != 1 || len(result.Attempts) != 1 || len(result.ModelSessions) != 1 {
				t.Fatalf("missing lifecycle result: %+v", result)
			}
			task, attempt := result.Tasks[0], result.Attempts[0]
			if task.Observer.Status != core.StatusFailed || task.Observer.Error != observationErr.Error() || errors.Is(err, observationErr) || strings.Contains(task.Error, observationErr.Error()) {
				t.Fatalf("observer error contaminated business outcome: %+v %v", result, err)
			}
			if result.ModelSessions[0].Status != core.StatusSucceeded || len(proxy.sessions) != 0 || result.AttemptSummary.Planned != 1 || result.AttemptSummary.Started != 1 {
				t.Fatal("model, session revocation, or attempt denominator changed")
			}
			if failure == "observer_only" {
				if err != nil || task.Error != "" || attempt.Status != core.StatusSucceeded || result.AttemptSummary.Succeeded != 1 || result.AttemptSummary.Failed != 0 {
					t.Fatalf("successful attempt failed because of observation: %+v %v", result, err)
				}
			} else if !errors.Is(err, businessErr) || attempt.Status != core.StatusFailed || result.AttemptSummary.Failed != 1 {
				t.Fatalf("real business failure was hidden: %+v %v", result, err)
			}
		})
	}
}

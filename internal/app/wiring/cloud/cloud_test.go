package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/internal/app"
	"github.com/hyscale-lab/aries/pkg/benchmark/codingcase"
	"github.com/hyscale-lab/aries/pkg/config"
	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/monitor"
	"github.com/hyscale-lab/aries/pkg/performance"
	"github.com/hyscale-lab/aries/pkg/runner"
	"github.com/hyscale-lab/aries/pkg/sandbox/e2b"
	gossh "golang.org/x/crypto/ssh"
)

func cloudConfig() config.CloudConfig {
	return config.CloudConfig{Name: "cloud-case", AgentType: "openclaw", Placement: "embedded", SandboxBackend: "e2b-cloud",
		Sandbox:   config.CloudSandboxConfig{AdapterURL: "http://127.0.0.1:8787", TokenEnv: "ADAPTER_TOKEN", Template: "explicit-template", TimeoutSeconds: 120},
		Harness:   config.CloudHarnessConfig{OpenClawPath: "/usr/local/bin/openclaw", OpenClawVersion: "2026.7.1", DockerSocket: "invalid://must-never-be-used"},
		Execution: config.CloudExecutionConfig{TaskTimeout: time.Minute, VerifierTimeout: time.Minute}}
}

func TestEmbeddedConstructionUsesOnlyScopedCredentialsAndNoDocker(t *testing.T) {
	cfg := cloudConfig()
	root := t.TempDir()
	lookup := func(name string) ([]byte, bool) {
		if name != "ADAPTER_TOKEN" {
			t.Fatalf("requested unexpected host credential %q", name)
		}
		return []byte(strings.Repeat("a", 32)), true
	}
	run, err := Prepare(context.Background(), cfg, "run", root, lookup, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := app.CloudAttempt{RunID: "run", LogicalID: "merge-ranges", OccurrenceID: "merge-ranges-1", OutputDir: root,
		Model: core.ModelConfig{Provider: "openai", BaseURL: "https://proxy.example/v1", Model: "explicit-model", APIKeyEnv: "REAL_KEY_MUST_NOT_BE_READ"}, AccessToken: []byte("attempt-scoped-token")}
	experiment, err := run.NewAttempt(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.NewAttempt(context.Background(), request); err == nil {
		t.Fatal("reused occurrence")
	}
	if err := experiment.Close(); err != nil {
		t.Fatal(err)
	}
	if err := run.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	request.OccurrenceID = "merge-ranges-2"
	if _, err := run.NewAttempt(context.Background(), request); err == nil {
		t.Fatal("admitted attempt after run close")
	}
	if string(request.AccessToken) != "attempt-scoped-token" {
		t.Fatal("mutated caller's credential")
	}
}

func TestHuaweiChecksAdapterProviderBeforeConstructingAttempts(t *testing.T) {
	for _, provider := range []string{"huawei-agentsphere", "e2b-cloud", ""} {
		t.Run("provider="+provider, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Method != http.MethodPost || r.URL.Path != "/v1/health" {
					t.Errorf("provider preflight allocated or executed work: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("h", 32) {
					t.Error("provider preflight omitted the configured adapter credential")
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body) != 0 {
					t.Error("provider preflight must send an empty object")
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "provider": provider, "assume_network_isolation": false})
			}))
			defer server.Close()
			cfg := cloudConfig()
			cfg.SandboxBackend = "huawei-agentsphere"
			cfg.Sandbox.AdapterURL = server.URL
			run, err := Prepare(context.Background(), cfg, "run", t.TempDir(), func(string) ([]byte, bool) {
				return []byte(strings.Repeat("h", 32)), true
			}, nil)
			if provider != "huawei-agentsphere" {
				if err == nil || run.NewAttempt != nil {
					t.Fatal("wrong or missing adapter provider admitted a Huawei run")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if err := run.Close(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			if requests.Load() != 1 {
				t.Fatalf("unexpected adapter request count: %d", requests.Load())
			}
		})
	}
}

func TestUnsupportedCloudBackendFailsBeforeCredentialLookup(t *testing.T) {
	cfg := cloudConfig()
	cfg.SandboxBackend = "huawei"
	_, err := Prepare(context.Background(), cfg, "run", t.TempDir(), func(string) ([]byte, bool) {
		t.Error("unsupported backend requested credentials")
		return []byte(strings.Repeat("h", 32)), true
	}, nil)
	if err == nil {
		t.Fatal("unsupported backend accepted")
	}
}

func TestHuaweiPrepareRequiresProfileAndAdapterAssumptionsToMatch(t *testing.T) {
	for _, profileAssumes := range []bool{false, true} {
		for _, adapterAssumes := range []bool{false, true} {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.URL.Path != "/v1/health" {
					t.Errorf("assumption preflight performed cloud work: %s", r.URL.Path)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"provider": "huawei-agentsphere", "assume_network_isolation": adapterAssumes})
			}))
			cfg := cloudConfig()
			cfg.SandboxBackend = "huawei-agentsphere"
			cfg.Sandbox.AdapterURL = server.URL
			cfg.Sandbox.AssumeNetworkIsolation = profileAssumes
			run, err := Prepare(context.Background(), cfg, "run", t.TempDir(), func(string) ([]byte, bool) {
				return []byte(strings.Repeat("h", 32)), true
			}, nil)
			server.Close()
			if profileAssumes != adapterAssumes {
				if err == nil || run.NewAttempt != nil {
					t.Fatal("assumption mismatch admitted a Huawei run")
				}
			} else {
				if err != nil || run.NewAttempt == nil {
					t.Fatalf("explicit matching configuration rejected: %v", err)
				}
				if err := run.Close(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			if requests.Load() != 1 {
				t.Fatalf("unexpected adapter request count: %d", requests.Load())
			}
		}
	}
}

func TestHuaweiAttemptManagerRetainsProviderAndFailedAllocationOwnership(t *testing.T) {
	var kills atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case "/v1/health":
			_ = json.NewEncoder(w).Encode(map[string]any{"provider": "huawei-agentsphere", "assume_network_isolation": true})
		case "/v1/create":
			_ = json.NewEncoder(w).Encode(map[string]any{"sandbox": body["sandbox"], "state": "active", "provider": "e2b-cloud"})
		case "/v1/kill":
			kills.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]bool{"absence_confirmed": true})
		default:
			t.Errorf("unexpected adapter request: %s", r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()
	cfg := cloudConfig()
	cfg.SandboxBackend = "huawei-agentsphere"
	cfg.Sandbox.AdapterURL = server.URL
	cfg.Sandbox.AssumeNetworkIsolation = true
	root := t.TempDir()
	p := &preparedRun{config: cfg, runID: "run", outputRoot: root, token: strings.Repeat("h", 32), attempts: make(map[string]*attempt)}
	defer func() {
		if err := p.close(context.Background()); err != nil {
			t.Error(err)
		}
	}()
	_, err := p.newAttempt(context.Background(), app.CloudAttempt{RunID: "run", LogicalID: "merge-ranges", OccurrenceID: "merge-ranges-1", OutputDir: root,
		Model: core.ModelConfig{Provider: "openai", BaseURL: "https://proxy.example/v1", Model: "explicit-model", APIKeyEnv: proxyTokenEnv}, AccessToken: []byte("attempt-scoped-token")})
	if err != nil {
		t.Fatal(err)
	}
	manager := p.attempts["merge-ranges-1"].sandboxes.manager
	if err := manager.CheckProvider(context.Background()); err != nil {
		t.Fatalf("attempt manager lost the explicit assumption: %v", err)
	}
	live, err := manager.Start(context.Background(), core.SandboxRequest{RunID: "run", TaskID: "merge-ranges-1", Environment: core.Environment{Image: cfg.Sandbox.Template, Workdir: codingcase.Workdir, AllowNetwork: true}})
	if err == nil || live == nil || !strings.Contains(err.Error(), "provider mismatch") {
		t.Fatalf("attempt did not retain Huawei provider or allocation ownership: %v", err)
	}
	if err := p.close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if kills.Load() != 1 {
		t.Fatalf("failed allocation was not cleaned exactly once: %d", kills.Load())
	}
}

func TestExternalRejectsUnreachableBridgeBeforeDockerEffects(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "0.0.0.0", "8.8.8.8", "example.com"} {
		cfg := cloudConfig()
		cfg.Placement = "external"
		cfg.Harness.BridgeHost = host
		_, err := Prepare(context.Background(), cfg, "run", t.TempDir(), func(string) ([]byte, bool) { return []byte(strings.Repeat("a", 32)), true }, nil)
		if err == nil || !strings.Contains(err.Error(), "private bridge bind IP") {
			t.Fatalf("unexpected result for %s: %v", host, err)
		}
	}
}

func TestExternalOccurrencesRetainSeparateBridgeEvidence(t *testing.T) {
	for _, overlap := range []bool{false, true} {
		name := "sequential"
		if overlap {
			name = "overlapping"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			cfg := cloudConfig()
			cfg.Placement = "external"
			cfg.Harness.BridgeHost = "127.0.0.1"
			cfg.Harness.DockerSocket = "unix://" + filepath.Join(root, "unused-docker.sock")
			cfg.Harness.SSHClientPath = filepath.Join(root, "unused-linux-client")
			cfg.Versions.OpenClaw.Image = "ghcr.io/openclaw/openclaw:2026.7.1"
			p := &preparedRun{config: cfg, runID: "run", outputRoot: root, token: strings.Repeat("a", 32), attempts: make(map[string]*attempt)}
			t.Cleanup(func() {
				if err := p.close(context.Background()); err != nil {
					t.Error(err)
				}
			})
			retained := make(map[string][]byte)
			for _, id := range []string{"merge-ranges-001", "merge-ranges-002"} {
				_, err := p.newAttempt(context.Background(), app.CloudAttempt{RunID: "run", LogicalID: "merge-ranges", OccurrenceID: id, OutputDir: root,
					Model: core.ModelConfig{Provider: "openai", BaseURL: "https://proxy.example/v1", Model: "explicit-model", APIKeyEnv: proxyTokenEnv}, AccessToken: []byte("scoped-" + id)})
				if err != nil {
					t.Fatal(err)
				}
				bridge := p.attempts[id].bridge
				endpoint, err := bridge.Start(context.Background(), &bridgeEvidenceSandbox{id: id})
				if err != nil {
					t.Fatalf("start %s: %v", id, err)
				}
				client, err := gossh.Dial("tcp", endpoint.Address, &gossh.ClientConfig{User: endpoint.Username, HostKeyCallback: gossh.InsecureIgnoreHostKey(), Timeout: time.Second})
				if err != nil {
					t.Fatal(err)
				}
				session, err := client.NewSession()
				if err != nil {
					_ = client.Close()
					t.Fatal(err)
				}
				// A rejected native command records evidence without executing a tool.
				err = session.Run("unsupported-dialect-" + id)
				_ = session.Close()
				_ = client.Close()
				if err == nil {
					t.Fatal("unexpectedly accepted unsupported command")
				}
				if !overlap {
					if err := bridge.Stop(context.Background()); err != nil {
						t.Fatal(err)
					}
				}
				if len(endpoint.LogPaths) != 2 {
					t.Fatalf("missing bridge evidence: %v", endpoint.LogPaths)
				}
				for _, path := range endpoint.LogPaths {
					if filepath.Dir(path) != filepath.Join(root, id, "bridge") {
						t.Fatalf("bridge evidence escaped occurrence: %s", path)
					}
					retained[path] = nil
					if !overlap {
						content, err := os.ReadFile(path)
						if err != nil || len(content) == 0 {
							t.Fatalf("missing request evidence: %s: %v", path, err)
						}
						retained[path] = content
					}
				}
			}
			if err := p.close(context.Background()); err != nil {
				t.Fatal(err)
			}
			for path, want := range retained {
				got, err := os.ReadFile(path)
				if err != nil || len(got) == 0 || want != nil && !bytes.Equal(got, want) {
					t.Fatalf("occurrence cleanup changed retained evidence: %s: %v", path, err)
				}
			}
		})
	}
}

type bridgeEvidenceSandbox struct{ id string }

func (s *bridgeEvidenceSandbox) ID() string { return s.id }
func (*bridgeEvidenceSandbox) Connectivity() core.HarnessConnectivity {
	return core.HarnessConnectivity{}
}
func (*bridgeEvidenceSandbox) Upload(context.Context, string, string) error   { return nil }
func (*bridgeEvidenceSandbox) Download(context.Context, string, string) error { return nil }
func (*bridgeEvidenceSandbox) Exec(_ context.Context, c core.Command) (core.CommandResult, error) {
	if len(c.Args) > 3 && c.Args[3] == "stop" {
		return core.CommandResult{Stdout: "ARIES_GROUP_EMPTY\n"}, nil
	}
	return core.CommandResult{}, nil
}
func (*bridgeEvidenceSandbox) ExecStream(_ context.Context, c core.Command, _ io.Reader, out, _ io.Writer) (core.CommandResult, error) {
	if c.Path != "/bin/sh" || c.User != "root" {
		return core.CommandResult{}, errors.New("unexpected tool execution")
	}
	_, err := io.WriteString(out, codingcase.Workdir+"\n")
	return core.CommandResult{}, err
}

func TestProxyLookupNeverFallsBackToHostEnvironment(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "must-not-leave-runner")
	a := &attempt{proxyToken: []byte("scoped")}
	if value, ok := a.lookupProxy("DEEPSEEK_API_KEY"); ok || len(value) != 0 {
		t.Fatal("leaked upstream key")
	}
	value, ok := a.lookupProxy(proxyTokenEnv)
	if !ok || string(value) != "scoped" {
		t.Fatal("missing scoped token")
	}
	clear(value)
	if string(a.proxyToken) != "scoped" {
		t.Fatal("lookup aliased owned credential")
	}
}

func metricFixture(t *testing.T, empty, fail bool) (*observedSandboxes, *atomic.Int32) {
	t.Helper()
	var mu sync.Mutex
	removed := make(map[string]bool)
	kills := new(atomic.Int32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("t", 32) {
			t.Error("missing adapter token")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		id, _ := body["sandbox"].(string)
		var response any
		switch r.URL.Path {
		case "/v1/create":
			if body["template"] != "pinned" {
				t.Error("template mapping lost")
			}
			response = map[string]any{"sandbox": id, "state": "active"}
		case "/v1/metrics":
			if removed[id] {
				t.Error("sampled positively removed sandbox")
			}
			if fail {
				w.WriteHeader(502)
				response = map[string]string{"error_code": "sdk_request_failed"}
				break
			}
			rows := []any{}
			if !empty {
				rows = append(rows, map[string]any{"cpu_used_pct": 12.5, "timestamp": "2026-10-22T01:02:03Z", "mem_used": 4096})
			}
			response = map[string]any{"source": "e2b_sdk", "nominal_interval_seconds": 5, "metrics": rows, "unsupported": []string{"disk_io"}}
		case "/v1/kill":
			removed[id] = true
			kills.Add(1)
			response = map[string]bool{"absence_confirmed": true}
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(400)
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	t.Cleanup(server.Close)
	root := t.TempDir()
	manager, err := e2b.New(e2b.Options{Endpoint: server.URL, Token: strings.Repeat("t", 32), Template: "pinned", TemplateMappings: map[string]string{"pinned": "pinned"}, OutputDir: root})
	if err != nil {
		t.Fatal(err)
	}
	observer := newObservedSandboxes(manager, "run", "occurrence", root, performance.ObservationL1)
	t.Cleanup(func() { _, _ = observer.finish(); _ = manager.Close(); _, _ = manager.FinishObservation() })
	return observer, kills
}

func TestSDKMetricsKeepRawGaugesAndSeparateTaskVerifierIdentities(t *testing.T) {
	observer, kills := metricFixture(t, false, false)
	for _, purpose := range []string{"", core.SandboxPurposeEvaluation} {
		live, err := observer.Start(context.Background(), core.SandboxRequest{RunID: "run", TaskID: "occurrence", Purpose: purpose, Environment: core.Environment{Image: "pinned", Workdir: "/workspace"}})
		if err != nil {
			t.Fatal(err)
		}
		if err := observer.Stop(context.Background(), live); err != nil {
			t.Fatal(err)
		}
	}
	report, err := observer.finish()
	if err != nil || report.Status != core.StatusSucceeded || report.SampleCount < 2 || kills.Load() != 2 {
		t.Fatalf("report %+v: %v, kills %d", report, err, kills.Load())
	}
	content, err := os.ReadFile(report.LogPaths[0])
	if err != nil {
		t.Fatal(err)
	}
	fileIO := observer.fileIOReport()
	if fileIO == nil || fileIO.Scope != "sandbox_metrics_jsonl_only" ||
		fileIO.ReturnedBytes != uint64(len(content)) || fileIO.RequestedBytes != fileIO.ReturnedBytes ||
		fileIO.WritesStarted != uint64(bytes.Count(content, []byte{'\n'})) || fileIO.WritesReturned != fileIO.WritesStarted ||
		fileIO.SyncsStarted != 1 || fileIO.SyncsReturned != 1 || fileIO.SyncErrors != 0 || fileIO.WriteErrors != 0 {
		t.Fatalf("raw metrics file calls were not measured: %+v", fileIO)
	}
	if _, err := observer.finish(); err != nil || *observer.fileIOReport() != *fileIO {
		t.Fatal("repeated finish changed metrics I/O accounting")
	}
	if bytes.Contains(content, []byte("cpu_usage_nanoseconds")) {
		t.Fatal("invented a cumulative CPU counter")
	}
	purposes := map[string]string{}
	for _, line := range bytes.Split(bytes.TrimSpace(content), []byte{'\n'}) {
		var snapshot metricSnapshot
		if err := json.Unmarshal(line, &snapshot); err != nil {
			t.Fatal(err)
		}
		if snapshot.ClientDurationNS < 0 || snapshot.RunID != "run" || snapshot.OccurrenceID != "occurrence" || len(snapshot.Snapshot.Metrics) != 1 {
			t.Fatalf("invalid snapshot: %+v", snapshot)
		}
		purposes[snapshot.Purpose] = snapshot.SandboxHandle
	}
	if purposes["task"] == "" || purposes["evaluation"] == "" || purposes["task"] == purposes["evaluation"] {
		t.Fatal("collapsed fresh verifier identity")
	}
	info, err := os.Stat(report.LogPaths[0])
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("metrics artifact is not private")
	}
	if filepath.Dir(report.LogPaths[0]) != filepath.Join(observer.outputRoot, "occurrence") {
		t.Fatal("artifact escaped occurrence")
	}
}

func TestSDKMetricAbsenceAndFailureNeverBecomeZeroResourceSuccess(t *testing.T) {
	for _, fail := range []bool{false, true} {
		observer, kills := metricFixture(t, true, fail)
		live, err := observer.Start(context.Background(), core.SandboxRequest{Environment: core.Environment{Workdir: "/workspace"}})
		if err != nil {
			t.Fatal(err)
		}
		if err := observer.Stop(context.Background(), live); err != nil {
			t.Fatalf("metrics failure changed confirmed cleanup: %v", err)
		}
		report, err := observer.finish()
		if kills.Load() != 1 {
			t.Fatal("sandbox cleanup was skipped")
		}
		if fail {
			if err == nil || report.Status != core.StatusFailed {
				t.Fatal("lost actual metrics failure")
			}
		} else if err != nil || report.Status != core.StatusNotRun {
			t.Fatalf("empty samples misreported: %+v %v", report, err)
		}
	}
}

func TestAttemptFinalizesOperationEvidenceAfterOwnedSandboxCleanup(t *testing.T) {
	observer, kills := metricFixture(t, false, false)
	if _, err := observer.Start(context.Background(), core.SandboxRequest{RunID: "run", TaskID: "occurrence", Environment: core.Environment{Workdir: "/workspace"}}); err != nil {
		t.Fatal(err)
	}
	a := &attempt{occurrenceID: "occurrence", outputRoot: observer.outputRoot, sandboxes: observer}
	if err := a.close(); err != nil {
		t.Fatal(err)
	}
	report, err := a.observation()
	if err != nil || report.Status != core.StatusSucceeded || kills.Load() != 1 {
		t.Fatalf("observation/cleanup: %+v %v", report, err)
	}
	content, err := os.ReadFile(filepath.Join(observer.outputRoot, "occurrence", "e2b-operations-index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var index struct {
		Status         string `json:"status"`
		Started, Ended int
	}
	if err := json.Unmarshal(content, &index); err != nil {
		t.Fatal(err)
	}
	if index.Status != "complete" || index.Started != index.Ended {
		t.Fatalf("finalized before cleanup: %s", content)
	}
	content, err = os.ReadFile(filepath.Join(observer.outputRoot, "occurrence", "e2b-operations.jsonl"))
	if err != nil || !bytes.Contains(content, []byte("adapter_rpc.kill")) {
		t.Fatal("lost final deletion RPC")
	}
	content, err = os.ReadFile(filepath.Join(observer.outputRoot, "occurrence", "cloud-observation.json"))
	if err != nil {
		t.Fatal(err)
	}
	var observation struct {
		FileIO *performance.FileIOCounters `json:"sandbox_metrics_file_io"`
	}
	if err := json.Unmarshal(content, &observation); err != nil || observation.FileIO == nil || *observation.FileIO != *observer.fileIOReport() {
		t.Fatalf("final observation lost metrics I/O evidence: %v", err)
	}
}

func TestSDKMetricFileFailureKeepsCleanupAndActualIO(t *testing.T) {
	observer, kills := metricFixture(t, false, false)
	live, err := observer.manager.Start(context.Background(), core.SandboxRequest{Environment: core.Environment{Workdir: "/workspace"}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = observer.Stop(context.Background(), live) }()
	observer.path = filepath.Join(observer.outputRoot, "closed-metrics.jsonl")
	observer.file, err = os.OpenFile(observer.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := observer.file.Close(); err != nil {
		t.Fatal(err)
	}
	if observer.fileIOReport() != nil {
		t.Fatal("unfinished measurements were published")
	}
	observer.sample(context.Background(), &metricSampler{sandbox: live.(*e2b.Sandbox), purpose: "task"})
	if err := observer.Stop(context.Background(), live); err != nil || kills.Load() != 1 {
		t.Fatalf("failed metrics I/O prevented confirmed cleanup: %v", err)
	}
	report, err := observer.finish()
	if err == nil || report.Status != core.StatusFailed {
		t.Fatal("closed metrics file reported success")
	}
	counters := observer.fileIOReport()
	if counters == nil || counters.WritesStarted != 1 || counters.WritesReturned != 1 || counters.WriteErrors != 1 ||
		counters.RequestedBytes == 0 || counters.ReturnedBytes != 0 || counters.ShortWrites != 1 || counters.InvalidReturns != 0 ||
		counters.SyncsStarted != 1 || counters.SyncsReturned != 1 || counters.SyncErrors != 1 {
		t.Fatalf("failed actual calls were lost: %+v", counters)
	}
	_, _ = observer.finish()
	if *observer.fileIOReport() != *counters {
		t.Fatal("repeated finish repeated failed file I/O")
	}
}

func TestSDKMetricIOIsAbsentWhenResourceObservationDisabled(t *testing.T) {
	for _, level := range []performance.ObservationLevel{performance.ObservationBaseline, performance.ObservationL0} {
		observer := newObservedSandboxes(nil, "run", "attempt", t.TempDir(), level)
		if _, err := observer.finish(); err != nil || observer.fileIOReport() != nil {
			t.Fatal("disabled resource writer produced a measured-zero report")
		}
	}
}

type brokenResourceSource struct{}

func (brokenResourceSource) Sample(context.Context) ([]core.ResourceReading, error) {
	return nil, errors.New("resource observer unavailable")
}
func (brokenResourceSource) Close() error { return nil }

type emptyBenchmark struct{ called bool }

func (b *emptyBenchmark) Tasks(context.Context) ([]core.Task, error) {
	b.called = true
	return nil, nil
}
func (*emptyBenchmark) PrepareSandbox(context.Context, core.Task, runner.Sandbox) error { return nil }
func (*emptyBenchmark) Evaluate(context.Context, core.Task, runner.Sandbox, runner.EvaluationSandboxes) (core.Evaluation, error) {
	return core.Evaluation{}, nil
}

type unusedHarness struct{}

func (unusedHarness) Start(context.Context, core.HarnessRequest) error { return nil }
func (unusedHarness) Run(context.Context, string) (core.HarnessResult, error) {
	return core.HarnessResult{}, nil
}
func (unusedHarness) Stop(context.Context) error { return nil }

type nativeObservationHarness struct {
	unusedHarness
	report core.ObserverResult
	err    error
}

func (h nativeObservationHarness) NativeIdentityObservation() (core.ObserverResult, error) {
	return h.report, h.err
}

func TestNativeIdentityObservationIsSeparateFromCleanup(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			root := t.TempDir()
			observer := newObservedSandboxes(nil, "run", "occurrence", root, performance.ObservationL0)
			h := nativeObservationHarness{report: core.ObserverResult{Status: core.StatusNotEnabled}}
			if enabled {
				h.err = errors.New("native producer evidence incomplete")
				h.report = core.ObserverResult{Status: core.StatusFailed, Error: h.err.Error(), LogPaths: []string{"native-tool-identities.index.json"}}
			}
			a := &attempt{occurrenceID: "occurrence", outputRoot: root, sandboxes: observer, harness: h, bridge: unusedBridge{}}
			if err := a.close(); err != nil || !a.closed {
				t.Fatalf("observer changed cleanup: %v", err)
			}
			report, err := a.observation()
			if enabled && (!errors.Is(err, h.err) || report.Status != core.StatusFailed) {
				t.Fatalf("lost native observer failure: %+v %v", report, err)
			}
			if !enabled && err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(root, "occurrence", "cloud-observation.json"))
			if err != nil {
				t.Fatal(err)
			}
			var doc map[string]json.RawMessage
			if err := json.Unmarshal(data, &doc); err != nil {
				t.Fatal(err)
			}
			_, present := doc["native_identity"]
			if present != enabled {
				t.Fatalf("native observer enabled=%v present=%v", enabled, present)
			}
		})
	}
}

type nativeToolObservationHarness struct {
	unusedHarness
}

func (nativeToolObservationHarness) NativeToolObservation() (core.ObserverResult, error) {
	return core.ObserverResult{Status: core.StatusFailed, Error: "native tool index incomplete", LogPaths: []string{"native-tool-events.index.json"}}, errors.New("native tool index incomplete")
}

func TestNativeToolObservationIsSeparateFromCleanup(t *testing.T) {
	root := t.TempDir()
	a := &attempt{occurrenceID: "occurrence", outputRoot: root,
		sandboxes: newObservedSandboxes(nil, "run", "occurrence", root, performance.ObservationL0), harness: nativeToolObservationHarness{}, bridge: unusedBridge{}}
	if err := a.close(); err != nil || !a.closed {
		t.Fatalf("observer changed cleanup: %v", err)
	}
	report, err := a.observation()
	if err == nil || report.Status != core.StatusFailed {
		t.Fatalf("native observer failure lost: %+v %v", report, err)
	}
	data, err := os.ReadFile(filepath.Join(root, "occurrence", "cloud-observation.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		NativeTools *core.ObserverResult `json:"native_tools"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if document.NativeTools == nil || document.NativeTools.Status != core.StatusFailed || len(document.NativeTools.LogPaths) != 1 {
		t.Fatal("native tool observer omitted from cloud evidence")
	}
}

type unusedBridge struct{}

func (unusedBridge) Start(context.Context, runner.Sandbox) (core.ToolEndpoint, error) {
	return core.ToolEndpoint{}, nil
}
func (unusedBridge) Stop(context.Context) error { return nil }

func TestObserverStartFailurePreservesRunnerExecutionAndError(t *testing.T) {
	root := t.TempDir()
	observer := newObservedSandboxes(nil, "run", "occurrence", root, performance.ObservationL1)
	recorder, err := monitor.New(monitor.Options{RunID: "run", TaskIDs: []string{"occurrence"}, OutputDir: root, Source: brokenResourceSource{}})
	if err != nil {
		t.Fatal(err)
	}
	benchmark := &emptyBenchmark{}
	r, err := runner.New(benchmark, unusedHarness{}, observer, unusedBridge{}, runner.Options{Name: "still-runs", RunID: "run", OutputDir: root})
	if err != nil {
		t.Fatal(err)
	}
	a := &attempt{occurrenceID: "occurrence", outputRoot: root, sandboxes: observer, runner: r, hostMonitor: recorder}
	result, err := a.run(context.Background())
	if !benchmark.called || result.Name != "still-runs" {
		t.Fatal("observer failure bypassed the Runner")
	}
	if err != nil {
		t.Fatalf("observer changed Runner error: %v", err)
	}
	if err := a.close(); err != nil {
		t.Fatalf("observer changed cleanup: %v", err)
	}
	report, err := a.observation()
	if err == nil || report.Status != core.StatusFailed || !strings.Contains(report.Error, "resource observer unavailable") {
		t.Fatalf("observer failure lost: %+v %v", report, err)
	}
}

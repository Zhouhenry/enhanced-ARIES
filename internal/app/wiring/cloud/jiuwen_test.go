package cloud

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hyscale-lab/aries/internal/app"
	"github.com/hyscale-lab/aries/pkg/config"
	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/harness/jiuwenswarm"
	"github.com/hyscale-lab/aries/pkg/performance"
)

const jiuwenExternalImage = "registry.example/jiuwenswarm:0.2.3@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func jiuwenConfig() config.CloudConfig {
	cfg := cloudConfig()
	cfg.AgentType = "jiuwenswarm"
	cfg.Harness = config.CloudHarnessConfig{JiuwenSourceDir: "/opt/jiuwenswarm", JiuwenPythonPath: "/opt/jiuwenswarm/.venv/bin/python"}
	return cfg
}

func TestJiuwenConfiguredPinsMatchNativeBootstrap(t *testing.T) {
	if config.JiuwenRevision != jiuwenswarm.NativeRevision || config.JiuwenLockSHA256 != jiuwenswarm.LockSHA256 {
		t.Fatal("cloud Tape identity differs from the native bootstrap pins")
	}
}

func TestJiuwenUnsupportedConfigurationFailsBeforeCredentials(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*config.CloudConfig)
	}{
		{"external missing image", func(c *config.CloudConfig) { c.Placement = "external" }},
		{"TerminalBench", func(c *config.CloudConfig) {
			c.Benchmark = &config.CloudBenchmarkConfig{Type: "terminalbench2", Root: "tasks", TaskID: "fix-git"}
		}},
		{"renderer", func(c *config.CloudConfig) { c.Harness.ToolOutputRenderer = "stdout-stderr-v1" }},
		{"native protocol", func(c *config.CloudConfig) { c.Harness.NativeObservationProtocol = "aries-native-process-v1" }},
		{"reasoning", func(c *config.CloudConfig) { c.Model.ReasoningEffort = "none" }},
		{"missing Python", func(c *config.CloudConfig) { c.Harness.JiuwenPythonPath = "" }},
		{"workload replay", func(c *config.CloudConfig) { c.Model.ReplayPolicy = "workload" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := jiuwenConfig()
			test.change(&cfg)
			_, err := Prepare(context.Background(), cfg, "run", t.TempDir(), func(string) ([]byte, bool) {
				t.Error("unsupported Jiuwen configuration requested credentials")
				return nil, false
			}, nil)
			if err == nil {
				t.Fatal("unsupported Jiuwen configuration accepted")
			}
		})
	}
}

func TestExternalJiuwenPreparesItsImageWithoutSSH(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "docker.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	var inspections atomic.Int32
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("API-Version", "1.52")
		if r.URL.Path == "/_ping" {
			_, _ = io.WriteString(w, "OK")
			return
		}
		if strings.Contains(r.URL.Path, "/images/") {
			inspections.Add(1)
			if !strings.Contains(r.URL.Path, jiuwenExternalImage) {
				t.Error("Jiuwen preparation used a different harness image")
			}
			http.Error(w, `{"message":"fixture inspect stop"}`, http.StatusForbidden)
			return
		}
		t.Errorf("image failure continued into deployment: %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	})}
	done := make(chan struct{})
	go func() { defer close(done); _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close(); <-done })
	cfg := jiuwenConfig()
	cfg.Placement = "external"
	cfg.Harness.JiuwenImage, cfg.Harness.BridgeHost, cfg.Harness.DockerSocket = jiuwenExternalImage, "10.0.0.2", socket
	cfg.Versions.OpenClaw.Image = "must-not-select-openclaw"
	_, err = Prepare(context.Background(), cfg, "run", t.TempDir(), func(string) ([]byte, bool) {
		return []byte(strings.Repeat("a", 32)), true
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "inspect Docker image") || inspections.Load() != 1 {
		t.Fatalf("native image was not selected before effects, or SSH was required: inspections=%d err=%v", inspections.Load(), err)
	}
}

func TestExternalJiuwenAttemptOwnsNativeHarnessBridgeAndHostMonitor(t *testing.T) {
	for _, provider := range []string{"e2b-cloud", "huawei-agentsphere"} {
		t.Run(provider, func(t *testing.T) {
			root := t.TempDir()
			cfg := jiuwenConfig()
			cfg.Placement, cfg.SandboxBackend = "external", provider
			cfg.Harness.JiuwenImage, cfg.Harness.BridgeHost = jiuwenExternalImage, "10.0.0.2"
			cfg.Harness.DockerSocket = "unix://" + filepath.Join(root, "unused-docker.sock")
			cfg.Versions.OpenClaw.Image = "must-not-select-openclaw"
			p := &preparedRun{config: cfg, runID: "run", outputRoot: root, token: strings.Repeat("a", 32), attempts: make(map[string]*attempt)}
			t.Cleanup(func() {
				if err := p.close(context.Background()); err != nil {
					t.Error(err)
				}
			})
			ctx := performance.WithObservationLevel(context.Background(), performance.ObservationL1)
			for _, id := range []string{"merge-ranges-001", "merge-ranges-002"} {
				request := app.CloudAttempt{RunID: "run", LogicalID: "merge-ranges", OccurrenceID: id, OutputDir: root,
					Model: core.ModelConfig{Provider: "deepseek", BaseURL: "https://proxy.example/v1", Model: "explicit-model"}, AccessToken: []byte("scoped-" + id)}
				if _, err := p.newAttempt(ctx, request); err != nil {
					t.Fatal(err)
				}
				a := p.attempts[id]
				if _, ok := a.harness.(*jiuwenswarm.External); !ok {
					t.Fatalf("external Jiuwen selected %T", a.harness)
				}
				if _, ok := a.bridge.(*jiuwenswarm.RemoteBridge); !ok {
					t.Fatalf("external Jiuwen selected %T", a.bridge)
				}
				if a.closeTransport == nil || a.hostSource == nil || a.hostMonitor == nil || a.runner == nil {
					t.Fatal("external Jiuwen lost shared Docker observation or rollback ownership")
				}
				if _, err := p.newAttempt(ctx, request); err == nil {
					t.Fatal("reused external occurrence")
				}
			}
			if p.attempts["merge-ranges-001"].bridge == p.attempts["merge-ranges-002"].bridge {
				t.Fatal("occurrences share a mutable native bridge")
			}
		})
	}
}

func TestExternalJiuwenConstructionFailureClosesOwnedBridge(t *testing.T) {
	root := t.TempDir()
	cfg := jiuwenConfig()
	cfg.Placement = "external"
	cfg.Harness.JiuwenImage, cfg.Harness.BridgeHost, cfg.Harness.DockerSocket = "", "10.0.0.2", "unix://"+filepath.Join(root, "unused-docker.sock")
	p := &preparedRun{config: cfg, runID: "run", outputRoot: root, token: strings.Repeat("a", 32), attempts: make(map[string]*attempt)}
	_, err := p.newAttempt(context.Background(), app.CloudAttempt{RunID: "run", LogicalID: "merge-ranges", OccurrenceID: "merge-ranges-001", OutputDir: root,
		Model: core.ModelConfig{Provider: "deepseek", BaseURL: "https://proxy.example/v1", Model: "explicit-model"}, AccessToken: []byte("scoped-token")})
	if err == nil {
		t.Fatal("invalid native harness image was accepted")
	}
	a := p.attempts["merge-ranges-001"]
	if a == nil || a.bridge == nil || !a.closed || strings.Trim(string(a.proxyToken), "\x00") != "" {
		t.Fatal("partial construction did not close its owned bridge and clear the scoped token")
	}
	if err := p.close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestJiuwenConstructionUsesProviderAndScopedModelToken(t *testing.T) {
	for _, provider := range []string{"e2b-cloud", "huawei-agentsphere"} {
		t.Run(provider, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.URL.Path != "/v1/health" || r.Method != http.MethodPost {
					t.Errorf("construction allocated or executed cloud work: %s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("a", 32) {
					t.Error("provider lookup lost the adapter token")
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"provider": provider, "assume_network_isolation": false})
			}))
			defer server.Close()
			cfg := jiuwenConfig()
			cfg.SandboxBackend, cfg.Sandbox.AdapterURL = provider, server.URL
			root := t.TempDir()
			run, err := Prepare(context.Background(), cfg, "run", root, func(name string) ([]byte, bool) {
				if name != "ADAPTER_TOKEN" {
					t.Fatalf("requested upstream credential: %s", name)
				}
				return []byte(strings.Repeat("a", 32)), true
			}, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := run.Close(context.Background()); err != nil {
					t.Error(err)
				}
			})
			request := app.CloudAttempt{RunID: "run", LogicalID: "merge-ranges", OccurrenceID: "merge-ranges-1", OutputDir: root,
				Model: core.ModelConfig{Provider: "deepseek", BaseURL: "https://proxy.example/v1", Model: "explicit-model", APIKeyEnv: "UPSTREAM_MUST_NOT_BE_READ"}, AccessToken: []byte("attempt-scoped-token")}
			experiment, err := run.NewAttempt(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := run.NewAttempt(context.Background(), request); err == nil {
				t.Fatal("reused native occurrence")
			}
			if err := experiment.Close(); err != nil {
				t.Fatal(err)
			}
			if string(request.AccessToken) != "attempt-scoped-token" {
				t.Fatal("mutated caller token")
			}
			var wantRequests int32
			if provider == "huawei-agentsphere" {
				wantRequests = 1
			}
			if requests.Load() != wantRequests {
				t.Fatalf("unexpected provider requests: %d", requests.Load())
			}
		})
	}
}

func TestJiuwenAttemptSelectsNativeHarnessAndItsBridge(t *testing.T) {
	root := t.TempDir()
	p := &preparedRun{config: jiuwenConfig(), runID: "run", outputRoot: root, token: strings.Repeat("a", 32), attempts: make(map[string]*attempt)}
	t.Cleanup(func() {
		if err := p.close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	_, err := p.newAttempt(context.Background(), app.CloudAttempt{RunID: "run", LogicalID: "merge-ranges", OccurrenceID: "merge-ranges-1", OutputDir: root,
		Model: core.ModelConfig{Provider: "deepseek", BaseURL: "https://proxy.example/v1", Model: "explicit-model"}, AccessToken: []byte("attempt-scoped-token")})
	if err != nil {
		t.Fatal(err)
	}
	a := p.attempts["merge-ranges-1"]
	native, ok := a.harness.(*jiuwenswarm.Embedded)
	if !ok {
		t.Fatalf("Jiuwen selected %T", a.harness)
	}
	if native == nil || a.bridge == nil || a.closeTransport != nil || a.hostMonitor != nil {
		t.Fatal("Jiuwen lacks its bridge or borrowed an external Docker transport")
	}
}

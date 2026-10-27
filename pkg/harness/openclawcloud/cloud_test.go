package openclawcloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/harness/internal/sandboxprocess"
	"github.com/hyscale-lab/aries/pkg/harness/openclaw"
	"github.com/hyscale-lab/aries/pkg/harness/openclaw/gateway"
	"github.com/hyscale-lab/aries/pkg/runner"
	gossh "golang.org/x/crypto/ssh"
)

type fakeSandbox struct {
	mu                    sync.Mutex
	prepareCode, stopCode int
	staged                []byte
	stopCalls             int
	lastCommand           core.Command
	aliasPrepared         bool
	devices               func(context.Context, core.Command) (core.CommandResult, error)
}

func TestExternalCloudTaskEnvironmentReachesNativePayload(t *testing.T) {
	sandbox := &fakeSandbox{}
	defaults := map[string]string{"PATH": "/usr/local/bin:/usr/bin:/bin:/app/bin", "TASK_VALUE": "original"}
	target := &cloudTarget{group: testProcessGroup(t, sandbox, "/app"),
		options: ExternalBridgeOptions{CommandEnv: defaults}}
	for _, override := range []bool{false, true} {
		command := core.Command{Path: "/bin/sh", Args: []string{"-c", "sqlite3 --version"}, Env: map[string]string{"CALL_VALUE": "native"}}
		if override {
			command.Env["PATH"] = "/native/explicit/path"
		}
		if _, err := target.ExecStream(context.Background(), command, nil, io.Discard, io.Discard); err != nil {
			t.Fatal(err)
		}
		var payload struct {
			Path string            `json:"path"`
			Args []string          `json:"args"`
			Dir  string            `json:"dir"`
			Env  map[string]string `json:"env"`
		}
		if err := json.Unmarshal([]byte(sandbox.lastCommand.Args[len(sandbox.lastCommand.Args)-1]), &payload); err != nil {
			t.Fatal(err)
		}
		wantPath := defaults["PATH"]
		if override {
			wantPath = command.Env["PATH"]
		}
		if payload.Path != command.Path || len(payload.Args) != 2 || payload.Args[1] != command.Args[1] || payload.Dir != "/app" || payload.Env["PATH"] != wantPath || payload.Env["TASK_VALUE"] != "original" || payload.Env["CALL_VALUE"] != "native" {
			t.Fatalf("task defaults or native command changed: %+v", payload)
		}
		if _, mutated := command.Env["TASK_VALUE"]; mutated || defaults["PATH"] != "/usr/local/bin:/usr/bin:/bin:/app/bin" {
			t.Fatal("mutated caller environment")
		}
	}
}

func (*fakeSandbox) ID() string                                     { return "owned-handle" }
func (*fakeSandbox) Connectivity() core.HarnessConnectivity         { return core.HarnessConnectivity{} }
func (*fakeSandbox) Upload(context.Context, string, string) error   { return nil }
func (*fakeSandbox) Download(context.Context, string, string) error { return nil }
func (f *fakeSandbox) Exec(ctx context.Context, c core.Command) (core.CommandResult, error) {
	if len(c.Args) > 3 && c.Args[2] == groupScript && c.Args[3] == "devices" && f.devices != nil {
		return f.devices(ctx, c)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(c.Args) > 3 && c.Args[2] == sandboxprocess.ControlScript {
		switch c.Args[3] {
		case "prepare":
			return core.CommandResult{ExitCode: f.prepareCode}, nil
		case "stop":
			f.stopCalls++
			return core.CommandResult{ExitCode: f.stopCode, Stdout: "ARIES_GROUP_EMPTY\n"}, nil
		}
	}
	if len(c.Args) > 2 && c.Args[2] == stageScript {
		f.staged = bytes.Clone(c.Stdin)
	}
	if len(c.Args) > 2 && c.Args[2] == evidenceScript {
		return core.CommandResult{Stdout: `{"processes":[{"pid":123,"uids":[60000,60000,60000,60000]}]}`}, nil
	}
	return core.CommandResult{}, nil
}
func (f *fakeSandbox) ExecStream(ctx context.Context, c core.Command, in io.Reader, out, errout io.Writer) (core.CommandResult, error) {
	if len(c.Args) > 3 && c.Args[2] == groupScript && c.Args[3] == "gateway" {
		<-ctx.Done()
		return core.CommandResult{ExitCode: -1}, ctx.Err()
	}
	f.mu.Lock()
	f.lastCommand = c
	setup := c.User == "root" && c.Path == "/bin/sh"
	if setup {
		f.aliasPrepared = true
	}
	f.mu.Unlock()
	if setup {
		_, err := io.WriteString(out, "/workspace\n")
		return core.CommandResult{}, err
	}
	if in != nil {
		_, err := io.Copy(out, in)
		return core.CommandResult{}, err
	}
	return core.CommandResult{}, nil
}

type fakeGateway struct {
	connect  func(context.Context, gateway.ConnectOptions) (gateway.ConnectSummary, error)
	agent    func(context.Context, gateway.AgentRequest) (gateway.AgentResult, error)
	events   chan gateway.Frame
	closeErr error
}

func (f *fakeGateway) Connect(ctx context.Context, options gateway.ConnectOptions) (gateway.ConnectSummary, error) {
	if f.connect != nil {
		return f.connect(ctx, options)
	}
	return gateway.ConnectSummary{Role: "operator", Scopes: []string{"operator.write"}}, nil
}
func (f *fakeGateway) Agent(ctx context.Context, r gateway.AgentRequest) (gateway.AgentResult, error) {
	result, err := f.agent(ctx, r)
	if result.RunID != "" {
		f.events <- gateway.Frame{"type": "event", "event": "agent", "payload": map[string]any{"sessionKey": r.SessionKey, "runId": result.RunID, "stream": "lifecycle", "seq": float64(1), "data": map[string]any{"phase": "end"}}}
	}
	return result, err
}
func (f *fakeGateway) RecvEvent(ctx context.Context) (gateway.Frame, error) {
	select {
	case frame := <-f.events:
		return frame, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (f *fakeGateway) Close() error { return f.closeErr }

func embeddedFixture(t *testing.T, sandbox *fakeSandbox, agent func(context.Context, gateway.AgentRequest) (gateway.AgentResult, error), inputs ...*openclaw.WorkloadInputs) *Embedded {
	t.Helper()
	options := EmbeddedOptions{OutputDir: t.TempDir(), Workdir: "/workspace", OpenClawPath: "/usr/local/bin/openclaw", ExpectedVersion: "2026.7.1", ModelProxyURL: "https://proxy.example/v1", ProxyAccessToken: []byte("session-proxy-secret"), CleanupTimeout: time.Second,
		GuestEndpoint: func(context.Context, runner.Sandbox, int) (string, http.Header, error) {
			return "https://guest.example", http.Header{"E2b-Traffic-Access-Token": []string{"private-traffic-token"}}, nil
		},
	}
	if len(inputs) != 0 {
		options.WorkloadInputs = inputs[0]
	}
	e, err := NewEmbedded(options)
	if err != nil {
		t.Fatal(err)
	}
	e.newGateway = func(rawURL string, headers http.Header, token []byte, device *gateway.DeviceIdentity) (gatewayConnection, error) {
		if rawURL != "wss://guest.example" || headers.Get("E2b-Traffic-Access-Token") == "" || len(token) == 0 || device == nil {
			t.Error("missing authenticated Gateway transport")
		}
		return &fakeGateway{agent: agent, events: make(chan gateway.Frame, 16)}, nil
	}
	if _, err := e.Bridge().Start(context.Background(), sandbox); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sandbox.mu.Lock(); sandbox.stopCode = 0; sandbox.mu.Unlock(); _ = e.Stop(context.Background()) })
	return e
}

func startEmbedded(t *testing.T, e *Embedded) {
	t.Helper()
	err := e.Start(context.Background(), core.HarnessRequest{RunID: "run", TaskID: "python-bug", Endpoint: core.ToolEndpoint{Protocol: "embedded", Workdir: "/workspace"}, Timeout: time.Second, Model: core.ModelConfig{Provider: "deepseek", BaseURL: "https://api.deepseek.com", Model: "explicit-test-model", APIKeyEnv: "DEEPSEEK_API_KEY"}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestEmbeddedWorkloadInputsKeepIndependentGroupsAndIdempotency(t *testing.T) {
	inputs := &openclaw.WorkloadInputs{SessionKey: "agent:main:aries-workload", SessionID: "8ad5eb11-e541-4cbf-8ae0-1f9e2822c342", EnvelopeTimestamp: "off"}
	var previousGroup, previousArtifactDir, previousIdempotency string
	for i := 0; i < 2; i++ {
		sandbox := &fakeSandbox{}
		var submitted gateway.AgentRequest
		e := embeddedFixture(t, sandbox, func(_ context.Context, request gateway.AgentRequest) (gateway.AgentResult, error) {
			submitted = request
			return gateway.AgentResult{RunID: "native-run", Text: "fixed"}, nil
		}, inputs)
		startEmbedded(t, e)
		result, err := e.Run(context.Background(), "fix the Python case")
		if err != nil {
			t.Fatal(err)
		}
		if submitted.SessionKey != inputs.SessionKey || submitted.SessionID != inputs.SessionID || result.SessionID != inputs.SessionKey {
			t.Fatalf("native fixture identity changed: %#v", submitted)
		}
		if submitted.IdempotencyKey == "" || submitted.IdempotencyKey == previousIdempotency || e.group.ControlDir() == previousGroup || e.artifactDir == previousArtifactDir {
			t.Fatal("fixed native fixture reused idempotency, group or private artifacts")
		}
		sandbox.mu.Lock()
		staged := bytes.Clone(sandbox.staged)
		sandbox.mu.Unlock()
		var spec struct {
			Configuration string `json:"configuration"`
		}
		if err := json.Unmarshal(staged, &spec); err != nil {
			t.Fatal(err)
		}
		var configuration struct {
			Agents struct {
				Defaults struct {
					EnvelopeTimestamp string `json:"envelopeTimestamp"`
				} `json:"defaults"`
			} `json:"agents"`
		}
		if err := json.Unmarshal([]byte(spec.Configuration), &configuration); err != nil {
			t.Fatal(err)
		}
		if configuration.Agents.Defaults.EnvelopeTimestamp != "off" {
			t.Fatal("native timestamp setting did not reach guest configuration")
		}
		previousGroup, previousArtifactDir, previousIdempotency = e.group.ControlDir(), e.artifactDir, submitted.IdempotencyKey
		if err := e.Stop(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEmbeddedUsesProxyNativeProtocolAndMeasuredRuntime(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "real-key-must-stay-outside")
	sandbox := &fakeSandbox{}
	e := embeddedFixture(t, sandbox, func(_ context.Context, r gateway.AgentRequest) (gateway.AgentResult, error) {
		if r.Message != "fix the Python case" || r.SessionKey == "" || r.IdempotencyKey == "" {
			t.Fatal("native Agent request missing causal identity")
		}
		return gateway.AgentResult{RunID: "native-run", Text: "fixed session-proxy-secret"}, nil
	})
	startEmbedded(t, e)
	sandbox.mu.Lock()
	staged := bytes.Clone(sandbox.staged)
	sandbox.mu.Unlock()
	if bytes.Contains(staged, []byte("real-key-must-stay-outside")) || bytes.Contains(staged, []byte("DEEPSEEK_API_KEY")) {
		t.Fatal("upstream DeepSeek credential reached guest")
	}
	if !bytes.Contains(staged, []byte("session-proxy-secret")) {
		t.Fatal("scoped proxy token was not staged")
	}
	result, err := e.Run(context.Background(), "fix the Python case")
	if err != nil || result.Status != core.StatusSucceeded {
		t.Fatalf("run: %+v %v", result, err)
	}
	if result.RuntimeExecutionDuration == nil || result.RuntimeStartedAt == nil || result.RuntimeFinishedAt == nil || *result.RuntimeExecutionDuration != result.RuntimeFinishedAt.Sub(*result.RuntimeStartedAt) || result.Duration < *result.RuntimeExecutionDuration {
		t.Fatal("native timing boundary missing or inconsistent")
	}
	if strings.Contains(result.FinalResponse, "session-proxy-secret") {
		t.Fatal("result leaked proxy token")
	}
	if err := e.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := e.Bridge().Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sandbox.stopCalls != 1 {
		t.Fatalf("duplicate shutdown used %d remote stops", sandbox.stopCalls)
	}
	for _, p := range result.LogPaths {
		content, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(content, []byte("session-proxy-secret")) {
			t.Fatal("artifact leaked credential")
		}
	}
}

func TestEmbeddedNativeObserverFailurePreservesAgentAndStop(t *testing.T) {
	sandbox := &fakeSandbox{}
	e := embeddedFixture(t, sandbox, func(context.Context, gateway.AgentRequest) (gateway.AgentResult, error) {
		return gateway.AgentResult{RunID: "native-run", Text: "unchanged result"}, nil
	})
	startEmbedded(t, e)
	if err := os.Mkdir(filepath.Join(e.artifactDir, "native-tool-events.index.json"), 0700); err != nil {
		t.Fatal(err)
	}
	result, err := e.Run(context.Background(), "repair")
	if err != nil || result.Status != core.StatusSucceeded || result.FinalResponse != "unchanged result" {
		t.Fatalf("observer changed Agent: %+v %v", result, err)
	}
	report, err := e.NativeToolObservation()
	if err == nil || report.Status != core.StatusFailed || len(report.LogPaths) != 1 {
		t.Fatalf("observer failure lost: %+v %v", report, err)
	}
	if err := e.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestEmbeddedIsolationFailureBlocksBridgeAndCanRetry(t *testing.T) {
	sandbox := &fakeSandbox{stopCode: 1}
	e := embeddedFixture(t, sandbox, func(context.Context, gateway.AgentRequest) (gateway.AgentResult, error) {
		return gateway.AgentResult{}, nil
	})
	startEmbedded(t, e)
	if err := e.Stop(context.Background()); err == nil {
		t.Fatal("failed kill was reported as confirmed isolation")
	}
	if err := e.Bridge().Stop(context.Background()); err == nil {
		t.Fatal("bridge falsely treated embedded stop as no-op")
	}
	if _, err := e.Run(context.Background(), "must not start"); err == nil {
		t.Fatal("admitted work after failed shutdown")
	}
	sandbox.mu.Lock()
	sandbox.stopCode = 0
	sandbox.mu.Unlock()
	if err := e.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestEmbeddedStopCancelsAndJoinsNativeAgent(t *testing.T) {
	entered := make(chan struct{})
	e := embeddedFixture(t, &fakeSandbox{}, func(ctx context.Context, _ gateway.AgentRequest) (gateway.AgentResult, error) {
		close(entered)
		<-ctx.Done()
		return gateway.AgentResult{}, ctx.Err()
	})
	startEmbedded(t, e)
	done := make(chan core.HarnessResult, 1)
	go func() { result, _ := e.Run(context.Background(), "wait for cancellation"); done <- result }()
	<-entered
	if err := e.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	result := <-done
	if result.Status != core.StatusCanceled || result.RuntimeExecutionDuration == nil {
		t.Fatalf("canceled native window lost: %+v", result)
	}
}

func TestContainmentUnsupportedDoesNotLaunchGateway(t *testing.T) {
	_, err := sandboxprocess.New(context.Background(), &fakeSandbox{prepareCode: 1}, "/workspace")
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("missing cgroup capability: %v", err)
	}
}

func TestEmbeddedRetainsTransportCloseFailureAfterPositiveContainment(t *testing.T) {
	sandbox := &fakeSandbox{}
	e := embeddedFixture(t, sandbox, func(context.Context, gateway.AgentRequest) (gateway.AgentResult, error) {
		return gateway.AgentResult{}, nil
	})
	startEmbedded(t, e)
	e.client.(*fakeGateway).closeErr = errors.New("private-traffic-token close failure")
	for i := 0; i < 2; i++ {
		err := e.Stop(context.Background())
		if err == nil || strings.Contains(err.Error(), "private-traffic-token") {
			t.Fatalf("close failure absent or leaked: %v", err)
		}
	}
	if !e.isolated || sandbox.stopCalls != 1 {
		t.Fatal("close error prevented containment or retry erased evidence")
	}
}

func TestCloudTargetPreservesArgvBinaryStreamAndClosesRootSetup(t *testing.T) {
	sandbox := &fakeSandbox{}
	g, err := sandboxprocess.New(context.Background(), sandbox, "/workspace")
	if err != nil {
		t.Fatal(err)
	}
	target := &cloudTarget{group: g, allowSetup: true}
	if _, err := target.ExecStream(context.Background(), core.Command{Path: "/bin/sh", User: "0:0"}, nil, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := target.ExecStream(context.Background(), core.Command{Path: "/bin/sh", User: "0:0"}, nil, io.Discard, io.Discard); err == nil {
		t.Fatal("root setup reused by tool")
	}
	want := []byte{0, 255, 0, 128, 'x'}
	var got bytes.Buffer
	command := core.Command{Path: "/bin/sh", Args: []string{"-c", "printf '%s' \"a ; $(literal)\""}, Dir: "/workspace", Env: map[string]string{"CUSTOM": "a\nb"}}
	if _, err := target.ExecStream(context.Background(), command, bytes.NewReader(want), &got, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Fatal("binary stdin/stdout corrupted")
	}
	var spec struct {
		Path string            `json:"path"`
		Args []string          `json:"args"`
		Env  map[string]string `json:"env"`
	}
	if err := json.Unmarshal([]byte(sandbox.lastCommand.Args[6]), &spec); err != nil {
		t.Fatal(err)
	}
	if spec.Path != command.Path || len(spec.Args) != 2 || spec.Args[1] != command.Args[1] || spec.Env["CUSTOM"] != "a\nb" {
		t.Fatalf("command semantics changed: %+v", spec)
	}
}

func TestExternalBridgeRejectsPublicUnauthenticatedBinding(t *testing.T) {
	for _, host := range []string{"0.0.0.0", "::", "8.8.8.8"} {
		if _, err := NewExternalBridge(ExternalBridgeOptions{BindHost: host}); err == nil {
			t.Fatalf("allowed public listener %q", host)
		}
	}
}

func TestGuestScriptsParseWithoutExecutingCloudEffects(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}
	for name, script := range map[string]string{"process": sandboxprocess.ControlScript, "group": groupScript, "stage": stageScript, "evidence": evidenceScript} {
		cmd := exec.Command(python, "-I", "-c", `import ast,sys;ast.parse(sys.stdin.read())`)
		cmd.Stdin = strings.NewReader(script)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s syntax: %s %v", name, output, err)
		}
	}
}

func TestExternalBridgeStartUsesExistingDialectAndIsolatesTools(t *testing.T) {
	b, err := NewExternalBridge(ExternalBridgeOptions{RunID: "run", TaskID: "task", SandboxID: "owned-handle", Workdir: "/workspace", OutputDir: t.TempDir(), ClientSourceFile: filepath.Join(t.TempDir(), "linux-client")})
	if err != nil {
		t.Fatal(err)
	}
	sandbox := &fakeSandbox{}
	endpoint, err := b.Start(context.Background(), sandbox)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Stop(context.Background()) })
	if endpoint.Protocol != "ssh" || endpoint.ClientCommand != "/opt/aries/bin/aries-ssh-client" || !sandbox.aliasPrepared {
		t.Fatal("native SSH endpoint contract not preserved")
	}
	client, err := gossh.Dial("tcp", endpoint.Address, &gossh.ClientConfig{User: "aries", HostKeyCallback: gossh.InsecureIgnoreHostKey(), Timeout: time.Second})
	if err != nil {
		t.Fatalf("native SSH handshake: %v", err)
	}
	_ = client.Close()
	if err := b.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sandbox.stopCalls != 1 {
		t.Fatal("tool descendants were not positively stopped")
	}
}

func TestExternalBridgeBindsAllocatedIdentityAndRejectsMismatch(t *testing.T) {
	options := ExternalBridgeOptions{RunID: "run", TaskID: "task", Workdir: "/workspace", OutputDir: t.TempDir(), ClientSourceFile: filepath.Join(t.TempDir(), "linux-client")}
	b, err := NewExternalBridge(options)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Start(context.Background(), &fakeSandbox{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Stop(context.Background()) })
	if b.options.SandboxID != "owned-handle" {
		t.Fatal("bridge used occurrence label as sandbox identity")
	}
	options.SandboxID = "different-sandbox"
	mismatch, err := NewExternalBridge(options)
	if err != nil {
		t.Fatal(err)
	}
	sandbox := &fakeSandbox{}
	if _, err := mismatch.Start(context.Background(), sandbox); err == nil {
		t.Fatal("accepted mismatched sandbox identity")
	}
	if sandbox.aliasPrepared || sandbox.stopCalls != 0 {
		t.Fatal("mismatch performed sandbox preparation")
	}
}

func testProcessGroup(t *testing.T, sandbox runner.Sandbox, workdir string) *sandboxprocess.Group {
	t.Helper()
	group, err := sandboxprocess.New(context.Background(), sandbox, workdir)
	if err != nil {
		t.Fatal(err)
	}
	return group
}

func TestPrepareFailureRetainsOpenClawGroupOwnership(t *testing.T) {
	sandbox := &fakeSandbox{prepareCode: 1}
	e := &Embedded{options: EmbeddedOptions{Workdir: "/workspace", CleanupTimeout: time.Second}}
	if _, err := e.Bridge().Start(context.Background(), sandbox); !errors.Is(err, ErrUnsupported) || e.group == nil {
		t.Fatalf("embedded partial group lost: %v", err)
	}
	if err := e.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	bridge, err := NewExternalBridge(ExternalBridgeOptions{RunID: "run", TaskID: "task", Workdir: "/workspace", OutputDir: t.TempDir(), ClientSourceFile: "/private/client"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bridge.Start(context.Background(), sandbox); !errors.Is(err, ErrUnsupported) || bridge.group == nil {
		t.Fatalf("external partial group lost: %v", err)
	}
	if err := bridge.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sandbox.stopCalls != 2 {
		t.Fatal("partial groups were not stopped")
	}
}

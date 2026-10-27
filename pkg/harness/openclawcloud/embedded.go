package openclawcloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/harness"
	"github.com/hyscale-lab/aries/pkg/harness/internal/privatefiles"
	"github.com/hyscale-lab/aries/pkg/harness/internal/sandboxprocess"
	"github.com/hyscale-lab/aries/pkg/harness/openclaw"
	"github.com/hyscale-lab/aries/pkg/harness/openclaw/gateway"
	"github.com/hyscale-lab/aries/pkg/performance"
	"github.com/hyscale-lab/aries/pkg/runner"
)

// GuestEndpoint resolves a provider-authenticated endpoint for the controller.
// Neither the URL nor headers are included in evidence or error messages.
type GuestEndpoint func(context.Context, runner.Sandbox, int) (string, http.Header, error)

type EmbeddedOptions struct {
	OutputDir            string
	Workdir              string
	OpenClawPath         string
	ExpectedVersion      string
	ModelProxyURL        string
	ProxyAccessToken     []byte
	GuestEndpoint        GuestEndpoint
	StartTimeout         time.Duration
	CleanupTimeout       time.Duration
	WorkloadInputs       *openclaw.WorkloadInputs
	ObservationLevel     performance.ObservationLevel
	ObserveNativeProcess bool
}

type gatewayConnection interface {
	Connect(context.Context, gateway.ConnectOptions) (gateway.ConnectSummary, error)
	Agent(context.Context, gateway.AgentRequest) (gateway.AgentResult, error)
	RecvEvent(context.Context) (gateway.Frame, error)
	Close() error
}

type Embedded struct {
	options                            EmbeddedOptions
	mu                                 sync.Mutex
	group                              *sandboxprocess.Group
	started, running, closed, isolated bool
	credentials                        *harness.Credentials
	model                              core.ModelConfig
	thinking                           string
	client                             gatewayConnection
	newGateway                         func(string, http.Header, []byte, *gateway.DeviceIdentity) (gatewayConnection, error)
	sessionKey, sessionID, idempotency string
	agentTimeout                       time.Duration
	processDone                        chan error
	processCancel                      context.CancelFunc
	runDone                            chan struct{}
	runCancel                          context.CancelFunc
	logs                               *boundedLog
	artifactDir                        string
	closeErr                           error
	nativeToolObservation              core.ObserverResult
	nativeToolObservationErr           error
}

var _ runner.AgentHarness = (*Embedded)(nil)

func NewEmbedded(options EmbeddedOptions) (*Embedded, error) {
	if options.OutputDir == "" || options.GuestEndpoint == nil || !path.IsAbs(options.OpenClawPath) || path.Clean(options.OpenClawPath) != options.OpenClawPath || options.ExpectedVersion == "" {
		return nil, errors.New("embedded OpenClaw requires output directory, guest endpoint resolver, absolute native executable and explicit version pin")
	}
	proxy, err := url.Parse(options.ModelProxyURL)
	if err != nil || proxy.Scheme != "https" || proxy.Hostname() == "" || proxy.User != nil || proxy.RawQuery != "" || proxy.Fragment != "" || strings.ContainsRune(options.ModelProxyURL, 0) {
		return nil, errors.New("embedded OpenClaw requires an authenticated external HTTPS model proxy")
	}
	if len(options.ProxyAccessToken) == 0 || bytes.ContainsAny(options.ProxyAccessToken, "\x00\r\n") {
		return nil, errors.New("embedded OpenClaw proxy session token is required")
	}
	if options.StartTimeout <= 0 {
		options.StartTimeout = time.Minute
	}
	if options.CleanupTimeout <= 0 {
		options.CleanupTimeout = 20 * time.Second
	}
	if options.ObservationLevel == "" {
		options.ObservationLevel = performance.ObservationL1
	}
	options.ProxyAccessToken = bytes.Clone(options.ProxyAccessToken)
	if options.WorkloadInputs != nil {
		inputs := *options.WorkloadInputs
		options.WorkloadInputs = &inputs
	}
	return &Embedded{options: options, newGateway: func(rawURL string, headers http.Header, token []byte, device *gateway.DeviceIdentity) (gatewayConnection, error) {
		dial, err := gateway.NewWebSocketDialer(gateway.WebSocketOptions{URL: rawURL, Headers: headers})
		if err != nil {
			return nil, err
		}
		return gateway.New(dial, gateway.Options{Token: string(token), Device: device, EventDisposition: gateway.EventDispositionDelivery, ToolEvents: performance.ObservesNative(options.ObservationLevel)})
	}}, nil
}

// Bridge binds the exact Runner-owned sandbox. Its Stop confirms containment;
// it is deliberately not a no-op, even though no network tool bridge exists.
func (e *Embedded) Bridge() runner.ToolBridge { return embeddedBridge{e} }

type embeddedBridge struct{ harness *Embedded }

func (b embeddedBridge) Start(ctx context.Context, sandbox runner.Sandbox) (core.ToolEndpoint, error) {
	e := b.harness
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.group != nil || e.closed {
		return core.ToolEndpoint{}, errors.New("embedded sandbox binding is single use")
	}
	group, err := sandboxprocess.New(ctx, sandbox, e.options.Workdir)
	e.group = group
	if err != nil {
		return core.ToolEndpoint{}, err
	}
	return core.ToolEndpoint{Protocol: "embedded", Workdir: e.options.Workdir}, nil
}

func (b embeddedBridge) Stop(ctx context.Context) error { return b.harness.Stop(ctx) }

func (e *Embedded) Start(ctx context.Context, request core.HarnessRequest) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.group == nil || e.started || e.closed {
		return errors.New("embedded harness requires an unused sandbox binding")
	}
	if request.Endpoint.Protocol != "embedded" || request.Endpoint.Workdir != e.options.Workdir {
		return errors.New("embedded harness endpoint does not match its sandbox binding")
	}
	if request.NoSandboxTools || len(request.Connectivity.MCPServers) != 0 || request.Connectivity.SearchURL != "" {
		return fmt.Errorf("%w: the initial embedded coding workload supports local exec only", ErrUnsupported)
	}
	if err := harness.ValidateRunID("OpenClaw", request.RunID); err != nil {
		return err
	}
	if err := harness.ValidateTaskID("OpenClaw", request.TaskID); err != nil {
		return err
	}
	if request.Timeout < 0 {
		return errors.New("embedded task timeout must not be negative")
	}
	e.agentTimeout = request.Timeout
	if e.agentTimeout == 0 {
		e.agentTimeout = 15 * time.Minute
	}
	e.model = request.Model
	e.thinking = openclaw.GatewayThinking(request.Model)
	e.model.BaseURL = e.options.ModelProxyURL
	e.model.APIKeyEnv = "ARIES_MODEL_PROXY_ACCESS_TOKEN"
	configuration, err := openclaw.RenderLocalConfig(e.model, e.options.Workdir, e.options.WorkloadInputs)
	if err != nil {
		return err
	}
	e.credentials = harness.NewCredentials("OpenClaw cloud")
	e.credentials.Set("proxy", e.options.ProxyAccessToken)
	clear(e.options.ProxyAccessToken)
	e.options.ProxyAccessToken = nil
	token, err := privatefiles.RandomSecret(32)
	if err != nil {
		return err
	}
	e.credentials.Set("gateway", token)
	clear(token)
	id, err := privatefiles.RandomID()
	if err != nil {
		return err
	}
	e.sessionKey, e.idempotency = "agent:main:aries-"+id, id
	if inputs := e.options.WorkloadInputs; inputs != nil {
		e.sessionKey, e.sessionID = inputs.SessionKey, inputs.SessionID
	}
	e.artifactDir = filepath.Join(e.options.OutputDir, request.TaskID, "harness")
	if err := privatefiles.EnsureDirectory(e.artifactDir); err != nil {
		return err
	}
	if err := privatefiles.WriteArtifact(filepath.Join(e.artifactDir, "openclaw.json"), configuration); err != nil {
		return err
	}
	launcher, proxy := openclaw.GatewayScripts()
	staging, err := json.Marshal(map[string]any{
		"configuration": string(configuration), "launcher": string(launcher), "proxy": string(proxy),
		"executable": e.options.OpenClawPath, "expected_version": e.options.ExpectedVersion,
		"proxy_token": string(e.credentials.Get("proxy")), "gateway_token": string(e.credentials.Get("gateway")),
		"workdir":                    e.options.Workdir,
		"native_process_observation": e.options.ObserveNativeProcess && performance.ObservesNative(e.options.ObservationLevel),
	})
	if err != nil {
		return err
	}
	defer clear(staging)
	result, err := e.group.Sandbox().Exec(ctx, core.Command{Path: "python3", Args: []string{"-I", "-c", stageScript, e.group.ControlDir()}, Dir: "/", User: "root", Stdin: staging, Timeout: e.options.StartTimeout, OutputLimitBytes: 4096})
	if err != nil {
		return e.credentials.RedactErr(err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%w: native executable, version pin or private staging validation failed", ErrUnsupported)
	}
	e.started = true
	e.logs = &boundedLog{max: 16 << 20}
	processCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	e.processCancel = cancel
	e.processDone = make(chan error, 1)
	group, logs, done := e.group, e.logs, e.processDone
	go func() {
		result, err := group.Sandbox().ExecStream(processCtx, core.Command{Path: "python3", Args: []string{"-I", "-c", groupScript, "gateway", group.ControlDir(), group.CgroupPath()}, Dir: "/", User: "root"}, nil, logs, logs)
		if err == nil && result.ExitCode != 0 {
			err = fmt.Errorf("native Gateway exited with code %d", result.ExitCode)
		}
		done <- err
		close(done)
	}()
	readyCtx, readyCancel := context.WithTimeout(ctx, e.options.StartTimeout)
	defer readyCancel()
	if err := e.waitReady(readyCtx); err != nil {
		return e.credentials.RedactErr(err)
	}
	rawURL, headers, err := e.options.GuestEndpoint(readyCtx, e.group.Sandbox(), 18789)
	if err != nil {
		return errors.New("resolve authenticated cloud Gateway endpoint failed")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || len(headers) == 0 {
		return errors.New("cloud Gateway endpoint must use HTTPS with provider access headers")
	}
	for name, values := range headers {
		for index, value := range values {
			e.credentials.Set(fmt.Sprintf("guest:%s:%d", name, index), []byte(value))
		}
	}
	parsed.Scheme = "wss"
	device, err := gateway.NewDeviceIdentity()
	if err != nil {
		return err
	}
	defer device.Destroy()
	e.client, err = e.newGateway(parsed.String(), headers, e.credentials.Get("gateway"), device)
	if err != nil {
		return e.credentials.RedactErr(err)
	}
	summary, err := e.client.Connect(readyCtx, gateway.ConnectOptions{})
	var pairing *gateway.PairingRequiredError
	if errors.As(err, &pairing) {
		if err := e.approveDevice(readyCtx, pairing.RequestID, device); err != nil {
			return e.credentials.RedactErr(err)
		}
		summary, err = e.client.Connect(readyCtx, gateway.ConnectOptions{})
	}
	if err != nil {
		return e.credentials.RedactErr(err)
	}
	if summary.Role != "operator" || !summary.HasScope("operator.write") {
		return errors.New("cloud Gateway requires operator.write scope for the operator role")
	}
	proof, err := e.group.Sandbox().Exec(readyCtx, core.Command{Path: "python3", Args: []string{"-I", "-c", evidenceScript, e.group.CgroupPath()}, Dir: "/", User: "root", OutputLimitBytes: 64 << 10})
	if err != nil {
		return e.credentials.RedactErr(err)
	}
	if proof.ExitCode != 0 || !json.Valid([]byte(proof.Stdout)) {
		return errors.New("native Gateway process location could not be observed")
	}
	if err := privatefiles.WriteArtifact(filepath.Join(e.artifactDir, "runtime-evidence.json"), e.credentials.Redact([]byte(proof.Stdout))); err != nil {
		return err
	}
	return nil
}

func (e *Embedded) waitReady(ctx context.Context) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		result, err := e.group.Sandbox().Exec(ctx, core.Command{Path: "python3", Args: []string{"-I", "-c", `import json,urllib.request; r=urllib.request.urlopen("http://127.0.0.1:18790/readyz",timeout=1); assert r.status==200 and json.load(r)["ready"] is True`}, Dir: "/", User: "root", Timeout: 2 * time.Second, OutputLimitBytes: 4096})
		if err == nil && result.ExitCode == 0 {
			return nil
		}
		select {
		case err := <-e.processDone:
			if err == nil {
				err = errors.New("native Gateway exited before readiness")
			}
			return err
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (e *Embedded) Run(ctx context.Context, instruction string) (result core.HarnessResult, err error) {
	started := time.Now()
	e.mu.Lock()
	if !e.started || e.closed || e.running || e.client == nil || strings.TrimSpace(instruction) == "" {
		e.mu.Unlock()
		return core.HarnessResult{Status: core.StatusFailed}, errors.New("embedded Gateway is not ready for a new task")
	}
	e.running = true
	e.runDone = make(chan struct{})
	runCtx, cancel := context.WithTimeout(ctx, e.agentTimeout)
	e.runCancel = cancel
	client, credentials := e.client, e.credentials.Snapshot()
	defer credentials.Clear()
	key, sessionID, idempotency, thinking, done := e.sessionKey, e.sessionID, e.idempotency, e.thinking, e.runDone
	e.mu.Unlock()
	defer close(done)
	defer cancel()
	var collector *openclaw.NativeToolCollector
	if performance.ObservesNative(e.options.ObservationLevel) {
		collector = openclaw.NewNativeToolCollector(key, credentials)
		collector.StartReading(runCtx, client)
	}
	runtimeStarted := time.Now()
	agentResult, err := client.Agent(runCtx, gateway.AgentRequest{Message: instruction, SessionKey: key, SessionID: sessionID, IdempotencyKey: idempotency, Thinking: thinking})
	runtimeFinished := time.Now()
	duration := runtimeFinished.Sub(runtimeStarted)
	err = credentials.RedactErr(err)
	result = core.HarnessResult{Status: core.StatusSucceeded, FinalResponse: string(credentials.Redact([]byte(agentResult.Text))), RuntimeStartedAt: &runtimeStarted, RuntimeFinishedAt: &runtimeFinished, RuntimeExecutionDuration: &duration, SessionID: key}
	if collector != nil {
		report, observationErr := collector.FinishObservation(agentResult.RunID, e.artifactDir, 2*time.Second)
		result.LogPaths = append(result.LogPaths, report.LogPaths...)
		e.mu.Lock()
		e.nativeToolObservation, e.nativeToolObservationErr = report, observationErr
		e.mu.Unlock()
	}
	artifact, writeErr := json.MarshalIndent(struct {
		NativeRunID string `json:"native_run_id,omitempty"`
		SessionKey  string `json:"session_key"`
		SessionID   string `json:"requested_native_session_id,omitempty"`
		Response    string `json:"response"`
	}{agentResult.RunID, key, sessionID, result.FinalResponse}, "", "  ")
	if writeErr == nil {
		p := filepath.Join(e.artifactDir, "agent-result.json")
		writeErr = privatefiles.WriteArtifact(p, credentials.Redact(artifact))
		if writeErr == nil {
			result.LogPaths = append(result.LogPaths, p)
		}
	}
	err = errors.Join(err, writeErr)
	if err != nil {
		result.Status, result.Error = core.StatusFailed, err.Error()
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			result.Status = core.StatusCanceled
		}
	}
	result.Duration = time.Since(started)
	return result, err
}

// NativeToolObservation is independent of native Agent and containment results.
func (e *Embedded) NativeToolObservation() (core.ObserverResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !performance.ObservesNative(e.options.ObservationLevel) {
		return core.ObserverResult{Status: core.StatusNotEnabled}, nil
	}
	if e.nativeToolObservation.Status == "" {
		return core.ObserverResult{Status: core.StatusNotRun, Reason: "native tool evidence was not collected"}, nil
	}
	report := e.nativeToolObservation
	report.LogPaths = append([]string(nil), report.LogPaths...)
	return report, e.nativeToolObservationErr
}

// Stop closes local admission, kills the contained Gateway and all descendants,
// and joins the controller-side call. It leaves the workspace live for verifiers.
func (e *Embedded) Stop(ctx context.Context) error {
	e.mu.Lock()
	e.closed = true
	if e.isolated {
		err := e.closeErr
		e.mu.Unlock()
		return err
	}
	group, client, cancel, runDone := e.group, e.client, e.runCancel, e.runDone
	processCancel, processDone := e.processCancel, e.processDone
	e.mu.Unlock()
	cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), e.options.CleanupTimeout)
	defer cleanupCancel()
	if cancel != nil {
		cancel()
	}
	if client != nil {
		if err := client.Close(); err != nil {
			e.mu.Lock()
			// The transport may clear its handle even when Close fails; preserve
			// that failure across retries while still completing containment.
			e.closeErr = errors.New("close native Gateway transport failed")
			e.mu.Unlock()
		}
	}
	e.mu.Lock()
	closeErr := e.closeErr
	e.mu.Unlock()
	if group != nil {
		if err := group.Stop(cleanupCtx); err != nil {
			return errors.Join(closeErr, err)
		}
	}
	if processCancel != nil {
		processCancel()
	}
	for _, done := range []<-chan struct{}{runDone} {
		if done != nil {
			select {
			case <-done:
			case <-cleanupCtx.Done():
				return errors.Join(closeErr, cleanupCtx.Err())
			}
		}
	}
	if processDone != nil {
		select {
		case <-processDone:
		case <-cleanupCtx.Done():
			return errors.Join(closeErr, cleanupCtx.Err())
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.credentials != nil {
		if e.logs != nil {
			if err := privatefiles.WriteArtifact(filepath.Join(e.artifactDir, "gateway.log"), e.credentials.Redact(e.logs.Bytes())); err != nil {
				return errors.Join(closeErr, err)
			}
		}
		e.credentials.Clear()
	}
	clear(e.options.ProxyAccessToken)
	e.options.ProxyAccessToken = nil
	e.isolated = true
	return e.closeErr
}

type boundedLog struct {
	mu        sync.Mutex
	content   []byte
	max       int
	truncated bool
}

func (b *boundedLog) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := min(len(p), b.max-len(b.content))
	b.content = append(b.content, p[:n]...)
	b.truncated = b.truncated || n < len(p)
	return len(p), nil
}
func (b *boundedLog) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.truncated {
		// Drop the incomplete final line before redaction, so truncation cannot
		// leave a prefix of a credential whose complete value would be scrubbed.
		last := bytes.LastIndexByte(b.content, '\n')
		return append(bytes.Clone(b.content[:last+1]), []byte("[ARIES gateway log truncated]\n")...)
	}
	return bytes.Clone(b.content)
}

var _ io.Writer = (*boundedLog)(nil)

const stageScript = `import json, os, pathlib, re, stat, subprocess, sys
control = pathlib.Path(sys.argv[1]); spec = json.load(sys.stdin)
exe = pathlib.Path(spec["executable"]).resolve(strict=True)
for p in [exe] + list(exe.parents):
    s = p.stat()
    if s.st_uid != 0 or s.st_mode & 0o022: raise RuntimeError("native installation must be root owned")
version = subprocess.run([str(exe),"--version"],capture_output=True,text=True,timeout=15,env={"PATH":"/usr/local/bin:/usr/bin:/bin"})
expected = re.escape(spec["expected_version"])
if version.returncode != 0 or not re.fullmatch(r"(?:OpenClaw )?"+expected+r"(?: \([0-9a-f]+\))?",version.stdout.strip()): raise RuntimeError("native version does not match pin")
runtime = pathlib.Path("/run/aries")
runtime.mkdir(mode=0o755)
home = runtime / "home"; home.mkdir(mode=0o700); os.chown(home,60000,60000)
def write(p, content, mode, owner=0):
    fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL,mode)
    with os.fdopen(fd,"w") as f: f.write(content)
    os.chown(p,owner,owner)
write(runtime/"openclaw.json",spec["configuration"],0o400,60000)
write(runtime/"gateway-proxy.js",spec["proxy"],0o555)
write(runtime/"gateway-launcher",spec["launcher"],0o555)
env={"HOME":str(home),"OPENCLAW_CONFIG_PATH":str(runtime/"openclaw.json"),"OPENCLAW_GATEWAY_TOKEN":spec["gateway_token"],"ARIES_MODEL_PROXY_ACCESS_TOKEN":spec["proxy_token"],"ARIES_OPENCLAW_EXECUTABLE":spec["executable"],"PATH":"/usr/local/bin:/usr/bin:/bin"}
if spec.get("native_process_observation"): env["ARIES_NATIVE_PROCESS_OBSERVATION"]="1"
launch={"path":"/bin/sh","args":[str(runtime/"gateway-launcher")],"dir":spec["workdir"],"env":env}
write(control/"gateway.json",json.dumps(launch),0o600)
`

const evidenceScript = `import json, os, pathlib, sys
group = pathlib.Path(sys.argv[1]); processes = []
for pid in (group/"cgroup.procs").read_text().split():
    proc = pathlib.Path("/proc")/pid
    try:
        status = dict(line.split(":",1) for line in (proc/"status").read_text().splitlines() if ":" in line)
        uids = [int(v) for v in status["Uid"].split()]
        if any(uid != 60000 for uid in uids): raise RuntimeError("unexpected contained process identity")
        processes.append({"pid":int(pid),"uids":uids,"executable":os.readlink(proc/"exe")})
    except FileNotFoundError: pass
if not processes: raise RuntimeError("no live contained Gateway process")
print(json.dumps({"cgroup":str(group),"hostname":os.uname().nodename,"processes":processes,"clock_domain":"guest_observation_no_cross_clock_duration"}))
`

package jiuwenswarm

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/harness"
	"github.com/hyscale-lab/aries/pkg/harness/internal/privatefiles"
	"github.com/hyscale-lab/aries/pkg/harness/internal/sandboxprocess"
	"github.com/hyscale-lab/aries/pkg/performance"
	"github.com/hyscale-lab/aries/pkg/runner"
)

const NativeRevision = "e2c9d519498080e249e34f16a80c8c97f0cd1962"
const LockSHA256 = "f28fbbd9b208bcf4f6e421818969ed2b6570f0648bed25c2ed15ee5bd551e5c9"

//go:embed bootstrap.py
var bootstrap string

//go:embed native_audit.py
var nativeAudit string

//go:embed native_process_extension.py
var nativeProcessExtension string

//go:embed model_branches.py
var modelBranchesExtension string

type EmbeddedOptions struct {
	OutputDir, Workdir, SourceDir, PythonPath, ModelProxyURL string
	ModelSessionProtocol                                     string
	ProxyAccessToken                                         []byte
	GuestEndpoint                                            func(context.Context, runner.Sandbox, int) (string, http.Header, error)
	StartTimeout, CleanupTimeout                             time.Duration
	ObservationLevel                                         performance.ObservationLevel
}

type nativeClient interface {
	Health(context.Context, string) error
	CreateSession(context.Context, string, string) (json.RawMessage, error)
	Run(context.Context, string, string, string, func(Frame)) error
	Close()
}

// Embedded borrows exactly the Runner-owned sandbox. Native LOCAL tools execute
// there, inside the same contained process group as AgentServer and its children.
type Embedded struct{ *nativeRuntime }

// nativeRuntime shares the native session and evidence, not placement ownership.
// Embedded owns a guest process group; External owns an ECS container.
type nativeRuntime struct {
	options                                           EmbeddedOptions
	mu, stopMu                                        sync.Mutex
	group                                             *sandboxprocess.Group
	sandbox                                           runner.Sandbox
	runID, taskID                                     string
	started, ready, running, closed, isolated         bool
	runtimeDir, artifactDir, requestPrefix, sessionID string
	timeout                                           time.Duration
	credentials                                       *harness.Credentials
	client                                            nativeClient
	newClient                                         func(string, http.Header) (nativeClient, error)
	processCancel, runCancel                          context.CancelFunc
	processDone                                       chan error
	runDone                                           chan struct{}
	logs                                              *evidenceBuffer
	observation                                       core.ObserverResult
	observationErr                                    error
	nativeCollected                                   bool
	providerObservationExpected                       bool
	processObservationSupported                       bool
	nativePID                                         int
	nativeStartTicks                                  string
	nativeStop                                        nativeStopReceipt
	resources                                         *cgroupResourceCollection
}

var _ runner.AgentHarness = (*Embedded)(nil)

func NewEmbedded(options EmbeddedOptions) (*Embedded, error) {
	if options.GuestEndpoint == nil {
		return nil, errors.New("Jiuwen embedded requires an authenticated guest resolver")
	}
	native, err := newNativeRuntime(options)
	if err != nil {
		return nil, err
	}
	return &Embedded{nativeRuntime: native}, nil
}

func newNativeRuntime(options EmbeddedOptions) (*nativeRuntime, error) {
	if options.ModelSessionProtocol != "" && options.ModelSessionProtocol != "jiuwen-tool-branches-v1" {
		return nil, errors.New("unsupported Jiuwen model session protocol")
	}
	for _, value := range []string{options.Workdir, options.SourceDir, options.PythonPath} {
		if !path.IsAbs(value) || path.Clean(value) != value || value == "/" || strings.ContainsRune(value, 0) {
			return nil, errors.New("Jiuwen requires clean absolute workspace, source and Python paths")
		}
	}
	proxy, err := url.Parse(options.ModelProxyURL)
	if err != nil || proxy.Scheme != "https" || proxy.Hostname() == "" || proxy.User != nil || proxy.RawQuery != "" || proxy.Fragment != "" {
		return nil, errors.New("Jiuwen cloud requires an external HTTPS model proxy")
	}
	if options.OutputDir == "" || len(options.ProxyAccessToken) == 0 || bytes.ContainsAny(options.ProxyAccessToken, "\x00\r\n") {
		return nil, errors.New("Jiuwen requires output directory, authenticated guest resolver and model proxy token")
	}
	if options.StartTimeout <= 0 {
		options.StartTimeout = 90 * time.Second
	}
	if options.CleanupTimeout <= 0 {
		options.CleanupTimeout = 30 * time.Second
	}
	if options.ObservationLevel == "" {
		options.ObservationLevel = performance.ObservationL1
	}
	options.ProxyAccessToken = bytes.Clone(options.ProxyAccessToken)
	return &nativeRuntime{options: options, newClient: func(endpoint string, headers http.Header) (nativeClient, error) { return NewClient(endpoint, headers) }}, nil
}

func (e *Embedded) Bridge() runner.ToolBridge { return embeddedBridge{e} }

type embeddedBridge struct{ harness *Embedded }

func (b embeddedBridge) Start(ctx context.Context, sandbox runner.Sandbox) (core.ToolEndpoint, error) {
	e := b.harness
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.group != nil || e.closed {
		return core.ToolEndpoint{}, errors.New("Jiuwen sandbox binding is single use")
	}
	group, err := sandboxprocess.New(ctx, sandbox, e.options.Workdir)
	// A partial prepare remains owned, including an ambiguous RPC failure.
	e.group = group
	e.sandbox = sandbox
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
		return errors.New("Jiuwen requires an unused sandbox binding")
	}
	if request.Endpoint.Protocol != "embedded" || request.Endpoint.Workdir != e.options.Workdir {
		return errors.New("Jiuwen endpoint does not match its sandbox")
	}
	if request.NoSandboxTools || len(request.Connectivity.MCPServers) != 0 || request.Connectivity.SearchURL != "" {
		return fmt.Errorf("%w: initial Jiuwen placement supports native local coding tools", sandboxprocess.ErrUnsupported)
	}
	if err := harness.ValidateRunID("Jiuwen", request.RunID); err != nil {
		return err
	}
	if err := harness.ValidateTaskID("Jiuwen", request.TaskID); err != nil {
		return err
	}
	if request.Timeout < 0 || request.Model.Model == "" {
		return errors.New("Jiuwen model and nonnegative timeout are required")
	}
	if request.Model.ReasoningEffort != "" || request.Model.MaxTokens != 0 || request.Model.ContextLength != 0 || request.Model.Temperature != nil {
		return errors.New("Jiuwen model generation overrides have not been mapped to the pinned native configuration")
	}
	e.started = true
	e.runID, e.taskID = request.RunID, request.TaskID
	e.timeout = request.Timeout
	if e.timeout == 0 {
		e.timeout = harness.DefaultAgentTimeout
	}
	id, err := privatefiles.RandomID()
	if err != nil {
		return err
	}
	e.requestPrefix = "aries-" + id
	e.runtimeDir = "/run/aries-jiuwen-" + id
	e.artifactDir = filepath.Join(e.options.OutputDir, request.TaskID, "harness")
	if err := privatefiles.EnsureDirectory(e.artifactDir); err != nil {
		return err
	}
	e.resources = &cgroupResourceCollection{runID: request.RunID, taskID: request.TaskID}
	e.collectCgroupResources(ctx, "start")
	e.credentials = harness.NewCredentials("Jiuwen cloud")
	e.credentials.Set("proxy", e.options.ProxyAccessToken)
	clear(e.options.ProxyAccessToken)
	e.options.ProxyAccessToken = nil
	stage, err := json.Marshal(map[string]any{"source_dir": e.options.SourceDir, "python_path": e.options.PythonPath, "workdir": e.options.Workdir,
		"model_proxy_url": e.options.ModelProxyURL, "proxy_token": string(e.credentials.Get("proxy")), "model": request.Model.Model, "bootstrap": bootstrap,
		"native_observation": nativeSpansEnabled(e.options.ObservationLevel), "native_audit": nativeAudit,
		"model_session_protocol": e.options.ModelSessionProtocol, "model_branches_extension": modelBranchesExtension,
		"native_process_observation": nativeSpansEnabled(e.options.ObservationLevel), "native_process_extension": nativeProcessExtension,
		"observation_binding": map[string]any{"schema_version": 1, "run_id": request.RunID, "task_id": request.TaskID}})
	if err != nil {
		return err
	}
	defer clear(stage)
	startCtx, cancel := context.WithTimeout(ctx, e.options.StartTimeout)
	defer cancel()
	result, err := e.group.Sandbox().Exec(startCtx, core.Command{Path: "python3", Args: []string{"-I", "-c", bootstrap, "stage", e.runtimeDir}, User: "root", Dir: "/", Stdin: stage, OutputLimitBytes: 4096})
	if err != nil {
		return e.credentials.RedactErr(err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%w: Jiuwen pinned installation or private staging failed", sandboxprocess.ErrUnsupported)
	}
	e.logs = &evidenceBuffer{max: 64 << 20}
	processCtx, processCancel := context.WithCancel(context.WithoutCancel(ctx))
	e.processCancel, e.processDone = processCancel, make(chan error, 1)
	group, done, logs := e.group, e.processDone, e.logs
	command := core.Command{Path: e.options.PythonPath, Args: []string{"-I", path.Join(e.runtimeDir, "launch.py"), "run", e.runtimeDir}, Dir: e.options.Workdir}
	go func() {
		result, err := group.ExecStream(processCtx, command, nil, logs, logs)
		if err == nil && result.ExitCode != 0 {
			err = fmt.Errorf("Jiuwen AgentServer exited with code %d", result.ExitCode)
		}
		done <- err
		close(done)
	}()
	endpoint, headers, err := e.options.GuestEndpoint(startCtx, e.group.Sandbox(), 18866)
	if err != nil {
		return errors.New("resolve authenticated Jiuwen guest endpoint failed")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || len(headers) == 0 {
		return errors.New("Jiuwen guest endpoint must use HTTPS with provider access headers")
	}
	e.credentials.AddGuestHeaders(headers)
	e.client, err = e.newClient(endpoint, headers)
	if err != nil {
		return e.credentials.RedactErr(err)
	}
	if err := e.waitReady(startCtx); err != nil {
		return e.credentials.RedactErr(err)
	}
	proof, err := e.group.Sandbox().Exec(startCtx, core.Command{Path: "python3", Args: []string{"-I", "-c", preparedScript, e.runtimeDir, e.group.CgroupPath()}, User: "root", Dir: "/", OutputLimitBytes: 64 << 10})
	if err != nil {
		return e.credentials.RedactErr(err)
	}
	var prepared struct {
		Revision         string `json:"revision"`
		Lock             string `json:"uv_lock_sha256"`
		Project          string `json:"project_id"`
		Workdir          string `json:"workdir"`
		Mode             string `json:"mode"`
		PID              int    `json:"pid"`
		StartTicks       string `json:"process_start_ticks"`
		ProcessSupported *bool  `json:"native_process_observation_supported"`
	}
	if proof.ExitCode != 0 || json.Unmarshal([]byte(proof.Stdout), &prepared) != nil || prepared.Revision != NativeRevision || prepared.Lock != LockSHA256 || prepared.Project == "" || prepared.Workdir != e.options.Workdir || prepared.Mode != "flash" || prepared.PID <= 1 || prepared.StartTicks == "" || prepared.ProcessSupported == nil || (*prepared.ProcessSupported && !nativeSpansEnabled(e.options.ObservationLevel)) {
		return errors.New("Jiuwen native configuration or process containment evidence is invalid")
	}
	e.nativePID, e.nativeStartTicks = prepared.PID, prepared.StartTicks
	e.processObservationSupported = *prepared.ProcessSupported
	if err := privatefiles.WriteArtifact(filepath.Join(e.artifactDir, "runtime-evidence.json"), e.credentials.Redact([]byte(proof.Stdout))); err != nil {
		return err
	}
	raw, err := e.client.CreateSession(startCtx, e.requestPrefix+"-session", prepared.Project)
	if err != nil {
		return e.credentials.RedactErr(err)
	}
	var session struct {
		ID      string `json:"session_id"`
		Workdir string `json:"projectDir"`
	}
	if json.Unmarshal(raw, &session) != nil || session.ID == "" || session.Workdir != e.options.Workdir {
		return errors.New("Jiuwen session did not bind to the prepared project")
	}
	e.sessionID = session.ID
	e.ready = true
	return nil
}

func (e *nativeRuntime) waitReady(ctx context.Context) error {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		probe, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := e.client.Health(probe, e.requestPrefix+"-health")
		cancel()
		if err == nil {
			return nil
		}
		select {
		case err := <-e.processDone:
			if err == nil {
				err = errors.New("Jiuwen AgentServer exited before readiness")
			}
			return err
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (e *nativeRuntime) Run(ctx context.Context, instruction string) (core.HarnessResult, error) {
	started := time.Now()
	e.mu.Lock()
	if !e.ready || e.closed || e.running || strings.TrimSpace(instruction) == "" || strings.ContainsRune(instruction, 0) {
		e.mu.Unlock()
		return core.HarnessResult{Status: core.StatusFailed}, errors.New("Jiuwen is not ready for a new task instruction")
	}
	e.running = true
	runCtx, cancel := context.WithTimeout(ctx, e.timeout)
	e.runCancel, e.runDone = cancel, make(chan struct{})
	client, credentials, done, session, requestID := e.client, e.credentials.Snapshot(), e.runDone, e.sessionID, e.requestPrefix+"-run"
	e.mu.Unlock()
	defer close(done)
	defer cancel()
	defer credentials.Clear()
	observation := newFrameEvidence(e.artifactDir, credentials, performance.ObservesNative(e.options.ObservationLevel))
	observation.officialSpansEnabled = nativeSpansEnabled(e.options.ObservationLevel)
	e.collectCgroupResources(runCtx, "runtime_start")
	runtimeStarted := time.Now()
	err := client.Run(runCtx, requestID, session, instruction, observation.consume)
	runtimeFinished := time.Now()
	duration := runtimeFinished.Sub(runtimeStarted)
	e.collectCgroupResources(runCtx, "runtime_end")
	report, observationErr := observation.finish(err == nil)
	e.mu.Lock()
	e.observation, e.observationErr = report, observationErr
	e.mu.Unlock()
	err = credentials.RedactErr(err)
	result := core.HarnessResult{Status: core.StatusSucceeded, SessionID: session, FinalResponse: string(credentials.Redact(observation.text.Bytes())),
		RuntimeStartedAt: &runtimeStarted, RuntimeFinishedAt: &runtimeFinished, RuntimeExecutionDuration: &duration, Duration: time.Since(started), LogPaths: report.LogPaths}
	if err != nil {
		result.Status, result.Error = core.StatusFailed, err.Error()
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			result.Status = core.StatusCanceled
		}
	}
	return result, err
}

func (e *nativeRuntime) NativeToolObservation() (core.ObserverResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !performance.ObservesNative(e.options.ObservationLevel) && e.observationErr == nil {
		return core.ObserverResult{Status: core.StatusNotEnabled}, nil
	}
	if e.observation.Status == "" {
		return core.ObserverResult{Status: core.StatusNotRun, Reason: "Jiuwen native stream was not collected"}, e.observationErr
	}
	result := e.observation
	result.LogPaths = append([]string(nil), result.LogPaths...)
	return result, e.observationErr
}

// Stop does not treat an interrupt acknowledgement or lost HTTP connection as
// isolation. It closes admission, confirms cgroup emptiness and joins both calls.
func (e *Embedded) Stop(ctx context.Context) error {
	e.stopMu.Lock()
	defer e.stopMu.Unlock()
	e.mu.Lock()
	e.closed = true
	if e.isolated {
		e.mu.Unlock()
		return nil
	}
	group, client, runCancel, runDone := e.group, e.client, e.runCancel, e.runDone
	processCancel, processDone := e.processCancel, e.processDone
	e.mu.Unlock()
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), e.options.CleanupTimeout)
	defer cancel()
	if runCancel != nil {
		runCancel()
	}
	if client != nil {
		client.Close()
	}
	// Native SIGTERM requests exporter draining. This never substitutes for the
	// cgroup kill/emptiness confirmation and joined calls below.
	if e.nativePID > 1 && !e.nativeStop.Attempted {
		e.nativeStop = e.gracefulNativeStop(cleanup)
	}
	if group != nil {
		if err := group.Stop(cleanup); err != nil {
			return err
		}
	}
	if processCancel != nil {
		processCancel()
	}
	if runDone != nil {
		select {
		case <-runDone:
		case <-cleanup.Done():
			return cleanup.Err()
		}
	}
	if processDone != nil {
		select {
		case <-processDone:
		case <-cleanup.Done():
			return cleanup.Err()
		}
	}
	// Collection cannot weaken isolation or prevent private-runtime cleanup.
	if e.resources != nil {
		deadline, _ := cleanup.Deadline()
		budget := min(2*time.Second, time.Until(deadline)-min(2*time.Second, e.options.CleanupTimeout/4))
		resourceCtx, resourceCancel := context.WithTimeout(cleanup, budget)
		e.collectCgroupResources(resourceCtx, "stopped")
		resourceCancel()
		e.finishCgroupResources()
	}
	if !e.nativeCollected && e.sessionID != "" {
		deadline, _ := cleanup.Deadline()
		budget := time.Until(deadline) - min(2*time.Second, e.options.CleanupTimeout/4)
		collection, stopCollection := context.WithTimeout(cleanup, budget)
		paths, err := e.collectNativeSpans(collection)
		stopCollection()
		e.mu.Lock()
		e.nativeCollected = true
		e.observation.LogPaths = append(e.observation.LogPaths, paths...)
		if nativeSpansEnabled(e.options.ObservationLevel) {
			e.observation.Reason = "native SSE and official spans; sampled SDK audit has separate coverage; LOCAL process timing unsupported by this installation"
			if e.processObservationSupported {
				e.observation.Reason = "native SSE and official spans; sampled SDK audit and LOCAL process evidence have separate clocks, identities and coverage"
			}
		}
		if err != nil {
			e.observationErr = errors.Join(e.observationErr, e.credentials.RedactErr(err))
			e.observation.Status, e.observation.Error = core.StatusFailed, e.observationErr.Error()
		}
		e.mu.Unlock()
	}
	if e.runtimeDir != "" {
		result, err := group.Sandbox().Exec(cleanup, core.Command{Path: "python3", Args: []string{"-I", "-c", `import pathlib,shutil,sys; p=pathlib.Path(sys.argv[1]); assert p.parent==pathlib.Path('/run') and p.name.startswith('aries-jiuwen-'); shutil.rmtree(p) if p.exists() else None`, e.runtimeDir}, User: "root", Dir: "/", OutputLimitBytes: 4096})
		if err != nil {
			return errors.New("Jiuwen private runtime cleanup failed")
		}
		if result.ExitCode != 0 {
			return errors.New("Jiuwen private runtime cleanup is unconfirmed")
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.logs != nil {
		logPath := filepath.Join(e.artifactDir, "agentserver.log")
		content := e.credentials.Redact(e.logs.Bytes())
		err := privatefiles.WriteArtifact(logPath, content)
		if e.logs.Truncated() {
			err = errors.Join(err, errors.New("Jiuwen AgentServer log exceeded 16 MiB"))
		}
		if err != nil {
			e.observationErr = errors.Join(e.observationErr, err)
			e.observation.Status = core.StatusFailed
			e.observation.Error = e.observationErr.Error()
		} else {
			e.observation.LogPaths = append(e.observation.LogPaths, logPath)
		}
	}
	if e.credentials != nil {
		e.credentials.Clear()
	}
	clear(e.options.ProxyAccessToken)
	e.options.ProxyAccessToken = nil
	e.isolated = true
	return nil
}

const preparedScript = `import json,pathlib,sys
runtime,group=map(pathlib.Path,sys.argv[1:3])
proof=json.loads((runtime/'home/prepared.json').read_text())
assert proof['uid']==60000 and proof['sandbox_enabled'] is False and proof['sandbox_startup_mode']=='external' and proof['skill_selection'] is False
assert type(proof['native_process_observation_supported']) is bool
if proof['native_process_observation_supported']:
 spec=json.loads((runtime/'spec.json').read_text())
 process=json.loads((runtime/'home/data/aries-process/process-index.json').read_text())
 assert process['schema_version']==1 and process['source']=='jiuwen_native_process'
 assert process['binding']=={key:spec['observation_binding'][key] for key in ('run_id','task_id')}
 assert process['observer_closed'] is False
 assert all(process['counters'][key]==0 for key in ('dropped','write_failed','index_write_failed','late_events','identity_failed'))
 assert (runtime/'home/data/aries-process/process-events.jsonl').is_file()
pids=[int(pid) for pid in (group/'cgroup.procs').read_text().split()]
assert proof['pid'] in pids
proof['contained_pids']=pids
proof['containment_cgroup']=str(group)
proof['process_start_ticks']=(pathlib.Path('/proc')/str(proof['pid'])/'stat').read_text().rsplit(')',1)[1].split()[19]
print(json.dumps(proof))
`

func (e *nativeRuntime) gracefulNativeStop(ctx context.Context) nativeStopReceipt {
	receipt := nativeStopReceipt{Attempted: true}
	graceful, cancel := context.WithTimeout(ctx, min(10*time.Second, e.options.CleanupTimeout/3))
	defer cancel()
	groupPath := ""
	if e.group != nil {
		groupPath = e.group.CgroupPath()
	}
	result, err := e.sandbox.Exec(graceful, core.Command{Path: "python3", Args: []string{"-I", "-c", nativeTerminateScript, strconv.Itoa(e.nativePID), e.nativeStartTicks, groupPath}, User: "root", Dir: "/", OutputLimitBytes: 4096})
	if err != nil || result.ExitCode != 0 || (strings.TrimSpace(result.Stdout) != "SIGTERM_SENT" && strings.TrimSpace(result.Stdout) != "ALREADY_ABSENT") {
		receipt.Error = "native graceful signal failed or process identity differs; cgroup fallback required"
		return receipt
	}
	receipt.SignalSent = strings.TrimSpace(result.Stdout) == "SIGTERM_SENT"
	select {
	case err := <-e.processDone:
		receipt.ExitObserved, receipt.CleanExit = true, err == nil
	case <-graceful.Done():
		receipt.Error = "native graceful exit timed out; cgroup fallback required"
	}
	return receipt
}

const nativeTerminateScript = `import os,pathlib,signal,sys
pid=int(sys.argv[1]); expected=sys.argv[2]; group=sys.argv[3]
process=pathlib.Path('/proc')/str(pid)
try:
 assert process.stat().st_uid==60000
 assert process.joinpath('stat').read_text().rsplit(')',1)[1].split()[19]==expected
 if group: assert str(pid) in (pathlib.Path(group)/'cgroup.procs').read_text().split()
 os.kill(pid,signal.SIGTERM)
 print('SIGTERM_SENT')
except ProcessLookupError: print('ALREADY_ABSENT')
except FileNotFoundError:
 assert not process.exists()
 print('ALREADY_ABSENT')
`

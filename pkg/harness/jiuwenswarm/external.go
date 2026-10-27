package jiuwenswarm

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"path/filepath"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/deployment"
	"github.com/hyscale-lab/aries/pkg/harness"
	"github.com/hyscale-lab/aries/pkg/harness/internal/privatefiles"
	"github.com/hyscale-lab/aries/pkg/performance"
	"github.com/hyscale-lab/aries/pkg/runner"
)

//go:embed remote/extension.py remote/provider.py remote/identity.py remote/wire.py
var remoteProviderSources embed.FS

type ExternalOptions struct {
	Runtime                                       harness.RuntimeOptions
	Bridge                                        *RemoteBridge
	Workdir, SourceDir, PythonPath, ModelProxyURL string
	ModelSessionProtocol                          string
	ProxyAccessToken                              []byte
	ObservationLevel                              performance.ObservationLevel
}

// External owns only the ECS Agent container. RemoteBridge lends the separately
// Runner-owned cloud sandbox through Jiuwen's official SANDBOX provider API.
type External struct {
	*nativeRuntime
	runtime    *harness.Runtime
	bridge     *RemoteBridge
	occurrence *harness.Occurrence
}

var _ runner.AgentHarness = (*External)(nil)

func NewExternal(options ExternalOptions) (*External, error) {
	if options.Bridge == nil {
		return nil, errors.New("Jiuwen external requires its remote bridge")
	}
	if options.Runtime.StartTimeout <= 0 {
		options.Runtime.StartTimeout = 90 * time.Second
	}
	native, err := newNativeRuntime(EmbeddedOptions{OutputDir: options.Runtime.OutputDir, Workdir: options.Workdir,
		SourceDir: options.SourceDir, PythonPath: options.PythonPath, ModelProxyURL: options.ModelProxyURL,
		ModelSessionProtocol: options.ModelSessionProtocol,
		ProxyAccessToken:     options.ProxyAccessToken, ObservationLevel: options.ObservationLevel,
		StartTimeout: options.Runtime.StartTimeout, CleanupTimeout: options.Runtime.CleanupTimeout})
	if err != nil {
		return nil, err
	}
	runtime, err := harness.NewRuntime("Jiuwen external", options.Runtime)
	if err != nil {
		clear(native.options.ProxyAccessToken)
		return nil, err
	}
	native.providerObservationExpected = nativeSpansEnabled(options.ObservationLevel)
	return &External{nativeRuntime: native, runtime: runtime, bridge: options.Bridge}, nil
}

func (e *External) Start(ctx context.Context, request core.HarnessRequest) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.started || e.closed {
		return errors.New("Jiuwen external harness is single use")
	}
	binding, err := e.bridge.ClientConfig()
	if err != nil {
		return err
	}
	if request.RunID != binding.RunID || request.TaskID != binding.TaskID || request.Endpoint.Protocol != "jiuwen-remote-v1" ||
		request.Endpoint.Address != binding.BaseURL || request.Endpoint.Workdir != binding.Workdir || binding.Workdir != e.options.Workdir {
		return errors.New("Jiuwen external endpoint does not match the owned remote sandbox")
	}
	if request.NoSandboxTools || len(request.Connectivity.MCPServers) != 0 || request.Connectivity.SearchURL != "" {
		return errors.New("Jiuwen external currently supports native coding tools")
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
	e.started, e.runID, e.taskID = true, request.RunID, request.TaskID
	e.timeout = request.Timeout
	if e.timeout == 0 {
		e.timeout = e.runtime.Options.AgentTimeout
	}
	id, err := privatefiles.RandomID()
	if err != nil {
		return err
	}
	e.requestPrefix, e.runtimeDir = "aries-"+id, "/run/aries-jiuwen-"+id
	e.artifactDir = filepath.Join(e.options.OutputDir, request.TaskID, "harness")
	if err := privatefiles.EnsureDirectory(e.artifactDir); err != nil {
		return err
	}
	e.credentials = harness.NewCredentials("Jiuwen external")
	e.credentials.Set("proxy", e.options.ProxyAccessToken)
	e.credentials.Set("bridge", []byte(binding.AccessToken))
	clear(e.options.ProxyAccessToken)
	e.options.ProxyAccessToken = nil
	e.occurrence = &harness.Occurrence{RunID: request.RunID, TaskID: request.TaskID, AttemptID: id,
		Name: core.RuntimeName("jiuwen", request.TaskID, id), ArtifactDir: e.artifactDir}
	if err := e.runtime.Own(e.occurrence); err != nil {
		return err
	}
	start, cancel := context.WithTimeout(ctx, e.options.StartTimeout)
	defer cancel()
	archive, err := harness.StageArchive(nil, nil)
	if err != nil {
		return err
	}
	defer clear(archive)
	container := deployment.Request{Name: e.occurrence.Name, Image: e.runtime.Options.Image, Workdir: "/",
		Entrypoint: []string{"/bin/sleep"}, Args: []string{"infinity"}, Init: true, NoNewPrivileges: true,
		Placement: request.Connectivity.Placement, CPU: request.CPU, MemoryMB: request.MemoryMB, ServicePort: 18866,
		Labels: map[string]string{"aries.managed": "true", "aries.kind": "jiuwen-harness", "aries.component": "harness",
			"aries.run": request.RunID, "aries.task": request.TaskID, "aries.attempt": id}}
	// The Runner calls Stop even after failed Start. Runtime retains any partial
	// Create identity so this path never hides an unconfirmed allocation.
	if err := e.runtime.Start(start, e.occurrence, container, archive, [][]byte{e.credentials.Get("proxy"), e.credentials.Get("bridge")}); err != nil {
		return e.credentials.RedactErr(err)
	}
	e.sandbox = &agentContainer{deployment: e.runtime.Options.Deployment, id: e.occurrence.ID}
	sources := make(map[string]string)
	for _, name := range []string{"extension.py", "provider.py", "identity.py", "wire.py"} {
		raw, err := remoteProviderSources.ReadFile("remote/" + name)
		if err != nil {
			return err
		}
		sources[name] = string(raw)
	}
	stage, err := json.Marshal(map[string]any{"source_dir": e.options.SourceDir, "python_path": e.options.PythonPath,
		"workdir": e.options.Workdir, "model_proxy_url": e.options.ModelProxyURL, "proxy_token": string(e.credentials.Get("proxy")),
		"model": request.Model.Model, "bootstrap": bootstrap, "native_observation": nativeSpansEnabled(e.options.ObservationLevel),
		"native_audit": nativeAudit, "observation_binding": map[string]any{"schema_version": 1, "run_id": request.RunID, "task_id": request.TaskID},
		"model_session_protocol": e.options.ModelSessionProtocol, "model_branches_extension": modelBranchesExtension,
		"remote_binding": binding, "remote_sources": sources})
	if err != nil {
		return err
	}
	defer clear(stage)
	result, err := e.sandbox.Exec(start, core.Command{Path: "python3", Args: []string{"-I", "-c", bootstrap, "stage", e.runtimeDir}, User: "root", Dir: "/", Stdin: stage, OutputLimitBytes: 4096})
	if err != nil {
		return e.credentials.RedactErr(err)
	}
	if result.ExitCode != 0 {
		return errors.New("Jiuwen external pinned installation or private staging failed")
	}
	e.logs = &evidenceBuffer{max: 64 << 20}
	process, processCancel := context.WithCancel(context.WithoutCancel(ctx))
	e.processCancel, e.processDone = processCancel, make(chan error, 1)
	done, logs, containerID := e.processDone, e.logs, e.occurrence.ID
	go func() {
		result, err := e.runtime.Options.Deployment.ExecStream(process, containerID, core.Command{Path: e.options.PythonPath,
			Args: []string{"-I", path.Join(e.runtimeDir, "launch.py"), "run", e.runtimeDir}, User: "60000:60000", Dir: e.options.Workdir}, nil, logs, logs)
		if err == nil && result.ExitCode != 0 {
			err = fmt.Errorf("Jiuwen AgentServer exited with code %d", result.ExitCode)
		}
		done <- err
		close(done)
	}()
	address, err := e.runtime.Options.Deployment.Address(start, containerID, 18866)
	if err != nil {
		return e.credentials.RedactErr(err)
	}
	e.client, err = e.newClient("http://"+address, http.Header{})
	if err != nil {
		return err
	}
	if err := e.waitReady(start); err != nil {
		return e.credentials.RedactErr(err)
	}
	proof, err := e.sandbox.Exec(start, core.Command{Path: "python3", Args: []string{"-I", "-c", externalPreparedScript, e.runtimeDir}, User: "root", Dir: "/", OutputLimitBytes: 64 << 10})
	if err != nil {
		return e.credentials.RedactErr(err)
	}
	var prepared struct {
		Revision       string `json:"revision"`
		Lock           string `json:"uv_lock_sha256"`
		Project        string `json:"project_id"`
		Workdir        string `json:"workdir"`
		Mode           string `json:"mode"`
		PID            int    `json:"pid"`
		StartTicks     string `json:"process_start_ticks"`
		SandboxType    string `json:"sandbox_type"`
		SandboxEnabled bool   `json:"sandbox_enabled"`
	}
	if proof.ExitCode != 0 || json.Unmarshal([]byte(proof.Stdout), &prepared) != nil || prepared.Revision != NativeRevision || prepared.Lock != LockSHA256 ||
		prepared.Project == "" || prepared.Workdir != e.options.Workdir || prepared.Mode != "flash" || prepared.PID <= 1 || prepared.StartTicks == "" ||
		prepared.SandboxType != "aries" || !prepared.SandboxEnabled {
		return errors.New("Jiuwen external native configuration evidence is invalid")
	}
	e.nativePID, e.nativeStartTicks = prepared.PID, prepared.StartTicks
	if err := privatefiles.WriteArtifact(filepath.Join(e.artifactDir, "runtime-evidence.json"), e.credentials.Redact([]byte(proof.Stdout))); err != nil {
		return err
	}
	raw, err := e.client.CreateSession(start, e.requestPrefix+"-session", prepared.Project)
	if err != nil {
		return e.credentials.RedactErr(err)
	}
	var session struct {
		ID      string `json:"session_id"`
		Workdir string `json:"projectDir"`
	}
	if json.Unmarshal(raw, &session) != nil || session.ID == "" || session.Workdir != e.options.Workdir {
		return errors.New("Jiuwen session did not bind to the prepared remote project")
	}
	e.sessionID, e.ready = session.ID, true
	e.runtime.Ready()
	return nil
}

func (e *External) Run(ctx context.Context, instruction string) (core.HarnessResult, error) {
	if err := e.runtime.AdmitRun(instruction); err != nil {
		return core.HarnessResult{Status: core.StatusFailed}, err
	}
	return e.nativeRuntime.Run(ctx, instruction)
}

func (e *External) Stop(ctx context.Context) error {
	e.stopMu.Lock()
	defer e.stopMu.Unlock()
	e.mu.Lock()
	e.closed = true
	if e.isolated {
		e.mu.Unlock()
		return nil
	}
	client, runCancel, runDone, processCancel, processDone := e.client, e.runCancel, e.runDone, e.processCancel, e.processDone
	attempt, owns := e.runtime.BeginStop()
	e.mu.Unlock()
	if !owns && e.occurrence == nil {
		clear(e.options.ProxyAccessToken)
		if e.credentials != nil {
			e.credentials.Clear()
		}
		return attempt.Wait(ctx)
	}
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), e.options.CleanupTimeout)
	defer cancel()
	if runCancel != nil {
		runCancel()
	}
	if client != nil {
		client.Close()
	}
	if e.nativePID > 1 && !e.nativeStop.Attempted {
		e.nativeStop = e.gracefulNativeStop(cleanup)
	}
	// Freeze evidence only after the dedicated Agent UID has no live processes.
	// Container removal below is the authoritative stop, even if this collection
	// preparation fails; no verifier may rely on the graceful signal alone.
	if e.sandbox != nil && !e.nativeCollected {
		collection, finish := context.WithTimeout(cleanup, min(8*time.Second, e.options.CleanupTimeout/3))
		result, err := e.sandbox.Exec(collection, core.Command{Path: "python3", Args: []string{"-I", "-c", externalQuiesceScript}, User: "root", Dir: "/", OutputLimitBytes: 4096})
		var paths []string
		if err == nil && result.ExitCode == 0 {
			if e.sessionID != "" {
				paths, err = e.collectNativeSpans(collection)
			}
		} else {
			err = errors.New("Jiuwen external evidence quiescence unconfirmed")
		}
		finish()
		e.mu.Lock()
		e.nativeCollected = true
		e.observation.LogPaths = append(e.observation.LogPaths, paths...)
		e.observationErr = errors.Join(e.observationErr, e.credentials.RedactErr(err))
		e.mu.Unlock()
	}
	err := e.runtime.Remove(cleanup, e.occurrence)
	if processCancel != nil {
		processCancel()
	}
	for _, done := range []<-chan struct{}{runDone} {
		if done != nil {
			select {
			case <-done:
			case <-cleanup.Done():
				err = errors.Join(err, cleanup.Err())
			}
		}
	}
	if processDone != nil {
		select {
		case <-processDone:
		case <-cleanup.Done():
			err = errors.Join(err, cleanup.Err())
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err == nil {
		if e.logs != nil {
			logPath := filepath.Join(e.artifactDir, "agentserver.log")
			writeErr := privatefiles.WriteArtifact(logPath, e.credentials.Redact(e.logs.Bytes()))
			if writeErr == nil {
				e.observation.LogPaths = append(e.observation.LogPaths, logPath)
			}
			if e.logs.Truncated() {
				writeErr = errors.Join(writeErr, errors.New("Jiuwen AgentServer log exceeded 16 MiB"))
			}
			e.observationErr = errors.Join(e.observationErr, writeErr)
		}
		if e.credentials != nil {
			e.credentials.Clear()
		}
		e.isolated = true
	}
	if e.observationErr != nil {
		e.observation.Status = core.StatusFailed
		e.observation.Error = e.observationErr.Error()
	}
	if owns {
		e.runtime.FinishStop(err)
	}
	return err
}

func (e *External) NativeToolObservation() (core.ObserverResult, error) {
	native, err := e.nativeRuntime.NativeToolObservation()
	remote, remoteErr := e.bridge.RemoteObservation()
	native.LogPaths = append(native.LogPaths, remote.LogPaths...)
	err = errors.Join(err, remoteErr)
	if err != nil {
		native.Status = core.StatusFailed
		native.Error = err.Error()
	}
	if remote.Reason != "" {
		native.Reason += "; " + remote.Reason
	}
	return native, err
}

func (e *External) Close() error {
	if err := e.Stop(context.Background()); err != nil {
		return err
	}
	return e.runtime.Close()
}

const externalPreparedScript = `import json,pathlib,sys
runtime=pathlib.Path(sys.argv[1]); proof=json.loads((runtime/'home/prepared.json').read_text())
assert proof['uid']==60000 and proof['sandbox_enabled'] is True and proof['sandbox_type']=='aries'
assert proof['sandbox_startup_mode']=='external' and proof['skill_selection'] is False
process=pathlib.Path('/proc')/str(proof['pid']); assert process.stat().st_uid==60000
proof['process_start_ticks']=(process/'stat').read_text().rsplit(')',1)[1].split()[19]
proof['placement']='external'; proof['containment']='dedicated_docker_container'
print(json.dumps(proof))
`

const externalQuiesceScript = `import os,pathlib,signal,time
def live():
 result=[]
 for p in pathlib.Path('/proc').iterdir():
  if not p.name.isdigit(): continue
  try:
   if p.stat().st_uid==60000 and (p/'stat').read_text().rsplit(')',1)[1].split()[0]!='Z': result.append(int(p.name))
  except (FileNotFoundError,ProcessLookupError): pass
 return result
end=time.monotonic()+3
while True:
 pids=live()
 if not pids: break
 for pid in pids:
  try: os.kill(pid,signal.SIGKILL)
  except ProcessLookupError: pass
 if time.monotonic()>=end: raise RuntimeError('Agent processes remain live')
 time.sleep(.025)
print('AGENT_PROCESSES_ABSENT')
`

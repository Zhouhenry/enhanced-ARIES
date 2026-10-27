package jiuwenswarm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
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
	"github.com/hyscale-lab/aries/pkg/performance"
	"github.com/hyscale-lab/aries/pkg/runner"
)

//go:embed remote/worker.py remote/wire.py
var remoteWorkerSources embed.FS

const remoteWorkerPort = 18867
const remoteRequestLimit = 24 << 20

type RemoteBridgeOptions struct {
	RunID, TaskID, OutputDir, Workdir, SourceDir, PythonPath string
	BindHost, AdvertiseHost                                  string
	GuestEndpoint                                            func(context.Context, runner.Sandbox, int) (string, http.Header, error)
	StartTimeout, CleanupTimeout                             time.Duration
	ObservationLevel                                         performance.ObservationLevel
}

// RemoteClientConfig is private runtime configuration, never a public endpoint receipt.
type RemoteClientConfig struct {
	SchemaVersion int    `json:"schema_version"`
	BaseURL       string `json:"base_url"`
	BindingID     string `json:"binding_id"`
	AccessToken   string `json:"access_token"`
	RunID         string `json:"run_id"`
	TaskID        string `json:"task_id"`
	Workdir       string `json:"workdir"`
}

// RemoteBridge lends native operations to one already allocated sandbox. It
// neither creates cloud resources nor gives provider credentials to the Agent.
type RemoteBridge struct {
	options                                             RemoteBridgeOptions
	mu, lifecycle                                       sync.Mutex
	used, ready, closed, isolated                       bool
	processObservationEnabled                           bool
	processObservationStatus                            string
	group                                               *sandboxprocess.Group
	sandboxID, bindingID, runtimeDir, address, guestURL string
	credentials                                         *harness.Credentials
	guestHeaders                                        http.Header
	client                                              *http.Client
	server                                              *http.Server
	serverDone, processDone                             chan struct{}
	processErr                                          error
	processCancel, callsCancel                          context.CancelFunc
	active                                              int
	drained                                             chan struct{}
	seen                                                map[string]bool
	logs, events                                        *evidenceBuffer
	observation                                         core.ObserverResult
	observationErr                                      error
	clock                                               *performance.Clock
	records, finishedCalls                              uint64
}

var _ runner.ToolBridge = (*RemoteBridge)(nil)

func NewRemoteBridge(options RemoteBridgeOptions) (*RemoteBridge, error) {
	if err := harness.ValidateRunID("Jiuwen remote bridge", options.RunID); err != nil {
		return nil, err
	}
	if err := harness.ValidateTaskID("Jiuwen remote bridge", options.TaskID); err != nil {
		return nil, err
	}
	for _, value := range []string{options.Workdir, options.SourceDir, options.PythonPath} {
		if !path.IsAbs(value) || path.Clean(value) != value || value == "/" || strings.ContainsRune(value, 0) {
			return nil, errors.New("Jiuwen remote bridge requires clean absolute workspace, source and Python paths")
		}
	}
	if options.BindHost == "" {
		options.BindHost = "127.0.0.1"
	}
	ip := net.ParseIP(options.BindHost)
	if ip == nil || (!ip.IsLoopback() && !ip.IsPrivate()) || ip.IsUnspecified() {
		return nil, errors.New("Jiuwen remote bridge requires a private or loopback bind IP")
	}
	if options.AdvertiseHost == "" {
		options.AdvertiseHost = options.BindHost
	}
	if !core.ValidEndpointHost(options.AdvertiseHost) || options.OutputDir == "" || options.GuestEndpoint == nil {
		return nil, errors.New("Jiuwen remote bridge requires an advertised host, evidence directory and guest resolver")
	}
	if options.StartTimeout <= 0 {
		options.StartTimeout = 90 * time.Second
	}
	if options.CleanupTimeout <= 0 {
		options.CleanupTimeout = 30 * time.Second
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.MaxIdleConnsPerHost = 32
	return &RemoteBridge{options: options, credentials: harness.NewCredentials("Jiuwen remote bridge"),
		client: &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		seen:   make(map[string]bool), logs: &evidenceBuffer{max: 64 << 20}, events: &evidenceBuffer{max: 64 << 20}}, nil
}

func (b *RemoteBridge) ClientConfig() (RemoteClientConfig, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.ready || b.closed {
		return RemoteClientConfig{}, errors.New("Jiuwen remote bridge is not available")
	}
	return RemoteClientConfig{SchemaVersion: 1, BaseURL: b.address, BindingID: b.bindingID, AccessToken: string(b.credentials.Get("client")),
		RunID: b.options.RunID, TaskID: b.options.TaskID, Workdir: b.options.Workdir}, nil
}

func (b *RemoteBridge) Start(ctx context.Context, sandbox runner.Sandbox) (core.ToolEndpoint, error) {
	b.lifecycle.Lock()
	defer b.lifecycle.Unlock()
	b.mu.Lock()
	if b.used || b.closed {
		b.mu.Unlock()
		return core.ToolEndpoint{}, errors.New("Jiuwen remote bridge is single use")
	}
	b.used = true
	b.clock = performance.ClockFromContext(ctx)
	b.processObservationStatus = "not_enabled"
	if nativeSpansEnabled(b.options.ObservationLevel) {
		b.processObservationStatus = "unconfirmed"
	}
	b.mu.Unlock()
	identity, ok := sandbox.(interface{ ID() string })
	if !ok || identity.ID() == "" {
		return core.ToolEndpoint{}, errors.New("Jiuwen remote bridge requires the allocated sandbox identity")
	}
	b.sandboxID = identity.ID()
	start, cancel := context.WithTimeout(ctx, b.options.StartTimeout)
	defer cancel()
	var err error
	b.group, err = sandboxprocess.New(start, sandbox, b.options.Workdir)
	if err != nil {
		return core.ToolEndpoint{}, err
	}
	b.bindingID, err = privatefiles.RandomID()
	if err != nil {
		return core.ToolEndpoint{}, err
	}
	b.runtimeDir = "/run/aries-jiuwen-remote-" + b.bindingID
	for _, name := range []string{"client", "worker"} {
		token, err := privatefiles.RandomSecret(32)
		if err != nil {
			return core.ToolEndpoint{}, err
		}
		b.credentials.Set(name, token)
		clear(token)
	}
	files := make(map[string]string)
	for _, name := range []string{"worker.py", "wire.py"} {
		content, err := remoteWorkerSources.ReadFile("remote/" + name)
		if err != nil {
			return core.ToolEndpoint{}, err
		}
		files[name] = string(content)
	}
	spec := map[string]any{"schema_version": 1, "binding_id": b.bindingID, "sandbox_id": b.sandboxID,
		"run_id": b.options.RunID, "task_id": b.options.TaskID, "workdir": b.options.Workdir,
		"source_dir": b.options.SourceDir, "python_path": b.options.PythonPath, "revision": NativeRevision, "uv_lock_sha256": LockSHA256,
		"worker_token": string(b.credentials.Get("worker")), "files": files, "home": path.Join(b.runtimeDir, "home"),
		"evidence_path": path.Join(b.runtimeDir, "home/worker-events.jsonl"), "listen_host": "0.0.0.0", "listen_port": remoteWorkerPort,
		"process_observation": nativeSpansEnabled(b.options.ObservationLevel)}
	staged, err := json.Marshal(spec)
	if err != nil {
		return core.ToolEndpoint{}, err
	}
	result, err := sandbox.Exec(start, core.Command{Path: "python3", Args: []string{"-I", "-c", remoteStageScript, b.runtimeDir}, User: "root", Dir: "/", Stdin: staged, OutputLimitBytes: 4096})
	clear(staged)
	if err != nil {
		return core.ToolEndpoint{}, b.credentials.RedactErr(err)
	}
	if result.ExitCode != 0 {
		return core.ToolEndpoint{}, errors.New("Jiuwen remote worker pinned installation or staging failed")
	}
	processCtx, processCancel := context.WithCancel(context.WithoutCancel(ctx))
	b.processCancel, b.processDone = processCancel, make(chan struct{})
	go func() {
		result, err := b.group.ExecStream(processCtx, core.Command{Path: b.options.PythonPath,
			Args: []string{"-I", path.Join(b.runtimeDir, "worker.py"), "--binding", path.Join(b.runtimeDir, "binding.json"), "--listen", "0.0.0.0:18867"},
			Dir:  b.options.Workdir, Env: map[string]string{"HOME": path.Join(b.runtimeDir, "home"), "PYTHONUNBUFFERED": "1", "PYTHONDONTWRITEBYTECODE": "1"}}, nil, b.logs, b.logs)
		if err == nil && result.ExitCode != 0 {
			err = fmt.Errorf("Jiuwen remote worker exited with code %d", result.ExitCode)
		}
		b.mu.Lock()
		b.processErr = err
		b.mu.Unlock()
		close(b.processDone)
	}()
	b.guestURL, b.guestHeaders, err = b.options.GuestEndpoint(start, sandbox, remoteWorkerPort)
	if err != nil {
		return core.ToolEndpoint{}, errors.New("resolve authenticated Jiuwen remote worker endpoint failed")
	}
	u, err := url.Parse(b.guestURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(b.guestHeaders) == 0 {
		return core.ToolEndpoint{}, errors.New("Jiuwen remote guest endpoint requires HTTPS and provider access headers")
	}
	b.guestURL = strings.TrimRight(b.guestURL, "/")
	b.guestHeaders = b.guestHeaders.Clone()
	b.credentials.AddGuestHeaders(b.guestHeaders)
	if err := b.waitWorker(start); err != nil {
		return core.ToolEndpoint{}, err
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(b.options.BindHost, "0"))
	if err != nil {
		return core.ToolEndpoint{}, err
	}
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	b.address = "http://" + net.JoinHostPort(b.options.AdvertiseHost, port)
	callsCtx, callsCancel := context.WithCancel(context.Background())
	b.callsCancel, b.drained, b.serverDone = callsCancel, make(chan struct{}), make(chan struct{})
	b.server = &http.Server{Handler: http.HandlerFunc(b.serve), ReadHeaderTimeout: 10 * time.Second, BaseContext: func(net.Listener) context.Context { return callsCtx }}
	go func() { defer close(b.serverDone); _ = b.server.Serve(listener) }()
	b.mu.Lock()
	b.ready = true
	b.mu.Unlock()
	return core.ToolEndpoint{Protocol: "jiuwen-remote-v1", Address: b.address, Workdir: b.options.Workdir}, nil
}

func (b *RemoteBridge) guestRequest(ctx context.Context, method, route string, body []byte) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, b.guestURL+route, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header = b.guestHeaders.Clone()
	request.Header.Set("X-Aries-Worker-Token", string(b.credentials.Get("worker")))
	request.Header.Set("Content-Type", "application/json")
	return b.client.Do(request)
}

func (b *RemoteBridge) waitWorker(ctx context.Context) error {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		probe, cancel := context.WithTimeout(ctx, 2*time.Second)
		response, err := b.guestRequest(probe, http.MethodGet, "/health", nil)
		if err == nil {
			var health struct {
				SchemaVersion  int    `json:"schema_version"`
				BindingID      string `json:"binding_id"`
				ProcessEnabled *bool  `json:"process_observation_enabled"`
				ProcessStatus  string `json:"process_observation_status"`
			}
			decodeErr := json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&health)
			response.Body.Close()
			cancel()
			if response.StatusCode == http.StatusOK && decodeErr == nil && health.SchemaVersion == 1 && health.BindingID == b.bindingID {
				if nativeSpansEnabled(b.options.ObservationLevel) {
					if health.ProcessEnabled == nil || (*health.ProcessEnabled && health.ProcessStatus != "enabled") || (!*health.ProcessEnabled && health.ProcessStatus != "unsupported_native_process_hook") {
						return errors.New("Jiuwen worker process observation capability is unconfirmed")
					}
					b.processObservationEnabled, b.processObservationStatus = *health.ProcessEnabled, health.ProcessStatus
				}
				return nil
			}
		} else {
			cancel()
		}
		select {
		case <-b.processDone:
			return errors.New("Jiuwen remote worker exited before readiness")
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

type remoteCall struct {
	SchemaVersion int    `json:"schema_version"`
	BindingID     string `json:"binding_id"`
	InvocationID  string `json:"invocation_id"`
	SessionID     string `json:"session_id"`
	Scope         string `json:"scope"`
	Identity      struct {
		RunID      string `json:"run_id"`
		TaskID     string `json:"task_id"`
		SessionID  string `json:"session_id"`
		RequestID  string `json:"request_id"`
		ToolCallID string `json:"tool_call_id"`
		Status     string `json:"identity_status"`
	} `json:"identity"`
	Operation struct {
		Kind   string `json:"kind"`
		Method string `json:"method"`
	} `json:"operation"`
}

func validRemoteID(value string) bool {
	return value != "" && len(value) <= 1024 && !strings.ContainsAny(value, "\x00\r\n\t")
}

func (b *RemoteBridge) serve(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	if b.closed || !b.ready {
		b.mu.Unlock()
		http.Error(w, "bridge revoked", http.StatusGone)
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+string(b.credentials.Get("client")))) != 1 {
		b.mu.Unlock()
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	b.active++
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		b.active--
		if b.closed && b.active == 0 {
			close(b.drained)
		}
	}()
	if r.URL.RawQuery != "" || (r.URL.Path != "/call" && r.URL.Path != "/cancel" && r.URL.Path != "/health") {
		http.Error(w, "unknown operation", http.StatusNotFound)
		return
	}
	if r.URL.Path == "/health" && r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"schema_version": 1, "binding_id": b.bindingID})
		return
	}
	if r.Method != http.MethodPost || r.URL.Path == "/health" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, remoteRequestLimit))
	if err != nil {
		http.Error(w, "request exceeds transport bound", http.StatusRequestEntityTooLarge)
		return
	}
	var call remoteCall
	if json.Unmarshal(raw, &call) != nil || call.SchemaVersion != 1 || call.BindingID != b.bindingID {
		http.Error(w, "invalid attempt binding", http.StatusBadRequest)
		return
	}
	if r.URL.Path == "/call" {
		if !validRemoteID(call.InvocationID) || call.Identity.RunID != b.options.RunID || call.Identity.TaskID != b.options.TaskID ||
			(call.Identity.Status != "known" && call.Identity.Status != "unavailable") ||
			(call.Identity.Status == "known" && !validRemoteID(call.Identity.ToolCallID)) ||
			(call.Identity.Status == "unavailable" && call.Identity.ToolCallID != "") ||
			(call.Operation.Kind != "fs" && call.Operation.Kind != "shell") || !validRemoteID(call.Operation.Method) {
			http.Error(w, "invalid native operation identity", http.StatusBadRequest)
			return
		}
		b.mu.Lock()
		duplicate := b.seen[call.InvocationID]
		if !duplicate {
			b.seen[call.InvocationID] = true
		}
		b.mu.Unlock()
		if duplicate {
			http.Error(w, "invocation already submitted", http.StatusConflict)
			return
		}
	} else if !validRemoteID(call.InvocationID) && !(call.Scope == "session_tree" && validRemoteID(call.SessionID)) {
		http.Error(w, "cancellation requires an explicit invocation or native session", http.StatusBadRequest)
		return
	}
	started := time.Now()
	var ended time.Time
	status := 0
	var transferred int64
	defer func() {
		if ended.IsZero() {
			ended = time.Now()
		}
		b.mu.Lock()
		defer b.mu.Unlock()
		b.records++
		if r.URL.Path == "/call" {
			b.finishedCalls++
		}
		row := map[string]any{"schema_version": 2, "sequence": b.records, "binding_id": b.bindingID, "sandbox_id": b.sandboxID,
			"invocation_id": call.InvocationID, "identity": call.Identity, "operation": call.Operation, "route": r.URL.Path,
			"controller_started_at": started.UTC().Format(time.RFC3339Nano), "controller_duration_ns": ended.Sub(started).Nanoseconds(),
			"http_status": status, "response_bytes": transferred, "transport_complete": err == nil,
			"measurement_scope": "controller_forwarding_not_guest_execution"}
		if b.clock != nil {
			row["clock_domain"] = b.clock.ClockDomain
			row["controller_start_offset_ns"] = b.clock.Offset(started)
			row["controller_end_offset_ns"] = b.clock.Offset(ended)
		}
		record, _ := json.Marshal(row)
		_, _ = b.events.Write(append(record, '\n'))
	}()
	response, err := b.guestRequest(r.Context(), http.MethodPost, r.URL.Path, raw)
	if err != nil {
		http.Error(w, "guest transport failed", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	status = response.StatusCode
	w.Header().Set("Content-Type", response.Header.Get("Content-Type"))
	w.WriteHeader(status)
	transferred, err = io.Copy(remoteFlushWriter{w}, response.Body)
	ended = time.Now()
}

type remoteFlushWriter struct{ http.ResponseWriter }

func (w remoteFlushWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if flush, ok := w.ResponseWriter.(http.Flusher); ok {
		flush.Flush()
	}
	return n, err
}

func (b *RemoteBridge) Stop(ctx context.Context) error {
	b.lifecycle.Lock()
	defer b.lifecycle.Unlock()
	b.mu.Lock()
	if b.isolated {
		b.mu.Unlock()
		return nil
	}
	if !b.closed {
		b.closed = true
		if b.drained != nil && b.active == 0 {
			close(b.drained)
		}
	}
	b.mu.Unlock()
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), b.options.CleanupTimeout)
	defer cancel()
	if b.callsCancel != nil {
		b.callsCancel()
	}
	if b.server != nil {
		_ = b.server.Close()
	}
	if b.processObservationEnabled && b.guestURL != "" {
		// Give native cancellation callbacks a bounded opportunity to record real
		// exits. The cgroup stop below remains the isolation proof on every path.
		quiesce, finish := context.WithTimeout(cleanup, min(3*time.Second, b.options.CleanupTimeout/4))
		body, _ := json.Marshal(map[string]any{"schema_version": 1, "binding_id": b.bindingID})
		response, err := b.guestRequest(quiesce, http.MethodPost, "/quiesce", body)
		if err == nil {
			var receipt struct {
				Complete bool   `json:"process_observation_complete"`
				Binding  string `json:"binding_id"`
			}
			if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&receipt) != nil || !receipt.Complete || receipt.Binding != b.bindingID {
				err = errors.New("Jiuwen worker process observation did not quiesce completely")
			}
			response.Body.Close()
		}
		finish()
		if err != nil {
			b.mu.Lock()
			b.observationErr = errors.Join(b.observationErr, errors.New("Jiuwen worker process observation shutdown incomplete"))
			b.mu.Unlock()
		}
	}
	// Closing transports only revokes access. Positive process-tree isolation is
	// still required, including commands accepted earlier in background mode.
	if b.group != nil {
		if err := b.group.Stop(cleanup); err != nil {
			return err
		}
	}
	if b.processCancel != nil {
		b.processCancel()
	}
	for _, done := range []chan struct{}{b.drained, b.serverDone, b.processDone} {
		if done != nil {
			select {
			case <-done:
			case <-cleanup.Done():
				return cleanup.Err()
			}
		}
	}
	b.client.CloseIdleConnections()
	deadline, _ := cleanup.Deadline()
	collection, finishCollection := context.WithTimeout(cleanup, min(5*time.Second, time.Until(deadline)-min(2*time.Second, b.options.CleanupTimeout/4)))
	b.collectRemoteEvidence(collection)
	finishCollection()
	if b.runtimeDir != "" {
		result, err := b.group.Sandbox().Exec(cleanup, core.Command{Path: "python3", Args: []string{"-I", "-c", remoteCleanupScript, b.runtimeDir}, User: "root", Dir: "/", OutputLimitBytes: 4096})
		if err != nil || result.ExitCode != 0 {
			return errors.New("Jiuwen remote worker private runtime cleanup is unconfirmed")
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.credentials.Clear()
	b.guestHeaders = nil
	b.isolated = true
	return nil
}

func (b *RemoteBridge) RemoteObservation() (core.ObserverResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.observation, b.observationErr
}

func (b *RemoteBridge) collectRemoteEvidence(ctx context.Context) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.observation.Status != "" {
		return
	}
	b.observation = core.ObserverResult{Status: core.StatusUnsupported, Reason: "remote bridge controller transport and explicit native identities; guest process coverage is unverified", SampleCount: len(b.seen)}
	if b.processObservationStatus == "unsupported_native_process_hook" {
		b.observation.Reason += "; process observation unsupported by installed native image"
	}
	write := func(name string, raw []byte) {
		file := filepath.Join(b.options.OutputDir, name)
		if err := privatefiles.WriteArtifact(file, b.credentials.Redact(raw)); err != nil {
			b.observationErr = errors.Join(b.observationErr, err)
		} else {
			b.observation.LogPaths = append(b.observation.LogPaths, file)
		}
	}
	write("remote-bridge-events.jsonl", b.events.Bytes())
	write("remote-worker.log", b.logs.Bytes())
	if b.events.Truncated() || b.logs.Truncated() {
		b.observationErr = errors.Join(b.observationErr, errors.New("Jiuwen remote bridge evidence exceeded retention bound"))
	}
	if b.runtimeDir != "" {
		if b.processObservationEnabled {
			paths, err := collectNativeProcessFiles(ctx, b.group.Sandbox(), path.Join(b.runtimeDir, "home"), b.options.OutputDir)
			b.observation.LogPaths = append(b.observation.LogPaths, paths...)
			b.observationErr = errors.Join(b.observationErr, err)
		}
		if downloader, ok := b.group.Sandbox().(runner.LimitedDownloader); ok {
			for _, names := range [][2]string{{"worker-events.jsonl", "remote-worker-events.jsonl"}, {"worker-index.json", "remote-worker-index.json"}} {
				file := filepath.Join(b.options.OutputDir, names[1])
				if err := privatefiles.EnsureDirectory(b.options.OutputDir); err != nil {
					b.observationErr = errors.Join(b.observationErr, err)
				} else if err := downloader.DownloadLimit(ctx, path.Join(b.runtimeDir, "home", names[0]), file, 64<<20); err != nil {
					b.observationErr = errors.Join(b.observationErr, errors.New("collect bounded native remote worker evidence failed"))
				} else {
					b.observation.LogPaths = append(b.observation.LogPaths, file)
				}
			}
		} else {
			b.observationErr = errors.Join(b.observationErr, errors.New("bounded native remote worker download unsupported"))
		}
	}
	hashes := make(map[string]string)
	for _, name := range []string{"worker.py", "wire.py"} {
		raw, _ := remoteWorkerSources.ReadFile("remote/" + name)
		sum := sha256.Sum256(raw)
		hashes[name] = hex.EncodeToString(sum[:])
	}
	receipt, _ := json.MarshalIndent(map[string]any{"schema_version": 2, "run_id": b.options.RunID, "task_id": b.options.TaskID, "sandbox_id": b.sandboxID, "binding_id": b.bindingID,
		"process_observation_requested": nativeSpansEnabled(b.options.ObservationLevel),
		"process_observation_expected":  b.processObservationEnabled,
		"process_observation_status":    b.processObservationStatus,
		"clock":                         b.clock, "accepted_calls": len(b.seen), "finished_calls": b.finishedCalls, "records": b.records, "truncated": b.events.Truncated(),
		"native_revision": NativeRevision, "uv_lock_sha256": LockSHA256, "worker_sources_sha256": hashes, "workdir": b.options.Workdir,
		"group_empty_confirmed": b.group != nil, "local_calls_joined": true, "coverage": "unknown"}, "", "  ")
	write("remote-bridge-index.json", append(receipt, '\n'))
	if b.observationErr != nil {
		b.observation.Status = core.StatusFailed
		b.observation.Error = b.observationErr.Error()
	}
}

const remoteStageScript = `import hashlib,json,os,pathlib,stat,sys
spec=json.load(sys.stdin)
source=pathlib.Path(spec['source_dir']); executable=pathlib.Path(spec['python_path'])
def trusted(p):
    for item in [p]+list(p.parents):
        s=item.stat()
        if s.st_uid!=0 or s.st_mode&0o022: raise RuntimeError('untrusted native installation')
if os.geteuid()!=0 or source.resolve(strict=True)!=source: raise RuntimeError('root canonical staging required')
for p in (source,executable,executable.resolve(strict=True),source/'aries-build.json',source/'uv.lock'): trusted(p)
build=json.loads((source/'aries-build.json').read_text())
if build.get('revision')!=spec['revision'] or build.get('uv_lock_sha256')!=spec['uv_lock_sha256'] or hashlib.sha256((source/'uv.lock').read_bytes()).hexdigest()!=spec['uv_lock_sha256']: raise RuntimeError('native pin mismatch')
spec['process_observation_files']={}
patches=source/'aries-patches.json'
if patches.exists():
    trusted(patches)
    overlay=json.loads(patches.read_text())
    if (overlay.get('schema_version')!=1 or not overlay.get('native_modified') or overlay.get('revision')!=spec['revision'] or overlay.get('uv_lock_sha256')!=spec['uv_lock_sha256'] or overlay.get('base_build_sha256')!=hashlib.sha256((source/'aries-build.json').read_bytes()).hexdigest()): raise RuntimeError('native patch baseline mismatch')
    for patch in overlay['patches']:
        patch_file=pathlib.Path(patch['patch_file']); trusted(patch_file)
        if hashlib.sha256(patch_file.read_bytes()).hexdigest()!=patch['patch_sha256']: raise RuntimeError('native patch evidence mismatch')
        for name,digests in patch['files'].items():
            installed=pathlib.Path(name); trusted(installed)
            if hashlib.sha256(installed.read_bytes()).hexdigest()!=digests['after_sha256']: raise RuntimeError('native patched installation mismatch')
        if patch.get('name')=='agent-core-process-observation':
            if spec['process_observation_files']: raise RuntimeError('duplicate native process patch')
            files={name:digests['after_sha256'] for name,digests in patch['files'].items() if name.endswith('/openjiuwen/core/sys_operation/local/aries_process_observer.py') or name.endswith('/openjiuwen/core/sys_operation/local/shell_operation.py')}
            if len(files)!=2: raise RuntimeError('native process patch lacks observer or creation hook')
            spec['process_observation_files']=files
runtime=pathlib.Path(sys.argv[1]); runtime.mkdir(mode=0o755); runtime.chmod(0o755)
home=runtime/'home'; home.mkdir(mode=0o700); os.chown(home,60000,60000)
for name,content in spec.pop('files').items():
    if name not in ('worker.py','wire.py'): raise RuntimeError('unexpected worker source')
    with open(runtime/name,'x') as f: f.write(content); os.fchmod(f.fileno(),0o444)
with open(runtime/'binding.json','x') as f: json.dump(spec,f); os.fchmod(f.fileno(),0o440)
os.chown(runtime/'binding.json',0,60000)
`

const remoteCleanupScript = `import pathlib,shutil,sys
p=pathlib.Path(sys.argv[1]); assert p.parent==pathlib.Path('/run') and p.name.startswith('aries-jiuwen-remote-')
if p.exists(): shutil.rmtree(p)
`

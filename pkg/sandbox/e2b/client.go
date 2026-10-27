// Package e2b connects Runner to the small official-Python-SDK adapter.
// Command durations are client wall time, never guest CPU or execution time.
package e2b

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/performance"
	"github.com/hyscale-lab/aries/pkg/runner"
)

const maxFileBytes = 64 << 20
const maxResponseBytes = 96 << 20 // Includes base64 expansion and JSON metadata.

type Options struct {
	Provider string
	Endpoint string
	Token    string
	Template string
	// TemplateMappings explicitly maps benchmark image identities to built E2B templates.
	TemplateMappings map[string]string
	TemplateBinding  *TaskTemplateBinding
	OutputDir        string
	LogicalTaskID    string
	// TemplateTaskID retains the original task identity for instruction variants.
	// Omission validates the binding against LogicalTaskID as before.
	TemplateTaskID string
	TimeoutSeconds int
	HTTPClient     *http.Client
	// Huawei-only assumption, matched against adapter health before allocation.
	AssumeNetworkIsolation bool
	// Connectivity is borrowed harness placement, unrelated to E2B lifecycle.
	Connectivity     core.HarnessConnectivity
	ObservationLevel performance.ObservationLevel
	// Clock is borrowed from the controller, including calls made with cleanup
	// or SSH contexts which no longer carry the recorder.
	Clock *performance.Clock
}

type Manager struct {
	options Options
	client  *http.Client
	mu      sync.Mutex
	owned   map[*Sandbox]struct{}
	handles map[string]*Sandbox
	logs    map[string]*operationLog
}

type Sandbox struct {
	manager                *Manager
	id                     string
	workdir                string
	user                   string
	defaultEnv             map[string]string
	runID, taskID, purpose string
	observation            *operationLog
	mu                     sync.Mutex
	absent                 bool
}

type Process struct {
	ID          string `json:"process"`
	PID         int    `json:"pid"`
	sandbox     *Sandbox
	started     time.Time
	executionID string
	observation *processObservation
}

type ProcessStatus struct {
	PID                   int  `json:"pid"`
	Done                  bool `json:"done"`
	ManagedProcessPresent bool `json:"managed_process_present"`
	ManagedProcessAbsent  bool `json:"managed_process_absent"`
	// Nil explicitly means no proof about descendants, including daemonized writers.
	DescendantsAbsent *bool `json:"descendants_absent"`
}

type GuestEndpoint struct {
	URL string `json:"url"`
	// Credentials: use only in the transport; never serialize this object into evidence.
	Headers map[string]string `json:"headers"`
}

type Metrics struct {
	Source                 string            `json:"source"`
	NominalIntervalSeconds *int              `json:"nominal_interval_seconds"`
	Metrics                []json.RawMessage `json:"metrics"`
	Unsupported            []string          `json:"unsupported"`
	Units                  map[string]string `json:"units,omitempty"`
}

type APIError struct {
	Code       string
	HTTPStatus int
}

func (e *APIError) Error() string { return "E2B adapter: " + e.Code }

var (
	_ runner.ToolSandbox       = (*Manager)(nil)
	_ runner.Sandbox           = (*Sandbox)(nil)
	_ runner.StreamExecutor    = (*Sandbox)(nil)
	_ runner.LimitedDownloader = (*Sandbox)(nil)
)

func New(options Options) (*Manager, error) {
	if options.Provider == "" {
		options.Provider = "e2b-cloud"
	}
	if options.ObservationLevel == "" {
		options.ObservationLevel = performance.ObservationL1
	}
	if options.Provider != "e2b-cloud" && options.Provider != "huawei-agentsphere" {
		return nil, errors.New("unsupported sandbox adapter provider")
	}
	if options.AssumeNetworkIsolation && options.Provider != "huawei-agentsphere" {
		return nil, errors.New("assume_network_isolation is only supported for huawei-agentsphere")
	}
	u, err := url.Parse(options.Endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("E2B adapter endpoint must be an HTTP(S) origin without credentials")
	}
	if len(options.Token) < 32 || strings.ContainsAny(options.Token, "\r\n") {
		return nil, errors.New("E2B adapter token must contain at least 32 characters")
	}
	if options.Template == "" && len(options.TemplateMappings) == 0 {
		return nil, errors.New("E2B template or explicit image mappings required")
	}
	if options.OutputDir == "" {
		return nil, errors.New("E2B output directory required")
	}
	if options.TemplateBinding != nil {
		if options.TemplateTaskID == "" {
			options.TemplateTaskID = options.LogicalTaskID
		}
		if err := options.TemplateBinding.ValidateProvider(options.Provider); err != nil {
			return nil, err
		}
		binding := options.TemplateBinding.clone()
		if err := binding.Validate(options.TemplateTaskID, binding.BenchmarkRevision, options.Template, binding.RequestedEnvironment); err != nil {
			return nil, err
		}
		options.TemplateBinding = binding
		options.TemplateMappings = maps.Clone(options.TemplateMappings)
	}
	if options.TimeoutSeconds == 0 {
		options.TimeoutSeconds = 300
	}
	if options.TimeoutSeconds < 1 {
		return nil, errors.New("E2B sandbox timeout must be positive")
	}
	options.Endpoint = strings.TrimRight(options.Endpoint, "/")
	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{Timeout: 45 * time.Second}
	}
	client := *options.HTTPClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return errors.New("redirect forbidden") }
	return &Manager{options: options, client: &client, owned: make(map[*Sandbox]struct{}), handles: make(map[string]*Sandbox), logs: make(map[string]*operationLog)}, nil
}

func identity() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (m *Manager) call(ctx context.Context, action string, body any, result any) (err error) {
	var span *operationSpan
	var expected *processObservation
	if fields, ok := body.(map[string]any); ok {
		if processID, ok := fields["process"].(string); ok && processID != "" {
			expected, err = processObservationFromContext(ctx, processID)
			if err != nil {
				return err
			}
		}
		if handle, ok := fields["sandbox"].(string); ok {
			m.mu.Lock()
			sandbox := m.handles[handle]
			m.mu.Unlock()
			if sandbox != nil && sandbox.observation != nil {
				processID, _ := fields["process"].(string)
				span = sandbox.observation.beginAt(ctx, sandbox, "adapter_rpc."+action, time.Now(), processID)
			}
		}
	}
	var calls []SDKCall
	var fileBytes *int64
	var creation *CreationEvidence
	defer func() { span.end(err, nil, nil, nil, fileBytes, nil, nil, calls, creation) }()
	data, err := json.Marshal(body)
	if err != nil {
		return errors.New("encode E2B adapter request")
	}
	req, err := http.NewRequestWithContext(ctx, "POST", m.options.Endpoint+"/v1/"+action, bytes.NewReader(data))
	if err != nil {
		return errors.New("construct E2B adapter request")
	}
	req.Header.Set("Authorization", "Bearer "+m.options.Token)
	req.Header.Set("Content-Type", "application/json")
	response, err := m.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("E2B adapter transport failed; submission may be ambiguous")
	}
	defer response.Body.Close()
	content, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(content) > maxResponseBytes {
		return errors.New("E2B adapter response incomplete or too large")
	}
	// Decode only the allowlisted observation fields, never request/response
	// bodies, provider identities, endpoints, command text, or credentials.
	var measured struct {
		SDKCalls []SDKCall `json:"sdk_calls"`
		Bytes    *int64    `json:"bytes"`
	}
	if measureErr := json.Unmarshal(content, &measured); measureErr == nil {
		for _, call := range measured.SDKCalls {
			if validSDKCall(call) {
				calls = append(calls, call)
			} else if span != nil {
				span.log.mu.Lock()
				span.log.problem("invalid_sdk_measurement")
				span.log.mu.Unlock()
			}
		}
		if (action == "upload" || action == "download") && measured.Bytes != nil {
			if *measured.Bytes >= 0 && *measured.Bytes <= maxFileBytes {
				fileBytes = measured.Bytes
			} else if span != nil {
				span.log.mu.Lock()
				span.log.problem("invalid_file_measurement")
				span.log.mu.Unlock()
			}
		}
	} else if span != nil {
		span.log.mu.Lock()
		span.log.problem("invalid_sdk_measurement")
		span.log.mu.Unlock()
	}
	if action == "create" && response.StatusCode == http.StatusOK {
		var evidence CreationEvidence
		if json.Unmarshal(content, &evidence) == nil {
			evidence.Resources, evidence.Region, evidence.ColdWarm = nil, nil, "unknown"
			creation = &evidence
		}
	}
	if response.StatusCode != http.StatusOK {
		var failure struct {
			Code string `json:"error_code"`
		}
		_ = json.Unmarshal(content, &failure)
		// Only machine codes can cross this boundary, never an SDK exception body.
		if failure.Code == "" || strings.IndexFunc(failure.Code, func(r rune) bool { return !(r >= 'a' && r <= 'z' || r == '_') }) >= 0 {
			failure.Code = "request_failed"
		}
		if failure.Code == "file_not_found" {
			return fmt.Errorf("E2B file: %w", runner.ErrNotFound)
		}
		return &APIError{Code: failure.Code, HTTPStatus: response.StatusCode}
	}
	if err := expected.verifyResponse(content); err != nil {
		return err
	}
	if result == nil {
		return nil
	}
	if err := json.Unmarshal(content, result); err != nil {
		return errors.New("invalid E2B adapter response")
	}
	return nil
}

// CheckProvider prevents a Huawei profile from allocating through an E2B endpoint.
// It also requires the profile's explicit assumption mode to match the adapter.
// These are authenticated configuration checks, not cloud capability acceptance.
func (m *Manager) CheckProvider(ctx context.Context) error {
	var health struct {
		Provider               string `json:"provider"`
		AssumeNetworkIsolation *bool  `json:"assume_network_isolation"`
	}
	if err := m.call(ctx, "health", map[string]any{}, &health); err != nil {
		return err
	}
	if health.Provider != m.options.Provider {
		return errors.New("sandbox adapter provider mismatch")
	}
	if m.options.Provider == "huawei-agentsphere" && (health.AssumeNetworkIsolation == nil || *health.AssumeNetworkIsolation != m.options.AssumeNetworkIsolation) {
		return errors.New("sandbox adapter assume_network_isolation missing or mismatched")
	}
	return nil
}

func (m *Manager) Start(ctx context.Context, request core.SandboxRequest) (runner.Sandbox, error) {
	env := request.Environment
	binding := m.options.TemplateBinding
	if binding == nil && (env.CPU != 0 || env.MemoryMB != 0 || env.StorageMB != 0 || env.GPUs != 0) {
		return nil, errors.New("E2B resources are template-defined; runtime overrides are unsupported")
	}
	if env.Services.SearchPort != 0 || len(env.Services.MCP) != 0 {
		return nil, errors.New("E2B task-service discovery is not yet supported")
	}
	if binding == nil && len(env.Env) != 0 {
		return nil, errors.New("E2B global task environment is unsupported; supply explicit command environment")
	}
	template := m.options.Template
	workdir, user := env.Workdir, env.ExecUser
	var defaults map[string]string
	if binding != nil {
		if err := binding.Validate(m.options.TemplateTaskID, binding.BenchmarkRevision, template, env); err != nil {
			return nil, err
		}
		workdir, user, defaults = binding.Workdir, "root", binding.DefaultEnvironment()
	} else if env.Image != "" {
		var ok bool
		template, ok = m.options.TemplateMappings[env.Image]
		if !ok {
			return nil, errors.New("benchmark image requires an explicit E2B template mapping")
		}
	}
	if !strings.HasPrefix(workdir, "/") {
		return nil, errors.New("E2B requires an explicit absolute benchmark workdir")
	}
	id, err := identity()
	if err != nil {
		return nil, err
	}
	purpose := request.Purpose
	if purpose == "" {
		purpose = "task"
	}
	s := &Sandbox{manager: m, id: id, workdir: workdir, user: user, defaultEnv: defaults, runID: request.RunID, taskID: request.TaskID, purpose: purpose}
	m.mu.Lock()
	var o *operationLog
	if performance.ObservesControl(m.options.ObservationLevel) {
		o = m.logs[request.TaskID]
		if o == nil {
			o = newOperationLog(m.options.OutputDir, request.TaskID, m.options.Clock)
			m.logs[request.TaskID] = o
		}
	}
	s.observation = o
	m.handles[id] = s
	m.owned[s] = struct{}{}
	m.mu.Unlock()
	var created struct {
		Sandbox  string `json:"sandbox"`
		State    string `json:"state"`
		Provider string `json:"provider"`
	}
	err = m.call(ctx, "create", map[string]any{"sandbox": id, "template": template, "timeout_seconds": m.options.TimeoutSeconds, "allow_internet_access": env.AllowNetwork}, &created)
	if err != nil {
		var apiError *APIError
		if errors.As(err, &apiError) && apiError.HTTPStatus >= 400 && apiError.HTTPStatus < 500 {
			// The adapter rejected this request before allocation. In particular,
			// a duplicate handle does not grant ownership of an existing resource.
			m.mu.Lock()
			delete(m.owned, s)
			m.mu.Unlock()
			return nil, err
		}
		// Keep the client-chosen identity for cleanup after a lost response.
		return s, err
	}
	if created.Sandbox != id || created.State != "active" {
		return s, errors.New("E2B create identity mismatch")
	}
	if created.Provider != m.options.Provider && (created.Provider != "" || m.options.Provider != "e2b-cloud") {
		return s, errors.New("sandbox adapter create provider mismatch")
	}
	return s, nil
}

func (m *Manager) Stop(ctx context.Context, target runner.Sandbox) error {
	s, ok := target.(*Sandbox)
	if !ok || s.manager != m {
		return errors.New("E2B sandbox is not owned by this manager")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.absent {
		return nil
	}
	var response struct {
		Confirmed bool `json:"absence_confirmed"`
	}
	if err := m.call(ctx, "kill", map[string]any{"sandbox": s.id}, &response); err != nil {
		return err
	}
	if !response.Confirmed {
		return errors.New("E2B sandbox absence unconfirmed")
	}
	s.absent = true
	m.mu.Lock()
	delete(m.owned, s)
	m.mu.Unlock()
	return nil
}

func (m *Manager) Close() error {
	m.mu.Lock()
	owned := make([]*Sandbox, 0, len(m.owned))
	for s := range m.owned {
		owned = append(owned, s)
	}
	m.mu.Unlock()
	var failures []error
	for _, s := range owned {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		err := m.Stop(ctx, s)
		cancel()
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (s *Sandbox) ID() string                             { return s.id }
func (s *Sandbox) Connectivity() core.HarnessConnectivity { return s.manager.options.Connectivity }

func (s *Sandbox) StartProcess(ctx context.Context, command core.Command) (*Process, error) {
	if command.Path == "" || strings.ContainsRune(command.Path, 0) {
		return nil, errors.New("E2B command path required")
	}
	if command.Dir == "" {
		command.Dir = s.workdir
	}
	if !strings.HasPrefix(command.Dir, "/") {
		return nil, errors.New("E2B command directory must be absolute")
	}
	if command.User == "" {
		command.User = s.user
	}
	if command.User == "0" {
		command.User = "root"
	}
	if strings.Contains(command.User, ":") {
		return nil, errors.New("E2B SDK expects a username; UID:GID is unsupported")
	}
	if len(s.defaultEnv) != 0 {
		env := maps.Clone(s.defaultEnv)
		maps.Copy(env, command.Env)
		command.Env = env
	}
	timeout := command.Timeout
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if timeout <= 0 || remaining < timeout {
			timeout = remaining
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = time.Duration(s.manager.options.TimeoutSeconds) * time.Second
	}
	limit := command.OutputLimitBytes
	if limit <= 0 {
		limit = 16 << 20
	}
	id, err := identity()
	if err != nil {
		return nil, err
	}
	observation, err := processObservationFromContext(ctx, id)
	if err != nil {
		return nil, err
	}
	p := &Process{ID: id, sandbox: s, started: time.Now(), executionID: performance.ExecutionID(ctx), observation: observation}
	body := map[string]any{"sandbox": s.id, "process": id, "argv": append([]string{command.Path}, command.Args...), "cwd": command.Dir, "env": command.Env, "user": command.User, "timeout_seconds": int(math.Ceil(timeout.Seconds())), "output_limit_bytes": limit}
	if observation != nil {
		body["observation"] = observation
	}
	if command.Env == nil {
		body["env"] = map[string]string{}
	}
	var response struct {
		ID  string `json:"process"`
		PID int    `json:"pid"`
	}
	if err := s.manager.call(ctx, "start", body, &response); err != nil {
		return p, err
	}
	if response.ID != id || response.PID <= 0 {
		return p, errors.New("E2B process identity mismatch")
	}
	p.PID = response.PID
	return p, nil
}

func (p *Process) SendStdin(ctx context.Context, data []byte, close bool) error {
	ctx = p.boundContext(ctx)
	return p.sandbox.manager.call(ctx, "stdin", map[string]any{"sandbox": p.sandbox.id, "process": p.ID, "data_base64": base64.StdEncoding.EncodeToString(data), "close": close}, nil)
}
func (p *Process) Status(ctx context.Context) (ProcessStatus, error) {
	ctx = p.boundContext(ctx)
	var result ProcessStatus
	err := p.sandbox.manager.call(ctx, "process", map[string]any{"sandbox": p.sandbox.id, "process": p.ID}, &result)
	return result, err
}
func (p *Process) Kill(ctx context.Context) (ProcessStatus, error) {
	ctx = p.boundContext(ctx)
	var result ProcessStatus
	err := p.sandbox.manager.call(ctx, "kill_process", map[string]any{"sandbox": p.sandbox.id, "process": p.ID}, &result)
	return result, err
}

func (p *Process) Wait(ctx context.Context, stdout, stderr io.Writer) (result core.CommandResult, err error) {
	ctx = p.boundContext(ctx)
	result = core.CommandResult{ExitCode: -1}
	var span *operationSpan
	if p.sandbox.observation != nil {
		span = p.sandbox.observation.beginAt(ctx, p.sandbox, "exec.wait", p.started, p.ID)
	}
	// Wait observes the submitted process, including submission and pipe drain.
	// The separate RPC records retain their own boundaries.
	var stdoutBytes, stderrBytes int64
	var exitCode *int
	var guestDuration *int64
	var guestBoundaries *GuestBoundaries
	var guestIdentityReceipt json.RawMessage
	defer func() {
		// A terminal in an invalid/truncated stream is not completion evidence.
		if err != nil {
			exitCode, guestDuration = nil, nil
		}
		if span != nil {
			span.record.GuestIdentityReceipt = guestIdentityReceipt
		}
		span.end(err, exitCode, &stdoutBytes, &stderrBytes, nil, guestDuration, guestBoundaries, nil, nil)
	}()
	cursor := 0
	terminal := false
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	for {
		var response struct {
			Events []json.RawMessage `json:"events"`
			Cursor int               `json:"cursor"`
			Done   bool              `json:"done"`
		}
		err := p.sandbox.manager.call(ctx, "events", map[string]any{"sandbox": p.sandbox.id, "process": p.ID, "cursor": cursor}, &response)
		result.Duration = time.Since(p.started)
		if err != nil {
			return result, err
		}
		if response.Cursor != cursor+len(response.Events) {
			return result, errors.New("E2B output cursor mismatch")
		}
		cursor = response.Cursor
		for _, raw := range response.Events {
			if terminal {
				return result, errors.New("E2B duplicate process terminal")
			}
			var event processEvent
			if err := json.Unmarshal(raw, &event); err != nil {
				return result, errors.New("invalid E2B process event")
			}
			if p.observation != nil {
				fields, fieldErr := observationObject(raw)
				if fieldErr != nil {
					return result, errors.New("invalid E2B identity event")
				}
				if err := p.observation.verifyEnvelope(event.Observation); err != nil {
					return result, err
				}
				if event.Kind != "" {
					if event.Kind != "execution_identity" || event.Phase != "before_popen" || guestIdentityReceipt != nil || len(fields) != 3 {
						return result, errors.New("invalid or duplicate E2B guest identity receipt")
					}
					guestIdentityReceipt = bytes.Clone(raw)
					continue
				}
				if guestIdentityReceipt == nil && event.Error == "" {
					return result, errors.New("E2B guest identity receipt missing before execution output")
				}
			} else if event.Kind != "" || event.Observation != nil {
				return result, errors.New("unexpected E2B guest identity for unlinked process")
			}
			if event.Error != "" {
				return result, errors.New("E2B process stream failed; termination is not confirmed")
			}
			if event.ExitCode != nil {
				if event.GuestDurationNS == nil || *event.GuestDurationNS < 0 || event.Stream != "" || event.Data != "" {
					return result, errors.New("invalid E2B guest terminal measurement")
				}
				for _, boundary := range []*int64{event.PopenReturnNS, event.FirstOutputNS, event.ChildExitNS, event.PipeEOFNS} {
					if boundary != nil && *boundary < 0 {
						return result, errors.New("invalid E2B guest boundary measurement")
					}
				}
				terminal = true
				result.ExitCode = *event.ExitCode
				exitCode, guestDuration = event.ExitCode, event.GuestDurationNS
				if event.PopenReturnNS != nil || event.FirstOutputNS != nil || event.ChildExitNS != nil || event.PipeEOFNS != nil {
					guestBoundaries = &GuestBoundaries{PopenReturnNS: event.PopenReturnNS,
						FirstOutputNS: event.FirstOutputNS, ChildExitObservedNS: event.ChildExitNS,
						PipeEOFNS: event.PipeEOFNS}
				}
				continue
			}
			data, err := base64.StdEncoding.DecodeString(event.Data)
			if err != nil {
				return result, errors.New("invalid E2B output frame")
			}
			var out io.Writer
			switch event.Stream {
			case "stdout":
				out = stdout
				stdoutBytes += int64(len(data))
			case "stderr":
				out = stderr
				stderrBytes += int64(len(data))
			default:
				return result, errors.New("invalid E2B output stream")
			}
			if _, err := out.Write(data); err != nil {
				return result, err
			}
		}
		if response.Done {
			if !terminal {
				return result, errors.New("E2B process terminal missing")
			}
			return result, nil
		}
	}
}

func (s *Sandbox) Exec(ctx context.Context, command core.Command) (core.CommandResult, error) {
	var stdout, stderr bytes.Buffer
	result, err := s.ExecStream(ctx, command, bytes.NewReader(command.Stdin), &stdout, &stderr)
	result.Stdout, result.Stderr = stdout.String(), stderr.String()
	return result, err
}

func (s *Sandbox) ExecStream(ctx context.Context, command core.Command, stdin io.Reader, stdout, stderr io.Writer) (core.CommandResult, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if command.Timeout > 0 {
		var stop context.CancelFunc
		ctx, stop = context.WithTimeout(ctx, command.Timeout)
		defer stop()
	}
	p, err := s.StartProcess(ctx, command)
	if err != nil {
		return core.CommandResult{ExitCode: -1}, err
	}
	inputDone := make(chan error, 1)
	go func() {
		if stdin != nil {
			buffer := make([]byte, 32<<10)
			for {
				n, readErr := stdin.Read(buffer)
				if n > 0 {
					if err := p.SendStdin(ctx, buffer[:n], false); err != nil {
						inputDone <- err
						var apiError *APIError
						if !(errors.As(err, &apiError) && apiError.Code == "stdin_closed") {
							cancel()
						}
						return
					}
				}
				if readErr == io.EOF {
					break
				}
				if readErr != nil {
					inputDone <- readErr
					cancel()
					return
				}
				if ctx.Err() != nil {
					inputDone <- ctx.Err()
					return
				}
			}
		}
		inputErr := p.SendStdin(ctx, nil, true)
		inputDone <- inputErr
		var apiError *APIError
		if inputErr != nil && !(errors.As(inputErr, &apiError) && apiError.Code == "stdin_closed") {
			cancel()
		}
	}()
	result, err := p.Wait(ctx, stdout, stderr)
	// Collect input failures before our own transport teardown can produce one.
	// Real input failures publish inputDone before canceling the wait context.
	select {
	case inputErr := <-inputDone:
		var apiError *APIError
		if inputErr != nil && !(errors.As(inputErr, &apiError) && apiError.Code == "stdin_closed") {
			err = errors.Join(err, inputErr)
		}
	default:
	}
	cancel() // Transport cleanup is not a process-termination certificate.
	return result, err
}

func (s *Sandbox) Upload(ctx context.Context, source, destination string) error {
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("E2B upload requires regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxFileBytes+1))
	if err != nil {
		return err
	}
	if len(data) > maxFileBytes {
		return errors.New("E2B upload exceeds 64 MiB")
	}
	return s.manager.call(ctx, "upload", map[string]any{"sandbox": s.id, "path": destination, "data_base64": base64.StdEncoding.EncodeToString(data)}, nil)
}
func (s *Sandbox) Download(ctx context.Context, source, destination string) error {
	return s.DownloadLimit(ctx, source, destination, maxFileBytes)
}
func (s *Sandbox) DownloadLimit(ctx context.Context, source, destination string, maxBytes int64) error {
	if maxBytes < 0 || maxBytes > maxFileBytes {
		return errors.New("E2B download limit must be between 0 and 64 MiB")
	}
	rootPath, err := filepath.Abs(s.manager.options.OutputDir)
	if err != nil {
		return err
	}
	dest, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(rootPath, dest)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("E2B artifact destination escapes output directory")
	}
	var response struct {
		Data string `json:"data_base64"`
	}
	if err := s.manager.call(ctx, "download", map[string]any{"sandbox": s.id, "path": source, "max_bytes": maxBytes}, &response); err != nil {
		return err
	}
	data, err := base64.StdEncoding.DecodeString(response.Data)
	if err != nil || int64(len(data)) > maxBytes {
		return errors.New("invalid or oversized E2B file response")
	}
	if err := os.MkdirAll(rootPath, 0700); err != nil {
		return err
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.MkdirAll(filepath.Dir(rel), 0700); err != nil {
		return err
	}
	file, err := root.OpenFile(rel, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	return errors.Join(writeErr, file.Close())
}
func (s *Sandbox) GuestEndpoint(ctx context.Context, port int) (GuestEndpoint, error) {
	var result GuestEndpoint
	err := s.manager.call(ctx, "guest", map[string]any{"sandbox": s.id, "port": port}, &result)
	return result, err
}
func (s *Sandbox) Metrics(ctx context.Context) (Metrics, error) {
	var result Metrics
	err := s.manager.call(ctx, "metrics", map[string]any{"sandbox": s.id}, &result)
	return result, err
}

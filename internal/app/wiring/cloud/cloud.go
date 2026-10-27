// Package cloud composes the bounded coding workload with the ordinary Runner.
package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hyscale-lab/aries/internal/app"
	"github.com/hyscale-lab/aries/pkg/benchmark/codingcase"
	"github.com/hyscale-lab/aries/pkg/benchmark/terminalbench"
	"github.com/hyscale-lab/aries/pkg/config"
	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/deployment"
	dockerdeployment "github.com/hyscale-lab/aries/pkg/deployment/docker"
	"github.com/hyscale-lab/aries/pkg/harness"
	"github.com/hyscale-lab/aries/pkg/harness/jiuwenswarm"
	"github.com/hyscale-lab/aries/pkg/harness/openclaw"
	"github.com/hyscale-lab/aries/pkg/harness/openclawcloud"
	"github.com/hyscale-lab/aries/pkg/monitor"
	"github.com/hyscale-lab/aries/pkg/performance"
	"github.com/hyscale-lab/aries/pkg/runner"
	"github.com/hyscale-lab/aries/pkg/sandbox/e2b"
	"github.com/sirupsen/logrus"
)

const cleanupTimeout = 45 * time.Second
const proxyTokenEnv = "ARIES_MODEL_PROXY_ACCESS_TOKEN"

type preparedRun struct {
	mu                       sync.Mutex
	config                   config.CloudConfig
	runID, outputRoot, token string
	logger                   *logrus.Logger
	placement                core.RuntimePlacement
	network                  deployment.RunEnvironment
	transport                *dockerdeployment.Manager
	attempts                 map[string]*attempt
	catalog                  *terminalbench.Benchmark
	binding                  *e2b.TaskTemplateBinding
	taskTimeout              time.Duration
	closed                   bool
}

// Prepare owns the external harness network only. Cloud allocations remain owned
// by each occurrence; embedded placement never constructs a Docker transport.
func Prepare(ctx context.Context, cfg config.CloudConfig, runID, outputRoot string, lookup func(string) ([]byte, bool), logger *logrus.Logger) (app.CloudRun, error) {
	if err := harness.ValidateRunID("cloud", runID); err != nil {
		return app.CloudRun{}, err
	}
	if (cfg.SandboxBackend != "e2b-cloud" && cfg.SandboxBackend != "huawei-agentsphere") || (cfg.Placement != "external" && cfg.Placement != "embedded") {
		return app.CloudRun{}, errors.New("cloud wiring requires external or embedded placement on e2b-cloud or huawei-agentsphere")
	}
	if err := cfg.ValidateAgent(); err != nil {
		return app.CloudRun{}, err
	}
	if outputRoot == "" || lookup == nil {
		return app.CloudRun{}, errors.New("cloud output directory and credential lookup are required")
	}
	outputRoot, err := filepath.Abs(outputRoot)
	if err != nil {
		return app.CloudRun{}, err
	}
	p := &preparedRun{config: cfg, runID: runID, outputRoot: outputRoot, logger: logger, attempts: make(map[string]*attempt)}
	if err := p.prepareTerminalBench(ctx); err != nil {
		return app.CloudRun{}, err
	}
	cfg = p.config // The task binding supplies an omitted TerminalBench template.
	token, ok := lookup(cfg.Sandbox.TokenEnv)
	if !ok {
		return app.CloudRun{}, errors.New("cloud sandbox adapter credential is missing")
	}
	defer clear(token)
	options := e2b.Options{Endpoint: cfg.Sandbox.AdapterURL, Provider: cfg.SandboxBackend, Token: string(token), Template: cfg.Sandbox.Template, OutputDir: outputRoot, TimeoutSeconds: cfg.Sandbox.TimeoutSeconds,
		ObservationLevel: performance.ObservationLevelFromContext(ctx), AssumeNetworkIsolation: cfg.Sandbox.AssumeNetworkIsolation}
	manager, err := e2b.New(options)
	if err != nil {
		return app.CloudRun{}, err
	}
	if cfg.SandboxBackend == "huawei-agentsphere" {
		if err := manager.CheckProvider(ctx); err != nil {
			return app.CloudRun{}, err
		}
	}
	if cfg.Placement == "external" {
		ip := net.ParseIP(cfg.Harness.BridgeHost)
		if ip == nil || !ip.IsPrivate() || ip.IsLoopback() || ip.IsUnspecified() {
			return app.CloudRun{}, errors.New("external Docker harness requires the ECS private bridge bind IP")
		}
		if cfg.AgentType == "openclaw" {
			if !filepath.IsAbs(cfg.Harness.SSHClientPath) {
				return app.CloudRun{}, errors.New("external harness requires an absolute Linux SSH client path")
			}
			info, err := os.Lstat(cfg.Harness.SSHClientPath)
			if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o555 {
				return app.CloudRun{}, errors.New("external SSH client must be a regular Linux executable with mode 0555")
			}
		}
	}
	p.token = string(token)
	if cfg.Placement == "external" {
		image := cfg.Versions.OpenClaw.Image
		if cfg.AgentType == "jiuwenswarm" {
			image = cfg.Harness.JiuwenImage
		}
		if err := dockerdeployment.PullImages(ctx, cfg.Harness.DockerSocket, []string{image}); err != nil {
			return app.CloudRun{}, err
		}
		p.transport, err = dockerdeployment.New(dockerdeployment.Options{Socket: cfg.Harness.DockerSocket, Logger: logger})
		if err != nil {
			return app.CloudRun{}, err
		}
		p.network = p.transport.NewRunEnvironment(runID)
		p.placement, err = p.network.Start(ctx)
		if err != nil {
			return app.CloudRun{Close: p.close}, errors.Join(err, p.close(context.Background()))
		}
	}
	return app.CloudRun{NewAttempt: p.newAttempt, Close: p.close, TaskTimeout: p.taskTimeout}, nil
}

func (p *preparedRun) newAttempt(ctx context.Context, request app.CloudAttempt) (app.CloudExperiment, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return app.CloudExperiment{}, err
	}
	if p.closed || request.RunID != p.runID || request.LogicalID == "" {
		return app.CloudExperiment{}, errors.New("cloud attempt does not match an open run")
	}
	if p.config.Benchmark != nil && (p.catalog == nil || p.binding == nil || request.LogicalID != p.config.Benchmark.LogicalTaskID()) {
		return app.CloudExperiment{}, errors.New("cloud attempt does not match the prepared TerminalBench task")
	}
	if err := harness.ValidateTaskID("cloud", request.OccurrenceID); err != nil {
		return app.CloudExperiment{}, err
	}
	if _, exists := p.attempts[request.OccurrenceID]; exists {
		return app.CloudExperiment{}, errors.New("cloud occurrence cannot be reused")
	}
	root, err := filepath.Abs(request.OutputDir)
	if err != nil || root != p.outputRoot {
		return app.CloudExperiment{}, errors.New("cloud attempt output root does not match the run")
	}
	if len(request.AccessToken) == 0 || bytes.ContainsAny(request.AccessToken, "\x00\r\n") {
		return app.CloudExperiment{}, errors.New("cloud attempt requires a scoped model proxy token")
	}
	model := request.Model
	model.APIKeyEnv = proxyTokenEnv
	if err := harness.ValidateModel("cloud proxy", model); err != nil {
		return app.CloudExperiment{}, err
	}
	a := &attempt{occurrenceID: request.OccurrenceID, outputRoot: root, proxyToken: bytes.Clone(request.AccessToken)}
	p.attempts[request.OccurrenceID] = a
	fail := func(err error) (app.CloudExperiment, error) {
		return app.CloudExperiment{}, errors.Join(err, a.close())
	}
	level := performance.ObservationLevelFromContext(ctx)
	templateTaskID := request.LogicalID
	if p.config.Benchmark != nil {
		templateTaskID = p.config.Benchmark.TaskID
	}
	manager, err := e2b.New(e2b.Options{Endpoint: p.config.Sandbox.AdapterURL, Provider: p.config.SandboxBackend, Token: p.token, Template: p.config.Sandbox.Template,
		TemplateMappings: map[string]string{p.config.Sandbox.Template: p.config.Sandbox.Template}, OutputDir: root, LogicalTaskID: request.LogicalID, TemplateTaskID: templateTaskID,
		TimeoutSeconds: p.config.Sandbox.TimeoutSeconds, Connectivity: core.HarnessConnectivity{Placement: p.placement}, ObservationLevel: level,
		Clock:                  performance.ClockFromContext(ctx),
		TemplateBinding:        p.binding,
		AssumeNetworkIsolation: p.config.Sandbox.AssumeNetworkIsolation})
	if err != nil {
		return fail(err)
	}
	a.sandboxes = newObservedSandboxes(manager, p.runID, request.OccurrenceID, root, level)
	var benchmark runner.Benchmark
	workdir := codingcase.Workdir
	var commandEnv map[string]string
	if p.catalog != nil {
		benchmark, err = p.catalog.ForTask(request.LogicalID, request.OccurrenceID)
		workdir = p.binding.Workdir
		commandEnv = p.binding.DefaultEnvironment()
	} else {
		benchmark, err = codingcase.New(codingcase.Options{TaskID: request.OccurrenceID, Workload: p.config.Workload, Template: p.config.Sandbox.Template, OutputDir: root,
			TaskTimeout: p.config.Execution.TaskTimeout, VerifierTimeout: p.config.Execution.VerifierTimeout})
	}
	if err != nil {
		return fail(err)
	}
	if p.config.AgentType == "jiuwenswarm" {
		if p.config.Placement == "external" {
			bridge, err := jiuwenswarm.NewRemoteBridge(jiuwenswarm.RemoteBridgeOptions{RunID: p.runID, TaskID: request.OccurrenceID,
				OutputDir: filepath.Join(root, request.OccurrenceID, "bridge"), Workdir: workdir,
				SourceDir: p.config.Harness.JiuwenSourceDir, PythonPath: p.config.Harness.JiuwenPythonPath,
				BindHost: p.config.Harness.BridgeHost, AdvertiseHost: p.config.Harness.BridgeHost,
				GuestEndpoint: guestEndpoint, CleanupTimeout: cleanupTimeout, ObservationLevel: level})
			if err != nil {
				return fail(err)
			}
			a.bridge = bridge
			transport, err := dockerdeployment.New(dockerdeployment.Options{Socket: p.config.Harness.DockerSocket, Logger: p.logger})
			if err != nil {
				return fail(err)
			}
			external, err := jiuwenswarm.NewExternal(jiuwenswarm.ExternalOptions{Runtime: harness.RuntimeOptions{
				Deployment: transport, Image: p.config.Harness.JiuwenImage, OutputDir: root,
				APIKeyLookup: a.lookupProxy, Logger: p.logger, CleanupTimeout: cleanupTimeout, AgentTimeout: p.config.Execution.TaskTimeout},
				Bridge: bridge, Workdir: workdir, SourceDir: p.config.Harness.JiuwenSourceDir, PythonPath: p.config.Harness.JiuwenPythonPath,
				ModelProxyURL: model.BaseURL, ProxyAccessToken: a.proxyToken, ObservationLevel: level})
			if err != nil {
				return fail(errors.Join(err, transport.Close()))
			}
			a.harness, a.closeTransport = external, external.Close
		} else {
			embedded, err := jiuwenswarm.NewEmbedded(jiuwenswarm.EmbeddedOptions{OutputDir: root, Workdir: workdir,
				SourceDir: p.config.Harness.JiuwenSourceDir, PythonPath: p.config.Harness.JiuwenPythonPath, ModelProxyURL: model.BaseURL,
				ProxyAccessToken: a.proxyToken, GuestEndpoint: guestEndpoint, CleanupTimeout: cleanupTimeout, ObservationLevel: level})
			if err != nil {
				return fail(err)
			}
			a.harness, a.bridge = embedded, embedded.Bridge()
		}
	} else {
		workloadInputs := &openclaw.WorkloadInputs{
			SessionKey: "agent:main:aries-workload", SessionID: "8ad5eb11-e541-4cbf-8ae0-1f9e2822c342", EnvelopeTimestamp: "off", SkipBootstrap: true,
			ToolOutputRenderer: p.config.Harness.ToolOutputRenderer,
			ReplayToolProtocol: p.config.Harness.ReplayToolProtocol,
		}
		if p.config.Placement == "external" {
			var nativeIdentity *openclaw.NativeIdentityScope
			if p.config.Harness.NativeObservationProtocol != "" {
				nativeIdentity = &openclaw.NativeIdentityScope{ScopeID: p.runID + "/" + request.OccurrenceID, AgentSessionID: workloadInputs.SessionKey}
			}
			transport, err := dockerdeployment.New(dockerdeployment.Options{Socket: p.config.Harness.DockerSocket, Logger: p.logger})
			if err != nil {
				return fail(err)
			}
			native, err := openclaw.New(openclaw.Options{Runtime: harness.RuntimeOptions{Deployment: transport, Image: p.config.Versions.OpenClaw.Image,
				OutputDir: root, APIKeyLookup: a.lookupProxy, Logger: p.logger, CleanupTimeout: cleanupTimeout}, Common: harness.Options{Mode: openclaw.ModeAgent}, ExpectedVersion: p.config.Harness.OpenClawVersion, WorkloadInputs: workloadInputs, NativeIdentity: nativeIdentity, ObserveNativeTools: performance.ObservesNative(level)})
			if err != nil {
				return fail(errors.Join(err, transport.Close()))
			}
			a.harness, a.closeTransport = native, native.Close
			bridgeOptions := openclawcloud.ExternalBridgeOptions{RunID: p.runID, TaskID: request.OccurrenceID,
				Workdir: workdir, CommandEnv: commandEnv, OutputDir: filepath.Join(root, request.OccurrenceID, "bridge"), BindHost: p.config.Harness.BridgeHost, AdvertiseHost: p.config.Harness.BridgeHost,
				ClientSourceFile: p.config.Harness.SSHClientPath, CleanupTimeout: cleanupTimeout}
			if p.binding != nil {
				bridgeOptions.InitialDirectoryMtimeUnixNS = p.binding.InitialDirectoryMtimeUnixNS
			}
			if nativeIdentity != nil {
				bridgeOptions.IdentityScopeID, bridgeOptions.IdentitySessionID = nativeIdentity.ScopeID, nativeIdentity.AgentSessionID
			}
			a.bridge, err = openclawcloud.NewExternalBridge(bridgeOptions)
			if err != nil {
				return fail(err)
			}
		} else {
			embedded, err := openclawcloud.NewEmbedded(openclawcloud.EmbeddedOptions{OutputDir: root, Workdir: workdir,
				OpenClawPath: p.config.Harness.OpenClawPath, ExpectedVersion: p.config.Harness.OpenClawVersion, ModelProxyURL: model.BaseURL,
				ProxyAccessToken: a.proxyToken, GuestEndpoint: guestEndpoint, CleanupTimeout: cleanupTimeout, WorkloadInputs: workloadInputs, ObservationLevel: level,
				ObserveNativeProcess: p.config.Harness.NativeObservationProtocol == "aries-native-process-v1"})
			if err != nil {
				return fail(err)
			}
			a.harness, a.bridge = embedded, embedded.Bridge()
		}
	}
	if p.config.Placement == "external" && performance.ObservesResources(level) {
		a.hostSource, err = dockerdeployment.NewResourceSource(dockerdeployment.ResourceOptions{RunID: p.runID, TaskIDs: []string{request.OccurrenceID}, DockerSocket: p.config.Harness.DockerSocket})
		if err != nil {
			return fail(err)
		}
		a.hostMonitor, err = monitor.New(monitor.Options{RunID: p.runID, TaskIDs: []string{request.OccurrenceID}, OutputDir: root, Source: a.hostSource, Logger: p.logger})
		if err != nil {
			return fail(err)
		}
	}
	runnerHarness := a.harness
	if request.OnHarnessRunReturned != nil {
		runnerHarness = modelAdmissionHarness{AgentHarness: a.harness, returned: request.OnHarnessRunReturned}
	}
	a.runner, err = runner.New(benchmark, runnerHarness, a.sandboxes, a.bridge, runner.Options{Name: p.config.Name, RunID: p.runID, OutputDir: root, Model: model, Logger: p.logger, CleanupTimeout: cleanupTimeout})
	if err != nil {
		return fail(err)
	}
	return app.CloudExperiment{Run: a.run, Close: a.close, Observation: a.observation}, nil
}

func guestEndpoint(ctx context.Context, live runner.Sandbox, port int) (string, http.Header, error) {
	sandbox, ok := live.(*e2b.Sandbox)
	if !ok {
		return "", nil, errors.New("embedded guest endpoint requires a cloud adapter sandbox")
	}
	endpoint, err := sandbox.GuestEndpoint(ctx, port)
	if err != nil {
		return "", nil, err
	}
	headers := make(http.Header, len(endpoint.Headers))
	for name, value := range endpoint.Headers {
		headers.Set(name, value)
	}
	return endpoint.URL, headers, nil
}

func (p *preparedRun) close(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	var errs []error
	for _, a := range p.attempts {
		errs = append(errs, a.close())
	}
	if p.network != nil {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
		err := p.network.Stop(cleanup)
		cancel()
		errs = append(errs, err)
		if err == nil {
			p.network = nil
		}
	}
	if p.transport != nil && p.network == nil {
		errs = append(errs, p.transport.Close())
		p.transport = nil
	}
	p.token = ""
	return errors.Join(errs...)
}

type attempt struct {
	mu                       sync.Mutex
	occurrenceID, outputRoot string
	proxyToken               []byte
	runner                   *runner.Runner
	harness                  runner.AgentHarness
	bridge                   runner.ToolBridge
	sandboxes                *observedSandboxes
	closeTransport           func() error
	hostSource               monitor.ResourceSource
	hostMonitor              *monitor.Recorder
	hostAttempted            bool
	hostStarted              bool
	hostStartErr             error
	ran, closed              bool
	observer                 core.ObserverResult
	observerErr              error
}

func (a *attempt) lookupProxy(name string) ([]byte, bool) {
	if name != proxyTokenEnv || len(a.proxyToken) == 0 {
		return nil, false
	}
	return bytes.Clone(a.proxyToken), true
}

func (a *attempt) run(ctx context.Context) (core.RunResult, error) {
	a.mu.Lock()
	if a.ran || a.closed {
		a.mu.Unlock()
		return core.RunResult{}, errors.New("cloud attempt is single use")
	}
	a.ran = true
	if a.hostMonitor != nil {
		a.hostAttempted = true
		a.hostStartErr = a.hostMonitor.Start(ctx)
		a.hostStarted = a.hostStartErr == nil
	}
	a.mu.Unlock()
	return a.runner.Run(ctx)
}

func (a *attempt) observation() (core.ObserverResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.observer, a.observerErr
}

func (a *attempt) finishObservation() {
	if a.observer.Status != "" || a.sandboxes == nil {
		return
	}
	raw, rawErr := a.sandboxes.finish()
	operations := core.ObserverResult{Status: core.StatusNotRun}
	var operationErr error
	if a.sandboxes.manager != nil {
		operations, operationErr = a.sandboxes.manager.FinishObservation()
	}
	host := core.ObserverResult{Status: core.StatusUnsupported, Reason: "embedded agent/tool resource separation is not supplied by the E2B SDK"}
	hostErr := a.hostStartErr
	if a.hostMonitor != nil {
		host = core.ObserverResult{Status: core.StatusNotRun, Reason: "external harness monitoring was not started"}
	}
	if a.hostStarted {
		ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		reports, err := a.hostMonitor.Stop(ctx)
		cancel()
		hostErr = err
		host = reports[a.occurrenceID]
	}
	if hostErr != nil {
		host.Status, host.Error = core.StatusFailed, hostErr.Error()
	}
	var nativeIdentity *core.ObserverResult
	var nativeErr error
	if source, ok := a.harness.(interface {
		NativeIdentityObservation() (core.ObserverResult, error)
	}); ok {
		report, err := source.NativeIdentityObservation()
		if report.Status != core.StatusNotEnabled {
			nativeIdentity, nativeErr = &report, err
		}
	}
	var nativeTools *core.ObserverResult
	var nativeToolErr error
	if source, ok := a.harness.(interface {
		NativeToolObservation() (core.ObserverResult, error)
	}); ok {
		report, err := source.NativeToolObservation()
		if report.Status != core.StatusNotEnabled {
			nativeTools, nativeToolErr = &report, err
		}
	}
	a.observer = raw
	a.observer.Reason = "sample_count counts SDK snapshots, not independent samples; SDK operations, Docker host and SDK guest measurements are separate in cloud-observation.json"
	a.observer.LogPaths = append(a.observer.LogPaths, host.LogPaths...)
	a.observer.LogPaths = append(a.observer.LogPaths, operations.LogPaths...)
	if nativeIdentity != nil {
		a.observer.LogPaths = append(a.observer.LogPaths, nativeIdentity.LogPaths...)
	}
	if nativeTools != nil {
		a.observer.LogPaths = append(a.observer.LogPaths, nativeTools.LogPaths...)
	}
	path := filepath.Join(a.outputRoot, a.occurrenceID, "cloud-observation.json")
	content, writeErr := json.MarshalIndent(struct {
		SchemaVersion        int                         `json:"schema_version"`
		Sandbox              core.ObserverResult         `json:"sandbox_sdk"`
		Harness              core.ObserverResult         `json:"external_harness"`
		Operations           core.ObserverResult         `json:"sandbox_operations"`
		NativeIdentity       *core.ObserverResult        `json:"native_identity,omitempty"`
		NativeTools          *core.ObserverResult        `json:"native_tools,omitempty"`
		SandboxMetricsFileIO *performance.FileIOCounters `json:"sandbox_metrics_file_io,omitempty"`
	}{SchemaVersion: 1, Sandbox: raw, Harness: host, Operations: operations, NativeIdentity: nativeIdentity, NativeTools: nativeTools,
		SandboxMetricsFileIO: a.sandboxes.fileIOReport()}, "", "  ")
	if writeErr == nil {
		writeErr = writePrivate(path, content)
	}
	if writeErr == nil {
		a.observer.LogPaths = append(a.observer.LogPaths, path)
	}
	a.observerErr = errors.Join(rawErr, hostErr, operationErr, nativeErr, nativeToolErr, writeErr)
	if a.observerErr != nil {
		a.observer.Status, a.observer.Error = core.StatusFailed, a.observerErr.Error()
	}
}

func (a *attempt) close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return nil
	}
	var errs []error
	for _, stop := range []func(context.Context) error{
		func(ctx context.Context) error {
			if a.harness != nil {
				return a.harness.Stop(ctx)
			}
			return nil
		},
		func(ctx context.Context) error {
			if a.bridge != nil {
				return a.bridge.Stop(ctx)
			}
			return nil
		},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		errs = append(errs, stop(ctx))
		cancel()
	}
	if a.sandboxes != nil {
		// Stop sampling RPCs before deletion; join and flush after owner cleanup.
		a.sandboxes.stopSampling()
		if a.sandboxes.manager != nil {
			errs = append(errs, a.sandboxes.manager.Close())
		}
	}
	a.finishObservation()
	if a.hostSource != nil && !a.hostAttempted {
		errs = append(errs, a.hostSource.Close())
	}
	// A failed runtime stop must retain its usable transport for the next retry.
	if errors.Join(errs...) == nil && a.closeTransport != nil {
		errs = append(errs, a.closeTransport())
	}
	clear(a.proxyToken)
	if err := errors.Join(errs...); err != nil {
		return err
	}
	a.closed = true
	return nil
}

func writePrivate(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(content)
	return errors.Join(err, f.Close())
}

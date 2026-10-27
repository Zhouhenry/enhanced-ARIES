package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hyscale-lab/aries/internal/modelreplay"
	"github.com/hyscale-lab/aries/pkg/config"
	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/performance"
	"github.com/sirupsen/logrus"
)

type CloudAttempt struct {
	RunID, LogicalID, OccurrenceID, OutputDir string
	Model                                     core.ModelConfig
	AccessToken                               []byte
	OnHarnessRunReturned                      func()
}

type CloudExperiment struct {
	Run         func(context.Context) (core.RunResult, error)
	Close       func() error
	Observation func() (core.ObserverResult, error)
}

type CloudRun struct {
	NewAttempt func(context.Context, CloudAttempt) (CloudExperiment, error)
	Close      func(context.Context) error
	// TaskTimeout comes from the selected original benchmark task, when present.
	TaskTimeout time.Duration
}

// Prepare owns run-level transport and attachment; NewAttempt supplies the
// same four-role Runner used by the legacy command. It is not a runtime driver.
type CloudWiring struct {
	Prepare func(context.Context, config.CloudConfig, string, string, func(string) ([]byte, bool), *logrus.Logger) (CloudRun, error)
}

func RunCloud(ctx context.Context, profilePath string, stdout io.Writer, dependencies Dependencies, wiring CloudWiring, lookup func(string) ([]byte, bool), tapeOverride string) error {
	resources := newControllerResources()
	logger := dependencies.Logger
	if logger == nil {
		logger = newLogger()
	}
	dependencies.Logger = logger
	err := runCloud(ctx, profilePath, stdout, dependencies, wiring, lookup, tapeOverride, resources)
	// Run after all runCloud cleanup defers, including its model HTTP server.
	// A panic does not reach this call and cannot produce a false final sample.
	if resources.finish() != nil {
		logger.WithField("error_category", "controller_resources_finalize_failed").Warn("controller resource observation incomplete")
	}
	return err
}

func runCloud(ctx context.Context, profilePath string, stdout io.Writer, dependencies Dependencies, wiring CloudWiring, lookup func(string) ([]byte, bool), tapeOverride string, resources *controllerResources) error {
	cfg, err := config.LoadCloud(profilePath)
	if err != nil {
		return err
	}
	if err := cfg.ResolveEnvironment(lookup); err != nil {
		return err
	}
	if wiring.Prepare == nil || lookup == nil {
		return errors.New("cloud component wiring and credential lookup are required")
	}
	if tapeOverride != "" {
		if cfg.Model.Mode != "replay" {
			return errors.New("--tape is only valid for a replay profile")
		}
		cfg.Model.TapeFile, err = filepath.Abs(tapeOverride)
		if err != nil {
			return err
		}
	}
	var tape *modelreplay.Tape
	if cfg.Model.Mode == "replay" {
		tape, err = modelreplay.Load(cfg.Model.TapeFile)
		if err != nil {
			return err
		}
		if tape.Identity.Placement != cfg.Placement || tape.Identity.HarnessVersion != cfg.HarnessVersion() || tape.Model != cfg.Model.ID || tape.ToolCallIDDialect != cfg.Model.ToolCallIDDialect || tape.HistoryDialect != cloudHistoryDialect(cfg) || tape.SessionProtocol() != cfg.Model.SessionProtocol {
			return errors.New("replay tape must match placement, native harness version, configured model and declared dialects")
		}
	}
	runID := newRunID(time.Now(), cfg.Name)
	outputRoot, err := resolveOutputRoot(cfg.OutputDir, runID)
	if err != nil {
		return err
	}
	if err := createRunOutputRoot(outputRoot); err != nil {
		return err
	}
	if resources.begin(outputRoot, runID) != nil {
		dependencies.Logger.WithField("error_category", "controller_resources_start_failed").Warn("controller resource observation incomplete")
	}
	profileBytes, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := persistRunResult(filepath.Join(outputRoot, "cloud-profile.json"), append(profileBytes, '\n')); err != nil {
		return err
	}
	if cfg.Model.Mode == "record" {
		upstream := cfg.CoreModel(cfg.Model.BaseURL, cfg.Model.APIKeyEnv)
		// Catalog discovery uses the generic /v1 API; the harness keeps the
		// DeepSeek dialect and the record proxy keeps the configured upstream root.
		upstream.Provider = "openai"
		catalogURL, _ := url.Parse(upstream.BaseURL) // ResolveEnvironment validated it.
		if catalogURL.Path == "" || catalogURL.Path == "/" {
			catalogURL.Path = "/v1"
		}
		upstream.BaseURL = catalogURL.String()
		validation, preflightErr := validateLiveModel(ctx, upstream, lookup, dependencies.PreflightClient, dependencies.PreflightSleep)
		if err := errors.Join(preflightErr, persistLiveValidation(outputRoot, validation)); err != nil {
			return err
		}
	}
	logger := dependencies.Logger
	if logger == nil {
		logger = newLogger()
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	mux := &cloudModelProxy{sessions: make(map[[32]byte]http.Handler)}
	listener, err := net.Listen("tcp", cfg.Model.ProxyListen)
	if err != nil {
		return errors.New("cannot bind private cloud model proxy listener")
	}
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second, MaxHeaderBytes: 64 << 10,
		BaseContext: func(net.Listener) context.Context { return runCtx }, ErrorLog: log.New(io.Discard, "", 0)}
	serverDone := make(chan error, 1)
	go func() {
		serveErr := server.Serve(listener)
		serverDone <- serveErr
		if !errors.Is(serveErr, http.ErrServerClosed) {
			cancel()
		}
	}()
	defer func() { cancel(); _ = server.Close(); <-serverDone }()
	return executeAndRecord(runCtx, func(runCtx context.Context) (core.RunResult, error) {
		return runWithPerformance(runCtx, outputRoot, runID, func(runCtx context.Context) (core.RunResult, error) {
			infraStarted := time.Now()
			cloud, prepareErr := wiring.Prepare(runCtx, cfg, runID, outputRoot, lookup, logger)
			infrastructure := &core.InfrastructureResult{NetworkPolicy: "cloud-provider-and-private-controller", StartupDuration: time.Since(infraStarted),
				Observer: core.ObserverResult{Status: core.StatusUnsupported, Reason: "see occurrence-scoped provider and harness evidence"}, Cleanup: core.CleanupResult{Status: core.StatusNotNeeded}}
			if cfg.SandboxBackend == "huawei-agentsphere" && cfg.Sandbox.AssumeNetworkIsolation {
				infrastructure.NetworkPolicy = "assumed-huawei-verifier-egress-unverified"
			}
			if prepareErr != nil {
				infrastructure.Error = prepareErr.Error()
			}
			ids := make([]string, cfg.Execution.Attempts)
			for i := range ids {
				ids[i] = cfg.Workload
				if cfg.Benchmark != nil {
					ids[i] = cfg.Benchmark.LogicalTaskID()
				}
			}
			result, runErr := runProfile(runCtx, cfg.Name, runID, ids, cfg.Execution.Concurrency, 0, nil,
				func(attemptCtx context.Context, occurrence taskOccurrence) (core.RunResult, error) {
					if prepareErr != nil {
						return core.RunResult{}, fmt.Errorf("prepare cloud infrastructure: %w", prepareErr)
					}
					if cloud.NewAttempt == nil {
						return core.RunResult{}, errors.New("cloud attempt constructor missing")
					}
					return runCloudAttempt(attemptCtx, cfg, runID, outputRoot, occurrence, tape, mux, cloud, lookup)
				}, cfg.Execution.Goodput)
			if cloud.Close != nil {
				cleanupStarted := time.Now()
				cleanup, stop := context.WithTimeout(context.WithoutCancel(runCtx), 45*time.Second)
				closeErr := cloud.Close(cleanup)
				stop()
				infrastructure.CleanupDuration = time.Since(cleanupStarted)
				infrastructure.Cleanup.Status = core.StatusSucceeded
				if closeErr != nil {
					infrastructure.Cleanup.Status = core.StatusFailed
					infrastructure.Cleanup.Error = closeErr.Error()
				}
				runErr = errors.Join(runErr, closeErr)
			}
			result.Infrastructure = infrastructure
			return result, errors.Join(runErr, prepareErr)
		})
	}, outputRoot, stdout)
}

func runCloudAttempt(ctx context.Context, cfg config.CloudConfig, runID, root string, occurrence taskOccurrence, tape *modelreplay.Tape, mux *cloudModelProxy, cloud CloudRun, lookup func(string) ([]byte, bool)) (result core.RunResult, returnErr error) {
	var entropy [48]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return result, err
	}
	sessionID := "model-" + hex.EncodeToString(entropy[:16])
	token := []byte(hex.EncodeToString(entropy[16:]))
	defer clear(token)
	output := filepath.Join(root, occurrence.executionID)
	if err := os.Mkdir(output, 0700); err != nil {
		return result, err
	}
	publicURL := strings.TrimRight(cfg.Model.ProxyPublicURL, "/") + "/v1"
	modelTimeout := cfg.Execution.TaskTimeout
	if cloud.TaskTimeout > 0 {
		modelTimeout = cloud.TaskTimeout
	}
	options := modelreplay.Options{Mode: cfg.Model.Mode, Model: cfg.Model.ID, AccessToken: token, Tape: tape,
		ReplayPolicy:      cfg.Model.ReplayPolicy,
		ToolCallIDDialect: cfg.Model.ToolCallIDDialect,
		HistoryDialect:    cloudHistoryDialect(cfg),
		ObservationLevel:  performance.ObservationLevelFromContext(ctx),
		Clock:             performance.ClockFromContext(ctx),
		Identity:          modelreplay.Identity{RunID: runID, AttemptID: runID + "/" + occurrence.executionID, SessionID: sessionID, Placement: cfg.Placement, HarnessVersion: cfg.HarnessVersion()},
		OutputDir:         filepath.Join(output, "model"), MaxTurns: cfg.Model.MaxTurns, Timeout: modelTimeout,
		Timing: modelreplay.Timing{Mode: cfg.Model.Timing, InitialDelay: cfg.Model.InitialDelay, FrameInterval: cfg.Model.FrameInterval}}
	if cfg.Model.Mode == "record" {
		options.UpstreamURL = cfg.Model.BaseURL
		options.UpstreamKey, _ = lookup(cfg.Model.APIKeyEnv)
	}
	var proxy interface {
		http.Handler
		Close() error
		CloseAdmission()
		ObservationError() error
		ReplayPolicy() string
	}
	var err error
	if cfg.Model.SessionProtocol == modelreplay.BranchProtocol {
		proxy, err = modelreplay.NewBundle(options)
	} else {
		proxy, err = modelreplay.New(options)
	}
	clear(options.UpstreamKey)
	if err != nil {
		return result, err
	}
	sessionCtx, sessionCancel := context.WithCancel(ctx)
	key := sha256.Sum256(token)
	mux.mu.Lock()
	mux.sessions[key] = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCtx, cancel := context.WithCancel(r.Context())
		stop := context.AfterFunc(sessionCtx, cancel)
		defer stop()
		defer cancel()
		proxy.ServeHTTP(w, r.WithContext(requestCtx))
	})
	mux.mu.Unlock()
	defer func() {
		sessionCancel()
		mux.mu.Lock()
		delete(mux.sessions, key)
		mux.mu.Unlock()
		closeErr := proxy.Close()
		// Observability failure is separate from model/replay and task correctness.
		if observationErr := proxy.ObservationError(); observationErr != nil {
			for i := range result.Tasks {
				result.Tasks[i].Observer.Status = core.StatusFailed
				if result.Tasks[i].Observer.Error != "" {
					result.Tasks[i].Observer.Error += "\n"
				}
				result.Tasks[i].Observer.Error += observationErr.Error()
			}
		}
		sessionResult := core.ModelSessionResult{AttemptID: options.Identity.AttemptID, SessionID: sessionID, Mode: cfg.Model.Mode, Timing: cfg.Model.Timing, Status: core.StatusSucceeded, ArtifactDir: options.OutputDir}
		sessionResult.ReplayPolicy = proxy.ReplayPolicy()
		if cfg.Model.Mode == "record" {
			sessionResult.Timing = "live"
		}
		if closeErr != nil {
			sessionResult.Status = core.StatusFailed
			sessionResult.Error = closeErr.Error()
		}
		result.ModelSessions = append(result.ModelSessions, sessionResult)
		returnErr = errors.Join(returnErr, closeErr)
	}()
	experiment, err := cloud.NewAttempt(ctx, CloudAttempt{RunID: runID, LogicalID: occurrence.logicalID, OccurrenceID: occurrence.executionID, OutputDir: root,
		Model: cfg.CoreModel(publicURL, "ARIES_MODEL_PROXY_ACCESS_TOKEN"), AccessToken: token, OnHarnessRunReturned: proxy.CloseAdmission})
	defer func() {
		if experiment.Close != nil {
			returnErr = errors.Join(returnErr, experiment.Close())
		}
		if experiment.Observation != nil {
			report, observationErr := experiment.Observation()
			if observationErr != nil {
				report.Status = core.StatusFailed
				if report.Error == "" {
					report.Error = observationErr.Error()
				} else if report.Error != observationErr.Error() {
					report.Error += "\n" + observationErr.Error()
				}
			}
			for i := range result.Tasks {
				result.Tasks[i].Observer = report
			}
		}
	}()
	if err != nil {
		return result, err
	}
	if experiment.Run == nil {
		return result, errors.New("cloud Runner missing")
	}
	return experiment.Run(ctx)
}

func cloudHistoryDialect(cfg config.CloudConfig) string {
	if cfg.AgentType == "jiuwenswarm" {
		return modelreplay.JiuwenHistoryDialect
	}
	return ""
}

// The native harness accepts an origin + /v1. Route by the explicit per-attempt
// credential, never by prompt contents or an inferred dialogue history.
type cloudModelProxy struct {
	mu       sync.RWMutex
	sessions map[[32]byte]http.Handler
}

func (p *cloudModelProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	value := r.Header.Get("Authorization")
	var handler http.Handler
	if strings.HasPrefix(value, "Bearer ") {
		key := sha256.Sum256([]byte(strings.TrimPrefix(value, "Bearer ")))
		p.mu.RLock()
		handler = p.sessions[key]
		p.mu.RUnlock()
	}
	if handler == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"code":"UNAUTHORIZED","message":"active attempt credential required"}}`)
		return
	}
	handler.ServeHTTP(w, r)
}

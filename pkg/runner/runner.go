package runner

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/performance"
	"github.com/sirupsen/logrus"
)

const defaultCleanupTimeout = 30 * time.Second

// Options contains the few experiment-level inputs needed by the Runner.
type Options struct {
	Name             string
	RunID            string
	OutputDir        string
	Model            core.ModelConfig
	CleanupTimeout   time.Duration
	Logger           *logrus.Logger
	RuntimeOverrides RuntimeOverrides
}

type ResourceOverrides struct {
	CPU      *float64
	MemoryMB *int
}

// RuntimeOverrides contains independent sparse harness and sandbox changes.
type RuntimeOverrides struct {
	HarnessResources      ResourceOverrides
	AgentSandboxResources ResourceOverrides
	AgentTimeout          *time.Duration
}

// Runner composes exactly the benchmark, harness, sandbox, and bridge roles.
type Runner struct {
	benchmark        Benchmark
	harness          AgentHarness
	toolSandbox      ToolSandbox
	bridge           ToolBridge
	name             string
	runID            string
	outputDir        string
	model            core.ModelConfig
	cleanupTimeout   time.Duration
	logger           *logrus.Logger
	runtimeOverrides RuntimeOverrides
}

// New constructs a Runner explicitly and rejects missing roles.
func New(benchmark Benchmark, harness AgentHarness, toolSandbox ToolSandbox, bridge ToolBridge, options Options) (*Runner, error) {
	if benchmark == nil {
		return nil, errors.New("benchmark is required")
	}
	if harness == nil {
		return nil, errors.New("agent harness is required")
	}
	if toolSandbox == nil {
		return nil, errors.New("tool sandbox is required")
	}
	if bridge == nil {
		return nil, errors.New("tool bridge is required")
	}
	if options.RunID == "" {
		return nil, errors.New("run ID is required")
	}
	if options.CleanupTimeout <= 0 {
		options.CleanupTimeout = defaultCleanupTimeout
	}
	if options.Logger == nil {
		options.Logger = logrus.StandardLogger()
	}
	return &Runner{
		benchmark:        benchmark,
		harness:          harness,
		toolSandbox:      toolSandbox,
		bridge:           bridge,
		name:             options.Name,
		runID:            options.RunID,
		outputDir:        options.OutputDir,
		model:            options.Model,
		cleanupTimeout:   options.CleanupTimeout,
		logger:           options.Logger,
		runtimeOverrides: cloneRuntimeOverrides(options.RuntimeOverrides),
	}, nil
}

// Run executes tasks sequentially. A task failure is recorded without hiding
// later independent task results unless the run context itself is cancelled.
func (r *Runner) Run(ctx context.Context) (core.RunResult, error) {
	started := time.Now()
	result := core.RunResult{Name: r.name, RunID: r.runID}

	tasks, err := r.benchmark.Tasks(ctx)
	if err != nil {
		result.Duration = time.Since(started)
		return result, fmt.Errorf("list benchmark tasks: %w", err)
	}

	var runErrors []error
	for _, task := range tasks {
		if err := ctx.Err(); err != nil {
			runErrors = append(runErrors, fmt.Errorf("schedule task %q: %w", task.ID, err))
			break
		}
		taskResult, taskErr := r.runTask(ctx, task)
		result.Tasks = append(result.Tasks, taskResult)
		if taskErr != nil {
			runErrors = append(runErrors, fmt.Errorf("task %q: %w", task.ID, taskErr))
		}
	}

	result.Summary = summarize(result.Tasks)
	result.Duration = time.Since(started)
	return result, errors.Join(runErrors...)
}

func (r *Runner) runTask(ctx context.Context, task core.Task) (core.TaskResult, error) {
	originalTask := task
	sandboxEnvironment, harnessCPU, harnessMemory, harnessTimeout := effectiveRuntime(task, r.runtimeOverrides)
	started := time.Now()
	ctx = performance.WithIdentity(ctx, performance.Identity{OccurrenceID: task.ID, Purpose: "task"})
	ctx, taskSpan := performance.Begin(ctx, "task.e2e")
	result := newTaskResult(task.ID)
	result.StartedAt = started
	result.Timing = &core.TaskTiming{}
	r.logger.WithContext(ctx).WithField("task_id", task.ID).Info("task started")

	var (
		sandbox       Sandbox
		sandboxActive bool
		bridgeActive  bool
		harnessActive bool
		allErrors     []error
		cleanupErrors []error
		cleanupCtx    context.Context
		cleanupCancel context.CancelFunc
		cleanupUsed   bool
		evaluation    = &evaluationSandboxes{toolSandbox: r.toolSandbox, runID: r.runID, taskID: task.ID}
	)
	defer func() {
		if cleanupCancel != nil {
			cleanupCancel()
		}
	}()
	ensureCleanupContext := func() context.Context {
		if cleanupCtx == nil {
			cleanupCtx, cleanupCancel = context.WithTimeout(context.WithoutCancel(ctx), r.cleanupTimeout)
		}
		return cleanupCtx
	}
	finish := func() (core.TaskResult, error) {
		cleanupStarted := time.Now()
		cleanup := ensureCleanupContext()
		if evaluation.active() {
			cleanupUsed = true
			if err := evaluation.stop(cleanup); err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("cleanup evaluation sandbox: %w", err))
			}
		}
		if harnessActive {
			cleanupUsed = true
			stageCtx, stage := performance.Begin(cleanup, "harness.stop")
			err := r.harness.Stop(stageCtx)
			stage.End(err)
			if err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("cleanup harness: %w", err))
			} else {
				harnessActive = false
			}
		}
		if bridgeActive {
			cleanupUsed = true
			stageCtx, stage := performance.Begin(cleanup, "bridge.revoke")
			err := r.bridge.Stop(stageCtx)
			stage.End(err)
			if err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("cleanup bridge: %w", err))
			} else {
				bridgeActive = false
			}
		}
		if sandboxActive {
			cleanupUsed = true
			stageCtx, stage := performance.Begin(cleanup, "sandbox.stop")
			err := r.toolSandbox.Stop(stageCtx, sandbox)
			stage.End(err)
			if err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("cleanup sandbox: %w", err))
			} else {
				sandboxActive = false
			}
		}
		if !cleanupUsed {
			result.Cleanup.Status = core.StatusNotNeeded
		} else if len(cleanupErrors) == 0 {
			result.Cleanup.Status = core.StatusSucceeded
		} else {
			result.Cleanup.Status = core.StatusFailed
			result.Cleanup.Error = errors.Join(cleanupErrors...).Error()
			allErrors = append(allErrors, cleanupErrors...)
		}
		if cleanupUsed {
			cleanupFinished := time.Now()
			cleanupDuration := cleanupFinished.Sub(cleanupStarted)
			result.Timing.CleanupFinishedAt = &cleanupFinished
			result.Timing.CleanupDuration = &cleanupDuration
		}
		if runContextErr := ctx.Err(); runContextErr != nil && !errors.Is(errors.Join(allErrors...), runContextErr) {
			allErrors = append(allErrors, fmt.Errorf("run context: %w", runContextErr))
		}
		result.Duration = time.Since(started)
		taskErr := errors.Join(allErrors...)
		taskSpan.End(taskErr)
		entry := r.logger.WithContext(context.WithoutCancel(ctx)).WithFields(logrus.Fields{
			"task_id": task.ID, "harness_status": result.Harness.Status,
			"evaluation_status": result.Evaluation.Status, "cleanup_status": result.Cleanup.Status,
		})
		if taskErr != nil {
			result.Error = taskErr.Error()
			entry = entry.WithError(taskErr)
		}
		entry.Info("task finished")
		return result, taskErr
	}

	var err error
	stageCtx, stage := performance.Begin(ctx, "sandbox.start")
	createStarted := time.Now()
	result.Timing.SandboxCreateStartedAt = &createStarted
	sandbox, err = r.toolSandbox.Start(stageCtx, core.SandboxRequest{
		RunID:       r.runID,
		TaskID:      task.ID,
		Environment: sandboxEnvironment,
	})
	createFinished := time.Now()
	createDuration := createFinished.Sub(createStarted)
	result.Timing.SandboxCreateDuration = &createDuration
	if err != nil {
		stage.End(err)
		allErrors = append(allErrors, fmt.Errorf("start sandbox: %w", err))
		return finish()
	}
	if sandbox == nil {
		err = errors.New("start sandbox: returned a nil sandbox")
		stage.End(err)
		allErrors = append(allErrors, err)
		return finish()
	}
	stage.End(nil)
	result.Timing.SandboxReadyAt = &createFinished
	sandboxActive = true
	stageCtx, stage = performance.Begin(ctx, "sandbox.prepare")
	prepareStarted := time.Now()
	err = r.benchmark.PrepareSandbox(stageCtx, task, sandbox)
	prepareFinished := time.Now()
	prepareDuration := prepareFinished.Sub(prepareStarted)
	result.Timing.SandboxPrepareDuration = &prepareDuration
	stage.End(err)
	if err != nil {
		allErrors = append(allErrors, fmt.Errorf("prepare sandbox: %w", err))
		return finish()
	}
	result.Timing.SandboxPreparedAt = &prepareFinished

	stageCtx, stage = performance.Begin(ctx, "bridge.register")
	bridgeStarted := time.Now()
	result.Timing.BridgePrepareStartedAt = &bridgeStarted
	endpoint, err := r.bridge.Start(stageCtx, sandbox)
	bridgeFinished := time.Now()
	bridgeDuration := bridgeFinished.Sub(bridgeStarted)
	result.Timing.BridgePrepareDuration = &bridgeDuration
	stage.End(err)
	// Start may fail after allocating task-local resources or after its internal
	// rollback fails. Stop is idempotent, so every Start attempt must be followed
	// by a positive revocation confirmation before sandbox cleanup.
	bridgeActive = true
	if err != nil {
		allErrors = append(allErrors, fmt.Errorf("start bridge: %w", err))
		return finish()
	}
	result.Timing.BridgeReadyAt = &bridgeFinished
	result.ToolLogPaths = append([]string(nil), endpoint.LogPaths...)

	request := core.HarnessRequest{
		Connectivity: sandbox.Connectivity(),
		RunID:        r.runID,
		TaskID:       task.ID,
		Endpoint:     endpoint,
		Model:        r.model,
		Timeout:      harnessTimeout,
		CPU:          harnessCPU,
		MemoryMB:     harnessMemory,
		OutputDir:    r.outputDir,
	}
	request.NoSandboxTools = task.NoSandboxTools
	stageCtx, stage = performance.Begin(ctx, "harness.start")
	harnessStart := time.Now()
	err = r.harness.Start(stageCtx, request)
	harnessReady := time.Now()
	harnessStartDuration := harnessReady.Sub(harnessStart)
	result.Timing.HarnessStartDuration = &harnessStartDuration
	stage.End(err)
	// Start may fail after allocating task-local resources. Stop is idempotent,
	// so every Start attempt must be followed by a positive stop confirmation
	// before the evaluator can inspect the sandbox.
	harnessActive = true
	if err != nil {
		result.Harness.Status = failureStatus(err)
		result.Harness.Error = err.Error()
		allErrors = append(allErrors, fmt.Errorf("start harness: %w", err))
	} else {
		result.Timing.HarnessReadyAt = &harnessReady
		stageCtx, stage = performance.Begin(ctx, "agent.run")
		harnessInvoked := time.Now()
		result.Timing.HarnessRunInvokedAt = &harnessInvoked
		harnessResult, runErr := r.harness.Run(stageCtx, task.Instruction)
		harnessReturned := time.Now()
		result.Timing.HarnessRunReturnedAt = &harnessReturned
		stage.End(runErr)
		result.Harness = harnessResult
		copyRuntimeTiming(result.Timing, harnessResult)
		if runErr == nil {
			if result.Harness.Status == "" {
				result.Harness.Status = core.StatusSucceeded
			}
		} else {
			result.Harness.Status = failureStatus(runErr)
			result.Harness.Error = runErr.Error()
			allErrors = append(allErrors, fmt.Errorf("run harness: %w", runErr))
		}
	}

	isolationErrors := make([]error, 0, 2)
	isolationCtx, cancelIsolation := context.WithTimeout(context.WithoutCancel(ctx), r.cleanupTimeout)
	isolationStarted := time.Now()
	stageCtx, stage = performance.Begin(isolationCtx, "harness.stop")
	err = r.harness.Stop(stageCtx)
	stage.End(err)
	if err != nil {
		isolationErrors = append(isolationErrors, fmt.Errorf("confirm harness stopped: %w", err))
	} else {
		harnessActive = false
		result.Isolation.HarnessStopped = true
	}
	stageCtx, stage = performance.Begin(isolationCtx, "bridge.revoke")
	err = r.bridge.Stop(stageCtx)
	stage.End(err)
	if err != nil {
		isolationErrors = append(isolationErrors, fmt.Errorf("confirm bridge revoked: %w", err))
	} else {
		bridgeActive = false
		result.Isolation.BridgeRevoked = true
	}
	cancelIsolation()
	isolationFinished := time.Now()
	isolationDuration := isolationFinished.Sub(isolationStarted)
	result.Timing.StopRevokeDuration = &isolationDuration

	if len(isolationErrors) != 0 {
		isolationErr := errors.Join(isolationErrors...)
		result.Isolation.Status = core.StatusBlockedIsolation
		result.Isolation.Error = isolationErr.Error()
		result.Evaluation.Status = core.StatusBlockedIsolation
		result.Evaluation.Error = "evaluation blocked because harness termination or bridge revocation was not confirmed"
		allErrors = append(allErrors, isolationErrors...)
		return finish()
	}

	result.Isolation.Status = core.StatusConfirmed
	result.Timing.IsolationFinishedAt = &isolationFinished
	stageCtx, stage = performance.Begin(ctx, "benchmark.evaluate")
	evaluationStarted := time.Now()
	taskEvaluation, evaluationErr := r.benchmark.Evaluate(stageCtx, originalTask, sandbox, evaluation)
	evaluationFinished := time.Now()
	evaluationDuration := evaluationFinished.Sub(evaluationStarted)
	result.Timing.VerifierDuration = &evaluationDuration
	stage.End(evaluationErr)
	result.Evaluation = taskEvaluation
	if validArtifactTiming(taskEvaluation.Artifact, started, evaluationFinished) {
		ready := taskEvaluation.Artifact.ObservedAt
		result.Timing.ArtifactReadyAt = &ready
	}
	// Several benchmarks initialize VerifierStatus to failed before they run
	// any tests. An evaluation error alone cannot prove verifier completion.
	if evaluationErr == nil && taskEvaluation.Status != core.StatusBlockedIsolation &&
		(taskEvaluation.VerifierStatus == core.StatusSucceeded || taskEvaluation.VerifierStatus == core.StatusFailed) {
		result.Timing.VerifierFinishedAt = &evaluationFinished
	}
	if evaluationErr == nil {
		if result.Evaluation.Status == "" {
			result.Evaluation.Status = core.StatusSucceeded
		}
	} else {
		result.Evaluation.Status = failureStatus(evaluationErr)
		result.Evaluation.Error = evaluationErr.Error()
		allErrors = append(allErrors, fmt.Errorf("evaluate sandbox: %w", evaluationErr))
	}

	return finish()
}

func copyRuntimeTiming(timing *core.TaskTiming, result core.HarnessResult) {
	if result.RuntimeStartedAt != nil && !result.RuntimeStartedAt.IsZero() {
		observed := *result.RuntimeStartedAt
		timing.AgentStartedAt = &observed
	}
	if result.RuntimeFinishedAt != nil && !result.RuntimeFinishedAt.IsZero() {
		observed := *result.RuntimeFinishedAt
		timing.AgentFinishedAt = &observed
	}
	if result.RuntimeExecutionDuration != nil && *result.RuntimeExecutionDuration >= 0 {
		duration := *result.RuntimeExecutionDuration
		timing.RuntimeExecutionDuration = &duration
	}
}

func validArtifactTiming(artifact *core.ArtifactEvidence, started, finished time.Time) bool {
	if artifact == nil || artifact.ObservedAt.IsZero() || artifact.ObservedAt.Before(started) || artifact.ObservedAt.After(finished) || strings.TrimSpace(artifact.Path) == "" {
		return false
	}
	digest, err := hex.DecodeString(artifact.SHA256)
	return err == nil && len(digest) == 32
}

func cloneRuntimeOverrides(overrides RuntimeOverrides) RuntimeOverrides {
	cloned := RuntimeOverrides{
		HarnessResources:      cloneResourceOverrides(overrides.HarnessResources),
		AgentSandboxResources: cloneResourceOverrides(overrides.AgentSandboxResources),
	}
	if overrides.AgentTimeout != nil {
		value := *overrides.AgentTimeout
		cloned.AgentTimeout = &value
	}
	return cloned
}

func cloneResourceOverrides(resources ResourceOverrides) ResourceOverrides {
	cloned := ResourceOverrides{}
	if resources.CPU != nil {
		value := *resources.CPU
		cloned.CPU = &value
	}
	if resources.MemoryMB != nil {
		value := *resources.MemoryMB
		cloned.MemoryMB = &value
	}
	return cloned
}

func effectiveRuntime(original core.Task, overrides RuntimeOverrides) (core.Environment, *float64, *int, time.Duration) {
	environment := original.Environment
	environment.Env = maps.Clone(original.Environment.Env)
	if overrides.AgentSandboxResources.CPU != nil {
		environment.CPU = *overrides.AgentSandboxResources.CPU
	}
	if overrides.AgentSandboxResources.MemoryMB != nil {
		environment.MemoryMB = *overrides.AgentSandboxResources.MemoryMB
	}
	// A zero harness dimension means unlimited, which the deployment
	// request expresses by omission.
	harnessCPU := cloneFloat(overrides.HarnessResources.CPU)
	if harnessCPU != nil && *harnessCPU == 0 {
		harnessCPU = nil
	}
	harnessMemory := cloneInt(overrides.HarnessResources.MemoryMB)
	if harnessMemory != nil && *harnessMemory == 0 {
		harnessMemory = nil
	}
	timeout := original.Timeout
	if overrides.AgentTimeout != nil {
		timeout = *overrides.AgentTimeout
	}
	return environment, harnessCPU, harnessMemory, timeout
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func newTaskResult(taskID string) core.TaskResult {
	return core.TaskResult{
		TaskID:     taskID,
		Harness:    core.HarnessResult{Status: core.StatusNotStarted},
		Isolation:  core.IsolationResult{Status: core.StatusNotStarted},
		Evaluation: core.Evaluation{Status: core.StatusNotRun},
		Observer:   core.ObserverResult{Status: core.StatusNotEnabled},
		Cleanup:    core.CleanupResult{Status: core.StatusNotNeeded},
	}
}

func failureStatus(err error) string {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return core.StatusCanceled
	}
	return core.StatusFailed
}

func summarize(tasks []core.TaskResult) core.RunSummary {
	var summary core.RunSummary
	summary.Tasks = len(tasks)
	for _, task := range tasks {
		if task.Harness.Status == core.StatusSucceeded {
			summary.HarnessSucceeded++
		} else if task.Harness.Status == core.StatusFailed || task.Harness.Status == core.StatusCanceled {
			summary.HarnessFailed++
		}
		switch task.Evaluation.Status {
		case core.StatusSucceeded:
			summary.EvaluationsRun++
			summary.EvaluationsSucceeded++
		case core.StatusFailed, core.StatusCanceled:
			summary.EvaluationsRun++
			summary.EvaluationsFailed++
		case core.StatusBlockedIsolation:
			summary.EvaluationsBlocked++
		}
		if task.Cleanup.Status == core.StatusFailed {
			summary.CleanupFailed++
		}
	}
	return summary
}

// evaluationSandboxes starts fresh sandboxes for one task evaluation. The
// Runner stops them, newest first, before stopping the task sandbox.
type evaluationSandboxes struct {
	toolSandbox ToolSandbox
	runID       string
	taskID      string
	mu          sync.Mutex
	started     []Sandbox
}

func (e *evaluationSandboxes) Start(ctx context.Context, environment core.Environment) (Sandbox, error) {
	ctx = performance.WithIdentity(ctx, performance.Identity{Purpose: core.SandboxPurposeEvaluation})
	ctx, span := performance.Begin(ctx, "sandbox.start")
	sandbox, err := e.toolSandbox.Start(ctx, core.SandboxRequest{
		RunID: e.runID, TaskID: e.taskID, Purpose: core.SandboxPurposeEvaluation, Environment: environment,
	})
	if err != nil {
		span.End(err)
		return nil, err
	}
	if sandbox == nil {
		err = errors.New("start evaluation sandbox: returned a nil sandbox")
		span.End(err)
		return nil, err
	}
	span.End(nil)
	e.mu.Lock()
	e.started = append(e.started, sandbox)
	e.mu.Unlock()
	return sandbox, nil
}

func (e *evaluationSandboxes) active() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.started) != 0
}

func (e *evaluationSandboxes) stop(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	var errs []error
	var remaining []Sandbox
	ctx = performance.WithIdentity(ctx, performance.Identity{Purpose: core.SandboxPurposeEvaluation})
	for index := len(e.started) - 1; index >= 0; index-- {
		stageCtx, stage := performance.Begin(ctx, "sandbox.stop")
		err := e.toolSandbox.Stop(stageCtx, e.started[index])
		stage.End(err)
		if err != nil {
			errs = append(errs, err)
			remaining = append(remaining, e.started[index])
		}
	}
	slices.Reverse(remaining)
	e.started = remaining
	return errors.Join(errs...)
}

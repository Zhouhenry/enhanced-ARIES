package core

import "time"

// ArtifactEvidence is a captured, stable candidate, not an agent success claim.
// ObservedAt is measured by the runner process after the copy is complete.
// A caller must leave this absent when only a remote wall clock is available.
type ArtifactEvidence struct {
	ObservedAt time.Time `json:"observed_at"`
	Path       string    `json:"path"`
	SHA256     string    `json:"sha256"`
}

// TaskTiming contains locally observed boundaries. Missing timestamps remain
// absent; a failed operation does not get a fabricated readiness timestamp.
// Durations are measured with the local monotonic clock, not reconstructed
// from remote wall clocks or percentile differences.
type TaskTiming struct {
	SandboxCreateStartedAt *time.Time     `json:"sandbox_create_started_at,omitempty"`
	SandboxReadyAt         *time.Time     `json:"sandbox_ready_at,omitempty"`
	SandboxPreparedAt      *time.Time     `json:"sandbox_prepared_at,omitempty"`
	BridgePrepareStartedAt *time.Time     `json:"bridge_prepare_started_at,omitempty"`
	BridgeReadyAt          *time.Time     `json:"bridge_ready_at,omitempty"`
	HarnessReadyAt         *time.Time     `json:"harness_ready_at,omitempty"`
	HarnessRunInvokedAt    *time.Time     `json:"harness_run_invoked_at,omitempty"`
	HarnessRunReturnedAt   *time.Time     `json:"harness_run_returned_at,omitempty"`
	AgentStartedAt         *time.Time     `json:"agent_started_at,omitempty"`
	AgentFinishedAt        *time.Time     `json:"agent_finished_at,omitempty"`
	IsolationFinishedAt    *time.Time     `json:"isolation_finished_at,omitempty"`
	ArtifactReadyAt        *time.Time     `json:"artifact_ready_at,omitempty"`
	VerifierFinishedAt     *time.Time     `json:"verifier_finished_at,omitempty"`
	CleanupFinishedAt      *time.Time     `json:"cleanup_finished_at,omitempty"`
	SandboxCreateDuration  *time.Duration `json:"sandbox_create_duration,omitempty"`
	SandboxPrepareDuration *time.Duration `json:"sandbox_prepare_duration,omitempty"`
	// Bridge setup is separate from the historical workspace and harness stages.
	BridgePrepareDuration    *time.Duration `json:"bridge_prepare_duration,omitempty"`
	HarnessStartDuration     *time.Duration `json:"harness_start_duration,omitempty"`
	RuntimeExecutionDuration *time.Duration `json:"runtime_execution_duration,omitempty"`
	StopRevokeDuration       *time.Duration `json:"stop_revoke_duration,omitempty"`
	VerifierDuration         *time.Duration `json:"verifier_duration,omitempty"`
	CleanupDuration          *time.Duration `json:"cleanup_duration,omitempty"`
}

// AttemptResult is one planned occurrence. Stage counts overlap intentionally;
// Status is the terminal accounting outcome, not the independent task score.
type AttemptResult struct {
	ModelSessionID     string         `json:"model_session_id,omitempty"`
	ModelSessionStatus string         `json:"model_session_status,omitempty"`
	AttemptID          string         `json:"attempt_id"`
	OccurrenceID       string         `json:"occurrence_id"`
	LogicalTaskID      string         `json:"logical_task_id"`
	ArrivalID          string         `json:"arrival_id"`
	AgentSessionID     string         `json:"agent_session_id,omitempty"`
	ScheduledAt        time.Time      `json:"scheduled_at"`
	AdmittedAt         *time.Time     `json:"admitted_at,omitempty"`
	StartedAt          *time.Time     `json:"started_at,omitempty"`
	FinishedAt         *time.Time     `json:"finished_at,omitempty"`
	QueueDuration      *time.Duration `json:"queue_duration,omitempty"`
	DispatchDelay      *time.Duration `json:"dispatch_delay,omitempty"`
	Duration           *time.Duration `json:"duration,omitempty"`
	Timing             *TaskTiming    `json:"timing,omitempty"`
	Status             string         `json:"status"`
	Reason             string         `json:"reason,omitempty"`
	Error              string         `json:"error,omitempty"`
	Executed           bool           `json:"executed"`
	Completed          bool           `json:"completed"`
	Validated          bool           `json:"validated"`
	ArtifactProven     bool           `json:"artifact_proven"`
	ArtifactLatency    *time.Duration `json:"artifact_e2e_latency,omitempty"`
	VerifiedLatency    *time.Duration `json:"verified_e2e_latency,omitempty"`
	EvaluationStatus   string         `json:"evaluation_status,omitempty"`
	IsolationStatus    string         `json:"isolation_status,omitempty"`
	CleanupStatus      string         `json:"cleanup_status,omitempty"`
}

// ModelSessionResult refers to the explicitly bound proxy session, not the
// Agent's native Gateway session. Details live in the private event/tape files.
type ModelSessionResult struct {
	AttemptID    string `json:"attempt_id"`
	SessionID    string `json:"session_id"`
	Mode         string `json:"mode"`
	ReplayPolicy string `json:"replay_policy,omitempty"`
	Timing       string `json:"timing"`
	Status       string `json:"status"`
	ArtifactDir  string `json:"artifact_dir"`
	Error        string `json:"error,omitempty"`
}

// AttemptSummary's rate denominator includes admission and drain. It never
// divides post-drain completions by the shorter load-offering window.
type AttemptSummary struct {
	Planned             int            `json:"planned"`
	Admitted            int            `json:"admitted"`
	Started             int            `json:"started"`
	Executed            int            `json:"executed"`
	Completed           int            `json:"completed"`
	ValidatedSucceeded  int            `json:"verified_succeeded"`
	Succeeded           int            `json:"succeeded"`
	Failed              int            `json:"failed"`
	Blocked             int            `json:"blocked"`
	Canceled            int            `json:"canceled"`
	TimedOut            int            `json:"timed_out"`
	Unknown             int            `json:"unknown"`
	NotStarted          int            `json:"not_started"`
	LoadgenDropped      int            `json:"loadgen_dropped"`
	CleanupFailed       int            `json:"cleanup_failed"`
	WindowStartedAt     time.Time      `json:"window_started_at"`
	AdmissionFinishedAt time.Time      `json:"admission_finished_at"`
	WindowFinishedAt    time.Time      `json:"window_finished_at"`
	ObservationDuration time.Duration  `json:"observation_duration"`
	DrainDuration       time.Duration  `json:"drain_duration"`
	AttemptsPerSecond   *float64       `json:"attempts_per_second,omitempty"`
	ExecutedPerSecond   *float64       `json:"executed_throughput,omitempty"`
	ExecutionThroughput *float64       `json:"execution_throughput,omitempty"`
	ValidatedThroughput *float64       `json:"validated_success_throughput,omitempty"`
	GoodputDeadline     *time.Duration `json:"goodput_deadline,omitempty"`
	GoodputCount        *int           `json:"goodput_count,omitempty"`
	Goodput             *float64       `json:"goodput,omitempty"`
}

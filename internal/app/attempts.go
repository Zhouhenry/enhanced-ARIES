package app

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
)

// The scheduler owns this ledger even when component construction returns no
// TaskResult. Old task arrays and summaries retain their original meaning.
func finishAttempt(attempt *core.AttemptResult, result core.RunResult, runErr error) {
	if len(result.ModelSessions) == 1 && result.ModelSessions[0].AttemptID == attempt.AttemptID {
		attempt.ModelSessionID = result.ModelSessions[0].SessionID
		attempt.ModelSessionStatus = result.ModelSessions[0].Status
	}
	attempt.Status = "unknown"
	if runErr != nil {
		attempt.Error = runErr.Error()
		attempt.Status, attempt.Reason = core.StatusFailed, "occurrence_failed"
		if errors.Is(runErr, context.DeadlineExceeded) {
			attempt.Status, attempt.Reason = "timeout", "context_deadline_exceeded"
		} else if errors.Is(runErr, context.Canceled) {
			attempt.Status, attempt.Reason = core.StatusCanceled, "context_canceled"
		}
	}
	if len(result.Tasks) != 1 || result.Tasks[0].TaskID != attempt.OccurrenceID {
		if len(result.Tasks) == 0 && runErr != nil {
			if attempt.Status == core.StatusFailed {
				attempt.Reason = "failed_before_task_result"
			}
		} else {
			attempt.Status, attempt.Reason = "unknown", "missing_or_ambiguous_task_result"
		}
		return
	}
	task := result.Tasks[0]
	attempt.AgentSessionID = task.Harness.SessionID
	attempt.Timing = task.Timing
	attempt.EvaluationStatus = task.Evaluation.Status
	attempt.IsolationStatus = task.Isolation.Status
	attempt.CleanupStatus = task.Cleanup.Status
	attempt.Executed = task.Timing != nil && task.Timing.HarnessRunInvokedAt != nil
	attempt.Completed = attempt.Executed && task.Harness.Status == core.StatusSucceeded
	attempt.Validated = task.Evaluation.Status == core.StatusSucceeded &&
		task.Evaluation.VerifierStatus == core.StatusSucceeded &&
		task.Isolation.Status == core.StatusConfirmed && task.Isolation.HarnessStopped && task.Isolation.BridgeRevoked
	if task.Timing != nil && withinAttempt(attempt, task.Timing.VerifierFinishedAt) {
		latency := task.Timing.VerifierFinishedAt.Sub(attempt.ScheduledAt)
		attempt.VerifiedLatency = &latency
	}
	if evidence := task.Evaluation.Artifact; evidence != nil && strings.TrimSpace(evidence.Path) != "" &&
		task.Timing != nil && withinAttempt(attempt, task.Timing.ArtifactReadyAt) &&
		task.Timing.ArtifactReadyAt.Equal(evidence.ObservedAt) {
		digest, err := hex.DecodeString(evidence.SHA256)
		if err == nil && len(digest) == 32 {
			attempt.ArtifactProven = true
			latency := evidence.ObservedAt.Sub(attempt.ScheduledAt)
			attempt.ArtifactLatency = &latency
		}
	}
	if runErr != nil {
		if attempt.Status == core.StatusFailed {
			switch {
			case task.Isolation.Status == core.StatusBlockedIsolation || task.Evaluation.Status == core.StatusBlockedIsolation:
				attempt.Status, attempt.Reason = "blocked", "isolation_unconfirmed"
			case task.Cleanup.Status == core.StatusFailed:
				attempt.Reason = "cleanup_failed"
			}
		}
		return
	}
	switch {
	case task.Error != "":
		attempt.Status, attempt.Reason, attempt.Error = core.StatusFailed, "task_failed", task.Error
	case task.Isolation.Status == core.StatusBlockedIsolation || task.Evaluation.Status == core.StatusBlockedIsolation:
		attempt.Status, attempt.Reason = "blocked", "isolation_unconfirmed"
	case task.Cleanup.Status == core.StatusFailed:
		attempt.Status, attempt.Reason = core.StatusFailed, "cleanup_failed"
	case task.Harness.Status == core.StatusFailed:
		attempt.Status, attempt.Reason = core.StatusFailed, "agent_failed"
	case task.Harness.Status == core.StatusCanceled:
		attempt.Status, attempt.Reason = core.StatusCanceled, "agent_canceled"
	case task.Evaluation.Status == core.StatusFailed:
		attempt.Status, attempt.Reason = core.StatusFailed, "verification_failed"
	case attempt.Completed && (attempt.Validated || task.Evaluation.Status == core.StatusNotEnabled) &&
		(task.Cleanup.Status == core.StatusSucceeded || task.Cleanup.Status == core.StatusNotNeeded):
		attempt.Status = core.StatusSucceeded
	default:
		attempt.Reason = "incomplete_stage_evidence"
	}
}

func withinAttempt(attempt *core.AttemptResult, at *time.Time) bool {
	return at != nil && !at.IsZero() && attempt.StartedAt != nil && attempt.FinishedAt != nil &&
		!at.Before(*attempt.StartedAt) && !at.After(*attempt.FinishedAt) && !at.Before(attempt.ScheduledAt)
}

func summarizeAttempts(attempts []core.AttemptResult, started, admissionFinished, finished time.Time, deadline time.Duration) *core.AttemptSummary {
	summary := &core.AttemptSummary{
		Planned: len(attempts), WindowStartedAt: started, AdmissionFinishedAt: admissionFinished,
		WindowFinishedAt: finished, ObservationDuration: finished.Sub(started), DrainDuration: finished.Sub(admissionFinished),
	}
	goodputCount := 0
	for _, attempt := range attempts {
		if attempt.AdmittedAt != nil {
			summary.Admitted++
		}
		if attempt.StartedAt != nil {
			summary.Started++
		}
		if attempt.Executed {
			summary.Executed++
		}
		if attempt.Completed {
			summary.Completed++
		}
		if attempt.Validated {
			summary.ValidatedSucceeded++
		}
		if attempt.CleanupStatus == core.StatusFailed {
			summary.CleanupFailed++
		}
		switch attempt.Status {
		case core.StatusSucceeded:
			summary.Succeeded++
		case core.StatusFailed:
			summary.Failed++
		case core.StatusCanceled:
			summary.Canceled++
		case "timeout":
			summary.TimedOut++
		case "blocked":
			summary.Blocked++
		case core.StatusNotStarted:
			summary.NotStarted++
		case "loadgen_dropped":
			summary.LoadgenDropped++
		default:
			summary.Unknown++
		}
		if deadline > 0 && attempt.Validated && attempt.ArtifactProven &&
			attempt.ArtifactLatency != nil && *attempt.ArtifactLatency >= 0 && *attempt.ArtifactLatency <= deadline {
			goodputCount++
		}
	}
	if deadline > 0 {
		summary.GoodputDeadline, summary.GoodputCount = &deadline, &goodputCount
	}
	if seconds := summary.ObservationDuration.Seconds(); seconds > 0 {
		rate := func(count int) *float64 { value := float64(count) / seconds; return &value }
		summary.AttemptsPerSecond = rate(summary.Started)
		summary.ExecutedPerSecond = rate(summary.Executed)
		summary.ExecutionThroughput = rate(summary.Completed)
		summary.ValidatedThroughput = rate(summary.ValidatedSucceeded)
		if deadline > 0 {
			summary.Goodput = rate(goodputCount)
		}
	}
	return summary
}

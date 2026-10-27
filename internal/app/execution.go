package app

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/monitor"
	"github.com/hyscale-lab/aries/pkg/performance"
	"github.com/hyscale-lab/aries/pkg/runner"
)

const observerStopTimeout = 15 * time.Second

type experiment struct {
	runner   *runner.Runner
	recorder *monitor.Recorder
	close    func() error
}

func (experiment *experiment) Run(ctx context.Context) (core.RunResult, error) {
	result, err := runObserved(ctx, experiment.runner.Run, experiment.recorder.Start, experiment.recorder.Stop, observerStopTimeout)
	if experiment.close != nil {
		err = errors.Join(err, experiment.close())
	}
	return result, err
}

type taskOccurrence struct {
	logicalID   string
	executionID string
	arrivalID   string
}

func nextTaskOccurrence(logicalID string, index *uint64) (taskOccurrence, error) {
	if *index == math.MaxUint64 {
		return taskOccurrence{}, errors.New("task occurrence index overflow")
	}
	*index++
	return taskOccurrence{
		logicalID: logicalID, executionID: fmt.Sprintf("%s-%03d", logicalID, *index),
		arrivalID: fmt.Sprintf("arrival-%06d", *index),
	}, nil
}

type occurrenceRunner func(context.Context, taskOccurrence) (core.RunResult, error)

// arrival is one scheduled task start, relative to the run start.
type arrival struct {
	logicalID string
	at        time.Duration
}

type occurrenceSlot struct {
	occurrence taskOccurrence
	attempt    *core.AttemptResult
	result     core.RunResult
	err        error
}

type plannedOccurrence struct {
	occurrence taskOccurrence
	slot       *occurrenceSlot
	at         time.Duration
	ctx        context.Context
}

func runProfile(ctx context.Context, name, runID string, taskIDs []string, concurrency int, loopDuration time.Duration, arrivals []arrival, run occurrenceRunner, deadlines ...time.Duration) (core.RunResult, error) {
	started := time.Now()
	result := core.RunResult{Name: name, RunID: runID}
	if concurrency <= 0 || len(taskIDs) == 0 {
		return result, errors.New("execution requires positive concurrency and at least one task")
	}
	if len(arrivals) > 0 && loopDuration > 0 {
		return result, errors.New("execution cannot combine an arrival schedule with a loop duration")
	}
	if len(arrivals) > 0 && len(arrivals) != len(taskIDs) {
		return result, fmt.Errorf("arrival schedule has %d entries for %d tasks", len(arrivals), len(taskIDs))
	}
	var goodputDeadline time.Duration
	if len(deadlines) > 1 || len(deadlines) == 1 && deadlines[0] < 0 {
		return result, errors.New("execution accepts at most one nonnegative Goodput deadline")
	}
	if len(deadlines) == 1 {
		goodputDeadline = deadlines[0]
	}
	var deadline <-chan time.Time
	var deadlineAt time.Time
	var timer *time.Timer
	if loopDuration > 0 {
		deadlineAt = started.Add(loopDuration)
		timer = time.NewTimer(loopDuration)
		defer timer.Stop()
		deadline = timer.C
	}
	capacity := make(chan struct{}, concurrency)
	var slots []*occurrenceSlot
	var wg sync.WaitGroup
	var index uint64
	anchorOffset := performance.AnchorOffset(ctx, started)
	recordArrival := func(next plannedOccurrence, eventType, status, reason string, duration *int64) {
		plannedOffset := int64(next.at)
		performance.Record(next.ctx, performance.Event{
			EventType: eventType, Operation: "arrival", Status: status, Reason: reason,
			PlannedOffsetNS: &plannedOffset, ScheduleAnchorOffsetNS: &anchorOffset, DurationNS: duration,
		})
	}
	plan := func(logicalID string, at time.Duration) (plannedOccurrence, error) {
		occurrence, err := nextTaskOccurrence(logicalID, &index)
		if err != nil {
			return plannedOccurrence{}, err
		}
		next := plannedOccurrence{
			occurrence: occurrence, at: at,
			ctx: performance.WithIdentity(ctx, performance.Identity{
				LogicalTaskID: logicalID, OccurrenceID: occurrence.executionID,
				ArrivalID: occurrence.arrivalID, Purpose: "task",
			}),
		}
		next.slot = &occurrenceSlot{occurrence: occurrence, attempt: &core.AttemptResult{
			AttemptID: runID + "/" + occurrence.executionID, OccurrenceID: occurrence.executionID,
			LogicalTaskID: logicalID, ArrivalID: occurrence.arrivalID,
			ScheduledAt: started.Add(at), Status: core.StatusNotStarted,
		}}
		slots = append(slots, next.slot)
		recordArrival(next, "arrival_planned", "succeeded", "", nil)
		return next, nil
	}
	stopReason := func() string {
		switch {
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			return "context_deadline_exceeded"
		case ctx.Err() != nil:
			return "context_canceled"
		default:
			return "loop_deadline"
		}
	}
	notStarted := func(next plannedOccurrence) {
		finished := time.Now()
		next.slot.attempt.FinishedAt = &finished
		next.slot.attempt.Reason = stopReason()
		recordArrival(next, "arrival_not_started", core.StatusNotStarted, next.slot.attempt.Reason, nil)
	}
	admit := func(next plannedOccurrence) bool {
		if !deadlineAt.IsZero() && !time.Now().Before(deadlineAt) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-deadline:
			return false
		case capacity <- struct{}{}:
		}
		if ctx.Err() != nil || !deadlineAt.IsZero() && !time.Now().Before(deadlineAt) {
			<-capacity
			return false
		}
		admitted := time.Now()
		queueDuration := admitted.Sub(next.slot.attempt.ScheduledAt)
		next.slot.attempt.AdmittedAt = &admitted
		next.slot.attempt.QueueDuration = &queueDuration
		queued := queueDuration.Nanoseconds()
		recordArrival(next, "arrival_admitted", "succeeded", "", &queued)
		slot := next.slot
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-capacity }()
			dispatched := time.Now()
			slot.attempt.StartedAt = &dispatched
			dispatchDelay := dispatched.Sub(admitted)
			slot.attempt.DispatchDelay = &dispatchDelay
			recordArrival(next, "arrival_dispatched", "succeeded", "", nil)
			slot.result, slot.err = run(next.ctx, next.occurrence)
			finished := time.Now()
			elapsed := finished.Sub(dispatched)
			slot.attempt.FinishedAt, slot.attempt.Duration = &finished, &elapsed
			finishAttempt(slot.attempt, slot.result, slot.err)
			duration := elapsed.Nanoseconds()
			status, reason := "succeeded", ""
			switch {
			case errors.Is(slot.err, context.DeadlineExceeded):
				status, reason = "timeout", "context_deadline_exceeded"
			case errors.Is(slot.err, context.Canceled):
				status, reason = "canceled", "context_canceled"
			case slot.err != nil:
				status, reason = "failed", "occurrence_failed"
			}
			recordArrival(next, "arrival_finished", status, reason, &duration)
		}()
		return true
	}
	if loopDuration == 0 {
		// Preallocate the finite plan so cancellation also accounts for tasks
		// whose planned arrival has not happened yet.
		planned := make([]plannedOccurrence, 0, len(taskIDs))
		for i, logicalID := range taskIDs {
			var at time.Duration
			if len(arrivals) > 0 {
				logicalID, at = arrivals[i].logicalID, arrivals[i].at
			}
			next, err := plan(logicalID, at)
			if err != nil {
				return result, err
			}
			planned = append(planned, next)
		}
		// Arrival replay retains blocking admission. A full pool delays actual
		// dispatch, so the schedule is not a strict open-loop load generator.
		for i, next := range planned {
			wait := time.Until(started.Add(next.at))
			if wait > 0 {
				timer := time.NewTimer(wait)
				select {
				case <-ctx.Done():
					timer.Stop()
				case <-timer.C:
				}
			}
			if ctx.Err() != nil || !admit(next) {
				for _, unstarted := range planned[i:] {
					notStarted(unstarted)
				}
				break
			}
		}
	} else {
	admissions:
		for {
			for _, logicalID := range taskIDs {
				if ctx.Err() != nil || !time.Now().Before(deadlineAt) {
					break admissions
				}
				next, err := plan(logicalID, time.Since(started))
				if err != nil {
					slots = append(slots, &occurrenceSlot{err: err})
					break admissions
				}
				if !admit(next) {
					notStarted(next)
					break admissions
				}
			}
		}
	}
	admissionFinished := time.Now()
	wg.Wait()
	finished := time.Now()
	var joined []error
	for _, slot := range slots {
		if slot.attempt != nil {
			result.Attempts = append(result.Attempts, *slot.attempt)
		}
		result.Tasks = append(result.Tasks, slot.result.Tasks...)
		result.ModelSessions = append(result.ModelSessions, slot.result.ModelSessions...)
		addSummary(&result.Summary, slot.result.Summary)
		if slot.err != nil {
			if slot.occurrence.executionID != "" {
				joined = append(joined, fmt.Errorf("task occurrence %s: %w", slot.occurrence.executionID, slot.err))
			} else {
				joined = append(joined, slot.err)
			}
		}
	}
	if ctx.Err() != nil {
		joined = append(joined, ctx.Err())
	}
	result.AttemptSummary = summarizeAttempts(result.Attempts, started, admissionFinished, finished, goodputDeadline)
	result.Duration = time.Since(started)
	return result, errors.Join(joined...)
}

func addSummary(total *core.RunSummary, next core.RunSummary) {
	total.Tasks += next.Tasks
	total.HarnessSucceeded += next.HarnessSucceeded
	total.HarnessFailed += next.HarnessFailed
	total.EvaluationsRun += next.EvaluationsRun
	total.EvaluationsSucceeded += next.EvaluationsSucceeded
	total.EvaluationsFailed += next.EvaluationsFailed
	total.EvaluationsBlocked += next.EvaluationsBlocked
	total.CleanupFailed += next.CleanupFailed
}

func runObserved(
	ctx context.Context,
	execute func(context.Context) (core.RunResult, error),
	startObserver func(context.Context) error,
	stopObserver func(context.Context) (map[string]core.ObserverResult, error),
	stopTimeout time.Duration,
) (core.RunResult, error) {
	startErr := startObserver(ctx)
	result, runErr := execute(ctx)

	var reports map[string]core.ObserverResult
	var stopErr error
	if startErr == nil {
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), stopTimeout)
		reports, stopErr = stopObserver(stopCtx)
		cancel()
	}

	var reportErrors []error
	for index := range result.Tasks {
		task := &result.Tasks[index]
		switch {
		case startErr != nil:
			task.Observer = failedObserverResult(fmt.Sprintf("start observer: %v", startErr))
		case reports[task.TaskID].Status != "":
			task.Observer = reports[task.TaskID]
			task.Observer.LogPaths = append([]string(nil), task.Observer.LogPaths...)
			if stopErr != nil {
				task.Observer.Status = core.StatusFailed
				stopMessage := fmt.Errorf("stop observer: %w", stopErr)
				if task.Observer.Error != "" {
					stopMessage = errors.Join(errors.New(task.Observer.Error), stopMessage)
				}
				task.Observer.Error = stopMessage.Error()
			}
		default:
			missingErr := fmt.Errorf("observer report missing for task %q", task.TaskID)
			message := error(missingErr)
			if stopErr != nil {
				message = errors.Join(fmt.Errorf("stop observer: %w", stopErr), missingErr)
			}
			task.Observer = failedObserverResult(message.Error())
			reportErrors = append(reportErrors, missingErr)
		}
	}

	if startErr != nil {
		startErr = fmt.Errorf("start observer: %w", startErr)
	}
	if stopErr != nil {
		stopErr = fmt.Errorf("stop observer: %w", stopErr)
	}
	return result, errors.Join(runErr, startErr, stopErr, errors.Join(reportErrors...))
}

func failedObserverResult(message string) core.ObserverResult {
	return core.ObserverResult{Status: core.StatusFailed, Error: message}
}

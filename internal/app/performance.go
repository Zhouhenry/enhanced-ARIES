package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/performance"
)

func runWithPerformance(ctx context.Context, outputRoot, runID string, execute func(context.Context) (core.RunResult, error)) (core.RunResult, error) {
	level, levelErr := performance.ObservationLevelFromEnv()
	if levelErr != nil {
		return core.RunResult{RunID: runID, Performance: &core.PerformanceResult{Status: core.StatusFailed, ObservationLevel: string(level), Error: levelErr.Error()}}, levelErr
	}
	ctx = performance.WithObservationLevel(ctx, level)
	if level == performance.ObservationBaseline {
		// Baseline keeps the Runner, strict replay, verifier, stop/revoke and
		// cleanup paths intact, while omitting optional event/metric writers.
		result, runErr := execute(ctx)
		result.Performance = &core.PerformanceResult{Status: core.StatusNotRun, ObservationLevel: string(level), ArtifactDir: filepath.Join(outputRoot, "performance")}
		return result, runErr
	}
	report := &core.PerformanceResult{Status: core.StatusFailed, ArtifactDir: filepath.Join(outputRoot, "performance"), ObservationLevel: string(level)}
	recorder, err := performance.Open(outputRoot, runID)
	if err != nil {
		report.Error = fmt.Sprintf("open performance records: %v", err)
		return core.RunResult{RunID: runID, Performance: report}, errors.New(report.Error)
	}
	// Finalize even during a panic; normal completion attaches the outcome before
	// executeAndRecord persists the run result. Close is idempotent.
	defer recorder.Close()
	result, runErr := execute(performance.WithRecorder(ctx, recorder))
	if closeErr := recorder.Close(); closeErr != nil {
		report.Error = fmt.Sprintf("finalize performance records: %v", closeErr)
		runErr = errors.Join(runErr, errors.New(report.Error))
	} else {
		report.Status = core.StatusSucceeded
	}
	result.Performance = report
	return result, runErr
}

package performance

import (
	"context"
	"errors"
	"os"
)

// ObservationLevel controls optional measurement work for paired perturbation
// experiments. It never changes the benchmark, model protocol, tool command,
// verifier or cleanup semantics.
type ObservationLevel string

const (
	ObservationBaseline ObservationLevel = "baseline"
	ObservationL0       ObservationLevel = "l0"
	ObservationL1       ObservationLevel = "l1"
	ObservationL2       ObservationLevel = "l2"
)

const observationLevelEnv = "ARIES_OBSERVATION_LEVEL"

type observationLevelKey struct{}

func ParseObservationLevel(value string) (ObservationLevel, error) {
	if value == "" {
		return ObservationL1, nil
	}
	level := ObservationLevel(value)
	switch level {
	case ObservationBaseline, ObservationL0, ObservationL1, ObservationL2:
		return level, nil
	default:
		return "", errors.New("observation level must be baseline, l0, l1 or l2")
	}
}

// ObservationLevelFromEnv defaults to L1 so existing runs retain their
// current observation behavior. Invalid values fail closed rather than
// silently becoming a different comparison condition.
func ObservationLevelFromEnv() (ObservationLevel, error) {
	return ParseObservationLevel(os.Getenv(observationLevelEnv))
}

func WithObservationLevel(ctx context.Context, level ObservationLevel) context.Context {
	return context.WithValue(ctx, observationLevelKey{}, level)
}

func ObservationLevelFromContext(ctx context.Context) ObservationLevel {
	if level, ok := ctx.Value(observationLevelKey{}).(ObservationLevel); ok && level != "" {
		return level
	}
	return ObservationL1
}

func ObservesControl(level ObservationLevel) bool {
	return level != ObservationBaseline
}

func ObservesResources(level ObservationLevel) bool {
	return level == ObservationL1 || level == ObservationL2
}

func ObservesNative(level ObservationLevel) bool {
	return level != ObservationBaseline
}

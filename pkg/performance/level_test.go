package performance

import (
	"context"
	"testing"
)

func TestObservationLevelsAndContext(t *testing.T) {
	for _, value := range []string{"baseline", "l0", "l1", "l2"} {
		level, err := ParseObservationLevel(value)
		if err != nil || string(level) != value {
			t.Fatalf("parse %q = %q, %v", value, level, err)
		}
	}
	if _, err := ParseObservationLevel("invalid"); err == nil {
		t.Fatal("invalid observation level accepted")
	}
	if got := ObservationLevelFromContext(context.Background()); got != ObservationL1 {
		t.Fatalf("default level = %q", got)
	}
	ctx := WithObservationLevel(context.Background(), ObservationBaseline)
	if got := ObservationLevelFromContext(ctx); got != ObservationBaseline || ObservesControl(got) || ObservesResources(got) {
		t.Fatalf("baseline capabilities = %q/%v/%v", got, ObservesControl(got), ObservesResources(got))
	}
	if !ObservesControl(ObservationL0) || ObservesResources(ObservationL0) || !ObservesResources(ObservationL1) {
		t.Fatal("observation capability matrix changed")
	}
}

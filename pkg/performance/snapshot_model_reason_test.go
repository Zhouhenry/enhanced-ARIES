package performance

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestModelFailureReasonRejectsUnknownTextInProjections(t *testing.T) {
	root, directory := traceFixture(t)
	events, index := boundaryFixture(t, root)
	events[7]["status"], events[7]["reason"] = "failed", "PRIVATE_UNKNOWN_REASON"
	writeBoundaryFixture(t, root, events, index)
	snapshot, err := BuildSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	trace, err := ProjectTrace(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	otlp, err := ProjectOTLP(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for name, projection := range map[string]any{"snapshot": snapshot, "perfetto": trace, "otlp": otlp.Request} {
		raw, err := json.Marshal(projection)
		if err != nil || !strings.Contains(string(raw), "unrecognized_reason") || strings.Contains(string(raw), "PRIVATE_UNKNOWN_REASON") {
			t.Fatalf("%s accepted arbitrary failure text", name)
		}
	}
}

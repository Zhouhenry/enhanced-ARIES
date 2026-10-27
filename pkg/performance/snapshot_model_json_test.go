package performance

import (
	"strings"
	"testing"
)

func TestSnapshotJSONResponseDoesNotInventSSETiming(t *testing.T) {
	for _, bad := range []bool{false, true} {
		root, directory := traceFixture(t)
		events, index := boundaryFixture(t, root)
		for _, e := range events {
			e["model_branch"] = "main/call_1"
			if e["event"] == "frame_flushed" {
				e["event"] = "json_flushed"
				if !bad {
					delete(e, "frame_index")
				}
			}
		}
		writeBoundaryFixture(t, root, events, index)
		snapshot, err := BuildSnapshot(directory)
		if (err != nil) != bad {
			t.Fatalf("unexpected JSON snapshot status: %v", err)
		}
		if bad {
			continue
		}
		found := false
		for _, point := range snapshot.Events {
			if point.Name == "json_flushed" {
				found = true
				if point.Attributes["model_branch"] != "main/call_1" {
					t.Fatal("branch identity missing")
				}
			}
		}
		if !found {
			t.Fatal("JSON delivery boundary missing")
		}
		for _, interval := range snapshot.Intervals {
			if strings.Contains(interval.Name, "first_sse") {
				t.Fatal("JSON invented SSE first frame")
			}
		}
		if _, err := ProjectTrace(snapshot); err != nil {
			t.Fatal(err)
		}
	}
}

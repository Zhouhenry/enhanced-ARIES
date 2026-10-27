package performance

import "testing"

func TestSnapshotModelAdmissionClosePreservesRealBoundary(t *testing.T) {
	for _, mutation := range []string{"none", "missing_close", "admitted_after_close", "rejected_before_close", "negative_close"} {
		t.Run(mutation, func(t *testing.T) {
			root, directory := traceFixture(t)
			events, index := boundaryFixture(t, root)
			index["admission_closed_offset_ns"] = 23
			// Request a was admitted before closing and may finish afterwards;
			// request b arrives during teardown and has no admitted/model turn.
			events[1]["monotonic_offset_ns"] = 31
			events[2]["monotonic_offset_ns"], events[2]["duration_ns"] = 32, 1
			events[2]["reason"] = "ADMISSION_CLOSED"
			delete(events[1], "turn")
			delete(events[2], "turn")
			switch mutation {
			case "missing_close":
				delete(index, "admission_closed_offset_ns")
			case "admitted_after_close":
				index["admission_closed_offset_ns"] = 20
			case "rejected_before_close":
				index["admission_closed_offset_ns"] = 33
			case "negative_close":
				index["admission_closed_offset_ns"] = -1
			}
			writeBoundaryFixture(t, root, events, index)
			snapshot, err := BuildSnapshot(directory)
			if mutation != "none" {
				if err == nil || snapshot.IntegrityStatus != "incomplete" {
					t.Fatal("inconsistent model admission evidence accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			closed, rejected := 0, 0
			for _, event := range snapshot.Events {
				if event.Name == "model.admission_closed" {
					closed++
					if event.OffsetNS != 23 || event.ClockDomain != "clock" || event.ModelSessionID != "model" || event.ModelRequestInstanceID != "" {
						t.Fatal("closure was assigned a fake time, clock, or request")
					}
				}
			}
			for _, interval := range snapshot.Intervals {
				if interval.Name == "model.http_request" && interval.ModelRequestInstanceID == "b" {
					rejected++
					if interval.DurationNS != 1 || interval.Outcome != "rejected" || interval.Attributes["reason"] != "ADMISSION_CLOSED" {
						t.Fatal("late request was hidden or changed")
					}
				}
				if interval.ModelRequestInstanceID == "b" && interval.Name != "model.http_request" {
					t.Fatal("late request gained a model phase")
				}
			}
			if closed != 1 || rejected != 1 {
				t.Fatal("closure or rejected request missing")
			}
			if _, err := ProjectTrace(snapshot); err != nil {
				t.Fatal(err)
			}
			if _, err := ProjectOTLP(snapshot); err != nil {
				t.Fatal(err)
			}
		})
	}
}

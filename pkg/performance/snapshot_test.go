package performance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func boundaryFixture(t *testing.T, root string) ([]map[string]any, map[string]any) {
	t.Helper()
	writeSummaryJSON(t, root, "run-result.json", map[string]any{"run_id": "run", "attempts": []traceAttempt{{AttemptID: "run/occ", OccurrenceID: "occ", ModelSessionID: "model"}}})
	identity := map[string]any{"run_id": "run", "attempt_id": "run/occ", "session_id": "model"}
	rows := []struct {
		id, event        string
		offset, duration int64
		status           string
	}{
		{"a", "http_request_received", 10, 0, ""},
		// Receipts captured on concurrent goroutines can be queued in another order.
		{"b", "http_request_received", 9, 0, ""},
		{"b", "http_request_finished", 20, 11, "rejected"},
		{"a", "request_admitted", 21, 0, ""},
		{"a", "request_body_read_started", 22, 0, ""},
		{"a", "request_body_read", 24, 0, "succeeded"},
		{"a", "frame_flushed", 25, 0, "succeeded"},
		{"a", "http_request_finished", 30, 20, "succeeded"},
	}
	var events []map[string]any
	for i, row := range rows {
		e := map[string]any{"schema_version": 1, "sequence": i + 1, "identity": identity, "model_request_instance_id": row.id, "event": row.event, "emitter_id": "model-writer", "clock_domain": "clock", "monotonic_offset_ns": row.offset, "measurement_scope": "http_handler", "producer_location": "model_proxy", "mode": "replay", "timing_source": "recorded", "turn": 0, "status": row.status, "request": "SECRET_PROMPT", "reason": "SECRET_REASON"}
		if row.event == "http_request_finished" {
			e["duration_ns"] = row.duration
		}
		if row.event == "frame_flushed" {
			e["frame_index"] = 0
			e["target_offset_ns"] = 3
			e["actual_offset_ns"] = 4
		}
		events = append(events, e)
	}
	index := map[string]any{"schema_version": 1, "identity": identity, "emitter_id": "model-writer", "status": "complete", "attempted": len(events), "written": len(events), "dropped": 0, "write_failed": 0, "requests_received": 2, "requests_finished": 2, "clock": map[string]any{"emitter_id": "emitter", "clock_domain": "clock", "started_at_utc": summaryAnchor}, "session_started_offset_ns": 0, "events_file": "request-boundaries.jsonl"}
	writeBoundaryFixture(t, root, events, index)
	return events, index
}

func writeBoundaryFixture(t *testing.T, root string, events []map[string]any, index map[string]any) {
	t.Helper()
	dir := filepath.Join(root, "occ", "model")
	writeTraceEvents(t, filepath.Join(dir, "request-boundaries.jsonl"), events)
	writeSummaryJSON(t, dir, "request-boundaries-index.json", index)
}

func TestSnapshotDetachesProjectionFromRawEvidence(t *testing.T) {
	root, directory := traceFixture(t)
	modelTraceFixture(t, root)
	toolTraceFixture(t, root)
	first, err := BuildSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildSnapshot(directory)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("unstable snapshot: %v", err)
	}
	before, _ := json.Marshal(first)
	if err := os.Remove(filepath.Join(root, "occ", "model", "events.jsonl")); err != nil {
		t.Fatal(err)
	}
	trace, err := ProjectTrace(first)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(first)
	if string(before) != string(after) || trace.Metadata.SnapshotID != first.SnapshotID {
		t.Fatal("projection modified snapshot")
	}
	for _, source := range first.Sources {
		if source.Status == "present" && len(source.SHA256) != 64 {
			t.Fatal("missing immutable source digest")
		}
	}
	if strings.Contains(string(before), "SECRET_") {
		t.Fatal("snapshot leaked private payload")
	}
	if first.CoverageStatus != "unknown" {
		t.Fatal("received pairs claimed independent expected coverage")
	}
	for _, edge := range first.Edges {
		if edge.Correlation != "exact_id" && edge.Correlation != "verified_mapping" {
			t.Fatal("heuristic edge")
		}
	}
	first.Intervals[0].DurationNS++
	if _, err := ProjectTrace(first); err == nil {
		t.Fatal("modified snapshot silently accepted")
	}
}

func TestSnapshotMissingNativeSourceIsNotComplete(t *testing.T) {
	root, directory := traceFixture(t)
	modelTraceFixture(t, root)
	snapshot, err := BuildSnapshot(directory)
	if err == nil || snapshot.IntegrityStatus != "incomplete" || snapshot.CoverageStatus != "incomplete" {
		t.Fatal("entire missing native source counted complete")
	}
	found := false
	for _, coverage := range snapshot.Coverage {
		if coverage.Source == "native_tool" {
			found = true
			if coverage.Status != "not_collected" || coverage.ExpectedOperations != nil {
				t.Fatal("missing tools replaced by zero denominator")
			}
		}
	}
	if !found {
		t.Fatal("native coverage omitted")
	}
	if summary, err := Analyze(directory); err != nil || summary.Status != "complete" {
		t.Fatal("auxiliary missing data changed legacy Runner summary")
	}
}

func TestSnapshotRequestInstancesAllowConcurrentSameTurnAndSharedClock(t *testing.T) {
	root, directory := traceFixture(t)
	boundaryFixture(t, root)
	snapshot, err := BuildSnapshot(directory)
	if err != nil {
		t.Fatalf("%v: %v", err, snapshot.Diagnostics)
	}
	if len(snapshot.Clocks) != 1 {
		t.Fatal("explicit same-process anchor was not reused")
	}
	requests := map[string]int64{}
	for _, interval := range snapshot.Intervals {
		if interval.Name == "model.http_request" {
			requests[interval.ModelRequestInstanceID] = interval.DurationNS
		}
	}
	if !reflect.DeepEqual(requests, map[string]int64{"a": 20, "b": 11}) {
		t.Fatalf("request instances conflated: %v", requests)
	}
	for _, coverage := range snapshot.Coverage {
		if coverage.Source == "model_request_boundaries" && (coverage.Status != "complete" || coverage.ExpectedOperations == nil || *coverage.ExpectedOperations != 2 || coverage.Paired != 2) {
			t.Fatalf("producer denominator lost: %+v", coverage)
		}
	}
	trace, err := ProjectTraceDomain(snapshot, "clock")
	if err != nil || trace.Metadata.View != "single_clock_domain" {
		t.Fatal("domain projection failed", err)
	}
	encoded, _ := json.Marshal(trace)
	if strings.Contains(string(encoded), "SECRET_") {
		t.Fatal("boundary payload leaked")
	}
}

func TestSnapshotRejectsMissingWholeRequestAndCorruptBoundaries(t *testing.T) {
	for _, mutation := range []string{"whole_pair", "duplicate_id", "foreign_session", "missing_terminal", "anchor_conflict", "missing_index", "sequence_gap", "negative_offset"} {
		t.Run(mutation, func(t *testing.T) {
			root, directory := traceFixture(t)
			events, index := boundaryFixture(t, root)
			switch mutation {
			case "whole_pair":
				index["requests_received"], index["requests_finished"] = 3, 3
			case "duplicate_id":
				events[1]["model_request_instance_id"] = "a"
			case "foreign_session":
				events[2]["identity"] = map[string]any{"run_id": "run", "attempt_id": "run/occ", "session_id": "foreign"}
			case "missing_terminal":
				events = events[:len(events)-1]
			case "anchor_conflict":
				index["clock"].(map[string]any)["started_at_utc"] = summaryAnchor.Add(1)
			case "sequence_gap":
				events[2]["sequence"] = 99
			case "negative_offset":
				events[0]["monotonic_offset_ns"] = -1
			}
			writeBoundaryFixture(t, root, events, index)
			if mutation == "missing_index" {
				if err := os.Remove(filepath.Join(root, "occ", "model", "request-boundaries-index.json")); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, err := BuildSnapshot(directory)
			if err == nil || snapshot.IntegrityStatus != "incomplete" {
				t.Fatalf("accepted %s", mutation)
			}
			for _, interval := range snapshot.Intervals {
				if interval.Category == "model" {
					t.Fatal("unreconciled source produced trusted intervals")
				}
			}
		})
	}
}

func TestSnapshotStagesKeepMissingDurationsAbsent(t *testing.T) {
	root, directory := traceFixture(t)
	writeSummaryJSON(t, root, "run-result.json", map[string]any{"run_id": "run", "attempts": []map[string]any{{"attempt_id": "run/occ", "occurrence_id": "occ", "status": "failed", "queue_duration": 0, "timing": map[string]any{"runtime_execution_duration": 7}}}})
	snapshot, err := BuildSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Attempts) != 1 || len(snapshot.Attempts[0].Stages) != 9 {
		t.Fatal("eight canonical stages or additional bridge preparation missing")
	}
	stages := snapshot.Attempts[0].Stages
	if stages[0].DurationNS == nil || *stages[0].DurationNS != 0 || stages[1].DurationNS != nil || stages[5].DurationNS == nil || *stages[5].DurationNS != 7 {
		t.Fatal("zero and missing durations conflated")
	}
	if stages[3].Name != "bridge_prepare" || stages[3].Status != "not_collected" || stages[3].DurationNS != nil {
		t.Fatal("historical bridge preparation fabricated")
	}
	for _, interval := range snapshot.Intervals {
		if interval.Name == "runtime_execution" {
			t.Fatal("duration-only stage got fabricated start")
		}
	}
}

func TestSnapshotRejectsLateMarkerEvenAfterCompleteIndex(t *testing.T) {
	_, directory := traceFixture(t)
	writeSummaryJSON(t, directory, "late-after-close.json", map[string]any{"schema_version": 2, "run_id": "run", "emitter_id": "emitter", "status": "incomplete"})
	snapshot, err := BuildSnapshot(directory)
	if err == nil || snapshot.Summary.Status != "incomplete" {
		t.Fatal("late writer marker ignored")
	}
	found := false
	for _, source := range snapshot.Sources {
		if source.File == "performance/late-after-close.json" {
			found = true
			if len(source.SHA256) != 64 {
				t.Fatal("marker not hashed")
			}
		}
	}
	if !found {
		t.Fatal("marker omitted")
	}
}

func TestSnapshotLegacyV1AndUTCJumpPreserveMonotonicDuration(t *testing.T) {
	directory := summaryRecording(t, summaryPair("op", "task", "task", "succeeded", 17))
	for _, file := range []string{"manifest.json", "index.json"} {
		data, err := os.ReadFile(filepath.Join(directory, file))
		if err != nil {
			t.Fatal(err)
		}
		var object map[string]any
		if err := json.Unmarshal(data, &object); err != nil {
			t.Fatal(err)
		}
		object["schema_version"] = 1
		writeSummaryJSON(t, directory, file, object)
	}
	data, err := os.ReadFile(filepath.Join(directory, "runner-events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if !traceLines(data, func(line []byte) bool {
		var row map[string]any
		if json.Unmarshal(line, &row) != nil {
			return false
		}
		row["schema_version"] = 1
		rows = append(rows, row)
		return true
	}) {
		t.Fatal("fixture read")
	}
	rows[1]["observed_at_utc"] = summaryAnchor.Add(-1)
	writeTraceEvents(t, filepath.Join(directory, "runner-events.jsonl"), rows)
	snapshot, err := BuildSnapshot(directory)
	if err != nil || len(snapshot.Intervals) != 1 || snapshot.Intervals[0].DurationNS != 17 {
		t.Fatalf("legacy or UTC jump rejected: %v %+v", err, snapshot.Diagnostics)
	}
	trace, err := ProjectTrace(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range trace.Events {
		if event.Phase == "X" && (event.Duration == nil || *event.Duration != 0.017) {
			t.Fatal("ns to microseconds changed duration")
		}
	}
}

func TestSnapshotSingleClockViewExcludesUnalignedDomains(t *testing.T) {
	root, directory := traceFixture(t)
	modelTraceFixture(t, root)
	toolTraceFixture(t, root)
	snapshot, err := BuildSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	trace, err := ProjectTraceDomain(snapshot, "model-session/model")
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range trace.Events {
		if event.Phase != "M" && event.Args["clock_domain"] != "model-session/model" {
			t.Fatal("mixed unsynchronized clocks in single view")
		}
	}
	if _, err := ProjectTraceDomain(snapshot, "absent"); err == nil {
		t.Fatal("missing clock accepted")
	}
}

func TestSnapshotGatewayUsesDeliveryIdentityRatherThanSharedRequest(t *testing.T) {
	var events []Event
	for _, item := range []struct {
		id, name string
		offset   int64
	}{
		{"accepted", "gateway.frame.received", 10}, {"terminal", "gateway.frame.received", 11},
		{"terminal", "gateway.response.enqueue_offer", 12}, {"accepted", "gateway.response.enqueue_offer", 13},
		{"terminal", "gateway.response.consumed", 20}, {"accepted", "gateway.response.consumed", 23},
	} {
		events = append(events, Event{Identity: Identity{OccurrenceID: "occ", Purpose: "task"}, EventType: "observation", Operation: item.name, ObservedOffsetNS: item.offset, SourceComponent: "gateway", MeasurementScope: "controller_observation", RequestInstanceID: "same-agent-request", GatewayDeliveryID: item.id})
	}
	directory := summaryRecording(t, events)
	snapshot, err := BuildSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	queues := map[string]int64{}
	for _, interval := range snapshot.Intervals {
		if interval.Name == "gateway.delivery.offer_to_consume" {
			queues[interval.GatewayDeliveryID] = interval.DurationNS
		}
	}
	if !reflect.DeepEqual(queues, map[string]int64{"accepted": 10, "terminal": 8}) {
		t.Fatalf("distinct responses conflated: %v", queues)
	}
	if len(snapshot.Summary.Operations) != 0 {
		t.Fatal("instantaneous observations polluted legacy operation stats")
	}
}

func TestSnapshotModelLateMarkerAndControlledReasons(t *testing.T) {
	root, directory := traceFixture(t)
	events, index := boundaryFixture(t, root)
	events[2]["reason"] = "SESSION_BUSY"
	events[5]["status"] = "failed"
	writeBoundaryFixture(t, root, events, index)
	snapshot, err := BuildSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	busy, failed := false, false
	for _, interval := range snapshot.Intervals {
		if interval.ModelRequestInstanceID == "b" && interval.Name == "model.http_request" {
			busy = interval.Attributes["reason"] == "SESSION_BUSY"
		}
		if interval.Name == "model.body_read" {
			failed = interval.Outcome == "failed"
		}
	}
	if !busy || !failed {
		t.Fatal("controlled reject reason or failed phase hidden")
	}
	writeSummaryJSON(t, filepath.Join(root, "occ", "model"), "request-boundaries-late-after-close.json", map[string]any{"status": "incomplete"})
	if snapshot, err := BuildSnapshot(directory); err == nil || snapshot.IntegrityStatus != "incomplete" {
		t.Fatal("late model writer accepted")
	}
}

func TestSnapshotModelFrameIdentityValidation(t *testing.T) {
	for _, family := range []string{"upstream_frame_received", "frame_flushed"} {
		for _, mutation := range []string{"valid", "missing", "null", "negative", "duplicate", "gap"} {
			t.Run(family+"/"+mutation, func(t *testing.T) {
				root, directory := traceFixture(t)
				events, index := boundaryFixture(t, root)
				base, terminal := events[6], events[7]
				events = events[:6]
				for frame := 0; frame < 2; frame++ {
					for _, name := range []string{"upstream_frame_received", "frame_flushed"} {
						event := make(map[string]any)
						for key, value := range base {
							event[key] = value
						}
						event["event"], event["frame_index"], event["mode"] = name, frame, "record"
						if name == family && frame == 1 {
							switch mutation {
							case "missing":
								delete(event, "frame_index")
							case "null":
								event["frame_index"] = nil
							case "negative":
								event["frame_index"] = -1
							case "duplicate":
								event["frame_index"] = 0
							case "gap":
								event["frame_index"] = 2
							}
						}
						events = append(events, event)
					}
				}
				events = append(events, terminal)
				for i, event := range events {
					event["sequence"] = i + 1
				}
				index["attempted"], index["written"] = len(events), len(events)
				writeBoundaryFixture(t, root, events, index)
				snapshot, err := BuildSnapshot(directory)
				if mutation == "valid" {
					if err != nil {
						t.Fatalf("valid frame stream rejected: %v", err)
					}
				} else {
					if err == nil || snapshot.IntegrityStatus != "incomplete" {
						t.Fatal("invalid frame identity accepted")
					}
					for _, interval := range snapshot.Intervals {
						if interval.Category == "model" {
							t.Fatal("corrupt frame source projected trusted interval")
						}
					}
				}
			})
		}
	}
}

func TestSnapshotUnfinishedModelPhasePreservesEvidence(t *testing.T) {
	for _, headers := range []bool{false, true} {
		name := "transport_failed"
		if headers {
			name = "headers_observed"
		}
		t.Run(name, func(t *testing.T) {
			root, directory := traceFixture(t)
			events, index := boundaryFixture(t, root)
			events[6]["event"] = "upstream_request_started"
			events[6]["measurement_scope"] = "upstream_http_wait_for_headers"
			delete(events[6], "frame_index")
			events[7]["status"], events[7]["reason"] = "failed", "DELIVERY_OR_UPSTREAM_FAILED"
			if headers {
				end := make(map[string]any)
				for key, value := range events[6] {
					end[key] = value
				}
				end["event"], end["monotonic_offset_ns"], end["http_status"] = "upstream_response_headers", 27, 503
				delete(end, "status")
				events = append(events[:7], end, events[7])
			}
			for i, event := range events {
				event["sequence"] = i + 1
			}
			index["attempted"], index["written"] = len(events), len(events)
			writeBoundaryFixture(t, root, events, index)
			snapshot, err := BuildSnapshot(directory)
			if err != nil || snapshot.IntegrityStatus != "complete" {
				t.Fatalf("legitimate failed HTTP request treated as corrupt evidence: %v", err)
			}
			startFound, phaseFound, failedRequest := false, false, false
			for _, event := range snapshot.Events {
				if event.Name == "upstream_request_started" {
					startFound = event.OffsetNS == 25
					if !headers && (event.Attributes["phase_coverage_status"] != "incomplete" || event.Attributes["missing_boundary"] != "terminal") {
						t.Fatal("unfinished start not marked")
					}
				}
			}
			for _, interval := range snapshot.Intervals {
				if interval.Name == "model.upstream_headers" {
					phaseFound = true
					if interval.DurationNS != 2 || interval.Outcome != "observed" {
						t.Fatal("header boundary changed")
					}
				}
				if interval.Name == "model.http_request" && interval.ModelRequestInstanceID == "a" {
					failedRequest = interval.Outcome == "failed" && interval.DurationNS == 20
				}
			}
			if !startFound || !failedRequest || phaseFound != headers {
				t.Fatal("lost real boundaries or invented phase endpoint")
			}
			for _, coverage := range snapshot.Coverage {
				if coverage.Source != "model_request_boundaries" {
					continue
				}
				if coverage.Integrity != "complete" || coverage.Paired != 2 {
					t.Fatal("request recording integrity changed")
				}
				want := "incomplete"
				if headers {
					want = "complete"
				}
				if coverage.Status != want {
					t.Fatalf("phase coverage = %s, want %s", coverage.Status, want)
				}
			}
			if !headers && (snapshot.CoverageStatus != "incomplete" || !strings.Contains(strings.Join(snapshot.Diagnostics, "\n"), "upstream_headers missing terminal")) {
				t.Fatal("unfinished phase hidden from coverage")
			}
			trace, err := ProjectTrace(snapshot)
			if err != nil || trace.Metadata.CoverageStatus != snapshot.CoverageStatus {
				t.Fatal("honest partial phase evidence could not be projected", err)
			}
		})
	}
}

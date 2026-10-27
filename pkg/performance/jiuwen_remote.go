package performance

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const jiuwenBridgeScope = "controller_forwarding_not_guest_execution"
const jiuwenWorkerScope = "worker_request_prepare_to_native_result_write_not_process"

type jiuwenRemoteIdentity struct {
	Run     string `json:"run_id"`
	Task    string `json:"task_id"`
	Session string `json:"session_id"`
	Request string `json:"request_id"`
	Tool    string `json:"tool_call_id"`
	Status  string `json:"identity_status"`
}
type jiuwenRemoteOperation struct {
	Kind   string `json:"kind"`
	Method string `json:"method"`
}
type jiuwenBridgeRow struct {
	Schema     int                   `json:"schema_version"`
	Sequence   uint64                `json:"sequence"`
	Binding    string                `json:"binding_id"`
	Sandbox    string                `json:"sandbox_id"`
	Invocation string                `json:"invocation_id"`
	Identity   jiuwenRemoteIdentity  `json:"identity"`
	Operation  jiuwenRemoteOperation `json:"operation"`
	Route      string                `json:"route"`
	Domain     string                `json:"clock_domain"`
	Start      *int64                `json:"controller_start_offset_ns"`
	End        *int64                `json:"controller_end_offset_ns"`
	Duration   *int64                `json:"controller_duration_ns"`
	HTTP       int                   `json:"http_status"`
	Complete   bool                  `json:"transport_complete"`
	Scope      string                `json:"measurement_scope"`
}
type jiuwenBridgeIndex struct {
	Schema    int     `json:"schema_version"`
	Run       string  `json:"run_id"`
	Task      string  `json:"task_id"`
	Binding   string  `json:"binding_id"`
	Sandbox   string  `json:"sandbox_id"`
	Clock     *Clock  `json:"clock"`
	Accepted  *uint64 `json:"accepted_calls"`
	Finished  *uint64 `json:"finished_calls"`
	Records   *uint64 `json:"records"`
	Truncated *bool   `json:"truncated"`
	Empty     bool    `json:"group_empty_confirmed"`
	Joined    bool    `json:"local_calls_joined"`
}
type jiuwenWorkerClock struct {
	Emitter string `json:"emitter_id"`
	Domain  string `json:"clock_domain"`
	Origin  int64  `json:"origin_monotonic_ns"`
	Unix    int64  `json:"origin_unix_ns"`
	Before  int64  `json:"anchor_before_monotonic_ns"`
	After   int64  `json:"anchor_after_monotonic_ns"`
}
type jiuwenWorkerIndex struct {
	Schema      int               `json:"schema_version"`
	Run         string            `json:"run_id"`
	Task        string            `json:"task_id"`
	Binding     string            `json:"binding_id"`
	Clock       jiuwenWorkerClock `json:"clock"`
	Admitted    *uint64           `json:"admitted"`
	Terminal    *uint64           `json:"terminal"`
	Active      *uint64           `json:"active"`
	Attempted   *uint64           `json:"attempted"`
	Written     *uint64           `json:"written"`
	Dropped     *uint64           `json:"dropped"`
	Failed      *uint64           `json:"write_failures"`
	IndexFailed *uint64           `json:"index_write_failures"`
	Scope       string            `json:"measurement_scope"`
}
type jiuwenWorkerRow struct {
	Schema     int                   `json:"schema_version"`
	Sequence   uint64                `json:"sequence"`
	Event      string                `json:"event"`
	Invocation string                `json:"invocation_id"`
	Identity   jiuwenRemoteIdentity  `json:"identity"`
	Operation  jiuwenRemoteOperation `json:"operation"`
	Domain     string                `json:"clock_domain"`
	Monotonic  int64                 `json:"monotonic_ns"`
	Unix       int64                 `json:"unix_ns"`
	Outcome    string                `json:"outcome"`
	Exit       *int                  `json:"exit_code"`
	Status     string                `json:"status"`
}

func (id jiuwenRemoteIdentity) valid(s *AnalysisSnapshot, a traceAttempt) bool {
	return id.Run == s.RunID && id.Task == a.OccurrenceID &&
		(id.Status == "known" && validAnalysisIdentity(id.Tool) || id.Status == "unavailable" && id.Tool == "") &&
		(id.Session == "" || validAnalysisIdentity(id.Session)) && (id.Request == "" || validAnalysisIdentity(id.Request))
}
func (op jiuwenRemoteOperation) valid() bool {
	return (op.Kind == "fs" || op.Kind == "shell") && nativeLabel(op.Method)
}
func jiuwenRemoteAnalysisIdentity(a traceAttempt, id jiuwenRemoteIdentity) AnalysisIdentity {
	return AnalysisIdentity{Identity: Identity{OccurrenceID: a.OccurrenceID, LogicalTaskID: a.LogicalTaskID, ArrivalID: a.ArrivalID},
		AttemptID: a.AttemptID, AgentSessionID: id.Session, RequestInstanceID: id.Request, ToolCallID: id.Tool}
}

// An exact identity relation supplies no latency or cross-clock ordering.
func (s *AnalysisSnapshot) jiuwenNativeToolLinks(attempt traceAttempt) {
	key := func(id AnalysisIdentity) [4]string {
		return [4]string{id.AttemptID, id.AgentSessionID, id.RequestInstanceID, id.ToolCallID}
	}
	native := make(map[[4]string][]AnalysisNativeSpan)
	for _, span := range s.NativeSpans {
		if span.AttemptID == attempt.AttemptID && span.AuthoritativeTool {
			native[key(span.AnalysisIdentity)] = append(native[key(span.AnalysisIdentity)], span)
		}
	}
	for i := range s.Intervals {
		interval := &s.Intervals[i]
		if interval.AttemptID != attempt.AttemptID || interval.Category != "jiuwen_remote_bridge" {
			continue
		}
		id := key(interval.AnalysisIdentity)
		status := "native_tool_not_found"
		matches := native[id]
		switch {
		case id[0] == "" || id[1] == "" || id[2] == "" || id[3] == "":
			status = "missing_identity"
		case len(matches) > 1:
			status = "ambiguous_native_tool"
		case len(matches) == 1:
			status = "exact_id"
			span := matches[0]
			interval.Attributes["native_tool_operation_id"] = span.ID
			s.Edges = append(s.Edges, AnalysisEdge{From: span.ID, To: interval.ID,
				Kind: "tool_call_identity", Correlation: "exact_id",
				EvidenceRefs: append([]string{span.EvidenceRef}, interval.EvidenceRefs...)})
		}
		interval.Attributes["native_tool_association"] = status
		interval.Attributes["native_tool_match_count"] = len(matches)
	}
}

// Remote operation evidence has its own boundaries; it is never OS process timing.
func (s *AnalysisSnapshot) jiuwenRemoteSnapshot(root string, a traceAttempt) {
	base := filepath.Join(a.OccurrenceID, "bridge")
	indexFile := filepath.Join(base, "remote-bridge-index.json")
	raw, err := readIdentityEvidence(root, indexFile)
	if os.IsNotExist(err) {
		for _, name := range []string{"remote-bridge-events.jsonl", "remote-worker-events.jsonl", "remote-worker-index.json"} {
			if _, probeErr := os.Lstat(filepath.Join(root, base, name)); !os.IsNotExist(probeErr) {
				s.problem("Jiuwen remote artifacts lack bridge index")
				return
			}
		}
		return
	}
	s.source(indexFile, raw, "identity_only", "not_a_clock", err)
	coverage := SourceCoverage{Source: "jiuwen_remote_bridge", AttemptID: a.AttemptID, Integrity: "incomplete", Status: "unknown", Missing: []string{"agent_to_bridge_submission", "guest_process_exit_and_pipe_drain"}}
	defer func() { s.Coverage = append(s.Coverage, coverage) }()
	var index jiuwenBridgeIndex
	if err != nil || identityJSON(raw, &index, false) != nil || (index.Schema != 1 && index.Schema != 2) || index.Run != s.RunID || index.Task != a.OccurrenceID || !validAnalysisIdentity(index.Binding) || !validAnalysisIdentity(index.Sandbox) {
		s.problem("Jiuwen remote bridge index invalid for " + a.AttemptID)
		return
	}
	file := filepath.Join(base, "remote-bridge-events.jsonl")
	data, readErr := readIdentityEvidence(root, file)
	eventRef := s.source(file, data, "identity_only", "not_a_clock", readErr)
	if readErr != nil {
		s.problem("Jiuwen remote bridge events unavailable for " + a.AttemptID)
		return
	}
	rows := []jiuwenBridgeRow{}
	valid := traceLines(data, func(line []byte) bool {
		var row jiuwenBridgeRow
		if identityJSON(line, &row, false) != nil {
			return false
		}
		rows = append(rows, row)
		return true
	})
	if !valid {
		s.problem("Jiuwen remote bridge JSON invalid")
		return
	}
	if index.Schema == 1 {
		// Historical durations are retained without deriving monotonic origins from wall time.
		for i, row := range rows {
			if row.Route == "/call" && row.Identity.valid(s, a) && row.Duration != nil && *row.Duration >= 0 {
				s.UntimedOperations = append(s.UntimedOperations, AnalysisUntimedOperation{AttemptID: a.AttemptID, OperationID: stableAnalysisID("jiuwen-remote-legacy", a.AttemptID, row.Invocation), Name: "jiuwen.bridge.forwarding", Outcome: "unknown", DurationNS: row.Duration, MeasurementStatus: "duration_only", EvidenceRef: eventRef, SourceLine: uint64(i + 1), Attributes: map[string]any{"invocation_id": row.Invocation, "tool_call_id": row.Identity.Tool, "measurement_scope": jiuwenBridgeScope}})
			}
		}
		coverage.Integrity = "complete"
		coverage.Missing = append(coverage.Missing, "legacy_clock_and_producer_counters_not_collected")
		return
	}
	if index.Accepted == nil || index.Finished == nil || index.Records == nil || *index.Accepted != *index.Finished || *index.Records != uint64(len(rows)) || index.Truncated == nil || *index.Truncated || !index.Empty || !index.Joined {
		s.problem("Jiuwen remote bridge counter, truncation or stop evidence incomplete")
		return
	}
	aligned := false
	if index.Clock != nil {
		for _, clock := range s.Clocks {
			if clock.ID == s.Summary.ClockDomain && index.Clock.ClockDomain == clock.ID && index.Clock.EmitterID == clock.EmitterID && clock.OriginUTC != nil && clock.OriginUTC.Equal(index.Clock.StartedAtUTC) {
				aligned = true
			}
		}
	}
	if index.Clock != nil && !aligned {
		s.problem("Jiuwen remote bridge clock differs from Runner")
		return
	}
	calls := map[string]jiuwenBridgeRow{}
	ids := map[string]string{}
	for i, row := range rows {
		if row.Schema != 2 || row.Sequence != uint64(i+1) || row.Binding != index.Binding || row.Sandbox != index.Sandbox || row.Duration == nil || *row.Duration < 0 || row.Scope != jiuwenBridgeScope || (row.Route != "/call" && row.Route != "/cancel") {
			s.problem("Jiuwen remote bridge row invalid")
			return
		}
		if row.Route != "/call" {
			continue
		}
		if !validAnalysisIdentity(row.Invocation) || !row.Identity.valid(s, a) || !row.Operation.valid() {
			s.problem("Jiuwen remote bridge invocation identity invalid")
			return
		}
		if _, duplicate := calls[row.Invocation]; duplicate {
			s.problem("duplicate Jiuwen remote invocation")
			return
		}
		calls[row.Invocation] = row
		id := stableAnalysisID("jiuwen-bridge", a.AttemptID, row.Invocation)
		ids[row.Invocation] = id
		attributes := map[string]any{"invocation_id": row.Invocation, "operation_kind": row.Operation.Kind, "operation_method": row.Operation.Method, "identity_status": row.Identity.Status, "http_status": row.HTTP, "transport_complete": row.Complete, "process_completion": "unknown"}
		outcome := "failed"
		if row.Complete && row.HTTP >= 200 && row.HTTP < 300 {
			outcome = "succeeded"
		}
		if !aligned {
			if row.Start != nil || row.End != nil || row.Domain != "" {
				s.problem("clockless Jiuwen bridge row has offsets")
				return
			}
			s.UntimedOperations = append(s.UntimedOperations, AnalysisUntimedOperation{AttemptID: a.AttemptID, OperationID: id, Name: "jiuwen.bridge.forwarding", Outcome: outcome, DurationNS: row.Duration, MeasurementStatus: "duration_only", EvidenceRef: eventRef, SourceLine: uint64(i + 1), Attributes: attributes})
			continue
		}
		if row.Domain != index.Clock.ClockDomain || row.Start == nil || row.End == nil || *row.Start < 0 || *row.End < *row.Start || *row.End-*row.Start != *row.Duration {
			s.problem("Jiuwen bridge monotonic bounds invalid")
			return
		}
		identity := jiuwenRemoteAnalysisIdentity(a, row.Identity)
		start := analysisEvent(eventRef, uint64(i+1), "jiuwen.bridge.start", "jiuwen_remote_bridge", row.Domain, jiuwenBridgeScope, *row.Start, identity, attributes)
		end := analysisEvent(eventRef, uint64(i+1), "jiuwen.bridge.end", "jiuwen_remote_bridge", row.Domain, jiuwenBridgeScope, *row.End, identity, attributes)
		end.ID = stableAnalysisID("jiuwen-bridge-end", start.ID)
		s.Events = append(s.Events, start, end)
		s.interval(id, "jiuwen.bridge.forwarding", "jiuwen_remote_bridge", jiuwenBridgeScope, outcome, start, end, attributes)
	}
	if uint64(len(calls)) != *index.Accepted {
		s.problem("Jiuwen bridge admitted invocation denominator differs")
		return
	}
	coverage.Integrity = "complete"
	coverage.ObservedEvents = uint64(len(rows))
	coverage.ExpectedEvents = index.Records
	coverage.ExpectedOperations = index.Accepted
	coverage.Started = *index.Accepted
	coverage.Paired = uint64(len(calls))
	if !aligned {
		coverage.Missing = append(coverage.Missing, "controller_monotonic_clock_not_collected")
	}
	s.jiuwenWorkerSnapshot(root, a, index, calls, ids)
}

func (s *AnalysisSnapshot) jiuwenWorkerSnapshot(root string, a traceAttempt, bridge jiuwenBridgeIndex, calls map[string]jiuwenBridgeRow, bridgeIDs map[string]string) {
	base := filepath.Join(a.OccurrenceID, "bridge")
	raw, err := readIdentityEvidence(root, filepath.Join(base, "remote-worker-index.json"))
	indexRef := s.source(filepath.Join(base, "remote-worker-index.json"), raw, "identity_only", "not_a_clock", err)
	coverage := SourceCoverage{Source: "jiuwen_remote_worker", AttemptID: a.AttemptID, Integrity: "incomplete", Status: "unknown", Missing: []string{"guest_process_exit_and_pipe_drain", "cross_host_clock_alignment"}}
	defer func() { s.Coverage = append(s.Coverage, coverage) }()
	var index jiuwenWorkerIndex
	if err != nil || identityJSON(raw, &index, false) != nil || index.Schema != 1 || index.Run != s.RunID || index.Task != a.OccurrenceID || index.Binding != bridge.Binding || index.Scope != jiuwenWorkerScope {
		s.problem("Jiuwen worker index invalid")
		return
	}
	if index.Admitted == nil || index.Terminal == nil || index.Active == nil || index.Attempted == nil || index.Written == nil || index.Dropped == nil || index.Failed == nil || index.IndexFailed == nil || *index.Admitted != *index.Terminal || *index.Active != 0 || *index.Attempted != *index.Written || *index.Dropped != 0 || *index.Failed != 0 || *index.IndexFailed != 0 {
		s.problem("Jiuwen worker recording or terminal counters incomplete")
		return
	}
	c := index.Clock
	if !validAnalysisIdentity(c.Emitter) || c.Domain != "jiuwen-worker/"+c.Emitter || c.Before <= 0 || c.After < c.Before || c.Origin != c.Before+(c.After-c.Before)/2 || c.Unix <= 0 {
		s.problem("Jiuwen worker local clock anchor invalid")
		return
	}
	origin := time.Unix(0, c.Unix).UTC()
	s.clock(AnalysisClock{ID: c.Domain, EmitterID: c.Emitter, OriginUTC: &origin, Alignment: "local_monotonic_anchor", EvidenceRef: indexRef})
	file := filepath.Join(base, "remote-worker-events.jsonl")
	data, err := readIdentityEvidence(root, file)
	ref := s.source(file, data, c.Domain, "local_monotonic_anchor", err)
	if err != nil {
		s.problem("Jiuwen worker events unavailable")
		return
	}
	starts := map[string]AnalysisEvent{}
	ends := map[string]bool{}
	returned := map[string]jiuwenWorkerRow{}
	var sequence uint64
	var last int64
	valid := traceLines(data, func(line []byte) bool {
		var row jiuwenWorkerRow
		if identityJSON(line, &row, false) != nil || row.Schema != 1 || row.Sequence != sequence+1 || row.Domain != c.Domain || row.Monotonic < c.Origin || row.Monotonic < last || row.Unix <= 0 || !row.Identity.valid(s, a) || !row.Operation.valid() {
			return false
		}
		sequence++
		last = row.Monotonic
		call, ok := calls[row.Invocation]
		if !ok || call.Identity != row.Identity || call.Operation != row.Operation {
			return false
		}
		if ends[row.Invocation] {
			return false
		}
		attrs := map[string]any{"invocation_id": row.Invocation, "operation_kind": row.Operation.Kind, "operation_method": row.Operation.Method, "identity_status": row.Identity.Status, "raw_unix_ns": row.Unix, "anchor_uncertainty_ns": (c.After - c.Before + 1) / 2, "cross_host_alignment": "unverified", "process_completion": "unknown"}
		identity := jiuwenRemoteAnalysisIdentity(a, row.Identity)
		point := analysisEvent(ref, sequence, "jiuwen.worker."+row.Event, "jiuwen_remote_worker", c.Domain, jiuwenWorkerScope, row.Monotonic-c.Origin, identity, attrs)
		switch row.Event {
		case "invocation_start":
			if _, exists := starts[row.Invocation]; exists {
				return false
			}
			starts[row.Invocation] = point
		case "invocation_returned":
			if _, ok := starts[row.Invocation]; !ok {
				return false
			}
			if _, duplicate := returned[row.Invocation]; duplicate {
				return false
			}
			returned[row.Invocation] = row
		case "invocation_cancelled", "transport_disconnected", "invocation_error":
			if _, ok := starts[row.Invocation]; !ok {
				return false
			}
		case "invocation_terminal":
			start, ok := starts[row.Invocation]
			if !ok {
				return false
			}
			outcome := "failed"
			switch row.Outcome {
			case "returned":
				if _, ok := returned[row.Invocation]; !ok {
					return false
				}
				outcome = "succeeded"
			case "canceled":
				outcome = "canceled"
			case "disconnected", "error":
			default:
				return false
			}
			if result, ok := returned[row.Invocation]; ok {
				attrs["native_exit_code"] = result.Exit
				attrs["native_status"] = result.Status
			}
			id := stableAnalysisID("jiuwen-worker", a.AttemptID, row.Invocation)
			s.interval(id, "jiuwen.worker.native_operation_request", "jiuwen_remote_worker", jiuwenWorkerScope, outcome, start, point, attrs)
			if call.Start != nil {
				s.Edges = append(s.Edges, AnalysisEdge{From: bridgeIDs[row.Invocation], To: id, Kind: "invocation_identity", Correlation: "exact_id", EvidenceRefs: []string{ref, indexRef}})
			}
			ends[row.Invocation] = true
		default:
			return false
		}
		s.Events = append(s.Events, point)
		return true
	})
	if !valid || sequence != *index.Written || uint64(len(starts)) != *index.Admitted || len(starts) != len(ends) {
		s.problem("Jiuwen worker identity, events or denominator incomplete")
		return
	}
	for id, call := range calls {
		if call.Complete && call.HTTP >= 200 && call.HTTP < 300 && !ends[id] {
			s.problem("successful Jiuwen bridge invocation missing worker evidence")
			return
		}
	}
	coverage.Integrity = "complete"
	coverage.ObservedEvents = sequence
	coverage.ExpectedEvents = index.Attempted
	coverage.DroppedEvents = index.Dropped
	coverage.WriteFailedEvents = index.Failed
	coverage.ExpectedOperations = index.Admitted
	coverage.Started = *index.Admitted
	coverage.Paired = *index.Terminal
	s.Limitations = append(s.Limitations, fmt.Sprintf("Jiuwen worker %s uses a local monotonic/Unix anchor with %d ns read-window uncertainty; it does not align guest and controller clocks. Request durations include result delivery and are not OS process execution or network latency.", c.Emitter, (c.After-c.Before+1)/2))
}

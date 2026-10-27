package performance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type modelTraceEvent struct {
	Event    string `json:"event"`
	Identity struct {
		RunID     string `json:"run_id"`
		AttemptID string `json:"attempt_id"`
		SessionID string `json:"session_id"`
	} `json:"identity"`
	Offset       *int64 `json:"observed_offset_ns"`
	Turn         int    `json:"turn"`
	Mode         string `json:"mode"`
	ReplayPolicy string `json:"replay_policy"`
	Timing       string `json:"timing_source"`
	Status       string `json:"status"`
	Duration     int64  `json:"duration_ns"`
	Frame        int    `json:"frame"`
	Target       *int64 `json:"target_offset_ns"`
	Actual       *int64 `json:"actual_offset_ns"`
	Received     *int64 `json:"received_offset_ns"`
}

type modelBoundary struct {
	SchemaVersion int    `json:"schema_version"`
	Sequence      uint64 `json:"sequence"`
	Identity      struct {
		RunID     string `json:"run_id"`
		AttemptID string `json:"attempt_id"`
		SessionID string `json:"session_id"`
	} `json:"identity"`
	RequestID              string                       `json:"model_request_instance_id"`
	Event                  string                       `json:"event"`
	EmitterID              string                       `json:"emitter_id"`
	ClockDomain            string                       `json:"clock_domain"`
	Offset                 *int64                       `json:"monotonic_offset_ns"`
	Scope                  string                       `json:"measurement_scope"`
	Location               string                       `json:"producer_location"`
	Route                  string                       `json:"route"`
	Mode                   string                       `json:"mode"`
	ReplayPolicy           string                       `json:"replay_policy"`
	ToolContentDifferences []modelToolContentDifference `json:"tool_content_differences"`
	Timing                 string                       `json:"timing_source"`
	Turn                   *int                         `json:"turn"`
	Frame                  *int                         `json:"frame_index"`
	Status                 string                       `json:"status"`
	Duration               *int64                       `json:"duration_ns"`
	HTTPStatus             *int                         `json:"http_status"`
	Reason                 string                       `json:"reason"`
	CachedRetry            bool                         `json:"cached_retry"`
	Branch                 string                       `json:"model_branch"`
	Target                 *int64                       `json:"target_offset_ns"`
	Actual                 *int64                       `json:"actual_offset_ns"`
	Received               *int64                       `json:"received_offset_ns"`
}

type modelBoundaryIndex struct {
	SchemaVersion int `json:"schema_version"`
	Identity      struct {
		RunID     string `json:"run_id"`
		AttemptID string `json:"attempt_id"`
		SessionID string `json:"session_id"`
	} `json:"identity"`
	EmitterID             string `json:"emitter_id"`
	ReplayPolicy          string `json:"replay_policy"`
	Status                string `json:"status"`
	Attempted             uint64 `json:"attempted"`
	Written               uint64 `json:"written"`
	Dropped               uint64 `json:"dropped"`
	WriteFailed           uint64 `json:"write_failed"`
	RequestsReceived      uint64 `json:"requests_received"`
	RequestsFinished      uint64 `json:"requests_finished"`
	EventsFile            string `json:"events_file"`
	SessionOffset         *int64 `json:"session_started_offset_ns"`
	AdmissionClosedOffset *int64 `json:"admission_closed_offset_ns"`
	Clock                 struct {
		EmitterID string    `json:"emitter_id"`
		Domain    string    `json:"clock_domain"`
		StartedAt time.Time `json:"started_at_utc"`
	} `json:"clock"`
	EventsSHA256 string `json:"events_sha256"`
	EventsBytes  int64  `json:"events_bytes"`
}

func (s *AnalysisSnapshot) modelSnapshot(root string, attempt traceAttempt) {
	file := filepath.Join(attempt.OccurrenceID, "model", "request-boundaries.jsonl")
	data, err := os.ReadFile(filepath.Join(root, file))
	indexFile := filepath.Join(attempt.OccurrenceID, "model", "request-boundaries-index.json")
	indexData, indexErr := os.ReadFile(filepath.Join(root, indexFile))
	lateFile := filepath.Join(attempt.OccurrenceID, "model", "request-boundaries-late-after-close.json")
	lateData, lateErr := os.ReadFile(filepath.Join(root, lateFile))
	if !os.IsNotExist(lateErr) {
		s.source(lateFile, lateData, "identity_only", "not_a_clock", lateErr)
		s.problem("model request boundary producer appended after close or late marker unreadable for " + attempt.AttemptID)
	}
	if os.IsNotExist(err) && os.IsNotExist(indexErr) && os.IsNotExist(lateErr) {
		s.legacyModel(root, attempt)
		return
	}
	coverage := SourceCoverage{Source: "model_request_boundaries", AttemptID: attempt.AttemptID, Integrity: "incomplete", Status: "incomplete", Missing: []string{}}
	var index modelBoundaryIndex
	validIndex := indexErr == nil && decodeDocument(indexData, &index, "attempted", "written", "dropped", "write_failed", "requests_received", "requests_finished") == nil
	if validIndex {
		coverage.ExpectedEvents, coverage.ExpectedOperations, coverage.DroppedEvents, coverage.WriteFailedEvents = &index.Attempted, &index.RequestsReceived, &index.Dropped, &index.WriteFailed
	}
	clock := index.Clock.Domain
	ref := s.source(file, data, clock, "explicit_local_clock_anchor", err)
	indexRef := s.source(indexFile, indexData, clock, "explicit_local_clock_anchor", indexErr)
	validIndex = validIndex && index.SchemaVersion == 1 && index.Identity.RunID == s.RunID && index.Identity.AttemptID == attempt.AttemptID && index.Identity.SessionID == attempt.ModelSessionID && index.EmitterID != "" && clock != "" && index.Clock.EmitterID != "" && !index.Clock.StartedAt.IsZero() && index.SessionOffset != nil && *index.SessionOffset >= 0 && index.EventsFile == "request-boundaries.jsonl" && index.Status == "complete" && index.Dropped == 0 && index.WriteFailed == 0 && index.Attempted == index.Written && index.RequestsReceived == index.RequestsFinished
	if validIndex && index.AdmissionClosedOffset != nil {
		validIndex = *index.AdmissionClosedOffset >= *index.SessionOffset
	}
	validIndex = validIndex && validModelReplayPolicy("replay", index.ReplayPolicy)
	if attempt.ModelMode != "" {
		validIndex = validIndex && index.ReplayPolicy == attempt.ReplayPolicy
	}
	if validIndex && index.EventsSHA256 != "" {
		digest := sha256.Sum256(data)
		validIndex = index.EventsBytes == int64(len(data)) && index.EventsSHA256 == hex.EncodeToString(digest[:])
	}
	if !validIndex || err != nil || !os.IsNotExist(lateErr) {
		coverage.Missing = append(coverage.Missing, "missing_or_invalid_boundary_index_or_events")
		s.Coverage = append(s.Coverage, coverage)
		s.problem("model request boundary index or source invalid for " + attempt.AttemptID)
		return
	}
	coverage.ExpectedEvents, coverage.ExpectedOperations = &index.Attempted, &index.RequestsReceived
	if !s.clock(AnalysisClock{ID: clock, EmitterID: index.Clock.EmitterID, OriginUTC: &index.Clock.StartedAt, Alignment: "local_monotonic_anchor", EvidenceRef: indexRef}) {
		s.Coverage = append(s.Coverage, coverage)
		return
	}
	type request struct {
		points                        map[string]AnalysisEvent
		start, finish                 *AnalysisEvent
		lastOffset                    int64
		receivedFrames, flushedFrames int
		flushFailed                   bool
	}
	requests := make(map[string]*request)
	var sequence uint64
	firstEvent, firstInterval := len(s.Events), len(s.Intervals)
	allowed := map[string]bool{"http_request_received": true, "request_admitted": true, "request_body_read_started": true, "request_body_read": true, "request_parse_started": true, "request_parsed": true, "request_match_started": true, "request_matched": true, "upstream_request_started": true, "upstream_response_headers": true, "upstream_frame_received": true, "upstream_first_sse": true, "upstream_first_content": true, "upstream_first_reasoning": true, "upstream_first_tool_call_delta": true, "frame_flushed": true, "frame_flush_failed": true, "downstream_first_sse": true, "downstream_first_content": true, "downstream_first_reasoning": true, "downstream_first_tool_call_delta": true, "http_request_finished": true}
	for _, event := range []string{"upstream_json_received", "json_flushed", "json_flush_failed"} {
		allowed[event] = true
	}
	valid := traceLines(data, func(line []byte) bool {
		var e modelBoundary
		if json.Unmarshal(line, &e) != nil || e.SchemaVersion != 1 || e.Sequence != sequence+1 || e.Identity.RunID != s.RunID || e.Identity.AttemptID != attempt.AttemptID || e.Identity.SessionID != attempt.ModelSessionID || e.EmitterID != index.EmitterID || e.ClockDomain != clock || e.Offset == nil || *e.Offset < 0 || !validAnalysisIdentity(e.RequestID) || !allowed[e.Event] || e.Location != "model_proxy" || e.Scope == "" {
			return false
		}
		if !validModelReplayPolicy(e.Mode, e.ReplayPolicy) || e.ReplayPolicy != index.ReplayPolicy || !validToolContentDifferences(e) {
			return false
		}
		if attempt.ModelMode != "" && e.Mode != attempt.ModelMode {
			return false
		}
		sequence = e.Sequence
		coverage.ObservedEvents++
		identity := AnalysisIdentity{AttemptID: attempt.AttemptID, ModelSessionID: attempt.ModelSessionID, ModelRequestInstanceID: e.RequestID, Identity: Identity{OccurrenceID: attempt.OccurrenceID}}
		attributes := map[string]any{"mode": e.Mode, "timing_source": e.Timing, "turn": e.Turn, "frame": e.Frame, "status": e.Status, "http_status": e.HTTPStatus, "cached_retry": e.CachedRetry, "producer_location": e.Location, "route": e.Route}
		attributes["replay_policy"] = observedReplayPolicy(e.Mode, e.ReplayPolicy)
		if e.Branch != "" {
			if !validAnalysisIdentity(e.Branch) {
				return false
			}
			attributes["model_branch"] = e.Branch
		}
		if len(e.ToolContentDifferences) > 0 {
			attributes["tool_content_differences"] = e.ToolContentDifferences
		}
		attributes["reason"] = modelBoundaryReason(e.Reason)
		attributes["target_offset_ns"], attributes["actual_offset_ns"], attributes["received_offset_ns"] = e.Target, e.Actual, e.Received
		point := analysisEvent(ref, e.Sequence, e.Event, "model", clock, e.Scope, *e.Offset, identity, attributes)
		r := requests[e.RequestID]
		if r == nil {
			r = &request{points: make(map[string]AnalysisEvent), lastOffset: -1}
			requests[e.RequestID] = r
		}
		if r.finish != nil || point.OffsetNS < r.lastOffset {
			return false
		}
		r.lastOffset = point.OffsetNS
		if e.Event == "http_request_received" {
			if r.start != nil || len(r.points) != 0 {
				return false
			}
			r.start = &point
			coverage.Started++
		} else if r.start == nil {
			return false
		}
		if index.AdmissionClosedOffset != nil && e.Event == "request_admitted" && *e.Offset > *index.AdmissionClosedOffset {
			return false
		}
		if e.Reason == "ADMISSION_CLOSED" && (index.AdmissionClosedOffset == nil || *e.Offset < *index.AdmissionClosedOffset) {
			return false
		}
		// Repeated frame events are distinguished by their real per-request
		// frame index, not merely by the writer's global event sequence.
		switch e.Event {
		case "upstream_json_received", "json_flushed", "json_flush_failed":
			if e.Frame != nil || r.receivedFrames != 0 || r.flushedFrames != 0 || r.flushFailed {
				return false
			}
			if e.Event == "json_flush_failed" {
				r.flushFailed = true
			}
			if e.Event == "json_flushed" && (e.Actual == nil || *e.Actual < 0 || (e.Received != nil && *e.Actual < *e.Received) || (e.Target != nil && *e.Actual < *e.Target)) {
				return false
			}
		case "upstream_frame_received":
			if e.Frame == nil || *e.Frame != r.receivedFrames || r.flushFailed {
				return false
			}
			r.receivedFrames++
		case "frame_flushed":
			if e.Frame == nil || *e.Frame != r.flushedFrames || r.flushFailed {
				return false
			}
			r.flushedFrames++
		case "frame_flush_failed":
			if e.Frame == nil || *e.Frame != r.flushedFrames {
				return false
			}
			r.flushFailed = true
		case "upstream_first_sse", "upstream_first_content", "upstream_first_reasoning", "upstream_first_tool_call_delta":
			if e.Frame == nil || *e.Frame < 0 || *e.Frame != r.receivedFrames-1 || r.flushFailed || (e.Event == "upstream_first_sse" && *e.Frame != 0) {
				return false
			}
		case "downstream_first_sse", "downstream_first_content", "downstream_first_reasoning", "downstream_first_tool_call_delta":
			if e.Frame == nil || *e.Frame < 0 || *e.Frame != r.flushedFrames-1 || r.flushFailed || (e.Event == "downstream_first_sse" && *e.Frame != 0) {
				return false
			}
		}
		if e.Event != "frame_flushed" && e.Event != "upstream_frame_received" {
			if _, exists := r.points[e.Event]; exists {
				return false
			}
			r.points[e.Event] = point
		}
		if e.Event == "http_request_finished" {
			if e.Duration == nil || *e.Duration < 0 || *e.Duration != point.OffsetNS-r.start.OffsetNS || e.Status == "" {
				return false
			}
			r.finish = &point
			coverage.Paired++
			s.interval(stableAnalysisID("operation", ref, e.RequestID), "model.http_request", "model", "model_proxy_handler", e.Status, *r.start, point, attributes)
		}
		s.Events = append(s.Events, point)
		return true
	})
	valid = valid && coverage.ObservedEvents == index.Written && coverage.Started == index.RequestsReceived && coverage.Paired == index.RequestsFinished
	if !valid {
		s.Events, s.Intervals = s.Events[:firstEvent], s.Intervals[:firstInterval]
		coverage.Missing = append(coverage.Missing, "invalid_request_identity_sequence_or_terminal")
		s.problem("model request boundary pairs/counts invalid for " + attempt.AttemptID)
	} else {
		coverage.Integrity, coverage.Status = "complete", "complete"
		if index.AdmissionClosedOffset != nil {
			identity := AnalysisIdentity{AttemptID: attempt.AttemptID, ModelSessionID: attempt.ModelSessionID, Identity: Identity{OccurrenceID: attempt.OccurrenceID}}
			s.Events = append(s.Events, analysisEvent(indexRef, 0, "model.admission_closed", "model", clock, "model_request_admission", *index.AdmissionClosedOffset, identity, nil))
		}
		for _, id := range sortedKeys(requests) {
			r := requests[id]
			for _, phase := range [][3]string{{"admission", "http_request_received", "request_admitted"}, {"body_read", "request_body_read_started", "request_body_read"}, {"parse", "request_parse_started", "request_parsed"}, {"match", "request_match_started", "request_matched"}, {"upstream_headers", "upstream_request_started", "upstream_response_headers"}} {
				start, hasStart := r.points[phase[1]]
				end, hasEnd := r.points[phase[2]]
				if !hasStart || !hasEnd {
					// A rejected request or /models query was never admitted.
					// For an explicitly started phase, retain its point instead
					// of inventing an endpoint from the HTTP handler terminal.
					if phase[0] != "admission" && hasStart != hasEnd {
						missing := "start"
						point := end
						if hasStart {
							missing, point = "terminal", start
						}
						point.Attributes["phase_coverage_status"] = "incomplete"
						point.Attributes["missing_boundary"] = missing
						coverage.Status = "incomplete"
						coverage.Missing = append(coverage.Missing, "phase_"+phase[0]+"_"+missing)
						s.Diagnostics = append(s.Diagnostics, "model phase "+phase[0]+" missing "+missing+" for "+attempt.AttemptID+" request "+id)
					}
					continue
				}
				child := stableAnalysisID("operation", ref, id, phase[0])
				outcome := "observed"
				if status, ok := end.Attributes["status"].(string); ok && status != "" {
					outcome = status
				}
				s.interval(child, "model."+phase[0], "model", "model_proxy_"+phase[0], outcome, start, end, end.Attributes)
				s.Edges = append(s.Edges, AnalysisEdge{From: stableAnalysisID("operation", ref, id), To: child, Kind: "parent", Correlation: "exact_id", EvidenceRefs: []string{ref}})
			}
		}
	}
	s.Coverage = append(s.Coverage, coverage)
}

func modelBoundaryReason(reason string) string {
	switch reason {
	case "", "HANDLER_INTERRUPTED", "SESSION_CLOSED", "ADMISSION_CLOSED", "UNAUTHORIZED", "UNSUPPORTED", "SESSION_BUSY", "SESSION_FAILED", "INVALID_REQUEST", "CREDENTIAL_IN_PAYLOAD", "REPLAY_DIVERGED", "TURN_LIMIT", "DELIVERY_OR_UPSTREAM_FAILED", "CANCELED_OR_TIMEOUT", "WRITE_OR_FLUSH_FAILED":
		return reason
	case "SSE_UNSUPPORTED_FINISH_REASON", "SSE_READ_INCOMPLETE", "SSE_FRAME_OR_RESPONSE_BOUND", "TAPE_BOUND", "DOWNSTREAM_WRITE_OR_FLUSH_FAILED", "UPSTREAM_TRANSPORT_FAILED", "UPSTREAM_STATUS_OR_PROTOCOL", "UPSTREAM_STREAM_ERROR", "SSE_RESPONSE_INCOMPLETE", "SSE_FINISH_TOOL_MISMATCH", "CREDENTIAL_IN_UPSTREAM_RESPONSE", "RECORD_FAILURE_UNCLASSIFIED":
		return reason
	default:
		return "unrecognized_reason"
	}
}

func (s *AnalysisSnapshot) legacyModel(root string, attempt traceAttempt) {
	file := filepath.Join(attempt.OccurrenceID, "model", "events.jsonl")
	data, err := os.ReadFile(filepath.Join(root, file))
	clock := "model-session/" + attempt.ModelSessionID
	ref := s.source(file, data, clock, "unaligned_session_origin_no_shared_anchor", err)
	coverage := SourceCoverage{Source: "model_legacy", AttemptID: attempt.AttemptID, Integrity: "incomplete", Status: "unknown", Missing: []string{"request_instance_id", "producer_event_count", "epoch_anchor"}}
	if err != nil {
		coverage.Status = "not_collected"
		s.problem("model events missing for " + attempt.AttemptID)
		s.Coverage = append(s.Coverage, coverage)
		return
	}
	s.clock(AnalysisClock{ID: clock, Alignment: "unaligned_no_epoch_anchor", EvidenceRef: ref})
	var pending *AnalysisEvent
	var pendingTurn int
	var previous int64 = -1
	var sessionMode, sessionPolicy string
	started, finished := false, false
	firstEvent, firstInterval := len(s.Events), len(s.Intervals)
	valid := traceLines(data, func(line []byte) bool {
		var e modelTraceEvent
		if json.Unmarshal(line, &e) != nil || e.Identity.RunID != s.RunID || e.Identity.AttemptID != attempt.AttemptID || e.Identity.SessionID != attempt.ModelSessionID || e.Offset == nil || *e.Offset < previous || finished {
			return false
		}
		if !validModelReplayPolicy(e.Mode, e.ReplayPolicy) {
			return false
		}
		if attempt.ModelMode != "" && (e.Mode != attempt.ModelMode || e.ReplayPolicy != attempt.ReplayPolicy) {
			return false
		}
		if started && (e.Mode != sessionMode || e.ReplayPolicy != sessionPolicy) {
			return false
		}
		previous = *e.Offset
		coverage.ObservedEvents++
		identity := AnalysisIdentity{AttemptID: attempt.AttemptID, ModelSessionID: attempt.ModelSessionID, Identity: Identity{OccurrenceID: attempt.OccurrenceID}}
		attributes := map[string]any{"turn": e.Turn, "mode": e.Mode, "timing_source": e.Timing, "status": e.Status}
		attributes["replay_policy"] = observedReplayPolicy(e.Mode, e.ReplayPolicy)
		point := analysisEvent(ref, coverage.ObservedEvents, e.Event, "model", clock, "legacy_model_proxy_observation", *e.Offset, identity, attributes)
		switch e.Event {
		case "session_started":
			if started {
				return false
			}
			started = true
			sessionMode, sessionPolicy = e.Mode, e.ReplayPolicy
		case "session_finished":
			if !started || pending != nil {
				return false
			}
			finished = true
		case "request_started":
			if !started || pending != nil {
				return false
			}
			pending = &point
			pendingTurn = e.Turn
			coverage.Started++
		case "request_finished":
			if pending == nil || e.Turn != pendingTurn {
				return false
			}
			attributes["request_observation_index"], attributes["admission_duration_ns"] = coverage.Started, e.Duration
			s.interval(stableAnalysisID("operation", ref, pending.ID), "model.proxy_observed_request", "model", "legacy_post_match_to_finish", e.Status, *pending, point, attributes)
			s.Intervals[len(s.Intervals)-1].Correlation = "unlinked"
			pending = nil
			coverage.Paired++
		case "request_retry", "request_rejected", "frame_sent", "first_sse", "first_content", "first_reasoning", "first_tool_call_delta":
			attributes["frame"], attributes["target_offset_ns"], attributes["actual_offset_ns"], attributes["received_offset_ns"] = e.Frame, e.Target, e.Actual, e.Received
		default:
			return false
		}
		s.Events = append(s.Events, point)
		return true
	})
	if !valid || !started || !finished {
		s.Events, s.Intervals = s.Events[:firstEvent], s.Intervals[:firstInterval]
		coverage.Status = "incomplete"
		s.problem("model event identity, order or terminal evidence incomplete for " + attempt.AttemptID)
	} else {
		coverage.Integrity = "complete"
	}
	s.Coverage = append(s.Coverage, coverage)
}

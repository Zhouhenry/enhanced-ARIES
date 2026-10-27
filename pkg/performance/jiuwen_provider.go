package performance

import (
	"maps"
	"time"
)

const jiuwenProviderScope = "agent_provider_request_including_transport_consumer_and_cleanup_not_guest_execution"

// JiuwenProviderObservation is the allowlisted attribute of an audited native
// SDK span. Its monotonic clock is independent of both native wall time and ECS.
type JiuwenProviderObservation struct {
	Schema     int                   `json:"schema_version"`
	Invocation string                `json:"invocation_id"`
	Operation  jiuwenRemoteOperation `json:"operation"`
	Clock      jiuwenWorkerClock     `json:"clock"`
	Start      int64                 `json:"start_offset_ns"`
	End        int64                 `json:"end_offset_ns"`
	Outcome    string                `json:"outcome"`
	Phases     []struct {
		Name   string `json:"name"`
		Offset int64  `json:"offset_ns"`
	} `json:"phases"`
}

func (p *JiuwenProviderObservation) Valid() bool {
	if p == nil || p.Schema != 1 || !validAnalysisIdentity(p.Invocation) || p.Start < 0 || p.End < p.Start {
		return false
	}
	c := p.Clock
	if !validAnalysisIdentity(c.Emitter) || c.Domain != "jiuwen-provider/"+c.Emitter || c.Before <= 0 || c.After < c.Before || c.Origin != c.Before+(c.After-c.Before)/2 || c.Unix <= 0 {
		return false
	}
	switch p.Operation.Kind + "/" + p.Operation.Method {
	case "fs/read_file", "fs/read_file_stream", "fs/write_file", "fs/list_files", "fs/list_directories", "fs/search_files", "shell/execute_cmd", "shell/execute_cmd_stream", "shell/execute_cmd_background":
	default:
		return false
	}
	switch p.Outcome {
	case "returned", "error", "cancelled", "consumer_closed":
	default:
		return false
	}
	names := []string{"http_submit", "response_headers", "first_frame", "terminal_frame"}
	if len(p.Phases) > len(names) || p.Outcome == "returned" && len(p.Phases) != len(names) {
		return false
	}
	last := p.Start
	for i, phase := range p.Phases {
		if phase.Name != names[i] || phase.Offset < last || phase.Offset > p.End {
			return false
		}
		last = phase.Offset
	}
	return true
}

func identityOnlyEdge(kind string) bool {
	switch kind {
	case "tool_call_identity", "invocation_identity", "provider_invocation_identity", "provider_measurement_identity", "process_tool_identity", "process_invocation_identity":
		return true
	}
	return false
}

func (s *AnalysisSnapshot) jiuwenProviderSnapshot(a traceAttempt) {
	coverageIndex := -1
	for i, c := range s.Coverage {
		if c.Source == "jiuwen_provider_bridge_identity" && c.AttemptID == a.AttemptID {
			coverageIndex = i
			break
		}
	}
	if coverageIndex < 0 {
		return
	} // Historical runs did not declare this instrumentation.
	bridgeDeclared := false
	expected := uint64(0)
	for _, c := range s.Coverage {
		if c.Source == "jiuwen_remote_bridge" && c.AttemptID == a.AttemptID {
			if c.Integrity != "complete" || c.ExpectedOperations == nil {
				s.problem("declared Jiuwen provider observation lacks a validated Bridge denominator")
				return
			}
			expected = *c.ExpectedOperations
			bridgeDeclared = true
			break
		}
	}
	if !bridgeDeclared {
		s.problem("declared Jiuwen provider observation lacks Bridge evidence")
		return
	}
	coverage := &s.Coverage[coverageIndex]
	coverage.ExpectedOperations = &expected
	coverage.Started = expected
	// Snapshot additions below do not append Coverage, so this reference stays stable.
	bridge := map[string][]AnalysisInterval{}
	for _, interval := range s.Intervals {
		if interval.AttemptID == a.AttemptID && interval.Category == "jiuwen_remote_bridge" {
			inv, _ := interval.Attributes["invocation_id"].(string)
			bridge[inv] = append(bridge[inv], interval)
		}
	}
	timed := uint64(0)
	for _, calls := range bridge {
		timed += uint64(len(calls))
	}
	if timed != expected {
		s.problem("Jiuwen accepted Bridge calls lack timed identities for provider reconciliation")
		return
	}
	seen := map[string]bool{}
	clocks := map[string]jiuwenWorkerClock{}
	for _, span := range s.NativeSpans {
		p := span.Provider
		if span.AttemptID != a.AttemptID || p == nil {
			continue
		}
		if !p.Valid() || seen[p.Invocation] {
			s.problem("Jiuwen provider timing or invocation is invalid/duplicate")
			return
		}
		seen[p.Invocation] = true
		if previous, ok := clocks[p.Clock.Domain]; ok && previous != p.Clock {
			s.problem("Jiuwen provider monotonic clock changed its anchor")
			return
		}
		clocks[p.Clock.Domain] = p.Clock
		origin := time.Unix(0, p.Clock.Unix).UTC()
		if !s.clock(AnalysisClock{ID: p.Clock.Domain, EmitterID: p.Clock.Emitter, OriginUTC: &origin, Alignment: "local_monotonic_anchor", EvidenceRef: span.EvidenceRef}) {
			return
		}
		id := stableAnalysisID("jiuwen-provider", a.AttemptID, p.Invocation)
		attrs := map[string]any{"invocation_id": p.Invocation, "operation_kind": p.Operation.Kind, "operation_method": p.Operation.Method, "native_provider_operation_id": span.ID, "anchor_uncertainty_ns": (p.Clock.After - p.Clock.Before + 1) / 2, "cross_host_alignment": "unverified", "process_completion": "unknown"}
		eventAttrs := maps.Clone(attrs)
		eventAttrs["parent_analysis_operation_id"] = id
		point := func(name string, offset int64) AnalysisEvent {
			e := analysisEvent(span.EvidenceRef, uint64(span.SourceLine), name, "jiuwen_remote_provider", p.Clock.Domain, jiuwenProviderScope, offset, span.AnalysisIdentity, eventAttrs)
			e.ID = stableAnalysisID("jiuwen-provider-event", span.ID, name)
			return e
		}
		start, end := point("operation_started", p.Start), point("operation_finished", p.End)
		s.Events = append(s.Events, start, end)
		for _, phase := range p.Phases {
			s.Events = append(s.Events, point("jiuwen.provider."+phase.Name, phase.Offset))
		}
		outcome := "failed"
		if p.Outcome == "returned" {
			outcome = "succeeded"
		} else if p.Outcome == "cancelled" || p.Outcome == "consumer_closed" {
			outcome = "canceled"
		}
		s.interval(id, "jiuwen.provider.request", "jiuwen_remote_provider", jiuwenProviderScope, outcome, start, end, attrs)
		s.Edges = append(s.Edges, AnalysisEdge{From: span.ID, To: id, Kind: "provider_measurement_identity", Correlation: "exact_id", EvidenceRefs: []string{span.EvidenceRef}})
		matches := bridge[p.Invocation]
		if len(matches) == 0 {
			attrs["bridge_association"] = "not_observed"
			eventAttrs["bridge_association"] = "not_observed"
			continue
		} // A request may fail before Bridge admission.
		if len(matches) != 1 {
			s.problem("Jiuwen provider invocation has ambiguous Bridge records")
			return
		}
		call := matches[0]
		if call.AnalysisIdentity != span.AnalysisIdentity || call.Attributes["operation_kind"] != p.Operation.Kind || call.Attributes["operation_method"] != p.Operation.Method {
			s.problem("Jiuwen provider and Bridge identity or operation differ")
			return
		}
		attrs["bridge_association"] = "exact_id"
		eventAttrs["bridge_association"] = "exact_id"
		s.Edges = append(s.Edges, AnalysisEdge{From: id, To: call.ID, Kind: "provider_invocation_identity", Correlation: "exact_id", EvidenceRefs: append([]string{span.EvidenceRef}, call.EvidenceRefs...)})
		coverage.Paired++
	}
	coverage.ObservedEvents = uint64(len(seen))
	if coverage.Paired != expected {
		s.problem("Jiuwen accepted Bridge calls lack declared provider observation")
		return
	}
	coverage.Integrity = "complete"
}

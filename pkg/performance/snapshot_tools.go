package performance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type toolTraceEvent struct {
	RunID           string   `json:"run_id"`
	SessionKey      string   `json:"session_key"`
	Stream          string   `json:"stream"`
	Phase           string   `json:"phase"`
	ToolCallID      string   `json:"tool_call_id"`
	Name            string   `json:"name"`
	Offset          *int64   `json:"host_observed_offset_ns"`
	NativeTimestamp *float64 `json:"native_timestamp_ms"`
	Sequence        int64    `json:"native_sequence"`
	NativeDuration  *float64 `json:"native_duration_ms"`
	ExitCode        *float64 `json:"exit_code"`
	IsError         *bool    `json:"is_error"`
}

func (s *AnalysisSnapshot) toolSnapshot(root string, attempt traceAttempt) {
	file := filepath.Join(attempt.OccurrenceID, "harness", "native-tool-events.jsonl")
	indexFile := filepath.Join(attempt.OccurrenceID, "harness", "native-tool-events.index.json")
	indexData, indexErr := os.ReadFile(filepath.Join(root, indexFile))
	clock := "native-tool-receive/" + attempt.AttemptID
	indexRef := s.source(indexFile, indexData, clock, "unaligned_collector_monotonic_origin", indexErr)
	coverage := SourceCoverage{Source: "native_tool", AttemptID: attempt.AttemptID, Integrity: "incomplete", Status: "unknown", Missing: []string{"independent_expected_tool_count", "guest_process_exit", "stdout_stderr_drain"}}
	if os.IsNotExist(indexErr) {
		coverage.Status = "not_collected"
		s.Coverage = append(s.Coverage, coverage)
		s.problem("native tool source/index not collected for " + attempt.AttemptID)
		return
	}
	var index struct {
		SchemaVersion    int       `json:"schema_version"`
		RunID            string    `json:"run_id"`
		SessionKey       string    `json:"session_key"`
		Complete         bool      `json:"complete"`
		TerminalObserved bool      `json:"terminal_observed"`
		Events           uint64    `json:"events"`
		Origin           time.Time `json:"host_origin_at_utc"`
		Clock            *struct {
			EmitterID string    `json:"emitter_id"`
			Domain    string    `json:"clock_domain"`
			StartedAt time.Time `json:"started_at_utc"`
		} `json:"clock"`
		OriginOffset *int64 `json:"host_origin_offset_ns"`
	}
	if indexErr != nil || decodeDocument(indexData, &index, "events", "complete", "terminal_observed") != nil || index.SchemaVersion != 1 || !validAnalysisIdentity(index.RunID) || index.SessionKey != attempt.AgentSessionID || !index.Complete || !index.TerminalObserved {
		coverage.Status = "incomplete"
		s.Coverage = append(s.Coverage, coverage)
		s.problem("native tool index incomplete or identity invalid for " + attempt.AttemptID)
		return
	}
	coverage.ExpectedEvents = &index.Events
	var originOffset int64
	if index.Clock != nil || index.OriginOffset != nil {
		if index.Clock == nil || index.OriginOffset == nil || *index.OriginOffset < 0 || index.Clock.Domain == "" || index.Clock.EmitterID == "" || index.Clock.StartedAt.IsZero() {
			coverage.Status = "incomplete"
			s.Coverage = append(s.Coverage, coverage)
			s.problem("native tool shared clock anchor invalid for " + attempt.AttemptID)
			return
		}
		clock = index.Clock.Domain
		originOffset = *index.OriginOffset
		for i := range s.Sources {
			if s.Sources[i].Ref == indexRef {
				s.Sources[i].ClockDomain = clock
				s.Sources[i].Alignment = "explicit_local_clock_anchor"
			}
		}
	}
	resultFile := filepath.Join(attempt.OccurrenceID, "harness", "agent-result.json")
	resultData, resultErr := os.ReadFile(filepath.Join(root, resultFile))
	s.source(resultFile, resultData, "identity_only", "not_a_clock", resultErr)
	var result struct {
		RunID      string `json:"native_run_id"`
		SessionKey string `json:"session_key"`
	}
	if resultErr != nil || decodeDocument(resultData, &result) != nil || result.RunID != index.RunID || result.SessionKey != index.SessionKey {
		coverage.Status = "incomplete"
		s.Coverage = append(s.Coverage, coverage)
		s.problem("native run identity unproven for " + attempt.AttemptID)
		return
	}
	data, err := os.ReadFile(filepath.Join(root, file))
	alignment := "unaligned_collector_monotonic_origin"
	if index.Clock != nil {
		alignment = "explicit_local_clock_anchor"
	}
	ref := s.source(file, data, clock, alignment, err)
	if err != nil {
		coverage.Status = "not_collected"
		s.Coverage = append(s.Coverage, coverage)
		s.problem("native tool events missing for " + attempt.AttemptID)
		return
	}
	clockInfo := AnalysisClock{ID: clock, Alignment: "unaligned_collector_origin", EvidenceRef: indexRef}
	if !index.Origin.IsZero() {
		clockInfo.OriginUTC = &index.Origin
	}
	if index.Clock != nil {
		clockInfo.EmitterID = index.Clock.EmitterID
		clockInfo.OriginUTC = &index.Clock.StartedAt
		clockInfo.Alignment = "local_monotonic_anchor"
	}
	if !s.clock(clockInfo) {
		coverage.Status = "incomplete"
		s.Coverage = append(s.Coverage, coverage)
		return
	}
	starts, ended := make(map[string]AnalysisEvent), make(map[string]bool)
	names := make(map[string]string)
	var previous int64 = -1
	var sequence int64
	firstEvent, firstInterval := len(s.Events), len(s.Intervals)
	valid := traceLines(data, func(line []byte) bool {
		var e toolTraceEvent
		if json.Unmarshal(line, &e) != nil || e.RunID != index.RunID || e.SessionKey != index.SessionKey || e.Offset == nil || *e.Offset < previous || e.Sequence <= sequence || !validAnalysisIdentity(e.ToolCallID) {
			return false
		}
		coverage.ObservedEvents++
		previous, sequence = *e.Offset, e.Sequence
		identity := AnalysisIdentity{AttemptID: attempt.AttemptID, AgentSessionID: e.SessionKey, NativeRunID: e.RunID, ToolCallID: e.ToolCallID, Identity: Identity{OccurrenceID: attempt.OccurrenceID}}
		attributes := map[string]any{"native_sequence": e.Sequence, "native_timestamp_ms": e.NativeTimestamp, "native_clock_domain": "native_runtime_wall_clock", "process_completion": "unknown", "native_duration_ms": e.NativeDuration, "exit_code": e.ExitCode, "is_error": e.IsError}
		if *e.Offset > int64(^uint64(0)>>1)-originOffset {
			return false
		}
		point := analysisEvent(ref, coverage.ObservedEvents, e.Stream+"."+e.Phase, "native_tool", clock, "native_event_consumer_observation", *e.Offset+originOffset, identity, attributes)
		point.Sequence = uint64(e.Sequence)
		if e.Stream == "tool" && e.Phase == "start" {
			if _, exists := starts[e.ToolCallID]; exists || e.Name == "" {
				return false
			}
			starts[e.ToolCallID] = point
			names[e.ToolCallID] = e.Name
			coverage.Started++
		} else if e.Stream == "tool" && e.Phase == "result" {
			start, exists := starts[e.ToolCallID]
			if !exists || ended[e.ToolCallID] || names[e.ToolCallID] != e.Name {
				return false
			}
			ended[e.ToolCallID] = true
			coverage.Paired++
			outcome := "unknown"
			if e.IsError != nil {
				outcome = "succeeded"
				if *e.IsError {
					outcome = "failed"
				}
			}
			s.interval(stableAnalysisID("operation", s.RunID, attempt.AttemptID, index.RunID, e.ToolCallID), "tool.observed."+e.Name, "native_tool", "native_event_consumer_interval", outcome, start, point, attributes)
		} else if e.Stream != "command_output" || e.Phase != "end" {
			return false
		}
		if _, exists := starts[e.ToolCallID]; !exists {
			return false
		}
		s.Events = append(s.Events, point)
		return true
	})
	if !valid || coverage.ObservedEvents != index.Events || len(ended) != len(starts) {
		s.Events, s.Intervals = s.Events[:firstEvent], s.Intervals[:firstInterval]
		coverage.Status = "incomplete"
		s.problem("native tool event pairs/counts invalid for " + attempt.AttemptID)
	} else {
		coverage.Integrity = "complete"
	}
	s.Coverage = append(s.Coverage, coverage)
}

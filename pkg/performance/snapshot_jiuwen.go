package performance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Native wall-clock spans are deliberately separate from monotonic intervals.
// The collector's validated index is the identity source; exporters never read
// the private OTLP payload or repeat its parent/session association.
type AnalysisNativeSpan struct {
	AnalysisIdentity
	Provider          *JiuwenProviderObservation `json:"provider_observation,omitempty"`
	ID                string                     `json:"operation_id"`
	Name              string                     `json:"name"`
	ClockDomain       string                     `json:"clock_domain"`
	TraceID           string                     `json:"native_trace_id"`
	SpanID            string                     `json:"native_span_id"`
	ParentSpanID      string                     `json:"native_parent_span_id,omitempty"`
	ParentStatus      string                     `json:"parent_status"`
	Start             string                     `json:"start_time_unix_nano"`
	End               string                     `json:"end_time_unix_nano"`
	DurationNS        int64                      `json:"wall_duration_ns"`
	StatusCode        int                        `json:"native_status_code"`
	AuthoritativeTool bool                       `json:"authoritative_tool"`
	Complete          bool                       `json:"native_complete"`
	ForcedClose       bool                       `json:"native_forced_close"`
	EvidenceRef       string                     `json:"evidence_ref"`
	SourceLine        int                        `json:"source_line"`
}

type jiuwenSpanIndex struct {
	Schema           int                  `json:"schema_version"`
	ProviderExpected *bool                `json:"provider_observation_expected,omitempty"`
	Format           string               `json:"format"`
	Clock            string               `json:"clock"`
	Enabled          *bool                `json:"enabled"`
	Session          string               `json:"native_session_id"`
	Request          string               `json:"native_request_id"`
	Records          int                  `json:"records_seen"`
	Invalid          int                  `json:"invalid_records"`
	Matched          int                  `json:"matched_spans"`
	Unassociated     int                  `json:"unassociated_spans"`
	Tools            int                  `json:"matched_authoritative_tool_spans"`
	CompleteRoots    int                  `json:"matched_complete_roots"`
	Truncated        bool                 `json:"truncated"`
	Error            string               `json:"error"`
	ProducerAudit    *jiuwenProducerAudit `json:"producer_audit"`
	NativeStop       struct {
		ExitObserved bool `json:"exit_observed"`
	} `json:"native_stop"`
	Files []struct {
		Name     string `json:"name"`
		Size     int    `json:"size"`
		Retained int    `json:"retained_bytes"`
		SHA      string `json:"retained_sha256"`
		Error    string `json:"error"`
	} `json:"files"`
	Spans []struct {
		Provider      *JiuwenProviderObservation `json:"provider_observation,omitempty"`
		File          string                     `json:"file"`
		Line          int                        `json:"line"`
		Trace         string                     `json:"trace_id"`
		Span          string                     `json:"span_id"`
		Parent        string                     `json:"parent_span_id"`
		Start         string                     `json:"start_time_unix_nano"`
		End           string                     `json:"end_time_unix_nano"`
		Session       string                     `json:"native_session_id"`
		Request       string                     `json:"native_request_id"`
		Tool          string                     `json:"tool_call_id"`
		ToolName      string                     `json:"tool_name"`
		Operation     string                     `json:"operation"`
		Status        int                        `json:"native_status_code"`
		Authoritative bool                       `json:"authoritative_tool"`
		Root          bool                       `json:"root"`
		Complete      bool                       `json:"complete"`
		Forced        bool                       `json:"forced_close"`
		Matched       bool                       `json:"matched_attempt"`
	} `json:"spans"`
}

func (s *AnalysisSnapshot) jiuwenSpanSnapshot(root string, attempt traceAttempt) bool {
	file := filepath.Join(attempt.OccurrenceID, "harness", "native-spans-index.json")
	data, err := readIdentityEvidence(root, file)
	profile, _ := readIdentityEvidence(root, "cloud-profile.json")
	var config struct {
		Agent string `json:"agent_type"`
	}
	_ = json.Unmarshal(profile, &config)
	if os.IsNotExist(err) && config.Agent != "jiuwenswarm" {
		return false
	}
	if config.Agent != "" && config.Agent != "jiuwenswarm" {
		s.problem("Jiuwen native span index present for another Harness")
		return false
	}
	if os.IsNotExist(err) && attempt.AgentSessionID == "" {
		return true
	}
	domain := "jiuwen-wall/" + attempt.AttemptID
	ref := s.source(file, data, domain, "unaligned_native_wall_clock", err)
	coverage := SourceCoverage{Source: "jiuwen_native_spans", AttemptID: attempt.AttemptID, Integrity: "incomplete", Status: "unknown",
		Missing: []string{"independent_producer_denominator", "export_failure_counter", "monotonic_timing", "guest_process_exit_and_pipe_drain", "cross_host_clock_alignment"}}
	defer func() { s.Coverage = append(s.Coverage, coverage) }()
	if os.IsNotExist(err) {
		declarationFile := filepath.Join(attempt.OccurrenceID, "harness", "native-events-index.json")
		declaration, declarationErr := readIdentityEvidence(root, declarationFile)
		var source struct {
			Enabled *bool `json:"official_span_collection_enabled"`
		}
		if declarationErr != nil && !os.IsNotExist(declarationErr) || declarationErr == nil && identityJSON(declaration, &source, false) != nil {
			s.problem("Jiuwen native span declaration is unreadable or invalid")
			return true
		}
		if declarationErr == nil {
			s.source(declarationFile, declaration, "identity_only", "not_a_clock", nil)
		}
		if source.Enabled == nil || !*source.Enabled {
			coverage.Integrity, coverage.Status = "complete", "not_collected"
			if source.Enabled != nil {
				coverage.Status = "not_enabled"
			}
			return true
		}
	}
	var index jiuwenSpanIndex
	if err != nil || identityJSON(data, &index, false) != nil || index.Schema != 1 || index.Format != "openjiuwen.single_span_otlp_jsonl.hex_ids.v1" || index.Clock != "native_unix_nanoseconds_unaligned" {
		coverage.Status = "not_collected"
		s.problem("Jiuwen native span index unavailable or invalid for " + attempt.AttemptID)
		return true
	}
	if index.Enabled != nil && !*index.Enabled {
		coverage.Integrity, coverage.Status = "complete", "not_enabled"
		return true
	}
	if index.Session != attempt.AgentSessionID || !validAnalysisIdentity(index.Session) || !validAnalysisIdentity(index.Request) || index.Invalid != 0 || index.Truncated || index.Error != "" || index.Records != len(index.Spans) || index.Matched+index.Unassociated != len(index.Spans) {
		s.problem("Jiuwen native span capture or session identity incomplete for " + attempt.AttemptID)
		return true
	}
	if index.ProviderExpected != nil && *index.ProviderExpected {
		s.Coverage = append(s.Coverage, SourceCoverage{Source: "jiuwen_provider_bridge_identity", AttemptID: attempt.AttemptID, Integrity: "incomplete", Status: "unknown", Missing: []string{"pre_admission_failures_have_no_downstream_denominator", "cross_host_clock_alignment", "guest_process_exit_and_pipe_drain"}})
	}
	files := make(map[string]string)
	for _, f := range index.Files {
		if filepath.Base(f.Name) != f.Name || !strings.HasPrefix(f.Name, "traces-") || !strings.HasSuffix(f.Name, ".jsonl") || files[f.Name] != "" {
			s.problem("invalid Jiuwen native span source name")
			return true
		}
		name := filepath.Join(attempt.OccurrenceID, "harness", "native-spans", f.Name)
		content, readErr := readIdentityEvidence(root, name)
		fileRef := s.source(name, content, domain, "unaligned_native_wall_clock", readErr)
		if readErr != nil || f.Error != "" || f.Retained != len(content) || f.Size != f.Retained || f.SHA != fmt.Sprintf("%x", sha256.Sum256(content)) {
			s.problem("Jiuwen native span source hash or size mismatch")
			return true
		}
		files[f.Name] = fileRef
	}
	spans := []AnalysisNativeSpan{}
	seen := make(map[string]bool)
	matched, tools, roots := 0, 0, 0
	for _, span := range index.Spans {
		start, startErr := strconv.ParseInt(span.Start, 10, 64)
		end, endErr := strconv.ParseInt(span.End, 10, 64)
		key := span.Trace + "/" + span.Span
		if !nativeSpanID(span.Trace, 32) || !nativeSpanID(span.Span, 16) || span.Parent != "" && !nativeSpanID(span.Parent, 16) || seen[key] || files[span.File] == "" || span.Line < 1 || startErr != nil || endErr != nil || start <= 0 || end < start || span.Status < 0 || span.Status > 2 {
			s.problem("Jiuwen native span identity or wall-clock interval invalid")
			return true
		}
		seen[key] = true
		if !span.Matched {
			continue
		}
		if span.Session != "" && span.Session != index.Session || span.Request != "" && span.Request != index.Request || span.Tool != "" && !validAnalysisIdentity(span.Tool) || span.Authoritative && span.Tool == "" {
			s.problem("Jiuwen native span matched a conflicting session or request")
			return true
		}
		if (span.Operation == "aries_provider_request") != (span.Provider != nil) || span.Provider != nil && (!span.Provider.Valid() || span.Authoritative || span.Provider.Outcome == "returned" && span.Status != 1 || span.Provider.Outcome != "returned" && span.Status != 2) {
			s.problem("Jiuwen provider span metadata invalid")
			return true
		}
		matched++
		name := "native.operation"
		if span.Authoritative {
			tools++
			name = "native.tool"
			if nativeLabel(span.ToolName) {
				name += "." + span.ToolName
			}
		} else {
			switch span.Operation {
			case "aries_provider_request":
				name = "jiuwen.provider.request.wall"
			case "chat", "invoke_agent", "execute_tool", "react_step":
				name = "native." + span.Operation
			}
		}
		if span.Root && span.Complete {
			roots++
		}
		spans = append(spans, AnalysisNativeSpan{AnalysisIdentity: AnalysisIdentity{Identity: Identity{OccurrenceID: attempt.OccurrenceID, LogicalTaskID: attempt.LogicalTaskID, ArrivalID: attempt.ArrivalID}, AttemptID: attempt.AttemptID, AgentSessionID: index.Session, RequestInstanceID: index.Request, ToolCallID: span.Tool},
			Provider: span.Provider, ID: stableAnalysisID("native-span", attempt.AttemptID, span.Trace, span.Span), Name: name, ClockDomain: domain,
			TraceID: span.Trace, SpanID: span.Span, ParentSpanID: span.Parent, Start: span.Start, End: span.End, DurationNS: end - start,
			StatusCode: span.Status, AuthoritativeTool: span.Authoritative, Complete: span.Complete, ForcedClose: span.Forced,
			EvidenceRef: files[span.File], SourceLine: span.Line})
	}
	if matched != index.Matched || tools != index.Tools || roots != index.CompleteRoots || matched == 0 {
		s.problem("Jiuwen native span index counts do not reconcile")
		return true
	}
	if index.ProducerAudit != nil {
		if !s.jiuwenProducerSnapshot(root, attempt, index) {
			return true
		}
		coverage.Missing[0] = "unsampled_and_uninstrumented_operations"
	}
	matchedIDs := make(map[string]bool)
	for _, span := range spans {
		matchedIDs[span.TraceID+"/"+span.SpanID] = true
	}
	for i := range spans {
		span := &spans[i]
		span.ParentStatus = "not_collected"
		if span.ParentSpanID == "" {
			span.ParentStatus = "root"
		} else if matchedIDs[span.TraceID+"/"+span.ParentSpanID] {
			span.ParentStatus = "exact_native_parent"
		}
	}
	s.clock(AnalysisClock{ID: domain, Alignment: "unaligned_native_wall_clock", EvidenceRef: ref})
	s.NativeSpans = append(s.NativeSpans, spans...)
	coverage.Integrity = "complete"
	coverage.ObservedEvents = uint64(index.Records)
	coverage.Started, coverage.Paired = uint64(matched), uint64(matched)
	if index.Unassociated > 0 {
		coverage.Missing = append(coverage.Missing, "unassociated_native_spans_excluded")
	}
	s.Limitations = append(s.Limitations, "Jiuwen native spans retain original Unix timestamps and IDs. Wall-clock durations are not monotonic or guest process timings; native roots and export success do not establish zero loss. No controller/native cross-clock latency is calculated.")
	return true
}

func nativeSpanID(value string, length int) bool {
	_, err := hex.DecodeString(value)
	return err == nil && len(value) == length && value == strings.ToLower(value) && strings.Trim(value, "0") != ""
}

func nativeLabel(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, c := range value {
		if c < 'a' || c > 'z' {
			if c < 'A' || c > 'Z' {
				if c < '0' || c > '9' {
					if c != '_' && c != '-' && c != '.' {
						return false
					}
				}
			}
		}
	}
	return true
}

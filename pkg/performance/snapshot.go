package performance

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/hyscale-lab/aries/pkg/core"
)

const AnalysisSchemaVersion = 1
const AnalysisParserVersion = "aries-analysis-v1.17.0-jiuwen-workload-context"

// AnalysisSnapshot is a detached, deterministic projection of immutable evidence.
// Exporters consume this value only; they must not reread or rematch source logs.
type AnalysisSnapshot struct {
	SchemaVersion        int                           `json:"analysis_schema_version"`
	ParserVersion        string                        `json:"parser_version"`
	SnapshotID           string                        `json:"snapshot_id"`
	RunID                string                        `json:"run_id"`
	IntegrityStatus      string                        `json:"integrity_status"`
	CoverageStatus       string                        `json:"coverage_status"`
	Summary              Summary                       `json:"summary"`
	Attempts             []AnalysisAttempt             `json:"attempts"`
	AttemptSummary       *core.AttemptSummary          `json:"attempt_summary,omitempty"`
	Sources              []EvidenceSource              `json:"sources"`
	Clocks               []AnalysisClock               `json:"clocks"`
	Events               []AnalysisEvent               `json:"events"`
	Intervals            []AnalysisInterval            `json:"intervals"`
	Edges                []AnalysisEdge                `json:"edges"`
	Coverage             []SourceCoverage              `json:"coverage"`
	Diagnostics          []string                      `json:"diagnostics"`
	Limitations          []string                      `json:"limitations"`
	UntimedOperations    []AnalysisUntimedOperation    `json:"untimed_operations,omitempty"`
	NativeToolIdentities *AnalysisNativeToolIdentities `json:"native_tool_identities,omitempty"`
	Resources            *AnalysisResources            `json:"resources,omitempty"`
	NativeSpans          []AnalysisNativeSpan          `json:"native_spans,omitempty"`
}

type EvidenceSource struct {
	Ref         string `json:"evidence_ref"`
	File        string `json:"file"`
	SHA256      string `json:"sha256,omitempty"`
	Bytes       int    `json:"bytes"`
	ClockDomain string `json:"clock_domain"`
	Alignment   string `json:"alignment"`
	Status      string `json:"status"`
}

type AnalysisClock struct {
	ID          string     `json:"clock_domain"`
	EmitterID   string     `json:"emitter_id,omitempty"`
	OriginUTC   *time.Time `json:"origin_utc,omitempty"`
	Alignment   string     `json:"alignment"`
	EvidenceRef string     `json:"evidence_ref"`
}

type AnalysisIdentity struct {
	Identity
	AttemptID              string `json:"attempt_id,omitempty"`
	ModelSessionID         string `json:"model_session_id,omitempty"`
	ModelRequestInstanceID string `json:"model_request_instance_id,omitempty"`
	RequestInstanceID      string `json:"request_instance_id,omitempty"`
	GatewayDeliveryID      string `json:"gateway_delivery_id,omitempty"`
	AgentSessionID         string `json:"agent_session_id,omitempty"`
	NativeRunID            string `json:"native_run_id,omitempty"`
	ToolCallID             string `json:"tool_call_id,omitempty"`
}

type AnalysisEvent struct {
	AnalysisIdentity
	ID          string         `json:"event_id"`
	Name        string         `json:"name"`
	Category    string         `json:"category"`
	ClockDomain string         `json:"clock_domain"`
	OffsetNS    int64          `json:"monotonic_offset_ns"`
	Sequence    uint64         `json:"sequence"`
	EvidenceRef string         `json:"evidence_ref"`
	SourceLine  uint64         `json:"source_line"`
	Scope       string         `json:"measurement_scope"`
	Attributes  map[string]any `json:"attributes,omitempty"`
}

type AnalysisInterval struct {
	AnalysisIdentity
	ID           string         `json:"operation_id"`
	Name         string         `json:"name"`
	Category     string         `json:"category"`
	ClockDomain  string         `json:"clock_domain"`
	StartNS      int64          `json:"start_offset_ns"`
	EndNS        int64          `json:"end_offset_ns"`
	DurationNS   int64          `json:"duration_ns"`
	StartEventID string         `json:"start_event_id"`
	EndEventID   string         `json:"end_event_id"`
	EvidenceRefs []string       `json:"evidence_refs"`
	Scope        string         `json:"measurement_scope"`
	Outcome      string         `json:"outcome"`
	Correlation  string         `json:"correlation_method"`
	Attributes   map[string]any `json:"attributes,omitempty"`
}

type AnalysisEdge struct {
	From         string   `json:"from_operation_id"`
	To           string   `json:"to_operation_id"`
	Kind         string   `json:"kind"`
	Correlation  string   `json:"correlation_method"`
	EvidenceRefs []string `json:"evidence_refs"`
}

// ExpectedEvents covers a producer/index count, not the set of all operations
// that should have been instrumented. A missing independent denominator is nil.
type SourceCoverage struct {
	Source             string   `json:"source"`
	AttemptID          string   `json:"attempt_id,omitempty"`
	Integrity          string   `json:"integrity"`
	Status             string   `json:"coverage_status"`
	ObservedEvents     uint64   `json:"observed_events"`
	ExpectedEvents     *uint64  `json:"expected_events"`
	DroppedEvents      *uint64  `json:"dropped_events"`
	WriteFailedEvents  *uint64  `json:"write_failed_events"`
	Started            uint64   `json:"started"`
	Paired             uint64   `json:"paired"`
	ExpectedOperations *uint64  `json:"expected_operations"`
	Missing            []string `json:"missing"`
}

type StageMeasurement struct {
	Name        string `json:"name"`
	DurationNS  *int64 `json:"duration_ns"`
	Status      string `json:"measurement_status"`
	Scope       string `json:"measurement_scope"`
	EvidenceRef string `json:"evidence_ref"`
}

type AnalysisAttempt struct {
	ModelMode      string             `json:"model_mode,omitempty"`
	ReplayPolicy   string             `json:"replay_policy,omitempty"`
	AttemptID      string             `json:"attempt_id"`
	OccurrenceID   string             `json:"occurrence_id"`
	LogicalTaskID  string             `json:"logical_task_id"`
	ArrivalID      string             `json:"arrival_id"`
	Status         string             `json:"status"`
	Executed       bool               `json:"executed"`
	Completed      bool               `json:"completed"`
	Validated      bool               `json:"validated"`
	ArtifactProven bool               `json:"artifact_proven"`
	Stages         []StageMeasurement `json:"stages"`
}

type traceAttempt struct {
	ModelMode      string           `json:"-"`
	ReplayPolicy   string           `json:"-"`
	AttemptID      string           `json:"attempt_id"`
	OccurrenceID   string           `json:"occurrence_id"`
	ModelSessionID string           `json:"model_session_id"`
	AgentSessionID string           `json:"agent_session_id"`
	Status         string           `json:"status"`
	LogicalTaskID  string           `json:"logical_task_id"`
	ArrivalID      string           `json:"arrival_id"`
	Executed       bool             `json:"executed"`
	Completed      bool             `json:"completed"`
	Validated      bool             `json:"validated"`
	ArtifactProven bool             `json:"artifact_proven"`
	QueueDuration  *time.Duration   `json:"queue_duration"`
	Timing         *core.TaskTiming `json:"timing"`
}

func BuildSnapshot(directory string) (AnalysisSnapshot, error) {
	a, runnerErr := analyze(directory)
	s := AnalysisSnapshot{SchemaVersion: AnalysisSchemaVersion, ParserVersion: AnalysisParserVersion,
		RunID: a.summary.RunID, IntegrityStatus: "complete", CoverageStatus: "unknown", Summary: a.summary,
		Attempts: []AnalysisAttempt{}, Sources: []EvidenceSource{}, Clocks: []AnalysisClock{}, Events: []AnalysisEvent{}, Intervals: []AnalysisInterval{},
		Edges: []AnalysisEdge{}, Coverage: []SourceCoverage{}, Diagnostics: []string{},
		Limitations: []string{"Coverage and recording integrity are independent of task correctness.",
			"No global critical path, network latency, model compute time or process completion is inferred.",
			"Independent zero origins are not simultaneous; unaligned domains require separate time views.",
			"An unknown expected-operation denominator cannot prove that entire missing event pairs were captured."}}
	s.runnerSnapshot(a, runnerErr)
	root := filepath.Dir(directory)
	data, err := os.ReadFile(filepath.Join(root, "run-result.json"))
	if os.IsNotExist(err) {
		s.Limitations = append(s.Limitations, "run-result.json unavailable; analysis scope is Runner recording only.")
	} else {
		ref := s.source("run-result.json", data, "identity_only", "not_a_clock", err)
		var run struct {
			RunID          string                    `json:"run_id"`
			Attempts       []traceAttempt            `json:"attempts"`
			AttemptSummary *core.AttemptSummary      `json:"attempt_summary"`
			ModelSessions  []core.ModelSessionResult `json:"model_sessions"`
		}
		if err != nil || decodeDocument(data, &run) != nil || run.RunID != s.RunID {
			s.problem("run-result identity or JSON invalid; auxiliary sources withheld")
		} else {
			s.AttemptSummary = run.AttemptSummary
			seen := make(map[string]bool)
			for _, attempt := range run.Attempts {
				id := attempt.OccurrenceID
				if id == "" || id == "." || id == ".." || filepath.Base(id) != id || strings.ContainsAny(id, `/\`) || attempt.AttemptID != run.RunID+"/"+id || seen[id] {
					s.problem("invalid or duplicate attempt identity; auxiliary source withheld")
					continue
				}
				seen[id] = true
				modelFound := false
				for _, session := range run.ModelSessions {
					if session.AttemptID != attempt.AttemptID {
						continue
					}
					if modelFound || session.SessionID != attempt.ModelSessionID || !validModelReplayPolicy(session.Mode, session.ReplayPolicy) {
						s.problem("model session identity or replay policy invalid for " + attempt.AttemptID)
						continue
					}
					modelFound = true
					attempt.ModelMode, attempt.ReplayPolicy = session.Mode, session.ReplayPolicy
				}
				if run.ModelSessions != nil && attempt.ModelSessionID != "" && !modelFound {
					s.problem("explicit model session list missing attempt " + attempt.AttemptID)
				}
				s.attempt(attempt, ref)
				if attempt.ModelSessionID != "" {
					s.modelSnapshot(root, attempt)
				}
				jiuwen := s.jiuwenSpanSnapshot(root, attempt)
				s.jiuwenRemoteSnapshot(root, attempt)
				s.jiuwenNativeToolLinks(attempt)
				s.jiuwenProviderSnapshot(attempt)
				s.jiuwenProcessSnapshot(root, attempt)
				if attempt.AgentSessionID != "" && !jiuwen {
					s.toolSnapshot(root, attempt)
				}
				s.nativeProcessSnapshot(root, attempt)
				operations := s.sdkOperationSnapshot(root, attempt)
				s.nativeIdentitySnapshot(root, attempt, operations)
				s.replayToolProtocolSnapshot(root, attempt)
			}
			s.resourceSnapshot(root)
		}
	}
	for _, coverage := range s.Coverage {
		if coverage.Integrity != "complete" {
			s.IntegrityStatus = "incomplete"
		}
		if coverage.Status == "incomplete" || coverage.Status == "not_collected" {
			s.CoverageStatus = "incomplete"
		}
	}
	s.gatewayIntervals()
	data, _ = json.Marshal(s)
	s.SnapshotID = stableAnalysisID("snapshot", string(data))
	if s.IntegrityStatus != "complete" {
		return s, errors.New("analysis evidence is incomplete; inspect diagnostics and coverage")
	}
	return s, nil
}

func (s *AnalysisSnapshot) attempt(a traceAttempt, ref string) {
	result := AnalysisAttempt{AttemptID: a.AttemptID, OccurrenceID: a.OccurrenceID, LogicalTaskID: a.LogicalTaskID, ArrivalID: a.ArrivalID, Status: a.Status, Executed: a.Executed, Completed: a.Completed, Validated: a.Validated, ArtifactProven: a.ArtifactProven, Stages: []StageMeasurement{}}
	result.ModelMode = a.ModelMode
	if a.ModelMode != "" {
		result.ReplayPolicy = observedReplayPolicy(a.ModelMode, a.ReplayPolicy)
	}
	timing := a.Timing
	if timing == nil {
		timing = &core.TaskTiming{}
	}
	for _, stage := range []struct {
		name, scope string
		duration    *time.Duration
	}{
		{"queue", "admission_queue", a.QueueDuration}, {"sandbox_create", "task_sandbox_create", timing.SandboxCreateDuration},
		{"workspace_prepare", "workspace_prepare", timing.SandboxPrepareDuration},
		{"bridge_prepare", "tool_bridge_start", timing.BridgePrepareDuration},
		{"harness_start", "harness_start", timing.HarnessStartDuration},
		{"runtime_execution", "runtime_execution", timing.RuntimeExecutionDuration}, {"stop_and_revoke", "harness_stop_and_bridge_revoke", timing.StopRevokeDuration},
		{"verifier", "independent_verification_including_candidate_capture", timing.VerifierDuration}, {"cleanup", "task_cleanup", timing.CleanupDuration},
	} {
		measurement := StageMeasurement{Name: stage.name, Scope: stage.scope, EvidenceRef: ref, Status: "not_collected"}
		if stage.duration != nil {
			value := int64(*stage.duration)
			if value < 0 {
				s.problem("negative stage duration in run-result")
			} else {
				measurement.DurationNS = &value
				measurement.Status = "duration_only"
			}
		}
		result.Stages = append(result.Stages, measurement)
	}
	s.Attempts = append(s.Attempts, result)
}

func stableAnalysisID(kind string, parts ...string) string {
	data, _ := json.Marshal(parts)
	sum := sha256.Sum256(data)
	return kind + "-" + hex.EncodeToString(sum[:])
}

func validAnalysisIdentity(value string) bool {
	return value != "" && len(value) <= 512 && strings.IndexFunc(value, unicode.IsControl) < 0
}

func (s *AnalysisSnapshot) problem(message string) {
	s.IntegrityStatus = "incomplete"
	s.Diagnostics = append(s.Diagnostics, message)
}

func (s *AnalysisSnapshot) source(file string, data []byte, clock, alignment string, err error) string {
	file = filepath.ToSlash(file)
	status, digest := "present", ""
	if err != nil {
		status = "not_collected"
	} else {
		sum := sha256.Sum256(data)
		digest = hex.EncodeToString(sum[:])
	}
	ref := stableAnalysisID("source", file, digest)
	s.Sources = append(s.Sources, EvidenceSource{Ref: ref, File: file, SHA256: digest, Bytes: len(data), ClockDomain: clock, Alignment: alignment, Status: status})
	return ref
}

func (s *AnalysisSnapshot) clock(clock AnalysisClock) bool {
	for _, existing := range s.Clocks {
		if existing.ID == clock.ID {
			if existing.EmitterID != clock.EmitterID || existing.OriginUTC != nil && clock.OriginUTC != nil && !existing.OriginUTC.Equal(*clock.OriginUTC) {
				s.problem("clock identity has conflicting anchors")
				return false
			}
			return true
		}
	}
	s.Clocks = append(s.Clocks, clock)
	return true
}

func analysisEvent(ref string, line uint64, name, category, clock, scope string, offset int64, identity AnalysisIdentity, attributes map[string]any) AnalysisEvent {
	return AnalysisEvent{AnalysisIdentity: identity, ID: stableAnalysisID("event", ref, fmt.Sprint(line)), Name: name, Category: category,
		ClockDomain: clock, OffsetNS: offset, Sequence: line, EvidenceRef: ref, SourceLine: line, Scope: scope, Attributes: attributes}
}

func (s *AnalysisSnapshot) interval(id, name, category, scope, outcome string, start, end AnalysisEvent, attributes map[string]any) {
	if start.ClockDomain != end.ClockDomain || end.OffsetNS < start.OffsetNS {
		s.problem("interval boundaries have incompatible clocks or order")
		return
	}
	s.Intervals = append(s.Intervals, AnalysisInterval{AnalysisIdentity: start.AnalysisIdentity, ID: id, Name: name, Category: category,
		ClockDomain: start.ClockDomain, StartNS: start.OffsetNS, EndNS: end.OffsetNS, DurationNS: end.OffsetNS - start.OffsetNS,
		StartEventID: start.ID, EndEventID: end.ID, EvidenceRefs: []string{start.EvidenceRef}, Scope: scope, Outcome: outcome, Correlation: "exact_id", Attributes: attributes})
}

func traceLines(data []byte, consume func([]byte) bool) bool {
	if len(data) != 0 && data[len(data)-1] != '\n' {
		return false
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		if !consume(scanner.Bytes()) {
			return false
		}
	}
	return scanner.Err() == nil
}

func (s *AnalysisSnapshot) runnerSnapshot(a analysis, parseErr error) {
	refs := make(map[string]string)
	for _, name := range []string{"manifest.json", "index.json", "runner-events.jsonl"} {
		data, ok := a.sources[name]
		var err error
		if !ok {
			err = os.ErrNotExist
		}
		refs[name] = s.source("performance/"+name, data, a.summary.ClockDomain, "recorder_monotonic_origin", err)
	}
	if data, ok := a.sources["late-after-close.json"]; ok {
		s.source("performance/late-after-close.json", data, a.summary.ClockDomain, "recorder_monotonic_origin", nil)
	}
	coverage := SourceCoverage{Source: "runner", Integrity: a.summary.Status, Status: "unknown", ObservedEvents: a.summary.EventCount, Missing: []string{}}
	if a.indexOK {
		count, dropped, failed := a.index.Attempted, a.index.Dropped, a.index.WriteFailed
		coverage.ExpectedEvents, coverage.DroppedEvents, coverage.WriteFailedEvents = &count, &dropped, &failed
	}
	for _, record := range a.operations {
		if record.start != nil {
			coverage.Started++
		}
		if record.start != nil && record.end != nil && !record.invalid {
			coverage.Paired++
		}
	}
	if parseErr != nil {
		coverage.Status = "incomplete"
		coverage.Missing = append(coverage.Missing, "unreconciled_runner_recording")
		s.problem("runner recording did not reconcile; intervals withheld; inspect summary.problems")
		s.Coverage = append(s.Coverage, coverage)
		return
	}
	s.Coverage = append(s.Coverage, coverage)
	origin := a.manifest.StartedAtUTC
	s.clock(AnalysisClock{ID: a.summary.ClockDomain, EmitterID: a.summary.EmitterID, OriginUTC: &origin, Alignment: "local_monotonic_anchor", EvidenceRef: refs["manifest.json"]})
	points := make(map[uint64]AnalysisEvent)
	for _, e := range a.events {
		identity := AnalysisIdentity{Identity: e.Identity, RequestInstanceID: e.RequestInstanceID, GatewayDeliveryID: e.GatewayDeliveryID, NativeRunID: e.NativeRunID, AgentSessionID: e.AgentSessionID, ToolCallID: e.ToolCallID}
		if e.OccurrenceID != "" {
			identity.AttemptID = e.RunID + "/" + e.OccurrenceID
		}
		attributes := map[string]any{"run_id": e.RunID, "status": e.Status, "operation_id": e.OperationID, "parent_operation_id": e.ParentOperationID, "error_class": e.ErrorClass}
		attributes["source_component"], attributes["native_sequence"] = e.SourceComponent, e.NativeSequence
		name, category := e.EventType, "scheduler"
		if strings.HasPrefix(e.EventType, "operation_") {
			category = "runner"
		}
		if e.EventType == "observation" {
			name, category = e.Operation, "observation"
		}
		scope := "controller_observation"
		if e.MeasurementScope != "" {
			scope = e.MeasurementScope
		}
		point := analysisEvent(refs["runner-events.jsonl"], e.Sequence, name, category, e.ClockDomain, scope, e.ObservedOffsetNS, identity, attributes)
		s.Events = append(s.Events, point)
		points[e.Sequence] = point
	}
	for _, id := range sortedKeys(a.operations) {
		r := a.operations[id]
		if r.start == nil || r.end == nil || r.invalid {
			continue
		}
		start, end := points[r.start.Sequence], points[r.end.Sequence]
		opID := stableAnalysisID("operation", s.RunID, a.summary.ClockDomain, id)
		s.interval(opID, r.start.Operation, "runner", "controller_operation", r.end.Status, start, end, end.Attributes)
		if parent := r.start.ParentOperationID; parent != "" {
			s.Edges = append(s.Edges, AnalysisEdge{From: stableAnalysisID("operation", s.RunID, a.summary.ClockDomain, parent), To: opID, Kind: "parent", Correlation: "exact_id", EvidenceRefs: []string{refs["runner-events.jsonl"]}})
		}
	}
}

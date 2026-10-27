package performance

import (
	"errors"
	"math/big"
	"strconv"
)

const TraceExporterVersion = "aries-chrome-trace-v3.1-identity-links"

// Trace is a display projection of AnalysisSnapshot, never a second parser.
type Trace struct {
	Events          []TraceEvent  `json:"traceEvents"`
	DisplayTimeUnit string        `json:"displayTimeUnit"`
	Metadata        TraceMetadata `json:"metadata"`
}

type TraceEvent struct {
	Name         string         `json:"name"`
	Category     string         `json:"cat,omitempty"`
	Phase        string         `json:"ph"`
	PID          int            `json:"pid"`
	TID          int            `json:"tid"`
	Timestamp    float64        `json:"ts"`
	Duration     *float64       `json:"dur,omitempty"`
	Scope        string         `json:"s,omitempty"`
	ID           string         `json:"id,omitempty"`
	BindingPoint string         `json:"bp,omitempty"`
	Args         map[string]any `json:"args,omitempty"`
}

type TraceMetadata struct {
	SchemaVersion           int              `json:"schema_version"`
	ExporterVersion         string           `json:"exporter_version"`
	RunID                   string           `json:"run_id"`
	Status                  string           `json:"status"`
	Diagnostics             []string         `json:"diagnostics"`
	Limitations             []string         `json:"limitations"`
	Sources                 []TraceSource    `json:"sources"`
	AnalysisSchemaVersion   int              `json:"analysis_schema_version"`
	SnapshotID              string           `json:"snapshot_id"`
	CoverageStatus          string           `json:"coverage_status"`
	Coverage                []SourceCoverage `json:"coverage"`
	View                    string           `json:"view"`
	ClockDomain             string           `json:"clock_domain,omitempty"`
	TimeUnit                string           `json:"timestamp_unit"`
	OriginalTimeUnit        string           `json:"original_timestamp_unit"`
	MaxProjectionErrorNS    float64          `json:"max_projection_error_ns"`
	ExcludedClockDomains    []string         `json:"excluded_clock_domains"`
	WallClockOriginUnixNano string           `json:"wall_clock_origin_unix_nano,omitempty"`
	IdentityLinks           []AnalysisEdge   `json:"identity_links,omitempty"`
}

type TraceSource struct {
	File        string `json:"file"`
	SHA256      string `json:"sha256"`
	ClockDomain string `json:"clock_domain"`
	Alignment   string `json:"alignment"`
}

// ExportTrace keeps the CLI's purpose while using only the primary Recorder
// domain. Unaligned evidence remains available through explicit domain views.
func ExportTrace(directory string) (Trace, error) {
	snapshot, err := BuildSnapshot(directory)
	trace, projectionErr := ProjectTrace(snapshot)
	return trace, errors.Join(err, projectionErr)
}

func ProjectTrace(snapshot AnalysisSnapshot) (Trace, error) { return ProjectTraceDomain(snapshot, "") }

func ProjectTraceDomain(snapshot AnalysisSnapshot, domain string) (Trace, error) {
	view := "single_clock_domain"
	if domain == "" {
		domain = snapshot.Summary.ClockDomain
		view = "primary_clock_domain"
	}
	t := Trace{Events: []TraceEvent{}, DisplayTimeUnit: "ms", Metadata: TraceMetadata{SchemaVersion: 3, ExporterVersion: TraceExporterVersion, RunID: snapshot.RunID, Status: snapshot.IntegrityStatus,
		Diagnostics: append([]string{}, snapshot.Diagnostics...), Limitations: append([]string{}, snapshot.Limitations...), Sources: []TraceSource{},
		AnalysisSchemaVersion: snapshot.SchemaVersion, SnapshotID: snapshot.SnapshotID, CoverageStatus: snapshot.CoverageStatus, Coverage: snapshot.Coverage,
		View: view, TimeUnit: "microseconds", OriginalTimeUnit: "nanoseconds", ClockDomain: domain, ExcludedClockDomains: []string{}}}
	if err := ValidateSnapshot(snapshot); err != nil {
		t.Metadata.Status = "incomplete"
		t.Metadata.Diagnostics = append(t.Metadata.Diagnostics, "snapshot schema or immutable content identity invalid")
		return t, errors.New("invalid analysis snapshot")
	}
	for _, source := range snapshot.Sources {
		t.Metadata.Sources = append(t.Metadata.Sources, TraceSource{File: source.File, SHA256: source.SHA256, ClockDomain: source.ClockDomain, Alignment: source.Alignment})
	}
	clocks := make(map[string]int)
	for _, clock := range snapshot.Clocks {
		if domain != clock.ID {
			t.Metadata.ExcludedClockDomains = append(t.Metadata.ExcludedClockDomains, clock.ID)
			continue
		}
		pid := len(clocks) + 1
		clocks[clock.ID] = pid
		name := "UNALIGNED | " + clock.ID
		if clock.ID == snapshot.Summary.ClockDomain {
			name = "Runner | " + clock.ID
		}
		t.lane(pid, 0, name, "observed events")
	}
	if len(t.Metadata.ExcludedClockDomains) > 0 {
		t.Metadata.Limitations = append(t.Metadata.Limitations, "Other clock domains are excluded (clock_unaligned); export each explicitly with --trace-clock. They have not silently disappeared from the snapshot.")
	}
	if domain != "" && len(clocks) == 0 {
		t.Metadata.Status = "incomplete"
		t.Metadata.Diagnostics = append(t.Metadata.Diagnostics, "requested clock domain not present")
		return t, errors.New("clock domain not present in snapshot")
	}
	threads := make(map[string]int)
	threadFor := func(clock, key, label string) int {
		key = clock + "/" + key
		if tid := threads[key]; tid != 0 {
			return tid
		}
		tid := len(threads) + 1
		threads[key] = tid
		t.lane(clocks[clock], tid, "", label)
		return tid
	}
	intervals := make(map[string]AnalysisInterval)
	intervalThreads := make(map[string]int)
	identityLinks := make(map[string][]AnalysisEdge)
	visible := make(map[string]bool)
	for _, interval := range snapshot.Intervals {
		visible[interval.ID] = interval.ClockDomain == domain
	}
	for _, span := range snapshot.NativeSpans {
		visible[span.ID] = span.ClockDomain == domain
	}
	for _, edge := range snapshot.Edges {
		if edge.Correlation == "exact_id" && identityOnlyEdge(edge.Kind) {
			identityLinks[edge.From] = append(identityLinks[edge.From], edge)
			identityLinks[edge.To] = append(identityLinks[edge.To], edge)
			if visible[edge.From] || visible[edge.To] {
				t.Metadata.IdentityLinks = append(t.Metadata.IdentityLinks, edge)
			}
		}
	}
	for _, interval := range snapshot.Intervals {
		pid := clocks[interval.ClockDomain]
		if pid == 0 {
			continue
		}
		// Separate operation tracks preserve overlap without inventing a thread stack.
		tid := threadFor(interval.ClockDomain, interval.ID, interval.Name+" | "+interval.AttemptID)
		args := traceAttributes(interval.AnalysisIdentity, interval.Attributes)
		args["operation_id"], args["clock_domain"], args["duration_ns"] = interval.ID, interval.ClockDomain, interval.DurationNS
		args["measurement_scope"], args["status"], args["correlation_method"] = interval.Scope, interval.Outcome, interval.Correlation
		args["start_event_id"], args["end_event_id"], args["evidence_refs"] = interval.StartEventID, interval.EndEventID, interval.EvidenceRefs
		args["start_offset_ns"], args["end_offset_ns"] = interval.StartNS, interval.EndNS
		traceIdentityLinks(args, identityLinks[interval.ID])
		duration := t.micros(interval.DurationNS)
		t.Events = append(t.Events, TraceEvent{Name: interval.Name, Category: interval.Category, Phase: "X", PID: pid, TID: tid, Timestamp: t.micros(interval.StartNS), Duration: &duration, Args: args})
		intervals[interval.ID], intervalThreads[interval.ID] = interval, tid
	}
	if err := t.nativeSpans(snapshot, domain, clocks, threadFor, identityLinks); err != nil {
		return t, err
	}
	for _, event := range snapshot.Events {
		pid := clocks[event.ClockDomain]
		if pid == 0 {
			continue
		}
		if event.Name == "operation_started" || event.Name == "operation_finished" {
			continue
		}
		tid := 0
		if event.Category != "scheduler" {
			tid = threadFor(event.ClockDomain, "events/"+event.AttemptID+"/"+event.Category, event.Category+" events | "+event.AttemptID)
		}
		args := traceAttributes(event.AnalysisIdentity, event.Attributes)
		args["event_id"], args["clock_domain"], args["monotonic_offset_ns"] = event.ID, event.ClockDomain, event.OffsetNS
		args["measurement_scope"], args["evidence_ref"], args["source_line"] = event.Scope, event.EvidenceRef, event.SourceLine
		t.Events = append(t.Events, TraceEvent{Name: event.Name, Category: event.Category, Phase: "i", PID: pid, TID: tid, Timestamp: t.micros(event.OffsetNS), Scope: "t", Args: args})
	}
	for edgeIndex, edge := range snapshot.Edges {
		if identityOnlyEdge(edge.Kind) {
			continue
		}
		from, hasFrom := intervals[edge.From]
		to, hasTo := intervals[edge.To]
		if !hasFrom || !hasTo || from.ClockDomain != to.ClockDomain || edge.Correlation != "exact_id" && edge.Correlation != "verified_mapping" {
			continue
		}
		// Perfetto parses a Chrome flow string ID as a hexadecimal uint64.
		// Use the original snapshot position, not a truncated digest or the
		// filtered view position. Keep the full evidence identity in args.
		id := strconv.FormatUint(uint64(edgeIndex)+1, 16)
		args := map[string]any{"correlation_method": edge.Correlation, "relation": edge.Kind, "evidence_refs": edge.EvidenceRefs,
			"causal_edge_id": stableAnalysisID("flow", edge.From, edge.To, edge.Kind), "from_operation_id": edge.From, "to_operation_id": edge.To}
		t.Events = append(t.Events, TraceEvent{Name: edge.Kind, Category: "causal_reference", Phase: "s", PID: clocks[from.ClockDomain], TID: intervalThreads[from.ID], Timestamp: t.micros(from.StartNS), ID: id, Args: args}, TraceEvent{Name: edge.Kind, Category: "causal_reference", Phase: "f", PID: clocks[to.ClockDomain], TID: intervalThreads[to.ID], Timestamp: t.micros(to.StartNS), ID: id, BindingPoint: "e", Args: args})
	}
	if snapshot.IntegrityStatus != "complete" {
		return t, errors.New("trace evidence is incomplete; inspect metadata.diagnostics and coverage")
	}
	return t, nil
}

// Subtract the native wall origin before converting to Chrome microseconds;
// this avoids rounding epoch nanoseconds and does not create a monotonic clock.
func (t *Trace) nativeSpans(snapshot AnalysisSnapshot, domain string, clocks map[string]int, threadFor func(string, string, string) int, identityLinks map[string][]AnalysisEdge) error {
	var origin int64
	starts := map[string]int64{}
	spans := map[string]AnalysisNativeSpan{}
	for _, span := range snapshot.NativeSpans {
		if span.ClockDomain != domain {
			continue
		}
		start, err := strconv.ParseInt(span.Start, 10, 64)
		end, endErr := strconv.ParseInt(span.End, 10, 64)
		if err != nil || endErr != nil || start <= 0 || end < start || span.DurationNS != end-start {
			return errors.New("invalid native wall-clock span")
		}
		if origin == 0 || start < origin {
			origin = start
		}
		starts[span.ID], spans[span.TraceID+"/"+span.SpanID] = start, span
	}
	if len(starts) == 0 {
		return nil
	}
	t.Metadata.WallClockOriginUnixNano = strconv.FormatInt(origin, 10)
	t.Metadata.OriginalTimeUnit = "unix_nanoseconds"
	t.Metadata.Limitations = append(t.Metadata.Limitations, "This view uses native wall-clock offsets from wall_clock_origin_unix_nano, not monotonic offsets. Wall-clock adjustments and cross-host skew are unmeasured; this view is not aligned to Runner.")
	for i, span := range snapshot.NativeSpans {
		start, exists := starts[span.ID]
		if !exists {
			continue
		}
		tid := threadFor(domain, span.ID, span.Name+" | "+span.ToolCallID)
		args := traceAttributes(span.AnalysisIdentity, nil)
		args["operation_id"], args["clock_domain"] = span.ID, domain
		args["native_trace_id"], args["native_span_id"], args["native_parent_span_id"] = span.TraceID, span.SpanID, span.ParentSpanID
		args["start_time_unix_nano"], args["end_time_unix_nano"], args["wall_duration_ns"] = span.Start, span.End, span.DurationNS
		args["measurement_scope"], args["parent_status"] = "native_wall_clock_span_not_process_timing", span.ParentStatus
		args["native_status_code"], args["native_complete"], args["native_forced_close"] = span.StatusCode, span.Complete, span.ForcedClose
		args["evidence_ref"], args["source_line"] = span.EvidenceRef, span.SourceLine
		traceIdentityLinks(args, identityLinks[span.ID])
		duration := t.micros(span.DurationNS)
		t.Events = append(t.Events, TraceEvent{Name: span.Name, Category: "native_span", Phase: "X", PID: clocks[domain], TID: tid, Timestamp: t.micros(start - origin), Duration: &duration, Args: args})
		if span.ParentStatus != "exact_native_parent" {
			continue
		}
		parent, exists := spans[span.TraceID+"/"+span.ParentSpanID]
		if !exists {
			return errors.New("native parent mapping missing from snapshot")
		}
		id := strconv.FormatUint(uint64(len(snapshot.Edges)+i+1), 16)
		parentTID := threadFor(domain, parent.ID, parent.Name+" | "+parent.ToolCallID)
		flowArgs := map[string]any{"correlation_method": "exact_native_parent", "from_operation_id": parent.ID, "to_operation_id": span.ID, "native_trace_id": span.TraceID}
		t.Events = append(t.Events, TraceEvent{Name: "native_parent", Category: "causal_reference", Phase: "s", PID: clocks[domain], TID: parentTID, Timestamp: t.micros(starts[parent.ID] - origin), ID: id, Args: flowArgs}, TraceEvent{Name: "native_parent", Category: "causal_reference", Phase: "f", PID: clocks[domain], TID: tid, Timestamp: t.micros(start - origin), ID: id, BindingPoint: "e", Args: flowArgs})
	}
	return nil
}

func traceIdentityLinks(args map[string]any, links []AnalysisEdge) {
	if len(links) > 0 {
		args["identity_links"] = links
		args["identity_link_policy"] = "relation_only_no_time_alignment"
	}
}

func traceAttributes(identity AnalysisIdentity, attributes map[string]any) map[string]any {
	args := make(map[string]any, len(attributes)+10)
	for key, value := range attributes {
		args[key] = value
	}
	args["attempt_id"], args["occurrence_id"], args["logical_task_id"], args["arrival_id"], args["purpose"] = identity.AttemptID, identity.OccurrenceID, identity.LogicalTaskID, identity.ArrivalID, identity.Purpose
	if identity.ModelSessionID != "" {
		args["model_session_id"] = identity.ModelSessionID
	}
	if identity.ModelRequestInstanceID != "" {
		args["model_request_instance_id"] = identity.ModelRequestInstanceID
	}
	if identity.RequestInstanceID != "" {
		args["request_instance_id"] = identity.RequestInstanceID
	}
	if identity.GatewayDeliveryID != "" {
		args["gateway_delivery_id"] = identity.GatewayDeliveryID
	}
	if identity.AgentSessionID != "" {
		args["agent_session_id"] = identity.AgentSessionID
	}
	if identity.NativeRunID != "" {
		args["native_run_id"] = identity.NativeRunID
	}
	if identity.ToolCallID != "" {
		args["tool_call_id"] = identity.ToolCallID
	}
	return args
}

func (t *Trace) lane(pid, tid int, process, thread string) {
	if process != "" {
		t.Events = append(t.Events, TraceEvent{Name: "process_name", Phase: "M", PID: pid, Args: map[string]any{"name": process}})
	}
	t.Events = append(t.Events, TraceEvent{Name: "thread_name", Phase: "M", PID: pid, TID: tid, Args: map[string]any{"name": thread}})
}

func (t *Trace) micros(ns int64) float64 {
	value := float64(ns) / 1000
	exact := new(big.Rat).SetFloat64(value)
	exact.Mul(exact, big.NewRat(1000, 1)).Sub(exact, new(big.Rat).SetInt64(ns))
	errorNS, _ := exact.Float64()
	if errorNS < 0 {
		errorNS = -errorNS
	}
	if errorNS > t.Metadata.MaxProjectionErrorNS {
		t.Metadata.MaxProjectionErrorNS = errorNS
	}
	return value
}

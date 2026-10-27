package performance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const OTLPExporterVersion = "aries-otlp-http-json-v1.3-process-events"

// OTLPExport separates the wire request from its offline projection receipt.
// It does not imply that a backend accepted or stored any spans.
type OTLPExport struct {
	Request  OTLPTraceRequest `json:"request"`
	Manifest OTLPManifest     `json:"manifest"`
}

type OTLPTraceRequest struct {
	ResourceSpans []OTLPResourceSpans `json:"resourceSpans"`
}

type OTLPResourceSpans struct {
	Resource   OTLPResource     `json:"resource"`
	ScopeSpans []OTLPScopeSpans `json:"scopeSpans"`
}

type OTLPResource struct {
	Attributes []OTLPKeyValue `json:"attributes"`
}

type OTLPScopeSpans struct {
	Scope OTLPScope  `json:"scope"`
	Spans []OTLPSpan `json:"spans"`
}

type OTLPScope struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// OTLP encodes IDs as hexadecimal, and 64-bit integer fields as decimal strings.
type OTLPSpan struct {
	TraceID           string          `json:"traceId"`
	SpanID            string          `json:"spanId"`
	ParentSpanID      string          `json:"parentSpanId,omitempty"`
	Name              string          `json:"name"`
	Kind              int             `json:"kind"`
	StartTimeUnixNano string          `json:"startTimeUnixNano"`
	EndTimeUnixNano   string          `json:"endTimeUnixNano"`
	Attributes        []OTLPKeyValue  `json:"attributes"`
	Events            []OTLPSpanEvent `json:"events,omitempty"`
	Links             []OTLPSpanLink  `json:"links,omitempty"`
	Status            OTLPStatus      `json:"status"`
}

type OTLPSpanEvent struct {
	TimeUnixNano string         `json:"timeUnixNano"`
	Name         string         `json:"name"`
	Attributes   []OTLPKeyValue `json:"attributes"`
}

type OTLPSpanLink struct {
	TraceID    string         `json:"traceId"`
	SpanID     string         `json:"spanId"`
	Attributes []OTLPKeyValue `json:"attributes"`
}

type OTLPStatus struct {
	Code int `json:"code"`
}

type OTLPKeyValue struct {
	Key   string       `json:"key"`
	Value OTLPAnyValue `json:"value"`
}

// The projector uses scalar AnyValues. Composite snapshot attributes are kept
// as JSON strings rather than flattened into ambiguous attribute names.
type OTLPAnyValue struct {
	StringValue *string  `json:"stringValue,omitempty"`
	IntValue    *string  `json:"intValue,omitempty"`
	BoolValue   *bool    `json:"boolValue,omitempty"`
	DoubleValue *float64 `json:"doubleValue,omitempty"`
}

type OTLPManifest struct {
	SchemaVersion         int                    `json:"schema_version"`
	ExporterVersion       string                 `json:"exporter_version"`
	AnalysisSchemaVersion int                    `json:"analysis_schema_version"`
	ParserVersion         string                 `json:"parser_version"`
	SnapshotID            string                 `json:"snapshot_id"`
	RunID                 string                 `json:"run_id"`
	Mode                  string                 `json:"mode"`
	IntegrityStatus       string                 `json:"integrity_status"`
	CoverageStatus        string                 `json:"coverage_status"`
	TraceID               string                 `json:"trace_id"`
	SpanCount             int                    `json:"span_count"`
	NativeSpanCount       int                    `json:"native_span_count,omitempty"`
	NativeTraceIDs        []string               `json:"native_trace_ids,omitempty"`
	Sources               []EvidenceSource       `json:"sources"`
	Operations            []OTLPSpanMapping      `json:"operations"`
	ExcludedIntervals     []OTLPExcludedInterval `json:"excluded_intervals"`
	ExcludedEdges         []OTLPExcludedEdge     `json:"excluded_edges"`
	UntimedOperationCount int                    `json:"untimed_operation_count"`
	UnprojectedEventCount int                    `json:"unprojected_event_count"`
	InstantEventCount     int                    `json:"instant_event_count"`
	SpanEventPolicy       string                 `json:"span_event_policy"`
	SpanEventCount        int                    `json:"span_event_count"`
	// Unassociated counts allowlisted instants only; exclusions also count other instants.
	UnassociatedInstantEventCount int            `json:"unassociated_instant_event_count"`
	ExcludedInstantEventCounts    map[string]int `json:"excluded_instant_event_counts"`
	Diagnostics                   []string       `json:"diagnostics"`
	Limitations                   []string       `json:"limitations"`
}

type OTLPSpanMapping struct {
	OperationID       string `json:"operation_id"`
	TraceID           string `json:"trace_id"`
	SpanID            string `json:"span_id"`
	ParentSpanID      string `json:"parent_span_id,omitempty"`
	ClockDomain       string `json:"clock_domain"`
	StartTimeUnixNano string `json:"start_time_unix_nano"`
	EndTimeUnixNano   string `json:"end_time_unix_nano"`
}

type OTLPExcludedInterval struct {
	OperationID string `json:"operation_id"`
	ClockDomain string `json:"clock_domain"`
	Reason      string `json:"reason"`
}

type OTLPExcludedEdge struct {
	Edge   AnalysisEdge `json:"edge"`
	Reason string       `json:"reason"`
}

// ProjectOTLP consumes the same immutable snapshot as Perfetto. It never reads
// evidence files, re-pairs events, recalculates metrics, or invents root spans.
func ProjectOTLP(snapshot AnalysisSnapshot) (OTLPExport, error) {
	if err := ValidateSnapshot(snapshot); err != nil {
		return OTLPExport{}, err
	}
	traceID := otlpID(16, "trace", snapshot.SnapshotID)
	out := OTLPExport{Manifest: OTLPManifest{
		SchemaVersion: 1, ExporterVersion: OTLPExporterVersion, AnalysisSchemaVersion: snapshot.SchemaVersion,
		ParserVersion: snapshot.ParserVersion, SnapshotID: snapshot.SnapshotID, RunID: snapshot.RunID, Mode: "offline",
		IntegrityStatus: snapshot.IntegrityStatus, CoverageStatus: snapshot.CoverageStatus, TraceID: traceID,
		Sources: append([]EvidenceSource{}, snapshot.Sources...), Operations: []OTLPSpanMapping{},
		ExcludedIntervals: []OTLPExcludedInterval{}, ExcludedEdges: []OTLPExcludedEdge{},
		UntimedOperationCount: len(snapshot.UntimedOperations), UnprojectedEventCount: len(snapshot.Events),
		SpanEventPolicy: "jiuwen-chat-provider-process-lifecycle-v3", ExcludedInstantEventCounts: map[string]int{},
		Diagnostics: append([]string{}, snapshot.Diagnostics...), Limitations: append([]string{}, snapshot.Limitations...),
	}}
	out.Manifest.Limitations = append(out.Manifest.Limitations,
		"Offline projection is not backend delivery or query verification; stable IDs do not guarantee exactly-once ingestion.",
		"Only intervals with an explicit local monotonic UTC anchor become spans; separate clock anchors do not establish cross-host alignment.",
		"Only allowlisted Jiuwen chat, provider and process lifecycle instants are span events, at most once per name and exact parent operation; other instants and duration-only operations remain in the snapshot. No time-proximity association is used.")
	clocks := make(map[string]AnalysisClock)
	for _, clock := range snapshot.Clocks {
		clocks[clock.ID] = clock
	}
	sourceHashes := make(map[string]string)
	for _, source := range snapshot.Sources {
		sourceHashes[source.Ref] = source.SHA256
	}
	hashes := func(refs []string) map[string]string {
		result := make(map[string]string, len(refs))
		for _, ref := range refs {
			result[ref] = sourceHashes[ref]
		}
		return result
	}
	spans := []OTLPSpan{}
	positions := make(map[string]int)
	intervals := make(map[string]AnalysisInterval)
	for _, interval := range snapshot.Intervals {
		if _, exists := intervals[interval.ID]; exists || interval.ID == "" {
			return OTLPExport{}, errors.New("OTLP projection requires unique nonempty operation IDs")
		}
		intervals[interval.ID] = interval
		if interval.StartNS < 0 || interval.EndNS < interval.StartNS || interval.DurationNS != interval.EndNS-interval.StartNS {
			return OTLPExport{}, fmt.Errorf("invalid interval timing for %s", interval.ID)
		}
		clock, exists := clocks[interval.ClockDomain]
		if !exists || clock.OriginUTC == nil || clock.OriginUTC.IsZero() || clock.Alignment != "local_monotonic_anchor" {
			out.Manifest.ExcludedIntervals = append(out.Manifest.ExcludedIntervals, OTLPExcludedInterval{interval.ID, interval.ClockDomain, "no_trusted_local_monotonic_utc_anchor"})
			continue
		}
		start, err := otlpEpoch(*clock.OriginUTC, interval.StartNS)
		if err != nil {
			return OTLPExport{}, fmt.Errorf("operation %s start: %w", interval.ID, err)
		}
		end, err := otlpEpoch(*clock.OriginUTC, interval.EndNS)
		if err != nil {
			return OTLPExport{}, fmt.Errorf("operation %s end: %w", interval.ID, err)
		}
		attrs := traceAttributes(interval.AnalysisIdentity, interval.Attributes)
		attrs["operation_id"], attrs["snapshot_id"], attrs["run_id"] = interval.ID, snapshot.SnapshotID, snapshot.RunID
		attrs["clock_domain"], attrs["clock_alignment"] = interval.ClockDomain, clock.Alignment
		attrs["clock_evidence_ref"], attrs["clock_source_sha256"] = clock.EvidenceRef, sourceHashes[clock.EvidenceRef]
		attrs["duration_ns"], attrs["start_offset_ns"], attrs["end_offset_ns"] = interval.DurationNS, interval.StartNS, interval.EndNS
		attrs["start_event_id"], attrs["end_event_id"] = interval.StartEventID, interval.EndEventID
		attrs["measurement_scope"], attrs["outcome"], attrs["correlation_method"] = interval.Scope, interval.Outcome, interval.Correlation
		attrs["category"], attrs["evidence_refs"], attrs["source_sha256"] = interval.Category, interval.EvidenceRefs, hashes(interval.EvidenceRefs)
		attrs["integrity_status"], attrs["coverage_status"] = snapshot.IntegrityStatus, snapshot.CoverageStatus
		attributes, err := otlpAttributes(attrs)
		if err != nil {
			return OTLPExport{}, err
		}
		positions[interval.ID] = len(spans)
		spans = append(spans, OTLPSpan{TraceID: traceID, SpanID: otlpID(8, "span", snapshot.SnapshotID, interval.ID), Name: interval.Name,
			Kind: 1, StartTimeUnixNano: start, EndTimeUnixNano: end, Attributes: attributes, Status: otlpOutcome(interval.Outcome)})
	}
	// Recorder operation IDs are retained in the snapshot separately from analysis IDs.
	// Resolve only that explicit identity, scoped to its original clock domain.
	operationIDs := make(map[[2]string][]string)
	for _, interval := range snapshot.Intervals {
		if id, _ := interval.Attributes["operation_id"].(string); id != "" {
			key := [2]string{interval.ClockDomain, id}
			operationIDs[key] = append(operationIDs[key], interval.ID)
		}
	}
	seenEvents := make(map[[2]string]bool)
	for _, event := range snapshot.Events {
		if event.Name == "operation_started" || event.Name == "operation_finished" {
			continue
		}
		out.Manifest.InstantEventCount++
		providerEvent := otlpProviderEvent(event.Name)
		processEvent := otlpProcessEvent(event.Name)
		if !otlpLifecycleEvent(event.Name) && !providerEvent && !processEvent {
			out.Manifest.ExcludedInstantEventCounts["not_allowlisted"]++
			continue
		}
		parent, _ := event.Attributes["parent_operation_id"].(string)
		candidates := operationIDs[[2]string{event.ClockDomain, parent}]
		if providerEvent || processEvent {
			parent, _ = event.Attributes["parent_analysis_operation_id"].(string)
			candidates = nil
			if interval, exists := intervals[parent]; exists && interval.ClockDomain == event.ClockDomain {
				candidates = []string{parent}
			}
		}
		reason := ""
		var interval AnalysisInterval
		position := 0
		switch {
		case parent == "":
			reason = "missing_parent_operation_id"
		case len(candidates) == 0:
			reason = "parent_interval_not_found_in_clock_domain"
		case len(candidates) != 1:
			reason = "ambiguous_parent_operation_id"
		default:
			interval = intervals[candidates[0]]
			var exported bool
			position, exported = positions[interval.ID]
			switch {
			case !exported:
				reason = "parent_interval_not_exported"
			case !providerEvent && !processEvent && (interval.Name != "jiuwen.chat" || interval.Correlation != "exact_id"):
				reason = "parent_not_exact_chat_interval"
			case providerEvent && (interval.Name != "jiuwen.provider.request" || interval.Category != "jiuwen_remote_provider" || event.Category != interval.Category || interval.Correlation != "exact_id"):
				reason = "parent_not_exact_provider_interval"
			case providerEvent && event.AnalysisIdentity != interval.AnalysisIdentity:
				reason = "provider_identity_mismatch"
			case processEvent && (interval.Name != "jiuwen.process.observation" || interval.Category != "jiuwen_process" || event.Category != interval.Category || interval.Correlation != "exact_id"):
				reason = "parent_not_exact_process_observation"
			case processEvent && event.AnalysisIdentity != interval.AnalysisIdentity:
				reason = "process_identity_mismatch"
			case event.Identity != interval.Identity || event.AttemptID != interval.AttemptID:
				reason = "attempt_identity_mismatch"
			case event.ID == "":
				reason = "missing_event_id"
			case event.OffsetNS < interval.StartNS || event.OffsetNS > interval.EndNS:
				reason = "event_outside_parent_interval"
			case seenEvents[[2]string{interval.ID, event.Name}]:
				reason = "duplicate_lifecycle_event"
			}
		}
		if reason != "" {
			out.Manifest.UnassociatedInstantEventCount++
			out.Manifest.ExcludedInstantEventCounts[reason]++
			continue
		}
		// The exported parent guarantees a trusted local anchor in this same domain.
		timestamp, err := otlpEpoch(*clocks[event.ClockDomain].OriginUTC, event.OffsetNS)
		if err != nil {
			return OTLPExport{}, fmt.Errorf("event %s: %w", event.ID, err)
		}
		attrs := traceAttributes(event.AnalysisIdentity, event.Attributes)
		attrs["event_id"], attrs["parent_analysis_operation_id"] = event.ID, interval.ID
		attrs["clock_domain"], attrs["monotonic_offset_ns"] = event.ClockDomain, event.OffsetNS
		attrs["measurement_scope"], attrs["category"], attrs["correlation_method"] = event.Scope, event.Category, "exact_id"
		attrs["evidence_ref"], attrs["source_line"], attrs["source_sha256"] = event.EvidenceRef, event.SourceLine, sourceHashes[event.EvidenceRef]
		attributes, err := otlpAttributes(attrs)
		if err != nil {
			return OTLPExport{}, err
		}
		spans[position].Events = append(spans[position].Events, OTLPSpanEvent{TimeUnixNano: timestamp, Name: event.Name, Attributes: attributes})
		seenEvents[[2]string{interval.ID, event.Name}] = true
		out.Manifest.SpanEventCount++
		out.Manifest.UnprojectedEventCount--
	}
	parents := make(map[string]string)
	native := make(map[string]AnalysisNativeSpan)
	for _, span := range snapshot.NativeSpans {
		native[span.ID] = span
	}
	for _, edge := range snapshot.Edges {
		from, hasFrom := positions[edge.From]
		to, hasTo := positions[edge.To]
		nativeFrom, hasNativeFrom := native[edge.From]
		reason := ""
		if edge.Correlation != "exact_id" && edge.Correlation != "verified_mapping" {
			reason = "unverified_correlation"
		}
		if (!hasFrom && !hasNativeFrom) || !hasTo {
			reason = "endpoint_not_exported"
		}
		if reason != "" {
			out.Manifest.ExcludedEdges = append(out.Manifest.ExcludedEdges, OTLPExcludedEdge{edge, reason})
			continue
		}
		attrs, err := otlpAttributes(map[string]any{"edge_kind": edge.Kind, "correlation_method": edge.Correlation,
			"from_operation_id": edge.From, "to_operation_id": edge.To, "evidence_refs": edge.EvidenceRefs, "source_sha256": hashes(edge.EvidenceRefs)})
		if err != nil {
			return OTLPExport{}, err
		}
		if hasFrom && edge.Kind == "parent" && edge.Correlation == "exact_id" && intervals[edge.From].ClockDomain == intervals[edge.To].ClockDomain {
			if previous, exists := parents[edge.To]; exists && previous != edge.From {
				return OTLPExport{}, errors.New("conflicting exact span parents")
			}
			for ancestor := edge.From; ancestor != ""; ancestor = parents[ancestor] {
				if ancestor == edge.To {
					return OTLPExport{}, errors.New("cyclic exact span parents")
				}
			}
			parents[edge.To] = edge.From
			spans[to].ParentSpanID = spans[from].SpanID
			// Preserve the parent evidence, which the OTLP parentSpanId alone cannot carry.
			parentAttrs, err := otlpAttributes(map[string]any{"parent_analysis_operation_id": edge.From, "parent_edge_kind": edge.Kind, "parent_correlation_method": edge.Correlation,
				"parent_evidence_refs": edge.EvidenceRefs, "parent_source_sha256": hashes(edge.EvidenceRefs)})
			if err != nil {
				return OTLPExport{}, err
			}
			if _, duplicate := otlpAttribute(spans[to].Attributes, "aries.parent_analysis_operation_id"); !duplicate {
				spans[to].Attributes = append(spans[to].Attributes, parentAttrs...)
			}
		} else {
			fromTraceID, fromSpanID := traceID, ""
			if hasFrom {
				fromSpanID = spans[from].SpanID
			} else {
				fromTraceID, fromSpanID = nativeFrom.TraceID, nativeFrom.SpanID
			}
			spans[to].Links = append(spans[to].Links, OTLPSpanLink{TraceID: fromTraceID, SpanID: fromSpanID, Attributes: attrs})
		}
	}
	for _, interval := range snapshot.Intervals {
		position, ok := positions[interval.ID]
		if !ok {
			continue
		}
		span := spans[position]
		out.Manifest.Operations = append(out.Manifest.Operations, OTLPSpanMapping{interval.ID, traceID, span.SpanID, span.ParentSpanID,
			interval.ClockDomain, span.StartTimeUnixNano, span.EndTimeUnixNano})
	}
	out.Manifest.SpanCount = len(spans)
	resourceAttrs, err := otlpAttributes(map[string]any{"snapshot_id": snapshot.SnapshotID, "run_id": snapshot.RunID,
		"parser_version": snapshot.ParserVersion, "exporter_version": OTLPExporterVersion, "offline": true,
		"integrity_status": snapshot.IntegrityStatus, "coverage_status": snapshot.CoverageStatus})
	if err != nil {
		return OTLPExport{}, err
	}
	service := "aries-offline"
	resourceAttrs = append(resourceAttrs, OTLPKeyValue{Key: "service.name", Value: OTLPAnyValue{StringValue: &service}})
	out.Request.ResourceSpans = []OTLPResourceSpans{{Resource: OTLPResource{Attributes: resourceAttrs},
		ScopeSpans: []OTLPScopeSpans{{Scope: OTLPScope{Name: "aries.performance", Version: OTLPExporterVersion}, Spans: spans}}}}
	if err := out.nativeSpans(snapshot); err != nil {
		return OTLPExport{}, err
	}
	if snapshot.IntegrityStatus != "complete" {
		return out, errors.New("OTLP evidence is incomplete; inspect manifest diagnostics and snapshot coverage")
	}
	return out, nil
}

func (out *OTLPExport) nativeSpans(snapshot AnalysisSnapshot) error {
	if len(snapshot.NativeSpans) == 0 {
		return nil
	}
	spans := make([]OTLPSpan, 0, len(snapshot.NativeSpans))
	traces := map[string]bool{}
	for _, span := range snapshot.NativeSpans {
		if !nativeSpanID(span.TraceID, 32) || !nativeSpanID(span.SpanID, 16) {
			return errors.New("invalid native OTLP identity")
		}
		attrs := traceAttributes(span.AnalysisIdentity, nil)
		attrs["operation_id"], attrs["clock_domain"], attrs["parent_status"] = span.ID, span.ClockDomain, span.ParentStatus
		attrs["measurement_scope"] = "native_wall_clock_span_not_process_timing"
		attrs["wall_duration_ns"], attrs["authoritative_tool"], attrs["native_complete"], attrs["native_forced_close"] = span.DurationNS, span.AuthoritativeTool, span.Complete, span.ForcedClose
		attrs["evidence_ref"], attrs["source_line"] = span.EvidenceRef, span.SourceLine
		attributes, err := otlpAttributes(attrs)
		if err != nil {
			return err
		}
		spans = append(spans, OTLPSpan{TraceID: span.TraceID, SpanID: span.SpanID, ParentSpanID: span.ParentSpanID, Name: span.Name, Kind: 1, StartTimeUnixNano: span.Start, EndTimeUnixNano: span.End, Attributes: attributes, Status: OTLPStatus{Code: span.StatusCode}})
		out.Manifest.Operations = append(out.Manifest.Operations, OTLPSpanMapping{span.ID, span.TraceID, span.SpanID, span.ParentSpanID, span.ClockDomain, span.Start, span.End})
		if !traces[span.TraceID] {
			out.Manifest.NativeTraceIDs = append(out.Manifest.NativeTraceIDs, span.TraceID)
			traces[span.TraceID] = true
		}
	}
	attributes, err := otlpAttributes(map[string]any{"snapshot_id": snapshot.SnapshotID, "run_id": snapshot.RunID, "parser_version": snapshot.ParserVersion, "offline": true, "alignment": "unaligned_native_wall_clock", "coverage_status": "unknown"})
	if err != nil {
		return err
	}
	service := "jiuwenswarm-native-offline"
	attributes = append(attributes, OTLPKeyValue{Key: "service.name", Value: OTLPAnyValue{StringValue: &service}})
	out.Request.ResourceSpans = append(out.Request.ResourceSpans, OTLPResourceSpans{Resource: OTLPResource{Attributes: attributes}, ScopeSpans: []OTLPScopeSpans{{Scope: OTLPScope{Name: "aries.native_wall_spans", Version: "1"}, Spans: spans}}})
	out.Manifest.NativeSpanCount = len(spans)
	out.Manifest.SpanCount += len(spans)
	out.Manifest.Limitations = append(out.Manifest.Limitations, "Native spans preserve original trace/span/parent IDs and Unix timestamps in separate traces; no controller parent or clock alignment is invented. Native payload and span events are private and are not re-exported; root completion is not a producer loss denominator.")
	return nil
}

func otlpLifecycleEvent(name string) bool {
	switch name {
	case "jiuwen.chat.request_started", "jiuwen.chat.response_headers", "jiuwen.chat.first_sse", "jiuwen.chat.terminal_received", "jiuwen.chat.stream_drained":
		return true
	default:
		return false
	}
}

func otlpProviderEvent(name string) bool {
	switch name {
	case "jiuwen.provider.http_submit", "jiuwen.provider.response_headers", "jiuwen.provider.first_frame", "jiuwen.provider.terminal_frame":
		return true
	default:
		return false
	}
}

func otlpProcessEvent(name string) bool {
	switch name {
	case "jiuwen.process.spawn_requested", "jiuwen.process.connection_made", "jiuwen.process.spawn_return", "jiuwen.process.stdout_first", "jiuwen.process.stderr_first", "jiuwen.process.child_exit", "jiuwen.process.stdout_closed", "jiuwen.process.stderr_closed", "jiuwen.process.spawn_failed":
		return true
	default:
		return false
	}
}

func otlpID(size int, parts ...string) string {
	data, _ := json.Marshal(append([]string{OTLPExporterVersion}, parts...))
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:size])
}

func otlpEpoch(origin time.Time, offset int64) (string, error) {
	const billion = uint64(1_000_000_000)
	maximum := ^uint64(0)
	seconds := origin.Unix()
	if seconds < 0 || offset < 0 || uint64(seconds) > maximum/billion {
		return "", errors.New("epoch outside OTLP uint64 nanoseconds")
	}
	base := uint64(seconds) * billion
	nanos := uint64(origin.Nanosecond())
	if nanos > maximum-base {
		return "", errors.New("epoch nanoseconds overflow")
	}
	base += nanos
	if uint64(offset) > maximum-base {
		return "", errors.New("epoch offset overflow")
	}
	return strconv.FormatUint(base+uint64(offset), 10), nil
}

func otlpOutcome(outcome string) OTLPStatus {
	switch outcome {
	case "succeeded":
		return OTLPStatus{Code: 1}
	case "failed", "canceled", "cancelled", "timeout", "timed_out":
		return OTLPStatus{Code: 2}
	default:
		return OTLPStatus{Code: 0}
	}
}

func otlpAttribute(attributes []OTLPKeyValue, key string) (OTLPAnyValue, bool) {
	for _, attribute := range attributes {
		if attribute.Key == key {
			return attribute.Value, true
		}
	}
	return OTLPAnyValue{}, false
}

func otlpAttributes(attributes map[string]any) ([]OTLPKeyValue, error) {
	keys := make([]string, 0, len(attributes))
	for key := range attributes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]OTLPKeyValue, 0, len(keys))
	for _, key := range keys {
		var value OTLPAnyValue
		switch v := attributes[key].(type) {
		case string:
			value.StringValue = &v
		case bool:
			value.BoolValue = &v
		default:
			data, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("encode OTLP attribute %s: %w", key, err)
			}
			text := string(data)
			// Canonical JSON numbers keep direct and UseNumber-loaded snapshots
			// identical, without rounding int64 nanoseconds through float64.
			if len(text) > 0 && (text[0] == '-' || text[0] >= '0' && text[0] <= '9') {
				if integer, err := strconv.ParseInt(text, 10, 64); err == nil {
					text = strconv.FormatInt(integer, 10)
					value.IntValue = &text
				} else if strings.ContainsAny(text, ".eE") {
					number, err := strconv.ParseFloat(text, 64)
					if err != nil {
						return nil, fmt.Errorf("encode OTLP number %s: %w", key, err)
					}
					value.DoubleValue = &number
				} else {
					value.StringValue = &text
				}
			} else {
				value.StringValue = &text
			}
		}
		result = append(result, OTLPKeyValue{Key: "aries." + key, Value: value})
	}
	return result, nil
}

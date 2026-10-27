package performance

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func sealOTLPFixture(t *testing.T, snapshot AnalysisSnapshot) AnalysisSnapshot {
	t.Helper()
	snapshot.SnapshotID = ""
	content, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.SnapshotID = stableAnalysisID("snapshot", string(content))
	return snapshot
}

func otlpFixture(t *testing.T) AnalysisSnapshot {
	t.Helper()
	origin := time.Date(2026, 10, 25, 1, 2, 3, 123456789, time.UTC)
	identity := AnalysisIdentity{Identity: Identity{OccurrenceID: "case-001", LogicalTaskID: "case"}, AttemptID: "run/case-001", ToolCallID: "tool-1"}
	interval := func(id string, start, end int64, outcome string) AnalysisInterval {
		return AnalysisInterval{AnalysisIdentity: identity, ID: id, Name: id, Category: "test", ClockDomain: "controller",
			StartNS: start, EndNS: end, DurationNS: end - start, StartEventID: id + "/start", EndEventID: id + "/end",
			EvidenceRefs: []string{"events"}, Scope: "client", Outcome: outcome, Correlation: "exact_id"}
	}
	return sealOTLPFixture(t, AnalysisSnapshot{SchemaVersion: AnalysisSchemaVersion, ParserVersion: AnalysisParserVersion,
		RunID: "run", IntegrityStatus: "complete", CoverageStatus: "unknown", Summary: Summary{ClockDomain: "controller"},
		Sources: []EvidenceSource{{Ref: "anchor", File: "manifest.json", SHA256: strings.Repeat("a", 64)},
			{Ref: "events", File: "events.jsonl", SHA256: strings.Repeat("b", 64)}},
		Clocks:    []AnalysisClock{{ID: "controller", OriginUTC: &origin, Alignment: "local_monotonic_anchor", EvidenceRef: "anchor"}},
		Intervals: []AnalysisInterval{interval("parent", 100, 900, "succeeded"), interval("child", 200, 600, "failed"), interval("zero", 700, 700, "exited")},
		Edges: []AnalysisEdge{{From: "parent", To: "child", Kind: "parent", Correlation: "exact_id", EvidenceRefs: []string{"events"}},
			{From: "child", To: "zero", Kind: "tool_execution", Correlation: "verified_mapping", EvidenceRefs: []string{"events"}}}})
}

func otlpSpans(export OTLPExport) []OTLPSpan {
	return export.Request.ResourceSpans[0].ScopeSpans[0].Spans
}

func TestProjectOTLPStableExactTimeIdentityAndCausality(t *testing.T) {
	snapshot := otlpFixture(t)
	snapshot.Intervals[1].Attributes = map[string]any{"large_integer": int64(9007199254740993), "sequence": uint64(23), "ratio": 1.25, "whole_float": float64(3)}
	snapshot = sealOTLPFixture(t, snapshot)
	before, _ := json.Marshal(snapshot)
	first, err := ProjectOTLP(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ProjectOTLP(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("same snapshot changed export")
	}
	after, _ := json.Marshal(snapshot)
	if !bytes.Equal(before, after) {
		t.Fatal("projection mutated snapshot")
	}
	var loaded AnalysisSnapshot
	decoder := json.NewDecoder(bytes.NewReader(before))
	decoder.UseNumber()
	if err := decoder.Decode(&loaded); err != nil {
		t.Fatal(err)
	}
	reloaded, err := ProjectOTLP(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, reloaded) {
		t.Fatal("JSON snapshot reload changed projection")
	}
	spans := otlpSpans(first)
	if len(spans) != 3 || first.Manifest.SpanCount != 3 || first.Manifest.CoverageStatus != "unknown" {
		t.Fatalf("wrong projection: %+v", first.Manifest)
	}
	traceBytes, err := hex.DecodeString(first.Manifest.TraceID)
	if err != nil || len(traceBytes) != 16 {
		t.Fatal("invalid OTLP trace ID")
	}
	for i, span := range spans {
		spanBytes, err := hex.DecodeString(span.SpanID)
		if err != nil || len(spanBytes) != 8 || span.TraceID != first.Manifest.TraceID {
			t.Fatal("invalid OTLP span identity")
		}
		start, _ := strconv.ParseUint(span.StartTimeUnixNano, 10, 64)
		end, _ := strconv.ParseUint(span.EndTimeUnixNano, 10, 64)
		interval := snapshot.Intervals[i]
		if start != uint64(snapshot.Clocks[0].OriginUTC.UnixNano()+interval.StartNS) || end-start != uint64(interval.DurationNS) {
			t.Fatalf("time changed: %+v", span)
		}
		operation, _ := otlpAttribute(span.Attributes, "aries.operation_id")
		if operation.StringValue == nil || *operation.StringValue != interval.ID {
			t.Fatal("operation identity missing")
		}
		sources, _ := otlpAttribute(span.Attributes, "aries.source_sha256")
		if sources.StringValue == nil || !strings.Contains(*sources.StringValue, snapshot.Sources[1].SHA256) {
			t.Fatal("source SHA missing")
		}
	}
	if spans[0].ParentSpanID != "" || spans[1].ParentSpanID != spans[0].SpanID || spans[2].ParentSpanID != "" {
		t.Fatal("invented or missing parent")
	}
	if len(spans[2].Links) != 1 || spans[2].Links[0].SpanID != spans[1].SpanID {
		t.Fatal("causal edge did not become a link")
	}
	kind, _ := otlpAttribute(spans[2].Links[0].Attributes, "aries.edge_kind")
	if kind.StringValue == nil || *kind.StringValue != "tool_execution" {
		t.Fatal("link evidence missing")
	}
	if spans[0].Status.Code != 1 || spans[1].Status.Code != 2 || spans[2].Status.Code != 0 {
		t.Fatal("outcome was inferred or discarded")
	}
	large, _ := otlpAttribute(spans[1].Attributes, "aries.large_integer")
	if large.IntValue == nil || *large.IntValue != "9007199254740993" {
		t.Fatal("integer precision lost")
	}
	if spans[2].StartTimeUnixNano != spans[2].EndTimeUnixNano {
		t.Fatal("zero duration widened")
	}
}

func TestProjectOTLPUnanchoredAndUntimedRemainExplicit(t *testing.T) {
	snapshot := otlpFixture(t)
	snapshot.Clocks = append(snapshot.Clocks, AnalysisClock{ID: "native", Alignment: "unaligned_native_monotonic"})
	native := snapshot.Intervals[0]
	native.ID = "native"
	native.ClockDomain = "native"
	snapshot.Intervals = append(snapshot.Intervals, native)
	snapshot.Edges = append(snapshot.Edges, AnalysisEdge{From: "native", To: "child", Kind: "tool", Correlation: "exact_id"},
		AnalysisEdge{From: "parent", To: "zero", Kind: "guess", Correlation: "time_nearest"})
	snapshot.Events = []AnalysisEvent{{ID: "instant", Name: "first_output", ClockDomain: "controller"}, {ID: "start", Name: "operation_started", ClockDomain: "controller"}}
	snapshot.UntimedOperations = []AnalysisUntimedOperation{{}}
	snapshot = sealOTLPFixture(t, snapshot)
	result, err := ProjectOTLP(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(otlpSpans(result)) != 3 || len(result.Manifest.ExcludedIntervals) != 1 || result.Manifest.ExcludedIntervals[0].OperationID != "native" {
		t.Fatal("unaligned interval was fabricated or silently lost")
	}
	if len(result.Manifest.ExcludedEdges) != 2 || result.Manifest.UntimedOperationCount != 1 || result.Manifest.UnprojectedEventCount != 2 || result.Manifest.InstantEventCount != 1 {
		t.Fatalf("missing exclusions: %+v", result.Manifest)
	}
	// An old collector wall clock is not a verified monotonic anchor.
	snapshot.Clocks[0].Alignment = "unaligned_collector_origin"
	result, err = ProjectOTLP(sealOTLPFixture(t, snapshot))
	if err != nil || result.Manifest.SpanCount != 0 || len(result.Manifest.ExcludedIntervals) != 4 {
		t.Fatal("wall-clock-only source was promoted to anchored spans")
	}
}

func TestProjectOTLPRejectsInvalidSnapshotAndTiming(t *testing.T) {
	t.Run("changed content", func(t *testing.T) {
		s := otlpFixture(t)
		s.Intervals[0].DurationNS++
		if _, err := ProjectOTLP(s); err == nil {
			t.Fatal("tampered snapshot accepted")
		}
	})
	for _, test := range []struct {
		name   string
		change func(*AnalysisSnapshot)
	}{
		{"duration mismatch", func(s *AnalysisSnapshot) { s.Intervals[0].DurationNS++ }},
		{"negative offset", func(s *AnalysisSnapshot) { s.Intervals[0].StartNS = -1 }},
		{"duplicate operation", func(s *AnalysisSnapshot) { s.Intervals[1].ID = s.Intervals[0].ID }},
		{"epoch overflow", func(s *AnalysisSnapshot) {
			origin := time.Date(3000, 1, 1, 0, 0, 0, 0, time.UTC)
			s.Clocks[0].OriginUTC = &origin
		}},
		{"negative epoch", func(s *AnalysisSnapshot) { origin := time.Unix(-1, 0); s.Clocks[0].OriginUTC = &origin }},
		{"parent cycle", func(s *AnalysisSnapshot) {
			s.Edges = append(s.Edges, AnalysisEdge{From: "child", To: "parent", Kind: "parent", Correlation: "exact_id"})
		}},
		{"conflicting parents", func(s *AnalysisSnapshot) {
			s.Edges = append(s.Edges, AnalysisEdge{From: "zero", To: "child", Kind: "parent", Correlation: "exact_id"})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := otlpFixture(t)
			test.change(&s)
			if _, err := ProjectOTLP(sealOTLPFixture(t, s)); err == nil {
				t.Fatal("invalid projection accepted")
			}
		})
	}
}

func TestProjectOTLPIncompleteAndCanceledDoNotBecomeSuccess(t *testing.T) {
	snapshot := otlpFixture(t)
	snapshot.IntegrityStatus = "incomplete"
	snapshot.Diagnostics = []string{"missing terminal event"}
	snapshot.Intervals[0].Outcome = "canceled"
	result, err := ProjectOTLP(sealOTLPFixture(t, snapshot))
	if err == nil || result.Manifest.IntegrityStatus != "incomplete" || result.Manifest.CoverageStatus != "unknown" {
		t.Fatal("incomplete evidence promoted")
	}
	if otlpSpans(result)[0].Status.Code != 2 || !reflect.DeepEqual(result.Manifest.Diagnostics, snapshot.Diagnostics) {
		t.Fatal("cancel or diagnostics lost")
	}
}

func TestProjectOTLPCrossClockRelationIsLinkNotParent(t *testing.T) {
	snapshot := otlpFixture(t)
	clock := snapshot.Clocks[0]
	clock.ID = "other"
	snapshot.Clocks = append(snapshot.Clocks, clock)
	snapshot.Intervals[1].ClockDomain = "other"
	result, err := ProjectOTLP(sealOTLPFixture(t, snapshot))
	if err != nil {
		t.Fatal(err)
	}
	spans := otlpSpans(result)
	if spans[1].ParentSpanID != "" || len(spans[1].Links) != 1 || spans[1].Links[0].SpanID != spans[0].SpanID {
		t.Fatal("cross-clock edge changed into an aligned parent")
	}
}

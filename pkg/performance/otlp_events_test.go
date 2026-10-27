package performance

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

func otlpLifecycleFixture(t *testing.T) AnalysisSnapshot {
	t.Helper()
	s := otlpFixture(t)
	s.Intervals[0].Name = "jiuwen.chat"
	s.Intervals[0].Attributes = map[string]any{"operation_id": "recorder-op-1"}
	for i, name := range []string{"request_started", "response_headers", "first_sse", "terminal_received", "stream_drained"} {
		identity := s.Intervals[0].AnalysisIdentity
		identity.RequestInstanceID, identity.AgentSessionID = "chat-request", "chat-session"
		s.Events = append(s.Events, AnalysisEvent{AnalysisIdentity: identity, ID: "event-" + name,
			Name: "jiuwen.chat." + name, Category: "observation", ClockDomain: "controller", OffsetNS: int64(100 + i*200),
			Sequence: uint64(i + 1), SourceLine: uint64(i + 2), EvidenceRef: "events", Scope: "controller_http_sse_consumption",
			Attributes: map[string]any{"parent_operation_id": "recorder-op-1", "source_component": "jiuwenswarm"}})
	}
	return sealOTLPFixture(t, s)
}

// This regression also runs unchanged against the release14 exporter.
func TestProjectOTLPLifecycleEventsWire(t *testing.T) {
	out, err := ProjectOTLP(otlpLifecycleFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(out.Request)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(wire, []byte(`"events":[`)) {
		t.Fatal("OTLP spans lost the five recorded Jiuwen lifecycle instants")
	}
}

func TestProjectOTLPLifecycleEventsExactIdentityTimeAndReload(t *testing.T) {
	snapshot := otlpLifecycleFixture(t)
	before, _ := json.Marshal(snapshot)
	out, err := ProjectOTLP(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	events := otlpSpans(out)[0].Events
	if len(events) != 5 || out.Manifest.SpanEventCount != 5 || out.Manifest.InstantEventCount != 5 || out.Manifest.UnprojectedEventCount != 0 || out.Manifest.UnassociatedInstantEventCount != 0 || len(out.Manifest.ExcludedInstantEventCounts) != 0 {
		t.Fatalf("wrong event accounting: %+v", out.Manifest)
	}
	for i, event := range events {
		original := snapshot.Events[i]
		want := strconv.FormatInt(snapshot.Clocks[0].OriginUTC.UnixNano()+original.OffsetNS, 10)
		if event.Name != original.Name || event.TimeUnixNano != want {
			t.Fatalf("event timing or order changed: %+v", event)
		}
		for key, want := range map[string]string{"event_id": original.ID, "parent_operation_id": "recorder-op-1", "parent_analysis_operation_id": "parent",
			"request_instance_id": "chat-request", "agent_session_id": "chat-session", "source_sha256": snapshot.Sources[1].SHA256, "correlation_method": "exact_id"} {
			value, ok := otlpAttribute(event.Attributes, "aries."+key)
			if !ok || value.StringValue == nil || *value.StringValue != want {
				t.Fatalf("event %s lost %s", event.Name, key)
			}
		}
		value, _ := otlpAttribute(event.Attributes, "aries.monotonic_offset_ns")
		if value.IntValue == nil || *value.IntValue != strconv.FormatInt(original.OffsetNS, 10) {
			t.Fatal("event nanoseconds lost precision")
		}
	}
	if len(otlpSpans(out)[1].Events) != 0 || len(otlpSpans(out)[2].Events) != 0 {
		t.Fatal("event associated with a nearer, unrelated span")
	}
	after, _ := json.Marshal(snapshot)
	if !bytes.Equal(before, after) {
		t.Fatal("event projection mutated snapshot")
	}
	path := filepath.Join(t.TempDir(), "snapshot.json")
	if err := os.WriteFile(path, before, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := ProjectOTLP(loaded)
	if err != nil || !reflect.DeepEqual(out, reloaded) {
		t.Fatalf("snapshot reload changed event projection: %v", err)
	}
}

func TestProjectOTLPLifecycleEventsExcludeWithoutGuessing(t *testing.T) {
	for _, test := range []struct {
		name, reason string
		change       func(*AnalysisSnapshot)
	}{
		{"missing parent", "missing_parent_operation_id", func(s *AnalysisSnapshot) { delete(s.Events[0].Attributes, "parent_operation_id") }},
		{"unknown parent", "parent_interval_not_found_in_clock_domain", func(s *AnalysisSnapshot) { s.Events[0].Attributes["parent_operation_id"] = "unknown" }},
		{"other clock", "parent_interval_not_found_in_clock_domain", func(s *AnalysisSnapshot) { s.Events[0].ClockDomain = "other" }},
		{"ambiguous parent", "ambiguous_parent_operation_id", func(s *AnalysisSnapshot) { s.Intervals[1].Attributes = s.Intervals[0].Attributes }},
		{"no anchor", "parent_interval_not_exported", func(s *AnalysisSnapshot) { s.Clocks[0].OriginUTC = nil }},
		{"untrusted anchor", "parent_interval_not_exported", func(s *AnalysisSnapshot) { s.Clocks[0].Alignment = "unaligned_collector_origin" }},
		{"wrong operation", "parent_not_exact_chat_interval", func(s *AnalysisSnapshot) { s.Intervals[0].Name = "agent.run" }},
		{"unverified interval", "parent_not_exact_chat_interval", func(s *AnalysisSnapshot) { s.Intervals[0].Correlation = "time_nearest" }},
		{"foreign occurrence", "attempt_identity_mismatch", func(s *AnalysisSnapshot) { s.Events[0].OccurrenceID = "other" }},
		{"foreign attempt", "attempt_identity_mismatch", func(s *AnalysisSnapshot) { s.Events[0].AttemptID = "other" }},
		{"missing event ID", "missing_event_id", func(s *AnalysisSnapshot) { s.Events[0].ID = "" }},
		{"before parent", "event_outside_parent_interval", func(s *AnalysisSnapshot) { s.Events[0].OffsetNS = 99 }},
		{"after parent", "event_outside_parent_interval", func(s *AnalysisSnapshot) { s.Events[0].OffsetNS = 901 }},
		{"negative time", "event_outside_parent_interval", func(s *AnalysisSnapshot) { s.Events[0].OffsetNS = -1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := otlpLifecycleFixture(t)
			s.Events = s.Events[:1]
			test.change(&s)
			out, err := ProjectOTLP(sealOTLPFixture(t, s))
			if err != nil {
				t.Fatal(err)
			}
			if out.Manifest.SpanEventCount != 0 || out.Manifest.UnprojectedEventCount != 1 || out.Manifest.UnassociatedInstantEventCount != 1 || out.Manifest.ExcludedInstantEventCounts[test.reason] != 1 {
				t.Fatalf("invalid association hidden: %+v", out.Manifest)
			}
		})
	}
}

func TestProjectOTLPLifecycleEventsBoundedAndPartial(t *testing.T) {
	s := otlpLifecycleFixture(t)
	s.Events = s.Events[:2] // A failed request has no invented terminal or drain event.
	s.Intervals[0].Outcome = "failed"
	duplicate := s.Events[0]
	duplicate.ID = "duplicate"
	s.Events = append(s.Events, duplicate)
	for _, name := range []string{"jiuwen.chat.token", "jiuwen.chat.frame", "operation_started", "operation_finished"} {
		event := duplicate
		event.ID, event.Name = name, name
		s.Events = append(s.Events, event)
	}
	out, err := ProjectOTLP(sealOTLPFixture(t, s))
	if err != nil {
		t.Fatal(err)
	}
	if len(otlpSpans(out)[0].Events) != 2 || otlpSpans(out)[0].Status.Code != 2 || out.Manifest.InstantEventCount != 5 || out.Manifest.SpanEventCount != 2 || out.Manifest.UnprojectedEventCount != 5 || out.Manifest.UnassociatedInstantEventCount != 1 || out.Manifest.ExcludedInstantEventCounts["duplicate_lifecycle_event"] != 1 || out.Manifest.ExcludedInstantEventCounts["not_allowlisted"] != 2 {
		t.Fatalf("partial or bounded lifecycle semantics changed: %+v", out.Manifest)
	}
	old, err := ProjectOTLP(otlpFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	wire, _ := json.Marshal(old.Request)
	if bytes.Contains(wire, []byte(`"events":`)) {
		t.Fatal("empty events changed legacy span wire output")
	}
}

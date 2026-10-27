package performance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func traceFixture(t *testing.T) (string, string) {
	t.Helper()
	events := summaryPair("operation", "task", "task", "timeout", 6000)
	for i := range events {
		events[i].OccurrenceID = "occ"
	}
	events = append(events, summaryArrival("arrival", "task", "not_started")...)
	directory := summaryRecording(t, events)
	root := t.TempDir()
	if err := os.Rename(directory, filepath.Join(root, "performance")); err != nil {
		t.Fatal(err)
	}
	return root, filepath.Join(root, "performance")
}

func writeTraceEvents(t *testing.T, path string, events []map[string]any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if err := json.NewEncoder(f).Encode(e); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func modelTraceFixture(t *testing.T, root string) []map[string]any {
	t.Helper()
	writeSummaryJSON(t, root, "run-result.json", map[string]any{"run_id": "run", "attempts": []traceAttempt{{AttemptID: "run/occ", OccurrenceID: "occ", ModelSessionID: "model", AgentSessionID: "native-session"}}})
	names := []string{"session_started", "request_started", "first_sse", "request_finished", "request_retry", "request_started", "request_finished", "session_finished"}
	var events []map[string]any
	for i, name := range names {
		events = append(events, map[string]any{"event": name, "identity": map[string]any{"run_id": "run", "attempt_id": "run/occ", "session_id": "model"}, "turn": 0, "observed_offset_ns": i * 1000, "mode": "replay", "timing_source": "recorded", "duration_ns": 9000, "status": "succeeded", "request": "SECRET_REQUEST", "reason": "SECRET_ERROR"})
	}
	events[2]["target_offset_ns"], events[2]["actual_offset_ns"] = 500, 600
	writeTraceEvents(t, filepath.Join(root, "occ", "model", "events.jsonl"), events)
	return events
}

func toolTraceFixture(t *testing.T, root string) []map[string]any {
	t.Helper()
	dir := filepath.Join(root, "occ", "harness")
	var events []map[string]any
	for i, phase := range []string{"start", "result"} {
		events = append(events, map[string]any{"run_id": "native-run", "session_key": "native-session", "stream": "tool", "phase": phase, "tool_call_id": "tool-1", "name": "exec", "native_sequence": i + 1, "host_observed_offset_ns": i * 4000, "native_timestamp_ms": 999999999, "command": "SECRET_COMMAND", "process_completion": "unknown"})
	}
	writeTraceEvents(t, filepath.Join(dir, "native-tool-events.jsonl"), events)
	writeSummaryJSON(t, dir, "native-tool-events.index.json", map[string]any{"schema_version": 1, "run_id": "native-run", "session_key": "native-session", "complete": true, "terminal_observed": true, "events": 2})
	writeSummaryJSON(t, dir, "agent-result.json", map[string]any{"native_run_id": "native-run", "session_key": "native-session", "response": "SECRET_RESPONSE"})
	return events
}

func TestTraceUsesPairedEvidenceAndKeepsClockDomainsSeparate(t *testing.T) {
	root, directory := traceFixture(t)
	modelTraceFixture(t, root)
	toolTraceFixture(t, root)
	trace, err := ExportTrace(directory)
	if err != nil || trace.Metadata.Status != "complete" {
		t.Fatalf("trace=%+v err=%v", trace.Metadata, err)
	}
	counts := make(map[string]int)
	if trace.Metadata.View != "primary_clock_domain" || len(trace.Metadata.ExcludedClockDomains) != 2 {
		t.Fatal("unaligned evidence was not explicitly excluded")
	}
	snapshot, err := BuildSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	allEvents := append([]TraceEvent{}, trace.Events...)
	for _, domain := range trace.Metadata.ExcludedClockDomains {
		view, err := ProjectTraceDomain(snapshot, domain)
		if err != nil {
			t.Fatal(err)
		}
		allEvents = append(allEvents, view.Events...)
	}
	for _, e := range allEvents {
		if e.Phase != "X" {
			continue
		}
		counts[e.Category]++
		switch e.Category {
		case "runner":
			if e.PID != 1 || e.Duration == nil || *e.Duration != 6 || e.Args["status"] != "timeout" || e.Args["attempt_id"] != "run/occ" {
				t.Fatalf("runner evidence lost: %+v", e)
			}
		case "model":
			if e.Duration == nil || *e.Duration == 9 || e.Args["timing_source"] != "recorded" || e.Args["clock_domain"] == snapshot.Summary.ClockDomain {
				t.Fatalf("admission duration used to fabricate placement: %+v", e)
			}
		case "native_tool":
			if e.Duration == nil || *e.Duration != 4 || e.Timestamp != 0 || e.Args["process_completion"] != "unknown" || e.Args["clock_domain"] == snapshot.Summary.ClockDomain {
				t.Fatalf("guest wall clock/process completion confused: %+v", e)
			}
		}
	}
	if counts["runner"] != 1 || counts["model"] != 2 || counts["native_tool"] != 1 {
		t.Fatalf("wrong intervals: %v", counts)
	}
	encoded, err := json.Marshal(trace)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"SECRET_REQUEST", "SECRET_ERROR", "SECRET_COMMAND", "SECRET_RESPONSE"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("trace leaked %s", secret)
		}
	}
	if !strings.Contains(string(encoded), "clock_unaligned") {
		t.Fatal("excluded clock status missing")
	}
	for _, source := range trace.Metadata.Sources {
		if len(source.SHA256) != 64 {
			t.Fatal("source hash missing")
		}
	}
}

func TestTraceRejectsUnreconciledRunnerWithoutInventingIntervals(t *testing.T) {
	_, directory := traceFixture(t)
	if err := os.Remove(filepath.Join(directory, "index.json")); err != nil {
		t.Fatal(err)
	}
	trace, err := ExportTrace(directory)
	if err == nil || trace.Metadata.Status != "incomplete" {
		t.Fatal("missing writer index accepted")
	}
	for _, event := range trace.Events {
		if event.Phase == "X" {
			t.Fatal("invented interval from incomplete source")
		}
	}
}

func TestTraceRejectsForeignSessionsMissingTerminalsAndToolIDs(t *testing.T) {
	for _, mutation := range []string{"model_session", "model_terminal", "model_offset", "tool_id", "native_run", "duplicate_attempt"} {
		t.Run(mutation, func(t *testing.T) {
			root, directory := traceFixture(t)
			models := modelTraceFixture(t, root)
			tools := toolTraceFixture(t, root)
			switch mutation {
			case "model_session":
				models[2]["identity"].(map[string]any)["session_id"] = "foreign"
			case "model_terminal":
				models = models[:len(models)-1]
			case "model_offset":
				delete(models[2], "observed_offset_ns")
			case "tool_id":
				tools[1]["tool_call_id"] = "foreign"
			case "native_run":
				writeSummaryJSON(t, filepath.Join(root, "occ", "harness"), "agent-result.json", map[string]any{"native_run_id": "foreign", "session_key": "native-session"})
			case "duplicate_attempt":
				a := traceAttempt{AttemptID: "run/occ", OccurrenceID: "occ", ModelSessionID: "model"}
				writeSummaryJSON(t, root, "run-result.json", map[string]any{"run_id": "run", "attempts": []traceAttempt{a, a}})
			}
			writeTraceEvents(t, filepath.Join(root, "occ", "model", "events.jsonl"), models)
			writeTraceEvents(t, filepath.Join(root, "occ", "harness", "native-tool-events.jsonl"), tools)
			trace, err := ExportTrace(directory)
			if err == nil || trace.Metadata.Status != "incomplete" || len(trace.Metadata.Diagnostics) == 0 {
				t.Fatalf("%s accepted", mutation)
			}
		})
	}
}

func TestTraceRunnerOnlyKeepsUnstartedArrivals(t *testing.T) {
	_, directory := traceFixture(t)
	trace, err := ExportTrace(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range trace.Events {
		if event.Name == "arrival_not_started" && event.Phase == "i" && event.Args["status"] == "not_started" {
			return
		}
	}
	t.Fatal("unstarted arrival omitted")
}

func TestTraceFlowIDsAreUint64StableAndBoundToTheirSlices(t *testing.T) {
	// Project an already validated snapshot with multiple same-kind edges,
	// a second clock view, and an unaligned relation that must stay unprojected.
	snapshot := AnalysisSnapshot{SchemaVersion: AnalysisSchemaVersion, RunID: "run", IntegrityStatus: "complete", CoverageStatus: "unknown",
		Summary: Summary{ClockDomain: "controller"}, Clocks: []AnalysisClock{{ID: "controller"}, {ID: "other"}}}
	addInterval := func(id, clock string, start, end int64) {
		snapshot.Intervals = append(snapshot.Intervals, AnalysisInterval{ID: id, Name: id, ClockDomain: clock,
			StartNS: start, EndNS: end, DurationNS: end - start, StartEventID: id + "-start", EndEventID: id + "-end", EvidenceRefs: []string{"source-evidence"}})
	}
	addInterval("parent", "controller", 100, 100000)
	addInterval("other-parent", "other", 100, 100000)
	addInterval("other-child", "other", 200, 500)
	snapshot.Edges = []AnalysisEdge{
		{From: "parent", To: "other-parent", Kind: "parent", Correlation: "exact_id"},
		{From: "other-parent", To: "other-child", Kind: "parent", Correlation: "verified_mapping"},
	}
	for i := 0; i < 12; i++ {
		id := "child-" + strconv.Itoa(i)
		addInterval(id, "controller", int64(200+i*1000), int64(500+i*1000))
		snapshot.Edges = append(snapshot.Edges, AnalysisEdge{From: "parent", To: id, Kind: "parent", Correlation: "exact_id", EvidenceRefs: []string{"source-evidence"}})
	}
	snapshot.Edges = append(snapshot.Edges, AnalysisEdge{From: "parent", To: "child-0", Kind: "parent", Correlation: "heuristic"})
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.SnapshotID = stableAnalysisID("snapshot", string(data))
	before, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	first, err := ProjectTrace(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ProjectTrace(snapshot)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatal("same snapshot produced different flow identities", err)
	}
	if first.Metadata.SchemaVersion != 3 || first.Metadata.ExporterVersion != TraceExporterVersion || first.Metadata.SnapshotID != snapshot.SnapshotID {
		t.Fatal("export format version or source snapshot identity lost")
	}
	for _, domain := range []string{"controller", "other"} {
		trace, err := ProjectTraceDomain(snapshot, domain)
		if err != nil {
			t.Fatal(err)
		}
		if domain == "controller" && !reflect.DeepEqual(trace.Events, first.Events) {
			t.Fatal("explicit primary view renumbered flows")
		}
		slices := make(map[string]TraceEvent)
		pairs := make(map[uint64]map[string]TraceEvent)
		for _, event := range trace.Events {
			if event.Phase == "X" {
				slices[event.Args["operation_id"].(string)] = event
			}
			if event.Phase != "s" && event.Phase != "f" {
				continue
			}
			id, err := strconv.ParseUint(event.ID, 16, 64)
			if err != nil || id == 0 || id > uint64(len(snapshot.Edges)) {
				t.Fatalf("invalid uint64 hex flow ID %q", event.ID)
			}
			edge := snapshot.Edges[id-1]
			if event.Args["from_operation_id"] != edge.From || event.Args["to_operation_id"] != edge.To ||
				event.Args["causal_edge_id"] != stableAnalysisID("flow", edge.From, edge.To, edge.Kind) || event.Name != edge.Kind {
				t.Fatal("display ID no longer identifies the full snapshot edge")
			}
			if pairs[id] == nil {
				pairs[id] = make(map[string]TraceEvent)
			}
			if _, exists := pairs[id][event.Phase]; exists {
				t.Fatal("same-kind edges collided")
			}
			pairs[id][event.Phase] = event
		}
		want := 12
		if domain == "other" {
			want = 1
		}
		if len(pairs) != want {
			t.Fatalf("view %s has %d flow pairs, want %d", domain, len(pairs), want)
		}
		for id, pair := range pairs {
			start, hasStart := pair["s"]
			end, hasEnd := pair["f"]
			if !hasStart || !hasEnd || start.ID != end.ID || !reflect.DeepEqual(start.Args, end.Args) {
				t.Fatal("unmatched flow endpoints")
			}
			edge := snapshot.Edges[id-1]
			from, hasFrom := slices[edge.From]
			to, hasTo := slices[edge.To]
			if !hasFrom || !hasTo || from.Args["clock_domain"] != domain || to.Args["clock_domain"] != domain {
				t.Fatal("unaligned or missing operation projected as a flow")
			}
			if start.TID != from.TID || end.TID != to.TID || start.Timestamp != from.Timestamp || end.Timestamp != to.Timestamp || start.BindingPoint != "" || end.BindingPoint != "e" {
				t.Fatal("flow does not bind to the actual endpoint slices")
			}
			encoded, err := json.Marshal(end)
			if err != nil || !strings.Contains(string(encoded), `"bp":"e"`) {
				t.Fatal("enclosing-slice binding not serialized")
			}
		}
		if _, exists := pairs[1]; exists {
			t.Fatal("cross-clock edge projected")
		}
		if _, exists := pairs[uint64(len(snapshot.Edges))]; exists {
			t.Fatal("heuristic edge projected")
		}
	}
	after, err := json.Marshal(snapshot)
	if err != nil || string(before) != string(after) {
		t.Fatal("display correction modified source snapshot")
	}
}

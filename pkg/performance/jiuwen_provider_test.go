package performance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func jiuwenProviderFixture(t *testing.T) (string, string, map[string]any) {
	t.Helper()
	root, dir, _, _, _, _ := jiuwenRemoteFixture(t)
	file := filepath.Join(root, "occ/harness/native-spans-index.json")
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var index map[string]any
	if json.Unmarshal(raw, &index) != nil {
		t.Fatal("index")
	}
	rows := index["spans"].([]any)
	for i, inv := range []string{"inv-a", "inv-b", "inv-aux"} {
		tool := "call-1"
		if i == 2 {
			tool = ""
		}
		rows = append(rows, map[string]any{"file": "traces-2026-10-27.jsonl", "line": 3 + i, "trace_id": strings.Repeat("a", 32), "span_id": fmt.Sprintf("%016x", i+3), "parent_span_id": strings.Repeat("2", 16), "start_time_unix_nano": "1793000000000001123", "end_time_unix_nano": "1793000000000002123", "native_session_id": "jiuwen-session", "native_request_id": "request-run", "tool_call_id": tool, "operation": "aries_provider_request", "native_status_code": 1, "matched_attempt": true, "provider_observation": map[string]any{
			"schema_version": 1, "invocation_id": inv, "operation": map[string]any{"kind": "shell", "method": "execute_cmd"},
			"clock":           map[string]any{"emitter_id": "provider", "clock_domain": "jiuwen-provider/provider", "origin_monotonic_ns": 1000, "origin_unix_ns": summaryAnchor.UnixNano(), "anchor_before_monotonic_ns": 998, "anchor_after_monotonic_ns": 1002},
			"start_offset_ns": 10 + i*100, "end_offset_ns": 100 + i*100, "outcome": "returned", "phases": []map[string]any{{"name": "http_submit", "offset_ns": 20 + i*100}, {"name": "response_headers", "offset_ns": 30 + i*100}, {"name": "first_frame", "offset_ns": 40 + i*100}, {"name": "terminal_frame", "offset_ns": 90 + i*100}},
		}})
	}
	index["spans"] = rows
	index["records_seen"] = 5
	index["matched_spans"] = 5
	index["provider_observation_expected"] = true
	writeSummaryJSON(t, filepath.Dir(file), filepath.Base(file), index)
	return root, dir, index
}

func TestJiuwenProviderOneSnapshotTimingIdentityAndAllExports(t *testing.T) {
	root, dir, _ := jiuwenProviderFixture(t)
	s, err := BuildSnapshot(dir)
	if err != nil {
		t.Fatal(err, s.Diagnostics)
	}
	providers, phases, links := 0, 0, 0
	events := map[string]bool{}
	for _, e := range s.Events {
		if e.Category != "jiuwen_remote_provider" && e.Category != "jiuwen_remote_bridge" {
			continue
		}
		if events[e.ID] {
			t.Fatal("duplicate source event identity", e.ID)
		}
		events[e.ID] = true
		if strings.HasPrefix(e.Name, "jiuwen.provider.") {
			phases++
		}
	}
	for _, i := range s.Intervals {
		if i.Category == "jiuwen_remote_provider" {
			providers++
			if i.DurationNS != 90 || i.Scope != jiuwenProviderScope {
				t.Fatal(i)
			}
		}
	}
	for _, e := range s.Edges {
		if e.Kind == "provider_invocation_identity" {
			links++
		}
	}
	if providers != 3 || phases != 12 || links != 3 {
		t.Fatal(providers, phases, links)
	}
	trace, err := ProjectTraceDomain(s, "jiuwen-provider/provider")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range trace.Events {
		if e.Phase == "s" {
			t.Fatal("cross-clock identity drawn as timing flow")
		}
	}
	otlp, err := ProjectOTLP(s)
	if err != nil || otlp.Manifest.NativeSpanCount != 5 {
		t.Fatal(err)
	}
	providerSpans := map[string]OTLPSpan{}
	for _, span := range otlpSpans(otlp) {
		if span.Name == "jiuwen.provider.request" {
			id, ok := otlpAttribute(span.Attributes, "aries.operation_id")
			if !ok || id.StringValue == nil || len(span.Events) != 4 {
				t.Fatal("provider span lost its identity or four phases", span)
			}
			providerSpans[*id.StringValue] = span
		}
	}
	if len(providerSpans) != providers {
		t.Fatal("provider OTLP span denominator", len(providerSpans))
	}
	for _, event := range s.Events {
		if !strings.HasPrefix(event.Name, "jiuwen.provider.") {
			continue
		}
		parent, _ := event.Attributes["parent_analysis_operation_id"].(string)
		span, ok := providerSpans[parent]
		if !ok {
			t.Fatal("provider phase has no exact exported parent", event)
		}
		matches := 0
		for _, projected := range span.Events {
			if projected.Name != event.Name {
				continue
			}
			matches++
			if projected.TimeUnixNano != strconv.FormatInt(summaryAnchor.UnixNano()+event.OffsetNS, 10) {
				t.Fatal("provider phase epoch changed", projected, event)
			}
			for key, want := range map[string]string{"event_id": event.ID, "parent_analysis_operation_id": parent, "invocation_id": event.Attributes["invocation_id"].(string), "clock_domain": event.ClockDomain, "attempt_id": event.AttemptID, "agent_session_id": event.AgentSessionID, "request_instance_id": event.RequestInstanceID, "tool_call_id": event.ToolCallID} {
				value, exists := otlpAttribute(projected.Attributes, "aries."+key)
				if want == "" && !exists {
					continue
				}
				if !exists || value.StringValue == nil || *value.StringValue != want {
					t.Fatal("provider phase identity changed", key, projected, event)
				}
			}
		}
		if matches != 1 {
			t.Fatal("provider phase missing or duplicated on its exact parent", event)
		}
	}
	metrics, err := ProjectMetrics(s)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal([]any{s, trace, otlp, metrics})
	if strings.Contains(string(raw), "PRIVATE_MODEL") {
		t.Fatal("payload leak")
	}
	count := uint64(0)
	for _, h := range metrics.Manifest.Histograms {
		if h.Metric == "aries.jiuwen.remote.duration" && h.Scope == jiuwenProviderScope {
			count += h.Count
		}
	}
	if count != 3 {
		t.Fatal("provider histogram denominator", count)
	}
	writeSummaryJSON(t, root, "snapshot.json", s)
	loaded, err := LoadSnapshot(filepath.Join(root, "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	again, err := ProjectTraceDomain(loaded, "jiuwen-provider/provider")
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(trace)
	after, _ := json.Marshal(again)
	if string(before) != string(after) {
		t.Fatal("snapshot reload changed projection")
	}
}

func TestJiuwenProviderRejectsMissingPairsBadClockAndIdentity(t *testing.T) {
	cases := map[string]func(map[string]any){
		"entire_provider_pair_missing": func(x map[string]any) {
			x["spans"] = x["spans"].([]any)[:4]
			x["records_seen"] = 4
			x["matched_spans"] = 4
		},
		"all_provider_pairs_missing": func(x map[string]any) {
			x["spans"] = x["spans"].([]any)[:2]
			x["records_seen"] = 2
			x["matched_spans"] = 2
		},
		"duplicate_invocation": func(x map[string]any) {
			x["spans"].([]any)[3].(map[string]any)["provider_observation"].(map[string]any)["invocation_id"] = "inv-a"
		},
		"wrong_tool": func(x map[string]any) { x["spans"].([]any)[2].(map[string]any)["tool_call_id"] = "other" },
		"bad_anchor": func(x map[string]any) {
			x["spans"].([]any)[2].(map[string]any)["provider_observation"].(map[string]any)["clock"].(map[string]any)["origin_monotonic_ns"] = 900
		},
		"conflicting_anchor": func(x map[string]any) {
			x["spans"].([]any)[3].(map[string]any)["provider_observation"].(map[string]any)["clock"].(map[string]any)["origin_unix_ns"] = summaryAnchor.UnixNano() + 1
		},
		"out_of_order_phase": func(x map[string]any) {
			x["spans"].([]any)[2].(map[string]any)["provider_observation"].(map[string]any)["phases"].([]map[string]any)[2]["offset_ns"] = 19
		},
	}
	for name, modify := range cases {
		t.Run(name, func(t *testing.T) {
			root, dir, index := jiuwenProviderFixture(t)
			modify(index)
			writeSummaryJSON(t, filepath.Join(root, "occ/harness"), "native-spans-index.json", index)
			s, err := BuildSnapshot(dir)
			if err == nil || s.IntegrityStatus != "incomplete" {
				t.Fatal("accepted invalid provider observation", s.Diagnostics)
			}
		})
	}
}

func TestJiuwenProviderRejectsClocklessBridgeWithAllProviderSpansMissing(t *testing.T) {
	root, dir, bridge, worker, bridges, workers := jiuwenRemoteFixture(t)
	delete(bridge, "clock")
	for _, row := range bridges {
		delete(row, "clock_domain")
		delete(row, "controller_start_offset_ns")
		delete(row, "controller_end_offset_ns")
	}
	writeJiuwenRemoteFixture(t, root, bridge, worker, bridges, workers)
	file := filepath.Join(root, "occ/harness/native-spans-index.json")
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var index map[string]any
	if err := json.Unmarshal(raw, &index); err != nil {
		t.Fatal(err)
	}
	index["provider_observation_expected"] = true
	writeSummaryJSON(t, filepath.Dir(file), filepath.Base(file), index)
	s, err := BuildSnapshot(dir)
	if err == nil || s.IntegrityStatus != "incomplete" {
		t.Fatal("accepted missing provider evidence with a clockless Bridge", s.Diagnostics)
	}
	bridgeFound, providerFound := false, false
	for _, coverage := range s.Coverage {
		switch coverage.Source {
		case "jiuwen_remote_bridge":
			bridgeFound = true
			if coverage.Integrity != "complete" || coverage.ExpectedOperations == nil || *coverage.ExpectedOperations != 3 {
				t.Fatal("clockless Bridge lost its accepted-call denominator", coverage)
			}
		case "jiuwen_provider_bridge_identity":
			providerFound = true
			if coverage.Integrity != "incomplete" || coverage.ExpectedOperations == nil || *coverage.ExpectedOperations != 3 {
				t.Fatal("missing provider evidence became zero expected operations", coverage)
			}
		}
	}
	if !bridgeFound || !providerFound || !strings.Contains(strings.Join(s.Diagnostics, "\n"), "lack timed identities for provider reconciliation") {
		t.Fatal("expected provider reconciliation failure", s.Diagnostics)
	}
}

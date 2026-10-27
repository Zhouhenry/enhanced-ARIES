package performance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
)

func jiuwenRemoteFixture(t *testing.T) (string, string, map[string]any, map[string]any, []map[string]any, []map[string]any) {
	t.Helper()
	root, dir, _ := jiuwenSnapshotFixture(t)
	summary := &core.AttemptSummary{Planned: 1, Admitted: 1, Started: 1, Executed: 1, Completed: 1, WindowStartedAt: summaryAnchor, WindowFinishedAt: summaryAnchor.Add(time.Second), ObservationDuration: time.Second}
	writeSummaryJSON(t, root, "run-result.json", map[string]any{"run_id": "run", "attempt_summary": summary, "attempts": []traceAttempt{{AttemptID: "run/occ", OccurrenceID: "occ", AgentSessionID: "jiuwen-session"}}})
	bridge := map[string]any{"schema_version": 2, "run_id": "run", "task_id": "occ", "binding_id": "binding", "sandbox_id": "sandbox", "clock": map[string]any{"emitter_id": "emitter", "clock_domain": "clock", "started_at_utc": summaryAnchor}, "accepted_calls": 3, "finished_calls": 3, "records": 3, "truncated": false, "group_empty_confirmed": true, "local_calls_joined": true}
	worker := map[string]any{"schema_version": 1, "run_id": "run", "task_id": "occ", "binding_id": "binding", "clock": map[string]any{"emitter_id": "worker", "clock_domain": "jiuwen-worker/worker", "origin_monotonic_ns": 1000, "origin_unix_ns": summaryAnchor.Add(time.Minute).UnixNano(), "anchor_before_monotonic_ns": 998, "anchor_after_monotonic_ns": 1002}, "admitted": 3, "terminal": 3, "active": 0, "closing": false, "attempted": 9, "written": 9, "dropped": 0, "write_failures": 0, "index_write_failures": 0, "measurement_scope": jiuwenWorkerScope}
	bridges := []map[string]any{}
	workers := []map[string]any{}
	for i := 0; i < 3; i++ {
		inv := []string{"inv-a", "inv-b", "inv-aux"}[i]
		id := map[string]any{"run_id": "run", "task_id": "occ", "session_id": "jiuwen-session", "request_id": "request-run", "tool_call_id": "call-1", "identity_status": "known"}
		if i == 2 {
			id["tool_call_id"] = nil
			id["identity_status"] = "unavailable"
		}
		op := map[string]any{"kind": "shell", "method": "execute_cmd"}
		bridges = append(bridges, map[string]any{"schema_version": 2, "sequence": i + 1, "binding_id": "binding", "sandbox_id": "sandbox", "invocation_id": inv, "identity": id, "operation": op, "route": "/call", "clock_domain": "clock", "controller_start_offset_ns": 100 + i*100, "controller_end_offset_ns": 1100 + i*100, "controller_duration_ns": 1000, "http_status": 200, "transport_complete": true, "measurement_scope": jiuwenBridgeScope})
		for j, event := range []string{"invocation_start", "invocation_returned", "invocation_terminal"} {
			row := map[string]any{"schema_version": 1, "sequence": i*3 + j + 1, "event": event, "invocation_id": inv, "identity": id, "operation": op, "clock_domain": "jiuwen-worker/worker", "monotonic_ns": 1100 + i*1000 + j*100, "unix_ns": summaryAnchor.UnixNano() + int64(i*1000+j*100)}
			if event == "invocation_terminal" {
				row["outcome"] = "returned"
			}
			if event == "invocation_returned" && i == 1 {
				row["status"] = "started"
			}
			if event == "invocation_returned" && i == 0 {
				row["exit_code"] = 7
			}
			workers = append(workers, row)
		}
	}
	writeJiuwenRemoteFixture(t, root, bridge, worker, bridges, workers)
	return root, dir, bridge, worker, bridges, workers
}
func writeJiuwenRemoteFixture(t *testing.T, root string, bridge, worker map[string]any, bridges, workers []map[string]any) {
	t.Helper()
	base := filepath.Join(root, "occ/bridge")
	if err := os.MkdirAll(base, 0700); err != nil {
		t.Fatal(err)
	}
	writeSummaryJSON(t, base, "remote-bridge-index.json", bridge)
	writeSummaryJSON(t, base, "remote-worker-index.json", worker)
	writeTraceEvents(t, filepath.Join(base, "remote-bridge-events.jsonl"), bridges)
	writeTraceEvents(t, filepath.Join(base, "remote-worker-events.jsonl"), workers)
}
func TestJiuwenRemoteSnapshotExportsSameEvidenceToTraceOTLPAndMetrics(t *testing.T) {
	root, dir, _, _, _, _ := jiuwenRemoteFixture(t)
	s, err := BuildSnapshot(dir)
	if err != nil {
		t.Fatal(err, s.Diagnostics)
	}
	counts := map[string]int{}
	for _, i := range s.Intervals {
		counts[i.Category]++
	}
	if counts["jiuwen_remote_bridge"] != 3 || counts["jiuwen_remote_worker"] != 3 {
		t.Fatal(counts)
	}
	toolLinks := 0
	for _, edge := range s.Edges {
		if edge.Kind == "tool_call_identity" {
			toolLinks++
		}
	}
	if toolLinks != 2 {
		t.Fatal("one native Tool must retain both provider invocations", toolLinks)
	}
	controller, err := ProjectTrace(s)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := ProjectTraceDomain(s, "jiuwen-worker/worker")
	if err != nil {
		t.Fatal(err)
	}
	for _, trace := range []Trace{controller, worker} {
		for _, event := range trace.Events {
			if event.Phase == "s" && event.Category == "causal_reference" {
				t.Fatal("cross-domain identity became timeline flow")
			}
		}
	}
	otlp, err := ProjectOTLP(s)
	if err != nil {
		t.Fatal(err)
	}
	links := 0
	workerSpans := 0
	for _, resource := range otlp.Request.ResourceSpans {
		for _, scope := range resource.ScopeSpans {
			for _, span := range scope.Spans {
				if span.Name == "jiuwen.worker.native_operation_request" {
					workerSpans++
					links += len(span.Links)
					if span.ParentSpanID != "" {
						t.Fatal("cross-clock parent invented")
					}
				}
			}
		}
	}
	if links != 3 || workerSpans != 3 {
		t.Fatalf("worker=%d links=%d", workerSpans, links)
	}
	nativeLinks := 0
	for _, resource := range otlp.Request.ResourceSpans {
		for _, scope := range resource.ScopeSpans {
			for _, span := range scope.Spans {
				if span.Name != "jiuwen.bridge.forwarding" {
					continue
				}
				if span.ParentSpanID != "" {
					t.Fatal("native tool identity invented a cross-clock parent")
				}
				for _, link := range span.Links {
					if link.TraceID != strings.Repeat("a", 32) || link.SpanID != strings.Repeat("2", 16) {
						t.Fatal("native trace/span identity rewritten", link)
					}
					nativeLinks++
				}
			}
		}
	}
	if nativeLinks != 2 || len(otlp.Manifest.ExcludedEdges) != 0 {
		t.Fatal("native links not projected", nativeLinks, otlp.Manifest.ExcludedEdges)
	}
	nativeTrace, err := ProjectTraceDomain(s, "jiuwen-wall/run/occ")
	if err != nil || len(nativeTrace.Metadata.IdentityLinks) != 2 {
		t.Fatal("native Perfetto view lost queryable identity links", err)
	}
	for _, trace := range []Trace{controller, nativeTrace} {
		seen := 0
		for _, event := range trace.Events {
			if event.Phase == "s" && (event.Name == "tool_call_identity" || event.Name == "invocation_identity") {
				t.Fatal("identity-only relationship became a timeline flow")
			}
			if refs, ok := event.Args["identity_links"].([]AnalysisEdge); ok {
				for _, ref := range refs {
					if ref.Kind == "tool_call_identity" {
						seen++
					}
				}
			}
		}
		if seen != 2 {
			t.Fatal("visible span lacks identity links", seen)
		}
	}
	metrics, err := ProjectMetrics(s)
	if err != nil {
		t.Fatal(err)
	}
	metric := findMetric(t, metrics, "aries.jiuwen.remote.duration")
	if len(metric.Histogram.DataPoints) != 2 {
		t.Fatal("remote layers not distinct")
	}
	members := 0
	sum := int64(0)
	for _, g := range metrics.Manifest.Histograms {
		if g.Metric == metric.Name {
			members += len(g.Members)
			sum += g.SumNS
			for _, m := range g.Members {
				if m.OperationID == "" {
					t.Fatal("metric member lost exact interval")
				}
			}
		}
	}
	if members != 6 || sum != 3600 {
		t.Fatalf("members=%d sum=%d", members, sum)
	}
	for _, i := range s.Intervals {
		if strings.HasPrefix(i.Category, "jiuwen_remote") && i.Attributes["process_completion"] != "unknown" {
			t.Fatal("request result became process completion")
		}
	}
	writeSummaryJSON(t, root, "snapshot.json", s)
	loaded, err := LoadSnapshot(filepath.Join(root, "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(root, "occ/bridge/remote-worker-events.jsonl"))
	after, err := ProjectOTLP(loaded)
	if err != nil || !reflect.DeepEqual(after, otlp) {
		t.Fatal("exporter reread raw evidence")
	}
}
func TestJiuwenRemoteSnapshotRejectsMissingPairsWrongIdentityAndClocks(t *testing.T) {
	for _, mode := range []string{"dropped", "index-write-failed", "entire-pair-missing", "missing-terminal", "missing-returned", "duplicate-returned", "duplicate-invocation", "wrong-binding", "wrong-tool", "wrong-clock", "duration", "unknown-terminal", "unconfirmed-stop"} {
		t.Run(mode, func(t *testing.T) {
			root, dir, b, w, br, wr := jiuwenRemoteFixture(t)
			switch mode {
			case "dropped":
				w["dropped"] = 1
			case "index-write-failed":
				w["index_write_failures"] = 1
			case "entire-pair-missing":
				wr = wr[:6]
			case "missing-terminal":
				wr = wr[:8]
			case "missing-returned":
				wr[1]["event"] = "invocation_error"
			case "duplicate-returned":
				wr[2]["event"] = "invocation_returned"
			case "duplicate-invocation":
				br[1]["invocation_id"] = "inv-a"
			case "wrong-binding":
				w["binding_id"] = "foreign"
			case "wrong-tool":
				wr[0]["identity"] = map[string]any{"run_id": "run", "task_id": "occ", "tool_call_id": "foreign", "identity_status": "known"}
			case "wrong-clock":
				w["clock"].(map[string]any)["origin_monotonic_ns"] = 99
			case "duration":
				br[0]["controller_duration_ns"] = 99
			case "unknown-terminal":
				wr[2]["outcome"] = "unknown"
			case "unconfirmed-stop":
				b["group_empty_confirmed"] = false
			}
			writeJiuwenRemoteFixture(t, root, b, w, br, wr)
			s, err := BuildSnapshot(dir)
			if err == nil || s.IntegrityStatus != "incomplete" {
				t.Fatal("invalid evidence accepted", s.Diagnostics)
			}
		})
	}
}
func TestJiuwenRemoteLegacyPreservesDurationWithoutInventingEpoch(t *testing.T) {
	root, dir, b, w, br, wr := jiuwenRemoteFixture(t)
	b["schema_version"] = 1
	writeJiuwenRemoteFixture(t, root, b, w, br, wr)
	s, err := BuildSnapshot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.UntimedOperations) != 3 {
		t.Fatal("legacy durations missing")
	}
	for _, i := range s.Intervals {
		if strings.HasPrefix(i.Category, "jiuwen_remote") {
			t.Fatal("invented legacy monotonic origin")
		}
	}
	before, _ := json.Marshal(s.Attempts)
	b["schema_version"] = 2
	writeJiuwenRemoteFixture(t, root, b, w, br, wr)
	after, err := BuildSnapshot(dir)
	if err != nil {
		t.Fatal(err)
	}
	afterBytes, _ := json.Marshal(after.Attempts)
	if string(before) != string(afterBytes) {
		t.Fatal("remote source changed stages")
	}
}

func TestJiuwenNativeToolAssociationRequiresUniqueCompleteIdentity(t *testing.T) {
	for _, mode := range []string{"missing-session", "missing-request", "wrong-session", "wrong-request", "wrong-tool", "ambiguous-native"} {
		t.Run(mode, func(t *testing.T) {
			root, directory, bridge, worker, bridges, workers := jiuwenRemoteFixture(t)
			identity := bridges[0]["identity"].(map[string]any)
			expected := "native_tool_not_found"
			switch mode {
			case "missing-session":
				identity["session_id"] = nil
				expected = "missing_identity"
			case "missing-request":
				identity["request_id"] = nil
				expected = "missing_identity"
			case "wrong-session":
				identity["session_id"] = "another-session"
			case "wrong-request":
				identity["request_id"] = "another-request"
			case "wrong-tool":
				identity["tool_call_id"] = "another-tool"
			case "ambiguous-native":
				expected = "ambiguous_native_tool"
				path := filepath.Join(root, "occ/harness/native-spans-index.json")
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var index map[string]any
				if err = json.Unmarshal(raw, &index); err != nil {
					t.Fatal(err)
				}
				spans := index["spans"].([]any)
				duplicate := map[string]any{}
				for k, v := range spans[1].(map[string]any) {
					duplicate[k] = v
				}
				duplicate["span_id"] = strings.Repeat("3", 16)
				duplicate["line"] = 3
				index["spans"] = append(spans, duplicate)
				index["records_seen"], index["matched_spans"], index["matched_authoritative_tool_spans"] = 3, 3, 2
				writeSummaryJSON(t, filepath.Dir(path), filepath.Base(path), index)
			}
			writeJiuwenRemoteFixture(t, root, bridge, worker, bridges, workers)
			snapshot, err := BuildSnapshot(directory)
			if err != nil {
				t.Fatal(err)
			}
			var operationID string
			for _, interval := range snapshot.Intervals {
				if interval.Category == "jiuwen_remote_bridge" && interval.Attributes["invocation_id"] == "inv-a" {
					operationID = interval.ID
					if interval.Attributes["native_tool_association"] != expected || interval.Attributes["native_tool_operation_id"] != nil {
						t.Fatal("association guessed", interval.Attributes)
					}
				}
			}
			if operationID == "" {
				t.Fatal("bridge operation missing")
			}
			for _, edge := range snapshot.Edges {
				if edge.Kind == "tool_call_identity" && edge.To == operationID {
					t.Fatal("incomplete or ambiguous identity got a trusted edge")
				}
			}
		})
	}
}

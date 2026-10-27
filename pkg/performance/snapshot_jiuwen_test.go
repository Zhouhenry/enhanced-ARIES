package performance

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestJiuwenNativeSnapshotAcceptsLargeValidatedPrivateSource(t *testing.T) {
	root, directory, index := jiuwenSnapshotFixture(t)
	content := append(bytes.Repeat([]byte("x"), 18<<20), '\n')
	file := index["files"].([]map[string]any)[0]
	file["size"], file["retained_bytes"], file["retained_sha256"] = len(content), len(content), fmt.Sprintf("%x", sha256.Sum256(content))
	base := filepath.Join(root, "occ", "harness")
	if err := os.WriteFile(filepath.Join(base, "native-spans", file["name"].(string)), content, 0600); err != nil {
		t.Fatal(err)
	}
	writeSummaryJSON(t, base, "native-spans-index.json", index)
	snapshot, err := BuildSnapshot(directory)
	if err != nil || len(snapshot.NativeSpans) != 2 {
		t.Fatal("analysis rejected a large collector-validated source", err)
	}
	if _, err := ProjectTraceDomain(snapshot, "jiuwen-wall/run/occ"); err != nil {
		t.Fatal(err)
	}
	if output, err := ProjectOTLP(snapshot); err != nil || output.Manifest.NativeSpanCount != 2 {
		t.Fatal("OTLP projection lost native spans", err)
	}
}

func jiuwenSnapshotFixture(t *testing.T) (string, string, map[string]any) {
	t.Helper()
	root, directory := traceFixture(t)
	writeSummaryJSON(t, root, "run-result.json", map[string]any{"run_id": "run", "attempts": []traceAttempt{{AttemptID: "run/occ", OccurrenceID: "occ", AgentSessionID: "jiuwen-session"}}})
	writeSummaryJSON(t, root, "cloud-profile.json", map[string]any{"agent_type": "jiuwenswarm"})
	base := filepath.Join(root, "occ", "harness")
	if err := os.MkdirAll(filepath.Join(base, "native-spans"), 0700); err != nil {
		t.Fatal(err)
	}
	writeSummaryJSON(t, base, "native-events-index.json", map[string]any{"official_span_collection_enabled": true})
	// The collector has already validated raw OTLP. The analysis checks the
	// immutable source hash and imports only its public identity index.
	content := []byte("PRIVATE_MODEL_PROMPT_AND_TOOL_OUTPUT\n")
	name := "traces-2026-10-27.jsonl"
	if err := os.WriteFile(filepath.Join(base, "native-spans", name), content, 0600); err != nil {
		t.Fatal(err)
	}
	trace := strings.Repeat("a", 32)
	rootSpan, toolSpan := strings.Repeat("1", 16), strings.Repeat("2", 16)
	index := map[string]any{"schema_version": 1, "format": "openjiuwen.single_span_otlp_jsonl.hex_ids.v1", "clock": "native_unix_nanoseconds_unaligned", "enabled": true,
		"native_session_id": "jiuwen-session", "native_request_id": "request-run", "records_seen": 2, "invalid_records": 0, "matched_spans": 2, "unassociated_spans": 0, "matched_authoritative_tool_spans": 1, "matched_complete_roots": 1,
		"files": []map[string]any{{"name": name, "size": len(content), "retained_bytes": len(content), "retained_sha256": fmt.Sprintf("%x", sha256.Sum256(content))}},
		"spans": []map[string]any{
			{"file": name, "line": 1, "trace_id": trace, "span_id": rootSpan, "start_time_unix_nano": "1793000000000000123", "end_time_unix_nano": "1793000000000009123", "native_session_id": "jiuwen-session", "native_request_id": "request-run", "root": true, "complete": true, "matched_attempt": true, "operation": "invoke_agent", "native_status_code": 1},
			{"file": name, "line": 2, "trace_id": trace, "span_id": toolSpan, "parent_span_id": rootSpan, "start_time_unix_nano": "1793000000000001123", "end_time_unix_nano": "1793000000000004123", "tool_call_id": "call-1", "authoritative_tool": true, "matched_attempt": true, "tool_name": "bash", "native_status_code": 2},
		}}
	writeSummaryJSON(t, base, "native-spans-index.json", index)
	return root, directory, index
}

func TestJiuwenNativeSpansPreserveEpochIDsAndRemainUnaligned(t *testing.T) {
	root, directory, _ := jiuwenSnapshotFixture(t)
	snapshot, err := BuildSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.NativeSpans) != 2 || snapshot.CoverageStatus != "unknown" {
		t.Fatalf("spans=%d coverage=%s", len(snapshot.NativeSpans), snapshot.CoverageStatus)
	}
	for _, clock := range snapshot.Clocks {
		if strings.HasPrefix(clock.ID, "jiuwen-wall/") && (clock.OriginUTC != nil || clock.Alignment != "unaligned_native_wall_clock") {
			t.Fatal("wall time became monotonic anchor")
		}
	}
	if snapshot.NativeSpans[1].ParentStatus != "exact_native_parent" || snapshot.NativeSpans[1].DurationNS != 3000 {
		t.Fatal("native identity or time lost")
	}
	primary, err := ProjectTrace(snapshot)
	if err != nil || len(primary.Metadata.ExcludedClockDomains) != 1 {
		t.Fatalf("primary: %v", err)
	}
	for _, e := range primary.Events {
		if e.Category == "native_span" {
			t.Fatal("unaligned native span in controller view")
		}
	}
	native, err := ProjectTraceDomain(snapshot, "jiuwen-wall/run/occ")
	if err != nil || native.Metadata.WallClockOriginUnixNano != "1793000000000000123" {
		t.Fatalf("native: %+v %v", native.Metadata, err)
	}
	count, flows := 0, 0
	for _, e := range native.Events {
		if e.Phase == "X" {
			count++
			if e.Name == "native.tool.bash" && (e.Timestamp != 1 || *e.Duration != 3 || e.Args["monotonic_offset_ns"] != nil) {
				t.Fatalf("incorrect wall projection: %+v", e)
			}
		}
		if e.Phase == "s" {
			flows++
		}
	}
	if count != 2 || flows != 1 {
		t.Fatal("native spans/parent not projected")
	}
	otlp, err := ProjectOTLP(snapshot)
	if err != nil || otlp.Manifest.NativeSpanCount != 2 {
		t.Fatalf("OTLP: %v", err)
	}
	spans := otlp.Request.ResourceSpans[1].ScopeSpans[0].Spans
	if spans[1].TraceID != strings.Repeat("a", 32) || spans[1].SpanID != strings.Repeat("2", 16) || spans[1].ParentSpanID != strings.Repeat("1", 16) || spans[1].StartTimeUnixNano != "1793000000000001123" || spans[1].Status.Code != 2 {
		t.Fatal("native OTLP IDs/timestamp/status rewritten")
	}
	for _, value := range []any{snapshot, native, otlp} {
		data, _ := json.Marshal(value)
		if strings.Contains(string(data), "PRIVATE_MODEL") {
			t.Fatal("private native payload leaked")
		}
	}
	writeSummaryJSON(t, root, "snapshot.json", snapshot)
	loaded, err := LoadSnapshot(filepath.Join(root, "snapshot.json"))
	if err != nil {
		t.Fatal(err)
	}
	after, err := ProjectOTLP(loaded)
	if err != nil || !reflect.DeepEqual(after, otlp) {
		t.Fatal("frozen native snapshot changed projection")
	}
}

func TestJiuwenNativeSpanBrokenEvidenceIsNotProjected(t *testing.T) {
	for _, problem := range []string{"foreign_session", "foreign_request", "invalid_non_tool_id", "duplicate_span", "negative_duration", "source_hash", "count", "truncated", "invalid", "missing_source"} {
		t.Run(problem, func(t *testing.T) {
			root, directory, index := jiuwenSnapshotFixture(t)
			spans := index["spans"].([]map[string]any)
			switch problem {
			case "foreign_session":
				index["native_session_id"] = "other"
			case "foreign_request":
				spans[1]["native_request_id"] = "other"
			case "invalid_non_tool_id":
				spans[0]["tool_call_id"] = "invalid\nidentifier"
			case "duplicate_span":
				spans[1]["span_id"] = spans[0]["span_id"]
			case "negative_duration":
				spans[1]["end_time_unix_nano"] = "1"
			case "source_hash":
				index["files"].([]map[string]any)[0]["retained_sha256"] = strings.Repeat("0", 64)
			case "count":
				index["matched_spans"] = 5
			case "truncated":
				index["truncated"] = true
			case "invalid":
				index["invalid_records"] = 1
			case "missing_source":
				if err := os.Remove(filepath.Join(root, "occ/harness/native-spans/traces-2026-10-27.jsonl")); err != nil {
					t.Fatal(err)
				}
			}
			writeSummaryJSON(t, filepath.Join(root, "occ/harness"), "native-spans-index.json", index)
			snapshot, err := BuildSnapshot(directory)
			if err == nil || snapshot.IntegrityStatus != "incomplete" || len(snapshot.NativeSpans) != 0 {
				t.Fatal("invalid native evidence projected")
			}
		})
	}
}

func TestJiuwenDisabledAndMissingNativeSpansRemainDistinct(t *testing.T) {
	root, directory, index := jiuwenSnapshotFixture(t)
	index["enabled"] = false
	writeSummaryJSON(t, filepath.Join(root, "occ/harness"), "native-spans-index.json", index)
	snapshot, err := BuildSnapshot(directory)
	if err != nil || len(snapshot.NativeSpans) != 0 {
		t.Fatal("disabled native collection treated as error")
	}
	found := false
	for _, c := range snapshot.Coverage {
		if c.Source == "jiuwen_native_spans" {
			found = c.Status == "not_enabled" && c.ExpectedEvents == nil && c.DroppedEvents == nil
		}
	}
	if !found {
		t.Fatal("disabled source not recorded")
	}
	if err := os.Remove(filepath.Join(root, "occ/harness/native-spans-index.json")); err != nil {
		t.Fatal(err)
	}
	snapshot, err = BuildSnapshot(directory)
	if err == nil || snapshot.IntegrityStatus != "incomplete" {
		t.Fatal("missing enabled source silently skipped")
	}
	writeSummaryJSON(t, filepath.Join(root, "occ/harness"), "native-events-index.json", map[string]any{"source": "jiuwenswarm.native_http_sse"})
	snapshot, err = BuildSnapshot(directory)
	if err != nil || snapshot.IntegrityStatus != "complete" || len(snapshot.NativeSpans) != 0 {
		t.Fatal("historical SSE-only capture was mislabeled as native span loss")
	}
}

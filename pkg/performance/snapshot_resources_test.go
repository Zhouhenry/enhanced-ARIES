package performance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func resourceFixture(t *testing.T) (string, string) {
	t.Helper()
	root, directory := traceFixture(t)
	writeSummaryJSON(t, root, "run-result.json", map[string]any{"run_id": "run", "attempts": []traceAttempt{{AttemptID: "run/occ", OccurrenceID: "occ"}}})
	return root, directory
}

func resourceControllerFixture(t *testing.T, root string) (map[string]any, map[string]any) {
	t.Helper()
	sample := func(start, finish, user, system, alloc, rss int64) map[string]any {
		return map[string]any{
			"sampling_started_offset_ns": start, "sampling_finished_offset_ns": finish, "cpu_status": "collected",
			"cpu": map[string]any{"user_ns": user, "system_ns": system}, "process_lifetime_peak_rss_bytes": rss,
			"allocation": map[string]any{"total_alloc_bytes": alloc, "mallocs": alloc / 10, "num_gc": alloc / 100, "gc_pause_total_ns": alloc * 2}, "heap_alloc_bytes": alloc / 2,
		}
	}
	first := map[string]any{"schema_version": 1, "run_id": "run", "pid": 42, "measurement_scope": "controller_shared_process_self", "anchor_utc": summaryAnchor,
		"status": "incomplete", "start_artifact_written": false, "start": sample(10, 20, 100, 20, 1000, 10000), "end": nil, "delta": nil}
	last := map[string]any{"schema_version": 1, "run_id": "run", "pid": 42, "measurement_scope": "controller_shared_process_self", "anchor_utc": summaryAnchor,
		"status": "complete", "start_artifact_written": true, "start": first["start"], "end": sample(100, 120, 200, 30, 2000, 10000),
		"delta": map[string]any{"cpu": map[string]any{"user_ns": 100, "system_ns": 10}, "allocation": map[string]any{"total_alloc_bytes": 1000, "mallocs": 100, "num_gc": 10, "gc_pause_total_ns": 2000}}, "private": "SECRET_CONTROLLER"}
	writeSummaryJSON(t, root, "controller-resources.start.json", first)
	writeSummaryJSON(t, root, "controller-resources.end.json", last)
	return first, last
}

func TestResourceSnapshotControllerAndFrozenCompatibility(t *testing.T) {
	root, directory := resourceFixture(t)
	old, err := BuildSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	if old.Resources != nil {
		t.Fatal("absent resources fabricated")
	}
	// A frozen schema-1 snapshot remains readable without adding the new field
	// or replacing its parser version or original content identity.
	old.ParserVersion = "aries-analysis-v1.4.0-native-first-output"
	old.SnapshotID = ""
	raw, _ := json.Marshal(old)
	old.SnapshotID = stableAnalysisID("snapshot", string(raw))
	frozen, _ := json.MarshalIndent(old, "", "  ")
	if strings.Contains(string(frozen), "\"resources\"") {
		t.Fatal("optional field changed old encoding")
	}
	path := filepath.Join(root, "old-snapshot.json")
	if err := os.WriteFile(path, frozen, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSnapshot(path)
	if err != nil || loaded.SnapshotID != old.SnapshotID {
		t.Fatalf("old frozen identity: %v", err)
	}
	resourceControllerFixture(t, root)
	got, err := BuildSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	if got.Resources == nil || len(got.Resources.Streams) != 1 {
		t.Fatal("controller stream missing")
	}
	stream := got.Resources.Streams[0]
	if stream.Scope != "controller_shared_process_self" || stream.OwnerID != "42" || len(stream.Samples) != 2 || stream.TimeBasis != "controller_sampling_completed" || len(stream.Delta) != 6 {
		t.Fatalf("controller contract: %+v", stream)
	}
	if !stream.Samples[1].ObservedAtUTC.Equal(summaryAnchor.Add(120*time.Nanosecond)) || *stream.Samples[1].SamplingStartedOffsetNS != 100 || *stream.Samples[1].SamplingFinishedOffsetNS != 120 {
		t.Fatal("sampling bracket/own anchor lost")
	}
	for _, v := range stream.Delta {
		if v.Name == "process_lifetime_peak_rss_bytes" {
			t.Fatal("lifetime RSS differenced into window peak")
		}
	}
	for _, pair := range [][2]any{{old.Events, got.Events}, {old.Intervals, got.Intervals}, {old.Edges, got.Edges}, {old.Summary, got.Summary}, {old.Attempts, got.Attempts}, {old.Clocks, got.Clocks}} {
		if !reflect.DeepEqual(pair[0], pair[1]) {
			t.Fatal("resource parsing changed existing evidence")
		}
	}
	content, _ := json.MarshalIndent(got, "", "  ")
	if strings.Contains(string(content), "SECRET_") {
		t.Fatal("non-allowlisted metadata copied")
	}
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadSnapshot(path)
	if err != nil || reloaded.SnapshotID != got.SnapshotID {
		t.Fatalf("typed resources changed on roundtrip: %v", err)
	}
	reloaded.Resources.Streams[0].Samples[0].Values[0].Value = json.Number("999")
	if ValidateSnapshot(reloaded) == nil {
		t.Fatal("changed resource value accepted under old ID")
	}
}

func TestResourceControllerMissingEndAndCounterMismatch(t *testing.T) {
	for _, name := range []string{"missing_end", "wrong_run", "changed_start", "reversed_bracket", "wrong_delta"} {
		t.Run(name, func(t *testing.T) {
			root, directory := resourceFixture(t)
			_, last := resourceControllerFixture(t, root)
			switch name {
			case "missing_end":
				if err := os.Remove(filepath.Join(root, "controller-resources.end.json")); err != nil {
					t.Fatal(err)
				}
			case "wrong_run":
				last["run_id"] = "other"
			case "changed_start":
				last["start"] = last["end"]
			case "reversed_bracket":
				last["end"].(map[string]any)["sampling_finished_offset_ns"] = int64(1)
			case "wrong_delta":
				last["delta"].(map[string]any)["cpu"].(map[string]any)["user_ns"] = 999
			}
			if name != "missing_end" {
				writeSummaryJSON(t, root, "controller-resources.end.json", last)
			}
			snapshot, err := BuildSnapshot(directory)
			if err == nil || snapshot.Resources == nil || snapshot.Resources.Streams[0].IntegrityStatus != "incomplete" {
				t.Fatal("invalid controller source accepted")
			}
		})
	}
}

func resourceMonitorFixture(t *testing.T, root string) map[string]any {
	t.Helper()
	rows := []map[string]any{}
	for i, owner := range []string{"container-a", "container-b", "container-a"} {
		rows = append(rows, map[string]any{
			"scope": "task", "run_id": "run", "task_id": "occ", "sequence": i, "second": i, "time": summaryAnchor.Add(time.Duration(i) * time.Second),
			"component": "harness", "runtime_id": owner, "runtime_name": "SECRET_RUNTIME_NAME", "cpu_usage_nanoseconds": 100 + i*10, "cpu_percent": 0,
			"memory_usage_bytes": 1024 + i, "memory_limit_bytes": 4096,
		})
	}
	dir := filepath.Join(root, "occ", "monitor")
	writeTraceEvents(t, filepath.Join(dir, "resources.jsonl"), rows)
	index := map[string]any{"schema_version": 5, "scope": "task", "run_id": "run", "task_id": "occ", "status": "succeeded", "resources_file": "resources.jsonl", "sample_count": 3,
		"components": []map[string]any{{"component": "harness", "runtime_id": "container-a", "sample_count": 2}, {"component": "harness", "runtime_id": "container-b", "sample_count": 1}}}
	writeSummaryJSON(t, dir, "index.json", index)
	return index
}

func TestResourceMonitorKeepsOwnersAndRawCounters(t *testing.T) {
	root, directory := resourceFixture(t)
	resourceMonitorFixture(t, root)
	snapshot, err := BuildSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	streams := snapshot.Resources.Streams
	if len(streams) != 2 || streams[0].ID == streams[1].ID || streams[0].OwnerID == streams[1].OwnerID {
		t.Fatal("same component owners merged")
	}
	if len(streams[0].Samples) != 2 || len(streams[1].Samples) != 1 {
		t.Fatal("owner samples lost")
	}
	for _, stream := range streams {
		for _, sample := range stream.Samples {
			if sample.TimeStatus != "verified" || sample.ObservedAtUTC == nil {
				t.Fatal("original monitor sample time lost")
			}
			for _, v := range sample.Values {
				if v.Name == "cpu_percent" && (v.ExportEligible || v.Value != "0" || v.Reason == "") {
					t.Fatal("placeholder CPU percentage exported or discarded")
				}
				if v.Name == "cpu_usage_nanoseconds" && (!v.ExportEligible || v.Kind != "cumulative_counter" || v.Unit != "ns") {
					t.Fatal("CPU counter changed into a rate")
				}
			}
		}
	}
	raw, _ := json.Marshal(snapshot)
	if strings.Contains(string(raw), "SECRET_") {
		t.Fatal("runtime name copied outside allowlist")
	}
	index := resourceMonitorFixture(t, root)
	index["sample_count"] = 4
	writeSummaryJSON(t, filepath.Join(root, "occ", "monitor"), "index.json", index)
	if failed, err := BuildSnapshot(directory); err == nil || failed.Resources.Streams[0].IntegrityStatus != "incomplete" {
		t.Fatal("monitor independent sample denominator ignored")
	}
}

func TestResourceMonitorRejectsNonconsecutiveSequence(t *testing.T) {
	root, directory := resourceFixture(t)
	resourceMonitorFixture(t, root)
	path := filepath.Join(root, "occ", "monitor", "resources.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(string(data), `"sequence":1`, `"sequence":0`, 1)
	if changed == string(data) {
		t.Fatal("fixture sequence missing")
	}
	if err := os.WriteFile(path, []byte(changed), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildSnapshot(directory); err == nil {
		t.Fatal("duplicate sequence accepted despite matching row count")
	}
}

func providerFixtureRows() []map[string]any {
	value := map[string]any{"timestamp": "2026-10-22T00:00:01+00:00", "cpu_count": 2, "cpu_used_pct": 0, "mem_used": 1024, "mem_total": 4096, "mem_cache": 12, "disk_used": 100, "disk_total": 1000, "private": "SECRET_PROVIDER"}
	makeRow := func(owner, purpose string, readings []map[string]any) map[string]any {
		return map[string]any{
			"schema_version": 1, "run_id": "run", "occurrence_id": "occ", "sandbox_handle": owner, "purpose": purpose,
			"requested_at_utc": summaryAnchor.Add(10 * time.Second), "client_rpc_duration_ns": 1000000,
			"snapshot": map[string]any{"source": "e2b_sdk", "nominal_interval_seconds": 5, "metrics": readings},
		}
	}
	return []map[string]any{makeRow("sandbox-a", "task", []map[string]any{value, value}), makeRow("sandbox-b", "evaluation", nil), makeRow("sandbox-a", "task", []map[string]any{value})}
}

func writeProviderFixture(t *testing.T, root string, rows []map[string]any) {
	t.Helper()
	base := filepath.Join(root, "occ")
	writeTraceEvents(t, filepath.Join(base, "sandbox-metrics.jsonl"), rows)
	successful := 0
	for _, r := range rows {
		if r["error"] == nil || r["error"] == "" {
			successful++
		}
	}
	writeSummaryJSON(t, base, "cloud-observation.json", map[string]any{"sandbox_sdk": map[string]any{"status": "succeeded", "sample_count": successful}})
}

func TestResourceProviderPreservesRepeatedAndEmptyReceipts(t *testing.T) {
	root, directory := resourceFixture(t)
	writeProviderFixture(t, root, providerFixtureRows())
	snapshot, err := BuildSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	streams := snapshot.Resources.Streams
	if len(streams) != 2 || len(streams[0].Samples) != 3 || len(streams[1].Samples) != 1 {
		t.Fatal("provider receipts deduplicated or empty RPC hidden")
	}
	task, empty := streams[0], streams[1].Samples[0]
	if got := task.ProviderCounts; got == nil || *got != (ProviderResourceCounts{RPCCount: 2, ReturnedReadingCount: 3, DuplicateReadingCount: 2, UniqueReadingCount: 1}) {
		t.Fatalf("legacy receipts were mistaken for independent points: %+v", got)
	}
	if empty.Status != "no_readings" || len(empty.Values) != 0 || empty.ObservedAtUTC != nil {
		t.Fatal("empty verifier reading filled with zero")
	}
	if task.Samples[0].ID == task.Samples[1].ID || task.Samples[0].ObservedAtUTC.Equal(*task.Samples[0].RequestedAtUTC) {
		t.Fatal("receipt identity or provider time conflated with request time")
	}
	for _, sample := range task.Samples {
		if sample.TimeStatus != "verified" || !sample.ObservedAtUTC.Equal(summaryAnchor.Add(time.Second)) {
			t.Fatal("native provider timestamp changed")
		}
		for _, v := range sample.Values {
			if v.Name == "mem_used" && (!v.ExportEligible || v.Unit != "bytes") {
				t.Fatal("documented E2B byte unit lost")
			}
		}
	}
	raw, _ := json.Marshal(snapshot)
	if strings.Contains(string(raw), "SECRET_") {
		t.Fatal("arbitrary provider metadata copied")
	}
}

func compactProviderRow(owner string, readings ...any) map[string]any {
	return map[string]any{"schema_version": 2, "run_id": "run", "occurrence_id": "occ", "sandbox_handle": owner, "purpose": "task",
		"requested_at_utc": summaryAnchor.Add(10 * time.Second), "client_rpc_duration_ns": 1000000,
		"snapshot": map[string]any{"source": "e2b_sdk", "nominal_interval_seconds": 5, "metrics": readings}}
}

func compactReadingReference(line uint64, index int) any {
	return ProviderReadingReference{SourceLine: line, ReadingIndex: index}
}

func TestResourceProviderSchema2UniqueSamplesAndExactCounts(t *testing.T) {
	root, directory := resourceFixture(t)
	a := map[string]any{"timestamp": "2026-10-22T00:00:10Z", "mem_used": 100}
	changed := map[string]any{"timestamp": "2026-10-22T00:00:10Z", "mem_used": 200}
	later := map[string]any{"timestamp": "2026-10-22T00:00:11Z", "mem_used": 100}
	lateArrival := map[string]any{"timestamp": "2026-10-22T00:00:05Z", "mem_used": 300}
	unkeyed := map[string]any{"mem_used": 100}
	invalidTime := map[string]any{"timestamp": "invalid", "mem_used": 100}
	rows := []map[string]any{
		compactProviderRow("sandbox-a", a, a, unkeyed, invalidTime),
		compactProviderRow("sandbox-a", nil, changed, later, lateArrival, unkeyed, invalidTime),
		compactProviderRow("sandbox-b", a),
		compactProviderRow("sandbox-a", nil, nil),
		compactProviderRow("sandbox-a"),
		compactProviderRow("sandbox-a"),
	}
	rows[1]["reading_references"] = []any{compactReadingReference(1, 0), nil, nil, nil, nil, nil}
	rows[3]["reading_references"] = []any{compactReadingReference(1, 0), compactReadingReference(2, 1)}
	rows[5]["error"] = "provider failed"
	writeProviderFixture(t, root, rows)
	snapshot, err := BuildSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	task, other := snapshot.Resources.Streams[0], snapshot.Resources.Streams[1]
	want := ProviderResourceCounts{RPCCount: 5, ReturnedReadingCount: 12, DuplicateReadingCount: 4, UniqueReadingCount: 4, UnkeyedReadingCount: 4}
	if task.ProviderCounts == nil || *task.ProviderCounts != want || len(task.Samples) != 10 {
		t.Fatalf("compacted evidence miscounted: %+v, samples=%d", task.ProviderCounts, len(task.Samples))
	}
	if other.ProviderCounts.UniqueReadingCount != 1 || len(other.Samples) != 1 {
		t.Fatal("same values from different sandboxes merged")
	}
	if task.Samples[0].SourceLine != 1 || task.Samples[0].ReadingIndex != 0 || task.Samples[3].SourceLine != 2 || task.Samples[3].ReadingIndex != 1 ||
		task.Samples[8].Status != "no_readings" || task.Samples[9].Status != "failed" {
		t.Fatal("first source, changed reading, empty/error receipt or response order lost")
	}
	if task.Samples[5].ProviderTimestamp == nil || string(task.Samples[5].ProviderTimestamp) != `"2026-10-22T00:00:05Z"` {
		t.Fatal("late-arriving history was discarded or reordered")
	}
	raw, _ := json.Marshal(snapshot)
	path := filepath.Join(root, "compact-snapshot.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSnapshot(path)
	if err != nil || !reflect.DeepEqual(loaded.Resources, snapshot.Resources) {
		t.Fatalf("schema-2 resource snapshot roundtrip: %v", err)
	}
	loaded.Resources.Streams[0].ProviderCounts.DuplicateReadingCount++
	loaded.SnapshotID = ""
	changedCounts, _ := json.Marshal(loaded)
	loaded.SnapshotID = stableAnalysisID("snapshot", string(changedCounts))
	if ValidateSnapshot(loaded) == nil {
		t.Fatal("inconsistent counts accepted even with a recomputed snapshot identity")
	}
}

func TestResourceProviderSchema2RejectsInvalidReferences(t *testing.T) {
	for _, name := range []string{"forward", "same_row", "wrong_index", "negative_index", "missing_index", "missing_reference", "other_owner", "other_source", "unkeyed_original", "failed_original", "reference_chain", "inline_with_reference", "wrong_length", "legacy_reference"} {
		t.Run(name, func(t *testing.T) {
			root, directory := resourceFixture(t)
			value := map[string]any{"timestamp": "2026-10-22T00:00:10Z", "mem_used": 100}
			rows := []map[string]any{compactProviderRow("sandbox-a", value), compactProviderRow("sandbox-a", nil)}
			rows[1]["reading_references"] = []any{compactReadingReference(1, 0)}
			switch name {
			case "forward":
				rows[1]["reading_references"] = []any{compactReadingReference(3, 0)}
			case "same_row":
				rows[1]["reading_references"] = []any{compactReadingReference(2, 0)}
			case "wrong_index":
				rows[1]["reading_references"] = []any{compactReadingReference(1, 1)}
			case "negative_index":
				rows[1]["reading_references"] = []any{compactReadingReference(1, -1)}
			case "missing_index":
				rows[1]["reading_references"] = []any{map[string]any{"source_line": 1}}
			case "missing_reference":
				delete(rows[1], "reading_references")
			case "other_owner":
				rows[1]["sandbox_handle"] = "sandbox-b"
			case "other_source":
				rows[1]["snapshot"].(map[string]any)["source"] = "agentsphere_gateway_envd_metrics"
			case "unkeyed_original":
				delete(value, "timestamp")
			case "failed_original":
				rows[0]["error"] = "failed"
			case "reference_chain":
				rows = append(rows, compactProviderRow("sandbox-a", nil))
				rows[2]["reading_references"] = []any{compactReadingReference(2, 0)}
			case "inline_with_reference":
				rows[1]["snapshot"].(map[string]any)["metrics"] = []any{value}
			case "wrong_length":
				rows[1]["reading_references"] = []any{compactReadingReference(1, 0), nil}
			case "legacy_reference":
				rows[1]["schema_version"] = 1
			}
			writeProviderFixture(t, root, rows)
			snapshot, err := BuildSnapshot(directory)
			if err == nil || snapshot.Resources == nil || snapshot.Resources.Streams[0].IntegrityStatus != "incomplete" {
				t.Fatal("invalid reference was accepted as observed resource evidence")
			}
		})
	}
}

func TestResourceHuaweiUnknownClockAndUnitsStayUnknown(t *testing.T) {
	root, directory := resourceFixture(t)
	rows := providerFixtureRows()[:1]
	rows[0]["snapshot"] = map[string]any{"source": "agentsphere_gateway_envd_metrics", "metrics": []map[string]any{{"ts": 1792627200, "cpu_count": 2, "cpu_used_pct": 1.5, "mem_used_mib": 12, "mem_used": 12582912}}, "units": map[string]string{"cpu_count": "count", "cpu_used_pct": "percent", "mem_used_mib": "MiB"}}
	writeProviderFixture(t, root, rows)
	snapshot, err := BuildSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	sample := snapshot.Resources.Streams[0].Samples[0]
	if sample.TimeStatus != "unverified" || sample.ObservedAtUTC != nil || string(sample.ProviderTimestamp) != "1792627200" || sample.RequestedAtUTC == nil {
		t.Fatal("unknown ts mapped onto a fake sample epoch")
	}
	for _, v := range sample.Values {
		if v.Name == "mem_used" && (v.ExportEligible || v.Unit != "unknown") {
			t.Fatal("undocumented byte unit guessed")
		}
		if v.Name == "mem_used_mib" && (!v.ExportEligible || v.Unit != "MiB") {
			t.Fatal("documented unit unnecessarily dropped")
		}
	}
}

func TestResourceProviderFailuresAndMissingTime(t *testing.T) {
	for _, name := range []string{"missing_time", "failed_rpc", "wrong_identity", "bad_count"} {
		t.Run(name, func(t *testing.T) {
			root, directory := resourceFixture(t)
			rows := providerFixtureRows()[:1]
			switch name {
			case "missing_time":
				delete(rows[0]["snapshot"].(map[string]any)["metrics"].([]map[string]any)[0], "timestamp")
			case "failed_rpc":
				rows[0]["error"] = "SECRET_RPC_FAILURE"
				rows[0]["snapshot"] = map[string]any{"source": "", "metrics": nil}
			case "wrong_identity":
				rows[0]["run_id"] = "other"
			}
			writeProviderFixture(t, root, rows)
			if name == "bad_count" {
				writeSummaryJSON(t, filepath.Join(root, "occ"), "cloud-observation.json", map[string]any{"sandbox_sdk": map[string]any{"status": "succeeded", "sample_count": 99}})
			}
			snapshot, err := BuildSnapshot(directory)
			if name == "wrong_identity" || name == "bad_count" {
				if err == nil {
					t.Fatal("invalid provider scope/count accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			sample := snapshot.Resources.Streams[0].Samples[0]
			if sample.ObservedAtUTC != nil || sample.TimeStatus == "verified" {
				t.Fatal("missing timestamp filled from controller time")
			}
			if name == "failed_rpc" && (sample.Status != "failed" || len(sample.Values) != 0) {
				t.Fatal("failed RPC replaced with resource zero")
			}
			raw, _ := json.Marshal(snapshot)
			if strings.Contains(string(raw), "SECRET_") {
				t.Fatal("private failure copied")
			}
		})
	}
}

func TestResourceParsingRequiresRunIdentity(t *testing.T) {
	root, directory := traceFixture(t)
	resourceControllerFixture(t, root)
	snapshot, err := BuildSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Resources != nil {
		t.Fatal("controller resource read without verified run-result")
	}
	writeSummaryJSON(t, root, "run-result.json", map[string]any{"run_id": "other", "attempts": []traceAttempt{}})
	snapshot, err = BuildSnapshot(directory)
	if err == nil || snapshot.Resources != nil {
		t.Fatal("resources accepted from mismatching run")
	}
}

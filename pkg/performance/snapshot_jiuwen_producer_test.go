package performance

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func jiuwenProducerSnapshotFixture(t *testing.T, root string, index map[string]any, change func(map[string]any)) {
	t.Helper()
	directory := filepath.Join(root, "occ/harness/native-producer")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	var ledger []byte
	sequence := 0
	for _, span := range index["spans"].([]map[string]any) {
		for _, event := range []string{"start", "end"} {
			sequence++
			row := map[string]any{"schema_version": 1, "sequence": sequence, "event": event, "trace_id": span["trace_id"], "span_id": span["span_id"], "start_time_unix_nano": span["start_time_unix_nano"]}
			if event == "end" {
				row["end_time_unix_nano"], row["status_code"] = span["end_time_unix_nano"], span["native_status_code"]
			}
			raw, _ := json.Marshal(row)
			ledger = append(ledger, append(raw, '\n')...)
		}
	}
	ledger = append(ledger, []byte(`{"schema_version":1,"sequence":5,"event":"processor_shutdown"}`+"\n")...)
	status := map[string]any{"schema_version": 1, "protocol": "jiuwen-native-span-audit-v1", "run_id": "run", "task_id": "occ", "pid": 42,
		"registered_before_provider": true, "attached": true, "status": "complete", "started_spans": 2, "ended_spans": 2,
		"written_records": 5, "written_bytes": len(ledger), "write_failures": 0, "dropped_records": 0, "limit_exceeded": false,
		"force_flush_callbacks": 0, "processor_shutdown_observed": true, "extension_shutdown_started": true, "extension_shutdown_returned": true,
		"ledger_sha256": fmt.Sprintf("%x", sha256.Sum256(ledger)), "errors": []string{}}
	if change != nil {
		change(status)
	}
	statusBytes, _ := json.Marshal(status)
	files := []map[string]any{}
	for _, source := range []struct {
		name string
		data []byte
	}{{"status.json", statusBytes}, {"events.jsonl", ledger}} {
		if err := os.WriteFile(filepath.Join(directory, source.name), source.data, 0600); err != nil {
			t.Fatal(err)
		}
		hash := fmt.Sprintf("%x", sha256.Sum256(source.data))
		files = append(files, map[string]any{"name": source.name, "size": len(source.data), "retained_bytes": len(source.data), "source_sha256": hash, "retained_sha256": hash})
	}
	index["native_stop"] = map[string]bool{"exit_observed": true}
	index["producer_audit"] = map[string]any{"status": "complete", "scope": "sampled_sdk_spans", "started_spans": 2, "ended_spans": 2, "exported_spans": 2, "missing_exports": 0, "unexpected_exports": 0, "files": files}
	writeSummaryJSON(t, filepath.Dir(directory), "native-spans-index.json", index)
}

func TestJiuwenProducerDenominatorIsScopedAndFrozen(t *testing.T) {
	root, directory, index := jiuwenSnapshotFixture(t)
	jiuwenProducerSnapshotFixture(t, root, index, nil)
	snapshot, err := BuildSnapshot(directory)
	if err != nil || snapshot.IntegrityStatus != "complete" || snapshot.CoverageStatus != "unknown" || len(snapshot.NativeSpans) != 2 {
		t.Fatalf("scoped audit failed: %+v %v", snapshot.Coverage, err)
	}
	found := false
	for _, coverage := range snapshot.Coverage {
		switch coverage.Source {
		case "jiuwen_sampled_sdk_span_exports":
			found = coverage.Status == "complete" && coverage.ExpectedEvents != nil && *coverage.ExpectedEvents == 2 && coverage.ObservedEvents == 2 && coverage.Started == 2 && coverage.Paired == 2 && coverage.WriteFailedEvents == nil
		case "jiuwen_native_spans":
			if coverage.Status != "unknown" || !strings.Contains(strings.Join(coverage.Missing, ","), "guest_process_exit_and_pipe_drain") || !strings.Contains(strings.Join(coverage.Missing, ","), "unsampled_and_uninstrumented_operations") {
				t.Fatal("SDK denominator removed native process/coverage gaps")
			}
		}
	}
	if !found {
		t.Fatal("sampled SDK export denominator missing")
	}
	writeSummaryJSON(t, root, "producer-snapshot.json", snapshot)
	loaded, err := LoadSnapshot(filepath.Join(root, "producer-snapshot.json"))
	if err != nil || !reflect.DeepEqual(loaded.Coverage, snapshot.Coverage) {
		t.Fatal("producer coverage failed snapshot round trip")
	}
}

func TestJiuwenProducerSourceStatusAndCountMismatchRejectsCompleteness(t *testing.T) {
	for _, scenario := range []string{"source_hash", "source_missing", "source_path", "wrong_run", "wrong_task", "unattached", "incomplete", "counter", "ledger_hash", "write_failure", "omitted_write_counter", "dropped", "unobserved_exit", "wrong_scope", "missing_export", "count_mismatch"} {
		t.Run(scenario, func(t *testing.T) {
			root, directory, index := jiuwenSnapshotFixture(t)
			jiuwenProducerSnapshotFixture(t, root, index, func(status map[string]any) {
				switch scenario {
				case "wrong_run":
					status["run_id"] = "other"
				case "wrong_task":
					status["task_id"] = "other"
				case "unattached":
					status["attached"] = false
				case "incomplete":
					status["status"] = "incomplete"
				case "counter":
					status["started_spans"] = 3
				case "ledger_hash":
					status["ledger_sha256"] = strings.Repeat("0", 64)
				case "write_failure":
					status["write_failures"] = 1
				case "omitted_write_counter":
					delete(status, "write_failures")
				case "dropped":
					status["dropped_records"] = 1
				}
			})
			audit := index["producer_audit"].(map[string]any)
			switch scenario {
			case "source_hash":
				audit["files"].([]map[string]any)[0]["source_sha256"] = strings.Repeat("0", 64)
			case "source_path":
				audit["files"].([]map[string]any)[0]["name"] = "../status.json"
			case "source_missing":
				if err := os.Remove(filepath.Join(root, "occ/harness/native-producer/events.jsonl")); err != nil {
					t.Fatal(err)
				}
			case "unobserved_exit":
				index["native_stop"] = map[string]bool{"exit_observed": false}
			case "wrong_scope":
				audit["scope"] = "all_native_work"
			case "missing_export":
				audit["missing_exports"] = 1
			case "count_mismatch":
				audit["exported_spans"] = 3
			}
			writeSummaryJSON(t, filepath.Join(root, "occ/harness"), "native-spans-index.json", index)
			snapshot, err := BuildSnapshot(directory)
			if err == nil || snapshot.IntegrityStatus != "incomplete" || len(snapshot.NativeSpans) != 0 {
				t.Fatal("invalid producer audit accepted")
			}
		})
	}
}

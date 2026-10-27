package performance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func cgroupFixture(t *testing.T, root string) []map[string]any {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "occ", "harness"), 0700); err != nil {
		t.Fatal(err)
	}
	rows := []map[string]any{}
	for index, boundary := range []string{"start", "runtime_start", "runtime_end", "stopped"} {
		group := "/sys/fs/cgroup/aries-test"
		row := map[string]any{"schema_version": 1, "run_id": "run", "task_id": "occ", "enabled": true, "boundary": boundary, "measurement_scope": "agent_process_group_and_descendants", "cgroup_path": group, "status": "collected", "requested_at_utc": summaryAnchor, "client_rpc_duration_ns": 500,
			"reading": map[string]any{"cgroup_path": group, "cgroup_inode": "123", "guest_monotonic_started_ns": fmt.Sprint(1000000 + index*1000), "guest_monotonic_finished_ns": fmt.Sprint(1000100 + index*1000), "observed_at_utc": summaryAnchor.Add(1000), "files": map[string]any{
				"cpu.stat":       map[string]any{"status": "collected", "raw": "usage_usec 9007199254741\nuser_usec 2\nsystem_usec 3\n"},
				"memory.current": map[string]any{"status": "collected", "raw": "4096\n"}, "memory.peak": map[string]any{"status": "collected", "raw": "8192\n"},
				"io.stat": map[string]any{"status": "collected", "raw": "8:0 rbytes=1024 wbytes=512 rios=2 wios=1\n253:0 rbytes=1024 wbytes=512 rios=2 wios=1\n"}}}}
		writeSummaryJSON(t, root, filepath.Join("occ", "harness", "cgroup-resources."+boundary+".json"), row)
		rows = append(rows, row)
	}
	return rows
}

func TestCgroupResourcesKeepUnitsOwnersClockBoundaryAndMetrics(t *testing.T) {
	root, directory := resourceFixture(t)
	cgroupFixture(t, root)
	snapshot, err := BuildSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Resources.Streams) != 3 {
		t.Fatal("group and two distinct devices required")
	}
	group := snapshot.Resources.Streams[0]
	if len(group.Samples) != 4 || group.Samples[3].Boundary != "stopped" || *group.Samples[1].SamplingStartedOffsetNS != 1000 || len(group.Delta) != 0 {
		t.Fatal("boundaries or clock meaning changed")
	}
	found := map[string]string{}
	for _, v := range group.Samples[0].Values {
		found[v.Name] = string(v.Value)
	}
	if found["cpu_usage_ns"] != "9007199254741000" || found["group_lifetime_memory_peak_bytes"] != "8192" || found["memory_current_bytes"] != "4096" {
		t.Fatalf("unit precision: %+v", found)
	}
	for i, stream := range snapshot.Resources.Streams {
		if i > 0 && (stream.OwnerID == group.OwnerID || stream.ClockDomain != group.ClockDomain || len(stream.Samples[0].Values) != 4) {
			t.Fatal("device ownership changed")
		}
		projected, err := ProjectResourceMetrics(snapshot, stream.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(projected.Request.ResourceMetrics) == 0 {
			t.Fatal("existing metrics adapter did not export actual values")
		}
	}
	raw, _ := json.Marshal(snapshot)
	file := filepath.Join(root, "snapshot.json")
	if err := os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadSnapshot(file)
	if err != nil || reloaded.SnapshotID != snapshot.SnapshotID {
		t.Fatalf("boundary roundtrip: %v", err)
	}
}

func TestCgroupResourcesCapabilitiesAndFailuresNeverBecomeZero(t *testing.T) {
	for _, test := range []struct {
		name, status string
		incomplete   bool
	}{{"unsupported", "unsupported", false}, {"empty_io", "empty", false}, {"permission", "failed", true}} {
		t.Run(test.name, func(t *testing.T) {
			root, directory := resourceFixture(t)
			rows := cgroupFixture(t, root)
			for _, row := range rows {
				files := row["reading"].(map[string]any)["files"].(map[string]any)
				files["memory.peak"] = map[string]string{"status": "unsupported", "error": "missing_file"}
				files["io.stat"] = map[string]string{"status": test.status, "raw": "", "error": "example"}
				writeSummaryJSON(t, root, filepath.Join("occ", "harness", "cgroup-resources."+row["boundary"].(string)+".json"), row)
			}
			snapshot, err := BuildSnapshot(directory)
			if (err != nil) != test.incomplete {
				t.Fatalf("wrong completeness: %v", err)
			}
			group := snapshot.Resources.Streams[0]
			if len(snapshot.Resources.Streams) != 1 || len(group.Samples[0].Unavailable) != 2 {
				t.Fatal("missing data filled")
			}
			for _, v := range group.Samples[0].Values {
				if strings.Contains(v.Name, "peak") || strings.Contains(v.Name, "io_") {
					t.Fatal("missing value became zero")
				}
			}
		})
	}
}

func TestCgroupResourcesRejectIdentityClockAndCounterCorruption(t *testing.T) {
	for _, field := range []string{"cgroup_inode", "guest_monotonic_finished_ns", "cpu.stat", "missing_stopped"} {
		t.Run(field, func(t *testing.T) {
			root, directory := resourceFixture(t)
			rows := cgroupFixture(t, root)
			row := rows[2]
			reading := row["reading"].(map[string]any)
			switch field {
			case "cgroup_inode":
				reading[field] = "999"
			case "guest_monotonic_finished_ns":
				reading[field] = "1"
			case "cpu.stat":
				reading["files"].(map[string]any)[field] = map[string]string{"status": "collected", "raw": "usage_usec -1\n"}
			case "missing_stopped":
				if err := os.Remove(filepath.Join(root, "occ/harness/cgroup-resources.stopped.json")); err != nil {
					t.Fatal(err)
				}
			}
			writeSummaryJSON(t, root, "occ/harness/cgroup-resources.runtime_end.json", row)
			if _, err := BuildSnapshot(directory); err == nil {
				t.Fatal("invalid resource evidence accepted")
			}
		})
	}
}

func TestCgroupCumulativeRegressionRemainsIncompletePerOwner(t *testing.T) {
	for _, test := range []struct{ name, file, raw string }{
		{"cpu", "cpu.stat", "usage_usec 9\nuser_usec 1\nsystem_usec 2\n"},
		{"io_bytes", "io.stat", "8:0 rbytes=1 wbytes=512 rios=2 wios=1\n253:0 rbytes=1024 wbytes=512 rios=2 wios=1\n"},
		{"io_operations", "io.stat", "8:0 rbytes=1024 wbytes=512 rios=1 wios=1\n253:0 rbytes=1024 wbytes=512 rios=2 wios=1\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root, directory := resourceFixture(t)
			rows := cgroupFixture(t, root)
			rows[2]["reading"].(map[string]any)["files"].(map[string]any)[test.file] = map[string]string{"status": "collected", "raw": test.raw}
			writeSummaryJSON(t, root, "occ/harness/cgroup-resources.runtime_end.json", rows[2])
			snapshot, err := BuildSnapshot(directory)
			if err == nil || snapshot.IntegrityStatus != "incomplete" {
				t.Fatal("counter rollback remained complete")
			}
			found := false
			for _, stream := range snapshot.Resources.Streams {
				if len(stream.Delta) != 0 {
					t.Fatal("regression created a delta")
				}
				if strings.Contains(strings.Join(stream.Diagnostics, " "), "cumulative counter decreased") {
					found = true
					if stream.IntegrityStatus != "incomplete" {
						t.Fatal("regressed owner remained complete")
					}
					if _, err := ProjectResourceMetrics(snapshot, stream.ID); err == nil {
						t.Fatal("regressed owner exported successfully")
					}
				}
			}
			if !found {
				t.Fatal("no exact owner regression diagnostic")
			}
		})
	}
	// Device identities and memory gauges must not be compared with one another.
	root, directory := resourceFixture(t)
	rows := cgroupFixture(t, root)
	for i, row := range rows {
		files := row["reading"].(map[string]any)["files"].(map[string]any)
		files["memory.current"] = map[string]string{"status": "collected", "raw": fmt.Sprint(4096 - i*10)}
		files["io.stat"] = map[string]string{"status": "collected", "raw": fmt.Sprintf("8:0 rbytes=%d rios=%d\n253:0 rbytes=%d rios=%d\n", 1000+i, 100+i, 1+i, 1+i)}
		writeSummaryJSON(t, root, "occ/harness/cgroup-resources."+row["boundary"].(string)+".json", row)
	}
	if _, err := BuildSnapshot(directory); err != nil {
		t.Fatalf("distinct device or gauge miscompared: %v", err)
	}
}

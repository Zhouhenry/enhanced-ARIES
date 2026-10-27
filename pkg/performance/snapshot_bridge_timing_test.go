package performance

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
)

func TestBridgePreparationReachesSnapshotTracesAndMetrics(t *testing.T) {
	for _, outcome := range []string{"succeeded", "failed", "canceled", "historical"} {
		t.Run(outcome, func(t *testing.T) {
			status := outcome
			if outcome == "historical" {
				status = "succeeded"
			}
			events := summaryPair("bridge", "task", "task", status, 6000)
			for i := range events {
				events[i].Operation, events[i].OccurrenceID = "bridge.register", "occ"
			}
			directory := summaryRecording(t, events)
			root := t.TempDir()
			if err := os.Rename(directory, filepath.Join(root, "performance")); err != nil {
				t.Fatal(err)
			}
			directory = filepath.Join(root, "performance")
			queue, prepare, harness := time.Duration(0), 100*time.Nanosecond, 200*time.Nanosecond
			timing := &core.TaskTiming{SandboxPrepareDuration: &prepare, HarnessStartDuration: &harness}
			if outcome != "historical" {
				// The outer raw span includes recorder overhead; do not copy it into the call timer.
				bridge := 5000 * time.Nanosecond
				timing.BridgePrepareDuration = &bridge
			}
			summary := &core.AttemptSummary{Planned: 1, Admitted: 1, Started: 1,
				WindowStartedAt: summaryAnchor, AdmissionFinishedAt: summaryAnchor,
				WindowFinishedAt: summaryAnchor.Add(time.Second), ObservationDuration: time.Second}
			switch status {
			case "succeeded":
				summary.Succeeded = 1
			case "failed":
				summary.Failed = 1
			case "canceled":
				summary.Canceled = 1
			}
			writeSummaryJSON(t, root, "run-result.json", map[string]any{"run_id": "run", "attempt_summary": summary,
				"attempts": []traceAttempt{{AttemptID: "run/occ", OccurrenceID: "occ", Status: status, Timing: timing, QueueDuration: &queue}}})
			snapshot, err := BuildSnapshot(directory)
			if err != nil {
				t.Fatal(err)
			}
			stages := snapshot.Attempts[0].Stages
			var names []string
			for _, stage := range stages {
				names = append(names, stage.Name)
			}
			if !reflect.DeepEqual(names, []string{"queue", "sandbox_create", "workspace_prepare", "bridge_prepare", "harness_start", "runtime_execution", "stop_and_revoke", "verifier", "cleanup"}) ||
				*stages[2].DurationNS != 100 || *stages[4].DurationNS != 200 {
				t.Fatal("historical stage names, order or duration meanings changed")
			}
			bridge := stages[3]
			if bridge.Scope != "tool_bridge_start" || bridge.EvidenceRef == "" {
				t.Fatal("bridge preparation lost its scope or source")
			}
			if outcome == "historical" {
				if bridge.DurationNS != nil || bridge.Status != "not_collected" {
					t.Fatal("missing historical call timing reconstructed from raw span")
				}
			} else if bridge.DurationNS == nil || *bridge.DurationNS != 5000 || bridge.Status != "duration_only" {
				t.Fatal("measured bridge preparation absent or changed")
			}
			if len(snapshot.Summary.Operations) != 1 || snapshot.Summary.Operations[0].Operation != "bridge.register" ||
				len(snapshot.Intervals) != 1 || snapshot.Intervals[0].Name != "bridge.register" || snapshot.Intervals[0].DurationNS != 6000 {
				t.Fatal("existing summary and raw bridge interval changed")
			}
			trace, err := ProjectTrace(snapshot)
			if err != nil || trace.Metadata.SnapshotID != snapshot.SnapshotID {
				t.Fatal("bridge evidence did not reach Perfetto projection", err)
			}
			otlp, err := ProjectOTLP(snapshot)
			if err != nil || otlp.Manifest.SnapshotID != snapshot.SnapshotID {
				t.Fatal("bridge evidence did not reach OTLP trace projection", err)
			}
			metrics, err := ProjectMetrics(snapshot)
			if err != nil || metrics.Manifest.SnapshotID != snapshot.SnapshotID {
				t.Fatal("bridge evidence did not reach OTLP metrics projection", err)
			}
			found := false
			for _, group := range metrics.Manifest.Histograms {
				if group.Phase == "bridge_prepare" {
					found = true
					if group.Outcome != status || group.Scope != "tool_bridge_start" || group.Count != 1 || group.SumNS != 5000 {
						t.Fatal("bridge histogram changed duration or attempt outcome")
					}
				}
			}
			if found != (outcome != "historical") {
				t.Fatal("bridge histogram missing or historical absence became zero")
			}
			if outcome == "historical" {
				for _, missing := range metrics.Manifest.NotCollected {
					if missing.Phase == "bridge_prepare" && missing.Reason == "stage_duration_not_collected" {
						return
					}
				}
				t.Fatal("historical bridge absence was not reported")
			}
		})
	}
}

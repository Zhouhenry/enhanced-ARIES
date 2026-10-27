package performance

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
)

func jiuwenProcessMetricsWindow(t *testing.T, directory string) {
	t.Helper()
	summary := &core.AttemptSummary{Planned: 1, Admitted: 1, Started: 1, Executed: 1, Completed: 1, WindowStartedAt: summaryAnchor, WindowFinishedAt: summaryAnchor.Add(time.Second), ObservationDuration: time.Second}
	writeSummaryJSON(t, filepath.Dir(directory), "run-result.json", map[string]any{"run_id": "run", "attempt_summary": summary, "attempts": []traceAttempt{{AttemptID: "run/occ", OccurrenceID: "occ", AgentSessionID: "jiuwen-session"}}})
}

func TestJiuwenProcessMetricsABSeparateIntervalsAndExactBuckets(t *testing.T) {
	for _, owner := range []string{"bridge", "harness"} {
		t.Run(owner, func(t *testing.T) {
			base, directory, index, rows := jiuwenProcessFixture(t, owner)
			jiuwenProcessMetricsWindow(t, directory)
			// Put the three real measurements in distinct buckets, including an exact bound.
			for i, timestamp := range []int{1100, 1_001_100, 2_001_100, 3_001_100, 4_001_100, 11_001_101, 12_001_101, 111_001_102} {
				rows[i]["monotonic_ns"] = timestamp
			}
			writeJiuwenProcessFixture(t, base, index, rows)
			snapshot, err := BuildSnapshot(directory)
			if err != nil {
				t.Fatal(err, snapshot.Diagnostics)
			}
			out, err := ProjectMetrics(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			metric := findMetric(t, out, "aries.jiuwen.process.duration")
			if metric.Unit != "s" || metric.Histogram == nil || metric.Histogram.AggregationTemporality != 2 || len(metric.Histogram.DataPoints) != 3 {
				t.Fatal("process durations were merged or observation container was counted", metric)
			}
			intervals := jiuwenProcessIntervals(snapshot)
			expected := map[string]struct {
				duration int64
				bucket   int
			}{
				"jiuwen.process.creation":            {1_000_000, 0},
				"jiuwen.process.command":             {10_000_001, 2},
				"jiuwen.process.exit_to_pipe_closed": {100_000_001, 3},
			}
			groups := map[string]MetricsHistogramEvidence{}
			remoteCount, remoteSum := uint64(0), int64(0)
			for _, group := range out.Manifest.Histograms {
				switch group.Metric {
				case "aries.jiuwen.process.duration":
					if _, duplicate := groups[group.Phase]; duplicate {
						t.Fatal("duplicate process histogram phase", group)
					}
					groups[group.Phase] = group
				case "aries.jiuwen.remote.duration":
					remoteCount += group.Count
					remoteSum += group.SumNS
				}
			}
			if len(groups) != len(expected) {
				t.Fatal("process evidence includes an overlapping container or misses a phase", groups)
			}
			bounds := []int64{1_000_000, 10_000_000, 100_000_000, 1_000_000_000, 5_000_000_000, 10_000_000_000, 30_000_000_000, 60_000_000_000, 300_000_000_000}
			seen := map[string]bool{}
			for _, point := range metric.Histogram.DataPoints {
				phase := metricLabel(point.Attributes, "phase")
				want, ok := expected[phase]
				if !ok || seen[phase] {
					t.Fatal("unexpected or duplicate process phase", phase)
				}
				seen[phase] = true
				interval, group := intervals[phase], groups[phase]
				if interval.DurationNS != want.duration || metricLabel(point.Attributes, "outcome") != interval.Outcome || metricLabel(point.Attributes, "scope") != interval.Scope || group.Outcome != interval.Outcome || group.Scope != interval.Scope {
					t.Fatal("process phase, outcome or measurement scope changed", point, group)
				}
				if point.Count != "1" || group.Count != 1 || group.SumNS != want.duration || point.Sum != float64(want.duration)/1e9 || group.SumSeconds != point.Sum {
					t.Fatal("process count or nanosecond sum changed", point, group)
				}
				counts := make([]uint64, len(bounds)+1)
				wireCounts := make([]string, len(bounds)+1)
				for i := range wireCounts {
					wireCounts[i] = "0"
				}
				counts[want.bucket], wireCounts[want.bucket] = 1, "1"
				wireBounds := make([]float64, len(bounds))
				for i, bound := range bounds {
					wireBounds[i] = float64(bound) / 1e9
				}
				if !reflect.DeepEqual(group.BoundsNS, bounds) || !reflect.DeepEqual(group.BucketCounts, counts) || !reflect.DeepEqual(point.ExplicitBounds, wireBounds) || !reflect.DeepEqual(point.BucketCounts, wireCounts) {
					t.Fatal("process histogram bucket boundary changed", point, group)
				}
				member := MetricsHistogramMember{AttemptID: interval.AttemptID, Phase: interval.Name, Outcome: interval.Outcome, Scope: interval.Scope, DurationNS: want.duration, MeasurementStatus: "monotonic_interval", EvidenceRef: interval.EvidenceRefs[0], OperationID: interval.ID}
				if !reflect.DeepEqual(group.Members, []MetricsHistogramMember{member}) || member.OperationID == intervals["jiuwen.process.observation"].ID {
					t.Fatal("process histogram lost its exact independent member", group)
				}
			}
			wantRemoteCount, wantRemoteSum := uint64(0), int64(0)
			if owner == "bridge" {
				wantRemoteCount, wantRemoteSum = 9, 3870
				remote := findMetric(t, out, "aries.jiuwen.remote.duration")
				if remote.Histogram == nil || len(remote.Histogram.DataPoints) != 3 {
					t.Fatal("existing Provider/Bridge/worker histogram groups changed", remote)
				}
				for _, point := range remote.Histogram.DataPoints {
					if point.Count != "3" {
						t.Fatal("process measurements changed an existing remote wire count", point)
					}
				}
			}
			if remoteCount != wantRemoteCount || remoteSum != wantRemoteSum {
				t.Fatal("process samples changed the remote-layer denominator", remoteCount, remoteSum)
			}
		})
	}
}

func TestJiuwenProcessMetricsMissingBoundariesNeverCreateZeroSamples(t *testing.T) {
	for _, tc := range []struct {
		name            string
		events          []string
		started, exited int
		partial         bool
		want            map[string]int64
	}{
		{"missing_exit", []string{"spawn_requested", "connection_made", "spawn_return"}, 1, 0, true, map[string]int64{"jiuwen.process.creation": 100}},
		{"missing_pipe_closure", []string{"spawn_requested", "connection_made", "spawn_return", "child_exit"}, 1, 1, true, map[string]int64{"jiuwen.process.creation": 100, "jiuwen.process.command": 200}},
		{"creation_failed", []string{"spawn_requested", "spawn_failed"}, 0, 0, false, map[string]int64{}},
		{"pipes_closed_before_exit", []string{"spawn_requested", "connection_made", "spawn_return", "stdout_closed", "stderr_closed", "child_exit"}, 1, 1, false, map[string]int64{"jiuwen.process.creation": 100, "jiuwen.process.command": 400}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base, directory, index, _ := jiuwenProcessFixture(t, "harness")
			jiuwenProcessMetricsWindow(t, directory)
			rows := jiuwenProcessRows("harness", tc.events)
			counters := index["counters"].(map[string]any)
			counters["started"], counters["exited"] = tc.started, tc.exited
			counters["events_attempted"], counters["events_written"] = len(rows), len(rows)
			if tc.partial {
				index["pending_processes"] = 1
			}
			writeJiuwenProcessFixture(t, base, index, rows)
			snapshot, err := BuildSnapshot(directory)
			if (err != nil) != tc.partial {
				t.Fatal("process source integrity changed", err, snapshot.Diagnostics)
			}
			out, err := ProjectMetrics(snapshot)
			if (err != nil) != tc.partial || tc.partial && out.Manifest.Status != "INCOMPLETE" {
				t.Fatal("metrics hid incomplete process evidence", err, out.Manifest)
			}
			groups := map[string]int64{}
			for _, group := range out.Manifest.Histograms {
				if group.Metric == "aries.jiuwen.process.duration" {
					if group.Count != 1 || group.SumNS <= 0 || len(group.Members) != 1 {
						t.Fatal("missing process boundary became a zero sample", group)
					}
					groups[group.Phase] = group.SumNS
				}
			}
			if !reflect.DeepEqual(groups, tc.want) {
				t.Fatal("missing command or closure was fabricated", groups, tc.want)
			}
			metrics, points := 0, 0
			for _, resource := range out.Request.ResourceMetrics {
				for _, scope := range resource.ScopeMetrics {
					for _, metric := range scope.Metrics {
						if metric.Name != "aries.jiuwen.process.duration" {
							continue
						}
						metrics++
						if metric.Histogram == nil {
							t.Fatal("process metric lost histogram type", metric)
						}
						for _, point := range metric.Histogram.DataPoints {
							phase := metricLabel(point.Attributes, "phase")
							want, exists := tc.want[phase]
							if !exists || point.Count != "1" || point.Sum != float64(want)/1e9 {
								t.Fatal("wire histogram fabricated a missing stage", point)
							}
							points++
						}
					}
				}
			}
			if points != len(tc.want) || len(tc.want) == 0 && metrics != 0 || len(tc.want) != 0 && metrics != 1 {
				t.Fatal("empty process durations must be absent, not zero samples", metrics, points)
			}
		})
	}
}

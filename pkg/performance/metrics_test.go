package performance

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
)

func sealMetricsFixture(t *testing.T, snapshot AnalysisSnapshot) AnalysisSnapshot {
	t.Helper()
	snapshot.SnapshotID = ""
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.SnapshotID = stableAnalysisID("snapshot", string(data))
	return snapshot
}

func metricsFixture(t *testing.T) AnalysisSnapshot {
	t.Helper()
	start := time.Date(2026, 10, 25, 1, 2, 3, 123456789, time.UTC)
	rate := func(value float64) *float64 { return &value }
	stage := func(name string, ns int64) StageMeasurement {
		return StageMeasurement{Name: name, DurationNS: &ns, Status: "duration_only", Scope: "scope_" + name, EvidenceRef: "run-result"}
	}
	missing := func(name string) StageMeasurement {
		return StageMeasurement{Name: name, Status: "not_collected", Scope: "scope_" + name, EvidenceRef: "run-result"}
	}
	return sealMetricsFixture(t, AnalysisSnapshot{SchemaVersion: AnalysisSchemaVersion, ParserVersion: AnalysisParserVersion,
		RunID: "private-run", IntegrityStatus: "complete", CoverageStatus: "unknown",
		Sources: []EvidenceSource{{Ref: "run-result", File: "run-result.json", SHA256: strings.Repeat("c", 64)}},
		AttemptSummary: &core.AttemptSummary{Planned: 3, Admitted: 2, Started: 2, Executed: 2, Completed: 1, ValidatedSucceeded: 1,
			Succeeded: 1, Canceled: 1, NotStarted: 1, CleanupFailed: 0,
			WindowStartedAt: start, AdmissionFinishedAt: start.Add(8 * time.Second), WindowFinishedAt: start.Add(12 * time.Second),
			// The stored monotonic duration need not equal serialized wall time.
			ObservationDuration: 10 * time.Second, DrainDuration: 2 * time.Second,
			AttemptsPerSecond: rate(0.2), ExecutedPerSecond: rate(0.2), ExecutionThroughput: rate(0.1), ValidatedThroughput: rate(0.1)},
		Attempts: []AnalysisAttempt{
			{AttemptID: "private-run/one", Status: "succeeded", Stages: []StageMeasurement{stage("runtime_execution", 0), stage("queue", 1_000_000), missing("verifier")}},
			{AttemptID: "private-run/two", Status: "canceled", Stages: []StageMeasurement{stage("runtime_execution", 1_000_001), stage("verifier", 124309), missing("queue")}},
			{AttemptID: "private-run/three", Status: "not_started", Stages: []StageMeasurement{missing("queue"), missing("runtime_execution"), missing("verifier")}},
		}})
}

func findMetric(t *testing.T, export OTLPMetricsExport, name string) OTLPMetric {
	t.Helper()
	for _, resource := range export.Request.ResourceMetrics {
		for _, scope := range resource.ScopeMetrics {
			for _, metric := range scope.Metrics {
				if metric.Name == name {
					return metric
				}
			}
		}
	}
	t.Fatalf("metric %s missing", name)
	return OTLPMetric{}
}

func metricLabel(attributes []OTLPKeyValue, key string) string {
	for _, attribute := range attributes {
		if attribute.Key == key && attribute.Value.StringValue != nil {
			return *attribute.Value.StringValue
		}
	}
	return ""
}

func TestProjectMetricsPreservesOriginalAccountingWindowAndLabels(t *testing.T) {
	snapshot := metricsFixture(t)
	before, _ := json.Marshal(snapshot)
	result, err := ProjectMetrics(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(snapshot)
	if !bytes.Equal(before, after) {
		t.Fatal("input mutated")
	}
	if result.Manifest.Status != "PROJECTED" || result.Manifest.CoverageStatus != "unknown" || !reflect.DeepEqual(result.Manifest.OriginalSummary, snapshot.AttemptSummary) {
		t.Fatal("original accounting or quality changed")
	}
	if result.Manifest.MetricCount != 6 || result.Manifest.DataPointCount != 25 {
		t.Fatal("metric or point denominator changed")
	}
	start := strconv.FormatInt(snapshot.AttemptSummary.WindowStartedAt.UnixNano(), 10)
	end := strconv.FormatInt(snapshot.AttemptSummary.WindowFinishedAt.UnixNano(), 10)
	if result.Manifest.Window.StartTimeUnixNano != start || result.Manifest.Window.TimeUnixNano != end || result.Manifest.Window.ObservationDurationNS != int64(10*time.Second) {
		t.Fatal("window fabricated or monotonic duration recalculated")
	}
	for _, expectation := range []struct {
		name, label string
		counts      map[string]string
	}{
		{"aries.attempt.lifecycle", "phase", map[string]string{"planned": "3", "admitted": "2", "started": "2", "executed": "2", "completed": "1", "verified_succeeded": "1"}},
		{"aries.attempt.outcomes", "outcome", map[string]string{"succeeded": "1", "failed": "0", "blocked": "0", "canceled": "1", "timed_out": "0", "unknown": "0", "not_started": "1", "loadgen_dropped": "0"}},
	} {
		metric := findMetric(t, result, expectation.name)
		if metric.Sum.AggregationTemporality != 2 || !metric.Sum.IsMonotonic || metric.Unit != "" {
			t.Fatal("count wire semantics wrong")
		}
		if len(metric.Sum.DataPoints) != len(expectation.counts) {
			t.Fatal("count lost")
		}
		for _, point := range metric.Sum.DataPoints {
			if point.AsInt == nil || *point.AsInt != expectation.counts[metricLabel(point.Attributes, expectation.label)] || point.StartTimeUnixNano != start || point.TimeUnixNano != end {
				t.Fatal("original count/window changed")
			}
		}
	}
	throughput := findMetric(t, result, "aries.attempt.throughput")
	if throughput.Unit != "1/s" {
		t.Fatal("throughput has ratio unit")
	}
	for _, point := range throughput.Gauge.DataPoints {
		want := 0.2
		if phase := metricLabel(point.Attributes, "phase"); phase == "completed" || phase == "verified_succeeded" {
			want = 0.1
		}
		if point.AsDouble == nil || *point.AsDouble != want || point.StartTimeUnixNano != "" || point.TimeUnixNano != end {
			t.Fatal("original rate replaced or timestamp changed")
		}
	}
	windows := findMetric(t, result, "aries.window.duration")
	for _, point := range windows.Gauge.DataPoints {
		want := 10.0
		if metricLabel(point.Attributes, "scope") == "drain" {
			want = 2
		}
		if *point.AsDouble != want {
			t.Fatal("duration inferred from wall timestamps")
		}
	}
	resource := result.Request.ResourceMetrics[0].Resource
	if len(resource.Attributes) != 1 || resource.Attributes[0].Key != "service.name" {
		t.Fatal("high-cardinality resource attributes added")
	}
	request, _ := json.Marshal(result.Request)
	if bytes.Contains(request, []byte(snapshot.RunID)) || bytes.Contains(request, []byte(snapshot.SnapshotID)) || bytes.Contains(request, []byte(snapshot.Sources[0].SHA256)) {
		t.Fatal("evidence identity leaked into timeseries")
	}
}

func TestProjectMetricsSeparatesMissingZeroAndCanceledStages(t *testing.T) {
	result, err := ProjectMetrics(metricsFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Manifest.Histograms) != 4 {
		t.Fatalf("want four observed groups, got %d", len(result.Manifest.Histograms))
	}
	zero, canceled, missing := false, false, false
	for _, histogram := range result.Manifest.Histograms {
		if histogram.Outcome == "not_started" {
			t.Fatal("unstarted stage manufactured")
		}
		if histogram.Phase == "runtime_execution" && histogram.Outcome == "succeeded" {
			zero = histogram.Count == 1 && histogram.SumNS == 0 && histogram.BucketCounts[0] == 1
		}
		if histogram.Phase == "verifier" && histogram.Outcome == "canceled" {
			canceled = histogram.Count == 1 && histogram.SumNS == 124309 && histogram.Members[0].Outcome == "canceled"
		}
	}
	for _, item := range result.Manifest.NotCollected {
		if item.AttemptID == "private-run/three" && item.Phase == "runtime_execution" {
			missing = true
		}
	}
	if !zero || !canceled || !missing {
		t.Fatal("zero, failed duration, or missing stage lost")
	}
	if result.Manifest.NotCollected[0].Metric != "resource_gauges" || result.Manifest.NotCollected[0].Reason != "snapshot_has_no_resource_samples" {
		t.Fatal("resource absence hidden")
	}
}

func TestProjectMetricsGoodputAbsentAndMeasuredZeroDiffer(t *testing.T) {
	snapshot := metricsFixture(t)
	result, err := ProjectMetrics(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, metric := range result.Request.ResourceMetrics[0].ScopeMetrics[0].Metrics {
		if strings.HasPrefix(metric.Name, "aries.attempt.goodput") {
			t.Fatal("missing Goodput fabricated")
		}
	}
	count, rate, deadline := 0, 0.0, 3*time.Second
	snapshot.AttemptSummary.GoodputCount = &count
	snapshot.AttemptSummary.Goodput = &rate
	snapshot.AttemptSummary.GoodputDeadline = &deadline
	result, err = ProjectMetrics(sealMetricsFixture(t, snapshot))
	if err != nil {
		t.Fatal(err)
	}
	if point := findMetric(t, result, "aries.attempt.goodput_count").Sum.DataPoints[0]; point.AsInt == nil || *point.AsInt != "0" {
		t.Fatal("measured zero Goodput count lost")
	}
	if point := findMetric(t, result, "aries.attempt.goodput").Gauge.DataPoints[0]; point.AsDouble == nil || *point.AsDouble != 0 {
		t.Fatal("measured zero Goodput rate lost")
	}
	if *findMetric(t, result, "aries.attempt.goodput_deadline").Gauge.DataPoints[0].AsDouble != 3 {
		t.Fatal("artifact deadline changed")
	}
}

func TestProjectMetricsHistogramBoundariesAndExactMembers(t *testing.T) {
	snapshot := metricsFixture(t)
	snapshot.Attempts = nil
	values := []int64{0, 1_000_000, 1_000_001, 300_000_000_000, 300_000_000_001}
	for i, value := range values {
		snapshot.Attempts = append(snapshot.Attempts, AnalysisAttempt{AttemptID: "run/" + strconv.Itoa(i), Status: "succeeded", Stages: []StageMeasurement{{Name: "runtime_execution", Scope: "runtime_execution", DurationNS: &value, Status: "duration_only", EvidenceRef: "run-result"}}})
	}
	result, err := ProjectMetrics(sealMetricsFixture(t, snapshot))
	if err != nil {
		t.Fatal(err)
	}
	group := result.Manifest.Histograms[0]
	if group.Count != 5 || group.SumNS != 600_002_000_002 || !reflect.DeepEqual(group.BucketCounts, []uint64{2, 1, 0, 0, 0, 0, 0, 0, 1, 1}) {
		t.Fatalf("bucket boundary or sum error: %+v", group)
	}
	for i, member := range group.Members {
		if member.DurationNS != values[i] || member.EvidenceRef != "run-result" {
			t.Fatal("raw member lost")
		}
	}
	point := findMetric(t, result, "aries.stage.duration").Histogram.DataPoints[0]
	if point.Count != "5" || !reflect.DeepEqual(point.BucketCounts, []string{"2", "1", "0", "0", "0", "0", "0", "0", "1", "1"}) || point.Sum != float64(group.SumNS)/1e9 {
		t.Fatal("wire histogram changed exact aggregate")
	}
}

func TestProjectMetricsStableRoundTripAndNilRates(t *testing.T) {
	snapshot := metricsFixture(t)
	snapshot.AttemptSummary.ExecutionThroughput = nil
	snapshot = sealMetricsFixture(t, snapshot)
	first, err := ProjectMetrics(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	content, _ := json.Marshal(snapshot)
	var loaded AnalysisSnapshot
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	if err := decoder.Decode(&loaded); err != nil {
		t.Fatal(err)
	}
	second, err := ProjectMetrics(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("frozen JSON roundtrip changed metrics")
	}
	for _, point := range findMetric(t, first, "aries.attempt.throughput").Gauge.DataPoints {
		if metricLabel(point.Attributes, "phase") == "completed" {
			t.Fatal("nil rate became zero")
		}
	}
}

func TestProjectMetricsMissingSummaryAndIncompleteRemainExplicit(t *testing.T) {
	snapshot := metricsFixture(t)
	snapshot.AttemptSummary = nil
	result, err := ProjectMetrics(sealMetricsFixture(t, snapshot))
	if err != nil || result.Manifest.Status != "NOT_COLLECTED" || result.Manifest.MetricCount != 0 || len(result.Request.ResourceMetrics) != 0 {
		t.Fatal("missing summary acquired fabricated window")
	}
	snapshot = metricsFixture(t)
	snapshot.IntegrityStatus = "incomplete"
	snapshot.Diagnostics = []string{"original incomplete source"}
	result, err = ProjectMetrics(sealMetricsFixture(t, snapshot))
	if err == nil || result.Manifest.Status != "INCOMPLETE" || result.Manifest.IntegrityStatus != "incomplete" || result.Manifest.CoverageStatus != "unknown" {
		t.Fatal("incomplete source promoted")
	}
}

func TestProjectMetricsRejectsInvalidProjectionInputs(t *testing.T) {
	t.Run("changed snapshot", func(t *testing.T) {
		snapshot := metricsFixture(t)
		snapshot.AttemptSummary.Planned++
		if _, err := ProjectMetrics(snapshot); err == nil {
			t.Fatal("tampered accounting accepted")
		}
	})
	for _, test := range []struct {
		name   string
		change func(*AnalysisSnapshot)
	}{
		{"missing window", func(s *AnalysisSnapshot) { s.AttemptSummary.WindowStartedAt = time.Time{} }},
		{"reverse window", func(s *AnalysisSnapshot) {
			s.AttemptSummary.WindowFinishedAt = s.AttemptSummary.WindowStartedAt.Add(-time.Second)
		}},
		{"negative count", func(s *AnalysisSnapshot) { s.AttemptSummary.Planned = -1 }},
		{"negative duration", func(s *AnalysisSnapshot) { value := int64(-1); s.Attempts[0].Stages[0].DurationNS = &value }},
		{"duplicate member", func(s *AnalysisSnapshot) {
			s.Attempts[0].Stages = append(s.Attempts[0].Stages, s.Attempts[0].Stages[0])
		}},
		{"sum overflow", func(s *AnalysisSnapshot) {
			large, one := int64(^uint64(0)>>1), int64(1)
			s.Attempts = []AnalysisAttempt{{AttemptID: "a", Status: "succeeded", Stages: []StageMeasurement{{Name: "queue", Status: "duration_only", DurationNS: &large}}}, {AttemptID: "b", Status: "succeeded", Stages: []StageMeasurement{{Name: "queue", Status: "duration_only", DurationNS: &one}}}}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := metricsFixture(t)
			test.change(&snapshot)
			if _, err := ProjectMetrics(sealMetricsFixture(t, snapshot)); err == nil {
				t.Fatal("invalid metrics accepted")
			}
		})
	}
}

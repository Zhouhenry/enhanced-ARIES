package performance

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
)

const MetricsExporterVersion = "aries-otlp-metrics-http-json-v1"

type OTLPMetricsExport struct {
	Request  OTLPMetricsRequest  `json:"request"`
	Manifest OTLPMetricsManifest `json:"manifest"`
}

type OTLPMetricsRequest struct {
	ResourceMetrics []OTLPResourceMetrics `json:"resourceMetrics"`
}

type OTLPResourceMetrics struct {
	Resource     OTLPResource       `json:"resource"`
	ScopeMetrics []OTLPScopeMetrics `json:"scopeMetrics"`
}

type OTLPScopeMetrics struct {
	Scope   OTLPScope    `json:"scope"`
	Metrics []OTLPMetric `json:"metrics"`
}

type OTLPMetric struct {
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Unit        string               `json:"unit,omitempty"`
	Sum         *OTLPMetricSum       `json:"sum,omitempty"`
	Gauge       *OTLPMetricGauge     `json:"gauge,omitempty"`
	Histogram   *OTLPMetricHistogram `json:"histogram,omitempty"`
}

type OTLPMetricSum struct {
	DataPoints             []OTLPNumberDataPoint `json:"dataPoints"`
	AggregationTemporality int                   `json:"aggregationTemporality"`
	IsMonotonic            bool                  `json:"isMonotonic"`
}

type OTLPMetricGauge struct {
	DataPoints []OTLPNumberDataPoint `json:"dataPoints"`
}

type OTLPNumberDataPoint struct {
	Attributes        []OTLPKeyValue `json:"attributes"`
	StartTimeUnixNano string         `json:"startTimeUnixNano,omitempty"`
	TimeUnixNano      string         `json:"timeUnixNano"`
	AsInt             *string        `json:"asInt,omitempty"`
	AsDouble          *float64       `json:"asDouble,omitempty"`
}

type OTLPMetricHistogram struct {
	DataPoints             []OTLPHistogramDataPoint `json:"dataPoints"`
	AggregationTemporality int                      `json:"aggregationTemporality"`
}

type OTLPHistogramDataPoint struct {
	Attributes        []OTLPKeyValue `json:"attributes"`
	StartTimeUnixNano string         `json:"startTimeUnixNano"`
	TimeUnixNano      string         `json:"timeUnixNano"`
	Count             string         `json:"count"`
	Sum               float64        `json:"sum"`
	BucketCounts      []string       `json:"bucketCounts"`
	ExplicitBounds    []float64      `json:"explicitBounds"`
}

type OTLPMetricsManifest struct {
	SchemaVersion         int                        `json:"schema_version"`
	ExporterVersion       string                     `json:"exporter_version"`
	AnalysisSchemaVersion int                        `json:"analysis_schema_version"`
	ParserVersion         string                     `json:"parser_version"`
	SnapshotID            string                     `json:"snapshot_id"`
	RunID                 string                     `json:"run_id"`
	Mode                  string                     `json:"mode"`
	Status                string                     `json:"status"`
	IntegrityStatus       string                     `json:"integrity_status"`
	CoverageStatus        string                     `json:"coverage_status"`
	Window                *MetricsWindow             `json:"window,omitempty"`
	OriginalSummary       *core.AttemptSummary       `json:"original_summary,omitempty"`
	Sources               []EvidenceSource           `json:"sources"`
	MetricCount           int                        `json:"metric_count"`
	DataPointCount        int                        `json:"data_point_count"`
	Histograms            []MetricsHistogramEvidence `json:"histograms"`
	NotCollected          []MetricsNotCollected      `json:"not_collected"`
	Diagnostics           []string                   `json:"diagnostics"`
	Limitations           []string                   `json:"limitations"`
	ResourceStream        *AnalysisResourceStream    `json:"resource_stream,omitempty"`
	ResourcePoints        []MetricsResourcePoint     `json:"resource_points,omitempty"`
}

type MetricsWindow struct {
	StartedAtUTC           time.Time `json:"started_at_utc"`
	AdmissionFinishedAtUTC time.Time `json:"admission_finished_at_utc"`
	FinishedAtUTC          time.Time `json:"finished_at_utc"`
	StartTimeUnixNano      string    `json:"start_time_unix_nano"`
	TimeUnixNano           string    `json:"time_unix_nano"`
	ObservationDurationNS  int64     `json:"observation_duration_ns"`
	DrainDurationNS        int64     `json:"drain_duration_ns"`
}

type MetricsHistogramEvidence struct {
	Metric       string                   `json:"metric"`
	Phase        string                   `json:"phase"`
	Outcome      string                   `json:"outcome"`
	Scope        string                   `json:"scope"`
	Count        uint64                   `json:"count"`
	SumNS        int64                    `json:"sum_ns"`
	SumSeconds   float64                  `json:"sum_seconds"`
	BoundsNS     []int64                  `json:"bounds_ns"`
	BucketCounts []uint64                 `json:"bucket_counts"`
	Members      []MetricsHistogramMember `json:"members"`
}

type MetricsHistogramMember struct {
	AttemptID         string `json:"attempt_id"`
	Phase             string `json:"phase"`
	Outcome           string `json:"outcome"`
	Scope             string `json:"scope"`
	DurationNS        int64  `json:"duration_ns"`
	MeasurementStatus string `json:"measurement_status"`
	EvidenceRef       string `json:"evidence_ref"`
	OperationID       string `json:"operation_id,omitempty"`
}

type MetricsNotCollected struct {
	Metric      string `json:"metric"`
	SourceField string `json:"source_field,omitempty"`
	AttemptID   string `json:"attempt_id,omitempty"`
	Phase       string `json:"phase,omitempty"`
	Reason      string `json:"reason"`
}

// ProjectMetrics preserves existing accounting. Only the new stage histogram
// aggregates individual, already validated snapshot measurements. No resource
// samples or stage wall-clock boundaries are inferred from those durations.
func ProjectMetrics(snapshot AnalysisSnapshot) (OTLPMetricsExport, error) {
	if err := ValidateSnapshot(snapshot); err != nil {
		return OTLPMetricsExport{}, err
	}
	out := OTLPMetricsExport{Request: OTLPMetricsRequest{ResourceMetrics: []OTLPResourceMetrics{}}, Manifest: OTLPMetricsManifest{
		SchemaVersion: 1, ExporterVersion: MetricsExporterVersion, AnalysisSchemaVersion: snapshot.SchemaVersion,
		ParserVersion: snapshot.ParserVersion, SnapshotID: snapshot.SnapshotID, RunID: snapshot.RunID,
		Mode: "offline_isolated_batch", Status: "PROJECTED", IntegrityStatus: snapshot.IntegrityStatus, CoverageStatus: snapshot.CoverageStatus,
		Sources: append([]EvidenceSource{}, snapshot.Sources...), Histograms: []MetricsHistogramEvidence{},
		NotCollected: []MetricsNotCollected{{Metric: "resource_gauges", Reason: "snapshot_has_no_resource_samples"}},
		Diagnostics:  append([]string{}, snapshot.Diagnostics...), Limitations: append([]string{}, snapshot.Limitations...),
	}}
	out.Manifest.Limitations = append(out.Manifest.Limitations,
		"One immutable historical batch requires an isolated metric series namespace or TSDB; run and attempt IDs are not labels.",
		"Lifecycle counts overlap. Original summary counts, rates and Goodput are copied, not recalculated or reclassified.",
		"A single terminal cumulative point does not establish rate() or increase(); gauges describe the original whole-window rate.",
		"Histogram outcome is the attempt outcome, not a stage success claim. Missing stages are not zero-duration samples.",
		"Histogram sum is projected to float64 seconds; exact member nanoseconds and integer sum remain in this manifest.",
		"Resource gauges, individual stage timestamps and Goodput member eligibility are unavailable in this snapshot schema.")
	if snapshot.Resources != nil {
		out.Manifest.NotCollected[0].Reason = "resource_samples_require_separate_owner_stream_export"
		out.Manifest.Limitations[len(out.Manifest.Limitations)-1] = "Resource observations use a separate per-owner export; individual stage timestamps and Goodput member eligibility are not inferred."
	}
	fail := func(err error) (OTLPMetricsExport, error) {
		out.Manifest.Status = "INVALID"
		out.Manifest.Diagnostics = append(out.Manifest.Diagnostics, err.Error())
		return out, err
	}
	summary := snapshot.AttemptSummary
	if summary == nil {
		out.Manifest.Status = "NOT_COLLECTED"
		out.Manifest.NotCollected = append(out.Manifest.NotCollected,
			MetricsNotCollected{Metric: "attempt_accounting", SourceField: "attempt_summary", Reason: "attempt_summary_missing"},
			MetricsNotCollected{Metric: "aries.stage.duration", Reason: "historical_attempt_window_missing"})
		if snapshot.IntegrityStatus != "complete" {
			return out, errors.New("metrics evidence is incomplete")
		}
		return out, nil
	}
	original := *summary
	out.Manifest.OriginalSummary = &original
	if summary.WindowStartedAt.IsZero() || summary.WindowFinishedAt.IsZero() || !summary.WindowFinishedAt.After(summary.WindowStartedAt) || summary.ObservationDuration < 0 || summary.DrainDuration < 0 {
		return fail(errors.New("invalid historical attempt window"))
	}
	start, err := otlpEpoch(summary.WindowStartedAt, 0)
	if err != nil {
		return fail(err)
	}
	end, err := otlpEpoch(summary.WindowFinishedAt, 0)
	if err != nil {
		return fail(err)
	}
	out.Manifest.Window = &MetricsWindow{StartedAtUTC: summary.WindowStartedAt, AdmissionFinishedAtUTC: summary.AdmissionFinishedAt,
		FinishedAtUTC: summary.WindowFinishedAt, StartTimeUnixNano: start, TimeUnixNano: end,
		ObservationDurationNS: int64(summary.ObservationDuration), DrainDurationNS: int64(summary.DrainDuration)}
	metrics := []OTLPMetric{}
	countPoint := func(count int, labels map[string]string) (OTLPNumberDataPoint, error) {
		if count < 0 {
			return OTLPNumberDataPoint{}, errors.New("negative original attempt count")
		}
		value := strconv.Itoa(count)
		return OTLPNumberDataPoint{Attributes: metricLabels(labels), StartTimeUnixNano: start, TimeUnixNano: end, AsInt: &value}, nil
	}
	appendCount := func(name, description string, count int) error {
		point, err := countPoint(count, nil)
		if err != nil {
			return err
		}
		metrics = append(metrics, OTLPMetric{Name: name, Description: description, Sum: &OTLPMetricSum{DataPoints: []OTLPNumberDataPoint{point}, AggregationTemporality: 2, IsMonotonic: true}})
		return nil
	}
	for _, family := range []struct {
		name, description, label string
		values                   []struct {
			name  string
			value int
		}
	}{
		{"aries.attempt.lifecycle", "Original cumulative lifecycle counts; phases overlap and must not be summed as independent attempts.", "phase", []struct {
			name  string
			value int
		}{
			{"planned", summary.Planned}, {"admitted", summary.Admitted}, {"started", summary.Started}, {"executed", summary.Executed}, {"completed", summary.Completed}, {"verified_succeeded", summary.ValidatedSucceeded}}},
		{"aries.attempt.outcomes", "Original terminal attempt counts, including failures and attempts not started.", "outcome", []struct {
			name  string
			value int
		}{
			{"succeeded", summary.Succeeded}, {"failed", summary.Failed}, {"blocked", summary.Blocked}, {"canceled", summary.Canceled}, {"timed_out", summary.TimedOut}, {"unknown", summary.Unknown}, {"not_started", summary.NotStarted}, {"loadgen_dropped", summary.LoadgenDropped}}},
	} {
		points := []OTLPNumberDataPoint{}
		for _, entry := range family.values {
			point, err := countPoint(entry.value, map[string]string{family.label: entry.name})
			if err != nil {
				return fail(err)
			}
			points = append(points, point)
		}
		metrics = append(metrics, OTLPMetric{Name: family.name, Description: family.description,
			Sum: &OTLPMetricSum{DataPoints: points, AggregationTemporality: 2, IsMonotonic: true}})
	}
	if err := appendCount("aries.attempt.cleanup_failed", "Original cleanup-failed count; it overlaps terminal attempt outcomes.", summary.CleanupFailed); err != nil {
		return fail(err)
	}
	gaugePoint := func(value float64, labels map[string]string) OTLPNumberDataPoint {
		return OTLPNumberDataPoint{Attributes: metricLabels(labels), TimeUnixNano: end, AsDouble: &value}
	}
	rates := []OTLPNumberDataPoint{}
	for _, rate := range []struct {
		phase, field string
		value        *float64
	}{
		{"started", "attempts_per_second", summary.AttemptsPerSecond}, {"executed", "executed_throughput", summary.ExecutedPerSecond},
		{"completed", "execution_throughput", summary.ExecutionThroughput}, {"verified_succeeded", "validated_success_throughput", summary.ValidatedThroughput},
	} {
		if rate.value == nil {
			out.Manifest.NotCollected = append(out.Manifest.NotCollected, MetricsNotCollected{Metric: "aries.attempt.throughput", SourceField: rate.field, Phase: rate.phase, Reason: "original_rate_absent"})
			continue
		}
		rates = append(rates, gaugePoint(*rate.value, map[string]string{"phase": rate.phase}))
	}
	if len(rates) > 0 {
		metrics = append(metrics, OTLPMetric{Name: "aries.attempt.throughput", Unit: "1/s", Description: "Original whole-window throughput, including admission and drain; not derived from this export.", Gauge: &OTLPMetricGauge{DataPoints: rates}})
	}
	if summary.GoodputCount != nil {
		if err := appendCount("aries.attempt.goodput_count", "Original independently validated, artifact-proven count within the configured artifact deadline.", *summary.GoodputCount); err != nil {
			return fail(err)
		}
	} else {
		out.Manifest.NotCollected = append(out.Manifest.NotCollected, MetricsNotCollected{Metric: "aries.attempt.goodput_count", SourceField: "goodput_count", Reason: "original_goodput_count_absent"})
	}
	if summary.Goodput != nil {
		metrics = append(metrics, OTLPMetric{Name: "aries.attempt.goodput", Description: "Original Goodput over the full observation window; missing eligibility is not reconstructed.", Unit: "1/s", Gauge: &OTLPMetricGauge{DataPoints: []OTLPNumberDataPoint{gaugePoint(*summary.Goodput, nil)}}})
	} else {
		out.Manifest.NotCollected = append(out.Manifest.NotCollected, MetricsNotCollected{Metric: "aries.attempt.goodput", SourceField: "goodput", Reason: "original_goodput_absent"})
	}
	if summary.GoodputDeadline != nil {
		metrics = append(metrics, OTLPMetric{Name: "aries.attempt.goodput_deadline", Unit: "s", Description: "Original artifact-ready deadline, not a verifier-completion deadline.", Gauge: &OTLPMetricGauge{DataPoints: []OTLPNumberDataPoint{gaugePoint(summary.GoodputDeadline.Seconds(), nil)}}})
	} else {
		out.Manifest.NotCollected = append(out.Manifest.NotCollected, MetricsNotCollected{Metric: "aries.attempt.goodput_deadline", SourceField: "goodput_deadline", Reason: "original_deadline_absent"})
	}
	metrics = append(metrics, OTLPMetric{Name: "aries.window.duration", Unit: "s", Description: "Original monotonic observation and drain durations, not recomputed from serialized UTC timestamps.", Gauge: &OTLPMetricGauge{DataPoints: []OTLPNumberDataPoint{
		gaugePoint(summary.ObservationDuration.Seconds(), map[string]string{"scope": "observation"}), gaugePoint(summary.DrainDuration.Seconds(), map[string]string{"scope": "drain"})}}})

	// These bounds are part of exporter v1. Counts are non-cumulative OTLP
	// buckets; a Prometheus receiver may expose their cumulative le projection.
	bounds := []int64{1_000_000, 10_000_000, 100_000_000, 1_000_000_000, 5_000_000_000, 10_000_000_000, 30_000_000_000, 60_000_000_000, 300_000_000_000}
	groups := map[string]*MetricsHistogramEvidence{}
	seen := map[string]bool{}
	for _, attempt := range snapshot.Attempts {
		for _, stage := range attempt.Stages {
			memberKey := attempt.AttemptID + "\x00" + stage.Name
			if seen[memberKey] {
				return fail(errors.New("duplicate attempt-stage histogram member"))
			}
			seen[memberKey] = true
			if stage.Status != "duration_only" || stage.DurationNS == nil {
				out.Manifest.NotCollected = append(out.Manifest.NotCollected, MetricsNotCollected{Metric: "aries.stage.duration", AttemptID: attempt.AttemptID, Phase: stage.Name, Reason: "stage_duration_not_collected"})
				continue
			}
			value := *stage.DurationNS
			if value < 0 {
				return fail(fmt.Errorf("negative stage duration for %s/%s", attempt.AttemptID, stage.Name))
			}
			key := stage.Name + "\x00" + attempt.Status + "\x00" + stage.Scope
			group := groups[key]
			if group == nil {
				group = &MetricsHistogramEvidence{Metric: "aries.stage.duration", Phase: stage.Name, Outcome: attempt.Status, Scope: stage.Scope,
					BoundsNS: append([]int64{}, bounds...), BucketCounts: make([]uint64, len(bounds)+1), Members: []MetricsHistogramMember{}}
				groups[key] = group
			}
			if value > int64(^uint64(0)>>1)-group.SumNS {
				return fail(errors.New("stage histogram nanosecond sum overflow"))
			}
			group.SumNS += value
			group.Count++
			bucket := sort.Search(len(bounds), func(i int) bool { return value <= bounds[i] })
			group.BucketCounts[bucket]++
			group.Members = append(group.Members, MetricsHistogramMember{attempt.AttemptID, stage.Name, attempt.Status, stage.Scope, value, stage.Status, stage.EvidenceRef, ""})
		}
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	histogramPoints := []OTLPHistogramDataPoint{}
	for _, key := range keys {
		group := groups[key]
		group.SumSeconds = float64(group.SumNS) / 1e9
		counts := make([]string, len(group.BucketCounts))
		for i, count := range group.BucketCounts {
			counts[i] = strconv.FormatUint(count, 10)
		}
		seconds := make([]float64, len(bounds))
		for i, bound := range bounds {
			seconds[i] = float64(bound) / 1e9
		}
		histogramPoints = append(histogramPoints, OTLPHistogramDataPoint{Attributes: metricLabels(map[string]string{"phase": group.Phase, "outcome": group.Outcome, "scope": group.Scope}),
			StartTimeUnixNano: start, TimeUnixNano: end, Count: strconv.FormatUint(group.Count, 10), Sum: group.SumSeconds, BucketCounts: counts, ExplicitBounds: seconds})
		out.Manifest.Histograms = append(out.Manifest.Histograms, *group)
	}
	if len(histogramPoints) > 0 {
		metrics = append(metrics, OTLPMetric{Name: "aries.stage.duration", Unit: "s", Description: "Measured stage durations grouped by terminal attempt outcome; not per-stage success or process runtime.", Histogram: &OTLPMetricHistogram{DataPoints: histogramPoints, AggregationTemporality: 2}})
	}
	remoteMetrics, remoteEvidence, remoteErr := jiuwenRemoteMetrics(snapshot, start, end)
	if remoteErr != nil {
		return fail(remoteErr)
	}
	metrics = append(metrics, remoteMetrics...)
	out.Manifest.Histograms = append(out.Manifest.Histograms, remoteEvidence...)
	service := "aries-offline-metrics"
	out.Request.ResourceMetrics = []OTLPResourceMetrics{{Resource: OTLPResource{Attributes: []OTLPKeyValue{{Key: "service.name", Value: OTLPAnyValue{StringValue: &service}}}},
		ScopeMetrics: []OTLPScopeMetrics{{Scope: OTLPScope{Name: "aries.performance", Version: MetricsExporterVersion}, Metrics: metrics}}}}
	out.Manifest.MetricCount = len(metrics)
	for _, metric := range metrics {
		if metric.Sum != nil {
			out.Manifest.DataPointCount += len(metric.Sum.DataPoints)
		}
		if metric.Gauge != nil {
			out.Manifest.DataPointCount += len(metric.Gauge.DataPoints)
		}
		if metric.Histogram != nil {
			out.Manifest.DataPointCount += len(metric.Histogram.DataPoints)
		}
	}
	if snapshot.IntegrityStatus != "complete" {
		out.Manifest.Status = "INCOMPLETE"
		return out, errors.New("metrics evidence is incomplete; inspect manifest diagnostics and snapshot coverage")
	}
	return out, nil
}

func metricLabels(labels map[string]string) []OTLPKeyValue {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]OTLPKeyValue, 0, len(keys))
	for _, key := range keys {
		value := labels[key]
		result = append(result, OTLPKeyValue{Key: key, Value: OTLPAnyValue{StringValue: &value}})
	}
	return result
}

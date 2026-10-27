package performance

import (
	"errors"
	"sort"
	"strconv"
)

// Jiuwen operation histograms are independent measurements, never additions to stage totals.
func jiuwenRemoteMetrics(snapshot AnalysisSnapshot, start, end string) ([]OTLPMetric, []MetricsHistogramEvidence, error) {
	const remoteName = "aries.jiuwen.remote.duration"
	const processName = "aries.jiuwen.process.duration"
	bounds := []int64{1_000_000, 10_000_000, 100_000_000, 1_000_000_000, 5_000_000_000, 10_000_000_000, 30_000_000_000, 60_000_000_000, 300_000_000_000}
	groups := map[string]*MetricsHistogramEvidence{}
	for _, interval := range snapshot.Intervals {
		name := remoteName
		switch interval.Category {
		case "jiuwen_remote_bridge", "jiuwen_remote_worker", "jiuwen_remote_provider":
		case "jiuwen_process":
			// The observation container overlaps these intervals and is not a stage.
			if interval.Name != "jiuwen.process.creation" && interval.Name != "jiuwen.process.command" && interval.Name != "jiuwen.process.exit_to_pipe_closed" {
				continue
			}
			name = processName
		default:
			continue
		}
		if interval.DurationNS < 0 || interval.EndNS-interval.StartNS != interval.DurationNS || len(interval.EvidenceRefs) == 0 {
			return nil, nil, errors.New("invalid Jiuwen operation metric duration or source")
		}
		key := name + "\x00" + interval.Name + "\x00" + interval.Outcome + "\x00" + interval.Scope
		g := groups[key]
		if g == nil {
			g = &MetricsHistogramEvidence{Metric: name, Phase: interval.Name, Outcome: interval.Outcome, Scope: interval.Scope, BoundsNS: bounds, BucketCounts: make([]uint64, len(bounds)+1), Members: []MetricsHistogramMember{}}
			groups[key] = g
		}
		if interval.DurationNS > int64(^uint64(0)>>1)-g.SumNS {
			return nil, nil, errors.New("Jiuwen operation histogram sum overflow")
		}
		g.Count++
		g.SumNS += interval.DurationNS
		g.BucketCounts[sort.Search(len(bounds), func(i int) bool { return interval.DurationNS <= bounds[i] })]++
		g.Members = append(g.Members, MetricsHistogramMember{AttemptID: interval.AttemptID, Phase: interval.Name, Outcome: interval.Outcome, Scope: interval.Scope, DurationNS: interval.DurationNS, MeasurementStatus: "monotonic_interval", EvidenceRef: interval.EvidenceRefs[0], OperationID: interval.ID})
	}
	if len(groups) == 0 {
		return nil, nil, nil
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	points := map[string][]OTLPHistogramDataPoint{}
	metricNames := []string{}
	evidence := []MetricsHistogramEvidence{}
	for _, key := range keys {
		g := groups[key]
		g.SumSeconds = float64(g.SumNS) / 1e9
		counts := make([]string, len(g.BucketCounts))
		for i, v := range g.BucketCounts {
			counts[i] = strconv.FormatUint(v, 10)
		}
		seconds := make([]float64, len(bounds))
		for i, v := range bounds {
			seconds[i] = float64(v) / 1e9
		}
		if len(points[g.Metric]) == 0 {
			metricNames = append(metricNames, g.Metric)
		}
		points[g.Metric] = append(points[g.Metric], OTLPHistogramDataPoint{Attributes: metricLabels(map[string]string{"phase": g.Phase, "outcome": g.Outcome, "scope": g.Scope}), StartTimeUnixNano: start, TimeUnixNano: end, Count: strconv.FormatUint(g.Count, 10), Sum: g.SumSeconds, BucketCounts: counts, ExplicitBounds: seconds})
		evidence = append(evidence, *g)
	}
	metrics := []OTLPMetric{}
	for _, name := range metricNames {
		description := "Measured Agent provider, Bridge forwarding and worker request intervals; separate overlapping layers, not process runtime or network latency."
		if name == processName {
			description = "Independent native subprocess creation, direct-child execution and observed pipe-closure intervals; excludes observation containers, descendant completion and natural EOF or consumer drain."
		}
		metrics = append(metrics, OTLPMetric{Name: name, Unit: "s", Description: description, Histogram: &OTLPMetricHistogram{DataPoints: points[name], AggregationTemporality: 2}})
	}
	return metrics, evidence, nil
}

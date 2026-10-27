package performance

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
)

const ResourceMetricsExporterVersion = "aries-otlp-resource-metrics-http-json-v1"

// MetricsResourcePoint retains all source observations of one exported point.
// Duplicate reads are receipts, not additional independent measurements.
type MetricsResourcePoint struct {
	Metric        string                `json:"metric"`
	TimeUnixNano  string                `json:"time_unix_nano"`
	SampleIDs     []string              `json:"sample_ids"`
	OriginalValue AnalysisResourceValue `json:"original_value"`
	ExportValue   float64               `json:"export_value"`
	ExportUnit    string                `json:"export_unit"`
}

// ProjectResourceMetrics emits one owner per isolated batch. It never adds
// asynchronous owners together or fills gaps to match a polling interval.
func ProjectResourceMetrics(snapshot AnalysisSnapshot, streamID string) (OTLPMetricsExport, error) {
	if err := ValidateSnapshot(snapshot); err != nil {
		return OTLPMetricsExport{}, err
	}
	var stream *AnalysisResourceStream
	if snapshot.Resources != nil {
		for i := range snapshot.Resources.Streams {
			if snapshot.Resources.Streams[i].ID == streamID {
				stream = &snapshot.Resources.Streams[i]
				break
			}
		}
	}
	if stream == nil {
		return OTLPMetricsExport{}, errors.New("resource stream not found in validated snapshot")
	}
	out := OTLPMetricsExport{Request: OTLPMetricsRequest{ResourceMetrics: []OTLPResourceMetrics{}}, Manifest: OTLPMetricsManifest{
		SchemaVersion: 1, ExporterVersion: ResourceMetricsExporterVersion, AnalysisSchemaVersion: snapshot.SchemaVersion,
		ParserVersion: snapshot.ParserVersion, SnapshotID: snapshot.SnapshotID, RunID: snapshot.RunID,
		Mode: "offline_isolated_resource_stream", Status: "PROJECTED", IntegrityStatus: snapshot.IntegrityStatus,
		CoverageStatus: snapshot.CoverageStatus, Sources: append([]EvidenceSource{}, snapshot.Sources...),
		ResourceStream: stream, Histograms: []MetricsHistogramEvidence{}, NotCollected: []MetricsNotCollected{},
		Diagnostics: append(append([]string{}, snapshot.Diagnostics...), stream.Diagnostics...),
		Limitations: []string{
			"One resource owner per isolated TSDB; owner/run/attempt IDs remain in the manifest, not labels.",
			"Values use the recorded observation boundary; a read time is never substituted for unknown provider sampling time.",
			"Cumulative readings are explicitly named observed Gauges, not counters with an invented process-start epoch.",
			"Identical timestamp/value reads share one point and retain every sample ID; equal values at different timestamps remain separate.",
			"No interpolation, cross-owner sum, per-Tool attribution or short-task peak is inferred; disk usage is not disk IO.",
			"Raw source numbers and sampling brackets remain in the selected stream; export uses float64 and backend timestamp precision may be lower.",
		},
	}}
	points := map[string]*MetricsResourcePoint{}
	for _, sample := range stream.Samples {
		if sample.TimeStatus != "verified" || sample.ObservedAtUTC == nil {
			out.Manifest.NotCollected = append(out.Manifest.NotCollected, MetricsNotCollected{Metric: "resource_gauges", SourceField: sample.ID, Reason: "sampling_timestamp_not_verified"})
			continue
		}
		epoch, err := otlpEpoch(*sample.ObservedAtUTC, 0)
		if err != nil {
			return out, err
		}
		for _, original := range sample.Values {
			if !original.ExportEligible {
				out.Manifest.NotCollected = append(out.Manifest.NotCollected, MetricsNotCollected{Metric: "resource_gauges", SourceField: sample.ID + "/" + original.Name, Reason: original.Reason})
				continue
			}
			name, unit, value, err := resourceMetricValue(original)
			if err != nil {
				return out, err
			}
			key := name + "\x00" + epoch
			if prior := points[key]; prior != nil {
				a, okA := new(big.Rat).SetString(string(prior.OriginalValue.Value))
				b, okB := new(big.Rat).SetString(string(original.Value))
				if !okA || !okB || a.Cmp(b) != 0 || prior.OriginalValue.Unit != original.Unit || prior.OriginalValue.Kind != original.Kind {
					out.Manifest.Status = "INCOMPLETE"
					out.Manifest.Diagnostics = append(out.Manifest.Diagnostics, "conflicting_resource_values_at_same_timestamp")
					return out, errors.New("conflicting resource values at one timestamp; no last-write-wins")
				}
				prior.SampleIDs = append(prior.SampleIDs, sample.ID)
				continue
			}
			points[key] = &MetricsResourcePoint{Metric: name, TimeUnixNano: epoch, SampleIDs: []string{sample.ID}, OriginalValue: original, ExportValue: value, ExportUnit: unit}
		}
	}
	ordered := make([]*MetricsResourcePoint, 0, len(points))
	for _, point := range points {
		ordered = append(ordered, point)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Metric != ordered[j].Metric {
			return ordered[i].Metric < ordered[j].Metric
		}
		a, _ := strconv.ParseUint(ordered[i].TimeUnixNano, 10, 64)
		b, _ := strconv.ParseUint(ordered[j].TimeUnixNano, 10, 64)
		return a < b
	})
	labels := map[string]string{"scope": stream.Scope}
	if stream.Purpose != "" {
		labels["purpose"] = stream.Purpose
	}
	if stream.Component != "" {
		labels["component"] = stream.Component
	}
	metrics := []OTLPMetric{}
	for _, point := range ordered {
		if len(metrics) == 0 || metrics[len(metrics)-1].Name != point.Metric {
			metrics = append(metrics, OTLPMetric{Name: point.Metric, Unit: point.ExportUnit,
				Description: "Recorded " + point.OriginalValue.Kind + " value from " + stream.Source + " at " + stream.TimeBasis + "; see source stream in manifest.",
				Gauge:       &OTLPMetricGauge{DataPoints: []OTLPNumberDataPoint{}}})
		}
		value := point.ExportValue
		gauge := metrics[len(metrics)-1].Gauge
		gauge.DataPoints = append(gauge.DataPoints, OTLPNumberDataPoint{Attributes: metricLabels(labels), TimeUnixNano: point.TimeUnixNano, AsDouble: &value})
		out.Manifest.ResourcePoints = append(out.Manifest.ResourcePoints, *point)
	}
	out.Manifest.MetricCount, out.Manifest.DataPointCount = len(metrics), len(ordered)
	if len(metrics) == 0 {
		out.Manifest.Status = "NOT_COLLECTED"
		out.Manifest.NotCollected = append(out.Manifest.NotCollected, MetricsNotCollected{Metric: "resource_gauges", Reason: "no_verified_time_and_unit_resource_points"})
	} else {
		service := "aries-offline-resources"
		out.Request.ResourceMetrics = []OTLPResourceMetrics{{Resource: OTLPResource{Attributes: []OTLPKeyValue{{Key: "service.name", Value: OTLPAnyValue{StringValue: &service}}}},
			ScopeMetrics: []OTLPScopeMetrics{{Scope: OTLPScope{Name: "aries.resources", Version: ResourceMetricsExporterVersion}, Metrics: metrics}}}}
	}
	if stream.IntegrityStatus != "complete" || snapshot.IntegrityStatus != "complete" {
		out.Manifest.Status = "INCOMPLETE"
		return out, errors.New("resource evidence is incomplete; inspect snapshot and stream diagnostics")
	}
	return out, nil
}

func resourceMetricValue(original AnalysisResourceValue) (string, string, float64, error) {
	value, err := original.Value.Float64()
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return "", "", 0, errors.New("invalid resource value")
	}
	name, unit := original.Name, ""
	switch original.Unit {
	case "ns":
		name = strings.TrimSuffix(strings.TrimSuffix(name, "_nanoseconds"), "_ns")
		unit, value = "s", value/1e9
	case "bytes":
		name, unit = strings.TrimSuffix(name, "_bytes"), "By"
	case "MiB":
		name, unit, value = strings.TrimSuffix(name, "_mib"), "By", value*1048576
	case "percent":
		name, unit = strings.TrimSuffix(name, "_pct"), "%"
	case "count":
	default:
		return "", "", 0, fmt.Errorf("unsupported resource unit %q", original.Unit)
	}
	if math.IsInf(value, 0) {
		return "", "", 0, errors.New("resource unit conversion overflow")
	}
	if original.Kind == "cumulative_counter" {
		name += "_observed"
	}
	return "aries.resource." + name, unit, value, nil
}

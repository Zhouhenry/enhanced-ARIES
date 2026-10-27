package performance

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func resourceMetricsFixture(t *testing.T) AnalysisSnapshot {
	t.Helper()
	snapshot := metricsFixture(t)
	at := time.Date(2026, 10, 25, 0, 0, 0, 123456789, time.UTC)
	sample := AnalysisResourceSample{ID: "first", TimeStatus: "verified", ObservedAtUTC: &at, Status: "collected", Values: []AnalysisResourceValue{
		{Name: "cpu_user_ns", Value: "1500000000", Unit: "ns", Kind: "cumulative_counter", ExportEligible: true},
		{Name: "memory_usage_bytes", Value: "9007199254740993", Unit: "bytes", Kind: "gauge", ExportEligible: true},
		{Name: "cpu_used_pct", Value: "12.5", Unit: "percent", Kind: "gauge", ExportEligible: true},
		{Name: "cpu_percent", Value: "0", Unit: "percent", Kind: "gauge", ExportEligible: false, Reason: "monitor_baseline_and_rate_semantics_not_projected"},
	}}
	duplicate := sample
	duplicate.ID = "repeat"
	duplicate.Values = append([]AnalysisResourceValue{}, sample.Values...)
	duplicate.Values[0].Value = "1500000000.0"
	later := sample
	later.ID = "later"
	end := at.Add(time.Second)
	later.ObservedAtUTC = &end
	snapshot.Resources = &AnalysisResources{SchemaVersion: 1, Streams: []AnalysisResourceStream{
		{ID: "owner-one", OwnerID: "private-owner", Source: "monitor", Scope: "container_resource", Component: "harness", IntegrityStatus: "complete", TimeBasis: "source_observed_time", Samples: []AnalysisResourceSample{sample, duplicate, later}},
		{ID: "owner-two", OwnerID: "another-owner", Source: "monitor", Scope: "container_resource", IntegrityStatus: "complete", Samples: []AnalysisResourceSample{sample}},
	}}
	return sealMetricsFixture(t, snapshot)
}

func TestResourceMetricsDeduplicateReceiptsKeepTimesAndOwners(t *testing.T) {
	snapshot := resourceMetricsFixture(t)
	before, _ := json.Marshal(snapshot)
	got, err := ProjectResourceMetrics(snapshot, "owner-one")
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(snapshot)
	if !bytes.Equal(before, after) {
		t.Fatal("source mutated")
	}
	if got.Manifest.DataPointCount != 6 || got.Manifest.MetricCount != 3 || got.Manifest.ResourceStream.OwnerID != "private-owner" {
		t.Fatal("receipts or different owners counted as samples")
	}
	for _, metric := range got.Request.ResourceMetrics[0].ScopeMetrics[0].Metrics {
		if metric.Sum != nil || metric.Gauge == nil || len(metric.Gauge.DataPoints) != 2 {
			t.Fatal("fabricated counter epoch or lost repeated value at distinct time")
		}
		for _, point := range metric.Gauge.DataPoints {
			if point.StartTimeUnixNano != "" || metricLabel(point.Attributes, "scope") != "container_resource" {
				t.Fatal("wrong resource time/scope")
			}
		}
	}
	for _, point := range got.Manifest.ResourcePoints {
		want := []string{"later"}
		if point.TimeUnixNano == "1792886400123456789" {
			want = []string{"first", "repeat"}
		}
		if !reflect.DeepEqual(point.SampleIDs, want) {
			t.Fatalf("duplicate lineage lost: %+v", point)
		}
	}
	cpu := findMetric(t, got, "aries.resource.cpu_user_observed")
	if cpu.Unit != "s" || *cpu.Gauge.DataPoints[0].AsDouble != 1.5 {
		t.Fatal("CPU nanoseconds conversion wrong")
	}
	memory := findMetric(t, got, "aries.resource.memory_usage")
	if memory.Unit != "By" || *memory.Gauge.DataPoints[0].AsDouble != 9007199254740992 {
		t.Fatal("unexpected float64 representation")
	}
	if findMetric(t, got, "aries.resource.cpu_used").Unit != "%" || *findMetric(t, got, "aries.resource.cpu_used").Gauge.DataPoints[0].AsDouble != 12.5 {
		t.Fatal("percentage rescaled")
	}
	exact := false
	for _, p := range got.Manifest.ResourcePoints {
		if p.OriginalValue.Value == "9007199254740993" {
			exact = true
		}
	}
	if !exact {
		t.Fatal("exact original integer lost")
	}
	request, _ := json.Marshal(got.Request)
	if bytes.Contains(request, []byte("private-owner")) || bytes.Contains(request, []byte("private-run")) {
		t.Fatal("owner IDs became labels")
	}
	other, err := ProjectResourceMetrics(snapshot, "owner-two")
	if err != nil || other.Manifest.DataPointCount != 3 {
		t.Fatal("independent owner merged")
	}
	path := filepath.Join(t.TempDir(), "snapshot.json")
	if err := os.WriteFile(path, before, 0600); err != nil {
		t.Fatal(err)
	}
	restored, err := LoadSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	again, err := ProjectResourceMetrics(restored, "owner-one")
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(got)
	b, _ := json.Marshal(again)
	if !bytes.Equal(a, b) {
		t.Fatal("frozen resource projection changed")
	}
}

func TestResourceMetricsMissingAndConflictingEvidence(t *testing.T) {
	for _, name := range []string{"unknown_time", "empty", "conflict", "incomplete", "missing_owner", "tampered"} {
		t.Run(name, func(t *testing.T) {
			snapshot := resourceMetricsFixture(t)
			stream := &snapshot.Resources.Streams[0]
			owner := "owner-one"
			switch name {
			case "unknown_time":
				for i := range stream.Samples {
					stream.Samples[i].TimeStatus = "unverified"
					stream.Samples[i].ObservedAtUTC = nil
				}
			case "empty":
				stream.Samples = nil
			case "conflict":
				stream.Samples[1].Values = append([]AnalysisResourceValue{}, stream.Samples[1].Values...)
				stream.Samples[1].Values[0].Value = "1600000000"
			case "incomplete":
				stream.IntegrityStatus = "incomplete"
			case "missing_owner":
				owner = "absent"
			}
			snapshot = sealMetricsFixture(t, snapshot)
			if name == "tampered" {
				snapshot.Resources.Streams[0].OwnerID = "changed"
			}
			got, err := ProjectResourceMetrics(snapshot, owner)
			if name == "empty" || name == "unknown_time" {
				if err != nil || got.Manifest.Status != "NOT_COLLECTED" || len(got.Request.ResourceMetrics) != 0 {
					t.Fatal("missing timestamps/values replaced with zero")
				}
			} else if err == nil {
				t.Fatal("invalid resource evidence accepted")
			}
		})
	}
}

func TestResourceMetricUnitsAndAccountingExportScope(t *testing.T) {
	for _, test := range []struct {
		name, value, unit, kind, wantName, wantUnit string
		want                                        float64
	}{
		{"mem_used_mib", "2", "MiB", "gauge", "mem_used", "By", 2097152},
		{"cpu_usage_nanoseconds", "2500000000", "ns", "cumulative_counter", "cpu_usage_observed", "s", 2.5},
		{"cpu_count", "2", "count", "gauge", "cpu_count", "", 2},
		{"process_lifetime_peak_rss_bytes", "1024", "bytes", "gauge", "process_lifetime_peak_rss", "By", 1024},
	} {
		name, unit, value, err := resourceMetricValue(AnalysisResourceValue{Name: test.name, Value: json.Number(test.value), Unit: test.unit, Kind: test.kind})
		if err != nil || name != "aries.resource."+test.wantName || unit != test.wantUnit || value != test.want {
			t.Fatal("unit/meaning changed", test, err)
		}
	}
	got, err := ProjectMetrics(resourceMetricsFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if got.Manifest.NotCollected[0].Reason != "resource_samples_require_separate_owner_stream_export" {
		t.Fatal("accounting export falsely claims resource samples absent")
	}
}

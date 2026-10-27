package performance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Resources are independent owner streams, not events placed on the Runner's
// monotonic clock. Empty receipts remain visible; schema-2 provider evidence
// retains repeated responses as references rather than independent samples.
type AnalysisResources struct {
	SchemaVersion int                      `json:"schema_version"`
	Streams       []AnalysisResourceStream `json:"streams"`
}

type AnalysisResourceStream struct {
	ID                     string                   `json:"stream_id"`
	Source                 string                   `json:"source"`
	Scope                  string                   `json:"measurement_scope"`
	AttemptID              string                   `json:"attempt_id,omitempty"`
	OwnerID                string                   `json:"owner_id"`
	Purpose                string                   `json:"purpose,omitempty"`
	Component              string                   `json:"component,omitempty"`
	ClockDomain            string                   `json:"clock_domain"`
	TimeBasis              string                   `json:"time_basis"`
	NominalIntervalSeconds *int                     `json:"nominal_interval_seconds,omitempty"`
	AnchorUTC              *time.Time               `json:"anchor_utc,omitempty"`
	IntegrityStatus        string                   `json:"integrity_status"`
	EvidenceRefs           []string                 `json:"evidence_refs"`
	Samples                []AnalysisResourceSample `json:"samples"`
	ProviderCounts         *ProviderResourceCounts  `json:"provider_counts,omitempty"`
	Delta                  []AnalysisResourceValue  `json:"delta,omitempty"`
	Diagnostics            []string                 `json:"diagnostics"`
}

// ProviderReadingReference addresses an inline reading in an earlier JSONL row.
// It never points to a guessed sample identity or another reference.
type ProviderReadingReference struct {
	SourceLine   uint64 `json:"source_line"`
	ReadingIndex int    `json:"reading_index"`
}

func (r *ProviderReadingReference) UnmarshalJSON(raw []byte) error {
	type reference ProviderReadingReference
	var decoded reference
	if err := decodeDocument(raw, &decoded, "source_line", "reading_index"); err != nil {
		return err
	}
	*r = ProviderReadingReference(decoded)
	return nil
}

type ProviderResourceCounts struct {
	RPCCount              uint64 `json:"rpc_count"`
	ReturnedReadingCount  uint64 `json:"returned_reading_count"`
	DuplicateReadingCount uint64 `json:"duplicate_reading_count"`
	UniqueReadingCount    uint64 `json:"unique_reading_count"`
	UnkeyedReadingCount   uint64 `json:"unkeyed_reading_count"`
}

// E2BMetricReadingKey preserves the original timestamp string and every JSON
// value, including numeric spelling. Only object ordering/whitespace are ignored.
// Missing/invalid timestamps cannot establish an independent provider sample.
func E2BMetricReadingKey(raw json.RawMessage) (string, bool) {
	if !json.Valid(raw) {
		return "", false
	}
	var fields map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&fields) != nil || fields == nil {
		return "", false
	}
	stamp, ok := fields["timestamp"].(string)
	if !ok {
		return "", false
	}
	if _, err := time.Parse(time.RFC3339Nano, stamp); err != nil {
		return "", false
	}
	canonical, err := json.Marshal(fields)
	return string(canonical), err == nil
}

type AnalysisResourceSample struct {
	Boundary                 string                  `json:"boundary,omitempty"`
	ID                       string                  `json:"sample_id"`
	EvidenceRef              string                  `json:"evidence_ref"`
	SourceLine               uint64                  `json:"source_line"`
	ReadingIndex             int                     `json:"reading_index"`
	Status                   string                  `json:"status"`
	ObservedAtUTC            *time.Time              `json:"observed_at_utc,omitempty"`
	TimeStatus               string                  `json:"time_status"`
	ProviderTimestamp        json.RawMessage         `json:"provider_timestamp,omitempty"`
	RequestedAtUTC           *time.Time              `json:"requested_at_utc,omitempty"`
	ClientRPCDurationNS      *int64                  `json:"client_rpc_duration_ns,omitempty"`
	SamplingStartedOffsetNS  *int64                  `json:"sampling_started_offset_ns,omitempty"`
	SamplingFinishedOffsetNS *int64                  `json:"sampling_finished_offset_ns,omitempty"`
	Values                   []AnalysisResourceValue `json:"values"`
	Unavailable              []string                `json:"unavailable,omitempty"`
}

type AnalysisResourceValue struct {
	Name           string      `json:"name"`
	Value          json.Number `json:"value"`
	Unit           string      `json:"unit"`
	Kind           string      `json:"kind"`
	ExportEligible bool        `json:"export_eligible"`
	Reason         string      `json:"reason,omitempty"`
}

func resourceStream(run, source, owner, attempt, scope, basis string) AnalysisResourceStream {
	id := stableAnalysisID("resource", run, source, attempt, owner)
	return AnalysisResourceStream{ID: id, Source: source, Scope: scope, AttemptID: attempt, OwnerID: owner,
		ClockDomain: id, TimeBasis: basis, IntegrityStatus: "complete", EvidenceRefs: []string{},
		Samples: []AnalysisResourceSample{}, Diagnostics: []string{}}
}

func resourceSample(ref string, line uint64, index int) AnalysisResourceSample {
	return AnalysisResourceSample{ID: stableAnalysisID("resource-sample", ref, strconv.FormatUint(line, 10), strconv.Itoa(index)),
		EvidenceRef: ref, SourceLine: line, ReadingIndex: index, Status: "collected", TimeStatus: "not_collected", Values: []AnalysisResourceValue{}}
}

func resourceValue(name string, value json.Number, unit, kind string, eligible bool, reason string) (AnalysisResourceValue, error) {
	n, err := value.Float64()
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
		return AnalysisResourceValue{}, fmt.Errorf("invalid resource value %s", name)
	}
	return AnalysisResourceValue{Name: name, Value: value, Unit: unit, Kind: kind, ExportEligible: eligible, Reason: reason}, nil
}

func (s *AnalysisSnapshot) resourceProblem(stream *AnalysisResourceStream, message string) {
	stream.IntegrityStatus = "incomplete"
	stream.Diagnostics = append(stream.Diagnostics, message)
	s.problem("resource " + stream.ID + ": " + message)
}

func (s *AnalysisSnapshot) addResource(stream AnalysisResourceStream) {
	if s.Resources == nil {
		s.Resources = &AnalysisResources{SchemaVersion: 1, Streams: []AnalysisResourceStream{}}
	}
	s.Resources.Streams = append(s.Resources.Streams, stream)
}

func (s *AnalysisSnapshot) resourceFile(root, name, clock string) ([]byte, string, bool) {
	data, err := os.ReadFile(filepath.Join(root, name))
	if os.IsNotExist(err) {
		return nil, "", false
	}
	ref := s.source(name, data, clock, "resource_observation_only", err)
	if err != nil {
		s.problem("resource source unreadable: " + filepath.ToSlash(name))
	}
	return data, ref, true
}

// Called only after run-result's identity and each selected attempt are checked.
func (s *AnalysisSnapshot) resourceSnapshot(root string) {
	s.controllerResourceSnapshot(root)
	for _, a := range s.Attempts {
		s.monitorResourceSnapshot(root, a)
		s.providerResourceSnapshot(root, a)
		s.cgroupResourceSnapshot(root, a)
	}
}

type controllerResourceEvidenceSample struct {
	Started    *int64                 `json:"sampling_started_offset_ns"`
	Finished   *int64                 `json:"sampling_finished_offset_ns"`
	CPU        map[string]json.Number `json:"cpu"`
	CPUStatus  string                 `json:"cpu_status"`
	RSS        *json.Number           `json:"process_lifetime_peak_rss_bytes"`
	Allocation map[string]json.Number `json:"allocation"`
	Heap       *json.Number           `json:"heap_alloc_bytes"`
}

type controllerResourceEvidence struct {
	Schema       int                               `json:"schema_version"`
	RunID        string                            `json:"run_id"`
	PID          int                               `json:"pid"`
	Scope        string                            `json:"measurement_scope"`
	Anchor       time.Time                         `json:"anchor_utc"`
	Status       string                            `json:"status"`
	StartWritten bool                              `json:"start_artifact_written"`
	Start        controllerResourceEvidenceSample  `json:"start"`
	End          *controllerResourceEvidenceSample `json:"end"`
	Delta        *struct {
		CPU        map[string]json.Number `json:"cpu"`
		Allocation map[string]json.Number `json:"allocation"`
	} `json:"delta"`
}

func controllerValues(cpu, allocation map[string]json.Number) ([]AnalysisResourceValue, error) {
	values := []AnalysisResourceValue{}
	for _, v := range []struct {
		key, name, unit string
		fields          map[string]json.Number
	}{
		{"user_ns", "cpu_user_ns", "ns", cpu}, {"system_ns", "cpu_system_ns", "ns", cpu},
		{"total_alloc_bytes", "total_alloc_bytes", "bytes", allocation}, {"mallocs", "mallocs", "count", allocation},
		{"num_gc", "num_gc", "count", allocation}, {"gc_pause_total_ns", "gc_pause_total_ns", "ns", allocation},
	} {
		number, exists := v.fields[v.key]
		if !exists {
			continue
		}
		value, err := resourceValue(v.name, number, v.unit, "cumulative_counter", true, "")
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func controllerSample(ref string, index int, anchor time.Time, raw controllerResourceEvidenceSample) (AnalysisResourceSample, error) {
	sample := resourceSample(ref, 0, index)
	if raw.Started == nil || raw.Finished == nil || *raw.Started < 0 || *raw.Finished < *raw.Started || anchor.IsZero() {
		return sample, fmt.Errorf("invalid controller sampling bracket")
	}
	sample.SamplingStartedOffsetNS, sample.SamplingFinishedOffsetNS = raw.Started, raw.Finished
	stamp := anchor.Add(time.Duration(*raw.Finished)).UTC()
	sample.ObservedAtUTC, sample.TimeStatus = &stamp, "verified"
	values, err := controllerValues(raw.CPU, raw.Allocation)
	if err != nil {
		return sample, err
	}
	sample.Values = values
	for _, v := range []struct {
		name   string
		number *json.Number
	}{
		{"process_lifetime_peak_rss_bytes", raw.RSS}, {"heap_alloc_bytes", raw.Heap},
	} {
		if v.number == nil {
			sample.Unavailable = append(sample.Unavailable, v.name)
			continue
		}
		value, err := resourceValue(v.name, *v.number, "bytes", "gauge", true, "")
		if err != nil {
			return sample, err
		}
		sample.Values = append(sample.Values, value)
	}
	if raw.CPUStatus != "collected" || len(raw.CPU) == 0 {
		sample.Unavailable = append(sample.Unavailable, "cpu")
	}
	return sample, nil
}

func (s *AnalysisSnapshot) controllerResourceSnapshot(root string) {
	startData, startRef, hasStart := s.resourceFile(root, "controller-resources.start.json", "controller_resource_anchor")
	endData, endRef, hasEnd := s.resourceFile(root, "controller-resources.end.json", "controller_resource_anchor")
	if !hasStart && !hasEnd {
		return
	}
	var first, last controllerResourceEvidence
	firstOK := hasStart && decodeDocument(startData, &first) == nil
	lastOK := hasEnd && decodeDocument(endData, &last) == nil
	identity := first
	if !firstOK {
		identity = last
	}
	stream := resourceStream(s.RunID, "controller_rusage_memstats", strconv.Itoa(identity.PID), "", "controller_shared_process_self", "controller_sampling_completed")
	stream.AnchorUTC = &identity.Anchor
	if hasStart {
		stream.EvidenceRefs = append(stream.EvidenceRefs, startRef)
	}
	if hasEnd {
		stream.EvidenceRefs = append(stream.EvidenceRefs, endRef)
	}
	defer func() { s.addResource(stream) }()
	valid := func(v controllerResourceEvidence) bool {
		return v.Schema == 1 && v.RunID == s.RunID && v.PID > 0 && v.Scope == stream.Scope && !v.Anchor.IsZero()
	}
	if !firstOK || !valid(first) {
		s.resourceProblem(&stream, "initial controller sample missing or invalid")
		return
	}
	initial, err := controllerSample(startRef, 0, first.Anchor, first.Start)
	if err != nil {
		s.resourceProblem(&stream, err.Error())
		return
	}
	stream.Samples = append(stream.Samples, initial)
	if !lastOK || !valid(last) || last.PID != first.PID || !last.Anchor.Equal(first.Anchor) || !last.StartWritten || last.End == nil {
		s.resourceProblem(&stream, "final controller sample missing or invalid")
		return
	}
	firstJSON, _ := json.Marshal(first.Start)
	lastJSON, _ := json.Marshal(last.Start)
	if !bytes.Equal(firstJSON, lastJSON) {
		s.resourceProblem(&stream, "controller initial sample changed in final report")
		return
	}
	final, err := controllerSample(endRef, 1, last.Anchor, *last.End)
	if err != nil {
		s.resourceProblem(&stream, err.Error())
		return
	}
	stream.Samples = append(stream.Samples, final)
	if *last.End.Started < *first.Start.Finished {
		s.resourceProblem(&stream, "controller sampling brackets overlap or reverse")
	}
	if last.Status != "complete" {
		s.resourceProblem(&stream, "controller final report incomplete")
	}
	if last.Delta == nil {
		s.resourceProblem(&stream, "controller delta missing")
		return
	}
	stream.Delta, err = controllerValues(last.Delta.CPU, last.Delta.Allocation)
	if err != nil {
		s.resourceProblem(&stream, err.Error())
		return
	}
	startValues, endValues := map[string]json.Number{}, map[string]json.Number{}
	for _, v := range initial.Values {
		startValues[v.Name] = v.Value
	}
	for _, v := range final.Values {
		endValues[v.Name] = v.Value
	}
	for _, v := range stream.Delta {
		a, okA := new(big.Int).SetString(string(startValues[v.Name]), 10)
		b, okB := new(big.Int).SetString(string(endValues[v.Name]), 10)
		d, okD := new(big.Int).SetString(string(v.Value), 10)
		if !okA || !okB || !okD || b.Cmp(a) < 0 || new(big.Int).Sub(b, a).Cmp(d) != 0 {
			s.resourceProblem(&stream, "controller delta disagrees with original counters")
			break
		}
	}
}

type resourceMonitorIndex struct {
	Schema      int    `json:"schema_version"`
	Scope       string `json:"scope"`
	RunID       string `json:"run_id"`
	TaskID      string `json:"task_id"`
	Status      string `json:"status"`
	SampleCount uint64 `json:"sample_count"`
	File        string `json:"resources_file"`
	Components  []struct {
		Component string `json:"component"`
		RuntimeID string `json:"runtime_id"`
		Count     uint64 `json:"sample_count"`
	} `json:"components"`
}

func (s *AnalysisSnapshot) monitorResourceSnapshot(root string, a AnalysisAttempt) {
	base := filepath.Join(a.OccurrenceID, "monitor")
	data, ref, present := s.resourceFile(root, filepath.Join(base, "resources.jsonl"), "monitor_source_observation")
	indexData, indexRef, indexPresent := s.resourceFile(root, filepath.Join(base, "index.json"), "monitor_source_observation")
	if !present && !indexPresent {
		return
	}
	var index resourceMonitorIndex
	indexOK := indexPresent && decodeDocument(indexData, &index) == nil && index.Schema >= 1 && index.Schema <= 5 && index.RunID == s.RunID && index.TaskID == a.OccurrenceID && index.Scope == "task" && index.File == "resources.jsonl" && index.Status == "succeeded"
	streams := []AnalysisResourceStream{}
	owners := map[string]int{}
	counts := map[string]uint64{}
	var line uint64
	valid := present && traceLines(data, func(raw []byte) bool {
		line++
		var row struct {
			Scope     string       `json:"scope"`
			RunID     string       `json:"run_id"`
			Sequence  uint64       `json:"sequence"`
			Time      time.Time    `json:"time"`
			TaskID    string       `json:"task_id"`
			Component string       `json:"component"`
			RuntimeID string       `json:"runtime_id"`
			CPU       *json.Number `json:"cpu_usage_nanoseconds"`
			Percent   *json.Number `json:"cpu_percent"`
			Memory    *json.Number `json:"memory_usage_bytes"`
			Limit     *json.Number `json:"memory_limit_bytes"`
		}
		if decodeDocument(raw, &row) != nil || row.RunID != s.RunID || row.TaskID != a.OccurrenceID || row.Scope != "task" || row.Sequence != line-1 || !validAnalysisIdentity(row.RuntimeID) || row.Component == "" {
			return false
		}
		key := row.Component + "/" + row.RuntimeID
		i, exists := owners[key]
		if !exists {
			i = len(streams)
			owners[key] = i
			stream := resourceStream(s.RunID, "monitor", row.RuntimeID, a.AttemptID, "container_resource", "source_observed_time")
			stream.Component = row.Component
			stream.EvidenceRefs = []string{ref}
			if indexPresent {
				stream.EvidenceRefs = append(stream.EvidenceRefs, indexRef)
			}
			streams = append(streams, stream)
		}
		sample := resourceSample(ref, line, 0)
		if !row.Time.IsZero() {
			stamp := row.Time.UTC()
			sample.ObservedAtUTC = &stamp
			sample.TimeStatus = "verified"
		}
		for _, v := range []struct {
			name, unit, kind string
			number           *json.Number
			eligible         bool
		}{
			{"cpu_usage_nanoseconds", "ns", "cumulative_counter", row.CPU, true}, {"cpu_percent", "percent", "gauge", row.Percent, false},
			{"memory_usage_bytes", "bytes", "gauge", row.Memory, true}, {"memory_limit_bytes", "bytes", "gauge", row.Limit, true},
		} {
			if v.number == nil {
				sample.Unavailable = append(sample.Unavailable, v.name)
				continue
			}
			reason := ""
			if !v.eligible {
				reason = "monitor_baseline_and_rate_semantics_not_projected"
			}
			value, err := resourceValue(v.name, *v.number, v.unit, v.kind, v.eligible, reason)
			if err != nil {
				return false
			}
			sample.Values = append(sample.Values, value)
		}
		streams[i].Samples = append(streams[i].Samples, sample)
		counts[key]++
		return true
	})
	if !indexOK || line != index.SampleCount {
		valid = false
	}
	if indexOK {
		for _, component := range index.Components {
			key := component.Component + "/" + component.RuntimeID
			if counts[key] != component.Count {
				valid = false
			}
			delete(counts, key)
		}
		if len(counts) > 0 {
			valid = false
		}
	}
	if len(streams) == 0 {
		stream := resourceStream(s.RunID, "monitor", a.OccurrenceID, a.AttemptID, "container_resource", "source_observed_time")
		if present {
			stream.EvidenceRefs = append(stream.EvidenceRefs, ref)
		}
		if indexPresent {
			stream.EvidenceRefs = append(stream.EvidenceRefs, indexRef)
		}
		streams = append(streams, stream)
	}
	for _, stream := range streams {
		if !valid {
			s.resourceProblem(&stream, "monitor rows, identity or finalized index invalid")
		}
		s.addResource(stream)
	}
}

type providerResourceEvidence struct {
	Schema            int                         `json:"schema_version"`
	RunID             string                      `json:"run_id"`
	OccurrenceID      string                      `json:"occurrence_id"`
	Owner             string                      `json:"sandbox_handle"`
	Purpose           string                      `json:"purpose"`
	Requested         time.Time                   `json:"requested_at_utc"`
	Duration          *int64                      `json:"client_rpc_duration_ns"`
	ReadingReferences []*ProviderReadingReference `json:"reading_references"`
	Snapshot          struct {
		Source                 string            `json:"source"`
		NominalIntervalSeconds *int              `json:"nominal_interval_seconds"`
		Metrics                []json.RawMessage `json:"metrics"`
		Units                  map[string]string `json:"units"`
	} `json:"snapshot"`
	Error string `json:"error"`
}

func providerResourceSample(ref string, line uint64, index int, receipt providerResourceEvidence, raw json.RawMessage) (AnalysisResourceSample, error) {
	sample := resourceSample(ref, line, index)
	sample.ClientRPCDurationNS = receipt.Duration
	if !receipt.Requested.IsZero() {
		stamp := receipt.Requested.UTC()
		sample.RequestedAtUTC = &stamp
	}
	if receipt.Error != "" {
		sample.Status = "failed"
		sample.Unavailable = append(sample.Unavailable, "provider_rpc_failed")
		return sample, nil
	}
	if len(raw) == 0 {
		sample.Status = "no_readings"
		sample.Unavailable = append(sample.Unavailable, "provider_readings")
		return sample, nil
	}
	var fields map[string]json.RawMessage
	if decodeDocument(raw, &fields) != nil {
		return sample, fmt.Errorf("invalid provider reading")
	}
	isE2B := receipt.Snapshot.Source == "e2b_sdk"
	timeKey := "ts"
	if isE2B {
		timeKey = "timestamp"
	}
	if stamp, ok := fields[timeKey]; ok && !bytes.Equal(bytes.TrimSpace(stamp), []byte("null")) {
		// Only scalar timestamps are retained; never arbitrary provider metadata.
		var scalar any
		decoder := json.NewDecoder(bytes.NewReader(stamp))
		decoder.UseNumber()
		if decoder.Decode(&scalar) != nil {
			return sample, fmt.Errorf("invalid provider timestamp")
		}
		switch scalar.(type) {
		case string, json.Number:
		default:
			return sample, fmt.Errorf("invalid provider timestamp type")
		}
		sample.ProviderTimestamp = append(json.RawMessage{}, stamp...)
		sample.TimeStatus = "unverified"
		if isE2B {
			var text string
			if json.Unmarshal(stamp, &text) == nil {
				if t, err := time.Parse(time.RFC3339Nano, text); err == nil {
					t = t.UTC()
					sample.ObservedAtUTC = &t
					sample.TimeStatus = "verified"
				}
			}
		}
	}
	for _, name := range []string{"cpu_count", "cpu_used_pct", "mem_total", "mem_used", "mem_cache", "disk_total", "disk_used", "mem_total_mib", "mem_used_mib"} {
		rawValue, exists := fields[name]
		if !exists || bytes.Equal(bytes.TrimSpace(rawValue), []byte("null")) {
			continue
		}
		var number json.Number
		if json.Unmarshal(rawValue, &number) != nil {
			return sample, fmt.Errorf("invalid provider gauge %s", name)
		}
		unit := "unknown"
		switch name {
		case "cpu_count":
			unit = "count"
		case "cpu_used_pct":
			unit = "percent"
		case "mem_total_mib", "mem_used_mib":
			unit = "MiB"
		default:
			if isE2B {
				unit = "bytes"
			}
		}
		// E2B 2.58.0 SandboxMetrics documents the byte fields. AgentSphere only
		// promises its explicitly suffixed/labelled gauges; unsuffixed fields stay raw.
		if !isE2B && unit != "unknown" && receipt.Snapshot.Units[name] != unit {
			unit = "unknown"
		}
		eligible := unit != "unknown"
		reason := ""
		if !eligible {
			reason = "unit_unverified"
		}
		value, err := resourceValue(name, number, unit, "gauge", eligible, reason)
		if err != nil {
			return sample, err
		}
		sample.Values = append(sample.Values, value)
	}
	return sample, nil
}

func (s *AnalysisSnapshot) providerResourceSnapshot(root string, a AnalysisAttempt) {
	name := filepath.Join(a.OccurrenceID, "sandbox-metrics.jsonl")
	data, ref, present := s.resourceFile(root, name, "provider_resource_timestamp")
	if !present {
		return
	}
	reportData, reportRef, reportPresent := s.resourceFile(root, filepath.Join(a.OccurrenceID, "cloud-observation.json"), "provider_resource_timestamp")
	var report struct {
		SDK struct {
			Status string `json:"status"`
			Count  int    `json:"sample_count"`
		} `json:"sandbox_sdk"`
	}
	reportOK := reportPresent && decodeDocument(reportData, &report) == nil && (report.SDK.Status == "succeeded" || report.SDK.Status == "not_run")
	streams := []AnalysisResourceStream{}
	owners := map[string]int{}
	type originalReading struct {
		owner, source, key string
	}
	originals := map[ProviderReadingReference]originalReading{}
	seen := map[string]map[string]bool{}
	successful := 0
	var line uint64
	valid := traceLines(data, func(raw []byte) bool {
		line++
		var row providerResourceEvidence
		if decodeDocument(raw, &row) != nil || (row.Schema != 1 && row.Schema != 2) || row.RunID != s.RunID || row.OccurrenceID != a.OccurrenceID || !validAnalysisIdentity(row.Owner) || (row.Purpose != "task" && row.Purpose != "evaluation") || row.Duration == nil || *row.Duration < 0 || row.Requested.IsZero() {
			return false
		}
		if len(row.ReadingReferences) > 0 && (row.Schema != 2 || row.Error != "" || row.Snapshot.Source != "e2b_sdk" || len(row.ReadingReferences) != len(row.Snapshot.Metrics)) {
			return false
		}
		if row.Snapshot.Source != "e2b_sdk" && row.Snapshot.Source != "agentsphere_gateway_envd_metrics" && row.Error == "" {
			return false
		}
		key := row.Owner
		i, exists := owners[key]
		if !exists {
			i = len(streams)
			owners[key] = i
			basis := "provider_timestamp"
			if row.Snapshot.Source != "e2b_sdk" {
				basis = "provider_timestamp_unit_unverified"
			}
			stream := resourceStream(s.RunID, row.Snapshot.Source, row.Owner, a.AttemptID, "sandbox_provider", basis)
			stream.Purpose = row.Purpose
			stream.ProviderCounts = &ProviderResourceCounts{}
			seen[key] = map[string]bool{}
			stream.NominalIntervalSeconds = row.Snapshot.NominalIntervalSeconds
			stream.EvidenceRefs = []string{ref}
			if reportPresent {
				stream.EvidenceRefs = append(stream.EvidenceRefs, reportRef)
			}
			streams = append(streams, stream)
		}
		if streams[i].Purpose != row.Purpose || (row.Error == "" && streams[i].Source != "" && streams[i].Source != row.Snapshot.Source) {
			return false
		}
		if streams[i].Source == "" && row.Snapshot.Source != "" {
			streams[i].Source = row.Snapshot.Source
			streams[i].TimeBasis = "provider_timestamp"
			if row.Snapshot.Source != "e2b_sdk" {
				streams[i].TimeBasis = "provider_timestamp_unit_unverified"
			}
		}
		counts := streams[i].ProviderCounts
		counts.RPCCount++
		if row.Error != "" || len(row.Snapshot.Metrics) == 0 {
			sample, err := providerResourceSample(ref, line, 0, row, nil)
			if err != nil {
				return false
			}
			streams[i].Samples = append(streams[i].Samples, sample)
		}
		if row.Error == "" {
			counts.ReturnedReadingCount += uint64(len(row.Snapshot.Metrics))
		}
		for index, reading := range row.Snapshot.Metrics {
			if row.Error != "" {
				break
			}
			if len(row.ReadingReferences) > 0 && row.ReadingReferences[index] != nil {
				reference := *row.ReadingReferences[index]
				original, exists := originals[reference]
				if !exists || reference.SourceLine == 0 || reference.SourceLine >= line || reference.ReadingIndex < 0 ||
					original.owner != row.Owner || original.source != row.Snapshot.Source || original.key == "" ||
					!bytes.Equal(bytes.TrimSpace(reading), []byte("null")) {
					return false
				}
				counts.DuplicateReadingCount++
				continue
			}
			if row.Schema == 2 && row.Snapshot.Source == "e2b_sdk" && bytes.Equal(bytes.TrimSpace(reading), []byte("null")) {
				return false
			}
			sample, err := providerResourceSample(ref, line, index, row, reading)
			if err != nil {
				return false
			}
			readingKey, keyed := "", false
			if row.Snapshot.Source == "e2b_sdk" {
				readingKey, keyed = E2BMetricReadingKey(reading)
			}
			duplicate := keyed && seen[row.Owner][readingKey]
			if !keyed {
				counts.UnkeyedReadingCount++
			} else {
				originals[ProviderReadingReference{SourceLine: line, ReadingIndex: index}] = originalReading{row.Owner, row.Snapshot.Source, readingKey}
				if duplicate {
					counts.DuplicateReadingCount++
				} else {
					counts.UniqueReadingCount++
					seen[row.Owner][readingKey] = true
				}
			}
			// Legacy snapshots retain their original receipt-level sample shape.
			// New evidence emits one sample per confirmed key, including inline
			// same-RPC duplicates that the writer deliberately did not reference.
			if row.Schema == 2 && duplicate {
				continue
			}
			streams[i].Samples = append(streams[i].Samples, sample)
		}
		if row.Error == "" {
			successful++
		}
		return true
	})
	if !reportOK || successful != report.SDK.Count {
		valid = false
	}
	if len(streams) == 0 {
		stream := resourceStream(s.RunID, "provider", a.OccurrenceID, a.AttemptID, "sandbox_provider", "provider_timestamp_unverified")
		stream.EvidenceRefs = []string{ref}
		if reportPresent {
			stream.EvidenceRefs = append(stream.EvidenceRefs, reportRef)
		}
		streams = append(streams, stream)
	}
	for _, stream := range streams {
		if !valid {
			s.resourceProblem(&stream, "provider rows, identity or finalized observer report invalid")
		}
		s.addResource(stream)
	}
}

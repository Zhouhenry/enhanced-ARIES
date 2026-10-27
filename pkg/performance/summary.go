package performance

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

const p99MinimumSamples = 1000

// Summary describes one recording, including any recoverable partial results.
// Status concerns recording integrity, independently of operation outcomes.
type Summary struct {
	SchemaVersion     int                `json:"schema_version"`
	RunID             string             `json:"run_id"`
	EmitterID         string             `json:"emitter_id"`
	ClockDomain       string             `json:"clock_domain"`
	Status            string             `json:"status"`
	Problems          []string           `json:"problems"`
	EventCount        uint64             `json:"event_count"`
	QuantileMethod    string             `json:"quantile_method"`
	QuantileMeaning   string             `json:"quantile_meaning"`
	P99MinimumSamples int                `json:"p99_minimum_samples"`
	Operations        []OperationSummary `json:"operations"`
	Arrivals          []ArrivalSummary   `json:"arrivals"`
}

type OutcomeCounts struct {
	Succeeded  uint64 `json:"succeeded"`
	Failed     uint64 `json:"failed"`
	Canceled   uint64 `json:"canceled"`
	Timeout    uint64 `json:"timeout"`
	Incomplete uint64 `json:"incomplete"`
}

// DurationStats contains only valid observations, never a zero for missing data.
type DurationStats struct {
	Samples uint64 `json:"samples"`
	MinNS   *int64 `json:"min_ns"`
	P50NS   *int64 `json:"p50_ns"`
	P95NS   *int64 `json:"p95_ns"`
	P99NS   *int64 `json:"p99_ns"`
	MaxNS   *int64 `json:"max_ns"`
}

type OperationSummary struct {
	LogicalTaskID string `json:"logical_task_id"`
	Operation     string `json:"operation"`
	Purpose       string `json:"purpose"`
	Started       uint64 `json:"started"`
	OutcomeCounts
	SuccessfulDurationNS DurationStats `json:"successful_duration_ns"`
}

type ArrivalSummary struct {
	LogicalTaskID string `json:"logical_task_id"`
	Planned       uint64 `json:"planned"`
	Admitted      uint64 `json:"admitted"`
	Dispatched    uint64 `json:"dispatched"`
	Finished      uint64 `json:"finished"`
	NotStarted    uint64 `json:"not_started"`
	OutcomeCounts
	AdmissionQueueNS DurationStats `json:"admission_queue_ns"`
}

type operationRecord struct {
	start, end *Event
	invalid    bool
}

type arrivalRecord struct {
	events  map[string]Event
	first   Event
	last    string
	invalid bool
}

type analysis struct {
	summary    Summary
	manifest   Manifest
	operations map[string]*operationRecord
	arrivals   map[string]*arrivalRecord
	sources    map[string][]byte
	events     []Event
	index      Index
	indexOK    bool
}

// Analyze reads a performance directory without modifying any artifacts. Even
// integrity failures return a report; callers must inspect the error and status.
func Analyze(directory string) (Summary, error) {
	a, err := analyze(directory)
	return a.summary, err
}

// Retain the exact bytes that were validated so projections cannot race a
// second read or independently reinterpret the same evidence.
func analyze(directory string) (analysis, error) {
	a := analysis{
		summary: Summary{SchemaVersion: SchemaVersion, Status: "complete", Problems: []string{},
			QuantileMethod: "nearest_rank", P99MinimumSamples: p99MinimumSamples,
			QuantileMeaning: "Descriptive sample quantiles, not independent statistical inference.",
			Operations:      []OperationSummary{}, Arrivals: []ArrivalSummary{}},
		operations: make(map[string]*operationRecord), arrivals: make(map[string]*arrivalRecord),
		sources: make(map[string][]byte),
	}
	manifestOK := true
	if err := a.document(directory, "manifest.json", &a.manifest); err != nil {
		a.problem("manifest: %v", err)
		manifestOK = false
	} else {
		a.summary.RunID, a.summary.EmitterID, a.summary.ClockDomain = a.manifest.RunID, a.manifest.EmitterID, a.manifest.ClockDomain
		if (a.manifest.SchemaVersion != 1 && a.manifest.SchemaVersion != 2) || a.manifest.RunID == "" || a.manifest.EmitterID == "" || a.manifest.ClockDomain == "" || a.manifest.StartedAtUTC.IsZero() || a.manifest.EventsFile != "runner-events.jsonl" || a.manifest.Scope != "scheduler_and_runner" {
			a.problem("manifest: unsupported schema, missing identity/anchor, or unexpected event source/scope")
			manifestOK = false
		}
	}
	var index Index
	indexOK := true
	if err := a.document(directory, "index.json", &index, "attempted", "written", "dropped", "write_failed"); err != nil {
		a.problem("index: %v; recording was not finalized", err)
		indexOK = false
	} else {
		if index.SchemaVersion != a.manifest.SchemaVersion || index.RunID != a.manifest.RunID || index.EmitterID != a.manifest.EmitterID {
			a.problem("index: schema or identity does not match manifest")
		}
		if index.Status != "complete" || index.Dropped != 0 || index.WriteFailed != 0 {
			a.problem("index: recording is incomplete (status=%q, dropped=%d, write_failed=%d)", index.Status, index.Dropped, index.WriteFailed)
		}
		if index.Written > index.Attempted || index.Dropped > index.Attempted-index.Written || index.WriteFailed != index.Attempted-index.Written-index.Dropped {
			a.problem("index: attempted/written/dropped/write_failed counts do not reconcile")
		}
	}
	a.index = index
	a.indexOK = indexOK
	data, err := os.ReadFile(filepath.Join(directory, "runner-events.jsonl"))
	var lastSequence, lines uint64
	if err != nil {
		a.problem("events: %v", err)
	} else {
		a.sources["runner-events.jsonl"] = data
		if len(data) > 0 && data[len(data)-1] != '\n' {
			a.problem("events: final JSONL record is not newline-terminated")
		}
		scanner := bufio.NewScanner(bytes.NewReader(data))
		scanner.Buffer(make([]byte, 4096), 1024*1024)
		for scanner.Scan() {
			lines++
			var event Event
			if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
				a.problem("events line %d: invalid JSON: %v", lines, err)
				continue
			}
			a.summary.EventCount++
			valid := manifestOK
			if err := requireFields(scanner.Bytes(), "observed_offset_ns"); err != nil {
				a.problem("events line %d: %v", lines, err)
				valid = false
			}
			if event.Sequence == 0 || event.Sequence != lastSequence+1 {
				a.problem("events line %d: noncontiguous sequence %d after %d", lines, event.Sequence, lastSequence)
				// A gap can leave later operations intact; duplicate or reordered
				// records cannot contribute a successful observation.
				valid = valid && event.Sequence > lastSequence
			}
			if event.Sequence > lastSequence {
				lastSequence = event.Sequence
			}
			if event.SchemaVersion != a.manifest.SchemaVersion || event.RunID != a.manifest.RunID || event.EmitterID != a.manifest.EmitterID || event.ClockDomain != a.manifest.ClockDomain {
				a.problem("events line %d: schema or identity does not match manifest", lines)
				continue
			}
			if event.ObservedOffsetNS < 0 || event.ObservedAtUTC.IsZero() {
				a.problem("events line %d: missing timestamp or negative observed offset", lines)
				valid = false
			}
			if manifestOK {
				a.add(event, valid)
				if valid {
					a.events = append(a.events, event)
				}
			}
		}
		if err := scanner.Err(); err != nil {
			a.problem("events: read failed: %v", err)
		}
	}
	if indexOK && (lines != index.Written || a.summary.EventCount != index.Written || lastSequence != index.Attempted) {
		a.problem("index/events counts disagree: lines=%d decoded=%d written=%d last_sequence=%d attempted=%d", lines, a.summary.EventCount, index.Written, lastSequence, index.Attempted)
	}
	if indexOK && index.EventsSHA256 != "" {
		digest := sha256.Sum256(data)
		if index.EventsBytes != int64(len(data)) || index.EventsSHA256 != hex.EncodeToString(digest[:]) {
			a.problem("index/events raw hash or byte count disagrees")
		}
	}
	if data, err := os.ReadFile(filepath.Join(directory, "late-after-close.json")); err == nil {
		a.sources["late-after-close.json"] = data
		a.problem("late-after-close marker: producer appended after finalized recording")
	} else if !os.IsNotExist(err) {
		a.problem("late-after-close marker: could not check recording finalization")
	}
	a.summarizeOperations()
	a.summarizeArrivals()
	if len(a.summary.Problems) != 0 {
		a.summary.Status = "incomplete"
		return a, fmt.Errorf("performance recording is incomplete (%d problems)", len(a.summary.Problems))
	}
	return a, nil
}

func (a *analysis) document(directory, name string, target any, required ...string) error {
	data, err := os.ReadFile(filepath.Join(directory, name))
	if err != nil {
		return err
	}
	a.sources[name] = data
	return decodeDocument(data, target, required...)
}

func decodeDocument(data []byte, target any, required ...string) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("expected exactly one JSON object")
	}
	if err := requireFields(raw, required...); err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

func requireFields(raw []byte, fields ...string) error {
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return err
	}
	for _, field := range fields {
		if value, ok := values[field]; !ok || string(value) == "null" {
			return fmt.Errorf("required field %q is missing or null", field)
		}
	}
	return nil
}

func (a *analysis) problem(format string, args ...any) {
	a.summary.Problems = append(a.summary.Problems, fmt.Sprintf(format, args...))
}

func (a *analysis) add(event Event, valid bool) {
	switch event.EventType {
	case "observation":
		if event.SchemaVersion != 2 || event.Operation == "" || event.DurationNS != nil || event.StartedAtUTC != nil || event.OperationID != "" || event.Status != "" && !terminalStatus(event.Status) {
			a.problem("sequence %d: invalid instantaneous observation", event.Sequence)
		}
	case "operation_started", "operation_finished":
		if event.OperationID == "" {
			a.problem("sequence %d: operation ID is missing", event.Sequence)
			return
		}
		record := a.operations[event.OperationID]
		if record == nil {
			record = &operationRecord{}
			a.operations[event.OperationID] = record
		}
		record.invalid = record.invalid || !valid
		target := &record.start
		if event.EventType == "operation_finished" {
			target = &record.end
		}
		if *target != nil {
			a.problem("operation %q: duplicate %s", event.OperationID, event.EventType)
			record.invalid = true
			return
		}
		*target = &event
	case "arrival_planned", "arrival_admitted", "arrival_dispatched", "arrival_finished", "arrival_not_started":
		a.addArrival(event, valid)
	default:
		a.problem("sequence %d: unknown event type %q", event.Sequence, event.EventType)
	}
}

func (a *analysis) addArrival(event Event, valid bool) {
	if event.ArrivalID == "" {
		a.problem("sequence %d: arrival ID is missing", event.Sequence)
		return
	}
	record := a.arrivals[event.ArrivalID]
	if record == nil {
		record = &arrivalRecord{events: make(map[string]Event), first: event}
		a.arrivals[event.ArrivalID] = record
	}
	record.invalid = record.invalid || !valid
	previous := map[string]string{"arrival_planned": "", "arrival_admitted": "arrival_planned", "arrival_dispatched": "arrival_admitted", "arrival_finished": "arrival_dispatched", "arrival_not_started": "arrival_planned"}
	_, duplicate := record.events[event.EventType]
	if duplicate || record.last != previous[event.EventType] {
		a.problem("arrival %q: duplicate or out-of-order %s after %s", event.ArrivalID, event.EventType, record.last)
		record.invalid = true
	}
	if previous, ok := record.events[record.last]; ok && event.ObservedOffsetNS < previous.ObservedOffsetNS {
		a.problem("arrival %q: observed offset moves backwards", event.ArrivalID)
		record.invalid = true
	}
	first := record.first
	if event.Identity != first.Identity || event.ParentOperationID != first.ParentOperationID || event.Operation != "arrival" || event.OperationID != "" || event.OccurrenceID == "" || !sameOffset(event.PlannedOffsetNS, first.PlannedOffsetNS) || !sameOffset(event.ScheduleAnchorOffsetNS, first.ScheduleAnchorOffsetNS) || event.PlannedOffsetNS == nil || event.ScheduleAnchorOffsetNS == nil || *event.PlannedOffsetNS < 0 || *event.ScheduleAnchorOffsetNS < 0 {
		a.problem("arrival %q: missing or inconsistent metadata", event.ArrivalID)
		record.invalid = true
	}
	wantsDuration := event.EventType == "arrival_admitted" || event.EventType == "arrival_finished"
	if wantsDuration && (event.DurationNS == nil || *event.DurationNS < 0) || !wantsDuration && event.DurationNS != nil {
		a.problem("arrival %q: invalid duration for %s", event.ArrivalID, event.EventType)
		record.invalid = true
	}
	validStatus := event.Status == "succeeded"
	if event.EventType == "arrival_finished" {
		validStatus = terminalStatus(event.Status)
	} else if event.EventType == "arrival_not_started" {
		validStatus = event.Status == "not_started" && event.Reason != ""
	}
	if !validStatus {
		a.problem("arrival %q: invalid status %q for %s", event.ArrivalID, event.Status, event.EventType)
		record.invalid = true
	}
	if !duplicate {
		record.events[event.EventType] = event
	}
	record.last = event.EventType
}

func sameOffset(a, b *int64) bool {
	return a != nil && b != nil && *a == *b
}

func terminalStatus(status string) bool {
	return status == "succeeded" || status == "failed" || status == "canceled" || status == "timeout"
}

func (counts *OutcomeCounts) add(status string) {
	switch status {
	case "succeeded":
		counts.Succeeded++
	case "failed":
		counts.Failed++
	case "canceled":
		counts.Canceled++
	case "timeout":
		counts.Timeout++
	default:
		counts.Incomplete++
	}
}

func (a *analysis) summarizeOperations() {
	type key struct{ task, operation, purpose string }
	groups := make(map[key]*OperationSummary)
	durations := make(map[key][]int64)
	ids := sortedKeys(a.operations)
	for _, id := range ids {
		record := a.operations[id]
		start, end := record.start, record.end
		metadata := start
		if metadata == nil {
			metadata = end
		}
		k := key{metadata.LogicalTaskID, metadata.Operation, metadata.Purpose}
		group := groups[k]
		if group == nil {
			group = &OperationSummary{LogicalTaskID: k.task, Operation: k.operation, Purpose: k.purpose}
			groups[k] = group
		}
		if start != nil {
			group.Started++
		}
		if start == nil || end == nil {
			a.problem("operation %q: missing start or terminal event", id)
			record.invalid = true
		} else {
			if start.Operation == "" || start.StartedAtUTC == nil || end.StartedAtUTC == nil || start.StartedAtUTC.IsZero() || !start.StartedAtUTC.Equal(*end.StartedAtUTC) || !start.StartedAtUTC.Equal(start.ObservedAtUTC) || start.Identity != end.Identity || start.Operation != end.Operation || start.ParentOperationID != end.ParentOperationID {
				a.problem("operation %q: missing or inconsistent metadata", id)
				record.invalid = true
			}
			if start.DurationNS != nil || start.Status != "" || !terminalStatus(end.Status) || end.DurationNS == nil || *end.DurationNS < 0 || end.Sequence <= start.Sequence || end.ObservedOffsetNS < start.ObservedOffsetNS || end.DurationNS != nil && *end.DurationNS != end.ObservedOffsetNS-start.ObservedOffsetNS {
				a.problem("operation %q: invalid order, duration, or status", id)
				record.invalid = true
			}
		}
		if record.invalid {
			group.Incomplete++
			continue
		}
		if parentID := start.ParentOperationID; parentID != "" {
			parent := a.operations[parentID]
			if parent == nil || parent.start == nil || parent.start.Sequence >= start.Sequence || parent.start.ObservedOffsetNS > start.ObservedOffsetNS || parent.end != nil && (parent.end.Sequence <= end.Sequence || parent.end.ObservedOffsetNS < end.ObservedOffsetNS) {
				a.problem("operation %q: missing parent or invalid parent timing", id)
				group.Incomplete++
				continue
			}
			p := parent.start.Identity
			if p.LogicalTaskID != "" && p.LogicalTaskID != start.LogicalTaskID || p.OccurrenceID != "" && p.OccurrenceID != start.OccurrenceID || p.ArrivalID != "" && p.ArrivalID != start.ArrivalID {
				a.problem("operation %q: identity conflicts with parent", id)
				group.Incomplete++
				continue
			}
		}
		group.OutcomeCounts.add(end.Status)
		if end.Status == "succeeded" {
			durations[k] = append(durations[k], *end.DurationNS)
		}
	}
	for k, group := range groups {
		group.SuccessfulDurationNS = durationStats(durations[k])
		a.summary.Operations = append(a.summary.Operations, *group)
	}
	sort.Slice(a.summary.Operations, func(i, j int) bool {
		a, b := a.summary.Operations[i], a.summary.Operations[j]
		if a.LogicalTaskID != b.LogicalTaskID {
			return a.LogicalTaskID < b.LogicalTaskID
		}
		if a.Operation != b.Operation {
			return a.Operation < b.Operation
		}
		return a.Purpose < b.Purpose
	})
}

func (a *analysis) summarizeArrivals() {
	groups := make(map[string]*ArrivalSummary)
	queues := make(map[string][]int64)
	for _, id := range sortedKeys(a.arrivals) {
		record := a.arrivals[id]
		task := record.first.LogicalTaskID
		group := groups[task]
		if group == nil {
			group = &ArrivalSummary{LogicalTaskID: task}
			groups[task] = group
		}
		for event, count := range map[string]*uint64{"arrival_planned": &group.Planned, "arrival_admitted": &group.Admitted, "arrival_dispatched": &group.Dispatched, "arrival_finished": &group.Finished, "arrival_not_started": &group.NotStarted} {
			if _, ok := record.events[event]; ok {
				*count++
			}
		}
		finished, hasFinished := record.events["arrival_finished"]
		_, notStarted := record.events["arrival_not_started"]
		if !hasFinished && !notStarted {
			a.problem("arrival %q: missing finished/not_started event", id)
			record.invalid = true
		}
		if record.invalid {
			group.Incomplete++
			continue
		}
		if hasFinished {
			group.OutcomeCounts.add(finished.Status)
			queues[task] = append(queues[task], *record.events["arrival_admitted"].DurationNS)
		}
	}
	for _, task := range sortedKeys(groups) {
		group := groups[task]
		group.AdmissionQueueNS = durationStats(queues[task])
		a.summary.Arrivals = append(a.summary.Arrivals, *group)
	}
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func durationStats(values []int64) DurationStats {
	stats := DurationStats{Samples: uint64(len(values))}
	if len(values) == 0 {
		return stats
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	stats.MinNS, stats.MaxNS = &values[0], &values[len(values)-1]
	stats.P50NS = &values[(len(values)*50+99)/100-1]
	stats.P95NS = &values[(len(values)*95+99)/100-1]
	if len(values) >= p99MinimumSamples {
		stats.P99NS = &values[(len(values)*99+99)/100-1]
	}
	return stats
}

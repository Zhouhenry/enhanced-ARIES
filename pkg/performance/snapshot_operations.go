package performance

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type sdkOperationEvidence struct {
	index, source    identityRow
	clock            *Clock
	started, records uint64
	operations       []identityOperation
	err              error
}

// AnalysisSDKCall has a duration in the adapter's clock, without a controller
// start anchor. It is an attribute, never a nested timeline interval.
type AnalysisSDKCall struct {
	Operation  string `json:"operation"`
	DurationNS int64  `json:"duration_ns"`
	Status     string `json:"status"`
}

// AnalysisUntimedOperation retains legacy outcomes without fabricating an origin.
type AnalysisUntimedOperation struct {
	AttemptID         string         `json:"attempt_id"`
	OperationID       string         `json:"operation_id"`
	Name              string         `json:"name"`
	Outcome           string         `json:"outcome"`
	DurationNS        *int64         `json:"duration_ns"`
	MeasurementStatus string         `json:"measurement_status"`
	EvidenceRef       string         `json:"evidence_ref"`
	SourceLine        uint64         `json:"source_line"`
	Attributes        map[string]any `json:"attributes,omitempty"`
}

func (s *AnalysisSnapshot) loadSDKOperations(root string, a traceAttempt) (*sdkOperationEvidence, error) {
	bad := errors.New("invalid operation identity source")
	name := filepath.Join(a.OccurrenceID, "e2b-operations-index.json")
	indexSource, err := s.identitySource(root, name)
	if err != nil {
		return nil, bad
	}
	if _, err := os.Lstat(filepath.Join(root, name+".incomplete")); !os.IsNotExist(err) {
		return nil, bad
	}
	var index struct {
		Schema   int             `json:"schema_version"`
		Status   string          `json:"status"`
		Records  *uint64         `json:"records"`
		Started  *uint64         `json:"started"`
		Ended    *uint64         `json:"ended"`
		Problems []string        `json:"problems"`
		FileIO   json.RawMessage `json:"file_io"`
		Clock    *Clock          `json:"clock"`
	}
	if identityJSON(indexSource.data, &index, true) != nil || !identityCanonicalKeys(indexSource.data, "schema_version", "status", "records", "started", "ended", "problems", "file_io", "clock") || index.Schema != 1 || index.Status != "complete" || index.Records == nil || index.Started == nil || index.Ended == nil || *index.Started != *index.Ended || len(index.Problems) != 0 {
		return nil, bad
	}
	if len(index.FileIO) != 0 {
		// Optional metering is evidence metadata, never an identity or timing source.
		var counters FileIOCounters
		var fields map[string]json.RawMessage
		keys := []string{"measurement_scope", "write_calls_started", "write_calls_returned", "write_requested_bytes", "write_returned_bytes", "write_errors", "short_writes", "invalid_write_returns", "sync_calls_started", "sync_calls_returned", "sync_errors"}
		if identityJSON(index.FileIO, &counters, true) != nil || json.Unmarshal(index.FileIO, &fields) != nil || len(fields) != len(keys) || counters.Scope != "e2b_operation_jsonl_only" {
			return nil, bad
		}
		for _, key := range keys {
			if len(fields[key]) == 0 || bytes.Equal(bytes.TrimSpace(fields[key]), []byte("null")) {
				return nil, bad
			}
		}
	}
	source, err := s.identitySource(root, filepath.Join(a.OccurrenceID, "e2b-operations.jsonl"))
	if err != nil {
		return nil, bad
	}
	rows, err := identityRows(source)
	if err != nil || uint64(len(rows)) != *index.Records {
		return nil, bad
	}
	evidence := &sdkOperationEvidence{index: indexSource, source: source, clock: index.Clock, started: *index.Started, records: *index.Records}
	starts, ends := map[uint64]identityOperation{}, map[uint64]bool{}
	result := []identityOperation{}
	for i, row := range rows {
		var v struct {
			Schema        int                 `json:"schema_version"`
			Sequence      uint64              `json:"sequence"`
			Event         string              `json:"event"`
			OperationID   uint64              `json:"operation_id"`
			Operation     string              `json:"operation"`
			Execution     string              `json:"execution_id"`
			Identity      *NativeToolIdentity `json:"native_identity"`
			Process       string              `json:"process_id"`
			Sandbox       string              `json:"sandbox_handle"`
			Run           string              `json:"run_id"`
			Task          string              `json:"task_id"`
			Receipt       json.RawMessage     `json:"guest_identity_receipt"`
			Exit          *int                `json:"exit_code"`
			GuestDuration *int64              `json:"guest_duration_ns"`
			Status        string              `json:"status"`
			Offset        *int64              `json:"observed_offset_ns"`
			Duration      *int64              `json:"client_duration_ns"`
			Logical       string              `json:"logical_task_id"`
			Purpose       string              `json:"purpose"`
			SDKCalls      []AnalysisSDKCall   `json:"sdk_calls"`
		}
		if identityJSON(row.data, &v, false) != nil || !identityCanonicalKeys(row.data, "schema_version", "sequence", "event", "operation_id", "operation", "execution_id", "native_identity", "process_id", "sandbox_handle", "run_id", "task_id", "guest_identity_receipt", "exit_code", "guest_duration_ns", "status", "observed_offset_ns", "client_duration_ns", "logical_task_id", "purpose", "sdk_calls") || v.Schema != 1 || v.Sequence != uint64(i+1) || v.OperationID == 0 || v.Run != s.RunID || v.Task != a.OccurrenceID {
			return nil, bad
		}
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(row.data, &fields)
		if raw, present := fields["native_identity"]; present {
			if v.Identity == nil || decodeIdentityMetadata(raw, v.Identity, true) != nil {
				return nil, bad
			}
		}
		if v.Identity != nil && (v.Identity.Validate() != nil || v.Identity.ScopeID != a.AttemptID || v.Identity.AgentSessionID != a.AgentSessionID || !validAnalysisIdentity(v.Execution) || !validAnalysisIdentity(v.Process) || !validAnalysisIdentity(v.Sandbox)) {
			return nil, bad
		}
		if len(v.Receipt) != 0 && (v.Identity == nil || v.Event != "end" || v.Operation != "exec.wait") {
			return nil, bad
		}
		op := identityOperation{row: row, id: v.Identity, event: v.Event, operation: v.Operation, operationID: v.OperationID, execution: v.Execution, process: v.Process, sandbox: v.Sandbox, receipt: v.Receipt, status: v.Status, offset: v.Offset, duration: v.Duration, exit: v.Exit, guestDuration: v.GuestDuration, calls: v.SDKCalls, logical: v.Logical, purpose: v.Purpose, terminal: v.Exit != nil && v.GuestDuration != nil && *v.GuestDuration >= 0 && (v.Status == "succeeded" || v.Status == "failed")}
		if v.Event == "start" {
			if _, exists := starts[v.OperationID]; exists {
				return nil, bad
			}
			starts[v.OperationID] = op
		} else if v.Event == "end" {
			start, exists := starts[v.OperationID]
			if !exists || ends[v.OperationID] || start.operation != op.operation || start.execution != op.execution || start.process != op.process || start.sandbox != op.sandbox || !sameNativeIdentity(start.id, op.id) {
				return nil, bad
			}
			ends[v.OperationID] = true
		} else {
			return nil, bad
		}
		result = append(result, op)
	}
	if uint64(len(starts)) != *index.Started || len(starts) != len(ends) {
		return nil, bad
	}
	evidence.operations = result
	return evidence, nil
}

func validClientOperation(name string) bool {
	switch name {
	case "exec.wait", "adapter_rpc.create", "adapter_rpc.kill", "adapter_rpc.start", "adapter_rpc.stdin", "adapter_rpc.process", "adapter_rpc.kill_process", "adapter_rpc.events", "adapter_rpc.upload", "adapter_rpc.download", "adapter_rpc.guest", "adapter_rpc.metrics":
		return true
	}
	return false
}

func validOperationOutcome(status string) bool {
	return status == "succeeded" || status == "failed" || status == "canceled" || status == "timeout"
}

func validAnalysisSDKCall(call AnalysisSDKCall) bool {
	if call.DurationNS < 0 || (call.Status != "succeeded" && call.Status != "failed") {
		return false
	}
	switch call.Operation {
	case "sandbox.create", "sandbox.kill", "sandbox.get_info", "sandbox.get_metrics", "controlplane.create", "controlplane.get_info", "gateway.metrics", "commands.run", "commands.send_stdin", "commands.close_stdin", "commands.kill", "commands.list", "files.write", "files.read":
		return true
	}
	return false
}

func (s *AnalysisSnapshot) sdkOperationSnapshot(root string, a traceAttempt) *sdkOperationEvidence {
	base := filepath.Join(root, a.OccurrenceID)
	_, indexErr := os.Lstat(filepath.Join(base, "e2b-operations-index.json"))
	_, dataErr := os.Lstat(filepath.Join(base, "e2b-operations.jsonl"))
	if os.IsNotExist(indexErr) && os.IsNotExist(dataErr) {
		return nil
	}
	evidence, err := s.loadSDKOperations(root, a)
	coverage := SourceCoverage{Source: "sandbox_client_operations", AttemptID: a.AttemptID, Integrity: "incomplete", Status: "incomplete", Missing: []string{"independent_expected_operation_count"}}
	defer func() { s.Coverage = append(s.Coverage, coverage) }()
	if err != nil {
		s.problem("SDK operation source/index invalid for " + a.AttemptID)
		return &sdkOperationEvidence{err: err}
	}
	coverage.ObservedEvents, coverage.ExpectedEvents = evidence.records, &evidence.records
	coverage.Started, coverage.Paired = evidence.started, evidence.started
	coverage.Integrity, coverage.Status = "complete", "unknown"
	aligned := false
	if c := evidence.clock; c != nil {
		for _, known := range s.Clocks {
			if c.ClockDomain == s.Summary.ClockDomain && c.ClockDomain == known.ID && c.EmitterID == known.EmitterID && known.OriginUTC != nil && !c.StartedAtUTC.IsZero() && c.StartedAtUTC.Equal(*known.OriginUTC) {
				aligned = true
			}
		}
	}
	valid := true
	starts := map[uint64]identityOperation{}
	for _, op := range evidence.operations {
		if !validClientOperation(op.operation) || op.duration != nil && *op.duration < 0 || op.guestDuration != nil && *op.guestDuration < 0 {
			valid = false
		}
		for _, call := range op.calls {
			if !validAnalysisSDKCall(call) {
				valid = false
			}
		}
		if op.event == "start" {
			starts[op.operationID] = op
		} else {
			start := starts[op.operationID]
			if op.status != "" && !validOperationOutcome(op.status) {
				valid = false
			}
			if aligned && (start.offset == nil || op.offset == nil || *start.offset < 0 || *op.offset < *start.offset || op.duration == nil || *op.duration != *op.offset-*start.offset || !validOperationOutcome(op.status)) {
				valid = false
			}
		}
		if evidence.clock == nil && op.offset != nil {
			valid = false
		}
	}
	if !valid || evidence.clock != nil && !aligned {
		coverage.Integrity, coverage.Status = "incomplete", "incomplete"
		coverage.Missing = append(coverage.Missing, "invalid_client_clock_or_boundaries")
		s.problem("SDK operation clock, duration or outcome invalid for " + a.AttemptID)
		return evidence
	}
	if !aligned {
		coverage.Status = "not_collected"
		coverage.Missing = append(coverage.Missing, "controller_monotonic_anchor")
	} else {
		for i := range s.Sources {
			if s.Sources[i].Ref == evidence.source.ref || s.Sources[i].Ref == evidence.index.ref {
				s.Sources[i].ClockDomain, s.Sources[i].Alignment = evidence.clock.ClockDomain, "local_monotonic_anchor"
			}
		}
	}
	points := map[uint64]AnalysisEvent{}
	for _, op := range evidence.operations {
		id := stableAnalysisID("operation", s.RunID, a.AttemptID, evidence.source.ref, fmt.Sprint(op.operationID))
		scope := "controller_adapter_rpc"
		if op.operation == "exec.wait" {
			scope = "controller_process_submission_to_wait_return"
		}
		attributes := map[string]any{"execution_id": op.execution, "process_id": op.process, "client_operation_id": op.operationID, "status": op.status}
		if op.event == "end" {
			attributes["exit_code"], attributes["guest_duration_ns"] = op.exit, op.guestDuration
			attributes["sdk_calls_duration_only"] = op.calls
			attributes["guest_timing_scope"] = "guest_wrapper_duration_only_no_controller_anchor"
		}
		if !aligned {
			if op.event == "end" {
				status, measurement := op.status, "duration_only"
				if status == "" {
					status = "unknown"
				}
				if op.duration == nil {
					measurement = "not_collected"
				}
				s.UntimedOperations = append(s.UntimedOperations, AnalysisUntimedOperation{AttemptID: a.AttemptID, OperationID: id, Name: op.operation, Outcome: status, DurationNS: op.duration, MeasurementStatus: measurement, EvidenceRef: op.row.ref, SourceLine: op.row.line, Attributes: attributes})
			}
			continue
		}
		identity := AnalysisIdentity{AttemptID: a.AttemptID, Identity: Identity{OccurrenceID: a.OccurrenceID, LogicalTaskID: a.LogicalTaskID, Purpose: op.purpose}}
		point := analysisEvent(op.row.ref, op.row.line, "sandbox_client."+op.event, "sandbox_client", evidence.clock.ClockDomain, scope, *op.offset, identity, attributes)
		s.Events = append(s.Events, point)
		if op.event == "start" {
			points[op.operationID] = point
		} else {
			s.interval(id, op.operation, "sandbox_client", scope, op.status, points[op.operationID], point, attributes)
		}
	}
	return evidence
}

package e2b

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/performance"
)

// These durations have different boundaries. Never subtract or sum nested RPC,
// SDK and guest measurements to infer harness overhead or guest CPU time.
type SDKCall struct {
	Operation  string `json:"operation"`
	DurationNS int64  `json:"duration_ns"`
	Status     string `json:"status"`
}

func validSDKCall(call SDKCall) bool {
	if call.DurationNS < 0 || (call.Status != "succeeded" && call.Status != "failed") {
		return false
	}
	switch call.Operation {
	case "sandbox.create", "sandbox.kill", "sandbox.get_info", "sandbox.get_metrics", "controlplane.create", "controlplane.get_info", "gateway.metrics", "commands.run", "commands.send_stdin", "commands.close_stdin", "commands.kill", "commands.list", "files.write", "files.read":
		return true
	default:
		return false
	}
}

type CreationEvidence struct {
	Provider         string `json:"provider,omitempty"`
	SDKVersion       string `json:"sdk_version,omitempty"`
	SDKPinnedVersion string `json:"sdk_pinned_version,omitempty"`
	Template         string `json:"template"`
	TimeoutSeconds   int    `json:"timeout_seconds"`
	// The public create result does not expose these. Raw resource gauges live
	// in sandbox-metrics.jsonl; do not infer region from an endpoint hostname.
	Resources any     `json:"resources"`
	Region    *string `json:"region"`
	ColdWarm  string  `json:"cold_warm"`
}

// GuestBoundaries are durations in the guest wrapper's own monotonic clock.
// They are independent of the client's RPC clock and must never be converted
// into host timestamps or subtracted across machines. A nil FirstOutputNS
// means no non-empty stdout/stderr chunk was observed.
type GuestBoundaries struct {
	PopenReturnNS       *int64 `json:"popen_return_ns,omitempty"`
	FirstOutputNS       *int64 `json:"first_output_ns,omitempty"`
	ChildExitObservedNS *int64 `json:"child_exit_observed_ns,omitempty"`
	PipeEOFNS           *int64 `json:"pipe_eof_ns,omitempty"`
}

type operationRecord struct {
	SchemaVersion        int                             `json:"schema_version"`
	Sequence             uint64                          `json:"sequence"`
	Event                string                          `json:"event"`
	OperationID          uint64                          `json:"operation_id"`
	ExecutionID          string                          `json:"execution_id,omitempty"`
	NativeIdentity       *performance.NativeToolIdentity `json:"native_identity,omitempty"`
	GuestIdentityReceipt json.RawMessage                 `json:"guest_identity_receipt,omitempty"`
	ProcessID            string                          `json:"process_id,omitempty"`
	Operation            string                          `json:"operation"`
	RunID                string                          `json:"run_id"`
	TaskID               string                          `json:"task_id"`
	LogicalTaskID        string                          `json:"logical_task_id,omitempty"`
	Purpose              string                          `json:"purpose"`
	SandboxHandle        string                          `json:"sandbox_handle"`
	StartedAtUTC         time.Time                       `json:"started_at_utc"`
	ObservedOffsetNS     *int64                          `json:"observed_offset_ns,omitempty"`
	ClientDurationNS     *int64                          `json:"client_duration_ns,omitempty"`
	Status               string                          `json:"status,omitempty"`
	ExitCode             *int                            `json:"exit_code,omitempty"`
	StdoutBytes          *int64                          `json:"stdout_bytes,omitempty"`
	StderrBytes          *int64                          `json:"stderr_bytes,omitempty"`
	FileBytes            *int64                          `json:"file_bytes,omitempty"`
	GuestDurationNS      *int64                          `json:"guest_duration_ns,omitempty"`
	GuestBoundaries      *GuestBoundaries                `json:"guest_boundaries,omitempty"`
	SDKCalls             []SDKCall                       `json:"sdk_calls,omitempty"`
	Creation             *CreationEvidence               `json:"creation,omitempty"`
}

type operationIndex struct {
	SchemaVersion int                         `json:"schema_version"`
	Status        string                      `json:"status"`
	Records       uint64                      `json:"records"`
	Started       uint64                      `json:"started"`
	Ended         uint64                      `json:"ended"`
	Problems      []string                    `json:"problems,omitempty"`
	FileIO        *performance.FileIOCounters `json:"file_io,omitempty"`
	Clock         *performance.Clock          `json:"clock,omitempty"`
}

type operationLog struct {
	mu                     sync.Mutex
	file, index            *os.File
	path, indexPath        string
	started                time.Time
	duration               time.Duration
	sequence, begun, ended uint64
	bytes                  int64
	fileIO                 performance.FileIOCounters
	problems               []string
	finished               bool
	clock                  *performance.Clock
}

type operationSpan struct {
	log     *operationLog
	started time.Time
	record  operationRecord
}

func newOperationLog(rootPath, taskID string, clock *performance.Clock) *operationLog {
	o := &operationLog{started: time.Now(), fileIO: performance.FileIOCounters{Scope: "e2b_operation_jsonl_only"}}
	if clock != nil {
		copy := *clock
		o.clock = &copy
	}
	if taskID == "" || taskID == "." || taskID == ".." || filepath.Base(taskID) != taskID || strings.ContainsAny(taskID, `/\\`) {
		o.problem("invalid_occurrence_artifact_directory")
		return o
	}
	if err := os.MkdirAll(rootPath, 0700); err != nil {
		o.problem("artifact_directory_unavailable")
		return o
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		o.problem("artifact_root_unavailable")
		return o
	}
	defer root.Close()
	if err := root.MkdirAll(taskID, 0700); err != nil {
		o.problem("artifact_directory_unavailable")
		return o
	}
	info, err := root.Stat(taskID)
	if err != nil || info.Mode().Perm()&0077 != 0 {
		o.problem("artifact_directory_not_private")
		return o
	}
	o.path = filepath.Join(rootPath, taskID, "e2b-operations.jsonl")
	o.indexPath = filepath.Join(rootPath, taskID, "e2b-operations-index.json")
	o.index, err = root.OpenFile(filepath.Join(taskID, "e2b-operations-index.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		o.problem("operation_index_create_failed")
		o.indexPath = ""
		return o
	}
	o.writeIndex("incomplete")
	o.file, err = root.OpenFile(filepath.Join(taskID, "e2b-operations.jsonl"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		o.problem("operation_log_create_failed")
		o.path = ""
	}
	return o
}

func (o *operationLog) problem(code string) {
	for _, existing := range o.problems {
		if existing == code {
			return
		}
	}
	o.problems = append(o.problems, code)
}

func (o *operationLog) write(record operationRecord) {
	if o.file == nil {
		return
	}
	content, err := json.Marshal(record)
	if err != nil {
		o.problem("operation_encode_failed")
		return
	}
	content = append(content, '\n')
	if o.bytes+int64(len(content)) > 64<<20 {
		o.problem("operation_artifact_limit")
		return
	}
	n, err := o.fileIO.Write(o.file, content)
	o.bytes += int64(n)
	if err != nil || n != len(content) {
		o.problem("operation_write_failed")
	}
}

func (o *operationLog) invalidate(code string) {
	o.problem(code)
	if o.indexPath != "" {
		f, err := os.OpenFile(o.indexPath+".incomplete", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			_, writeErr := f.WriteString(code + "\n")
			if errors.Join(writeErr, f.Sync(), f.Close()) != nil {
				o.problem("incomplete_marker_write_failed")
			}
		} else if !errors.Is(err, os.ErrExist) {
			o.problem("incomplete_marker_create_failed")
		}
	}
}

func (o *operationLog) begin(s *Sandbox, operation string) *operationSpan {
	return o.beginAt(context.Background(), s, operation, time.Now(), "")
}

func (o *operationLog) beginAt(ctx context.Context, s *Sandbox, operation string, started time.Time, processID string) *operationSpan {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.finished {
		// A late operation must invalidate even an earlier complete index.
		o.invalidate("operation_after_finalization")
		return nil
	}
	o.begun++
	o.sequence++
	record := operationRecord{SchemaVersion: 1, Sequence: o.sequence, Event: "start", OperationID: o.begun,
		ExecutionID: performance.ExecutionID(ctx), ProcessID: processID,
		Operation: operation, RunID: s.runID, TaskID: s.taskID, LogicalTaskID: s.manager.options.LogicalTaskID,
		Purpose: s.purpose, SandboxHandle: s.id, StartedAtUTC: started.UTC()}
	if o.clock != nil {
		offset := o.clock.Offset(started)
		record.ObservedOffsetNS = &offset
	}
	if processID != "" {
		if identity, ok := performance.NativeToolIdentityFromContext(ctx); ok {
			record.NativeIdentity = &identity
		}
	}
	o.write(record)
	return &operationSpan{log: o, started: started, record: record}
}

func (span *operationSpan) end(err error, exitCode *int, stdoutBytes, stderrBytes, fileBytes, guestDuration *int64, boundaries *GuestBoundaries, calls []SDKCall, creation *CreationEvidence) {
	if span == nil {
		return
	}
	ended := time.Now()
	duration := ended.Sub(span.started).Nanoseconds()
	o := span.log
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.finished {
		o.problem("operation_unfinished_at_finalization")
		return
	}
	status := "succeeded"
	if err != nil {
		status = "failed"
		if errors.Is(err, context.Canceled) {
			status = "canceled"
		}
		if errors.Is(err, context.DeadlineExceeded) {
			status = "timeout"
		}
	} else if exitCode != nil && *exitCode != 0 {
		status = "failed"
	}
	o.ended++
	o.sequence++
	record := span.record
	record.Event, record.Sequence, record.Status = "end", o.sequence, status
	if o.clock != nil {
		offset := o.clock.Offset(ended)
		record.ObservedOffsetNS = &offset
	}
	record.ClientDurationNS, record.ExitCode = &duration, exitCode
	record.StdoutBytes, record.StderrBytes, record.FileBytes = stdoutBytes, stderrBytes, fileBytes
	record.GuestDurationNS, record.GuestBoundaries, record.SDKCalls, record.Creation = guestDuration, boundaries, calls, creation
	o.write(record)
}

func (o *operationLog) writeIndex(status string) {
	if o.index == nil {
		return
	}
	content, err := json.MarshalIndent(operationIndex{SchemaVersion: 1, Status: status, Records: o.sequence,
		Started: o.begun, Ended: o.ended, Problems: o.problems, FileIO: &o.fileIO, Clock: o.clock}, "", "  ")
	if err == nil {
		_, err = o.index.WriteAt(append(content, '\n'), 0)
	}
	if err == nil {
		err = o.index.Truncate(int64(len(content) + 1))
	}
	if err == nil {
		err = o.index.Sync()
	}
	if err != nil {
		o.invalidate("operation_index_write_failed")
	}
}

// FinishObservation follows sandbox cleanup. It does not alter cleanup status.
// Missing terminal events, write failures and remaining ownership are incomplete.
func (m *Manager) FinishObservation() (core.ObserverResult, error) {
	m.mu.Lock()
	logs := make([]*operationLog, 0, len(m.logs))
	for _, o := range m.logs {
		logs = append(logs, o)
	}
	remaining := len(m.owned)
	m.mu.Unlock()
	report := core.ObserverResult{Status: core.StatusNotRun, Reason: "client RPC and SDK durations are nested; exec guest wall time includes pipe drain; these are not ToolCall or CPU measurements"}
	var failures []error
	for _, o := range logs {
		o.mu.Lock()
		if !o.finished {
			if remaining != 0 {
				o.problem("owned_sandboxes_remaining")
			}
			if o.begun != o.ended {
				o.problem("unfinished_operations")
			}
			if o.file != nil {
				if err := errors.Join(o.fileIO.Sync(o.file), o.file.Close()); err != nil {
					o.problem("operation_close_failed")
				}
			}
			status := "complete"
			if len(o.problems) != 0 {
				status = "incomplete"
			}
			o.writeIndex(status)
			if o.index != nil {
				if err := o.index.Close(); err != nil {
					o.invalidate("operation_index_close_failed")
				}
			}
			o.duration = time.Since(o.started)
			o.finished = true
		}
		if o.path != "" && o.file != nil {
			report.LogPaths = append(report.LogPaths, o.path)
		}
		if o.indexPath != "" {
			report.LogPaths = append(report.LogPaths, o.indexPath)
		}
		report.SampleCount += int(o.ended)
		if o.duration > report.Duration {
			report.Duration = o.duration
		}
		if len(o.problems) != 0 {
			failures = append(failures, errors.New("E2B operation observation incomplete: "+strings.Join(o.problems, ", ")))
		}
		o.mu.Unlock()
	}
	if len(logs) > 0 {
		report.Status = core.StatusSucceeded
	}
	err := errors.Join(failures...)
	if err != nil {
		report.Status, report.Error = core.StatusFailed, err.Error()
	}
	return report, err
}

package e2b

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/performance"
)

func readOperationRecords(t *testing.T, m *Manager) ([]operationRecord, operationIndex) {
	t.Helper()
	path := filepath.Join(m.options.OutputDir, "task", "e2b-operations.jsonl")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"secret-command", "secret-env", "provider-private-id", "secret-path", strings.Repeat("t", 32)} {
		if bytes.Contains(content, []byte(secret)) {
			t.Fatalf("sensitive material in operation evidence: %s", secret)
		}
	}
	var records []operationRecord
	for _, line := range bytes.Split(bytes.TrimSpace(content), []byte{'\n'}) {
		var record operationRecord
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	var index operationIndex
	content, err = os.ReadFile(filepath.Join(m.options.OutputDir, "task", "e2b-operations-index.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(content, &index); err != nil {
		t.Fatal(err)
	}
	return records, index
}

func TestOperationEvidenceSeparatesRPCGuestAndFileBytes(t *testing.T) {
	m := testManager(t, func(action string, body map[string]any) (int, any) {
		switch action {
		case "create":
			return 200, map[string]any{"sandbox": body["sandbox"], "state": "active", "sdk_version": "2.58.0", "sdk_pinned_version": "2.58.0", "template": "pinned", "timeout_seconds": 60,
				"provider_id": "provider-private-id", "sdk_calls": []SDKCall{{Operation: "sandbox.create", DurationNS: 50, Status: "succeeded"}}}
		case "start":
			return 200, map[string]any{"process": body["process"], "pid": 1}
		case "stdin":
			return 200, map[string]bool{"accepted": true}
		case "events":
			return 200, map[string]any{"cursor": 3, "done": true, "events": []any{
				map[string]any{"stream": "stdout", "data_base64": "AP8="}, map[string]any{"stream": "stderr", "data_base64": "eA=="}, map[string]any{"exit_code": 7, "guest_duration_ns": 99,
					"popen_return_ns": 3, "first_output_ns": 4, "child_exit_observed_ns": 5, "pipe_eof_ns": 6}}}
		case "upload":
			return 200, map[string]any{"bytes": 2, "sdk_calls": []SDKCall{{Operation: "files.write", DurationNS: 12, Status: "succeeded"}}}
		case "download":
			return 200, map[string]any{"bytes": 2, "data_base64": "AP8="}
		case "kill":
			return 200, map[string]bool{"absence_confirmed": true}
		default:
			t.Errorf("unexpected %s", action)
			return 500, nil
		}
	})
	m.options.LogicalTaskID = "logical-task"
	recorder, err := performance.Open(t.TempDir(), "run")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = recorder.Close() })
	m.options.Clock = performance.ClockFromContext(performance.WithRecorder(context.Background(), recorder))
	// Calls deliberately use Background, as SSH and final owner cleanup do.
	live, err := m.Start(context.Background(), request())
	if err != nil {
		t.Fatal(err)
	}
	process, err := live.(*Sandbox).StartProcess(context.Background(), core.Command{Path: "secret-command", Env: map[string]string{"KEY": "secret-env"}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := process.Wait(context.Background(), nil, nil)
	if err != nil || result.ExitCode != 7 {
		t.Fatalf("execution contract changed: %+v %v", result, err)
	}
	source := filepath.Join(t.TempDir(), "input")
	if err := os.WriteFile(source, []byte{0, 255}, 0600); err != nil {
		t.Fatal(err)
	}
	if err := live.Upload(context.Background(), source, "/secret-path"); err != nil {
		t.Fatal(err)
	}
	if err := live.Download(context.Background(), "/secret-path", filepath.Join(m.options.OutputDir, "copy")); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	report, err := m.FinishObservation()
	if err != nil || report.Status != core.StatusSucceeded {
		t.Fatalf("observation: %+v %v", report, err)
	}
	records, index := readOperationRecords(t, m)
	if index.Status != "complete" || index.Started != index.Ended || int(index.Records) != len(records) {
		t.Fatalf("index: %+v", index)
	}
	if index.Clock == nil || index.Clock.ClockDomain != m.options.Clock.ClockDomain || index.Clock.EmitterID != m.options.Clock.EmitterID || !index.Clock.StartedAtUTC.Equal(m.options.Clock.StartedAtUTC) {
		t.Fatal("shared controller clock was lost across Background contexts")
	}
	content, err := os.ReadFile(filepath.Join(m.options.OutputDir, "task", "e2b-operations.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	counts := index.FileIO
	if counts == nil || counts.Scope != "e2b_operation_jsonl_only" || counts.WritesStarted != uint64(len(records)) || counts.WritesReturned != counts.WritesStarted ||
		counts.RequestedBytes != uint64(len(content)) || counts.ReturnedBytes != counts.RequestedBytes || counts.WriteErrors != 0 || counts.ShortWrites != 0 ||
		counts.InvalidReturns != 0 || counts.SyncsStarted != 1 || counts.SyncsReturned != 1 || counts.SyncErrors != 0 {
		t.Fatalf("operation file I/O counts: %+v", counts)
	}
	if _, err := m.FinishObservation(); err != nil {
		t.Fatal(err)
	}
	_, repeatedIndex := readOperationRecords(t, m)
	if repeatedIndex.FileIO == nil || *repeatedIndex.FileIO != *counts {
		t.Fatalf("repeated finish changed file I/O counts: %+v", repeatedIndex.FileIO)
	}
	observation := live.(*Sandbox).observation
	observation.mu.Lock()
	repeatedCounts := observation.fileIO
	observation.mu.Unlock()
	if repeatedCounts != *counts {
		t.Fatalf("repeated finish performed additional I/O: %+v", repeatedCounts)
	}
	starts := map[uint64]operationRecord{}
	seenExec, seenCreate, files := false, false, 0
	for i, record := range records {
		if record.Sequence != uint64(i+1) || record.TaskID != "task" || record.LogicalTaskID != "logical-task" || record.Purpose != "task" || record.SandboxHandle != live.(*Sandbox).ID() {
			t.Fatalf("identity/sequence: %+v", record)
		}
		if record.Event == "start" {
			starts[record.OperationID] = record
			continue
		}
		start := starts[record.OperationID]
		if record.ClientDurationNS == nil || *record.ClientDurationNS < 0 || !record.StartedAtUTC.Equal(start.StartedAtUTC) {
			t.Fatalf("bad duration: %+v", record)
		}
		if start.ObservedOffsetNS == nil || record.ObservedOffsetNS == nil || *start.ObservedOffsetNS < 0 || *record.ObservedOffsetNS-*start.ObservedOffsetNS != *record.ClientDurationNS {
			t.Fatalf("controller monotonic boundaries changed duration: %+v", record)
		}
		if record.Operation == "exec.wait" {
			seenExec = true
			if record.Status != "failed" || record.ExitCode == nil || *record.ExitCode != 7 || record.GuestDurationNS == nil || *record.GuestDurationNS != 99 || *record.StdoutBytes != 2 || *record.StderrBytes != 1 {
				t.Fatalf("execution evidence: %+v", record)
			}
			if record.GuestBoundaries == nil || *record.GuestBoundaries.PopenReturnNS != 3 || *record.GuestBoundaries.FirstOutputNS != 4 || *record.GuestBoundaries.ChildExitObservedNS != 5 || *record.GuestBoundaries.PipeEOFNS != 6 {
				t.Fatalf("guest boundary evidence: %+v", record.GuestBoundaries)
			}
			if *record.ClientDurationNS < result.Duration.Nanoseconds() {
				t.Fatal("client duration replaced with guest duration")
			}
		}
		if record.Operation == "adapter_rpc.create" {
			seenCreate = true
			if record.Creation == nil || record.Creation.SDKVersion != "2.58.0" || record.Creation.ColdWarm != "unknown" || record.Creation.Region != nil || record.Creation.Resources != nil || len(record.SDKCalls) != 1 {
				t.Fatalf("create metadata: %+v", record)
			}
		}
		if record.FileBytes != nil {
			files++
			if *record.FileBytes != 2 {
				t.Fatal("wrong transfer byte count")
			}
		}
	}
	if !seenExec || !seenCreate || files != 2 {
		t.Fatal("missing required evidence")
	}
	for _, path := range report.LogPaths {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("non-private artifact %s", path)
		}
	}
}

func TestObservationFailureDoesNotChangeCleanupAndCannotBeComplete(t *testing.T) {
	for _, unfinished := range []bool{false, true} {
		t.Run(map[bool]string{false: "write_failure", true: "unfinished_operation"}[unfinished], func(t *testing.T) {
			kills := 0
			m := testManager(t, func(action string, body map[string]any) (int, any) {
				if action == "create" {
					return 200, createResponse(body)
				}
				kills++
				return 200, map[string]bool{"absence_confirmed": true}
			})
			live, err := m.Start(context.Background(), request())
			if err != nil {
				t.Fatal(err)
			}
			sandbox := live.(*Sandbox)
			if unfinished {
				sandbox.observation.begin(sandbox, "interrupted")
			} else if err := sandbox.observation.file.Close(); err != nil {
				t.Fatal(err)
			}
			if err := m.Close(); err != nil || kills != 1 {
				t.Fatalf("observer changed cleanup: %v %d", err, kills)
			}
			report, err := m.FinishObservation()
			if err == nil || report.Status != core.StatusFailed {
				t.Fatal("incomplete evidence reported complete")
			}
			_, index := readOperationRecords(t, m)
			if index.Status != "incomplete" || len(index.Problems) == 0 {
				t.Fatalf("index lost failure: %+v", index)
			}
			counts := index.FileIO
			if counts == nil || counts.WritesStarted != index.Records || counts.WritesReturned != counts.WritesStarted || counts.SyncsStarted != 1 || counts.SyncsReturned != 1 {
				t.Fatalf("failed operation file I/O counts: %+v", counts)
			}
			if unfinished {
				if counts.WriteErrors != 0 || counts.ShortWrites != 0 || counts.RequestedBytes != counts.ReturnedBytes || counts.SyncErrors != 0 {
					t.Fatalf("unfinished operation fabricated I/O failures: %+v", counts)
				}
			} else if counts.WriteErrors != 2 || counts.ShortWrites != 2 || counts.ReturnedBytes >= counts.RequestedBytes || counts.SyncErrors != 1 {
				t.Fatalf("closed-file errors not counted at actual I/O boundary: %+v", counts)
			}
		})
	}
}

func TestExistingEvidenceNotOverwrittenAndLateRPCInvalidatesIndex(t *testing.T) {
	m := testManager(t, func(action string, body map[string]any) (int, any) {
		if action == "create" {
			return 200, createResponse(body)
		}
		return 200, map[string]any{"absence_confirmed": true, "metrics": []any{}}
	})
	live, err := m.Start(context.Background(), request())
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.FinishObservation(); err != nil {
		t.Fatal(err)
	}
	if _, err := live.(*Sandbox).Metrics(context.Background()); err != nil {
		t.Fatal("observation changed RPC semantics")
	}
	if _, err := os.Stat(filepath.Join(m.options.OutputDir, "task", "e2b-operations-index.json.incomplete")); err != nil {
		t.Fatal("late operation retained a complete index")
	}
	if report, err := m.FinishObservation(); err == nil || report.Status != core.StatusFailed {
		t.Fatal("late observation error hidden")
	}
	prior, err := os.ReadFile(filepath.Join(m.options.OutputDir, "task", "e2b-operations-index.json"))
	if err != nil {
		t.Fatal(err)
	}
	duplicate := newOperationLog(m.options.OutputDir, "task", nil)
	if len(duplicate.problems) == 0 {
		t.Fatal("reused existing evidence")
	}
	after, err := os.ReadFile(filepath.Join(m.options.OutputDir, "task", "e2b-operations-index.json"))
	if err != nil || !bytes.Equal(prior, after) {
		t.Fatal("overwrote prior evidence")
	}
}

func TestInvalidSDKMeasurementFailsObservationWithoutChangingRPC(t *testing.T) {
	m := testManager(t, func(action string, body map[string]any) (int, any) {
		if action == "create" {
			return 200, createResponse(body)
		}
		return 200, map[string]any{"absence_confirmed": true, "sdk_calls": []SDKCall{{Operation: "secret-command", DurationNS: -1, Status: "succeeded"}}}
	})
	if _, err := m.Start(context.Background(), request()); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("measurement changed successful cleanup: %v", err)
	}
	if report, err := m.FinishObservation(); err == nil || report.Status != core.StatusFailed {
		t.Fatal("invalid SDK timing silently became complete")
	}
	_, index := readOperationRecords(t, m)
	if index.Status != "incomplete" {
		t.Fatal("invalid SDK timing published as complete")
	}
}

package e2b

import (
	"context"
	"sync"
	"testing"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/performance"
)

func TestProcessEvidenceKeepsExecutionBindingAcrossContextsAndRetries(t *testing.T) {
	var mu sync.Mutex
	var starts []string
	m := testManager(t, func(action string, body map[string]any) (int, any) {
		if _, ok := body["execution_id"]; ok {
			t.Error("observation identity leaked into adapter command protocol")
		}
		switch action {
		case "create":
			return 200, createResponse(body)
		case "start":
			mu.Lock()
			starts = append(starts, body["process"].(string))
			mu.Unlock()
			return 200, map[string]any{"process": body["process"], "pid": 41}
		case "stdin":
			return 200, map[string]bool{"accepted": true}
		case "process", "kill_process":
			return 200, ProcessStatus{PID: 41, Done: true, ManagedProcessAbsent: true}
		case "events":
			return 200, map[string]any{"cursor": 1, "done": true, "events": []any{map[string]any{"exit_code": 0, "guest_duration_ns": 17}}}
		case "kill":
			return 200, map[string]bool{"absence_confirmed": true}
		default:
			t.Errorf("unexpected action %s", action)
			return 500, nil
		}
	})
	live, err := m.Start(context.Background(), request())
	if err != nil {
		t.Fatal(err)
	}
	sandbox := live.(*Sandbox)
	processes := make([]*Process, 0, 3)
	for _, id := range []string{"execution-one", "execution-two", "execution-retry"} {
		p, err := sandbox.StartProcess(performance.WithExecutionID(context.Background(), id), core.Command{Path: "same-command"})
		if err != nil {
			t.Fatal(err)
		}
		processes = append(processes, p)
	}
	foreign := performance.WithExecutionID(context.Background(), "different-session")
	for i := len(processes) - 1; i >= 1; i-- {
		p := processes[i]
		if err := p.SendStdin(foreign, nil, true); err != nil {
			t.Fatal(err)
		}
		if _, err := p.Wait(foreign, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	canceled, cancel := context.WithCancel(foreign)
	cancel()
	if _, err := processes[0].Wait(canceled, nil, nil); err == nil {
		t.Fatal("canceled wait succeeded")
	}
	if _, err := processes[0].Kill(context.WithoutCancel(canceled)); err != nil {
		t.Fatal(err)
	}
	if _, err := processes[0].Status(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.FinishObservation(); err != nil {
		t.Fatal(err)
	}
	records, index := readOperationRecords(t, m)
	if index.Status != "complete" {
		t.Fatalf("index: %+v", index)
	}
	want := map[string]string{}
	for _, p := range processes {
		if _, ok := want[p.ID]; ok {
			t.Fatal("same command reused process identity")
		}
		want[p.ID] = p.executionID
	}
	paired := map[uint64]operationRecord{}
	waits := map[string]operationRecord{}
	for _, r := range records {
		if r.ProcessID != "" && (want[r.ProcessID] == "" || r.ExecutionID != want[r.ProcessID]) {
			t.Fatalf("execution crossed a process/session boundary: %+v", r)
		}
		if r.ProcessID == "" && r.ExecutionID != "" {
			t.Fatalf("lifecycle inherited tool identity: %+v", r)
		}
		if r.Event == "start" {
			paired[r.OperationID] = r
			continue
		}
		start, ok := paired[r.OperationID]
		if !ok || start.ProcessID != r.ProcessID || start.ExecutionID != r.ExecutionID {
			t.Fatalf("start/end binding changed: %+v", r)
		}
		if r.Operation == "exec.wait" {
			waits[r.ProcessID] = r
		}
	}
	if len(starts) != 3 || len(waits) != 3 {
		t.Fatalf("starts=%d waits=%d", len(starts), len(waits))
	}
	for i, p := range processes {
		r := waits[p.ID]
		if i == 0 {
			if r.Status != "canceled" || r.GuestDurationNS != nil || r.ExitCode != nil {
				t.Fatalf("cancellation invented terminal: %+v", r)
			}
		} else if r.GuestDurationNS == nil || *r.GuestDurationNS != 17 || r.ExitCode == nil || *r.ExitCode != 0 {
			t.Fatalf("guest result lost binding: %+v", r)
		}
	}
}

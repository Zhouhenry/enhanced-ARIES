package ssh

import (
	"context"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/performance"
)

type identityExecutor struct {
	contractSandbox
	execute func(context.Context, core.Command) (core.CommandResult, error)
}

func (e *identityExecutor) ExecStream(ctx context.Context, command core.Command, _ io.Reader, _, _ io.Writer) (core.CommandResult, error) {
	return e.execute(ctx, command)
}

func TestAcceptedExecutionIdentitySurvivesReverseCompletionAndCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	file, content := memoryAuditFile()
	writer := newAuditWriter(file, nil)
	type entry struct {
		id      string
		command core.Command
		release chan struct{}
	}
	entered := make(chan entry, 4)
	target := &identityExecutor{execute: func(ctx context.Context, command core.Command) (core.CommandResult, error) {
		e := entry{performance.ExecutionID(ctx), command, make(chan struct{})}
		entered <- e
		select {
		case <-e.release:
			return core.CommandResult{ExitCode: 0}, nil
		case <-ctx.Done():
			return core.CommandResult{ExitCode: -1}, ctx.Err()
		}
	}}
	first := &bridgeSession{sandbox: target, sandboxID: "same-sandbox", audit: writer}
	second := &bridgeSession{sandbox: target, sandboxID: "same-sandbox", audit: writer}
	command := core.Command{Path: "/bin/sh", Args: []string{"-c", "same command"}, Dir: "/workspace", Env: map[string]string{"KEY": "unchanged"}}
	prepared := Prepared{Action: Execute, Command: command, HashInput: "same command"}
	var entries []entry
	var done []chan int
	for i := range 3 {
		session := first
		if i == 2 {
			session = second
		}
		finished := make(chan int, 1)
		done = append(done, finished)
		go func() {
			finished <- session.execute(performance.WithExecutionID(ctx, "foreign-parent"), &stubSSHChannel{}, prepared, requestAudit{})
		}()
		select {
		case e := <-entered:
			entries = append(entries, e)
		case <-ctx.Done():
			t.Fatal("execution did not enter target")
		}
	}
	ids := map[string]bool{}
	for _, e := range entries {
		if e.id == "" || e.id == "foreign-parent" || ids[e.id] || !reflect.DeepEqual(e.command, command) {
			t.Fatalf("execution identity collision or command mutation: %+v", e)
		}
		ids[e.id] = true
	}
	for i := len(entries) - 1; i >= 0; i-- {
		close(entries[i].release)
		select {
		case exit := <-done[i]:
			if exit != 0 {
				t.Fatalf("exit=%d", exit)
			}
		case <-ctx.Done():
			t.Fatal("execution did not finish")
		}
	}
	cancelCtx, stop := context.WithCancel(ctx)
	finished := make(chan int, 1)
	go func() { finished <- first.execute(cancelCtx, &stubSSHChannel{}, prepared, requestAudit{}) }()
	var canceled entry
	select {
	case canceled = <-entered:
	case <-ctx.Done():
		t.Fatal("canceled execution did not enter target")
	}
	stop()
	select {
	case exit := <-finished:
		if exit != 255 {
			t.Fatalf("canceled exit=%d", exit)
		}
	case <-ctx.Done():
		t.Fatal("canceled execution did not return")
	}
	if canceled.id == "" || ids[canceled.id] {
		t.Fatal("canceled execution reused an identity")
	}
	if err := writer.sealAndWait(ctx); err != nil {
		t.Fatal(err)
	}
	records := decodeAuditLines(t, content.Bytes())
	if len(records) != 4 {
		t.Fatalf("records=%d", len(records))
	}
	for i := range 3 {
		if records[i]["execution_id"] != entries[2-i].id {
			t.Fatal("completion order was used as execution identity")
		}
	}
	if records[3]["execution_id"] != canceled.id || records[3]["status"] != "canceled" {
		t.Fatalf("canceled identity lost: %+v", records[3])
	}
}

func TestDrainOnlyDoesNotClaimGuestExecution(t *testing.T) {
	file, content := memoryAuditFile()
	writer := newAuditWriter(file, nil)
	target := &identityExecutor{execute: func(context.Context, core.Command) (core.CommandResult, error) {
		t.Fatal("drain-only reached target")
		return core.CommandResult{}, nil
	}}
	session := &bridgeSession{sandbox: target, audit: writer}
	session.execute(context.Background(), &stubSSHChannel{}, Prepared{Action: DrainOnly}, requestAudit{})
	if err := writer.sealAndWait(context.Background()); err != nil {
		t.Fatal(err)
	}
	records := decodeAuditLines(t, content.Bytes())
	if len(records) != 1 || records[0]["execution_id"] != nil {
		t.Fatal("drain-only invented an execution")
	}
}

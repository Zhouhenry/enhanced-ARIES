package ssh

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/bridge/ssh/execidentity"
	sshclient "github.com/hyscale-lab/aries/pkg/bridge/ssh/openclaw/client"
	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/performance"
	gossh "golang.org/x/crypto/ssh"
)

func wireIdentity(invocation, request string) execidentity.Metadata {
	return execidentity.Metadata{SchemaVersion: 1, ScopeID: "scope", AgentSessionID: "session", ToolCallID: "same-tool", NativeInvocationID: invocation, SSHRequestID: request}
}

func identityRequest(m execidentity.Metadata) *gossh.Request {
	payload, _ := json.Marshal(m)
	return &gossh.Request{Type: execidentity.RequestType, WantReply: true, Payload: payload}
}

func execRequest() *gossh.Request {
	return &gossh.Request{Type: "exec", WantReply: true, Payload: gossh.Marshal(struct{ Command string }{encodeCanonicalTokens([]string{"/bin/sh", "-c", "same command"})})}
}

func handleIdentityRequests(s *bridgeSession, ctx context.Context, requests ...*gossh.Request) {
	queue := make(chan *gossh.Request, len(requests))
	for _, r := range requests {
		queue <- r
	}
	close(queue)
	s.handleSession(ctx, &stubSSHChannel{}, queue)
}

func TestNativeIdentitySessionProtocolRejectsAmbiguityBeforeExec(t *testing.T) {
	valid := wireIdentity("native", "ssh")
	for _, test := range []struct {
		name     string
		enabled  bool
		requests []*gossh.Request
		executed bool
		linked   bool
	}{
		{"accepted", true, []*gossh.Request{identityRequest(valid), execRequest()}, true, true},
		{"legacy", true, []*gossh.Request{execRequest()}, true, false},
		{"disabled", false, []*gossh.Request{identityRequest(valid), execRequest()}, false, false},
		{"duplicate", true, []*gossh.Request{identityRequest(valid), identityRequest(valid), execRequest()}, false, false},
		{"conflict", true, []*gossh.Request{identityRequest(valid), identityRequest(wireIdentity("other", "other")), execRequest()}, false, false},
		{"no-reply", true, []*gossh.Request{{Type: execidentity.RequestType, Payload: identityRequest(valid).Payload}, execRequest()}, false, false},
		{"unknown-field", true, []*gossh.Request{{Type: execidentity.RequestType, WantReply: true, Payload: bytes.Replace(identityRequest(valid).Payload, []byte(`"schema_version":1`), []byte(`"schema_version":1,"extra":true`), 1)}, execRequest()}, false, false},
		{"wrong-scope", true, []*gossh.Request{identityRequest(execidentity.Metadata{SchemaVersion: 1, ScopeID: "other", AgentSessionID: "session", ToolCallID: "tool", NativeInvocationID: "n", SSHRequestID: "r"}), execRequest()}, false, false},
		{"wrong-session", true, []*gossh.Request{identityRequest(execidentity.Metadata{SchemaVersion: 1, ScopeID: "scope", AgentSessionID: "other", ToolCallID: "tool", NativeInvocationID: "n", SSHRequestID: "r"}), execRequest()}, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			file, content := memoryAuditFile()
			writer := newAuditWriter(file, nil)
			var calls int
			target := &identityExecutor{execute: func(ctx context.Context, command core.Command) (core.CommandResult, error) {
				calls++
				m, linked := execidentity.FromContext(ctx)
				if linked != test.linked || linked && m != valid {
					t.Errorf("context identity=%+v %v", m, linked)
				}
				if command.Path != "/bin/sh" || !reflect.DeepEqual(command.Args, []string{"-c", "same command"}) {
					t.Error("command mutated")
				}
				if performance.ExecutionID(ctx) == "" || performance.ExecutionID(ctx) == "foreign-parent" {
					t.Error("execution ID absent or inherited")
				}
				return core.CommandResult{}, nil
			}}
			s := &bridgeSession{dialect: testDialect{}, sandbox: target, audit: writer, identities: &identityRegistry{limit: 4}, replyRequest: func(*gossh.Request, bool) error { return nil }}
			if test.enabled {
				s.identityScopeID = "scope"
				s.identitySessionID = "session"
			}
			foreign := execidentity.WithContext(performance.WithExecutionID(context.Background(), "foreign-parent"), wireIdentity("foreign", "foreign-request"))
			handleIdentityRequests(s, foreign, test.requests...)
			if (calls == 1) != test.executed {
				t.Fatalf("target calls=%d", calls)
			}
			if err := writer.sealAndWait(context.Background()); err != nil {
				t.Fatal(err)
			}
			records := decodeAuditLines(t, content.Bytes())
			if len(records) != 1 {
				t.Fatalf("records=%d", len(records))
			}
			if test.executed {
				expected := "unlinked"
				if test.linked {
					expected = "exact_channel"
				}
				if records[0]["identity_status"] != expected {
					t.Fatalf("audit identity=%+v", records[0])
				}
			}
		})
	}
}

func TestIdentityRegistryBoundedAndConcurrentReservations(t *testing.T) {
	r := &identityRegistry{limit: 2}
	var wg sync.WaitGroup
	accepted := make(chan bool, 32)
	for range 32 {
		wg.Add(1)
		go func() { defer wg.Done(); accepted <- r.reserve(wireIdentity("native", "request")) == nil }()
	}
	wg.Wait()
	close(accepted)
	count := 0
	for ok := range accepted {
		if ok {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("accepted duplicate %d times", count)
	}
	if r.reserve(wireIdentity("native", "new-request")) == nil || r.reserve(wireIdentity("new-native", "request")) == nil {
		t.Fatal("reconnect bypassed reservation")
	}
	if err := r.reserve(wireIdentity("retry-native", "retry-request")); err != nil {
		t.Fatal("explicit retry with same tool ID rejected", err)
	}
	if r.reserve(wireIdentity("overflow", "overflow")) == nil {
		t.Fatal("registry exceeded cap")
	}
}

func TestNativeIdentityReverseCompletionCancellationAndReconnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	file, content := memoryAuditFile()
	writer := newAuditWriter(file, nil)
	type call struct {
		identity  execidentity.Metadata
		execution string
		release   chan struct{}
	}
	entered := make(chan call, 4)
	target := &identityExecutor{execute: func(ctx context.Context, command core.Command) (core.CommandResult, error) {
		m, ok := execidentity.FromContext(ctx)
		if !ok {
			t.Error("missing native context")
		}
		c := call{m, performance.ExecutionID(ctx), make(chan struct{})}
		entered <- c
		select {
		case <-c.release:
			return core.CommandResult{}, nil
		case <-ctx.Done():
			return core.CommandResult{}, ctx.Err()
		}
	}}
	s := &bridgeSession{dialect: testDialect{}, sandbox: target, audit: writer, identityScopeID: "scope", identitySessionID: "session", identities: &identityRegistry{limit: 8}, replyRequest: func(*gossh.Request, bool) error { return nil }}
	var calls []call
	var done []chan struct{}
	for _, m := range []execidentity.Metadata{wireIdentity("n1", "r1"), wireIdentity("n2", "r2")} {
		d := make(chan struct{})
		done = append(done, d)
		go func() { defer close(d); handleIdentityRequests(s, ctx, identityRequest(m), execRequest()) }()
		select {
		case c := <-entered:
			calls = append(calls, c)
		case <-ctx.Done():
			t.Fatal("missing native execution")
		}
	}
	for i := 1; i >= 0; i-- {
		close(calls[i].release)
		select {
		case <-done[i]:
		case <-ctx.Done():
			t.Fatal("execution hung")
		}
	}
	// The same reservations apply to new channels and connections within the scope.
	handleIdentityRequests(s, ctx, identityRequest(wireIdentity("n1", "reconnected")), execRequest())
	cancelCtx, stop := context.WithCancel(ctx)
	d := make(chan struct{})
	go func() {
		defer close(d)
		handleIdentityRequests(s, cancelCtx, identityRequest(wireIdentity("n3", "r3")), execRequest())
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("missing cancel execution")
	}
	stop()
	select {
	case <-d:
	case <-ctx.Done():
		t.Fatal("cancel hung")
	}
	if err := writer.sealAndWait(ctx); err != nil {
		t.Fatal(err)
	}
	records := decodeAuditLines(t, content.Bytes())
	if len(records) != 4 {
		t.Fatalf("records=%d", len(records))
	}
	for i := range 2 {
		if records[i]["execution_id"] != calls[1-i].execution {
			t.Fatal("execution followed completion order")
		}
		m := records[i]["native_identity"].(map[string]any)
		if m["native_invocation_id"] != calls[1-i].identity.NativeInvocationID {
			t.Fatal("native identity changed")
		}
	}
	if records[2]["status"] != "rejected" || records[3]["status"] != "canceled" {
		t.Fatalf("negative records=%+v", records)
	}
}

// Keep the executor's stream signature checked; cancellation does not claim guest termination.
var _ interface {
	ExecStream(context.Context, core.Command, io.Reader, io.Writer, io.Writer) (core.CommandResult, error)
} = (*identityExecutor)(nil)

func TestNativeIdentityRealSSHReconnectRetryAndCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	seen := make(chan execidentity.Metadata, 4)
	target := &identityExecutor{execute: func(ctx context.Context, _ core.Command) (core.CommandResult, error) {
		m, ok := execidentity.FromContext(ctx)
		if !ok {
			t.Error("identity absent on real SSH")
		}
		seen <- m
		if m.NativeInvocationID == "cancel" {
			<-ctx.Done()
			return core.CommandResult{}, ctx.Err()
		}
		return core.CommandResult{}, nil
	}}
	manager, err := New(Options{Dialect: testDialect{}, HostSigner: fixtureHostSigner(t), SandboxID: "s", ResolveListen: loopbackListen, OutputDir: t.TempDir(), IdentityScopeID: "scope", IdentitySessionID: "session"})
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := manager.StartTarget(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		stopCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := manager.Stop(stopCtx); err != nil {
			t.Error(err)
		}
	}()
	run := func(ctx context.Context, m execidentity.Metadata) (int, error) {
		return sshclient.Run(ctx, sshclient.Config{Address: endpoint.Address, User: endpoint.Username, Identity: &m}, encodeCanonicalTokens([]string{"/bin/sh", "-c", "same command"}), io.NopCloser(bytes.NewReader(nil)), io.Discard, io.Discard)
	}
	if code, err := run(ctx, wireIdentity("first", "request1")); err != nil || code != 0 {
		t.Fatalf("first %d %v", code, err)
	}
	<-seen
	if code, err := run(ctx, wireIdentity("first", "request2")); err == nil || code != 255 {
		t.Fatal("reconnect reused invocation")
	}
	if code, err := run(ctx, wireIdentity("retry", "request3")); err != nil || code != 0 {
		t.Fatalf("retry %d %v", code, err)
	}
	if got := <-seen; got.NativeInvocationID != "retry" || got.ToolCallID != "same-tool" {
		t.Fatal("retry identity lost")
	}
	cancelCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { _, err := run(cancelCtx, wireIdentity("cancel", "request4")); done <- err }()
	select {
	case <-seen:
	case <-ctx.Done():
		t.Fatal("cancel call absent")
	}
	stop()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancel succeeded")
		}
	case <-ctx.Done():
		t.Fatal("cancel hung")
	}
}

package jiuwenswarm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/harness"
	"github.com/hyscale-lab/aries/pkg/harness/internal/sandboxprocess"
	"github.com/hyscale-lab/aries/pkg/runner"
)

type testSandbox struct {
	mu                                                                     sync.Mutex
	prepareFails, stageFails, stopFails                                    bool
	stopCalls                                                              int
	staged                                                                 []byte
	launched                                                               chan struct{}
	terminated                                                             chan struct{}
	nativeRequest                                                          string
	nativeData                                                             []byte
	producerStatus, producerLedger                                         []byte
	producerReadFails                                                      bool
	processReadFails                                                       bool
	processUnsupported, processCapabilityMissing, processRegistrationFails bool
	nativeReadFails, gracefulFails                                         bool
	inventoryCalls, downloadCalls                                          int
	resourceCalls                                                          int
	resourceFails                                                          bool
	cleanupFails                                                           bool
}

func (*testSandbox) Connectivity() core.HarnessConnectivity         { return core.HarnessConnectivity{} }
func (*testSandbox) Upload(context.Context, string, string) error   { return nil }
func (*testSandbox) Download(context.Context, string, string) error { return nil }
func (s *testSandbox) Exec(_ context.Context, cmd core.Command) (core.CommandResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cleanupFails && strings.Contains(cmd.Args[2], "shutil.rmtree(p)") {
		return core.CommandResult{ExitCode: 1}, nil
	}
	if cmd.Args[2] == sandboxprocess.ResourceScript {
		s.resourceCalls++
		if s.resourceFails {
			return core.CommandResult{ExitCode: 1}, nil
		}
		raw, _ := json.Marshal(map[string]any{"cgroup_path": cmd.Args[3], "cgroup_inode": "123", "files": map[string]any{"cpu.stat": map[string]string{"status": "collected", "raw": "usage_usec 1\nuser_usec 1\nsystem_usec 0\n"}}})
		return core.CommandResult{Stdout: string(raw)}, nil
	}
	if cmd.Args[2] == preparedScript {
		if s.processRegistrationFails {
			return core.CommandResult{ExitCode: 1}, nil
		}
		var spec map[string]any
		_ = json.Unmarshal(s.staged, &spec)
		proof := map[string]any{"revision": NativeRevision, "uv_lock_sha256": LockSHA256, "project_id": "project", "workdir": "/workspace", "mode": "flash", "pid": 42, "process_start_ticks": "100"}
		if !s.processCapabilityMissing {
			proof["native_process_observation_supported"] = spec["native_process_observation"] == true && !s.processUnsupported
		}
		raw, _ := json.Marshal(proof)
		return core.CommandResult{Stdout: string(raw)}, nil
	}
	if cmd.Args[2] == nativeTerminateScript {
		if s.gracefulFails {
			return core.CommandResult{ExitCode: 1}, nil
		}
		close(s.terminated)
		return core.CommandResult{Stdout: "SIGTERM_SENT"}, nil
	}
	if cmd.Args[2] == nativeSpanInventoryScript {
		s.inventoryCalls++
		if s.stopCalls == 0 || s.nativeReadFails {
			return core.CommandResult{ExitCode: 1}, nil
		}
		if s.nativeData == nil {
			s.nativeData = nativeTestLine("0123456789abcdef", "", "s", s.nativeRequest, "")
		}
		raw, _ := json.Marshal(map[string]any{"files": []nativeSpanFile{{Name: "traces-2026-10-27.jsonl", Size: int64(len(s.nativeData))}}})
		return core.CommandResult{Stdout: string(raw)}, nil
	}
	if cmd.Args[2] == nativeProducerInventoryScript {
		if s.stopCalls == 0 || s.producerReadFails {
			return core.CommandResult{ExitCode: 1}, nil
		}
		if s.producerStatus == nil {
			index := newNativeSpanIndex("s", s.nativeRequest)
			index.read("traces-2026-10-27.jsonl", s.nativeData)
			status, ledger := nativeProducerFixture(index.Spans)
			s.producerStatus, _ = json.Marshal(status)
			s.producerLedger = ledger
		}
		raw, _ := json.Marshal([]nativeSpanFile{{Name: "status.json", Size: int64(len(s.producerStatus))}, {Name: "events.jsonl", Size: int64(len(s.producerLedger))}})
		return core.CommandResult{Stdout: string(raw)}, nil
	}
	if len(cmd.Args) > 3 {
		switch cmd.Args[3] {
		case "prepare":
			if s.prepareFails {
				return core.CommandResult{ExitCode: 1}, nil
			}
		case "stage":
			s.staged = append([]byte(nil), cmd.Stdin...)
			s.nativeRequest = "aries-" + strings.TrimPrefix(cmd.Args[4], "/run/aries-jiuwen-") + "-run"
			if s.stageFails {
				return core.CommandResult{ExitCode: 1}, nil
			}
		case "stop":
			s.stopCalls++
			if s.stopFails {
				return core.CommandResult{Stdout: "not empty"}, nil
			}
			return core.CommandResult{Stdout: "ARIES_GROUP_EMPTY"}, nil
		}
	}
	return core.CommandResult{}, nil
}
func (s *testSandbox) ExecStream(ctx context.Context, _ core.Command, _ io.Reader, out, errout io.Writer) (core.CommandResult, error) {
	close(s.launched)
	_, _ = out.Write([]byte("native started\n"))
	select {
	case <-ctx.Done():
		return core.CommandResult{ExitCode: 130}, ctx.Err()
	case <-s.terminated:
		return core.CommandResult{}, nil
	}
}

func (s *testSandbox) DownloadLimit(_ context.Context, source, destination string, limit int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.downloadCalls++
	content := s.nativeData
	if strings.Contains(source, "/aries-process/") {
		if s.processReadFails {
			return errors.New("fixture process evidence unavailable")
		}
		content = []byte("{}\n")
	} else if strings.HasSuffix(source, "/aries-native-audit/status.json") {
		content = s.producerStatus
	} else if strings.HasSuffix(source, "/aries-native-audit/events.jsonl") {
		content = s.producerLedger
	}
	if s.stopCalls != 1 || int64(len(content)) > limit {
		return errors.New("download before containment or over limit")
	}
	return os.WriteFile(destination, content, 0o600)
}

type testNative struct {
	run          func(context.Context, func(Frame)) error
	wrongProject bool
}

func (*testNative) Health(context.Context, string) error { return nil }
func (n *testNative) CreateSession(context.Context, string, string) (json.RawMessage, error) {
	if n.wrongProject {
		return json.RawMessage(`{"session_id":"s","projectDir":"/other"}`), nil
	}
	return json.RawMessage(`{"session_id":"s","projectDir":"/workspace"}`), nil
}
func (n *testNative) Run(ctx context.Context, _, _, _ string, consume func(Frame)) error {
	if n.run != nil {
		return n.run(ctx, consume)
	}
	raw := json.RawMessage(`{"request_id":"r","is_final":true,"body":{"result":{}}}`)
	consume(Frame{IsFinal: true, Body: json.RawMessage(`{"result":{}}`), Raw: raw, ReceivedAt: time.Now()})
	return nil
}
func (*testNative) Close() {}

func testEmbedded(t *testing.T, s *testSandbox, n *testNative) *Embedded {
	t.Helper()
	s.launched = make(chan struct{})
	s.terminated = make(chan struct{})
	e, err := NewEmbedded(EmbeddedOptions{OutputDir: t.TempDir(), Workdir: "/workspace", SourceDir: "/opt/jiuwenswarm", PythonPath: "/opt/jiuwenswarm/.venv/bin/python", ModelProxyURL: "https://model.test/v1", ProxyAccessToken: []byte("private-token"),
		GuestEndpoint: func(context.Context, runner.Sandbox, int) (string, http.Header, error) {
			return "https://guest.test", http.Header{"X-Access": []string{"guest-secret"}}, nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	e.newClient = func(string, http.Header) (nativeClient, error) { return n, nil }
	t.Cleanup(func() {
		s.mu.Lock()
		s.stopFails = false
		s.mu.Unlock()
		if err := e.Stop(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return e
}
func startEmbedded(t *testing.T, e *Embedded, s *testSandbox) error {
	t.Helper()
	endpoint, err := e.Bridge().Start(context.Background(), s)
	if err != nil {
		return err
	}
	return e.Start(context.Background(), core.HarnessRequest{RunID: "run", TaskID: "task", Endpoint: endpoint, Model: core.ModelConfig{Model: "model"}})
}

func TestEmbeddedRejectsMissingProcessCapabilityOrFailedRegistration(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "registration_failed", true: "capability_missing"}[missing], func(t *testing.T) {
			s := &testSandbox{processCapabilityMissing: missing, processRegistrationFails: !missing}
			e := testEmbedded(t, s, &testNative{})
			if err := startEmbedded(t, e, s); err == nil || e.ready {
				t.Fatal("unsupported or failed observation registration was silently enabled")
			}
		})
	}
}

func TestEmbeddedNativeBindingAndRealRuntimeBoundaries(t *testing.T) {
	s := &testSandbox{}
	e := testEmbedded(t, s, &testNative{})
	if err := startEmbedded(t, e, s); err != nil {
		t.Fatal(err)
	}
	result, err := e.Run(context.Background(), "repair")
	if err != nil || result.Status != core.StatusSucceeded || result.SessionID != "s" || result.RuntimeExecutionDuration == nil || *result.RuntimeExecutionDuration < 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if _, err := e.Run(context.Background(), "again"); err == nil {
		t.Fatal("second instruction admitted")
	}
	if err := e.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := e.Bridge().Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.stopCalls != 1 || !e.isolated {
		t.Fatal("containment was not confirmed once")
	}
	var staged map[string]any
	if json.Unmarshal(s.staged, &staged) != nil || staged["model"] != "model" || staged["proxy_token"] != "private-token" {
		t.Fatal("private native model staging changed")
	}
	observation, err := e.NativeToolObservation()
	if err != nil || observation.Status != core.StatusUnsupported || observation.SampleCount != 1 {
		t.Fatalf("scoped native evidence must not imply complete execution coverage: %+v %v", observation, err)
	}
	for _, p := range observation.LogPaths {
		raw, _ := os.ReadFile(p)
		if strings.Contains(string(raw), "private-token") || strings.Contains(string(raw), "guest-secret") {
			t.Fatal("secret in exported evidence")
		}
	}
}

func TestEmbeddedPartialStartsRemainOwned(t *testing.T) {
	for _, phase := range []string{"prepare", "stage", "session"} {
		t.Run(phase, func(t *testing.T) {
			s := &testSandbox{prepareFails: phase == "prepare", stageFails: phase == "stage"}
			e := testEmbedded(t, s, &testNative{wrongProject: phase == "session"})
			if err := startEmbedded(t, e, s); err == nil {
				t.Fatal("partial startup succeeded")
			}
			if e.group == nil {
				t.Fatal("partial group ownership lost")
			}
			if err := e.Stop(context.Background()); err != nil {
				t.Fatal(err)
			}
			if s.stopCalls != 1 {
				t.Fatal("partial group was not stopped")
			}
		})
	}
}

func TestEmbeddedStopFailureRetainsOwnershipAndJoinsCanceledRun(t *testing.T) {
	entered := make(chan struct{})
	s := &testSandbox{stopFails: true}
	e := testEmbedded(t, s, &testNative{run: func(ctx context.Context, _ func(Frame)) error { close(entered); <-ctx.Done(); return ctx.Err() }})
	if err := startEmbedded(t, e, s); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { _, err := e.Run(context.Background(), "task"); result <- err }()
	<-entered
	if err := e.Stop(context.Background()); err == nil || e.isolated {
		t.Fatal("missing positive containment accepted")
	}
	if _, err := e.Run(context.Background(), "new"); err == nil {
		t.Fatal("stop failure reopened admission")
	}
	s.mu.Lock()
	s.stopFails = false
	s.mu.Unlock()
	if err := e.Bridge().Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("run=%v", err)
	}
	select {
	case <-e.processDone:
	default:
		t.Fatal("owned process stream not joined")
	}
}

func TestEmbeddedEvidenceFailureDoesNotChangeAgentOrIsolation(t *testing.T) {
	s := &testSandbox{}
	e := testEmbedded(t, s, &testNative{})
	if err := startEmbedded(t, e, s); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(e.artifactDir, "native-events.jsonl"), 0o700); err != nil {
		t.Fatal(err)
	}
	result, err := e.Run(context.Background(), "task")
	if err != nil || result.Status != core.StatusSucceeded {
		t.Fatalf("observation changed native outcome: %v", err)
	}
	if err := e.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	obs, err := e.NativeToolObservation()
	if err == nil || obs.Status != core.StatusFailed {
		t.Fatal("observation write failure lost")
	}
}

func TestEvidenceIgnoresModelVolumeAndPreservesToolResultBytes(t *testing.T) {
	credentials := harness.NewCredentials("test")
	credentials.Set("token", []byte("private-token"))
	e := newFrameEvidence(t.TempDir(), credentials, true)
	for i := 0; i < 20000; i++ {
		e.consume(Frame{Body: json.RawMessage(`{"event_type":"context.usage"}`)})
	}
	raw := json.RawMessage(`{"body":{"event_type":"chat.tool_result","delta":{"tool_call_id":"c1","result":"out\nerr\n"}}}`)
	e.consume(Frame{Raw: raw, Body: json.RawMessage(`{"event_type":"chat.tool_result"}`), ReceivedAt: time.Now()})
	obs, err := e.finish(true)
	if err != nil || e.retained != 1 || obs.SampleCount != 1 {
		t.Fatalf("unexpected retention: %+v %v", obs, err)
	}
	bytes, err := os.ReadFile(obs.LogPaths[0])
	if err != nil || !strings.Contains(string(bytes), `"result":"out\nerr\n"`) {
		t.Fatal("tool result bytes changed")
	}
	tiny := newFrameEvidence(t.TempDir(), credentials, true)
	tiny.events.max = 1
	tiny.consume(Frame{Raw: raw, Body: json.RawMessage(`{"event_type":"chat.tool_result"}`)})
	report, err := tiny.finish(true)
	if err == nil || report.Status != core.StatusFailed || report.SampleCount != 0 || tiny.selected != 1 {
		t.Fatal("retention overflow became successful observation")
	}
}

func TestObservationDisabledDoesNotRetainToolBodies(t *testing.T) {
	e := newFrameEvidence(t.TempDir(), harness.NewCredentials("test"), false)
	e.consume(Frame{Body: json.RawMessage(`{"event_type":"chat.tool_result"}`)})
	report, err := e.finish(true)
	if err != nil || report.Status != core.StatusNotEnabled || len(e.events.Bytes()) != 0 {
		t.Fatal("disabled observation retained tool evidence")
	}
}

func TestEvidencePreservesNativeFinalAnswerShapes(t *testing.T) {
	cases := []struct {
		name   string
		bodies []string
		want   string
	}{
		{"stream_then_marker", []string{`{"event_type":"chat.delta","delta_kind":"text","delta":"answer"}`, `{"event_type":"chat.final","delta":{"event_type":"chat.final","content":""}}`, `{"result":{}}`}, "answer"},
		{"final_without_delta", []string{`{"event_type":"chat.final","delta":{"event_type":"chat.final","content":"answer"}}`, `{"result":{}}`}, "answer"},
		{"terminal_payload", []string{`{"event_type":"chat.delta","delta_kind":"text","delta":"partial"}`, `{"result":{"event_type":"chat.final","content":"complete answer"}}`}, "complete answer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newFrameEvidence(t.TempDir(), harness.NewCredentials("test"), false)
			for i, body := range tc.bodies {
				e.consume(Frame{Body: json.RawMessage(body), IsFinal: i == len(tc.bodies)-1})
			}
			if string(e.text.Bytes()) != tc.want {
				t.Fatalf("answer=%q", e.text.Bytes())
			}
		})
	}
}

func TestEvidenceRetainsPauseAndReportsDisabledAnswerTruncation(t *testing.T) {
	e := newFrameEvidence(t.TempDir(), harness.NewCredentials("test"), true)
	raw := json.RawMessage(`{"is_final":false,"body":{"event_type":"chat.invocation_paused","awaiting_user_input":true}}`)
	e.consume(Frame{Raw: raw, Body: json.RawMessage(`{"event_type":"chat.invocation_paused","awaiting_user_input":true}`)})
	report, err := e.finish(false)
	if err != nil || report.SampleCount != 1 {
		t.Fatal("pause evidence missing")
	}
	disabled := newFrameEvidence(t.TempDir(), harness.NewCredentials("test"), false)
	disabled.text.max = 1
	disabled.consume(Frame{Body: json.RawMessage(`{"event_type":"chat.delta","delta_kind":"text","delta":"long answer"}`)})
	report, err = disabled.finish(true)
	if err == nil || report.Status != core.StatusFailed || len(report.LogPaths) != 1 {
		t.Fatal("answer truncation hidden by observation level")
	}
	raw, err = os.ReadFile(report.LogPaths[0])
	if err != nil || !strings.Contains(string(raw), `"final_response_truncated": true`) {
		t.Fatal("missing final-answer truncation evidence")
	}
}

func TestEvidenceRetainsFilePushWithoutChangingChatAnswer(t *testing.T) {
	e := newFrameEvidence(t.TempDir(), harness.NewCredentials("test"), true)
	e.consume(Frame{Source: "chat", Body: json.RawMessage(`{"event_type":"chat.delta","delta_kind":"text","delta":"answer"}`)})
	body := json.RawMessage(`{"event_type":"chat.file","files":[{"path":"/workspace/result.pptx","name":"result.pptx"}]}`)
	raw := json.RawMessage(`{"session_id":"s","request_id":"push-r","body":` + string(body) + `}`)
	e.consume(Frame{Source: "push", Body: body, Raw: raw})
	report, err := e.finish(true)
	if err != nil || report.SampleCount != 1 || string(e.text.Bytes()) != "answer" {
		t.Fatal("native file push changed chat outcome or was not retained", err)
	}
	var record struct {
		Source string          `json:"source_channel"`
		Frame  json.RawMessage `json:"native_frame"`
	}
	if json.Unmarshal(bytes.TrimSpace(e.events.Bytes()), &record) != nil || record.Source != "push" || !bytes.Equal(record.Frame, raw) {
		t.Fatal("file push source or native bytes changed")
	}
}

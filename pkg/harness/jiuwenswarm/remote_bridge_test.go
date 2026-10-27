package jiuwenswarm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/performance"
	"github.com/hyscale-lab/aries/pkg/runner"
)

type remoteTestSandbox struct {
	mu                                                 sync.Mutex
	spec                                               map[string]any
	prepareFails, stageFails, stopFails, downloadFails bool
	processUnsupported, processHealthMissing           bool
	stopCalls, cleanupCalls                            int
	terminated                                         chan struct{}
	terminateOnce                                      sync.Once
}

func (*remoteTestSandbox) ID() string                                     { return "runner-owned-sandbox" }
func (*remoteTestSandbox) Connectivity() core.HarnessConnectivity         { return core.HarnessConnectivity{} }
func (*remoteTestSandbox) Upload(context.Context, string, string) error   { return nil }
func (*remoteTestSandbox) Download(context.Context, string, string) error { return nil }
func (s *remoteTestSandbox) DownloadLimit(_ context.Context, _ string, destination string, _ int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.downloadFails {
		return errors.New("fixture download failure")
	}
	if s.stopCalls == 0 {
		return errors.New("download before confirmed Stop")
	}
	return os.WriteFile(destination, []byte("{\"measurement_scope\":\"native_operation_not_process\"}\n"), 0600)
}
func (s *remoteTestSandbox) Exec(_ context.Context, c core.Command) (core.CommandResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(c.Args) < 3 {
		return core.CommandResult{}, errors.New("unexpected command")
	}
	if c.Args[2] == remoteStageScript {
		if err := json.Unmarshal(c.Stdin, &s.spec); err != nil {
			return core.CommandResult{}, err
		}
		if s.stageFails {
			return core.CommandResult{ExitCode: 1}, nil
		}
	} else if c.Args[2] == remoteCleanupScript {
		s.cleanupCalls++
	} else if len(c.Args) > 3 {
		switch c.Args[3] {
		case "prepare":
			if s.prepareFails {
				return core.CommandResult{ExitCode: 1}, nil
			}
		case "stop":
			s.stopCalls++
			if s.stopFails {
				return core.CommandResult{Stdout: "not empty"}, nil
			}
			s.terminateOnce.Do(func() { close(s.terminated) })
			return core.CommandResult{Stdout: "ARIES_GROUP_EMPTY"}, nil
		}
	}
	return core.CommandResult{}, nil
}
func (s *remoteTestSandbox) ExecStream(ctx context.Context, _ core.Command, _ io.Reader, stdout, stderr io.Writer) (core.CommandResult, error) {
	_, _ = stdout.Write([]byte("worker ready\n"))
	select {
	case <-ctx.Done():
		return core.CommandResult{}, ctx.Err()
	case <-s.terminated:
		return core.CommandResult{}, nil
	}
}

func newRemoteTestBridge(t *testing.T, s *remoteTestSandbox, call http.HandlerFunc) *RemoteBridge {
	t.Helper()
	s.terminated = make(chan struct{})
	guest := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		token, _ := s.spec["worker_token"].(string)
		binding := s.spec["binding_id"]
		processRequested, _ := s.spec["process_observation"].(bool)
		processUnsupported, processHealthMissing := s.processUnsupported, s.processHealthMissing
		s.mu.Unlock()
		if r.Header.Get("X-Provider") != "provider-private" || r.Header.Get("Authorization") != "provider-authorization" || r.Header.Get("X-Aries-Worker-Token") != token {
			http.Error(w, "wrong worker authentication", http.StatusForbidden)
			return
		}
		if r.URL.Path == "/health" {
			health := map[string]any{"schema_version": 1, "binding_id": binding}
			if !processHealthMissing {
				status := "not_enabled"
				if processRequested {
					status = "enabled"
				}
				if processRequested && processUnsupported {
					status = "unsupported_native_process_hook"
				}
				health["process_observation_enabled"], health["process_observation_status"] = processRequested && !processUnsupported, status
			}
			_ = json.NewEncoder(w).Encode(health)
			return
		}
		call(w, r)
	}))
	t.Cleanup(guest.Close)
	b, err := NewRemoteBridge(RemoteBridgeOptions{RunID: "run", TaskID: "task", OutputDir: t.TempDir(), Workdir: "/workspace", SourceDir: "/opt/jiuwen", PythonPath: "/opt/jiuwen/.venv/bin/python",
		GuestEndpoint: func(_ context.Context, got runner.Sandbox, port int) (string, http.Header, error) {
			if got != s || port != remoteWorkerPort {
				t.Error("resolver did not receive exact sandbox and worker port")
			}
			return guest.URL, http.Header{"X-Provider": []string{"provider-private"}, "Authorization": []string{"provider-authorization"}}, nil
		}, StartTimeout: time.Second, CleanupTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	b.client.Transport = guest.Client().Transport
	t.Cleanup(func() {
		s.mu.Lock()
		s.stopFails = false
		s.mu.Unlock()
		if err := b.Stop(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return b
}

func startRemoteTest(t *testing.T, b *RemoteBridge, s *remoteTestSandbox) RemoteClientConfig {
	t.Helper()
	endpoint, err := b.Start(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	config, err := b.ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	if endpoint.Protocol != "jiuwen-remote-v1" || endpoint.Address != config.BaseURL || endpoint.Workdir != "/workspace" {
		t.Fatalf("unexpected public endpoint: %+v", endpoint)
	}
	encoded, _ := json.Marshal(endpoint)
	if bytes.Contains(encoded, []byte(config.AccessToken)) || bytes.Contains(encoded, []byte("provider-private")) {
		t.Fatal("public endpoint contains a credential")
	}
	return config
}

func remotePayload(config RemoteClientConfig, id string) []byte {
	raw, _ := json.Marshal(map[string]any{"schema_version": 1, "binding_id": config.BindingID, "invocation_id": id, "cwd": "/workspace/subdir",
		"identity":  map[string]string{"run_id": "run", "task_id": "task", "session_id": "native-session", "request_id": "native-request", "tool_call_id": "native-call", "identity_status": "known"},
		"operation": map[string]string{"kind": "shell", "method": "execute_cmd"}, "params": map[string]any{"command": "printf 'original' >&2", "timeout": 7}, "stream": true})
	return raw
}

func remoteTestRequest(config RemoteClientConfig, route string, raw []byte) (int, []byte, error) {
	req, err := http.NewRequest(http.MethodPost, config.BaseURL+route, bytes.NewReader(raw))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+config.AccessToken)
	req.Header.Set("X-Untrusted-Target", "https://other.invalid")
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	return response.StatusCode, body, err
}

func TestRemoteBridgePreservesNativePayloadAndResult(t *testing.T) {
	s := &remoteTestSandbox{}
	var received []byte
	want := []byte("{\"event\":\"result\",\"result\":{\"code\":0,\"data\":{\"stderr\":\"original\"}},\"invocation_id\":\"inv-1\"}\n{\"event\":\"end\",\"invocation_id\":\"inv-1\"}\n")
	b := newRemoteTestBridge(t, s, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Untrusted-Target") != "" {
			t.Error("Agent header crossed provider credential boundary")
		}
		received, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write(want)
	})
	config := startRemoteTest(t, b, s)
	raw := remotePayload(config, "inv-1")
	status, got, err := remoteTestRequest(config, "/call", raw)
	if err != nil || status != 200 || !bytes.Equal(got, want) || !bytes.Equal(received, raw) {
		t.Fatalf("native bytes changed: status=%d err=%v", status, err)
	}
	status, _, err = remoteTestRequest(config, "/call", raw)
	if err != nil || status != http.StatusConflict {
		t.Fatalf("ambiguous duplicate resubmitted: %d %v", status, err)
	}
	if err := b.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := b.ClientConfig(); err == nil {
		t.Fatal("revoked private configuration remained available")
	}
	for _, name := range []string{"remote-bridge-events.jsonl", "remote-worker.log", "remote-bridge-index.json"} {
		data, err := os.ReadFile(filepath.Join(b.options.OutputDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte(config.AccessToken)) || bytes.Contains(data, []byte("provider-private")) || bytes.Contains(data, []byte("printf")) {
			t.Fatal("credential or command body entered bridge evidence")
		}
	}
}

func TestRemoteBridgeParallelStopJoinsCallsAndConfirmsGroup(t *testing.T) {
	s := &remoteTestSandbox{}
	entered := make(chan struct{}, 2)
	b := newRemoteTestBridge(t, s, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		entered <- struct{}{}
		select {
		case <-r.Context().Done():
		case <-s.terminated:
		}
	})
	config := startRemoteTest(t, b, s)
	finished := make(chan struct{}, 2)
	for _, id := range []string{"parallel-a", "parallel-b"} {
		go func(id string) {
			_, _, _ = remoteTestRequest(config, "/call", remotePayload(config, id))
			finished <- struct{}{}
		}(id)
	}
	for range 2 {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatal("native parallel calls serialized")
		}
	}
	if err := b.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Fatal("request not joined")
		}
	}
	b.mu.Lock()
	active := b.active
	isolated := b.isolated
	b.mu.Unlock()
	s.mu.Lock()
	stops, cleanups := s.stopCalls, s.cleanupCalls
	s.mu.Unlock()
	if active != 0 || !isolated || stops != 1 || cleanups != 1 {
		t.Fatalf("incomplete Stop: active=%d isolated=%t stop=%d cleanup=%d", active, isolated, stops, cleanups)
	}
	if _, _, err := remoteTestRequest(config, "/call", remotePayload(config, "late")); err == nil {
		t.Fatal("listener survived revocation")
	}
}

func TestRemoteBridgeRetainsOwnershipAfterUnconfirmedStop(t *testing.T) {
	s := &remoteTestSandbox{}
	b := newRemoteTestBridge(t, s, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("{}")) })
	_ = startRemoteTest(t, b, s)
	s.mu.Lock()
	s.stopFails = true
	s.mu.Unlock()
	if err := b.Stop(context.Background()); err == nil {
		t.Fatal("unconfirmed cgroup was accepted")
	}
	s.mu.Lock()
	cleanups := s.cleanupCalls
	s.stopFails = false
	s.mu.Unlock()
	if cleanups != 0 || b.isolated {
		t.Fatal("failed isolation discarded owned runtime")
	}
	if err := b.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !b.isolated {
		t.Fatal("confirmed retry did not complete cleanup")
	}
}

func TestProcessQuiesceCannotReplaceOrPreventConfirmedGroupStop(t *testing.T) {
	for _, mode := range []string{"complete", "incomplete", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			s := &remoteTestSandbox{}
			b := newRemoteTestBridge(t, s, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/quiesce" {
					http.Error(w, "unexpected call", http.StatusNotFound)
					return
				}
				var request struct {
					Binding string `json:"binding_id"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if mode == "timeout" {
					select {
					case <-r.Context().Done():
					case <-time.After(500 * time.Millisecond):
					}
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"binding_id": request.Binding, "process_observation_complete": mode == "complete"})
			})
			b.options.ObservationLevel = performance.ObservationL1
			startRemoteTest(t, b, s)
			if err := b.Stop(context.Background()); err != nil || !b.isolated {
				t.Fatalf("quiesce prevented actual group isolation: %v", err)
			}
			if s.stopCalls != 1 || s.cleanupCalls != 1 || s.spec["process_observation"] != true {
				t.Fatal("process observation bypassed native Stop or staging")
			}
			if (b.observationErr != nil) != (mode != "complete") {
				t.Fatal("quiesce failure status lost or invented")
			}
			for _, name := range []string{"jiuwen-process-index.json", "jiuwen-process-events.jsonl"} {
				if _, err := os.Stat(filepath.Join(b.options.OutputDir, name)); err != nil {
					t.Fatal("post-isolation process evidence not collected", err)
				}
			}
		})
	}
}

func TestRemoteBridgeUnsupportedProcessHookPreservesNativeCalls(t *testing.T) {
	s := &remoteTestSandbox{processUnsupported: true}
	want := []byte("{\"event\":\"end\",\"invocation_id\":\"old-image-call\"}\n")
	b := newRemoteTestBridge(t, s, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/call" {
			t.Error("unsupported observer received quiesce", r.URL.Path)
		}
		_, _ = w.Write(want)
	})
	b.options.ObservationLevel = performance.ObservationL1
	config := startRemoteTest(t, b, s)
	status, body, err := remoteTestRequest(config, "/call", remotePayload(config, "old-image-call"))
	if err != nil || status != http.StatusOK || !bytes.Equal(body, want) {
		t.Fatal("old image native request changed", status, err)
	}
	if err := b.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	observation, err := b.RemoteObservation()
	if err != nil || !strings.Contains(observation.Reason, "unsupported by installed native image") || !b.isolated || s.stopCalls != 1 {
		t.Fatal("unsupported observer changed isolation or observation status", observation, err)
	}
	for _, name := range []string{"jiuwen-process-index.json", "jiuwen-process-events.jsonl"} {
		if _, err := os.Stat(filepath.Join(b.options.OutputDir, name)); !os.IsNotExist(err) {
			t.Fatal("unsupported process evidence was downloaded", name, err)
		}
	}
	raw, err := os.ReadFile(filepath.Join(b.options.OutputDir, "remote-bridge-index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var index map[string]any
	if json.Unmarshal(raw, &index) != nil || index["process_observation_requested"] != true || index["process_observation_expected"] != false || index["process_observation_status"] != "unsupported_native_process_hook" {
		t.Fatal("unsupported capability was hidden or declared enabled", string(raw))
	}
}

func TestRemoteBridgeMissingProcessCapabilityDoesNotBecomeUnsupported(t *testing.T) {
	s := &remoteTestSandbox{processHealthMissing: true}
	b := newRemoteTestBridge(t, s, func(w http.ResponseWriter, r *http.Request) {
		t.Error("unconfirmed observer received a call", r.URL.Path)
	})
	b.options.ObservationLevel = performance.ObservationL1
	if _, err := b.Start(context.Background(), s); err == nil || !strings.Contains(err.Error(), "capability is unconfirmed") {
		t.Fatal("missing actual capability accepted", err)
	}
	if err := b.Stop(context.Background()); err != nil || !b.isolated {
		t.Fatal("unconfirmed observer prevented cleanup", err)
	}
}

func TestRemoteBridgeObserverFailureDoesNotFailConfirmedIsolation(t *testing.T) {
	s := &remoteTestSandbox{downloadFails: true}
	b := newRemoteTestBridge(t, s, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("{}")) })
	_ = startRemoteTest(t, b, s)
	if err := b.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	report, err := b.RemoteObservation()
	if err == nil || report.Status != core.StatusFailed || !b.isolated {
		t.Fatalf("observer not separated: %+v %v", report, err)
	}
	s.mu.Lock()
	cleanups := s.cleanupCalls
	s.mu.Unlock()
	if cleanups != 1 {
		t.Fatal("observation failure prevented runtime removal")
	}
}

func TestRemoteBridgeCleansPartialPrepareAndStage(t *testing.T) {
	for _, phase := range []string{"prepare", "stage"} {
		t.Run(phase, func(t *testing.T) {
			s := &remoteTestSandbox{prepareFails: phase == "prepare", stageFails: phase == "stage"}
			b := newRemoteTestBridge(t, s, func(http.ResponseWriter, *http.Request) { t.Error("worker request after failed preparation") })
			if _, err := b.Start(context.Background(), s); err == nil {
				t.Fatal("partial start succeeded")
			}
			if b.group == nil {
				t.Fatal("ambiguous group ownership discarded")
			}
			if err := b.Stop(context.Background()); err != nil {
				t.Fatal(err)
			}
			s.mu.Lock()
			stops := s.stopCalls
			s.mu.Unlock()
			if stops != 1 {
				t.Fatal("partial group not stopped")
			}
		})
	}
}

func TestRemoteBridgeRejectsMismatchedIdentityAllowsExplicitUnavailableAndCancel(t *testing.T) {
	s := &remoteTestSandbox{}
	var mu sync.Mutex
	var routes []string
	b := newRemoteTestBridge(t, s, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		routes = append(routes, r.URL.Path)
		mu.Unlock()
		_, _ = w.Write([]byte("{\"event\":\"end\"}\n"))
	})
	config := startRemoteTest(t, b, s)
	bad := bytes.Replace(remotePayload(config, "bad"), []byte(`"task_id":"task"`), []byte(`"task_id":"different"`), 1)
	if status, _, err := remoteTestRequest(config, "/call", bad); err != nil || status != 400 {
		t.Fatalf("cross-attempt admitted: %d %v", status, err)
	}
	unknown := bytes.Replace(remotePayload(config, "startup"), []byte(`"identity_status":"known"`), []byte(`"identity_status":"unavailable"`), 1)
	unknown = bytes.Replace(unknown, []byte(`"tool_call_id":"native-call"`), []byte(`"tool_call_id":""`), 1)
	if status, _, err := remoteTestRequest(config, "/call", unknown); err != nil || status != 200 {
		t.Fatalf("explicit unavailable rejected: %d %v", status, err)
	}
	cancelBody, _ := json.Marshal(map[string]any{"schema_version": 1, "binding_id": config.BindingID, "scope": "session_tree", "session_id": "native-session"})
	if status, _, err := remoteTestRequest(config, "/cancel", cancelBody); err != nil || status != 200 {
		t.Fatalf("native cancellation not forwarded: %d %v", status, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(routes, ",") != "/call,/cancel" {
		t.Fatalf("unexpected forwarding: %v", routes)
	}
}

func TestRemoteBridgeDoesNotFollowGuestRedirects(t *testing.T) {
	leaked := false
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { leaked = true }))
	defer other.Close()
	s := &remoteTestSandbox{}
	b := newRemoteTestBridge(t, s, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	})
	config := startRemoteTest(t, b, s)
	status, _, err := remoteTestRequest(config, "/call", remotePayload(config, "redirect"))
	if err != nil || status != 307 || leaked {
		t.Fatalf("provider credential redirect boundary: %d %v leaked=%t", status, err, leaked)
	}
}

type remoteCloseTimingTransport struct {
	http.RoundTripper
	closed chan time.Time
}

func (r remoteCloseTimingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := r.RoundTripper.RoundTrip(request)
	if err == nil {
		response.Body = remoteCloseTimingBody{response.Body, r.closed}
	}
	return response, err
}

type remoteCloseTimingBody struct {
	io.ReadCloser
	closed chan time.Time
}

func (b remoteCloseTimingBody) Close() error {
	b.closed <- time.Now()
	return b.ReadCloser.Close()
}

func TestRemoteBridgeObservationUsesRunnerClockAndIndependentAdmission(t *testing.T) {
	s := &remoteTestSandbox{}
	b := newRemoteTestBridge(t, s, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		time.Sleep(time.Millisecond)
		_, _ = w.Write([]byte("native\n"))
	})
	recorder, err := performance.Open(t.TempDir(), "run")
	if err != nil {
		t.Fatal(err)
	}
	defer recorder.Close()
	ctx := performance.WithRecorder(context.Background(), recorder)
	clock := performance.ClockFromContext(ctx)
	if _, err = b.Start(ctx, s); err != nil {
		t.Fatal(err)
	}
	closed := make(chan time.Time, 2)
	b.client.Transport = remoteCloseTimingTransport{b.client.Transport, closed}
	config, err := b.ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"clock-a", "clock-b"} {
		if code, _, err := remoteTestRequest(config, "/call", remotePayload(config, id)); err != nil || code != 200 {
			t.Fatal(code, err)
		}
	}
	if code, _, _ := remoteTestRequest(config, "/call", remotePayload(config, "clock-a")); code != 409 {
		t.Fatal("duplicate admitted")
	}
	if err = b.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(b.options.OutputDir, "remote-bridge-index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var index struct {
		Schema   int               `json:"schema_version"`
		Clock    performance.Clock `json:"clock"`
		Accepted int               `json:"accepted_calls"`
		Finished int               `json:"finished_calls"`
		Records  int               `json:"records"`
	}
	if json.Unmarshal(raw, &index) != nil || index.Schema != 2 || index.Accepted != 2 || index.Finished != 2 || index.Records != 2 || index.Clock.ClockDomain != clock.ClockDomain || !index.Clock.StartedAtUTC.Equal(clock.StartedAtUTC) {
		t.Fatal("clock/admission receipt mismatch")
	}
	raw, err = os.ReadFile(filepath.Join(b.options.OutputDir, "remote-bridge-events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for i, line := range bytes.Split(bytes.TrimSpace(raw), []byte("\n")) {
		var row struct {
			Sequence int    `json:"sequence"`
			Start    int64  `json:"controller_start_offset_ns"`
			End      int64  `json:"controller_end_offset_ns"`
			Duration int64  `json:"controller_duration_ns"`
			Scope    string `json:"measurement_scope"`
		}
		if json.Unmarshal(line, &row) != nil || row.Sequence != i+1 || row.Start < 0 || row.End-row.Start != row.Duration || row.Duration <= 0 || row.Scope != "controller_forwarding_not_guest_execution" {
			t.Fatal("real monotonic bounds lost")
		}
		if row.End > clock.Offset(<-closed) {
			t.Fatal("controller range includes response body cleanup after io.Copy")
		}
	}
}

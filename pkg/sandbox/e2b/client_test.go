package e2b

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/runner"
)

func testManager(t *testing.T, serve func(string, map[string]any) (int, any)) *Manager {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("t", 32) {
			t.Error("missing auth")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		status, response := serve(strings.TrimPrefix(r.URL.Path, "/v1/"), body)
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(response)
	}))
	t.Cleanup(server.Close)
	m, err := New(Options{Endpoint: server.URL, Token: strings.Repeat("t", 32), Template: "pinned", OutputDir: t.TempDir(), TimeoutSeconds: 60})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = m.FinishObservation() })
	return m
}

func createResponse(body map[string]any) any {
	return map[string]any{"sandbox": body["sandbox"], "state": "active"}
}
func request() core.SandboxRequest {
	return core.SandboxRequest{RunID: "run", TaskID: "task", Environment: core.Environment{Workdir: "/workspace"}}
}

func TestBinaryStreamingArgumentsAndStdin(t *testing.T) {
	var mu sync.Mutex
	var input []byte
	inputClosed := make(chan struct{})
	args := []string{"a b", "'", "$HOME", "`id`", ""}
	payload := []byte{0, 255, 254, 13, 10}
	m := testManager(t, func(action string, body map[string]any) (int, any) {
		switch action {
		case "create":
			return 200, createResponse(body)
		case "start":
			want, _ := json.Marshal(append([]string{"/usr/bin/tool"}, args...))
			got, _ := json.Marshal(body["argv"])
			if !bytes.Equal(want, got) {
				t.Errorf("argv changed: %s", got)
			}
			return 200, map[string]any{"process": body["process"], "pid": 42}
		case "stdin":
			data, _ := base64.StdEncoding.DecodeString(body["data_base64"].(string))
			mu.Lock()
			input = append(input, data...)
			mu.Unlock()
			if body["close"] == true {
				close(inputClosed)
			}
			return 200, map[string]bool{"accepted": true}
		case "events":
			<-inputClosed
			return 200, map[string]any{"cursor": 3, "done": true, "events": []any{map[string]any{"stream": "stdout", "data_base64": base64.StdEncoding.EncodeToString(payload)}, map[string]any{"stream": "stderr", "data_base64": "ZXJy"}, map[string]any{"exit_code": 7, "guest_duration_ns": 123456}}}
		default:
			t.Errorf("unexpected action %s", action)
			return 500, nil
		}
	})
	live, err := m.Start(context.Background(), request())
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	result, err := live.(*Sandbox).ExecStream(context.Background(), core.Command{Path: "/usr/bin/tool", Args: args}, bytes.NewReader(payload), &stdout, &stderr)
	if err != nil || result.ExitCode != 7 || !bytes.Equal(stdout.Bytes(), payload) || stderr.String() != "err" {
		t.Fatalf("exec: %+v %v %q", result, err, stdout.Bytes())
	}
	mu.Lock()
	defer mu.Unlock()
	if !bytes.Equal(input, payload) {
		t.Fatalf("stdin %v", input)
	}
}

func TestCreationLossRetainsCleanupIdentityAndRequiresPositiveAbsence(t *testing.T) {
	kills := 0
	m := testManager(t, func(action string, body map[string]any) (int, any) {
		if action == "create" {
			return 502, map[string]string{"error_code": "allocation_unconfirmed"}
		}
		kills++
		return 200, map[string]bool{"absence_confirmed": kills > 1}
	})
	live, err := m.Start(context.Background(), request())
	if err == nil || live == nil {
		t.Fatal("lost ownership")
	}
	if err = m.Stop(context.Background(), live); err == nil {
		t.Fatal("unconfirmed removal accepted")
	}
	if err = m.Stop(context.Background(), live); err != nil {
		t.Fatal(err)
	}
	if err = m.Stop(context.Background(), live); err != nil || kills != 2 {
		t.Fatalf("idempotent stop: %v, %d", err, kills)
	}
}

func TestUnsupportedPolicyRejectedBeforeCreate(t *testing.T) {
	calls := 0
	m := testManager(t, func(string, map[string]any) (int, any) { calls++; return 500, nil })
	for _, env := range []core.Environment{{Workdir: "/x", CPU: 2}, {Workdir: "/x", Image: "docker:tag"}, {Workdir: "relative"}, {Workdir: "/x", Env: map[string]string{"X": "v"}}} {
		r := request()
		r.Environment = env
		if _, err := m.Start(context.Background(), r); err == nil {
			t.Fatalf("accepted %+v", env)
		}
	}
	if calls != 0 {
		t.Fatalf("made %d effects", calls)
	}
}

func TestTaskTemplateBindingUsesFrozenTemplateAndCommandDefaults(t *testing.T) {
	for _, provider := range []string{"e2b-cloud", "huawei-agentsphere"} {
		t.Run(provider, func(t *testing.T) {
			testTaskTemplateBindingUsesFrozenTemplateAndCommandDefaults(t, provider)
		})
	}
}

func testTaskTemplateBindingUsesFrozenTemplateAndCommandDefaults(t *testing.T, provider string) {
	t.Helper()
	binding := testTaskTemplateBinding()
	if provider == "huawei-agentsphere" {
		binding.Provider = provider
	}
	storage := 20480
	binding.TemplateConfig.StorageMB = &storage
	original := binding.clone()
	var creates []map[string]any
	var commands []map[string]any
	base := testManager(t, func(action string, body map[string]any) (int, any) {
		switch action {
		case "create":
			creates = append(creates, body)
			return 200, map[string]any{"sandbox": body["sandbox"], "state": "active", "provider": provider}
		case "start":
			commands = append(commands, body)
			return 200, map[string]any{"process": body["process"], "pid": 42}
		case "kill":
			return 200, map[string]bool{"absence_confirmed": true}
		default:
			t.Errorf("unexpected action %s", action)
			return 500, nil
		}
	})
	options := base.options
	options.Provider = provider
	options.TimeoutSeconds = 4200
	options.OutputDir = t.TempDir()
	options.Template, options.LogicalTaskID, options.TemplateBinding = binding.Template, binding.TaskID, binding
	m, err := New(options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close(); _, _ = m.FinishObservation() })
	// New takes a snapshot of the operator binding. Later caller mutation must
	// neither authorize a different task nor change already-bound defaults.
	binding.RequestedEnvironment.Env["TASK_ONLY"] = "mutated"
	binding.CommandEnv["OVERRIDE"] = "mutated"
	binding.Adaptations[0] = "mutated"
	binding.Provider = "other-cloud"
	*binding.TemplateConfig.StorageMB = 1
	if !reflect.DeepEqual(m.options.TemplateBinding, original) {
		t.Fatal("manager aliases caller binding")
	}
	req := core.SandboxRequest{RunID: "run", TaskID: "fix-git-repetition-1", Environment: original.clone().RequestedEnvironment}
	live, err := m.Start(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(req.Environment, original.RequestedEnvironment) {
		t.Fatal("Start rewrote the original task environment")
	}
	if len(creates) != 1 || creates[0]["template"] != original.Template || creates[0]["allow_internet_access"] != true || creates[0]["timeout_seconds"] != float64(4200) {
		t.Fatalf("unexpected create: %#v", creates)
	}
	if len(creates[0]) != 4 {
		t.Fatalf("create gained runtime resource overrides: %#v", creates[0])
	}
	sandbox := live.(*Sandbox)
	perCall := map[string]string{"OVERRIDE": "call", "CALL_ONLY": "call"}
	if _, err := sandbox.StartProcess(context.Background(), core.Command{Path: "/bin/true", Env: perCall}); err != nil {
		t.Fatal(err)
	}
	if _, err := sandbox.StartProcess(context.Background(), core.Command{Path: "/bin/true", Dir: "/tmp", User: "user"}); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 2 {
		t.Fatalf("commands = %d", len(commands))
	}
	if commands[0]["cwd"] != "/app" || commands[0]["user"] != "root" || commands[1]["cwd"] != "/tmp" || commands[1]["user"] != "user" {
		t.Fatalf("workdir/user defaults or explicit overrides changed: %#v", commands)
	}
	wantFirst := map[string]any{"TASK_ONLY": "task", "OVERRIDE": "call", "ADAPTED": "template", "CALL_ONLY": "call"}
	wantSecond := map[string]any{"TASK_ONLY": "task", "OVERRIDE": "template", "ADAPTED": "template"}
	if !reflect.DeepEqual(commands[0]["env"], wantFirst) || !reflect.DeepEqual(commands[1]["env"], wantSecond) {
		t.Fatalf("command environment precedence: first=%#v second=%#v", commands[0]["env"], commands[1]["env"])
	}
	if !reflect.DeepEqual(perCall, map[string]string{"OVERRIDE": "call", "CALL_ONLY": "call"}) {
		t.Fatal("StartProcess mutated caller environment")
	}
}

func TestTaskTemplateBindingMismatchRejectedBeforeCreate(t *testing.T) {
	calls := 0
	base := testManager(t, func(string, map[string]any) (int, any) { calls++; return 500, nil })
	binding := testTaskTemplateBinding()
	options := base.options
	options.Template, options.LogicalTaskID, options.TemplateBinding = binding.Template, binding.TaskID, binding
	m, err := New(options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = m.FinishObservation() })
	for _, change := range []func(*core.Environment){
		func(e *core.Environment) { e.Image = "other:pinned" },
		func(e *core.Environment) { e.CPU = 0 },
		func(e *core.Environment) { e.Env["TASK_ONLY"] = "changed" },
		func(e *core.Environment) { e.AllowNetwork = false },
	} {
		env := binding.clone().RequestedEnvironment
		change(&env)
		if _, err := m.Start(context.Background(), core.SandboxRequest{RunID: "run", TaskID: "occurrence", Environment: env}); err == nil {
			t.Fatal("mismatched environment accepted")
		}
	}
	options.LogicalTaskID = "other-task"
	if _, err := New(options); err == nil {
		t.Fatal("binding accepted different logical task")
	}
	options.LogicalTaskID = binding.TaskID
	options.Template = "other-template:frozen-build"
	if _, err := New(options); err == nil {
		t.Fatal("binding accepted different template")
	}
	options.Template, options.Provider = binding.Template, "huawei-agentsphere"
	if _, err := New(options); err == nil {
		t.Fatal("E2B task binding accepted by Huawei provider")
	}
	binding.Provider, options.Provider = "huawei-agentsphere", "e2b-cloud"
	if _, err := New(options); err == nil {
		t.Fatal("Huawei task binding accepted by E2B provider")
	}
	if calls != 0 {
		t.Fatalf("mismatch caused %d adapter effects", calls)
	}
}

func TestHuaweiProviderCheckAndRejectedVerifierLeaveNoOwnership(t *testing.T) {
	provider := "e2b-cloud"
	creates, kills := 0, 0
	m := testManager(t, func(action string, body map[string]any) (int, any) {
		switch action {
		case "health":
			return 200, map[string]any{"provider": provider, "assume_network_isolation": false}
		case "create":
			creates++
			if body["allow_internet_access"] != false {
				t.Error("verifier network restriction changed")
			}
			return 422, map[string]string{"error_code": "network_isolation_unsupported"}
		case "kill":
			kills++
		}
		return 500, nil
	})
	m.options.Provider = "huawei-agentsphere"
	if err := m.CheckProvider(context.Background()); err == nil || creates != 0 {
		t.Fatal("wrong provider accepted or allocated during health check")
	}
	provider = "huawei-agentsphere"
	if err := m.CheckProvider(context.Background()); err != nil {
		t.Fatal(err)
	}
	r := request()
	r.Purpose = "verifier"
	live, err := m.Start(context.Background(), r)
	var rejected *APIError
	if live != nil || !errors.As(err, &rejected) || rejected.HTTPStatus != 422 || rejected.Code != "network_isolation_unsupported" {
		t.Fatalf("unexpected verifier result: %v %v", live, err)
	}
	if err := m.Close(); err != nil || creates != 1 || kills != 0 || len(m.owned) != 0 {
		t.Fatalf("rejected request retained allocation: %v creates=%d kills=%d", err, creates, kills)
	}
}

func TestE2BRejectsHuaweiNetworkIsolationAssumption(t *testing.T) {
	for _, provider := range []string{"", "e2b-cloud"} {
		_, err := New(Options{Provider: provider, Endpoint: "http://adapter.invalid", Token: strings.Repeat("t", 32),
			Template: "pinned", OutputDir: t.TempDir(), AssumeNetworkIsolation: true})
		if err == nil || !strings.Contains(err.Error(), "only supported for huawei-agentsphere") {
			t.Fatalf("E2B accepted a Huawei-only assumption: %v", err)
		}
	}
}

func TestHuaweiNetworkIsolationAssumptionMustMatchHealthBeforeCreate(t *testing.T) {
	for _, profileAssumes := range []bool{false, true} {
		for _, adapterMode := range []any{nil, false, true} {
			creates := 0
			m := testManager(t, func(action string, body map[string]any) (int, any) {
				switch action {
				case "health":
					health := map[string]any{"provider": "huawei-agentsphere"}
					if adapterMode != nil {
						health["assume_network_isolation"] = adapterMode
					}
					return 200, health
				case "create":
					creates++
					if body["allow_internet_access"] != false {
						t.Error("assumption enabled public network in verifier request")
					}
					return 200, map[string]any{"sandbox": body["sandbox"], "state": "active", "provider": "huawei-agentsphere"}
				case "kill":
					return 200, map[string]bool{"absence_confirmed": true}
				}
				return 500, nil
			})
			m.options.Provider = "huawei-agentsphere"
			m.options.AssumeNetworkIsolation = profileAssumes
			err := m.CheckProvider(context.Background())
			if adapterMode == nil || adapterMode != profileAssumes {
				if err == nil || creates != 0 {
					t.Fatalf("health assumption mismatch accepted: profile=%t adapter=%v", profileAssumes, adapterMode)
				}
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			if profileAssumes {
				r := request()
				r.Purpose = "verifier"
				if _, err := m.Start(context.Background(), r); err != nil || creates != 1 {
					t.Fatalf("explicit matching assumption failed: %v", err)
				}
				if err := m.Close(); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

func TestProviderMismatchAfterCreateRetainsCleanupAndUnknownMetricsCadence(t *testing.T) {
	kills := 0
	m := testManager(t, func(action string, body map[string]any) (int, any) {
		switch action {
		case "create":
			return 200, map[string]any{"sandbox": body["sandbox"], "state": "active", "provider": "e2b-cloud"}
		case "metrics":
			return 200, map[string]any{"source": "agentsphere_gateway_envd_metrics", "nominal_interval_seconds": nil,
				"metrics": []any{map[string]int{"mem_used": 123}}, "units": map[string]string{"mem_used": "bytes"}}
		case "kill":
			kills++
			return 200, map[string]bool{"absence_confirmed": true}
		}
		return 500, nil
	})
	m.options.Provider = "huawei-agentsphere"
	live, err := m.Start(context.Background(), request())
	if live == nil || err == nil {
		t.Fatal("provider mismatch lost cleanup identity or was accepted")
	}
	metrics, err := live.(*Sandbox).Metrics(context.Background())
	if err != nil || metrics.NominalIntervalSeconds != nil || metrics.Source != "agentsphere_gateway_envd_metrics" || metrics.Units["mem_used"] != "bytes" {
		t.Fatalf("metrics lost provider semantics: %+v %v", metrics, err)
	}
	encoded, err := json.Marshal(metrics)
	if err != nil || !bytes.Contains(encoded, []byte(`"nominal_interval_seconds":null`)) {
		t.Fatal("unknown interval became zero")
	}
	if err := m.Close(); err != nil || kills != 1 {
		t.Fatalf("mismatched allocation not cleaned: %v kills=%d", err, kills)
	}
}

func TestHarnessPlacementIsBorrowedWithoutChangingE2BOwnership(t *testing.T) {
	m := testManager(t, func(action string, body map[string]any) (int, any) {
		if action == "create" {
			return 200, createResponse(body)
		}
		if action != "kill" {
			t.Errorf("unexpected action %s", action)
		}
		return 200, map[string]bool{"absence_confirmed": true}
	})
	m.options.Connectivity = core.HarnessConnectivity{Placement: core.RuntimePlacement{AttachmentID: "run-owned-network"}}
	live, err := m.Start(context.Background(), request())
	if err != nil {
		t.Fatal(err)
	}
	if live.Connectivity().Placement.AttachmentID != "run-owned-network" {
		t.Fatal("lost borrowed harness placement")
	}
	if err := m.Stop(context.Background(), live); err != nil {
		t.Fatal(err)
	}
	if m.options.Connectivity.Placement.AttachmentID != "run-owned-network" {
		t.Fatal("sandbox teardown changed shared placement")
	}
}

func TestMissingFileAndDownloadConfinement(t *testing.T) {
	m := testManager(t, func(action string, body map[string]any) (int, any) {
		if action == "create" {
			return 200, createResponse(body)
		}
		if body["path"] == "/missing" {
			return 404, map[string]string{"error_code": "file_not_found"}
		}
		return 200, map[string]string{"data_base64": "AP8="}
	})
	live, err := m.Start(context.Background(), request())
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(m.options.OutputDir, "evidence.bin")
	if err = live.Download(context.Background(), "/missing", dest); !errors.Is(err, runner.ErrNotFound) {
		t.Fatal(err)
	}
	if err = live.Download(context.Background(), "/x", dest); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(dest)
	if !bytes.Equal(data, []byte{0, 255}) {
		t.Fatal(data)
	}
	outside := t.TempDir()
	if err = os.Symlink(outside, filepath.Join(m.options.OutputDir, "link")); err != nil {
		t.Fatal(err)
	}
	if err = live.Download(context.Background(), "/x", filepath.Join(m.options.OutputDir, "link", "escape")); err == nil {
		t.Fatal("symlink escape accepted")
	}
	if _, err = os.Stat(filepath.Join(outside, "escape")); !os.IsNotExist(err) {
		t.Fatalf("outside modified: %v", err)
	}
}

func TestFileTransfers64MiBAndRequestedBounds(t *testing.T) {
	for _, provider := range []string{"e2b-cloud", "huawei-agentsphere"} {
		t.Run(provider, func(t *testing.T) {
			// Both providers use this client. A 64 MiB response also exceeds
			// the former 24 MiB HTTP envelope after base64 expansion.
			content := bytes.Repeat([]byte{0, 255, 17, 42}, (64<<20)/4)
			want := sha256.Sum256(content)
			m := testManager(t, func(action string, body map[string]any) (int, any) {
				switch action {
				case "create":
					return 200, map[string]any{"sandbox": body["sandbox"], "state": "active", "provider": provider}
				case "upload":
					data, err := base64.StdEncoding.DecodeString(body["data_base64"].(string))
					if err != nil || len(data) != 64<<20 || sha256.Sum256(data) != want {
						t.Error("upload changed complete file bytes")
					}
					return 200, map[string]int{"bytes": len(data)}
				case "download":
					return 200, map[string]any{"data_base64": base64.StdEncoding.EncodeToString(content), "bytes": len(content)}
				default:
					t.Errorf("unexpected action %s", action)
					return 500, nil
				}
			})
			m.options.Provider = provider
			live, err := m.Start(context.Background(), request())
			if err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(m.options.OutputDir, "source.bin")
			if err := os.WriteFile(source, content, 0600); err != nil {
				t.Fatal(err)
			}
			if err := live.Upload(context.Background(), source, "/deck.pptx"); err != nil {
				t.Fatal(err)
			}
			destination := filepath.Join(m.options.OutputDir, "download.bin")
			if err := live.Download(context.Background(), "/native-spans.jsonl", destination); err != nil {
				t.Fatal(err)
			}
			actual, err := os.ReadFile(destination)
			if err != nil || len(actual) != len(content) || sha256.Sum256(actual) != want {
				t.Fatal("download changed complete file bytes", err)
			}
			for _, limit := range []int64{-1, (64 << 20) + 1} {
				if err := live.(*Sandbox).DownloadLimit(context.Background(), "/x", destination, limit); err == nil {
					t.Fatalf("accepted invalid bound %d", limit)
				}
			}
			if err := live.(*Sandbox).DownloadLimit(context.Background(), "/x", destination, int64(len(content)-1)); err == nil {
				t.Fatal("accepted file exceeding requested bound")
			}
			if err := os.Truncate(source, (64<<20)+1); err != nil {
				t.Fatal(err)
			}
			if err := live.Upload(context.Background(), source, "/too-big"); err == nil {
				t.Fatal("accepted upload beyond 64 MiB")
			}
		})
	}
}

func TestCancellationIsNotProcessTermination(t *testing.T) {
	var mu sync.Mutex
	kills := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Path {
		case "/v1/create":
			_ = json.NewEncoder(w).Encode(createResponse(body))
		case "/v1/start":
			_ = json.NewEncoder(w).Encode(map[string]any{"process": body["process"], "pid": 12})
		case "/v1/events":
			<-r.Context().Done()
		case "/v1/kill", "/v1/kill_process":
			mu.Lock()
			kills++
			mu.Unlock()
		default:
			_, _ = io.WriteString(w, "{}")
		}
	}))
	defer server.Close()
	m, err := New(Options{Endpoint: server.URL, Token: strings.Repeat("t", 32), Template: "pinned", OutputDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer m.FinishObservation()
	live, err := m.Start(context.Background(), request())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err = live.Exec(ctx, core.Command{Path: "sleep", Args: []string{"20"}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancellation: %v", err)
	}
	records, _ := readOperationRecords(t, m)
	foundTimeout := false
	for _, record := range records {
		if record.Operation == "exec.wait" && record.Event == "end" {
			foundTimeout = record.Status == "timeout" && record.ExitCode == nil && record.GuestDurationNS == nil
		}
	}
	if !foundTimeout {
		t.Fatal("timeout invented a guest terminal or lost observation status")
	}
	mu.Lock()
	defer mu.Unlock()
	if kills != 0 {
		t.Fatal("borrowed command cancellation deleted or killed resource")
	}
}

func TestMalformedOutputCannotBecomeSuccess(t *testing.T) {
	for _, events := range []any{[]any{map[string]any{"exit_code": 0, "guest_duration_ns": 0}, map[string]any{"exit_code": 0, "guest_duration_ns": 0}}, []any{map[string]any{"exit_code": 0, "guest_duration_ns": -1}}, []any{map[string]any{"exit_code": 0}}, []any{map[string]any{"stream": "stdout", "data_base64": "!"}}, []any{}} {
		m := testManager(t, func(action string, body map[string]any) (int, any) {
			switch action {
			case "create":
				return 200, createResponse(body)
			case "start":
				return 200, map[string]any{"process": body["process"], "pid": 3}
			default:
				return 200, map[string]any{"events": events, "cursor": len(events.([]any)), "done": true}
			}
		})
		live, err := m.Start(context.Background(), request())
		if err != nil {
			t.Fatal(err)
		}
		p, err := live.(*Sandbox).StartProcess(context.Background(), core.Command{Path: "true"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = p.Wait(context.Background(), nil, nil); err == nil {
			t.Fatal("malformed output accepted")
		}
	}
}

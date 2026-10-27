//go:build integration && linux

package jiuwenswarm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/runner"
)

type remoteHostSandbox struct{ *embeddedHostSandbox }

func (*remoteHostSandbox) ID() string { return "explicit-linux-host-fixture" }
func (s *remoteHostSandbox) Exec(ctx context.Context, c core.Command) (core.CommandResult, error) {
	if len(c.Args) == 4 && c.Args[2] == remoteStageScript {
		s.owned["runtime_dir"] = c.Args[3]
	}
	return s.embeddedHostSandbox.Exec(ctx, c)
}

// This exercises the actual guest worker and shared Linux containment. It does
// not allocate a cloud sandbox, call a model, or claim a Benchmark score.
func TestRemoteBridgeNativeHostContainment(t *testing.T) {
	root := os.Getenv("ARIES_JIUWEN_REMOTE_TEST_ROOT")
	if root == "" {
		t.Skip("requires explicit owned Linux cgroup fixture and pinned native installation")
	}
	if os.Geteuid() != 0 || !filepath.IsAbs(root) || filepath.Clean(root) != root || root == "/" {
		t.Fatal("explicit root-owned absolute test root required")
	}
	source, python := os.Getenv("ARIES_JIUWEN_REMOTE_TEST_SOURCE"), os.Getenv("ARIES_JIUWEN_REMOTE_TEST_PYTHON")
	if source == "" || python == "" {
		t.Fatal("pinned native installation is required")
	}
	work, output := filepath.Join(root, "work"), filepath.Join(root, "artifacts")
	for _, dir := range []string{work, output} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	s := &remoteHostSandbox{&embeddedHostSandbox{root: root, owned: make(map[string]string)}}
	target, _ := url.Parse("http://127.0.0.1:18867")
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = &http.Transport{Proxy: nil}
	guest := httptest.NewTLSServer(proxy)
	t.Cleanup(guest.Close)
	b, err := NewRemoteBridge(RemoteBridgeOptions{RunID: "remote-contained-fixture", TaskID: "probe", OutputDir: output, Workdir: work, SourceDir: source, PythonPath: python,
		StartTimeout: 45 * time.Second, CleanupTimeout: 15 * time.Second,
		GuestEndpoint: func(_ context.Context, got runner.Sandbox, port int) (string, http.Header, error) {
			if got != s || port != remoteWorkerPort {
				return "", nil, errors.New("wrong owned target")
			}
			return guest.URL, http.Header{"X-Fixture-Access": []string{"no-cloud-credential"}}, nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	b.client.Transport = guest.Client().Transport
	t.Cleanup(func() {
		if err := b.Stop(context.Background()); err != nil {
			t.Error(err)
			return
		}
		if group := s.owned["cgroup"]; group != "" {
			if err := os.Remove(group); err != nil {
				t.Error(err)
			}
		}
		if control := s.owned["control_dir"]; control != "" {
			if err := os.RemoveAll(control); err != nil {
				t.Error(err)
			}
		}
	})
	config := startRemoteHost(t, b, s)
	call := func(id, method, command string) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(map[string]any{"schema_version": 1, "binding_id": config.BindingID, "invocation_id": id, "cwd": work,
			"identity":  map[string]string{"run_id": config.RunID, "task_id": config.TaskID, "session_id": "controlled-fixture-session", "request_id": "controlled-fixture-request", "tool_call_id": id, "identity_status": "known"},
			"operation": map[string]string{"kind": "shell", "method": method}, "params": map[string]any{"command": command}, "stream": false})
		// Native background launch keeps its default five-second early-failure grace.
		request, err := http.NewRequest(http.MethodPost, config.BaseURL+"/call", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+config.AccessToken)
		response, err := (&http.Client{Timeout: 15 * time.Second}).Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		status := response.StatusCode
		if err != nil || status != http.StatusOK {
			t.Fatalf("native transport failed: status=%d err=%v", status, err)
		}
		decoder := json.NewDecoder(bytes.NewReader(body))
		ended := false
		var result map[string]any
		for {
			var frame map[string]any
			err := decoder.Decode(&frame)
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			if frame["invocation_id"] != id {
				t.Fatal("native invocation identity changed")
			}
			switch frame["event"] {
			case "result":
				result, _ = frame["result"].(map[string]any)
			case "end":
				ended = true
			case "error":
				t.Fatal("native operation returned an error frame")
			}
		}
		if result == nil || !ended {
			t.Fatal("native result or terminal frame missing")
		}
		data, ok := result["data"].(map[string]any)
		if !ok {
			t.Fatal("native result has no data")
		}
		return data
	}
	foreground := call("explicit-fixture-foreground", "execute_cmd", "printf 'ARIES_REMOTE_BRIDGE_PROBE\\n' > aries-remote-probe.txt; exit 7")
	if foreground["exit_code"] != float64(7) {
		t.Fatal("native nonzero exit changed")
	}
	background := call("explicit-fixture-background", "execute_cmd_background", "sleep 120")
	pid, ok := background["pid"].(float64)
	if !ok || pid <= 1 {
		t.Fatal("native background PID missing")
	}
	pidPath := filepath.Join("/proc", strconv.Itoa(int(pid)))
	if _, err := os.Stat(pidPath); err != nil {
		t.Fatal("background process was not live before Stop")
	}
	membership, err := os.ReadFile(filepath.Join(pidPath, "cgroup"))
	if err != nil || !strings.Contains(string(membership), filepath.Base(b.group.CgroupPath())) {
		t.Fatal("background process escaped owned cgroup")
	}
	if err := b.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-b.processDone:
	default:
		t.Fatal("worker was not joined")
	}
	events, err := os.ReadFile(filepath.Join(b.group.CgroupPath(), "cgroup.events"))
	if err != nil || !strings.Contains(string(events), "populated 0") {
		t.Fatal("cgroup emptiness unconfirmed")
	}
	content, err := os.ReadFile(filepath.Join(work, "aries-remote-probe.txt"))
	if err != nil || string(content) != "ARIES_REMOTE_BRIDGE_PROBE\n" {
		t.Fatal("independent post-Stop file verification failed")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, err := os.Stat(pidPath)
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("background PID remains after Stop")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if conn, err := net.DialTimeout("tcp", "127.0.0.1:18867", 200*time.Millisecond); err == nil {
		conn.Close()
		t.Fatal("worker listener remains")
	}
	if _, err := os.Stat(b.runtimeDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("private worker runtime remains")
	}
	report, err := b.RemoteObservation()
	if err != nil || len(report.LogPaths) != 4 {
		t.Fatalf("observer archive failed: %v", err)
	}
	guestEvents, err := os.ReadFile(filepath.Join(output, "remote-worker-events.jsonl"))
	if err != nil || !bytes.Contains(guestEvents, []byte("explicit-fixture-background")) {
		t.Fatal("guest invocation evidence was not archived before deletion")
	}
	if err := embeddedWriteJSON(filepath.Join(root, "result.json"), map[string]any{"status": "PASS_HOST_REMOTE_BRIDGE_CONTAINMENT", "scope": "controlled Linux host fixture; not cloud, model or Benchmark", "identity_source": "explicit fixture invocation IDs; native Agent rail not exercised", "foreground_exit_code": 7, "background_pid": int(pid), "background_live_before_stop": true, "background_absent_after_stop": true, "group_empty_confirmed": true, "worker_joined": true, "guest_port_absent": true, "private_runtime_absent": true, "guest_evidence_archived": true, "coverage": "unknown"}); err != nil {
		t.Fatal(err)
	}
}

func startRemoteHost(t *testing.T, b *RemoteBridge, s runner.Sandbox) RemoteClientConfig {
	t.Helper()
	if _, err := b.Start(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	config, err := b.ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	return config
}

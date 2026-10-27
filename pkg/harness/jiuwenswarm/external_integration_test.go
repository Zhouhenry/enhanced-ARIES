//go:build integration && linux

package jiuwenswarm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
	dockerdeployment "github.com/hyscale-lab/aries/pkg/deployment/docker"
	"github.com/hyscale-lab/aries/pkg/harness"
	"github.com/hyscale-lab/aries/pkg/performance"
	"github.com/hyscale-lab/aries/pkg/runner"
)

// An actual pinned Agent container uses the official SANDBOX provider and a
// separate Linux-host worker. The model is controlled; this is not a cloud score.
func TestExternalNativeContainerRemoteTools(t *testing.T) {
	root := os.Getenv("ARIES_JIUWEN_EXTERNAL_TEST_ROOT")
	modelFailure := os.Getenv("ARIES_JIUWEN_EXTERNAL_TEST_MODEL_FAILURE") == "1"
	if root == "" {
		t.Skip("requires explicit owned ECS host/Docker/native fixture")
	}
	host, image := os.Getenv("ARIES_JIUWEN_EXTERNAL_TEST_HOST"), os.Getenv("ARIES_JIUWEN_EXTERNAL_TEST_IMAGE")
	source, python := os.Getenv("ARIES_JIUWEN_EMBEDDED_TEST_SOURCE"), os.Getenv("ARIES_JIUWEN_EMBEDDED_TEST_PYTHON")
	if os.Geteuid() != 0 || !filepath.IsAbs(root) || net.ParseIP(host) == nil || image == "" || source == "" || python == "" {
		t.Fatal("explicit ECS fixture settings required")
	}
	analysisRoot := filepath.Join(root, "artifacts")
	output, work := filepath.Join(analysisRoot, "remote-native", "harness"), filepath.Join(root, "work")
	bridgeOutput := filepath.Join(analysisRoot, "remote-native", "bridge")
	if err := os.MkdirAll(output, 0700); err != nil {
		t.Fatal(err)
	}
	if err := embeddedWriteJSON(filepath.Join(analysisRoot, "cloud-profile.json"), map[string]any{"agent_type": "jiuwenswarm", "placement": "external"}); err != nil {
		t.Fatal(err)
	}
	recorder, err := performance.Open(analysisRoot, "jiuwen-external-fixture")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = recorder.Close() })
	sandbox := &remoteHostSandbox{&embeddedHostSandbox{root: root, owned: make(map[string]string)}}
	worker, _ := url.Parse("http://127.0.0.1:18867")
	guest := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(worker))
	defer guest.Close()
	bridge, err := NewRemoteBridge(RemoteBridgeOptions{RunID: "jiuwen-external-fixture", TaskID: "remote-native", OutputDir: bridgeOutput,
		Workdir: work, SourceDir: source, PythonPath: python, BindHost: host, GuestEndpoint: func(context.Context, runner.Sandbox, int) (string, http.Header, error) {
			return guest.URL, http.Header{"X-Fixture": []string{"test"}}, nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	bridge.client.Transport = guest.Client().Transport
	t.Cleanup(func() {
		if err := bridge.Stop(context.Background()); err != nil {
			t.Error(err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	ctx = performance.WithRecorder(ctx, recorder)
	ctx = performance.WithIdentity(ctx, performance.Identity{OccurrenceID: "remote-native"})
	endpoint, err := bridge.Start(ctx, sandbox)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	model := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Stream   bool `json:"stream"`
			Messages []struct {
				Role       string `json:"role"`
				ToolCallID string `json:"tool_call_id"`
			} `json:"messages"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&body) != nil {
			http.Error(w, "invalid request", 400)
			return
		}
		n := calls.Add(1)
		if modelFailure && n > 1 {
			http.Error(w, "controlled upstream failure", http.StatusInternalServerError)
			return
		}
		var message map[string]any
		finish := "stop"
		if n == 1 {
			args, _ := json.Marshal(map[string]string{"command": "printf 'ARIES_EXTERNAL_NATIVE_PROBE\\n' > external-probe.txt; nohup sleep 600 </dev/null >/dev/null 2>&1 & echo $! > background.pid"})
			message = map[string]any{"role": "assistant", "content": nil, "tool_calls": []map[string]any{{"id": "call_external_native_probe", "type": "function", "function": map[string]string{"name": "bash", "arguments": string(args)}}}}
			finish = "tool_calls"
		} else {
			matched := false
			for _, m := range body.Messages {
				if m.Role == "tool" && m.ToolCallID == "call_external_native_probe" {
					matched = true
				}
			}
			file, readErr := os.ReadFile(filepath.Join(work, "external-probe.txt"))
			if n != 2 || !matched || readErr != nil || string(file) != "ARIES_EXTERNAL_NATIVE_PROBE\n" {
				http.Error(w, "real remote result missing", 500)
				return
			}
			message = map[string]any{"role": "assistant", "content": "Remote native tool complete."}
		}
		base := map[string]any{"id": "chatcmpl-external-fixture", "model": "aries-protocol-probe", "created": 0}
		if body.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			if tools, ok := message["tool_calls"].([]map[string]any); ok {
				for i := range tools {
					tools[i]["index"] = i
				}
			}
			base["object"] = "chat.completion.chunk"
			for _, choice := range []map[string]any{{"index": 0, "delta": message, "finish_reason": nil}, {"index": 0, "delta": map[string]any{}, "finish_reason": finish}} {
				base["choices"] = []map[string]any{choice}
				raw, _ := json.Marshal(base)
				fmt.Fprintf(w, "data: %s\n\n", raw)
				w.(http.Flusher).Flush()
			}
			fmt.Fprint(w, "data: [DONE]\n\n")
		} else {
			w.Header().Set("Content-Type", "application/json")
			base["object"] = "chat.completion"
			base["choices"] = []map[string]any{{"index": 0, "message": message, "finish_reason": finish}}
			_ = json.NewEncoder(w).Encode(base)
		}
	}))
	model.Listener.Close()
	model.Listener, err = net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		t.Fatal(err)
	}
	model.Start()
	defer model.Close()
	docker, err := dockerdeployment.New(dockerdeployment.Options{})
	if err != nil {
		t.Fatal(err)
	}
	environment := docker.NewRunEnvironment("jiuwen-external-fixture")
	placement, err := environment.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := environment.Stop(context.Background()); err != nil {
			t.Error(err)
		}
		_ = docker.Close()
	})
	agent, err := NewExternal(ExternalOptions{Runtime: harness.RuntimeOptions{Deployment: docker, Image: image, OutputDir: analysisRoot, CleanupTimeout: 40 * time.Second},
		Bridge: bridge, Workdir: work, SourceDir: "/opt/jiuwenswarm", PythonPath: "/opt/jiuwenswarm/.venv/bin/python", ModelProxyURL: "https://fixture.invalid/v1",
		ProxyAccessToken: []byte("external-fixture-token"), ObservationLevel: performance.ObservationL1})
	if err != nil {
		t.Fatal(err)
	}
	// Package-private fixture seam; production still requires external HTTPS.
	agent.options.ModelProxyURL = model.URL + "/v1"
	t.Cleanup(func() {
		if err := agent.Stop(context.Background()); err != nil {
			t.Error(err)
		}
	})
	request := core.HarnessRequest{RunID: "jiuwen-external-fixture", TaskID: "remote-native", Endpoint: endpoint, Connectivity: core.HarnessConnectivity{Placement: placement}, Model: core.ModelConfig{Model: "aries-protocol-probe"}, Timeout: 90 * time.Second}
	if err := agent.Start(ctx, request); err != nil {
		t.Fatal(err)
	}
	result, runErr := agent.Run(ctx, "Use bash to write the requested probe in this project's directory and launch its background sleep, then finish.")
	if err := embeddedWriteJSON(filepath.Join(output, "harness-result.json"), result); err != nil {
		t.Fatal(err)
	}
	if !modelFailure && runErr != nil {
		t.Fatal(runErr)
	}
	// Prove the file exists only in the remote workspace, not the Agent container.
	local, err := agent.sandbox.Exec(ctx, core.Command{Path: "test", Args: []string{"!", "-e", filepath.Join(work, "external-probe.txt")}, User: "root", Dir: "/"})
	if err != nil || local.ExitCode != 0 {
		t.Fatal("task file was executed on Agent host")
	}
	pidRaw, err := os.ReadFile(filepath.Join(work, "background.pid"))
	pid, parseErr := strconv.Atoi(strings.TrimSpace(string(pidRaw)))
	if err != nil || parseErr != nil || pid < 2 {
		t.Fatal("real background process evidence missing")
	}
	process := filepath.Join("/proc", strconv.Itoa(pid))
	stat, err := os.ReadFile(filepath.Join(process, "stat"))
	if err != nil || strings.Fields(strings.SplitN(string(stat), ")", 2)[1])[0] == "Z" {
		t.Fatal("background process was not live before Stop")
	}
	membership, err := os.ReadFile(filepath.Join(process, "cgroup"))
	if err != nil || strings.TrimSpace(string(membership)) != "0::/"+filepath.Base(bridge.group.CgroupPath()) {
		t.Fatal("background process does not belong to the remote guest group")
	}
	if modelFailure {
		if runErr == nil || result.Status != core.StatusFailed || result.Error != ErrAgentFailed.Error() || calls.Load() < 2 {
			t.Fatalf("native model HTTP500 became success: status=%s error=%v requests=%d", result.Status, runErr, calls.Load())
		}
		if err := agent.Stop(ctx); err != nil {
			t.Fatal(err)
		}
		if err := bridge.Stop(ctx); err != nil {
			t.Fatal(err)
		}
		if !agent.isolated || agent.occurrence.ID != "" || !bridge.isolated {
			t.Fatal("failed model run did not confirm Stop and revocation")
		}
		events, err := os.ReadFile(filepath.Join(output, "native-events.jsonl"))
		if err != nil || !strings.Contains(string(events), `"event_type":"chat.error"`) {
			t.Fatal("native run-level error evidence was not retained", err)
		}
		if err := environment.Stop(ctx); err != nil {
			t.Fatal(err)
		}
		if err := recorder.Close(); err != nil {
			t.Fatal(err)
		}
		if err := embeddedWriteJSON(filepath.Join(output, "acceptance.json"), map[string]any{"status": "PASS_NATIVE_MODEL_FAILURE_FIXTURE", "model_requests": calls.Load(), "harness_status": result.Status, "agent_container_absent": true, "guest_group_empty": true, "native_chat_error_retained": true, "remote_tool_executed": true, "scope": "real native Agent executes a remote tool before controlled model HTTP500; run must fail and cleanup must succeed"}); err != nil {
			t.Fatal(err)
		}
		return
	}

	if err := agent.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := bridge.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	observation, observationErr := agent.NativeToolObservation()
	if err := embeddedWriteJSON(filepath.Join(output, "native-observation.json"), observation); err != nil {
		t.Fatal(err)
	}
	if observationErr != nil {
		t.Fatal(observationErr)
	}
	if !agent.isolated || agent.occurrence.ID != "" || !bridge.isolated || calls.Load() != 2 || result.Status != core.StatusSucceeded {
		t.Fatal("real external lifecycle incomplete")
	}
	// Match the model-issued ID through the real native Tool, provider and worker.
	// Equal counts or command text alone would not establish this association.
	invocationID := ""
	for _, name := range []string{"remote-bridge-events.jsonl", "remote-worker-events.jsonl"} {
		file, err := os.Open(filepath.Join(bridgeOutput, name))
		if err != nil {
			t.Fatal(err)
		}
		decoder, observed := json.NewDecoder(file), 0
		for {
			var record struct {
				remoteCall
				Route string `json:"route"`
				Event string `json:"event"`
			}
			if err := decoder.Decode(&record); errors.Is(err, io.EOF) {
				break
			} else if err != nil {
				file.Close()
				t.Fatal(err)
			}
			if record.Route != "/call" && record.Event != "invocation_start" && record.Event != "invocation_returned" {
				continue
			}
			if record.Identity.Status != "known" || record.Identity.ToolCallID != "call_external_native_probe" ||
				record.Identity.RunID != request.RunID || record.Identity.TaskID != request.TaskID ||
				record.Identity.SessionID != result.SessionID || record.InvocationID == "" {
				file.Close()
				t.Fatal("native ToolCall identity did not reach the exact remote invocation")
			}
			if invocationID == "" {
				invocationID = record.InvocationID
			} else if record.InvocationID != invocationID {
				file.Close()
				t.Fatal("controlled single ToolCall produced an unexpected invocation")
			}
			observed++
		}
		file.Close()
		want := 1
		if name == "remote-worker-events.jsonl" {
			want = 2
		}
		if observed != want {
			t.Fatalf("%s: observed %d identity records, want %d", name, observed, want)
		}
	}
	// Use the production snapshot and exporters on actual native/Bridge/worker
	// evidence. This fixture has no independent Benchmark score or Goodput.
	if err := environment.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	if err := embeddedWriteJSON(filepath.Join(analysisRoot, "run-result.json"), map[string]any{
		"run_id": request.RunID, "attempts": []core.AttemptResult{{
			AttemptID: request.RunID + "/" + request.TaskID, OccurrenceID: request.TaskID,
			AgentSessionID: result.SessionID, Status: "succeeded", Executed: true, Completed: true,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := performance.BuildSnapshot(filepath.Join(analysisRoot, "performance"))
	if err != nil {
		t.Fatalf("real observation snapshot: %v; %v", err, snapshot.Diagnostics)
	}
	counts, workerDomain := map[string]int{}, ""
	for _, interval := range snapshot.Intervals {
		if interval.Category == "jiuwen_remote_bridge" || interval.Category == "jiuwen_remote_worker" || interval.Category == "jiuwen_remote_provider" {
			if interval.ToolCallID != "call_external_native_probe" || interval.DurationNS <= 0 {
				t.Fatal("real remote interval lost native identity or monotonic duration")
			}
			counts[interval.Category]++
			if interval.Category == "jiuwen_remote_worker" {
				workerDomain = interval.ClockDomain
			}
		}
	}
	if counts["jiuwen_remote_bridge"] != 1 || counts["jiuwen_remote_worker"] != 1 || counts["jiuwen_remote_provider"] != 1 || workerDomain == snapshot.Summary.ClockDomain {
		t.Fatal("actual controller/worker intervals not preserved in separate clocks", counts)
	}
	nativeToolFound := false
	for _, span := range snapshot.NativeSpans {
		if span.AuthoritativeTool && span.ToolCallID == "call_external_native_probe" && span.AgentSessionID == result.SessionID {
			nativeToolFound = true
		}
	}
	if !nativeToolFound {
		t.Fatal("native Tool span missing the exact model-issued ToolCall identity")
	}
	controllerTrace, err := performance.ProjectTrace(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	workerTrace, err := performance.ProjectTraceDomain(snapshot, workerDomain)
	if err != nil {
		t.Fatal(err)
	}
	otlp, err := performance.ProjectOTLP(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]any{"analysis-snapshot.json": snapshot, "controller-trace.json": controllerTrace, "worker-trace.json": workerTrace, "otlp.json": otlp} {
		if err := embeddedWriteJSON(filepath.Join(output, name), value); err != nil {
			t.Fatal(err)
		}
	}
	for deadline := time.Now().Add(3 * time.Second); ; time.Sleep(25 * time.Millisecond) {
		_, err = os.Stat(filepath.Join("/proc", strconv.Itoa(pid)))
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("guest background PID survived Bridge.Stop")
		}
	}
	if err := embeddedWriteJSON(filepath.Join(output, "acceptance.json"), map[string]any{"status": "PASS_NATIVE_EXTERNAL_FIXTURE", "model_requests": calls.Load(), "agent_container_absent": true, "guest_group_empty": true, "remote_file_verified": true, "agent_host_file_absent": true, "native_toolcall_id": "call_external_native_probe", "bridge_worker_identity_confirmed": true, "snapshot_id": snapshot.SnapshotID, "remote_intervals": counts, "scope": "controlled model; actual native Agent, exact ToolCall-to-worker identity, process containment and same-snapshot Trace projections; no cloud benchmark or global coverage claim"}); err != nil {
		t.Fatal(err)
	}
}

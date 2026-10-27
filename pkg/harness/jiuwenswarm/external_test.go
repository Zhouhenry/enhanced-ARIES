package jiuwenswarm

import (
	"archive/tar"
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
	"github.com/hyscale-lab/aries/pkg/deployment"
	"github.com/hyscale-lab/aries/pkg/harness"
	"github.com/hyscale-lab/aries/pkg/performance"
)

type externalArchiveDeployment struct {
	deployment.Deployment
	content []byte
}

func (d externalArchiveDeployment) DownloadArchive(_ context.Context, id, source string) (io.ReadCloser, deployment.FileInfo, error) {
	if id != "owned-agent-container" || source != "/run/owned/home/data/aries-traces/traces-2026-10-27.jsonl" {
		return nil, deployment.FileInfo{}, errors.New("wrong native evidence source")
	}
	var archive bytes.Buffer
	w := tar.NewWriter(&archive)
	if err := w.WriteHeader(&tar.Header{Name: filepath.Base(source), Mode: 0o600, Size: int64(len(d.content))}); err != nil {
		return nil, deployment.FileInfo{}, err
	}
	if _, err := w.Write(d.content); err != nil {
		return nil, deployment.FileInfo{}, err
	}
	if err := w.Close(); err != nil {
		return nil, deployment.FileInfo{}, err
	}
	return io.NopCloser(bytes.NewReader(archive.Bytes())), deployment.FileInfo{Mode: 0o600, Size: int64(len(d.content))}, nil
}

func TestExternalDownloadReplacesCollectorsReservedTemporaryFile(t *testing.T) {
	content := []byte("{\"trace_id\":\"fixture-original-bytes\"}\n")
	container := agentContainer{deployment: externalArchiveDeployment{content: content}, id: "owned-agent-container"}
	directory := t.TempDir()
	destination, err := os.CreateTemp(directory, ".native-span-download-")
	if err != nil {
		t.Fatal(err)
	}
	if err := destination.Close(); err != nil {
		t.Fatal(err)
	}
	if err := container.DownloadLimit(context.Background(), "/run/owned/home/data/aries-traces/traces-2026-10-27.jsonl", destination.Name(), int64(len(content))); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(destination.Name())
	if err != nil || !bytes.Equal(got, content) {
		t.Fatalf("reserved destination did not receive original native bytes: %v", err)
	}
	info, err := os.Stat(destination.Name())
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("downloaded native evidence is not private: %v", err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 {
		t.Fatalf("atomic download left temporary files: %v", err)
	}
}

// Unexpected deployment operations fail instead of borrowing Embedded's sandbox.
type externalTestDeployment struct {
	deployment.Deployment
	mu                       sync.Mutex
	request                  deployment.Request
	staged                   []byte
	secrets                  [][]byte
	calls                    []string
	createErr, stopErr       error
	stageFails, quiesceFails bool
	removed                  bool
	stopContextErr           error
	stopBounded              bool
	terminated               chan struct{}
	terminateOnce            sync.Once
	stopEntered, stopRelease chan struct{}
	processRelease           chan struct{}
	processReleaseOnce       sync.Once
}

func (d *externalTestDeployment) Create(_ context.Context, request deployment.Request) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.request = request
	d.calls = append(d.calls, "create")
	return "owned-agent-container", d.createErr
}
func (d *externalTestDeployment) UploadArchive(_ context.Context, id, target string, archive io.Reader) error {
	if id != "owned-agent-container" || target != "/" {
		return errors.New("wrong staging identity")
	}
	_, err := io.Copy(io.Discard, archive)
	return err
}
func (d *externalTestDeployment) Validate(_ context.Context, id string, request deployment.Request, secrets [][]byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if id != "owned-agent-container" {
		return errors.New("wrong validation identity")
	}
	metadata, _ := json.Marshal(request)
	for _, secret := range secrets {
		if len(secret) == 0 || bytes.Contains(metadata, secret) {
			return errors.New("secret present in runtime metadata")
		}
		d.secrets = append(d.secrets, bytes.Clone(secret))
	}
	d.calls = append(d.calls, "validate")
	return nil
}
func (d *externalTestDeployment) Start(_ context.Context, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if id != "owned-agent-container" {
		return errors.New("wrong start identity")
	}
	d.calls = append(d.calls, "start")
	return nil
}
func (*externalTestDeployment) Address(_ context.Context, id string, port int) (string, error) {
	if id != "owned-agent-container" || port != 18866 {
		return "", errors.New("wrong AgentServer address target")
	}
	return "127.0.0.1:18866", nil
}
func (d *externalTestDeployment) Exec(_ context.Context, id string, c core.Command) (core.CommandResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if id != "owned-agent-container" || c.User != "0:0" || len(c.Args) < 3 || d.removed {
		return core.CommandResult{}, errors.New("invalid control execution")
	}
	switch c.Args[2] {
	case bootstrap:
		if len(c.Args) != 5 || c.Args[3] != "stage" {
			return core.CommandResult{}, errors.New("unexpected bootstrap operation")
		}
		d.staged = bytes.Clone(c.Stdin)
		if d.stageFails {
			return core.CommandResult{ExitCode: 1}, nil
		}
	case externalPreparedScript:
		raw, _ := json.Marshal(map[string]any{"revision": NativeRevision, "uv_lock_sha256": LockSHA256, "project_id": "project", "workdir": "/workspace", "mode": "flash", "pid": 42, "process_start_ticks": "100", "sandbox_type": "aries", "sandbox_enabled": true, "native_process_observation_supported": false})
		return core.CommandResult{Stdout: string(raw)}, nil
	case nativeTerminateScript:
		d.terminateOnce.Do(func() { close(d.terminated) })
		return core.CommandResult{Stdout: "SIGTERM_SENT"}, nil
	case externalQuiesceScript:
		if d.quiesceFails {
			return core.CommandResult{ExitCode: 1}, nil
		}
	case nativeSpanInventoryScript:
		return core.CommandResult{ExitCode: 1}, nil
	default:
		return core.CommandResult{}, errors.New("unexpected Agent control script")
	}
	return core.CommandResult{}, nil
}
func (d *externalTestDeployment) ExecStream(ctx context.Context, id string, c core.Command, _ io.Reader, out, errout io.Writer) (core.CommandResult, error) {
	if id != "owned-agent-container" || c.User != "60000:60000" || c.Dir != "/workspace" || len(c.Args) != 4 || c.Args[2] != "run" {
		return core.CommandResult{}, errors.New("native process escaped dedicated container identity")
	}
	_, _ = out.Write([]byte("private-proxy-token native started\n"))
	if d.processRelease != nil {
		<-d.processRelease
		return core.CommandResult{}, nil
	}
	select {
	case <-ctx.Done():
		return core.CommandResult{ExitCode: 130}, ctx.Err()
	case <-d.terminated:
		return core.CommandResult{}, nil
	}
}
func (d *externalTestDeployment) Stop(ctx context.Context, id string) error {
	d.mu.Lock()
	d.calls = append(d.calls, "stop:"+id)
	d.stopContextErr = ctx.Err()
	_, d.stopBounded = ctx.Deadline()
	entered, release, err := d.stopEntered, d.stopRelease, d.stopErr
	d.mu.Unlock()
	if id != "owned-agent-container" {
		return errors.New("lost partial Create identity")
	}
	if entered != nil {
		close(entered)
	}
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err != nil {
		return err
	}
	d.mu.Lock()
	d.removed = true
	d.mu.Unlock()
	d.terminateOnce.Do(func() { close(d.terminated) })
	return nil
}
func (*externalTestDeployment) Close() error { return nil }

func externalFixture(t *testing.T, d *externalTestDeployment, n *testNative) (*External, *remoteTestSandbox, core.HarnessRequest) {
	t.Helper()
	s := &remoteTestSandbox{}
	b := newRemoteTestBridge(t, s, func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "unexpected provider call", 500) })
	config := startRemoteTest(t, b, s)
	d.terminated = make(chan struct{})
	e, err := NewExternal(ExternalOptions{Runtime: harness.RuntimeOptions{Deployment: d, Image: "fixture/jiuwen:1", OutputDir: t.TempDir(), StartTimeout: time.Second, CleanupTimeout: time.Second},
		Bridge: b, Workdir: "/workspace", SourceDir: "/opt/jiuwenswarm", PythonPath: "/opt/jiuwenswarm/.venv/bin/python", ModelProxyURL: "https://model.test/v1", ProxyAccessToken: []byte("private-proxy-token"), ObservationLevel: performance.ObservationL0})
	if err != nil {
		t.Fatal(err)
	}
	e.newClient = func(address string, headers http.Header) (nativeClient, error) {
		if address != "http://127.0.0.1:18866" || len(headers) != 0 {
			t.Error("wrong native Agent endpoint")
		}
		return n, nil
	}
	t.Cleanup(func() {
		if d.processRelease != nil {
			d.processReleaseOnce.Do(func() { close(d.processRelease) })
		}
		d.mu.Lock()
		d.stopErr = nil
		d.stopEntered = nil
		d.stopRelease = nil
		d.mu.Unlock()
		if err := e.Stop(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return e, s, core.HarnessRequest{RunID: "run", TaskID: "task", Endpoint: core.ToolEndpoint{Protocol: "jiuwen-remote-v1", Address: config.BaseURL, Workdir: "/workspace"}, Model: core.ModelConfig{Model: "fixture-model"}}
}

func TestExternalStagesRemoteBindingOutsideDeploymentMetadata(t *testing.T) {
	d := &externalTestDeployment{}
	e, _, request := externalFixture(t, d, &testNative{})
	cpu, memory := 2.0, 4096
	request.CPU, request.MemoryMB = &cpu, &memory
	if err := e.Start(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	var staged struct {
		RemoteBinding RemoteClientConfig `json:"remote_binding"`
		RemoteSources map[string]string  `json:"remote_sources"`
		ProxyToken    string             `json:"proxy_token"`
	}
	if err := json.Unmarshal(d.staged, &staged); err != nil {
		t.Fatal(err)
	}
	binding, err := e.bridge.ClientConfig()
	if err != nil || staged.RemoteBinding != binding || staged.ProxyToken != "private-proxy-token" || len(staged.RemoteSources) != 4 {
		t.Fatal("private stage lost exact remote binding or pinned extension")
	}
	for _, name := range []string{"extension.py", "provider.py", "identity.py", "wire.py"} {
		want, _ := remoteProviderSources.ReadFile("remote/" + name)
		if staged.RemoteSources[name] != string(want) {
			t.Fatalf("staged %s differs", name)
		}
	}
	if len(d.secrets) != 2 || string(d.secrets[0]) != staged.ProxyToken || string(d.secrets[1]) != binding.AccessToken {
		t.Fatal("deployment metadata validation did not receive both credentials")
	}
	if d.request.ServicePort != 18866 || !d.request.Init || !d.request.NoNewPrivileges || len(d.request.Mounts) != 0 || len(d.request.Env) != 0 {
		t.Fatal("Agent deployment isolation changed")
	}
	if d.request.CPU == nil || *d.request.CPU != cpu || d.request.MemoryMB == nil || *d.request.MemoryMB != memory {
		t.Fatal("Agent deployment lost requested CPU or memory")
	}
	if e.group != nil {
		t.Fatal("External borrowed Embedded process placement")
	}
	if err := e.Start(context.Background(), request); err == nil {
		t.Fatal("reused External was admitted")
	}
	result, err := e.Run(context.Background(), "fixture instruction")
	if err != nil || result.Status != core.StatusSucceeded {
		t.Fatalf("native run: %+v %v", result, err)
	}
	if err := e.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !e.isolated || !d.removed || e.occurrence.ID != "" {
		t.Fatal("deployment absence not confirmed")
	}
	if _, err := e.Run(context.Background(), "after stop"); err == nil {
		t.Fatal("run admitted after removal")
	}
	err = filepath.WalkDir(e.artifactDir, func(p string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		raw, err := os.ReadFile(p)
		if bytes.Contains(raw, []byte(staged.ProxyToken)) || bytes.Contains(raw, []byte(binding.AccessToken)) {
			t.Errorf("secret in public evidence %s", filepath.Base(p))
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestExternalRejectsForeignEndpointBeforeDeployment(t *testing.T) {
	for _, field := range []string{"run", "task", "protocol", "address", "workdir"} {
		t.Run(field, func(t *testing.T) {
			d := &externalTestDeployment{}
			e, _, request := externalFixture(t, d, &testNative{})
			switch field {
			case "run":
				request.RunID = "other"
			case "task":
				request.TaskID = "other"
			case "protocol":
				request.Endpoint.Protocol = "embedded"
			case "address":
				request.Endpoint.Address += "/other"
			case "workdir":
				request.Endpoint.Workdir = "/other"
			}
			if err := e.Start(context.Background(), request); err == nil {
				t.Fatal("foreign endpoint accepted")
			}
			if len(d.calls) != 0 || e.runtime.InUse() {
				t.Fatal("rejected endpoint allocated an Agent runtime")
			}
		})
	}
}

func TestExternalPartialStartsRetainDeploymentIdentityUntilStop(t *testing.T) {
	for _, phase := range []string{"create", "stage", "session"} {
		t.Run(phase, func(t *testing.T) {
			d := &externalTestDeployment{stageFails: phase == "stage"}
			if phase == "create" {
				d.createErr = errors.New("partial Create transport failure")
			}
			e, _, request := externalFixture(t, d, &testNative{wrongProject: phase == "session"})
			if err := e.Start(context.Background(), request); err == nil {
				t.Fatal("partial startup succeeded")
			}
			if e.occurrence == nil || e.occurrence.ID != "owned-agent-container" || !e.runtime.InUse() {
				t.Fatal("partial allocation identity was lost")
			}
			if phase == "create" && len(d.calls) != 1 {
				t.Fatal("continued startup after partial Create")
			}
			if _, err := e.Run(context.Background(), "premature"); err == nil {
				t.Fatal("partial runtime admitted native instruction")
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := e.Stop(ctx); err != nil {
				t.Fatal(err)
			}
			if !d.removed || !e.isolated || e.runtime.InUse() || d.stopContextErr != nil || !d.stopBounded {
				t.Fatal("partial cleanup did not confirm absence with fresh bounded context")
			}
		})
	}
}

func TestExternalStopWaitsForConfirmedRemovalAndRetries(t *testing.T) {
	d := &externalTestDeployment{stopErr: errors.New("absence unconfirmed"), stopEntered: make(chan struct{}), stopRelease: make(chan struct{})}
	e, _, request := externalFixture(t, d, &testNative{})
	if err := e.Start(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- e.Stop(context.Background()) }()
	select {
	case <-d.stopEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not reach deployment")
	}
	e.mu.Lock()
	isolated, id := e.isolated, e.occurrence.ID
	e.mu.Unlock()
	if isolated || id != "owned-agent-container" || !e.runtime.InUse() {
		t.Fatal("isolation published before deployment confirmed absence")
	}
	close(d.stopRelease)
	if err := <-done; err == nil {
		t.Fatal("unconfirmed removal succeeded")
	}
	if e.isolated || e.occurrence.ID == "" || !e.runtime.InUse() || len(e.credentials.Get("proxy")) == 0 {
		t.Fatal("failed Stop discarded owned runtime or cleanup credentials")
	}
	if _, err := e.Run(context.Background(), "after failed stop"); err == nil {
		t.Fatal("failed Stop reopened admission")
	}
	d.mu.Lock()
	d.stopErr = nil
	d.stopEntered = nil
	d.stopRelease = nil
	d.mu.Unlock()
	if err := e.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !e.isolated || !d.removed || e.runtime.InUse() || e.occurrence.ID != "" || e.credentials.Get("proxy") != nil {
		t.Fatal("cleanup retry did not release ownership")
	}
	if err := e.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, call := range d.calls {
		if strings.HasPrefix(call, "stop:") {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("Stop retries=%d; want exactly two", count)
	}
}

func TestExternalStopCancelsAndJoinsNativeRun(t *testing.T) {
	entered := make(chan struct{})
	d := &externalTestDeployment{}
	e, _, request := externalFixture(t, d, &testNative{run: func(ctx context.Context, _ func(Frame)) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	}})
	if err := e.Start(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	done := make(chan core.HarnessResult, 1)
	go func() { result, _ := e.Run(context.Background(), "long native request"); done <- result }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("native run did not start")
	}
	if err := e.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-done:
		if result.Status != core.StatusCanceled {
			t.Fatalf("native cancellation became %s", result.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("Stop returned before native Run joined")
	}
	select {
	case <-e.runDone:
	default:
		t.Fatal("native request still active")
	}
	select {
	case <-e.processDone:
	default:
		t.Fatal("native launch transport still active")
	}
	if !d.removed || !e.isolated {
		t.Fatal("joined HTTP call replaced positive runtime removal")
	}
}

func TestExternalRemovedRuntimeStillRequiresJoinAndAllowsRetry(t *testing.T) {
	d := &externalTestDeployment{processRelease: make(chan struct{})}
	e, _, request := externalFixture(t, d, &testNative{})
	if err := e.Start(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if err := e.Stop(context.Background()); err == nil {
		t.Fatal("Stop succeeded while launch transport was still active")
	}
	if !d.removed || e.isolated || e.occurrence.ID != "" {
		t.Fatal("runtime absence and completed isolation were conflated")
	}
	d.processReleaseOnce.Do(func() { close(d.processRelease) })
	select {
	case <-e.processDone:
	case <-time.After(time.Second):
		t.Fatal("launch transport did not finish")
	}
	if err := e.Stop(context.Background()); err != nil {
		t.Fatalf("confirmed removal with completed join cannot recover: %v", err)
	}
	if !e.isolated {
		t.Fatal("completed retry did not confirm isolation")
	}
	count := 0
	for _, call := range d.calls {
		if strings.HasPrefix(call, "stop:") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("already absent runtime removed %d times", count)
	}
}

func TestExternalObserverFailureDoesNotPreventStopAndMergesBridgeReceipt(t *testing.T) {
	d := &externalTestDeployment{quiesceFails: true}
	e, s, request := externalFixture(t, d, &testNative{})
	s.downloadFails = true
	if err := e.Start(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	result, err := e.Run(context.Background(), "fixture instruction")
	if err != nil || result.Status != core.StatusSucceeded {
		t.Fatal("observer setup altered native result")
	}
	if err := e.Stop(context.Background()); err != nil {
		t.Fatalf("observer failure became operational Stop failure: %v", err)
	}
	if err := e.bridge.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	observation, err := e.NativeToolObservation()
	if err == nil || observation.Status != core.StatusFailed || !strings.Contains(err.Error(), "quiescence") || !strings.Contains(err.Error(), "remote worker evidence") {
		t.Fatalf("merged observer causes missing: %+v %v", observation, err)
	}
	found := map[string]bool{}
	for _, name := range observation.LogPaths {
		found[filepath.Base(name)] = true
	}
	for _, name := range []string{"agentserver.log", "remote-worker.log", "remote-bridge-events.jsonl", "remote-bridge-index.json"} {
		if !found[name] {
			t.Errorf("missing A evidence %s", name)
		}
	}
	if !strings.Contains(observation.Reason, "guest process coverage is unverified") || !e.isolated || !d.removed || !e.bridge.isolated {
		t.Fatal("operational isolation and scoped observer status were conflated")
	}
}

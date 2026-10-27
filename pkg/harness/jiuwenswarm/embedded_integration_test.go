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
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/harness/internal/sandboxprocess"
	"github.com/hyscale-lab/aries/pkg/performance"
	"github.com/hyscale-lab/aries/pkg/runner"
)

// This fixture executes on the explicitly opted-in Linux host. It is neither a
// cloud Sandbox implementation nor a Terminal Bench task or verifier.
type embeddedHostSandbox struct {
	root                     string
	owned                    map[string]string
	leased, released         bool
	nativeFilesBeforeRemoval int
}

func (*embeddedHostSandbox) Connectivity() core.HarnessConnectivity {
	return core.HarnessConnectivity{}
}
func (*embeddedHostSandbox) Upload(context.Context, string, string) error {
	return errors.New("host fixture upload unsupported")
}
func (*embeddedHostSandbox) Download(context.Context, string, string) error {
	return errors.New("host fixture download unsupported")
}
func (s *embeddedHostSandbox) DownloadLimit(_ context.Context, source, destination string, maxBytes int64) error {
	events, err := os.ReadFile(filepath.Join(s.owned["cgroup"], "cgroup.events"))
	if err != nil || !strings.Contains(string(events), "populated 0") {
		return errors.New("native archive attempted before confirmed containment")
	}
	if _, err := os.Stat(s.owned["runtime_dir"]); err != nil {
		return errors.New("native archive attempted after runtime deletion")
	}
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || int64(len(content)) > maxBytes {
		return errors.New("host fixture download exceeded bound")
	}
	return os.WriteFile(destination, content, 0o600)
}
func (s *embeddedHostSandbox) Start(_ context.Context, request core.SandboxRequest) (runner.Sandbox, error) {
	if s.leased || request.Purpose != "" || request.Environment.Workdir != filepath.Join(s.root, "work") {
		return nil, errors.New("unexpected host fixture lease")
	}
	s.leased = true
	return s, nil
}
func (s *embeddedHostSandbox) Stop(_ context.Context, target runner.Sandbox) error {
	if target != s {
		return errors.New("foreign host fixture lease")
	}
	s.released = true
	return nil
}
func (s *embeddedHostSandbox) Exec(ctx context.Context, command core.Command) (core.CommandResult, error) {
	// Persist exact ownership before submission so the bounded outer driver can
	// clean a partial start even if the test process exits unexpectedly.
	if len(command.Args) >= 6 && command.Args[2] == sandboxprocess.ControlScript && command.Args[3] == "prepare" {
		s.owned["control_dir"], s.owned["cgroup"] = command.Args[4], command.Args[5]
	}
	if len(command.Args) >= 5 && command.Args[2] == bootstrap && command.Args[3] == "stage" {
		s.owned["runtime_dir"] = command.Args[4]
	}
	if len(command.Args) == 4 && command.Args[3] == s.owned["runtime_dir"] && strings.Contains(command.Args[2], "shutil.rmtree") {
		files, err := filepath.Glob(filepath.Join(s.owned["runtime_dir"], "home/data/aries-traces/traces-*.jsonl"))
		if err != nil {
			return core.CommandResult{}, err
		}
		s.nativeFilesBeforeRemoval = len(files)
	}
	if err := embeddedWriteJSON(filepath.Join(s.root, "owned.json"), s.owned); err != nil {
		return core.CommandResult{}, err
	}
	limit := command.OutputLimitBytes
	if limit <= 0 {
		limit = 1 << 20
	}
	out, stderr := &evidenceBuffer{max: limit}, &evidenceBuffer{max: limit}
	result, err := s.ExecStream(ctx, command, bytes.NewReader(command.Stdin), out, stderr)
	result.Stdout, result.Stderr = string(out.Bytes()), string(stderr.Bytes())
	return result, err
}
func (*embeddedHostSandbox) ExecStream(ctx context.Context, command core.Command, input io.Reader, stdout, stderr io.Writer) (core.CommandResult, error) {
	if command.User != "root" {
		return core.CommandResult{}, errors.New("host fixture accepts only explicit root control calls")
	}
	if command.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, command.Timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, command.Path, command.Args...)
	cmd.Dir, cmd.Stdin, cmd.Stdout, cmd.Stderr = command.Dir, input, stdout, stderr
	cmd.Env = []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C.UTF-8"}
	for key, value := range command.Env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	cmd.WaitDelay = 5 * time.Second
	started := time.Now()
	err := cmd.Run()
	result := core.CommandResult{Duration: time.Since(started)}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	var exited *exec.ExitError
	if errors.As(err, &exited) {
		result.ExitCode = exited.ExitCode()
		return result, nil
	}
	return result, err
}

type embeddedObservedBridge struct {
	runner.ToolBridge
	revoked bool
}

func (b *embeddedObservedBridge) Stop(ctx context.Context) error {
	err := b.ToolBridge.Stop(ctx)
	b.revoked = err == nil
	return err
}

type embeddedProbeBenchmark struct {
	root      string
	harness   *Embedded
	bridge    *embeddedObservedBridge
	evaluated bool
}

func (b *embeddedProbeBenchmark) Tasks(context.Context) ([]core.Task, error) {
	return []core.Task{{ID: "contained-probe", Instruction: "Use bash to create the requested containment probe file and background sleep, then finish.", Timeout: 90 * time.Second, Environment: core.Environment{Workdir: filepath.Join(b.root, "work")}}}, nil
}
func (b *embeddedProbeBenchmark) PrepareSandbox(context.Context, core.Task, runner.Sandbox) error {
	return nil
}
func (b *embeddedProbeBenchmark) Evaluate(_ context.Context, _ core.Task, _ runner.Sandbox, _ runner.EvaluationSandboxes) (core.Evaluation, error) {
	fail := func(message string) (core.Evaluation, error) {
		return core.Evaluation{Status: core.StatusFailed, VerifierStatus: core.StatusFailed}, errors.New(message)
	}
	if !b.harness.isolated || !b.bridge.revoked {
		return fail("private probe reached before both isolation gates")
	}
	select {
	case <-b.harness.runDone:
	default:
		return fail("Run was not joined before independent evaluation")
	}
	select {
	case <-b.harness.processDone:
	default:
		return fail("AgentServer process was not joined before independent evaluation")
	}
	events, err := os.ReadFile(filepath.Join(b.harness.group.CgroupPath(), "cgroup.events"))
	if err != nil || !strings.Contains(string(events), "populated 0") {
		return fail("owned cgroup is not confirmed empty")
	}
	content, err := os.ReadFile(filepath.Join(b.root, "work", "aries-embedded-probe.txt"))
	if err != nil || string(content) != "ARIES_JIUWEN_EMBEDDED_PROBE\n" {
		return fail("real native tool output differs")
	}
	pidText, err := os.ReadFile(filepath.Join(b.root, "work", "background.pid"))
	pid, parseErr := strconv.Atoi(strings.TrimSpace(string(pidText)))
	if err != nil || parseErr != nil || pid <= 1 {
		return fail("background PID evidence missing")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, err = os.Stat(filepath.Join("/proc", strconv.Itoa(pid)))
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if time.Now().After(deadline) {
			return fail("background process PID still present after Stop")
		}
		time.Sleep(25 * time.Millisecond)
	}
	for _, port := range []string{"18865", "18866"} {
		connection, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 200*time.Millisecond)
		if err == nil {
			connection.Close()
			return fail("native listener remains after Stop")
		}
	}
	if _, err := os.Stat(b.harness.runtimeDir); !errors.Is(err, os.ErrNotExist) {
		return fail("private runtime directory remains after Stop")
	}
	b.evaluated = true
	return core.Evaluation{Status: core.StatusSucceeded, VerifierStatus: core.StatusSucceeded, Score: 1}, nil
}

func embeddedWriteJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path+".tmp", append(raw, '\n'), 0600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

func TestEmbeddedNativeHostContainment(t *testing.T) {
	root := os.Getenv("ARIES_JIUWEN_EMBEDDED_TEST_ROOT")
	if root == "" {
		t.Skip("requires explicit owned Linux host fixture, pinned native installation and controlled model server")
	}
	if os.Geteuid() != 0 || !filepath.IsAbs(root) || filepath.Clean(root) != root || root == "/" {
		t.Fatal("explicit root-owned absolute test root required")
	}
	source, python := os.Getenv("ARIES_JIUWEN_EMBEDDED_TEST_SOURCE"), os.Getenv("ARIES_JIUWEN_EMBEDDED_TEST_PYTHON")
	modelURL := os.Getenv("ARIES_JIUWEN_EMBEDDED_TEST_MODEL_URL")
	if source == "" || python == "" || modelURL != "http://127.0.0.1:18877/v1" {
		t.Fatal("pinned native paths and dedicated local model fixture required")
	}
	output := filepath.Join(root, "artifacts")
	if err := os.Mkdir(output, 0700); err != nil {
		t.Fatal(err)
	}
	level := performance.ObservationL1
	if os.Getenv("ARIES_JIUWEN_EMBEDDED_TEST_LEVEL") == "l0" {
		level = performance.ObservationL0
	}
	e, err := NewEmbedded(EmbeddedOptions{OutputDir: output, Workdir: filepath.Join(root, "work"), SourceDir: source, PythonPath: python, ObservationLevel: level,
		ModelProxyURL: "https://model.fixture.invalid/v1", ProxyAccessToken: []byte("aries-contained-fixture-token"), StartTimeout: 90 * time.Second, CleanupTimeout: 30 * time.Second,
		GuestEndpoint: func(context.Context, runner.Sandbox, int) (string, http.Header, error) {
			return "https://guest.fixture.invalid", http.Header{"X-Fixture-Access": []string{"aries-contained-guest-token"}}, nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	// Deliberate package-private test seams; production HTTPS checks stay intact.
	e.options.ModelProxyURL = modelURL
	e.newClient = func(string, http.Header) (nativeClient, error) { return NewClient("http://127.0.0.1:18866", nil) }
	t.Cleanup(func() {
		if err := e.Stop(context.Background()); err != nil {
			t.Error(err)
		}
	})
	sandbox := &embeddedHostSandbox{root: root, owned: map[string]string{}, nativeFilesBeforeRemoval: -1}
	bridge := &embeddedObservedBridge{ToolBridge: e.Bridge()}
	benchmark := &embeddedProbeBenchmark{root: root, harness: e, bridge: bridge}
	r, err := runner.New(benchmark, e, sandbox, bridge, runner.Options{Name: "Linux host containment fixture (not cloud/TB)", RunID: "jiuwen-embedded-contained", OutputDir: output, Model: core.ModelConfig{Model: "aries-protocol-probe"}, CleanupTimeout: 35 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	recorder, err := performance.Open(output, "jiuwen-embedded-contained")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(performance.WithRecorder(context.Background(), recorder), 180*time.Second)
	defer cancel()
	result, runErr := r.Run(ctx)
	recorderErr := recorder.Close()
	if err := embeddedWriteJSON(filepath.Join(output, "run-result.json"), result); err != nil {
		t.Fatal(err)
	}
	observation, observationErr := e.NativeToolObservation()
	if err := embeddedWriteJSON(filepath.Join(output, "native-observation.json"), observation); err != nil {
		t.Fatal(err)
	}
	if runErr != nil || recorderErr != nil || observationErr != nil {
		t.Fatalf("run=%v recorder=%v observation=%v", runErr, recorderErr, observationErr)
	}
	if len(result.Tasks) != 1 || result.Tasks[0].Harness.Status != core.StatusSucceeded || result.Tasks[0].Isolation.Status != core.StatusConfirmed || !benchmark.evaluated || !sandbox.released {
		t.Fatal("native lifecycle or independent probe did not complete")
	}
	if result.Tasks[0].Harness.FinalResponse == "" || observation.Status != core.StatusUnsupported {
		t.Fatal("native response missing or guest coverage overstated")
	}
	var index nativeSpanIndex
	indexPath := filepath.Join(output, "contained-probe", "harness", "native-spans-index.json")
	raw, err := os.ReadFile(indexPath)
	if err != nil || json.Unmarshal(raw, &index) != nil {
		t.Fatal("native span receipt missing")
	}
	if !e.nativeStop.ExitObserved || !e.nativeStop.CleanExit {
		t.Fatalf("native graceful shutdown incomplete: %+v", e.nativeStop)
	}
	if level == performance.ObservationL0 {
		if index.Enabled || index.CollectionStatus != "not_enabled" || len(index.Spans) != 0 || sandbox.nativeFilesBeforeRemoval != 0 {
			t.Fatal("L0 produced or collected official span evidence")
		}
	} else {
		found := false
		for _, span := range index.Spans {
			if span.AuthoritativeTool && span.ToolCallID == "call_aries_contained_probe" && span.Matched && span.ToolName == "bash" && span.Operation == "execute_tool" {
				found = true
			}
		}
		if !found || index.Invalid != 0 || !index.Enabled || index.CollectionStatus != "collected_partial" || index.Coverage != "partial" {
			t.Fatalf("official authoritative tool span or scope missing: %+v", index)
		}
	}
	snapshot, err := performance.BuildSnapshot(filepath.Join(output, "performance"))
	if err != nil {
		t.Fatal(err)
	}
	trace, err := performance.ProjectTrace(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]any{"snapshot.json": snapshot, "perfetto.json": trace} {
		if err := embeddedWriteJSON(filepath.Join(output, name), value); err != nil {
			t.Fatal(err)
		}
	}
	instants := 0
	for _, event := range snapshot.Events {
		if strings.HasPrefix(event.Name, "jiuwen.chat.") {
			instants++
		}
	}
	if instants != 5 {
		t.Fatalf("expected five controller HTTP/SSE lifecycle instants, got %d", instants)
	}
	if _, err := e.Run(context.Background(), "second instruction"); err == nil {
		t.Fatal("Stop reopened admission")
	}
	t.Logf("real pinned Flash B Harness + native tool + background process containment PASS; independent local probe only; controller lifecycle instants=%d; guest timing/coverage unsupported", instants)
}

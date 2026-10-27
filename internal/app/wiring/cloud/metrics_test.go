package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/performance"
	"github.com/hyscale-lab/aries/pkg/sandbox/e2b"
)

type metricGateFile struct {
	*os.File
	action  string
	fail    bool
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	closed  atomic.Bool
}

func (f *metricGateFile) wait(action string) error {
	if action != f.action {
		return nil
	}
	f.once.Do(func() { close(f.entered) })
	<-f.release
	if f.fail {
		return errors.New("metric file fault")
	}
	return nil
}
func (f *metricGateFile) Write(data []byte) (int, error) {
	if err := f.wait("write"); err != nil {
		return 0, err
	}
	return f.File.Write(data)
}
func (f *metricGateFile) Sync() error {
	if err := f.wait("sync"); err != nil {
		return err
	}
	return f.File.Sync()
}
func (f *metricGateFile) Close() error {
	defer f.closed.Store(true)
	return f.File.Close()
}

func metricGate(t *testing.T, observer *observedSandboxes, action string, fail bool) (*metricGateFile, func()) {
	t.Helper()
	observer.path = filepath.Join(observer.outputRoot, "gated-metrics.jsonl")
	file, err := os.OpenFile(observer.path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	gate := &metricGateFile{File: file, action: action, fail: fail, entered: make(chan struct{}), release: make(chan struct{})}
	observer.file = gate
	var once sync.Once
	release := func() { once.Do(func() { close(gate.release) }) }
	t.Cleanup(release)
	return gate, release
}

func waitMetricSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatal("metrics test operation did not reach its boundary")
	}
}

func TestSDKMetricBlockedWriteDoesNotBlockSandboxStop(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "error"}[fail], func(t *testing.T) {
			observer, kills := metricFixture(t, false, false)
			gate, release := metricGate(t, observer, "write", fail)
			live, err := observer.Start(context.Background(), core.SandboxRequest{Environment: core.Environment{Workdir: "/workspace"}})
			if err != nil {
				t.Fatal(err)
			}
			waitMetricSignal(t, gate.entered)
			stopResult := make(chan error, 1)
			go func() { stopResult <- observer.Stop(context.Background(), live) }()
			select {
			case err := <-stopResult:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("metrics Write blocked sandbox deletion")
			}
			if kills.Load() != 1 {
				t.Fatal("sandbox absence was not confirmed while Write remained blocked")
			}
			if err := observer.Stop(context.Background(), live); err != nil || kills.Load() != 1 {
				t.Fatal("repeat Stop changed cleanup")
			}
			observer.samplersMu.Lock()
			sampler := observer.samplers[live.(*e2b.Sandbox)]
			observer.samplersMu.Unlock()
			select {
			case <-sampler.done:
				t.Fatal("blocked writer reported a joined sampler")
			default:
			}
			finished := make(chan struct{})
			var report core.ObserverResult
			var finishErr error
			go func() { report, finishErr = observer.finish(); close(finished) }()
			select {
			case <-finished:
				t.Fatal("finish returned before the actual writer")
			default:
			}
			release()
			waitMetricSignal(t, finished)
			waitMetricSignal(t, sampler.done)
			if !gate.closed.Load() {
				t.Fatal("metrics file was not closed")
			}
			io := observer.fileIOReport()
			if io == nil || io.WritesStarted != 2 || io.WritesReturned != 2 || io.SyncsStarted != 1 || io.SyncsReturned != 1 {
				t.Fatalf("sample/final record or file counts changed: %+v", io)
			}
			if fail {
				if finishErr == nil || report.Status != core.StatusFailed || io.WriteErrors != 2 || report.SampleCount != 0 {
					t.Fatalf("metrics errors disappeared: %+v %+v %v", report, io, finishErr)
				}
			} else if finishErr != nil || report.Status != core.StatusSucceeded || report.SampleCount != 2 {
				t.Fatalf("successful samples changed: %+v %v", report, finishErr)
			}
			before := *io
			_, _ = observer.finish()
			if *observer.fileIOReport() != before {
				t.Fatal("repeated finish repeated file calls")
			}
		})
	}
}

func TestAttemptMetricFileWaitOccursAfterOwnedSandboxCleanup(t *testing.T) {
	for _, action := range []string{"write", "sync"} {
		t.Run(action, func(t *testing.T) {
			observer, kills := metricFixture(t, false, false)
			gate, release := metricGate(t, observer, action, true)
			_, err := observer.Start(context.Background(), core.SandboxRequest{Environment: core.Environment{Workdir: "/workspace"}})
			if err != nil {
				t.Fatal(err)
			}
			if action == "write" {
				waitMetricSignal(t, gate.entered)
			}
			a := &attempt{occurrenceID: "occurrence", outputRoot: observer.outputRoot, sandboxes: observer}
			finished := make(chan struct{})
			var closeErr error
			go func() { closeErr = a.close(); close(finished) }()
			waitMetricSignal(t, gate.entered)
			deadline := time.After(5 * time.Second)
			ticker := time.NewTicker(time.Millisecond)
			defer ticker.Stop()
			for kills.Load() != 1 {
				select {
				case <-ticker.C:
				case <-deadline:
					t.Fatal("metrics file I/O blocked fallback manager cleanup")
				}
			}
			select {
			case <-finished:
				t.Fatal("attempt claimed completion while writer remained blocked")
			default:
			}
			release()
			waitMetricSignal(t, finished)
			if closeErr != nil {
				t.Fatalf("observer fault changed successful cleanup: %v", closeErr)
			}
			report, err := a.observation()
			if err == nil || report.Status != core.StatusFailed || !gate.closed.Load() {
				t.Fatalf("observer fault or writer close missing: %+v %v", report, err)
			}
			io := observer.fileIOReport()
			if io == nil || io.WritesStarted != io.WritesReturned || io.SyncsStarted != 1 || io.SyncsReturned != 1 {
				t.Fatalf("actual file returns were not retained: %+v", io)
			}
			if action == "sync" && io.SyncErrors != 1 {
				t.Fatal("Sync failure was discarded")
			}
			if err := a.close(); err != nil || kills.Load() != 1 || *observer.fileIOReport() != *io {
				t.Fatal("repeat close repeated cleanup or file I/O")
			}
		})
	}
}

func TestSDKMetricsFailedStartAndCanceledRPCDoNotAddPostDeleteSamples(t *testing.T) {
	var creates, metrics, kills atomic.Int32
	var absent atomic.Bool
	firstRPC := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Path {
		case "/v1/create":
			state := "active"
			if creates.Add(1) == 1 {
				state = "unexpected"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"sandbox": body["sandbox"], "state": state})
		case "/v1/metrics":
			if absent.Load() {
				t.Error("Metrics issued after confirmed deletion")
			}
			if metrics.Add(1) == 1 {
				close(firstRPC)
				<-r.Context().Done()
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"source": "e2b_sdk", "metrics": []any{map[string]any{"cpu_used_pct": 12.5}}})
		case "/v1/kill":
			if kills.Add(1) == 2 {
				absent.Store(true)
			}
			_ = json.NewEncoder(w).Encode(map[string]bool{"absence_confirmed": true})
		default:
			t.Errorf("unexpected action: %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	root := t.TempDir()
	manager, err := e2b.New(e2b.Options{Endpoint: server.URL, Token: strings.Repeat("t", 32), Template: "pinned", OutputDir: root})
	if err != nil {
		t.Fatal(err)
	}
	observer := newObservedSandboxes(manager, "run", "attempt", root, performance.ObservationL1)
	t.Cleanup(func() {
		observer.stopSampling()
		_ = manager.Close()
		_, _ = observer.finish()
		_, _ = manager.FinishObservation()
	})
	request := core.SandboxRequest{Environment: core.Environment{Workdir: "/workspace"}}
	partial, err := observer.Start(context.Background(), request)
	if err == nil || partial == nil {
		t.Fatal("fixture did not return the owned partial start")
	}
	if err := observer.Stop(context.Background(), partial); err != nil {
		t.Fatal(err)
	}
	if metrics.Load() != 0 {
		t.Fatal("failed start acquired a sampler")
	}
	live, err := observer.Start(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	waitMetricSignal(t, firstRPC)
	stopped := make(chan error, 1)
	go func() { stopped <- observer.Stop(context.Background(), live) }()
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("sampling RPC did not observe cancellation")
	}
	for i := 0; i < 2; i++ {
		if err := observer.Stop(context.Background(), live); err != nil {
			t.Fatal(err)
		}
		report, err := observer.finish()
		if err != nil || report.Status != core.StatusSucceeded || report.SampleCount != 1 {
			t.Fatalf("canceled initial RPC or final sample miscounted: %+v %v", report, err)
		}
	}
	if metrics.Load() != 2 || kills.Load() != 2 {
		t.Fatalf("duplicate final RPC/deletion: metrics=%d kills=%d", metrics.Load(), kills.Load())
	}
}

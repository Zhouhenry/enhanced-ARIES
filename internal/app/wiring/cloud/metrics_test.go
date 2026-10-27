package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/performance"
	"github.com/hyscale-lab/aries/pkg/sandbox/e2b"
)

func metricEvidenceRecord(owner, source string, readings ...json.RawMessage) *metricSnapshot {
	return &metricSnapshot{SchemaVersion: 2, RunID: "run", OccurrenceID: "occ", SandboxHandle: owner, Purpose: "task",
		RequestedAt: time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC), ClientDurationNS: 123,
		Snapshot: e2b.Metrics{Source: source, Metrics: readings}}
}

func TestSDKMetricsSchema2PreservesEmptyAndNilResponseArrays(t *testing.T) {
	observer := newObservedSandboxes(nil, "run", "occ", t.TempDir(), performance.ObservationL1)
	for _, readings := range [][]json.RawMessage{{}, nil} {
		observer.writeSample(metricEvidenceRecord("sandbox-a", "e2b_sdk", readings...), nil)
	}
	report, err := observer.finish()
	if err != nil || report.SampleCount != 2 || report.Status != core.StatusNotRun {
		t.Fatalf("empty RPC receipts changed: %+v %v", report, err)
	}
	raw, err := os.ReadFile(report.LogPaths[0])
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(raw), []byte{'\n'})
	if len(lines) != 2 {
		t.Fatal("empty RPC receipt omitted")
	}
	for i, want := range []string{"[]", "null"} {
		var row struct {
			Snapshot struct {
				Metrics json.RawMessage `json:"metrics"`
			} `json:"snapshot"`
		}
		if err := json.Unmarshal(lines[i], &row); err != nil {
			t.Fatal(err)
		}
		if string(row.Snapshot.Metrics) != want {
			t.Fatalf("response array shape changed: got %s want %s", row.Snapshot.Metrics, want)
		}
	}
}

func TestSDKMetricsSchema2ReferencesPreserveResponseOrderAndOwners(t *testing.T) {
	a := json.RawMessage(`{"timestamp":"2026-10-27T00:00:10Z","cpu_used_pct":1,"mem_used":100}`)
	reordered := json.RawMessage(`{"mem_used":100,"cpu_used_pct":1,"timestamp":"2026-10-27T00:00:10Z"}`)
	changed := json.RawMessage(`{"timestamp":"2026-10-27T00:00:10Z","cpu_used_pct":2,"mem_used":100}`)
	later := json.RawMessage(`{"timestamp":"2026-10-27T00:00:11Z","cpu_used_pct":1,"mem_used":100}`)
	lateArrival := json.RawMessage(`{"timestamp":"2026-10-27T00:00:05Z","cpu_used_pct":3,"mem_used":100}`)
	unkeyed := json.RawMessage(`{"cpu_used_pct":1}`)
	invalidTime := json.RawMessage(`{"timestamp":"not-a-time","cpu_used_pct":1}`)
	rows := []*metricSnapshot{
		metricEvidenceRecord("sandbox-a", "e2b_sdk", a, a, unkeyed, invalidTime),
		metricEvidenceRecord("sandbox-a", "e2b_sdk", reordered, changed, later, lateArrival, unkeyed, invalidTime),
		metricEvidenceRecord("sandbox-b", "e2b_sdk", a),
		metricEvidenceRecord("sandbox-c", "agentsphere_gateway_envd_metrics", a),
		metricEvidenceRecord("sandbox-c", "agentsphere_gateway_envd_metrics", a),
		metricEvidenceRecord("sandbox-a", "e2b_sdk", a, changed, later, lateArrival, unkeyed, invalidTime),
	}
	before, _ := json.Marshal(rows)
	observer := newObservedSandboxes(nil, "run", "occ", t.TempDir(), performance.ObservationL1)
	for _, row := range rows {
		observer.writeSample(row, nil)
	}
	report, err := observer.finish()
	if err != nil || report.SampleCount != len(rows) || report.Status != core.StatusSucceeded {
		t.Fatalf("RPC count/outcome changed: %+v %v", report, err)
	}
	after, _ := json.Marshal(rows)
	if !bytes.Equal(before, after) {
		t.Fatal("raw input response was rewritten")
	}
	raw, err := os.ReadFile(report.LogPaths[0])
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(raw), []byte{'\n'})
	if len(lines) != len(rows) {
		t.Fatal("RPC receipts were removed")
	}
	var stored []metricSnapshot
	for i, line := range lines {
		var row metricSnapshot
		if err := json.Unmarshal(line, &row); err != nil {
			t.Fatal(err)
		}
		wantSchema := 1
		if rows[i].Snapshot.Source == "e2b_sdk" {
			wantSchema = 2
		}
		if row.SchemaVersion != wantSchema || row.RequestedAt != rows[i].RequestedAt || row.ClientDurationNS != 123 || len(row.Snapshot.Metrics) != len(rows[i].Snapshot.Metrics) {
			t.Fatal("response timing or ordered denominator lost")
		}
		stored = append(stored, row)
	}
	if stored[0].ReadingReferences != nil || stored[2].ReadingReferences != nil || stored[3].ReadingReferences != nil || stored[4].ReadingReferences != nil {
		t.Fatal("same-RPC, different-owner or non-E2B rows were incorrectly referenced")
	}
	ref := stored[1].ReadingReferences[0]
	if ref == nil || ref.SourceLine != 1 || ref.ReadingIndex != 0 || string(stored[1].Snapshot.Metrics[0]) != "null" {
		t.Fatal("repeat did not reference the first committed inline reading")
	}
	for i := 1; i < len(stored[1].Snapshot.Metrics); i++ {
		if stored[1].ReadingReferences[i] != nil || !bytes.Equal(stored[1].Snapshot.Metrics[i], rows[1].Snapshot.Metrics[i]) {
			t.Fatal("changed, late-arriving or unkeyed reading was discarded")
		}
	}
	want := []*performance.ProviderReadingReference{{SourceLine: 1, ReadingIndex: 0}, {SourceLine: 2, ReadingIndex: 1}, {SourceLine: 2, ReadingIndex: 2}, {SourceLine: 2, ReadingIndex: 3}, nil, nil}
	if !reflect.DeepEqual(stored[5].ReadingReferences, want) {
		t.Fatalf("final reading points to a reference or wrong ordered source: %+v", stored[5].ReadingReferences)
	}
}

type metricPartialWriteFile struct {
	*os.File
	calls [][]byte
}

func (f *metricPartialWriteFile) Write(data []byte) (int, error) {
	f.calls = append(f.calls, append([]byte(nil), data...))
	if len(f.calls) == 2 {
		n, _ := f.File.Write(data[:len(data)/2])
		return n, errors.New("partial metric write")
	}
	return f.File.Write(data)
}

func TestSDKMetricsSchema2WriteFailureNeverPublishesReferences(t *testing.T) {
	observer := newObservedSandboxes(nil, "run", "occ", t.TempDir(), performance.ObservationL1)
	file, err := os.CreateTemp(observer.outputRoot, "metrics-")
	if err != nil {
		t.Fatal(err)
	}
	fault := &metricPartialWriteFile{File: file}
	observer.file, observer.path = fault, file.Name()
	a := json.RawMessage(`{"timestamp":"2026-10-27T00:00:10Z","mem_used":100}`)
	b := json.RawMessage(`{"timestamp":"2026-10-27T00:00:11Z","mem_used":200}`)
	observer.writeSample(metricEvidenceRecord("sandbox-a", "e2b_sdk", a), nil)
	observer.writeSample(metricEvidenceRecord("sandbox-a", "e2b_sdk", b), nil)
	observer.writeSample(metricEvidenceRecord("sandbox-a", "e2b_sdk", a, b), nil)
	var final metricSnapshot
	if err := json.Unmarshal(fault.calls[2], &final); err != nil {
		t.Fatal(err)
	}
	if final.ReadingReferences != nil || !bytes.Equal(final.Snapshot.Metrics[0], a) || !bytes.Equal(final.Snapshot.Metrics[1], b) || !observer.referencesDisabled {
		t.Fatal("write failure leaked a reference with an uncertain physical line")
	}
	report, err := observer.finish()
	if err == nil || report.Status != core.StatusFailed || report.SampleCount != 2 {
		t.Fatal("partial write failure or successful RPC write denominator lost")
	}
}

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

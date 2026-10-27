package modelreplay

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
)

type boundaryFaultFile struct {
	*os.File
	fault                 string
	entered               chan struct{}
	release               chan struct{}
	once                  sync.Once
	writeCalls, syncCalls uint64
}

func (f *boundaryFaultFile) block(phase string) {
	if f.fault == phase {
		f.once.Do(func() { close(f.entered) })
		<-f.release
	}
}
func (f *boundaryFaultFile) Write(data []byte) (int, error) {
	f.writeCalls++
	f.block("write_block")
	switch f.fault {
	case "disk_full":
		return 0, syscall.ENOSPC
	case "read_only":
		return 0, syscall.EROFS
	case "short_write":
		return f.File.Write(data[:len(data)/2])
	}
	return f.File.Write(data)
}
func (f *boundaryFaultFile) Sync() error {
	f.syncCalls++
	f.block("sync_block")
	if f.fault == "sync_error" {
		return syscall.EIO
	}
	return f.File.Sync()
}
func (f *boundaryFaultFile) Close() error {
	err := f.File.Close()
	if f.fault == "close_error" {
		return errors.Join(err, syscall.EIO)
	}
	return err
}
func faultBoundaryRecorder(t *testing.T, fault string) (*boundaryRecorder, *boundaryFaultFile, string) {
	t.Helper()
	root := t.TempDir()
	r, err := newBoundaryRecorder(Options{OutputDir: root, Identity: testIdentity("fault-boundary")}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	f := &boundaryFaultFile{File: r.file.(*os.File), fault: fault, entered: make(chan struct{}), release: make(chan struct{})}
	r.file = f
	return r, f, root
}

func TestBoundaryFilesystemFaultsRemainIncomplete(t *testing.T) {
	for _, fault := range []string{"disk_full", "read_only", "short_write", "sync_error", "close_error"} {
		t.Run(fault, func(t *testing.T) {
			r, file, root := faultBoundaryRecorder(t, fault)
			s := &Server{boundaries: r}
			p := s.beginRequest(time.Now(), "models")
			p.finish()
			err := r.close()
			if err == nil {
				t.Fatal("filesystem fault hidden")
			}
			if fault == "short_write" && !errors.Is(err, io.ErrShortWrite) {
				t.Fatalf("short write error=%v", err)
			}
			if r.index.Status != "incomplete" || r.index.Attempted != r.index.Written+r.index.Dropped+r.index.WriteFailed || r.index.RequestsReceived != 1 || r.index.RequestsFinished != 1 {
				t.Fatalf("bad fault accounting: %+v", r.index)
			}
			if fault == "sync_error" && r.index.DurabilityState != "events_sync_failed" {
				t.Fatalf("sync fault missing: %+v", r.index)
			}
			if fault == "close_error" && r.index.DurabilityState != "events_close_failed" {
				t.Fatalf("close fault missing: %+v", r.index)
			}
			if r.index.FileIO == nil || r.index.FileIO.WritesStarted != file.writeCalls || r.index.FileIO.WritesReturned != file.writeCalls || r.index.FileIO.SyncsStarted != file.syncCalls || file.syncCalls != 1 {
				t.Fatal("model boundary file call counts disagree with observed calls")
			}
			if (fault == "disk_full" || fault == "read_only" || fault == "short_write") && (file.writeCalls != 1 || r.index.Attempted <= file.writeCalls) {
				t.Fatal("latched model events incorrectly counted as actual writes")
			}
			if fault == "short_write" && (r.index.FileIO.ShortWrites != 1 || r.index.FileIO.WriteErrors != 0 || r.index.FileIO.ReturnedBytes == 0) {
				t.Fatal("model short write return accounting changed")
			}
			if fault == "sync_error" && r.index.FileIO.SyncErrors != 1 {
				t.Fatal("model Sync error not measured")
			}
			if r.close() == nil || s.ObservationError() == nil {
				t.Fatal("duplicate Close hid fault")
			}
			if _, err := os.Stat(filepath.Join(root, "request-boundaries-index.json")); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBoundaryCloseExposesUncancellableIOBoundary(t *testing.T) {
	for _, fault := range []string{"write_block", "sync_block"} {
		t.Run(fault, func(t *testing.T) {
			r, f, root := faultBoundaryRecorder(t, fault)
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(f.release) }) }
			defer release()
			r.record(boundaryEvent{Event: "observation"}, time.Now())
			finished := make(chan error, 1)
			go func() { finished <- r.close() }()
			select {
			case <-f.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("fault did not enter")
			}
			select {
			case err := <-finished:
				t.Fatalf("Close returned before underlying I/O: %v", err)
			case <-time.After(20 * time.Millisecond):
			}
			if _, err := os.Stat(filepath.Join(root, "request-boundaries-index.json")); !os.IsNotExist(err) {
				t.Fatalf("index became final before I/O: %v", err)
			}
			release()
			select {
			case err := <-finished:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("released I/O left Close stuck")
			}
			t.Log("BLOCKED contract: ordinary-file Write/Sync is uncancellable; test released fault and joined writer, no bounded production Close claim")
		})
	}
}

func TestBoundaryWholeMissingRequestPairRetainsProducerDenominator(t *testing.T) {
	root := t.TempDir()
	file, err := os.Create(filepath.Join(root, "request-boundaries.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	r := &boundaryRecorder{file: file, queue: make(chan boundaryEvent, 1), done: make(chan struct{}), anchor: time.Now(), directory: root, index: boundaryIndex{SchemaVersion: boundarySchemaVersion, EventsFile: "request-boundaries.jsonl"}}
	r.record(boundaryEvent{Event: "filler"}, time.Now())
	s := &Server{boundaries: r}
	p := s.beginRequest(time.Now(), "models")
	p.finish()
	go r.write()
	if r.close() == nil {
		t.Fatal("whole request pair loss hidden")
	}
	if r.index.Attempted != 3 || r.index.Written != 1 || r.index.Dropped != 2 || r.index.RequestsReceived != 1 || r.index.RequestsFinished != 1 || r.index.Status != "incomplete" {
		t.Fatalf("whole pair denominator lost: %+v", r.index)
	}
}

func TestBoundaryCloseDetectsUnfinishedProducer(t *testing.T) {
	r, _, _ := faultBoundaryRecorder(t, "")
	s := &Server{boundaries: r}
	p := s.beginRequest(time.Now(), "models")
	if r.close() == nil {
		t.Fatal("unfinished request marked complete")
	}
	if r.index.ProducerState != "unfinished_requests" || r.index.Status != "incomplete" {
		t.Fatalf("inflight lost: %+v", r.index)
	}
	p.finish()
	if r.close() == nil || s.ObservationError() == nil {
		t.Fatal("late terminal hidden")
	}
}

type blockedBoundaryResponse struct {
	*httptest.ResponseRecorder
	entered, release chan struct{}
	once             sync.Once
}

func (w *blockedBoundaryResponse) Write(data []byte) (int, error) {
	w.once.Do(func() { close(w.entered) })
	<-w.release
	return w.ResponseRecorder.Write(data)
}

func TestServerCloseJoinsNonAdmittedHTTPProducers(t *testing.T) {
	for _, route := range []string{"models", "unauthorized", "unsupported"} {
		t.Run(route, func(t *testing.T) {
			s := replayServer(t, "close-"+route, fixtureTape(t), Timing{Mode: "zero"})
			for turn := 0; turn < 3; turn++ {
				if post(s, requestAt(turn)).Code != http.StatusOK {
					t.Fatal("replay failed")
				}
			}
			req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			if route != "unauthorized" {
				req.Header.Set("Authorization", "Bearer "+testToken)
			}
			if route == "unsupported" {
				req.URL.Path = "/unsupported"
			}
			w := &blockedBoundaryResponse{ResponseRecorder: httptest.NewRecorder(), entered: make(chan struct{}), release: make(chan struct{})}
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(w.release) }) }
			defer release()
			handlerDone := make(chan struct{})
			go func() { defer close(handlerDone); s.ServeHTTP(w, req) }()
			select {
			case <-w.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("handler did not enter writer")
			}
			closeDone := make(chan error, 1)
			go func() { closeDone <- s.Close() }()
			select {
			case err := <-closeDone:
				t.Fatalf("Close skipped active %s handler: %v", route, err)
			case <-time.After(20 * time.Millisecond):
			}
			release()
			select {
			case <-handlerDone:
			case <-time.After(5 * time.Second):
				t.Fatal("handler leaked")
			}
			select {
			case err := <-closeDone:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Close leaked")
			}
			assertBoundaryRequests(t, s, 4)
			if !bytes.Contains(w.Body.Bytes(), []byte("model")) && route == "models" {
				t.Fatal("model response changed")
			}
		})
	}
}

func TestBoundaryAbruptExitHasNoFinalIndex(t *testing.T) {
	const key = "ARIES_TEST_R05_BOUNDARY_CRASH_ROOT"
	if root := os.Getenv(key); root != "" {
		r, err := newBoundaryRecorder(Options{OutputDir: root, Identity: testIdentity("crash")}, time.Now())
		if err != nil {
			os.Exit(24)
		}
		r.record(boundaryEvent{Event: "before-crash"}, time.Now())
		os.Exit(23)
	}
	root := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestBoundaryAbruptExitHasNoFinalIndex$")
	cmd.Env = append(os.Environ(), key+"="+root)
	if err := cmd.Run(); err == nil {
		t.Fatal("child did not exit abruptly")
	} else if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 23 {
		t.Fatalf("unexpected child failure: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "request-boundaries-index.json")); !os.IsNotExist(err) {
		t.Fatalf("abrupt exit fabricated final index: %v", err)
	}
}

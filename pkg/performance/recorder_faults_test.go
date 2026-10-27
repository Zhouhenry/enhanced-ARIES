package performance

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
)

// These faults use real temporary files, with injected syscall results or
// releasable waits. They never fill a real volume or leave a blocked goroutine.
type recorderFaultFile struct {
	*os.File
	fault                 string
	entered               chan struct{}
	release               chan struct{}
	once                  sync.Once
	writeCalls, syncCalls uint64
}

func (f *recorderFaultFile) block(phase string) {
	if f.fault == phase {
		f.once.Do(func() { close(f.entered) })
		<-f.release
	}
}
func (f *recorderFaultFile) Write(data []byte) (int, error) {
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
func (f *recorderFaultFile) Sync() error {
	f.syncCalls++
	f.block("sync_block")
	if f.fault == "sync_error" {
		return syscall.EIO
	}
	return f.File.Sync()
}
func (f *recorderFaultFile) Close() error {
	err := f.File.Close()
	if f.fault == "close_error" {
		return errors.Join(err, syscall.EIO)
	}
	return err
}

func faultRecorder(t *testing.T, fault string) (*Recorder, *recorderFaultFile, string) {
	t.Helper()
	root := t.TempDir()
	r, err := Open(root, "fault-run")
	if err != nil {
		t.Fatal(err)
	}
	file := &recorderFaultFile{File: r.file.(*os.File), fault: fault, entered: make(chan struct{}), release: make(chan struct{})}
	// The writer cannot access this file before receiving its first event.
	r.file = file
	return r, file, root
}

func TestRecorderFilesystemFaultsRemainIncomplete(t *testing.T) {
	for _, fault := range []string{"disk_full", "read_only", "short_write", "sync_error", "close_error"} {
		t.Run(fault, func(t *testing.T) {
			r, file, root := faultRecorder(t, fault)
			_, span := Begin(WithRecorder(context.Background(), r), "fault-operation")
			span.End(nil)
			err := r.Close()
			if err == nil {
				t.Fatal("filesystem fault hidden")
			}
			if fault == "short_write" && !errors.Is(err, io.ErrShortWrite) {
				t.Fatalf("short write error = %v", err)
			}
			index := readIndex(t, root)
			if index.FileIO == nil || index.FileIO.WritesStarted != file.writeCalls || index.FileIO.WritesReturned != file.writeCalls || index.FileIO.SyncsStarted != file.syncCalls || file.syncCalls != 1 {
				t.Fatal("actual file calls disagree with reported I/O counters")
			}
			if (fault == "disk_full" || fault == "read_only" || fault == "short_write") && (file.writeCalls != 1 || index.Attempted != 2) {
				t.Fatal("latched failure was incorrectly counted as another file write")
			}
			if fault == "short_write" && (index.FileIO.ShortWrites != 1 || index.FileIO.WriteErrors != 0 || index.FileIO.ReturnedBytes == 0 || index.FileIO.ReturnedBytes >= index.FileIO.RequestedBytes) {
				t.Fatal("short nil-error write lost its real return count")
			}
			if fault == "sync_error" && index.FileIO.SyncErrors != 1 {
				t.Fatal("Sync failure missing from physical call accounting")
			}

			if index.Status != "incomplete" || index.Attempted != index.Written+index.Dropped+index.WriteFailed || index.OperationsStarted != 1 || index.OperationsFinished != 1 {
				t.Fatalf("bad fault accounting: %+v", index)
			}
			if fault == "sync_error" && index.DurabilityState != "events_sync_failed" {
				t.Fatalf("sync failure missing: %+v", index)
			}
			if fault == "close_error" && index.DurabilityState != "events_close_failed" {
				t.Fatalf("close failure missing: %+v", index)
			}
			if err := r.Close(); err == nil {
				t.Fatal("duplicate Close hid failure")
			}
		})
	}
}

func TestRecorderCloseExposesUncancellableIOBoundary(t *testing.T) {
	for _, fault := range []string{"write_block", "sync_block"} {
		t.Run(fault, func(t *testing.T) {
			r, f, root := faultRecorder(t, fault)
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(f.release) }) }
			defer release()
			Record(WithRecorder(context.Background(), r), Event{EventType: "observation", Operation: "blocked-io"})
			finished := make(chan error, 1)
			go func() { finished <- r.Close() }()
			select {
			case <-f.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("fault did not enter")
			}
			select {
			case err := <-finished:
				t.Fatalf("Close returned while underlying I/O still blocked: %v", err)
			case <-time.After(20 * time.Millisecond):
			}
			if _, err := os.Stat(filepath.Join(root, "performance", "index.json")); !os.IsNotExist(err) {
				t.Fatalf("index became final before I/O completed: %v", err)
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

func TestRecorderCloseDetectsUnfinishedProducer(t *testing.T) {
	root := t.TempDir()
	r, err := Open(root, "unfinished")
	if err != nil {
		t.Fatal(err)
	}
	_, span := Begin(WithRecorder(context.Background(), r), "unfinished")
	if err := r.Close(); err == nil {
		t.Fatal("unfinished producer marked complete")
	}
	index := readIndex(t, root)
	if index.ProducerState != "unfinished_operations" || index.Status != "incomplete" || index.OperationsStarted != 1 || index.OperationsFinished != 0 {
		t.Fatalf("inflight lost: %+v", index)
	}
	span.End(nil)
	if err := r.Close(); err == nil {
		t.Fatal("late terminal hidden")
	}
}

func TestRecorderAbruptExitHasNoFinalIndex(t *testing.T) {
	const key = "ARIES_TEST_R05_RECORDER_CRASH_ROOT"
	if root := os.Getenv(key); root != "" {
		r, err := Open(root, "crash")
		if err != nil {
			os.Exit(24)
		}
		Record(WithRecorder(context.Background(), r), Event{EventType: "observation", Operation: "before-crash"})
		os.Exit(23)
	}
	root := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestRecorderAbruptExitHasNoFinalIndex$")
	cmd.Env = append(os.Environ(), key+"="+root)
	if err := cmd.Run(); err == nil {
		t.Fatal("child did not exit abruptly")
	} else if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 23 {
		t.Fatalf("unexpected child failure: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "performance", "index.json")); !os.IsNotExist(err) {
		t.Fatalf("abrupt exit fabricated final index: %v", err)
	}
}

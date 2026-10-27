package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sirupsen/logrus"
)

func readControllerResources(t *testing.T, path string) controllerResources {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var report controllerResources
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	return report
}

func TestControllerResourcesRetainStartAndMeasureActualAllocation(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	r := newControllerResources()
	if err := r.begin(root, "sample-run"); err != nil {
		t.Fatal(err)
	}
	startPath := filepath.Join(root, "controller-resources.start.json")
	original, _ := os.ReadFile(startPath)
	start := readControllerResources(t, startPath)
	if start.Status != "incomplete" || start.End != nil || start.Delta != nil {
		t.Fatal("a first sample falsely claims completion")
	}
	allocation := make([]byte, 2<<20)
	for i := range allocation {
		allocation[i] = byte(i)
	}
	if err := r.finish(); err != nil {
		t.Fatal(err)
	}
	runtime.KeepAlive(allocation)
	end := readControllerResources(t, filepath.Join(root, "controller-resources.end.json"))
	if end.Status != "complete" || !end.StartWritten || end.End == nil || end.Delta == nil || end.Delta.CPU == nil || end.PID != os.Getpid() || end.Scope != "controller_shared_process_self" {
		t.Fatalf("missing scoped resource pair: %+v", end)
	}
	if end.Delta.Allocation.TotalBytes < uint64(len(allocation)) || end.End.StartedOffsetNS < end.Start.FinishedOffsetNS || end.End.FinishedOffsetNS < end.End.StartedOffsetNS || end.End.PeakRSSBytes == nil || *end.End.PeakRSSBytes <= 0 {
		t.Fatal("allocation, sampling interval or lifetime RSS was not measured")
	}
	after, _ := os.ReadFile(startPath)
	if string(after) != string(original) {
		t.Fatal("finalization rewrote initial evidence")
	}
	if err := r.finish(); !errors.Is(err, os.ErrExist) {
		t.Fatal("duplicate finalization overwrote original report")
	}
}

func TestControllerResourcesUnavailableAndResetCountersAreNotZero(t *testing.T) {
	start := controllerResourceSample{Allocation: controllerAllocation{TotalBytes: 10}}
	end := controllerResourceSample{Allocation: controllerAllocation{TotalBytes: 11}}
	d, err := controllerDelta(start, end)
	if err != nil || d.CPU != nil || d.Allocation.TotalBytes != 1 {
		t.Fatal("missing CPU fabricated as zero")
	}
	if d, err := controllerDelta(end, start); err == nil || d != nil {
		t.Fatal("decreased allocation counter wrapped into a delta")
	}
	start.CPU, end.CPU = &controllerCPU{UserNS: 10}, &controllerCPU{UserNS: 9}
	if d, err := controllerDelta(start, end); err == nil || d != nil {
		t.Fatal("decreased CPU counter accepted")
	}
}

func TestCloudResourcesPersistOnPreflightFailureAtBothLevels(t *testing.T) {
	for _, level := range []string{"baseline", "l0"} {
		t.Run(level, func(t *testing.T) {
			t.Setenv("ARIES_OBSERVATION_LEVEL", level)
			output := filepath.Join(t.TempDir(), "runs")
			profile := writeCloudRecordProfile(t, output, "", "https://api.deepseek.com")
			doer := &sglangPreflightDoer{t: t, wantURL: "https://api.deepseek.com/v1/models", status: http.StatusUnauthorized, body: `{}`}
			logger := logrus.New()
			logger.SetOutput(io.Discard)
			err := RunCloud(context.Background(), profile, io.Discard, Dependencies{PreflightClient: doer, Logger: logger}, forbidCloudPrepare(t), cloudRecordLookup, "")
			var failure *liveValidationError
			if !errors.As(err, &failure) || failure.category != liveValidationUnauthorized {
				t.Fatalf("resource sampling changed actual preflight failure: %v", err)
			}
			paths, err := filepath.Glob(filepath.Join(output, "*", "controller-resources.end.json"))
			if err != nil || len(paths) != 1 {
				t.Fatal("failed run lost resource evidence")
			}
			report := readControllerResources(t, paths[0])
			if report.Status != "complete" || !report.StartWritten || report.Delta == nil {
				t.Fatal("failed workload did not complete its separate measurement")
			}
		})
	}
}

func TestControllerResourcesFailedStartRemainsIncomplete(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "controller-resources.start.json")
	if err := os.WriteFile(path, []byte("previous evidence"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := newControllerResources()
	if err := r.begin(root, "failed-start"); !errors.Is(err, os.ErrExist) {
		t.Fatal("start evidence collision hidden")
	}
	if err := r.finish(); err != nil {
		t.Fatal(err)
	}
	report := readControllerResources(t, filepath.Join(root, "controller-resources.end.json"))
	if report.Status != "incomplete" || report.StartWritten {
		t.Fatal("failed initial persistence was masked")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "previous evidence" {
		t.Fatal("prior evidence replaced")
	}
}

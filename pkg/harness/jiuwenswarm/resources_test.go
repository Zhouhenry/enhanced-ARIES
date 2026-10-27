package jiuwenswarm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/performance"
)

func TestCgroupBoundaryReadsAndObserverFailureKeepRunAndIsolation(t *testing.T) {
	for _, test := range []struct {
		name  string
		level performance.ObservationLevel
		fails bool
		calls int
	}{
		{"enabled", performance.ObservationL1, false, 4}, {"read_failure", performance.ObservationL1, true, 4}, {"disabled", performance.ObservationL0, false, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := &testSandbox{resourceFails: test.fails}
			e := testEmbedded(t, s, &testNative{})
			e.options.ObservationLevel = test.level
			if err := startEmbedded(t, e, s); err != nil {
				t.Fatal(err)
			}
			result, err := e.Run(context.Background(), "repair")
			if err != nil || result.Status != core.StatusSucceeded {
				t.Fatal("observation changed business result")
			}
			if err := e.Stop(context.Background()); err != nil || !e.isolated {
				t.Fatalf("observation changed Stop: %v", err)
			}
			if s.resourceCalls != test.calls || s.stopCalls != 1 {
				t.Fatalf("reads=%d stop=%d", s.resourceCalls, s.stopCalls)
			}
			report, err := e.NativeToolObservation()
			if test.fails && (err == nil || report.Status != core.StatusFailed) {
				t.Fatal("read failure not retained in observer")
			}
			boundaries := []string{"start"}
			if test.calls > 0 {
				boundaries = append(boundaries, "runtime_start", "runtime_end", "stopped")
			}
			for _, boundary := range boundaries {
				raw, err := os.ReadFile(filepath.Join(e.artifactDir, "cgroup-resources."+boundary+".json"))
				if err != nil {
					t.Fatal(err)
				}
				var evidence cgroupResourceEvidence
				if json.Unmarshal(raw, &evidence) != nil || evidence.Boundary != boundary || evidence.RunID != "run" || evidence.TaskID != "task" {
					t.Fatal("boundary identity lost")
				}
				if test.calls == 0 && (evidence.Enabled || evidence.Reading != nil || evidence.Status != "not_enabled") {
					t.Fatal("disabled sample invented")
				}
			}
		})
	}
}

func TestCgroupReadEvidenceSurvivesUnconfirmedStop(t *testing.T) {
	s := &testSandbox{}
	e := testEmbedded(t, s, &testNative{})
	if err := startEmbedded(t, e, s); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Run(context.Background(), "repair"); err != nil {
		t.Fatal(err)
	}
	s.stopFails = true
	if err := e.Stop(context.Background()); err == nil {
		t.Fatal("unconfirmed Stop succeeded")
	}
	if _, err := os.Stat(filepath.Join(e.artifactDir, "cgroup-resources.runtime_end.json")); err != nil {
		t.Fatal("earlier reading lost")
	}
	if _, err := os.Stat(filepath.Join(e.artifactDir, "cgroup-resources.stopped.json")); !os.IsNotExist(err) {
		t.Fatal("unconfirmed Stop fabricated final reading")
	}
}

func TestCgroupFinalReadRemainsSingleAcrossCleanupRetry(t *testing.T) {
	s := &testSandbox{}
	e := testEmbedded(t, s, &testNative{})
	if err := startEmbedded(t, e, s); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Run(context.Background(), "repair"); err != nil {
		t.Fatal(err)
	}
	s.cleanupFails = true
	if err := e.Stop(context.Background()); err == nil {
		t.Fatal("private cleanup failure hidden")
	}
	if s.resourceCalls != 4 {
		t.Fatal("final read missing")
	}
	s.cleanupFails = false
	if err := e.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.resourceCalls != 4 || !e.isolated {
		t.Fatal("cleanup retry duplicated final reading")
	}
}

func TestCgroupObservationDoesNotReplaceNativeErrorAndWriteFailureIsReported(t *testing.T) {
	for _, readFailure := range []bool{false, true} {
		t.Run(fmt.Sprint(readFailure), func(t *testing.T) {
			s := &testSandbox{resourceFails: readFailure}
			e := testEmbedded(t, s, &testNative{run: func(context.Context, func(Frame)) error { return errors.New("native execution failed") }})
			if err := startEmbedded(t, e, s); err != nil {
				t.Fatal(err)
			}
			if !readFailure {
				if err := os.Mkdir(filepath.Join(e.artifactDir, "cgroup-resources.runtime_start.json"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			result, err := e.Run(context.Background(), "repair")
			if err == nil || err.Error() != "native execution failed" || result.Error != "native execution failed" || result.Status != core.StatusFailed {
				t.Fatal("resource observation replaced native failure")
			}
			if err := e.Stop(context.Background()); err != nil || !e.isolated {
				t.Fatalf("resource failure changed confirmed Stop: %v", err)
			}
			report, err := e.NativeToolObservation()
			if err == nil || report.Status != core.StatusFailed {
				t.Fatal("read or artifact write failure lost")
			}
		})
	}
}

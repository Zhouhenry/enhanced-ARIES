package modelreplay

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/performance"
)

type legacyIOFaultFile struct {
	*os.File
	fault                       string
	writes, requested, returned uint64
	syncs, closes               uint64
}

func (f *legacyIOFaultFile) Write(data []byte) (int, error) {
	f.writes++
	f.requested += uint64(len(data))
	if f.fault == "enospc" {
		return 0, errors.New("injected ENOSPC")
	}
	if f.fault == "negative_n" {
		return -1, nil
	}
	if f.fault == "excess_n" {
		return len(data) + 1, nil
	}
	if f.fault == "short_nil" {
		data = data[:len(data)/2]
	}
	n, err := f.File.Write(data)
	f.returned += uint64(n)
	return n, err
}

func (f *legacyIOFaultFile) Sync() error {
	f.syncs++
	if f.fault == "sync_error" {
		return errors.New("injected sync failure")
	}
	return f.File.Sync()
}

func (f *legacyIOFaultFile) Close() error {
	f.closes++
	err := f.File.Close()
	if f.fault == "close_error" {
		return errors.Join(err, errors.New("injected close failure"))
	}
	return err
}

func completeLegacyIOReplay(t *testing.T, s *Server) {
	t.Helper()
	for i := range s.tape.Turns {
		w := post(s, requestAt(i))
		if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), bytes.Join(framesAt(i), nil)) {
			t.Fatalf("replay response %d changed: status=%d", i, w.Code)
		}
	}
}

func readLegacyIOReport(t *testing.T, s *Server) (legacyFileIOReport, []byte) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(s.options.OutputDir, "events-file-io.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report legacyFileIOReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	return report, data
}

func TestLegacyFileIOCountsActualCallsAndSeparatesObservationFailure(t *testing.T) {
	for _, fault := range []string{"none", "enospc", "short_nil", "sync_error", "close_error", "negative_n", "excess_n"} {
		t.Run(fault, func(t *testing.T) {
			s := replayServer(t, "legacy-io-"+fault, fixtureTape(t), Timing{Mode: "zero"})
			initial := s.observations.counters // session_started already reached the real file.
			file := &legacyIOFaultFile{File: s.observations.file.(*os.File), fault: fault}
			s.observations.file = file
			completeLegacyIOReplay(t, s)
			if err := s.Close(); err != nil {
				t.Fatalf("observation failure changed successful replay: fault=%s err=%v", fault, err)
			}
			observationErr := s.ObservationError()
			if (observationErr != nil) != (fault != "none") {
				t.Fatalf("observation failure was lost: fault=%s err=%v", fault, observationErr)
			}
			if fault == "short_nil" && !errors.Is(observationErr, io.ErrShortWrite) {
				t.Fatalf("nil-error short write was hidden: %v", observationErr)
			}
			report, original := readLegacyIOReport(t, s)
			c := report.FileIO
			if report.SchemaVersion != 1 || report.Identity != s.options.Identity || report.EventsFile != "events.jsonl" || c.Scope != "model_legacy_events_jsonl_only" {
				t.Fatal("file accounting lost identity or scope")
			}
			if c.WritesStarted != initial.WritesStarted+file.writes || c.WritesReturned != c.WritesStarted ||
				c.RequestedBytes != initial.RequestedBytes+file.requested || c.ReturnedBytes != initial.ReturnedBytes+file.returned ||
				c.SyncsStarted != file.syncs || c.SyncsReturned != file.syncs || file.syncs != 1 || file.closes != 1 {
				t.Fatalf("counters differ from actual calls: %+v, %+v", c, file)
			}
			info, err := os.Stat(filepath.Join(s.options.OutputDir, "events.jsonl"))
			if err != nil || uint64(info.Size()) != c.ReturnedBytes {
				t.Fatalf("returned bytes differ from file bytes: %v", err)
			}
			invalid := fault == "negative_n" || fault == "excess_n"
			if (report.Status == "complete") != (fault == "none") || (c.InvalidReturns != 0) != invalid {
				t.Fatalf("wrong I/O completion status: %+v", report)
			}
			if invalid && (c.InvalidReturns != file.writes || c.ReturnedBytes != initial.ReturnedBytes || !strings.Contains(observationErr.Error(), "invalid byte count")) {
				t.Fatal("invalid Write return was accepted or its bytes invented")
			}
			if fault == "enospc" && (file.writes != 1 || c.WriteErrors != 1 || c.ShortWrites != 1) {
				t.Fatal("latched observation failure invented subsequent writes")
			}
			if fault == "short_nil" && (c.ShortWrites != file.writes || c.WriteErrors != 0 || c.ReturnedBytes >= c.RequestedBytes) {
				t.Fatal("nil-error short writes were not preserved and counted")
			}
			if (c.SyncErrors != 0) != (fault == "sync_error") {
				t.Fatal("sync errors miscounted")
			}
			if err := s.Close(); err != nil { // Existing repeated-Close contract returns nil.
				t.Fatal(err)
			}
			again, rawAgain := readLegacyIOReport(t, s)
			if !bytes.Equal(original, rawAgain) || again != report || s.observations.counters != c || file.syncs != 1 || file.closes != 1 {
				t.Fatal("repeated Close changed accounting or repeated file operations")
			}
			if s.legacyIOReportError != nil {
				t.Fatalf("sidecar publication unexpectedly failed: %v", s.legacyIOReportError)
			}
			if again := s.ObservationError(); (again == nil) != (observationErr == nil) || (again != nil && again.Error() != observationErr.Error()) {
				t.Fatalf("repeated Close lost or changed observation failure: %v", again)
			}
			// The file is closed and the boundary writer joined. Reporting must
			// retain the failure independently of optional file/counter objects.
			s.observations, s.boundaries = nil, nil
			if err := s.ObservationError(); (err == nil) != (observationErr == nil) || (err != nil && err.Error() != observationErr.Error()) {
				t.Fatalf("optional writer absence hid a latched failure: %v", err)
			}
		})
	}
}

func TestLegacyFileIOFailureDoesNotMaskStrictFailure(t *testing.T) {
	for _, outcome := range []string{"incomplete", "diverged"} {
		t.Run(outcome, func(t *testing.T) {
			s := replayServer(t, "legacy-io-"+outcome, fixtureTape(t), Timing{Mode: "zero"})
			s.observations.file = &legacyIOFaultFile{File: s.observations.file.(*os.File), fault: "enospc"}
			if w := post(s, requestAt(0)); w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), bytes.Join(framesAt(0), nil)) {
				t.Fatal("observation fault changed the first response")
			}
			if outcome == "diverged" {
				raw := bytes.Replace(requestAt(1), []byte("result-0"), []byte("changed-result"), 1)
				if w := post(s, raw); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "REPLAY_DIVERGED") {
					t.Fatal("observation fault hid strict divergence")
				}
				if post(s, requestAt(1)).Code != http.StatusConflict {
					t.Fatal("diverged session silently recovered")
				}
			}
			if err := s.Close(); err == nil || !strings.Contains(err.Error(), "model session incomplete") || strings.Contains(err.Error(), "ENOSPC") {
				t.Fatalf("model and observation failures were not separated: %v", err)
			}
			if err := s.ObservationError(); err == nil || !strings.Contains(err.Error(), "ENOSPC") {
				t.Fatalf("model failure hid the observation fault: %v", err)
			}
			if err := s.Close(); err != nil {
				t.Fatalf("repeated Close contract changed: %v", err)
			}
		})
	}
}

func TestLegacyFileIORecordFailureDoesNotMaskTapeOutcome(t *testing.T) {
	for _, fault := range []string{"none", "enospc"} {
		for _, conflict := range []bool{false, true} {
			name := fault
			if conflict {
				name += "-tape-conflict"
			}
			t.Run(name, func(t *testing.T) {
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					i := int(calls.Add(1)) - 1
					raw, err := io.ReadAll(r.Body)
					if err != nil || !bytes.Equal(raw, requestAt(i)) {
						t.Error("observation failure changed the upstream request")
					}
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = w.Write(bytes.Join(framesAt(i), nil))
				}))
				defer upstream.Close()
				s, err := New(Options{Mode: "record", Identity: testIdentity("legacy-record-" + name), Model: "test-model",
					AccessToken: []byte(testToken), UpstreamURL: upstream.URL, UpstreamKey: []byte(testUpstreamKey),
					OutputDir: filepath.Join(t.TempDir(), "session")})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = s.Close() })
				s.observations.file = &legacyIOFaultFile{File: s.observations.file.(*os.File), fault: fault}
				for i := 0; i < 3; i++ {
					w := post(s, requestAt(i))
					if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), bytes.Join(framesAt(i), nil)) {
						t.Fatalf("recorded response %d changed", i)
					}
					if i == 0 {
						retry := post(s, requestAt(i))
						if retry.Code != http.StatusOK || !bytes.Equal(retry.Body.Bytes(), w.Body.Bytes()) {
							t.Fatal("observation failure changed cached retry")
						}
					}
				}
				if calls.Load() != 3 {
					t.Fatalf("cached retry called upstream: %d", calls.Load())
				}
				tapePath := filepath.Join(s.options.OutputDir, "tape.json")
				if conflict {
					if err := os.WriteFile(tapePath, []byte("existing tape sentinel"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				closeErr := s.Close()
				if (closeErr != nil) != conflict || (conflict && !errors.Is(closeErr, os.ErrExist)) {
					t.Fatalf("Tape failure was masked or telemetry changed success: %v", closeErr)
				}
				if err := s.ObservationError(); (err != nil) != (fault != "none") {
					t.Fatalf("wrong independent observation result: %v", err)
				}
				if conflict {
					data, err := os.ReadFile(tapePath)
					if err != nil || string(data) != "existing tape sentinel" {
						t.Fatal("existing Tape overwritten")
					}
				} else {
					tape, err := Load(tapePath)
					if err != nil || !tape.Complete || len(tape.Turns) != 3 {
						t.Fatalf("valid recorded Tape lost to observation failure: %v", err)
					}
					replay := replayServer(t, "legacy-record-replay-"+name, tape, Timing{Mode: "zero"})
					completeLegacyIOReplay(t, replay)
					if err := replay.Close(); err != nil {
						t.Fatal(err)
					}
				}
				if err := s.Close(); err != nil {
					t.Fatalf("repeated Close contract changed: %v", err)
				}
			})
		}
	}
}

func TestLegacyFileIOSidecarFailureDoesNotChangeSuccessfulReplay(t *testing.T) {
	s := replayServer(t, "legacy-io-sidecar", fixtureTape(t), Timing{Mode: "zero"})
	path := filepath.Join(s.options.OutputDir, "events-file-io.json")
	if err := os.WriteFile(path, []byte("existing sentinel"), 0600); err != nil {
		t.Fatal(err)
	}
	completeLegacyIOReplay(t, s)
	if err := s.Close(); err != nil {
		t.Fatalf("metering sidecar changed successful Close: %v", err)
	}
	if !errors.Is(s.legacyFileIOError(), os.ErrExist) {
		t.Fatal("sidecar publication failure was silently accepted")
	}
	if !errors.Is(s.ObservationError(), os.ErrExist) {
		t.Fatal("sidecar failure missing from the public observation result")
	}
	// Close has joined the writer, so removing this reference cannot race it.
	boundaries := s.boundaries
	s.boundaries = nil
	if !errors.Is(s.ObservationError(), os.ErrExist) {
		t.Fatal("missing boundary recorder hid the sidecar failure")
	}
	s.boundaries = boundaries
	boundaries.record(boundaryEvent{Event: "late_test_event"}, time.Now())
	if err := s.ObservationError(); !errors.Is(err, os.ErrExist) || !strings.Contains(err.Error(), "model boundary events after close") {
		t.Fatalf("sidecar and boundary failures must both be retained: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("observation lookup changed repeated-Close behavior: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "existing sentinel" {
		t.Fatal("existing sidecar was overwritten")
	}
}

func TestLegacyFileIOBaselineDoesNotCreateOptionalEvidence(t *testing.T) {
	s, err := New(Options{Mode: "replay", Identity: testIdentity("legacy-io-baseline"), AccessToken: []byte(testToken),
		Tape: fixtureTape(t), OutputDir: filepath.Join(t.TempDir(), "session"), Timing: Timing{Mode: "zero"},
		ObservationLevel: performance.ObservationBaseline})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	completeLegacyIOReplay(t, s)
	if err := s.Close(); err != nil || s.ObservationError() != nil || s.observations != nil {
		t.Fatalf("baseline changed: %v", err)
	}
	for _, name := range []string{"events.jsonl", "events-file-io.json"} {
		if _, err := os.Stat(filepath.Join(s.options.OutputDir, name)); !os.IsNotExist(err) {
			t.Fatalf("baseline unexpectedly created %s", name)
		}
	}
}

func TestLegacyFileIOSerializesConcurrentObservationWrites(t *testing.T) {
	s := replayServer(t, "legacy-io-concurrent", fixtureTape(t), Timing{Mode: "zero"})
	initial := s.observations.counters
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.observe(observation{Event: "frame_sent"})
		}()
	}
	wg.Wait()
	if s.observations.counters.WritesStarted != initial.WritesStarted+16 {
		t.Fatal("concurrent writes lost accounting")
	}
	completeLegacyIOReplay(t, s)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	report, _ := readLegacyIOReport(t, s)
	if report.Status != "complete" || report.FileIO.WritesStarted != report.FileIO.WritesReturned {
		t.Fatal("concurrent write accounting incomplete")
	}
}

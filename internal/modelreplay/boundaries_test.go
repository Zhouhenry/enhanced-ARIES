package modelreplay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/performance"
)

func readBoundaries(t *testing.T, s *Server) ([]boundaryEvent, boundaryIndex) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(s.options.OutputDir, "request-boundaries-index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var index boundaryIndex
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join(s.options.OutputDir, "request-boundaries.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{testToken, testUpstreamKey, "private test system", "read, modify, then pytest", "result-0"} {
		if bytes.Contains(data, []byte(private)) {
			t.Fatal("private model content in boundary evidence")
		}
	}
	var events []boundaryEvent
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var event boundaryEvent
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	return events, index
}

func assertBoundaryRequests(t *testing.T, s *Server, count uint64) map[string][]boundaryEvent {
	t.Helper()
	events, index := readBoundaries(t, s)
	if err := s.ObservationError(); err != nil {
		t.Fatal(err)
	}
	if index.Status != "complete" || index.RequestsReceived != count || index.RequestsFinished != count ||
		index.Attempted != uint64(len(events)) || index.Written != index.Attempted || index.Dropped != 0 || index.WriteFailed != 0 {
		t.Fatalf("inconsistent request boundary index: %+v", index)
	}
	requests := make(map[string][]boundaryEvent)
	for i, event := range events {
		if event.SchemaVersion != boundarySchemaVersion || event.Sequence != uint64(i+1) ||
			event.Identity != s.options.Identity || event.RequestID == "" || event.ClockDomain != index.Clock.ClockDomain ||
			event.EmitterID != index.EmitterID || event.Producer != "model_proxy" || event.Scope == "" {
			t.Fatal("invalid request event identity or measurement scope")
		}
		requests[event.RequestID] = append(requests[event.RequestID], event)
	}
	if uint64(len(requests)) != count {
		t.Fatal("request instances were merged")
	}
	for _, request := range requests {
		first, last := request[0], request[len(request)-1]
		if first.Event != "http_request_received" || last.Event != "http_request_finished" || last.DurationNS == nil ||
			*last.DurationNS != last.OffsetNS-first.OffsetNS || *last.DurationNS < 0 {
			t.Fatal("handler duration does not use its actual entry/terminal boundaries")
		}
	}
	return requests
}

func TestRequestBoundariesIncludeRejectedRequestsAndShareRecorderClock(t *testing.T) {
	root := t.TempDir()
	recorder, err := performance.Open(root, "test-run")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = recorder.Close() })
	clock := performance.ClockFromContext(performance.WithRecorder(context.Background(), recorder))
	s, err := New(Options{Mode: "replay", Identity: testIdentity("clock"), AccessToken: []byte(testToken),
		Tape: fixtureTape(t), Timing: Timing{Mode: "zero"}, OutputDir: filepath.Join(root, "model"), Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	unauthorized := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(testUpstreamKey))
	w := httptest.NewRecorder()
	s.ServeHTTP(w, unauthorized)
	if w.Code != http.StatusUnauthorized {
		t.Fatal("authentication behavior changed")
	}
	models := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	models.Header.Set("Authorization", "Bearer "+testToken)
	s.ServeHTTP(httptest.NewRecorder(), models)
	for turn := 0; turn < 3; turn++ {
		if w := post(s, requestAt(turn)); w.Code != 200 || !bytes.Equal(w.Body.Bytes(), bytes.Join(framesAt(turn), nil)) {
			t.Fatal("observations changed replay output")
		}
		if turn == 0 && post(s, requestAt(turn)).Code != 200 {
			t.Fatal("cached retry failed")
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	requests := assertBoundaryRequests(t, s, 6)
	_, index := readBoundaries(t, s)
	if index.Clock.EmitterID != clock.EmitterID || index.Clock.ClockDomain != clock.ClockDomain ||
		!index.Clock.StartedAtUTC.Equal(clock.StartedAtUTC) || index.SessionStartedOffsetNS != clock.Offset(s.started) || index.EmitterID == clock.EmitterID {
		t.Fatal("model emitter identity or shared clock mapping is incorrect")
	}
	retries, unauthorizedCount := 0, 0
	for _, request := range requests {
		last := request[len(request)-1]
		if last.Reason == "UNAUTHORIZED" {
			unauthorizedCount++
			if len(request) != 2 || last.Turn != nil {
				t.Fatal("rejected request fabricated admission/turn")
			}
		}
		if last.CachedRetry {
			retries++
		}
		for _, event := range request {
			if strings.HasPrefix(event.Event, "upstream_") {
				t.Fatal("replay fabricated an upstream observation")
			}
		}
	}
	if retries != 1 || unauthorizedCount != 1 {
		t.Fatal("retry/auth outcomes missing")
	}
	for _, event := range readObservations(t, s) {
		if event.Event != "session_started" && event.Event != "session_finished" {
			if _, ok := requests[event.RequestID]; !ok {
				t.Fatal("legacy event lacks its exact HTTP request identity")
			}
		}
	}
}

func TestRecordBoundariesKeepReceiptAndDeliverySeparate(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		turn := int(calls.Add(1)) - 1
		w.Header().Set("Content-Type", "text/event-stream")
		for _, data := range observationFrames(turn) {
			_, _ = w.Write(data)
			w.(http.Flusher).Flush()
		}
	}))
	defer upstream.Close()
	s, err := New(Options{Mode: "record", Identity: testIdentity("record-boundaries"), Model: "test-model", AccessToken: []byte(testToken),
		UpstreamURL: upstream.URL, UpstreamKey: []byte(testUpstreamKey), Timing: Timing{Mode: "zero"}, OutputDir: filepath.Join(t.TempDir(), "model")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	for turn := 0; turn < 3; turn++ {
		if w := post(s, requestAt(turn)); w.Code != 200 || !bytes.Equal(w.Body.Bytes(), bytes.Join(observationFrames(turn), nil)) {
			t.Fatal("recording changed the real SSE stream")
		}
		if turn == 0 && post(s, requestAt(0)).Code != 200 {
			t.Fatal("retry failed")
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatal("observations added upstream calls")
	}
	requests := assertBoundaryRequests(t, s, 4)
	for _, request := range requests {
		last := request[len(request)-1]
		upstreamStarts, headers, receivedFirst, flushedFirst := 0, 0, 0, 0
		flushed := map[int]int64{}
		for _, event := range request {
			switch event.Event {
			case "upstream_request_started":
				upstreamStarts++
			case "upstream_response_headers":
				headers++
			case "upstream_first_sse":
				receivedFirst++
			case "frame_flushed":
				if event.Frame == nil || event.ActualNS == nil {
					t.Fatal("flush lacks frame identity/time")
				}
				flushed[*event.Frame] = event.OffsetNS
			case "downstream_first_sse":
				flushedFirst++
			}
			if strings.HasPrefix(event.Event, "downstream_first_") {
				if event.Frame == nil || flushed[*event.Frame] != event.OffsetNS {
					t.Fatal("first delivery did not use the actual flush boundary")
				}
			}
		}
		if flushedFirst != 1 {
			t.Fatal("first flushed SSE missing")
		}
		if last.CachedRetry {
			if upstreamStarts != 0 || headers != 0 || receivedFirst != 0 || last.Timing != "cached_retry_zero" {
				t.Fatal("retry fabricated upstream timing")
			}
		} else if upstreamStarts != 1 || headers != 1 || receivedFirst != 1 {
			t.Fatal("real upstream boundaries missing")
		}
	}
	if _, err := Load(filepath.Join(s.options.OutputDir, "tape.json")); err != nil {
		t.Fatal(err)
	}
}

func TestRequestBoundaryFailuresDoNotInventSuccessfulDelivery(t *testing.T) {
	t.Run("strict_history", func(t *testing.T) {
		s := replayServer(t, "history-boundaries", fixtureTape(t), Timing{Mode: "zero"})
		if post(s, requestAt(0)).Code != 200 {
			t.Fatal("first turn failed")
		}
		changed := bytes.Replace(requestAt(1), []byte("private test system"), []byte("changed earlier history"), 1)
		if post(s, changed).Code != 409 || post(s, requestAt(1)).Code != 409 {
			t.Fatal("strict rejection semantics changed")
		}
		if s.Close() == nil {
			t.Fatal("diverged session marked complete")
		}
		requests := assertBoundaryRequests(t, s, 3)
		diverged, sessionFailed := false, false
		for _, request := range requests {
			last := request[len(request)-1]
			if last.Reason == "REPLAY_DIVERGED" {
				diverged = true
				found := false
				for _, event := range request {
					if event.Event == "request_matched" && event.Status == "failed" {
						found = true
					}
					if event.Event == "frame_flushed" {
						t.Fatal("diverged request delivered response frames")
					}
				}
				if !found {
					t.Fatal("failed strict decision has no measured boundary")
				}
			}
			if last.Reason == "SESSION_FAILED" {
				sessionFailed = true
			}
		}
		if !diverged || !sessionFailed {
			t.Fatal("failed requests disappeared from observations")
		}
	})
	t.Run("flush", func(t *testing.T) {
		s := replayServer(t, "flush-boundaries", fixtureTape(t), Timing{Mode: "zero"})
		r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(requestAt(0)))
		r.Header.Set("Authorization", "Bearer "+testToken)
		s.ServeHTTP(&failedFlushWriter{httptest.NewRecorder()}, r)
		if s.Close() == nil {
			t.Fatal("failed flush marked complete")
		}
		requests := assertBoundaryRequests(t, s, 1)
		for _, request := range requests {
			failed := 0
			for _, event := range request {
				if event.Event == "frame_flush_failed" {
					failed++
				}
				if event.Event == "frame_flushed" || strings.HasPrefix(event.Event, "downstream_first_") {
					t.Fatal("failed flush marked delivered")
				}
			}
			if failed != 1 || request[len(request)-1].Status != "failed" {
				t.Fatal("flush failure not retained")
			}
		}
	})
}

func TestBoundaryWriteFailureDoesNotChangeReplayOutcome(t *testing.T) {
	s := replayServer(t, "observation-write-failure", fixtureTape(t), Timing{Mode: "zero"})
	if err := s.boundaries.file.Close(); err != nil {
		t.Fatal(err)
	}
	for turn := 0; turn < 3; turn++ {
		if w := post(s, requestAt(turn)); w.Code != 200 || !bytes.Equal(w.Body.Bytes(), bytes.Join(framesAt(turn), nil)) {
			t.Fatal("observation failure changed replay")
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal("boundary observation failure changed model session outcome", err)
	}
	if s.ObservationError() == nil {
		t.Fatal("boundary write failure hidden")
	}
	_, index := readBoundaries(t, s)
	if index.Status != "incomplete" || index.WriteFailed == 0 || index.Attempted != index.Written+index.Dropped+index.WriteFailed {
		t.Fatal("write failure missing from independent index")
	}
}

type boundaryReadFailure struct{}

func (boundaryReadFailure) Read([]byte) (int, error) {
	return 0, errors.New("private reader failure")
}

func (boundaryReadFailure) Close() error { return nil }

func TestRequestReadParseAndUpstreamFailuresRetainBoundaries(t *testing.T) {
	for _, failure := range []string{"body", "parse", "status", "truncated"} {
		t.Run(failure, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				if failure == "status" {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write(framesAt(0)[0])
				w.(http.Flusher).Flush()
				_, _ = w.Write([]byte("data: incomplete"))
			}))
			defer upstream.Close()
			s, err := New(Options{Mode: "record", Identity: testIdentity(failure), Model: "test-model", AccessToken: []byte(testToken),
				UpstreamURL: upstream.URL, UpstreamKey: []byte(testUpstreamKey), OutputDir: filepath.Join(t.TempDir(), "model")})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = s.Close() })
			r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(requestAt(0)))
			if failure == "body" {
				r.Body = boundaryReadFailure{}
			}
			if failure == "parse" {
				r = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("invalid JSON"))
			}
			r.Header.Set("Authorization", "Bearer "+testToken)
			s.ServeHTTP(httptest.NewRecorder(), r)
			if s.Close() == nil {
				t.Fatal("failed model request marked complete")
			}
			requests := assertBoundaryRequests(t, s, 1)
			for _, request := range requests {
				last := request[len(request)-1]
				if last.Status != "failed" && last.Status != "rejected" {
					t.Fatal("failed request outcome missing")
				}
				seen := map[string]boundaryEvent{}
				for _, event := range request {
					seen[event.Event] = event
				}
				switch failure {
				case "body":
					if seen["request_body_read"].Status != "failed" || calls.Load() != 0 {
						t.Fatal("body failure boundary incorrect")
					}
				case "parse":
					if seen["request_parsed"].Status != "failed" || calls.Load() != 0 {
						t.Fatal("parse failure boundary incorrect")
					}
				case "status":
					if seen["upstream_response_headers"].HTTPStatus != 503 || seen["frame_flushed"].Event != "" || calls.Load() != 1 {
						t.Fatal("upstream failure fabricated response frames")
					}
				case "truncated":
					if seen["frame_flushed"].Event == "" || seen["upstream_response_headers"].HTTPStatus != 200 || calls.Load() != 1 {
						t.Fatal("partial delivery evidence missing")
					}
				}
			}
		})
	}
}

func TestBoundaryQueueOverflowPreservesIndependentDenominator(t *testing.T) {
	root := t.TempDir()
	file, err := os.Create(filepath.Join(root, "request-boundaries.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	r := &boundaryRecorder{file: file, queue: make(chan boundaryEvent, 1), done: make(chan struct{}), anchor: time.Now(), directory: root,
		index: boundaryIndex{SchemaVersion: boundarySchemaVersion, EventsFile: "request-boundaries.jsonl"}}
	r.record(boundaryEvent{Event: "first"}, time.Now())
	r.record(boundaryEvent{Event: "dropped"}, time.Now())
	go r.write()
	if r.close() == nil {
		t.Fatal("overflow marked complete")
	}
	if r.index.Attempted != 2 || r.index.Written != 1 || r.index.Dropped != 1 || r.index.Status != "incomplete" {
		t.Fatal("drop denominator lost")
	}
}

func TestBoundaryCloseIsIdempotentAndBindsRawEventsHash(t *testing.T) {
	root := t.TempDir()
	file, err := os.Create(filepath.Join(root, "request-boundaries.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	r := &boundaryRecorder{file: file, queue: make(chan boundaryEvent, 4), done: make(chan struct{}), anchor: time.Now(), directory: root,
		index: boundaryIndex{SchemaVersion: boundarySchemaVersion, EventsFile: "request-boundaries.jsonl"}}
	r.record(boundaryEvent{Event: "first"}, time.Now())
	go r.write()
	first := r.close()
	second := r.close()
	if first != nil || second != nil {
		t.Fatalf("idempotent close failed: first=%v second=%v", first, second)
	}
	if r.index.FlushStatus != "queue_drained_sync_close" || len(r.index.EventsSHA256) != 64 || r.index.EventsBytes <= 0 || r.index.FinalizedAtUTC.IsZero() {
		t.Fatalf("raw flush evidence missing: %+v", r.index)
	}
	if r.index.ProducerState != "admission_closed" || r.index.QueueState != "drained" || r.index.WriterState != "drained" ||
		r.index.DurabilityState != "events_sync_ok" {
		t.Fatalf("lifecycle evidence missing: %+v", r.index)
	}
	if got, n, err := hashFile(filepath.Join(root, "request-boundaries.jsonl")); err != nil || got != r.index.EventsSHA256 || n != r.index.EventsBytes {
		t.Fatalf("raw hash mismatch: got=%s/%d err=%v index=%s/%d", got, n, err, r.index.EventsSHA256, r.index.EventsBytes)
	}
}

func TestBoundaryIndexPublicationFailureIsNotClaimedComplete(t *testing.T) {
	root := t.TempDir()
	file, err := os.Create(filepath.Join(root, "request-boundaries.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	r := &boundaryRecorder{file: file, queue: make(chan boundaryEvent, 2), done: make(chan struct{}), anchor: time.Now(), directory: root,
		index: boundaryIndex{SchemaVersion: boundarySchemaVersion, EventsFile: "request-boundaries.jsonl"}}
	r.record(boundaryEvent{Event: "first"}, time.Now())
	go r.write()
	indexPath := filepath.Join(root, "request-boundaries-index.json")
	if err := os.WriteFile(indexPath, []byte("existing-index"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := r.close(); err == nil {
		t.Fatal("index publication failure hidden")
	}
	data, err := os.ReadFile(indexPath)
	if err != nil || string(data) != "existing-index" {
		t.Fatalf("existing index was overwritten: %q, %v", data, err)
	}
}

func TestModelBoundaryLateProducerInvalidatesWithoutRewritingIndex(t *testing.T) {
	s := replayServer(t, "late-boundaries", fixtureTape(t), Timing{Mode: "zero"})
	for turn := 0; turn < 3; turn++ {
		if post(s, requestAt(turn)).Code != 200 {
			t.Fatal("replay failed")
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.options.OutputDir, "request-boundaries-index.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if post(s, requestAt(0)).Code != http.StatusConflict {
		t.Fatal("closed session accepted request")
	}
	if s.ObservationError() == nil {
		t.Fatal("late producer was hidden")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("finalized index was overwritten")
	}
	data, err := os.ReadFile(filepath.Join(s.options.OutputDir, "request-boundaries-late-after-close.json"))
	if err != nil {
		t.Fatal(err)
	}
	var invalidation struct {
		Status   string   `json:"status"`
		Identity Identity `json:"identity"`
	}
	if json.Unmarshal(data, &invalidation) != nil || invalidation.Status != "incomplete" || invalidation.Identity != s.options.Identity {
		t.Fatal("late-producer invalidation lacks its session identity")
	}
}

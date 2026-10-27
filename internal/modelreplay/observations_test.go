package modelreplay

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func observationFrames(turn int) [][]byte {
	if turn == 0 {
		return [][]byte{
			[]byte(": heartbeat\n\n"),
			frame(map[string]any{"role": "assistant", "content": ""}, nil),
			frame(map[string]any{"reasoning_content": "reasoning"}, nil),
			frame(map[string]any{"tool_calls": []any{map[string]any{"index": 0, "type": "function", "function": map[string]any{"arguments": ""}}}}, nil),
			frame(map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "call_0", "function": map[string]any{"name": "exec", "arguments": "{\"command\":"}}}}, nil),
			frame(map[string]any{"tool_calls": []any{map[string]any{"index": 0, "function": map[string]any{"arguments": "\"step0\"}"}}}}, nil),
			frame(map[string]any{}, "tool_calls"),
			[]byte("data: [DONE]\n\n"),
		}
	}
	if turn == 2 {
		return append([][]byte{frame(map[string]any{"role": "assistant", "content": ""}, nil)}, framesAt(turn)...)
	}
	return framesAt(turn)
}

func readObservations(t *testing.T, s *Server) []observation {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(s.options.OutputDir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var events []observation
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
		var e observation
		if err := json.Unmarshal(line, &e); err != nil {
			t.Fatal(err)
		}
		events = append(events, e)
	}
	return events
}

func assertFirstOutputs(t *testing.T, events []observation, tape *Tape, timing Timing, cachedRetry bool) {
	t.Helper()
	expected := []map[string]int{
		{"first_sse": 0, "first_reasoning": 2, "first_tool_call_delta": 4},
		{"first_sse": 0, "first_reasoning": 0, "first_tool_call_delta": 0},
		{"first_sse": 0, "first_content": 1},
	}
	type outputKey struct {
		turn  int
		event string
	}
	seen := make(map[outputKey]bool)
	count := 0
	for _, e := range events {
		if !strings.HasPrefix(e.Event, "first_") || strings.HasPrefix(e.Timing, "cached_retry_") != cachedRetry {
			continue
		}
		turn := e.Turn
		if cachedRetry {
			if e.Turn != 1 {
				t.Fatal("retry must retain its current request position")
			}
			turn = 0 // The immediately preceding response is delivered again.
		}
		if turn < 0 || turn >= len(expected) {
			t.Fatal("unexpected turn")
		}
		wantFrame, ok := expected[turn][e.Event]
		key := outputKey{turn, e.Event}
		if !ok || seen[key] || e.Frame != wantFrame {
			t.Fatalf("unexpected or duplicate first output: turn=%d event=%s frame=%d", e.Turn, e.Event, e.Frame)
		}
		seen[key] = true
		count++
		if e.HeapBytes != 0 || e.PeakRSSBytes != 0 || e.Goroutines != 0 {
			t.Fatal("first-output events must not add process resource sampling")
		}
		if e.Timing == "observed_upstream" {
			if e.ReceivedNS == nil || *e.ReceivedNS != tape.Turns[turn].Frames[e.Frame].OffsetNS || e.ActualNS != nil || e.TargetNS != nil {
				t.Fatal("upstream receipt confused with downstream delivery or replay target")
			}
			continue
		}
		wantTiming := timing.Mode
		if cachedRetry {
			wantTiming = "cached_retry_" + wantTiming
		}
		if e.Timing != wantTiming {
			t.Fatal("incorrect timing source")
		}
		wantTarget := int64(0)
		switch timing.Mode {
		case "recorded":
			wantTarget = tape.Turns[turn].Frames[e.Frame].OffsetNS
		case "synthetic":
			wantTarget = int64(timing.InitialDelay + time.Duration(e.Frame)*timing.FrameInterval)
		}
		if e.ReceivedNS != nil || e.TargetNS == nil || *e.TargetNS != wantTarget || e.ActualNS == nil || *e.ActualNS < wantTarget {
			t.Fatal("replay first output lacks its actual flush time or has incorrect target")
		}
	}
	wantCount := 8
	if cachedRetry {
		wantCount = 3
	}
	if count != wantCount {
		t.Fatalf("first output count=%d, want %d", count, wantCount)
	}
}

func TestRecordFirstOutputsSeparateReceiptAndCachedDelivery(t *testing.T) {
	for _, timing := range []Timing{{Mode: "zero"}, {Mode: "recorded"}} {
		t.Run(timing.Mode, func(t *testing.T) { checkRecordFirstOutputs(t, timing) })
	}
}

func checkRecordFirstOutputs(t *testing.T, timing Timing) {
	t.Helper()
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		turn := int(calls.Add(1)) - 1
		w.Header().Set("Content-Type", "text/event-stream")
		for _, data := range observationFrames(turn) {
			_, _ = w.Write(data)
			w.(http.Flusher).Flush()
		}
	}))
	defer upstream.Close()
	s, err := New(Options{Mode: "record", Identity: testIdentity("first-outputs"), Model: "test-model", AccessToken: []byte(testToken), UpstreamURL: upstream.URL, UpstreamKey: []byte(testUpstreamKey), OutputDir: filepath.Join(t.TempDir(), "record"), Timing: timing})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	for turn := 0; turn < 3; turn++ {
		if w := post(s, requestAt(turn)); w.Code != http.StatusOK {
			t.Fatal(w.Body.String())
		}
		if turn == 0 {
			if w := post(s, requestAt(turn)); w.Code != http.StatusOK {
				t.Fatal(w.Body.String())
			}
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatal("cached retry contacted upstream")
	}
	tape, err := Load(filepath.Join(s.options.OutputDir, "tape.json"))
	if err != nil {
		t.Fatal(err)
	}
	events := readObservations(t, s)
	assertFirstOutputs(t, events, tape, timing, false)
	assertFirstOutputs(t, events, tape, timing, true)
	for _, e := range events {
		if e.Event == "frame_sent" && e.Timing == "observed_upstream" {
			if e.ReceivedNS == nil || e.ActualNS == nil || e.TargetNS != nil || *e.ActualNS < *e.ReceivedNS {
				t.Fatal("record frame receipt is not a scheduled replay target")
			}
		}
	}
}

func TestReplayFirstOutputsUseActualFlushAndExplicitTargets(t *testing.T) {
	tape := fixtureTape(t)
	for i := range tape.Turns {
		turn := &tape.Turns[i]
		turn.Frames = nil
		for j, data := range observationFrames(i) {
			turn.Frames = append(turn.Frames, Frame{OffsetNS: int64(j+1) * int64(time.Millisecond), Data: data})
		}
		turn.SHA256 = turnDigest(*turn)
	}
	tape.SHA256 = tapeDigest(*tape)
	for _, timing := range []Timing{{Mode: "zero"}, {Mode: "recorded"}, {Mode: "synthetic", InitialDelay: time.Millisecond, FrameInterval: time.Millisecond}} {
		t.Run(timing.Mode, func(t *testing.T) {
			s := replayServer(t, "first-"+timing.Mode, tape, timing)
			for turn := 0; turn < 3; turn++ {
				if w := post(s, requestAt(turn)); w.Code != http.StatusOK {
					t.Fatal(w.Body.String())
				}
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			assertFirstOutputs(t, readObservations(t, s), tape, timing, false)
		})
	}
}

type failedFlushWriter struct{ *httptest.ResponseRecorder }

func (*failedFlushWriter) FlushError() error { return io.ErrClosedPipe }

func TestReplayDoesNotClaimFirstOutputWhenFlushFails(t *testing.T) {
	s := replayServer(t, "failed-flush", fixtureTape(t), Timing{Mode: "zero"})
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(requestAt(0)))
	r.Header.Set("Authorization", "Bearer "+testToken)
	s.ServeHTTP(&failedFlushWriter{httptest.NewRecorder()}, r)
	if s.Close() == nil {
		t.Fatal("failed delivery claimed complete")
	}
	for _, e := range readObservations(t, s) {
		if strings.HasPrefix(e.Event, "first_") || e.Event == "frame_sent" {
			t.Fatal("failed flush claimed downstream output")
		}
	}
}

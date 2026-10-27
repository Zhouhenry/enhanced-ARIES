package modelreplay

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hyscale-lab/aries/pkg/performance"
)

func TestRecordLoadReplayBeyondLegacyFrameCapacity(t *testing.T) {
	var bodies [3][]byte
	for i := range bodies {
		// Two tool turns cross the old session-wide 65,536-frame limit while
		// staying far below both the old and expanded byte limits.
		if i < 2 {
			bodies[i] = bytes.Repeat([]byte(":\n\n"), 32768)
		}
		bodies[i] = append(bodies[i], bytes.Join(framesAt(i), nil)...)
	}
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(calls.Add(1)) - 1
		if i >= len(bodies) {
			t.Error("unexpected upstream request")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		if !bytes.Equal(raw, requestAt(i)) {
			t.Error("recording changed the request")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write(bodies[i])
	}))
	defer upstream.Close()
	dir := filepath.Join(t.TempDir(), "record")
	s, err := New(Options{Mode: "record", Identity: testIdentity("capacity-record"), Model: "test-model",
		AccessToken: []byte(testToken), UpstreamURL: upstream.URL, UpstreamKey: []byte(testUpstreamKey),
		OutputDir: dir, ObservationLevel: performance.ObservationL0})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	for i, body := range bodies {
		if response := post(s, requestAt(i)); response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), body) {
			t.Fatalf("record turn %d lost or changed SSE frames", i)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	tape, err := Load(filepath.Join(dir, "tape.json"))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for i, turn := range tape.Turns {
		var body []byte
		for _, frame := range turn.Frames {
			body = append(body, frame.Data...)
		}
		if !bytes.Equal(body, bodies[i]) {
			t.Fatalf("persisted turn %d changed SSE bytes", i)
		}
		count += len(turn.Frames)
	}
	if count != 65536+len(framesAt(0))+len(framesAt(1))+len(framesAt(2)) || calls.Load() != 3 {
		t.Fatalf("unexpected recorded denominator: %d frames, %d upstream calls", count, calls.Load())
	}
	for _, timing := range []string{"zero", "recorded", "synthetic"} {
		t.Run(timing, func(t *testing.T) {
			r, err := New(Options{Mode: "replay", Identity: testIdentity("capacity-" + timing),
				AccessToken: []byte(testToken), Tape: tape, OutputDir: filepath.Join(t.TempDir(), "replay"),
				Timing: Timing{Mode: timing}, ObservationLevel: performance.ObservationL0})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = r.Close() })
			for i, body := range bodies {
				if response := post(r, requestAt(i)); response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), body) {
					t.Fatalf("replay turn %d lost or changed SSE frames", i)
				}
			}
			if r.ReplayPolicy() != ReplayPolicyStrict || r.Close() != nil {
				t.Fatal("strict replay did not complete")
			}
		})
	}
}

func TestRecordCapacityBoundaries(t *testing.T) {
	body := bytes.Join(framesAt(2), nil)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write(body)
	}))
	defer upstream.Close()
	for _, tc := range []struct {
		name          string
		bytes, frames int
		complete      bool
	}{
		{"exact-frame-capacity", 0, maxFrames - len(framesAt(2)), true},
		{"one-frame-over", 0, maxFrames - len(framesAt(2)) + 1, false},
		{"exact-byte-capacity", maxTape - len(requestAt(0)) - len(body), 0, true},
		{"one-byte-over", maxTape - len(requestAt(0)) - len(body) + 1, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := New(Options{Mode: "record", Identity: testIdentity(tc.name), Model: "test-model",
				AccessToken: []byte(testToken), UpstreamURL: upstream.URL, UpstreamKey: []byte(testUpstreamKey),
				OutputDir: filepath.Join(t.TempDir(), "record"), ObservationLevel: performance.ObservationL0})
			if err != nil {
				t.Fatal(err)
			}
			// Seed prior-turn accounting to exercise exact session budget edges
			// without allocating a 256 MiB fixture or two million frame objects.
			s.bytes, s.frames = tc.bytes, tc.frames
			response := post(s, requestAt(0))
			if tc.complete {
				if !bytes.Equal(response.Body.Bytes(), body) || s.Close() != nil {
					t.Fatal("exact capacity was rejected")
				}
			} else if s.Close() == nil || s.failed != "DELIVERY_OR_UPSTREAM_FAILED" || bytes.Equal(response.Body.Bytes(), body) {
				t.Fatal("capacity overflow was hidden")
			}
		})
	}
}

func TestFrameCapacityDoesNotWidenToolIndices(t *testing.T) {
	for _, index := range []int{65535, 65536} {
		var response streamResult
		err := response.feed(frame(map[string]any{"tool_calls": []any{map[string]any{"index": index}}}, nil))
		if index == 65535 && err != nil || index == 65536 && (err == nil || !strings.Contains(err.Error(), "tool index")) {
			t.Fatalf("tool index boundary changed at %d: %v", index, err)
		}
	}
}

func TestRecordLoadExpandedRequestAndFrame(t *testing.T) {
	raw := bytes.Replace(requestAt(0), []byte("read, modify, then pytest"), bytes.Repeat([]byte("x"), (16<<20)+1), 1)
	largeFrame := append([]byte(":"), bytes.Repeat([]byte("x"), maxFrame-3)...)
	largeFrame = append(largeFrame, '\n', '\n')
	body := append(bytes.Clone(largeFrame), bytes.Join(framesAt(2), nil)...)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ := io.ReadAll(r.Body)
		if !bytes.Equal(got, raw) {
			t.Error("large request changed before upstream")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write(body)
	}))
	defer upstream.Close()
	dir := filepath.Join(t.TempDir(), "record")
	s, err := New(Options{Mode: "record", Identity: testIdentity("large-request-frame"), Model: "test-model",
		AccessToken: []byte(testToken), UpstreamURL: upstream.URL, UpstreamKey: []byte(testUpstreamKey),
		OutputDir: dir, ObservationLevel: performance.ObservationL0})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if response := post(s, raw); response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), body) {
		t.Fatal("expanded request or SSE frame was truncated/rejected")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	tape, err := Load(filepath.Join(dir, "tape.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(tape.Turns) != 1 || len(tape.Turns[0].Frames) == 0 || !bytes.Equal(tape.Turns[0].Frames[0].Data, largeFrame) {
		t.Fatal("expanded SSE frame did not survive tape persistence and validation")
	}
	// Tape JSON is indented on disk; only its structural whitespace may differ.
	var compact bytes.Buffer
	if err := json.Compact(&compact, tape.Turns[0].Request); err != nil || !bytes.Equal(compact.Bytes(), raw) {
		t.Fatal("expanded request content changed during tape persistence")
	}
	replay, err := New(Options{Mode: "replay", Identity: testIdentity("large-request-frame-replay"),
		AccessToken: []byte(testToken), Tape: tape, OutputDir: filepath.Join(t.TempDir(), "replay"),
		Timing: Timing{Mode: "zero"}, ObservationLevel: performance.ObservationL0})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = replay.Close() })
	if response := post(replay, raw); response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), body) {
		t.Fatal("expanded persisted request/frame failed strict replay")
	}
	if replay.ReplayPolicy() != ReplayPolicyStrict || replay.Close() != nil {
		t.Fatal("expanded strict replay did not complete")
	}
}

func TestSSEByteCapacityBoundaries(t *testing.T) {
	frame := append([]byte(":"), bytes.Repeat([]byte("x"), maxFrame-3)...)
	frame = append(frame, '\n', '\n')
	for _, tc := range []struct {
		name          string
		frames, extra int
		wantBytes     int
		ok            bool
	}{
		{"exact-frame", 1, 0, maxFrame, true},
		{"frame-over", 0, maxFrame + 1, 0, false},
		{"exact-response", maxBody / maxFrame, 0, maxBody, true},
		{"response-over", maxBody / maxFrame, 1, maxBody, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var readers []io.Reader
			for i := 0; i < tc.frames; i++ {
				readers = append(readers, bytes.NewReader(frame))
			}
			if tc.extra > 0 {
				readers = append(readers, strings.NewReader(strings.Repeat("x", tc.extra)))
			}
			received := 0
			err := readFrames(io.MultiReader(readers...), func(data []byte) error {
				if !bytes.Equal(data, frame) {
					t.Fatal("read buffer fragmented or changed an SSE event")
				}
				received += len(data)
				return nil
			})
			if (err == nil) != tc.ok || received != tc.wantBytes {
				t.Fatalf("received %d bytes, err=%v", received, err)
			}
			if !tc.ok && err.Error() != "response/frame exceeds bound" {
				t.Fatalf("wrong capacity failure: %v", err)
			}
		})
	}
}

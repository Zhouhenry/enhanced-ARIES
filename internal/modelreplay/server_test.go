package modelreplay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
)

const testToken = "test-session-access-token"
const testUpstreamKey = "test-upstream-secret-key"

func testIdentity(id string) Identity {
	return Identity{RunID: "test-run", AttemptID: "attempt-" + id, SessionID: id, Placement: "external", HarnessVersion: "openclaw-test-pin"}
}

func requestAt(turn int) []byte {
	ms := []any{map[string]any{"role": "system", "content": "private test system"}, map[string]any{"role": "user", "content": "read, modify, then pytest"}}
	for i := 0; i < turn; i++ {
		args := fmt.Sprintf(`{"command":"step%d"}`, i)
		ms = append(ms, map[string]any{"role": "assistant", "content": nil, "reasoning_content": "reasoning", "tool_calls": []any{map[string]any{"id": fmt.Sprintf("call_%d", i), "type": "function", "function": map[string]any{"name": "exec", "arguments": args}}}}, map[string]any{"role": "tool", "tool_call_id": fmt.Sprintf("call_%d", i), "content": fmt.Sprintf("result-%d", i)})
	}
	b, _ := json.Marshal(map[string]any{"model": "test-model", "stream": true, "temperature": 0.2, "messages": ms, "tools": []any{map[string]any{"type": "function", "function": map[string]any{"name": "exec", "parameters": map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string"}}}}}}})
	return b
}

func frame(delta any, finish any) []byte {
	b, _ := json.Marshal(map[string]any{"id": "completion", "object": "chat.completion.chunk", "model": "test-model", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
	return append(append([]byte("data: "), b...), []byte("\r\n\r\n")...)
}

func framesAt(turn int) [][]byte {
	var frames [][]byte
	if turn < 2 {
		// Arguments and the UTF-8 text are split across transport writes below.
		frames = append(frames, frame(map[string]any{"role": "assistant", "reasoning_content": "reasoning", "tool_calls": []any{map[string]any{"index": 0, "id": fmt.Sprintf("call_%d", turn), "type": "function", "function": map[string]any{"name": "exec", "arguments": "{\"command\":"}}}}, nil))
		frames = append(frames, frame(map[string]any{"tool_calls": []any{map[string]any{"index": 0, "function": map[string]any{"arguments": fmt.Sprintf("\"step%d\"}", turn)}}}}, nil))
		frames = append(frames, frame(map[string]any{}, "tool_calls"))
	} else {
		frames = append(frames, frame(map[string]any{"role": "assistant", "content": "完成"}, nil), frame(map[string]any{}, "stop"))
	}
	return append(frames, []byte("data: [DONE]\r\n\r\n"))
}

func post(s *Server, raw []byte) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(raw))
	r.Header.Set("Authorization", "Bearer "+testToken)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func fixtureTape(t *testing.T) *Tape {
	t.Helper()
	tape := Tape{Version: 1, Identity: testIdentity("recorded"), Model: "test-model", StartedAt: time.Unix(1, 0), UpstreamHost: "test.invalid", Complete: true}
	for i := 0; i < 3; i++ {
		turn := Turn{Request: requestAt(i), StatusCode: 200}
		var response streamResult
		for j, data := range framesAt(i) {
			if err := response.feed(data); err != nil {
				t.Fatal(err)
			}
			turn.Frames = append(turn.Frames, Frame{OffsetNS: int64(j+1) * int64(time.Millisecond), Data: data})
		}
		if err := response.complete(); err != nil {
			t.Fatal(err)
		}
		turn.Finish = response.finish
		if calls := response.calls(); len(calls) > 0 {
			turn.Call = &calls[0]
		}
		turn.SHA256 = turnDigest(turn)
		tape.Turns = append(tape.Turns, turn)
	}
	tape.SHA256 = tapeDigest(tape)
	if err := validateTape(tape); err != nil {
		t.Fatal(err)
	}
	return &tape
}

func replayServer(t *testing.T, id string, tape *Tape, timing Timing) *Server {
	t.Helper()
	s, err := New(Options{Mode: "replay", Identity: testIdentity(id), AccessToken: []byte(testToken), Tape: tape, OutputDir: filepath.Join(t.TempDir(), "session"), Timing: timing})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestRecordRealHTTPBoundaryAndReplay(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" || r.Header.Get("Authorization") != "Bearer "+testUpstreamKey {
			t.Error("upstream path/auth mismatch")
		}
		i := int(calls.Add(1)) - 1
		raw, _ := io.ReadAll(r.Body)
		if !bytes.Equal(raw, requestAt(i)) {
			t.Error("upstream request was changed")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, data := range framesAt(i) {
			for _, b := range data {
				_, _ = w.Write([]byte{b})
			}
			w.(http.Flusher).Flush()
		}
	}))
	defer upstream.Close()
	dir := filepath.Join(t.TempDir(), "record")
	s, err := New(Options{Mode: "record", Identity: testIdentity("record"), Model: "test-model", AccessToken: []byte(testToken), UpstreamURL: upstream.URL, UpstreamKey: []byte(testUpstreamKey), OutputDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		w := post(s, requestAt(i))
		if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), bytes.Join(framesAt(i), nil)) {
			t.Fatalf("turn %d: status=%d body=%s", i, w.Code, w.Body)
		}
		if i == 0 {
			if w := post(s, requestAt(i)); w.Code != 200 {
				t.Fatal(w.Code)
			}
		}
	}
	if calls.Load() != 3 {
		t.Fatalf("retry called upstream: %d", calls.Load())
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	tape, err := Load(filepath.Join(dir, "tape.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"tape.json", "events.jsonl"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte(testToken)) || bytes.Contains(data, []byte(testUpstreamKey)) {
			t.Fatal("credential persisted")
		}
		info, _ := os.Stat(filepath.Join(dir, name))
		if info.Mode().Perm() != 0600 {
			t.Fatal("artifact permissions")
		}
	}
	r := replayServer(t, "replay", tape, Timing{Mode: "zero"})
	// Mutating the caller's tape cannot mutate the loaded immutable session.
	tape.Turns[0].Frames[0].Data[0] = 'X'
	for i := 0; i < 3; i++ {
		w := post(r, requestAt(i))
		if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), bytes.Join(framesAt(i), nil)) {
			t.Fatalf("replay turn %d status %d", i, w.Code)
		}
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestStrictReplayRejectsEntireHistoryAndSchema(t *testing.T) {
	for _, field := range []string{"old_tool_result", "old_assistant_arguments", "system", "schema", "parameter", "model", "stream"} {
		t.Run(field, func(t *testing.T) {
			s := replayServer(t, field, fixtureTape(t), Timing{Mode: "zero"})
			for i := 0; i < 2; i++ {
				if w := post(s, requestAt(i)); w.Code != 200 {
					t.Fatal(w.Body.String())
				}
			}
			v, _ := decode(requestAt(2))
			req := v.(map[string]any)
			ms := req["messages"].([]any)
			switch field {
			case "old_tool_result":
				ms[3].(map[string]any)["content"] = "changed-private-output"
			case "old_assistant_arguments":
				ms[2].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)["function"].(map[string]any)["arguments"] = `{"command":"changed"}`
			case "system":
				ms[0].(map[string]any)["content"] = "changed-system"
			case "schema":
				req["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)["parameters"] = map[string]any{"type": "string"}
			case "parameter":
				req["temperature"] = 1
			case "model":
				req["model"] = "changed-model"
			case "stream":
				req["stream"] = false
			}
			raw, _ := json.Marshal(req)
			w := post(s, raw)
			if w.Code != 409 || !strings.Contains(w.Body.String(), "REPLAY_DIVERGED") {
				t.Fatalf("not rejected: %d %s", w.Code, w.Body)
			}
			if strings.Contains(w.Body.String(), "changed-") {
				t.Fatal("private payload leaked")
			}
			if post(s, requestAt(2)).Code != 409 {
				t.Fatal("diverged session recovered silently")
			}
			if s.Close() == nil {
				t.Fatal("diverged session declared complete")
			}
		})
	}
}

func TestConcurrentSessionsAndRetriesAreIsolated(t *testing.T) {
	tape := fixtureTape(t)
	a := replayServer(t, "a", tape, Timing{Mode: "zero"})
	b := replayServer(t, "b", tape, Timing{Mode: "recorded"})
	var wg sync.WaitGroup
	for _, s := range []*Server{a, b} {
		wg.Add(1)
		go func(s *Server) {
			defer wg.Done()
			for i := 0; i < 3; i++ {
				for attempt := 0; attempt < 2; attempt++ {
					w := post(s, requestAt(i))
					if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), bytes.Join(framesAt(i), nil)) {
						t.Errorf("session replay failed at %d: %d", i, w.Code)
					}
				}
			}
		}(s)
	}
	wg.Wait()
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	for _, s := range []*Server{a, b} {
		requests := assertBoundaryRequests(t, s, 6)
		for id := range requests {
			other := a
			if s == a {
				other = b
			}
			if strings.HasPrefix(id, other.boundaries.index.EmitterID+"/") {
				t.Fatal("model request identity crossed sessions")
			}
		}
		data, _ := os.ReadFile(filepath.Join(s.options.OutputDir, "events.jsonl"))
		other := "a"
		if s.options.Identity.SessionID == "a" {
			other = "b"
		}
		if bytes.Contains(data, []byte(`"session_id":"`+other+`"`)) {
			t.Fatal("session identities mixed")
		}
	}
}

func TestCancelAndBusyFailClosed(t *testing.T) {
	s := replayServer(t, "cancel", fixtureTape(t), Timing{Mode: "synthetic", InitialDelay: time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(requestAt(0))).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer "+testToken)
	done := make(chan struct{})
	w := httptest.NewRecorder()
	go func() { s.ServeHTTP(w, r); close(done) }()
	deadline := time.Now().Add(time.Second)
	for {
		s.mu.Lock()
		busy := s.busy
		s.mu.Unlock()
		if busy {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("request not admitted")
		}
		time.Sleep(time.Millisecond)
	}
	if w := post(s, requestAt(0)); w.Code != 409 || !strings.Contains(w.Body.String(), "SESSION_BUSY") {
		t.Fatalf("busy: %d %s", w.Code, w.Body)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancellation did not stop replay")
	}
	if w.Body.Len() != 0 {
		t.Fatal("canceled delayed response sent data")
	}
	if s.Close() == nil {
		t.Fatal("cancel declared complete")
	}
	requests := assertBoundaryRequests(t, s, 2)
	busy, canceled := false, false
	for _, request := range requests {
		last := request[len(request)-1]
		if last.Reason == "SESSION_BUSY" {
			busy = true
			if last.Turn != nil || len(request) != 2 {
				t.Fatal("busy request fabricated admission")
			}
		}
		if last.Reason == "CANCELED_OR_TIMEOUT" {
			canceled = true
		}
	}
	if !busy || !canceled {
		t.Fatal("busy/canceled HTTP attempts missing from boundary denominator")
	}
}

func TestTimingEvidenceAndDuplicateJSON(t *testing.T) {
	for _, mode := range []string{"zero", "recorded", "synthetic"} {
		t.Run(mode, func(t *testing.T) {
			timing := Timing{Mode: mode}
			if mode == "synthetic" {
				timing.InitialDelay = 2 * time.Millisecond
				timing.FrameInterval = time.Millisecond
			}
			s := replayServer(t, mode, fixtureTape(t), timing)
			for i := 0; i < 3; i++ {
				if w := post(s, requestAt(i)); w.Code != 200 {
					t.Fatal(w.Code)
				}
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			data, _ := os.ReadFile(filepath.Join(s.options.OutputDir, "events.jsonl"))
			for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
				var e observation
				if json.Unmarshal(line, &e) != nil {
					t.Fatal("invalid observation")
				}
				if e.Event == "frame_sent" {
					if e.TargetNS == nil || e.ActualNS == nil || *e.ActualNS < *e.TargetNS {
						t.Fatal("frame emitted before target")
					}
					if mode == "zero" && *e.TargetNS != 0 {
						t.Fatal("zero added wait was delayed")
					}
				}
			}
		})
	}
	s := replayServer(t, "duplicate", fixtureTape(t), Timing{Mode: "zero"})
	raw := bytes.Replace(requestAt(0), []byte(`"stream":true`), []byte(`"stream":false,"stream":true`), 1)
	if post(s, raw).Code != 409 {
		t.Fatal("duplicate key accepted")
	}
}

func TestIncompleteStreamAndKnownSecretsNeverBecomeReplayable(t *testing.T) {
	for _, kind := range []string{"truncated", "secret_request", "secret_response", "split_secret_response", "escaped_secret_response", "escaped_arguments_secret_response"} {
		t.Run(kind, func(t *testing.T) {
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				if kind == "secret_response" {
					_, _ = w.Write(frame(map[string]any{"content": testUpstreamKey}, nil))
					return
				}
				if kind == "split_secret_response" {
					_, _ = w.Write(frame(map[string]any{"content": testUpstreamKey[:10]}, nil))
					_, _ = w.Write(frame(map[string]any{"content": testUpstreamKey[10:]}, nil))
					return
				}
				if kind == "escaped_secret_response" || kind == "escaped_arguments_secret_response" {
					var escaped strings.Builder
					for _, char := range testUpstreamKey {
						fmt.Fprintf(&escaped, `\u%04x`, char)
					}
					if kind == "escaped_arguments_secret_response" {
						args := `{"value":"` + escaped.String() + `"}`
						_, _ = w.Write(frame(map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "call", "type": "function", "function": map[string]any{"name": "exec", "arguments": args}}}}, nil))
						_, _ = w.Write(frame(map[string]any{}, "tool_calls"))
						_, _ = w.Write([]byte("data: [DONE]\n\n"))
					} else {
						data := bytes.Replace(frame(map[string]any{"content": testUpstreamKey}, nil), []byte(testUpstreamKey), []byte(escaped.String()), 1)
						_, _ = w.Write(data)
					}
					return
				}
				_, _ = w.Write(framesAt(0)[0])
			}))
			defer up.Close()
			dir := filepath.Join(t.TempDir(), "record")
			s, err := New(Options{Mode: "record", Identity: testIdentity(kind), Model: "test-model", AccessToken: []byte(testToken), UpstreamURL: up.URL, UpstreamKey: []byte(testUpstreamKey), OutputDir: dir})
			if err != nil {
				t.Fatal(err)
			}
			raw := requestAt(0)
			if kind == "secret_request" {
				raw = bytes.Replace(raw, []byte("private test system"), []byte(testUpstreamKey), 1)
			}
			post(s, raw)
			if s.Close() == nil {
				t.Fatal("incomplete recording succeeded")
			}
			if _, err := Load(filepath.Join(dir, "tape.json")); err == nil {
				t.Fatal("incomplete recording replayable")
			}
			for _, name := range []string{"tape.json", "events.jsonl"} {
				b, _ := os.ReadFile(filepath.Join(dir, name))
				if bytes.Contains(b, []byte(testUpstreamKey)) {
					t.Fatal("secret on disk")
				}
			}
			if strings.HasSuffix(kind, "secret_response") {
				b, err := os.ReadFile(filepath.Join(dir, "tape.json"))
				var tape Tape
				if err != nil || json.Unmarshal(b, &tape) != nil || len(tape.Turns) != 0 {
					t.Fatal("unsafe response turn was persisted")
				}
			}
		})
	}
}

func TestKnownCredentialCannotBePersistedAsSessionMetadata(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "session")
	id := testIdentity("metadata")
	id.SessionID = testToken
	_, err := New(Options{Mode: "replay", Identity: id, AccessToken: []byte(testToken), Tape: fixtureTape(t), Timing: Timing{Mode: "zero"}, OutputDir: dir})
	if err == nil {
		t.Fatal("credential accepted as session identity")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("rejected metadata created an artifact directory")
	}
}

func TestTapeChecksumsAndUnsupportedTrajectories(t *testing.T) {
	tape := fixtureTape(t)
	tape.Turns[0].Frames[0].Data[0] = 'X'
	if validateTape(*tape) == nil {
		t.Fatal("corrupt tape accepted")
	}
	for _, kind := range []string{"compaction", "parallel", "wrong_id", "rewritten_assistant", "rewritten_reasoning"} {
		t.Run(kind, func(t *testing.T) {
			v, _ := decode(requestAt(1))
			req := v.(map[string]any)
			ms := req["messages"].([]any)
			switch kind {
			case "compaction":
				req["messages"] = ms[1:]
			case "parallel":
				a := ms[2].(map[string]any)
				a["tool_calls"] = append(a["tool_calls"].([]any), a["tool_calls"].([]any)[0])
			case "wrong_id":
				ms[3].(map[string]any)["tool_call_id"] = "wrong"
			case "rewritten_assistant":
				ms[2].(map[string]any)["content"] = "not the emitted text"
			case "rewritten_reasoning":
				ms[2].(map[string]any)["reasoning_content"] = "not the emitted reasoning"
			}
			if nextRequest(req, &fixtureTape(t).Turns[0], "", "") == nil {
				t.Fatal("unsupported history accepted")
			}
		})
	}
}

type disconnectedWriter struct{ header http.Header }

func (w *disconnectedWriter) Header() http.Header       { return w.header }
func (w *disconnectedWriter) WriteHeader(int)           {}
func (w *disconnectedWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func (w *disconnectedWriter) Flush()                    {}

func TestDisconnectDoesNotAdvanceAndAuthDoesNotExposeModel(t *testing.T) {
	s := replayServer(t, "disconnect", fixtureTape(t), Timing{Mode: "zero"})
	get := httptest.NewRequest("GET", "/v1/models", nil)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, get)
	if w.Code != 401 {
		t.Fatal("unauthenticated model catalog")
	}
	r := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(requestAt(0)))
	r.Header.Set("Authorization", "Bearer "+testToken)
	s.ServeHTTP(&disconnectedWriter{header: make(http.Header)}, r)
	if s.position != 0 || s.Close() == nil {
		t.Fatal("failed delivery advanced/finished")
	}
}

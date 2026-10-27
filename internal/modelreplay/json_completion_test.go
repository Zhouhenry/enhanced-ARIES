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
)

func jsonRequestAt(turn int) []byte {
	return bytes.Replace(requestAt(turn), []byte(`"stream":true`), []byte(`"stream":false`), 1)
}

func jsonCompletionAt(t *testing.T, turn int) []byte {
	t.Helper()
	var parsed streamResult
	for _, frame := range framesAt(turn) {
		if err := parsed.feed(frame); err != nil {
			t.Fatal(err)
		}
	}
	if err := parsed.complete(); err != nil {
		t.Fatal(err)
	}
	message := map[string]any{"role": "assistant", "content": parsed.text, "reasoning_content": parsed.reasoning}
	if calls := parsed.calls(); len(calls) > 0 {
		var tools []any
		for _, call := range calls {
			tools = append(tools, map[string]any{"id": call.ID, "type": "function", "function": map[string]any{"name": call.Name, "arguments": call.Arguments}})
		}
		message["tool_calls"] = tools
	}
	raw, err := json.MarshalIndent(map[string]any{"id": "original-completion", "choices": []any{map[string]any{"index": 0, "finish_reason": parsed.finish, "message": message}}, "usage": map[string]any{"total_tokens": 42}}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(raw, '\n')
}

func TestJSONCompletionRecordLoadStrictReplayAndRetry(t *testing.T) {
	bodies := [][]byte{jsonCompletionAt(t, 0), jsonCompletionAt(t, 1), jsonCompletionAt(t, 2)}
	var calls atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(calls.Add(1)) - 1
		if i >= len(bodies) {
			t.Error("retry called upstream")
			w.WriteHeader(500)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		if !bytes.Equal(raw, jsonRequestAt(i)) || r.Header.Get("Accept") != "application/json" {
			t.Error("request was changed")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(bodies[i])
	}))
	defer up.Close()
	id := testIdentity("json-record")
	id.HarnessVersion = jiuwenFixedTailHarnessVersion
	s, err := New(Options{Mode: "record", Identity: id, HistoryDialect: JiuwenSubagentHistoryDialect, Model: "test-model", AccessToken: []byte(testToken), UpstreamURL: up.URL, UpstreamKey: []byte(testUpstreamKey), OutputDir: filepath.Join(t.TempDir(), "record")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	for i := 0; i < 3; i++ {
		w := post(s, jsonRequestAt(i))
		if w.Code != 200 || w.Header().Get("Content-Type") != "application/json" || !bytes.Equal(w.Body.Bytes(), bodies[i]) {
			t.Fatalf("JSON turn %d failed", i)
		}
		if retry := post(s, jsonRequestAt(i)); !bytes.Equal(retry.Body.Bytes(), bodies[i]) {
			t.Fatal("cached JSON retry changed bytes")
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	tape, err := Load(filepath.Join(s.options.OutputDir, "tape.json"))
	if err != nil {
		t.Fatal(err)
	}
	if tape.Version != jsonResponseVersion || calls.Load() != 3 {
		t.Fatal("wrong format or upstream retry")
	}
	for i, turn := range tape.Turns {
		if len(turn.Frames) != 0 || turn.ResponseOffsetNS == nil || !bytes.Equal(turn.ResponseJSON, bodies[i]) {
			t.Fatal("JSON represented as synthetic SSE or changed")
		}
	}
	for _, changed := range []bool{false, true} {
		id.SessionID = "json-replay"
		r, err := New(Options{Mode: "replay", Identity: id, HistoryDialect: JiuwenSubagentHistoryDialect, AccessToken: []byte(testToken), Tape: tape, Timing: Timing{Mode: "recorded"}, OutputDir: filepath.Join(t.TempDir(), "replay")})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = r.Close() })
		for i := 0; i < 3; i++ {
			raw := jsonRequestAt(i)
			if changed && i == 2 {
				raw = bytes.Replace(raw, []byte("result-0"), []byte("changed-result-0"), 1)
			}
			w := post(r, raw)
			if changed && i == 2 {
				if w.Code != 409 || !strings.Contains(w.Body.String(), "REPLAY_DIVERGED") {
					t.Fatal("old tool result was not strictly checked")
				}
				break
			}
			if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), bodies[i]) {
				t.Fatal("Strict JSON replay failed")
			}
		}
		if err := r.Close(); (err != nil) != changed {
			t.Fatalf("wrong replay completion: %v", err)
		}
	}
	for _, request := range assertBoundaryRequests(t, s, 6) {
		jsonFlush := 0
		for _, e := range request {
			if e.Event == "json_flushed" {
				jsonFlush++
			}
			if e.Frame != nil || strings.Contains(e.Event, "first_sse") || e.Event == "frame_flushed" {
				t.Fatal("JSON response invented SSE frame timing")
			}
		}
		if jsonFlush != 1 {
			t.Fatal("JSON delivery boundary missing")
		}
	}
}

func TestJSONCompletionRejectsMalformedAndVersionConfusion(t *testing.T) {
	valid := jsonCompletionAt(t, 2)
	for _, raw := range [][]byte{append(bytes.Clone(valid), []byte(`{}`)...), bytes.Replace(valid, []byte(`"stop"`), []byte(`"length"`), 1), bytes.Replace(valid, []byte(`"assistant"`), []byte(`"user"`), 1), []byte(`{"choices":[]}`)} {
		if _, err := parseJSONCompletion(raw); err == nil {
			t.Fatal("invalid JSON completion accepted")
		}
	}
	if _, err := parseRequest(jsonRequestAt(0), "test-model"); err == nil {
		t.Fatal("default streaming contract changed")
	}
	tape := fixtureTape(t)
	tape.Turns[0].ResponseJSON = valid
	offset := int64(1)
	tape.Turns[0].ResponseOffsetNS = &offset
	tape.Turns[0].SHA256 = turnDigest(tape.Turns[0])
	tape.SHA256 = tapeDigest(*tape)
	if err := validateTape(*tape); err == nil {
		t.Fatal("old tape version accepted JSON fields")
	}
}

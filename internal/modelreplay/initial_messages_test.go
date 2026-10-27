package modelreplay

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func multiUserRequest(turn int) []byte {
	req, _ := parseRequest(requestAt(turn), "test-model")
	ms := req["messages"].([]any)
	extra := map[string]any{"role": "user", "content": "Additional initial context.\nPreserve this exactly. "}
	req["messages"] = append([]any{ms[0], extra}, ms[1:]...)
	raw, _ := json.Marshal(req)
	return raw
}

// The native first request has system/user/user, with ordinary text content.
// The complete bytes must reach upstream, persist, and replay unchanged.
func TestInitialMultiUserRecordAndStrictReplay(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(calls.Add(1)) - 1
		raw, _ := io.ReadAll(r.Body)
		if i != 0 || !bytes.Equal(raw, multiUserRequest(i)) {
			t.Error("upstream initial/history request changed")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write(bytes.Join(framesAt(2), nil))
	}))
	defer upstream.Close()
	dir := filepath.Join(t.TempDir(), "record")
	id := testIdentity("multi-user")
	id.HarnessVersion = jiuwenFixedTailHarnessVersion
	s, err := New(Options{Mode: "record", Identity: id, HistoryDialect: jiuwenFixedTailHistoryDialect, Model: "test-model", AccessToken: []byte(testToken), UpstreamURL: upstream.URL, UpstreamKey: []byte(testUpstreamKey), OutputDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	for i := 0; i < 1; i++ {
		if w := post(s, multiUserRequest(i)); w.Code != http.StatusOK {
			t.Fatalf("multi-user turn %d rejected: status=%d", i, w.Code)
		}
	}
	for i := range s.tape.Turns {
		if !bytes.Equal(s.tape.Turns[i].Request, multiUserRequest(i)) {
			t.Fatal("recorded request bytes changed")
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	tape, err := Load(filepath.Join(dir, "tape.json"))
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatal("unexpected upstream call count")
	}
	replay := func() *Server {
		r, err := New(Options{Mode: "replay", Identity: id, HistoryDialect: jiuwenFixedTailHistoryDialect, AccessToken: []byte(testToken), Tape: tape, OutputDir: filepath.Join(t.TempDir(), "replay"), Timing: Timing{Mode: "zero"}})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = r.Close() })
		return r
	}
	r := replay()
	for i := 0; i < 1; i++ {
		// JSON indentation may change on Save; every field and content string
		// must remain exact after Load.
		actual, _ := decode(tape.Turns[i].Request)
		expected, _ := decode(multiUserRequest(i))
		if digest(actual) != digest(expected) {
			t.Fatal("persisted request fields changed")
		}
		if w := post(r, multiUserRequest(i)); w.Code != http.StatusOK {
			t.Fatalf("strict turn %d rejected", i)
		}
	}
	for _, index := range []int{1, 2} {
		r := replay()
		req, _ := parseRequest(multiUserRequest(0), "test-model")
		msg := req["messages"].([]any)[index].(map[string]any)
		msg["content"] = msg["content"].(string) + " "
		raw, _ := json.Marshal(req)
		if w := post(r, raw); w.Code != http.StatusConflict {
			t.Fatal("strict replay accepted changed initial user content")
		}
	}
}

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
)

const openClawIDDialect = "openclaw-2026.7.1"

func dialectRequestAt(turn int) []byte {
	// The upstream fixture emits call_0/call_1; native outgoing history uses call0/call1.
	return bytes.ReplaceAll(requestAt(turn), []byte(`"call_`), []byte(`"call`))
}

func dialectTape(t *testing.T) *Tape {
	t.Helper()
	tape := fixtureTape(t)
	tape.ToolCallIDDialect = openClawIDDialect
	tape.Identity.HarnessVersion = "2026.7.1"
	for i := range tape.Turns {
		tape.Turns[i].Request = dialectRequestAt(i)
		tape.Turns[i].SHA256 = turnDigest(tape.Turns[i])
	}
	tape.SHA256 = tapeDigest(*tape)
	if err := validateTape(*tape); err != nil {
		t.Fatal(err)
	}
	return tape
}

func dialectReplay(t *testing.T, tape *Tape) *Server {
	t.Helper()
	id := tape.Identity
	id.SessionID = "replay"
	s, err := New(Options{Mode: "replay", Identity: id, ToolCallIDDialect: openClawIDDialect,
		AccessToken: []byte(testToken), Tape: tape, OutputDir: filepath.Join(t.TempDir(), "replay"), Timing: Timing{Mode: "zero"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestDeclaredDialectRecordLoadReplayAndRetry(t *testing.T) {
	fixture := dialectTape(t)
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(calls.Add(1)) - 1
		raw, _ := io.ReadAll(r.Body)
		if i >= len(fixture.Turns) || !bytes.Equal(raw, fixture.Turns[i].Request) {
			t.Error("record changed request or repeated an upstream call")
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, data := range framesAt(i) {
			_, _ = w.Write(data)
		}
	}))
	defer upstream.Close()
	dir := filepath.Join(t.TempDir(), "record")
	s, err := New(Options{Mode: "record", Identity: fixture.Identity, ToolCallIDDialect: openClawIDDialect,
		Model: fixture.Model, AccessToken: []byte(testToken), UpstreamURL: upstream.URL,
		UpstreamKey: []byte(testUpstreamKey), OutputDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	for i := range fixture.Turns {
		for retry := 0; retry < 2; retry++ {
			w := post(s, fixture.Turns[i].Request)
			if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), bytes.Join(framesAt(i), nil)) {
				t.Fatalf("record turn %d: status %d", i, w.Code)
			}
		}
	}
	if calls.Load() != 3 || s.Close() != nil {
		t.Fatal("record/retry did not complete exactly three upstream turns")
	}
	tape, err := Load(filepath.Join(dir, "tape.json"))
	if err != nil || tape.ToolCallIDDialect != openClawIDDialect {
		t.Fatalf("declared tape did not load: %v", err)
	}
	r := dialectReplay(t, tape)
	for i, turn := range tape.Turns {
		a, _ := decode(turn.Request)
		b, _ := decode(fixture.Turns[i].Request)
		if digest(a) != digest(b) || digest(turn.toolCalls()) != digest(fixture.Turns[i].toolCalls()) {
			t.Fatal("recorded request or original upstream ID changed")
		}
		for j, frame := range turn.Frames {
			if !bytes.Equal(frame.Data, framesAt(i)[j]) {
				t.Fatal("recorded SSE changed")
			}
		}
		for retry := 0; retry < 2; retry++ {
			w := post(r, turn.Request)
			if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), bytes.Join(framesAt(i), nil)) {
				t.Fatalf("replay turn %d: status %d", i, w.Code)
			}
		}
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDeclaredDialectReplayStillRejectsEntireRequest(t *testing.T) {
	for _, field := range []string{"mapped_id", "older_history", "old_tool_result", "schema"} {
		t.Run(field, func(t *testing.T) {
			s := dialectReplay(t, dialectTape(t))
			for i := 0; i < 2; i++ {
				if post(s, dialectRequestAt(i)).Code != 200 {
					t.Fatal("prefix rejected")
				}
			}
			v, _ := decode(dialectRequestAt(2))
			req := v.(map[string]any)
			ms := req["messages"].([]any)
			switch field {
			case "mapped_id":
				// Both map to the recorded ID, but replay must not normalize either.
				ms[4].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)["id"] = "call_1"
				ms[5].(map[string]any)["tool_call_id"] = "call_1"
			case "older_history":
				ms[0].(map[string]any)["content"] = "changed system"
			case "old_tool_result":
				ms[3].(map[string]any)["content"] = "changed result"
			case "schema":
				req["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)["parameters"] = map[string]any{"type": "string"}
			}
			raw, _ := json.Marshal(req)
			w := post(s, raw)
			if w.Code != 409 || !strings.Contains(w.Body.String(), "REPLAY_DIVERGED") {
				t.Fatal("declared dialect hid replay divergence")
			}
			if w = post(s, dialectRequestAt(2)); w.Code != 409 || !strings.Contains(w.Body.String(), "SESSION_FAILED") || s.Close() == nil {
				t.Fatal("divergence was not terminal")
			}
		})
	}
}

func TestDeclaredDialectRejectsUnsupportedIDsAndHistoricalCollisions(t *testing.T) {
	for _, raw := range []string{"", "_-", "call|id", "工具1", "functions.exec:1", strings.Repeat("a", 41)} {
		if _, err := requestToolCallID(raw, openClawIDDialect, nil); err == nil {
			t.Fatal("unsupported ID accepted")
		}
	}
	for _, raw := range []string{"call_0", "call-0", "call0"} {
		history, _ := decode(dialectRequestAt(3))
		if _, err := requestToolCallID(raw, openClawIDDialect, history.(map[string]any)["messages"].([]any)); err == nil || !strings.Contains(err.Error(), "colliding") {
			t.Fatal("collision with oldest history was accepted")
		}
	}
	previous := &fixtureTape(t).Turns[0]
	for _, dialect := range []string{"", openClawIDDialect} {
		raw := dialectRequestAt(1)
		if dialect != "" {
			raw = requestAt(1) // The declaration requires the mapped ID, not either form.
		}
		req, _ := parseRequest(raw, "test-model")
		if nextRequest(req, previous, dialect, "") == nil {
			t.Fatal("undeclared mapping or declared raw-ID fallback accepted")
		}
	}
}

func TestDeclaredDialectCollisionFailsBeforeUpstreamAndOnLoad(t *testing.T) {
	tape := dialectTape(t)
	tape.Turns[1].Call.ID = "call-0"
	for i := range tape.Turns[1].Frames {
		frame := &tape.Turns[1].Frames[i]
		frame.Data = bytes.ReplaceAll(frame.Data, []byte(`"call_1"`), []byte(`"call-0"`))
	}
	tape.Turns[1].SHA256 = turnDigest(tape.Turns[1])
	tape.Turns[2].Request = bytes.ReplaceAll(tape.Turns[2].Request, []byte(`"call1"`), []byte(`"call0"`))
	tape.Turns[2].SHA256 = turnDigest(tape.Turns[2])
	tape.SHA256 = tapeDigest(*tape)
	path := filepath.Join(t.TempDir(), "collision.json")
	data, err := json.Marshal(tape)
	if err != nil || os.WriteFile(path, data, 0600) != nil {
		t.Fatal("cannot write collision fixture")
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "colliding") {
		t.Fatal("checksummed causal collision loaded")
	}
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(calls.Add(1)) - 1
		if i >= 2 {
			t.Error("collision reached upstream")
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, frame := range tape.Turns[i].Frames {
			_, _ = w.Write(frame.Data)
		}
	}))
	defer upstream.Close()
	s, err := New(Options{Mode: "record", Identity: tape.Identity, ToolCallIDDialect: openClawIDDialect,
		Model: tape.Model, AccessToken: []byte(testToken), UpstreamURL: upstream.URL,
		UpstreamKey: []byte(testUpstreamKey), OutputDir: filepath.Join(t.TempDir(), "record")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	for i := 0; i < 2; i++ {
		if post(s, tape.Turns[i].Request).Code != 200 {
			t.Fatal("valid prefix rejected")
		}
	}
	w := post(s, tape.Turns[2].Request)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "UNSUPPORTED") || calls.Load() != 2 || s.Close() == nil {
		t.Fatal("recorded collision did not fail before upstream")
	}
}

func TestToolCallIDDialectDeclarationIsValidatedAndChecksummed(t *testing.T) {
	for _, kind := range []string{"unknown", "wrong_version", "replay_missing", "replay_extra"} {
		t.Run(kind, func(t *testing.T) {
			tape := dialectTape(t)
			o := Options{Mode: "replay", Identity: tape.Identity, ToolCallIDDialect: openClawIDDialect,
				AccessToken: []byte(testToken), Tape: tape, OutputDir: filepath.Join(t.TempDir(), "rejected"), Timing: Timing{Mode: "zero"}}
			switch kind {
			case "unknown":
				o.ToolCallIDDialect = "unknown"
			case "wrong_version":
				o.Identity.HarnessVersion = "2026.7.2"
			case "replay_missing":
				o.ToolCallIDDialect = ""
			case "replay_extra":
				o.Tape = fixtureTape(t)
				o.Tape.Identity.HarnessVersion = "2026.7.1"
				o.Tape.SHA256 = tapeDigest(*o.Tape)
			}
			if _, err := New(o); err == nil {
				t.Fatal("invalid or mismatched declaration accepted")
			}
			if _, err := os.Stat(o.OutputDir); !os.IsNotExist(err) {
				t.Fatal("invalid declaration created output")
			}
			if kind == "unknown" || kind == "wrong_version" {
				tape.ToolCallIDDialect, tape.Identity.HarnessVersion = o.ToolCallIDDialect, o.Identity.HarnessVersion
				tape.SHA256 = tapeDigest(*tape)
				if validateTape(*tape) == nil {
					t.Fatal("loader accepted unsupported declaration")
				}
				o.Mode, o.Tape, o.Model = "record", nil, "test-model"
				o.UpstreamURL, o.UpstreamKey = "https://test.invalid", []byte(testUpstreamKey)
				if _, err := New(o); err == nil {
					t.Fatal("recorder accepted unsupported declaration")
				}
			}
		})
	}
	tape := fixtureTape(t)
	tape.Identity.HarnessVersion = "2026.7.1"
	tape.SHA256 = tapeDigest(*tape)
	tape.ToolCallIDDialect = openClawIDDialect
	if err := validateTape(*tape); err == nil || err.Error() != "tape checksum mismatch" {
		t.Fatal("declaration was not covered by checksum")
	}
}

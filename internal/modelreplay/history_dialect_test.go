package modelreplay

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func requestTailRequest(turn int) []byte {
	req, _ := parseRequest(requestAt(turn), "test-model")
	mode := "agent"
	if turn > 0 {
		mode = "智能体模式"
	}
	req["messages"] = append(req["messages"].([]any), map[string]any{"role": "user", "content": jiuwenAttachmentPrefix + "runtime\">\nCurrent mode: " + mode + "\n</prompt-attachment>\n</system-reminder>"})
	raw, _ := json.Marshal(req)
	return raw
}

func requestTailTape(t *testing.T) *Tape {
	t.Helper()
	tape := fixtureTape(t)
	tape.Identity.HarnessVersion = jiuwenFixedTailHarnessVersion
	tape.HistoryDialect = JiuwenHistoryDialect
	for i := range tape.Turns {
		tape.Turns[i].Request = requestTailRequest(i)
		tape.Turns[i].SHA256 = turnDigest(tape.Turns[i])
	}
	tape.SHA256 = tapeDigest(*tape)
	if err := validateTape(*tape); err != nil {
		t.Fatal(err)
	}
	return tape
}

func requestTailReplay(t *testing.T, tape *Tape) *Server {
	t.Helper()
	s, err := New(Options{Mode: "replay", Identity: tape.Identity, HistoryDialect: JiuwenHistoryDialect, AccessToken: []byte(testToken), Tape: tape, OutputDir: filepath.Join(t.TempDir(), "replay"), Timing: Timing{Mode: "zero"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestRequestTailHistoryRecordAndStrictReplay(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(calls.Add(1)) - 1
		raw, _ := io.ReadAll(r.Body)
		if i >= 3 || !bytes.Equal(raw, requestTailRequest(i)) {
			t.Error("fixed-tail request changed before upstream")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write(bytes.Join(framesAt(i), nil))
	}))
	defer upstream.Close()
	id := testIdentity("fixed-tail")
	id.HarnessVersion = jiuwenFixedTailHarnessVersion
	dir := filepath.Join(t.TempDir(), "record")
	s, err := New(Options{Mode: "record", Identity: id, HistoryDialect: JiuwenHistoryDialect, Model: "test-model", AccessToken: []byte(testToken), UpstreamURL: upstream.URL, UpstreamKey: []byte(testUpstreamKey), OutputDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	for i := 0; i < 3; i++ {
		if w := post(s, requestTailRequest(i)); w.Code != http.StatusOK {
			t.Fatalf("fixed-tail turn %d rejected: status=%d", i, w.Code)
		}
	}
	for i := range s.tape.Turns {
		if !bytes.Equal(s.tape.Turns[i].Request, requestTailRequest(i)) {
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
	if calls.Load() != 3 || tape.HistoryDialect != JiuwenHistoryDialect {
		t.Fatal("recording did not freeze the declared history dialect")
	}
	r := requestTailReplay(t, tape)
	for i := 0; i < 3; i++ {
		// JSON indentation may change on Save; every field and content string
		// must remain exact after Load.
		actual, _ := decode(tape.Turns[i].Request)
		expected, _ := decode(requestTailRequest(i))
		if digest(actual) != digest(expected) {
			t.Fatal("persisted request fields changed")
		}
		if w := post(r, requestTailRequest(i)); w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), bytes.Join(framesAt(i), nil)) {
			t.Fatalf("strict response differs at turn %d", i)
		}
	}
}

func TestRequestTailHistoryRejectsRewritesAndCausalityChanges(t *testing.T) {
	tape := requestTailTape(t)
	for _, change := range []string{"prefix", "tail_shape", "tail_role", "assistant", "arguments", "id", "result_id", "extra", "default_dialect"} {
		t.Run(change, func(t *testing.T) {
			req, _ := parseRequest(requestTailRequest(1), "test-model")
			ms := req["messages"].([]any)
			historyDialect := JiuwenHistoryDialect
			switch change {
			case "prefix":
				ms[1].(map[string]any)["content"] = "rewritten task"
			case "tail_shape":
				ms[len(ms)-1].(map[string]any)["content"] = "changed attachment"
			case "tail_role":
				ms[len(ms)-1].(map[string]any)["role"] = "system"
			case "assistant":
				ms[2].(map[string]any)["content"] = "unrecorded assistant text"
			case "arguments", "id":
				call := ms[2].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)
				if change == "arguments" {
					call["function"].(map[string]any)["arguments"] = `{"command":"changed"}`
				} else {
					call["id"] = "other-call"
				}
			case "result_id":
				ms[3].(map[string]any)["tool_call_id"] = "other-call"
			case "extra":
				req["messages"] = append(ms, map[string]any{"role": "user", "content": "extra"})
			case "default_dialect":
				historyDialect = ""
			}
			if nextRequest(req, &tape.Turns[0], "", historyDialect) == nil {
				t.Fatal("unsupported history or tool causality accepted")
			}
			if change == "default_dialect" {
				return
			}
			r := requestTailReplay(t, tape)
			if w := post(r, requestTailRequest(0)); w.Code != http.StatusOK {
				t.Fatal("initial strict request failed")
			}
			raw, _ := json.Marshal(req)
			if w := post(r, raw); w.Code != http.StatusConflict {
				t.Fatal("strict matcher accepted a changed complete request")
			}
		})
	}
}

func TestHistoryDialectDeclarationIsBoundAndChecksummed(t *testing.T) {
	for _, change := range []string{"unknown", "wrong_version", "replay_missing", "replay_extra"} {
		t.Run(change, func(t *testing.T) {
			tape := requestTailTape(t)
			o := Options{Mode: "replay", Identity: tape.Identity, HistoryDialect: JiuwenHistoryDialect, AccessToken: []byte(testToken), Tape: tape, OutputDir: filepath.Join(t.TempDir(), "rejected"), Timing: Timing{Mode: "zero"}}
			switch change {
			case "unknown":
				o.HistoryDialect = "unknown"
			case "wrong_version":
				o.Identity.HarnessVersion += "-changed"
			case "replay_missing":
				o.HistoryDialect = ""
			case "replay_extra":
				o.Tape = fixtureTape(t)
				o.Tape.Identity.HarnessVersion = o.Identity.HarnessVersion
				o.Tape.SHA256 = tapeDigest(*o.Tape)
			}
			if _, err := New(o); err == nil {
				t.Fatal("invalid declaration accepted")
			}
			if _, err := os.Stat(o.OutputDir); !os.IsNotExist(err) {
				t.Fatal("invalid declaration created output")
			}
			if change == "unknown" || change == "wrong_version" {
				tape.HistoryDialect, tape.Identity.HarnessVersion = o.HistoryDialect, o.Identity.HarnessVersion
				tape.SHA256 = tapeDigest(*tape)
				if validateTape(*tape) == nil {
					t.Fatal("loader accepted invalid declaration")
				}
			}
		})
	}
	tape := fixtureTape(t)
	tape.Identity.HarnessVersion = jiuwenFixedTailHarnessVersion
	tape.SHA256 = tapeDigest(*tape)
	tape.HistoryDialect = JiuwenHistoryDialect
	if err := validateTape(*tape); err == nil || err.Error() != "tape checksum mismatch" {
		t.Fatal("history dialect is not covered by the checksum")
	}
}

func TestInitialHistoryRoleBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name  string
		roles []string
		valid bool
	}{
		{"single_user", []string{"user"}, true},
		{"native_multi_user_requires_dialect", []string{"system", "user", "user"}, false},
		{"input_segments_require_dialect", []string{"system", "developer", "user", "system", "user"}, false},
		{"assistant_history", []string{"system", "assistant", "user"}, false},
		{"tool_history", []string{"tool", "user"}, false},
		{"no_user", []string{"system", "developer"}, false},
		{"non_user_last", []string{"user", "system"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := parseRequest(requestAt(0), "test-model")
			ms := make([]any, 0, len(tc.roles))
			for _, role := range tc.roles {
				ms = append(ms, map[string]any{"role": role, "content": "literal public fixture"})
			}
			req["messages"] = ms
			if (nextRequest(req, nil, "", "") == nil) != tc.valid {
				t.Fatal("initial history grammar differs")
			}
		})
	}
}

func TestRequestTailUpdatesRemainLiteralInStrictReplay(t *testing.T) {
	tape := requestTailTape(t)
	req, _ := parseRequest(requestTailRequest(1), "test-model")
	tail := req["messages"].([]any)[4].(map[string]any)
	// This is a valid per-call attachment, but differs from the recorded turn.
	tail["content"] = jiuwenAttachmentPrefix + "runtime\">\nCurrent mode: agent\n</prompt-attachment>\n</system-reminder>"
	if err := nextRequest(req, &tape.Turns[0], "", JiuwenHistoryDialect); err != nil {
		t.Fatal("recording rejected a valid fresh request attachment")
	}
	r := requestTailReplay(t, tape)
	if w := post(r, requestTailRequest(0)); w.Code != http.StatusOK {
		t.Fatal("initial strict request failed")
	}
	raw, _ := json.Marshal(req)
	if w := post(r, raw); w.Code != http.StatusConflict {
		t.Fatal("strict replay ignored the recorded attachment content")
	}
}

func TestRequestTailRequiresPinnedRendererEnvelope(t *testing.T) {
	for _, change := range []string{"prefix", "suffix", "role", "extra_field", "not_text", "no_core_history"} {
		t.Run(change, func(t *testing.T) {
			req, _ := parseRequest(requestTailRequest(0), "test-model")
			ms := req["messages"].([]any)
			tail := ms[len(ms)-1].(map[string]any)
			switch change {
			case "prefix":
				tail["content"] = "different prefix" + tail["content"].(string)
			case "suffix":
				tail["content"] = tail["content"].(string) + "trailing text"
			case "role":
				tail["role"] = "system"
			case "extra_field":
				tail["name"] = "extra"
			case "not_text":
				tail["content"] = nil
			case "no_core_history":
				req["messages"] = []any{tail}
			}
			if nextRequest(req, nil, "", JiuwenHistoryDialect) == nil {
				t.Fatal("unsupported request attachment shape accepted")
			}
		})
	}
}

func TestEarlierFixedTailDeclarationKeepsItsMeaning(t *testing.T) {
	tape := requestTailTape(t)
	tape.HistoryDialect = jiuwenFixedTailHistoryDialect
	tape.SHA256 = tapeDigest(*tape)
	if validateTape(*tape) == nil {
		t.Fatal("old declaration accepted changing attachments")
	}
	initial, _ := parseRequest(tape.Turns[0].Request, tape.Model)
	fixed := initial["messages"].([]any)[2]
	for i := range tape.Turns {
		req, _ := parseRequest(tape.Turns[i].Request, tape.Model)
		ms := req["messages"].([]any)
		ms[len(ms)-1] = fixed
		tape.Turns[i].Request, _ = json.Marshal(req)
		tape.Turns[i].SHA256 = turnDigest(tape.Turns[i])
	}
	tape.SHA256 = tapeDigest(*tape)
	if err := validateTape(*tape); err != nil {
		t.Fatal("old fixed-tail tape no longer loads")
	}
	r, err := New(Options{Mode: "replay", Identity: tape.Identity, HistoryDialect: jiuwenFixedTailHistoryDialect, AccessToken: []byte(testToken), Tape: tape, OutputDir: filepath.Join(t.TempDir(), "old-replay"), Timing: Timing{Mode: "zero"}})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for _, turn := range tape.Turns {
		if w := post(r, turn.Request); w.Code != http.StatusOK {
			t.Fatal("old fixed-tail tape no longer replays")
		}
	}
	req, _ := parseRequest(requestTailRequest(1), tape.Model)
	if nextRequest(req, &tape.Turns[0], "", jiuwenFixedTailHistoryDialect) == nil {
		t.Fatal("old fixed-tail causality was loosened")
	}
}

func TestInitialMultiUserRequiresJiuwenHistoryDeclaration(t *testing.T) {
	for _, dialect := range []string{"", "openclaw-2026.7.1"} {
		req, _ := parseRequest(requestTailRequest(0), "test-model")
		if nextRequest(req, nil, dialect, "") == nil {
			t.Fatal("default/OpenClaw initial multi-user rule was loosened")
		}
	}
	for _, dialect := range []string{jiuwenFixedTailHistoryDialect, JiuwenHistoryDialect} {
		req, _ := parseRequest(requestTailRequest(0), "test-model")
		if err := nextRequest(req, nil, "", dialect); err != nil {
			t.Fatal("declared Jiuwen initial multi-user input rejected")
		}
	}
}

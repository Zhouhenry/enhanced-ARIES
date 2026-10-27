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

func goalResponse(arguments string) [][]byte {
	return [][]byte{
		frame(map[string]any{"role": "assistant", "reasoning_content": "reasoning", "tool_calls": []any{map[string]any{"index": 0, "id": "call_0", "type": "function", "function": map[string]any{"name": "exec", "arguments": arguments}}}}, nil),
		frame(map[string]any{}, "tool_calls"), []byte("data: [DONE]\r\n\r\n"),
	}
}

func goalRequest(turn int, arguments string) []byte {
	req, _ := parseRequest(requestTailRequest(turn), "test-model")
	if turn > 0 {
		assistant := req["messages"].([]any)[2].(map[string]any)
		assistant["tool_calls"].([]any)[0].(map[string]any)["function"].(map[string]any)["arguments"] = arguments
	}
	raw, _ := json.Marshal(req)
	return raw
}

func TestJiuwenDisplayArgumentsExactCausality(t *testing.T) {
	for _, tc := range []struct {
		name, source, history string
		valid                 bool
	}{
		{"goal", `{"command":"step0","call_goal":"read project"}`, `{"command":"step0"}`, true},
		{"alias", `{"command":"step0","callGoal":"read project"}`, `{"command":"step0"}`, true},
		{"both_removed", `{"command":"step0","call_goal":"first","callGoal":"second"}`, `{"command":"step0"}`, true},
		{"nonstrings_also_removed", `{"command":"step0","call_goal":false,"callGoal":{"nested":1}}`, `{"command":"step0"}`, true},
		{"null_and_empty", `{"command":"step0","call_goal":null,"callGoal":""}`, `{"command":"step0"}`, true},
		{"no_metadata", `{"command":"step0","description":"keep","display_name":"keep","nested":{"call_goal":"keep"}}`, `{"command":"step0","description":"keep","display_name":"keep","nested":{"call_goal":"keep"}}`, true},
		{"nonobject_unchanged", `["call_goal",1]`, `["call_goal",1]`, true},
		{"command_changed", `{"command":"step0","call_goal":"label"}`, `{"command":"other"}`, false},
		{"description_changed", `{"command":"step0","description":"keep","call_goal":"label"}`, `{"command":"step0","description":"other"}`, false},
		{"extra_field", `{"command":"step0","call_goal":"label"}`, `{"command":"step0","added":true}`, false},
		{"display_name_removed", `{"command":"step0","display_name":"keep","call_goal":"label"}`, `{"command":"step0"}`, false},
		{"nested_removed", `{"command":"step0","nested":{"call_goal":"keep"}}`, `{"command":"step0","nested":{}}`, false},
		{"one_of_both_retained", `{"command":"step0","call_goal":"first","callGoal":"second"}`, `{"command":"step0","callGoal":"second"}`, false},
		{"metadata_retained", `{"command":"step0","call_goal":"label"}`, `{"command":"step0","call_goal":"label"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			previous := Turn{Request: requestTailRequest(0), Call: &Call{ID: "call_0", Name: "exec", Arguments: tc.source}}
			for i, data := range goalResponse(tc.source) {
				previous.Frames = append(previous.Frames, Frame{OffsetNS: int64(i + 1), Data: data})
			}
			raw := goalRequest(1, tc.history)
			req, _ := parseRequest(raw, "test-model")
			before := digest(previous)
			if (nextRequest(req, &previous, "", JiuwenHistoryDialect) == nil) != tc.valid {
				t.Fatal("native argument extraction contract differs")
			}
			after, _ := json.Marshal(req)
			original, _ := decode(raw)
			if digest(previous) != before || digest(req) != digest(original) {
				t.Fatal("causal check mutated response or request")
			}
			if _, err := parseRequest(after, "test-model"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOlderDialectsDoNotExtractDisplayArguments(t *testing.T) {
	for _, history := range []string{"", jiuwenFixedTailHistoryDialect, jiuwenRequestTailHistoryDialect} {
		t.Run(history, func(t *testing.T) {
			previous := Turn{Request: requestTailRequest(0), Call: &Call{ID: "call_0", Name: "exec", Arguments: `{"command":"step0","call_goal":"label"}`}}
			for i, data := range goalResponse(previous.Call.Arguments) {
				previous.Frames = append(previous.Frames, Frame{OffsetNS: int64(i + 1), Data: data})
			}
			req, _ := parseRequest(goalRequest(1, `{"command":"step0"}`), "test-model")
			old, _ := parseRequest(previous.Request, "test-model")
			oldms := old["messages"].([]any)
			ms := req["messages"].([]any)
			if history == "" {
				old["messages"] = oldms[:len(oldms)-1]
				req["messages"] = ms[:len(ms)-1]
			} else {
				ms[len(ms)-1] = oldms[len(oldms)-1]
			}
			previous.Request, _ = json.Marshal(old)
			if err := nextRequest(req, &previous, "", history); err == nil || err.Error() != "UNSUPPORTED: tool call causality mismatch" {
				t.Fatalf("old dialect changed: %v", err)
			}
		})
	}
	// A previously valid request-tail-v1 Tape retains its original interpretation.
	tape := requestTailTape(t)
	tape.HistoryDialect = jiuwenRequestTailHistoryDialect
	tape.SHA256 = tapeDigest(*tape)
	if err := validateTape(*tape); err != nil {
		t.Fatal(err)
	}
	s, err := New(Options{Mode: "replay", Identity: tape.Identity, HistoryDialect: tape.HistoryDialect, AccessToken: []byte(testToken), Tape: tape, OutputDir: filepath.Join(t.TempDir(), "old"), Timing: Timing{Mode: "zero"}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, turn := range tape.Turns {
		if post(s, turn.Request).Code != http.StatusOK {
			t.Fatal("old request-tail replay rejected")
		}
	}
}

func TestDisplayExtractionRecordLoadAndLiteralStrictReplay(t *testing.T) {
	const originalArgs = `{"command":"step0","description":"keep","call_goal":"first label","callGoal":"second label"}`
	const historyArgs = `{"command":"step0","description":"keep"}`
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(calls.Add(1)) - 1
		raw, _ := io.ReadAll(r.Body)
		if i >= 3 || !bytes.Equal(raw, goalRequest(i, historyArgs)) {
			t.Error("request changed before upstream")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		response := framesAt(i)
		if i == 0 {
			response = goalResponse(originalArgs)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write(bytes.Join(response, nil))
	}))
	defer upstream.Close()
	id := testIdentity("call-goal")
	id.HarnessVersion = jiuwenFixedTailHarnessVersion
	dir := filepath.Join(t.TempDir(), "record")
	s, err := New(Options{Mode: "record", Identity: id, HistoryDialect: JiuwenHistoryDialect, Model: "test-model", AccessToken: []byte(testToken), UpstreamURL: upstream.URL, UpstreamKey: []byte(testUpstreamKey), OutputDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	for i := 0; i < 3; i++ {
		if post(s, goalRequest(i, historyArgs)).Code != http.StatusOK {
			t.Fatal("native display extraction rejected")
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	tape, err := Load(filepath.Join(dir, "tape.json"))
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 || tape.Turns[0].toolCalls()[0].Arguments != originalArgs {
		t.Fatal("original response arguments not retained")
	}
	originalFrames := goalResponse(originalArgs)
	for i, f := range tape.Turns[0].Frames {
		if !bytes.Equal(f.Data, originalFrames[i]) {
			t.Fatal("original SSE changed")
		}
	}
	for i, turn := range tape.Turns {
		got, _ := decode(turn.Request)
		want, _ := decode(goalRequest(i, historyArgs))
		if digest(got) != digest(want) {
			t.Fatal("stored request changed")
		}
	}
	replay := requestTailReplay(t, tape)
	for i := 0; i < 3; i++ {
		if post(replay, goalRequest(i, historyArgs)).Code != http.StatusOK {
			t.Fatal("literal strict replay rejected")
		}
	}
	for _, change := range []string{"result", "command", "description", "restore_metadata", "prefix"} {
		t.Run(change, func(t *testing.T) {
			replay := requestTailReplay(t, tape)
			if post(replay, goalRequest(0, historyArgs)).Code != http.StatusOK {
				t.Fatal("initial strict failed")
			}
			req, _ := parseRequest(goalRequest(1, historyArgs), "test-model")
			ms := req["messages"].([]any)
			f := ms[2].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)["function"].(map[string]any)
			switch change {
			case "result":
				ms[3].(map[string]any)["content"] = "different real tool output"
			case "command":
				f["arguments"] = `{"command":"different","description":"keep"}`
			case "description":
				f["arguments"] = `{"command":"step0","description":"different"}`
			case "restore_metadata":
				f["arguments"] = originalArgs
			case "prefix":
				ms[1].(map[string]any)["content"] = "changed task"
			}
			raw, _ := json.Marshal(req)
			if post(replay, raw).Code != http.StatusConflict {
				t.Fatal("Strict ignored a recorded request difference")
			}
		})
	}
}

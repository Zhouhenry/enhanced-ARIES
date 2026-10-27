package modelreplay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func multiFrames(turn int) [][]byte {
	if turn == 2 {
		return framesAt(2)
	}
	count := 2
	if turn == 1 {
		count = 1
	}
	var frames [][]byte
	// Deliberately interleave indices and deliver index 1 before index 0.
	for i := count - 1; i >= 0; i-- {
		frames = append(frames, frame(map[string]any{"tool_calls": []any{map[string]any{"index": i, "id": fmt.Sprintf("call_%d_%d", turn, i), "type": "function", "function": map[string]any{"name": "exec", "arguments": `{"command":`}}}}, nil))
	}
	var fragments []any
	for i := 0; i < count; i++ {
		fragments = append(fragments, map[string]any{"index": i, "function": map[string]any{"arguments": `"same command"}`}})
	}
	frames = append(frames, frame(map[string]any{"tool_calls": fragments}, nil), frame(map[string]any{}, "tool_calls"), []byte("data: [DONE]\r\n\r\n"))
	return frames
}

func multiRequest(turn int, dialect bool) []byte {
	v, _ := decode(requestAt(0))
	req := v.(map[string]any)
	ms := req["messages"].([]any)
	for round := 0; round < turn; round++ {
		count := 2
		if round == 1 {
			count = 1
		}
		var calls []any
		var ids []string
		for i := 0; i < count; i++ {
			id := fmt.Sprintf("call_%d_%d", round, i)
			if dialect {
				id = strings.ReplaceAll(id, "_", "")
			}
			ids = append(ids, id)
			calls = append(calls, map[string]any{"id": id, "type": "function", "function": map[string]any{"name": "exec", "arguments": `{"command":"same command"}`}})
		}
		ms = append(ms, map[string]any{"role": "assistant", "content": nil, "tool_calls": calls})
		// Result arrival order may differ while recording. Replay must preserve it.
		for i := count - 1; i >= 0; i-- {
			ms = append(ms, map[string]any{"role": "tool", "tool_call_id": ids[i], "content": fmt.Sprintf("output-%d-%d", round, i)})
		}
	}
	req["messages"] = ms
	raw, _ := json.Marshal(req)
	return raw
}

func multiTape(t *testing.T, dialect bool) *Tape {
	t.Helper()
	tape := fixtureTape(t)
	tape.Version = Version
	if dialect {
		tape.ToolCallIDDialect = openClawIDDialect
		tape.Identity.HarnessVersion = "2026.7.1"
	}
	for i := range tape.Turns {
		turn := &tape.Turns[i]
		turn.Request = multiRequest(i, dialect)
		turn.Call = nil
		turn.Frames = nil
		var response streamResult
		for j, data := range multiFrames(i) {
			if err := response.feed(data); err != nil {
				t.Fatal(err)
			}
			turn.Frames = append(turn.Frames, Frame{OffsetNS: int64(j + 1), Data: data})
		}
		if err := response.complete(); err != nil {
			t.Fatal(err)
		}
		turn.Calls = response.calls()
		turn.Finish = response.finish
		turn.SHA256 = turnDigest(*turn)
	}
	tape.SHA256 = tapeDigest(*tape)
	if err := validateTape(*tape); err != nil {
		t.Fatal(err)
	}
	return tape
}

func TestMultipleCallsRecordReplayAndCachedRetry(t *testing.T) {
	for _, dialect := range []bool{false, true} {
		t.Run(fmt.Sprint(dialect), func(t *testing.T) {
			fixture := multiTape(t, dialect)
			var requests atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				i := int(requests.Add(1)) - 1
				if i >= 3 {
					t.Error("cached retry reached upstream")
					w.WriteHeader(500)
					return
				}
				raw, _ := io.ReadAll(r.Body)
				if !bytes.Equal(raw, fixture.Turns[i].Request) {
					t.Error("request rewritten")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				for _, f := range fixture.Turns[i].Frames {
					_, _ = w.Write(f.Data)
					w.(http.Flusher).Flush()
				}
			}))
			defer upstream.Close()
			dir := filepath.Join(t.TempDir(), "record")
			s, err := New(Options{Mode: "record", Identity: fixture.Identity, ToolCallIDDialect: fixture.ToolCallIDDialect, Model: fixture.Model, AccessToken: []byte(testToken), UpstreamURL: upstream.URL, UpstreamKey: []byte(testUpstreamKey), OutputDir: dir})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = s.Close() })
			for i := 0; i < 3; i++ {
				for retry := 0; retry < 2; retry++ {
					w := post(s, fixture.Turns[i].Request)
					if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), bytes.Join(multiFrames(i), nil)) {
						t.Fatalf("record turn %d status %d", i, w.Code)
					}
				}
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			tape, err := Load(filepath.Join(dir, "tape.json"))
			if err != nil {
				t.Fatal(err)
			}
			if tape.Version != 2 || len(tape.Turns[0].Calls) != 2 || tape.Turns[0].Call != nil || requests.Load() != 3 {
				t.Fatal("v2 call denominator or cached retry changed")
			}
			for _, timing := range []string{"zero", "recorded", "synthetic"} {
				t.Run(timing, func(t *testing.T) {
					r, err := New(Options{Mode: "replay", Identity: tape.Identity, ToolCallIDDialect: tape.ToolCallIDDialect, AccessToken: []byte(testToken), Tape: tape, OutputDir: filepath.Join(t.TempDir(), "replay"), Timing: Timing{Mode: timing}})
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = r.Close() })
					for i := 0; i < 3; i++ {
						for retry := 0; retry < 2; retry++ {
							w := post(r, fixture.Turns[i].Request)
							if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), bytes.Join(multiFrames(i), nil)) {
								t.Fatalf("replay %d: %d", i, w.Code)
							}
						}
					}
					if err := r.Close(); err != nil {
						t.Fatal(err)
					}
				})
			}
		})
	}
}

func TestMultipleCallsCausalityAndStrictHistory(t *testing.T) {
	for _, change := range []string{"missing_result", "duplicate_result", "wrong_result_id", "arguments", "call_order", "result_order", "old_result", "old_history", "collision"} {
		t.Run(change, func(t *testing.T) {
			tape := multiTape(t, false)
			turn := 1
			if change == "old_result" || change == "old_history" {
				turn = 2
			}
			v, _ := decode(tape.Turns[turn].Request)
			req := v.(map[string]any)
			ms := req["messages"].([]any)
			calls := ms[2].(map[string]any)["tool_calls"].([]any)
			switch change {
			case "missing_result":
				ms = ms[:len(ms)-1]
			case "duplicate_result":
				ms[4] = ms[3]
			case "wrong_result_id":
				ms[3].(map[string]any)["tool_call_id"] = "wrong"
			case "arguments":
				calls[1].(map[string]any)["function"].(map[string]any)["arguments"] = `{"command":"changed"}`
			case "call_order":
				calls[0], calls[1] = calls[1], calls[0]
			case "result_order":
				ms[3], ms[4] = ms[4], ms[3]
			case "old_result":
				ms[3].(map[string]any)["content"] = "changed"
			case "old_history":
				ms[0].(map[string]any)["content"] = "changed"
			case "collision":
				calls[1].(map[string]any)["id"] = calls[0].(map[string]any)["id"]
			}
			req["messages"] = ms
			if change != "result_order" && nextRequest(req, &tape.Turns[turn-1], "", "") == nil {
				t.Fatal("causal mismatch accepted while recording")
			}
			s := replayServer(t, change, tape, Timing{Mode: "zero"})
			for i := 0; i < turn; i++ {
				if post(s, tape.Turns[i].Request).Code != 200 {
					t.Fatal("valid prefix rejected")
				}
			}
			raw, _ := json.Marshal(req)
			w := post(s, raw)
			if w.Code != 409 || !strings.Contains(w.Body.String(), "REPLAY_DIVERGED") {
				t.Fatal("strict divergence hidden")
			}
			if post(s, tape.Turns[turn].Request).Code != 409 || s.Close() == nil {
				t.Fatal("divergence recovered silently")
			}
		})
	}
}

func TestMultipleCallsRejectBadIndicesIDsAndVersion(t *testing.T) {
	for _, change := range []string{"sparse", "negative", "fractional", "duplicate_id", "changing_id", "invalid_arguments", "version1", "mixed_metadata", "dialect_collision"} {
		t.Run(change, func(t *testing.T) {
			tape := multiTape(t, false)
			turn := &tape.Turns[0]
			switch change {
			case "version1":
				tape.Version = 1
			case "mixed_metadata":
				turn.Call = &turn.Calls[0]
			case "dialect_collision":
				tape = multiTape(t, true)
				turn = &tape.Turns[0]
				turn.Frames[0].Data = bytes.ReplaceAll(turn.Frames[0].Data, []byte("call_0_1"), []byte("call-0-0"))
				turn.Calls[1].ID = "call-0-0"
			default:
				f := &turn.Frames[0].Data
				switch change {
				case "sparse":
					*f = bytes.ReplaceAll(*f, []byte(`"index":1`), []byte(`"index":3`))
				case "negative":
					*f = bytes.ReplaceAll(*f, []byte(`"index":1`), []byte(`"index":-1`))
				case "fractional":
					*f = bytes.ReplaceAll(*f, []byte(`"index":1`), []byte(`"index":0.5`))
				case "duplicate_id":
					*f = bytes.ReplaceAll(*f, []byte("call_0_1"), []byte("call_0_0"))
				case "changing_id":
					turn.Frames[1].Data = bytes.ReplaceAll(turn.Frames[1].Data, []byte(`"index":0`), []byte(`"index":1`))
				case "invalid_arguments":
					turn.Frames[2].Data = bytes.ReplaceAll(turn.Frames[2].Data, []byte(`same command`), []byte(`bad"json`))
				}
			}
			turn.SHA256 = turnDigest(*turn)
			tape.SHA256 = tapeDigest(*tape)
			if validateTape(*tape) == nil {
				t.Fatal("malformed multcall tape accepted")
			}
		})
	}
}

func TestMultipleCallsSecretsAndFirstDeltaIncludeNonzeroIndex(t *testing.T) {
	var response streamResult
	for _, part := range []string{`{"secret":"test-upstream-`, `secret-key"}`} {
		f := frame(map[string]any{"tool_calls": []any{map[string]any{"index": 1, "function": map[string]any{"arguments": part}}}}, nil)
		if err := response.feed(f); err != nil {
			t.Fatal(err)
		}
	}
	if !response.hasToolDelta() || !response.containsSecret(func(b []byte) bool { return bytes.Contains(b, []byte(testUpstreamKey)) }) {
		t.Fatal("second call was not observed or secret was missed")
	}
}

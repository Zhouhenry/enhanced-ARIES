package mockmodel_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/internal/mockmodel"
	"github.com/hyscale-lab/aries/pkg/config"
)

const probeFixture = `{
  "name":"probe", "tool":"terminal", "prompt_contains":"task marker", "response_delay_ms":0,
  "steps":[
    {"arguments":{"command":"printf first","env":{"PROBE":"one"}},"output_contains":"first marker"},
    {"arguments":{"command":"printf second"},"output_contains":"second marker"}
  ],
  "final_response":"Probe complete."
}`

type toolCall struct {
	Index    int    `json:"index,omitempty"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type message struct {
	Role       string     `json:"role"`
	Content    any        `json:"content,omitempty"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type response struct {
	ID      string          `json:"id"`
	Object  string          `json:"object"`
	Model   string          `json:"model"`
	Usage   json.RawMessage `json:"usage"`
	Choices []struct {
		Index        int     `json:"index"`
		Message      message `json:"message"`
		Delta        message `json:"delta"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
}

func newServer(t *testing.T, fixture string) *mockmodel.Server {
	t.Helper()
	s, err := mockmodel.New([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func initialMessages() []message {
	return []message{{Role: "user", Content: "Please run the task marker probe."}}
}

func requestBody(s *mockmodel.Server, tool string, messages []message, stream bool) map[string]any {
	return map[string]any{
		"model": s.ModelID(), "messages": messages, "stream": stream,
		"tools": []any{map[string]any{"type": "function", "function": map[string]any{
			"name": tool, "parameters": map[string]any{"type": "object"},
		}}},
	}
}

func serve(t *testing.T, s *mockmodel.Server, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer unused-local-token")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	return w
}

func completion(t *testing.T, s *mockmodel.Server, tool string, messages []message, stream bool) (message, string) {
	t.Helper()
	w := serve(t, s, requestBody(s, tool, messages, stream))
	if w.Code != http.StatusOK {
		t.Fatalf("completion returned %d: %s", w.Code, w.Body.String())
	}
	decode := func(data []byte) response {
		t.Helper()
		var r response
		if err := json.Unmarshal(data, &r); err != nil {
			t.Fatalf("decode completion: %v: %s", err, data)
		}
		if r.ID == "" || r.Model != s.ModelID() || len(r.Choices) != 1 || r.Choices[0].Index != 0 {
			t.Fatalf("invalid completion envelope: %s", data)
		}
		if len(r.Usage) != 0 && string(r.Usage) != "null" {
			t.Fatalf("mock fabricated usage: %s", r.Usage)
		}
		return r
	}
	if !stream {
		if !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
			t.Fatalf("JSON content type: %s", w.Header().Get("Content-Type"))
		}
		r := decode(w.Body.Bytes())
		if r.Object != "chat.completion" {
			t.Fatalf("JSON object = %q", r.Object)
		}
		return r.Choices[0].Message, r.Choices[0].FinishReason
	}
	if !strings.HasPrefix(w.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("SSE content type: %s", w.Header().Get("Content-Type"))
	}
	var result message
	var finish, id string
	done := false
	for _, line := range strings.Split(w.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if done {
			t.Fatal("SSE emitted data after [DONE]")
		}
		if data == "[DONE]" {
			done = true
			continue
		}
		r := decode([]byte(data))
		if r.Object != "chat.completion.chunk" || id != "" && id != r.ID {
			t.Fatalf("inconsistent SSE envelope: %s", data)
		}
		id = r.ID
		choice := r.Choices[0]
		if choice.Delta.Role != "" {
			result.Role = choice.Delta.Role
		}
		if content, ok := choice.Delta.Content.(string); ok {
			previous, _ := result.Content.(string)
			result.Content = previous + content
		}
		for _, call := range choice.Delta.ToolCalls {
			if call.Index != 0 {
				t.Fatalf("unexpected tool index %d", call.Index)
			}
			if len(result.ToolCalls) == 0 {
				result.ToolCalls = append(result.ToolCalls, toolCall{})
			}
			merged := &result.ToolCalls[0]
			merged.ID += call.ID
			merged.Type += call.Type
			merged.Function.Name += call.Function.Name
			merged.Function.Arguments += call.Function.Arguments
		}
		if choice.FinishReason != "" {
			finish = choice.FinishReason
		}
	}
	if !done || finish == "" {
		t.Fatalf("incomplete SSE response: %s", w.Body.String())
	}
	return result, finish
}

func TestModelIdentityAndDiscovery(t *testing.T) {
	s := newServer(t, probeFixture)
	digest := sha256.Sum256([]byte(probeFixture))
	wantHash := hex.EncodeToString(digest[:])
	if s.FixtureSHA256() != wantHash || s.ModelID() != "aries-mock-probe-"+wantHash[:12] {
		t.Fatalf("fixture identity = %s / %s", s.ModelID(), s.FixtureSHA256())
	}
	if newServer(t, probeFixture+"\n").ModelID() == s.ModelID() {
		t.Fatal("model identity ignored original fixture bytes")
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer unused-local-token")
	s.ServeHTTP(w, req)
	var listing struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listing); err != nil || w.Code != http.StatusOK || len(listing.Data) != 1 || listing.Data[0].ID != s.ModelID() {
		t.Fatalf("model listing = %d %s (%v)", w.Code, w.Body.String(), err)
	}
}

func TestCheckedInProfileMatchesFixture(t *testing.T) {
	data, err := os.ReadFile("../../configs/mock/hermes-sandbox-probe.json")
	if err != nil {
		t.Fatal(err)
	}
	s := newServer(t, string(data))
	profile, err := config.Load("../../profiles/hermes-tb2-fix-git-mock.json")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Model.ID != s.ModelID() || profile.Runtime.Backend != "openai" || profile.Runtime.Mode != "external" {
		t.Fatalf("example profile no longer targets its fixture: model=%s, runtime=%+v", profile.Model.ID, profile.Runtime)
	}
}

func TestConversationJSONAndSSE(t *testing.T) {
	for _, tool := range []string{"terminal", "exec"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", tool, stream), func(t *testing.T) {
				s := newServer(t, strings.Replace(probeFixture, `"terminal"`, fmt.Sprintf("%q", tool), 1))
				messages := []message{
					{Role: "system", Content: "System policy."}, {Role: "developer", Content: "Developer policy."},
					{Role: "user", Content: []any{map[string]any{"type": "text", "text": "Please run the task marker probe."}}},
				}
				for i, marker := range []string{"first marker", "second marker"} {
					assistant, finish := completion(t, s, tool, messages, stream)
					if assistant.Role != "assistant" || finish != "tool_calls" || len(assistant.ToolCalls) != 1 {
						t.Fatalf("step %d: %#v / %q", i, assistant, finish)
					}
					call := &assistant.ToolCalls[0]
					if call.ID == "" || call.Type != "function" || call.Function.Name != tool {
						t.Fatalf("invalid tool call: %#v", call)
					}
					retry, _ := completion(t, s, tool, messages, stream)
					if !reflect.DeepEqual(assistant, retry) {
						t.Fatal("retry changed the next action")
					}
					if i == 0 {
						call.Function.Arguments = `{ "env": { "PROBE": "one" }, "command": "printf first" }`
					}
					messages = append(messages, assistant, message{Role: "tool", ToolCallID: call.ID, Content: "output: " + marker + "\n"})
				}
				final, finish := completion(t, s, tool, messages, stream)
				if final.Role != "assistant" || final.Content != "Probe complete." || len(final.ToolCalls) != 0 || finish != "stop" {
					t.Fatalf("final response: %#v / %q", final, finish)
				}
			})
		}
	}
}

func TestRejectsInvalidConversation(t *testing.T) {
	s := newServer(t, probeFixture)
	first, _ := completion(t, s, "terminal", initialMessages(), false)
	firstResult := message{Role: "tool", ToolCallID: first.ToolCalls[0].ID, Content: "first marker"}
	valid := append(initialMessages(), first, firstResult)
	second, _ := completion(t, s, "terminal", valid, false)
	complete := append(append([]message{}, valid...), second, message{Role: "tool", ToolCallID: second.ToolCalls[0].ID, Content: "second marker"})
	final, _ := completion(t, s, "terminal", complete, false)
	cases := []struct {
		name   string
		change func(map[string]any)
	}{
		{"wrong model", func(b map[string]any) { b["model"] = "aries-mock-other" }},
		{"missing tool declaration", func(b map[string]any) { delete(b, "tools") }},
		{"wrong tool declaration", func(b map[string]any) { b["tools"] = requestBody(s, "exec", nil, false)["tools"] }},
		{"unmatched prompt", func(b map[string]any) { b["messages"] = []message{{Role: "user", Content: "another task"}} }},
		{"auxiliary call without tools", func(b map[string]any) {
			delete(b, "tools")
			b["messages"] = []message{{Role: "user", Content: "Summarize this conversation."}}
		}},
		{"missing result", func(b map[string]any) { b["messages"] = append(initialMessages(), first) }},
		{"wrong result ID", func(b map[string]any) {
			r := firstResult
			r.ToolCallID = "another-call"
			b["messages"] = append(initialMessages(), first, r)
		}},
		{"wrong result output", func(b map[string]any) {
			r := firstResult
			r.Content = "command failed"
			b["messages"] = append(initialMessages(), first, r)
		}},
		{"changed arguments", func(b map[string]any) {
			m := first
			m.ToolCalls = append([]toolCall{}, first.ToolCalls...)
			m.ToolCalls[0].Function.Arguments = `{"command":"different command"}`
			b["messages"] = append(initialMessages(), m, firstResult)
		}},
		{"changed assistant tool", func(b map[string]any) {
			m := first
			m.ToolCalls = append([]toolCall{}, first.ToolCalls...)
			m.ToolCalls[0].Function.Name = "exec"
			b["messages"] = append(initialMessages(), m, firstResult)
		}},
		{"multiple assistant calls", func(b map[string]any) {
			m := first
			m.ToolCalls = []toolCall{first.ToolCalls[0], first.ToolCalls[0]}
			b["messages"] = append(initialMessages(), m, firstResult)
		}},
		{"truncated earlier step", func(b map[string]any) { b["messages"] = append(initialMessages(), complete[3:]...) }},
		{"missing original user", func(b map[string]any) { b["messages"] = valid[1:] }},
		{"additional user", func(b map[string]any) { b["messages"] = append(append([]message{}, valid...), initialMessages()...) }},
		{"additional assistant", func(b map[string]any) {
			b["messages"] = append(initialMessages(), message{Role: "assistant", Content: "Extra reply."})
		}},
		{"system after user", func(b map[string]any) {
			b["messages"] = append(initialMessages(), message{Role: "system", Content: "Extra policy."})
		}},
		{"after final reply", func(b map[string]any) { b["messages"] = append(append([]message{}, complete...), final) }},
		{"user after final reply", func(b map[string]any) {
			b["messages"] = append(append(append([]message{}, complete...), final), message{Role: "user", Content: "task marker again"})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := requestBody(s, "terminal", append([]message{}, valid...), false)
			tc.change(body)
			w := serve(t, s, body)
			var payload struct {
				Error json.RawMessage `json:"error"`
			}
			if w.Code < 400 || w.Code >= 500 || json.Unmarshal(w.Body.Bytes(), &payload) != nil || len(payload.Error) == 0 {
				t.Fatalf("expected explicit client error, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestConcurrentConversationsAndRetries(t *testing.T) {
	s := newServer(t, probeFixture)
	first, _ := completion(t, s, "terminal", initialMessages(), false)
	afterFirst := append(initialMessages(), first, message{Role: "tool", ToolCallID: first.ToolCalls[0].ID, Content: "first marker"})
	second, _ := completion(t, s, "terminal", afterFirst, false)
	var workers sync.WaitGroup
	for i := 0; i < 12; i++ {
		workers.Go(func() {
			for retry := 0; retry < 3; retry++ {
				messages, want := initialMessages(), first
				if i%2 == 1 {
					messages, want = afterFirst, second
				}
				got, _ := completion(t, s, "terminal", messages, i%3 == 0)
				if !reflect.DeepEqual(got, want) {
					t.Errorf("conversation %d retry %d advanced another conversation", i, retry)
				}
			}
		})
	}
	workers.Wait()
}

func TestResponseDelayHonorsCancellation(t *testing.T) {
	s := newServer(t, strings.Replace(probeFixture, `"response_delay_ms":0`, `"response_delay_ms":1000`, 1))
	body, _ := json.Marshal(requestBody(s, "terminal", initialMessages(), false))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body)).WithContext(ctx)
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { defer close(done); s.ServeHTTP(w, req) }()
	select {
	case <-done:
		t.Fatal("response completed before the configured delay")
	case <-time.After(25 * time.Millisecond):
	}
	cancel()
	select {
	case <-done:
	case <-time.After(400 * time.Millisecond):
		t.Fatal("cancellation did not interrupt response delay")
	}
	if strings.Contains(w.Body.String(), `"choices"`) {
		t.Fatalf("canceled request emitted a completion: %s", w.Body.String())
	}
}

func TestRejectsInvalidFixture(t *testing.T) {
	cases := map[string]string{
		"malformed": "{", "trailing document": probeFixture + `{}`,
		"unknown field":       strings.Replace(probeFixture, `"name":"probe"`, `"name":"probe","unexpected":true`, 1),
		"missing name":        strings.Replace(probeFixture, `"name":"probe",`, "", 1),
		"missing tool":        strings.Replace(probeFixture, `"tool":"terminal",`, "", 1),
		"missing prompt":      strings.Replace(probeFixture, `"prompt_contains":"task marker",`, "", 1),
		"negative delay":      strings.Replace(probeFixture, `"response_delay_ms":0`, `"response_delay_ms":-1`, 1),
		"missing arguments":   strings.Replace(probeFixture, `"arguments":{"command":"printf second"},`, "", 1),
		"nonobject arguments": strings.Replace(probeFixture, `{"command":"printf second"}`, `"printf second"`, 1),
		"missing output":      strings.Replace(probeFixture, `,"output_contains":"second marker"`, "", 1),
		"empty final":         strings.Replace(probeFixture, `"Probe complete."`, `""`, 1),
		"empty steps":         `{"name":"probe","tool":"terminal","prompt_contains":"task marker","steps":[],"final_response":"done"}`,
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := mockmodel.New([]byte(data)); err == nil {
				t.Fatal("accepted invalid fixture")
			}
		})
	}
}

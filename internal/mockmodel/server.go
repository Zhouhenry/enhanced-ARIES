// Package mockmodel serves a fixed tool workload through Chat Completions.
// Progress belongs to the supplied conversation, never to the server process.
package mockmodel

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type fixture struct {
	Name            string `json:"name"`
	Tool            string `json:"tool"`
	PromptContains  string `json:"prompt_contains"`
	ResponseDelayMS int    `json:"response_delay_ms"`
	Steps           []struct {
		Arguments      json.RawMessage `json:"arguments"`
		OutputContains string          `json:"output_contains"`
	} `json:"steps"`
	FinalResponse string `json:"final_response"`
}

// Server is immutable after construction and safe for concurrent conversations.
type Server struct {
	fixture fixture
	digest  string
	modelID string
}

func New(data []byte) (*Server, error) {
	s := &Server{}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&s.fixture); err != nil {
		return nil, fmt.Errorf("decode fixture: %w", err)
	}
	if err := requireEOF(d); err != nil {
		return nil, err
	}
	f := &s.fixture
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,47}$`).MatchString(f.Name) {
		return nil, errors.New("fixture name must be 1–48 lowercase letters, digits or hyphens")
	}
	if strings.TrimSpace(f.Tool) == "" || strings.TrimSpace(f.PromptContains) == "" || strings.TrimSpace(f.FinalResponse) == "" || len(f.Steps) == 0 {
		return nil, errors.New("fixture requires tool, prompt_contains, steps and final_response")
	}
	if f.ResponseDelayMS < 0 || f.ResponseDelayMS > 60000 {
		return nil, errors.New("response_delay_ms must be between 0 and 60000")
	}
	for i := range f.Steps {
		args, err := canonicalObject(f.Steps[i].Arguments)
		if err != nil || strings.TrimSpace(f.Steps[i].OutputContains) == "" {
			return nil, fmt.Errorf("step %d requires object arguments and nonempty output_contains", i)
		}
		f.Steps[i].Arguments = args
	}
	sum := sha256.Sum256(data)
	s.digest = hex.EncodeToString(sum[:])
	s.modelID = "aries-mock-" + f.Name + "-" + s.digest[:12]
	return s, nil
}

func (s *Server) ModelID() string       { return s.modelID }
func (s *Server) FixtureSHA256() string { return s.digest }

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type message struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolCalls  []toolCall      `json:"tool_calls"`
	ToolCallID string          `json:"tool_call_id"`
}

type chatRequest struct {
	Model    string    `json:"model"`
	Stream   bool      `json:"stream"`
	Messages []message `json:"messages"`
	Tools    []struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	} `json:"tools"`
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
		writeJSON(w, map[string]any{"object": "list", "data": []any{map[string]any{
			"id": s.modelID, "object": "model", "created": 0, "owned_by": "aries-mock",
		}}})
		return
	}
	if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
		http.NotFound(w, r)
		return
	}
	var req chatRequest
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<20))
	if err := d.Decode(&req); err != nil {
		writeError(w, "invalid Chat Completions JSON")
		return
	}
	if err := requireEOF(d); err != nil {
		writeError(w, err.Error())
		return
	}
	step, err := s.nextStep(req)
	if err != nil {
		writeError(w, err.Error())
		return
	}
	if s.fixture.ResponseDelayMS > 0 {
		timer := time.NewTimer(time.Duration(s.fixture.ResponseDelayMS) * time.Millisecond)
		defer timer.Stop()
		select {
		case <-r.Context().Done():
			return
		case <-timer.C:
		}
	}
	if r.Context().Err() != nil {
		return
	}
	msg := map[string]any{"role": "assistant", "content": s.fixture.FinalResponse}
	finish := "stop"
	if step < len(s.fixture.Steps) {
		call := map[string]any{"id": s.callID(step), "type": "function", "function": map[string]any{
			"name": s.fixture.Tool, "arguments": string(s.fixture.Steps[step].Arguments),
		}}
		if req.Stream {
			call["index"] = 0
		}
		msg["content"] = nil
		msg["tool_calls"] = []any{call}
		finish = "tool_calls"
	}
	response := map[string]any{"id": s.callID(step), "object": "chat.completion", "created": 0, "model": s.modelID}
	if !req.Stream {
		response["choices"] = []any{map[string]any{"index": 0, "message": msg, "finish_reason": finish}}
		writeJSON(w, response)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	response["object"] = "chat.completion.chunk"
	for _, choice := range []any{
		map[string]any{"index": 0, "delta": msg, "finish_reason": nil},
		map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": finish},
	} {
		response["choices"] = []any{choice}
		data, _ := json.Marshal(response)
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
}

func (s *Server) callID(step int) string {
	return fmt.Sprintf("aries_mock_%s_%d", s.digest[:12], step)
}

func (s *Server) nextStep(req chatRequest) (int, error) {
	if req.Model != s.modelID {
		return 0, errors.New("model must match the fixture ID from /v1/models")
	}
	declared := false
	for _, tool := range req.Tools {
		declared = declared || (tool.Type == "function" && tool.Function.Name == s.fixture.Tool)
	}
	if !declared {
		return 0, errors.New("request must declare the fixture tool; auxiliary model calls are unsupported")
	}
	messages := req.Messages
	for len(messages) > 0 && (messages[0].Role == "system" || messages[0].Role == "developer") {
		messages = messages[1:]
	}
	if len(messages) == 0 || messages[0].Role != "user" {
		return 0, errors.New("conversation must begin with the fixture task prompt")
	}
	prompt, err := textContent(messages[0].Content)
	if err != nil || !strings.Contains(prompt, s.fixture.PromptContains) {
		return 0, errors.New("task prompt does not match fixture prompt_contains")
	}
	messages = messages[1:]
	step := 0
	for len(messages) > 0 {
		if step >= len(s.fixture.Steps) || len(messages) < 2 {
			return 0, errors.New("unexpected or incomplete conversation after fixture step")
		}
		assistant, result := messages[0], messages[1]
		if assistant.Role != "assistant" || len(assistant.ToolCalls) != 1 || result.Role != "tool" {
			return 0, errors.New("expected one assistant tool call followed by its tool result")
		}
		call := assistant.ToolCalls[0]
		args, err := canonicalObject([]byte(call.Function.Arguments))
		if err != nil || call.ID != s.callID(step) || call.Type != "function" || call.Function.Name != s.fixture.Tool || !bytes.Equal(args, s.fixture.Steps[step].Arguments) || result.ToolCallID != call.ID {
			return 0, fmt.Errorf("tool transcript differs from fixture at step %d", step)
		}
		output, err := textContent(result.Content)
		if err != nil || !strings.Contains(output, s.fixture.Steps[step].OutputContains) {
			return 0, fmt.Errorf("tool result lacks expected output at step %d", step)
		}
		messages = messages[2:]
		step++
	}
	return step, nil
}

func textContent(raw json.RawMessage) (string, error) {
	var text string
	if len(raw) > 0 && raw[0] == '"' && json.Unmarshal(raw, &text) == nil {
		return text, nil
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil || len(parts) == 0 {
		return "", errors.New("expected text content")
	}
	var b strings.Builder
	for _, p := range parts {
		if p.Type != "text" {
			return "", errors.New("only text content parts are supported")
		}
		b.WriteString(p.Text)
	}
	return b.String(), nil
}

func canonicalObject(raw []byte) ([]byte, error) {
	var obj map[string]any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(&obj); err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, errors.New("expected JSON object")
	}
	if err := requireEOF(d); err != nil {
		return nil, err
	}
	return json.Marshal(obj)
}

func requireEOF(d *json.Decoder) error {
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("expected exactly one JSON value")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{
		"type": "invalid_request_error", "message": message,
	}})
}

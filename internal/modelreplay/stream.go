package modelreplay

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
)

// readFrames preserves SSE event bytes and bounds both events and whole responses.
func readFrames(r io.Reader, visit func([]byte) error) error {
	reader := bufio.NewReaderSize(io.LimitReader(r, maxBody+1), 32<<10)
	var frame []byte
	total := 0
	for {
		line, err := reader.ReadSlice('\n')
		total += len(line)
		frame = append(frame, line...)
		if total > maxBody || len(frame) > maxFrame {
			return errors.New("response/frame exceeds bound")
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		if err != nil {
			if err == io.EOF && len(frame) == 0 {
				return nil
			}
			return errors.New("incomplete SSE event or response read failure")
		}
		if bytes.Equal(line, []byte("\n")) || bytes.Equal(line, []byte("\r\n")) {
			if err := visit(frame); err != nil {
				return err
			}
			frame = nil
		}
	}
}

func eventData(frame []byte) ([]byte, error) {
	if !bytes.HasSuffix(frame, []byte("\n\n")) && !bytes.HasSuffix(frame, []byte("\r\n\r\n")) {
		return nil, errors.New("incomplete SSE frame")
	}
	var lines []string
	for _, line := range strings.Split(string(frame), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(line, "data:") {
			lines = append(lines, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if len(lines) == 0 {
		return nil, nil
	}
	return []byte(strings.Join(lines, "\n")), nil
}

type streamResult struct {
	finish          string
	text, reasoning string
	canonicalFrame  []byte
	toolCalls       map[int]*streamCall
	done            bool
}

type streamCall struct {
	Call
	canonicalArguments []byte
}

// Called after complete: indices, not fragment arrival, define response order.
func (s *streamResult) calls() []Call {
	var out []Call
	for i := 0; i < len(s.toolCalls); i++ {
		out = append(out, s.toolCalls[i].Call)
	}
	return out
}

func (s *streamResult) hasToolDelta() bool {
	for _, call := range s.toolCalls {
		if len(call.ID)+len(call.Name)+len(call.Arguments) > 0 {
			return true
		}
	}
	return false
}

func (s *streamResult) containsSecret(check func([]byte) bool) bool {
	if check(s.canonicalFrame) || check([]byte(s.text)) || check([]byte(s.reasoning)) {
		return true
	}
	for _, call := range s.toolCalls {
		if check([]byte(call.ID)) || check([]byte(call.Name)) || check([]byte(call.Arguments)) || check(call.canonicalArguments) {
			return true
		}
	}
	return false
}

func (s *streamResult) feed(frame []byte) error {
	data, err := eventData(frame)
	if err != nil {
		return err
	}
	if data == nil {
		return nil
	}
	if s.done {
		return errors.New("UNSUPPORTED: data after DONE")
	}
	if string(data) == "[DONE]" {
		s.done = true
		return nil
	}
	v, err := decode(data)
	if err != nil {
		return err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return errors.New("invalid SSE JSON object")
	}
	s.canonicalFrame, _ = json.Marshal(m)
	if _, exists := m["error"]; exists {
		return errors.New("upstream stream error")
	}
	choices, ok := m["choices"].([]any)
	if !ok || len(choices) > 1 {
		return errors.New("UNSUPPORTED: SSE choices")
	}
	if len(choices) == 0 {
		return nil
	} // Usage-only frame.
	c, ok := choices[0].(map[string]any)
	if !ok || c["index"] != json.Number("0") {
		return errors.New("UNSUPPORTED: choice index")
	}
	if reason, ok := c["finish_reason"].(string); ok {
		if s.finish != "" || reason != "stop" && reason != "tool_calls" {
			return errors.New("UNSUPPORTED: finish reason")
		}
		s.finish = reason
	}
	delta, ok := c["delta"].(map[string]any)
	if !ok {
		return errors.New("UNSUPPORTED: SSE delta required")
	}
	if text, ok := delta["content"].(string); ok {
		s.text += text
	}
	for _, key := range []string{"reasoning_content", "reasoning", "reasoning_text"} {
		if text, ok := delta[key].(string); ok {
			s.reasoning += text
		}
	}
	if raw, exists := delta["tool_calls"]; exists {
		calls, ok := raw.([]any)
		if !ok {
			return errors.New("invalid tool calls")
		}
		for _, raw := range calls {
			call, ok := raw.(map[string]any)
			if !ok {
				return errors.New("invalid tool call")
			}
			index, ok := call["index"].(json.Number)
			i, err := strconv.Atoi(string(index))
			if !ok || err != nil || i < 0 || i >= maxToolCalls {
				return errors.New("UNSUPPORTED: tool index")
			}
			if s.toolCalls == nil {
				s.toolCalls = make(map[int]*streamCall)
			}
			current := s.toolCalls[i]
			if current == nil {
				current = &streamCall{}
				s.toolCalls[i] = current
			}
			if id, ok := call["id"].(string); ok && id != "" {
				if current.ID != "" && current.ID != id {
					return errors.New("UNSUPPORTED: changing tool ID")
				}
				current.ID = id
			}
			if typ, exists := call["type"]; exists && typ != "function" {
				return errors.New("UNSUPPORTED: tool type")
			}
			if fn, ok := call["function"].(map[string]any); ok {
				if name, ok := fn["name"].(string); ok {
					current.Name += name
				}
				if args, ok := fn["arguments"].(string); ok {
					current.Arguments += args
				}
			}
		}
	}
	return nil
}

func (s *streamResult) complete() error {
	if !s.done || s.finish == "" {
		return errors.New("incomplete SSE response")
	}
	if (s.finish == "tool_calls") != (len(s.toolCalls) > 0) {
		return errors.New("finish reason/tool mismatch")
	}
	seen := make(map[string]bool)
	for i := 0; i < len(s.toolCalls); i++ {
		call := s.toolCalls[i]
		if call == nil {
			return errors.New("non-contiguous tool indices")
		}
		if call.ID == "" || call.Name == "" || strings.Contains(call.ID, "|") || seen[call.ID] {
			return errors.New("UNSUPPORTED: tool ID or name")
		}
		seen[call.ID] = true
		args, err := decode([]byte(call.Arguments))
		if err != nil {
			return errors.New("invalid tool arguments")
		}
		if _, ok := args.(map[string]any); !ok {
			return errors.New("tool arguments must be an object")
		}
		call.canonicalArguments, _ = json.Marshal(args)
	}
	return nil
}

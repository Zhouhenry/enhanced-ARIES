package modelreplay

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

type requestDiagnostic struct {
	MessageCount        *int                       `json:"message_count"`
	Messages            []requestMessageDiagnostic `json:"messages,omitempty"`
	SuppliedIdentifiers map[string]string          `json:"supplied_identifiers"`
}

type requestMessageDiagnostic struct {
	Role        string                           `json:"role"`
	ContentKind string                           `json:"content_kind"`
	Text        map[string]requestTextDiagnostic `json:"text,omitempty"`
	ToolCallID  string                           `json:"tool_call_id,omitempty"`
	ToolCalls   []requestToolDiagnostic          `json:"tool_calls,omitempty"`
}

type requestToolDiagnostic struct {
	ID        string                 `json:"id,omitempty"`
	Arguments *requestTextDiagnostic `json:"arguments,omitempty"`
}

type requestTextDiagnostic struct {
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
}

func textDiagnostic(text string) requestTextDiagnostic {
	hash := sha256.Sum256([]byte(text))
	return requestTextDiagnostic{Bytes: len(text), SHA256: hex.EncodeToString(hash[:])}
}

// Called only after both raw and decoded credential checks. Do not retain the
// request map: observation writing is asynchronous and never needs its content.
func (p *requestObservation) replayDiverged(req map[string]any) {
	matchedAt := time.Now()
	var diagnostic *requestDiagnostic
	if p.server.boundaries != nil {
		diagnostic = summarizeRejectedRequest(req)
	}
	p.emit(boundaryEvent{Event: "request_matched", Scope: "request_history_and_trajectory_validation",
		Status: "failed", Reason: "REPLAY_DIVERGED", RequestDiagnostic: diagnostic}, matchedAt)
}

func summarizeRejectedRequest(req map[string]any) *requestDiagnostic {
	result := &requestDiagnostic{}
	// These are supplied fields, not verified native identities. Never infer
	// identity from a prompt, arbitrary metadata, or the model session identity.
	for _, key := range []string{"native_run_id", "session_id", "session_key"} {
		if value, ok := req[key].(string); ok && value != "" {
			if result.SuppliedIdentifiers == nil {
				result.SuppliedIdentifiers = make(map[string]string)
			}
			result.SuppliedIdentifiers[key] = value
		}
	}
	if messages, ok := req["messages"].([]any); ok {
		count := len(messages)
		result.MessageCount = &count
		for _, item := range messages {
			message, _ := item.(map[string]any)
			detail := requestMessageDiagnostic{Role: "unknown", ContentKind: "missing"}
			if role, ok := message["role"].(string); ok {
				switch role {
				case "system", "developer", "user", "assistant", "tool", "function":
					detail.Role = role
				}
			}
			if content, exists := message["content"]; exists {
				detail.ContentKind = "non_text"
				if content == nil {
					detail.ContentKind = "null"
				} else if _, ok := content.(string); ok {
					detail.ContentKind = "text"
				}
			}
			for _, key := range []string{"content", "reasoning_content"} {
				if value, ok := message[key].(string); ok {
					if detail.Text == nil {
						detail.Text = make(map[string]requestTextDiagnostic)
					}
					detail.Text[key] = textDiagnostic(value)
				}
			}
			detail.ToolCallID, _ = message["tool_call_id"].(string)
			calls, _ := message["tool_calls"].([]any)
			for _, item := range calls {
				call, _ := item.(map[string]any)
				function, _ := call["function"].(map[string]any)
				tool := requestToolDiagnostic{}
				tool.ID, _ = call["id"].(string)
				if arguments, ok := function["arguments"].(string); ok {
					summary := textDiagnostic(arguments)
					tool.Arguments = &summary
				}
				detail.ToolCalls = append(detail.ToolCalls, tool)
			}
			result.Messages = append(result.Messages, detail)
		}
	}
	return result
}

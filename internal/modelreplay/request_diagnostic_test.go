package modelreplay

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStrictRejectionDiagnosticPreservesReplayAndPrivacy(t *testing.T) {
	for _, completed := range []int{1, 3} {
		name := "history_mismatch"
		if completed == 3 {
			name = "after_tape_end"
		}
		t.Run(name, func(t *testing.T) {
			s := replayServer(t, name, fixtureTape(t), Timing{Mode: "zero"})
			for turn := 0; turn < completed; turn++ {
				if got := post(s, requestAt(turn)); got.Code != http.StatusOK || !bytes.Equal(got.Body.Bytes(), bytes.Join(framesAt(turn), nil)) {
					t.Fatal("successful strict replay changed")
				}
			}
			if got := post(s, requestAt(completed-1)); got.Code != http.StatusOK || !bytes.Equal(got.Body.Bytes(), bytes.Join(framesAt(completed-1), nil)) {
				t.Fatal("cached retry changed")
			}
			decoded, err := decode(requestAt(completed))
			if err != nil {
				t.Fatal(err)
			}
			req := decoded.(map[string]any)
			messages := req["messages"].([]any)
			const changedText = "changed earlier 历史"
			messages[0].(map[string]any)["content"] = changedText
			messages[2].(map[string]any)["reasoning_content"] = "private-reasoning-payload"
			req["native_run_id"], req["session_key"] = "native-run-observed", "agent:main:observed"
			req["metadata"] = map[string]any{"session_id": "do-not-infer-metadata", "token": "private-metadata-value"}
			raw, _ := json.Marshal(req)
			if got := post(s, raw); got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), "REPLAY_DIVERGED") {
				t.Fatal("strict mismatch response changed")
			}
			// This early rejection has no parsed body; do not invent a diagnostic.
			if got := post(s, raw); got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), "SESSION_FAILED") {
				t.Fatal("failed-session response changed")
			}
			if s.position != completed || s.Close() == nil {
				t.Fatal("diagnostic changed progress or session failure")
			}
			requests := assertBoundaryRequests(t, s, uint64(completed+3))
			diagnostics := 0
			for _, events := range requests {
				last := events[len(events)-1]
				if (last.Reason == "REPLAY_DIVERGED" && len(events) != 9) || (last.Reason == "SESSION_FAILED" && len(events) != 2) {
					t.Fatal("rejected request event denominator changed")
				}
				for _, event := range events {
					if event.RequestDiagnostic == nil {
						continue
					}
					diagnostics++
					if event.Event != "request_matched" || event.Status != "failed" || event.Reason != "REPLAY_DIVERGED" || event.CachedRetry {
						t.Fatal("diagnostic attached to a different decision")
					}
					detail := event.RequestDiagnostic
					if detail.MessageCount == nil || *detail.MessageCount != len(messages) || len(detail.Messages) != len(messages) {
						t.Fatal("message denominator changed")
					}
					if detail.Messages[0].Role != "system" || detail.Messages[1].Role != "user" || detail.Messages[2].Role != "assistant" || detail.Messages[3].Role != "tool" {
						t.Fatal("role order changed")
					}
					hash := sha256.Sum256([]byte(changedText))
					if got := detail.Messages[0].Text["content"]; got.Bytes != len(changedText) || got.SHA256 != hex.EncodeToString(hash[:]) {
						t.Fatal("text digest is not exact UTF-8 bytes")
					}
					if detail.Messages[2].ContentKind != "null" || detail.Messages[2].ToolCalls[0].ID != "call_0" || detail.Messages[3].ToolCallID != "call_0" || detail.Messages[2].ToolCalls[0].Arguments == nil {
						t.Fatal("tool call/result identity or argument summary missing")
					}
					if len(detail.SuppliedIdentifiers) != 2 || detail.SuppliedIdentifiers["native_run_id"] != "native-run-observed" || detail.SuppliedIdentifiers["session_key"] != "agent:main:observed" {
						t.Fatal("supplied identity was inferred or lost")
					}
				}
			}
			if diagnostics != 1 {
				t.Fatalf("want only first strict rejection diagnostic, got %d", diagnostics)
			}
			data, err := os.ReadFile(filepath.Join(s.options.OutputDir, "request-boundaries.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			for _, private := range []string{changedText, "private-metadata-value", "do-not-infer-metadata", "step0", "private-reasoning-payload"} {
				if bytes.Contains(data, []byte(private)) {
					t.Fatal("request content leaked into diagnostic")
				}
			}
		})
	}
}

func TestStrictRejectionDiagnosticDoesNotReadRejectedCredentials(t *testing.T) {
	s := replayServer(t, "credentials", fixtureTape(t), Timing{Mode: "zero"})
	raw := bytes.Replace(requestAt(0), []byte("private test system"), []byte(testToken), 1)
	if got := post(s, raw); got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), "CREDENTIAL_IN_PAYLOAD") {
		t.Fatal("credential rejection changed")
	}
	_ = s.Close()
	for _, events := range assertBoundaryRequests(t, s, 1) {
		for _, event := range events {
			if event.RequestDiagnostic != nil {
				t.Fatal("credential rejection must not summarize payload")
			}
		}
	}
}

func TestRejectedRequestDiagnosticKeepsUnsupportedShapeUnknown(t *testing.T) {
	for _, raw := range []string{
		`{"messages":"private unsupported value"}`,
		`{"messages":[null,{"role":"private role","content":[{"type":"image_url","image_url":{"url":"private url"}}],"tool_calls":[null]}]}`,
	} {
		parsed, err := decode([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		detail := summarizeRejectedRequest(parsed.(map[string]any))
		if detail.SuppliedIdentifiers != nil {
			t.Fatal("missing identities were invented")
		}
		if len(detail.Messages) == 0 && detail.MessageCount != nil {
			t.Fatal("unsupported messages counted as zero")
		}
		data, _ := json.Marshal(detail)
		if bytes.Contains(data, []byte("private")) {
			t.Fatal("unsupported values leaked")
		}
		if len(detail.Messages) > 0 && (detail.Messages[1].Role != "unknown" || detail.Messages[1].ContentKind != "non_text" || detail.Messages[1].Text != nil) {
			t.Fatal("unsupported message content was presented as text")
		}
	}
}

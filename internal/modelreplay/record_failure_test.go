package modelreplay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hyscale-lab/aries/pkg/performance"
)

func TestRecordFailureReasonPreservesFailureAndPrivatePayload(t *testing.T) {
	const private = "private-upstream-error-body"
	content := frame(map[string]any{"content": "prefix"}, nil)
	for _, tc := range []struct {
		name, reason string
		body         []byte
		flushFailed  bool
	}{
		{"finish-length", "SSE_UNSUPPORTED_FINISH_REASON", bytes.Join([][]byte{content, frame(map[string]any{}, "length"), []byte("data: [DONE]\n\n")}, nil), false},
		{"incomplete-read", "SSE_READ_INCOMPLETE", append(bytes.Clone(content), []byte("data: "+private)...), false},
		{"missing-done", "SSE_RESPONSE_INCOMPLETE", append(bytes.Clone(content), frame(map[string]any{}, "stop")...), false},
		{"frame-bound", "SSE_FRAME_OR_RESPONSE_BOUND", append(bytes.Clone(content), []byte("data: "+strings.Repeat("x", maxFrame)+"\n\n")...), false},
		{"upstream-error", "UPSTREAM_STREAM_ERROR", append(bytes.Clone(content), []byte("data: {\"error\":{\"message\":\""+private+"\"}}\n\n")...), false},
		{"unclassified-json", "RECORD_FAILURE_UNCLASSIFIED", append(bytes.Clone(content), []byte("data: "+private+"\n\n")...), false},
		{"downstream-flush", "DOWNSTREAM_WRITE_OR_FLUSH_FAILED", bytes.Join(framesAt(0), nil), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			recorder, err := performance.Open(root, "test-run")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = recorder.Close() })
			ctx := performance.WithIdentity(performance.WithRecorder(context.Background(), recorder), performance.Identity{OccurrenceID: "occ"})
			_, span := performance.Begin(ctx, "record.request")
			identity := testIdentity(tc.name)
			identity.AttemptID = "test-run/occ"
			if err := os.Mkdir(filepath.Join(root, "occ"), 0700); err != nil {
				t.Fatal(err)
			}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write(tc.body)
			}))
			defer upstream.Close()
			s, err := New(Options{Mode: "record", Identity: identity, Model: "test-model", AccessToken: []byte(testToken), UpstreamURL: upstream.URL, UpstreamKey: []byte(testUpstreamKey), OutputDir: filepath.Join(root, "occ", "model"), Clock: performance.ClockFromContext(ctx)})
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(requestAt(0)))
			r.Header.Set("Authorization", "Bearer "+testToken)
			w := httptest.NewRecorder()
			if tc.flushFailed {
				s.ServeHTTP(&failedFlushWriter{w}, r)
			} else {
				s.ServeHTTP(w, r)
			}
			if s.failed != "DELIVERY_OR_UPSTREAM_FAILED" || s.position != 0 || s.Close() == nil {
				t.Fatal("diagnostics changed failed-session semantics")
			}
			for _, request := range assertBoundaryRequests(t, s, 1) {
				last := request[len(request)-1]
				if last.Status != "failed" || last.Reason != tc.reason {
					t.Fatalf("failure boundary: %s/%s", last.Status, last.Reason)
				}
			}
			var tape Tape
			raw, err := os.ReadFile(filepath.Join(s.options.OutputDir, "tape.json"))
			if err != nil || json.Unmarshal(raw, &tape) != nil || tape.Complete || tape.Failure != "DELIVERY_OR_UPSTREAM_FAILED" || len(tape.Turns) != 1 {
				t.Fatal("partial Tape was promoted or rewritten")
			}
			for _, recorded := range tape.Turns[0].Frames {
				if bytes.Contains(recorded.Data, []byte(private)) {
					t.Fatal("rejected frame content was retained in the Tape")
				}
			}
			if bytes.Contains(raw, []byte(private)) || strings.Contains(w.Body.String(), private) {
				t.Fatal("rejected frame content was retained or delivered")
			}
			for _, name := range []string{"events.jsonl", "request-boundaries.jsonl"} {
				raw, err := os.ReadFile(filepath.Join(s.options.OutputDir, name))
				if err != nil || bytes.Contains(raw, []byte(private)) || bytes.Contains(raw, []byte(testUpstreamKey)) {
					t.Fatal("private error text in diagnostic events")
				}
				if !bytes.Contains(raw, []byte(`"reason":"`+tc.reason+`"`)) {
					t.Fatal("safe failure category missing from an existing event stream")
				}
			}
			span.End(errors.New("record failed"))
			if err := recorder.Close(); err != nil {
				t.Fatal(err)
			}
			run, err := json.Marshal(map[string]any{"run_id": identity.RunID, "attempts": []any{map[string]any{"attempt_id": identity.AttemptID, "occurrence_id": "occ", "model_session_id": identity.SessionID}}})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "run-result.json"), run, 0600); err != nil {
				t.Fatal(err)
			}
			snapshot, err := performance.BuildSnapshot(filepath.Join(root, "performance"))
			if err != nil {
				t.Fatal(err)
			}
			trace, err := performance.ProjectTrace(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			otlp, err := performance.ProjectOTLP(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			for name, projection := range map[string]any{"snapshot": snapshot, "perfetto": trace, "otlp": otlp.Request} {
				raw, err := json.Marshal(projection)
				if err != nil || !bytes.Contains(raw, []byte(tc.reason)) || bytes.Contains(raw, []byte(private)) || bytes.Contains(raw, []byte(testUpstreamKey)) {
					t.Fatalf("%s lost the safe failure reason or exposed private text", name)
				}
			}
		})
	}
}

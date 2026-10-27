package jiuwenswarm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/performance"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	return newPushTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/events/stream" {
			w.Header().Set("Content-Type", "text/event-stream")
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		handler(w, r)
	})
}

func newPushTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := NewClient(server.URL, http.Header{"Authorization": {"Bearer private-test-token"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return client
}

func emitFrame(w http.ResponseWriter, requestID, sessionID string, terminal bool) {
	frame := map[string]any{"protocol_version": "1.0", "request_id": requestID, "session_id": sessionID, "sequence": 0, "is_final": terminal, "status": "in_progress", "response_kind": "e2a.chunk", "body": map[string]any{"delta_kind": "text", "delta": "hello"}}
	if terminal {
		frame["status"] = "succeeded"
		frame["response_kind"] = "e2a.complete"
		frame["body"] = map[string]any{"result": map[string]any{}}
	}
	data, _ := json.Marshal(frame)
	fmt.Fprintf(w, "id: %s\r\nevent: %s\r\ndata: %s\r\n\r\n", requestID, frame["response_kind"], data)
}

func TestNativeSessionAndChat(t *testing.T) {
	var requests []string
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer private-test-token" || r.Header.Get("X-Channel-Id") != "web" {
			t.Error("missing scoped headers")
		}
		rid := r.Header.Get("X-Request-Id")
		switch r.URL.Path {
		case "/api/v1/sessions":
			var params map[string]any
			if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
				t.Error(err)
			}
			if params["mode"] != "agent" || params["project_id"] != "project-1" || params["create_token"] != rid {
				t.Error("incorrect native session parameters")
			}
			w.WriteHeader(201)
			fmt.Fprintf(w, `{"request_id":%q,"ok":true,"data":{"result":{"session_id":"native-session","prewarm_hit":false}}}`, rid)
		case "/api/v1/chat/completions":
			if r.Header.Get("Accept") != "text/event-stream" || r.Header.Get("X-Session-Id") != "native-session" {
				t.Error("wrong streaming identity")
			}
			var params map[string]any
			if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
				t.Error(err)
			}
			if params["query"] != "repair the real task" || params["mode"] != "agent" || params["enable_streaming"] != true {
				t.Error("chat parameters changed")
			}
			w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
			fmt.Fprint(w, ": ping\r\n\r\n")
			emitFrame(w, rid, "native-session", false)
			emitFrame(w, rid, "native-session", true)
		case "/api/v1/chat/native-session/actions/interrupt":
			fmt.Fprintf(w, `{"request_id":%q,"ok":true,"data":{"result":{}}}`, rid)
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	data, err := client.CreateSession(ctx, "create-1", "project-1")
	if err != nil || !strings.Contains(string(data), `"session_id":"native-session"`) {
		t.Fatalf("session: %s, %v", data, err)
	}
	var frames []Frame
	err = client.Run(ctx, "run-1", "native-session", "repair the real task", func(f Frame) { frames = append(frames, f) })
	if err != nil || len(frames) != 2 || !frames[1].IsFinal || !json.Valid(frames[0].Raw) {
		t.Fatalf("frames %d, error %v", len(frames), err)
	}
	if err := client.Interrupt(ctx, "stop-1", "native-session"); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 3 {
		t.Fatalf("unexpected request retries: %v", requests)
	}
}

func TestStreamFailuresAreNotSuccess(t *testing.T) {
	cases := []struct {
		name, data string
		want       error
	}{
		{"eof", `data: {"request_id":"r","status":"in_progress","body":{}}` + "\n\n", ErrIncomplete},
		{"paused", `data: {"request_id":"r","status":"in_progress","body":{"event_type":"chat.invocation_paused","awaiting_user_input":true}}` + "\n\n", ErrPaused},
		{"failed", `data: {"request_id":"r","status":"failed","is_final":true,"body":{"message":"private-test-token"}}` + "\n\n", ErrAgentFailed},
		{"native_chat_error_before_success", `data: {"request_id":"r","status":"in_progress","body":{"event_type":"chat.error","message":"private-test-token"}}` + "\n\n" + `data: {"request_id":"r","is_final":true,"status":"succeeded","response_kind":"e2a.complete","body":{"result":{}}}` + "\n\n", ErrAgentFailed},
		{"wrong_request", `data: {"request_id":"other","body":{}}` + "\n\n", nil},
		{"wrong_session", `data: {"request_id":"r","session_id":"other","body":{}}` + "\n\n", nil},
		{"invalid_json", "data: {\n\n", nil},
		{"truncated_frame", `data: {"request_id":"r","body":{}}`, nil},
		{"unknown_terminal", `data: {"request_id":"r","is_final":true,"status":"succeeded","response_kind":"ext","body":{}}` + "\n\n", ErrIncomplete},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, tc.data)
			})
			err := client.Run(context.Background(), "r", "s", "task", nil)
			if err == nil || tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("wanted %v, got %v", tc.want, err)
			}
			if strings.Contains(err.Error(), "private-test-token") {
				t.Fatal("native error content escaped")
			}
		})
	}
}

func TestNativeToolFailureCanBeFollowedBySuccessfulAgentCompletion(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"request_id":"r","status":"in_progress","body":{"event_type":"chat.tool_result","result":{"exit_code":7,"error":"tool failed"}}}`+"\n\n")
		emitFrame(w, "r", "s", true)
	})
	if err := client.Run(context.Background(), "r", "s", "task", nil); err != nil {
		t.Fatal("individual tool failure became a run-level failure", err)
	}
}

func TestHTTPFailureDoesNotRetryOrFollowRedirect(t *testing.T) {
	for _, status := range []int{302, 307, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			count := 0
			client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				count++
				w.Header().Set("Location", "/unexpected")
				w.WriteHeader(status)
				fmt.Fprint(w, "private-test-token")
			})
			err := client.Run(context.Background(), "r", "s", "task", nil)
			if err == nil || count != 1 || strings.Contains(err.Error(), "private-test-token") {
				t.Fatalf("count=%d error=%v", count, err)
			}
		})
	}
}

func TestCancellationClosesRequest(t *testing.T) {
	started := make(chan struct{})
	closed := make(chan struct{})
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		emitFrame(w, "r", "s", false)
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
		close(closed)
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- client.Run(ctx, "r", "s", "task", nil) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("client failed to cancel")
	}
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("HTTP request remained live")
	}
}

func TestConcurrentSessionsKeepTheirFrames(t *testing.T) {
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		arrived <- struct{}{}
		<-release
		w.Header().Set("Content-Type", "text/event-stream")
		emitFrame(w, r.Header.Get("X-Request-Id"), r.Header.Get("X-Session-Id"), true)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for _, id := range []string{"one", "two"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := client.Run(ctx, id, id, "task", func(f Frame) {
				if f.RequestID != id || f.SessionID != id {
					t.Error("cross-session frame")
				}
			}); err != nil {
				t.Error(err)
			}
		}()
	}
	for range 2 {
		select {
		case <-arrived:
		case <-ctx.Done():
			close(release)
			wg.Wait()
			t.Fatal("requests serialized")
		}
	}
	close(release)
	wg.Wait()
}

func TestSSEMultilineAndRetentionLimit(t *testing.T) {
	var raw string
	err := readFrames(strings.NewReader(": ping\r\nevent: e2a.chunk\r\ndata: {\r\ndata: \"body\":{}}\r\n\r\n"), func(b []byte) error { raw = string(b); return nil })
	if err != nil || !json.Valid([]byte(raw)) {
		t.Fatalf("%s %v", raw, err)
	}
	err = readFrames(strings.NewReader("data: "+strings.Repeat("x", maxFrameBytes)+"\n\n"), func([]byte) error { t.Error("oversized frame accepted"); return nil })
	if err == nil {
		t.Fatal("frame bound lost")
	}
}

func TestObservationUsesOneClockWithoutPayloads(t *testing.T) {
	root := t.TempDir()
	recorder, err := performance.Open(root, "jiuwen-observation")
	if err != nil {
		t.Fatal(err)
	}
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		emitFrame(w, "request-1", "session-1", false)
		emitFrame(w, "request-1", "session-1", true)
	})
	ctx := performance.WithRecorder(context.Background(), recorder)
	err = client.Run(ctx, "request-1", "session-1", "private task instruction", nil)
	closeErr := recorder.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("run=%v recorder=%v", err, closeErr)
	}
	raw, err := os.ReadFile(filepath.Join(root, "performance", "runner-events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "private task instruction") || strings.Contains(string(raw), "private-test-token") || strings.Contains(string(raw), "hello") {
		t.Fatal("payload copied to timing evidence")
	}
	want := []string{"jiuwen.chat.request_started", "jiuwen.chat.response_headers", "jiuwen.chat.first_sse", "jiuwen.chat.terminal_received", "jiuwen.chat.stream_drained"}
	var offset int64
	var clock string
	var observed []string
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var event performance.Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if clock == "" {
			clock = event.ClockDomain
		}
		if event.ClockDomain != clock || clock == "" || event.ObservedOffsetNS < offset {
			t.Fatal("timing clock/order mismatch")
		}
		offset = event.ObservedOffsetNS
		if event.SourceComponent != "jiuwenswarm" {
			continue
		}
		if event.RequestInstanceID != "request-1" || event.AgentSessionID != "session-1" || event.MeasurementScope != "controller_http_sse_consumption" {
			t.Fatal("observation identity/scope lost")
		}
		observed = append(observed, event.Operation)
	}
	if strings.Join(observed, ",") != strings.Join(want, ",") {
		t.Fatalf("boundaries %v", observed)
	}
}

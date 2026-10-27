package jiuwenswarm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func awaitPushTest(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("native stream did not reach the expected lifecycle boundary")
	}
}

func TestNativePushWireAndChatCallbacksAreSerialized(t *testing.T) {
	const count = 100
	const session = "session/+native"
	// build_server_push_wire retains the tool request ID; custom payload is body.delta.
	const wire = `{"protocol_version":"1.0","request_id":"tool-request","session_id":"session/+native","sequence":0,"is_final":false,"status":"in_progress","response_kind":"e2a.chunk","body":{"delta_kind":"custom","delta":{"event_type":"chat.file","files":[{"path":"/workspace/slides.pptx","name":"slides.pptx","size":1234,"mime_type":"application/vnd.openxmlformats-officedocument.presentationml.presentation","download_url":"/api/v1/files/download/fixture","download_token":"fixture-download-token","expires_at":1800000000}]},"event_type":"chat.file"},"metadata":{"_jiuwenswarm_server_push":true}}`
	registered, chatStarted, pushSeen, pushClosed := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	client := newPushTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if r.URL.Path == "/api/v1/events/stream" {
			defer close(pushClosed)
			if r.Method != http.MethodGet || r.URL.Query().Get("session_id") != session || r.Header.Get("X-Session-Id") != session || r.Header.Get("Accept") != "text/event-stream" || r.Header.Get("X-Jiuwen-Push-Consumer") != "" || r.Header.Get("Authorization") != "Bearer private-test-token" {
				t.Error("push subscription changed the native filtering or ordinary-consumer contract")
			}
			close(registered)
			w.(http.Flusher).Flush() // Ordinary native SSE sends no ready event.
			select {
			case <-chatStarted:
			case <-r.Context().Done():
				return
			}
			for range count {
				fmt.Fprintf(w, "event: e2a.chunk\ndata: %s\n\n", wire)
				w.(http.Flusher).Flush()
			}
			<-r.Context().Done()
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/chat/completions" {
			t.Error("unexpected chat request")
		}
		select {
		case <-registered:
		default:
			t.Error("chat was submitted before the push subscription")
		}
		close(chatStarted)
		for range count {
			emitFrame(w, "r", session, false)
			w.(http.Flusher).Flush()
		}
		select {
		case <-pushSeen:
		case <-r.Context().Done():
			return
		}
		emitFrame(w, "r", session, true)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var active atomic.Int32
	var frames []Frame // Deliberately not synchronized: Run owns callback serialization.
	pushCount := 0
	err := client.Run(ctx, "r", session, "task", func(frame Frame) {
		if active.Add(1) != 1 {
			t.Error("chat and push callbacks overlapped")
		}
		defer active.Add(-1)
		runtime.Gosched()
		frames = append(frames, frame)
		if frame.Source == "push" {
			if string(frame.Raw) != wire || frame.RequestID != "tool-request" || frame.ReceivedAt.IsZero() {
				t.Error("push bytes or identity were normalized")
			}
			pushCount++
			if pushCount == count {
				close(pushSeen)
			}
		} else if frame.Source != "chat" {
			t.Error("missing stream source")
		}
	})
	if err != nil || pushCount != count || len(frames) != count*2+1 {
		t.Fatalf("run=%v push=%d frames=%d", err, pushCount, len(frames))
	}
	awaitPushTest(t, pushClosed)
}

func TestPushCancellationAndDisconnectCloseBothStreams(t *testing.T) {
	for _, disconnect := range []bool{false, true} {
		t.Run(fmt.Sprint(disconnect), func(t *testing.T) {
			chatStarted, pushClosed, chatClosed := make(chan struct{}), make(chan struct{}), make(chan struct{})
			client := newPushTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.(http.Flusher).Flush()
				if r.URL.Path == "/api/v1/events/stream" {
					defer close(pushClosed)
					if disconnect {
						select {
						case <-chatStarted:
						case <-r.Context().Done():
						}
						return
					}
					<-r.Context().Done()
					return
				}
				defer close(chatClosed)
				close(chatStarted)
				<-r.Context().Done()
			})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- client.Run(ctx, "r", "s", "task", nil) }()
			awaitPushTest(t, chatStarted)
			if !disconnect {
				cancel()
			}
			select {
			case err := <-done:
				if disconnect && (err == nil || !strings.Contains(err.Error(), "push subscription ended")) || !disconnect && !errors.Is(err, context.Canceled) {
					t.Fatalf("disconnect=%v error=%v", disconnect, err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Run did not join its streams")
			}
			awaitPushTest(t, pushClosed)
			awaitPushTest(t, chatClosed)
		})
	}
}

func TestPushSubscriptionFailureDoesNotSubmitChat(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusOK} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var chats atomic.Int32
			client := newPushTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/events/stream" {
					chats.Add(1)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				fmt.Fprint(w, "private-test-token")
			})
			if err := client.Run(context.Background(), "r", "s", "task", nil); err == nil || chats.Load() != 0 || strings.Contains(err.Error(), "private-test-token") {
				t.Fatalf("chat submissions=%d error=%v", chats.Load(), err)
			}
		})
	}
}

func TestPushFrameDoesNotUseChatTerminalValidation(t *testing.T) {
	seen := make(chan struct{})
	client := newPushTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if r.URL.Path == "/api/v1/events/stream" {
			fmt.Fprint(w, "data: {\"protocol_version\":\"1.0\",\"request_id\":\"other-request\",\"session_id\":\"s\",\"is_final\":true,\"status\":\"succeeded\",\"response_kind\":\"e2a.push\",\"body\":{\"event_type\":\"chat.file\"}}\n\n")
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		select {
		case <-seen:
		case <-r.Context().Done():
			return
		}
		emitFrame(w, "r", "s", false)
		emitFrame(w, "r", "s", true)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Run(ctx, "r", "s", "task", func(frame Frame) {
		if frame.Source == "push" {
			close(seen)
		}
	}); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidPushFramesCancelRun(t *testing.T) {
	for _, raw := range []string{
		`{`,
		`{"session_id":"another-session","body":{}}`,
		`{"session_id":"s","protocol_version":"2.0","body":{}}`,
	} {
		t.Run(raw, func(t *testing.T) {
			client := newPushTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				if r.URL.Path == "/api/v1/events/stream" {
					fmt.Fprintf(w, "data: %s\n\n", raw)
				}
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := client.Run(ctx, "r", "s", "task", nil); err == nil || errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("invalid push did not stop Run: %v", err)
			}
		})
	}
}

func TestSSEAcceptsFileFramesAbovePreviousLimit(t *testing.T) {
	raw := `{"body":{"event_type":"chat.file","files":[{"data":"` + strings.Repeat("A", 9<<20) + `"}]}}`
	var got string
	if err := readFrames(strings.NewReader("data: "+raw+"\n\n"), func(data []byte) error {
		got = string(data)
		return nil
	}); err != nil || got != raw {
		t.Fatalf("larger native file frame was not preserved: %v", err)
	}
}

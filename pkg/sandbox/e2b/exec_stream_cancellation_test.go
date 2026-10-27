package e2b

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
)

type execStreamTransport func(*http.Request) (*http.Response, error)

func (f execStreamTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Exercise the real client RPC path without a provider or a TCP scheduling dependency.
func streamSandbox(transport execStreamTransport) *Sandbox {
	m := &Manager{options: Options{Endpoint: "http://adapter.invalid", Token: strings.Repeat("t", 32), TimeoutSeconds: 60},
		client: &http.Client{Transport: transport}}
	return &Sandbox{manager: m, id: "owned-test", workdir: "/workspace", user: "user"}
}

func streamResponse(status int, value any) (*http.Response, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body))}, nil
}

func streamStart(r *http.Request) (*http.Response, error) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, err
	}
	return streamResponse(200, map[string]any{"process": body["process"], "pid": 42})
}

func streamTerminal(exit int) (*http.Response, error) {
	return streamResponse(200, map[string]any{"cursor": 3, "done": true, "events": []any{
		map[string]any{"stream": "stdout", "data_base64": "AP8="},
		map[string]any{"stream": "stderr", "data_base64": "ZXJy"},
		map[string]any{"exit_code": exit, "guest_duration_ns": 123},
	}})
}

func TestExecStreamCompletedProcessDoesNotAcquireTeardownCancellation(t *testing.T) {
	// Keep EOF delivery in flight until Wait has read a genuine terminal. Repetition
	// explores the two ready goroutines; no sleep guesses when the terminal arrives.
	for i := 0; i < 512; i++ {
		inputEntered, inputReturned := make(chan struct{}), make(chan struct{})
		sandbox := streamSandbox(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/v1/start":
				return streamStart(r)
			case "/v1/stdin":
				close(inputEntered)
				<-r.Context().Done()
				defer close(inputReturned)
				return nil, r.Context().Err()
			case "/v1/events":
				select {
				case <-inputEntered:
				case <-r.Context().Done():
					return nil, r.Context().Err()
				}
				return streamTerminal(i % 2 * 7)
			default:
				return nil, errors.New("unexpected RPC")
			}
		})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		var stdout, stderr bytes.Buffer
		result, err := sandbox.ExecStream(ctx, core.Command{Path: "tool"}, nil, &stdout, &stderr)
		select {
		case <-inputReturned:
		case <-ctx.Done():
			t.Fatal("pending EOF was not canceled")
		}
		cancel()
		if err != nil || result.ExitCode != i%2*7 || !bytes.Equal(stdout.Bytes(), []byte{0, 255}) || stderr.String() != "err" {
			t.Fatalf("iteration %d: completed process changed by teardown: exit=%d err=%v stdout=%v stderr=%q", i, result.ExitCode, err, stdout.Bytes(), stderr.String())
		}
	}
}

type streamErrorReader struct{ err error }

func (r streamErrorReader) Read([]byte) (int, error) { return 0, r.err }

func TestExecStreamRealInputFailuresRemainErrors(t *testing.T) {
	brokenInput := errors.New("input source failed")
	for _, tc := range []struct {
		name    string
		reader  io.Reader
		want    error
		adapter bool
	}{
		{"read", streamErrorReader{brokenInput}, brokenInput, false},
		{"read_canceled", streamErrorReader{context.Canceled}, context.Canceled, false},
		{"adapter", nil, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sandbox := streamSandbox(func(r *http.Request) (*http.Response, error) {
				switch r.URL.Path {
				case "/v1/start":
					return streamStart(r)
				case "/v1/stdin":
					return streamResponse(500, map[string]string{"error_code": "input_failed"})
				case "/v1/events":
					<-r.Context().Done()
					return nil, r.Context().Err()
				default:
					return nil, errors.New("unexpected RPC")
				}
			})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, err := sandbox.ExecStream(ctx, core.Command{Path: "tool"}, tc.reader, io.Discard, io.Discard)
			if tc.adapter {
				var apiError *APIError
				if !errors.As(err, &apiError) || apiError.Code != "input_failed" {
					t.Fatalf("lost adapter failure: %v", err)
				}
			} else if !errors.Is(err, tc.want) {
				t.Fatalf("lost input failure: %v", err)
			}
		})
	}
}

func TestExecStreamExternalCancellationRemainsError(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(map[bool]string{false: "parent_cancel", true: "command_timeout"}[timeout], func(t *testing.T) {
			inputEntered, waitEntered, inputReturned := make(chan struct{}), make(chan struct{}), make(chan struct{})
			sandbox := streamSandbox(func(r *http.Request) (*http.Response, error) {
				switch r.URL.Path {
				case "/v1/start":
					return streamStart(r)
				case "/v1/stdin":
					close(inputEntered)
					<-r.Context().Done()
					defer close(inputReturned)
					return nil, r.Context().Err()
				case "/v1/events":
					close(waitEntered)
					<-r.Context().Done()
					return nil, r.Context().Err()
				default:
					return nil, errors.New("unexpected RPC")
				}
			})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			command := core.Command{Path: "tool"}
			want := error(context.Canceled)
			if timeout {
				command.Timeout = 50 * time.Millisecond
				want = context.DeadlineExceeded
			}
			done := make(chan error, 1)
			go func() { _, err := sandbox.ExecStream(ctx, command, nil, io.Discard, io.Discard); done <- err }()
			select {
			case <-inputEntered:
			case <-ctx.Done():
				t.Fatal("stdin not entered")
			}
			select {
			case <-waitEntered:
			case <-ctx.Done():
				t.Fatal("wait not entered")
			}
			if !timeout {
				cancel()
			}
			select {
			case err := <-done:
				if !errors.Is(err, want) {
					t.Fatalf("external cancellation lost: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("external cancellation did not finish")
			}
			select {
			case <-inputReturned:
			case <-time.After(5 * time.Second):
				t.Fatal("stdin not canceled")
			}
		})
	}
}

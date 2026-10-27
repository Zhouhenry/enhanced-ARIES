package modelreplay

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// Native clients may close their request as soon as the terminal frame flushes.
type terminalDeliveryWriter struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
	early  bool
	fail   bool
}

func (w *terminalDeliveryWriter) FlushError() error {
	if w.early || bytes.Contains(w.Body.Bytes(), []byte("data: [DONE]")) {
		if w.fail {
			return io.ErrClosedPipe
		}
		w.cancel()
	}
	w.ResponseRecorder.Flush()
	return nil
}

func TestTerminalDeliveryCancellationAfterCompleteFlush(t *testing.T) {
	for _, mode := range []string{"record", "replay", "cached_retry"} {
		t.Run(mode, func(t *testing.T) {
			var s *Server
			if mode == "replay" {
				s = replayServer(t, "terminal", fixtureTape(t), Timing{Mode: "zero"})
			} else {
				turn := 0
				s = admissionRecordServer(t, func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = w.Write(bytes.Join(framesAt(turn), nil))
					turn++
				})
			}
			for i := 0; i < 2; i++ {
				if post(s, requestAt(i)).Code != http.StatusOK {
					t.Fatal("prefix failed")
				}
			}
			if mode == "cached_retry" && post(s, requestAt(2)).Code != http.StatusOK {
				t.Fatal("initial final response failed")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(requestAt(2))).WithContext(ctx)
			r.Header.Set("Authorization", "Bearer "+testToken)
			w := &terminalDeliveryWriter{ResponseRecorder: httptest.NewRecorder(), cancel: cancel}
			s.ServeHTTP(w, r)
			if ctx.Err() == nil || !bytes.Equal(w.Body.Bytes(), bytes.Join(framesAt(2), nil)) {
				t.Fatal("fixture did not close after the exact final response")
			}
			if err := s.Close(); err != nil {
				t.Fatalf("completed delivery incorrectly failed: %v", err)
			}
			if mode != "replay" {
				tape, err := Load(filepath.Join(s.options.OutputDir, "tape.json"))
				if err != nil {
					t.Fatal(err)
				}
				replay := replayServer(t, "terminal-strict", tape, Timing{Mode: "zero"})
				for i := 0; i < 3; i++ {
					if got := post(replay, requestAt(i)); !bytes.Equal(got.Body.Bytes(), bytes.Join(framesAt(i), nil)) {
						t.Fatal("strict replay changed original frame bytes")
					}
				}
				if err := replay.Close(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestTerminalDeliveryStillRejectsCancellationAndFlushFailure(t *testing.T) {
	for _, mode := range []string{"record", "replay"} {
		for _, early := range []bool{true, false} {
			t.Run(mode+"/"+map[bool]string{true: "early_cancel", false: "terminal_flush_failure"}[early], func(t *testing.T) {
				s := replayServer(t, "failed-terminal", fixtureTape(t), Timing{Mode: "zero"})
				if mode == "record" {
					s = admissionRecordServer(t, func(w http.ResponseWriter, _ *http.Request) {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = w.Write(bytes.Join(framesAt(0), nil))
					})
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(requestAt(0))).WithContext(ctx)
				r.Header.Set("Authorization", "Bearer "+testToken)
				w := &terminalDeliveryWriter{ResponseRecorder: httptest.NewRecorder(), cancel: cancel, early: early, fail: !early}
				s.ServeHTTP(w, r)
				if s.failed == "" || s.position != 0 || s.Close() == nil {
					t.Fatal("incomplete delivery was promoted")
				}
			})
		}
	}
}

type terminalEOFBody struct {
	io.Reader
	beforeEOF func()
}

func (b *terminalEOFBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	if err == io.EOF {
		b.beforeEOF()
	}
	return n, err
}
func (*terminalEOFBody) Close() error { return nil }

type terminalRoundTripper struct{ body io.ReadCloser }

func (t terminalRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: t.body}, nil
}

func TestRecordWithholdsDoneUntilUpstreamEOF(t *testing.T) {
	s := admissionRecordServer(t, func(http.ResponseWriter, *http.Request) { t.Error("unexpected network call") })
	w := httptest.NewRecorder()
	prefix := bytes.Join(framesAt(2)[:2], nil)
	response := append(bytes.Join(framesAt(2), nil), []byte(": legal trailing comment\n\n")...)
	reachedEOF := false
	s.client = &http.Client{Transport: terminalRoundTripper{&terminalEOFBody{
		Reader: bytes.NewReader(response),
		beforeEOF: func() {
			reachedEOF = true
			if !bytes.Equal(w.Body.Bytes(), prefix) {
				t.Error("nonterminal frames must stream, but DONE must wait for verified EOF")
			}
		},
	}}}
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(requestAt(0)))
	r.Header.Set("Authorization", "Bearer "+testToken)
	s.ServeHTTP(w, r)
	if !reachedEOF || !bytes.Equal(w.Body.Bytes(), response) {
		t.Fatal("response tail was not checked and preserved")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	tape, err := Load(filepath.Join(s.options.OutputDir, "tape.json"))
	if err != nil {
		t.Fatal(err)
	}
	replay := replayServer(t, "terminal-comment", tape, Timing{Mode: "zero"})
	if got := post(replay, requestAt(0)); !bytes.Equal(got.Body.Bytes(), response) {
		t.Fatal("legal tail comment lost during Strict replay")
	}
	if err := replay.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRecordRejectsInvalidTailBeforeSendingDone(t *testing.T) {
	for _, tail := range []string{"data: {}\n\n", "data: truncated", ": partial comment"} {
		t.Run(tail, func(t *testing.T) {
			s := admissionRecordServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write(append(bytes.Join(framesAt(2), nil), []byte(tail)...))
			})
			w := post(s, requestAt(0))
			if strings.Contains(w.Body.String(), "[DONE]") || s.Close() == nil {
				t.Fatal("invalid upstream tail was delivered as a complete response")
			}
			if _, err := Load(filepath.Join(s.options.OutputDir, "tape.json")); err == nil {
				t.Fatal("partial recording became replayable")
			}
		})
	}
}

package modelreplay

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func admissionRecordServer(t *testing.T, handler http.HandlerFunc) *Server {
	t.Helper()
	upstream := httptest.NewServer(handler)
	t.Cleanup(upstream.Close)
	s, err := New(Options{Mode: "record", Identity: testIdentity("admission"), Model: "test-model",
		AccessToken: []byte(testToken), UpstreamKey: []byte(testUpstreamKey), UpstreamURL: upstream.URL,
		OutputDir: filepath.Join(t.TempDir(), "model")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestCloseAdmissionPreservesCompletedRecordAndReplay(t *testing.T) {
	for _, mode := range []string{"record", "replay"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			var s *Server
			if mode == "record" {
				s = admissionRecordServer(t, func(w http.ResponseWriter, _ *http.Request) {
					turn := int(calls.Add(1)) - 1
					w.Header().Set("Content-Type", "text/event-stream")
					for _, data := range framesAt(turn) {
						_, _ = w.Write(data)
					}
				})
			} else {
				s = replayServer(t, "admission", fixtureTape(t), Timing{Mode: "zero"})
			}
			for i := 0; i < 3; i++ {
				if response := post(s, requestAt(i)); response.Code != http.StatusOK {
					t.Fatalf("original request failed: %d %s", response.Code, response.Body)
				}
			}
			before, _ := json.Marshal(s.tape.Turns)
			s.CloseAdmission()
			closedAt := *s.boundaries.index.AdmissionClosedOffsetNS
			s.CloseAdmission()
			if *s.boundaries.index.AdmissionClosedOffsetNS != closedAt {
				t.Fatal("idempotent close changed its actual boundary")
			}
			response := post(s, requestAt(3))
			if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "ADMISSION_CLOSED") {
				t.Fatalf("late request not rejected at admission: %d %s", response.Code, response.Body)
			}
			after, _ := json.Marshal(s.tape.Turns)
			if !bytes.Equal(before, after) || s.position != 3 || s.failed != "" || mode == "record" && calls.Load() != 3 {
				t.Fatal("late request altered the completed trajectory or called upstream")
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			requests := assertBoundaryRequests(t, s, 4)
			late := 0
			for _, events := range requests {
				terminal := events[len(events)-1]
				if terminal.Reason != "ADMISSION_CLOSED" {
					continue
				}
				late++
				if terminal.OffsetNS < closedAt || terminal.Status != "rejected" || terminal.Turn != nil || len(events) != 2 {
					t.Fatal("late request gained a model turn or lost its real rejection boundary")
				}
			}
			if late != 1 {
				t.Fatal("late request evidence missing")
			}
			if mode == "record" {
				data, err := os.ReadFile(filepath.Join(s.options.OutputDir, "tape.json"))
				var tape Tape
				if err != nil || json.Unmarshal(data, &tape) != nil || !tape.Complete || tape.Failure != "" || len(tape.Turns) != 3 {
					t.Fatal("completed record was poisoned during teardown")
				}
			}
		})
	}
}

type admissionBlockedBody struct {
	io.Reader
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *admissionBlockedBody) Read(data []byte) (int, error) {
	b.once.Do(func() { close(b.entered); <-b.release })
	return b.Reader.Read(data)
}

func (*admissionBlockedBody) Close() error { return nil }

func TestCloseAdmissionKeepsAlreadyAdmittedSuccessAndFailure(t *testing.T) {
	for _, mode := range []string{"record", "replay"} {
		for _, changed := range []bool{false, true} {
			name := mode + "/success"
			if changed {
				name = mode + "/strict_failure"
			}
			t.Run(name, func(t *testing.T) {
				var calls atomic.Int32
				var s *Server
				if mode == "record" {
					s = admissionRecordServer(t, func(w http.ResponseWriter, _ *http.Request) {
						i := int(calls.Add(1)) - 1
						w.Header().Set("Content-Type", "text/event-stream")
						for _, data := range framesAt(i) {
							_, _ = w.Write(data)
						}
					})
				} else {
					s = replayServer(t, "admitted", fixtureTape(t), Timing{Mode: "zero"})
				}
				for i := 0; i < 2; i++ {
					if post(s, requestAt(i)).Code != http.StatusOK {
						t.Fatal("fixture prefix failed")
					}
				}
				raw := requestAt(2)
				if changed {
					raw = bytes.Replace(raw, []byte("private test system"), []byte("changed old history"), 1)
				}
				body := &admissionBlockedBody{Reader: bytes.NewReader(raw), entered: make(chan struct{}), release: make(chan struct{})}
				request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
				request.Body = body
				request.Header.Set("Authorization", "Bearer "+testToken)
				response := httptest.NewRecorder()
				done := make(chan struct{})
				go func() { defer close(done); s.ServeHTTP(response, request) }()
				<-body.entered
				s.CloseAdmission()
				late := post(s, requestAt(3))
				close(body.release)
				<-done
				if late.Code != http.StatusConflict || !strings.Contains(late.Body.String(), "ADMISSION_CLOSED") {
					t.Fatal("new request admitted while an earlier request drained")
				}
				err := s.Close()
				if changed {
					want := "UNSUPPORTED"
					if mode == "replay" {
						want = "REPLAY_DIVERGED"
					}
					if response.Code != http.StatusConflict || s.failed != want || err == nil {
						t.Fatalf("admitted history failure was erased: %d %s %v", response.Code, s.failed, err)
					}
				} else if response.Code != http.StatusOK || err != nil || s.position != 3 {
					t.Fatalf("admitted request was canceled by admission close: %d %v", response.Code, err)
				}
				requests := assertBoundaryRequests(t, s, 4)
				closedAt := *s.boundaries.index.AdmissionClosedOffsetNS
				crossed := false
				for _, events := range requests {
					for _, event := range events {
						if event.Event == "request_admitted" && event.OffsetNS > closedAt {
							t.Fatal("admission boundary moved past close")
						}
						if event.Event == "request_body_read_started" && event.Turn != nil && *event.Turn == 2 {
							crossed = event.OffsetNS <= closedAt && events[len(events)-1].OffsetNS >= closedAt
						}
					}
				}
				if !crossed {
					t.Fatal("fixture did not cross the real admission-close boundary")
				}
			})
		}
	}
}

func TestCloseAdmissionDoesNotEraseEarlierFailureOrCompleteMissingTurns(t *testing.T) {
	for _, failFirst := range []bool{false, true} {
		s := replayServer(t, "incomplete", fixtureTape(t), Timing{Mode: "zero"})
		if post(s, requestAt(0)).Code != http.StatusOK {
			t.Fatal("fixture prefix failed")
		}
		if failFirst {
			if post(s, requestAt(2)).Code != http.StatusConflict {
				t.Fatal("strict mismatch accepted")
			}
		}
		before := s.failed
		s.CloseAdmission()
		_ = post(s, requestAt(1))
		if s.failed != before || s.position != 1 || s.Close() == nil {
			t.Fatal("admission close erased failure or completed an interrupted trajectory")
		}
	}
}

func TestFinalResponseDoesNotCloseAdmissionByItself(t *testing.T) {
	s := admissionRecordServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, data := range framesAt(2) {
			_, _ = w.Write(data)
		}
	})
	if post(s, requestAt(0)).Code != http.StatusOK {
		t.Fatal("fixture final response failed")
	}
	response := post(s, requestAt(1))
	if response.Code != http.StatusConflict || s.failed != "UNSUPPORTED" || s.admissionClosed {
		t.Fatal("final SSE response hid an unsupported continuation while the Harness was active")
	}
	s.CloseAdmission()
	if s.failed != "UNSUPPORTED" || s.Close() == nil {
		t.Fatal("Run terminal boundary erased a real earlier protocol failure")
	}
}

package modelreplay

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func branchRootRequest(turn int) []byte {
	raw := bytes.ReplaceAll(multiRequest(turn, false), []byte(`"exec"`), []byte(`"task_tool"`))
	v, _ := decode(raw)
	req := v.(map[string]any)
	req["messages"] = append(req["messages"].([]any), map[string]any{"role": "user", "content": jiuwenAttachmentPrefix + "runtime\">\nCurrent mode: agent\n</prompt-attachment>\n</system-reminder>"})
	raw, _ = json.Marshal(req)
	return raw
}

func branchRootResponse(turn int) []byte {
	if turn == 1 {
		return bytes.Join(framesAt(2), nil)
	}
	return bytes.ReplaceAll(bytes.Join(multiFrames(0), nil), []byte(`"exec"`), []byte(`"task_tool"`))
}

func branchPost(b *Bundle, branch string, raw []byte) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(raw))
	r.Header.Set("Authorization", "Bearer "+testToken)
	r.Header.Set(BranchHeader, branch)
	w := httptest.NewRecorder()
	b.ServeHTTP(w, r)
	return w
}

func TestBundleConcurrentJSONBranchesOneTapeAndStrictReplay(t *testing.T) {
	jsonBodies := [][]byte{jsonCompletionAt(t, 0), jsonCompletionAt(t, 1), jsonCompletionAt(t, 2)}
	var calls atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		raw, _ := io.ReadAll(r.Body)
		if r.Header.Get(BranchHeader) != "" {
			t.Error("private branch metadata leaked upstream")
		}
		for i := 0; i < 2; i++ {
			if bytes.Equal(raw, branchRootRequest(i)) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write(branchRootResponse(i))
				return
			}
		}
		for i := 0; i < 3; i++ {
			if bytes.Equal(raw, jsonRequestAt(i)) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(jsonBodies[i])
				return
			}
		}
		t.Error("unexpected upstream body")
		w.WriteHeader(400)
	}))
	defer up.Close()
	id := testIdentity("branches")
	id.HarnessVersion = jiuwenFixedTailHarnessVersion
	options := Options{Mode: "record", Identity: id, HistoryDialect: JiuwenHistoryDialect, Model: "test-model", AccessToken: []byte(testToken), UpstreamURL: up.URL, UpstreamKey: []byte(testUpstreamKey), OutputDir: filepath.Join(t.TempDir(), "record")}
	b, err := NewBundle(options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	run := func(b *Bundle, skip bool) {
		if w := branchPost(b, "main", branchRootRequest(0)); w.Code != 200 || !bytes.Equal(w.Body.Bytes(), branchRootResponse(0)) {
			t.Fatalf("root request failed: %d", w.Code)
		}
		var wg sync.WaitGroup
		for _, branch := range []string{"main/call_0_0", "main/call_0_1"} {
			if skip && branch == "main/call_0_1" {
				continue
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < 3; i++ {
					for retry := 0; retry < 2; retry++ {
						w := branchPost(b, branch, jsonRequestAt(i))
						if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), jsonBodies[i]) {
							t.Errorf("child %s turn %d status %d", branch, i, w.Code)
							return
						}
					}
				}
			}()
		}
		wg.Wait()
		if w := branchPost(b, "main", branchRootRequest(1)); w.Code != 200 {
			t.Fatalf("root completion: %d", w.Code)
		}
	}
	run(b, false)
	b.CloseAdmission()
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 8 {
		t.Fatal("child retry reached upstream")
	}
	if len(assertBoundaryRequests(t, b.root, 14)) != 14 {
		t.Fatal("child request denominator lost")
	}
	tape, err := Load(filepath.Join(options.OutputDir, "tape.json"))
	if err != nil {
		t.Fatal(err)
	}
	if tape.Version != bundleVersion || len(tape.Children) != 2 || tape.SessionProtocol() != BranchProtocol {
		t.Fatal("missing immutable child tapes")
	}
	for _, child := range tape.Children {
		if len(child.Turns) != 3 || !child.Complete {
			t.Fatal("incomplete child")
		}
	}
	options.Mode, options.Tape = "replay", tape
	options.Timing = Timing{Mode: "zero"}
	options.UpstreamKey = nil
	options.UpstreamURL = ""
	options.OutputDir = filepath.Join(t.TempDir(), "wrong-entry")
	if _, err := New(options); err == nil {
		t.Fatal("root-only replay accepted a branch tape")
	}
	for _, kind := range []string{"complete", "unused", "changed-history", "unbound"} {
		t.Run(kind, func(t *testing.T) {
			o := options
			o.OutputDir = filepath.Join(t.TempDir(), "replay")
			r, err := NewBundle(o)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = r.Close() })
			if kind == "complete" || kind == "unused" {
				run(r, kind == "unused")
			} else {
				if w := branchPost(r, "main", branchRootRequest(0)); w.Code != 200 {
					t.Fatal("root failed")
				}
				if kind == "unbound" {
					if w := branchPost(r, "main/not-a-tool-call", jsonRequestAt(0)); w.Code != 409 {
						t.Fatal("unbound child accepted")
					}
				} else {
					for i := 0; i < 2; i++ {
						if w := branchPost(r, "main/call_0_0", jsonRequestAt(i)); w.Code != 200 {
							t.Fatal("child failed")
						}
					}
					changed := bytes.Replace(jsonRequestAt(2), []byte("result-0"), []byte("different-result"), 1)
					if w := branchPost(r, "main/call_0_0", changed); w.Code != 409 {
						t.Fatal("old child history difference ignored")
					}
				}
			}
			if err := r.Close(); (err == nil) != (kind == "complete") {
				t.Fatalf("wrong completion for %s: %v", kind, err)
			}
		})
	}
	if tape.SHA256 != tapeDigest(*tape) || calls.Load() != 8 {
		t.Fatal("replay modified tape or called upstream")
	}
}

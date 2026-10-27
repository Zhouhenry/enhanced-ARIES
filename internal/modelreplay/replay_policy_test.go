package modelreplay

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func policyServer(t *testing.T, id, policy string, tape *Tape) *Server {
	t.Helper()
	s, err := New(Options{Mode: "replay", ReplayPolicy: policy, Identity: testIdentity(id),
		AccessToken: []byte(testToken), Tape: tape, OutputDir: filepath.Join(t.TempDir(), "session"), Timing: Timing{Mode: "zero"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func policyRequest(t *testing.T, turn int, change func(map[string]any, []any)) []byte {
	t.Helper()
	value, err := decode(requestAt(turn))
	if err != nil {
		t.Fatal(err)
	}
	req := value.(map[string]any)
	change(req, req["messages"].([]any))
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestWorkloadReplayRetainsHistoricalDifferencesAndExactRetry(t *testing.T) {
	tape := fixtureTape(t)
	original, _ := json.Marshal(tape)
	s := policyServer(t, "workload", ReplayPolicyWorkload, tape)
	const first = "actual first result 时间=08:01 pid=421"
	const older = "actual older result 时间=08:02 pid=422"
	const failedTool = "command not found; actual exit=127"
	requests := [][]byte{requestAt(0), policyRequest(t, 1, func(_ map[string]any, ms []any) {
		ms[3].(map[string]any)["content"] = first
	}), policyRequest(t, 2, func(_ map[string]any, ms []any) {
		ms[3].(map[string]any)["content"] = older
		ms[5].(map[string]any)["content"] = failedTool
	})}
	for i, request := range requests {
		unchanged := bytes.Clone(request)
		if w := post(s, request); w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), bytes.Join(framesAt(i), nil)) {
			t.Fatalf("workload turn %d did not replay the original frames", i)
		}
		if !bytes.Equal(request, unchanged) {
			t.Fatal("actual tool failure/output request was rewritten")
		}
		if i == 1 {
			if w := post(s, request); w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), bytes.Join(framesAt(i), nil)) || s.position != 2 {
				t.Fatal("identical actual-request retry advanced the trajectory")
			}
		}
	}
	if s.position != 3 || s.Close() != nil {
		t.Fatal("workload model session did not complete")
	}
	after, _ := json.Marshal(tape)
	internal, _ := json.Marshal(s.tape)
	if !bytes.Equal(original, after) || !bytes.Equal(original, internal) {
		t.Fatal("workload policy changed the recorded source")
	}
	assertBoundaryRequests(t, s, 4)
	events, index := readBoundaries(t, s)
	if index.ReplayPolicy != ReplayPolicyWorkload || s.ReplayPolicy() != ReplayPolicyWorkload {
		t.Fatal("workload session policy missing")
	}
	var matches []boundaryEvent
	for _, e := range events {
		if e.ReplayPolicy != ReplayPolicyWorkload {
			t.Fatal("boundary mislabeled as strict/default")
		}
		if e.Event == "request_matched" && e.Status == "succeeded" {
			matches = append(matches, e)
		}
	}
	if len(matches) != 4 || len(matches[0].ToolContentDifferences) != 0 || len(matches[1].ToolContentDifferences) != 1 || !matches[2].CachedRetry || len(matches[3].ToolContentDifferences) != 2 {
		t.Fatal("missing historical or retry difference evidence")
	}
	for _, check := range []struct {
		match, diff            int
		path, recorded, actual string
	}{
		{1, 0, "/messages/3/content", "result-0", first},
		{2, 0, "/messages/3/content", "result-0", first},
		{3, 0, "/messages/3/content", "result-0", older},
		{3, 1, "/messages/5/content", "result-1", failedTool},
	} {
		got := matches[check.match].ToolContentDifferences[check.diff]
		if got.SchemaVersion != 1 || got.Path != check.path || got.Encoding != "utf-8" || got.Recorded != textDiagnostic(check.recorded) || got.Actual != textDiagnostic(check.actual) {
			t.Fatal("difference evidence does not describe the original UTF-8 bytes")
		}
	}
	for _, e := range readObservations(t, s) {
		if e.ReplayPolicy != ReplayPolicyWorkload {
			t.Fatal("legacy/session evidence lost policy")
		}
	}
	for _, name := range []string{"events.jsonl", "request-boundaries.jsonl", "request-boundaries-index.json"} {
		b, err := os.ReadFile(filepath.Join(s.options.OutputDir, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, body := range []string{first, older, failedTool} {
			if bytes.Contains(b, []byte(body)) {
				t.Fatal("allowed tool body leaked into evidence")
			}
		}
	}
}

func TestWorkloadReplayRejectsEveryNonTextBodyChange(t *testing.T) {
	for _, field := range []string{"old_arguments", "old_call_id", "old_call_name", "old_result_id", "old_assistant", "old_reasoning", "system", "user", "role", "order", "missing_result", "extra_result", "missing_content", "null_content", "object_content", "array_content", "extra_tool_field", "schema", "parameter", "model", "stream"} {
		t.Run(field, func(t *testing.T) {
			s := policyServer(t, field, ReplayPolicyWorkload, fixtureTape(t))
			for i := 0; i < 2; i++ {
				if post(s, requestAt(i)).Code != http.StatusOK {
					t.Fatal("setup replay failed")
				}
			}
			raw := policyRequest(t, 2, func(req map[string]any, ms []any) {
				ms[3].(map[string]any)["content"] = "allowed old body difference"
				call := ms[2].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)
				switch field {
				case "old_arguments":
					call["function"].(map[string]any)["arguments"] = `{"command":"changed"}`
				case "old_call_id":
					call["id"] = "changed"
				case "old_call_name":
					call["function"].(map[string]any)["name"] = "changed"
				case "old_result_id":
					ms[3].(map[string]any)["tool_call_id"] = "changed"
				case "old_assistant":
					ms[2].(map[string]any)["content"] = "changed"
				case "old_reasoning":
					ms[2].(map[string]any)["reasoning_content"] = "changed"
				case "system":
					ms[0].(map[string]any)["content"] = "changed"
				case "user":
					ms[1].(map[string]any)["content"] = "changed"
				case "role":
					ms[3].(map[string]any)["role"] = "user"
				case "order":
					ms[3], ms[5] = ms[5], ms[3]
				case "missing_result":
					req["messages"] = ms[:len(ms)-1]
				case "extra_result":
					req["messages"] = append(ms, ms[5])
				case "missing_content":
					delete(ms[5].(map[string]any), "content")
				case "null_content":
					ms[5].(map[string]any)["content"] = nil
				case "object_content":
					ms[5].(map[string]any)["content"] = map[string]any{"text": "changed"}
				case "array_content":
					ms[5].(map[string]any)["content"] = []any{"changed"}
				case "extra_tool_field":
					ms[5].(map[string]any)["status"] = "success"
				case "schema":
					req["tools"] = []any{}
				case "parameter":
					req["temperature"] = 1
				case "model":
					req["model"] = "changed"
				case "stream":
					req["stream"] = false
				}
			})
			w := post(s, raw)
			if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "REPLAY_DIVERGED") || s.position != 2 {
				t.Fatal("workload policy accepted a non-body trajectory change")
			}
			if post(s, requestAt(2)).Code != http.StatusConflict || s.Close() == nil {
				t.Fatal("failed workload session recovered silently")
			}
			events, _ := readBoundaries(t, s)
			for _, e := range events {
				if len(e.ToolContentDifferences) != 0 {
					t.Fatal("rejected request labeled as accepted body differences")
				}
			}
		})
	}
}

func TestWorkloadReplayPreservesParallelToolResultOrder(t *testing.T) {
	tape := multiTape(t, false)
	for _, reorder := range []bool{false, true} {
		s := policyServer(t, "parallel-results", ReplayPolicyWorkload, tape)
		if post(s, multiRequest(0, false)).Code != http.StatusOK {
			t.Fatal("first request")
		}
		v, _ := decode(multiRequest(1, false))
		req := v.(map[string]any)
		ms := req["messages"].([]any)
		ms[3].(map[string]any)["content"] = "actual parallel output one"
		ms[4].(map[string]any)["content"] = "actual parallel output two"
		if reorder {
			ms[3], ms[4] = ms[4], ms[3]
		}
		raw, _ := json.Marshal(req)
		response := post(s, raw)
		want := http.StatusOK
		if reorder {
			want = http.StatusConflict
		}
		if response.Code != want {
			t.Fatal("parallel result order was loosened or content rejected")
		}
	}
}

func TestReplayPolicyDefaultsStrictAndRequiresReplay(t *testing.T) {
	for _, policy := range []string{"", ReplayPolicyStrict} {
		s := policyServer(t, "strict", policy, fixtureTape(t))
		if s.ReplayPolicy() != ReplayPolicyStrict || post(s, requestAt(0)).Code != http.StatusOK {
			t.Fatal("strict default changed")
		}
		changed := policyRequest(t, 1, func(_ map[string]any, ms []any) { ms[3].(map[string]any)["content"] = "different" })
		if post(s, changed).Code != http.StatusConflict || s.Close() == nil {
			t.Fatal("strict accepted changed body")
		}
		events, index := readBoundaries(t, s)
		if index.ReplayPolicy != ReplayPolicyStrict {
			t.Fatal("default strict not explicit in index")
		}
		for _, e := range events {
			if e.ReplayPolicy != ReplayPolicyStrict {
				t.Fatal("default strict not explicit in event")
			}
		}
	}
	for _, mode := range []string{"record", "replay"} {
		for _, policy := range []string{ReplayPolicyStrict, ReplayPolicyWorkload, "automatic"} {
			if mode == "replay" && policy != "automatic" {
				continue
			}
			_, err := New(Options{Mode: mode, ReplayPolicy: policy, Identity: testIdentity("invalid"), AccessToken: []byte(testToken),
				Tape: fixtureTape(t), OutputDir: filepath.Join(t.TempDir(), "invalid"), Timing: Timing{Mode: "zero"}})
			if err == nil || !strings.Contains(err.Error(), "replay_policy") {
				t.Fatal("invalid policy mode accepted or not diagnosed")
			}
		}
	}
}

func TestWorkloadReplayDifferentActualRequestIsNotCachedRetry(t *testing.T) {
	s := policyServer(t, "not-a-retry", ReplayPolicyWorkload, fixtureTape(t))
	if post(s, requestAt(0)).Code != http.StatusOK || post(s, requestAt(1)).Code != http.StatusOK {
		t.Fatal("setup")
	}
	changed := policyRequest(t, 1, func(_ map[string]any, ms []any) { ms[3].(map[string]any)["content"] = "new body on previous request" })
	if post(s, changed).Code != http.StatusConflict || s.position != 2 || s.Close() == nil {
		t.Fatal("non-identical request treated as cached retry")
	}
	for _, events := range assertBoundaryRequests(t, s, 3) {
		last := events[len(events)-1]
		if last.Reason == "REPLAY_DIVERGED" && last.CachedRetry {
			t.Fatal("workload comparison used as retry key")
		}
	}
}

func TestWorkloadReplayKeepsDeliveryAndCancellationFailures(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		s := policyServer(t, "delivery", ReplayPolicyWorkload, fixtureTape(t))
		ctx, cancel := context.WithCancel(context.Background())
		if canceled {
			cancel()
		}
		defer cancel()
		r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(requestAt(0))).WithContext(ctx)
		r.Header.Set("Authorization", "Bearer "+testToken)
		if canceled {
			s.ServeHTTP(httptest.NewRecorder(), r)
		} else {
			s.ServeHTTP(&failedFlushWriter{httptest.NewRecorder()}, r)
		}
		want := "DELIVERY_OR_UPSTREAM_FAILED"
		if canceled {
			want = "CANCELED_OR_TIMEOUT"
		}
		if s.failed != want || s.position != 0 || s.Close() == nil {
			t.Fatal("actual failure became workload success")
		}
	}
}

func TestWorkloadAndStrictSessionsStayIsolated(t *testing.T) {
	tape := fixtureTape(t)
	a := policyServer(t, "workload-a", ReplayPolicyWorkload, tape)
	b := policyServer(t, "workload-b", ReplayPolicyWorkload, tape)
	strict := policyServer(t, "strict-c", ReplayPolicyStrict, tape)
	var wg sync.WaitGroup
	for _, s := range []*Server{a, b, strict} {
		wg.Add(1)
		go func(s *Server) {
			defer wg.Done()
			for i := 0; i < 3; i++ {
				raw := policyRequest(t, i, func(_ map[string]any, ms []any) {
					for _, item := range ms {
						m := item.(map[string]any)
						if m["role"] == "tool" {
							m["content"] = s.options.Identity.SessionID + " actual"
						}
					}
				})
				got := post(s, raw)
				if s == strict && i == 1 {
					if got.Code != http.StatusConflict {
						t.Error("strict inherited workload policy")
					}
					return
				}
				if got.Code != http.StatusOK {
					t.Error("independent workload session failed")
					return
				}
			}
		}(s)
	}
	wg.Wait()
	if a.Close() != nil || b.Close() != nil || strict.Close() == nil {
		t.Fatal("session outcomes mixed")
	}
}

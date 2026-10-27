package e2b

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/performance"
)

func nativeIdentity(invocation string) performance.NativeToolIdentity {
	return performance.NativeToolIdentity{SchemaVersion: 1, ScopeID: "scope", AgentSessionID: "session", ToolCallID: "tool", NativeInvocationID: invocation, SSHRequestID: "ssh-" + invocation}
}

func nativeContext(invocation string) context.Context {
	return performance.WithNativeToolIdentity(performance.WithExecutionID(context.Background(), "execution-"+invocation), nativeIdentity(invocation))
}

func TestNativeIdentityEnvelopeStrictExactMatch(t *testing.T) {
	want := &processObservation{1, nativeIdentity("n"), "execution", "process"}
	raw, _ := json.Marshal(want)
	if err := want.verifyResponse([]byte(`{"observation":` + string(raw) + `}`)); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		`{}`, `{"observation":null}`,
		`{"observation":` + string(raw) + `,"observation":` + string(raw) + `}`,
		`{"observation":` + strings.Replace(string(raw), `"process_id":"process"`, `"process_id":"other"`, 1) + `}`,
		`{"observation":` + strings.Replace(string(raw), `"execution_id":"execution"`, `"execution_id":"other"`, 1) + `}`,
		`{"observation":` + strings.Replace(string(raw), `"scope_id":"scope"`, `"scope_id":"scope","scope_id":"scope"`, 1) + `}`,
		`{"observation":` + strings.Replace(string(raw), `"scope_id":"scope"`, `"scope_id":"other"`, 1) + `}`,
		`{"observation":` + strings.Replace(string(raw), `"tool_call_id":"tool"`, `"tool_call_id":null`, 1) + `}`,
		`{"observation":` + strings.Replace(string(raw), `"process_id":"process"`, `"Process_ID":"process"`, 1) + `}`,
		`{"observation":` + strings.TrimSuffix(string(raw), `}`) + `,"extra":1}}`,
		`{"observation":` + string(raw) + `} {}`,
	} {
		if err := want.verifyResponse([]byte(bad)); err == nil {
			t.Fatalf("invalid identity accepted: %s", bad)
		}
	}
}

func TestNativeIdentityProcessBindingAcrossCallerContexts(t *testing.T) {
	var mu sync.Mutex
	observations := map[string]any{}
	wantEnv := map[string]string{"UNCHANGED": "value"}
	wantArgs := []string{"a b", "$HOME", "'", ""}
	m := testManager(t, func(action string, body map[string]any) (int, any) {
		if action == "create" {
			return 200, createResponse(body)
		}
		if action == "kill" {
			return 200, map[string]bool{"absence_confirmed": true}
		}
		process, _ := body["process"].(string)
		mu.Lock()
		defer mu.Unlock()
		if action == "start" {
			if _, present := body["execution_id"]; present {
				t.Error("execution identity mixed into command fields")
			}
			args, _ := json.Marshal(body["argv"])
			want, _ := json.Marshal(append([]string{"/bin/tool"}, wantArgs...))
			if !bytes.Equal(args, want) {
				t.Error("argv changed")
			}
			env, _ := json.Marshal(body["env"])
			want, _ = json.Marshal(wantEnv)
			if !bytes.Equal(env, want) {
				t.Error("environment changed")
			}
			observations[process] = body["observation"]
			response := map[string]any{"process": process, "pid": 42}
			if o := observations[process]; o != nil {
				response["observation"] = o
			}
			return 200, response
		}
		if _, present := body["observation"]; present {
			t.Error("later request attempted to replace job identity")
		}
		response := map[string]any{}
		o := observations[process]
		if o != nil {
			response["observation"] = o
		}
		switch action {
		case "stdin":
			response["accepted"] = true
		case "process", "kill_process":
			response["pid"] = 42
			response["done"] = true
			response["managed_process_absent"] = true
		case "events":
			terminal := map[string]any{"exit_code": 0, "guest_duration_ns": 17}
			output := map[string]any{"stream": "stdout", "data_base64": "AP8="}
			events := []any{}
			if o != nil {
				events = append(events, map[string]any{"kind": "execution_identity", "phase": "before_popen", "observation": o})
				terminal["observation"] = o
				output["observation"] = o
			}
			events = append(events, output, terminal)
			response["events"] = events
			response["cursor"] = len(events)
			response["done"] = true
		default:
			t.Errorf("unexpected action %s", action)
			return 500, nil
		}
		return 200, response
	})
	live, err := m.Start(context.Background(), request())
	if err != nil {
		t.Fatal(err)
	}
	sandbox := live.(*Sandbox)
	var processes []*Process
	for _, ctx := range []context.Context{nativeContext("one"), nativeContext("two"), context.Background()} {
		p, err := sandbox.StartProcess(ctx, core.Command{Path: "/bin/tool", Args: wantArgs, Env: wantEnv})
		if err != nil {
			t.Fatal(err)
		}
		processes = append(processes, p)
	}
	foreign := nativeContext("foreign")
	for i := len(processes) - 1; i >= 0; i-- {
		p := processes[i]
		if err := p.SendStdin(foreign, []byte{0, 255}, true); err != nil {
			t.Fatal(err)
		}
		if _, err := p.Status(foreign); err != nil {
			t.Fatal(err)
		}
		var stdout bytes.Buffer
		if result, err := p.Wait(foreign, &stdout, nil); err != nil || result.ExitCode != 0 || !bytes.Equal(stdout.Bytes(), []byte{0, 255}) {
			t.Fatalf("wait %+v %v output=%v", result, err, stdout.Bytes())
		}
		if _, err := p.Kill(foreign); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.FinishObservation(); err != nil {
		t.Fatal(err)
	}
	records, index := readOperationRecords(t, m)
	if index.Status != "complete" {
		t.Fatalf("index=%+v", index)
	}
	waits := 0
	for _, r := range records {
		if r.ProcessID == "" {
			if r.NativeIdentity != nil {
				t.Fatal("lifecycle inherited native identity")
			}
			continue
		}
		var p *Process
		for _, candidate := range processes {
			if candidate.ID == r.ProcessID {
				p = candidate
				break
			}
		}
		if p == nil {
			t.Fatal("unknown process")
		}
		if p.observation == nil {
			if r.NativeIdentity != nil || r.ExecutionID != "" {
				t.Fatal("unlinked process inherited foreign identity")
			}
		} else if r.NativeIdentity == nil || *r.NativeIdentity != p.observation.NativeIdentity || r.ExecutionID != p.executionID {
			t.Fatalf("process identity changed: %+v", r)
		}
		if r.Operation == "exec.wait" && r.Event == "end" {
			waits++
			if (r.GuestIdentityReceipt != nil) != (p.observation != nil) {
				t.Fatal("guest receipt missing or invented")
			}
		}
	}
	if waits != 3 {
		t.Fatalf("wait records=%d", waits)
	}
}

func TestNativeIdentityRejectsMissingWrongAndReorderedGuestReceipt(t *testing.T) {
	for _, mode := range []string{"missing_top", "wrong_top", "missing_receipt", "duplicate_receipt", "output_before_receipt", "wrong_receipt", "missing_output_identity", "wrong_terminal", "receipt_then_error"} {
		t.Run(mode, func(t *testing.T) {
			var observation any
			m := testManager(t, func(action string, body map[string]any) (int, any) {
				switch action {
				case "create":
					return 200, createResponse(body)
				case "start":
					observation = body["observation"]
					return 200, map[string]any{"process": body["process"], "pid": 42, "observation": observation}
				case "events":
					wrong := map[string]any{}
					for k, v := range observation.(map[string]any) {
						wrong[k] = v
					}
					wrong["process_id"] = "foreign"
					receipt := map[string]any{"kind": "execution_identity", "phase": "before_popen", "observation": observation}
					output := map[string]any{"stream": "stdout", "data_base64": "eA==", "observation": observation}
					terminal := map[string]any{"exit_code": 0, "guest_duration_ns": 17, "observation": observation}
					events := []any{receipt, output, terminal}
					top := observation
					switch mode {
					case "missing_top":
						top = nil
					case "wrong_top":
						top = wrong
					case "missing_receipt":
						events = []any{output, terminal}
					case "duplicate_receipt":
						events = []any{receipt, receipt, terminal}
					case "output_before_receipt":
						events = []any{output, receipt, terminal}
					case "wrong_receipt":
						receipt["observation"] = wrong
					case "missing_output_identity":
						delete(output, "observation")
					case "wrong_terminal":
						terminal["observation"] = wrong
					case "receipt_then_error":
						events = []any{receipt, map[string]any{"error_code": "guest_terminal_missing", "observation": observation}}
					}
					response := map[string]any{"cursor": len(events), "done": true, "events": events}
					if top != nil {
						response["observation"] = top
					}
					return 200, response
				case "kill":
					return 200, map[string]bool{"absence_confirmed": true}
				default:
					t.Fatalf("unexpected %s", action)
					return 500, nil
				}
			})
			live, err := m.Start(context.Background(), request())
			if err != nil {
				t.Fatal(err)
			}
			p, err := live.(*Sandbox).StartProcess(nativeContext("one"), core.Command{Path: "unchanged"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := p.Wait(context.Background(), nil, nil); err == nil {
				t.Fatal("invalid guest identity stream succeeded")
			}
			if err := m.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := m.FinishObservation(); err != nil {
				t.Fatal(err)
			}
			records, _ := readOperationRecords(t, m)
			for _, r := range records {
				if r.Operation == "exec.wait" && r.Event == "end" {
					if r.Status != "failed" || r.ExitCode != nil || r.GuestDurationNS != nil {
						t.Fatalf("identity failure invented success %+v", r)
					}
					if mode == "receipt_then_error" && r.GuestIdentityReceipt == nil {
						t.Fatal("real receipt lost after later failure")
					}
				}
			}
		})
	}
}

func TestNativeIdentityInvalidStartNeverSubmits(t *testing.T) {
	starts := 0
	m := testManager(t, func(action string, body map[string]any) (int, any) {
		if action == "create" {
			return 200, createResponse(body)
		}
		starts++
		return 500, nil
	})
	live, err := m.Start(context.Background(), request())
	if err != nil {
		t.Fatal(err)
	}
	bad := nativeIdentity("bad")
	bad.SSHRequestID = ""
	for i, ctx := range []context.Context{performance.WithNativeToolIdentity(context.Background(), nativeIdentity("missing-execution")), performance.WithNativeToolIdentity(performance.WithExecutionID(context.Background(), "exec"), bad)} {
		if _, err := live.(*Sandbox).StartProcess(ctx, core.Command{Path: "same"}); err == nil {
			t.Fatalf("invalid start %d accepted", i)
		}
	}
	if starts != 0 {
		t.Fatal("invalid identity reached adapter")
	}
	if got, ok := performance.NativeToolIdentityFromContext(performance.WithoutNativeToolIdentity(nativeContext("foreign"))); ok || !reflect.DeepEqual(got, performance.NativeToolIdentity{}) {
		t.Fatal("identity removal failed")
	}
}

func TestNativeIdentityRequiredForEverySuccessfulProcessResponse(t *testing.T) {
	for _, action := range []string{"start", "stdin", "process", "kill_process"} {
		t.Run(action, func(t *testing.T) {
			var observation any
			m := testManager(t, func(got string, body map[string]any) (int, any) {
				if got == "create" {
					return 200, createResponse(body)
				}
				if got == "start" {
					observation = body["observation"]
				}
				response := map[string]any{"process": body["process"], "pid": 42, "accepted": true, "done": true, "managed_process_absent": true}
				if got != action {
					response["observation"] = observation
				}
				return 200, response
			})
			live, err := m.Start(context.Background(), request())
			if err != nil {
				t.Fatal(err)
			}
			p, err := live.(*Sandbox).StartProcess(nativeContext("one"), core.Command{Path: "same"})
			if action == "start" {
				if err == nil || p == nil {
					t.Fatal("ambiguous start lost identity or succeeded")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			switch action {
			case "stdin":
				err = p.SendStdin(context.Background(), nil, true)
			case "process":
				_, err = p.Status(context.Background())
			case "kill_process":
				_, err = p.Kill(context.Background())
			}
			if err == nil {
				t.Fatal("response without identity succeeded")
			}
		})
	}
}

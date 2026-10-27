package performance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
)

type identitySnapshotFixture struct {
	root, directory                          string
	producer, bridge, operations, transcript []map[string]any
	index, operationIndex                    map[string]any
}

func newIdentitySnapshotFixture(t *testing.T) *identitySnapshotFixture {
	t.Helper()
	root, directory := traceFixture(t)
	f := &identitySnapshotFixture{root: root, directory: directory}
	writeSummaryJSON(t, root, "run-result.json", map[string]any{"run_id": "run", "attempts": []traceAttempt{{AttemptID: "run/occ", OccurrenceID: "occ", AgentSessionID: "session-key"}}})
	writeSummaryJSON(t, root, "cloud-profile.json", map[string]any{"placement": "external", "harness": map[string]any{"native_observation_protocol": "aries-ssh-identity-v1"}})
	h := filepath.Join(root, "occ", "harness")
	if err := os.MkdirAll(filepath.Join(h, "telemetry"), 0700); err != nil {
		t.Fatal(err)
	}
	writeSummaryJSON(t, h, "agent-result.json", map[string]any{"run_id": "gateway-run", "requested_native_session_id": "native-session", "response": "SECRET_RESPONSE"})
	writeSummaryJSON(t, h, "telemetry.index.json", map[string]any{"paths": []string{"telemetry/native-session.jsonl", "telemetry/native-session.trajectory.jsonl", "telemetry/sessions.json"}})
	writeSummaryJSON(t, filepath.Join(h, "telemetry"), "sessions.json", map[string]any{"session-key": map[string]any{"sessionId": "native-session", "sessionFile": "/container/not-a-host-path/native-session.jsonl"}})
	writeTraceEvents(t, filepath.Join(h, "telemetry", "native-session.trajectory.jsonl"), []map[string]any{{"type": "session.started", "sessionId": "native-session", "sessionKey": "session-key"}})
	f.transcript = []map[string]any{{"type": "session", "version": 3, "id": "native-session"},
		{"type": "message", "message": map[string]any{"role": "assistant", "content": []map[string]any{{"type": "text", "text": "SECRET_THINKING"}, {"type": "toolCall", "name": "exec", "id": "tool-one", "arguments": map[string]any{"command": "SECRET_SAME_COMMAND"}}, {"type": "toolCall", "name": "process", "id": "process-poll"}}}},
		{"type": "message", "message": map[string]any{"role": "assistant", "content": []map[string]any{{"type": "toolCall", "name": "exec", "id": "tool-two", "arguments": map[string]any{"command": "SECRET_SAME_COMMAND"}}}}}}
	for i, call := range []string{"tool-two", "tool-one", "tool-one"} {
		n := string(rune('a' + i))
		id := map[string]any{"schema_version": 1, "scope_id": "run/occ", "agent_session_id": "session-key", "tool_call_id": call, "native_invocation_id": "inv-" + n}
		f.producer = append(f.producer, id)
		wire := map[string]any{}
		for k, v := range id {
			wire[k] = v
		}
		wire["ssh_request_id"] = "ssh-" + n
		f.bridge = append(f.bridge, map[string]any{"sequence": i + 1, "run_id": "run", "task_id": "occ", "identity_status": "exact_channel", "native_identity": wire, "execution_id": "exec-" + n, "sandbox_id": "sandbox", "command": "SECRET_SAME_COMMAND", "timestamp": "ignored"})
		for j, event := range []string{"start", "end", "start", "end"} {
			op := "adapter_rpc.start"
			if j >= 2 {
				op = "exec.wait"
			}
			r := map[string]any{"schema_version": 1, "sequence": len(f.operations) + 1, "event": event, "operation_id": i*2 + j/2 + 1, "operation": op, "run_id": "run", "task_id": "occ", "native_identity": wire, "execution_id": "exec-" + n, "process_id": "process-" + n, "sandbox_handle": "sandbox"}
			if j == 3 {
				r["guest_identity_receipt"] = map[string]any{"kind": "execution_identity", "phase": "before_popen", "observation": map[string]any{"schema_version": 1, "native_identity": wire, "execution_id": "exec-" + n, "process_id": "process-" + n}}
				r["exit_code"] = 0
				r["guest_duration_ns"] = 123
				r["status"] = "succeeded"
			}
			f.operations = append(f.operations, r)
		}
	}
	f.bridge = append(f.bridge, map[string]any{"sequence": 4, "run_id": "run", "task_id": "occ", "identity_status": "unlinked", "execution_id": "auxiliary", "sandbox_id": "sandbox"})
	f.index = map[string]any{"schema_version": 1, "protocol": "aries-ssh-identity-v1", "scope_id": "run/occ", "agent_session_id": "session-key", "file_integrity": "valid", "expected_native_calls": nil, "coverage": "producer_only_independent_denominator_required"}
	f.operationIndex = map[string]any{"schema_version": 1, "status": "complete"}
	f.write(t)
	return f
}

func (f *identitySnapshotFixture) write(t *testing.T) {
	t.Helper()
	h := filepath.Join(f.root, "occ", "harness")
	writeTraceEvents(t, filepath.Join(h, "telemetry", "native-session.jsonl"), f.transcript)
	writeTraceEvents(t, filepath.Join(h, "native-tool-identities.jsonl"), f.producer)
	f.reindex(t)
	writeTraceEvents(t, filepath.Join(f.root, "occ", "bridge", "tool-calls.jsonl"), f.bridge)
	writeTraceEvents(t, filepath.Join(f.root, "occ", "e2b-operations.jsonl"), f.operations)
	f.operationIndex["records"] = len(f.operations)
	f.operationIndex["started"] = len(f.operations) / 2
	f.operationIndex["ended"] = len(f.operations) / 2
	writeSummaryJSON(t, filepath.Join(f.root, "occ"), "e2b-operations-index.json", f.operationIndex)
}

func (f *identitySnapshotFixture) reindex(t *testing.T) {
	t.Helper()
	h := filepath.Join(f.root, "occ", "harness")
	data, err := os.ReadFile(filepath.Join(h, "native-tool-identities.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	f.index["sha256"] = hex.EncodeToString(sum[:])
	f.index["events"] = strings.Count(string(data), "\n")
	writeSummaryJSON(t, h, "native-tool-identities.index.json", f.index)
}

func identityGroup(t *testing.T, f *identitySnapshotFixture) (AnalysisSnapshot, AnalysisNativeIdentityAttempt) {
	t.Helper()
	s, err := BuildSnapshot(f.directory)
	if err == nil || s.IntegrityStatus != "incomplete" || s.CoverageStatus != "incomplete" {
		t.Fatal("A missing native timing became complete")
	}
	if s.NativeToolIdentities == nil || len(s.NativeToolIdentities.Attempts) != 1 {
		t.Fatal("identity analysis missing")
	}
	return s, s.NativeToolIdentities.Attempts[0]
}

func TestNativeIdentitySnapshotExactEvidenceWithoutInventedTiming(t *testing.T) {
	f := newIdentitySnapshotFixture(t)
	s, g := identityGroup(t, f)
	if g.Status != "exact_for_archived_exec_observations" || g.ArchivedExecCalls == nil || *g.ArchivedExecCalls != 2 || g.ProducerInvocations == nil || *g.ProducerInvocations != 3 || g.CoveredExecCalls != 2 || g.AuxiliaryBridgeRecords == nil || *g.AuxiliaryBridgeRecords != 1 || g.ProducerHealth != "unknown" {
		t.Fatalf("group=%+v", g)
	}
	if len(g.Mappings) != 3 || g.Mappings[0].ToolCallID != "tool-one" || g.Mappings[1].ToolCallID != "tool-one" || g.Mappings[0].NativeInvocationID == g.Mappings[1].NativeInvocationID {
		t.Fatal("independent invocations conflated")
	}
	for _, coverage := range s.Coverage {
		if coverage.Source == "native_exec_identity" && (coverage.ObservedEvents != 2 || coverage.Started != 2 || coverage.Paired != 2 || coverage.ExpectedOperations == nil || *coverage.ExpectedOperations != 2 || coverage.Status != "unknown") {
			t.Fatal("identity coverage mixed invocation and archived call units")
		}
	}
	sources := map[string]EvidenceSource{}
	for _, source := range s.Sources {
		sources[source.Ref] = source
	}
	for _, m := range g.Mappings {
		if m.Status != "exact" || m.GuestCompletion != "validated_terminal_fields_present" || len(m.Evidence) != 6 {
			t.Fatalf("mapping=%+v", m)
		}
		for _, e := range m.Evidence {
			source, ok := sources[e.EvidenceRef]
			if !ok || e.SourceLine == 0 {
				t.Fatal("missing evidence position")
			}
			data, err := os.ReadFile(filepath.Join(f.root, source.File))
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(data)
			if source.SHA256 != hex.EncodeToString(sum[:]) || int(e.SourceLine) > strings.Count(string(data), "\n") {
				t.Fatal("incorrect source digest or line")
			}
		}
	}
	raw, _ := json.Marshal(g)
	if strings.Contains(string(raw), "SECRET_") || strings.Contains(string(raw), "duration") || strings.Contains(string(raw), "timestamp") {
		t.Fatal("identity extension leaked payload or invented timing")
	}
	second, _ := BuildSnapshot(f.directory)
	if !reflect.DeepEqual(s, second) {
		t.Fatal("snapshot nondeterministic")
	}
	writeSummaryJSON(t, f.root, "cloud-profile.json", map[string]any{"placement": "external", "harness": map[string]any{}})
	for _, name := range []string{"native-tool-identities.jsonl", "native-tool-identities.index.json"} {
		if err := os.Remove(filepath.Join(f.root, "occ", "harness", name)); err != nil {
			t.Fatal(err)
		}
	}
	legacy, _ := BuildSnapshot(f.directory)
	if legacy.NativeToolIdentities != nil || !reflect.DeepEqual(legacy.Events, s.Events) || !reflect.DeepEqual(legacy.Intervals, s.Intervals) || !reflect.DeepEqual(legacy.Edges, s.Edges) || !reflect.DeepEqual(legacy.Clocks, s.Clocks) {
		t.Fatal("identity analysis changed timed projection or legacy behavior")
	}
}

func TestNativeIdentitySnapshotPreDispatchDisposition(t *testing.T) {
	for _, kind := range []string{"rejected", "elevated-rejected", "host-rejected", "unknown", "conflicting-bridge", "conflicting-sdk"} {
		t.Run(kind, func(t *testing.T) {
			f := newIdentitySnapshotFixture(t)
			legacy, _ := identityGroup(t, f)
			reason := "shell_preflight_rejected"
			if kind == "elevated-rejected" {
				reason = "elevated_policy_rejected"
			}
			if kind == "host-rejected" {
				reason = "host_policy_rejected"
			}
			for _, p := range f.producer {
				p["schema_version"], p["phase"], p["outcome"], p["dispatched"], p["reason"] = 2, "invocation", "started", nil, ""
			}
			if kind != "unknown" {
				rejected := map[string]any{}
				for key, value := range f.producer[0] {
					rejected[key] = value
				}
				rejected["phase"], rejected["outcome"], rejected["dispatched"], rejected["reason"] = "pre_dispatch", "rejected", false, reason
				f.producer = append(f.producer, rejected)
			}
			if kind != "conflicting-bridge" {
				f.bridge = f.bridge[1:]
			}
			if kind != "conflicting-sdk" {
				f.operations = f.operations[4:]
			}
			for i, row := range f.bridge {
				row["sequence"] = i + 1
			}
			for i, row := range f.operations {
				row["sequence"] = i + 1
			}
			f.write(t)
			s, group := identityGroup(t, f)
			if group.ProducerInvocations == nil || *group.ProducerInvocations != 3 || group.ArchivedExecCalls == nil || *group.ArchivedExecCalls != 2 || group.CoveredExecCalls != 1 || group.Dispatch == nil {
				t.Fatalf("mixed denominators: %+v", group)
			}
			if group.Dispatch.DispatchedInvocations != 2 {
				t.Fatalf("wrong dispatched count: %+v", group.Dispatch)
			}
			var rejected AnalysisNativeIdentityMapping
			for _, mapping := range group.Mappings {
				if mapping.ToolCallID == "tool-two" {
					rejected = mapping
				}
			}
			if kind == "rejected" || kind == "elevated-rejected" || kind == "host-rejected" {
				if group.Status != "accounted_for_archived_exec_observations" || group.Dispatch.RejectedInvocations != 1 || group.Dispatch.UnknownInvocations != 0 || group.Dispatch.ResolvedExecCalls != 2 || group.Dispatch.ProducerRecords != 4 {
					t.Fatalf("rejection unaccounted: %+v", group)
				}
				if rejected.Status != "rejected_before_dispatch" || rejected.GuestCompletion != "not_dispatched" || rejected.ExecutionID != "" || rejected.ProcessID != "" || rejected.RejectionReason != reason || len(rejected.Evidence) != 3 {
					t.Fatalf("invented transport or lost rejection: %+v", rejected)
				}
			} else if group.Status != "incomplete" || group.Dispatch.RejectedInvocations != 0 || group.Dispatch.UnknownInvocations != 1 || group.Dispatch.ResolvedExecCalls != 1 || rejected.Status != "unlinked" || rejected.GuestCompletion != "unknown" {
				t.Fatalf("missing or conflicting evidence became rejection: %+v", group)
			}
			if !reflect.DeepEqual(s.Events, legacy.Events) || !reflect.DeepEqual(s.Intervals, legacy.Intervals) || !reflect.DeepEqual(s.Edges, legacy.Edges) {
				t.Fatal("identity-only rejection invented timed events")
			}
			if err := ValidateSnapshot(legacy); err != nil {
				t.Fatal("old frozen identity changed", err)
			}
			data, _ := json.Marshal(legacy)
			if strings.Contains(string(data), `"dispatch"`) || strings.Contains(string(data), `"dispatch_status"`) {
				t.Fatal("v1 shape changed")
			}
		})
	}
}

func TestNativeIdentitySnapshotOptionalOperationFileIO(t *testing.T) {
	f := newIdentitySnapshotFixture(t)
	legacy, legacyGroup := identityGroup(t, f)
	data, err := os.ReadFile(filepath.Join(f.root, "occ", "e2b-operations.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	f.operationIndex["file_io"] = FileIOCounters{Scope: "e2b_operation_jsonl_only",
		WritesStarted: uint64(len(f.operations)), WritesReturned: uint64(len(f.operations)),
		RequestedBytes: uint64(len(data)), ReturnedBytes: uint64(len(data)), SyncsStarted: 1, SyncsReturned: 1}
	f.write(t)
	current, group := identityGroup(t, f)
	if group.Status != "exact_for_archived_exec_observations" || !reflect.DeepEqual(group.Mappings, legacyGroup.Mappings) || group.CoveredExecCalls != legacyGroup.CoveredExecCalls {
		t.Fatal("optional file I/O metadata changed exact identity mapping")
	}
	if !reflect.DeepEqual(current.Events, legacy.Events) || !reflect.DeepEqual(current.Intervals, legacy.Intervals) || !reflect.DeepEqual(current.Edges, legacy.Edges) || !reflect.DeepEqual(current.Clocks, legacy.Clocks) || !reflect.DeepEqual(current.Coverage, legacy.Coverage) {
		t.Fatal("file I/O metadata changed timing or coverage")
	}
	if current.SnapshotID == legacy.SnapshotID {
		t.Fatal("new evidence bytes did not change snapshot identity")
	}
}

func TestNativeIdentitySnapshotRejectsMalformedOperationFileIO(t *testing.T) {
	valid, err := json.Marshal(FileIOCounters{Scope: "e2b_operation_jsonl_only", WritesStarted: 12, WritesReturned: 12,
		RequestedBytes: 1024, ReturnedBytes: 1024, SyncsStarted: 1, SyncsReturned: 1})
	if err != nil {
		t.Fatal(err)
	}
	for name, raw := range map[string][]byte{
		"null":             []byte("null"),
		"array":            []byte("[]"),
		"string":           []byte(`"not-counters"`),
		"empty-object":     []byte("{}"),
		"unknown-field":    bytesReplace(valid, `"write_errors":0`, `"write_errors":0,"future_counter":0`),
		"duplicate-field":  bytesReplace(valid, `"write_errors":0`, `"write_errors":0,"write_errors":0`),
		"case-alias":       bytesReplace(valid, `"write_errors":0`, `"Write_Errors":0`),
		"missing-field":    bytesReplace(valid, `"write_errors":0,`, ""),
		"null-counter":     bytesReplace(valid, `"write_errors":0`, `"write_errors":null`),
		"negative-counter": bytesReplace(valid, `"write_errors":0`, `"write_errors":-1`),
		"string-counter":   bytesReplace(valid, `"write_errors":0`, `"write_errors":"0"`),
		"fraction-counter": bytesReplace(valid, `"write_errors":0`, `"write_errors":0.5`),
		"overflow-counter": bytesReplace(valid, `"write_errors":0`, `"write_errors":18446744073709551616`),
		"wrong-scope":      bytesReplace(valid, `"e2b_operation_jsonl_only"`, `"adapter_process"`),
	} {
		t.Run(name, func(t *testing.T) {
			f := newIdentitySnapshotFixture(t)
			index, err := json.Marshal(f.operationIndex)
			if err != nil {
				t.Fatal(err)
			}
			// Write raw bytes to preserve duplicate keys for the evidence parser.
			index = append(index[:len(index)-1], []byte(`,"file_io":`)...)
			index = append(index, raw...)
			index = append(index, '}', '\n')
			if err := os.WriteFile(filepath.Join(f.root, "occ", "e2b-operations-index.json"), index, 0600); err != nil {
				t.Fatal(err)
			}
			_, group := identityGroup(t, f)
			if group.Status != "incomplete" || len(group.Diagnostics) == 0 {
				t.Fatalf("malformed operation metadata accepted: %+v", group)
			}
		})
	}
}

func TestNativeIdentitySnapshotRejectsMissingAmbiguousAndForeignEvidence(t *testing.T) {
	cases := map[string]func(*testing.T, *identitySnapshotFixture){
		"both-producer-files-missing": func(t *testing.T, f *identitySnapshotFixture) {
			for _, n := range []string{"native-tool-identities.jsonl", "native-tool-identities.index.json"} {
				if err := os.Remove(filepath.Join(f.root, "occ", "harness", n)); err != nil {
					t.Fatal(err)
				}
			}
		},
		"wrong-header": func(t *testing.T, f *identitySnapshotFixture) { f.transcript[0]["id"] = "other"; f.write(t) },
		"duplicate-assistant-id": func(t *testing.T, f *identitySnapshotFixture) {
			f.transcript = append(f.transcript, f.transcript[1])
			f.write(t)
		},
		"unindexed-session": func(t *testing.T, f *identitySnapshotFixture) {
			writeTraceEvents(t, filepath.Join(f.root, "occ", "harness", "telemetry", "duplicate.jsonl"), f.transcript)
		},
		"wrong-session-key": func(t *testing.T, f *identitySnapshotFixture) {
			writeSummaryJSON(t, filepath.Join(f.root, "occ", "harness", "telemetry"), "sessions.json", map[string]any{"session-key": map[string]any{"sessionId": "other"}})
		},
		"index-path-escape": func(t *testing.T, f *identitySnapshotFixture) {
			writeSummaryJSON(t, filepath.Join(f.root, "occ", "harness"), "telemetry.index.json", map[string]any{"paths": []string{"../other.jsonl"}})
		},
		"producer-symlink": func(t *testing.T, f *identitySnapshotFixture) {
			p := filepath.Join(f.root, "occ", "harness", "native-tool-identities.jsonl")
			if err := os.Rename(p, p+".saved"); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(p+".saved", p); err != nil {
				t.Fatal(err)
			}
		},
		"producer-duplicate": func(t *testing.T, f *identitySnapshotFixture) {
			f.producer = append(f.producer, f.producer[0])
			f.write(t)
		},
		"producer-foreign":     func(t *testing.T, f *identitySnapshotFixture) { f.producer[0]["scope_id"] = "other/occ"; f.write(t) },
		"producer-unknown-key": func(t *testing.T, f *identitySnapshotFixture) { f.producer[0]["sequence"] = 1; f.write(t) },
		"producer-sha": func(t *testing.T, f *identitySnapshotFixture) {
			f.index["sha256"] = strings.Repeat("0", 64)
			writeSummaryJSON(t, filepath.Join(f.root, "occ", "harness"), "native-tool-identities.index.json", f.index)
		},
		"producer-count": func(t *testing.T, f *identitySnapshotFixture) {
			f.index["events"] = 99
			writeSummaryJSON(t, filepath.Join(f.root, "occ", "harness"), "native-tool-identities.index.json", f.index)
		},
		"bridge-missing": func(t *testing.T, f *identitySnapshotFixture) {
			if err := os.Remove(filepath.Join(f.root, "occ", "bridge", "tool-calls.jsonl")); err != nil {
				t.Fatal(err)
			}
		},
		"bridge-execution-key-alias": func(t *testing.T, f *identitySnapshotFixture) {
			f.bridge[0]["Execution_ID"] = "exec-a"
			f.write(t)
		},
		"operation-key-alias": func(t *testing.T, f *identitySnapshotFixture) {
			f.operations[0]["Process_ID"] = "process-a"
			f.write(t)
		},
		"assistant-role-key-alias": func(t *testing.T, f *identitySnapshotFixture) {
			f.transcript[1]["message"].(map[string]any)["Role"] = "user"
			f.write(t)
		},
		"indexed-duplicate-session": func(t *testing.T, f *identitySnapshotFixture) {
			h := filepath.Join(f.root, "occ", "harness")
			writeTraceEvents(t, filepath.Join(h, "telemetry", "duplicate.jsonl"), f.transcript)
			writeSummaryJSON(t, h, "telemetry.index.json", map[string]any{"paths": []string{"telemetry/native-session.jsonl", "telemetry/duplicate.jsonl", "telemetry/sessions.json"}})
		},
		"bridge-cross-session": func(t *testing.T, f *identitySnapshotFixture) {
			f.bridge[0]["native_identity"].(map[string]any)["agent_session_id"] = "other"
			f.write(t)
		},
		"bridge-duplicate-execution": func(t *testing.T, f *identitySnapshotFixture) {
			f.bridge[1]["execution_id"] = f.bridge[0]["execution_id"]
			f.write(t)
		},
		"operation-sequence":         func(t *testing.T, f *identitySnapshotFixture) { f.operations[1]["sequence"] = 1; f.write(t) },
		"operation-index-incomplete": func(t *testing.T, f *identitySnapshotFixture) { f.operationIndex["status"] = "incomplete"; f.write(t) },
		"operation-index-unknown-field": func(t *testing.T, f *identitySnapshotFixture) {
			f.operationIndex["future_field"] = true
			f.write(t)
		},
		"operation-index-file-io-alias": func(t *testing.T, f *identitySnapshotFixture) {
			f.operationIndex["File_IO"] = FileIOCounters{Scope: "e2b_operation_jsonl_only"}
			f.write(t)
		},
		"operation-incomplete-marker": func(t *testing.T, f *identitySnapshotFixture) {
			if err := os.WriteFile(filepath.Join(f.root, "occ", "e2b-operations-index.json.incomplete"), []byte("late"), 0600); err != nil {
				t.Fatal(err)
			}
		},
		"receipt-missing": func(t *testing.T, f *identitySnapshotFixture) {
			delete(f.operations[3], "guest_identity_receipt")
			f.write(t)
		},
		"receipt-conflicting-process": func(t *testing.T, f *identitySnapshotFixture) {
			f.operations[3]["guest_identity_receipt"].(map[string]any)["observation"].(map[string]any)["process_id"] = "other"
			f.write(t)
		},
		"duplicate-process-uuid": func(t *testing.T, f *identitySnapshotFixture) {
			for _, row := range f.operations[4:8] {
				row["process_id"] = "process-a"
			}
			f.write(t)
		},
		"producer-orphan": func(t *testing.T, f *identitySnapshotFixture) {
			f.producer[0]["tool_call_id"] = "not-requested"
			f.write(t)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := newIdentitySnapshotFixture(t)
			mutate(t, f)
			_, g := identityGroup(t, f)
			if g.Status != "incomplete" {
				t.Fatalf("accepted %s: %+v", name, g)
			}
			if len(g.Mappings) > 0 {
				unlinked := false
				for _, m := range g.Mappings {
					unlinked = unlinked || m.Status == "unlinked"
				}
				if !unlinked && len(g.Diagnostics) == 0 {
					t.Fatal("failure has no diagnostic or unlinked mapping")
				}
			}
		})
	}
}

func TestNativeIdentitySnapshotStrictJSONAndMissingPairs(t *testing.T) {
	for name, change := range map[string]func([]byte) []byte{
		"duplicate-key": func(b []byte) []byte {
			return bytesReplace(b, `"scope_id":"run/occ"`, `"scope_id":"run/occ","scope_id":"run/occ"`)
		},
		"partial-line":  func(b []byte) []byte { return b[:len(b)-1] },
		"invalid-utf8":  func(b []byte) []byte { return append(b, 0xff, '\n') },
		"trailing-json": func(b []byte) []byte { return append(b, []byte("{} {}\n")...) },
	} {
		t.Run(name, func(t *testing.T) {
			f := newIdentitySnapshotFixture(t)
			p := filepath.Join(f.root, "occ", "harness", "native-tool-identities.jsonl")
			data, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, change(data), 0600); err != nil {
				t.Fatal(err)
			}
			f.reindex(t)
			_, g := identityGroup(t, f)
			if g.Status != "incomplete" {
				t.Fatal("accepted invalid JSON")
			}
		})
	}
	t.Run("entire-operation-pair-missing", func(t *testing.T) {
		f := newIdentitySnapshotFixture(t)
		f.operations = f.operations[2:]
		for i, row := range f.operations {
			row["sequence"] = i + 1
		}
		f.write(t)
		_, g := identityGroup(t, f)
		if g.Status != "incomplete" {
			t.Fatal("whole missing pair counted captured")
		}
	})
	t.Run("receipt-not-completion", func(t *testing.T) {
		f := newIdentitySnapshotFixture(t)
		delete(f.operations[3], "exit_code")
		delete(f.operations[3], "guest_duration_ns")
		f.write(t)
		_, g := identityGroup(t, f)
		for _, m := range g.Mappings {
			if m.NativeInvocationID == "inv-a" && (m.Status != "exact" || m.GuestCompletion != "unknown") {
				t.Fatal("receipt inferred terminal")
			}
		}
	})
}

func bytesReplace(b []byte, old, new string) []byte {
	return []byte(strings.Replace(string(b), old, new, 1))
}

func TestNativeIdentitySnapshotProducerHealthRequiresArchivedStatus(t *testing.T) {
	for _, kind := range []string{"healthy", "missing", "failed", "wrong-sha", "null-field", "long-instance", "space-instance", "unicode-instance"} {
		t.Run(kind, func(t *testing.T) {
			f := newIdentitySnapshotFixture(t)
			h := filepath.Join(f.root, "occ", "harness")
			health := map[string]any{"schema_version": 1, "scope_id": "run/occ", "agent_session_id": "session-key", "producer_instance_id": "instance", "attempted": 3, "written": 3, "write_failed": 0, "status_write_failed": 0, "failed_latched": false}
			if kind == "failed" {
				health["attempted"] = 4
				health["write_failed"] = 1
				health["failed_latched"] = true
			}
			if kind == "long-instance" {
				health["producer_instance_id"] = strings.Repeat("x", 257)
			}
			if kind == "space-instance" {
				health["producer_instance_id"] = "instance 1"
			}
			if kind == "unicode-instance" {
				health["producer_instance_id"] = "实例"
			}
			if kind == "null-field" {
				health["status_write_failed"] = nil
			}
			writeSummaryJSON(t, h, "native-tool-identities.status.json", health)
			data, err := os.ReadFile(filepath.Join(h, "native-tool-identities.status.json"))
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(data)
			f.index["producer_health"] = health
			f.index["producer_status_sha256"] = hex.EncodeToString(sum[:])
			if kind == "wrong-sha" {
				f.index["producer_status_sha256"] = strings.Repeat("0", 64)
			}
			f.reindex(t)
			if kind == "missing" {
				if err := os.Remove(filepath.Join(h, "native-tool-identities.status.json")); err != nil {
					t.Fatal(err)
				}
			}
			_, g := identityGroup(t, f)
			if kind == "healthy" {
				if g.Status != "exact_for_archived_exec_observations" || g.ProducerHealth != "healthy_for_archived_producer_instance" {
					t.Fatalf("healthy %+v", g)
				}
			} else if g.Status != "incomplete" || g.ProducerHealth != "invalid" {
				t.Fatalf("invalid health %+v", g)
			}
		})
	}
}

func TestNativeIdentitySnapshotVersionCompatibilityAndTamperDetection(t *testing.T) {
	root, directory := traceFixture(t)
	modelTraceFixture(t, root)
	toolTraceFixture(t, root)
	old, err := BuildSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	old.ParserVersion = "aries-analysis-v1"
	old.SnapshotID = ""
	raw, _ := json.Marshal(old)
	old.SnapshotID = stableAnalysisID("snapshot", string(raw))
	raw, _ = json.Marshal(old)
	if strings.Contains(string(raw), "native_tool_identities") {
		t.Fatal("optional extension altered old shape")
	}
	var restored AnalysisSnapshot
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if _, err := ProjectTrace(restored); err != nil {
		t.Fatalf("old snapshot rejected: %v", err)
	}
	f := newIdentitySnapshotFixture(t)
	snapshot, _ := identityGroup(t, f)
	snapshot.NativeToolIdentities.Attempts[0].Mappings[0].ProcessID = "tampered"
	trace, err := ProjectTrace(snapshot)
	if err == nil || !strings.Contains(strings.Join(trace.Metadata.Diagnostics, " "), "snapshot schema or immutable content identity invalid") {
		t.Fatal("mapping mutation did not invalidate snapshot")
	}
}

func TestNativeIdentitySnapshotMissingCountsStayUnknown(t *testing.T) {
	for _, kind := range []string{"missing-producer", "partial-producer", "missing-bridge", "partial-bridge"} {
		t.Run(kind, func(t *testing.T) {
			f := newIdentitySnapshotFixture(t)
			name := filepath.Join(f.root, "occ", "harness", "native-tool-identities.jsonl")
			if strings.HasSuffix(kind, "bridge") {
				name = filepath.Join(f.root, "occ", "bridge", "tool-calls.jsonl")
			}
			if strings.HasPrefix(kind, "missing") {
				if err := os.Remove(name); err != nil {
					t.Fatal(err)
				}
			} else {
				data, err := os.ReadFile(name)
				if err != nil {
					t.Fatal(err)
				}
				// A full valid prefix does not authorize a total count.
				if err := os.WriteFile(name, append(data, []byte("{}\n")...), 0600); err != nil {
					t.Fatal(err)
				}
			}
			_, g := identityGroup(t, f)
			if strings.HasSuffix(kind, "producer") && g.ProducerInvocations != nil {
				t.Fatal("invalid producer counted as known")
			}
			if strings.HasSuffix(kind, "bridge") && g.AuxiliaryBridgeRecords != nil {
				t.Fatal("invalid bridge prefix counted as total")
			}
			raw, err := json.Marshal(g)
			if err != nil {
				t.Fatal(err)
			}
			key := `"producer_invocations":null`
			if strings.HasSuffix(kind, "bridge") {
				key = `"auxiliary_bridge_records":null`
			}
			if !strings.Contains(string(raw), key) {
				t.Fatal("unknown count not explicit null")
			}
		})
	}
}

func TestIdentityEvidenceRejectsNonregularFileBeforeOpen(t *testing.T) {
	root := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(root, "pipe"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readIdentityEvidence(root, "pipe"); err == nil {
		t.Fatal("FIFO accepted as evidence")
	}
	if _, err := readIdentityEvidence(root, "."); err == nil {
		t.Fatal("directory accepted as evidence")
	}
}

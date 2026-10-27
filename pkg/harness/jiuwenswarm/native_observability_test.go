package jiuwenswarm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/performance"
)

func nativeTestLine(id, parent, session, request, tool string) []byte {
	attrs := []any{}
	for key, value := range map[string]string{"openjiuwen.session.id": session, "openjiuwen.request.id": request, "gen_ai.tool.call.id": tool} {
		if value != "" {
			attrs = append(attrs, map[string]any{"key": key, "value": map[string]string{"stringValue": value}})
		}
	}
	if tool != "" {
		attrs = append(attrs, map[string]any{"key": "openjiuwen.tool.authoritative", "value": map[string]bool{"boolValue": true}})
	}
	span := map[string]any{"traceId": "0123456789abcdef0123456789abcdef", "spanId": id, "startTimeUnixNano": "1793000000000000001", "endTimeUnixNano": "1793000000000000009", "attributes": attrs}
	if parent != "" {
		span["parentSpanId"] = parent
	}
	raw, _ := json.Marshal(map[string]any{"resourceSpans": []any{map[string]any{"scopeSpans": []any{map[string]any{"spans": []any{span}}}}}})
	return append(raw, '\n')
}

func TestNativeSpanIndexUsesExactIdentityAndPreservesUnixNanoseconds(t *testing.T) {
	root := nativeTestLine("1111111111111111", "", "s", "r", "")
	child := nativeTestLine("2222222222222222", "1111111111111111", "", "", "call-real")
	foreign := nativeTestLine("3333333333333333", "1111111111111111", "other", "r", "call-foreign")
	orphan := nativeTestLine("4444444444444444", "aaaaaaaaaaaaaaaa", "", "", "call-orphan")
	index := newNativeSpanIndex("s", "r")
	index.read("traces-2026-10-27.jsonl", bytes.Join([][]byte{root, child, foreign, orphan}, nil))
	index.associate()
	if index.Invalid != 0 || index.Matched != 2 || index.Unassociated != 2 || index.Tools != 1 || !index.Spans[1].Matched || index.Spans[2].Matched {
		t.Fatalf("identity association=%+v", index)
	}
	if index.Spans[1].Start != "1793000000000000001" || index.Spans[1].End != "1793000000000000009" || index.Clock != "native_unix_nanoseconds_unaligned" || index.Coverage != "partial" {
		t.Fatal("native wall timestamps changed or coverage overstated")
	}
}

func TestNativeSpanIndexRejectsMalformedDuplicateAndPartialRecords(t *testing.T) {
	valid := nativeTestLine("1111111111111111", "", "s", "r", "call")
	for name, content := range map[string][]byte{
		"bad_json":     []byte("{\n"),
		"bad_time":     bytes.ReplaceAll(valid, []byte("1793000000000000009"), []byte("1")),
		"bad_identity": bytes.ReplaceAll(valid, []byte("1111111111111111"), []byte("0000000000000000")),
		"duplicate":    append(bytes.Clone(valid), valid...),
		"partial_line": valid[:len(valid)-1],
	} {
		t.Run(name, func(t *testing.T) {
			index := newNativeSpanIndex("s", "r")
			index.read("source", content)
			index.associate()
			if index.Invalid == 0 {
				t.Fatal("invalid native export accepted")
			}
		})
	}
}

func TestNativeSpanIndexExposesOnlyKnownOperationAndIdentifierToolName(t *testing.T) {
	raw := nativeTestLine("1111111111111111", "", "s", "r", "call")
	raw = bytes.Replace(raw, []byte(`"attributes":[`), []byte(`"attributes":[{"key":"gen_ai.operation.name","value":{"stringValue":"execute_tool"}},{"key":"gen_ai.tool.name","value":{"stringValue":"bash"}},`), 1)
	raw = bytes.Replace(raw, []byte(`"traceId":`), []byte(`"status":{"code":2},"traceId":`), 1)
	index := newNativeSpanIndex("s", "r")
	index.read("source", raw)
	if len(index.Spans) != 1 || index.Spans[0].Operation != "execute_tool" || index.Spans[0].ToolName != "bash" || index.Spans[0].StatusCode != 2 {
		t.Fatal("native operation or failure status lost")
	}
	index = newNativeSpanIndex("s", "r")
	raw = bytes.ReplaceAll(raw, []byte(`"execute_tool"`), []byte(`"arbitrary prompt text"`))
	raw = bytes.ReplaceAll(raw, []byte(`"bash"`), []byte(`"bash private argument"`))
	index.read("source", raw)
	if len(index.Spans) != 1 || index.Spans[0].Operation != "" || index.Spans[0].ToolName != "" {
		t.Fatal("native arbitrary body copied into safe index")
	}
}

func TestNativeSpanCollectionFailureKeepsSuccessfulRunAndStop(t *testing.T) {
	s := &testSandbox{nativeReadFails: true, gracefulFails: true}
	e := testEmbedded(t, s, &testNative{})
	if err := startEmbedded(t, e, s); err != nil {
		t.Fatal(err)
	}
	result, err := e.Run(context.Background(), "task")
	if err != nil || result.Status != core.StatusSucceeded {
		t.Fatal("native outcome changed")
	}
	if err := e.Stop(context.Background()); err != nil || !e.isolated {
		t.Fatalf("observer prevented isolation: %v", err)
	}
	obs, err := e.NativeToolObservation()
	if err == nil || obs.Status != core.StatusFailed || s.stopCalls != 1 {
		t.Fatal("observer failure or fallback containment missing")
	}
	raw, err := os.ReadFile(filepath.Join(e.artifactDir, "native-spans-index.json"))
	if err != nil || !bytes.Contains(raw, []byte(`"matched_spans": 0`)) || !bytes.Contains(raw, []byte("cgroup fallback required")) {
		t.Fatal("missing partial/fallback receipt")
	}
}

func TestNativeSpanCollectionUsesRemainingCleanupBudget(t *testing.T) {
	s := &testSandbox{nativeDownloadDelay: 6 * time.Second}
	e := testEmbedded(t, s, &testNative{})
	e.options.CleanupTimeout = 12 * time.Second
	if err := startEmbedded(t, e, s); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Run(context.Background(), "task"); err != nil {
		t.Fatal(err)
	}
	if err := e.Stop(context.Background()); err != nil || !e.isolated {
		t.Fatalf("stop/isolation: %v", err)
	}
	obs, err := e.NativeToolObservation()
	if err != nil || obs.Status == core.StatusFailed || s.downloadCanceled {
		t.Fatalf("download within cleanup budget failed: status=%s canceled=%v err=%v", obs.Status, s.downloadCanceled, err)
	}
	raw, err := os.ReadFile(filepath.Join(e.artifactDir, "native-spans-index.json"))
	var index nativeSpanIndex
	if err != nil || json.Unmarshal(raw, &index) != nil || index.ProducerAudit == nil || index.ProducerAudit.Status != "complete" || len(index.ProducerAudit.Files) != 2 || s.downloadCalls != 5 {
		t.Fatal("delayed native trace or producer audit was not collected completely")
	}
	if s.cleanupCalls != 1 || s.cleanupContextErr != nil || s.cleanupTimeRemaining <= 0 {
		t.Fatal("private runtime cleanup did not retain its live budget")
	}
}

func TestNativeSpanCollectionDeadlinePreservesCleanupReserve(t *testing.T) {
	s := &testSandbox{nativeDownloadDelay: 2 * time.Second}
	e := testEmbedded(t, s, &testNative{})
	e.options.CleanupTimeout = 800 * time.Millisecond
	if err := startEmbedded(t, e, s); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Run(context.Background(), "task"); err != nil {
		t.Fatal(err)
	}
	if err := e.Stop(context.Background()); err != nil || !e.isolated {
		t.Fatalf("collection timeout prevented isolation or cleanup: %v", err)
	}
	obs, err := e.NativeToolObservation()
	if err == nil || obs.Status != core.StatusFailed || !s.downloadCanceled {
		t.Fatal("download deadline was not reported as an observation failure")
	}
	if s.cleanupCalls != 1 || s.cleanupContextErr != nil || s.cleanupTimeRemaining <= 0 || s.stopCalls != 1 {
		t.Fatal("collection exhausted the reserved cleanup budget or bypassed isolation")
	}
}

func TestNativeSpanCollectionPreservesPrivateBytesAndRedactsCredentialFailures(t *testing.T) {
	for _, secret := range []bool{false, true} {
		t.Run(map[bool]string{false: "exact", true: "credential"}[secret], func(t *testing.T) {
			s := &testSandbox{}
			e := testEmbedded(t, s, &testNative{})
			if err := startEmbedded(t, e, s); err != nil {
				t.Fatal(err)
			}
			s.nativeData = nativeTestLine("1111111111111111", "", "s", s.nativeRequest, "call")
			if secret {
				s.nativeData = bytes.ReplaceAll(s.nativeData, []byte(`"call"`), []byte(`"private-token"`))
			}
			if _, err := e.Run(context.Background(), "task"); err != nil {
				t.Fatal(err)
			}
			declaration, err := os.ReadFile(filepath.Join(e.artifactDir, "native-events-index.json"))
			if err != nil || !bytes.Contains(declaration, []byte(`"official_span_collection_enabled": true`)) {
				t.Fatal("L1 official span collection declaration missing")
			}
			if err := e.Stop(context.Background()); err != nil {
				t.Fatal(err)
			}
			obs, err := e.NativeToolObservation()
			if (err != nil) != secret || (obs.Status == core.StatusFailed) != secret {
				t.Fatalf("secret=%v obs=%+v err=%v", secret, obs, err)
			}
			file := filepath.Join(e.artifactDir, "native-spans", "traces-2026-10-27.jsonl")
			raw, err := os.ReadFile(file)
			if err != nil || bytes.Contains(raw, []byte("private-token")) || (!secret && !bytes.Equal(raw, s.nativeData)) {
				t.Fatal("private span bytes or credentials wrong")
			}
			info, _ := os.Stat(file)
			if info.Mode().Perm() != 0o600 {
				t.Fatal("native span artifact is not private")
			}
			if s.downloadCalls != 5 || !e.nativeStop.ExitObserved || !e.nativeStop.CleanExit {
				t.Fatal("native shutdown/collection not exercised")
			}
			if !secret {
				indexBytes, err := os.ReadFile(filepath.Join(e.artifactDir, "native-spans-index.json"))
				var index nativeSpanIndex
				if err != nil || json.Unmarshal(indexBytes, &index) != nil || index.ProducerAudit == nil || index.ProducerAudit.Status != "complete" || index.ProducerAudit.Started != 1 || len(index.ProducerAudit.Files) != 2 || index.Coverage != "partial" {
					t.Fatal("scoped producer audit missing or full coverage overstated")
				}
			}
		})
	}
}

func TestNativeSpanCollectionRetainsLongTaskTrace(t *testing.T) {
	s := &testSandbox{}
	e := testEmbedded(t, s, &testNative{})
	if err := startEmbedded(t, e, s); err != nil {
		t.Fatal(err)
	}
	// The first PPT run ended 424 spans; its 18.3 MB source was rejected before parsing.
	padding := bytes.Repeat([]byte("x"), 44<<10)
	for i := 1; i <= 424; i++ {
		line := nativeTestLine(fmt.Sprintf("%016x", i), "", "s", s.nativeRequest, fmt.Sprintf("call-%d", i))
		line = bytes.Replace(line, []byte(`"attributes":[`), append(append([]byte(`"attributes":[{"key":"private.body","value":{"stringValue":"`), padding...), []byte(`"}},`)...), 1)
		s.nativeData = append(s.nativeData, line...)
	}
	if len(s.nativeData) <= 16<<20 {
		t.Fatal("fixture does not cross the previous collection limit")
	}
	if _, err := e.Run(context.Background(), "task"); err != nil {
		t.Fatal(err)
	}
	if err := e.Stop(context.Background()); err != nil || !e.isolated {
		t.Fatal("collection changed isolation", err)
	}
	if obs, err := e.NativeToolObservation(); err != nil || obs.Status == core.StatusFailed {
		t.Fatalf("large trace collection failed: %+v %v", obs, err)
	}
	raw, err := os.ReadFile(filepath.Join(e.artifactDir, "native-spans", "traces-2026-10-27.jsonl"))
	if err != nil || !bytes.Equal(raw, s.nativeData) {
		t.Fatal("large private trace bytes lost or changed", err)
	}
	raw, err = os.ReadFile(filepath.Join(e.artifactDir, "native-spans-index.json"))
	var index nativeSpanIndex
	if err != nil || json.Unmarshal(raw, &index) != nil || index.Truncated || index.Invalid != 0 || index.Records != 424 || index.Tools != 424 || index.ProducerAudit == nil || index.ProducerAudit.Status != "complete" || index.ProducerAudit.Exported != 424 {
		t.Fatalf("large trace identity or producer reconciliation failed: records=%d tools=%d audit=%+v err=%v", index.Records, index.Tools, index.ProducerAudit, err)
	}
}

func TestNativeSpanCollectionRejectsOverBudgetSourceAfterIsolation(t *testing.T) {
	s := &testSandbox{nativeData: make([]byte, nativeSpanByteLimit+1)}
	e := testEmbedded(t, s, &testNative{})
	if err := startEmbedded(t, e, s); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Run(context.Background(), "task"); err != nil {
		t.Fatal(err)
	}
	if err := e.Stop(context.Background()); err != nil || !e.isolated {
		t.Fatal("oversized observation prevented isolation", err)
	}
	if obs, err := e.NativeToolObservation(); err == nil || obs.Status != core.StatusFailed {
		t.Fatal("oversized native source was accepted")
	}
	raw, err := os.ReadFile(filepath.Join(e.artifactDir, "native-spans-index.json"))
	var index nativeSpanIndex
	if err != nil || json.Unmarshal(raw, &index) != nil || !index.Truncated || index.Records != 0 || len(index.Files) != 1 || index.Files[0].RetainedBytes != 0 {
		t.Fatal("oversized source loss was not explicit")
	}
}

func TestL0DisablesNativeSpanCollectionAndConfigRequest(t *testing.T) {
	s := &testSandbox{}
	e := testEmbedded(t, s, &testNative{})
	e.options.ObservationLevel = performance.ObservationL0
	if err := startEmbedded(t, e, s); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Run(context.Background(), "task"); err != nil {
		t.Fatal(err)
	}
	declaration, err := os.ReadFile(filepath.Join(e.artifactDir, "native-events-index.json"))
	if err != nil || !bytes.Contains(declaration, []byte(`"official_span_collection_enabled": false`)) {
		t.Fatal("L0 disabled official span collection declaration missing")
	}
	if err := e.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if json.Unmarshal(s.staged, &spec) != nil || spec["native_observation"] != false || s.inventoryCalls != 0 || s.downloadCalls != 0 {
		t.Fatal("L0 requested or collected native tracing")
	}
	obs, err := e.NativeToolObservation()
	if err != nil || obs.Status != core.StatusUnsupported {
		t.Fatal("existing L0 SSE observation changed")
	}
	raw, err := os.ReadFile(filepath.Join(e.artifactDir, "native-spans-index.json"))
	if err != nil || !bytes.Contains(raw, []byte(`"collection_status": "not_enabled"`)) || !bytes.Contains(raw, []byte(`"enabled": false`)) {
		t.Fatal("L0 disabled span receipt missing")
	}
}

func TestMissingProcessEvidenceDoesNotEraseNativeSpansOrIsolation(t *testing.T) {
	s := &testSandbox{processReadFails: true}
	e := testEmbedded(t, s, &testNative{})
	if err := startEmbedded(t, e, s); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Run(context.Background(), "task"); err != nil {
		t.Fatal(err)
	}
	if err := e.Stop(context.Background()); err != nil || !e.isolated {
		t.Fatalf("process observation failure changed confirmed isolation: %v", err)
	}
	obs, err := e.NativeToolObservation()
	if err == nil || obs.Status != core.StatusFailed {
		t.Fatal("missing process files were silently accepted")
	}
	raw, err := os.ReadFile(filepath.Join(e.artifactDir, "native-spans-index.json"))
	var index nativeSpanIndex
	if err != nil || json.Unmarshal(raw, &index) != nil || index.ProcessExpected == nil || !*index.ProcessExpected || len(index.Spans) == 0 {
		t.Fatal("process declaration or independent native evidence was lost")
	}
}

func TestLegacyImageKeepsSDKSpansWithoutProcessEvidenceDownloads(t *testing.T) {
	s := &testSandbox{processUnsupported: true, processReadFails: true}
	e := testEmbedded(t, s, &testNative{})
	if err := startEmbedded(t, e, s); err != nil {
		t.Fatal(err)
	}
	result, err := e.Run(context.Background(), "task")
	if err != nil || result.Status != core.StatusSucceeded {
		t.Fatal("legacy native execution changed", err)
	}
	if err := e.Stop(context.Background()); err != nil || !e.isolated {
		t.Fatal("legacy native isolation changed", err)
	}
	obs, err := e.NativeToolObservation()
	if err != nil || obs.Status == core.StatusFailed || s.downloadCalls != 3 {
		t.Fatalf("legacy SDK collection=%+v downloads=%d err=%v", obs, s.downloadCalls, err)
	}
	raw, err := os.ReadFile(filepath.Join(e.artifactDir, "native-spans-index.json"))
	var index nativeSpanIndex
	if err != nil || json.Unmarshal(raw, &index) != nil || index.ProcessExpected == nil || *index.ProcessExpected || index.ProcessTiming != "unsupported" || len(index.Spans) == 0 {
		t.Fatal("legacy process capability or SDK spans were misreported")
	}
	if _, err := os.Stat(filepath.Join(e.artifactDir, "jiuwen-process-index.json")); !os.IsNotExist(err) {
		t.Fatal("legacy image attempted process evidence collection")
	}
}

func TestNativeProviderObservationIsTypedAndRejectsMalformedMetadata(t *testing.T) {
	observation := map[string]any{"schema_version": 1, "invocation_id": "call-transport", "operation": map[string]any{"kind": "shell", "method": "execute_cmd"}, "clock": map[string]any{"emitter_id": "agent", "clock_domain": "jiuwen-provider/agent", "origin_monotonic_ns": 1000, "origin_unix_ns": 1793000000000000000, "anchor_before_monotonic_ns": 998, "anchor_after_monotonic_ns": 1002}, "start_offset_ns": 1, "end_offset_ns": 9, "outcome": "error", "phases": []map[string]any{{"name": "http_submit", "offset_ns": 2}}, "private_unrecognized": "DO_NOT_EXPORT"}
	raw, _ := json.Marshal(observation)
	for _, bad := range []bool{false, true} {
		value := string(raw)
		if bad {
			value = "malformed"
		}
		attributes, _ := json.Marshal([]any{map[string]any{"key": "gen_ai.operation.name", "value": map[string]string{"stringValue": "aries_provider_request"}}, map[string]any{"key": "aries.provider.observation", "value": map[string]string{"stringValue": value}}})
		line := nativeTestLine("1111111111111111", "", "s", "r", "")
		line = bytes.Replace(line, []byte(`"attributes":[`), append(append([]byte(`"attributes":[`), attributes[1:len(attributes)-1]...), ','), 1)
		index := newNativeSpanIndex("s", "r")
		index.read("source", line)
		if (index.Invalid > 0) != bad {
			t.Fatal("provider malformed metadata not classified", index.Invalid)
		}
		encoded, _ := json.Marshal(index)
		if bytes.Contains(encoded, []byte("DO_NOT_EXPORT")) {
			t.Fatal("unknown provider attribute exported")
		}
		if !bad && (len(index.Spans) != 1 || index.Spans[0].Provider == nil || index.Spans[0].Provider.Invocation != "call-transport") {
			t.Fatal("typed provider observation missing")
		}
	}
}

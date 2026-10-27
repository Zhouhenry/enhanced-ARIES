package jiuwenswarm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyscale-lab/aries/pkg/core"
)

func nativeProducerFixture(spans []nativeSpanIdentity) (nativeProducerStatus, []byte) {
	var ledger []byte
	sequence := 0
	appendEvent := func(row nativeProducerEvent) {
		sequence++
		row.Schema, row.Sequence = 1, sequence
		data, _ := json.Marshal(row)
		ledger = append(ledger, append(data, '\n')...)
	}
	for _, span := range spans {
		row := nativeProducerEvent{Event: "start", Trace: span.TraceID, Span: span.SpanID, Parent: span.ParentSpanID, Start: span.Start, Tool: span.ToolCallID}
		appendEvent(row)
		row.Event, row.End, row.Status = "end", span.End, span.StatusCode
		appendEvent(row)
	}
	appendEvent(nativeProducerEvent{Event: "force_flush"})
	appendEvent(nativeProducerEvent{Event: "processor_shutdown"})
	status := nativeProducerStatus{Schema: 1, Protocol: "jiuwen-native-span-audit-v1", RunID: "run", TaskID: "task", PID: 42,
		Before: true, Attached: true, Status: "complete", Started: len(spans), Ended: len(spans), Records: sequence,
		Bytes: len(ledger), Flushes: 1, Shutdown: true, StopStarted: true, StopReturned: true, SHA: fmt.Sprintf("%x", sha256.Sum256(ledger))}
	return status, ledger
}

func TestNativeProducerReconciliationRejectsLossAndWrongTerminals(t *testing.T) {
	span := nativeSpanIdentity{TraceID: "0123456789abcdef0123456789abcdef", SpanID: "1111111111111111", Start: "1793000000000000001", End: "1793000000000000009", StatusCode: 2, ToolCallID: "call-real"}
	for _, scenario := range []string{"complete", "dropped_whole_pair", "missing_export", "unexpected_export", "duplicate_export", "corrupt_ledger", "duplicate_start", "unended_span", "wrong_exit_status", "wrong_end_time", "wrong_tool_id", "no_shutdown", "write_failure", "not_attached"} {
		t.Run(scenario, func(t *testing.T) {
			spans := []nativeSpanIdentity{span}
			status, ledger := nativeProducerFixture(spans)
			switch scenario {
			case "dropped_whole_pair":
				_, ledger = nativeProducerFixture(nil)
				status.Records -= 2
			case "missing_export":
				spans = nil
			case "unexpected_export":
				spans[0].SpanID = "2222222222222222"
			case "duplicate_export":
				spans = append(spans, span)
			case "corrupt_ledger":
				ledger[0] = '['
			case "duplicate_start":
				ledger = bytes.Replace(ledger, []byte(`"event":"end"`), []byte(`"event":"start"`), 1)
			case "unended_span":
				lines := bytes.Split(ledger, []byte("\n"))
				ledger = bytes.Join(append(lines[:1], lines[2:]...), []byte("\n"))
				ledger = bytes.ReplaceAll(ledger, []byte(`"sequence":3`), []byte(`"sequence":2`))
				ledger = bytes.ReplaceAll(ledger, []byte(`"sequence":4`), []byte(`"sequence":3`))
				status.Ended, status.Records = 0, 3
			case "wrong_exit_status":
				spans[0].StatusCode = 1
			case "wrong_end_time":
				spans[0].End = "1793000000000000010"
			case "wrong_tool_id":
				spans[0].ToolCallID = "call-other"
			case "no_shutdown":
				status.StopReturned = false
			case "write_failure":
				status.WriteFailures = 1
			case "not_attached":
				status.Attached = false
			}
			// Keep envelope hashes valid so loss cases exercise semantic reconciliation.
			status.Bytes, status.SHA = len(ledger), fmt.Sprintf("%x", sha256.Sum256(ledger))
			audit := nativeProducerAudit{Status: "incomplete", Scope: "sampled_sdk_spans"}
			err := audit.reconcile(status, ledger, spans)
			if scenario == "complete" {
				if err != nil || audit.Status != "complete" || audit.Started != 1 || audit.Ended != 1 || audit.Exported != 1 {
					t.Fatalf("valid producer/export reconciliation failed: %+v %v", audit, err)
				}
			} else if err == nil || audit.Status == "complete" {
				t.Fatalf("invalid producer/export evidence accepted: %+v", audit)
			}
		})
	}
}

func TestNativeProducerCollectionFailureDoesNotChangeBusinessOrIsolation(t *testing.T) {
	for _, scenario := range []string{"missing_audit", "foreign_binding", "wrong_pid", "ledger_hash"} {
		t.Run(scenario, func(t *testing.T) {
			s := &testSandbox{producerReadFails: scenario == "missing_audit"}
			e := testEmbedded(t, s, &testNative{})
			if err := startEmbedded(t, e, s); err != nil {
				t.Fatal(err)
			}
			index := newNativeSpanIndex("s", s.nativeRequest)
			s.nativeData = nativeTestLine("1111111111111111", "", "s", s.nativeRequest, "call")
			index.read("traces-2026-10-27.jsonl", s.nativeData)
			status, ledger := nativeProducerFixture(index.Spans)
			switch scenario {
			case "foreign_binding":
				status.RunID = "other-run"
			case "wrong_pid":
				status.PID++
			case "ledger_hash":
				status.SHA = "wrong"
			}
			s.producerStatus, _ = json.Marshal(status)
			s.producerLedger = ledger
			result, err := e.Run(context.Background(), "task")
			if err != nil || result.Status != core.StatusSucceeded {
				t.Fatal("audit changed successful business outcome")
			}
			if err := e.Stop(context.Background()); err != nil || !e.isolated {
				t.Fatal("audit prevented confirmed Stop")
			}
			observation, err := e.NativeToolObservation()
			if err == nil || observation.Status != core.StatusFailed {
				t.Fatal("audit failure silently upgraded")
			}
			raw, err := os.ReadFile(filepath.Join(e.artifactDir, "native-spans-index.json"))
			if err != nil || json.Unmarshal(raw, &index) != nil || index.ProducerAudit == nil || index.ProducerAudit.Status != "incomplete" || index.ProducerCoverage != "unknown_no_independent_denominator_or_export_failure_counter" {
				t.Fatal("partial audit and unknown producer status not retained")
			}
		})
	}
}

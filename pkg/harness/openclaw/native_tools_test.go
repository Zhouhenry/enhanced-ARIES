package openclaw

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/harness"
	"github.com/hyscale-lab/aries/pkg/harness/openclaw/gateway"
)

type scriptedNativeEvents struct {
	frames  chan gateway.Frame
	failure error
}

func (s scriptedNativeEvents) RecvEvent(ctx context.Context) (gateway.Frame, error) {
	select {
	case frame, ok := <-s.frames:
		if !ok {
			return nil, s.failure
		}
		return frame, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func nativeEvent(session, run, stream string, seq int, data map[string]any) gateway.Frame {
	return gateway.Frame{"type": "event", "event": "agent", "payload": map[string]any{"sessionKey": session, "runId": run, "stream": stream, "seq": float64(seq), "ts": float64(1700000000000 + seq), "data": data}}
}
func toolData(phase string) map[string]any {
	return map[string]any{"phase": phase, "toolCallId": "tool-1", "name": "exec"}
}

func collectNativeEvents(t *testing.T, frames []gateway.Frame, limit int, failure error) (nativeToolIndex, []nativeToolEvent, error) {
	t.Helper()
	credentials := harness.NewCredentials("test")
	credentials.Set("proxy", []byte("private-token"))
	t.Cleanup(credentials.Clear)
	c := NewNativeToolCollector("session-1", credentials)
	if limit > 0 {
		c.maxEvents = limit
	}
	input := make(chan gateway.Frame, len(frames))
	for _, frame := range frames {
		input <- frame
	}
	if failure != nil {
		close(input)
	}
	c.StartReading(context.Background(), scriptedNativeEvents{frames: input, failure: failure})
	dir := t.TempDir()
	_, err := c.Finish("run-1", dir, 50*time.Millisecond)
	content, readErr := os.ReadFile(filepath.Join(dir, "native-tool-events.index.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	var index nativeToolIndex
	if e := json.Unmarshal(content, &index); e != nil {
		t.Fatal(e)
	}
	content, readErr = os.ReadFile(filepath.Join(dir, "native-tool-events.jsonl"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if bytes.Contains(content, []byte("private-token")) {
		t.Fatal("proxy token retained")
	}
	var events []nativeToolEvent
	for _, line := range bytes.Split(bytes.TrimSpace(content), []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		var event nativeToolEvent
		if e := json.Unmarshal(line, &event); e != nil {
			t.Fatal(e)
		}
		events = append(events, event)
	}
	return index, events, err
}

func TestNativeToolEventsCorrelateAndKeepClockDomainsAndUnknownExit(t *testing.T) {
	start := toolData("start")
	start["args"] = map[string]any{"command": "echo \"private-token\"", "env": map[string]any{"SECRET": "not-recorded"}}
	result := toolData("result")
	result["result"] = map[string]any{"details": map[string]any{"status": "running"}}
	result["isError"] = false
	command := toolData("end")
	command["durationMs"] = float64(17)
	frames := []gateway.Frame{
		nativeEvent("other-session", "run-1", "tool", 1, start),
		nativeEvent("session-1", "other-run", "tool", 1, toolData("start")),
		nativeEvent("session-1", "run-1", "tool", 1, start),
		nativeEvent("session-1", "run-1", "tool", 3, result),
		nativeEvent("session-1", "run-1", "command_output", 4, command),
		nativeEvent("session-1", "run-1", "lifecycle", 5, map[string]any{"phase": "end"}),
	}
	index, events, err := collectNativeEvents(t, frames, 0, nil)
	if err != nil || !index.Complete || index.DiscardedForeign != 2 || len(events) != 3 {
		t.Fatalf("index=%+v events=%d err=%v", index, len(events), err)
	}
	if events[0].Command == nil || !strings.Contains(*events[0].Command, "[REDACTED]") {
		t.Fatalf("missing redacted command: %+v", events[0])
	}
	if events[2].ExitCode != nil || events[2].NativeDurationMS == nil || *events[2].NativeDurationMS != 17 || events[2].ProcessCompletion != "unknown" {
		t.Fatal("background completion or zero exit invented")
	}
	for _, event := range events {
		if event.RunID != "run-1" || event.SessionKey != "session-1" || event.NativeTimestampMS == nil || event.ObservedOffsetNS < 0 || event.ObservedAt.IsZero() {
			t.Fatalf("uncorrelated/unclocked event: %+v", event)
		}
	}
}

func TestNativeToolObservationFailuresAreVisible(t *testing.T) {
	start := nativeEvent("session-1", "run-1", "tool", 1, toolData("start"))
	result := nativeEvent("session-1", "run-1", "tool", 2, toolData("result"))
	terminal := nativeEvent("session-1", "run-1", "lifecycle", 3, map[string]any{"phase": "end"})
	for _, tc := range []struct {
		name    string
		frames  []gateway.Frame
		limit   int
		failure error
		issue   string
	}{
		{"missing-result", []gateway.Frame{start, terminal}, 0, nil, "incomplete_tool_pair"},
		{"missing-start", []gateway.Frame{result, terminal}, 0, nil, "incomplete_tool_pair"},
		{"missing-terminal", []gateway.Frame{start, result}, 0, nil, "native_terminal_not_observed"},
		{"duplicate", []gateway.Frame{start, start, result, terminal}, 0, nil, "event_sequence_regressed"},
		{"non-tool-sequence-regression", []gateway.Frame{start, nativeEvent("session-1", "run-1", "assistant", 1, nil), result, terminal}, 0, nil, "event_sequence_regressed"},
		{"native-error", []gateway.Frame{start, result, nativeEvent("session-1", "run-1", "error", 3, nil), nativeEvent("session-1", "run-1", "lifecycle", 4, map[string]any{"phase": "error"})}, 0, nil, "native_error_event"},
		{"overflow", []gateway.Frame{start, result, terminal}, 1, nil, "event_limit_exceeded"},
		{"stream-failed", []gateway.Frame{start}, 0, errors.New("private-token transport failed"), "event_stream_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			index, _, err := collectNativeEvents(t, tc.frames, tc.limit, tc.failure)
			if err == nil || index.Complete || !strings.Contains(strings.Join(index.Issues, ","), tc.issue) {
				t.Fatalf("hidden incomplete evidence: %+v %v", index, err)
			}
		})
	}
}

func TestNativeCollectorNonToolTrafficDoesNotConsumeToolLimit(t *testing.T) {
	credentials := harness.NewCredentials("test")
	defer credentials.Clear()
	collector := NewNativeToolCollector("session-1", credentials)
	frames := make(chan gateway.Frame, 20007)
	sequence := 0
	appendEvent := func(stream string, data map[string]any) {
		sequence++
		frames <- nativeEvent("session-1", "run-1", stream, sequence, data)
	}
	for _, id := range []string{"tool-1", "tool-2"} {
		start := toolData("start")
		start["toolCallId"] = id
		appendEvent("tool", start)
		for i := 0; i < 10000; i++ {
			if i%2 == 0 {
				appendEvent("assistant", map[string]any{"delta": "synthetic model text"})
			} else {
				appendEvent("lifecycle", map[string]any{"phase": "start"})
			}
		}
		command := toolData("end")
		command["toolCallId"], command["exitCode"], command["durationMs"] = id, float64(0), float64(17)
		appendEvent("command_output", command)
		result := toolData("result")
		result["toolCallId"], result["isError"] = id, false
		appendEvent("tool", result)
	}
	appendEvent("lifecycle", map[string]any{"phase": "end"})
	collector.StartReading(context.Background(), scriptedNativeEvents{frames: frames})
	directory := t.TempDir()
	report, observationErr := collector.FinishObservation("run-1", directory, 5*time.Second)
	content, err := os.ReadFile(filepath.Join(directory, "native-tool-events.index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var index nativeToolIndex
	if err := json.Unmarshal(content, &index); err != nil {
		t.Fatal(err)
	}
	if observationErr != nil || report.Status != "succeeded" {
		t.Fatalf("non-tool traffic exhausted tool retention: %+v %v", index, observationErr)
	}
	if !index.Complete || !index.TerminalObserved || index.Events != 6 || len(frames) != 0 || len(collector.records) != 6 {
		t.Fatalf("tool tail or native terminal was lost: %+v retained=%d", index, len(collector.records))
	}
	for i, record := range collector.records {
		wantID := "tool-1"
		if i >= 3 {
			wantID = "tool-2"
		}
		if record.ToolCallID != wantID {
			t.Fatalf("tool identity changed: %+v", record)
		}
		if i%3 == 1 && (record.Stream != "command_output" || record.ExitCode == nil || *record.ExitCode != 0 || record.NativeDurationMS == nil || *record.NativeDurationMS != 17) {
			t.Fatalf("command terminal fields lost: %+v", record)
		}
		if i%3 == 2 && (record.Phase != "result" || record.IsError == nil || *record.IsError) {
			t.Fatalf("tool result lost: %+v", record)
		}
	}
}

func TestNativeEmptyToolObservationDoesNotClaimZeroExecutions(t *testing.T) {
	index, events, err := collectNativeEvents(t, []gateway.Frame{nativeEvent("session-1", "run-1", "lifecycle", 1, map[string]any{"phase": "end"})}, 0, nil)
	if err != nil || !index.Complete || len(events) != 0 || index.Coverage != "no_tool_events_observed" {
		t.Fatalf("empty coverage: %+v %v", index, err)
	}
}

func TestNativeCollectorLimitsRetainFailureAndContinueDraining(t *testing.T) {
	for _, limit := range []string{"events", "bytes"} {
		t.Run(limit, func(t *testing.T) {
			credentials := harness.NewCredentials("test")
			defer credentials.Clear()
			collector := NewNativeToolCollector("session-1", credentials)
			if limit == "events" {
				collector.maxEvents = 1
			} else {
				collector.maxBytes = 1
			}
			frames := make(chan gateway.Frame, 5)
			frames <- nativeEvent("session-1", "run-1", "tool", 1, toolData("start"))
			frames <- nativeEvent("session-1", "run-1", "tool", 2, toolData("result"))
			frames <- nativeEvent("session-1", "run-1", "assistant", 3, nil)
			frames <- nativeEvent("session-1", "run-1", "error", 2, nil)
			frames <- nativeEvent("session-1", "run-1", "lifecycle", 4, map[string]any{"phase": "end"})
			collector.StartReading(context.Background(), scriptedNativeEvents{frames: frames})
			directory := t.TempDir()
			report, err := collector.FinishObservation("run-1", directory, time.Second)
			if err == nil || report.Status != "failed" {
				t.Fatal("bounded collector hid incomplete evidence")
			}
			content, err := os.ReadFile(filepath.Join(directory, "native-tool-events.index.json"))
			if err != nil {
				t.Fatal(err)
			}
			var index nativeToolIndex
			if err := json.Unmarshal(content, &index); err != nil {
				t.Fatal(err)
			}
			if index.Complete || !index.TerminalObserved || len(frames) != 0 || len(collector.records) > 1 {
				t.Fatalf("retention limit stopped the consumer or exceeded retained bound: %+v", index)
			}
			limitIssue := "event_limit_exceeded"
			if limit == "bytes" {
				limitIssue = "event_byte_limit_exceeded"
			}
			for _, issue := range []string{limitIssue, "event_sequence_regressed", "native_error_event"} {
				if !strings.Contains(strings.Join(index.Issues, ","), issue) {
					t.Fatalf("drained stream hid %s: %+v", issue, index)
				}
			}
		})
	}
}

func TestNativeCollectorCancellationWithoutAcknowledgementDoesNotInventRun(t *testing.T) {
	credentials := harness.NewCredentials("test")
	defer credentials.Clear()
	c := NewNativeToolCollector("session-1", credentials)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.StartReading(ctx, scriptedNativeEvents{frames: make(chan gateway.Frame)})
	dir := t.TempDir()
	if _, err := c.Finish("", dir, time.Hour); err == nil {
		t.Fatal("unknown run was silently accepted")
	}
	content, err := os.ReadFile(filepath.Join(dir, "native-tool-events.index.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(content, []byte("acknowledged_run_id_unavailable")) || bytes.Contains(content, []byte(`"run_id"`)) {
		t.Fatal("run identity invented")
	}
}

package openclaw

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"path/filepath"
	"sync"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/harness"
	"github.com/hyscale-lab/aries/pkg/harness/internal/privatefiles"
	"github.com/hyscale-lab/aries/pkg/harness/openclaw/gateway"
	"github.com/hyscale-lab/aries/pkg/performance"
)

// These are native event observations, not SDK command spans or proof that a
// background process exited. Native timestamps/durations use the runtime wall
// clock, whether the Gateway runs in the external harness or in the sandbox.
type nativeToolEvent struct {
	RunID             string    `json:"run_id"`
	SessionKey        string    `json:"session_key"`
	Stream            string    `json:"stream"`
	Phase             string    `json:"phase"`
	ToolCallID        string    `json:"tool_call_id,omitempty"`
	Name              string    `json:"name,omitempty"`
	Sequence          int64     `json:"native_sequence"`
	NativeTimestampMS *float64  `json:"native_timestamp_ms"`
	ObservedAt        time.Time `json:"host_observed_at_utc"`
	ObservedOffsetNS  int64     `json:"host_observed_offset_ns"`
	Command           *string   `json:"command"`
	ExitCode          *float64  `json:"exit_code"`
	NativeDurationMS  *float64  `json:"native_duration_ms"`
	IsError           *bool     `json:"is_error"`
	ProcessCompletion string    `json:"process_completion"`
}

type nativeToolIndex struct {
	SchemaVersion      int                `json:"schema_version"`
	RunID              string             `json:"run_id,omitempty"`
	SessionKey         string             `json:"session_key"`
	Complete           bool               `json:"complete"`
	Coverage           string             `json:"coverage"`
	TerminalObserved   bool               `json:"terminal_observed"`
	Events             int                `json:"events"`
	DiscardedForeign   int                `json:"discarded_foreign"`
	Issues             []string           `json:"issues"`
	HostOriginAt       time.Time          `json:"host_origin_at_utc"`
	HostClock          string             `json:"host_clock"`
	NativeClock        string             `json:"native_clock"`
	Clock              *performance.Clock `json:"clock,omitempty"`
	HostOriginOffsetNS *int64             `json:"host_origin_offset_ns,omitempty"`
}

type eventReceiver interface {
	RecvEvent(context.Context) (gateway.Frame, error)
}

// NativeToolCollector retains native Gateway events for both cloud placements.
// Its intervals are controller receive observations, not guest execution spans.
type NativeToolCollector struct {
	mu                  sync.Mutex
	start               time.Time
	clock               *performance.Clock
	session             string
	credentials         *harness.Credentials
	cancel              context.CancelFunc
	done                chan struct{}
	changed             chan struct{}
	records             []nativeToolEvent
	terminal            map[string]bool
	lastSequence        map[string]int64
	issues              []string
	foreign, size       int
	maxEvents, maxBytes int
	retentionStopped    bool
}

// NewNativeToolCollector creates one collector for an exact native session.
func NewNativeToolCollector(session string, credentials *harness.Credentials) *NativeToolCollector {
	return &NativeToolCollector{start: time.Now(), session: session, credentials: credentials,
		done: make(chan struct{}), changed: make(chan struct{}, 1), terminal: make(map[string]bool), lastSequence: make(map[string]int64), maxEvents: 4096, maxBytes: 4 << 20}
}

// NativeToolObservation reports observer-only outcomes after Run, including
// incomplete evidence. Native execution and isolation retain their own results.
func (manager *Manager) NativeToolObservation() (core.ObserverResult, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if !manager.options.ObserveNativeTools || manager.options.Common.Mode != ModeAgent {
		return core.ObserverResult{Status: core.StatusNotEnabled}, nil
	}
	if manager.nativeToolObservation.Status == "" {
		return core.ObserverResult{Status: core.StatusNotRun, Reason: "native tool evidence was not collected"}, nil
	}
	report := manager.nativeToolObservation
	report.LogPaths = append([]string(nil), report.LogPaths...)
	return report, manager.nativeToolObservationErr
}

// StartReading drains events until Finish cancels and joins this consumer.
func (c *NativeToolCollector) StartReading(parent context.Context, receiver eventReceiver) {
	c.clock = performance.ClockFromContext(parent)
	ctx, cancel := context.WithCancel(context.WithoutCancel(parent))
	c.cancel = cancel
	go func() {
		defer close(c.done)
		for {
			frame, err := receiver.RecvEvent(ctx)
			if err != nil {
				if ctx.Err() == nil {
					c.mu.Lock()
					c.issue("event_stream_failed")
					c.mu.Unlock()
				}
				return
			}
			c.mu.Lock()
			c.observe(frame)
			c.mu.Unlock()
			select {
			case c.changed <- struct{}{}:
			default:
			}
		}
	}()
}

func (c *NativeToolCollector) issue(code string) {
	for _, previous := range c.issues {
		if previous == code {
			return
		}
	}
	c.issues = append(c.issues, code)
}

func (c *NativeToolCollector) observe(frame gateway.Frame) {
	if frame.String("event") != "agent" {
		return
	}
	p := gateway.Frame(frame.Map("payload"))
	if p.String("sessionKey") != c.session {
		c.foreign++
		return
	}
	runID := p.String("runId")
	seq := nativeNumber(p["seq"])
	if runID == "" || len(runID) > 512 || seq == nil || *seq < 1 || *seq > 9007199254740991 || math.Trunc(*seq) != *seq {
		c.issue("invalid_event_identity")
		return
	}
	data := gateway.Frame(p.Map("data"))
	stream, phase := p.String("stream"), data.String("phase")
	if stream == "lifecycle" && (phase == "end" || phase == "error") {
		c.terminal[runID] = true
	}
	if int64(*seq) <= c.lastSequence[runID] {
		c.issue("event_sequence_regressed")
	}
	c.lastSequence[runID] = int64(*seq)
	if stream == "error" {
		c.issue("native_error_event")
	}
	// A full observer must still drain and validate the native connection.
	if c.retentionStopped {
		return
	}
	if stream != "tool" && stream != "command_output" {
		return
	}
	if stream == "tool" && phase != "start" && phase != "result" {
		return
	}
	if stream == "command_output" && phase != "end" {
		return
	}
	toolID, name := data.String("toolCallId"), data.String("name")
	if toolID == "" || len(toolID) > 512 || len(name) > 128 {
		c.issue("invalid_tool_identity")
		return
	}
	// Model deltas and lifecycle events do not consume tool retention capacity.
	if len(c.records) >= c.maxEvents {
		c.issue("event_limit_exceeded")
		c.retentionStopped = true
		return
	}
	now := time.Now()
	r := nativeToolEvent{RunID: runID, SessionKey: c.session, Stream: stream, Phase: phase, ToolCallID: toolID, Name: name, Sequence: int64(*seq), NativeTimestampMS: nativeNumber(p["ts"]), ObservedAt: now.UTC(), ObservedOffsetNS: now.Sub(c.start).Nanoseconds(), ProcessCompletion: "unknown"}
	if stream == "tool" && phase == "start" {
		if command, ok := data.Map("args")["command"].(string); ok {
			if len(command) > 64<<10 {
				c.issue("command_limit_exceeded")
				return
			}
			value := string(c.credentials.Redact([]byte(command)))
			r.Command = &value
		}
	}
	if value, ok := data["isError"].(bool); ok {
		r.IsError = &value
	}
	if stream == "command_output" {
		r.ExitCode = nativeNumber(data["exitCode"])
		r.NativeDurationMS = nativeNumber(data["durationMs"])
		// Even phase=end may describe a background launch. Preserve only actual
		// fields; a missing exit code is not zero or confirmed process completion.
	}
	content, _ := json.Marshal(r)
	c.size += len(content)
	if c.size > c.maxBytes {
		c.issue("event_byte_limit_exceeded")
		c.retentionStopped = true
		return
	}
	c.records = append(c.records, r)
}

func nativeNumber(value any) *float64 {
	n, ok := value.(float64)
	if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
		return nil
	}
	return &n
}

// Finish bounds terminal draining, joins the reader and writes private evidence.
func (c *NativeToolCollector) Finish(runID, directory string, drainTimeout time.Duration) ([]string, error) {
	if runID != "" {
		timer := time.NewTimer(drainTimeout)
		defer timer.Stop()
	drain:
		for {
			c.mu.Lock()
			terminal := c.terminal[runID]
			c.mu.Unlock()
			if terminal {
				break
			}
			select {
			case <-c.changed:
			case <-c.done:
				break drain
			case <-timer.C:
				break drain
			}
		}
	}
	c.cancel()
	<-c.done
	c.mu.Lock()
	defer c.mu.Unlock()
	index := nativeToolIndex{SchemaVersion: 1, RunID: runID, SessionKey: c.session, TerminalObserved: c.terminal[runID], DiscardedForeign: c.foreign, HostOriginAt: c.start.UTC(), HostClock: "controller_monotonic_consume_offset; UTC consumer timestamp", NativeClock: "native_runtime_wall_clock; no cross-clock subtraction", Coverage: "no_tool_events_observed"}
	if c.clock != nil {
		offset := c.clock.Offset(c.start)
		index.Clock, index.HostOriginOffsetNS = c.clock, &offset
	}
	if runID == "" {
		c.issue("acknowledged_run_id_unavailable")
	}
	if !index.TerminalObserved {
		c.issue("native_terminal_not_observed")
	}
	var content bytes.Buffer
	encoder := json.NewEncoder(&content)
	starts, results := make(map[string]int), make(map[string]int)
	names := make(map[string]string)
	commandEnds := make(map[string]bool)
	for _, r := range c.records {
		if runID == "" || r.RunID != runID {
			index.DiscardedForeign++
			continue
		}
		if r.Stream == "tool" {
			if name, seen := names[r.ToolCallID]; seen && name != r.Name {
				c.issue("tool_name_changed")
			}
			names[r.ToolCallID] = r.Name
			if r.Phase == "start" {
				starts[r.ToolCallID]++
			} else {
				results[r.ToolCallID]++
			}
		}
		if r.Stream == "command_output" {
			commandEnds[r.ToolCallID] = true
		}
		if err := encoder.Encode(r); err != nil {
			return nil, err
		}
		index.Events++
	}
	for id, count := range starts {
		if count != 1 || results[id] != 1 {
			c.issue("incomplete_tool_pair")
		}
	}
	for id, count := range results {
		if count != 1 || starts[id] != 1 {
			c.issue("incomplete_tool_pair")
		}
	}
	for id := range commandEnds {
		if starts[id] != 1 || results[id] != 1 {
			c.issue("incomplete_tool_pair")
		}
	}
	if index.Events > 0 {
		index.Coverage = "native_events_observed"
	}
	index.Issues = append([]string{}, c.issues...)
	index.Complete = len(index.Issues) == 0
	metadata, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, artifact := range []struct {
		name string
		data []byte
	}{{"native-tool-events.jsonl", content.Bytes()}, {"native-tool-events.index.json", metadata}} {
		p := filepath.Join(directory, artifact.name)
		if err := privatefiles.WriteArtifact(p, c.credentials.Redact(artifact.data)); err != nil {
			return paths, err
		}
		paths = append(paths, p)
	}
	if !index.Complete {
		return paths, errors.New("native tool observation is incomplete; see private event index")
	}
	return paths, nil
}

// FinishObservation reports collection separately from native execution. Its
// duration covers terminal drain and artifact persistence after Agent returns.
func (c *NativeToolCollector) FinishObservation(runID, directory string, drainTimeout time.Duration) (core.ObserverResult, error) {
	started := time.Now()
	paths, err := c.Finish(runID, directory, drainTimeout)
	err = c.credentials.RedactErr(err)
	report := core.ObserverResult{Status: core.StatusSucceeded, Duration: time.Since(started), LogPaths: paths,
		Reason: "native event receipt evidence only; process completion and independent coverage require separate proof"}
	if err != nil {
		report.Status, report.Error = core.StatusFailed, err.Error()
	}
	return report, err
}

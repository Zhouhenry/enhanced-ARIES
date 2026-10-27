package performance

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const nativeProcessMarker = "ARIES_NATIVE_PROCESS "
const nativeProcessPrefix = "[aries/native-process] " + nativeProcessMarker

// The Gateway CLI adds this compact console envelope; it is not a clock anchor.
func nativeProcessPayload(line []byte) ([]byte, bool) {
	if bytes.HasPrefix(line, []byte(nativeProcessPrefix)) {
		return line[len(nativeProcessPrefix):], true
	}
	stamp, rest, found := bytes.Cut(line, []byte(" "))
	if !found || !bytes.HasPrefix(rest, []byte(nativeProcessPrefix)) {
		return nil, false
	}
	const layout = "2006-01-02T15:04:05.000-07:00"
	parsed, err := time.Parse(layout, string(stamp))
	if err != nil || parsed.Format(layout) != string(stamp) {
		return nil, false
	}
	return rest[len(nativeProcessPrefix):], true
}

type nativeProcessRow struct {
	Schema         int             `json:"schema_version"`
	Scope          string          `json:"scope"`
	Mode           string          `json:"mode"`
	Event          string          `json:"event"`
	ToolCallID     string          `json:"tool_call_id"`
	RunID          string          `json:"native_run_id"`
	AgentSessionID string          `json:"native_agent_session_id"`
	SessionKey     string          `json:"native_session_key"`
	ProcessSession string          `json:"process_session_id"`
	SupervisorRun  string          `json:"supervisor_run_id"`
	PID            int             `json:"pid"`
	EmitterPID     int             `json:"emitter_pid"`
	Emitter        string          `json:"emitter_id"`
	Clock          string          `json:"clock_id"`
	Domain         string          `json:"clock_domain"`
	Origin         string          `json:"clock_origin_monotonic_ns"`
	Observed       string          `json:"observed_monotonic_ns"`
	Sequence       uint64          `json:"sequence"`
	PreviousFailed *bool           `json:"previous_emit_failed"`
	Code           json.RawMessage `json:"code"`
	Signal         *string         `json:"signal"`
	WaitSource     string          `json:"source"`
}

type nativeProcessClock struct {
	domain, emitter string
	origin, latest  int64
	pid             int
	sequence        uint64
}

type nativeProcessState struct {
	id         string
	first      nativeProcessRow
	identity   AnalysisIdentity
	points     map[string]AnalysisEvent
	attributes map[string]any
}

// Gateway logs survive Run through Stop. They provide callback observations in
// the native process clock, never an anchor into the controller's timeline.
func (s *AnalysisSnapshot) nativeProcessSnapshot(root string, attempt traceAttempt) {
	file := filepath.Join(attempt.OccurrenceID, "harness", "gateway.log")
	data, err := os.ReadFile(filepath.Join(root, file))
	if os.IsNotExist(err) || err == nil && !bytes.Contains(data, []byte(nativeProcessMarker)) {
		return // Archived runs without the opt-in source retain their prior projection.
	}
	ref := s.source(file, data, "native_process_monotonic", "unaligned_native_monotonic", err)
	coverage := SourceCoverage{Source: "native_process", AttemptID: attempt.AttemptID, Integrity: "incomplete", Status: "incomplete",
		Missing: []string{"independent_expected_process_count", "all_descendants_termination", "pty_pipe_drain"}}
	defer func() { s.Coverage = append(s.Coverage, coverage) }()
	if err != nil {
		s.problem("native process log unavailable for " + attempt.AttemptID)
		return
	}
	resultData, resultErr := os.ReadFile(filepath.Join(root, attempt.OccurrenceID, "harness", "agent-result.json"))
	var result struct {
		SessionID string `json:"requested_native_session_id"`
	}
	if resultErr != nil || decodeDocument(resultData, &result) != nil || !validAnalysisIdentity(result.SessionID) {
		s.problem("native process Agent session identity unproven for " + attempt.AttemptID)
		return
	}
	// toolSnapshot has already validated its index, Agent result and exact pairs.
	tools := make(map[string]AnalysisEvent)
	for _, point := range s.Events {
		if point.Category == "native_tool" && point.Name == "tool.start" && point.AttemptID == attempt.AttemptID {
			tools[point.ToolCallID] = point
		}
	}
	clocks := make(map[string]*nativeProcessClock)
	clockOrder := []string{}
	processes := make(map[string]*nativeProcessState)
	processOrder := []string{}
	points := []AnalysisEvent{}
	var lineNumber uint64
	var schema int
	failedEmission := false
	valid := traceLines(data, func(line []byte) bool {
		lineNumber++
		if !bytes.Contains(line, []byte(nativeProcessMarker)) {
			return !bytes.Equal(line, []byte("[ARIES gateway log truncated]"))
		}
		coverage.ObservedEvents++
		payload, found := nativeProcessPayload(line)
		if !found {
			return false
		}
		var row nativeProcessRow
		if decodeDocument(payload, &row) != nil || (row.Schema != 1 && row.Schema != 2) || row.Scope != "native_child_callback_observation" || row.Mode != "child" || row.Domain != "native_process_monotonic" || row.PID <= 0 || row.EmitterPID <= 0 || row.PreviousFailed == nil {
			return false
		}
		if schema == 0 {
			schema = row.Schema
		} else if row.Schema != schema {
			return false
		}
		tool, found := tools[row.ToolCallID]
		if !found || row.RunID != tool.NativeRunID || row.SessionKey != tool.AgentSessionID || row.AgentSessionID != result.SessionID || !validAnalysisIdentity(row.ProcessSession) || !validAnalysisIdentity(row.SupervisorRun) || !validAnalysisIdentity(row.Clock) || !validAnalysisIdentity(row.Emitter) {
			return false
		}
		origin, originErr := strconv.ParseInt(row.Origin, 10, 64)
		observed, observedErr := strconv.ParseInt(row.Observed, 10, 64)
		if originErr != nil || observedErr != nil || origin < 0 || observed < origin {
			return false
		}
		clock := clocks[row.Clock]
		if clock == nil {
			clock = &nativeProcessClock{domain: "native-process/" + attempt.AttemptID + "/" + row.Clock, emitter: row.Emitter, origin: origin, latest: origin, pid: row.EmitterPID}
			clocks[row.Clock] = clock
			clockOrder = append(clockOrder, row.Clock)
		}
		if row.Emitter != clock.emitter || row.EmitterPID != clock.pid || origin != clock.origin || observed < clock.latest || row.Sequence != clock.sequence+1 {
			return false
		}
		clock.latest, clock.sequence = observed, row.Sequence
		failedEmission = failedEmission || *row.PreviousFailed
		key := stableAnalysisID("native-process", attempt.AttemptID, row.Clock, row.SupervisorRun, row.ProcessSession, strconv.Itoa(row.PID))
		process := processes[key]
		if process == nil {
			if row.Event != "child_spawn_return" {
				return false
			}
			process = &nativeProcessState{id: key, first: row, identity: tool.AnalysisIdentity, points: make(map[string]AnalysisEvent), attributes: map[string]any{
				"native_agent_session_id": row.AgentSessionID, "process_session_id": row.ProcessSession, "supervisor_run_id": row.SupervisorRun,
				"pid": row.PID, "emitter_pid": row.EmitterPID, "emitter_id": row.Emitter, "clock_id": row.Clock, "clock_origin_monotonic_ns": row.Origin,
				"native_process_schema_version": row.Schema, "all_descendants_completion": "unknown", "process_completion": "unknown", "pipe_drain": "unknown"}}
			processes[key] = process
			processOrder = append(processOrder, key)
			coverage.Started++
		}
		if row.ToolCallID != process.first.ToolCallID || row.RunID != process.first.RunID || row.SessionKey != process.first.SessionKey || row.AgentSessionID != process.first.AgentSessionID {
			return false
		}
		if _, exists := process.points[row.Event]; exists {
			return false
		}
		attrs := map[string]any{"process_session_id": row.ProcessSession, "supervisor_run_id": row.SupervisorRun, "native_agent_session_id": row.AgentSessionID,
			"pid": row.PID, "emitter_pid": row.EmitterPID, "clock_id": row.Clock, "clock_origin_monotonic_ns": row.Origin, "observed_monotonic_ns": row.Observed,
			"native_process_schema_version": row.Schema, "previous_emit_failed": *row.PreviousFailed}
		switch row.Event {
		case "child_spawn_return", "stdout_end", "stderr_end", "stdout_close", "stderr_close", "observer_dispose":
		case "stdout_first_nonempty", "stderr_first_nonempty":
			stream := strings.TrimSuffix(row.Event, "_first_nonempty")
			if _, ended := process.points[stream+"_end"]; row.Schema != 2 || ended {
				return false
			}
		case "child_exit", "child_close", "wait_settle":
			var code *int
			if len(row.Code) == 0 || json.Unmarshal(row.Code, &code) != nil || code == nil && (row.Signal == nil || *row.Signal == "") {
				return false
			}
			attrs["exit_code"], attrs["exit_signal"] = code, row.Signal
			if row.Event == "child_exit" {
				process.attributes["exit_code"], process.attributes["exit_signal"] = code, row.Signal
			}
			if row.Event == "wait_settle" {
				if row.WaitSource != "child_close" && row.WaitSource != "windows_observed_exit" && row.WaitSource != "force_kill_fallback" {
					return false
				}
				attrs["wait_source"], process.attributes["wait_source"] = row.WaitSource, row.WaitSource
			}
		case "child_error":
			var code *string
			if json.Unmarshal(row.Code, &code) != nil {
				return false
			}
			attrs["error_code"] = code
		default:
			return false
		}
		point := analysisEvent(ref, lineNumber, row.Event, "native_process", clock.domain, "native_child_callback_observation", observed-origin, process.identity, attrs)
		point.Sequence = row.Sequence
		process.points[row.Event] = point
		points = append(points, point)
		return true
	})
	if !valid {
		s.problem("native process log identity, sequence, clock or format invalid for " + attempt.AttemptID + "; process projection withheld")
		return
	}
	for _, key := range clockOrder {
		clock := clocks[key]
		if !s.clock(AnalysisClock{ID: clock.domain, EmitterID: clock.emitter, Alignment: "unaligned_native_monotonic", EvidenceRef: ref}) {
			return
		}
	}
	s.Events = append(s.Events, points...)
	coverage.Integrity, coverage.Status = "complete", "unknown"
	missingExit, missingEOF := false, false
	for _, key := range processOrder {
		process := processes[key]
		start := process.points["child_spawn_return"]
		exit, exited := process.points["child_exit"]
		stdout, hasStdout := process.points["stdout_end"]
		stderr, hasStderr := process.points["stderr_end"]
		missingEOF = missingEOF || !hasStdout || !hasStderr
		process.attributes["first_output"] = "not_collected"
		if process.first.Schema == 2 {
			process.attributes["first_output"] = "unknown"
			var first AnalysisEvent
			for _, stream := range []string{"stdout", "stderr"} {
				point, found := process.points[stream+"_first_nonempty"]
				if found && (first.ID == "" || point.OffsetNS < first.OffsetNS || point.OffsetNS == first.OffsetNS && point.Sequence < first.Sequence) {
					first = point
					process.attributes["first_output_stream"] = stream
				}
			}
			if first.ID != "" {
				process.attributes["first_output"] = "observed"
				s.interval(stableAnalysisID("operation", process.id, "first-output"), "process.spawn_to_first_output", "native_process", "native_spawn_return_to_first_nonempty_pipe_data_callback", "observed", start, first, process.attributes)
			} else if hasStdout && hasStderr && !failedEmission {
				process.attributes["first_output"] = "no_nonempty_chunk_observed"
			}
		}
		if !exited {
			missingExit = true
			continue
		}
		coverage.Paired++
		process.attributes["process_completion"] = "direct_child_exit_observed"
		if hasStdout && hasStderr {
			process.attributes["stdout_end_offset_ns"], process.attributes["stderr_end_offset_ns"] = stdout.OffsetNS, stderr.OffsetNS
			last := stdout
			if stderr.OffsetNS > last.OffsetNS {
				last = stderr
			}
			if last.OffsetNS > exit.OffsetNS {
				process.attributes["pipe_drain"] = "last_natural_eof_after_child_exit"
				s.interval(stableAnalysisID("operation", process.id, "drain"), "process.exit_to_pipe_eof", "native_process", "native_exit_to_last_natural_pipe_end_callback", "observed", exit, last, process.attributes)
			} else {
				// Natural pipe EOF can precede the child's exit callback. Preserve
				// that order without a negative, reversed or fabricated zero span.
				process.attributes["pipe_drain"] = "both_natural_eof_before_or_at_child_exit"
			}
		}
		s.interval(stableAnalysisID("operation", process.id, "lifetime"), "process.spawn_to_exit", "native_process", "native_spawn_return_to_child_exit_callback", "exited", start, exit, process.attributes)
	}
	if missingExit || missingEOF || failedEmission {
		coverage.Integrity, coverage.Status = "incomplete", "incomplete"
		if missingExit {
			coverage.Missing = append(coverage.Missing, "native_child_exit")
		}
		if missingEOF {
			coverage.Missing = append(coverage.Missing, "natural_stdout_stderr_eof")
		}
		if failedEmission {
			coverage.Missing = append(coverage.Missing, "previous_native_emission_failed")
		}
		s.problem("native process lifecycle observations incomplete for " + attempt.AttemptID)
	}
	s.Limitations = append(s.Limitations, strings.Join([]string{
		"Native process intervals are direct-child callback observations on an unaligned Node monotonic clock.",
		"First output is the first nonempty raw stdout/stderr chunk at the native data callback, before decoding; schema v1 did not collect it.",
		"They do not measure all descendants, PTY drain, kernel scheduling or sandbox total time; logger cost is included.",
		"A child exit code, including nonzero, proves the observed exit only, not task success."}, " "))
}

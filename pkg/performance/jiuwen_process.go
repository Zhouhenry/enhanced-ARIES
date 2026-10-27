package performance

import (
	"crypto/sha256"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"time"
)

const jiuwenProcessSource = "jiuwen_native_process"
const jiuwenProcessScope = "observed_process_lifecycle_window_including_creation_and_pipe_closure_not_additive_stage"

type jiuwenProcessIdentity struct {
	Run        string `json:"run_id"`
	Task       string `json:"task_id"`
	Session    string `json:"session_id"`
	Request    string `json:"request_id"`
	Tool       string `json:"tool_call_id"`
	Invocation string `json:"invocation_id"`
	Status     string `json:"identity_status"`
}

type jiuwenProcessIndex struct {
	Schema  int    `json:"schema_version"`
	Source  string `json:"source"`
	Binding struct {
		Run  string `json:"run_id"`
		Task string `json:"task_id"`
	} `json:"binding"`
	Clock    jiuwenWorkerClock `json:"clock"`
	Counters struct {
		Attempted       *uint64 `json:"spawn_attempted"`
		Started         *uint64 `json:"started"`
		Exited          *uint64 `json:"exited"`
		EventsAttempted *uint64 `json:"events_attempted"`
		EventsWritten   *uint64 `json:"events_written"`
		Dropped         *uint64 `json:"dropped"`
		WriteFailed     *uint64 `json:"write_failed"`
		IndexFailed     *uint64 `json:"index_write_failed"`
		LateEvents      *uint64 `json:"late_events"`
		IdentityFailed  *uint64 `json:"identity_failed"`
	} `json:"counters"`
	Pending *uint64 `json:"pending_processes"`
	Closed  bool    `json:"observer_closed"`
	SHA     string  `json:"ledger_sha256"`
	Bytes   *uint64 `json:"written_bytes"`
}

type jiuwenProcessRow struct {
	Schema     int                   `json:"schema_version"`
	Source     string                `json:"source"`
	Sequence   uint64                `json:"sequence"`
	Domain     string                `json:"clock_domain"`
	Monotonic  int64                 `json:"monotonic_ns"`
	Event      string                `json:"event"`
	Process    string                `json:"process_id"`
	PID        *int                  `json:"pid"`
	Identity   jiuwenProcessIdentity `json:"identity"`
	ReturnCode *int                  `json:"returncode"`
	ErrorType  *string               `json:"error_type"`
}

type jiuwenProcessState struct {
	id         string
	identity   jiuwenProcessIdentity
	pid        int
	points     map[string]AnalysisEvent
	last       AnalysisEvent
	attributes map[string]any
}

func (s *AnalysisSnapshot) jiuwenProcessSnapshot(root string, attempt traceAttempt) {
	for _, owner := range []string{"harness", "bridge"} {
		base := filepath.Join(attempt.OccurrenceID, owner)
		declarationName := "native-spans-index.json"
		if owner == "bridge" {
			declarationName = "remote-bridge-index.json"
		}
		declaration, declarationErr := readIdentityEvidence(root, filepath.Join(base, declarationName))
		var declared struct {
			Expected bool `json:"process_observation_expected"`
		}
		if declarationErr == nil && identityJSON(declaration, &declared, false) != nil {
			s.problem("Jiuwen process observation declaration invalid")
			continue
		}
		indexFile, eventFile := filepath.Join(base, "jiuwen-process-index.json"), filepath.Join(base, "jiuwen-process-events.jsonl")
		raw, indexErr := readIdentityEvidence(root, indexFile)
		data, eventErr := readIdentityEvidence(root, eventFile)
		if !declared.Expected && os.IsNotExist(indexErr) && os.IsNotExist(eventErr) {
			continue
		}
		s.jiuwenProcessSourceSnapshot(attempt, owner, indexFile, eventFile, raw, data, indexErr, eventErr)
	}
}

func (s *AnalysisSnapshot) jiuwenProcessSourceSnapshot(a traceAttempt, owner, indexFile, eventFile string, raw, data []byte, indexErr, eventErr error) {
	indexRef := s.source(indexFile, raw, "identity_only", "not_a_clock", indexErr)
	coverage := SourceCoverage{Source: "jiuwen_process_" + owner, AttemptID: a.AttemptID, Integrity: "incomplete", Status: "incomplete",
		Missing: []string{"all_descendants_termination", "natural_pipe_eof", "cross_host_clock_alignment"}}
	defer func() { s.Coverage = append(s.Coverage, coverage) }()
	var index jiuwenProcessIndex
	if indexErr != nil || eventErr != nil || identityJSON(raw, &index, false) != nil || index.Schema != 1 || index.Source != jiuwenProcessSource || index.Binding.Run != s.RunID || index.Binding.Task != a.OccurrenceID {
		s.source(eventFile, data, "identity_only", "not_a_clock", eventErr)
		s.problem("Jiuwen process index or events unavailable, invalid or bound to another attempt")
		return
	}
	c, n := index.Clock, index.Counters
	ref := s.source(eventFile, data, c.Domain, "local_monotonic_anchor", nil)
	if !nativeSpanID(c.Emitter, 32) || c.Domain != "jiuwen-process/"+c.Emitter || c.Before <= 0 || c.After < c.Before || c.Origin != c.Before+(c.After-c.Before)/2 || c.Unix <= 0 {
		s.problem("Jiuwen process monotonic clock anchor invalid")
		return
	}
	if n.Attempted == nil || n.Started == nil || n.Exited == nil || n.EventsAttempted == nil || n.EventsWritten == nil || n.Dropped == nil || n.WriteFailed == nil || n.IndexFailed == nil || n.LateEvents == nil || n.IdentityFailed == nil || index.Pending == nil || index.Bytes == nil {
		s.problem("Jiuwen process producer counters missing")
		return
	}
	coverage.ExpectedOperations, coverage.ExpectedEvents = n.Attempted, n.EventsAttempted
	coverage.DroppedEvents, coverage.WriteFailedEvents = n.Dropped, n.WriteFailed
	if *index.Bytes != uint64(len(data)) || index.SHA != fmt.Sprintf("%x", sha256.Sum256(data)) {
		s.problem("Jiuwen process ledger hash or byte count differs")
		return
	}
	states := map[string]*jiuwenProcessState{}
	order := []string{}
	points := []AnalysisEvent{}
	var sequence, started, exited uint64
	valid := traceLines(data, func(line []byte) bool {
		var row jiuwenProcessRow
		if identityJSON(line, &row, false) != nil || row.Schema != 1 || row.Source != jiuwenProcessSource || row.Sequence != sequence+1 || row.Domain != c.Domain || row.Monotonic < c.Origin || !nativeSpanID(row.Process, 32) {
			return false
		}
		identity := row.Identity
		remoteIdentity := jiuwenRemoteIdentity{Run: identity.Run, Task: identity.Task, Session: identity.Session, Request: identity.Request, Tool: identity.Tool, Status: identity.Status}
		if !remoteIdentity.valid(s, a) || owner == "bridge" && !validAnalysisIdentity(identity.Invocation) || owner == "harness" && identity.Invocation != "" || row.ErrorType != nil && !nativeLabel(*row.ErrorType) {
			return false
		}
		sequence = row.Sequence
		state := states[row.Process]
		if state == nil {
			if row.Event != "spawn_requested" || row.PID != nil || row.ReturnCode != nil {
				return false
			}
			id := stableAnalysisID("jiuwen-process", a.AttemptID, c.Domain, row.Process)
			state = &jiuwenProcessState{id: id, identity: identity, points: map[string]AnalysisEvent{}, attributes: map[string]any{
				"process_id": row.Process, "invocation_id": identity.Invocation, "identity_status": identity.Status, "emitter_id": c.Emitter,
				"source_owner": owner, "anchor_uncertainty_ns": (c.After - c.Before + 1) / 2, "cross_host_alignment": "unverified",
				"all_descendants_completion": "unknown", "natural_pipe_eof": "unproven", "process_completion": "unknown", "pipe_closure": "unknown", "first_output": "unknown"}}
			states[row.Process] = state
			order = append(order, row.Process)
		} else if row.Event == "spawn_requested" || identity != state.identity {
			return false
		}
		if _, duplicate := state.points[row.Event]; duplicate {
			return false
		}
		if row.Event == "connection_made" {
			if row.PID == nil || *row.PID <= 0 || state.pid != 0 {
				return false
			}
			state.pid = *row.PID
			state.attributes["pid"] = state.pid
			started++
		} else if state.pid > 0 {
			if row.PID == nil || *row.PID != state.pid {
				return false
			}
		} else if row.PID != nil {
			return false
		}
		attrs := maps.Clone(state.attributes)
		attrs["parent_analysis_operation_id"], attrs["observed_monotonic_ns"] = state.id, row.Monotonic
		if row.ErrorType != nil {
			attrs["error_type"] = *row.ErrorType
		}
		switch row.Event {
		case "spawn_requested", "connection_made":
		case "spawn_return":
			if state.pid == 0 || state.points["spawn_failed"].ID != "" {
				return false
			}
		case "spawn_failed":
			if row.ErrorType == nil || state.points["spawn_return"].ID != "" {
				return false
			}
			state.attributes["spawn_error_type"] = *row.ErrorType
		case "stdout_first", "stderr_first":
			closed := "stdout_closed"
			if row.Event == "stderr_first" {
				closed = "stderr_closed"
			}
			if state.pid == 0 || state.points[closed].ID != "" {
				return false
			}
		case "child_exit":
			if state.pid == 0 || row.ReturnCode == nil {
				return false
			}
			exited++
			attrs["returncode"] = *row.ReturnCode
			state.attributes["returncode"] = *row.ReturnCode
		case "stdout_closed", "stderr_closed":
			if state.pid == 0 {
				return false
			}
			attrs["pipe_closure"] = "pipe_closed_observed"
		default:
			return false
		}
		if row.Event != "child_exit" && row.ReturnCode != nil {
			return false
		}
		point := analysisEvent(ref, row.Sequence, "jiuwen.process."+row.Event, "jiuwen_process", c.Domain, jiuwenProcessScope, row.Monotonic-c.Origin, jiuwenRemoteAnalysisIdentity(a, remoteIdentity), attrs)
		state.points[row.Event] = point
		if state.last.ID == "" || point.OffsetNS > state.last.OffsetNS {
			state.last = point
		}
		points = append(points, point)
		return true
	})
	if !valid {
		s.problem("Jiuwen process event identity, sequence, clock or lifecycle invalid")
		return
	}
	origin := time.Unix(0, c.Unix).UTC()
	if !s.clock(AnalysisClock{ID: c.Domain, EmitterID: c.Emitter, OriginUTC: &origin, Alignment: "local_monotonic_anchor", EvidenceRef: indexRef}) {
		return
	}
	s.Events = append(s.Events, points...)
	coverage.ObservedEvents, coverage.Started, coverage.Paired = sequence, uint64(len(states)), 0
	recordingComplete := index.Closed && *n.EventsAttempted == *n.EventsWritten && *n.EventsWritten == sequence && *n.Dropped == 0 && *n.WriteFailed == 0 && *n.IndexFailed == 0 && *n.LateEvents == 0 && *n.IdentityFailed == 0 && *index.Pending == 0 && *n.Attempted == uint64(len(states)) && *n.Started == started && *n.Exited == exited
	complete := recordingComplete
	for _, process := range order {
		state := states[process]
		request := state.points["spawn_requested"]
		connection, hasConnection := state.points["connection_made"]
		exit, hasExit := state.points["child_exit"]
		stdout, hasStdout := state.points["stdout_closed"]
		stderr, hasStderr := state.points["stderr_closed"]
		_, returned := state.points["spawn_return"]
		_, failed := state.points["spawn_failed"]
		terminal := failed && !hasConnection || hasConnection && hasExit && hasStdout && hasStderr && (returned || failed)
		associationComplete := s.jiuwenProcessIdentityLinks(a, state, request.AnalysisIdentity, ref)
		status, outcome := "complete", "observed"
		if !terminal || !recordingComplete || !associationComplete {
			status, outcome = "partial", "partial"
			complete = false
		}
		if terminal {
			coverage.Paired++
		}
		if failed && !hasConnection {
			outcome = "failed"
		}
		state.attributes["observation_status"] = status
		if hasExit {
			state.attributes["process_completion"] = "direct_child_exit_observed"
		}
		if hasStdout && hasStderr {
			state.attributes["pipe_closure"] = "pipe_closed_observed"
		}
		for _, name := range []string{"stdout_first", "stderr_first"} {
			if first, found := state.points[name]; found {
				state.attributes[name+"_offset_ns"] = first.OffsetNS
				state.attributes["first_output"] = "observed"
			}
		}
		if state.attributes["first_output"] == "unknown" && hasStdout && hasStderr && recordingComplete {
			state.attributes["first_output"] = "no_nonempty_chunk_observed"
		}
		s.interval(state.id, "jiuwen.process.observation", "jiuwen_process", jiuwenProcessScope, outcome, request, state.last, state.attributes)
		if hasConnection {
			s.interval(stableAnalysisID("jiuwen-process-creation", state.id), "jiuwen.process.creation", "jiuwen_process", "subprocess_creation_request_to_connection_made_callback", "observed", request, connection, state.attributes)
		}
		if hasExit {
			s.interval(stableAnalysisID("jiuwen-process-command", state.id), "jiuwen.process.command", "jiuwen_process", "connection_made_to_direct_child_exit_callback_not_descendant_completion", "exited", connection, exit, state.attributes)
			if hasStdout && hasStderr {
				lastClosed := stdout
				if stderr.OffsetNS > stdout.OffsetNS {
					lastClosed = stderr
				}
				if lastClosed.OffsetNS > exit.OffsetNS {
					s.interval(stableAnalysisID("jiuwen-process-pipe-close", state.id), "jiuwen.process.exit_to_pipe_closed", "jiuwen_process", "direct_child_exit_to_last_pipe_closed_callback_not_natural_eof_or_consumer_drain", "observed", exit, lastClosed, state.attributes)
				} else {
					state.attributes["pipe_closure_order"] = "both_closed_before_or_at_child_exit"
				}
			}
		}
	}
	if complete {
		coverage.Integrity, coverage.Status = "complete", "unknown"
	} else {
		coverage.Missing = append(coverage.Missing, "producer_counters_close_terminal_callbacks_or_exact_identity")
		s.problem("Jiuwen process producer counters, closure, terminal observations or exact identity incomplete")
	}
	s.Limitations = append(s.Limitations, "Jiuwen process timings are user-space direct-child and pipe-closure callbacks on an independent local monotonic clock. Pipe closure is not natural EOF or consumer drain; the observation container overlaps its creation/command intervals and must not be added to stages. Missing callbacks remain unknown, including background or canceled work.")
}

func (s *AnalysisSnapshot) jiuwenProcessIdentityLinks(a traceAttempt, state *jiuwenProcessState, identity AnalysisIdentity, ref string) bool {
	state.attributes["native_tool_association"], state.attributes["provider_association"] = "missing_identity", "not_applicable"
	complete := true
	if state.identity.Status == "known" && (identity.AgentSessionID == "" || identity.RequestInstanceID == "") {
		s.problem("Jiuwen process known Tool identity lacks a session or request")
		complete = false
	}
	if identity.AgentSessionID != "" && identity.RequestInstanceID != "" && identity.ToolCallID != "" {
		matches := []AnalysisNativeSpan{}
		for _, span := range s.NativeSpans {
			if span.AttemptID == a.AttemptID && span.AuthoritativeTool && span.AnalysisIdentity == identity {
				matches = append(matches, span)
			}
		}
		state.attributes["native_tool_association"] = "not_observed"
		if len(matches) != 1 {
			s.problem("Jiuwen process native Tool identity is unobserved or ambiguous")
			complete = false
		}
		if len(matches) == 1 {
			state.attributes["native_tool_association"] = "exact_id"
			s.Edges = append(s.Edges, AnalysisEdge{From: matches[0].ID, To: state.id, Kind: "process_tool_identity", Correlation: "exact_id", EvidenceRefs: []string{matches[0].EvidenceRef, ref}})
		}
	}
	if state.identity.Invocation != "" {
		state.attributes["provider_association"] = "not_observed"
		matches := []AnalysisInterval{}
		for _, interval := range s.Intervals {
			if interval.AttemptID == a.AttemptID && interval.Category == "jiuwen_remote_provider" && interval.Attributes["invocation_id"] == state.identity.Invocation {
				matches = append(matches, interval)
			}
		}
		if len(matches) == 0 {
			s.problem("Jiuwen process Provider invocation identity has no observed Provider interval")
			return false
		}
		if len(matches) > 1 || len(matches) == 1 && matches[0].AnalysisIdentity != identity {
			s.problem("Jiuwen process and Provider invocation identity differ or are ambiguous")
			return false
		}
		if len(matches) == 1 {
			state.attributes["provider_association"] = "exact_id"
			s.Edges = append(s.Edges, AnalysisEdge{From: matches[0].ID, To: state.id, Kind: "process_invocation_identity", Correlation: "exact_id", EvidenceRefs: append([]string{ref}, matches[0].EvidenceRefs...)})
		}
	}
	return complete
}

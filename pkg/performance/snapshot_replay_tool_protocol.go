package performance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
)

type replayProtocolRow struct {
	Schema         int                `json:"schema_version"`
	Protocol       string             `json:"protocol"`
	Sequence       uint64             `json:"sequence"`
	Event          string             `json:"event"`
	Identity       NativeToolIdentity `json:"identity"`
	Handle         string             `json:"logical_handle"`
	Session        string             `json:"native_session_id"`
	PID            *int               `json:"native_pid"`
	Emitter        string             `json:"emitter_id"`
	EmitterPID     int                `json:"emitter_pid"`
	Clock          string             `json:"clock_id"`
	Domain         string             `json:"clock_domain"`
	Origin         string             `json:"clock_origin_monotonic_ns"`
	Observed       string             `json:"observed_monotonic_ns"`
	PreviousFailed *bool              `json:"previous_emit_failed"`
	WaitCall       string             `json:"wait_tool_call_id"`
	WaitInvocation string             `json:"wait_invocation_id"`
	Outcome        string             `json:"outcome"`
	Status         string             `json:"status"`
	ExitCode       *int               `json:"exit_code"`
	ExitSignal     *string            `json:"exit_signal"`
	Renderer       *string            `json:"output_renderer"`
	Truncated      *bool              `json:"output_truncated"`
}

type replayProtocolJob struct {
	row   replayProtocolRow
	start AnalysisEvent
	end   *AnalysisEvent
	id    string
}

// This source measures native promise/wait observations, not guest execution.
func (s *AnalysisSnapshot) replayToolProtocolSnapshot(root string, attempt traceAttempt) {
	base := filepath.Join(attempt.OccurrenceID, "harness")
	indexName, rawName := filepath.Join(base, "replay-tool-protocol.index.json"), filepath.Join(base, "replay-tool-protocol.jsonl")
	_, indexPresence := os.Lstat(filepath.Join(root, indexName))
	_, rawPresence := os.Lstat(filepath.Join(root, rawName))
	if os.IsNotExist(indexPresence) && os.IsNotExist(rawPresence) {
		return
	}
	coverage := SourceCoverage{Source: "native_replay_tool_protocol", AttemptID: attempt.AttemptID, Integrity: "incomplete", Status: "incomplete",
		Missing: []string{"producer_terminal_receipt", "producer_total_and_write_failure_counts", "independent_native_invocation_denominator"}}
	defer func() { s.Coverage = append(s.Coverage, coverage) }()
	fail := func(reason string) { s.problem("replay tool protocol " + reason + " for " + attempt.AttemptID) }
	indexSource, indexErr := s.identitySource(root, indexName)
	var index struct {
		Schema    int    `json:"schema_version"`
		Protocol  string `json:"protocol"`
		Integrity string `json:"file_integrity"`
		SHA       string `json:"sha256"`
		Issue     string `json:"issue"`
	}
	if indexErr != nil || identityJSON(indexSource.data, &index, true) != nil || index.Schema != 1 || index.Protocol != "terminal-result-v1" || index.Integrity != "retained" || index.Issue != "" {
		if index.Integrity == "not_collected" {
			coverage.Status = "not_collected"
		}
		fail("index not retained")
		return
	}
	data, err := readIdentityEvidence(root, rawName)
	ref := s.source(rawName, data, "native_replay_tool_monotonic", "unaligned_native_monotonic", err)
	digest := sha256.Sum256(data)
	if err != nil || hex.EncodeToString(digest[:]) != index.SHA {
		fail("raw source differs from index")
		return
	}
	rows, err := identityRows(identityRow{data: data, ref: ref})
	if err != nil || len(rows) == 0 {
		fail("raw source is empty or malformed")
		return
	}
	var group *AnalysisNativeIdentityAttempt
	if s.NativeToolIdentities != nil {
		for i := range s.NativeToolIdentities.Attempts {
			g := &s.NativeToolIdentities.Attempts[i]
			if g.AttemptID == attempt.AttemptID {
				group = g
			}
		}
	}
	if group == nil || group.NativeSessionID == "" || group.AgentSessionID != attempt.AgentSessionID {
		fail("native session binding missing")
		return
	}
	waits, accepted, transcriptRef, err := s.replayProtocolCalls(root, base, group.NativeSessionID)
	if err != nil {
		fail("archived calls invalid")
		return
	}
	tools := map[string]AnalysisInterval{}
	for _, interval := range s.Intervals {
		if interval.AttemptID == attempt.AttemptID && interval.Category == "native_tool" {
			tools[interval.ToolCallID] = interval
		}
	}
	mappings := map[string]AnalysisNativeIdentityMapping{}
	for _, mapping := range group.Mappings {
		mappings[mapping.NativeInvocationID] = mapping
	}
	jobs := map[string]*replayProtocolJob{}
	jobOrder := []string{}
	waitStarts, waitEnds := map[string]AnalysisEvent{}, map[string]bool{}
	waitOrder := []string{}
	waitHandles := map[string]string{}
	waitCallsSeen, acceptedCallsSeen := map[string]bool{}, map[string]bool{}
	points, intervals, edges := []AnalysisEvent{}, []AnalysisInterval{}, []AnalysisEdge{}
	var first replayProtocolRow
	var origin, previous int64
	var domain string
	failedEmission := false
	addInterval := func(id, name, scope, outcome string, start, end AnalysisEvent, attrs map[string]any) {
		intervals = append(intervals, AnalysisInterval{AnalysisIdentity: start.AnalysisIdentity, ID: id, Name: name, Category: "native_replay_tool", ClockDomain: domain,
			StartNS: start.OffsetNS, EndNS: end.OffsetNS, DurationNS: end.OffsetNS - start.OffsetNS, StartEventID: start.ID, EndEventID: end.ID,
			EvidenceRefs: []string{ref, transcriptRef}, Scope: scope, Outcome: outcome, Correlation: "exact_id", Attributes: attrs})
	}
	for i, source := range rows {
		var row replayProtocolRow
		if identityJSON(source.data, &row, true) != nil || row.Schema != 1 || row.Protocol != "terminal-result-v1" || row.Sequence != uint64(i+1) ||
			row.Domain != "native_replay_tool_monotonic" || !validAnalysisIdentity(row.Emitter) || !validAnalysisIdentity(row.Clock) || row.EmitterPID <= 0 || row.PreviousFailed == nil ||
			!validAnalysisIdentity(row.Handle) || !validAnalysisIdentity(row.Session) || row.PID != nil && *row.PID <= 0 {
			fail("event identity/sequence invalid")
			return
		}
		o, e1 := strconv.ParseInt(row.Origin, 10, 64)
		now, e2 := strconv.ParseInt(row.Observed, 10, 64)
		if e1 != nil || e2 != nil || o < 0 || now < o {
			fail("clock invalid")
			return
		}
		if i == 0 {
			first, origin, previous = row, o, o
			domain = "native-replay-tool/" + attempt.AttemptID + "/" + row.Clock
		}
		if row.Emitter != first.Emitter || row.EmitterPID != first.EmitterPID || row.Clock != first.Clock || o != origin || now < previous {
			fail("clock changed or regressed")
			return
		}
		previous, failedEmission = now, failedEmission || *row.PreviousFailed
		mapping, exists := mappings[row.Identity.NativeInvocationID]
		original := mapping.NativeToolIdentity
		original.SSHRequestID = ""
		if !exists || mapping.Status != "exact" || original != row.Identity || row.Identity.ScopeID != attempt.AttemptID || row.Identity.AgentSessionID != attempt.AgentSessionID || accepted[row.Identity.ToolCallID] != row.Handle {
			fail("job has no exact accepted exec binding")
			return
		}
		execTool, exists := tools[row.Identity.ToolCallID]
		if !exists || execTool.Name != "tool.observed.exec" {
			fail("exec consumer interval missing")
			return
		}
		identity := execTool.AnalysisIdentity
		attrs := map[string]any{"logical_handle": row.Handle, "exec_tool_call_id": row.Identity.ToolCallID, "native_invocation_id": row.Identity.NativeInvocationID,
			"native_session_id": row.Session, "native_pid": row.PID, "emitter_id": row.Emitter, "emitter_pid": row.EmitterPID, "clock_id": row.Clock,
			"clock_origin_monotonic_ns": row.Origin, "observed_monotonic_ns": row.Observed, "previous_emit_failed": *row.PreviousFailed, "guest_completion": "unknown"}
		job := jobs[row.Handle]
		if row.Event != "accepted" && (job == nil || job.row.Identity != row.Identity || job.row.Session != row.Session || !sameReplayPID(job.row.PID, row.PID)) {
			fail("job ownership changed or absent")
			return
		}
		if row.Event == "wait_started" || row.Event == "wait_finished" {
			tool, ok := tools[row.WaitCall]
			if !ok || tool.Name != "tool.observed.process" || !validAnalysisIdentity(row.WaitInvocation) || waits[row.WaitCall] != row.Handle {
				fail("wait caller binding invalid")
				return
			}
			identity = tool.AnalysisIdentity
			attrs["wait_tool_call_id"], attrs["wait_invocation_id"] = row.WaitCall, row.WaitInvocation
		} else if row.WaitCall != "" || row.WaitInvocation != "" {
			fail("job event carries a wait identity")
			return
		}
		point := analysisEvent(ref, source.line, "replay_tool."+row.Event, "native_replay_tool", domain, "native_promise_observation", now-origin, identity, attrs)
		point.Sequence = row.Sequence
		coverage.ObservedEvents++
		switch row.Event {
		case "accepted":
			if job != nil {
				fail("duplicate accepted handle")
				return
			}
			jobs[row.Handle] = &replayProtocolJob{row: row, start: point, id: stableAnalysisID("replay-job", attempt.AttemptID, row.Identity.NativeInvocationID)}
			jobOrder = append(jobOrder, row.Handle)
			acceptedCallsSeen[row.Identity.ToolCallID] = true
		case "terminal_result", "terminal_error":
			if job.end != nil || row.Event == "terminal_result" && row.Status != "completed" && row.Status != "failed" {
				fail("duplicate or invalid terminal result")
				return
			}
			attrs["status"], attrs["exit_code"], attrs["exit_signal"], attrs["output_renderer"], attrs["output_truncated"] = row.Status, row.ExitCode, row.ExitSignal, row.Renderer, row.Truncated
			job.end = &point
			outcome := row.Status
			if row.Event == "terminal_error" {
				outcome = "failed"
			}
			addInterval(job.id, "replay.job.accepted_to_terminal", "native_job_registration_to_final_result", outcome, job.start, point, attrs)
			edges = append(edges, AnalysisEdge{From: execTool.ID, To: job.id, Kind: "job_registration", Correlation: "verified_mapping", EvidenceRefs: []string{ref, transcriptRef}})
		case "wait_started":
			if _, exists := waitStarts[row.WaitInvocation]; exists {
				fail("duplicate wait invocation")
				return
			}
			waitStarts[row.WaitInvocation], waitHandles[row.WaitInvocation] = point, row.Handle
			waitOrder = append(waitOrder, row.WaitInvocation)
			waitCallsSeen[row.WaitCall] = true
			coverage.Started++
		case "wait_finished":
			start, ok := waitStarts[row.WaitInvocation]
			if !ok || waitEnds[row.WaitInvocation] || start.ToolCallID != row.WaitCall || waitHandles[row.WaitInvocation] != row.Handle || row.Outcome != "returned" && row.Outcome != "interrupted" || row.Outcome == "returned" && (job.end == nil || job.end.Name != "replay_tool.terminal_result") {
				fail("wait terminal binding invalid")
				return
			}
			waitEnds[row.WaitInvocation] = true
			coverage.Paired++
			attrs["wait_outcome"] = row.Outcome
			id := stableAnalysisID("replay-wait", attempt.AttemptID, row.WaitInvocation)
			addInterval(id, "replay.process.wait", "native_wait_invocation_to_return", row.Outcome, start, point, attrs)
			edges = append(edges, AnalysisEdge{From: execTool.ID, To: tools[row.WaitCall].ID, Kind: "waits_for", Correlation: "verified_mapping", EvidenceRefs: []string{ref, transcriptRef}},
				AnalysisEdge{From: tools[row.WaitCall].ID, To: id, Kind: "native_wait_observation", Correlation: "verified_mapping", EvidenceRefs: []string{ref, transcriptRef}})
		default:
			fail("event unsupported")
			return
		}
		points = append(points, point)
	}
	if !s.clock(AnalysisClock{ID: domain, EmitterID: first.Emitter, Alignment: "unaligned_native_monotonic", EvidenceRef: ref}) {
		return
	}
	for i := range s.Sources {
		if s.Sources[i].Ref == ref {
			s.Sources[i].ClockDomain = domain
		}
	}
	coverage.Integrity, coverage.Status = "complete", "unknown"
	missing := len(waitEnds) != len(waitStarts) || len(waitCallsSeen) != len(waits) || len(acceptedCallsSeen) != len(accepted)
	for _, handle := range jobOrder {
		job := jobs[handle]
		if job.end == nil {
			missing = true
			continue
		}
		for _, invocation := range waitOrder {
			if waitHandles[invocation] == handle && waitEnds[invocation] {
				edges = append(edges, AnalysisEdge{From: job.id, To: stableAnalysisID("replay-wait", attempt.AttemptID, invocation), Kind: "waits_for", Correlation: "verified_mapping", EvidenceRefs: []string{ref}})
			}
		}
	}
	s.Events, s.Intervals, s.Edges = append(s.Events, points...), append(s.Intervals, intervals...), append(s.Edges, edges...)
	if missing || failedEmission {
		coverage.Integrity, coverage.Status = "incomplete", "incomplete"
		coverage.Missing = append(coverage.Missing, "observed_job_wait_or_emission_incomplete")
		fail("observed lifecycle incomplete")
	}
}

func sameReplayPID(a, b *int) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }

// The archived native session supplies the independent accepted/wait call sets.
func (s *AnalysisSnapshot) replayProtocolCalls(root, base, session string) (map[string]string, map[string]string, string, error) {
	source, err := s.identitySource(root, filepath.Join(base, "telemetry", session+".jsonl"))
	if err != nil {
		return nil, nil, source.ref, err
	}
	rows, err := identityRows(source)
	if err != nil {
		return nil, nil, source.ref, err
	}
	waits, accepted := map[string]string{}, map[string]string{}
	bad := errors.New("invalid archived replay protocol calls")
	for _, row := range rows {
		var record struct {
			Message struct {
				Role       string          `json:"role"`
				Content    json.RawMessage `json:"content"`
				ToolCallID string          `json:"toolCallId"`
				ToolName   string          `json:"toolName"`
				Details    struct {
					Status    string `json:"status"`
					Protocol  string `json:"protocol"`
					SessionID string `json:"sessionId"`
				} `json:"details"`
			} `json:"message"`
		}
		if identityJSON(row.data, &record, false) != nil {
			return nil, nil, source.ref, bad
		}
		m := record.Message
		if m.Role == "assistant" {
			var items []struct {
				Type      string `json:"type"`
				Name      string `json:"name"`
				ID        string `json:"id"`
				Arguments struct {
					Action    string `json:"action"`
					SessionID string `json:"sessionId"`
				} `json:"arguments"`
			}
			if identityJSON(m.Content, &items, false) != nil {
				return nil, nil, source.ref, bad
			}
			for _, item := range items {
				if item.Type != "toolCall" || item.Name != "process" || item.Arguments.Action != "wait" {
					continue
				}
				if !validAnalysisIdentity(item.ID) || !validAnalysisIdentity(item.Arguments.SessionID) || waits[item.ID] != "" {
					return nil, nil, source.ref, bad
				}
				waits[item.ID] = item.Arguments.SessionID
			}
		}
		if m.Role == "toolResult" && m.ToolName == "exec" && m.Details.Status == "accepted" {
			if m.Details.Protocol != "terminal-result-v1" || !validAnalysisIdentity(m.ToolCallID) || !validAnalysisIdentity(m.Details.SessionID) || accepted[m.ToolCallID] != "" {
				return nil, nil, source.ref, bad
			}
			accepted[m.ToolCallID] = m.Details.SessionID
		}
	}
	return waits, accepted, source.ref, nil
}

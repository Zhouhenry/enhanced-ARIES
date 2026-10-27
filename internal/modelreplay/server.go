package modelreplay

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/hyscale-lab/aries/pkg/performance"
	"golang.org/x/sys/unix"
)

type Timing struct {
	Mode          string        `json:"mode"` // zero, recorded, synthetic
	InitialDelay  time.Duration `json:"initial_delay_ns,omitempty"`
	FrameInterval time.Duration `json:"frame_interval_ns,omitempty"`
}

type Options struct {
	Mode              string
	ReplayPolicy      string // replay only; omission means strict.
	Identity          Identity
	ToolCallIDDialect string
	HistoryDialect    string
	Model             string
	AccessToken       []byte
	UpstreamURL       string
	UpstreamKey       []byte
	Client            *http.Client
	Tape              *Tape
	OutputDir         string // Must not exist; it is private and owned by this session.
	Timing            Timing
	Timeout           time.Duration
	MaxTurns          int
	Clock             *performance.Clock // Optional same-process Recorder anchor.
	ObservationLevel  performance.ObservationLevel
}

// Server binds one listener/handler to one explicit attempt and session.
// Multiple servers can run in one process without sharing progress or credentials.
type Server struct {
	options                    Options
	upstream                   string
	client                     *http.Client
	mu                         sync.Mutex
	busy, closing, closed      bool
	settled                    chan struct{}
	admissionClosed            bool
	failed                     string
	tape                       Tape
	position                   int
	lastRequest                string
	lastTurn                   *Turn
	lastToolContentDifferences []toolContentDifference
	bytes                      int
	frames                     int
	started                    time.Time
	active                     sync.WaitGroup
	observationMu              sync.Mutex
	observations               *legacyObservationFile
	observationError           error
	legacyIOReportError        error
	boundaries                 *boundaryRecorder
	branch                     string
	recordInParent             bool
}

// The encoder keeps its original writes and error handling. These counters
// cover only events.jsonl calls, not metadata, sockets or physical disk I/O.
type legacyObservationFile struct {
	file interface {
		io.Writer
		Sync() error
		Close() error
	}
	counters performance.FileIOCounters
}

func (f *legacyObservationFile) Write(data []byte) (int, error) {
	return f.counters.Write(f.file, data)
}

func (f *legacyObservationFile) Sync() error  { return f.counters.Sync(f.file) }
func (f *legacyObservationFile) Close() error { return f.file.Close() }

type legacyFileIOReport struct {
	SchemaVersion int                        `json:"schema_version"`
	Identity      Identity                   `json:"identity"`
	EventsFile    string                     `json:"events_file"`
	Status        string                     `json:"status"`
	FileIO        performance.FileIOCounters `json:"file_io"`
}

func (s *Server) legacyFileIOError() error {
	s.observationMu.Lock()
	defer s.observationMu.Unlock()
	return errors.Join(s.observationError, s.legacyIOReportError)
}

type observation struct {
	RequestID    string   `json:"model_request_instance_id,omitempty"`
	Event        string   `json:"event"`
	Identity     Identity `json:"identity"`
	Mode         string   `json:"mode"`
	ReplayPolicy string   `json:"replay_policy,omitempty"`
	Timing       string   `json:"timing_source"`
	OffsetNS     int64    `json:"observed_offset_ns"`
	Turn         int      `json:"turn"`
	Status       string   `json:"status,omitempty"`
	Reason       string   `json:"reason,omitempty"`
	DurationNS   int64    `json:"duration_ns,omitempty"`
	Frame        int      `json:"frame,omitempty"`
	Bytes        int      `json:"bytes,omitempty"`
	TargetNS     *int64   `json:"target_offset_ns,omitempty"`
	ActualNS     *int64   `json:"actual_offset_ns,omitempty"`
	ReceivedNS   *int64   `json:"received_offset_ns,omitempty"`
	HeapBytes    uint64   `json:"process_heap_bytes,omitempty"`
	PeakRSSBytes int64    `json:"process_peak_rss_bytes,omitempty"`
	UserCPUNS    int64    `json:"process_user_cpu_ns,omitempty"`
	SystemCPUNS  int64    `json:"process_system_cpu_ns,omitempty"`
	Goroutines   int      `json:"process_goroutines,omitempty"`
}

func New(o Options) (*Server, error) {
	return newServer(o, false)
}

func newServer(o Options, bundleRoot bool) (*Server, error) {
	if err := validateIdentity(o.Identity); err != nil {
		return nil, err
	}
	if err := ValidateToolCallIDDialect(o.ToolCallIDDialect, o.Identity.HarnessVersion); err != nil {
		return nil, err
	}
	if err := ValidateHistoryDialect(o.HistoryDialect, o.Identity.HarnessVersion); err != nil {
		return nil, err
	}
	if o.Mode == "replay" {
		if o.ReplayPolicy == "" {
			o.ReplayPolicy = ReplayPolicyStrict
		}
		if o.ReplayPolicy != ReplayPolicyStrict && o.ReplayPolicy != ReplayPolicyWorkload {
			return nil, errors.New("replay_policy must be strict or workload")
		}
	} else if o.ReplayPolicy != "" {
		return nil, errors.New("replay_policy requires replay mode")
	}
	if len(o.AccessToken) < 16 {
		return nil, errors.New("access token must contain at least 16 bytes")
	}
	if o.Timeout == 0 {
		o.Timeout = 5 * time.Minute
	}
	if o.Timeout < 0 || o.Timeout > 24*time.Hour {
		return nil, errors.New("invalid request timeout")
	}
	if o.MaxTurns == 0 {
		o.MaxTurns = 128
	}
	if o.MaxTurns < 1 || o.MaxTurns > maxTurns {
		return nil, errors.New("invalid turn limit")
	}
	o.AccessToken = bytes.Clone(o.AccessToken)
	o.UpstreamKey = bytes.Clone(o.UpstreamKey)
	s := &Server{options: o, started: time.Now()}
	switch o.Mode {
	case "record":
		if o.Model == "" || len(o.UpstreamKey) == 0 || o.Tape != nil {
			return nil, errors.New("record requires model/upstream key and no tape")
		}
		u, err := url.Parse(o.UpstreamURL)
		if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
			return nil, errors.New("invalid upstream URL")
		}
		if u.Scheme != "https" && !(u.Scheme == "http" && loopback(u.Hostname())) {
			return nil, errors.New("upstream requires HTTPS (HTTP only on loopback for tests)")
		}
		u.Path = strings.TrimRight(u.Path, "/") + "/chat/completions"
		s.upstream = u.String()
		s.client = &http.Client{}
		if o.Client != nil {
			*s.client = *o.Client
		}
		s.client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		s.tape = Tape{Version: Version, Identity: o.Identity, ToolCallIDDialect: o.ToolCallIDDialect, HistoryDialect: o.HistoryDialect, Model: o.Model, UpstreamHost: u.Host, StartedAt: time.Now().UTC()}
		if o.HistoryDialect == JiuwenSubagentHistoryDialect {
			s.tape.Version = jsonResponseVersion
		}
	case "replay":
		if o.Tape == nil || o.UpstreamURL != "" || len(o.UpstreamKey) != 0 || o.Client != nil {
			return nil, errors.New("replay requires tape and forbids upstream configuration")
		}
		if err := validateTape(*o.Tape); err != nil {
			return nil, err
		}
		if o.Tape.Version == bundleVersion && !bundleRoot {
			return nil, errors.New("branch tape requires the complete bundle session")
		}
		if o.ToolCallIDDialect != o.Tape.ToolCallIDDialect {
			return nil, errors.New("replay tool-call ID dialect must match tape")
		}
		if o.HistoryDialect != o.Tape.HistoryDialect {
			return nil, errors.New("replay history dialect must match tape")
		}
		// Detach all slices from the caller so the replay source remains immutable.
		data, _ := json.Marshal(o.Tape)
		_ = json.Unmarshal(data, &s.tape)
		if o.Identity.Placement != s.tape.Identity.Placement || o.Identity.HarnessVersion != s.tape.Identity.HarnessVersion {
			return nil, errors.New("replay placement/harness version must match tape")
		}
		if o.Model != "" && o.Model != s.tape.Model {
			return nil, errors.New("replay model must match tape")
		}
		s.options.Model = s.tape.Model
		switch o.Timing.Mode {
		case "zero", "recorded", "synthetic":
		default:
			return nil, errors.New("timing must be zero, recorded or synthetic")
		}
		if o.Timing.InitialDelay < 0 || o.Timing.FrameInterval < 0 || o.Timing.InitialDelay > time.Hour || o.Timing.FrameInterval > time.Minute {
			return nil, errors.New("invalid synthetic timing")
		}
		if o.Timing.Mode != "synthetic" && (o.Timing.InitialDelay != 0 || o.Timing.FrameInterval != 0) {
			return nil, errors.New("synthetic delays require synthetic mode")
		}
	default:
		return nil, errors.New("mode must be record or replay")
	}
	metadata, _ := json.Marshal(struct {
		Identity     Identity
		Model        string
		UpstreamHost string
	}{o.Identity, s.options.Model, s.tape.UpstreamHost})
	if s.containsSecret(metadata) {
		return nil, errors.New("known credential appears in session metadata")
	}
	if o.OutputDir == "" {
		return nil, errors.New("private output directory is required")
	}
	if err := os.Mkdir(o.OutputDir, 0700); err != nil {
		return nil, fmt.Errorf("create fresh replay output directory: %w", err)
	}
	if o.ObservationLevel == "" {
		o.ObservationLevel = performance.ObservationL1
	}
	s.options.ObservationLevel = o.ObservationLevel
	if performance.ObservesControl(o.ObservationLevel) {
		f, err := os.OpenFile(filepath.Join(o.OutputDir, "events.jsonl"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return nil, err
		}
		s.observations = &legacyObservationFile{file: f,
			counters: performance.FileIOCounters{Scope: "model_legacy_events_jsonl_only"}}
		s.boundaries, err = newBoundaryRecorder(o, s.started)
		if err != nil {
			_ = f.Close()
			return nil, err
		}
	}
	s.observe(observation{Event: "session_started"})
	return s, nil
}

func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) Model() string { return s.options.Model }

func (s *Server) ReplayPolicy() string { return s.options.ReplayPolicy }

func (s *Server) observe(e observation) {
	if s.observations == nil {
		return
	}
	e.Identity = s.options.Identity
	e.Mode = s.options.Mode
	e.ReplayPolicy = s.options.ReplayPolicy
	if e.Timing == "" {
		e.Timing = s.options.Timing.Mode
		if e.Mode == "record" {
			e.Timing = "observed_upstream"
		}
	}
	e.OffsetNS = time.Since(s.started).Nanoseconds()
	if e.Event != "frame_sent" && !strings.HasPrefix(e.Event, "first_") {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		e.HeapBytes = mem.Alloc
		e.Goroutines = runtime.NumGoroutine()
		var usage unix.Rusage
		if unix.Getrusage(unix.RUSAGE_SELF, &usage) == nil {
			e.UserCPUNS = usage.Utime.Nano()
			e.SystemCPUNS = usage.Stime.Nano()
			e.PeakRSSBytes = usage.Maxrss
			if runtime.GOOS != "darwin" {
				e.PeakRSSBytes *= 1024
			}
		}
	}
	s.observationMu.Lock()
	defer s.observationMu.Unlock()
	if s.observationError == nil {
		s.observationError = json.NewEncoder(s.observations).Encode(e)
	}
}

// These are first complete SSE/delta observations, not model tokens. The caller
// supplies upstream receipt or downstream flush offsets from request admission.
func (s *Server) observeFirstOutputs(seen *[4]bool, parsed *streamResult, e observation) {
	for i, output := range []struct {
		event   string
		present bool
	}{
		{"first_sse", true},
		{"first_content", len(parsed.text) > 0},
		{"first_reasoning", len(parsed.reasoning) > 0},
		{"first_tool_call_delta", parsed.hasToolDelta()},
	} {
		if output.present && !seen[i] {
			seen[i] = true
			e.Event = output.event
			s.observe(e)
		}
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	received := time.Now()
	if entry, ok := r.Context().Value(requestReceivedKey{}).(time.Time); ok {
		received = entry
	}
	route := "unsupported"
	if r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions" && r.URL.RawQuery == "" {
		route = "chat_completions"
	} else if r.Method == http.MethodGet && r.URL.Path == "/v1/models" && r.URL.RawQuery == "" {
		route = "models"
	}
	// Register every handler, including rejected requests and /models, before
	// Close can stop admission and drain its observation writer.
	s.mu.Lock()
	registered := !s.closing
	if registered {
		s.active.Add(1)
	}
	s.mu.Unlock()
	observed := s.beginRequest(received, route)
	defer func() {
		observed.finish()
		if registered {
			s.active.Done()
		}
	}()
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		failure(w, 409, "SESSION_CLOSED", "session closed")
		observed.result("rejected", "SESSION_CLOSED", http.StatusConflict)
		return
	}
	authorized := subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), append([]byte("Bearer "), s.options.AccessToken...)) == 1
	s.mu.Unlock()
	if !authorized {
		failure(w, http.StatusUnauthorized, "UNAUTHORIZED", "access token required")
		observed.result("rejected", "UNAUTHORIZED", http.StatusUnauthorized)
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/v1/models" && r.URL.RawQuery == "" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": []any{map[string]string{"id": s.Model(), "object": "model", "owned_by": "aries-model-replay"}}})
		observed.result("succeeded", "", http.StatusOK)
		return
	}
	if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" || r.URL.RawQuery != "" {
		failure(w, 404, "UNSUPPORTED", "only streaming Chat Completions is supported")
		observed.result("rejected", "UNSUPPORTED", http.StatusNotFound)
		return
	}
	if r.Header.Get("Content-Encoding") != "" && r.Header.Get("Content-Encoding") != "identity" {
		failure(w, 400, "UNSUPPORTED", "encoded request body")
		observed.result("rejected", "UNSUPPORTED", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	if s.closing || s.admissionClosed || s.failed != "" || s.busy {
		reason := "SESSION_BUSY"
		if s.closing {
			reason = "SESSION_CLOSED"
		} else if s.admissionClosed {
			reason = "ADMISSION_CLOSED"
		} else if s.failed != "" {
			reason = "SESSION_FAILED"
		}
		s.mu.Unlock()
		failure(w, 409, reason, "session cannot admit this request")
		observed.result("rejected", reason, http.StatusConflict)
		observed.legacy(observation{Event: "request_rejected", Reason: reason})
		return
	}
	started := time.Now()
	s.busy = true
	s.settled = make(chan struct{})
	position := s.position
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.busy = false; close(s.settled); s.mu.Unlock() }()
	observed.turn = &position
	observed.emit(boundaryEvent{Event: "request_admitted", Scope: "request_admission"}, started)
	ctx, cancel := context.WithTimeout(r.Context(), s.options.Timeout)
	defer cancel()
	r = r.WithContext(ctx)
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(started.Add(s.options.Timeout))
	_ = controller.SetWriteDeadline(started.Add(s.options.Timeout))
	defer controller.SetReadDeadline(time.Time{})
	defer controller.SetWriteDeadline(time.Time{})
	observed.emit(boundaryEvent{Event: "request_body_read_started", Scope: "request_body_read"}, time.Now())
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	bodyReadAt := time.Now()
	bodyStatus := "succeeded"
	if err != nil {
		bodyStatus = "failed"
	}
	observed.emit(boundaryEvent{Event: "request_body_read", Scope: "request_body_read", Status: bodyStatus, Bytes: len(raw)}, bodyReadAt)
	if err != nil {
		s.reject(w, observed, position, "INVALID_REQUEST", "request body exceeds bound or cannot be read")
		return
	}
	if s.containsSecret(raw) {
		s.reject(w, observed, position, "CREDENTIAL_IN_PAYLOAD", "known credential appears in payload")
		return
	}
	var req map[string]any
	observed.emit(boundaryEvent{Event: "request_parse_started", Scope: "request_decode_and_protocol_validation"}, time.Now())
	if s.options.Mode == "replay" {
		// Compare the complete recorded request before applying record-only
		// protocol validation. A changed model or stream parameter is a
		// divergence, just like a changed tool schema or older tool result.
		var decoded any
		decoded, err = decode(raw)
		if err == nil {
			var ok bool
			req, ok = decoded.(map[string]any)
			if !ok {
				err = errors.New("request must be an object")
			}
		}
	} else {
		req, err = parseRequestProtocol(raw, s.Model(), s.options.HistoryDialect == JiuwenSubagentHistoryDialect)
	}
	parseStatus := "succeeded"
	if err != nil {
		parseStatus = "failed"
	}
	observed.emit(boundaryEvent{Event: "request_parsed", Scope: "request_decode_and_protocol_validation", Status: parseStatus}, time.Now())
	if err != nil {
		s.reject(w, observed, position, "UNSUPPORTED", "unsupported or invalid model request")
		return
	}
	canonical, _ := json.Marshal(req)
	if s.containsSecret(canonical) {
		s.reject(w, observed, position, "CREDENTIAL_IN_PAYLOAD", "known credential appears in payload")
		return
	}
	observed.emit(boundaryEvent{Event: "request_match_started", Scope: "request_history_and_trajectory_validation"}, time.Now())
	key := digest(req)
	var turn *Turn
	var toolContentDifferences []toolContentDifference
	retry := s.lastTurn != nil && key == s.lastRequest
	observed.retry = retry
	if retry {
		// Only a completed identical immediately preceding request is retryable.
		// Re-delivery never advances progress or repeats a paid upstream call.
		turn = s.lastTurn
		toolContentDifferences = s.lastToolContentDifferences
		if s.options.Mode == "record" {
			timing := s.options.Timing.Mode
			if timing != "recorded" && timing != "synthetic" {
				timing = "zero"
			}
			observed.timing = "cached_retry_" + timing
		}
	} else if s.options.Mode == "replay" {
		if position >= len(s.tape.Turns) {
			observed.replayDiverged(req)
			s.reject(w, observed, position, "REPLAY_DIVERGED", "unexpected request after tape end")
			return
		}
		expected, _ := decode(s.tape.Turns[position].Request)
		path, allowed := replayRequestDifference(expected, req, s.options.ReplayPolicy)
		if path != "" {
			observed.replayDiverged(req)
			s.reject(w, observed, position, "REPLAY_DIVERGED", "request differs at "+path)
			return
		}
		toolContentDifferences = allowed
		turn = &s.tape.Turns[position]
	} else {
		if position >= s.options.MaxTurns {
			observed.emit(boundaryEvent{Event: "request_matched", Scope: "request_history_and_trajectory_validation", Status: "failed", Reason: "TURN_LIMIT"}, time.Now())
			s.reject(w, observed, position, "TURN_LIMIT", "session turn bound reached")
			return
		}
		var previous *Turn
		if position > 0 {
			previous = &s.tape.Turns[position-1]
		}
		if err := nextRequest(req, previous, s.options.ToolCallIDDialect, s.options.HistoryDialect); err != nil {
			observed.emit(boundaryEvent{Event: "request_matched", Scope: "request_history_and_trajectory_validation", Status: "failed", Reason: "UNSUPPORTED"}, time.Now())
			s.reject(w, observed, position, "UNSUPPORTED", "history does not match the complete tool-call trajectory")
			return
		}
	}
	if !retry || s.options.Mode == "replay" {
		observed.timing = s.options.Timing.Mode
		if s.options.Mode == "record" {
			observed.timing = "observed_upstream"
		}
	}
	observed.emit(boundaryEvent{Event: "request_matched", Scope: "request_history_and_trajectory_validation", Status: "succeeded",
		ToolContentDifferences: toolContentDifferences}, time.Now())
	if retry {
		observed.legacy(observation{Event: "request_retry", Turn: position})
	}
	observed.legacy(observation{Event: "request_started", Turn: position, Bytes: len(raw)})
	var result Turn
	if turn != nil {
		result = *turn
		err = s.replay(w, r, result, position, started, observed)
	} else {
		result, err = s.record(w, r, raw, position, started, observed, req["stream"] == true)
	}
	if err != nil {
		reason := "DELIVERY_OR_UPSTREAM_FAILED"
		if ctx.Err() != nil {
			reason = "CANCELED_OR_TIMEOUT"
		}
		s.mu.Lock()
		s.failed = reason
		s.mu.Unlock()
		if s.options.Mode == "record" && len(result.Request) != 0 {
			result.SHA256 = turnDigest(result)
			s.tape.Turns = append(s.tape.Turns, result)
		}
		// Keep session/Tape failure semantics; diagnose the actual record boundary
		// with fixed categories, never an error string that may contain payload.
		if s.options.Mode == "record" && ctx.Err() == nil {
			reason = recordFailureReason(err)
		}
		observed.result("failed", reason, observed.httpStatus)
		observed.legacy(observation{Event: "request_finished", Turn: position, Status: "failed", Reason: reason, DurationNS: time.Since(started).Nanoseconds()})
		return
	}
	if !retry {
		s.mu.Lock()
		if s.options.Mode == "record" {
			result.SHA256 = turnDigest(result)
			s.tape.Turns = append(s.tape.Turns, result)
		}
		s.position++
		s.lastRequest = key
		clone := result
		s.lastTurn = &clone
		s.lastToolContentDifferences = toolContentDifferences
		s.mu.Unlock()
	}
	observed.result("succeeded", "", http.StatusOK)
	observed.legacy(observation{Event: "request_finished", Turn: position, Status: "succeeded", DurationNS: time.Since(started).Nanoseconds()})
}

func (s *Server) reject(w http.ResponseWriter, observed *requestObservation, turn int, code, message string) {
	s.mu.Lock()
	s.failed = code
	s.mu.Unlock()
	failure(w, http.StatusConflict, code, message)
	observed.result("rejected", code, http.StatusConflict)
	observed.legacy(observation{Event: "request_rejected", Turn: turn, Status: "failed", Reason: code})
}

func failure(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"type": code, "code": code, "message": message}})
}

func (s *Server) containsSecret(data []byte) bool {
	for _, secret := range [][]byte{s.options.AccessToken, s.options.UpstreamKey} {
		if len(secret) > 0 && bytes.Contains(data, secret) {
			return true
		}
		if len(secret) > 0 {
			escaped, _ := json.Marshal(string(secret))
			if bytes.Contains(data, escaped[1:len(escaped)-1]) {
				return true
			}
		}
	}
	return false
}

func (s *Server) record(w http.ResponseWriter, r *http.Request, raw []byte, position int, started time.Time, observed *requestObservation, streaming bool) (Turn, error) {
	t := Turn{Request: bytes.Clone(raw)}
	up, err := http.NewRequestWithContext(r.Context(), http.MethodPost, s.upstream, bytes.NewReader(raw))
	if err != nil {
		return t, err
	}
	up.Header.Set("Content-Type", "application/json")
	up.Header.Set("Accept", "text/event-stream")
	if !streaming {
		up.Header.Set("Accept", "application/json")
	}
	up.Header.Set("Accept-Encoding", "identity")
	up.Header.Set("Authorization", "Bearer "+string(s.options.UpstreamKey))
	observed.emit(boundaryEvent{Event: "upstream_request_started", Scope: "upstream_http_wait_for_headers"}, time.Now())
	response, err := s.client.Do(up)
	if err != nil {
		failure(w, 502, "UPSTREAM_FAILED", "upstream transport failed")
		observed.httpStatus = http.StatusBadGateway
		return t, errors.New("upstream transport failed")
	}
	observed.emit(boundaryEvent{Event: "upstream_response_headers", Scope: "upstream_http_wait_for_headers", HTTPStatus: response.StatusCode}, time.Now())
	defer response.Body.Close()
	t.StatusCode = response.StatusCode
	if !streaming {
		return s.recordJSON(w, r, t, response, started, observed)
	}
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		failure(w, 502, "UPSTREAM_FAILED", "upstream did not return a successful SSE response")
		observed.httpStatus = http.StatusBadGateway
		return t, errors.New("upstream status/protocol unsupported")
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	var parsed streamResult
	var firstOutputs [4]bool
	var classified [3]bool
	var firstFlushed [4]bool
	var content []byte
	unsafe := false
	deferredFrame := -1
	flush := func(frameIndex int) error {
		frame := t.Frames[frameIndex]
		offset := frame.OffsetNS
		if err := send(w, frame.Data); err != nil {
			observed.emit(boundaryEvent{Event: "frame_flush_failed", Scope: "downstream_frame_write_and_flush", Frame: &frameIndex, Status: "failed", Reason: "WRITE_OR_FLUSH_FAILED"}, time.Now())
			return errors.New("record frame write or flush failed")
		}
		flushedAt := time.Now()
		actual := flushedAt.Sub(started).Nanoseconds()
		observed.emit(boundaryEvent{Event: "frame_flushed", Scope: "downstream_frame_flush_complete", Frame: &frameIndex, Bytes: len(frame.Data), ReceivedNS: &offset, ActualNS: &actual}, flushedAt)
		observed.firstFlushed(&firstFlushed, &parsed, boundaryEvent{Scope: "downstream_frame_flush_complete", Frame: &frameIndex, ReceivedNS: &offset, ActualNS: &actual}, flushedAt)
		observed.legacy(observation{Event: "frame_sent", Turn: position, Frame: frameIndex, Bytes: len(frame.Data), ReceivedNS: &offset, ActualNS: &actual})
		return nil
	}
	err = readFrames(response.Body, func(data []byte) error {
		receivedAt := time.Now()
		offset := receivedAt.Sub(started).Nanoseconds()
		frameIndex := len(t.Frames)
		observed.emit(boundaryEvent{Event: "upstream_frame_received", Scope: "upstream_complete_sse_receipt", Frame: &frameIndex, Bytes: len(data), ReceivedNS: &offset}, receivedAt)
		if frameIndex == 0 {
			observed.emit(boundaryEvent{Event: "upstream_first_sse", Scope: "upstream_complete_sse_receipt", Frame: &frameIndex, ReceivedNS: &offset}, receivedAt)
		}
		content = append(content, data...)
		if s.containsSecret(content) {
			unsafe = true
			return errors.New("credential in upstream response")
		}
		if s.bytes+len(raw)+len(content) > maxTape || s.frames+len(t.Frames) >= maxFrames {
			return errors.New("tape byte bound exceeded")
		}
		if err := parsed.feed(data); err != nil {
			return err
		}
		if parsed.done {
			if err := parsed.complete(); err != nil {
				return err
			}
		}
		if parsed.containsSecret(s.containsSecret) {
			unsafe = true
			return errors.New("credential in decoded upstream response")
		}
		t.Frames = append(t.Frames, Frame{OffsetNS: offset, Data: bytes.Clone(data)})
		for i, first := range []struct {
			event   string
			present bool
		}{
			{"upstream_first_content", len(parsed.text) > 0},
			{"upstream_first_reasoning", len(parsed.reasoning) > 0},
			{"upstream_first_tool_call_delta", parsed.hasToolDelta()},
		} {
			if first.present && !classified[i] {
				classified[i] = true
				observed.emit(boundaryEvent{Event: first.event, Scope: "upstream_complete_sse_receipt", Frame: &frameIndex, ReceivedNS: &offset}, receivedAt)
			}
		}
		s.observeFirstOutputs(&firstOutputs, &parsed, observation{RequestID: observed.id, Turn: position, Frame: len(t.Frames) - 1, ReceivedNS: &offset})
		// Verify the upstream tail before DONE lets a native client close the request.
		if parsed.done {
			if deferredFrame < 0 {
				deferredFrame = frameIndex
			}
		} else if err := flush(frameIndex); err != nil {
			return err
		}
		return r.Context().Err()
	})
	if err == nil {
		err = parsed.complete()
	}
	if err != nil {
		if unsafe {
			return Turn{}, err
		}
		return t, err
	}
	if err := r.Context().Err(); err != nil {
		return t, err
	}
	for i := deferredFrame; i < len(t.Frames); i++ {
		if err := flush(i); err != nil {
			return t, err
		}
	}
	t.Finish = parsed.finish
	t.Calls = parsed.calls()
	s.bytes += len(raw) + len(content)
	s.frames += len(t.Frames)
	return t, nil
}

func recordFailureReason(err error) string {
	// Matching exact locally authored messages is an allowlist. Unknown messages
	// (including JSON decoder errors) must never be copied into observation files.
	switch err.Error() {
	case "UNSUPPORTED: finish reason":
		return "SSE_UNSUPPORTED_FINISH_REASON"
	case "incomplete SSE event or response read failure":
		return "SSE_READ_INCOMPLETE"
	case "response/frame exceeds bound":
		return "SSE_FRAME_OR_RESPONSE_BOUND"
	case "tape byte bound exceeded":
		return "TAPE_BOUND"
	case "record frame write or flush failed":
		return "DOWNSTREAM_WRITE_OR_FLUSH_FAILED"
	case "upstream transport failed":
		return "UPSTREAM_TRANSPORT_FAILED"
	case "upstream status/protocol unsupported":
		return "UPSTREAM_STATUS_OR_PROTOCOL"
	case "upstream stream error":
		return "UPSTREAM_STREAM_ERROR"
	case "incomplete SSE response":
		return "SSE_RESPONSE_INCOMPLETE"
	case "finish reason/tool mismatch":
		return "SSE_FINISH_TOOL_MISMATCH"
	case "credential in upstream response", "credential in decoded upstream response":
		return "CREDENTIAL_IN_UPSTREAM_RESPONSE"
	default:
		return "RECORD_FAILURE_UNCLASSIFIED"
	}
}

func (s *Server) replay(w http.ResponseWriter, r *http.Request, t Turn, position int, started time.Time, observed *requestObservation) error {
	if t.ResponseJSON != nil {
		return s.replayJSON(w, r, t, started, observed)
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	var parsed streamResult
	var firstOutputs [4]bool
	var firstFlushed [4]bool
	timing := "zero"
	if s.options.Timing.Mode == "recorded" || s.options.Timing.Mode == "synthetic" {
		timing = s.options.Timing.Mode
	}
	timingSource := timing
	if s.options.Mode == "record" {
		timingSource = "cached_retry_" + timing
	}
	for i, frame := range t.Frames {
		if err := parsed.feed(frame.Data); err != nil {
			return err
		}
		var target time.Duration
		switch s.options.Timing.Mode {
		case "recorded":
			target = time.Duration(frame.OffsetNS)
		case "synthetic":
			target = s.options.Timing.InitialDelay + time.Duration(i)*s.options.Timing.FrameInterval
		}
		if err := wait(r.Context(), time.Until(started.Add(target))); err != nil {
			return err
		}
		if err := send(w, frame.Data); err != nil {
			observed.emit(boundaryEvent{Event: "frame_flush_failed", Scope: "downstream_frame_write_and_flush", Frame: &i, Status: "failed", Reason: "WRITE_OR_FLUSH_FAILED"}, time.Now())
			return err
		}
		flushedAt := time.Now()
		actual := flushedAt.Sub(started).Nanoseconds()
		planned := int64(target)
		observed.emit(boundaryEvent{Event: "frame_flushed", Scope: "downstream_frame_flush_complete", Frame: &i, Bytes: len(frame.Data), TargetNS: &planned, ActualNS: &actual}, flushedAt)
		observed.firstFlushed(&firstFlushed, &parsed, boundaryEvent{Scope: "downstream_frame_flush_complete", Frame: &i, TargetNS: &planned, ActualNS: &actual}, flushedAt)
		s.observeFirstOutputs(&firstOutputs, &parsed, observation{RequestID: observed.id, Turn: position, Frame: i, Timing: timingSource, TargetNS: &planned, ActualNS: &actual})
		observed.legacy(observation{Event: "frame_sent", Turn: position, Frame: i, Timing: timingSource, Bytes: len(frame.Data), TargetNS: &planned, ActualNS: &actual})
	}
	// All recorded frames were successfully flushed; a client may now close.
	return nil
}

func send(w http.ResponseWriter, data []byte) error {
	n, err := w.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	return http.NewResponseController(w).Flush()
}

func wait(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// CloseAdmission follows Harness.Run returning. Already admitted requests keep
// their original completion/error semantics; HTTP observation stays open for Stop.
func (s *Server) CloseAdmission() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.admissionClosed || s.closing {
		return
	}
	at := time.Now()
	s.admissionClosed = true
	if s.boundaries != nil {
		s.boundaries.mu.Lock()
		offset := s.boundaries.offset(at)
		if s.boundaries.index.AdmissionClosedOffsetNS == nil {
			s.boundaries.index.AdmissionClosedOffsetNS = &offset
		}
		s.boundaries.mu.Unlock()
	}
}

// Close must follow HTTP shutdown. It never silently produces a complete tape
// for an interrupted trajectory, nor overwrites an existing artifact.
// Event-file failures are retained separately by ObservationError.
func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closing = true
	s.mu.Unlock()
	s.active.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	complete := s.failed == "" && s.lastTurn != nil && s.lastTurn.Finish == "stop"
	if s.options.Mode == "replay" {
		complete = complete && s.position == len(s.tape.Turns)
	}
	s.observe(observation{Event: "session_finished", Turn: s.position, Status: map[bool]string{true: "succeeded", false: "incomplete"}[complete], Reason: s.failed, DurationNS: time.Since(s.started).Nanoseconds()})
	var outputErr error
	if s.options.Mode == "record" {
		s.tape.Complete = complete
		s.tape.Failure = s.failed
		s.tape.SHA256 = tapeDigest(s.tape)
		if !s.recordInParent {
			data, err := json.MarshalIndent(s.tape, "", "  ")
			if err == nil {
				err = writePrivate(filepath.Join(s.options.OutputDir, "tape.json"), data)
			}
			outputErr = err
		}
	}
	if s.observations != nil {
		s.observationMu.Lock()
		s.observationError = errors.Join(s.observationError, s.observations.Sync(), s.observations.Close())
		// The encoder can accept a nil-error short/invalid Write result. Preserve
		// its calls, but never report that lost evidence as a healthy observer.
		if s.observations.counters.ShortWrites != 0 {
			s.observationError = errors.Join(s.observationError, io.ErrShortWrite)
		}
		if s.observations.counters.InvalidReturns != 0 {
			s.observationError = errors.Join(s.observationError, errors.New("legacy model events writer returned invalid byte count"))
		}
		report := legacyFileIOReport{SchemaVersion: 1, Identity: s.options.Identity,
			EventsFile: "events.jsonl", Status: "complete", FileIO: s.observations.counters}
		if s.observationError != nil {
			report.Status = "incomplete"
		}
		data, reportErr := json.MarshalIndent(report, "", "  ")
		if reportErr == nil {
			reportErr = writePrivateAtomic(filepath.Join(s.options.OutputDir, "events-file-io.json"), data)
		}
		// Both event and sidecar failures belong to the observer, independently
		// of the model protocol and the required Tape artifact.
		s.legacyIOReportError = reportErr
		s.observationMu.Unlock()
	}
	if s.boundaries != nil && !s.recordInParent {
		// Boundary persistence is an independent evidence channel. A failed
		// writer must be reported through ObservationError without changing a
		// successfully replayed model session's protocol outcome.
		_ = s.boundaries.close()
	}
	clear(s.options.AccessToken)
	clear(s.options.UpstreamKey)
	if !complete {
		outputErr = errors.Join(outputErr, errors.New("model session incomplete; see private events"))
	}
	return outputErr
}

func writePrivate(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(data)
	return errors.Join(writeErr, f.Sync(), f.Close())
}

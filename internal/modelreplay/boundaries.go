package modelreplay

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/hyscale-lab/aries/pkg/performance"
)

const boundarySchemaVersion = 1
const boundaryBufferSize = 16384

// These events add measured HTTP boundaries without changing the legacy model
// event vocabulary, request durations, or recorded SSE timing.
type boundaryEvent struct {
	SchemaVersion          int                     `json:"schema_version"`
	Sequence               uint64                  `json:"sequence"`
	Identity               Identity                `json:"identity"`
	RequestID              string                  `json:"model_request_instance_id"`
	Event                  string                  `json:"event"`
	EmitterID              string                  `json:"emitter_id"`
	ClockDomain            string                  `json:"clock_domain"`
	OffsetNS               int64                   `json:"monotonic_offset_ns"`
	Scope                  string                  `json:"measurement_scope"`
	Producer               string                  `json:"producer_location"`
	Mode                   string                  `json:"mode"`
	ReplayPolicy           string                  `json:"replay_policy,omitempty"`
	Timing                 string                  `json:"timing_source"`
	Route                  string                  `json:"route"`
	Turn                   *int                    `json:"turn,omitempty"`
	Frame                  *int                    `json:"frame_index,omitempty"`
	Status                 string                  `json:"status,omitempty"`
	Reason                 string                  `json:"reason,omitempty"`
	HTTPStatus             int                     `json:"http_status,omitempty"`
	DurationNS             *int64                  `json:"duration_ns,omitempty"`
	Bytes                  int                     `json:"bytes,omitempty"`
	CachedRetry            bool                    `json:"cached_retry,omitempty"`
	TargetNS               *int64                  `json:"target_offset_ns,omitempty"`
	ActualNS               *int64                  `json:"actual_offset_ns,omitempty"`
	ReceivedNS             *int64                  `json:"received_offset_ns,omitempty"`
	RequestDiagnostic      *requestDiagnostic      `json:"request_diagnostic,omitempty"`
	ToolContentDifferences []toolContentDifference `json:"tool_content_differences,omitempty"`
}

type boundaryClock struct {
	EmitterID    string    `json:"emitter_id"`
	ClockDomain  string    `json:"clock_domain"`
	StartedAtUTC time.Time `json:"started_at_utc"`
}

type boundaryIndex struct {
	SchemaVersion           int           `json:"schema_version"`
	Identity                Identity      `json:"identity"`
	ReplayPolicy            string        `json:"replay_policy,omitempty"`
	EmitterID               string        `json:"emitter_id"`
	EventsFile              string        `json:"events_file"`
	Clock                   boundaryClock `json:"clock"`
	SessionStartedOffsetNS  int64         `json:"session_started_offset_ns"`
	AdmissionClosedOffsetNS *int64        `json:"admission_closed_offset_ns,omitempty"`
	Status                  string        `json:"status"`
	Attempted               uint64        `json:"attempted"`
	Written                 uint64        `json:"written"`
	Dropped                 uint64        `json:"dropped"`
	WriteFailed             uint64        `json:"write_failed"`
	RequestsReceived        uint64        `json:"requests_received"`
	RequestsFinished        uint64        `json:"requests_finished"`
	EventsSHA256            string        `json:"events_sha256,omitempty"`
	EventsBytes             int64         `json:"events_bytes,omitempty"`
	FinalizedAtUTC          time.Time     `json:"finalized_at_utc,omitempty"`
	FlushStatus             string        `json:"flush_status,omitempty"`
	// Lifecycle fields keep producer, queue, writer and durability boundaries
	// independently auditable. A complete status requires all of them.
	ProducerState   string                      `json:"producer_state,omitempty"`
	QueueState      string                      `json:"queue_state,omitempty"`
	WriterState     string                      `json:"writer_state,omitempty"`
	DurabilityState string                      `json:"durability_state,omitempty"`
	FileIO          *performance.FileIOCounters `json:"file_io,omitempty"`
}

type boundaryRecorder struct {
	mu              sync.Mutex
	file            boundaryFile
	queue           chan boundaryEvent
	done            chan struct{}
	index           boundaryIndex
	clock           *performance.Clock
	anchor          time.Time
	directory       string
	closed          bool
	late            uint64
	written, failed uint64 // Writer goroutine; inspected only after done closes.
	writeErr        error
	closeErr        error
	lateErr         error
	closeOnce       sync.Once
	fileIO          performance.FileIOCounters
}

// Regular-file I/O is not context-cancellable; this interface enables bounded,
// releasable fault injection without pretending that Go can cancel fsync.
type boundaryFile interface {
	io.WriteCloser
	Sync() error
}

func newBoundaryRecorder(o Options, started time.Time) (*boundaryRecorder, error) {
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return nil, err
	}
	emitter := "model-" + hex.EncodeToString(entropy[:])
	r := &boundaryRecorder{queue: make(chan boundaryEvent, boundaryBufferSize), done: make(chan struct{}),
		anchor: started, directory: o.OutputDir,
		fileIO: performance.FileIOCounters{Scope: "model_request_boundary_jsonl_only"},
		index: boundaryIndex{SchemaVersion: boundarySchemaVersion, Identity: o.Identity, ReplayPolicy: o.ReplayPolicy, EmitterID: emitter,
			EventsFile: "request-boundaries.jsonl", Clock: boundaryClock{EmitterID: emitter, ClockDomain: emitter, StartedAtUTC: started.UTC()}}}
	if o.Clock != nil {
		clock := *o.Clock
		r.clock = &clock
		r.index.Clock = boundaryClock{EmitterID: clock.EmitterID, ClockDomain: clock.ClockDomain, StartedAtUTC: clock.StartedAtUTC}
		r.index.SessionStartedOffsetNS = clock.Offset(started)
	}
	file, err := os.OpenFile(filepath.Join(o.OutputDir, r.index.EventsFile), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}
	r.file = file
	go r.write()
	return r, nil
}

func (r *boundaryRecorder) offset(at time.Time) int64 {
	if r.clock != nil {
		return r.clock.Offset(at)
	}
	return at.Sub(r.anchor).Nanoseconds()
}

func (r *boundaryRecorder) write() {
	defer close(r.done)
	for event := range r.queue {
		if r.writeErr == nil {
			var data []byte
			data, r.writeErr = json.Marshal(event)
			if r.writeErr == nil {
				data = append(data, '\n')
				var n int
				n, r.writeErr = r.fileIO.Write(r.file, data)
				if r.writeErr == nil && n != len(data) {
					r.writeErr = io.ErrShortWrite
				}
			}
		}
		if r.writeErr != nil {
			r.failed++
		} else {
			r.written++
		}
	}
}

func (r *boundaryRecorder) record(event boundaryEvent, at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		r.late++
		if r.late == 1 {
			// Invalidate separately: a finalized index cannot predict a late
			// producer, and must not be rewritten after consumers read it.
			data, err := json.Marshal(struct {
				SchemaVersion int      `json:"schema_version"`
				Identity      Identity `json:"identity"`
				EmitterID     string   `json:"emitter_id"`
				Status        string   `json:"status"`
			}{boundarySchemaVersion, r.index.Identity, r.index.EmitterID, "incomplete"})
			if err == nil {
				err = writePrivate(filepath.Join(r.directory, "request-boundaries-late-after-close.json"), data)
			}
			r.lateErr = err
		}
		return
	}
	r.index.Attempted++
	if event.Event == "http_request_finished" {
		r.index.RequestsFinished++
	}
	event.SchemaVersion, event.Sequence = boundarySchemaVersion, r.index.Attempted
	event.Identity, event.EmitterID = r.index.Identity, r.index.EmitterID
	event.ClockDomain, event.OffsetNS = r.index.Clock.ClockDomain, r.offset(at)
	event.Producer = "model_proxy"
	select {
	case r.queue <- event:
	default:
		r.index.Dropped++
	}
}

// close follows HTTP shutdown, as Server.Close already requires. The bounded
// queue never waits for filesystem I/O in a request; failure is reported apart
// from the HTTP result and the replay/record session outcome.
func (r *boundaryRecorder) close() error {
	r.closeOnce.Do(func() {
		r.mu.Lock()
		r.closed = true
		close(r.queue)
		r.mu.Unlock()
		<-r.done
		r.mu.Lock()
		defer r.mu.Unlock()
		syncErr := r.fileIO.Sync(r.file)
		closeErr := r.file.Close()
		r.closeErr = errors.Join(r.writeErr, syncErr, closeErr)
		r.index.Written, r.index.WriteFailed = r.written, r.failed
		r.index.FileIO = &r.fileIO
		r.index.Status = "complete"
		r.index.FlushStatus = "queue_drained_sync_close"
		r.index.ProducerState = "admission_closed"
		r.index.QueueState = "drained"
		r.index.WriterState = "drained"
		r.index.DurabilityState = "events_sync_ok"
		if r.index.Dropped != 0 {
			r.index.QueueState = "drained_with_drops"
		}
		if r.writeErr != nil {
			r.index.WriterState = "drained_with_write_errors"
		}
		if syncErr != nil {
			r.index.DurabilityState = "events_sync_failed"
			r.index.FlushStatus = "queue_drained_sync_failed"
		} else if closeErr != nil {
			r.index.DurabilityState = "events_close_failed"
			r.index.FlushStatus = "queue_drained_close_failed"
		}
		r.index.FinalizedAtUTC = time.Now().UTC()
		if hash, bytes, err := hashFile(filepath.Join(r.directory, r.index.EventsFile)); err != nil {
			r.closeErr = errors.Join(r.closeErr, fmt.Errorf("hash model boundary events: %w", err))
			r.index.DurabilityState = "events_hash_failed"
			r.index.FlushStatus = "hash_failed"
		} else {
			r.index.EventsSHA256, r.index.EventsBytes = hash, bytes
		}
		if r.index.Dropped != 0 || r.index.RequestsReceived != r.index.RequestsFinished || r.late != 0 {
			r.closeErr = errors.Join(r.closeErr, errors.New("model request boundary observations incomplete"))
		}
		if r.index.RequestsReceived != r.index.RequestsFinished {
			r.index.ProducerState = "unfinished_requests"
		}
		if r.closeErr != nil {
			r.index.Status = "incomplete"
		}
		data, err := json.MarshalIndent(r.index, "", "  ")
		if err == nil {
			err = writePrivateAtomic(filepath.Join(r.directory, "request-boundaries-index.json"), data)
		}
		r.closeErr = errors.Join(r.closeErr, err)
	})
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.late != 0 {
		return errors.Join(r.closeErr, r.lateErr, fmt.Errorf("model boundary events after close: %d", r.late))
	}
	return r.closeErr
}

func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", n, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// writePrivateAtomic makes the finalized index either absent or fully visible.
// A crash may leave the temporary file behind, but never a partial index at the
// public path; an absent index is intentionally treated as incomplete.
func writePrivateAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".index-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return os.ErrExist
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(tmpPath, path)
}

// ObservationError is inspected after Close. It does not change model HTTP
// outcomes or mark a valid tape incomplete solely because telemetry was lost.
func (s *Server) ObservationError() error {
	// Read independently: never hold observationMu while acquiring the boundary
	// writer lock, and retain sidecar failures even without a boundary recorder.
	legacyErr := s.legacyFileIOError()
	r := s.boundaries
	if r == nil {
		return legacyErr
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.late != 0 {
		return errors.Join(legacyErr, r.closeErr, r.lateErr, fmt.Errorf("model boundary events after close: %d", r.late))
	}
	return errors.Join(legacyErr, r.closeErr)
}

type requestObservation struct {
	server         *Server
	id             string
	received       time.Time
	turn           *int
	route, timing  string
	retry          bool
	status, reason string
	httpStatus     int
}

func (s *Server) beginRequest(at time.Time, route string) *requestObservation {
	r := s.boundaries
	if r == nil {
		return &requestObservation{server: s, received: at, route: route, timing: "undetermined", status: "incomplete", reason: "HANDLER_INTERRUPTED"}
	}
	r.mu.Lock()
	r.index.RequestsReceived++
	id := r.index.EmitterID + "/http-" + strconv.FormatUint(r.index.RequestsReceived, 10)
	r.mu.Unlock()
	p := &requestObservation{server: s, id: id, received: at, route: route,
		timing: "undetermined", status: "incomplete", reason: "HANDLER_INTERRUPTED"}
	p.emit(boundaryEvent{Event: "http_request_received", Scope: "http_handler"}, at)
	return p
}

func (p *requestObservation) emit(event boundaryEvent, at time.Time) {
	if p.server.boundaries == nil {
		return
	}
	event.RequestID, event.Turn = p.id, p.turn
	event.Mode, event.Timing, event.Route = p.server.options.Mode, p.timing, p.route
	event.ReplayPolicy = p.server.options.ReplayPolicy
	event.CachedRetry = p.retry
	p.server.boundaries.record(event, at)
}

func (p *requestObservation) legacy(event observation) {
	event.RequestID = p.id
	p.server.observe(event)
}

func (p *requestObservation) firstFlushed(seen *[4]bool, parsed *streamResult, event boundaryEvent, at time.Time) {
	for i, first := range []struct {
		event   string
		present bool
	}{
		{"downstream_first_sse", true},
		{"downstream_first_content", len(parsed.text) > 0},
		{"downstream_first_reasoning", len(parsed.reasoning) > 0},
		{"downstream_first_tool_call_delta", parsed.hasToolDelta()},
	} {
		if first.present && !seen[i] {
			seen[i] = true
			event.Event = first.event
			p.emit(event, at)
		}
	}
}

func (p *requestObservation) result(status, reason string, httpStatus int) {
	p.status, p.reason, p.httpStatus = status, reason, httpStatus
}

func (p *requestObservation) finish() {
	if p.server.boundaries == nil {
		return
	}
	at := time.Now()
	duration := at.Sub(p.received).Nanoseconds()
	p.emit(boundaryEvent{Event: "http_request_finished", Scope: "http_handler", Status: p.status,
		Reason: p.reason, HTTPStatus: p.httpStatus, DurationNS: &duration}, at)
}

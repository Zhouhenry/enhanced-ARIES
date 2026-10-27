// Package performance records client-side operation timing without changing
// task outcomes. Each recorder owns one process's monotonic clock domain.
package performance

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

const SchemaVersion = 2
const eventBufferSize = 16384

type Identity struct {
	LogicalTaskID string `json:"logical_task_id,omitempty"`
	OccurrenceID  string `json:"occurrence_id,omitempty"`
	ArrivalID     string `json:"arrival_id,omitempty"`
	Purpose       string `json:"purpose,omitempty"`
}

// Event contains measurements, identifiers and bounded outcome categories, never
// command text, credentials, model messages or raw provider errors.
type Event struct {
	Identity
	SchemaVersion          int        `json:"schema_version"`
	RunID                  string     `json:"run_id"`
	EmitterID              string     `json:"emitter_id"`
	ClockDomain            string     `json:"clock_domain"`
	Sequence               uint64     `json:"sequence"`
	EventType              string     `json:"event_type"`
	Operation              string     `json:"operation"`
	OperationID            string     `json:"operation_id,omitempty"`
	ParentOperationID      string     `json:"parent_operation_id,omitempty"`
	ObservedAtUTC          time.Time  `json:"observed_at_utc"`
	ObservedOffsetNS       int64      `json:"observed_offset_ns"`
	StartedAtUTC           *time.Time `json:"started_at_utc,omitempty"`
	DurationNS             *int64     `json:"duration_ns,omitempty"`
	ScheduleAnchorOffsetNS *int64     `json:"schedule_anchor_offset_ns,omitempty"`
	PlannedOffsetNS        *int64     `json:"planned_offset_ns,omitempty"`
	Status                 string     `json:"status,omitempty"`
	Reason                 string     `json:"reason,omitempty"`
	ErrorClass             string     `json:"error_class,omitempty"`
	SourceComponent        string     `json:"source_component,omitempty"`
	MeasurementScope       string     `json:"measurement_scope,omitempty"`
	RequestInstanceID      string     `json:"request_instance_id,omitempty"`
	NativeRunID            string     `json:"native_run_id,omitempty"`
	AgentSessionID         string     `json:"agent_session_id,omitempty"`
	ToolCallID             string     `json:"tool_call_id,omitempty"`
	NativeSequence         *int64     `json:"native_sequence,omitempty"`
	GatewayDeliveryID      string     `json:"gateway_delivery_id,omitempty"`
}

type Manifest struct {
	SchemaVersion int       `json:"schema_version"`
	RunID         string    `json:"run_id"`
	EmitterID     string    `json:"emitter_id"`
	ClockDomain   string    `json:"clock_domain"`
	StartedAtUTC  time.Time `json:"started_at_utc"`
	EventsFile    string    `json:"events_file"`
	Scope         string    `json:"scope"`
	GoVersion     string    `json:"go_version"`
	Platform      string    `json:"platform"`
	BuildRevision string    `json:"build_revision,omitempty"`
	BuildModified *bool     `json:"build_modified,omitempty"`
}

// Index is written only after the writer has drained. Its absence means the
// recording was not finalized, even if the last JSONL line happens to be valid.
type Index struct {
	SchemaVersion      int       `json:"schema_version"`
	RunID              string    `json:"run_id"`
	EmitterID          string    `json:"emitter_id"`
	Status             string    `json:"status"`
	Attempted          uint64    `json:"attempted"`
	Written            uint64    `json:"written"`
	Dropped            uint64    `json:"dropped"`
	WriteFailed        uint64    `json:"write_failed"`
	OperationsStarted  uint64    `json:"operations_started"`
	OperationsFinished uint64    `json:"operations_finished"`
	EventsSHA256       string    `json:"events_sha256,omitempty"`
	EventsBytes        int64     `json:"events_bytes,omitempty"`
	FinalizedAtUTC     time.Time `json:"finalized_at_utc,omitempty"`
	FlushStatus        string    `json:"flush_status,omitempty"`
	// Lifecycle fields keep producer, queue, writer and durability boundaries
	// independently auditable. A complete status requires all of them.
	ProducerState   string          `json:"producer_state,omitempty"`
	QueueState      string          `json:"queue_state,omitempty"`
	WriterState     string          `json:"writer_state,omitempty"`
	DurabilityState string          `json:"durability_state,omitempty"`
	FileIO          *FileIOCounters `json:"file_io,omitempty"`
}

type Recorder struct {
	manifest           Manifest
	dir                string
	file               eventFile
	anchor             time.Time
	now                func() time.Time
	queue              chan Event
	done               chan struct{}
	mu                 sync.Mutex
	closed             bool
	sequence           uint64
	dropped            uint64
	written            uint64 // Writer goroutine only, read after done.
	failed             uint64
	writeErr           error
	closeErr           error
	close              sync.Once
	opIndex            atomic.Uint64
	late               uint64
	lateErr            error
	operationsStarted  uint64
	operationsFinished uint64
	fileIO             FileIOCounters
}

// Ordinary file Write/Sync calls have no cancellation contract. The narrow
// interface also permits controlled faults without exhausting a real disk.
type eventFile interface {
	io.WriteCloser
	Sync() error
}

// Open creates private artifacts exclusively; existing evidence is never replaced.
// The caller owns the output root and must close the recorder after all producers.
func Open(outputRoot, runID string) (*Recorder, error) {
	if runID == "" {
		return nil, errors.New("performance run ID is required")
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, err
	}
	dir := filepath.Join(outputRoot, "performance")
	if err := os.Mkdir(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create performance directory: %w", err)
	}
	file, err := os.OpenFile(filepath.Join(dir, "runner-events.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, errors.Join(err, os.Remove(dir))
	}
	anchor := time.Now()
	emitter := hex.EncodeToString(id[:])
	r := &Recorder{
		dir: dir, file: file, anchor: anchor, now: time.Now,
		fileIO: FileIOCounters{Scope: "runner_event_jsonl_only"},
		queue:  make(chan Event, eventBufferSize), done: make(chan struct{}),
		manifest: Manifest{
			SchemaVersion: SchemaVersion, RunID: runID, EmitterID: emitter,
			ClockDomain: emitter, StartedAtUTC: anchor.UTC(), EventsFile: "runner-events.jsonl",
			Scope: "scheduler_and_runner", GoVersion: runtime.Version(), Platform: runtime.GOOS + "/" + runtime.GOARCH,
		},
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				r.manifest.BuildRevision = setting.Value
			case "vcs.modified":
				value := setting.Value == "true"
				r.manifest.BuildModified = &value
			}
		}
	}
	if err := writeJSON(filepath.Join(dir, "manifest.json"), r.manifest); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	go r.write()
	return r, nil
}

func (r *Recorder) write() {
	defer close(r.done)
	for event := range r.queue {
		if r.writeErr != nil {
			r.failed++
			continue
		}
		data, err := json.Marshal(event)
		if err == nil {
			data = append(data, '\n')
			var n int
			n, err = r.fileIO.Write(r.file, data)
			if err == nil && n != len(data) {
				err = io.ErrShortWrite
			}
		}
		if err != nil {
			r.writeErr = err
			r.failed++
			continue
		}
		r.written++
	}
}

func (r *Recorder) record(event Event, at time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		r.late++
		if r.late == 1 {
			// A finalized index cannot foresee a producer violating its lifetime.
			// Persist a separate invalidation, never rewrite the original evidence.
			r.lateErr = writeJSON(filepath.Join(r.dir, "late-after-close.json"), struct {
				SchemaVersion int    `json:"schema_version"`
				RunID         string `json:"run_id"`
				EmitterID     string `json:"emitter_id"`
				Status        string `json:"status"`
			}{SchemaVersion, r.manifest.RunID, r.manifest.EmitterID, "incomplete"})
		}
		return
	}
	r.sequence++
	switch event.EventType {
	case "operation_started":
		r.operationsStarted++
	case "operation_finished":
		r.operationsFinished++
	}
	event.SchemaVersion = SchemaVersion
	event.RunID = r.manifest.RunID
	event.EmitterID = r.manifest.EmitterID
	event.ClockDomain = r.manifest.ClockDomain
	event.Sequence = r.sequence
	event.ObservedAtUTC = at.UTC()
	event.ObservedOffsetNS = at.Sub(r.anchor).Nanoseconds()
	select {
	case r.queue <- event:
	default:
		r.dropped++
	}
}

func (r *Recorder) Close() error {
	r.close.Do(func() {
		r.mu.Lock()
		r.closed = true
		close(r.queue)
		r.mu.Unlock()
		<-r.done
		syncErr := r.fileIO.Sync(r.file)
		closeErr := r.file.Close()
		r.closeErr = errors.Join(r.writeErr, syncErr, closeErr)
		index := Index{SchemaVersion: SchemaVersion, RunID: r.manifest.RunID, EmitterID: r.manifest.EmitterID,
			Attempted: r.sequence, Written: r.written, Dropped: r.dropped, WriteFailed: r.failed, Status: "complete",
			OperationsStarted: r.operationsStarted, OperationsFinished: r.operationsFinished,
			FinalizedAtUTC: time.Now().UTC(), FlushStatus: "queue_drained_sync_close",
			ProducerState: "admission_closed", QueueState: "drained", WriterState: "drained",
			DurabilityState: "events_sync_ok", FileIO: &r.fileIO}
		if r.dropped != 0 {
			index.QueueState = "drained_with_drops"
		}
		if r.operationsStarted != r.operationsFinished {
			index.ProducerState = "unfinished_operations"
			r.closeErr = errors.Join(r.closeErr, errors.New("performance operations unfinished at close"))
		}
		if r.writeErr != nil {
			index.WriterState = "drained_with_write_errors"
		}
		if syncErr != nil {
			index.DurabilityState = "events_sync_failed"
			index.FlushStatus = "queue_drained_sync_failed"
		} else if closeErr != nil {
			index.DurabilityState = "events_close_failed"
			index.FlushStatus = "queue_drained_close_failed"
		}
		if hash, bytes, err := hashFile(filepath.Join(r.dir, r.manifest.EventsFile)); err != nil {
			r.closeErr = errors.Join(r.closeErr, fmt.Errorf("hash performance events: %w", err))
			index.DurabilityState = "events_hash_failed"
			index.FlushStatus = "hash_failed"
		} else {
			index.EventsSHA256, index.EventsBytes = hash, bytes
		}
		if r.dropped != 0 {
			r.closeErr = errors.Join(r.closeErr, fmt.Errorf("performance buffer dropped %d events", r.dropped))
		}
		if r.closeErr != nil {
			index.Status = "incomplete"
		}
		indexErr := writeJSONAtomic(filepath.Join(r.dir, "index.json"), index)
		r.closeErr = errors.Join(r.closeErr, indexErr)
	})
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.late != 0 {
		return errors.Join(r.closeErr, r.lateErr, fmt.Errorf("performance recorder received %d events after close", r.late))
	}
	return r.closeErr
}

func writeJSON(path string, value any) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return errors.Join(encoder.Encode(value), file.Sync(), file.Close())
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

// writeJSONAtomic makes a finalized index either absent or fully visible. A
// crash may leave a temporary file, but never a partial index at its final path.
func writeJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".index-*.tmp")
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

type contextKey struct{}
type recordingContext struct {
	recorder *Recorder
	identity Identity
	parent   string
}

// Clock is an in-process clock capability. The private monotonic anchor is
// deliberately not serializable: a decoded wall anchor cannot align processes.
type Clock struct {
	EmitterID    string    `json:"emitter_id"`
	ClockDomain  string    `json:"clock_domain"`
	StartedAtUTC time.Time `json:"started_at_utc"`
	anchor       time.Time
}

// ClockFromContext returns a copy of this recorder's clock, or nil when disabled.
func ClockFromContext(ctx context.Context) *Clock {
	r := recording(ctx).recorder
	if r == nil {
		return nil
	}
	return &Clock{EmitterID: r.manifest.EmitterID, ClockDomain: r.manifest.ClockDomain,
		StartedAtUTC: r.manifest.StartedAtUTC, anchor: r.anchor}
}

// Offset accepts a time captured in this process. Callers must not pass a
// timestamp decoded from JSON or obtained from a remote service.
func (c *Clock) Offset(at time.Time) int64 { return at.Sub(c.anchor).Nanoseconds() }

// WithoutParent preserves the recorder and attempt identity for asynchronous
// readers which outlive the operation that created them.
func WithoutParent(ctx context.Context) context.Context {
	value := recording(ctx)
	value.parent = ""
	return context.WithValue(ctx, contextKey{}, value)
}

func recording(ctx context.Context) recordingContext {
	value, _ := ctx.Value(contextKey{}).(recordingContext)
	return value
}

func WithRecorder(ctx context.Context, recorder *Recorder) context.Context {
	value := recording(ctx)
	value.recorder = recorder
	return context.WithValue(ctx, contextKey{}, value)
}

func WithIdentity(ctx context.Context, identity Identity) context.Context {
	value := recording(ctx)
	if identity.LogicalTaskID != "" {
		value.identity.LogicalTaskID = identity.LogicalTaskID
	}
	if identity.OccurrenceID != "" {
		value.identity.OccurrenceID = identity.OccurrenceID
	}
	if identity.ArrivalID != "" {
		value.identity.ArrivalID = identity.ArrivalID
	}
	if identity.Purpose != "" {
		value.identity.Purpose = identity.Purpose
	}
	return context.WithValue(ctx, contextKey{}, value)
}

func Record(ctx context.Context, event Event) {
	value := recording(ctx)
	if value.recorder == nil {
		return
	}
	event.Identity = value.identity
	event.ParentOperationID = value.parent
	value.recorder.record(event, value.recorder.now())
}

// ObserveAt records an instant captured at a local call boundary. It does not
// accept remote or deserialized timestamps and never represents a duration.
func ObserveAt(ctx context.Context, event Event, at time.Time) {
	value := recording(ctx)
	if value.recorder == nil {
		return
	}
	event.Identity, event.ParentOperationID = value.identity, value.parent
	event.EventType = "observation"
	value.recorder.record(event, at)
}

// AnchorOffset relates a scheduler anchor to this recorder within one process.
// It is not a conversion between different processes' wall clocks.
func AnchorOffset(ctx context.Context, at time.Time) int64 {
	value := recording(ctx)
	if value.recorder == nil {
		return 0
	}
	return at.Sub(value.recorder.anchor).Nanoseconds()
}

type Span struct {
	recorder *Recorder
	event    Event
	started  time.Time
	ended    atomic.Bool
}

func Begin(ctx context.Context, operation string) (context.Context, *Span) {
	value := recording(ctx)
	if value.recorder == nil {
		return ctx, nil
	}
	r := value.recorder
	started := r.now()
	startedUTC := started.UTC()
	event := Event{Identity: value.identity, EventType: "operation_started", Operation: operation,
		OperationID:       fmt.Sprintf("%s-%d", r.manifest.EmitterID, r.opIndex.Add(1)),
		ParentOperationID: value.parent, StartedAtUTC: &startedUTC}
	r.record(event, started)
	span := &Span{recorder: r, event: event, started: started}
	value.parent = event.OperationID
	return context.WithValue(ctx, contextKey{}, value), span
}

func (span *Span) End(err error) {
	if span == nil || span.ended.Swap(true) {
		return
	}
	ended := span.recorder.now()
	event := span.event
	event.EventType = "operation_finished"
	duration := ended.Sub(span.started).Nanoseconds()
	event.DurationNS = &duration
	event.Status = "succeeded"
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			event.Status, event.ErrorClass = "timeout", "deadline_exceeded"
		case errors.Is(err, context.Canceled):
			event.Status, event.ErrorClass = "canceled", "canceled"
		default:
			event.Status, event.ErrorClass = "failed", "operation_error"
		}
	}
	span.recorder.record(event, ended)
}

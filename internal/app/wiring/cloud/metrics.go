package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/performance"
	"github.com/hyscale-lab/aries/pkg/runner"
	"github.com/hyscale-lab/aries/pkg/sandbox/e2b"
)

// Raw provider gauges do not implement monitor.ResourceSource's CPU counter contract.
// A snapshot can repeat provider timestamps; it is not an independent sample.
type metricSnapshot struct {
	SchemaVersion    int         `json:"schema_version"`
	RunID            string      `json:"run_id"`
	OccurrenceID     string      `json:"occurrence_id"`
	SandboxHandle    string      `json:"sandbox_handle"`
	Purpose          string      `json:"purpose"`
	RequestedAt      time.Time   `json:"requested_at_utc"`
	ClientDurationNS int64       `json:"client_rpc_duration_ns"`
	Snapshot         e2b.Metrics `json:"snapshot"`
	Error            string      `json:"error,omitempty"`
}

type metricFile interface {
	Write([]byte) (int, error)
	Sync() error
	Close() error
}

type metricSampler struct {
	mu       sync.Mutex
	stopped  bool
	final    *metricSnapshot
	finalErr error
	cancel   context.CancelFunc
	done     chan struct{}
	stopOnce sync.Once
	sandbox  *e2b.Sandbox
	purpose  string
}

type observedSandboxes struct {
	manager                         *e2b.Manager
	runID, occurrenceID, outputRoot string
	started                         time.Time
	mu                              sync.Mutex
	samplersMu                      sync.Mutex
	samplingStopped                 bool
	samplers                        map[*e2b.Sandbox]*metricSampler
	file                            metricFile
	path                            string
	snapshots, readings             int
	bytes                           int64
	fileIO                          performance.FileIOCounters
	err                             error
	finished                        bool
	observationLevel                performance.ObservationLevel
	report                          core.ObserverResult
}

func newObservedSandboxes(manager *e2b.Manager, runID, occurrenceID, root string, level performance.ObservationLevel) *observedSandboxes {
	if level == "" {
		level = performance.ObservationL1
	}
	return &observedSandboxes{manager: manager, runID: runID, occurrenceID: occurrenceID, outputRoot: root, started: time.Now(), samplers: make(map[*e2b.Sandbox]*metricSampler), observationLevel: level,
		fileIO: performance.FileIOCounters{Scope: "sandbox_metrics_jsonl_only"}}
}

func (o *observedSandboxes) Start(ctx context.Context, request core.SandboxRequest) (runner.Sandbox, error) {
	live, err := o.manager.Start(ctx, request)
	if err != nil {
		return live, err
	}
	sandbox := live.(*e2b.Sandbox)
	purpose := request.Purpose
	if purpose == "" {
		purpose = "task"
	}
	if !performance.ObservesResources(o.observationLevel) {
		return live, nil
	}
	sampling, cancel := context.WithCancel(context.WithoutCancel(ctx))
	s := &metricSampler{cancel: cancel, done: make(chan struct{}), sandbox: sandbox, purpose: purpose}
	o.samplersMu.Lock()
	if o.samplingStopped {
		o.samplersMu.Unlock()
		cancel()
		return live, errors.New("E2B observation already finalized")
	}
	o.samplers[sandbox] = s
	o.samplersMu.Unlock()
	go func() {
		defer close(s.done)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			o.sample(sampling, s)
			select {
			case <-sampling.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return live, nil
}

func (o *observedSandboxes) Stop(ctx context.Context, live runner.Sandbox) error {
	if sandbox, ok := live.(*e2b.Sandbox); ok {
		o.samplersMu.Lock()
		s := o.samplers[sandbox]
		o.samplersMu.Unlock()
		if s != nil {
			o.stopSampler(s)
		}
	}
	// Observation failure remains independent of confirmed sandbox destruction.
	return o.manager.Stop(ctx, live)
}

func (o *observedSandboxes) stopSampler(s *metricSampler) {
	s.stopOnce.Do(func() {
		s.cancel()
		s.mu.Lock()
		defer s.mu.Unlock()
		s.stopped = true
		// Keep the final RPC before deletion without waiting for file I/O.
		s.final, s.finalErr = o.collect(context.Background(), s)
	})
}

func (o *observedSandboxes) stopSampling() []*metricSampler {
	o.samplersMu.Lock()
	o.samplingStopped = true
	samplers := make([]*metricSampler, 0, len(o.samplers))
	for _, sampler := range o.samplers {
		samplers = append(samplers, sampler)
	}
	o.samplersMu.Unlock()
	for _, sampler := range samplers {
		o.stopSampler(sampler)
	}
	return samplers
}

func (o *observedSandboxes) sample(base context.Context, sampler *metricSampler) {
	sampler.mu.Lock()
	if sampler.stopped {
		sampler.mu.Unlock()
		return
	}
	record, err := o.collect(base, sampler)
	sampler.mu.Unlock()
	o.writeSample(record, err)
}

func (o *observedSandboxes) collect(base context.Context, sampler *metricSampler) (*metricSnapshot, error) {
	ctx, cancel := context.WithTimeout(base, 10*time.Second)
	defer cancel()
	started := time.Now()
	metrics, err := sampler.sandbox.Metrics(ctx)
	duration := time.Since(started)
	if err != nil && base.Err() != nil {
		return nil, nil
	}
	record := metricSnapshot{SchemaVersion: 1, RunID: o.runID, OccurrenceID: o.occurrenceID, SandboxHandle: sampler.sandbox.ID(), Purpose: sampler.purpose,
		RequestedAt: started.UTC(), ClientDurationNS: duration.Nanoseconds(), Snapshot: metrics}
	if err != nil {
		record.Error = err.Error()
	}
	return &record, err
}

func (o *observedSandboxes) writeSample(record *metricSnapshot, err error) {
	if record == nil {
		return
	}
	content, encodeErr := json.Marshal(record)
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.finished {
		return
	}
	if o.err == nil {
		o.err = errors.Join(err, encodeErr)
	}
	if encodeErr != nil {
		return
	}
	if o.file == nil {
		o.path = filepath.Join(o.outputRoot, o.occurrenceID, "sandbox-metrics.jsonl")
		if e := os.MkdirAll(filepath.Dir(o.path), 0o700); e != nil {
			if o.err == nil {
				o.err = e
			}
			return
		}
		file, e := os.OpenFile(o.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if e != nil {
			if o.err == nil {
				o.err = e
			}
			return
		}
		o.file = file
	}
	if o.bytes+int64(len(content))+1 > 64<<20 {
		if o.err == nil {
			o.err = errors.New("raw E2B metrics exceeded the 64 MiB artifact bound")
		}
		return
	}
	n, writeErr := o.fileIO.Write(o.file, append(content, '\n'))
	o.bytes += int64(n)
	if writeErr != nil {
		if o.err == nil {
			o.err = writeErr
		}
		return
	}
	if err == nil {
		o.snapshots++
		o.readings += len(record.Snapshot.Metrics)
	}
}

func (o *observedSandboxes) finish() (core.ObserverResult, error) {
	samplers := o.stopSampling()
	for _, sampler := range samplers {
		<-sampler.done
		sampler.mu.Lock()
		o.writeSample(sampler.final, sampler.finalErr)
		sampler.final = nil
		sampler.mu.Unlock()
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.finished {
		return o.report, o.err
	}
	o.finished = true
	o.report = core.ObserverResult{Status: core.StatusSucceeded, Duration: time.Since(o.started), SampleCount: o.snapshots,
		Reason: "Provider snapshots retain native gauges and available timestamps; collection interval is not the provider update cadence; repeated snapshots are not independent samples; disk/network I/O and agent/tool attribution unsupported"}
	if o.file != nil {
		o.err = errors.Join(o.err, o.fileIO.Sync(o.file), o.file.Close())
		o.report.LogPaths = []string{o.path}
	}
	if o.readings == 0 {
		o.report.Status = core.StatusNotRun
		o.report.Reason += "; no guest metric readings observed"
	}
	if o.err != nil {
		o.report.Status, o.report.Error = core.StatusFailed, o.err.Error()
	}
	return o.report, o.err
}

// The observation report is a separate file and is excluded from this scope.
func (o *observedSandboxes) fileIOReport() *performance.FileIOCounters {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.finished || o.file == nil {
		return nil
	}
	report := o.fileIO
	return &report
}

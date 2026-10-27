package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"golang.org/x/sys/unix"
)

// These are process totals: the Runner, model proxy and Go collectors share a
// heap and CPU accounting domain. They must never be summed as separate owners.
type controllerCPU struct {
	UserNS   int64 `json:"user_ns"`
	SystemNS int64 `json:"system_ns"`
}

type controllerAllocation struct {
	TotalBytes uint64 `json:"total_alloc_bytes"`
	Mallocs    uint64 `json:"mallocs"`
	NumGC      uint32 `json:"num_gc"`
	GCPauseNS  uint64 `json:"gc_pause_total_ns"`
}

type controllerResourceSample struct {
	StartedOffsetNS  int64                `json:"sampling_started_offset_ns"`
	FinishedOffsetNS int64                `json:"sampling_finished_offset_ns"`
	CPU              *controllerCPU       `json:"cpu"`
	CPUStatus        string               `json:"cpu_status"`
	PeakRSSBytes     *int64               `json:"process_lifetime_peak_rss_bytes"`
	Allocation       controllerAllocation `json:"allocation"`
	HeapAllocBytes   uint64               `json:"heap_alloc_bytes"`
}

type controllerResourceDelta struct {
	CPU        *controllerCPU       `json:"cpu"`
	Allocation controllerAllocation `json:"allocation"`
}

type controllerResources struct {
	SchemaVersion int                       `json:"schema_version"`
	RunID         string                    `json:"run_id"`
	PID           int                       `json:"pid"`
	Scope         string                    `json:"measurement_scope"`
	AnchorUTC     time.Time                 `json:"anchor_utc"`
	Status        string                    `json:"status"`
	StartWritten  bool                      `json:"start_artifact_written"`
	Start         controllerResourceSample  `json:"start"`
	End           *controllerResourceSample `json:"end"`
	Delta         *controllerResourceDelta  `json:"delta"`
	anchor        time.Time
	root          string
}

func newControllerResources() *controllerResources {
	anchor := time.Now()
	r := &controllerResources{SchemaVersion: 1, PID: os.Getpid(), Scope: "controller_shared_process_self",
		AnchorUTC: anchor.UTC(), Status: "incomplete", anchor: anchor}
	r.Start = sampleControllerResources(anchor)
	return r
}

func sampleControllerResources(anchor time.Time) controllerResourceSample {
	s := controllerResourceSample{StartedOffsetNS: time.Since(anchor).Nanoseconds(), CPUStatus: "unavailable"}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	s.Allocation = controllerAllocation{TotalBytes: m.TotalAlloc, Mallocs: m.Mallocs, NumGC: m.NumGC, GCPauseNS: m.PauseTotalNs}
	s.HeapAllocBytes = m.Alloc
	var usage unix.Rusage
	if unix.Getrusage(unix.RUSAGE_SELF, &usage) == nil {
		s.CPU = &controllerCPU{UserNS: usage.Utime.Nano(), SystemNS: usage.Stime.Nano()}
		peak := usage.Maxrss
		if runtime.GOOS != "darwin" {
			peak *= 1024
		}
		s.PeakRSSBytes, s.CPUStatus = &peak, "collected"
	}
	s.FinishedOffsetNS = time.Since(anchor).Nanoseconds()
	return s
}

func (r *controllerResources) begin(root, runID string) error {
	r.root, r.RunID = root, runID
	// Keep the first sample as an immutable artifact. Missing final data after
	// panic, SIGKILL or a blocked close must remain visibly incomplete.
	if err := r.persist("controller-resources.start.json"); err != nil {
		return err
	}
	r.StartWritten = true
	return nil
}

func controllerDelta(start, end controllerResourceSample) (*controllerResourceDelta, error) {
	a, b := start.Allocation, end.Allocation
	if b.TotalBytes < a.TotalBytes || b.Mallocs < a.Mallocs || b.NumGC < a.NumGC || b.GCPauseNS < a.GCPauseNS {
		return nil, errors.New("controller allocation counters decreased")
	}
	d := &controllerResourceDelta{Allocation: controllerAllocation{TotalBytes: b.TotalBytes - a.TotalBytes,
		Mallocs: b.Mallocs - a.Mallocs, NumGC: b.NumGC - a.NumGC, GCPauseNS: b.GCPauseNS - a.GCPauseNS}}
	if start.CPU != nil && end.CPU != nil {
		if end.CPU.UserNS < start.CPU.UserNS || end.CPU.SystemNS < start.CPU.SystemNS {
			return nil, errors.New("controller CPU counters decreased")
		}
		d.CPU = &controllerCPU{UserNS: end.CPU.UserNS - start.CPU.UserNS, SystemNS: end.CPU.SystemNS - start.CPU.SystemNS}
	}
	return d, nil
}

func (r *controllerResources) finish() error {
	if r.root == "" {
		return nil // Configuration failed before a run directory existed.
	}
	end := sampleControllerResources(r.anchor)
	r.End = &end
	delta, err := controllerDelta(r.Start, end)
	r.Delta = delta
	if err == nil && r.StartWritten && delta.CPU != nil {
		r.Status = "complete"
	}
	// This final report's serialization and I/O are outside the measured pair.
	// The outer process wait4/time observation still includes that fixed cost.
	return errors.Join(err, r.persist("controller-resources.end.json"))
}

func (r *controllerResources) persist(name string) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return persistRunResult(filepath.Join(r.root, name), append(data, '\n'))
}

package jiuwenswarm

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/harness/internal/privatefiles"
	"github.com/hyscale-lab/aries/pkg/performance"
)

type cgroupResourceEvidence struct {
	Schema    int             `json:"schema_version"`
	RunID     string          `json:"run_id"`
	TaskID    string          `json:"task_id"`
	Enabled   bool            `json:"enabled"`
	Boundary  string          `json:"boundary"`
	Scope     string          `json:"measurement_scope"`
	Group     string          `json:"cgroup_path"`
	Status    string          `json:"status"`
	Requested time.Time       `json:"requested_at_utc"`
	RPCNS     int64           `json:"client_rpc_duration_ns"`
	Reading   json.RawMessage `json:"reading,omitempty"`
}

type cgroupResourceCollection struct {
	runID, taskID string
	paths         []string
	err           error
	seen          map[string]bool
	finished      bool
}

// Start owns initialization; Run owns its two reads; Stop joins Run before the
// final read. Each boundary is immutable, so failed Stop retains earlier samples.
func (e *nativeRuntime) collectCgroupResources(ctx context.Context, boundary string) {
	if e.resources == nil {
		return
	}
	enabled := performance.ObservesResources(e.options.ObservationLevel)
	if !enabled && boundary != "start" {
		return
	}
	if e.resources.seen == nil {
		e.resources.seen = map[string]bool{}
	}
	if e.resources.seen[boundary] {
		return
	}
	e.resources.seen[boundary] = true
	row := cgroupResourceEvidence{Schema: 1, RunID: e.resources.runID, TaskID: e.resources.taskID,
		Enabled: enabled, Boundary: boundary, Scope: "agent_process_group_and_descendants", Group: e.group.CgroupPath(), Status: "not_enabled"}
	if enabled {
		row.Requested = time.Now().UTC()
		began := time.Now()
		raw, err := e.group.ReadResources(ctx)
		row.RPCNS = time.Since(began).Nanoseconds()
		row.Reading, row.Status = raw, "collected"
		if err != nil {
			row.Status = "failed"
			e.resources.err = errors.Join(e.resources.err, err)
		}
		var receipt struct {
			Files map[string]struct {
				Status string `json:"status"`
			} `json:"files"`
		}
		if err == nil && json.Unmarshal(raw, &receipt) == nil {
			for name, f := range receipt.Files {
				if f.Status == "failed" {
					e.resources.err = errors.Join(e.resources.err, errors.New("cgroup resource file failed: "+name))
					row.Status = "partial"
				}
			}
		}
	}
	raw, err := json.MarshalIndent(row, "", "  ")
	target := filepath.Join(e.artifactDir, "cgroup-resources."+boundary+".json")
	if err == nil {
		err = privatefiles.WriteArtifact(target, raw)
	}
	if err != nil {
		e.resources.err = errors.Join(e.resources.err, err)
	} else {
		e.resources.paths = append(e.resources.paths, target)
	}
}

func (e *nativeRuntime) finishCgroupResources() {
	if e.resources == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.resources.finished {
		return
	}
	e.resources.finished = true
	e.observation.LogPaths = append(e.observation.LogPaths, e.resources.paths...)
	if e.resources.err != nil {
		e.observationErr = errors.Join(e.observationErr, e.resources.err)
		e.observation.Status, e.observation.Error = core.StatusFailed, e.observationErr.Error()
	}
}

package performance

import (
	"encoding/json"
	"fmt"
	"math/big"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

type cgroupResourceFile struct {
	Status string `json:"status"`
	Raw    string `json:"raw"`
	Error  string `json:"error"`
}
type cgroupResourceReceipt struct {
	Schema    int       `json:"schema_version"`
	RunID     string    `json:"run_id"`
	TaskID    string    `json:"task_id"`
	Enabled   bool      `json:"enabled"`
	Boundary  string    `json:"boundary"`
	Scope     string    `json:"measurement_scope"`
	Group     string    `json:"cgroup_path"`
	Status    string    `json:"status"`
	Requested time.Time `json:"requested_at_utc"`
	RPCNS     int64     `json:"client_rpc_duration_ns"`
	Reading   *struct {
		Group    string                        `json:"cgroup_path"`
		Inode    string                        `json:"cgroup_inode"`
		Started  string                        `json:"guest_monotonic_started_ns"`
		Finished string                        `json:"guest_monotonic_finished_ns"`
		Observed time.Time                     `json:"observed_at_utc"`
		Files    map[string]cgroupResourceFile `json:"files"`
	} `json:"reading"`
}

func cgroupNumber(raw string) (json.Number, error) {
	if raw == "" || strings.Trim(raw, "0123456789") != "" {
		return "", fmt.Errorf("invalid cgroup counter")
	}
	if _, err := strconv.ParseUint(raw, 10, 64); err != nil {
		return "", fmt.Errorf("invalid cgroup counter")
	}
	return json.Number(raw), nil
}

func cgroupValues(files map[string]cgroupResourceFile) ([]AnalysisResourceValue, map[string][]AnalysisResourceValue, []string, error) {
	values := []AnalysisResourceValue{}
	devices := map[string][]AnalysisResourceValue{}
	unavailable := []string{}
	for _, name := range []string{"cpu.stat", "memory.current", "memory.peak", "io.stat"} {
		file, ok := files[name]
		if !ok {
			return nil, nil, nil, fmt.Errorf("missing cgroup file status: %s", name)
		}
		switch file.Status {
		case "unsupported":
			unavailable = append(unavailable, name+":unsupported")
			continue
		case "failed":
			unavailable = append(unavailable, name+":failed")
			continue
		case "empty":
			if name != "io.stat" || strings.TrimSpace(file.Raw) != "" {
				return nil, nil, nil, fmt.Errorf("invalid empty cgroup file: %s", name)
			}
			unavailable = append(unavailable, "io.stat:empty_no_device_counters")
			continue
		case "collected":
		default:
			return nil, nil, nil, fmt.Errorf("invalid cgroup file status: %s", name)
		}
		if name == "memory.current" || name == "memory.peak" {
			n, err := cgroupNumber(strings.TrimSpace(file.Raw))
			if err != nil {
				return nil, nil, nil, err
			}
			metric := "memory_current_bytes"
			if name == "memory.peak" {
				metric = "group_lifetime_memory_peak_bytes"
			}
			values = append(values, AnalysisResourceValue{Name: metric, Value: n, Unit: "bytes", Kind: "gauge", ExportEligible: true})
			continue
		}
		seen := map[string]bool{}
		for _, line := range strings.Split(strings.TrimSpace(file.Raw), "\n") {
			fields := strings.Fields(line)
			if name == "cpu.stat" {
				if len(fields) != 2 || seen[fields[0]] {
					return nil, nil, nil, fmt.Errorf("invalid cpu.stat row")
				}
				seen[fields[0]] = true
				n, err := cgroupNumber(fields[1])
				if err != nil {
					return nil, nil, nil, err
				}
				metric := map[string]string{"usage_usec": "cpu_usage_ns", "user_usec": "cpu_user_ns", "system_usec": "cpu_system_ns"}[fields[0]]
				if metric != "" {
					v, _ := new(big.Int).SetString(string(n), 10)
					v.Mul(v, big.NewInt(1000))
					values = append(values, AnalysisResourceValue{Name: metric, Value: json.Number(v.String()), Unit: "ns", Kind: "cumulative_counter", ExportEligible: true})
				}
			} else {
				if len(fields) < 2 || seen[fields[0]] {
					return nil, nil, nil, fmt.Errorf("invalid io.stat row")
				}
				seen[fields[0]] = true
				parts := strings.Split(fields[0], ":")
				if len(parts) != 2 {
					return nil, nil, nil, fmt.Errorf("invalid I/O device")
				}
				for _, part := range parts {
					if _, err := cgroupNumber(part); err != nil {
						return nil, nil, nil, err
					}
				}
				keys := map[string]bool{}
				for _, field := range fields[1:] {
					pair := strings.Split(field, "=")
					if len(pair) != 2 || keys[pair[0]] {
						return nil, nil, nil, fmt.Errorf("invalid io.stat counter")
					}
					keys[pair[0]] = true
					n, err := cgroupNumber(pair[1])
					if err != nil {
						return nil, nil, nil, err
					}
					units := map[string]string{"rbytes": "bytes", "wbytes": "bytes", "dbytes": "bytes", "rios": "count", "wios": "count", "dios": "count"}
					if unit := units[pair[0]]; unit != "" {
						devices[fields[0]] = append(devices[fields[0]], AnalysisResourceValue{Name: "block_io_" + pair[0], Value: n, Unit: unit, Kind: "cumulative_counter", ExportEligible: true})
					}
				}
			}
		}
		if name == "cpu.stat" {
			for _, key := range []string{"usage_usec", "user_usec", "system_usec"} {
				if !seen[key] {
					unavailable = append(unavailable, "cpu.stat:"+key+":unsupported")
				}
			}
		}
	}
	return values, devices, unavailable, nil
}

// Only compare the same owner's observed cumulative fields. Missing readings
// remain missing; a reset never creates a delta or a new counter epoch.
func (s *AnalysisSnapshot) checkCgroupCounters(stream *AnalysisResourceStream, sample AnalysisResourceSample) {
	for _, value := range sample.Values {
		if value.Kind != "cumulative_counter" {
			continue
		}
		current, _ := new(big.Int).SetString(string(value.Value), 10)
		found := false
		for i := len(stream.Samples) - 1; i >= 0 && !found; i-- {
			for _, older := range stream.Samples[i].Values {
				if older.Name != value.Name {
					continue
				}
				previous, _ := new(big.Int).SetString(string(older.Value), 10)
				if current.Cmp(previous) < 0 {
					s.resourceProblem(stream, "cumulative counter decreased at "+sample.Boundary+": "+value.Name)
				}
				found = true
				break
			}
		}
	}
}

func (s *AnalysisSnapshot) cgroupResourceSnapshot(root string, a AnalysisAttempt) {
	var stream *AnalysisResourceStream
	devices := map[string]*AnalysisResourceStream{}
	inode := ""
	anchor, previous := int64(-1), int64(-1)
	for index, boundary := range []string{"start", "runtime_start", "runtime_end", "stopped"} {
		name := filepath.Join(a.OccurrenceID, "harness", "cgroup-resources."+boundary+".json")
		raw, ref, exists := s.resourceFile(root, name, "guest_cgroup_resource_clock")
		if !exists {
			if stream != nil && !slices.Contains(stream.Diagnostics, "not_enabled") {
				s.resourceProblem(stream, "missing boundary: "+boundary)
			}
			continue
		}
		var row cgroupResourceReceipt
		valid := decodeDocument(raw, &row) == nil && row.Schema == 1 && row.RunID == s.RunID && row.TaskID == a.OccurrenceID && row.Boundary == boundary && row.Scope == "agent_process_group_and_descendants" && strings.HasPrefix(row.Group, "/sys/fs/cgroup/aries-") && filepath.Clean(row.Group) == row.Group
		if stream == nil {
			v := resourceStream(s.RunID, "cgroup_v2", row.Group, a.AttemptID, "agent_process_group_and_descendants", "guest_wall_clock_unaligned_and_monotonic_sampling_bracket")
			stream = &v
			stream.Component = "harness"
			stream.Diagnostics = []string{"four boundary samples, not periodic; includes Agent and descendant tools", "memory peak covers cgroup lifetime; never reset or differenced", "controller RPC cost is outside guest sampling bracket; no cross-clock alignment", "no per-tool attribution, network counters or exact Runtime resource delta"}
			if index != 0 {
				s.resourceProblem(stream, "missing boundary: start")
			}
		}
		stream.EvidenceRefs = append(stream.EvidenceRefs, ref)
		if !valid || row.Group != stream.OwnerID {
			s.resourceProblem(stream, "invalid boundary identity: "+boundary)
			continue
		}
		if !row.Enabled {
			if boundary != "start" || row.Status != "not_enabled" || row.Reading != nil {
				s.resourceProblem(stream, "invalid disabled collection")
			}
			stream.Diagnostics = append(stream.Diagnostics, "not_enabled")
			continue
		}
		if slices.Contains(stream.Diagnostics, "not_enabled") {
			s.resourceProblem(stream, "enabled boundary after disabled collection")
			continue
		}
		sample := resourceSample(ref, 0, index)
		sample.Boundary = boundary
		sample.Status = row.Status
		sample.RequestedAtUTC = &row.Requested
		sample.ClientRPCDurationNS = &row.RPCNS
		if row.Status == "failed" && row.Reading == nil {
			sample.Unavailable = []string{"cgroup_read:failed"}
			stream.Samples = append(stream.Samples, sample)
			s.resourceProblem(stream, "read failed: "+boundary)
			continue
		}
		r := row.Reading
		if r == nil || r.Group != row.Group || r.Inode == "" || (inode != "" && inode != r.Inode) || r.Observed.IsZero() || row.Requested.IsZero() || row.RPCNS < 0 || (row.Status != "collected" && row.Status != "partial") {
			s.resourceProblem(stream, "invalid cgroup reading: "+boundary)
			continue
		}
		start, err1 := strconv.ParseInt(r.Started, 10, 64)
		end, err2 := strconv.ParseInt(r.Finished, 10, 64)
		if err1 != nil || err2 != nil || start < 0 || end < start || start < previous {
			s.resourceProblem(stream, "invalid guest monotonic bracket: "+boundary)
			continue
		}
		inode = r.Inode
		if anchor < 0 {
			anchor = start
		}
		previous = end
		beginOffset, endOffset := start-anchor, end-anchor
		sample.SamplingStartedOffsetNS = &beginOffset
		sample.SamplingFinishedOffsetNS = &endOffset
		sample.ObservedAtUTC = &r.Observed
		sample.TimeStatus = "verified" // Observed guest clock, explicitly unaligned.
		values, ioValues, unavailable, err := cgroupValues(r.Files)
		if err != nil {
			s.resourceProblem(stream, err.Error())
			continue
		}
		sample.Values, sample.Unavailable = values, unavailable
		s.checkCgroupCounters(stream, sample)
		stream.Samples = append(stream.Samples, sample)
		for _, file := range r.Files {
			if file.Status == "failed" {
				s.resourceProblem(stream, "file read failed: "+boundary)
			}
		}
		deviceNames := make([]string, 0, len(ioValues))
		for device := range ioValues {
			deviceNames = append(deviceNames, device)
		}
		sort.Strings(deviceNames)
		for _, device := range deviceNames {
			values := ioValues[device]
			d := devices[device]
			if d == nil {
				v := resourceStream(s.RunID, "cgroup_v2", row.Group+"/device/"+device, a.AttemptID, "agent_process_group_block_device", "guest_wall_clock_unaligned_and_monotonic_sampling_bracket")
				d = &v
				d.Component = "harness"
				d.ClockDomain = stream.ClockDomain
				d.Diagnostics = []string{"per-device block I/O; do not sum stacked devices; excludes network"}
				devices[device] = d
			}
			ds := sample
			ds.ID = stableAnalysisID("resource-sample", sample.ID, device)
			ds.Values = values
			ds.Unavailable = nil
			s.checkCgroupCounters(d, ds)
			d.Samples = append(d.Samples, ds)
			d.EvidenceRefs = append(d.EvidenceRefs, ref)
		}
	}
	if stream == nil {
		return
	}
	s.addResource(*stream)
	keys := make([]string, 0, len(devices))
	for key := range devices {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		d := devices[key]
		if stream.IntegrityStatus != "complete" {
			d.IntegrityStatus = "incomplete"
		}
		s.addResource(*d)
	}
}

package performance

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"path/filepath"
)

type jiuwenProducerAudit struct {
	Status     string `json:"status"`
	Scope      string `json:"scope"`
	Started    int    `json:"started_spans"`
	Ended      int    `json:"ended_spans"`
	Exported   int    `json:"exported_spans"`
	Missing    int    `json:"missing_exports"`
	Unexpected int    `json:"unexpected_exports"`
	Error      string `json:"error"`
	Files      []struct {
		Name      string `json:"name"`
		Size      int    `json:"size"`
		Retained  int    `json:"retained_bytes"`
		SourceSHA string `json:"source_sha256"`
		SHA       string `json:"retained_sha256"`
		Error     string `json:"error"`
	} `json:"files"`
}

// This verifies the collector's frozen ID audit, not private OTLP payloads.
func (s *AnalysisSnapshot) jiuwenProducerSnapshot(root string, attempt traceAttempt, index jiuwenSpanIndex) bool {
	audit := index.ProducerAudit
	coverage := SourceCoverage{Source: "jiuwen_sampled_sdk_span_exports", AttemptID: attempt.AttemptID, Integrity: "incomplete", Status: "incomplete"}
	defer func() { s.Coverage = append(s.Coverage, coverage) }()
	fail := func() bool {
		s.problem("Jiuwen sampled SDK producer audit sources, status or counts do not reconcile for " + attempt.AttemptID)
		return false
	}
	if audit.Status != "complete" || audit.Scope != "sampled_sdk_spans" || audit.Error != "" || audit.Started < 0 || audit.Started != audit.Ended || audit.Ended != audit.Exported || audit.Exported != index.Records || audit.Missing != 0 || audit.Unexpected != 0 || len(audit.Files) != 2 || !index.NativeStop.ExitObserved {
		return fail()
	}
	contents := make(map[string][]byte)
	for _, file := range audit.Files {
		if file.Name != "status.json" && file.Name != "events.jsonl" || contents[file.Name] != nil {
			return fail()
		}
		name := filepath.Join(attempt.OccurrenceID, "harness", "native-producer", file.Name)
		data, err := readIdentityEvidence(root, name)
		s.source(name, data, "identity_only", "not_a_clock", err)
		if err != nil || file.Error != "" || file.Size <= 0 || file.Size != file.Retained || file.Retained != len(data) || file.SourceSHA != file.SHA || file.SHA != fmt.Sprintf("%x", sha256.Sum256(data)) {
			return fail()
		}
		contents[file.Name] = data
	}
	var status struct {
		Schema        int      `json:"schema_version"`
		Protocol      string   `json:"protocol"`
		Run           string   `json:"run_id"`
		Task          string   `json:"task_id"`
		PID           int      `json:"pid"`
		Before        bool     `json:"registered_before_provider"`
		Attached      bool     `json:"attached"`
		Status        string   `json:"status"`
		Started       int      `json:"started_spans"`
		Ended         int      `json:"ended_spans"`
		Records       int      `json:"written_records"`
		Bytes         int      `json:"written_bytes"`
		WriteFailures *int     `json:"write_failures"`
		Dropped       *int     `json:"dropped_records"`
		LimitExceeded *bool    `json:"limit_exceeded"`
		Flushes       *int     `json:"force_flush_callbacks"`
		Shutdown      bool     `json:"processor_shutdown_observed"`
		StopStarted   bool     `json:"extension_shutdown_started"`
		StopReturned  bool     `json:"extension_shutdown_returned"`
		SHA           string   `json:"ledger_sha256"`
		Errors        []string `json:"errors"`
	}
	ledger := contents["events.jsonl"]
	if identityJSON(contents["status.json"], &status, false) != nil || status.Schema != 1 || status.Protocol != "jiuwen-native-span-audit-v1" || status.Run != s.RunID || status.Task != attempt.OccurrenceID || status.PID <= 0 || !status.Before || !status.Attached || status.Status != "complete" || !status.Shutdown || !status.StopStarted || !status.StopReturned || status.Started != audit.Started || status.Ended != audit.Ended || status.WriteFailures == nil || *status.WriteFailures != 0 || status.Dropped == nil || *status.Dropped != 0 || status.LimitExceeded == nil || *status.LimitExceeded || len(status.Errors) != 0 || status.Flushes == nil || *status.Flushes < 0 || status.Records != status.Started+status.Ended+*status.Flushes+1 || status.Records != bytes.Count(ledger, []byte{'\n'}) || len(ledger) == 0 || ledger[len(ledger)-1] != '\n' || status.Bytes != len(ledger) || status.SHA != fmt.Sprintf("%x", sha256.Sum256(ledger)) {
		return fail()
	}
	expected := uint64(audit.Ended)
	coverage.Integrity, coverage.Status = "complete", "complete"
	coverage.ExpectedEvents, coverage.ObservedEvents = &expected, uint64(audit.Exported)
	coverage.Started, coverage.Paired = uint64(audit.Started), uint64(audit.Ended)
	s.Limitations = append(s.Limitations, "Jiuwen producer reconciliation covers only sampled SDK span IDs and terminal fields after Stop. It does not cover unsampled or uninstrumented work, provide an exporter error counter, establish bounded flush, or align native and controller clocks.")
	return true
}

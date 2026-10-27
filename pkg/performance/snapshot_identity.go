package performance

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// This extension contains identity evidence only: no event, clock or duration.
type AnalysisNativeToolIdentities struct {
	SchemaVersion int                             `json:"schema_version"`
	Attempts      []AnalysisNativeIdentityAttempt `json:"attempts"`
}

type IdentityEvidence struct {
	Role        string `json:"role"`
	EvidenceRef string `json:"evidence_ref"`
	SourceLine  uint64 `json:"source_line"`
	JSONPointer string `json:"json_pointer"`
}

type AnalysisNativeIdentityAttempt struct {
	AttemptID              string                          `json:"attempt_id"`
	AgentSessionID         string                          `json:"agent_session_id"`
	NativeSessionID        string                          `json:"native_session_id,omitempty"`
	DenominatorScope       string                          `json:"denominator_scope"`
	ArchivedExecCalls      *uint64                         `json:"archived_exec_calls"`
	ProducerInvocations    *uint64                         `json:"producer_invocations"`
	ProducerHealth         string                          `json:"producer_health"`
	CoveredExecCalls       uint64                          `json:"covered_exec_calls"`
	AuxiliaryBridgeRecords *uint64                         `json:"auxiliary_bridge_records"`
	Status                 string                          `json:"status"`
	Bindings               []IdentityEvidence              `json:"bindings"`
	Mappings               []AnalysisNativeIdentityMapping `json:"mappings"`
	Diagnostics            []string                        `json:"diagnostics"`
	Dispatch               *AnalysisNativeDispatch         `json:"dispatch,omitempty"`
}

// These are invocation counts. ResolvedExecCalls uses the independent archived
// call denominator; one call may have several distinct native invocations.
type AnalysisNativeDispatch struct {
	ProducerRecords       uint64 `json:"producer_records"`
	DispatchedInvocations uint64 `json:"dispatched_invocations"`
	RejectedInvocations   uint64 `json:"rejected_invocations"`
	UnknownInvocations    uint64 `json:"unknown_invocations"`
	ResolvedExecCalls     uint64 `json:"resolved_exec_calls"`
}

type AnalysisNativeIdentityMapping struct {
	NativeToolIdentity
	ExecutionID     string             `json:"execution_id,omitempty"`
	ProcessID       string             `json:"process_id,omitempty"`
	SandboxHandle   string             `json:"sandbox_handle,omitempty"`
	Status          string             `json:"status"`
	GuestCompletion string             `json:"guest_completion"`
	Evidence        []IdentityEvidence `json:"evidence"`
	Reasons         []string           `json:"reasons"`
	DispatchStatus  string             `json:"dispatch_status,omitempty"`
	RejectionReason string             `json:"rejection_reason,omitempty"`
}

type identityRow struct {
	data []byte
	ref  string
	line uint64
}

type identityProducerHealth struct {
	SchemaVersion     int    `json:"schema_version"`
	ScopeID           string `json:"scope_id"`
	AgentSessionID    string `json:"agent_session_id"`
	InstanceID        string `json:"producer_instance_id"`
	Attempted         uint64 `json:"attempted"`
	Written           uint64 `json:"written"`
	WriteFailed       uint64 `json:"write_failed"`
	StatusWriteFailed uint64 `json:"status_write_failed"`
	FailedLatched     bool   `json:"failed_latched"`
}

func (r identityRow) evidence(role, pointer string) IdentityEvidence {
	return IdentityEvidence{Role: role, EvidenceRef: r.ref, SourceLine: r.line, JSONPointer: pointer}
}

// Do not follow evidence-supplied absolute paths or symlinks, even within a run.
// Archives must be quiescent: ordinary filesystem I/O is not cancellable, and
// concurrent replacement between Lstat and Open remains outside this contract.
func readIdentityEvidence(root, name string) ([]byte, error) {
	if !filepath.IsLocal(name) || filepath.Clean(name) != name {
		return nil, errors.New("nonlocal evidence")
	}
	dir, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	parts := strings.Split(name, string(filepath.Separator))
	for i := range parts {
		info, err := dir.Lstat(filepath.Join(parts[:i+1]...))
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 || (i < len(parts)-1 && !info.IsDir()) || (i == len(parts)-1 && !info.Mode().IsRegular()) {
			return nil, errors.New("invalid evidence path")
		}
	}
	f, err := dir.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil || !before.Mode().IsRegular() || before.Size() > 64<<20 {
		return nil, errors.New("invalid evidence file")
	}
	data, err := io.ReadAll(io.LimitReader(f, (64<<20)+1))
	after, statErr := dir.Lstat(name)
	if err != nil || statErr != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) || before.Size() != int64(len(data)) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return nil, errors.New("evidence changed or exceeded bound")
	}
	return data, nil
}

// Validate duplicate keys recursively, including nested identities. Standard
// Unmarshal silently chooses one duplicate and cannot establish exact identity.
func identityJSON(data []byte, target any, exact bool) error {
	if !utf8.Valid(data) {
		return errors.New("invalid JSON UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var value func(int) error
	value = func(depth int) error {
		if depth > 128 {
			return errors.New("JSON nesting bound")
		}
		token, err := d.Token()
		if err != nil {
			return err
		}
		open, container := token.(json.Delim)
		if !container {
			return nil
		}
		keys := map[string]bool{}
		for d.More() {
			if open == '{' {
				key, err := d.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || keys[name] {
					return errors.New("duplicate JSON key")
				}
				keys[name] = true
			}
			if err := value(depth + 1); err != nil {
				return err
			}
		}
		close, err := d.Token()
		if err != nil || open != '{' && open != '[' || open == '{' && close != json.Delim('}') || open == '[' && close != json.Delim(']') {
			return errors.New("invalid JSON container")
		}
		return nil
	}
	if err := value(0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("trailing JSON")
	}
	d = json.NewDecoder(bytes.NewReader(data))
	if exact {
		d.DisallowUnknownFields()
	}
	return d.Decode(target)
}

// encoding/json also accepts case-insensitive struct keys. Evidence pointers
// name exact keys, so aliases must never override the value at that pointer.
func identityCanonicalKeys(data []byte, keys ...string) bool {
	var object map[string]json.RawMessage
	if json.Unmarshal(data, &object) != nil || object == nil {
		return false
	}
	for key := range object {
		for _, wanted := range keys {
			if key != wanted && strings.EqualFold(key, wanted) {
				return false
			}
		}
	}
	return true
}

func (s *AnalysisSnapshot) identitySource(root, name string) (identityRow, error) {
	data, err := readIdentityEvidence(root, name)
	return identityRow{data: data, ref: s.source(name, data, "identity_only", "not_a_clock", err), line: 1}, err
}

func identityRows(source identityRow) ([]identityRow, error) {
	if len(source.data) > 0 && source.data[len(source.data)-1] != '\n' {
		return nil, errors.New("partial JSONL")
	}
	rows := []identityRow{}
	scan := bufio.NewScanner(bytes.NewReader(source.data))
	scan.Buffer(make([]byte, 4096), 1<<20)
	for scan.Scan() {
		if len(rows) >= 100000 {
			return nil, errors.New("JSONL row bound")
		}
		data := bytes.Clone(scan.Bytes())
		var object map[string]json.RawMessage
		if identityJSON(data, &object, false) != nil || object == nil {
			return nil, errors.New("invalid JSONL object")
		}
		rows = append(rows, identityRow{data: data, ref: source.ref, line: uint64(len(rows) + 1)})
	}
	return rows, scan.Err()
}

func (s *AnalysisSnapshot) nativeIdentitySnapshot(root string, attempt traceAttempt, operationEvidence *sdkOperationEvidence) {
	base := filepath.Join(attempt.OccurrenceID, "harness")
	producerName := filepath.Join(base, "native-tool-identities.jsonl")
	indexName := filepath.Join(base, "native-tool-identities.index.json")
	profileData, profileErr := readIdentityEvidence(root, "cloud-profile.json")
	var profile struct {
		Placement string `json:"placement"`
		Harness   struct {
			Protocol string `json:"native_observation_protocol"`
		} `json:"harness"`
	}
	profileValid := profileErr == nil && identityJSON(profileData, &profile, false) == nil && identityCanonicalKeys(profileData, "placement", "harness")
	var profileFields map[string]json.RawMessage
	_ = json.Unmarshal(profileData, &profileFields)
	profileValid = profileValid && identityCanonicalKeys(profileFields["harness"], "native_observation_protocol")
	_, producerErr := os.Lstat(filepath.Join(root, producerName))
	_, indexErr := os.Lstat(filepath.Join(root, indexName))
	if (!profileValid || profile.Harness.Protocol == "") && os.IsNotExist(producerErr) && os.IsNotExist(indexErr) {
		return
	}
	// Embedded process callbacks use gateway.log, not the external SSH identity source.
	if profileValid && profile.Placement == "embedded" && profile.Harness.Protocol == "aries-native-process-v1" && os.IsNotExist(producerErr) && os.IsNotExist(indexErr) {
		return
	}
	group := AnalysisNativeIdentityAttempt{AttemptID: attempt.AttemptID, AgentSessionID: attempt.AgentSessionID,
		DenominatorScope: "archived_assistant_exec_observations; native_capture_loss_unknown", Status: "incomplete", ProducerHealth: "unknown",
		Bindings: []IdentityEvidence{}, Mappings: []AnalysisNativeIdentityMapping{}, Diagnostics: []string{}}
	defer func() {
		if s.NativeToolIdentities == nil {
			s.NativeToolIdentities = &AnalysisNativeToolIdentities{SchemaVersion: 1, Attempts: []AnalysisNativeIdentityAttempt{}}
		}
		s.NativeToolIdentities.Attempts = append(s.NativeToolIdentities.Attempts, group)
		coverage := SourceCoverage{Source: "native_exec_identity", AttemptID: attempt.AttemptID, Integrity: "complete", Status: "unknown",
			ExpectedOperations: group.ArchivedExecCalls, Paired: group.CoveredExecCalls,
			Missing: []string{"native_transcript_capture_loss_unknown", "identity_only_no_native_tool_timing"}}
		// This coverage row counts archived exec observations, not invocations.
		// One observation may have several independently identified invocations.
		if group.ArchivedExecCalls != nil {
			coverage.ObservedEvents, coverage.Started = *group.ArchivedExecCalls, *group.ArchivedExecCalls
		}
		if group.Dispatch != nil {
			coverage.Paired = group.Dispatch.ResolvedExecCalls
		}
		if group.Status != "exact_for_archived_exec_observations" && group.Status != "accounted_for_archived_exec_observations" {
			coverage.Integrity, coverage.Status = "incomplete", "incomplete"
			s.problem("native exec identity incomplete for " + attempt.AttemptID)
		}
		s.Coverage = append(s.Coverage, coverage)
	}()
	group.Bindings = append(group.Bindings, IdentityEvidence{Role: "profile", EvidenceRef: s.source("cloud-profile.json", profileData, "identity_only", "not_a_clock", profileErr), SourceLine: 1, JSONPointer: "/harness/native_observation_protocol"})
	if !profileValid || profile.Placement != "external" || profile.Harness.Protocol != "aries-ssh-identity-v1" || !validAnalysisIdentity(attempt.AgentSessionID) {
		group.Diagnostics = append(group.Diagnostics, "identity_opt_in_or_placement_invalid")
		return
	}
	calls, err := s.identityCalls(root, base, attempt, &group)
	if err != nil {
		group.Diagnostics = append(group.Diagnostics, "native_session_denominator_invalid")
		return
	}
	producer, producerReadErr := s.identitySource(root, producerName)
	indexSource, indexReadErr := s.identitySource(root, indexName)
	var index struct {
		SchemaVersion  int                     `json:"schema_version"`
		Protocol       string                  `json:"protocol"`
		ScopeID        string                  `json:"scope_id"`
		AgentSessionID string                  `json:"agent_session_id"`
		Integrity      string                  `json:"file_integrity"`
		Events         *uint64                 `json:"events"`
		Expected       *uint64                 `json:"expected_native_calls"`
		Coverage       string                  `json:"coverage"`
		SHA            string                  `json:"sha256"`
		Issue          string                  `json:"issue"`
		Health         *identityProducerHealth `json:"producer_health,omitempty"`
		StatusSHA      string                  `json:"producer_status_sha256,omitempty"`
	}
	rows, rowsErr := identityRows(producer)
	sum := sha256.Sum256(producer.data)
	var indexFields map[string]json.RawMessage
	indexObjectValid := identityJSON(indexSource.data, &indexFields, false) == nil && indexFields != nil && identityCanonicalKeys(indexSource.data, "schema_version", "protocol", "scope_id", "agent_session_id", "file_integrity", "events", "expected_native_calls", "coverage", "sha256", "issue", "producer_health", "producer_status_sha256")
	validProducer := producerReadErr == nil && indexReadErr == nil && identityJSON(indexSource.data, &index, true) == nil && rowsErr == nil &&
		index.SchemaVersion == 1 && index.Protocol == "aries-ssh-identity-v1" && index.ScopeID == attempt.AttemptID && index.AgentSessionID == attempt.AgentSessionID &&
		index.Integrity == "valid" && index.Issue == "" && index.Events != nil && *index.Events == uint64(len(rows)) && index.SHA == hex.EncodeToString(sum[:]) && index.Expected == nil && index.Coverage == "producer_only_independent_denominator_required" && indexObjectValid && bytes.Equal(indexFields["expected_native_calls"], []byte("null"))
	group.Bindings = append(group.Bindings, indexSource.evidence("producer_index", ""))
	_, healthDeclared := indexFields["producer_health"]
	_, statusPresenceErr := os.Lstat(filepath.Join(root, base, "native-tool-identities.status.json"))
	if healthDeclared || index.StatusSHA != "" || !os.IsNotExist(statusPresenceErr) {
		group.ProducerHealth = "invalid"
		statusSource, statusErr := s.identitySource(root, filepath.Join(base, "native-tool-identities.status.json"))
		group.Bindings = append(group.Bindings, statusSource.evidence("producer_status", ""))
		var health, indexedHealth identityProducerHealth
		statusSHA := sha256.Sum256(statusSource.data)
		if statusErr == nil && decodeProducerHealth(statusSource.data, &health) == nil && decodeProducerHealth(indexFields["producer_health"], &indexedHealth) == nil &&
			index.Health != nil && health == *index.Health && index.StatusSHA == hex.EncodeToString(statusSHA[:]) && health.SchemaVersion == 1 &&
			health.ScopeID == attempt.AttemptID && health.AgentSessionID == attempt.AgentSessionID && validIdentityProducerInstance(health.InstanceID) &&
			health.Written == uint64(len(rows)) && health.Attempted == health.Written && health.WriteFailed == 0 && health.StatusWriteFailed == 0 && !health.FailedLatched {
			group.ProducerHealth = "healthy_for_archived_producer_instance"
		} else {
			validProducer = false
			group.Diagnostics = append(group.Diagnostics, "producer_health_missing_or_invalid")
		}
	}
	byCall := map[string][]identityProducer{}
	if validProducer {
		records, decodeErr := DecodeNativeProducerRecords(producer.data)
		if decodeErr != nil {
			validProducer = false
		}
		rejections := map[string]int{}
		for i, record := range records {
			id := record.Identity
			if id.ScopeID != attempt.AttemptID || id.AgentSessionID != attempt.AgentSessionID {
				validProducer = false
				break
			}
			if record.Phase == "pre_dispatch" {
				rejections[id.NativeInvocationID] = i
			}
		}
		if validProducer {
			if len(records) > 0 && records[0].SchemaVersion == 2 {
				group.Dispatch = &AnalysisNativeDispatch{ProducerRecords: uint64(len(records))}
			}
			for i, record := range records {
				if record.Phase != "invocation" {
					continue
				}
				id := record.Identity
				p := identityProducer{id: id, row: rows[i], version: record.SchemaVersion}
				if rejection, exists := rejections[id.NativeInvocationID]; exists {
					p.rejection, p.rejectionReason = &rows[rejection], records[rejection].Reason
				}
				byCall[id.ToolCallID] = append(byCall[id.ToolCallID], p)
			}
		}
	}
	if !validProducer {
		byCall = map[string][]identityProducer{}
		group.Diagnostics = append(group.Diagnostics, "producer_missing_or_invalid")
	} else {
		var count uint64
		for _, producers := range byCall {
			count += uint64(len(producers))
		}
		group.ProducerInvocations = &count
	}
	bridges, bridgeErr := s.identityBridges(root, attempt, &group)
	operations, operationErr := identityOperations(operationEvidence, &group)
	if bridgeErr != nil {
		group.Diagnostics = append(group.Diagnostics, "bridge_source_missing_or_invalid")
	}
	if operationErr != nil {
		group.Diagnostics = append(group.Diagnostics, "operation_source_missing_or_invalid")
	}
	used := map[string]bool{}
	for _, call := range calls {
		producers := byCall[call.id]
		if len(producers) == 0 {
			group.Mappings = append(group.Mappings, AnalysisNativeIdentityMapping{NativeToolIdentity: NativeToolIdentity{SchemaVersion: 1, ScopeID: attempt.AttemptID, AgentSessionID: attempt.AgentSessionID, ToolCallID: call.id},
				Status: "unlinked", GuestCompletion: "unknown", Evidence: []IdentityEvidence{call.evidence}, Reasons: []string{"producer_missing_or_invalid"}})
			continue
		}
		allExact, allResolved := true, true
		for _, producer := range producers {
			m := identityMapping(producer, bridges, operations)
			m.Evidence = append([]IdentityEvidence{call.evidence}, m.Evidence...)
			group.Mappings = append(group.Mappings, m)
			used[producer.id.NativeInvocationID] = true
			allExact = allExact && m.Status == "exact"
			allResolved = allResolved && (m.Status == "exact" || m.Status == "rejected_before_dispatch")
			if group.Dispatch != nil {
				switch m.DispatchStatus {
				case "dispatched":
					group.Dispatch.DispatchedInvocations++
				case "rejected_before_dispatch":
					group.Dispatch.RejectedInvocations++
				default:
					group.Dispatch.UnknownInvocations++
				}
			}
		}
		if allExact {
			group.CoveredExecCalls++
		}
		if allResolved && group.Dispatch != nil {
			group.Dispatch.ResolvedExecCalls++
		}
	}
	// Orphan producer/transport identities remain visible; never guess a call.
	orphans := map[string]NativeToolIdentity{}
	for _, producers := range byCall {
		for _, p := range producers {
			if !used[p.id.NativeInvocationID] {
				orphans[p.id.NativeInvocationID] = p.id
				if group.Dispatch != nil {
					group.Dispatch.UnknownInvocations++
				}
			}
		}
	}
	for _, b := range bridges {
		if b.id != nil && !used[b.id.NativeInvocationID] {
			orphans[b.id.NativeInvocationID] = *b.id
		}
	}
	for _, op := range operations {
		if op.id != nil && !used[op.id.NativeInvocationID] {
			orphans[op.id.NativeInvocationID] = *op.id
		}
	}
	keys := make([]string, 0, len(orphans))
	for key := range orphans {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		id := orphans[key]
		refs := []IdentityEvidence{}
		for _, ps := range byCall {
			for _, p := range ps {
				if p.id.NativeInvocationID == key {
					refs = append(refs, p.row.evidence("orphan_producer", ""))
				}
			}
		}
		for _, b := range bridges {
			if b.id != nil && b.id.NativeInvocationID == key {
				refs = append(refs, b.row.evidence("orphan_bridge", "/native_identity"))
			}
		}
		for _, op := range operations {
			if op.id != nil && op.id.NativeInvocationID == key {
				refs = append(refs, op.row.evidence("orphan_operation", "/native_identity"))
			}
		}
		group.Mappings = append(group.Mappings, AnalysisNativeIdentityMapping{NativeToolIdentity: id, Status: "unlinked", GuestCompletion: "unknown", Evidence: refs, Reasons: []string{"no_unique_archived_exec_observation"}})
	}
	if len(group.Diagnostics) == 0 && len(orphans) == 0 && group.ArchivedExecCalls != nil && group.CoveredExecCalls == *group.ArchivedExecCalls {
		group.Status = "exact_for_archived_exec_observations"
	} else if len(group.Diagnostics) == 0 && len(orphans) == 0 && group.ArchivedExecCalls != nil && group.Dispatch != nil && group.Dispatch.ResolvedExecCalls == *group.ArchivedExecCalls {
		group.Status = "accounted_for_archived_exec_observations"
	}
}

type identityCall struct {
	id       string
	evidence IdentityEvidence
}
type identityProducer struct {
	id              NativeToolIdentity
	row             identityRow
	version         int
	rejection       *identityRow
	rejectionReason string
}

func validIdentityProducerInstance(id string) bool {
	if len(id) == 0 || len(id) > 256 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("._:/|-", c)) {
			return false
		}
	}
	return true
}

func decodeProducerHealth(raw []byte, health *identityProducerHealth) error {
	var fields map[string]json.RawMessage
	if identityJSON(raw, &fields, false) != nil || len(fields) != 9 {
		return errors.New("invalid producer health fields")
	}
	for _, key := range []string{"schema_version", "scope_id", "agent_session_id", "producer_instance_id", "attempted", "written", "write_failed", "status_write_failed", "failed_latched"} {
		if len(fields[key]) == 0 || bytes.Equal(fields[key], []byte("null")) {
			return errors.New("missing producer health field")
		}
	}
	return identityJSON(raw, health, true)
}

func decodeIdentityMetadata(raw []byte, id *NativeToolIdentity, wire bool) error {
	var fields map[string]json.RawMessage
	if len(raw) > 4096 || identityJSON(raw, &fields, false) != nil {
		return errors.New("invalid native identity")
	}
	keys := []string{"schema_version", "scope_id", "agent_session_id", "tool_call_id", "native_invocation_id"}
	if wire {
		keys = append(keys, "ssh_request_id")
	}
	if len(fields) != len(keys) {
		return errors.New("invalid native identity keys")
	}
	for _, key := range keys {
		if len(fields[key]) == 0 || bytes.Equal(fields[key], []byte("null")) {
			return errors.New("missing native identity key")
		}
	}
	if identityJSON(raw, id, true) != nil {
		return errors.New("invalid native identity values")
	}
	if wire {
		return id.Validate()
	}
	return id.ValidateNative()
}

func (s *AnalysisSnapshot) identityCalls(root, base string, a traceAttempt, group *AnalysisNativeIdentityAttempt) ([]identityCall, error) {
	bad := errors.New("invalid native session evidence")
	resultSource, err := s.identitySource(root, filepath.Join(base, "agent-result.json"))
	if err != nil {
		return nil, bad
	}
	var result struct {
		Requested string `json:"requested_native_session_id"`
		Run       string `json:"run_id"`
	}
	if identityJSON(resultSource.data, &result, false) != nil || !identityCanonicalKeys(resultSource.data, "requested_native_session_id", "run_id") || !validAnalysisIdentity(result.Requested) || filepath.Base(result.Requested) != result.Requested || strings.ContainsAny(result.Requested, `/\\`) || !validAnalysisIdentity(result.Run) {
		return nil, bad
	}
	group.NativeSessionID = result.Requested
	group.Bindings = append(group.Bindings, resultSource.evidence("requested_native_session", "/requested_native_session_id"))
	indexSource, err := s.identitySource(root, filepath.Join(base, "telemetry.index.json"))
	if err != nil {
		return nil, bad
	}
	var index struct {
		Paths []string `json:"paths"`
	}
	if identityJSON(indexSource.data, &index, true) != nil || !identityCanonicalKeys(indexSource.data, "paths") || len(index.Paths) > 1024 {
		return nil, bad
	}
	selected := "telemetry/" + result.Requested + ".jsonl"
	paths := map[string]bool{}
	matching := 0
	var selectedRows []identityRow
	for _, name := range index.Paths {
		if !filepath.IsLocal(name) || filepath.ToSlash(filepath.Clean(name)) != name || filepath.Dir(name) != "telemetry" || paths[name] {
			return nil, bad
		}
		paths[name] = true
		if !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		source, err := s.identitySource(root, filepath.Join(base, name))
		if err != nil {
			return nil, bad
		}
		rows, err := identityRows(source)
		if err != nil || len(rows) == 0 {
			return nil, bad
		}
		var header struct {
			Type    string `json:"type"`
			Version int    `json:"version"`
			ID      string `json:"id"`
		}
		if identityJSON(rows[0].data, &header, false) != nil || !identityCanonicalKeys(rows[0].data, "type", "version", "id") {
			return nil, bad
		}
		if header.Type == "session" && header.ID == result.Requested {
			matching++
			if name != selected || header.Version != 3 {
				return nil, bad
			}
			selectedRows = rows
		}
	}
	if matching != 1 || !paths[selected] {
		return nil, bad
	}
	// Reject unindexed session files: a duplicate cannot hide outside the index.
	dir, err := os.OpenRoot(root)
	if err != nil {
		return nil, bad
	}
	defer dir.Close()
	telemetryDir := filepath.Join(base, "telemetry")
	for name := telemetryDir; name != "."; name = filepath.Dir(name) {
		info, err := dir.Lstat(name)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, bad
		}
	}
	f, err := dir.Open(telemetryDir)
	if err != nil {
		return nil, bad
	}
	entries, listErr := f.ReadDir(100001)
	closeErr := f.Close()
	if listErr != nil && listErr != io.EOF || closeErr != nil || len(entries) > 100000 {
		return nil, bad
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".jsonl") && (!paths["telemetry/"+entry.Name()] || !entry.Type().IsRegular()) {
			return nil, bad
		}
	}
	// Check the archived key-to-UUID binding, never open its container path.
	if !paths["telemetry/sessions.json"] {
		return nil, bad
	}
	sessionsSource, err := s.identitySource(root, filepath.Join(base, "telemetry", "sessions.json"))
	if err != nil {
		return nil, bad
	}
	var sessions map[string]struct {
		ID string `json:"sessionId"`
	}
	if identityJSON(sessionsSource.data, &sessions, false) != nil || sessions[a.AgentSessionID].ID != result.Requested {
		return nil, bad
	}
	var sessionEntries map[string]json.RawMessage
	_ = json.Unmarshal(sessionsSource.data, &sessionEntries)
	if !identityCanonicalKeys(sessionEntries[a.AgentSessionID], "sessionId") {
		return nil, bad
	}
	group.Bindings = append(group.Bindings, indexSource.evidence("telemetry_index", "/paths"), selectedRows[0].evidence("native_session_header", "/id"), sessionsSource.evidence("native_session_key", "/"+strings.NewReplacer("~", "~0", "/", "~1").Replace(a.AgentSessionID)+"/sessionId"))
	calls := []identityCall{}
	seen := map[string]bool{}
	for i, row := range selectedRows {
		var e struct {
			Type    string `json:"type"`
			Message struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if identityJSON(row.data, &e, false) != nil || !identityCanonicalKeys(row.data, "type", "message") || i > 0 && e.Type == "session" {
			return nil, bad
		}
		if e.Type != "message" {
			continue
		}
		var messageFields map[string]json.RawMessage
		_ = json.Unmarshal(row.data, &messageFields)
		if !identityCanonicalKeys(messageFields["message"], "role", "content") {
			return nil, bad
		}
		if e.Message.Role != "assistant" {
			continue
		}
		var content []json.RawMessage
		if identityJSON(e.Message.Content, &content, false) != nil || len(bytes.TrimSpace(e.Message.Content)) == 0 || bytes.TrimSpace(e.Message.Content)[0] != '[' {
			return nil, bad
		}
		for j, raw := range content {
			var item struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
			}
			if identityJSON(raw, &item, false) != nil || !identityCanonicalKeys(raw, "type", "id", "name") {
				return nil, bad
			}
			if item.Type != "toolCall" || item.Name != "exec" {
				continue
			}
			if !validAnalysisIdentity(item.ID) || seen[item.ID] {
				return nil, bad
			}
			seen[item.ID] = true
			calls = append(calls, identityCall{item.ID, row.evidence("archived_assistant_exec", fmt.Sprintf("/message/content/%d", j))})
		}
	}
	count := uint64(len(calls))
	group.ArchivedExecCalls = &count
	return calls, nil
}

type identityBridge struct {
	row                identityRow
	id                 *NativeToolIdentity
	execution, sandbox string
}

func (s *AnalysisSnapshot) identityBridges(root string, a traceAttempt, group *AnalysisNativeIdentityAttempt) ([]identityBridge, error) {
	source, err := s.identitySource(root, filepath.Join(a.OccurrenceID, "bridge", "tool-calls.jsonl"))
	if err != nil {
		return nil, err
	}
	rows, err := identityRows(source)
	if err != nil {
		return nil, err
	}
	result := []identityBridge{}
	var auxiliary uint64
	seenExecution, seenSSH, seenInvocation := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for i, row := range rows {
		var b struct {
			Sequence  uint64              `json:"sequence"`
			Status    string              `json:"identity_status"`
			Identity  *NativeToolIdentity `json:"native_identity"`
			Execution string              `json:"execution_id"`
			Run       string              `json:"run_id"`
			Task      string              `json:"task_id"`
			Sandbox   string              `json:"sandbox_id"`
		}
		if identityJSON(row.data, &b, false) != nil || !identityCanonicalKeys(row.data, "sequence", "identity_status", "native_identity", "execution_id", "run_id", "task_id", "sandbox_id") || b.Sequence != uint64(i+1) || b.Run != s.RunID || b.Task != a.OccurrenceID {
			return nil, errors.New("invalid bridge identity source")
		}
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(row.data, &fields)
		if raw, present := fields["native_identity"]; present {
			if b.Identity == nil || decodeIdentityMetadata(raw, b.Identity, true) != nil {
				return nil, errors.New("invalid bridge native metadata")
			}
		}
		if b.Identity == nil {
			if b.Status != "unlinked" {
				return nil, errors.New("invalid bridge unlinked status")
			}
			auxiliary++
			continue
		}
		if b.Status != "exact_channel" || b.Identity.Validate() != nil || b.Identity.ScopeID != a.AttemptID || b.Identity.AgentSessionID != a.AgentSessionID || !validAnalysisIdentity(b.Execution) || !validAnalysisIdentity(b.Sandbox) || seenExecution[b.Execution] || seenSSH[b.Identity.SSHRequestID] || seenInvocation[b.Identity.NativeInvocationID] {
			return nil, errors.New("ambiguous bridge identity")
		}
		seenExecution[b.Execution], seenSSH[b.Identity.SSHRequestID], seenInvocation[b.Identity.NativeInvocationID] = true, true, true
		result = append(result, identityBridge{row, b.Identity, b.Execution, b.Sandbox})
	}
	group.AuxiliaryBridgeRecords = &auxiliary
	return result, nil
}

type identityOperation struct {
	row                                           identityRow
	id                                            *NativeToolIdentity
	event, operation, execution, process, sandbox string
	operationID                                   uint64
	receipt                                       json.RawMessage
	terminal                                      bool
	status, logical, purpose                      string
	offset, duration, guestDuration               *int64
	exit                                          *int
	calls                                         []AnalysisSDKCall
}

func identityOperations(evidence *sdkOperationEvidence, group *AnalysisNativeIdentityAttempt) ([]identityOperation, error) {
	if evidence == nil {
		return nil, errors.New("operation evidence not collected")
	}
	if evidence.err != nil {
		return nil, evidence.err
	}
	group.Bindings = append(group.Bindings, evidence.index.evidence("operation_index", ""))
	var operations []identityOperation
	for _, op := range evidence.operations {
		if op.id != nil {
			operations = append(operations, op)
		}
	}
	return operations, nil
}

func sameNativeIdentity(a, b *NativeToolIdentity) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func identityMapping(p identityProducer, bridges []identityBridge, operations []identityOperation) AnalysisNativeIdentityMapping {
	m := AnalysisNativeIdentityMapping{NativeToolIdentity: p.id, Status: "unlinked", GuestCompletion: "unknown", Evidence: []IdentityEvidence{p.row.evidence("native_producer", "")}, Reasons: []string{}}
	if p.version == 2 {
		m.DispatchStatus = "unknown"
	}
	if p.rejection != nil {
		m.Evidence = append(m.Evidence, p.rejection.evidence("native_pre_dispatch_rejection", ""))
		m.RejectionReason = p.rejectionReason
		for _, b := range bridges {
			if b.id.NativeInvocationID == p.id.NativeInvocationID {
				m.Reasons = append(m.Reasons, "pre_dispatch_rejection_conflicts_with_transport")
				return m
			}
		}
		for _, op := range operations {
			if op.id.NativeInvocationID == p.id.NativeInvocationID {
				m.Reasons = append(m.Reasons, "pre_dispatch_rejection_conflicts_with_transport")
				return m
			}
		}
		m.Status, m.DispatchStatus, m.GuestCompletion = "rejected_before_dispatch", "rejected_before_dispatch", "not_dispatched"
		return m
	}
	var bridge *identityBridge
	for i := range bridges {
		b := &bridges[i]
		native := *b.id
		native.SSHRequestID = ""
		if native == p.id {
			if bridge != nil {
				m.Reasons = append(m.Reasons, "bridge_identity_ambiguous")
				return m
			}
			bridge = b
		}
	}
	if bridge == nil {
		m.Reasons = append(m.Reasons, "bridge_identity_missing_or_invalid")
		return m
	}
	m.NativeToolIdentity = *bridge.id
	m.ExecutionID = bridge.execution
	m.SandboxHandle = bridge.sandbox
	m.Evidence = append(m.Evidence, bridge.row.evidence("bridge_audit", "/native_identity"))
	if p.version == 2 {
		m.DispatchStatus = "dispatched"
	}
	var start, wait *identityOperation
	conflict := false
	for i := range operations {
		op := &operations[i]
		if op.id.NativeInvocationID != p.id.NativeInvocationID {
			continue
		}
		if *op.id != *bridge.id || op.execution != bridge.execution || op.sandbox != bridge.sandbox {
			conflict = true
			continue
		}
		if m.ProcessID != "" && m.ProcessID != op.process {
			conflict = true
		}
		m.ProcessID = op.process
		if op.operation == "adapter_rpc.start" && op.event == "start" {
			if start != nil {
				conflict = true
			}
			start = op
		}
		if op.operation == "exec.wait" && op.event == "end" {
			if wait != nil {
				conflict = true
			}
			wait = op
		}
	}
	// A process UUID cannot belong to another invocation in this attempt.
	for _, op := range operations {
		if m.ProcessID != "" && op.process == m.ProcessID && op.id.NativeInvocationID != p.id.NativeInvocationID {
			conflict = true
		}
	}
	if conflict || start == nil || wait == nil {
		m.Reasons = append(m.Reasons, "process_identity_missing_or_ambiguous")
		return m
	}
	m.Evidence = append(m.Evidence, start.row.evidence("process_submission", "/process_id"), wait.row.evidence("process_wait", "/process_id"))
	var receipt struct {
		Kind        string `json:"kind"`
		Phase       string `json:"phase"`
		Observation struct {
			Schema    int                `json:"schema_version"`
			Identity  NativeToolIdentity `json:"native_identity"`
			Execution string             `json:"execution_id"`
			Process   string             `json:"process_id"`
		} `json:"observation"`
	}
	if identityJSON(wait.receipt, &receipt, true) != nil || receipt.Kind != "execution_identity" || receipt.Phase != "before_popen" || receipt.Observation.Schema != 1 || receipt.Observation.Identity != *bridge.id || receipt.Observation.Execution != bridge.execution || receipt.Observation.Process != m.ProcessID {
		m.Reasons = append(m.Reasons, "guest_receipt_missing_or_conflicting")
		return m
	}
	var receiptFields map[string]json.RawMessage
	_ = json.Unmarshal(wait.receipt, &receiptFields)
	var envelope map[string]json.RawMessage
	_ = json.Unmarshal(receiptFields["observation"], &envelope)
	if len(receiptFields) != 3 || len(receiptFields["kind"]) == 0 || len(receiptFields["phase"]) == 0 || len(envelope) != 4 || len(envelope["schema_version"]) == 0 || len(envelope["execution_id"]) == 0 || len(envelope["process_id"]) == 0 || decodeIdentityMetadata(envelope["native_identity"], &receipt.Observation.Identity, true) != nil {
		m.Reasons = append(m.Reasons, "guest_receipt_invalid_metadata")
		return m
	}
	m.Evidence = append(m.Evidence, wait.row.evidence("raw_guest_before_popen", "/guest_identity_receipt"))
	m.Status = "exact"
	if wait.terminal {
		m.GuestCompletion = "validated_terminal_fields_present"
	}
	return m
}

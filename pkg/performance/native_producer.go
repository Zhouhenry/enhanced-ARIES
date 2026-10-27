package performance

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
)

// NativeProducerRecord keeps producer lifecycle evidence separate from the
// unchanged v1 identity sent through SSH. It contains no command or ToolResult.
type NativeProducerRecord struct {
	SchemaVersion int
	Identity      NativeToolIdentity
	Phase         string
	Outcome       string
	Dispatched    *bool
	Reason        string
}

// DecodeNativeProducerRecords accepts archived v1 identities and v2 producer
// records. A rejection must follow its one exact invocation in the same stream.
func DecodeNativeProducerRecords(content []byte) ([]NativeProducerRecord, error) {
	if len(content) == 0 || content[len(content)-1] != '\n' {
		return nil, errors.New("native producer JSONL is empty or partial")
	}
	records := []NativeProducerRecord{}
	invoked := map[string]NativeToolIdentity{}
	rejected := map[string]bool{}
	version := 0
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 4096), 4097)
	for scanner.Scan() {
		record, err := decodeNativeProducerRecord(scanner.Bytes())
		if err != nil {
			return nil, err
		}
		if version != 0 && version != record.SchemaVersion {
			return nil, errors.New("mixed native producer record versions")
		}
		version = record.SchemaVersion
		id := record.Identity
		if record.Phase == "invocation" {
			if _, exists := invoked[id.NativeInvocationID]; exists {
				return nil, errors.New("duplicate native producer invocation")
			}
			invoked[id.NativeInvocationID] = id
		} else {
			original, exists := invoked[id.NativeInvocationID]
			if !exists || original != id || rejected[id.NativeInvocationID] {
				return nil, errors.New("rejection lacks one preceding exact invocation")
			}
			rejected[id.NativeInvocationID] = true
		}
		records = append(records, record)
		if len(records) > 65536 {
			return nil, errors.New("native producer record bound exceeded")
		}
	}
	return records, scanner.Err()
}

func decodeNativeProducerRecord(raw []byte) (NativeProducerRecord, error) {
	var record NativeProducerRecord
	var fields map[string]json.RawMessage
	if len(raw) > 4096 || identityJSON(raw, &fields, false) != nil || fields == nil || json.Unmarshal(fields["schema_version"], &record.SchemaVersion) != nil {
		return record, errors.New("invalid native producer record")
	}
	if record.SchemaVersion == 1 {
		if err := decodeIdentityMetadata(raw, &record.Identity, false); err != nil {
			return record, err
		}
		record.Phase, record.Outcome = "invocation", "unknown"
		return record, nil
	}
	if record.SchemaVersion != 2 || len(fields) != 9 {
		return record, errors.New("unsupported native producer fields or version")
	}
	identity := map[string]json.RawMessage{"schema_version": json.RawMessage("1")}
	for _, key := range []string{"scope_id", "agent_session_id", "tool_call_id", "native_invocation_id"} {
		identity[key] = fields[key]
	}
	encoded, err := json.Marshal(identity)
	if err != nil || decodeIdentityMetadata(encoded, &record.Identity, false) != nil {
		return record, errors.New("invalid native producer identity")
	}
	for _, field := range []struct {
		key   string
		value *string
	}{{"phase", &record.Phase}, {"outcome", &record.Outcome}, {"reason", &record.Reason}} {
		if bytes.Equal(fields[field.key], []byte("null")) || json.Unmarshal(fields[field.key], field.value) != nil {
			return record, errors.New("invalid native producer disposition")
		}
	}
	dispatched, exists := fields["dispatched"]
	if !exists || json.Unmarshal(dispatched, &record.Dispatched) != nil {
		return record, errors.New("invalid native producer dispatch state")
	}
	invocation := record.Phase == "invocation" && record.Outcome == "started" && record.Dispatched == nil && record.Reason == ""
	rejection := record.Phase == "pre_dispatch" && record.Outcome == "rejected" && record.Dispatched != nil && !*record.Dispatched &&
		(record.Reason == "shell_preflight_rejected" || record.Reason == "elevated_policy_rejected" || record.Reason == "host_policy_rejected")
	if !invocation && !rejection {
		return record, errors.New("unsupported native producer disposition")
	}
	return record, nil
}

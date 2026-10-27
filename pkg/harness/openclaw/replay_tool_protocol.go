package openclaw

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/hyscale-lab/aries/pkg/runner"
)

const replayToolProtocolPath = "/tmp/aries-replay-tool-protocol.jsonl"

type replayToolProtocolIndex struct {
	SchemaVersion int    `json:"schema_version"`
	Protocol      string `json:"protocol"`
	FileIntegrity string `json:"file_integrity"`
	SHA256        string `json:"sha256,omitempty"`
	Issue         string `json:"issue,omitempty"`
}

// Retention is independent of the Agent result. Protocol semantics and coverage
// still require reconciliation against the original native invocation records.
func (manager *Manager) collectReplayToolProtocol(ctx context.Context, active *session) error {
	inputs := manager.options.WorkloadInputs
	if inputs == nil || inputs.ReplayToolProtocol == "" {
		return nil
	}
	index := replayToolProtocolIndex{SchemaVersion: 1, Protocol: inputs.ReplayToolProtocol, FileIntegrity: "incomplete"}
	content, err := manager.readNativeIdentityArchive(ctx, active, replayToolProtocolPath, maxNativeIdentityBytes)
	if errors.Is(err, runner.ErrNotFound) {
		index.FileIntegrity, index.Issue = "not_collected", "producer_artifact_missing"
	} else if err != nil {
		index.Issue = "producer_artifact_unreadable"
	}
	if content != nil {
		redacted := active.Credentials.Redact(content)
		if !bytes.Equal(content, redacted) {
			err = errors.Join(err, errors.New("replay tool protocol artifact contains a credential value"))
			index.Issue = "credential_value_redacted"
		}
		if writeErr := active.Artifacts.Write("replay-tool-protocol.jsonl", redacted); writeErr != nil {
			err = errors.Join(err, writeErr)
			index.Issue = "artifact_write_failed"
		} else {
			digest := sha256.Sum256(redacted)
			index.SHA256 = hex.EncodeToString(digest[:])
		}
	}
	if err == nil {
		index.FileIntegrity = "retained"
	}
	encoded, encodeErr := json.MarshalIndent(index, "", "  ")
	if encodeErr == nil {
		encodeErr = active.Artifacts.Write("replay-tool-protocol.index.json", append(encoded, '\n'))
	}
	return errors.Join(err, encodeErr)
}

package openclaw

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"
	"unicode/utf8"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/deployment"
	"github.com/hyscale-lab/aries/pkg/performance"
)

const nativeIdentityScopePath = "/run/aries/tool-identity-scope.json"
const nativeIdentityProducerPath = "/tmp/aries-native-tool-identities.jsonl"
const nativeIdentityStatusPath = "/tmp/aries-native-tool-identities.status.json"
const maxNativeIdentityBytes = 4 << 20

// NativeIdentityScope is an opt-in contract with the isolated native candidate.
// It is not a model input or a replacement for the native session key.
type NativeIdentityScope struct {
	ScopeID        string `json:"scope_id"`
	AgentSessionID string `json:"agent_session_id"`
}

var nativeScopeChars = regexp.MustCompile(`^[A-Za-z0-9._:/|-]+$`)

func (scope NativeIdentityScope) validate() error {
	for _, value := range []string{scope.ScopeID, scope.AgentSessionID} {
		if len(value) == 0 || len(value) > 256 || !nativeScopeChars.MatchString(value) {
			return errors.New("native identity scope must match the candidate's bounded ASCII contract")
		}
	}
	return nil
}

func (manager *Manager) stageNativeIdentity(active *session) error {
	scope := manager.options.NativeIdentity
	if scope == nil {
		return nil
	}
	if scope.ScopeID != active.RunID+"/"+active.TaskID {
		return errors.New("native identity scope differs from the harness occurrence")
	}
	content, err := json.Marshal(scope)
	if err != nil {
		return err
	}
	source := filepath.Join(active.ArtifactDir, "tool-identity-scope.json")
	file, err := os.OpenFile(source, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o444)
	if err != nil {
		return fmt.Errorf("create native identity scope: %w", err)
	}
	_, writeErr := file.Write(content)
	err = errors.Join(writeErr, file.Chmod(0o444), file.Sync(), file.Close())
	if err != nil {
		_ = os.Remove(source)
		return fmt.Errorf("retain native identity scope: %w", err)
	}
	// A mode bit alone is insufficient in a node-owned directory. Docker's
	// verified single-file read-only bind prevents replacement by the agent.
	active.DeploymentRequest.Mounts = append(active.DeploymentRequest.Mounts, deployment.Mount{
		Source: source, Target: nativeIdentityScopePath, ReadOnly: true,
	})
	active.Artifacts.Add(source)
	return nil
}

type nativeIdentityIndex struct {
	SchemaVersion        int                   `json:"schema_version"`
	Protocol             string                `json:"protocol"`
	ScopeID              string                `json:"scope_id"`
	AgentSessionID       string                `json:"agent_session_id"`
	FileIntegrity        string                `json:"file_integrity"`
	Events               int                   `json:"events"`
	ExpectedNativeCalls  *int                  `json:"expected_native_calls"`
	Coverage             string                `json:"coverage"`
	SHA256               string                `json:"sha256,omitempty"`
	Issue                string                `json:"issue,omitempty"`
	ProducerHealth       *nativeProducerHealth `json:"producer_health,omitempty"`
	ProducerStatusSHA256 string                `json:"producer_status_sha256,omitempty"`
}

type nativeProducerHealth struct {
	SchemaVersion      int    `json:"schema_version"`
	ScopeID            string `json:"scope_id"`
	AgentSessionID     string `json:"agent_session_id"`
	ProducerInstanceID string `json:"producer_instance_id"`
	Attempted          uint64 `json:"attempted"`
	Written            uint64 `json:"written"`
	WriteFailed        uint64 `json:"write_failed"`
	StatusWriteFailed  uint64 `json:"status_write_failed"`
	FailedLatched      bool   `json:"failed_latched"`
}

// NativeIdentityObservation reports producer evidence separately from Agent and
// lifecycle results. Valid files still need an independent invocation denominator.
func (manager *Manager) NativeIdentityObservation() (core.ObserverResult, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.options.NativeIdentity == nil {
		return core.ObserverResult{Status: core.StatusNotEnabled}, nil
	}
	if manager.nativeObservation.Status == "" {
		return core.ObserverResult{Status: core.StatusNotRun, Reason: "native identity evidence was not collected"}, nil
	}
	report := manager.nativeObservation
	report.LogPaths = append([]string(nil), report.LogPaths...)
	return report, manager.nativeObservationErr
}

func (manager *Manager) collectNativeIdentities(ctx context.Context, active *session) (returnErr error) {
	scope := manager.options.NativeIdentity
	if scope == nil {
		return nil
	}
	started := time.Now()
	var paths []string
	defer func() {
		report := core.ObserverResult{Status: core.StatusSucceeded, Duration: time.Since(started), LogPaths: paths,
			Reason: "native producer file integrity only; independent invocation denominator required"}
		returnErr = active.Credentials.RedactErr(returnErr)
		if returnErr != nil {
			report.Status, report.Error = core.StatusFailed, returnErr.Error()
		}
		manager.mu.Lock()
		manager.nativeObservation, manager.nativeObservationErr = report, returnErr
		manager.mu.Unlock()
	}()
	retain := func(name string, content []byte) error {
		if err := active.Artifacts.Write(name, content); err != nil {
			return err
		}
		paths = append(paths, filepath.Join(active.ArtifactDir, name))
		return nil
	}
	index := nativeIdentityIndex{SchemaVersion: 1, Protocol: "aries-ssh-identity-v1", ScopeID: scope.ScopeID,
		AgentSessionID: scope.AgentSessionID, FileIntegrity: "incomplete", Coverage: "producer_only_independent_denominator_required"}
	content, err := manager.readNativeIdentityArchive(ctx, active, nativeIdentityProducerPath, maxNativeIdentityBytes)
	if content != nil {
		redacted := active.Credentials.Redact(content)
		if !bytes.Equal(content, redacted) {
			err = errors.Join(err, errors.New("native identity artifact contains a credential value"))
		}
		content = redacted
		digest := sha256.Sum256(content)
		index.SHA256 = hex.EncodeToString(digest[:])
		err = errors.Join(err, retain("native-tool-identities.jsonl", content))
	}
	if err == nil {
		index.Events, err = validateNativeIdentities(content, *scope)
	}
	status, statusErr := manager.readNativeIdentityArchive(ctx, active, nativeIdentityStatusPath, 8192)
	if status != nil {
		redacted := active.Credentials.Redact(status)
		if !bytes.Equal(status, redacted) {
			statusErr = errors.Join(statusErr, errors.New("native identity status contains a credential value"))
		}
		status = redacted
		digest := sha256.Sum256(status)
		index.ProducerStatusSHA256 = hex.EncodeToString(digest[:])
		statusErr = errors.Join(statusErr, retain("native-tool-identities.status.json", status))
	}
	if statusErr == nil {
		index.ProducerHealth, statusErr = decodeNativeProducerHealth(status, *scope)
		if statusErr == nil {
			health := index.ProducerHealth
			if health.Attempted < health.Written || health.WriteFailed != health.Attempted-health.Written ||
				health.Written != uint64(index.Events) || health.WriteFailed != 0 || health.StatusWriteFailed != 0 || health.FailedLatched {
				statusErr = errors.New("native identity producer health is failed or differs from retained events")
			}
		}
	}
	err = errors.Join(err, statusErr)
	if err == nil {
		index.FileIntegrity = "valid"
	} else {
		index.Issue = "producer_artifact_missing_or_invalid"
	}
	encoded, encodeErr := json.MarshalIndent(index, "", "  ")
	if encodeErr == nil {
		encodeErr = retain("native-tool-identities.index.json", append(encoded, '\n'))
	}
	return errors.Join(err, encodeErr)
}

func (manager *Manager) readNativeIdentityArchive(ctx context.Context, active *session, path string, limit int64) (retained []byte, returnErr error) {
	source, _, err := manager.runtime.Options.Deployment.DownloadArchive(ctx, active.ID, path)
	if err != nil {
		return nil, fmt.Errorf("collect native identity producer: %w", err)
	}
	defer func() { returnErr = errors.Join(returnErr, source.Close()) }()
	archive, err := io.ReadAll(io.LimitReader(source, limit+(64<<10)+1))
	if err != nil || int64(len(archive)) > limit+(64<<10) {
		return nil, errors.New("native identity archive exceeds its bound or is unreadable")
	}
	reader := tar.NewReader(bytes.NewReader(archive))
	header, err := reader.Next()
	if err != nil || (header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA) ||
		filepath.Base(header.Name) != filepath.Base(path) || header.Mode&0o777 != 0o600 ||
		header.Size < 1 || header.Size > limit {
		return nil, errors.New("native identity source must be one bounded private regular file")
	}
	content, err := io.ReadAll(io.LimitReader(reader, header.Size+1))
	if err != nil || int64(len(content)) != header.Size {
		return content, errors.New("native identity source is truncated")
	}
	if _, err := reader.Next(); err != io.EOF {
		return content, errors.New("native identity archive has extra or invalid entries")
	}
	return content, nil
}

func decodeNativeProducerHealth(content []byte, scope NativeIdentityScope) (*nativeProducerHealth, error) {
	if !utf8.Valid(content) {
		return nil, errors.New("invalid native producer health encoding")
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return nil, errors.New("native producer health must be an object")
	}
	seen := make(map[string]bool)
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[key] {
			return nil, errors.New("duplicate native producer health field")
		}
		switch key {
		case "schema_version", "scope_id", "agent_session_id", "producer_instance_id", "attempted", "written", "write_failed", "status_write_failed", "failed_latched":
		default:
			return nil, errors.New("unknown native producer health field")
		}
		seen[key] = true
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, errors.New("invalid native producer health field")
		}
	}
	if _, err := decoder.Token(); err != nil || len(seen) != 9 {
		return nil, errors.New("incomplete native producer health")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errors.New("trailing native producer health")
	}
	var health nativeProducerHealth
	if err := json.Unmarshal(content, &health); err != nil {
		return nil, errors.New("invalid native producer health types")
	}
	if health.SchemaVersion != 1 || health.ScopeID != scope.ScopeID || health.AgentSessionID != scope.AgentSessionID ||
		len(health.ProducerInstanceID) == 0 || len(health.ProducerInstanceID) > 256 || !nativeScopeChars.MatchString(health.ProducerInstanceID) ||
		health.Attempted > 1<<53-1 || health.Written > 1<<53-1 || health.WriteFailed > 1<<53-1 || health.StatusWriteFailed > 1<<53-1 {
		return nil, errors.New("invalid native producer health identity or counter")
	}
	return &health, nil
}

func validateNativeIdentities(content []byte, scope NativeIdentityScope) (int, error) {
	records, err := performance.DecodeNativeProducerRecords(content)
	if err != nil {
		return 0, err
	}
	for _, record := range records {
		if record.Identity.ScopeID != scope.ScopeID || record.Identity.AgentSessionID != scope.AgentSessionID {
			return 0, errors.New("native identity producer has foreign invocation metadata")
		}
	}
	// Producer health counts written records, not unique tool invocations.
	return len(records), nil
}

package performance

import (
	"encoding/hex"
	"strconv"
	"strings"
)

// Read only the redacted evidence fields; never carry tool text into projections.
type modelToolContentDifference struct {
	SchemaVersion int                    `json:"schema_version"`
	Path          string                 `json:"path"`
	Encoding      string                 `json:"encoding"`
	Recorded      modelToolContentDigest `json:"recorded"`
	Actual        modelToolContentDigest `json:"actual"`
}

type modelToolContentDigest struct {
	Bytes  *int64 `json:"bytes"`
	SHA256 string `json:"sha256"`
}

// Prompt context differences remain separate from tool result differences.
// Both receipts contain only full-string byte counts and hashes.
type modelPromptContextDifference struct {
	SchemaVersion int                    `json:"schema_version"`
	Path          string                 `json:"path"`
	Encoding      string                 `json:"encoding"`
	Category      string                 `json:"category"`
	Recorded      modelToolContentDigest `json:"recorded"`
	Actual        modelToolContentDigest `json:"actual"`
}

func (d *modelPromptContextDifference) UnmarshalJSON(data []byte) error {
	type receipt modelPromptContextDifference
	return identityJSON(data, (*receipt)(d), true)
}

func validModelReplayPolicy(mode, policy string) bool {
	return policy == "" || mode == "replay" && (policy == "strict" || policy == "workload")
}

func observedReplayPolicy(mode, policy string) string {
	if policy != "" {
		return policy
	}
	if mode == "replay" {
		return "unknown"
	}
	return "not_applicable"
}

func validToolContentDifferences(e modelBoundary) bool {
	if len(e.ToolContentDifferences) == 0 {
		return true
	}
	if e.Mode != "replay" || e.ReplayPolicy != "workload" || e.Event != "request_matched" || e.Status != "succeeded" {
		return false
	}
	seen := make(map[string]bool)
	for _, difference := range e.ToolContentDifferences {
		if seen[difference.Path] || !validModelContentDifference(difference) {
			return false
		}
		seen[difference.Path] = true
	}
	return true
}

func validPromptContextDifferences(e modelBoundary) bool {
	if len(e.PromptContextDifferences) == 0 {
		return true
	}
	if e.Mode != "replay" || e.ReplayPolicy != "workload" || e.Event != "request_matched" || e.Status != "succeeded" {
		return false
	}
	seen := make(map[string]bool)
	for _, difference := range e.PromptContextDifferences {
		if difference.Category != "jiuwen_system_context" && difference.Category != "jiuwen_user_timestamp" {
			return false
		}
		if seen[difference.Path] || !validModelContentDifference(modelToolContentDifference{
			SchemaVersion: difference.SchemaVersion, Path: difference.Path, Encoding: difference.Encoding,
			Recorded: difference.Recorded, Actual: difference.Actual}) {
			return false
		}
		seen[difference.Path] = true
	}
	return true
}

func validModelContentDifference(difference modelToolContentDifference) bool {
	index := strings.TrimSuffix(strings.TrimPrefix(difference.Path, "/messages/"), "/content")
	n, err := strconv.Atoi(index)
	if err != nil || n < 0 || difference.Path != "/messages/"+strconv.Itoa(n)+"/content" || difference.SchemaVersion != 1 || difference.Encoding != "utf-8" {
		return false
	}
	for _, digest := range []modelToolContentDigest{difference.Recorded, difference.Actual} {
		decoded, err := hex.DecodeString(digest.SHA256)
		if digest.Bytes == nil || *digest.Bytes < 0 || err != nil || len(decoded) != 32 {
			return false
		}
	}
	return difference.Recorded.SHA256 != difference.Actual.SHA256
}

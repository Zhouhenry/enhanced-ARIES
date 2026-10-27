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
		index := strings.TrimSuffix(strings.TrimPrefix(difference.Path, "/messages/"), "/content")
		n, err := strconv.Atoi(index)
		if err != nil || n < 0 || difference.Path != "/messages/"+strconv.Itoa(n)+"/content" || seen[difference.Path] || difference.SchemaVersion != 1 || difference.Encoding != "utf-8" {
			return false
		}
		seen[difference.Path] = true
		for _, digest := range []modelToolContentDigest{difference.Recorded, difference.Actual} {
			decoded, err := hex.DecodeString(digest.SHA256)
			if digest.Bytes == nil || *digest.Bytes < 0 || err != nil || len(decoded) != 32 {
				return false
			}
		}
		if difference.Recorded.SHA256 == difference.Actual.SHA256 {
			return false
		}
	}
	return true
}

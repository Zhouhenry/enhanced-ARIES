package performance

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

// LoadSnapshot reads a frozen analysis without consulting the original evidence.
// UseNumber preserves nanosecond-valued attributes through the identity check.
func LoadSnapshot(path string) (AnalysisSnapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return AnalysisSnapshot{}, err
	}
	defer f.Close()
	decoder := json.NewDecoder(f)
	decoder.UseNumber()
	var snapshot AnalysisSnapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return snapshot, fmt.Errorf("decode analysis snapshot: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return snapshot, errors.New("analysis snapshot contains trailing content")
	}
	// Schema 1 hashes typed receipts inside otherwise free-form attributes.
	// Restore their field order before checking the original ID.
	for _, event := range snapshot.Events {
		if err := restoreSnapshotAttributes(event.Attributes); err != nil {
			return snapshot, err
		}
	}
	for _, interval := range snapshot.Intervals {
		if err := restoreSnapshotAttributes(interval.Attributes); err != nil {
			return snapshot, err
		}
	}
	for _, operation := range snapshot.UntimedOperations {
		if err := restoreSnapshotAttributes(operation.Attributes); err != nil {
			return snapshot, err
		}
	}
	return snapshot, ValidateSnapshot(snapshot)
}

func restoreSnapshotAttributes(attributes map[string]any) error {
	if err := restoreSDKCalls(attributes); err != nil {
		return err
	}
	value, exists := attributes["tool_content_differences"]
	if !exists {
		return nil
	}
	content, err := json.Marshal(value)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var differences []modelToolContentDifference
	if err := decoder.Decode(&differences); err != nil {
		return fmt.Errorf("decode snapshot tool content differences: %w", err)
	}
	attributes["tool_content_differences"] = differences
	return nil
}

func restoreSDKCalls(attributes map[string]any) error {
	value, exists := attributes["sdk_calls_duration_only"]
	if !exists {
		return nil
	}
	content, err := json.Marshal(value)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var calls []AnalysisSDKCall
	if err := decoder.Decode(&calls); err != nil {
		return fmt.Errorf("decode snapshot SDK receipts: %w", err)
	}
	attributes["sdk_calls_duration_only"] = calls
	return nil
}

// ValidateSnapshot checks the version and immutable content identity shared by
// every exporter. Integrity and coverage retain their separate meanings.
func ValidateSnapshot(snapshot AnalysisSnapshot) error {
	check := snapshot
	check.SnapshotID = ""
	data, err := json.Marshal(check)
	if err != nil || snapshot.SchemaVersion != AnalysisSchemaVersion || snapshot.SnapshotID != stableAnalysisID("snapshot", string(data)) {
		return errors.New("invalid analysis snapshot")
	}
	return nil
}

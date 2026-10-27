package performance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSnapshotPreservesIntegerAttributesAndRejectsChangedContent(t *testing.T) {
	_, directory := traceFixture(t)
	snapshot, err := BuildSnapshot(directory)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Intervals[0].Attributes = map[string]any{"epoch_ns": int64(1792927634650437350), "sdk_calls_duration_only": []AnalysisSDKCall{{Operation: "process.wait", Status: "succeeded"}}}
	snapshot.SnapshotID = ""
	content, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.SnapshotID = stableAnalysisID("snapshot", string(content))
	content, _ = json.MarshalIndent(snapshot, "", "  ")
	path := filepath.Join(t.TempDir(), "analysis-snapshot.json")
	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SnapshotID != snapshot.SnapshotID || loaded.Intervals[0].Attributes["epoch_ns"].(json.Number).String() != "1792927634650437350" {
		t.Fatal("snapshot integer precision lost")
	}
	if _, err := ProjectTrace(loaded); err != nil {
		t.Fatal(err)
	}
	loaded.Intervals[0].DurationNS++
	if ValidateSnapshot(loaded) == nil {
		t.Fatal("changed snapshot accepted")
	}
	if err := os.WriteFile(path, append(content, []byte("\n{}")...), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSnapshot(path); err == nil {
		t.Fatal("trailing snapshot accepted")
	}
}

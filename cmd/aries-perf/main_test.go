package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyscale-lab/aries/pkg/performance"
)

func TestRunReadsBothPathsWithoutWritingArtifacts(t *testing.T) {
	root := t.TempDir()
	recorder, err := performance.Open(root, "run")
	if err != nil {
		t.Fatal(err)
	}
	_, span := performance.Begin(performance.WithRecorder(context.Background(), recorder), "operation")
	span.End(nil)
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{root, filepath.Join(root, "performance")} {
		var output, diagnostics bytes.Buffer
		if code := run([]string{path}, &output, &diagnostics); code != 0 {
			t.Fatalf("exit %d: %s", code, diagnostics.String())
		}
		var summary performance.Summary
		if err := json.Unmarshal(output.Bytes(), &summary); err != nil || summary.Status != "complete" || summary.RunID != "run" {
			t.Fatalf("report=%s, error=%v", output.String(), err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, "performance"))
	if err != nil || len(entries) != 3 {
		t.Fatalf("analyzer modified evidence: %v, %v", entries, err)
	}
}

func TestRunPrintsPartialReportWithNonzeroExit(t *testing.T) {
	root := t.TempDir()
	recorder, err := performance.Open(root, "partial")
	if err != nil {
		t.Fatal(err)
	}
	_, span := performance.Begin(performance.WithRecorder(context.Background(), recorder), "operation")
	span.End(nil)
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "performance", "index.json")); err != nil {
		t.Fatal(err)
	}
	var output, diagnostics bytes.Buffer
	if code := run([]string{root}, &output, &diagnostics); code != 1 {
		t.Fatalf("exit=%d, diagnostics=%s", code, diagnostics.String())
	}
	var summary performance.Summary
	if err := json.Unmarshal(output.Bytes(), &summary); err != nil || summary.Status != "incomplete" || len(summary.Operations) != 1 || summary.Operations[0].Succeeded != 1 || diagnostics.Len() == 0 {
		t.Fatalf("partial report lost: %s, %v", output.String(), err)
	}
}

func TestRunUsageAndOutputFailure(t *testing.T) {
	for _, args := range [][]string{nil, {""}, {"a", "b"}, {"--unknown"}, {"--trace"}, {"--trace", "--unknown"}, {"--snapshot"}, {"--trace-clock"}, {"--trace-clock", "clock"}, {"--snapshot", "--trace-clock", "clock", "path"}, {"--resource-metrics"}, {"--resource-metrics", "owner", "path"}, {"--resource-metrics", "", "--from-snapshot", "path"}, {"--resource-metrics", "owner", "--metrics", "--from-snapshot", "path"}} {
		var output, diagnostics bytes.Buffer
		if code := run(args, &output, &diagnostics); code != 2 || output.Len() != 0 || diagnostics.Len() == 0 {
			t.Fatalf("args=%v exit=%d stdout=%s stderr=%s", args, code, output.String(), diagnostics.String())
		}
	}
	var diagnostics bytes.Buffer
	if code := run([]string{t.TempDir()}, rejectedWriter{}, &diagnostics); code != 1 || diagnostics.Len() == 0 {
		t.Fatalf("output error hidden: exit=%d stderr=%s", code, diagnostics.String())
	}
}

func TestRunSnapshotAndExplicitClockView(t *testing.T) {
	root := t.TempDir()
	recorder, err := performance.Open(root, "run")
	if err != nil {
		t.Fatal(err)
	}
	_, span := performance.Begin(performance.WithRecorder(context.Background(), recorder), "operation")
	span.End(nil)
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	var output, diagnostics bytes.Buffer
	if code := run([]string{"--snapshot", root}, &output, &diagnostics); code != 0 {
		t.Fatalf("snapshot exit=%d %s", code, diagnostics.String())
	}
	var snapshot performance.AnalysisSnapshot
	if err := json.Unmarshal(output.Bytes(), &snapshot); err != nil || snapshot.SchemaVersion != performance.AnalysisSchemaVersion || snapshot.SnapshotID == "" {
		t.Fatal("invalid snapshot", err)
	}
	output.Reset()
	diagnostics.Reset()
	if code := run([]string{"--trace-clock", snapshot.Summary.ClockDomain, root}, &output, &diagnostics); code != 0 {
		t.Fatalf("trace exit=%d %s", code, diagnostics.String())
	}
	var trace performance.Trace
	if err := json.Unmarshal(output.Bytes(), &trace); err != nil || trace.Metadata.SnapshotID != snapshot.SnapshotID || trace.Metadata.View != "single_clock_domain" {
		t.Fatal("snapshot and trace are not the same analysis", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "performance"))
	if err != nil || len(entries) != 3 {
		t.Fatal("analysis wrote original artifacts")
	}
}

func TestRunExportsTraceWithoutChangingInput(t *testing.T) {
	root := t.TempDir()
	recorder, err := performance.Open(root, "run")
	if err != nil {
		t.Fatal(err)
	}
	_, span := performance.Begin(performance.WithRecorder(context.Background(), recorder), "agent.run")
	span.End(errors.New("private error detail"))
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	var output, diagnostics bytes.Buffer
	if code := run([]string{"--trace", root}, &output, &diagnostics); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, diagnostics.String())
	}
	var trace performance.Trace
	if err := json.Unmarshal(output.Bytes(), &trace); err != nil || trace.Metadata.Status != "complete" {
		t.Fatalf("trace=%s err=%v", output.String(), err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "performance"))
	if err != nil || len(entries) != 3 {
		t.Fatalf("input changed: %v %v", entries, err)
	}
	if bytes.Contains(output.Bytes(), []byte("private error detail")) {
		t.Fatal("raw error leaked")
	}
}

type rejectedWriter struct{}

func (rejectedWriter) Write([]byte) (int, error) { return 0, errors.New("output unavailable") }

func TestFrozenSnapshotFeedsBothExportersWithoutRawEvidence(t *testing.T) {
	root := t.TempDir()
	recorder, err := performance.Open(root, "frozen")
	if err != nil {
		t.Fatal(err)
	}
	_, span := performance.Begin(performance.WithRecorder(context.Background(), recorder), "agent.run")
	span.End(nil)
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	var snapshot, diagnostics bytes.Buffer
	if code := run([]string{"--snapshot", root}, &snapshot, &diagnostics); code != 0 {
		t.Fatal(diagnostics.String())
	}
	path := filepath.Join(t.TempDir(), "snapshot.json")
	if err := os.WriteFile(path, snapshot.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	loaded, err := performance.LoadSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"--trace", "--otlp", "--metrics"} {
		var output bytes.Buffer
		diagnostics.Reset()
		if code := run([]string{mode, "--from-snapshot", path}, &output, &diagnostics); code != 0 {
			t.Fatalf("%s exit %d: %s", mode, code, diagnostics.String())
		}
		if !bytes.Contains(output.Bytes(), []byte(loaded.SnapshotID)) {
			t.Fatal("frozen snapshot identity lost")
		}
	}
	loaded.RunID = "tampered"
	changed, _ := json.Marshal(loaded)
	if err := os.WriteFile(path, changed, 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if code := run([]string{"--otlp", "--from-snapshot", path}, &output, &diagnostics); code != 1 || output.Len() != 0 {
		t.Fatal("changed snapshot projected")
	}
}

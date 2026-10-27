package performance

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSharedClockAndCapturedObservation(t *testing.T) {
	root := t.TempDir()
	r, err := Open(root, "clock-run")
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithRecorder(context.Background(), r)
	ctx, span := Begin(ctx, "outer")
	clock := ClockFromContext(ctx)
	at := r.anchor.Add(1234567 * time.Nanosecond)
	if clock.Offset(at) != 1234567 || ClockFromContext(context.Background()) != nil {
		t.Fatal("clock domain lost")
	}
	ObserveAt(WithoutParent(ctx), Event{Operation: "receive", SourceComponent: "gateway"}, at)
	span.End(nil)
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	events := readEvents(t, root)
	if events[1].ObservedOffsetNS != 1234567 || events[1].ParentOperationID != "" || events[1].EventType != "observation" || events[1].ClockDomain != clock.ClockDomain {
		t.Fatalf("observation = %+v", events[1])
	}
	data, err := json.Marshal(clock)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil || len(fields) != 3 {
		t.Fatalf("public clock anchor: %s, %v", data, err)
	}
}

func TestLateProducerInvalidatesFinalizedEvidence(t *testing.T) {
	root := t.TempDir()
	r, err := Open(root, "late-run")
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithRecorder(context.Background(), r)
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(root, "performance", "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); Record(ctx, Event{Operation: "late"}) }()
	}
	wg.Wait()
	if err := r.Close(); err == nil {
		t.Fatal("late observations hidden")
	}
	after, err := os.ReadFile(filepath.Join(root, "performance", "index.json"))
	if err != nil || string(before) != string(after) {
		t.Fatal("original index rewritten")
	}
	var marker struct {
		Status, RunID string
		SchemaVersion int `json:"schema_version"`
	}
	data, err := os.ReadFile(filepath.Join(root, "performance", "late-after-close.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &marker); err != nil || marker.Status != "incomplete" || marker.SchemaVersion != SchemaVersion {
		t.Fatalf("marker=%s err=%v", data, err)
	}
}

func TestWholeMissingPairStillAccountsDrops(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "performance")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(dir, "runner-events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	r := &Recorder{dir: dir, file: file, anchor: time.Now(), now: time.Now, queue: make(chan Event, 1), done: make(chan struct{}), manifest: Manifest{RunID: "run", EmitterID: "emitter"}}
	ctx := WithRecorder(context.Background(), r)
	Record(ctx, Event{EventType: "observation", Operation: "filler"})
	_, span := Begin(ctx, "entirely_missing")
	span.End(nil)
	go r.write()
	if err := r.Close(); err == nil {
		t.Fatal("whole missing pair hidden")
	}
	index := readIndex(t, root)
	if index.Attempted != 3 || index.Written != 1 || index.Dropped != 2 || index.Status != "incomplete" {
		t.Fatalf("index=%+v", index)
	}
}

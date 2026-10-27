package performance

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func readEvents(t *testing.T, root string) []Event {
	t.Helper()
	file, err := os.Open(filepath.Join(root, "performance", "runner-events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var events []Event
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return events
}

func readIndex(t *testing.T, root string) Index {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "performance", "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var index Index
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatal(err)
	}
	return index
}

func TestRecorderPreservesDurationIdentityAndFailureClasses(t *testing.T) {
	root := t.TempDir()
	r, err := Open(root, "run")
	if err != nil {
		t.Fatal(err)
	}
	clock := r.anchor.Add(time.Second)
	r.now = func() time.Time { return clock }
	ctx := WithIdentity(WithRecorder(context.Background(), r), Identity{LogicalTaskID: "logical", OccurrenceID: "occ-1", ArrivalID: "arrival-1", Purpose: "task"})
	ctx, parent := Begin(ctx, "task.e2e")
	for _, cause := range []error{nil, context.Canceled, context.DeadlineExceeded, errors.New("secret provider message")} {
		_, child := Begin(WithIdentity(context.WithoutCancel(ctx), Identity{Purpose: "evaluation"}), "sandbox.start")
		clock = clock.Add(5 * time.Millisecond)
		child.End(cause)
		child.End(cause)
	}
	parent.End(nil)
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	events := readEvents(t, root)
	if len(events) != 10 {
		t.Fatalf("events = %d; want paired operations without duplicate end", len(events))
	}
	statuses := []string{"succeeded", "canceled", "timeout", "failed"}
	for i, status := range statuses {
		event := events[2+2*i]
		if event.Status != status || event.DurationNS == nil || *event.DurationNS != int64(5*time.Millisecond) {
			t.Fatalf("end = %+v", event)
		}
		if event.ParentOperationID != events[0].OperationID || event.Purpose != "evaluation" || event.LogicalTaskID != "logical" || event.ArrivalID != "arrival-1" {
			t.Fatalf("identity/parent = %+v", event)
		}
	}
	for i, event := range events {
		if event.Sequence != uint64(i+1) || event.ClockDomain != event.EmitterID || event.RunID != "run" {
			t.Fatalf("recording metadata = %+v", event)
		}
	}
	data, err := os.ReadFile(filepath.Join(root, "performance", "runner-events.jsonl"))
	if err != nil || strings.Contains(string(data), "secret") {
		t.Fatalf("raw error leaked or read failed: %v", err)
	}
	index := readIndex(t, root)
	if index.Status != "complete" || index.Attempted != 10 || index.Written != 10 || index.Dropped != 0 {
		t.Fatalf("index = %+v", index)
	}
	if index.FlushStatus != "queue_drained_sync_close" || len(index.EventsSHA256) != 64 || index.EventsBytes <= 0 || index.FinalizedAtUTC.IsZero() {
		t.Fatalf("index lacks raw flush evidence: %+v", index)
	}
	if index.ProducerState != "admission_closed" || index.QueueState != "drained" || index.WriterState != "drained" ||
		index.DurabilityState != "events_sync_ok" {
		t.Fatalf("index lacks lifecycle evidence: %+v", index)
	}
	for _, name := range []string{"manifest.json", "runner-events.jsonl", "index.json"} {
		info, err := os.Stat(filepath.Join(root, "performance", name))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("private artifact %s: %v, %v", name, info, err)
		}
	}
	if _, err := Open(root, "run"); err == nil {
		t.Fatal("existing artifacts overwritten")
	}
}

func TestRecorderConcurrentProducersHaveUniqueOrderedSequence(t *testing.T) {
	root := t.TempDir()
	r, err := Open(root, "run")
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithRecorder(context.Background(), r)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, span := Begin(ctx, "sandbox.start")
			span.End(nil)
		}()
	}
	wg.Wait()
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	events := readEvents(t, root)
	if len(events) != 200 {
		t.Fatalf("events = %d", len(events))
	}
	starts := make(map[string]Event)
	for i, event := range events {
		if event.Sequence != uint64(i+1) {
			t.Fatalf("sequence = %d", event.Sequence)
		}
		if event.EventType == "operation_started" {
			if _, exists := starts[event.OperationID]; exists {
				t.Fatal("duplicate operation identity")
			}
			starts[event.OperationID] = event
		} else if event.DurationNS == nil || *event.DurationNS < 0 || starts[event.OperationID].OperationID == "" {
			t.Fatalf("unmatched/invalid terminal = %+v", event)
		}
	}
}

func TestRecorderReportsWriteFailure(t *testing.T) {
	root := t.TempDir()
	r, err := Open(root, "run")
	if err != nil {
		t.Fatal(err)
	}
	if err := r.file.Close(); err != nil {
		t.Fatal(err)
	}
	_, span := Begin(WithRecorder(context.Background(), r), "sandbox.start")
	span.End(nil)
	if err := r.Close(); err == nil {
		t.Fatal("write failure hidden")
	}
	index := readIndex(t, root)
	if index.Status != "incomplete" || index.WriteFailed != 2 || index.Written != 0 || index.Attempted != 2 {
		t.Fatalf("index = %+v", index)
	}
}

func TestRecorderIndexPublicationFailureIsNotClaimedComplete(t *testing.T) {
	root := t.TempDir()
	r, err := Open(root, "run")
	if err != nil {
		t.Fatal(err)
	}
	Record(WithRecorder(context.Background(), r), Event{EventType: "observation", Operation: "task"})
	indexPath := filepath.Join(root, "performance", "index.json")
	if err := os.WriteFile(indexPath, []byte("existing-index"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err == nil {
		t.Fatal("index publication failure hidden")
	}
	data, err := os.ReadFile(indexPath)
	if err != nil || string(data) != "existing-index" {
		t.Fatalf("existing index was overwritten: %q, %v", data, err)
	}
}

func TestRecorderOverflowIsVisibleWithoutBlockingProducer(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "performance")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(dir, "runner-events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	// Hold the consumer until a bounded queue fills, independent of disk speed.
	r := &Recorder{dir: dir, file: file, anchor: time.Now(), now: time.Now,
		queue: make(chan Event, 1), done: make(chan struct{}), manifest: Manifest{RunID: "run", EmitterID: "emitter"}}
	_, span := Begin(WithRecorder(context.Background(), r), "sandbox.start")
	span.End(nil)
	go r.write()
	if err := r.Close(); err == nil || !strings.Contains(err.Error(), "dropped 1") {
		t.Fatalf("overflow error = %v", err)
	}
	index := readIndex(t, root)
	if index.Status != "incomplete" || index.Attempted != 2 || index.Written != 1 || index.Dropped != 1 {
		t.Fatalf("index = %+v", index)
	}
}

func TestRecorderAbsentIsNoop(t *testing.T) {
	ctx := context.Background()
	got, span := Begin(ctx, "unused")
	if got != ctx || span != nil {
		t.Fatal("absent recorder changed context")
	}
	span.End(errors.New("ignored"))
	Record(ctx, Event{EventType: "unused"})
}

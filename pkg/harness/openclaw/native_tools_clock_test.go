package openclaw

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/harness"
	"github.com/hyscale-lab/aries/pkg/harness/openclaw/gateway"
	"github.com/hyscale-lab/aries/pkg/performance"
)

func TestNativeToolClockSharesControllerAnchorWithoutRewritingNativeTime(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		name := "without-recorder"
		if enabled {
			name = "with-recorder"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			if enabled {
				recorder, err := performance.Open(t.TempDir(), "native-clock-test")
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := recorder.Close(); err != nil {
						t.Error(err)
					}
				})
				ctx = performance.WithRecorder(ctx, recorder)
			}
			clock := performance.ClockFromContext(ctx)
			credentials := harness.NewCredentials("native-clock-test")
			defer credentials.Clear()
			collector := NewNativeToolCollector("session-1", credentials)
			input := make(chan gateway.Frame, 4)
			input <- nativeEvent("session-1", "run-1", "tool", 1, toolData("start"))
			input <- nativeEvent("session-1", "run-1", "tool", 2, toolData("result"))
			command := toolData("end")
			command["durationMs"], command["exitCode"] = float64(17), float64(0)
			input <- nativeEvent("session-1", "run-1", "command_output", 3, command)
			input <- nativeEvent("session-1", "run-1", "lifecycle", 4, map[string]any{"phase": "end"})
			collector.StartReading(ctx, scriptedNativeEvents{frames: input})
			directory := t.TempDir()
			if _, err := collector.Finish("run-1", directory, time.Second); err != nil {
				t.Fatal(err)
			}
			metadata, err := os.ReadFile(filepath.Join(directory, "native-tool-events.index.json"))
			if err != nil {
				t.Fatal(err)
			}
			var index nativeToolIndex
			if err := json.Unmarshal(metadata, &index); err != nil {
				t.Fatal(err)
			}
			if !index.Complete || index.Events != 3 || !index.HostOriginAt.Equal(collector.start) {
				t.Fatalf("collector origin or coverage changed: %+v", index)
			}
			if enabled {
				if index.Clock == nil || index.HostOriginOffsetNS == nil || index.Clock.EmitterID != clock.EmitterID || index.Clock.ClockDomain != clock.ClockDomain || !index.Clock.StartedAtUTC.Equal(clock.StartedAtUTC) || *index.HostOriginOffsetNS != clock.Offset(collector.start) {
					t.Fatalf("collector was not anchored to the recorder's monotonic clock: %+v", index)
				}
			} else if index.Clock != nil || index.HostOriginOffsetNS != nil || bytes.Contains(metadata, []byte(`"clock"`)) || bytes.Contains(metadata, []byte(`"host_origin_offset_ns"`)) {
				t.Fatal("collector invented a shared clock without a recorder")
			}
			content, err := os.ReadFile(filepath.Join(directory, "native-tool-events.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			lines := bytes.Split(bytes.TrimSpace(content), []byte{'\n'})
			if len(lines) != len(collector.records) {
				t.Fatal("native events missing")
			}
			for i, line := range lines {
				var event nativeToolEvent
				if err := json.Unmarshal(line, &event); err != nil {
					t.Fatal(err)
				}
				original := collector.records[i]
				// The shared origin is additive metadata. Existing collector offsets
				// and guest wall timestamps remain original evidence, not rebased UTC.
				if event.ObservedOffsetNS != original.ObservedOffsetNS || !event.ObservedAt.Equal(original.ObservedAt) || event.NativeTimestampMS == nil || *event.NativeTimestampMS != float64(1700000000001+i) {
					t.Fatalf("native clock evidence was rewritten: %+v", event)
				}
				if i == 2 && (event.NativeDurationMS == nil || *event.NativeDurationMS != 17) {
					t.Fatal("guest duration was replaced by controller timing")
				}
			}
		})
	}
}

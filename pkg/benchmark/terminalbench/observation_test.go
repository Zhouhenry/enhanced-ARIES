package terminalbench

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/performance"
)

func TestVerifierObservationPreservesScoreAndTransportFailure(t *testing.T) {
	for _, kind := range []string{"pass", "failed_answer", "transport_error"} {
		t.Run(kind, func(t *testing.T) {
			root := writeFixture(t)
			b, err := New(testOptions(root, []string{fixGitID}, t.TempDir()))
			if err != nil {
				t.Fatal(err)
			}
			tasks, err := b.Tasks(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			report, reward := validFixGitCTRF, "1\n"
			if kind == "failed_answer" {
				report, reward = validRewardZeroCTRF, "0\n"
			}
			sandbox := &fakeSandbox{verifierOutputs: map[string][]byte{
				filepath.Join(verifierLogPath, "ctrf.json"):  []byte(report),
				filepath.Join(verifierLogPath, "reward.txt"): []byte(reward),
			}}
			if kind == "transport_error" {
				sandbox.commandErr = errors.New("transport lost")
			}
			out := t.TempDir()
			recorder, err := performance.Open(out, "terminalbench")
			if err != nil {
				t.Fatal(err)
			}
			evaluation, evalErr := b.Evaluate(performance.WithRecorder(context.Background(), recorder), tasks[0], sandbox, nil)
			if err := recorder.Close(); err != nil {
				t.Fatal(err)
			}
			if (evalErr != nil) != (kind == "transport_error") {
				t.Fatalf("evaluation error: %v", evalErr)
			}
			if (evaluation.Status == core.StatusSucceeded) != (kind == "pass") {
				t.Fatalf("outcome changed: %+v", evaluation)
			}
			if evaluation.Artifact != nil {
				t.Fatal("verifier reports are not candidate capture evidence")
			}
			file, err := os.Open(filepath.Join(out, "performance", "runner-events.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			starts, ends := map[string]performance.Event{}, map[string]performance.Event{}
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				var event performance.Event
				if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
					t.Fatal(err)
				}
				if event.EventType == "operation_started" {
					starts[event.Operation] = event
				} else {
					ends[event.Operation] = event
				}
			}
			if err := scanner.Err(); err != nil {
				t.Fatal(err)
			}
			if len(starts) != 3 || len(ends) != 3 {
				t.Fatalf("phase coverage: %v / %v", starts, ends)
			}
			for _, name := range []string{"verifier.prepare", "verifier.exec", "verifier.report"} {
				start, end := starts[name], ends[name]
				if start.OperationID == "" || start.OperationID != end.OperationID || end.DurationNS == nil || *end.DurationNS < 0 {
					t.Fatalf("unpaired phase: %s", name)
				}
				want := "succeeded"
				if kind == "transport_error" && name != "verifier.prepare" {
					want = "failed"
				}
				if end.Status != want {
					t.Fatalf("%s status=%s, want %s", name, end.Status, want)
				}
			}
		})
	}
}

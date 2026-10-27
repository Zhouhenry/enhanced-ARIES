package codingcase

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

func TestEvaluationObservationPreservesFailureAndRealPhaseBoundaries(t *testing.T) {
	for _, kind := range []string{"success", "candidate_missing", "fresh_failed", "exec_failed"} {
		t.Run(kind, func(t *testing.T) {
			b, task := benchmarkTask(t)
			agent, verifier := candidate(t), newFakeSandbox()
			verifier.files[verifierDir+"/report.xml"] = junit(0)
			factory := &freshFactory{sandbox: verifier}
			failure := errors.New("private transport diagnostic")
			switch kind {
			case "candidate_missing":
				delete(agent.files, Workdir+"/intervals.py")
			case "fresh_failed":
				factory.err = failure
			case "exec_failed":
				verifier.execErr = failure
			}
			root := t.TempDir()
			r, err := performance.Open(root, "verification")
			if err != nil {
				t.Fatal(err)
			}
			evaluation, evalErr := b.Evaluate(performance.WithRecorder(context.Background(), r), task, agent, factory)
			if err := r.Close(); err != nil {
				t.Fatal(err)
			}
			if (kind == "success") != (evalErr == nil && evaluation.Status == core.StatusSucceeded) {
				t.Fatalf("semantic outcome changed: %+v %v", evaluation, evalErr)
			}
			file, err := os.Open(filepath.Join(root, "performance", "runner-events.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			starts, ends := map[string]performance.Event{}, map[string]performance.Event{}
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				var e performance.Event
				if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
					t.Fatal(err)
				}
				if e.EventType == "operation_started" {
					starts[e.Operation] = e
				} else {
					ends[e.Operation] = e
				}
			}
			if err := scanner.Err(); err != nil {
				t.Fatal(err)
			}
			want := 4
			if kind == "candidate_missing" || kind == "fresh_failed" {
				want = 1
			}
			if len(starts) != want || len(ends) != want {
				t.Fatalf("invented or missing phases: %v %v", starts, ends)
			}
			for name, end := range ends {
				if end.OperationID != starts[name].OperationID || end.DurationNS == nil || *end.DurationNS < 0 {
					t.Fatal("invalid phase pairing")
				}
			}
			if kind == "candidate_missing" && ends["candidate.capture"].Status != "failed" {
				t.Fatal("candidate failure hidden")
			}
			if kind == "fresh_failed" && ends["candidate.capture"].Status != "succeeded" {
				t.Fatal("later failure changed earlier candidate fact")
			}
			if kind == "exec_failed" && ends["verifier.exec"].Status != "failed" {
				t.Fatal("verifier process failure hidden")
			}
		})
	}
}

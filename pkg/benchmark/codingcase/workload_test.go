package codingcase

import (
	"context"
	"reflect"
	"testing"

	"github.com/hyscale-lab/aries/pkg/core"
)

func TestWorkloadSelectionPreservesPublicInputsAndOccurrence(t *testing.T) {
	for _, tc := range []struct {
		name, workload, taskID, instruction string
	}{
		{"default", "", WorkloadLegacy, instruction},
		{"legacy", WorkloadLegacy, WorkloadLegacy, instruction},
		{"portable", WorkloadPortable, WorkloadPortable, portableInstruction},
		{"multicall", WorkloadMultiCall, WorkloadMultiCall, multiCallInstruction},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := New(Options{Workload: tc.workload, Template: "explicit-template", OutputDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			tasks, err := b.Tasks(context.Background())
			if err != nil || len(tasks) != 1 {
				t.Fatalf("tasks: %v %v", tasks, err)
			}
			if tasks[0].ID != tc.taskID || tasks[0].Instruction != tc.instruction {
				t.Fatal("workload selected the wrong task ID or instructions")
			}
			occurrence, err := b.ForTask(tc.taskID, "occurrence-1")
			if err != nil {
				t.Fatal(err)
			}
			tasks, err = occurrence.Tasks(context.Background())
			if err != nil || len(tasks) != 1 || tasks[0].ID != "occurrence-1" || tasks[0].Instruction != tc.instruction {
				t.Fatalf("occurrence lost workload selection: %v %v", tasks, err)
			}
			sandbox := newFakeSandbox()
			if err := occurrence.PrepareSandbox(context.Background(), tasks[0], sandbox); err != nil {
				t.Fatal(err)
			}
			want := map[string][]byte{
				Workdir + "/intervals.py":   []byte(buggySource),
				Workdir + "/test_public.py": []byte(publicTests),
				Workdir + "/README.md":      []byte(tasks[0].Instruction),
			}
			if !reflect.DeepEqual(sandbox.uploaded, want) {
				t.Fatal("workload changed the shared source/tests or README disagrees with task instructions")
			}
		})
	}
	if instruction == portableInstruction {
		t.Fatal("portable workload must have distinct instructions")
	}
	if _, err := New(Options{Workload: "merge-ranges-portable-v2", Template: "explicit-template", OutputDir: t.TempDir()}); err == nil {
		t.Fatal("unknown workload was silently accepted")
	}
}

func TestWorkloadRejectsOtherVersionBeforeSandboxAccess(t *testing.T) {
	for _, workload := range []string{WorkloadLegacy, WorkloadPortable, WorkloadMultiCall} {
		t.Run(workload, func(t *testing.T) {
			b, err := New(Options{Workload: workload, TaskID: "same-task", Template: "explicit-template", OutputDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			tasks, err := b.Tasks(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			task := tasks[0]
			if workload == WorkloadLegacy {
				task.Instruction = portableInstruction
			} else {
				task.Instruction = instruction
			}
			sandbox := newFakeSandbox()
			if err := b.PrepareSandbox(context.Background(), task, sandbox); err == nil {
				t.Fatal("prepare accepted a task from the other workload")
			}
			factory := &freshFactory{sandbox: newFakeSandbox()}
			if _, err := b.Evaluate(context.Background(), task, sandbox, factory); err == nil {
				t.Fatal("evaluate accepted a task from the other workload")
			}
			if len(sandbox.commands) != 0 || len(sandbox.uploaded) != 0 || len(sandbox.downloadLimits) != 0 || factory.calls != 0 {
				t.Fatal("wrong workload accessed a sandbox before rejection")
			}
		})
	}
}

func TestWorkloadsUseIdenticalIndependentVerifier(t *testing.T) {
	for _, failures := range []int{0, 1} {
		name := "pass"
		if failures != 0 {
			name = "fail"
		}
		t.Run(name, func(t *testing.T) {
			var previous *fakeSandbox
			for _, workload := range []string{WorkloadLegacy, WorkloadPortable, WorkloadMultiCall} {
				b, err := New(Options{Workload: workload, Template: "explicit-template", OutputDir: t.TempDir()})
				if err != nil {
					t.Fatal(err)
				}
				tasks, err := b.Tasks(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				verifier := newFakeSandbox()
				verifier.exitCode = failures
				verifier.files[verifierDir+"/report.xml"] = junit(failures)
				factory := &freshFactory{sandbox: verifier}
				evaluation, err := b.Evaluate(context.Background(), tasks[0], candidate(t), factory)
				if err != nil || factory.calls != 1 || factory.environment.AllowNetwork {
					t.Fatalf("%s fresh verifier: %+v %v", workload, evaluation, err)
				}
				if failures == 0 && (evaluation.Status != core.StatusSucceeded || evaluation.Score != 1) {
					t.Fatalf("%s rejected complete passing report: %+v", workload, evaluation)
				}
				if failures != 0 && (evaluation.Status != core.StatusFailed || evaluation.Score != 0) {
					t.Fatalf("%s accepted failed report: %+v", workload, evaluation)
				}
				if previous != nil && (!reflect.DeepEqual(previous.commands, verifier.commands) || !reflect.DeepEqual(previous.uploaded, verifier.uploaded) || !reflect.DeepEqual(previous.downloadLimits, verifier.downloadLimits)) {
					t.Fatal("workload changed independent verifier commands, private inputs, or download bounds")
				}
				previous = verifier
			}
		})
	}
}

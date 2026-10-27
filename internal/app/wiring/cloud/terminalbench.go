package cloud

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"

	"github.com/hyscale-lab/aries/pkg/benchmark/terminalbench"
	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/sandbox/e2b"
)

type terminalBenchVariantEvidence struct {
	ID                         string `json:"id"`
	InstructionAppendixFile    string `json:"instruction_appendix_file"`
	OriginalInstructionSHA256  string `json:"original_instruction_sha256"`
	AppendixSHA256             string `json:"appendix_sha256"`
	EffectiveInstructionSHA256 string `json:"effective_instruction_sha256"`
}

// prepareTerminalBench keeps the original task and private evaluator intact.
// The separate binding describes the adapted cloud environment, not a new task.
func (p *preparedRun) prepareTerminalBench(ctx context.Context) error {
	selection := p.config.Benchmark
	if selection == nil {
		return nil
	}
	if err := selection.ValidateVariant(); err != nil {
		return err
	}
	if selection.Type != "terminalbench2" || p.config.Placement != "external" ||
		(p.config.SandboxBackend != "e2b-cloud" && p.config.SandboxBackend != "huawei-agentsphere") ||
		p.config.Execution.Attempts != 1 || p.config.Execution.Concurrency != 1 || p.config.Workload != "" {
		return errors.New("TerminalBench cloud requires one external E2B or Huawei task and no codingcase workload")
	}
	catalog, err := terminalbench.New(terminalbench.Options{Root: selection.Root, TaskIDs: []string{selection.TaskID},
		OutputDir: p.outputRoot, Revision: p.config.Versions.TerminalBench2.Revision})
	if err != nil {
		return err
	}
	tasks, err := catalog.Tasks(ctx)
	if err != nil {
		return fmt.Errorf("load original TerminalBench task: %w", err)
	}
	if len(tasks) != 1 || tasks[0].ID != selection.TaskID {
		return errors.New("TerminalBench catalog did not return the selected task")
	}
	binding, err := e2b.LoadTaskTemplateBinding(p.config.Sandbox.BindingFile)
	if err != nil {
		return err
	}
	if err := binding.ValidateProvider(p.config.SandboxBackend); err != nil {
		return err
	}
	template := p.config.Sandbox.Template
	if template == "" {
		template = binding.Template
	}
	if err := binding.Validate(selection.TaskID, p.config.Versions.TerminalBench2.Revision, template, tasks[0].Environment); err != nil {
		return err
	}
	if time.Duration(p.config.Sandbox.TimeoutSeconds)*time.Second <= tasks[0].Timeout {
		return errors.New("TerminalBench sandbox TTL must exceed the original agent timeout and include verifier/cleanup time")
	}
	var variantEvidence *terminalBenchVariantEvidence
	var effectiveTask *core.Task
	if variant := selection.Variant; variant != nil {
		appendix, err := os.ReadFile(variant.InstructionAppendixFile)
		if err != nil {
			return fmt.Errorf("read TerminalBench instruction appendix: %w", err)
		}
		appendixSHA := fmt.Sprintf("%x", sha256.Sum256(appendix))
		if appendixSHA != variant.AppendixSHA256 {
			return errors.New("TerminalBench instruction appendix SHA-256 mismatch")
		}
		if !utf8.Valid(appendix) {
			return errors.New("TerminalBench instruction appendix must be UTF-8 text")
		}
		catalog, err = catalog.WithInstructionAppendix(selection.TaskID, variant.ID, string(appendix))
		if err != nil {
			return err
		}
		effective, err := catalog.Tasks(ctx)
		if err != nil {
			return err
		}
		effectiveTask = &effective[0]
		variantEvidence = &terminalBenchVariantEvidence{
			ID: variant.ID, InstructionAppendixFile: variant.InstructionAppendixFile,
			OriginalInstructionSHA256:  fmt.Sprintf("%x", sha256.Sum256([]byte(tasks[0].Instruction))),
			AppendixSHA256:             appendixSHA,
			EffectiveInstructionSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(effectiveTask.Instruction))),
		}
	}
	record := struct {
		Task             core.Task                     `json:"original_task"`
		EffectiveTask    *core.Task                    `json:"effective_task,omitempty"`
		Variant          *terminalBenchVariantEvidence `json:"variant,omitempty"`
		Binding          *e2b.TaskTemplateBinding      `json:"binding"`
		ResourceEvidence string                        `json:"resource_evidence"`
		EvaluationMethod string                        `json:"evaluation_method"`
	}{Task: tasks[0], EffectiveTask: effectiveTask, Variant: variantEvidence, Binding: binding,
		ResourceEvidence: "template_build_configuration_only; actual resource readback is separate", EvaluationMethod: "original_sandbox_after_confirmed_stop_and_revoke"}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(p.outputRoot, "task-environment.json"), append(data, '\n'), 0600); err != nil {
		return err
	}
	p.catalog, p.binding, p.taskTimeout = catalog, binding, tasks[0].Timeout
	p.config.Sandbox.Template = template
	return nil
}

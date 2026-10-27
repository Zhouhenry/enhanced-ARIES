package cloud

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/internal/app"
	"github.com/hyscale-lab/aries/pkg/config"
	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/sandbox/e2b"
)

func terminalCloudFixture(t *testing.T, provider string) (*preparedRun, core.Environment) {
	t.Helper()
	dataset := t.TempDir()
	files := map[string]string{
		"tasks/fix-git/instruction.md": "Original task instruction.\n",
		"tasks/fix-git/tests/test.sh":  "#!/bin/bash\nprintf 'private verifier'\n",
		"tasks/fix-git/task.toml": `schema_version = "1.1"
[task]
name = "terminal-bench/fix-git"
[agent]
timeout_sec = 900
[verifier]
timeout_sec = 800
[environment]
build_timeout_sec = 600
docker_image = "example.invalid/fix-git:source"
cpus = 1
memory_mb = 2048
storage_mb = 10240
gpus = 0
allow_internet = true
[environment.env]
ORIGINAL = "task-value"
`,
	}
	for name, body := range files {
		path := filepath.Join(dataset, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git := func(args ...string) string {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", dataset}, args...)...)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("fixture git: %v: %s", err, output)
		}
		return strings.TrimSpace(string(output))
	}
	git("init", "--quiet")
	git("add", ".")
	git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "fixture")
	revision := git("rev-parse", "HEAD")
	environment := core.Environment{Image: "example.invalid/fix-git:source", CPU: 1, MemoryMB: 2048, StorageMB: 10240, AllowNetwork: true, Env: map[string]string{"ORIGINAL": "task-value"}}
	binding := e2b.TaskTemplateBinding{SchemaVersion: 1, TaskID: "fix-git", BenchmarkRevision: revision, RequestedEnvironment: environment,
		SourceImageDigest: "sha256:" + strings.Repeat("a", 64), Template: "task-template:11111111-1111-4111-8111-111111111111", Workdir: "/app/personal-site",
		TemplateConfig: e2b.TemplateResources{CPUCount: 2, MemoryMB: 2048}, CommandEnv: map[string]string{"PATH": "/usr/local/bin:/usr/bin:/bin:/app/bin"},
		InitialDirectoryMtimeUnixNS: map[string]int64{"/": 1761350400000000000},
		Adaptations:                 []string{"Template CPU is 2 instead of requested 1; disk quota not established; UID60000 and declared PATH adaptation"}, BuildEvidenceSHA256: strings.Repeat("b", 64)}
	if provider == "huawei-agentsphere" {
		binding.Provider = provider
	}
	data, err := json.Marshal(binding)
	if err != nil {
		t.Fatal(err)
	}
	bindingPath := filepath.Join(t.TempDir(), "binding.json")
	if err := os.WriteFile(bindingPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	cfg := cloudConfig()
	cfg.SandboxBackend = provider
	cfg.Placement = "external"
	cfg.Benchmark = &config.CloudBenchmarkConfig{Type: "terminalbench2", Root: dataset, TaskID: "fix-git"}
	cfg.Execution = config.CloudExecutionConfig{Concurrency: 1, Attempts: 1}
	cfg.Sandbox.Template, cfg.Sandbox.BindingFile, cfg.Sandbox.TimeoutSeconds = "", bindingPath, 1900
	cfg.Versions.TerminalBench2.Revision = revision
	cfg.Versions.OpenClaw.Image = "ghcr.io/openclaw/openclaw:2026.7.1"
	cfg.Harness.BridgeHost = "127.0.0.1"
	cfg.Harness.SSHClientPath = filepath.Join(t.TempDir(), "unused-client")
	cfg.Harness.DockerSocket = "unix://" + filepath.Join(t.TempDir(), "unused-docker.sock")
	return &preparedRun{config: cfg, runID: "run", outputRoot: t.TempDir(), token: strings.Repeat("t", 32), attempts: make(map[string]*attempt)}, environment
}

func TestTerminalBenchVariantKeepsOriginalBindingAndSeparateIdentity(t *testing.T) {
	for _, provider := range []string{"e2b-cloud", "huawei-agentsphere"} {
		t.Run(provider, func(t *testing.T) {
			p, originalEnvironment := terminalCloudFixture(t, provider)
			const variantID = "fix-git-terminal-result-v1"
			const appendix = "Public observation appendix.\n"
			appendixPath := filepath.Join(t.TempDir(), "appendix.md")
			if err := os.WriteFile(appendixPath, []byte(appendix), 0600); err != nil {
				t.Fatal(err)
			}
			p.config.Benchmark.Variant = &config.CloudBenchmarkVariantConfig{ID: variantID, InstructionAppendixFile: appendixPath,
				AppendixSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(appendix)))}
			if err := p.prepareTerminalBench(context.Background()); err != nil {
				t.Fatal(err)
			}
			if p.binding.TaskID != "fix-git" || p.config.Benchmark.TaskID != "fix-git" || p.taskTimeout != 15*time.Minute {
				t.Fatal("variant changed original binding, source task, or budget")
			}
			data, err := os.ReadFile(filepath.Join(p.outputRoot, "task-environment.json"))
			if err != nil {
				t.Fatal(err)
			}
			var record struct {
				Original  core.Task                    `json:"original_task"`
				Effective core.Task                    `json:"effective_task"`
				Variant   terminalBenchVariantEvidence `json:"variant"`
			}
			if err := json.Unmarshal(data, &record); err != nil {
				t.Fatal(err)
			}
			wantInstruction := "Original task instruction.\n\n" + appendix
			if record.Original.ID != "fix-git" || record.Original.Instruction != "Original task instruction." || record.Effective.ID != variantID || record.Effective.Instruction != wantInstruction || !reflect.DeepEqual(record.Effective.Environment, originalEnvironment) {
				t.Fatal("original/effective task or exact appendix bytes lost")
			}
			if record.Variant.ID != variantID || record.Variant.AppendixSHA256 != p.config.Benchmark.Variant.AppendixSHA256 ||
				record.Variant.OriginalInstructionSHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(record.Original.Instruction))) ||
				record.Variant.EffectiveInstructionSHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(wantInstruction))) || strings.Contains(string(data), "private verifier") {
				t.Fatal("instruction evidence is incomplete or exposes the private verifier")
			}
			request := app.CloudAttempt{RunID: "run", LogicalID: "fix-git", OccurrenceID: "fix-git-001", OutputDir: p.outputRoot,
				Model: core.ModelConfig{Provider: "openai", BaseURL: "https://proxy.example/v1", Model: "model", APIKeyEnv: proxyTokenEnv}, AccessToken: []byte("scoped-token")}
			if _, err := p.newAttempt(context.Background(), request); err == nil {
				t.Fatal("variant admitted under the original logical ID")
			}
			request.LogicalID, request.OccurrenceID = variantID, variantID+"-001"
			experiment, err := p.newAttempt(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			if err := experiment.Close(); err != nil {
				t.Fatal(err)
			}
			if err := p.close(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTerminalBenchVariantRejectsInvalidAppendixBeforeRuntime(t *testing.T) {
	for _, failure := range []string{"missing", "hash", "empty", "invalid_utf8", "dirty_checkout", "binding_task"} {
		t.Run(failure, func(t *testing.T) {
			p, _ := terminalCloudFixture(t, "e2b-cloud")
			body := []byte("Public instruction.\n")
			if failure == "empty" {
				body = []byte(" \n")
			} else if failure == "invalid_utf8" {
				body = []byte{0xff}
			}
			appendixPath := filepath.Join(t.TempDir(), "appendix.md")
			if failure != "missing" {
				if err := os.WriteFile(appendixPath, body, 0600); err != nil {
					t.Fatal(err)
				}
			}
			variant := &config.CloudBenchmarkVariantConfig{ID: "fix-git-variant-v1", InstructionAppendixFile: appendixPath, AppendixSHA256: fmt.Sprintf("%x", sha256.Sum256(body))}
			p.config.Benchmark.Variant = variant
			switch failure {
			case "hash":
				variant.AppendixSHA256 = strings.Repeat("a", 64)
			case "dirty_checkout":
				if err := os.WriteFile(filepath.Join(p.config.Benchmark.Root, "tasks/fix-git/instruction.md"), body, 0600); err != nil {
					t.Fatal(err)
				}
			case "binding_task":
				binding, err := e2b.LoadTaskTemplateBinding(p.config.Sandbox.BindingFile)
				if err != nil {
					t.Fatal(err)
				}
				binding.TaskID = variant.ID
				data, err := json.Marshal(binding)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p.config.Sandbox.BindingFile, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := p.prepareTerminalBench(context.Background()); err == nil {
				t.Fatal("invalid variant preparation accepted")
			}
			if p.catalog != nil || p.binding != nil || len(p.attempts) != 0 {
				t.Fatal("invalid appendix published runtime owners")
			}
		})
	}
}

func TestTerminalBenchCloudPreservesOriginalTaskAndRecordsAdaptation(t *testing.T) {
	for _, provider := range []string{"e2b-cloud", "huawei-agentsphere"} {
		t.Run(provider, func(t *testing.T) {
			testTerminalBenchCloudPreservesOriginalTaskAndRecordsAdaptation(t, provider)
		})
	}
}

func testTerminalBenchCloudPreservesOriginalTaskAndRecordsAdaptation(t *testing.T, provider string) {
	t.Helper()
	p, original := terminalCloudFixture(t, provider)
	if err := p.prepareTerminalBench(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p.taskTimeout != 15*time.Minute || p.config.Sandbox.Template != p.binding.Template {
		t.Fatal("lost original task timeout or selected template")
	}
	benchmark, err := p.catalog.ForTask("fix-git", "fix-git-001")
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := benchmark.Tasks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].ID != "fix-git-001" || tasks[0].Instruction != "Original task instruction." || tasks[0].Timeout != 15*time.Minute || !reflect.DeepEqual(tasks[0].Environment, original) {
		t.Fatalf("cloud wiring rewrote the original task: %+v", tasks)
	}
	data, err := os.ReadFile(filepath.Join(p.outputRoot, "task-environment.json"))
	if err != nil {
		t.Fatal(err)
	}
	var record struct {
		Task             core.Task               `json:"original_task"`
		Binding          e2b.TaskTemplateBinding `json:"binding"`
		ResourceEvidence string                  `json:"resource_evidence"`
	}
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	if record.Task.ID != "fix-git" || !reflect.DeepEqual(record.Task.Environment, original) || record.Binding.TemplateConfig.CPUCount != 2 || record.Binding.TemplateConfig.StorageMB != nil || !strings.Contains(record.ResourceEvidence, "readback is separate") {
		t.Fatal("lost requested/configured/unknown resource distinction")
	}
	if err := record.Binding.ValidateProvider(provider); err != nil {
		t.Fatal(err)
	}
	if p.config.Sandbox.AssumeNetworkIsolation || !record.Task.Environment.AllowNetwork {
		t.Fatal("original task network policy was changed")
	}
	if !reflect.DeepEqual(record.Binding.InitialDirectoryMtimeUnixNS, p.binding.InitialDirectoryMtimeUnixNS) || record.Binding.InitialDirectoryMtimeUnixNS["/"] != 1761350400000000000 {
		t.Fatal("lost declared initial directory mtime adaptation")
	}
	if bytes := string(data); strings.Contains(bytes, "private verifier") {
		t.Fatal("private verifier leaked into environment artifact")
	}
	request := app.CloudAttempt{RunID: "run", LogicalID: "wrong-task", OccurrenceID: "wrong-task-001", OutputDir: p.outputRoot,
		Model: core.ModelConfig{Provider: "openai", BaseURL: "https://proxy.example/v1", Model: "model", APIKeyEnv: proxyTokenEnv}, AccessToken: []byte("scoped-token")}
	if _, err := p.newAttempt(context.Background(), request); err == nil {
		t.Fatal("unselected task admitted")
	}
	request.LogicalID, request.OccurrenceID = "fix-git", "fix-git-001"
	experiment, err := p.newAttempt(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := experiment.Close(); err != nil {
		t.Fatal(err)
	}
	if err := p.close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestTerminalBenchCloudRejectsStaleBindingBeforeRuntimeConstruction(t *testing.T) {
	for _, failure := range []string{"source_revision", "template", "task", "resources", "ttl", "dirty_checkout", "legacy_provider", "wrong_provider", "unsupported_provider"} {
		t.Run(failure, func(t *testing.T) {
			p, _ := terminalCloudFixture(t, "e2b-cloud")
			switch failure {
			case "source_revision":
				p.config.Versions.TerminalBench2.Revision = strings.Repeat("a", 40)
			case "template":
				p.config.Sandbox.Template = "different:22222222-2222-4222-8222-222222222222"
			case "task":
				p.config.Benchmark.TaskID = "different"
			case "resources", "wrong_provider":
				binding, err := e2b.LoadTaskTemplateBinding(p.config.Sandbox.BindingFile)
				if err != nil {
					t.Fatal(err)
				}
				if failure == "resources" {
					binding.RequestedEnvironment.CPU = 4
				} else {
					binding.Provider = "huawei-agentsphere"
				}
				data, err := json.Marshal(binding)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p.config.Sandbox.BindingFile, data, 0600); err != nil {
					t.Fatal(err)
				}
			case "ttl":
				p.config.Sandbox.TimeoutSeconds = 900
			case "dirty_checkout":
				if err := os.WriteFile(filepath.Join(p.config.Benchmark.Root, "tasks/fix-git/instruction.md"), []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			case "legacy_provider":
				p.config.SandboxBackend = "huawei-agentsphere"
			case "unsupported_provider":
				p.config.SandboxBackend = "other-cloud"
			}
			if err := p.prepareTerminalBench(context.Background()); err == nil {
				t.Fatal("stale or incompatible environment accepted")
			}
			if p.catalog != nil || p.binding != nil || len(p.attempts) != 0 {
				t.Fatal("failed preparation published runtime owners")
			}
		})
	}
}

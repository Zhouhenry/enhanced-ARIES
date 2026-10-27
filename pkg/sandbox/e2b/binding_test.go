package e2b

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hyscale-lab/aries/pkg/core"
)

func testTaskTemplateBinding() *TaskTemplateBinding {
	return &TaskTemplateBinding{
		SchemaVersion: 1, TaskID: "fix-git", BenchmarkRevision: strings.Repeat("a", 40),
		RequestedEnvironment: core.Environment{Image: "task-image:pinned", CPU: 1, MemoryMB: 1024, StorageMB: 10240,
			AllowNetwork: true, Env: map[string]string{"TASK_ONLY": "task", "OVERRIDE": "task"}},
		SourceImageDigest: "sha256:" + strings.Repeat("b", 64), Template: "task-template:ea001e75-54b9-407b-890c-f20c96e036c2", Workdir: "/app",
		TemplateConfig:      TemplateResources{CPUCount: 2, MemoryMB: 2048},
		CommandEnv:          map[string]string{"OVERRIDE": "template", "ADAPTED": "template"},
		Adaptations:         []string{"Template has 2 CPUs and 2048 MiB; storage is unverified; image workdir is /app; command environment is explicit."},
		BuildEvidenceSHA256: strings.Repeat("c", 64),
	}
}

func TestTaskTemplateBindingPreservesOriginalEnvironment(t *testing.T) {
	binding := testTaskTemplateBinding()
	env := binding.clone().RequestedEnvironment
	if err := binding.Validate(binding.TaskID, binding.BenchmarkRevision, binding.Template, env); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*core.Environment)
	}{
		{"image", func(e *core.Environment) { e.Image = "other:pinned" }},
		{"workdir", func(e *core.Environment) { e.Workdir = "/app" }},
		{"cpu", func(e *core.Environment) { e.CPU = 2 }},
		{"memory", func(e *core.Environment) { e.MemoryMB = 2048 }},
		{"storage", func(e *core.Environment) { e.StorageMB = 0 }},
		{"network", func(e *core.Environment) { e.AllowNetwork = false }},
		{"env", func(e *core.Environment) { e.Env["TASK_ONLY"] = "changed" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := binding.clone().RequestedEnvironment
			tc.change(&changed)
			if err := binding.Validate(binding.TaskID, binding.BenchmarkRevision, binding.Template, changed); err == nil {
				t.Fatal("accepted changed benchmark environment")
			}
		})
	}
	if !reflect.DeepEqual(env, binding.RequestedEnvironment) {
		t.Fatal("validation mutated the benchmark environment")
	}
	for _, identity := range [][3]string{
		{"other-task", binding.BenchmarkRevision, binding.Template},
		{binding.TaskID, strings.Repeat("d", 40), binding.Template},
		{binding.TaskID, binding.BenchmarkRevision, "other-template:frozen-build"},
	} {
		if err := binding.Validate(identity[0], identity[1], identity[2], env); err == nil {
			t.Fatal("accepted a different pinned identity")
		}
	}
}

func TestTaskTemplateBindingRequiresPinnedEvidenceAndExplicitAdaptation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*TaskTemplateBinding)
	}{
		{"schema", func(b *TaskTemplateBinding) { b.SchemaVersion = 2 }},
		{"provider", func(b *TaskTemplateBinding) { b.Provider = "other-cloud" }},
		{"revision", func(b *TaskTemplateBinding) { b.BenchmarkRevision = "main" }},
		{"template", func(b *TaskTemplateBinding) { b.Template = "mutable-alias" }},
		{"mutable-build", func(b *TaskTemplateBinding) { b.Template = "task-template:latest" }},
		{"noncanonical-build", func(b *TaskTemplateBinding) { b.Template = "task-template:ea001e7554b9407b890cf20c96e036c2" }},
		{"image-digest", func(b *TaskTemplateBinding) { b.SourceImageDigest = "task:latest" }},
		{"build-evidence", func(b *TaskTemplateBinding) { b.BuildEvidenceSHA256 = "" }},
		{"unresolved-workdir", func(b *TaskTemplateBinding) { b.Workdir = "" }},
		{"root-workdir", func(b *TaskTemplateBinding) { b.Workdir = "/" }},
		{"resources", func(b *TaskTemplateBinding) { b.TemplateConfig.CPUCount = 0 }},
		{"unexplained-adaptation", func(b *TaskTemplateBinding) { b.Adaptations = nil }},
		{"empty-adaptation", func(b *TaskTemplateBinding) { b.Adaptations = []string{" "} }},
		{"invalid-env", func(b *TaskTemplateBinding) { b.CommandEnv["INVALID=NAME"] = "value" }},
		{"relative-mtime-directory", func(b *TaskTemplateBinding) { b.InitialDirectoryMtimeUnixNS = map[string]int64{"app": 1} }},
		{"unclean-mtime-directory", func(b *TaskTemplateBinding) { b.InitialDirectoryMtimeUnixNS = map[string]int64{"/app/..": 1} }},
		{"negative-mtime", func(b *TaskTemplateBinding) { b.InitialDirectoryMtimeUnixNS = map[string]int64{"/": -1} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := testTaskTemplateBinding()
			tc.change(b)
			if err := b.Validate(b.TaskID, b.BenchmarkRevision, b.Template, b.RequestedEnvironment); err == nil {
				t.Fatal("accepted invalid template binding")
			}
		})
	}
}

func TestTaskTemplateBindingProviderSelection(t *testing.T) {
	for _, tc := range []struct {
		name, bound, selected string
		valid                 bool
	}{
		{"legacy E2B", "", "e2b-cloud", true},
		{"explicit E2B", "e2b-cloud", "e2b-cloud", true},
		{"explicit Huawei", "huawei-agentsphere", "huawei-agentsphere", true},
		{"legacy is not Huawei", "", "huawei-agentsphere", false},
		{"E2B is not Huawei", "e2b-cloud", "huawei-agentsphere", false},
		{"Huawei is not E2B", "huawei-agentsphere", "e2b-cloud", false},
		{"unsupported bound", "other-cloud", "other-cloud", false},
		{"unsupported selection", "e2b-cloud", "other-cloud", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			binding := testTaskTemplateBinding()
			binding.Provider = tc.bound
			if err := binding.ValidateProvider(tc.selected); (err == nil) != tc.valid {
				t.Fatalf("valid=%t error=%v", tc.valid, err)
			}
			if binding.Provider != tc.bound {
				t.Fatal("provider validation rewrote the original binding")
			}
			if !tc.valid {
				return
			}
			raw, err := json.Marshal(binding)
			if err != nil {
				t.Fatal(err)
			}
			if tc.bound == "" && strings.Contains(string(raw), `"provider"`) {
				t.Fatal("legacy binding serialization gained a provider field")
			}
			filename := filepath.Join(t.TempDir(), "binding.json")
			if err := os.WriteFile(filename, raw, 0600); err != nil {
				t.Fatal(err)
			}
			loaded, err := LoadTaskTemplateBinding(filename)
			if err != nil || !reflect.DeepEqual(loaded, binding) {
				t.Fatalf("provider binding round trip: %#v, %v", loaded, err)
			}
		})
	}
}

func TestTaskTemplateBindingInitialDirectoryMtimeIsExplicitAndCloned(t *testing.T) {
	b := testTaskTemplateBinding()
	b.RequestedEnvironment = core.Environment{Workdir: b.Workdir}
	b.CommandEnv, b.Adaptations = nil, nil
	b.InitialDirectoryMtimeUnixNS = map[string]int64{"/": 1761350400000000000}
	if err := b.Validate(b.TaskID, b.BenchmarkRevision, b.Template, b.RequestedEnvironment); err == nil {
		t.Fatal("accepted unexplained initial directory mtime adaptation")
	}
	b.Adaptations = []string{"Initialize root directory mtime after bridge preparation; task file metadata is unchanged."}
	if err := b.Validate(b.TaskID, b.BenchmarkRevision, b.Template, b.RequestedEnvironment); err != nil {
		t.Fatal(err)
	}
	copy := b.clone()
	copy.InitialDirectoryMtimeUnixNS["/"] = 0
	if b.InitialDirectoryMtimeUnixNS["/"] == 0 {
		t.Fatal("clone aliases initial directory mtime map")
	}
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(t.TempDir(), "binding.json")
	if err := os.WriteFile(filename, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadTaskTemplateBinding(filename)
	if err != nil || !reflect.DeepEqual(loaded, b) {
		t.Fatalf("directory mtime binding round trip: %#v, %v", loaded, err)
	}
}

func TestTaskTemplateBindingEmptyMapsAndDefaultEnvironment(t *testing.T) {
	b := testTaskTemplateBinding()
	defaults := b.DefaultEnvironment()
	want := map[string]string{"TASK_ONLY": "task", "OVERRIDE": "template", "ADAPTED": "template"}
	if !reflect.DeepEqual(defaults, want) {
		t.Fatalf("defaults = %#v", defaults)
	}
	defaults["TASK_ONLY"], defaults["OVERRIDE"] = "changed", "changed"
	if !reflect.DeepEqual(b.DefaultEnvironment(), want) {
		t.Fatal("default map aliases binding maps")
	}
	b.RequestedEnvironment.Env = nil
	env := b.RequestedEnvironment
	env.Env = map[string]string{}
	env.Services.MCP = []core.TaskMCPService{}
	if err := b.Validate(b.TaskID, b.BenchmarkRevision, b.Template, env); err != nil {
		t.Fatal(err)
	}
}

func TestLoadTaskTemplateBindingRejectsUnknownAndTrailingJSON(t *testing.T) {
	b := testTaskTemplateBinding()
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, content string
		valid         bool
	}{
		{"valid", string(raw), true},
		{"unknown", strings.TrimSuffix(string(raw), "}") + `,"unknown":true}`, false},
		{"trailing", string(raw) + "\n{}", false},
		{"invalid-binding", `{"schema_version":1}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "binding.json")
			if err := os.WriteFile(file, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := LoadTaskTemplateBinding(file)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%t error=%v", tc.valid, err)
			}
			if tc.valid && !reflect.DeepEqual(got, b) {
				t.Fatalf("binding changed: %#v", got)
			}
		})
	}
}

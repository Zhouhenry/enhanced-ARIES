package e2b

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"reflect"
	"strings"

	"github.com/google/uuid"
	"github.com/hyscale-lab/aries/pkg/core"
)

// TemplateResources describes the frozen template configuration, not a runtime
// resource measurement. Nil storage means the build evidence does not prove it.
type TemplateResources struct {
	CPUCount  int  `json:"cpu_count"`
	MemoryMB  int  `json:"memory_mb"`
	StorageMB *int `json:"storage_mb"`
	GPUCount  int  `json:"gpu_count"`
}

// TaskTemplateBinding records the explicit adaptation of one pinned benchmark
// task to one built cloud template. RequestedEnvironment remains the original task.
type TaskTemplateBinding struct {
	SchemaVersion int `json:"schema_version"`
	// An omitted provider retains the original E2B binding contract.
	Provider                    string            `json:"provider,omitempty"`
	TaskID                      string            `json:"task_id"`
	BenchmarkRevision           string            `json:"benchmark_revision"`
	RequestedEnvironment        core.Environment  `json:"requested_environment"`
	SourceImageDigest           string            `json:"source_image_digest"`
	Template                    string            `json:"template"`
	Workdir                     string            `json:"workdir"`
	TemplateConfig              TemplateResources `json:"template_config"`
	CommandEnv                  map[string]string `json:"command_env,omitempty"`
	InitialDirectoryMtimeUnixNS map[string]int64  `json:"initial_directory_mtime_unix_ns,omitempty"`
	Adaptations                 []string          `json:"adaptations"`
	BuildEvidenceSHA256         string            `json:"build_evidence_sha256"`
}

func LoadTaskTemplateBinding(filename string) (*TaskTemplateBinding, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	decoder := json.NewDecoder(f)
	decoder.DisallowUnknownFields()
	var binding TaskTemplateBinding
	if err := decoder.Decode(&binding); err != nil {
		return nil, fmt.Errorf("decode task template binding: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.New("task template binding must contain one JSON object")
	}
	if err := binding.Validate(binding.TaskID, binding.BenchmarkRevision, binding.Template, binding.RequestedEnvironment); err != nil {
		return nil, err
	}
	return &binding, nil
}

func bindingHash(value string, bytes int) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == bytes
}

func normalizedEnvironment(env core.Environment) core.Environment {
	if len(env.Env) == 0 {
		env.Env = nil
	}
	if len(env.Services.MCP) == 0 {
		env.Services.MCP = nil
	}
	return env
}

func (b *TaskTemplateBinding) Validate(taskID, revision, template string, env core.Environment) error {
	if b == nil || b.SchemaVersion != 1 || b.TaskID == "" || b.TaskID != taskID ||
		b.BenchmarkRevision != revision || !bindingHash(b.BenchmarkRevision, 20) || b.Template != template {
		return errors.New("task template binding identity does not match the pinned task")
	}
	if b.Provider != "" && b.Provider != "e2b-cloud" && b.Provider != "huawei-agentsphere" {
		return errors.New("task template binding has an unsupported provider")
	}
	name, build, pinned := strings.Cut(b.Template, ":")
	if !pinned || name == "" || build == "" || strings.ContainsAny(b.Template, " /\\\t\r\n\x00") || strings.Contains(build, ":") {
		return errors.New("task template binding requires an explicit template:build")
	}
	buildID, err := uuid.Parse(build)
	if err != nil || buildID.String() != build {
		return errors.New("task template binding requires a canonical build UUID")
	}
	digest, ok := strings.CutPrefix(b.SourceImageDigest, "sha256:")
	if !ok || !bindingHash(digest, 32) || !bindingHash(b.BuildEvidenceSHA256, 32) {
		return errors.New("task template binding requires image and build evidence SHA-256")
	}
	if !reflect.DeepEqual(normalizedEnvironment(b.RequestedEnvironment), normalizedEnvironment(env)) {
		return errors.New("task template binding does not match the original environment")
	}
	if b.Workdir == "/" || !path.IsAbs(b.Workdir) || path.Clean(b.Workdir) != b.Workdir || strings.ContainsRune(b.Workdir, 0) {
		return errors.New("task template binding requires a resolved absolute workdir")
	}
	r := b.TemplateConfig
	if r.CPUCount <= 0 || r.MemoryMB <= 0 || r.GPUCount < 0 || r.StorageMB != nil && *r.StorageMB <= 0 ||
		env.CPU < 0 || env.MemoryMB < 0 || env.StorageMB < 0 || env.GPUs < 0 {
		return errors.New("task template binding has invalid resources")
	}
	changed := env.CPU != 0 && env.CPU != float64(r.CPUCount) || env.MemoryMB != 0 && env.MemoryMB != r.MemoryMB ||
		env.StorageMB != 0 && (r.StorageMB == nil || env.StorageMB != *r.StorageMB) || env.GPUs != r.GPUCount ||
		env.Workdir != b.Workdir || len(b.CommandEnv) != 0 || len(b.InitialDirectoryMtimeUnixNS) != 0
	if changed && len(b.Adaptations) == 0 {
		return errors.New("template differences require explicit adaptation notes")
	}
	for _, note := range b.Adaptations {
		if strings.TrimSpace(note) == "" {
			return errors.New("task template adaptation note is empty")
		}
	}
	for name, value := range b.DefaultEnvironment() {
		if name == "" || strings.ContainsAny(name, "=\x00") || strings.ContainsRune(value, 0) {
			return errors.New("task template binding has an invalid command environment")
		}
	}
	for directory, timestamp := range b.InitialDirectoryMtimeUnixNS {
		if !path.IsAbs(directory) || path.Clean(directory) != directory || strings.ContainsRune(directory, 0) || timestamp < 0 {
			return errors.New("initial directory mtime requires a clean absolute path and nonnegative Unix nanoseconds")
		}
	}
	return nil
}

// ValidateProvider prevents a template binding from being reused across clouds.
// Legacy bindings without a provider remain E2B-only; Huawei must be explicit.
func (b *TaskTemplateBinding) ValidateProvider(provider string) error {
	if b == nil {
		return errors.New("task template binding is missing")
	}
	boundProvider := b.Provider
	if boundProvider == "" {
		boundProvider = "e2b-cloud"
	}
	if (provider != "e2b-cloud" && provider != "huawei-agentsphere") || boundProvider != provider {
		return errors.New("task template binding provider does not match the selected sandbox provider")
	}
	return nil
}

// DefaultEnvironment returns a fresh map: explicit template adaptations override
// task defaults, and a command may subsequently override either value.
func (b *TaskTemplateBinding) DefaultEnvironment() map[string]string {
	env := maps.Clone(b.RequestedEnvironment.Env)
	if env == nil {
		env = make(map[string]string)
	}
	maps.Copy(env, b.CommandEnv)
	return env
}

func (b *TaskTemplateBinding) clone() *TaskTemplateBinding {
	copy := *b
	copy.RequestedEnvironment.Env = maps.Clone(b.RequestedEnvironment.Env)
	copy.RequestedEnvironment.Services.MCP = append([]core.TaskMCPService(nil), b.RequestedEnvironment.Services.MCP...)
	copy.CommandEnv = maps.Clone(b.CommandEnv)
	copy.InitialDirectoryMtimeUnixNS = maps.Clone(b.InitialDirectoryMtimeUnixNS)
	copy.Adaptations = append([]string(nil), b.Adaptations...)
	if b.TemplateConfig.StorageMB != nil {
		storage := *b.TemplateConfig.StorageMB
		copy.TemplateConfig.StorageMB = &storage
	}
	return &copy
}

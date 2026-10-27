package config

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hyscale-lab/aries/pkg/containerimage"
	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/model"
)

const JiuwenRevision = "e2c9d519498080e249e34f16a80c8c97f0cd1962"
const JiuwenLockSHA256 = "f28fbbd9b208bcf4f6e421818969ed2b6570f0648bed25c2ed15ee5bd551e5c9"

// CloudConfig is the bounded cloud sandbox coding workload. It shares Runner and the
// occurrence scheduler with legacy profiles without claiming a Docker sandbox.
type CloudConfig struct {
	Name           string                `json:"name"`
	AgentType      string                `json:"agent_type"`
	Placement      string                `json:"placement"`
	SandboxBackend string                `json:"sandbox_backend"`
	Workload       string                `json:"workload,omitempty"`
	Benchmark      *CloudBenchmarkConfig `json:"benchmark,omitempty"`
	VersionsFile   string                `json:"versions_file"`
	OutputDir      string                `json:"output_dir"`
	Sandbox        CloudSandboxConfig    `json:"sandbox"`
	Harness        CloudHarnessConfig    `json:"harness"`
	Model          CloudModelConfig      `json:"model"`
	Execution      CloudExecutionConfig  `json:"execution"`
	Versions       Versions              `json:"-"`
}

// HarnessVersion preserves existing OpenClaw tapes and pins Jiuwen independently.
func (c CloudConfig) HarnessVersion() string {
	if c.AgentType == "jiuwenswarm" {
		return "jiuwenswarm:" + JiuwenRevision + ":" + JiuwenLockSHA256
	}
	return c.Harness.OpenClawVersion
}

// CloudBenchmarkConfig selects one original task; the default remains codingcase.
type CloudBenchmarkConfig struct {
	Type    string                       `json:"type"`
	Root    string                       `json:"root"`
	TaskID  string                       `json:"task_id"`
	Variant *CloudBenchmarkVariantConfig `json:"variant,omitempty"`
}

type CloudBenchmarkVariantConfig struct {
	ID                      string `json:"id"`
	InstructionAppendixFile string `json:"instruction_appendix_file"`
	AppendixSHA256          string `json:"appendix_sha256"`
}

func (b CloudBenchmarkConfig) LogicalTaskID() string {
	if b.Variant != nil {
		return b.Variant.ID
	}
	return b.TaskID
}

func (b CloudBenchmarkConfig) ValidateVariant() error {
	if b.Variant == nil {
		return nil
	}
	v := b.Variant
	if v.ID == "" || v.ID == b.TaskID {
		return errors.New("benchmark.variant.id must be distinct from the original task_id")
	}
	if err := validateExperimentName(v.ID); err != nil {
		return fmt.Errorf("benchmark.variant.id: %w", err)
	}
	digest, err := hex.DecodeString(v.AppendixSHA256)
	if strings.TrimSpace(v.InstructionAppendixFile) == "" || err != nil || len(digest) != 32 || v.AppendixSHA256 != strings.ToLower(v.AppendixSHA256) {
		return errors.New("benchmark.variant requires instruction_appendix_file and a lowercase SHA-256 appendix_sha256")
	}
	return nil
}

type CloudSandboxConfig struct {
	AdapterURL     string `json:"adapter_url,omitempty"`
	Template       string `json:"template,omitempty"`
	TokenEnv       string `json:"token_env,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
	BindingFile    string `json:"binding_file,omitempty"`
	// Huawei-only experiment assumption; never evidence of verified isolation.
	AssumeNetworkIsolation bool `json:"assume_network_isolation,omitempty"`
}

type CloudHarnessConfig struct {
	OpenClawPath              string `json:"openclaw_path"`
	OpenClawVersion           string `json:"openclaw_version"`
	JiuwenSourceDir           string `json:"jiuwen_source_dir,omitempty"`
	JiuwenPythonPath          string `json:"jiuwen_python_path,omitempty"`
	JiuwenImage               string `json:"jiuwen_image,omitempty"`
	SSHClientPath             string `json:"ssh_client_path,omitempty"`
	BridgeHost                string `json:"bridge_host,omitempty"`
	DockerSocket              string `json:"docker_socket,omitempty"`
	NativeObservationProtocol string `json:"native_observation_protocol,omitempty"`
	ToolOutputRenderer        string `json:"tool_output_renderer,omitempty"`
	ReplayToolProtocol        string `json:"replay_tool_protocol,omitempty"`
}

// ValidateAgent also protects programmatic wiring before credentials or effects.
func (c CloudConfig) ValidateAgent() error {
	if c.Model.SessionProtocol != "" && (c.AgentType != "jiuwenswarm" || c.Model.SessionProtocol != "jiuwen-tool-branches-v1") {
		return errors.New("model.session_protocol requires jiuwen-tool-branches-v1 and agent_type=jiuwenswarm")
	}
	switch c.AgentType {
	case "openclaw":
		if c.Harness.JiuwenSourceDir != "" || c.Harness.JiuwenPythonPath != "" || c.Harness.JiuwenImage != "" {
			return errors.New("jiuwen_source_dir, jiuwen_python_path and jiuwen_image require agent_type=jiuwenswarm")
		}
		return c.Harness.ValidateNativeOptions(c.Placement)
	case "jiuwenswarm":
		if c.Placement != "embedded" && c.Placement != "external" {
			return errors.New("Jiuwen requires external or embedded placement")
		}
		if c.Placement == "external" {
			if err := containerimage.Validate(c.Harness.JiuwenImage); err != nil {
				return fmt.Errorf("external Jiuwen requires harness.jiuwen_image pinned by sha256: %w", err)
			}
			if err := containerimage.ValidatePinnedTag(c.Harness.JiuwenImage); err != nil {
				return fmt.Errorf("external Jiuwen image: %w", err)
			}
		} else if c.Harness.JiuwenImage != "" || c.Harness.BridgeHost != "" || c.Harness.DockerSocket != "" {
			return errors.New("Jiuwen jiuwen_image, bridge_host and docker_socket require external placement")
		}
		if c.Benchmark != nil {
			return errors.New("Jiuwen currently supports codingcase only; TerminalBench is unsupported")
		}
		for _, value := range []string{c.Harness.JiuwenSourceDir, c.Harness.JiuwenPythonPath} {
			if !filepath.IsAbs(value) || filepath.Clean(value) != value || value == "/" || strings.ContainsRune(value, 0) {
				return errors.New("Jiuwen requires clean non-root absolute guest jiuwen_source_dir and jiuwen_python_path")
			}
		}
		if c.Harness.OpenClawPath != "" || c.Harness.OpenClawVersion != "" || c.Harness.SSHClientPath != "" ||
			c.Harness.NativeObservationProtocol != "" || c.Harness.ToolOutputRenderer != "" || c.Harness.ReplayToolProtocol != "" || c.Model.ToolCallIDDialect != "" {
			return errors.New("Jiuwen does not accept OpenClaw harness, SSH client, native protocol, renderer or tool ID dialect settings")
		}
		if c.Model.ReasoningEffort != "" || c.Model.MaxTokens != 0 || c.Model.ContextLength != 0 || c.Model.Temperature != nil {
			return errors.New("Jiuwen reasoning_effort, max_tokens, context_length and temperature overrides are unsupported")
		}
		if c.Model.ReplayPolicy != "" && c.Model.ReplayPolicy != "strict" {
			return errors.New("Jiuwen currently supports only strict model replay")
		}
		return nil
	default:
		return errors.New("cloud agent_type must be openclaw or jiuwenswarm")
	}
}

// ValidateNativeOptions keeps experimental native behavior explicit and bounded.
func (h CloudHarnessConfig) ValidateNativeOptions(placement string) error {
	if h.NativeObservationProtocol != "" {
		valid := h.NativeObservationProtocol == "aries-ssh-identity-v1" && placement == "external" ||
			h.NativeObservationProtocol == "aries-native-process-v1" && placement == "embedded"
		if !valid {
			return errors.New("native_observation_protocol requires aries-ssh-identity-v1 for external or aries-native-process-v1 for embedded placement")
		}
	}
	if h.ToolOutputRenderer != "" && h.ToolOutputRenderer != "stdout-stderr-v1" {
		return errors.New("tool_output_renderer must be stdout-stderr-v1 when supplied")
	}
	if h.ReplayToolProtocol != "" && (h.ReplayToolProtocol != "terminal-result-v1" || placement != "external" ||
		h.NativeObservationProtocol != "aries-ssh-identity-v1" || h.ToolOutputRenderer != "stdout-stderr-v1") {
		return errors.New("replay_tool_protocol requires terminal-result-v1, external placement, aries-ssh-identity-v1 and stdout-stderr-v1")
	}
	return nil
}

type CloudModelConfig struct {
	Mode                        string        `json:"mode"`
	SessionProtocol             string        `json:"session_protocol,omitempty"`
	ReplayPolicy                string        `json:"replay_policy,omitempty"`
	ToolCallIDDialect           string        `json:"tool_call_id_dialect,omitempty"`
	TapeFile                    string        `json:"tape_file,omitempty"`
	Timing                      string        `json:"timing,omitempty"`
	ProxyListen                 string        `json:"proxy_listen,omitempty"`
	ProxyPublicURL              string        `json:"proxy_public_url,omitempty"`
	ID                          string        `json:"id,omitempty"`
	BaseURL                     string        `json:"base_url,omitempty"`
	APIKeyEnv                   string        `json:"api_key_env,omitempty"`
	MaxTurns                    int           `json:"max_turns,omitempty"`
	MaxTokens                   int           `json:"max_tokens,omitempty"`
	ContextLength               int           `json:"context_length,omitempty"`
	Temperature                 *float64      `json:"temperature,omitempty"`
	ReasoningEffort             string        `json:"reasoning_effort,omitempty"`
	SyntheticInitialDelay       string        `json:"synthetic_initial_delay,omitempty"`
	SyntheticFrameInterval      string        `json:"synthetic_frame_interval,omitempty"`
	InitialDelay, FrameInterval time.Duration `json:"-"`
}

type CloudExecutionConfig struct {
	Concurrency                           int           `json:"concurrency,omitempty"`
	Attempts                              int           `json:"attempts,omitempty"`
	TaskTimeoutText                       string        `json:"task_timeout,omitempty"`
	VerifierTimeoutText                   string        `json:"verifier_timeout,omitempty"`
	GoodputDeadline                       string        `json:"goodput_deadline,omitempty"`
	TaskTimeout, VerifierTimeout, Goodput time.Duration `json:"-"`
}

func LoadCloud(path string) (CloudConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return CloudConfig{}, fmt.Errorf("open cloud profile: %w", err)
	}
	defer f.Close()
	c, err := DecodeCloud(f)
	if err != nil {
		return CloudConfig{}, err
	}
	resolve := func(p string) string {
		if p == "" || filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(filepath.Dir(path), p)
	}
	c.Versions, err = LoadVersions(resolve(c.VersionsFile))
	if err != nil {
		return CloudConfig{}, err
	}
	if c.AgentType == "openclaw" {
		imageTag, _, _ := strings.Cut(c.Versions.OpenClaw.Image, "@")
		experimental := c.Harness.NativeObservationProtocol != "" || c.Harness.ToolOutputRenderer != ""
		if experimental {
			if err := containerimage.Validate(c.Versions.OpenClaw.Image); err != nil {
				return CloudConfig{}, fmt.Errorf("experimental OpenClaw image requires an immutable sha256 pin: %w", err)
			}
		} else if imageTag != "ghcr.io/openclaw/openclaw:"+c.Harness.OpenClawVersion {
			return CloudConfig{}, errors.New("published OpenClaw image tag must match harness.openclaw_version for the A/B workload")
		}
	}
	c.Model.TapeFile = resolve(c.Model.TapeFile)
	c.Sandbox.BindingFile = resolve(c.Sandbox.BindingFile)
	if c.Benchmark != nil {
		c.Benchmark.Root = resolve(c.Benchmark.Root)
		if c.Benchmark.Variant != nil {
			c.Benchmark.Variant.InstructionAppendixFile = resolve(c.Benchmark.Variant.InstructionAppendixFile)
		}
	}
	if c.Harness.SSHClientPath != "" {
		c.Harness.SSHClientPath, err = filepath.Abs(resolve(c.Harness.SSHClientPath))
		if err != nil {
			return CloudConfig{}, err
		}
	}
	return c, nil
}

func DecodeCloud(r io.Reader) (CloudConfig, error) {
	const omittedTimeout = "\x00omitted"
	c := CloudConfig{OutputDir: "runs",
		Model:     CloudModelConfig{ProxyListen: "127.0.0.1:8082", APIKeyEnv: "DEEPSEEK_API_KEY", MaxTurns: 32},
		Execution: CloudExecutionConfig{Concurrency: 1, Attempts: 1, TaskTimeoutText: omittedTimeout, VerifierTimeoutText: omittedTimeout}}
	if err := decodeStrictJSON(r, &c, "cloud profile"); err != nil {
		return CloudConfig{}, err
	}
	if c.Execution.TaskTimeoutText == omittedTimeout {
		c.Execution.TaskTimeoutText = "5m"
		if c.Benchmark != nil {
			c.Execution.TaskTimeoutText = ""
		}
	}
	if c.Execution.VerifierTimeoutText == omittedTimeout {
		c.Execution.VerifierTimeoutText = "1m"
		if c.Benchmark != nil {
			c.Execution.VerifierTimeoutText = ""
		}
	}
	if c.Name == "" || c.VersionsFile == "" || c.OutputDir == "" || (c.AgentType != "openclaw" && c.AgentType != "jiuwenswarm") ||
		(c.Placement != "external" && c.Placement != "embedded") || (c.SandboxBackend != "e2b-cloud" && c.SandboxBackend != "huawei-agentsphere") {
		return CloudConfig{}, errors.New("cloud profile requires name, versions_file, output_dir, agent_type=openclaw|jiuwenswarm, placement=external|embedded and sandbox_backend=e2b-cloud|huawei-agentsphere")
	}
	if err := c.ValidateAgent(); err != nil {
		return CloudConfig{}, err
	}
	if c.Benchmark != nil {
		if c.Benchmark.Type != "terminalbench2" || strings.TrimSpace(c.Benchmark.Root) == "" || c.Benchmark.TaskID == "" {
			return CloudConfig{}, errors.New("cloud benchmark requires type=terminalbench2, root and one task_id")
		}
		if err := validateExperimentName(c.Benchmark.TaskID); err != nil {
			return CloudConfig{}, fmt.Errorf("benchmark.task_id: %w", err)
		}
		if err := c.Benchmark.ValidateVariant(); err != nil {
			return CloudConfig{}, err
		}
		if c.Placement != "external" || c.Execution.Concurrency != 1 || c.Execution.Attempts != 1 {
			return CloudConfig{}, errors.New("TerminalBench cloud currently requires one external E2B or Huawei task with concurrency=1 and attempts=1")
		}
		if c.Workload != "" || c.Sandbox.BindingFile == "" || c.Execution.TaskTimeoutText != "" || c.Execution.VerifierTimeoutText != "" {
			return CloudConfig{}, errors.New("TerminalBench cloud requires sandbox.binding_file, omits workload, and uses task.toml agent/verifier timeouts")
		}
	} else {
		if c.Sandbox.BindingFile != "" {
			return CloudConfig{}, errors.New("sandbox.binding_file requires a TerminalBench benchmark")
		}
		if c.Workload == "" {
			c.Workload = "merge-ranges"
		}
		if c.Workload != "merge-ranges" && c.Workload != "merge-ranges-portable-v1" && c.Workload != "merge-ranges-multicall-v1" && c.Workload != "llm-development-pptx-v1" {
			return CloudConfig{}, errors.New("cloud workload must be merge-ranges, merge-ranges-portable-v1, merge-ranges-multicall-v1 or llm-development-pptx-v1")
		}
	}
	if c.Sandbox.TokenEnv == "" {
		c.Sandbox.TokenEnv = "ARIES_E2B_ADAPTER_TOKEN"
		if c.SandboxBackend == "huawei-agentsphere" {
			c.Sandbox.TokenEnv = "ARIES_HUAWEI_ADAPTER_TOKEN"
		}
	}
	if c.Sandbox.AssumeNetworkIsolation && c.SandboxBackend != "huawei-agentsphere" {
		return CloudConfig{}, errors.New("sandbox.assume_network_isolation is only supported for huawei-agentsphere")
	}
	if err := validateExperimentName(c.Name); err != nil {
		return CloudConfig{}, err
	}
	if c.AgentType == "openclaw" && (!filepath.IsAbs(c.Harness.OpenClawPath) || c.Harness.OpenClawVersion == "") {
		return CloudConfig{}, errors.New("cloud harness requires absolute openclaw_path and explicit openclaw_version")
	}
	if c.AgentType == "openclaw" && c.Placement == "external" && c.Harness.SSHClientPath == "" {
		return CloudConfig{}, errors.New("external placement requires a Linux ssh_client_path")
	}
	if !validEnvName(c.Sandbox.TokenEnv) || !validEnvName(c.Model.APIKeyEnv) {
		return CloudConfig{}, errors.New("cloud credentials must use environment-variable references")
	}
	if c.Execution.Concurrency < 1 || c.Execution.Attempts < 1 || c.Execution.Concurrency > 32 || c.Execution.Attempts > 10000 {
		return CloudConfig{}, errors.New("cloud execution concurrency/attempts are outside the bounded range")
	}
	if c.Sandbox.TimeoutSeconds < 0 || c.Model.MaxTurns < 1 || c.Model.MaxTurns > 1024 {
		return CloudConfig{}, errors.New("invalid cloud timeout or model turn bound")
	}
	var err error
	c.Execution.TaskTimeout, err = cloudDuration(c.Execution.TaskTimeoutText, c.Benchmark != nil)
	if err != nil {
		return CloudConfig{}, fmt.Errorf("execution.task_timeout: %w", err)
	}
	c.Execution.VerifierTimeout, err = cloudDuration(c.Execution.VerifierTimeoutText, c.Benchmark != nil)
	if err != nil {
		return CloudConfig{}, fmt.Errorf("execution.verifier_timeout: %w", err)
	}
	c.Execution.Goodput, err = cloudDuration(c.Execution.GoodputDeadline, true)
	if err != nil {
		return CloudConfig{}, fmt.Errorf("execution.goodput_deadline: %w", err)
	}
	if c.Model.ToolCallIDDialect != "" && (c.Model.ToolCallIDDialect != "openclaw-2026.7.1" || c.Harness.OpenClawVersion != "2026.7.1") {
		return CloudConfig{}, errors.New("tool_call_id_dialect requires openclaw-2026.7.1 with harness version 2026.7.1")
	}
	switch c.Model.Mode {
	case "record":
		if c.Model.ReplayPolicy != "" {
			return CloudConfig{}, errors.New("model.replay_policy requires replay mode")
		}
		if c.Model.TapeFile != "" || c.Model.Timing != "" {
			return CloudConfig{}, errors.New("record does not accept a replay tape or replay timing")
		}
	case "replay":
		if c.Model.ReplayPolicy == "" {
			c.Model.ReplayPolicy = "strict"
		}
		if c.Model.ReplayPolicy != "strict" && c.Model.ReplayPolicy != "workload" {
			return CloudConfig{}, errors.New("model.replay_policy must be strict or workload")
		}
		if c.Model.TapeFile == "" || (c.Model.Timing != "zero" && c.Model.Timing != "recorded" && c.Model.Timing != "synthetic") {
			return CloudConfig{}, errors.New("replay requires tape_file and timing=zero|recorded|synthetic")
		}
	default:
		return CloudConfig{}, errors.New("model.mode must be record or replay")
	}
	if c.Model.Timing != "synthetic" && (c.Model.SyntheticInitialDelay != "" || c.Model.SyntheticFrameInterval != "") {
		return CloudConfig{}, errors.New("synthetic delays require synthetic replay timing")
	}
	c.Model.InitialDelay, err = cloudDuration(c.Model.SyntheticInitialDelay, true)
	if err != nil {
		return CloudConfig{}, err
	}
	c.Model.FrameInterval, err = cloudDuration(c.Model.SyntheticFrameInterval, true)
	if err != nil {
		return CloudConfig{}, err
	}
	if c.Model.InitialDelay > time.Hour || c.Model.FrameInterval > time.Minute {
		return CloudConfig{}, errors.New("synthetic timing exceeds supported bounds")
	}
	if c.Model.MaxTokens != 0 || c.Model.ContextLength != 0 || c.Model.Temperature != nil {
		return CloudConfig{}, errors.New("cloud OpenClaw max_tokens, context_length and temperature overrides are unsupported")
	}
	if _, err := model.ReasoningBody(c.CoreModel("", "")); err != nil {
		return CloudConfig{}, err
	}
	if c.Model.ProxyListen != "127.0.0.1:8082" {
		return CloudConfig{}, errors.New("model.proxy_listen must be 127.0.0.1:8082 to match the cloud Caddy upstream")
	}
	return c, nil
}

func cloudDuration(text string, optional bool) (time.Duration, error) {
	if optional && text == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(text)
	if err != nil || d <= 0 || d > 24*time.Hour {
		return 0, errors.New("expected a positive Go duration of at most 24h")
	}
	return d, nil
}

// ResolveEnvironment consumes only named settings, never stages the whole host
// environment. Keys remain behind lookup and out of the effective configuration.
func (c *CloudConfig) ResolveEnvironment(lookup func(string) ([]byte, bool)) error {
	adapterEnv, templateEnv, adapterDefault := "ARIES_E2B_ADAPTER_URL", "E2B_TEMPLATE_ID", "http://127.0.0.1:8787"
	switch c.SandboxBackend {
	case "e2b-cloud":
	case "huawei-agentsphere":
		adapterEnv, templateEnv, adapterDefault = "ARIES_HUAWEI_ADAPTER_URL", "HUAWEI_SANDBOX_TEMPLATE_ID", "http://127.0.0.1:8788"
	default:
		return errors.New("unsupported cloud sandbox_backend")
	}
	get := func(name string) string {
		if lookup == nil {
			return ""
		}
		b, _ := lookup(name)
		defer clear(b)
		return strings.TrimSpace(string(b))
	}
	for _, setting := range []struct {
		destination *string
		name        string
	}{
		{&c.Sandbox.AdapterURL, adapterEnv},
		{&c.Model.ID, "DEEPSEEK_MODEL"},
		{&c.Model.BaseURL, "DEEPSEEK_BASE_URL"}, {&c.Model.ProxyPublicURL, "ARIES_MODEL_PROXY_PUBLIC_URL"},
	} {
		if *setting.destination == "" {
			*setting.destination = get(setting.name)
		}
	}
	if (c.AgentType == "openclaw" || c.Placement == "external") && c.Harness.BridgeHost == "" {
		c.Harness.BridgeHost = get("ARIES_ECS_PRIVATE_IP")
	}
	if c.Benchmark == nil && c.Sandbox.Template == "" {
		c.Sandbox.Template = get(templateEnv)
	}
	if c.Sandbox.AdapterURL == "" {
		c.Sandbox.AdapterURL = adapterDefault
	}
	adapter, err := url.Parse(c.Sandbox.AdapterURL)
	if err != nil || (adapter.Scheme != "http" && adapter.Scheme != "https") || adapter.Hostname() == "" || adapter.Opaque != "" ||
		adapter.User != nil || adapter.RawQuery != "" || adapter.ForceQuery || adapter.Fragment != "" || strings.Contains(c.Sandbox.AdapterURL, "#") ||
		adapter.RawPath != "" || (adapter.Path != "" && adapter.Path != "/") {
		return errors.New("sandbox.adapter_url must be an HTTP(S) origin without credentials, query, fragment or path")
	}
	if c.Model.BaseURL == "" {
		c.Model.BaseURL = "https://api.deepseek.com"
	}
	if c.Sandbox.Template == "" && c.Benchmark == nil || c.Model.ID == "" {
		return fmt.Errorf("%s and DEEPSEEK_MODEL must be configured for a formal cloud workload", templateEnv)
	}
	for _, limit := range []struct {
		name             string
		actual, fallback int
	}{
		{"ARIES_MAX_CONCURRENCY", c.Execution.Concurrency, 1}, {"ARIES_MAX_ATTEMPTS", c.Execution.Attempts, 2},
	} {
		maximum := limit.fallback
		if v := get(limit.name); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				return fmt.Errorf("%s must be a positive integer", limit.name)
			}
			maximum = n
		}
		if limit.actual > maximum {
			return fmt.Errorf("cloud profile exceeds %s", limit.name)
		}
	}
	maximumTimeout := 600
	if v := get("ARIES_SANDBOX_TIMEOUT_SECONDS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 86400 {
			return errors.New("ARIES_SANDBOX_TIMEOUT_SECONDS must be positive and at most 86400")
		}
		maximumTimeout = n
	}
	if c.Sandbox.TimeoutSeconds == 0 {
		c.Sandbox.TimeoutSeconds = maximumTimeout
	}
	if c.Sandbox.TimeoutSeconds > maximumTimeout {
		return errors.New("cloud profile exceeds ARIES_SANDBOX_TIMEOUT_SECONDS")
	}
	if c.Execution.GoodputDeadline == "" {
		if v := get("ARIES_GOODPUT_DEADLINE_SECONDS"); v != "" {
			seconds, err := strconv.ParseFloat(v, 64)
			if err != nil || !(seconds > 0) || seconds > 86400 {
				return errors.New("ARIES_GOODPUT_DEADLINE_SECONDS must be finite, positive and at most 86400")
			}
			c.Execution.Goodput = time.Duration(seconds * float64(time.Second))
			if c.Execution.Goodput <= 0 {
				return errors.New("Goodput deadline is below clock resolution")
			}
			c.Execution.GoodputDeadline = c.Execution.Goodput.String()
		}
	}
	if c.Placement == "external" {
		ip := net.ParseIP(c.Harness.BridgeHost)
		if ip == nil || !ip.IsPrivate() || ip.IsLoopback() {
			return errors.New("external placement requires ARIES_ECS_PRIVATE_IP or harness.bridge_host reachable from its Docker network")
		}
	}
	for _, endpoint := range []string{c.Model.BaseURL, c.Model.ProxyPublicURL} {
		u, err := url.Parse(endpoint)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" {
			return errors.New("model upstream and public proxy require HTTPS URLs without credentials, queries or fragments")
		}
	}
	public, _ := url.Parse(c.Model.ProxyPublicURL)
	if public.Path != "" && public.Path != "/" {
		return errors.New("model public proxy must be an HTTPS origin without a path")
	}
	if public.Hostname() == "localhost" || net.ParseIP(public.Hostname()) != nil && net.ParseIP(public.Hostname()).IsLoopback() {
		return errors.New("model public proxy must be reachable from the cloud sandbox, not loopback")
	}
	return nil
}

func (c CloudConfig) CoreModel(proxyURL, credentialEnv string) core.ModelConfig {
	// The proxy changes transport, not the native DeepSeek request/history dialect.
	return core.ModelConfig{Provider: "deepseek", BaseURL: proxyURL, Model: c.Model.ID, APIKeyEnv: credentialEnv,
		ReasoningEffort: c.Model.ReasoningEffort}
}

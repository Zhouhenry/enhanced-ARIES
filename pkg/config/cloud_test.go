package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/model"
)

const cloudProfile = `{"name":"cloud-test","agent_type":"openclaw","placement":"external","sandbox_backend":"e2b-cloud","versions_file":"pins.json","harness":{"openclaw_path":"/usr/local/bin/openclaw","openclaw_version":"2026.7.1","ssh_client_path":"bin/aries-ssh-client"},"model":{"mode":"record"}}`

func TestCloudTerminalBenchSelectionAndOriginalTimeouts(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(map[string]any)
		valid  bool
	}{
		{"single original task", func(map[string]any) {}, true},
		{"codingcase conflict", func(p map[string]any) { p["workload"] = "merge-ranges" }, false},
		{"Huawei original task", func(p map[string]any) { p["sandbox_backend"] = "huawei-agentsphere" }, true},
		{"wrong backend", func(p map[string]any) { p["sandbox_backend"] = "unknown" }, false},
		{"embedded unsupported", func(p map[string]any) { p["placement"] = "embedded" }, false},
		{"multiple attempts", func(p map[string]any) { p["execution"] = map[string]any{"attempts": 2} }, false},
		{"concurrent unsupported", func(p map[string]any) { p["execution"] = map[string]any{"concurrency": 2} }, false},
		{"agent override", func(p map[string]any) { p["execution"] = map[string]any{"task_timeout": "1m"} }, false},
		{"verifier override", func(p map[string]any) { p["execution"] = map[string]any{"verifier_timeout": "1m"} }, false},
		{"missing binding", func(p map[string]any) { delete(p, "sandbox") }, false},
		{"unsafe task", func(p map[string]any) { p["benchmark"].(map[string]any)["task_id"] = "../fix-git" }, false},
		{"unknown benchmark", func(p map[string]any) { p["benchmark"].(map[string]any)["type"] = "other" }, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var p map[string]any
			if err := json.Unmarshal([]byte(cloudProfile), &p); err != nil {
				t.Fatal(err)
			}
			p["benchmark"] = map[string]any{"type": "terminalbench2", "root": "dataset", "task_id": "fix-git"}
			p["sandbox"] = map[string]any{"binding_file": "fix-git.binding.json"}
			test.change(p)
			data, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := DecodeCloud(strings.NewReader(string(data)))
			if !test.valid {
				if err == nil {
					t.Fatal("invalid original-task cloud configuration accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Workload != "" || cfg.Execution.TaskTimeout != 0 || cfg.Execution.VerifierTimeout != 0 || cfg.Benchmark.TaskID != "fix-git" {
				t.Fatalf("codingcase defaults leaked into original task: %+v", cfg)
			}
			if cfg.SandboxBackend == "huawei-agentsphere" && (cfg.Sandbox.TokenEnv != "ARIES_HUAWEI_ADAPTER_TOKEN" || cfg.Sandbox.AssumeNetworkIsolation) {
				t.Fatal("Huawei task gained the wrong credential reference or an isolation assumption")
			}
		})
	}
	for _, field := range []string{"task_timeout", "verifier_timeout"} {
		input := strings.Replace(cloudProfile, `"model":`, `"execution":{"`+field+`":""},"model":`, 1)
		if _, err := DecodeCloud(strings.NewReader(input)); err == nil {
			t.Fatalf("legacy empty %s was accepted", field)
		}
	}
}

func TestCloudTerminalBenchPathsAndNoGlobalTemplateFallback(t *testing.T) {
	for _, provider := range []string{"e2b-cloud", "huawei-agentsphere"} {
		t.Run(provider, func(t *testing.T) {
			testCloudTerminalBenchPathsAndNoGlobalTemplateFallback(t, provider)
		})
	}
}

func testCloudTerminalBenchPathsAndNoGlobalTemplateFallback(t *testing.T, provider string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pins.json"), []byte(validVersions), 0600); err != nil {
		t.Fatal(err)
	}
	input := strings.Replace(cloudProfile, `"sandbox_backend":"e2b-cloud"`, `"sandbox_backend":"`+provider+`"`, 1)
	input = strings.Replace(input, `"model":`, `"benchmark":{"type":"terminalbench2","root":"dataset","task_id":"fix-git"},"sandbox":{"binding_file":"bindings/fix-git.json","timeout_seconds":4200},"model":`, 1)
	profile := filepath.Join(dir, "cloud.json")
	if err := os.WriteFile(profile, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadCloud(profile)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Benchmark.Root != filepath.Join(dir, "dataset") || cfg.Sandbox.BindingFile != filepath.Join(dir, "bindings", "fix-git.json") {
		t.Fatal("task paths not relative to profile")
	}
	if err := cfg.ResolveEnvironment(func(name string) ([]byte, bool) {
		values := map[string]string{"E2B_TEMPLATE_ID": "unrelated-e2b-codingcase-template", "HUAWEI_SANDBOX_TEMPLATE_ID": "unrelated-huawei-codingcase-template",
			"ARIES_SANDBOX_TIMEOUT_SECONDS": "4200", "DEEPSEEK_MODEL": "model", "ARIES_MODEL_PROXY_PUBLIC_URL": "https://proxy.example.test", "ARIES_ECS_PRIVATE_IP": "10.0.0.2"}
		value, ok := values[name]
		return []byte(value), ok
	}); err != nil {
		t.Fatal(err)
	}
	if cfg.Sandbox.Template != "" {
		t.Fatal("TerminalBench silently reused the global codingcase template")
	}
	if cfg.Sandbox.TimeoutSeconds != 4200 || cfg.Execution.TaskTimeout != 0 || cfg.Execution.VerifierTimeout != 0 {
		t.Fatal("cloud configuration changed TTL or overrode original task budgets")
	}
}

func TestCloudWorkloadSelectionRejectsUnknownVersions(t *testing.T) {
	for _, value := range []string{"", "merge-ranges", "merge-ranges-portable-v1", "merge-ranges-multicall-v1", "llm-development-pptx-v1", "merge-ranges-portable-v2"} {
		t.Run(value, func(t *testing.T) {
			input := cloudProfile
			if value != "" {
				input = strings.Replace(input, `"model":`, `"workload":"`+value+`","model":`, 1)
			}
			cfg, err := DecodeCloud(strings.NewReader(input))
			if value == "merge-ranges-portable-v2" {
				if err == nil {
					t.Fatal("unknown workload version accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if value == "" {
				value = "merge-ranges"
			}
			if cfg.Workload != value {
				t.Fatalf("workload = %q, want %q", cfg.Workload, value)
			}
			data, err := json.Marshal(cfg)
			if err != nil || !strings.Contains(string(data), `"workload":"`+value+`"`) {
				t.Fatal("effective profile omitted workload version")
			}
		})
	}
}

func TestLoadCloudPreservesVersionTagAndOptionalDigest(t *testing.T) {
	const imageTag = "ghcr.io/openclaw/openclaw:2026.7.1"
	digest := "@sha256:" + strings.Repeat("a", 64)
	for _, test := range []struct {
		image string
		valid bool
	}{
		{imageTag, true},
		{imageTag + digest, true},
		{imageTag + "@sha256:short", false},
		{imageTag + "@sha256:" + strings.Repeat("A", 64), false},
		{imageTag + "@sha512:" + strings.Repeat("a", 128), false},
		{imageTag + "@", false},
		{imageTag + digest + "\n", false},
		{"ghcr.io/openclaw/openclaw:2026.7.2" + digest, false},
		{"example.invalid/openclaw:2026.7.1" + digest, false},
		{"ghcr.io/openclaw/openclaw" + digest, false},
	} {
		t.Run(test.image, func(t *testing.T) {
			directory := t.TempDir()
			var pins map[string]any
			if err := json.Unmarshal([]byte(validVersions), &pins); err != nil {
				t.Fatal(err)
			}
			pins["openclaw"] = map[string]string{"image": test.image}
			encoded, err := json.Marshal(pins)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, "pins.json"), encoded, 0600); err != nil {
				t.Fatal(err)
			}
			profile := filepath.Join(directory, "profile.json")
			if err := os.WriteFile(profile, []byte(cloudProfile), 0600); err != nil {
				t.Fatal(err)
			}
			configuration, err := LoadCloud(profile)
			if !test.valid {
				if err == nil {
					t.Fatal("invalid OpenClaw image pin accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if configuration.Versions.OpenClaw.Image != test.image {
				t.Fatal("image pin was rewritten or digest dropped")
			}
		})
	}
}

func cloudLookup(name string) ([]byte, bool) {
	values := map[string]string{"E2B_TEMPLATE_ID": "explicit-template", "DEEPSEEK_MODEL": "catalog-model", "ARIES_ECS_PRIVATE_IP": "10.0.0.2", "ARIES_MODEL_PROXY_PUBLIC_URL": "https://proxy.example.test", "ARIES_GOODPUT_DEADLINE_SECONDS": "2.5"}
	v, found := values[name]
	return []byte(v), found
}

func TestCloudNetworkIsolationAssumptionIsExplicitAndHuaweiOnly(t *testing.T) {
	for _, backend := range []string{"e2b-cloud", "huawei-agentsphere"} {
		for _, value := range []string{"", "false", "true", `"true"`} {
			t.Run(backend+"/"+value, func(t *testing.T) {
				profile := strings.Replace(cloudProfile, `"e2b-cloud"`, `"`+backend+`"`, 1)
				if value != "" {
					profile = strings.Replace(profile, `"versions_file"`, `"sandbox":{"assume_network_isolation":`+value+`},"versions_file"`, 1)
				}
				cfg, err := DecodeCloud(strings.NewReader(profile))
				if value == `"true"` || backend == "e2b-cloud" && value == "true" {
					if err == nil {
						t.Fatal("invalid isolation assumption accepted")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				lookup := func(name string) ([]byte, bool) {
					if name == "HUAWEI_AGENTSPHERE_ASSUME_NETWORK_ISOLATION" {
						t.Error("adapter environment must not relax the profile")
						return []byte("true"), true
					}
					if name == "HUAWEI_SANDBOX_TEMPLATE_ID" {
						return []byte("huawei-template:build"), true
					}
					return cloudLookup(name)
				}
				if err := cfg.ResolveEnvironment(lookup); err != nil {
					t.Fatal(err)
				}
				if cfg.Sandbox.AssumeNetworkIsolation != (value == "true") {
					t.Fatal("network isolation assumption changed during environment resolution")
				}
				encoded, err := json.Marshal(cfg)
				if err != nil || value == "true" && !strings.Contains(string(encoded), `"assume_network_isolation":true`) {
					t.Fatal("explicit assumption missing from effective profile")
				}
			})
		}
	}
}

func TestCloudConfigurationResolvesOnlyNonsecretSettingsAndLimits(t *testing.T) {
	c, err := DecodeCloud(strings.NewReader(cloudProfile))
	if err != nil {
		t.Fatal(err)
	}
	lookup := func(name string) ([]byte, bool) {
		if strings.Contains(name, "TOKEN") || strings.Contains(name, "API_KEY") {
			t.Errorf("config resolver requested secret %s", name)
		}
		return cloudLookup(name)
	}
	if err := c.ResolveEnvironment(lookup); err != nil {
		t.Fatal(err)
	}
	if c.Model.ID != "catalog-model" || c.Sandbox.Template != "explicit-template" || c.Execution.Goodput != 2500*time.Millisecond || c.Execution.TaskTimeout != 5*time.Minute {
		t.Fatalf("resolved config: %+v", c)
	}
	c.Execution.Attempts = 3
	if err := c.ResolveEnvironment(lookup); err == nil {
		t.Fatal("attempt budget silently exceeded")
	}
}

func TestCloudBackendEnvironmentDefaultsRemainSeparate(t *testing.T) {
	for _, test := range []struct {
		backend, adapterEnv, templateEnv, tokenEnv, defaultURL string
	}{
		{"e2b-cloud", "ARIES_E2B_ADAPTER_URL", "E2B_TEMPLATE_ID", "ARIES_E2B_ADAPTER_TOKEN", "http://127.0.0.1:8787"},
		{"huawei-agentsphere", "ARIES_HUAWEI_ADAPTER_URL", "HUAWEI_SANDBOX_TEMPLATE_ID", "ARIES_HUAWEI_ADAPTER_TOKEN", "http://127.0.0.1:8788"},
	} {
		t.Run(test.backend, func(t *testing.T) {
			profile := strings.Replace(cloudProfile, `"e2b-cloud"`, `"`+test.backend+`"`, 1)
			for _, scenario := range []string{"environment", "loopback-default", "explicit", "missing-template"} {
				t.Run(scenario, func(t *testing.T) {
					input := profile
					if scenario == "explicit" {
						input = strings.Replace(input, `"versions_file":"pins.json"`, `"versions_file":"pins.json","sandbox":{"adapter_url":"https://explicit-adapter.example.test","template":"explicit-pin","token_env":"CUSTOM_ADAPTER_TOKEN"}`, 1)
					}
					c, err := DecodeCloud(strings.NewReader(input))
					if err != nil {
						t.Fatal(err)
					}
					values := map[string]string{
						"ARIES_E2B_ADAPTER_URL": "https://e2b-adapter.example.test", "E2B_TEMPLATE_ID": "e2b-pin",
						"ARIES_HUAWEI_ADAPTER_URL": "https://huawei-adapter.example.test", "HUAWEI_SANDBOX_TEMPLATE_ID": "huawei-pin",
					}
					if scenario == "loopback-default" {
						delete(values, test.adapterEnv)
					}
					if scenario == "missing-template" {
						delete(values, test.templateEnv)
					}
					lookup := func(name string) ([]byte, bool) {
						if name == "ARIES_E2B_ADAPTER_URL" || name == "ARIES_HUAWEI_ADAPTER_URL" || name == "E2B_TEMPLATE_ID" || name == "HUAWEI_SANDBOX_TEMPLATE_ID" {
							if name != test.adapterEnv && name != test.templateEnv || scenario == "explicit" {
								t.Errorf("unexpected backend setting lookup: %s", name)
							}
							value, found := values[name]
							return []byte(value), found
						}
						if strings.Contains(name, "TOKEN") || strings.Contains(name, "API_KEY") {
							t.Errorf("resolver requested a credential: %s", name)
						}
						return cloudLookup(name)
					}
					err = c.ResolveEnvironment(lookup)
					if scenario == "missing-template" {
						if err == nil || !strings.Contains(err.Error(), test.templateEnv) {
							t.Fatalf("missing backend template fell back to another provider: %v", err)
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					wantURL, wantTemplate, wantToken := values[test.adapterEnv], values[test.templateEnv], test.tokenEnv
					if scenario == "loopback-default" {
						wantURL = test.defaultURL
					}
					if scenario == "explicit" {
						wantURL, wantTemplate, wantToken = "https://explicit-adapter.example.test", "explicit-pin", "CUSTOM_ADAPTER_TOKEN"
					}
					if c.Sandbox.AdapterURL != wantURL || c.Sandbox.Template != wantTemplate || c.Sandbox.TokenEnv != wantToken {
						t.Fatalf("backend settings changed: %+v", c.Sandbox)
					}
				})
			}
		})
	}
}

func TestCloudConfigurationRejectsUnsupportedOrUnsafeInputs(t *testing.T) {
	for _, change := range [][2]string{
		{`"openclaw"`, `"hermes"`}, {`"external"`, `"managed"`}, {`"e2b-cloud"`, `"docker"`},
		{`"cloud-test"`, `"../../escape"`}, {`"mode":"record"`, `"mode":"replay"`},
		{`"mode":"record"`, `"mode":"record","timing":"zero"`},
		{`"mode":"record"`, `"mode":"record","api_key":"do-not-accept-credentials"`},
		{`"mode":"record"`, `"mode":"record","proxy_listen":"0.0.0.0:8082"`},
	} {
		if _, err := DecodeCloud(strings.NewReader(strings.Replace(cloudProfile, change[0], change[1], 1))); err == nil {
			t.Fatalf("accepted unsupported config replacement %s", change[1])
		}
	}
	for _, value := range []string{"http://proxy.example.test", "https://user:password@proxy.example.test", "https://localhost", "https://proxy.example.test/prefix"} {
		c, err := DecodeCloud(strings.NewReader(cloudProfile))
		if err != nil {
			t.Fatal(err)
		}
		c.Model.ProxyPublicURL = value
		if err := c.ResolveEnvironment(cloudLookup); err == nil {
			t.Fatal("unsafe public proxy accepted")
		}
	}
}

func TestCloudReplayAndExplicitGoodputRemainDistinct(t *testing.T) {
	profile := strings.Replace(cloudProfile, `"mode":"record"`, `"mode":"replay","timing":"zero","tape_file":"private/tape.json"`, 1)
	c, err := DecodeCloud(strings.NewReader(profile))
	if err != nil {
		t.Fatal(err)
	}
	if c.Model.Timing != "zero" || c.Execution.Goodput != 0 {
		t.Fatal("zero wait created an implicit deadline")
	}
	c.Model.ProxyPublicURL, c.Model.ID, c.Sandbox.Template, c.Harness.BridgeHost = "https://proxy.example.test", "model", "template", "10.0.0.2"
	if err := c.ResolveEnvironment(func(string) ([]byte, bool) { return nil, false }); err != nil {
		t.Fatal(err)
	}
	if c.Execution.Goodput != 0 {
		t.Fatal("missing SLO must omit Goodput")
	}
}

func TestCloudToolCallIDDialectRequiresItsPinnedHarness(t *testing.T) {
	profile := strings.Replace(cloudProfile, `"mode":"record"`, `"mode":"record","tool_call_id_dialect":"openclaw-2026.7.1"`, 1)
	c, err := DecodeCloud(strings.NewReader(profile))
	if err != nil || c.Model.ToolCallIDDialect != "openclaw-2026.7.1" {
		t.Fatalf("explicit dialect lost: %v", err)
	}
	for _, changed := range []string{
		strings.Replace(profile, `"openclaw-2026.7.1"`, `"automatic"`, 1),
		strings.Replace(profile, `"openclaw_version":"2026.7.1"`, `"openclaw_version":"2026.7.2"`, 1),
	} {
		if _, err := DecodeCloud(strings.NewReader(changed)); err == nil {
			t.Fatal("unsupported ID dialect or harness version accepted")
		}
	}
}

func TestCloudRejectsUnmappedGenerationAndMismatchedProxyListener(t *testing.T) {
	for _, field := range []string{
		`"max_tokens":512`, `"context_length":32768`, `"temperature":0`,
		`"reasoning_effort":"medium"`, `"reasoning_effort":"unknown"`,
		`"proxy_listen":"127.0.0.1:8083"`, `"proxy_listen":"127.0.0.1:0"`,
		`"proxy_listen":"[::1]:8082"`,
	} {
		t.Run(field, func(t *testing.T) {
			input := strings.Replace(cloudProfile, `"mode":"record"`, `"mode":"record",`+field, 1)
			if _, err := DecodeCloud(strings.NewReader(input)); err == nil {
				t.Fatal("accepted a setting the cloud runtime cannot honor")
			}
		})
	}
}

func TestCloudRetainsNativeDeepSeekReasoningDialectThroughProxy(t *testing.T) {
	for _, test := range []struct{ effort, expected string }{
		{"off", `{"thinking":{"type":"disabled"}}`},
		{"high", `{"reasoning_effort":"high","thinking":{"type":"enabled"}}`},
	} {
		input := strings.Replace(cloudProfile, `"mode":"record"`, `"mode":"record","reasoning_effort":"`+test.effort+`"`, 1)
		c, err := DecodeCloud(strings.NewReader(input))
		if err != nil {
			t.Fatal(err)
		}
		native := c.CoreModel("https://proxy.example.test/v1", "ATTEMPT_TOKEN")
		if native.Provider != "deepseek" || native.BaseURL != "https://proxy.example.test/v1" || native.APIKeyEnv != "ATTEMPT_TOKEN" {
			t.Fatal("proxy transport changed the native model contract")
		}
		body, err := model.ReasoningBody(native)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		if string(encoded) != test.expected {
			t.Fatalf("wrong native DeepSeek request body: %s", encoded)
		}
	}
}

func TestCloudRejectsAdapterCredentialsBeforeReturningEffectiveConfiguration(t *testing.T) {
	for _, endpoint := range []string{
		"https://user:private-canary@adapter.example.test",
		"https://adapter.example.test?token=private-canary",
		"https://adapter.example.test#private-canary",
		"https://adapter.example.test/path",
		"https://adapter.example.test?",
	} {
		c, err := DecodeCloud(strings.NewReader(cloudProfile))
		if err != nil {
			t.Fatal(err)
		}
		c.Sandbox.AdapterURL = endpoint
		if err := c.ResolveEnvironment(cloudLookup); err == nil || strings.Contains(err.Error(), "private-canary") {
			t.Fatal("unsafe adapter origin was accepted or echoed in the error")
		}
	}
}

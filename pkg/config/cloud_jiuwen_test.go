package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const jiuwenCloudProfile = `{"name":"jiuwen-cloud","agent_type":"jiuwenswarm","placement":"embedded","sandbox_backend":"e2b-cloud","versions_file":"pins.json","harness":{"jiuwen_source_dir":"/opt/jiuwenswarm","jiuwen_python_path":"/opt/jiuwenswarm/.venv/bin/python"},"model":{"mode":"record"}}`
const jiuwenExternalImage = "registry.example/jiuwenswarm:0.2.3@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestCloudHarnessVersionPreservesOpenClawIdentity(t *testing.T) {
	for _, agent := range []string{"", "openclaw"} {
		cfg := CloudConfig{AgentType: agent, Harness: CloudHarnessConfig{OpenClawVersion: "2026.7.1"}}
		if cfg.HarnessVersion() != "2026.7.1" {
			t.Fatal("existing OpenClaw Tape identity changed")
		}
	}
}

func TestJiuwenCloudSelectionAndUnsupportedOptions(t *testing.T) {
	for _, test := range []struct {
		name      string
		change    func(map[string]any)
		wantError string
	}{
		{"E2B record", func(map[string]any) {}, ""},
		{"Huawei record", func(p map[string]any) { p["sandbox_backend"] = "huawei-agentsphere" }, ""},
		{"strict replay default", func(p map[string]any) {
			p["model"] = map[string]any{"mode": "replay", "tape_file": "jiuwen.tape.json", "timing": "recorded"}
		}, ""},
		{"external missing image", func(p map[string]any) { p["placement"] = "external" }, "jiuwen_image pinned"},
		{"external E2B", func(p map[string]any) {
			p["placement"] = "external"
			p["harness"].(map[string]any)["jiuwen_image"] = jiuwenExternalImage
			p["harness"].(map[string]any)["docker_socket"] = "unix:///var/run/docker.sock"
			p["harness"].(map[string]any)["bridge_host"] = "10.0.0.2"
		}, ""},
		{"external Huawei", func(p map[string]any) {
			p["placement"], p["sandbox_backend"] = "external", "huawei-agentsphere"
			p["harness"].(map[string]any)["jiuwen_image"] = jiuwenExternalImage
		}, ""},
		{"external unpinned image", func(p map[string]any) {
			p["placement"] = "external"
			p["harness"].(map[string]any)["jiuwen_image"] = "registry.example/jiuwenswarm:0.2.3"
		}, "jiuwen_image pinned"},
		{"external implicit image tag", func(p map[string]any) {
			p["placement"] = "external"
			p["harness"].(map[string]any)["jiuwen_image"] = strings.Replace(jiuwenExternalImage, ":0.2.3", "", 1)
		}, "explicit tag"},
		{"embedded image", func(p map[string]any) { p["harness"].(map[string]any)["jiuwen_image"] = jiuwenExternalImage }, "require external"},
		{"embedded Docker", func(p map[string]any) { p["harness"].(map[string]any)["docker_socket"] = "unix:///var/run/docker.sock" }, "require external"},
		{"embedded bridge", func(p map[string]any) { p["harness"].(map[string]any)["bridge_host"] = "10.0.0.2" }, "require external"},
		{"TerminalBench", func(p map[string]any) {
			p["benchmark"] = map[string]any{"type": "terminalbench2", "root": "tasks", "task_id": "fix-git"}
		}, "codingcase only"},
		{"relative source", func(p map[string]any) { p["harness"].(map[string]any)["jiuwen_source_dir"] = "source" }, "absolute guest"},
		{"missing Python", func(p map[string]any) { delete(p["harness"].(map[string]any), "jiuwen_python_path") }, "absolute guest"},
		{"workload replay", func(p map[string]any) {
			p["model"] = map[string]any{"mode": "replay", "replay_policy": "workload", "tape_file": "jiuwen.tape.json", "timing": "recorded"}
		}, ""},
		{"OpenClaw dialect", func(p map[string]any) { p["model"].(map[string]any)["tool_call_id_dialect"] = "openclaw-2026.7.1" }, "OpenClaw"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var profile map[string]any
			if err := json.Unmarshal([]byte(jiuwenCloudProfile), &profile); err != nil {
				t.Fatal(err)
			}
			test.change(profile)
			data, err := json.Marshal(profile)
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := DecodeCloud(strings.NewReader(string(data)))
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("want %q, got %v", test.wantError, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Workload != "merge-ranges" || cfg.Harness.OpenClawVersion != "" || cfg.Model.MaxTurns != 32 {
				t.Fatalf("native or codingcase defaults changed: %+v", cfg)
			}
			if cfg.Model.Mode == "replay" {
				wantPolicy, _ := profile["model"].(map[string]any)["replay_policy"].(string)
				if wantPolicy == "" {
					wantPolicy = "strict"
				}
				if cfg.Model.ReplayPolicy != wantPolicy {
					t.Fatal("replay policy changed or strict default lost")
				}
			}
			if cfg.SandboxBackend == "huawei-agentsphere" && cfg.Sandbox.TokenEnv != "ARIES_HUAWEI_ADAPTER_TOKEN" {
				t.Fatal("wrong provider credential reference")
			}
		})
	}
	for _, field := range []string{"openclaw_path", "openclaw_version", "ssh_client_path", "native_observation_protocol", "tool_output_renderer", "replay_tool_protocol"} {
		t.Run(field, func(t *testing.T) {
			profile := strings.Replace(jiuwenCloudProfile, `"harness":{`, `"harness":{"`+field+`":"explicit-value",`, 1)
			if _, err := DecodeCloud(strings.NewReader(profile)); err == nil || !strings.Contains(err.Error(), "OpenClaw") {
				t.Fatalf("unsupported native option accepted: %v", err)
			}
		})
	}
	for _, option := range []string{`"reasoning_effort":"none"`, `"max_tokens":1024`, `"context_length":4096`, `"temperature":0`} {
		t.Run(option, func(t *testing.T) {
			profile := strings.Replace(jiuwenCloudProfile, `"mode":"record"`, `"mode":"record",`+option, 1)
			if _, err := DecodeCloud(strings.NewReader(profile)); err == nil || !strings.Contains(err.Error(), "overrides are unsupported") {
				t.Fatalf("unsupported model override accepted: %v", err)
			}
		})
	}
	profile := strings.Replace(cloudProfile, `"harness":{`, `"harness":{"jiuwen_source_dir":"/opt/jiuwenswarm",`, 1)
	if _, err := DecodeCloud(strings.NewReader(profile)); err == nil {
		t.Fatal("OpenClaw accepted a Jiuwen setting")
	}
}

func TestLoadJiuwenCloudKeepsGuestPathsAndProviderEnvironment(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pins.json"), []byte(validVersions), 0600); err != nil {
		t.Fatal(err)
	}
	profile := filepath.Join(dir, "cloud.json")
	if err := os.WriteFile(profile, []byte(jiuwenCloudProfile), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadCloud(profile)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Harness.JiuwenSourceDir != "/opt/jiuwenswarm" || cfg.Harness.JiuwenPythonPath != "/opt/jiuwenswarm/.venv/bin/python" {
		t.Fatal("guest paths were resolved against controller files")
	}
	if err := cfg.ResolveEnvironment(func(name string) ([]byte, bool) {
		if name == "ARIES_ECS_PRIVATE_IP" {
			t.Error("embedded Jiuwen requested the OpenClaw SSH bridge address")
		}
		values := map[string]string{"E2B_TEMPLATE_ID": "jiuwen-template", "DEEPSEEK_MODEL": "explicit-model", "ARIES_MODEL_PROXY_PUBLIC_URL": "https://proxy.example.test"}
		value, ok := values[name]
		return []byte(value), ok
	}); err != nil {
		t.Fatal(err)
	}
	if err := cfg.ValidateAgent(); err != nil {
		t.Fatalf("environment introduced unsupported native settings: %v", err)
	}
}

func TestExternalJiuwenEnvironmentUsesPrivateBridgeWithoutSSH(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "environment", true: "explicit"}[explicit], func(t *testing.T) {
			profile := strings.Replace(jiuwenCloudProfile, `"placement":"embedded"`, `"placement":"external"`, 1)
			profile = strings.Replace(profile, `"harness":{`, `"harness":{"jiuwen_image":"`+jiuwenExternalImage+`",`, 1)
			cfg, err := DecodeCloud(strings.NewReader(profile))
			if err != nil {
				t.Fatal(err)
			}
			if explicit {
				cfg.Harness.BridgeHost = "10.0.0.3"
			}
			if err := cfg.ResolveEnvironment(func(name string) ([]byte, bool) {
				if explicit && name == "ARIES_ECS_PRIVATE_IP" {
					t.Error("explicit bridge address was overridden")
				}
				values := map[string]string{"ARIES_ECS_PRIVATE_IP": "10.0.0.2", "E2B_TEMPLATE_ID": "jiuwen-template",
					"DEEPSEEK_MODEL": "explicit-model", "ARIES_MODEL_PROXY_PUBLIC_URL": "https://proxy.example.test"}
				value, ok := values[name]
				return []byte(value), ok
			}); err != nil {
				t.Fatal(err)
			}
			want := "10.0.0.2"
			if explicit {
				want = "10.0.0.3"
			}
			if cfg.Harness.BridgeHost != want || cfg.Harness.SSHClientPath != "" || cfg.Harness.JiuwenImage != jiuwenExternalImage {
				t.Fatal("external Jiuwen lost its own image/address or inherited an SSH client")
			}
			if err := cfg.ValidateAgent(); err != nil {
				t.Fatal(err)
			}
		})
	}
	profile := strings.Replace(cloudProfile, `"harness":{`, `"harness":{"jiuwen_image":"`+jiuwenExternalImage+`",`, 1)
	if _, err := DecodeCloud(strings.NewReader(profile)); err == nil || !strings.Contains(err.Error(), "agent_type=jiuwenswarm") {
		t.Fatalf("OpenClaw accepted a Jiuwen image: %v", err)
	}
}

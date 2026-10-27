package openclaw

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestLocalConfigReusesModelAndSelectsGatewayTools(t *testing.T) {
	configuration, err := RenderLocalConfig(testModel(), "/workspace", nil)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(configuration, &config); err != nil {
		t.Fatal(err)
	}
	defaults := config["agents"].(map[string]any)["defaults"].(map[string]any)
	sandbox := defaults["sandbox"].(map[string]any)
	tools := config["tools"].(map[string]any)
	execution := tools["exec"].(map[string]any)
	if sandbox["mode"] != "off" || len(sandbox) != 1 || defaults["workspace"] != "/workspace" || execution["host"] != "gateway" || execution["mode"] != "full" {
		t.Fatalf("local placement configuration: %s", configuration)
	}
	for _, legacy := range []string{"ask", "security"} {
		if _, exists := execution[legacy]; exists {
			t.Fatalf("native exec mode cannot be combined with %s", legacy)
		}
	}
	if strings.Contains(string(configuration), "127.0.0.1:1") || strings.Contains(string(configuration), "/unused") {
		t.Fatal("SSH rendering placeholder escaped into native config")
	}
	if config["models"].(map[string]any)["providers"].(map[string]any)["aries"] == nil {
		t.Fatal("shared model configuration missing")
	}
	launcher, _ := GatewayScripts()
	if !strings.Contains(string(launcher), `"$ARIES_OPENCLAW_EXECUTABLE" gateway run`) {
		t.Fatal("launcher lost the explicitly pinned native executable")
	}
}

func TestWorkloadInputsOnlyChangeDeclaredNativeDefaults(t *testing.T) {
	inputs := &WorkloadInputs{SessionKey: "agent:main:aries-workload", SessionID: "8ad5eb11-e541-4cbf-8ae0-1f9e2822c342", EnvelopeTimestamp: "off", SkipBootstrap: true}
	for placement, render := range map[string]func(*WorkloadInputs) ([]byte, error){
		"external": func(inputs *WorkloadInputs) ([]byte, error) {
			return renderConfig(testModel(), inputs, testEndpoint(), ModeAgent, "", false, false, false, 0)
		},
		"embedded": func(inputs *WorkloadInputs) ([]byte, error) {
			return RenderLocalConfig(testModel(), "/workspace", inputs)
		},
	} {
		t.Run(placement, func(t *testing.T) {
			ordinary, err := render(nil)
			if err != nil {
				t.Fatal(err)
			}
			unset, err := render(&WorkloadInputs{})
			if err != nil || string(unset) != string(ordinary) {
				t.Fatalf("zero workload inputs changed native defaults: %v", err)
			}
			workload, err := render(inputs)
			if err != nil {
				t.Fatal(err)
			}
			var original, configured map[string]any
			if err := json.Unmarshal(ordinary, &original); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(workload, &configured); err != nil {
				t.Fatal(err)
			}
			ordinaryDefaults := original["agents"].(map[string]any)["defaults"].(map[string]any)
			if _, exists := ordinaryDefaults["skipBootstrap"]; exists {
				t.Fatal("ordinary configuration changed native bootstrap behavior")
			}
			defaults := configured["agents"].(map[string]any)["defaults"].(map[string]any)
			if defaults["envelopeTimestamp"] != "off" || defaults["skipBootstrap"] != true {
				t.Fatal("workload did not use the declared native timestamp and bootstrap switches")
			}
			delete(defaults, "envelopeTimestamp")
			delete(defaults, "skipBootstrap")
			if !reflect.DeepEqual(original, configured) {
				t.Fatal("workload changed model, tool or other native configuration")
			}
		})
	}
}

func TestToolOutputRendererPreservesNativeToolPolicyAndParameters(t *testing.T) {
	for name, render := range map[string]func(*WorkloadInputs) ([]byte, error){
		"external": func(inputs *WorkloadInputs) ([]byte, error) {
			return renderConfig(testModel(), inputs, testEndpoint(), ModeAgent, "", false, false, false, 0)
		},
		"embedded": func(inputs *WorkloadInputs) ([]byte, error) {
			return RenderLocalConfig(testModel(), "/workspace", inputs)
		},
	} {
		t.Run(name, func(t *testing.T) {
			inputs := &WorkloadInputs{SessionKey: "agent:main:aries-workload", SessionID: "8ad5eb11-e541-4cbf-8ae0-1f9e2822c342", EnvelopeTimestamp: "off", SkipBootstrap: true}
			baseline, err := render(inputs)
			if err != nil {
				t.Fatal(err)
			}
			configured := *inputs
			configured.ToolOutputRenderer = "stdout-stderr-v1"
			candidate, err := render(&configured)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(baseline, candidate) {
				t.Fatal("renderer changed native configuration, including tool policy or parameters")
			}
			var config map[string]any
			if err := json.Unmarshal(candidate, &config); err != nil {
				t.Fatal(err)
			}
			tools := config["tools"].(map[string]any)
			for _, denied := range tools["deny"].([]any) {
				if denied == "process" || denied == "exec" {
					t.Fatalf("renderer withheld native tool %s", denied)
				}
			}
			toolConfig, err := json.Marshal(tools)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("baseline_candidate_native_config_bytes_equal=true config_sha256=%x tools_config_sha256=%x process_and_exec_enabled=true", sha256.Sum256(candidate), sha256.Sum256(toolConfig))
			if _, err := render(&WorkloadInputs{ToolOutputRenderer: "unknown"}); err == nil {
				t.Fatal("unknown renderer accepted")
			}
		})
	}
}

package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func nativeCloudProfile(fields string) string {
	return strings.Replace(cloudProfile, `"harness":{`, `"harness":{`+fields, 1)
}

func TestCloudNativeOptionsAreExplicitAndPlacementBound(t *testing.T) {
	for _, tc := range []struct {
		name, fields, placement string
		valid                   bool
	}{
		{"default", "", "external", true},
		{"identity", `"native_observation_protocol":"aries-ssh-identity-v1",`, "external", true},
		{"embedded-identity", `"native_observation_protocol":"aries-ssh-identity-v1",`, "embedded", false},
		{"embedded-process", `"native_observation_protocol":"aries-native-process-v1",`, "embedded", true},
		{"external-process", `"native_observation_protocol":"aries-native-process-v1",`, "external", false},
		{"unknown-identity", `"native_observation_protocol":"v2",`, "external", false},
		{"boolean-identity", `"native_observation_protocol":true,`, "external", false},
		{"renderer-A", `"tool_output_renderer":"stdout-stderr-v1",`, "external", true},
		{"renderer-B", `"tool_output_renderer":"stdout-stderr-v1",`, "embedded", true},
		{"unknown-renderer", `"tool_output_renderer":"combined",`, "external", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profile := strings.Replace(nativeCloudProfile(tc.fields), `"placement":"external"`, `"placement":"`+tc.placement+`"`, 1)
			cfg, err := DecodeCloud(strings.NewReader(profile))
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v error=%v", tc.valid, err)
			}
			if tc.name == "default" && (cfg.Harness.NativeObservationProtocol != "" || cfg.Harness.ToolOutputRenderer != "") {
				t.Fatal("default profile enabled experiment")
			}
		})
	}
}

func TestCloudDerivedImageRequiresOptInAndImmutablePin(t *testing.T) {
	for _, fields := range []string{"", `"native_observation_protocol":"aries-ssh-identity-v1",`, `"tool_output_renderer":"stdout-stderr-v1",`} {
		for _, pinned := range []bool{false, true} {
			dir := t.TempDir()
			var pins map[string]any
			if err := json.Unmarshal([]byte(validVersions), &pins); err != nil {
				t.Fatal(err)
			}
			image := "aries-local/openclaw:r01-candidate"
			if pinned {
				image += "@sha256:" + strings.Repeat("a", 64)
			}
			pins["openclaw"] = map[string]string{"image": image}
			data, err := json.Marshal(pins)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "pins.json"), data, 0600); err != nil {
				t.Fatal(err)
			}
			profile := filepath.Join(dir, "profile.json")
			if err := os.WriteFile(profile, []byte(nativeCloudProfile(fields)), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := LoadCloud(profile)
			valid := fields != "" && pinned
			if (err == nil) != valid {
				t.Fatalf("fields=%q pinned=%v error=%v", fields, pinned, err)
			}
			if valid && (cfg.Versions.OpenClaw.Image != image || cfg.Harness.OpenClawVersion != "2026.7.1") {
				t.Fatal("candidate identity or expected native version rewritten")
			}
		}
	}
}

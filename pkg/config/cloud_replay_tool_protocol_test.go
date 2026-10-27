package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCloudReplayToolProtocolRequiresExplicitNativeCandidate(t *testing.T) {
	for _, test := range []struct {
		name, placement, protocol, identity, renderer string
		valid                                         bool
	}{
		{"default", "external", "", "", "", true},
		{"explicit candidate", "external", "terminal-result-v1", "aries-ssh-identity-v1", "stdout-stderr-v1", true},
		{"unknown protocol", "external", "future-v2", "aries-ssh-identity-v1", "stdout-stderr-v1", false},
		{"embedded", "embedded", "terminal-result-v1", "aries-native-process-v1", "stdout-stderr-v1", false},
		{"missing identity", "external", "terminal-result-v1", "", "stdout-stderr-v1", false},
		{"missing renderer", "external", "terminal-result-v1", "aries-ssh-identity-v1", "", false},
		{"unknown renderer", "external", "terminal-result-v1", "aries-ssh-identity-v1", "future-v2", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var profile map[string]any
			if err := json.Unmarshal([]byte(cloudProfile), &profile); err != nil {
				t.Fatal(err)
			}
			profile["placement"] = test.placement
			h := profile["harness"].(map[string]any)
			if test.protocol != "" {
				h["replay_tool_protocol"] = test.protocol
			}
			h["native_observation_protocol"], h["tool_output_renderer"] = test.identity, test.renderer
			data, err := json.Marshal(profile)
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := DecodeCloud(strings.NewReader(string(data)))
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v error=%v", test.valid, err)
			}
			if !test.valid {
				return
			}
			if cfg.Harness.ReplayToolProtocol != test.protocol {
				t.Fatal("protocol input was not retained")
			}
			encoded, err := json.Marshal(cfg.Harness)
			if err != nil {
				t.Fatal(err)
			}
			if test.protocol == "" && strings.Contains(string(encoded), "replay_tool_protocol") {
				t.Fatal("default configuration gained a replay protocol")
			}
		})
	}
}

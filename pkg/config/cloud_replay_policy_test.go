package config

import (
	"strings"
	"testing"
)

func TestCloudReplayPolicyIsExplicitAndDefaultsStrict(t *testing.T) {
	for _, policy := range []string{"", "strict", "workload", "automatic"} {
		setting := `"mode":"replay","timing":"zero","tape_file":"private/tape.json"`
		if policy != "" {
			setting += `,"replay_policy":"` + policy + `"`
		}
		cfg, err := DecodeCloud(strings.NewReader(strings.Replace(cloudProfile, `"mode":"record"`, setting, 1)))
		if policy == "automatic" {
			if err == nil {
				t.Fatal("unknown replay policy accepted")
			}
			continue
		}
		want := policy
		if want == "" {
			want = "strict"
		}
		if err != nil || cfg.Model.ReplayPolicy != want || cfg.Model.Mode != "replay" {
			t.Fatalf("lost replay policy: %q %v", cfg.Model.ReplayPolicy, err)
		}
	}
	for _, policy := range []string{"strict", "workload"} {
		profile := strings.Replace(cloudProfile, `"mode":"record"`, `"mode":"record","replay_policy":"`+policy+`"`, 1)
		if _, err := DecodeCloud(strings.NewReader(profile)); err == nil || !strings.Contains(err.Error(), "replay_policy") {
			t.Fatal("record accepted replay policy")
		}
	}
	cfg, err := DecodeCloud(strings.NewReader(cloudProfile))
	if err != nil || cfg.Model.ReplayPolicy != "" {
		t.Fatal("record default changed")
	}
}

package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCloudTerminalBenchVariantValidationAndPaths(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(map[string]any)
		valid  bool
	}{
		{"explicit variant", func(map[string]any) {}, true},
		{"same identity", func(v map[string]any) { v["id"] = "fix-git" }, false},
		{"empty identity", func(v map[string]any) { v["id"] = "" }, false},
		{"unsafe identity", func(v map[string]any) { v["id"] = "../variant" }, false},
		{"missing appendix", func(v map[string]any) { delete(v, "instruction_appendix_file") }, false},
		{"missing hash", func(v map[string]any) { delete(v, "appendix_sha256") }, false},
		{"malformed hash", func(v map[string]any) { v["appendix_sha256"] = strings.Repeat("g", 64) }, false},
		{"short hash", func(v map[string]any) { v["appendix_sha256"] = "aa" }, false},
		{"uppercase hash", func(v map[string]any) { v["appendix_sha256"] = strings.Repeat("A", 64) }, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var profile map[string]any
			if err := json.Unmarshal([]byte(cloudProfile), &profile); err != nil {
				t.Fatal(err)
			}
			variant := map[string]any{"id": "fix-git-variant-v1", "instruction_appendix_file": "appendices/public.md", "appendix_sha256": strings.Repeat("a", 64)}
			test.change(variant)
			profile["benchmark"] = map[string]any{"type": "terminalbench2", "root": "dataset", "task_id": "fix-git", "variant": variant}
			profile["sandbox"] = map[string]any{"binding_file": "binding.json"}
			data, err := json.Marshal(profile)
			if err != nil {
				t.Fatal(err)
			}
			directory := t.TempDir()
			profilePath := filepath.Join(directory, "profile.json")
			if err := os.WriteFile(profilePath, data, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, "pins.json"), []byte(validVersions), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := LoadCloud(profilePath)
			if !test.valid {
				if err == nil {
					t.Fatal("invalid instruction variant accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Benchmark.TaskID != "fix-git" || cfg.Benchmark.LogicalTaskID() != "fix-git-variant-v1" || cfg.Benchmark.Variant.InstructionAppendixFile != filepath.Join(directory, "appendices/public.md") {
				t.Fatal("original selection, variant identity, or relative appendix path changed")
			}
			cfg.Benchmark.Variant = nil
			if cfg.Benchmark.LogicalTaskID() != "fix-git" {
				t.Fatal("original logical identity changed")
			}
		})
	}
}

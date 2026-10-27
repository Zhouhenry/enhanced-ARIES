package openclaw

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hyscale-lab/aries/pkg/core"
)

func TestNativeVersionPinIsMeasuredAndMismatchRollsBack(t *testing.T) {
	for _, tc := range []struct {
		name, version string
		probeErr      error
		wantPass      bool
	}{
		{"same-pin", "OpenClaw 2026.7.1 (abc123)", nil, true},
		{"wrong-pin", "OpenClaw 2026.7.2", nil, false},
		{"probe-failed", "", errors.New("model-secret transport failure"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeDeployment()
			manager := newTestManager(t, fake, []byte("model-secret"))
			manager.options.ExpectedVersion = "2026.7.1"
			var probes []string
			fake.exec = func(_ context.Context, _ string, command core.Command) (core.CommandResult, error) {
				if command.Path == "/usr/local/bin/openclaw" || command.Path == "/usr/local/bin/node" {
					probes = append(probes, command.Path)
					if command.Dir != "/app" || len(command.Args) != 1 || command.Args[0] != "--version" {
						t.Fatal("native version command changed")
					}
					if command.Path == "/usr/local/bin/openclaw" {
						return core.CommandResult{Stdout: tc.version}, tc.probeErr
					}
					return core.CommandResult{Stdout: "v24.7.9\n"}, nil
				}
				return core.CommandResult{}, nil
			}
			request := core.HarnessRequest{Connectivity: core.HarnessConnectivity{Placement: core.RuntimePlacement{AttachmentID: "aries-net-test"}}, RunID: "run", TaskID: "case", Endpoint: endpointFiles(t), Model: testModel()}
			err := manager.Start(context.Background(), request)
			if (err == nil) != tc.wantPass {
				t.Fatalf("start = %v", err)
			}
			if err != nil && strings.Contains(err.Error(), "model-secret") {
				t.Fatal("probe error leaked credential")
			}
			p := filepath.Join(manager.runtime.Options.OutputDir, "case", "native-version.json")
			content, readErr := os.ReadFile(p)
			if readErr != nil {
				t.Fatal(readErr)
			}
			var evidence map[string]any
			if e := json.Unmarshal(content, &evidence); e != nil {
				t.Fatal(e)
			}
			info, e := os.Stat(p)
			if e != nil || info.Mode().Perm() != 0o600 {
				t.Fatal("version evidence is not private")
			}
			if tc.wantPass {
				if len(probes) != 2 || evidence["status"] != "PASS" || evidence["node"] != "v24.7.9" || evidence["openclaw"] != tc.version {
					t.Fatalf("actual version proof lost: %s", content)
				}
				if err := manager.Stop(context.Background()); err != nil {
					t.Fatal(err)
				}
			} else if !fake.removed || evidence["status"] != "FAIL" || manager.active != nil {
				t.Fatal("failed native check did not roll back before admission")
			}
			if !tc.wantPass {
				if _, err := os.Stat(filepath.Join(manager.runtime.Options.OutputDir, "case", "harness")); !os.IsNotExist(err) {
					t.Fatal("failed startup retained its staged harness directory")
				}
			}
		})
	}
}

func TestNativeVersionCheckIsOptInForExistingProfiles(t *testing.T) {
	fake := newFakeDeployment()
	manager := newTestManager(t, fake, []byte("model-secret"))
	fake.exec = func(_ context.Context, _ string, command core.Command) (core.CommandResult, error) {
		if command.Path == "/usr/local/bin/openclaw" || command.Path == "/usr/local/bin/node" {
			t.Fatal("unrequested native probe")
		}
		return core.CommandResult{}, nil
	}
	if err := manager.Start(context.Background(), core.HarnessRequest{Connectivity: core.HarnessConnectivity{Placement: core.RuntimePlacement{AttachmentID: "aries-net-test"}}, RunID: "run", TaskID: "case", Endpoint: endpointFiles(t), Model: testModel()}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

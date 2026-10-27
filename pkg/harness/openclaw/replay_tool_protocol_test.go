package openclaw

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/deployment"
	"github.com/hyscale-lab/aries/pkg/harness"
	"github.com/hyscale-lab/aries/pkg/runner"
)

func TestReplayToolProtocolRequiresIdentityAndRendererBeforeCreate(t *testing.T) {
	for _, test := range []struct {
		name, protocol, renderer string
		identity, valid          bool
	}{
		{"default", "", "", false, true},
		{"candidate", "terminal-result-v1", "stdout-stderr-v1", true, true},
		{"unknown", "future-v2", "stdout-stderr-v1", true, false},
		{"missing identity", "terminal-result-v1", "stdout-stderr-v1", false, false},
		{"missing renderer", "terminal-result-v1", "", true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := newFakeDeployment()
			options := Options{Runtime: harness.RuntimeOptions{Deployment: fake, Image: testOpenClawImage, OutputDir: t.TempDir()},
				WorkloadInputs: &WorkloadInputs{SessionKey: nativeScope().AgentSessionID, ReplayToolProtocol: test.protocol, ToolOutputRenderer: test.renderer}}
			if test.identity {
				options.NativeIdentity = nativeScope()
			}
			manager, err := New(options)
			if (err == nil) != test.valid || fake.createCalls != 0 {
				t.Fatalf("valid=%v creates=%d error=%v", test.valid, fake.createCalls, err)
			}
			if manager != nil {
				_ = manager.Close()
			}
		})
	}
}

func TestReplayToolProtocolIsExplicitLaunchInputWithoutNativeConfigChanges(t *testing.T) {
	var baselineConfig []byte
	var baselineEnv []string
	for _, enabled := range []bool{false, true} {
		fake := newFakeDeployment()
		manager := newTestManager(t, fake, []byte("model-secret"))
		manager.options.NativeIdentity = nativeScope()
		manager.options.WorkloadInputs = &WorkloadInputs{SessionKey: nativeScope().AgentSessionID, ToolOutputRenderer: "stdout-stderr-v1"}
		if enabled {
			manager.options.WorkloadInputs.ReplayToolProtocol = "terminal-result-v1"
		}
		request := core.HarnessRequest{RunID: "run-1", TaskID: "fix-git", Endpoint: endpointFiles(t), Model: testModel(), Connectivity: core.HarnessConnectivity{Placement: core.RuntimePlacement{AttachmentID: "test-network"}}}
		if err := manager.Start(context.Background(), request); err != nil {
			t.Fatal(err)
		}
		config, err := os.ReadFile(filepath.Join(manager.active.ArtifactDir, "openclaw.json"))
		if err != nil {
			t.Fatal(err)
		}
		var nativeEnv []string
		found := 0
		for _, item := range fake.created.Env {
			if strings.HasPrefix(item, "ARIES_REPLAY_TOOL_PROTOCOL=") {
				if item != "ARIES_REPLAY_TOOL_PROTOCOL=terminal-result-v1" {
					t.Fatal("wrong native protocol")
				}
				found++
			} else {
				nativeEnv = append(nativeEnv, item)
			}
		}
		if enabled {
			if found != 1 || !bytes.Equal(config, baselineConfig) || !reflect.DeepEqual(nativeEnv, baselineEnv) {
				t.Fatal("protocol changed other native config or launch environment")
			}
		} else {
			if found != 0 {
				t.Fatal("default launch enabled protocol")
			}
			baselineConfig, baselineEnv = config, nativeEnv
		}
		if err := manager.Stop(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := RenderLocalConfig(testModel(), "/workspace", &WorkloadInputs{ReplayToolProtocol: "terminal-result-v1"}); err == nil {
		t.Fatal("embedded rendering accepted external-only protocol")
	}
}

type replayProtocolDeployment struct {
	*fakeDeployment
	content   []byte
	sourceErr error
	mode      int64
	reads     int
}

func (d *replayProtocolDeployment) DownloadArchive(ctx context.Context, id, path string) (io.ReadCloser, deployment.FileInfo, error) {
	if path != replayToolProtocolPath {
		return d.fakeDeployment.DownloadArchive(ctx, id, path)
	}
	d.reads++
	if d.sourceErr != nil {
		return nil, deployment.FileInfo{}, d.sourceErr
	}
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	_ = writer.WriteHeader(&tar.Header{Name: filepath.Base(path), Mode: d.mode, Size: int64(len(d.content))})
	_, _ = writer.Write(d.content)
	_ = writer.Close()
	return io.NopCloser(bytes.NewReader(archive.Bytes())), deployment.FileInfo{}, nil
}

func TestReplayToolEvidencePreservesBusinessAndMissingEvidence(t *testing.T) {
	for _, name := range []string{"disabled", "retained", "missing", "unreadable", "not-private", "credential", "write-failed", "index-write-failed"} {
		t.Run(name, func(t *testing.T) {
			fake := newFakeDeployment()
			manager := newTestManager(t, fake, []byte("model-secret"))
			manager.options.NativeIdentity = nativeScope()
			manager.options.WorkloadInputs = &WorkloadInputs{SessionKey: nativeScope().AgentSessionID, ToolOutputRenderer: "stdout-stderr-v1"}
			if name != "disabled" {
				manager.options.WorkloadInputs.ReplayToolProtocol = "terminal-result-v1"
			}
			request := core.HarnessRequest{RunID: "run-1", TaskID: "fix-git", Endpoint: endpointFiles(t), Model: testModel(), Connectivity: core.HarnessConnectivity{Placement: core.RuntimePlacement{AttachmentID: "test-network"}}}
			if err := manager.Start(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			d := &replayProtocolDeployment{fakeDeployment: fake, mode: 0600, content: []byte("{\"schema_version\":1,\"protocol\":\"terminal-result-v1\",\"event\":\"wait_started\"}\n")}
			switch name {
			case "missing":
				d.sourceErr = runner.ErrNotFound
			case "unreadable":
				d.sourceErr = errors.New("source unavailable")
			case "not-private":
				d.mode = 0644
			case "credential":
				d.content = []byte("model-secret\n")
			case "write-failed":
				if err := os.Mkdir(filepath.Join(manager.active.ArtifactDir, "replay-tool-protocol.jsonl"), 0700); err != nil {
					t.Fatal(err)
				}
			case "index-write-failed":
				if err := os.Mkdir(filepath.Join(manager.active.ArtifactDir, "replay-tool-protocol.index.json"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			manager.runtime.Options.Deployment = d
			result, err := manager.Run(context.Background(), "repair git")
			if err != nil || result.Status != core.StatusSucceeded || result.FinalResponse != "task complete" {
				t.Fatalf("observer changed Agent result: %+v %v", result, err)
			}
			indexPath := filepath.Join(manager.active.ArtifactDir, "replay-tool-protocol.index.json")
			if name == "disabled" {
				if d.reads != 0 {
					t.Fatal("default path collected new protocol")
				}
				if _, err := os.Stat(indexPath); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("default path wrote protocol index")
				}
			} else if name != "index-write-failed" {
				data, err := os.ReadFile(indexPath)
				if err != nil {
					t.Fatal(err)
				}
				var index replayToolProtocolIndex
				if err := json.Unmarshal(data, &index); err != nil {
					t.Fatal(err)
				}
				if index.Protocol != "terminal-result-v1" || index.SchemaVersion != 1 {
					t.Fatal("incorrect evidence identity")
				}
				if name == "retained" {
					raw, err := os.ReadFile(filepath.Join(manager.active.ArtifactDir, "replay-tool-protocol.jsonl"))
					if err != nil {
						t.Fatal(err)
					}
					digest := sha256.Sum256(raw)
					if index.FileIntegrity != "retained" || index.Issue != "" || !bytes.Equal(raw, d.content) || index.SHA256 != hex.EncodeToString(digest[:]) {
						t.Fatal("raw evidence or receipt changed")
					}
				} else if name == "missing" {
					if index.FileIntegrity != "not_collected" || index.Issue != "producer_artifact_missing" || index.SHA256 != "" {
						t.Fatal("missing evidence became success")
					}
				} else if index.FileIntegrity != "incomplete" || index.Issue == "" {
					t.Fatal("failed observation became retained")
				}
				if name == "credential" {
					raw, err := os.ReadFile(filepath.Join(manager.active.ArtifactDir, "replay-tool-protocol.jsonl"))
					if err != nil || bytes.Contains(raw, []byte("model-secret")) {
						t.Fatal("credential leaked into artifact")
					}
				}
			}
			if err := manager.Stop(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

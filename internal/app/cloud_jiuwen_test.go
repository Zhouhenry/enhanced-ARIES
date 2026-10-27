package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/internal/modelreplay"
	"github.com/hyscale-lab/aries/pkg/config"
	"github.com/hyscale-lab/aries/pkg/core"
)

func TestJiuwenCloudRecordAndStrictReplayUseNativePins(t *testing.T) {
	for _, placement := range []string{"embedded", "external"} {
		for _, protocol := range []string{"", modelreplay.BranchProtocol} {
			t.Run(placement+"/"+protocol, func(t *testing.T) { testJiuwenCloudRecordAndStrictReplayUseNativePins(t, placement, protocol) })
		}
	}
}

func testJiuwenCloudRecordAndStrictReplayUseNativePins(t *testing.T, placement, protocol string) {
	t.Helper()
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer upstream.Close()
	cfg := config.CloudConfig{AgentType: "jiuwenswarm", Placement: placement,
		Model:     config.CloudModelConfig{Mode: "record", ID: "test-model", BaseURL: upstream.URL, APIKeyEnv: "TEST_KEY", ProxyPublicURL: "https://proxy.example.test", MaxTurns: 2},
		Execution: config.CloudExecutionConfig{TaskTimeout: time.Second}}
	cfg.Model.SessionProtocol = protocol
	var tape *modelreplay.Tape
	for _, policy := range []string{"record", "strict", "workload"} {
		mode := "replay"
		if policy == "record" {
			mode = "record"
		}
		cfg.Model.Mode = mode
		if mode == "replay" {
			cfg.Model.Timing, cfg.Model.ReplayPolicy = "zero", policy
		}
		root := t.TempDir()
		proxy := &cloudModelProxy{sessions: make(map[[32]byte]http.Handler)}
		cloud := CloudRun{NewAttempt: func(_ context.Context, attempt CloudAttempt) (CloudExperiment, error) {
			return CloudExperiment{Run: func(ctx context.Context) (core.RunResult, error) {
				attachment := "<system-reminder>\nThe following context is automatically attached for this model call only.\nIt may or may not be relevant to your tasks. Do not respond to it unless it highly relevant to your task.\n\n<prompt-attachment type=\"runtime\">\nCurrent mode: agent\n</prompt-attachment>\n</system-reminder>"
				body, _ := json.Marshal(map[string]any{"model": "test-model", "stream": true, "messages": []any{
					map[string]any{"role": "user", "content": "task"},
					map[string]any{"role": "user", "content": attachment},
				}})
				r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body))).WithContext(ctx)
				r.Header.Set("Authorization", "Bearer "+string(attempt.AccessToken))
				if protocol != "" {
					r.Header.Set(modelreplay.BranchHeader, "main")
				}
				w := httptest.NewRecorder()
				proxy.ServeHTTP(w, r)
				if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "[DONE]") {
					t.Fatalf("%s model handoff failed: %d", mode, w.Code)
				}
				return core.RunResult{}, nil
			}}, nil
		}}
		result, err := runCloudAttempt(context.Background(), cfg, "run", root, taskOccurrence{logicalID: "merge-ranges", executionID: "merge-ranges-001"}, tape, proxy, cloud,
			func(name string) ([]byte, bool) {
				if mode != "record" || name != "TEST_KEY" {
					t.Error("replay or native wiring requested upstream credentials")
				}
				return []byte("fixture-upstream-key"), true
			})
		if err != nil || len(result.ModelSessions) != 1 || result.ModelSessions[0].Status != core.StatusSucceeded || len(proxy.sessions) != 0 {
			t.Fatalf("%s model session or cleanup failed: %+v %v", mode, result.ModelSessions, err)
		}
		if mode == "replay" && result.ModelSessions[0].ReplayPolicy != policy {
			t.Fatal("Jiuwen replay policy lost at the application boundary")
		}
		if mode == "record" {
			tape, err = modelreplay.Load(filepath.Join(root, "merge-ranges-001", "model", "tape.json"))
			if err != nil {
				t.Fatal(err)
			}
			if tape.Identity.Placement != placement || tape.Identity.HarnessVersion != cfg.HarnessVersion() || tape.ToolCallIDDialect != "" || tape.HistoryDialect != modelreplay.JiuwenHistoryDialect || tape.SessionProtocol() != protocol {
				t.Fatal("Jiuwen Tape used OpenClaw identity or dialect")
			}
		}
	}
	if upstreamCalls.Load() != 1 {
		t.Fatal("replay called the upstream model")
	}
	if protocol != "" {
		return
	}

	for _, wrongVersion := range []string{"2026.7.1", "jiuwenswarm:" + strings.Repeat("0", 40) + ":" + config.JiuwenLockSHA256, "jiuwenswarm:" + config.JiuwenRevision + ":" + strings.Repeat("0", 64)} {
		t.Run(wrongVersion, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "runs")
			profilePath := writeCommandProfile(t, output)
			wrong := *tape
			wrong.Identity.HarnessVersion = wrongVersion
			// Drop the Jiuwen-only declaration to test the app's independent
			// version check; the model package separately rejects a foreign pin
			// carrying a Jiuwen history declaration.
			wrong.HistoryDialect = ""
			// Keep this foreign tape valid under the default initial grammar,
			// so the app's independent harness pin check is the failing boundary.
			wrong.Turns = append([]modelreplay.Turn(nil), tape.Turns...)
			for i := range wrong.Turns {
				wrong.Turns[i].Request = json.RawMessage(`{"model":"test-model","stream":true,"messages":[{"role":"user","content":"task"}]}`)
				wrong.Turns[i].SHA256 = ""
				turnData, err := json.Marshal(wrong.Turns[i])
				if err != nil {
					t.Fatal(err)
				}
				wrong.Turns[i].SHA256 = fmt.Sprintf("%x", sha256.Sum256(turnData))
			}
			wrong.SHA256 = ""
			data, err := json.Marshal(wrong)
			if err != nil {
				t.Fatal(err)
			}
			wrong.SHA256 = fmt.Sprintf("%x", sha256.Sum256(data))
			data, err = json.Marshal(wrong)
			if err != nil {
				t.Fatal(err)
			}
			tapePath := filepath.Join(filepath.Dir(profilePath), "foreign.tape.json")
			if err := os.WriteFile(tapePath, data, 0600); err != nil {
				t.Fatal(err)
			}
			profile := cfg
			profile.Name, profile.OutputDir, profile.VersionsFile, profile.SandboxBackend = "jiuwen", output, "versions.json", "e2b-cloud"
			profile.Harness = config.CloudHarnessConfig{JiuwenSourceDir: "/opt/jiuwenswarm", JiuwenPythonPath: "/opt/jiuwenswarm/.venv/bin/python"}
			if placement == "external" {
				profile.Harness.JiuwenImage = "registry.example/jiuwenswarm:0.2.3@sha256:" + strings.Repeat("a", 64)
				profile.Harness.BridgeHost = "10.0.0.2"
			}
			profile.Model.BaseURL, profile.Model.TapeFile = "https://api.deepseek.com", tapePath
			data, err = json.Marshal(profile)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(profilePath, data, 0600); err != nil {
				t.Fatal(err)
			}
			err = RunCloud(context.Background(), profilePath, io.Discard, Dependencies{}, forbidCloudPrepare(t), cloudRecordLookup, "")
			if err == nil || !strings.Contains(err.Error(), "native harness version") {
				t.Fatalf("foreign Tape was not rejected before cloud preparation: %v", err)
			}
		})
	}
}

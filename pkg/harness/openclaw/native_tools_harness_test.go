package openclaw

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/harness"
	"github.com/hyscale-lab/aries/pkg/harness/openclaw/gateway"
	"github.com/hyscale-lab/aries/pkg/performance"
)

func TestNativeToolObservationThroughAgentGatewayToSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		enabled, missingResult bool
	}{{"baseline", false, false}, {"observed", true, false}, {"incomplete", true, true}} {
		t.Run(tc.name, func(t *testing.T) {
			serverDone := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(serverDone)
				upgrader := websocket.Upgrader{}
				connection, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					t.Error(err)
					return
				}
				defer connection.Close()
				_ = connection.SetReadDeadline(time.Now().Add(5 * time.Second))
				write := func(frame gateway.Frame) bool {
					if err := connection.WriteJSON(frame); err != nil {
						t.Error(err)
						return false
					}
					return true
				}
				if !write(gateway.Frame{"type": "event", "event": "connect.challenge", "payload": map[string]any{"nonce": "test-nonce"}}) {
					return
				}
				var connect gateway.Frame
				if err := connection.ReadJSON(&connect); err != nil {
					t.Error(err)
					return
				}
				caps, _ := connect.Map("params")["caps"].([]any)
				if (tc.enabled && (len(caps) != 1 || caps[0] != "tool-events")) || (!tc.enabled && len(caps) != 0) {
					t.Errorf("native capability enabled=%v caps=%v", tc.enabled, caps)
				}
				if !write(gateway.Frame{"type": "res", "id": connect["id"], "ok": true, "payload": map[string]any{"auth": map[string]any{"scopes": []string{"operator.write"}}}}) {
					return
				}
				var request gateway.Frame
				if err := connection.ReadJSON(&request); err != nil {
					t.Error(err)
					return
				}
				session := gateway.Frame(request.Map("params")).String("sessionKey")
				if request.String("method") != "agent" || session != "agent:main:aries-fix-git" {
					t.Error("native Agent request changed")
				}
				if !write(gateway.Frame{"type": "res", "id": request["id"], "ok": true, "payload": map[string]any{"status": "accepted", "runId": "actual-native-run"}}) {
					return
				}
				if tc.enabled {
					if !write(nativeEvent(session, "actual-native-run", "tool", 1, toolData("start"))) {
						return
					}
					if !tc.missingResult {
						result := nativeEvent(session, "actual-native-run", "tool", 2, toolData("result"))
						// The native wall clock jumps backwards; receive offsets must not.
						result.Map("payload")["ts"] = float64(1)
						if !write(result) {
							return
						}
					}
					if !write(nativeEvent(session, "actual-native-run", "lifecycle", 3, map[string]any{"phase": "end"})) {
						return
					}
				}
				if !write(gateway.Frame{"type": "res", "id": request["id"], "ok": true, "payload": map[string]any{"status": "ok", "runId": "actual-native-run", "result": map[string]any{"payloads": []any{map[string]any{"text": "unchanged Agent result"}}}}}) {
					return
				}
				// Keep the event stream open until the client closes it after collection.
				_, _, _ = connection.ReadMessage()
			}))
			defer server.Close()
			root := t.TempDir()
			manager, err := New(Options{Runtime: harness.RuntimeOptions{Deployment: newFakeDeployment(), Image: testOpenClawImage,
				OutputDir: root, AgentTimeout: 3 * time.Second, APIKeyLookup: func(string) ([]byte, bool) { return []byte("test-model-secret"), true }}, ObserveNativeTools: tc.enabled})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			recorder, err := performance.Open(root, "run-1")
			if err != nil {
				t.Fatal(err)
			}
			defer recorder.Close()
			ctx = performance.WithIdentity(performance.WithRecorder(ctx, recorder), performance.Identity{OccurrenceID: "fix-git", Purpose: "task"})
			request := core.HarnessRequest{RunID: "run-1", TaskID: "fix-git", Endpoint: endpointFiles(t), Model: testModel(),
				Connectivity: core.HarnessConnectivity{Placement: core.RuntimePlacement{AttachmentID: "test-network"}}}
			if err := manager.Start(ctx, request); err != nil {
				t.Fatal(err)
			}
			defer manager.Stop(context.Background())
			manager.active.gatewayURL = "ws" + strings.TrimPrefix(server.URL, "http")
			result, err := manager.Run(ctx, "repair git")
			if err != nil || result.Status != core.StatusSucceeded || result.FinalResponse != "unchanged Agent result" {
				t.Fatalf("observer changed Agent outcome: %+v %v", result, err)
			}
			if result.RuntimeExecutionDuration == nil || *result.RuntimeExecutionDuration != result.RuntimeFinishedAt.Sub(*result.RuntimeStartedAt) {
				t.Fatal("native Runtime timing boundary changed")
			}
			<-serverDone
			report, observationErr := manager.NativeToolObservation()
			wantStatus := core.StatusSucceeded
			if !tc.enabled {
				wantStatus = core.StatusNotEnabled
			}
			if tc.missingResult {
				wantStatus = core.StatusFailed
			}
			if report.Status != wantStatus || (observationErr != nil) != tc.missingResult {
				t.Fatalf("observation=%+v err=%v", report, observationErr)
			}
			resultBytes, err := os.ReadFile(filepath.Join(root, "fix-git", "harness", "agent-result.json"))
			if err != nil {
				t.Fatal(err)
			}
			var identity struct {
				Run     string `json:"run_id"`
				Native  string `json:"native_run_id"`
				Session string `json:"session_key"`
			}
			if err := json.Unmarshal(resultBytes, &identity); err != nil {
				t.Fatal(err)
			}
			if identity.Run != "actual-native-run" || identity.Native != identity.Run || identity.Session != result.SessionID {
				t.Fatalf("native identity changed: %+v", identity)
			}
			if err := recorder.Close(); err != nil {
				t.Fatal(err)
			}
			run, _ := json.Marshal(map[string]any{"run_id": "run-1", "attempts": []any{map[string]any{"attempt_id": "run-1/fix-git", "occurrence_id": "fix-git", "agent_session_id": result.SessionID}}})
			if err := os.WriteFile(filepath.Join(root, "run-result.json"), run, 0600); err != nil {
				t.Fatal(err)
			}
			snapshot, snapshotErr := performance.BuildSnapshot(filepath.Join(root, "performance"))
			if (snapshotErr == nil) != (tc.enabled && !tc.missingResult) {
				t.Fatalf("snapshot status=%s err=%v diagnostics=%v", snapshot.IntegrityStatus, snapshotErr, snapshot.Diagnostics)
			}
			intervals := 0
			for _, interval := range snapshot.Intervals {
				if interval.Category != "native_tool" {
					continue
				}
				intervals++
				if interval.ToolCallID != "tool-1" || interval.NativeRunID != identity.Run || interval.ClockDomain != snapshot.Summary.ClockDomain || interval.Scope != "native_event_consumer_interval" || interval.Attributes["native_clock_domain"] != "native_runtime_wall_clock" || interval.Attributes["process_completion"] != "unknown" {
					t.Fatalf("native observation identity/clock/completion changed: %+v", interval)
				}
				nativeTimestamp, ok := interval.Attributes["native_timestamp_ms"].(*float64)
				if !ok || nativeTimestamp == nil || *nativeTimestamp != 1 {
					t.Fatal("native wall timestamp was rewritten")
				}
				trace, err := performance.ProjectTrace(snapshot)
				if err != nil {
					t.Fatal(err)
				}
				found := false
				for _, event := range trace.Events {
					if event.Category == "native_tool" && event.Phase == "X" {
						found = event.Duration != nil && *event.Duration == float64(interval.DurationNS)/1000
					}
				}
				if !found {
					t.Fatal("Perfetto span did not project the validated receive interval")
				}
			}
			wantIntervals := 0
			if tc.enabled && !tc.missingResult {
				wantIntervals = 1
			}
			if intervals != wantIntervals {
				t.Fatalf("native intervals=%d want=%d", intervals, wantIntervals)
			}
		})
	}
}

//go:build integration

package jiuwenswarm

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/performance"
)

// This explicit opt-in uses a real pinned AgentServer and its real local tools
// against a controlled model endpoint. It is not a cloud benchmark or Replay test.
func TestNativeAgentServerToolRoundTrip(t *testing.T) {
	endpoint := os.Getenv("ARIES_JIUWEN_NATIVE_TEST_ENDPOINT")
	if endpoint == "" {
		t.Skip("requires a prepared pinned native AgentServer")
	}
	project := os.Getenv("ARIES_JIUWEN_NATIVE_TEST_PROJECT")
	workdir := os.Getenv("ARIES_JIUWEN_NATIVE_TEST_WORKDIR")
	if project == "" || !filepath.IsAbs(workdir) {
		t.Fatal("native project and absolute workdir required")
	}
	output := filepath.Join(workdir, "aries-jiuwen-probe.txt")
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("probe output must not preexist")
	}
	client, err := NewClient(endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	prefix := "native-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := client.Health(ctx, prefix+"-health"); err != nil {
		t.Fatal(err)
	}
	data, err := client.CreateSession(ctx, prefix+"-create", project)
	if err != nil {
		t.Fatal(err)
	}
	var session struct {
		ID         string `json:"session_id"`
		ProjectDir string `json:"projectDir"`
	}
	if err := json.Unmarshal(data, &session); err != nil || session.ID == "" || session.ProjectDir != workdir {
		t.Fatalf("native session binding invalid: %v", err)
	}
	artifact := os.Getenv("ARIES_JIUWEN_NATIVE_TEST_ARTIFACTS")
	if artifact == "" {
		artifact = t.TempDir()
	}
	recorder, err := performance.Open(artifact, "jiuwen-native-protocol")
	if err != nil {
		t.Fatal(err)
	}
	ctx = performance.WithRecorder(ctx, recorder)
	frames, err := os.OpenFile(filepath.Join(artifact, "native-frames.jsonl"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		_ = recorder.Close()
		t.Fatal(err)
	}
	var observationErr error
	count := 0
	runErr := client.Run(ctx, prefix+"-run", session.ID, "Use bash to create aries-jiuwen-probe.txt in this project's working directory, containing exactly ARIES_JIUWEN_NATIVE_PROBE followed by one newline. Then finish.", func(f Frame) {
		count++
		if _, err := frames.Write(append(f.Raw, '\n')); err != nil && observationErr == nil {
			observationErr = err
		}
	})
	fileErr := frames.Close()
	recorderErr := recorder.Close()
	if runErr != nil || observationErr != nil || fileErr != nil || recorderErr != nil {
		t.Fatalf("run=%v observation=%v file=%v recorder=%v", runErr, observationErr, fileErr, recorderErr)
	}
	actual, err := os.ReadFile(output)
	if err != nil || string(actual) != "ARIES_JIUWEN_NATIVE_PROBE\n" {
		t.Fatalf("real tool result not observed: %v", err)
	}
	if count < 2 {
		t.Fatal("native stream lacked intermediate events")
	}
	t.Logf("native protocol completed; real tool file verified; frames=%d; cloud acceptance remains unrun", count)
}

package cloud

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/internal/app"
	"github.com/hyscale-lab/aries/pkg/bridge/ssh/execidentity"
	"github.com/hyscale-lab/aries/pkg/core"
	gossh "golang.org/x/crypto/ssh"
)

func TestNativeIdentityWiringBindsDistinctOccurrences(t *testing.T) {
	root := t.TempDir()
	cfg := cloudConfig()
	cfg.Placement = "external"
	cfg.Harness.BridgeHost = "127.0.0.1"
	cfg.Harness.NativeObservationProtocol = "aries-ssh-identity-v1"
	cfg.Harness.SSHClientPath = filepath.Join(root, "unused-client")
	cfg.Harness.DockerSocket = "unix://" + filepath.Join(root, "unused-docker.sock")
	cfg.Versions.OpenClaw.Image = "ghcr.io/openclaw/openclaw:2026.7.1"
	p := &preparedRun{config: cfg, runID: "run", outputRoot: root, token: strings.Repeat("a", 32), attempts: make(map[string]*attempt)}
	t.Cleanup(func() {
		if err := p.close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	for _, occurrence := range []string{"task-1", "task-2"} {
		_, err := p.newAttempt(context.Background(), app.CloudAttempt{RunID: "run", LogicalID: "merge-ranges", OccurrenceID: occurrence, OutputDir: root,
			Model: core.ModelConfig{Provider: "openai", BaseURL: "https://proxy.example/v1", Model: "model", APIKeyEnv: proxyTokenEnv}, AccessToken: []byte("scoped-token")})
		if err != nil {
			t.Fatal(err)
		}
		endpoint, err := p.attempts[occurrence].bridge.Start(context.Background(), &bridgeEvidenceSandbox{id: occurrence})
		if err != nil {
			t.Fatal(err)
		}
		client, err := gossh.Dial("tcp", endpoint.Address, &gossh.ClientConfig{User: "aries", HostKeyCallback: gossh.InsecureIgnoreHostKey(), Timeout: time.Second})
		if err != nil {
			t.Fatal(err)
		}
		for _, scope := range []string{"run/" + occurrence, "foreign/" + occurrence} {
			session, err := client.NewSession()
			if err != nil {
				t.Fatal(err)
			}
			metadata := execidentity.Metadata{SchemaVersion: 1, ScopeID: scope, AgentSessionID: "agent:main:aries-workload", ToolCallID: "tool", NativeInvocationID: scope, SSHRequestID: scope}
			payload, err := json.Marshal(metadata)
			if err != nil {
				t.Fatal(err)
			}
			accepted, err := session.SendRequest(execidentity.RequestType, true, payload)
			_ = session.Close()
			if err != nil || accepted != (scope == "run/"+occurrence) {
				t.Fatalf("scope=%s accepted=%v err=%v", scope, accepted, err)
			}
		}
		_ = client.Close()
	}
}

func TestNativeIdentityEmbeddedOptionRejectedBeforeCredentialsOrEffects(t *testing.T) {
	cfg := cloudConfig()
	cfg.Harness.NativeObservationProtocol = "aries-ssh-identity-v1"
	_, err := Prepare(context.Background(), cfg, "run", t.TempDir(), func(string) ([]byte, bool) { t.Fatal("read credentials before rejecting placement"); return nil, false }, nil)
	if err == nil {
		t.Fatal("embedded SSH identity experiment accepted")
	}
}

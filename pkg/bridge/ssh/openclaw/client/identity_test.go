package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/bridge/ssh/execidentity"
	gossh "golang.org/x/crypto/ssh"
)

func clientIdentity() execidentity.Metadata {
	return execidentity.Metadata{SchemaVersion: 1, ScopeID: "scope", AgentSessionID: "session", ToolCallID: "tool", NativeInvocationID: "native", SSHRequestID: "request"}
}

func TestOpaqueIdentityUsesBoundedWireEncoding(t *testing.T) {
	value := strings.Repeat("<>&", 85) + "x"
	identity := execidentity.Metadata{SchemaVersion: 1, ScopeID: value, AgentSessionID: value, ToolCallID: value, NativeInvocationID: value, SSHRequestID: value}
	if expanded, err := json.Marshal(identity); err != nil || len(expanded) <= execidentity.MaxPayload {
		t.Fatal("fixture does not exercise HTML encoding expansion")
	}
	config := serveClientTest(t, func(channel gossh.Channel, requests <-chan *gossh.Request) {
		r := <-requests
		if r == nil || r.Type != execidentity.RequestType {
			t.Error("identity request missing")
			return
		}
		got, err := execidentity.Decode(r.Payload)
		if err != nil || got != identity {
			_ = r.Reply(false, nil)
			t.Errorf("opaque identity changed or exceeded bounds: %v", err)
			return
		}
		_ = r.Reply(true, nil)
		r = <-requests
		if r == nil || r.Type != "exec" {
			t.Error("exec missing")
			return
		}
		_ = r.Reply(true, nil)
		_, _ = io.Copy(io.Discard, channel)
		_, _ = channel.SendRequest("exit-status", false, gossh.Marshal(struct{ Status uint32 }{0}))
	})
	config.Identity = &identity
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if code, err := Run(ctx, config, "same command", io.NopCloser(strings.NewReader("")), io.Discard, io.Discard); code != 0 || err != nil {
		t.Fatalf("opaque identity rejected: %d %v", code, err)
	}
}

func TestIdentityAcknowledgmentPrecedesExecAndInput(t *testing.T) {
	for _, accepted := range []bool{true, false} {
		name := "rejected"
		if accepted {
			name = "accepted"
		}
		t.Run(name, func(t *testing.T) {
			identity := clientIdentity()
			sawExec := make(chan bool, 1)
			config := serveClientTest(t, func(channel gossh.Channel, requests <-chan *gossh.Request) {
				r := <-requests
				if r == nil || r.Type != execidentity.RequestType || !r.WantReply {
					t.Error("identity not sent first")
					return
				}
				got, err := execidentity.Decode(r.Payload)
				if err != nil || got != identity {
					t.Error("identity changed", err)
				}
				_ = r.Reply(accepted, nil)
				r = <-requests
				if !accepted {
					sawExec <- r != nil
					return
				}
				if r == nil || r.Type != "exec" {
					t.Error("exec absent")
					return
				}
				sawExec <- true
				var payload struct{ Command string }
				if gossh.Unmarshal(r.Payload, &payload) != nil || payload.Command != "unchanged command" {
					t.Error("command changed")
				}
				_ = r.Reply(true, nil)
				input, err := io.ReadAll(channel)
				if err != nil || !bytes.Equal(input, []byte{0, 255, 'x'}) {
					t.Error("stdin changed")
				}
				_, _ = channel.Write(input)
				_, _ = channel.SendRequest("exit-status", false, gossh.Marshal(struct{ Status uint32 }{0}))
			})
			config.Identity = &identity
			var output bytes.Buffer
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			code, err := Run(ctx, config, "unchanged command", io.NopCloser(bytes.NewReader([]byte{0, 255, 'x'})), &output, io.Discard)
			if accepted {
				if err != nil || code != 0 || !bytes.Equal(output.Bytes(), []byte{0, 255, 'x'}) {
					t.Fatalf("accepted=%d %v", code, err)
				}
			} else if err == nil || code != 255 {
				t.Fatal("rejected identity ran")
			}
			select {
			case got := <-sawExec:
				if got != accepted {
					t.Fatal("exec sent after rejection")
				}
			case <-ctx.Done():
				t.Fatal("server did not finish")
			}
		})
	}
}

func TestNativeIdentityPrivateFileAndPerClientRequestID(t *testing.T) {
	dir := testOpenClawSSHDirectory(t)
	configPath := filepath.Join(dir, "config")
	if err := os.WriteFile(configPath, []byte(lockedConfigContent()), 0o600); err != nil {
		t.Fatal(err)
	}
	legacy, err := loadClientConfig(configPath)
	if err != nil || legacy.identity != nil {
		t.Fatal("legacy config invented identity")
	}
	path := filepath.Join(dir, "aries-tool-identity.json")
	m := clientIdentity()
	m.SSHRequestID = ""
	content, _ := json.Marshal(m)
	if err := os.WriteFile(path, content, 0o400); err != nil {
		t.Fatal(err)
	}
	one, err := loadClientConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	two, err := loadClientConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if one.identity == nil || one.identity.SSHRequestID == "" || two.identity.SSHRequestID == one.identity.SSHRequestID || one.identity.NativeInvocationID != m.NativeInvocationID {
		t.Fatal("request IDs missing/colliding or native identity changed")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadClientConfig(configPath); err == nil {
		t.Fatal("writable identity file accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "missing"), path); err != nil {
		t.Fatal(err)
	}
	if _, err := loadClientConfig(configPath); err == nil {
		t.Fatal("dangling symlink treated as absent identity")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimSuffix(string(content), "}")+`,"ssh_request_id":"forged"}`), 0o400); err != nil {
		t.Fatal(err)
	}
	if _, err := loadClientConfig(configPath); err == nil {
		t.Fatal("native supplied request ID accepted")
	}
}

package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/hyscale-lab/aries/pkg/core"
)

func TestGuestHeaderRedactionPreservesPublicRoutingPort(t *testing.T) {
	for _, portHeader := range []string{"e2b-sandbox-port", "E2b-Sandbox-Port", "E2B-SANDBOX-PORT", "e2B-sAnDbOx-PoRt"} {
		t.Run(portHeader, func(t *testing.T) {
			credentials := NewCredentials("guest")
			defer credentials.Clear()
			credentials.AddGuestHeaders(http.Header{
				portHeader:                 {"18866"},
				"Authorization":            {"Bearer authorization-secret"},
				"X-API-Key":                {"api-secret"},
				"x-access-token":           {"access-secret"},
				"E2b-Traffic-Access-Token": {"traffic-secret"},
				"e2b-sandbox-id":           {"sandbox-private-id"},
				"X-Unknown-Header":         {"unknown-secret-one", "unknown-secret-two"},
				"X-Unknown-Numeric":        {"18867"},
			})
			public := []byte(`{"span_id":"a18866b1234567890","start_time_unix_nano":1886612345678900000,"body":"tool listened on port 18866"}`)
			if got := credentials.Redact(public); !bytes.Equal(got, public) || !json.Valid(got) {
				t.Fatalf("public trace bytes changed: %s", got)
			}
			raw := []byte(`{"span_id":"a18866b1234567890","port":18866,"body":"port 18866","values":["Bearer authorization-secret","api-secret","access-secret","traffic-secret","sandbox-private-id","unknown-secret-one","unknown-secret-two","18867"]}`)
			original := bytes.Clone(raw)
			want := `{"span_id":"a18866b1234567890","port":18866,"body":"port 18866","values":["[REDACTED]","[REDACTED]","[REDACTED]","[REDACTED]","[REDACTED]","[REDACTED]","[REDACTED]","[REDACTED]"]}`
			if got := credentials.Redact(raw); string(got) != want || !json.Valid(got) {
				t.Fatalf("authentication/unknown headers not redacted as expected: %s", got)
			}
			if !bytes.Equal(raw, original) {
				t.Fatal("redaction modified the source trace buffer")
			}
		})
	}
}

func TestCredentialSnapshotSurvivesOwnerCleanup(t *testing.T) {
	source := []byte("private-model-key")
	owner := NewCredentials("Hermes")
	if ok, err := owner.Load("model", "MODEL_KEY", func(string) ([]byte, bool) { return source, true }); !ok || err != nil {
		t.Fatalf("load: %v, %v", ok, err)
	}
	if !bytes.Equal(source, make([]byte, len(source))) {
		t.Fatal("lookup buffer not cleared")
	}
	calls := 0
	servers := []core.MCPServerConfig{{Name: "one", SecretEnv: map[string]string{"TOKEN": "MCP_KEY"}}, {Name: "two", SecretEnv: map[string]string{"TOKEN": "MCP_KEY"}}}
	if err := owner.LoadMCP(servers, []byte("config"), func(string) ([]byte, bool) { calls++; return []byte("private-mcp-key"), true }); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("duplicate credential lookups: %d", calls)
	}
	snapshot := owner.Snapshot()
	ownerKey := owner.Get("model")
	owner.Clear()
	if !bytes.Equal(ownerKey, make([]byte, len(ownerKey))) {
		t.Fatal("owner buffer retained")
	}
	if string(snapshot.Get("model")) != "private-model-key" || string(snapshot.MCPFiles()["MCP_KEY"]) != "private-mcp-key" {
		t.Fatal("snapshot destroyed by owner cleanup")
	}
	if got := string(snapshot.Redact([]byte("private-model-key private-mcp-key"))); got != "[REDACTED] [REDACTED]" {
		t.Fatalf("snapshot redaction: %q", got)
	}
	snapshotKey := snapshot.MCPFiles()["MCP_KEY"]
	snapshot.Clear()
	if !bytes.Equal(snapshotKey, make([]byte, len(snapshotKey))) {
		t.Fatal("snapshot buffer retained")
	}
}

func TestRedactErrDoesNotExposeOriginalCause(t *testing.T) {
	credentials := NewCredentials("Hermes")
	credentials.Set("model", []byte("private-key"))
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded, errors.New("provider error")} {
		raw := fmt.Errorf("provider echoed private-key: %w", cause)
		redacted := credentials.RedactErr(raw)
		for current := redacted; current != nil; current = errors.Unwrap(current) {
			if strings.Contains(current.Error(), "private-key") {
				t.Fatal("error chain exposes credential")
			}
		}
		if !strings.Contains(redacted.Error(), "[REDACTED]") {
			t.Fatal("secret was not redacted")
		}
		if (cause == context.Canceled || cause == context.DeadlineExceeded) && !errors.Is(redacted, cause) {
			t.Fatal("lost cancellation classification")
		}
	}
	if credentials.RedactErr(nil) != nil {
		t.Fatal("nil error changed")
	}
}

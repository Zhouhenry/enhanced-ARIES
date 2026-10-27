package execidentity

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestIdentityStrictSchemaAndNativeBoundary(t *testing.T) {
	m := Metadata{SchemaVersion: 1, ScopeID: "scope", AgentSessionID: "session", ToolCallID: "tool", NativeInvocationID: "invocation", SSHRequestID: "request"}
	content, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(content)
	if err != nil || got != m {
		t.Fatalf("roundtrip=%+v %v", got, err)
	}
	for _, bad := range []string{
		string(content) + ` {}`, `[]`, `null`, `{}`,
		strings.Replace(string(content), `"schema_version":1`, `"schema_version":1,"schema_version":1`, 1),
		strings.Replace(string(content), `"scope_id":"scope"`, `"scope_id":"scope","scope_id":"other"`, 1),
		strings.Replace(string(content), `"scope_id":"scope"`, `"scope_id":null`, 1),
		strings.Replace(string(content), `"schema_version":1`, `"schema_version":2`, 1),
		strings.Replace(string(content), `"scope_id":"scope"`, `"Scope_ID":"scope"`, 1),
		strings.Replace(string(content), `"tool_call_id":"tool"`, `"tool_call_id":""`, 1),
		strings.Replace(string(content), `"tool_call_id":"tool"`, `"tool_call_id":"a\u0000b"`, 1),
		strings.Replace(string(content), `"tool_call_id":"tool"`, `"tool_call_id":"`+strings.Repeat("x", 257)+`"`, 1),
	} {
		if _, err := Decode([]byte(bad)); err == nil {
			t.Fatalf("accepted invalid metadata: %q", bad)
		}
	}
	if _, err := DecodeNative(content); err == nil {
		t.Fatal("native producer supplied SSH request ID")
	}
	m.SSHRequestID = ""
	native, _ := json.Marshal(m)
	if _, err := Decode(native); err == nil {
		t.Fatal("wire missing request ID accepted")
	}
	if got, err := DecodeNative(native); err != nil || got != m {
		t.Fatalf("native=%+v %v", got, err)
	}
	if _, ok := FromContext(context.Background()); ok {
		t.Fatal("invented identity")
	}
	if got, ok := FromContext(WithContext(context.Background(), m)); !ok || got != m {
		t.Fatal("context identity changed")
	}
}

package performance

import (
	"context"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// NativeToolIdentity binds one native invocation to its accepted transport request.
// It is observation metadata, never part of a command or its environment.
type NativeToolIdentity struct {
	SchemaVersion      int    `json:"schema_version"`
	ScopeID            string `json:"scope_id"`
	AgentSessionID     string `json:"agent_session_id"`
	ToolCallID         string `json:"tool_call_id"`
	NativeInvocationID string `json:"native_invocation_id"`
	SSHRequestID       string `json:"ssh_request_id,omitempty"`
}

func (m NativeToolIdentity) Validate() error { return m.validate(true) }

// ValidateNative checks the producer identity before a transport request exists.
func (m NativeToolIdentity) ValidateNative() error {
	if m.SSHRequestID != "" {
		return errors.New("native identity cannot supply SSH request ID")
	}
	return m.validate(false)
}

func (m NativeToolIdentity) validate(requireSSH bool) error {
	if m.SchemaVersion != 1 {
		return errors.New("unsupported tool identity schema")
	}
	fields := []string{m.ScopeID, m.AgentSessionID, m.ToolCallID, m.NativeInvocationID}
	if requireSSH {
		fields = append(fields, m.SSHRequestID)
	}
	for _, value := range fields {
		if value == "" || len(value) > 256 || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
			return errors.New("invalid tool identity field")
		}
		for _, c := range value {
			if unicode.IsControl(c) {
				return errors.New("invalid tool identity control character")
			}
		}
	}
	return nil
}

type nativeToolIdentityKey struct{}
type nativeToolIdentityValue struct{ identity *NativeToolIdentity }

func WithNativeToolIdentity(ctx context.Context, identity NativeToolIdentity) context.Context {
	return context.WithValue(ctx, nativeToolIdentityKey{}, nativeToolIdentityValue{&identity})
}

func WithoutNativeToolIdentity(ctx context.Context) context.Context {
	return context.WithValue(ctx, nativeToolIdentityKey{}, nativeToolIdentityValue{})
}

func NativeToolIdentityFromContext(ctx context.Context) (NativeToolIdentity, bool) {
	value, ok := ctx.Value(nativeToolIdentityKey{}).(nativeToolIdentityValue)
	if !ok || value.identity == nil {
		return NativeToolIdentity{}, false
	}
	return *value.identity, true
}

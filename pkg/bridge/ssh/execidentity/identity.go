// Package execidentity carries native tool identity on the same SSH channel as exec.
package execidentity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/hyscale-lab/aries/pkg/performance"
)

const RequestType = "aries-tool-identity@aries/v1"
const MaxPayload = 4096

type Metadata = performance.NativeToolIdentity

// Decode accepts only the complete wire schema, including a client request ID.
func Decode(payload []byte) (Metadata, error) { return decode(payload, true) }

// DecodeNative accepts the private native producer file; only the client may add ssh_request_id.
func DecodeNative(payload []byte) (Metadata, error) { return decode(payload, false) }

func decode(payload []byte, wire bool) (Metadata, error) {
	var m Metadata
	if len(payload) > MaxPayload || !utf8.Valid(payload) {
		return m, errors.New("invalid tool identity payload")
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return m, errors.New("tool identity must be an object")
	}
	seen := map[string]bool{}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return Metadata{}, err
		}
		name, ok := key.(string)
		if !ok || seen[name] {
			return Metadata{}, errors.New("duplicate tool identity field")
		}
		seen[name] = true
		var dst any
		switch name {
		case "schema_version":
			dst = &m.SchemaVersion
		case "scope_id":
			dst = &m.ScopeID
		case "agent_session_id":
			dst = &m.AgentSessionID
		case "tool_call_id":
			dst = &m.ToolCallID
		case "native_invocation_id":
			dst = &m.NativeInvocationID
		case "ssh_request_id":
			if !wire {
				return Metadata{}, errors.New("native identity cannot supply SSH request ID")
			}
			dst = &m.SSHRequestID
		default:
			return Metadata{}, errors.New("unknown tool identity field")
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return Metadata{}, err
		}
		if bytes.Equal(raw, []byte("null")) {
			return Metadata{}, errors.New("null tool identity field")
		}
		if err := json.Unmarshal(raw, dst); err != nil {
			return Metadata{}, fmt.Errorf("invalid tool identity field type: %w", err)
		}
	}
	if _, err := decoder.Token(); err != nil {
		return Metadata{}, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return Metadata{}, errors.New("trailing tool identity payload")
	}
	validate := m.ValidateNative
	if wire {
		validate = m.Validate
	}
	if err := validate(); err != nil {
		return Metadata{}, err
	}
	return m, nil
}

func WithContext(ctx context.Context, m Metadata) context.Context {
	return performance.WithNativeToolIdentity(ctx, m)
}
func FromContext(ctx context.Context) (Metadata, bool) {
	return performance.NativeToolIdentityFromContext(ctx)
}

func WithoutContext(ctx context.Context) context.Context {
	return performance.WithoutNativeToolIdentity(ctx)
}

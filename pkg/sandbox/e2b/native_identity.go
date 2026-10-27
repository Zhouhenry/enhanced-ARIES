package e2b

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"

	"github.com/hyscale-lab/aries/pkg/performance"
)

type processObservation struct {
	SchemaVersion  int                            `json:"schema_version"`
	NativeIdentity performance.NativeToolIdentity `json:"native_identity"`
	ExecutionID    string                         `json:"execution_id"`
	ProcessID      string                         `json:"process_id"`
}

type processEvent struct {
	Kind            string          `json:"kind"`
	Phase           string          `json:"phase"`
	Observation     json.RawMessage `json:"observation"`
	Stream          string          `json:"stream"`
	Data            string          `json:"data_base64"`
	ExitCode        *int            `json:"exit_code"`
	Error           string          `json:"error_code"`
	GuestDurationNS *int64          `json:"guest_duration_ns"`
	PopenReturnNS   *int64          `json:"popen_return_ns"`
	FirstOutputNS   *int64          `json:"first_output_ns"`
	ChildExitNS     *int64          `json:"child_exit_observed_ns"`
	PipeEOFNS       *int64          `json:"pipe_eof_ns"`
}

func processObservationFromContext(ctx context.Context, processID string) (*processObservation, error) {
	identity, ok := performance.NativeToolIdentityFromContext(ctx)
	if !ok {
		return nil, nil
	}
	if err := identity.Validate(); err != nil {
		return nil, err
	}
	executionID := performance.ExecutionID(ctx)
	if executionID == "" || processID == "" {
		return nil, errors.New("native tool observation requires execution and process IDs")
	}
	return &processObservation{1, identity, executionID, processID}, nil
}

func (p *Process) boundContext(ctx context.Context) context.Context {
	ctx = performance.WithExecutionID(ctx, p.executionID)
	if p.observation == nil {
		return performance.WithoutNativeToolIdentity(ctx)
	}
	return performance.WithNativeToolIdentity(ctx, p.observation.NativeIdentity)
}

// Reject duplicate identity keys before normal JSON decoding can overwrite them.
func observationObject(raw []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return nil, errors.New("observation must be an object")
	}
	fields := map[string]json.RawMessage{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, errors.New("invalid observation key")
		}
		if _, exists := fields[key]; exists {
			return nil, errors.New("duplicate observation key")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		fields[key] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errors.New("trailing observation value")
	}
	return fields, nil
}

func (want *processObservation) verifyResponse(content []byte) error {
	if want == nil {
		return nil
	}
	fields, err := observationObject(content)
	if err != nil {
		return errors.New("invalid E2B identity response")
	}
	return want.verifyEnvelope(fields["observation"])
}

func (want *processObservation) verifyEnvelope(raw []byte) error {
	if len(raw) > 4096 || !utf8.Valid(raw) {
		return errors.New("invalid E2B process observation")
	}
	envelope, err := observationObject(raw)
	if err != nil || len(envelope) != 4 {
		return errors.New("missing or invalid E2B process observation")
	}
	native, err := observationObject(envelope["native_identity"])
	if err != nil || len(native) != 6 {
		return errors.New("invalid E2B native identity")
	}
	for _, key := range []string{"schema_version", "scope_id", "agent_session_id", "tool_call_id", "native_invocation_id", "ssh_request_id"} {
		if raw, ok := native[key]; !ok || bytes.Equal(raw, []byte("null")) {
			return errors.New("missing E2B native identity field")
		}
	}
	for _, key := range []string{"schema_version", "native_identity", "execution_id", "process_id"} {
		if raw, ok := envelope[key]; !ok || bytes.Equal(raw, []byte("null")) {
			return errors.New("missing E2B observation field")
		}
	}
	var got processObservation
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&got); err != nil || got != *want {
		return errors.New("E2B process observation mismatch")
	}
	return nil
}

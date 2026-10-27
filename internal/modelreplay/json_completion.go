package modelreplay

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Only metadata is decoded. The stored and delivered JSON bytes remain original.
func parseJSONCompletion(raw []byte) (streamResult, error) {
	var result streamResult
	v, err := decode(raw)
	if err != nil {
		return result, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return result, errors.New("invalid JSON completion")
	}
	if _, exists := m["error"]; exists {
		return result, errors.New("upstream stream error")
	}
	choices, ok := m["choices"].([]any)
	if !ok || len(choices) != 1 {
		return result, errors.New("invalid JSON completion choices")
	}
	choice, ok := choices[0].(map[string]any)
	if !ok || choice["index"] != json.Number("0") {
		return result, errors.New("invalid JSON completion choice")
	}
	message, ok := choice["message"].(map[string]any)
	if !ok || message["role"] != "assistant" {
		return result, errors.New("invalid JSON completion message")
	}
	result.canonicalFrame, _ = json.Marshal(m)
	if content := message["content"]; content != nil {
		if _, ok := content.(string); !ok {
			return result, errors.New("invalid JSON completion content")
		}
	}
	if rawCalls, exists := message["tool_calls"]; exists && rawCalls != nil {
		calls, ok := rawCalls.([]any)
		if !ok || len(calls) > maxToolCalls {
			return result, errors.New("invalid JSON completion tools")
		}
		for i, raw := range calls {
			call, ok := raw.(map[string]any)
			if !ok {
				return result, errors.New("invalid JSON completion tool")
			}
			call["index"] = json.Number(strconv.Itoa(i))
		}
	} else {
		delete(message, "tool_calls")
	}
	if err := result.applyChoice(map[string]any{"finish_reason": choice["finish_reason"], "delta": message}); err != nil {
		return result, err
	}
	result.done = true
	return result, result.complete()
}

func (s *Server) recordJSON(w http.ResponseWriter, r *http.Request, t Turn, response *http.Response, started time.Time, observed *requestObservation) (Turn, error) {
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "application/json") {
		failure(w, http.StatusBadGateway, "UPSTREAM_FAILED", "upstream did not return a successful JSON response")
		observed.httpStatus = http.StatusBadGateway
		return t, errors.New("upstream status/protocol unsupported")
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxBody+1))
	if err != nil || len(raw) > maxBody {
		return t, errors.New("incomplete or oversized JSON response")
	}
	receivedAt := time.Now()
	offset := receivedAt.Sub(started).Nanoseconds()
	observed.emit(boundaryEvent{Event: "upstream_json_received", Scope: "upstream_complete_json_receipt", Bytes: len(raw), ReceivedNS: &offset}, receivedAt)
	parsed, err := parseJSONCompletion(raw)
	if err != nil {
		return t, err
	}
	if s.containsSecret(raw) || parsed.containsSecret(s.containsSecret) {
		return Turn{}, errors.New("credential in upstream response")
	}
	if s.bytes+len(t.Request)+len(raw) > maxTape {
		return t, errors.New("tape byte bound exceeded")
	}
	t.ResponseJSON, t.ResponseOffsetNS = bytes.Clone(raw), &offset
	if err := r.Context().Err(); err != nil {
		return t, err
	}
	if err := flushJSON(w, t.ResponseJSON, started, observed, &offset, nil); err != nil {
		return t, err
	}
	t.Finish, t.Calls = parsed.finish, parsed.calls()
	s.bytes += len(t.Request) + len(raw)
	return t, nil
}

func (s *Server) replayJSON(w http.ResponseWriter, r *http.Request, t Turn, started time.Time, observed *requestObservation) error {
	var target time.Duration
	switch s.options.Timing.Mode {
	case "recorded":
		target = time.Duration(*t.ResponseOffsetNS)
	case "synthetic":
		target = s.options.Timing.InitialDelay
	}
	if err := wait(r.Context(), time.Until(started.Add(target))); err != nil {
		return err
	}
	planned := int64(target)
	return flushJSON(w, t.ResponseJSON, started, observed, nil, &planned)
}

func flushJSON(w http.ResponseWriter, raw []byte, started time.Time, observed *requestObservation, received, target *int64) error {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	if err := send(w, raw); err != nil {
		observed.emit(boundaryEvent{Event: "json_flush_failed", Scope: "downstream_json_write_and_flush", Status: "failed", Reason: "WRITE_OR_FLUSH_FAILED"}, time.Now())
		return errors.New("record frame write or flush failed")
	}
	at := time.Now()
	actual := at.Sub(started).Nanoseconds()
	observed.emit(boundaryEvent{Event: "json_flushed", Scope: "downstream_json_flush_complete", Bytes: len(raw), ReceivedNS: received, TargetNS: target, ActualNS: &actual}, at)
	return nil
}

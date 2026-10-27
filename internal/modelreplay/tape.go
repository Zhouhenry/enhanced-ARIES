// Package modelreplay records model HTTP exchanges and replays only that boundary.
// Agent execution and tools remain outside this package.
package modelreplay

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const Version = 2
const jsonResponseVersion = 3
const bundleVersion = 4

const maxBody = 64 << 20
const maxFrame = 8 << 20
const maxTape = 256 << 20
const maxTapeFile = 2 * maxTape
const maxTurns = 1024

// Keep frame capacity proportional to the tape budget, with a conservative
// 128-byte allowance per frame. Raw request/SSE bytes remain bounded separately.
const maxFrames = maxTape / 128

// Tool-call indices are a separate protocol bound, not an SSE frame count.
const maxToolCalls = 65536

type Identity struct {
	RunID          string `json:"run_id"`
	AttemptID      string `json:"attempt_id"`
	SessionID      string `json:"session_id"`
	Placement      string `json:"placement"`
	HarnessVersion string `json:"harness_version"`
}

// Frame is one complete SSE event, including its original line endings.
// OffsetNS is measured at the proxy from request admission to event receipt.
// A frame is neither a model token nor a TCP packet.
type Frame struct {
	OffsetNS int64  `json:"offset_ns"`
	Data     []byte `json:"data"`
}

type Call struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Turn struct {
	Request          json.RawMessage `json:"request"`
	StatusCode       int             `json:"status_code"`
	Frames           []Frame         `json:"frames"`
	ResponseJSON     []byte          `json:"response_json,omitempty"`
	ResponseOffsetNS *int64          `json:"response_offset_ns,omitempty"`
	Finish           string          `json:"finish_reason"`
	Call             *Call           `json:"tool_call,omitempty"`
	Calls            []Call          `json:"tool_calls,omitempty"`
	SHA256           string          `json:"sha256"`
}

// Version 1 keeps its original JSON shape and checksum; new recordings use v2.
func (t Turn) toolCalls() []Call {
	if t.Call != nil {
		return []Call{*t.Call}
	}
	return t.Calls
}

type Tape struct {
	Version           int             `json:"version"`
	Identity          Identity        `json:"identity"`
	ToolCallIDDialect string          `json:"tool_call_id_dialect,omitempty"`
	HistoryDialect    string          `json:"history_dialect,omitempty"`
	Model             string          `json:"model"`
	UpstreamHost      string          `json:"upstream_host"`
	StartedAt         time.Time       `json:"started_at"`
	Complete          bool            `json:"complete"`
	Failure           string          `json:"failure,omitempty"`
	Turns             []Turn          `json:"turns"`
	Children          map[string]Tape `json:"children,omitempty"`
	SHA256            string          `json:"sha256"`
}

func digest(value any) string {
	b, _ := json.Marshal(value)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func turnDigest(t Turn) string { t.SHA256 = ""; return digest(t) }
func tapeDigest(t Tape) string { t.SHA256 = ""; return digest(t) }

// ValidateToolCallIDDialect binds the optional recording causality rule to its
// supported native version. Empty keeps exact response/request tool IDs.
func ValidateToolCallIDDialect(dialect, harnessVersion string) error {
	if dialect == "" {
		return nil
	}
	if dialect != "openclaw-2026.7.1" {
		return errors.New("unsupported tool-call ID dialect")
	}
	if harnessVersion != "2026.7.1" {
		return errors.New("tool-call ID dialect requires OpenClaw 2026.7.1")
	}
	return nil
}

// JiuwenHistoryDialect validates the pinned native request attachment and
// display-argument extraction. Requests, SSE and strict matching stay literal.
const JiuwenHistoryDialect = "jiuwenswarm-e2c9d519-request-tail-call-goal-v2"

// Subagents have their own persistent history and may use JSON completions.
const JiuwenSubagentHistoryDialect = "jiuwenswarm-e2c9d519-subagent-v1"

const jiuwenRequestTailHistoryDialect = "jiuwenswarm-e2c9d519-request-tail-v1"

// Keep the earlier fixed-tail declaration immutable for existing tapes.
const jiuwenFixedTailHistoryDialect = "jiuwenswarm-e2c9d519-fixed-tail-v1"

const jiuwenFixedTailHarnessVersion = "jiuwenswarm:e2c9d519498080e249e34f16a80c8c97f0cd1962:f28fbbd9b208bcf4f6e421818969ed2b6570f0648bed25c2ed15ee5bd551e5c9"

func ValidateHistoryDialect(dialect, harnessVersion string) error {
	if dialect == "" {
		return nil
	}
	if (dialect != JiuwenHistoryDialect && dialect != JiuwenSubagentHistoryDialect && dialect != jiuwenRequestTailHistoryDialect && dialect != jiuwenFixedTailHistoryDialect) || harnessVersion != jiuwenFixedTailHarnessVersion {
		return errors.New("history dialect requires the pinned Jiuwen source and dependency lock")
	}
	return nil
}

// Load checks the entire immutable tape, including every request and SSE event.
// A checksum detects corruption; it does not authenticate who produced a tape.
func Load(path string) (*Tape, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxTapeFile+1))
	if err != nil || len(b) > maxTapeFile {
		return nil, errors.New("tape exceeds read bound")
	}
	var t Tape
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&t); err != nil {
		return nil, errors.New("invalid tape JSON")
	}
	if err := eof(d); err != nil {
		return nil, err
	}
	if err := validateTape(t); err != nil {
		return nil, err
	}
	return &t, nil
}

func validateTape(t Tape) error {
	if (t.Version != 1 && t.Version != Version && t.Version != jsonResponseVersion && t.Version != bundleVersion) || !t.Complete || len(t.Turns) == 0 || len(t.Turns) > maxTurns || t.Failure != "" {
		return errors.New("tape is incomplete or its version is unsupported")
	}
	if err := validateIdentity(t.Identity); err != nil {
		return err
	}
	if err := ValidateToolCallIDDialect(t.ToolCallIDDialect, t.Identity.HarnessVersion); err != nil {
		return err
	}
	if err := ValidateHistoryDialect(t.HistoryDialect, t.Identity.HarnessVersion); err != nil {
		return err
	}
	if t.Model == "" || t.SHA256 != tapeDigest(t) {
		return errors.New("tape checksum mismatch")
	}
	var previous *Turn
	totalBytes := 0
	totalFrames := 0
	for i := range t.Turns {
		turn := &t.Turns[i]
		if t.Version == 1 && turn.Calls != nil || t.Version >= Version && turn.Call != nil {
			return errors.New("tool metadata does not match tape version")
		}
		if t.Version < jsonResponseVersion && (turn.ResponseJSON != nil || turn.ResponseOffsetNS != nil) {
			return errors.New("JSON response requires tape version 3")
		}
		if turn.SHA256 != turnDigest(*turn) || turn.StatusCode != 200 {
			return fmt.Errorf("turn %d checksum/status mismatch", i)
		}
		req, err := parseRequestProtocol(turn.Request, t.Model, t.HistoryDialect == JiuwenSubagentHistoryDialect)
		if err != nil {
			return fmt.Errorf("turn %d: %w", i, err)
		}
		if err := nextRequest(req, previous, t.ToolCallIDDialect, t.HistoryDialect); err != nil {
			return fmt.Errorf("turn %d: %w", i, err)
		}
		var response streamResult
		var offset int64
		turnBytes := 0
		totalFrames += len(turn.Frames)
		if totalFrames > maxFrames || len(turn.Request) > maxBody {
			return errors.New("tape turn exceeds bound")
		}
		for _, frame := range turn.Frames {
			turnBytes += len(frame.Data)
			if turnBytes > maxBody || len(frame.Data) > maxFrame {
				return errors.New("tape frames exceed bound")
			}
			if frame.OffsetNS < offset || frame.OffsetNS > int64(24*time.Hour) {
				return errors.New("invalid frame timing")
			}
			offset = frame.OffsetNS
			if err := response.feed(frame.Data); err != nil {
				return err
			}
		}
		if req["stream"] == false {
			if len(turn.Frames) != 0 || turn.ResponseOffsetNS == nil || *turn.ResponseOffsetNS < 0 || *turn.ResponseOffsetNS > int64(24*time.Hour) || len(turn.ResponseJSON) > maxBody {
				return errors.New("invalid JSON response representation or timing")
			}
			response, err = parseJSONCompletion(turn.ResponseJSON)
			if err != nil {
				return err
			}
			turnBytes += len(turn.ResponseJSON)
		} else if turn.ResponseJSON != nil || turn.ResponseOffsetNS != nil {
			return errors.New("streaming request cannot contain a JSON completion")
		}
		if err := response.complete(); err != nil {
			return err
		}
		if turn.Finish != response.finish || digest(turn.toolCalls()) != digest(response.calls()) {
			return errors.New("response metadata does not match SSE")
		}
		previous = turn
		totalBytes += turnBytes + len(turn.Request)
		if totalBytes > maxTape {
			return errors.New("tape exceeds byte bound")
		}
	}
	if previous.Finish != "stop" {
		return errors.New("tape has no final model response")
	}
	if t.Version == bundleVersion {
		if err := validateBundleChildren(t); err != nil {
			return err
		}
	} else if len(t.Children) != 0 {
		return errors.New("child trajectories require tape version 4")
	}
	return nil
}

func validateIdentity(id Identity) error {
	for _, s := range []string{id.RunID, id.AttemptID, id.SessionID, id.HarnessVersion} {
		if strings.TrimSpace(s) == "" || len(s) > 256 || strings.ContainsAny(s, "\r\n\x00") {
			return errors.New("run/attempt/session/harness identity is required")
		}
	}
	if id.Placement != "external" && id.Placement != "embedded" {
		return errors.New("placement must be external or embedded")
	}
	return nil
}

func eof(d *json.Decoder) error {
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("expected exactly one JSON value")
	}
	return nil
}

// decode rejects duplicate keys so canonical matching cannot hide changed fields.
func decode(raw []byte) (any, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	v, err := value(d, 0)
	if err != nil {
		return nil, err
	}
	return v, eof(d)
}

func value(d *json.Decoder, depth int) (any, error) {
	if depth > 64 {
		return nil, errors.New("JSON nesting exceeds bound")
	}
	t, err := d.Token()
	if err != nil {
		return nil, errors.New("invalid JSON")
	}
	switch t {
	case json.Delim('{'):
		m := map[string]any{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return nil, err
			}
			k, ok := key.(string)
			if !ok {
				return nil, errors.New("invalid JSON key")
			}
			if _, ok := m[k]; ok {
				return nil, errors.New("duplicate JSON key")
			}
			v, err := value(d, depth+1)
			if err != nil {
				return nil, err
			}
			m[k] = v
		}
		_, err := d.Token()
		return m, err
	case json.Delim('['):
		a := []any{}
		for d.More() {
			v, err := value(d, depth+1)
			if err != nil {
				return nil, err
			}
			a = append(a, v)
		}
		_, err := d.Token()
		return a, err
	default:
		if _, bad := t.(json.Delim); bad {
			return nil, errors.New("unexpected JSON delimiter")
		}
		return t, nil
	}
}

func parseRequest(raw []byte, model string) (map[string]any, error) {
	return parseRequestProtocol(raw, model, false)
}

func parseRequestProtocol(raw []byte, model string, allowJSON bool) (map[string]any, error) {
	v, err := decode(raw)
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New("request must be an object")
	}
	if m["model"] != model || (m["stream"] != true && !(allowJSON && m["stream"] == false)) {
		return nil, errors.New("UNSUPPORTED: model must match and stream must be true")
	}
	if n, ok := m["n"]; ok && n != json.Number("1") {
		return nil, errors.New("UNSUPPORTED: multiple choices")
	}
	ms, ok := m["messages"].([]any)
	if !ok || len(ms) == 0 {
		return nil, errors.New("UNSUPPORTED: messages are required")
	}
	for _, item := range ms {
		msg, ok := item.(map[string]any)
		if !ok {
			return nil, errors.New("invalid message")
		}
		if c, ok := msg["content"]; ok && c != nil {
			if _, ok := c.(string); !ok {
				return nil, errors.New("UNSUPPORTED: only text message content")
			}
		}
	}
	return m, nil
}

func nextRequest(req map[string]any, previous *Turn, dialect, historyDialect string) error {
	ms := req["messages"].([]any)
	requestTail := historyDialect == JiuwenHistoryDialect || historyDialect == jiuwenRequestTailHistoryDialect
	jiuwenHistory := requestTail || historyDialect == jiuwenFixedTailHistoryDialect
	if requestTail && !jiuwenRequestTail(ms) {
		return errors.New("UNSUPPORTED: native request attachment shape")
	}
	if previous == nil {
		for i, item := range ms {
			role := item.(map[string]any)["role"]
			if i == len(ms)-1 {
				if role != "user" {
					return errors.New("UNSUPPORTED: initial user message required")
				}
			} else if role != "system" && role != "developer" && (role != "user" || !jiuwenHistory) {
				return errors.New("UNSUPPORTED: initial history")
			}
		}
		return nil
	}
	expectedCalls := previous.toolCalls()
	if len(expectedCalls) == 0 {
		return errors.New("UNSUPPORTED: request after final response")
	}
	old, _ := decode(previous.Request)
	oldMsgs := old.(map[string]any)["messages"].([]any)
	if len(ms) != len(oldMsgs)+1+len(expectedCalls) {
		return errors.New("UNSUPPORTED: history rewrite, compaction or extra messages")
	}
	insertion, resultsEnd := len(oldMsgs), len(ms)
	if jiuwenHistory {
		// Native PromptAttachmentManager appends fresh request-only context
		// to a copy of the history. Validate that tail separately from the
		// persistent prefix and new assistant/tool exchange. Every request,
		// including its entire attachment, remains frozen for strict replay.
		if requestTail {
			if !jiuwenRequestTail(oldMsgs) {
				return errors.New("UNSUPPORTED: native request attachment shape")
			}
		} else if len(oldMsgs) < 2 || oldMsgs[len(oldMsgs)-1].(map[string]any)["role"] != "user" ||
			digest(oldMsgs[len(oldMsgs)-1]) != digest(ms[len(ms)-1]) {
			return errors.New("UNSUPPORTED: fixed history tail was rewritten")
		}
		insertion--
		resultsEnd--
	}
	if digest(ms[:insertion]) != digest(oldMsgs[:insertion]) {
		return errors.New("UNSUPPORTED: history rewrite, compaction or extra messages")
	}
	a := ms[insertion].(map[string]any)
	calls, ok := a["tool_calls"].([]any)
	if a["role"] != "assistant" || !ok || len(calls) != len(expectedCalls) {
		return errors.New("UNSUPPORTED: assistant tool-call count mismatch")
	}
	expectedIDs := make(map[string]bool, len(expectedCalls))
	for i, expectedCall := range expectedCalls {
		c, ok := calls[i].(map[string]any)
		if !ok {
			return errors.New("invalid tool call")
		}
		f, ok := c["function"].(map[string]any)
		if !ok {
			return errors.New("invalid function call")
		}
		args, ok := f["arguments"].(string)
		if !ok {
			return errors.New("invalid arguments")
		}
		x, err := decode([]byte(args))
		if err != nil {
			return err
		}
		y, _ := decode([]byte(expectedCall.Arguments))
		if historyDialect == JiuwenHistoryDialect || historyDialect == JiuwenSubagentHistoryDialect {
			// Native extract_call_goal pops both top-level keys, even nonstrings.
			// y is freshly decoded for this causal check; stored SSE and requests
			// are untouched. Nested keys and every execution argument stay exact.
			if object, ok := y.(map[string]any); ok {
				delete(object, "call_goal")
				delete(object, "callGoal")
			}
		}
		expectedID, err := requestToolCallID(expectedCall.ID, dialect, oldMsgs)
		if err != nil {
			return err
		}
		if expectedIDs[expectedID] {
			return errors.New("UNSUPPORTED: colliding tool-call IDs within response")
		}
		expectedIDs[expectedID] = true
		if c["type"] != "function" || c["id"] != expectedID || f["name"] != expectedCall.Name || digest(x) != digest(y) {
			return errors.New("UNSUPPORTED: tool call causality mismatch")
		}
	}
	// Record the actual result order, but require one result for every exact ID.
	// Replay compares the entire recorded request unchanged, including this order.
	for _, item := range ms[insertion+1 : resultsEnd] {
		result := item.(map[string]any)
		id, ok := result["tool_call_id"].(string)
		if result["role"] != "tool" || !ok || !expectedIDs[id] {
			return errors.New("UNSUPPORTED: missing, duplicate or unknown tool result")
		}
		delete(expectedIDs, id)
		if _, ok := result["content"].(string); !ok {
			return errors.New("UNSUPPORTED: textual tool result required")
		}
	}
	var response streamResult
	for _, frame := range previous.Frames {
		if err := response.feed(frame.Data); err != nil {
			return err
		}
	}
	if previous.ResponseJSON != nil {
		var err error
		response, err = parseJSONCompletion(previous.ResponseJSON)
		if err != nil {
			return err
		}
	}
	content, _ := a["content"].(string)
	if content != response.text {
		return errors.New("UNSUPPORTED: assistant content was rewritten")
	}
	if response.reasoning != "" && a["reasoning_content"] != response.reasoning {
		return errors.New("UNSUPPORTED: reasoning history dialect")
	}
	return nil
}

// This is the pinned renderer's envelope, not a content filter. Attachment
// bodies may change between calls; no bytes are removed or rewritten.
const jiuwenAttachmentPrefix = "<system-reminder>\n" +
	"The following context is automatically attached for this model call only.\n" +
	"It may or may not be relevant to your tasks. Do not respond to it unless it highly relevant to your task.\n\n" +
	"<prompt-attachment type=\""

func jiuwenRequestTail(messages []any) bool {
	if len(messages) < 2 {
		return false
	}
	tail := messages[len(messages)-1].(map[string]any)
	content, ok := tail["content"].(string)
	return len(tail) == 2 && tail["role"] == "user" && ok &&
		strings.HasPrefix(content, jiuwenAttachmentPrefix) &&
		strings.HasSuffix(content, "\n</prompt-attachment>\n</system-reminder>")
}

func requestToolCallID(raw, dialect string, history []any) (string, error) {
	if dialect == "" {
		return raw, nil
	}
	if dialect != "openclaw-2026.7.1" || raw == "" || len(raw) > 40 {
		return "", errors.New("UNSUPPORTED: tool-call ID dialect or shape")
	}
	// Pinned OpenClaw strict mode strips punctuation. This subset needs neither
	// its special native-ID rules nor truncation, collision hashes or fallbacks.
	var id strings.Builder
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			id.WriteByte(c)
		case c == '_', c == '-':
		default:
			return "", errors.New("UNSUPPORTED: tool-call ID outside declared dialect subset")
		}
	}
	expected := id.String()
	if expected == "" {
		return "", errors.New("UNSUPPORTED: empty mapped tool-call ID")
	}
	for _, item := range history {
		message := item.(map[string]any)
		if message["role"] != "assistant" {
			continue
		}
		calls, _ := message["tool_calls"].([]any)
		for _, item := range calls {
			call, _ := item.(map[string]any)
			if call["id"] == expected {
				return "", errors.New("UNSUPPORTED: repeated or colliding mapped tool-call ID")
			}
		}
	}
	return expected, nil
}

// difference exposes only a location, never prompt text, tool output or credentials.
func difference(a, b any, path string) string {
	if digest(a) == digest(b) {
		return ""
	}
	if am, ok := a.(map[string]any); ok {
		if bm, ok := b.(map[string]any); ok {
			keys := make([]string, 0, len(am)+len(bm))
			seen := map[string]bool{}
			for k := range am {
				seen[k] = true
				keys = append(keys, k)
			}
			for k := range bm {
				if !seen[k] {
					keys = append(keys, k)
				}
			}
			sort.Strings(keys)
			for _, k := range keys {
				// Arbitrary object keys may themselves contain private data.
				p := path + "/field-" + digest(k)[:8]
				switch k {
				case "messages", "tools", "role", "content", "tool_calls", "tool_call_id", "function", "arguments", "parameters", "properties", "name", "type", "model", "temperature", "stream", "reasoning_content", "id":
					p = path + "/" + k
				}
				av, aok := am[k]
				bv, bok := bm[k]
				if aok != bok {
					return p
				}
				if d := difference(av, bv, p); d != "" {
					return d
				}
			}
		}
	}
	if aa, ok := a.([]any); ok {
		if ba, ok := b.([]any); ok {
			if len(aa) != len(ba) {
				return path + "/length"
			}
			for i := range aa {
				if d := difference(aa[i], ba[i], path+"/"+strconv.Itoa(i)); d != "" {
					return d
				}
			}
		}
	}
	if path == "" {
		return "/"
	}
	return path
}

package jiuwenswarm

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/harness"
	"github.com/hyscale-lab/aries/pkg/harness/internal/privatefiles"
)

// evidenceBuffer keeps complete retained lines on truncation, allowing literal
// redaction to remove a credential even when writes split it across chunks.
type evidenceBuffer struct {
	mu        sync.Mutex
	content   []byte
	max       int
	truncated bool
}

func (b *evidenceBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := min(len(p), b.max-len(b.content))
	b.content = append(b.content, p[:n]...)
	b.truncated = b.truncated || n < len(p)
	return len(p), nil
}
func (b *evidenceBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	content := b.content
	if b.truncated {
		content = content[:bytes.LastIndexByte(content, '\n')+1]
	}
	return bytes.Clone(content)
}
func (b *evidenceBuffer) Truncated() bool { b.mu.Lock(); defer b.mu.Unlock(); return b.truncated }

type frameEvidence struct {
	dir                       string
	credentials               *harness.Credentials
	enabled                   bool
	officialSpansEnabled      bool
	text, events              evidenceBuffer
	input, selected, retained int
	malformed                 bool
}

func newFrameEvidence(dir string, credentials *harness.Credentials, enabled bool) *frameEvidence {
	return &frameEvidence{dir: dir, credentials: credentials, enabled: enabled, text: evidenceBuffer{max: 32 << 20}, events: evidenceBuffer{max: 64 << 20}}
}

func (e *frameEvidence) consume(frame Frame) {
	e.input++
	var body struct {
		EventType string          `json:"event_type"`
		DeltaKind string          `json:"delta_kind"`
		Delta     json.RawMessage `json:"delta"`
		Result    json.RawMessage `json:"result"`
	}
	if json.Unmarshal(frame.Body, &body) != nil {
		e.malformed = true
		return
	}
	if frame.Source != "push" && body.EventType == "chat.delta" && body.DeltaKind == "text" {
		var text string
		if json.Unmarshal(body.Delta, &text) == nil {
			_, _ = e.text.Write([]byte(text))
		}
	}
	// Native stream_utils emits an empty chat.final when text was already
	// streamed, and a nonempty one for an answer without prior text deltas.
	if frame.Source != "push" && body.EventType == "chat.final" {
		var final struct {
			Content string `json:"content"`
		}
		if json.Unmarshal(body.Delta, &final) == nil && final.Content != "" {
			_, _ = e.text.Write([]byte(final.Content))
		}
	}
	if frame.Source != "push" && frame.IsFinal && len(body.Result) > 0 {
		var final struct {
			EventType string `json:"event_type"`
			Content   string `json:"content"`
		}
		if json.Unmarshal(body.Result, &final) == nil && final.EventType == "chat.final" && final.Content != "" {
			// A complete native final payload takes precedence over its deltas.
			e.text.content = nil
			e.text.truncated = false
			_, _ = e.text.Write([]byte(final.Content))
		}
	}
	if !e.enabled {
		return
	}
	// Model-token/context updates are not tool evidence and do not consume the
	// retention budget. Preserve actual ToolResult bytes, without normalization.
	if !frame.IsFinal && frame.Status != "failed" && body.EventType != "chat.error" && body.EventType != "chat.invocation_paused" && body.EventType != "chat.final" && body.EventType != "chat.file" && body.EventType != "chat.tool_call" && body.EventType != "chat.tool_result" && body.EventType != "chat.tool_update" {
		return
	}
	record, err := json.Marshal(struct {
		ReceivedAt string          `json:"controller_received_at"`
		Source     string          `json:"source_channel,omitempty"`
		Scope      string          `json:"measurement_scope"`
		Frame      json.RawMessage `json:"native_frame"`
	}{frame.ReceivedAt.Format("2006-01-02T15:04:05.999999999Z07:00"), frame.Source, "controller_sse_consumption_not_guest_execution", frame.Raw})
	if err != nil {
		e.malformed = true
		return
	}
	e.selected++
	wasTruncated := e.events.Truncated()
	_, _ = e.events.Write(append(e.credentials.Redact(record), '\n'))
	if !wasTruncated && !e.events.Truncated() {
		e.retained++
	}
}

func (e *frameEvidence) finish(protocolSucceeded bool) (core.ObserverResult, error) {
	if !e.enabled && !e.text.Truncated() {
		return core.ObserverResult{Status: core.StatusNotEnabled}, nil
	}
	report := core.ObserverResult{Status: core.StatusUnsupported, SampleCount: e.retained,
		Reason: "native SSE tool evidence only; guest invocation/process timing and complete producer coverage are not yet supplied"}
	var err error
	if e.events.Truncated() || e.text.Truncated() {
		err = errors.New("Jiuwen retained stream evidence exceeded its byte budget")
	}
	if e.malformed {
		err = errors.Join(err, errors.New("Jiuwen native evidence contained an invalid frame"))
	}
	var writeErr error
	if e.enabled {
		eventPath := filepath.Join(e.dir, "native-events.jsonl")
		writeErr = privatefiles.WriteArtifact(eventPath, e.events.Bytes())
		if writeErr == nil {
			report.LogPaths = append(report.LogPaths, eventPath)
		}
	}
	err = errors.Join(err, writeErr)
	index, _ := json.MarshalIndent(map[string]any{
		"schema_version": 1, "source": "jiuwenswarm.native_http_sse", "measurement_scope": "controller_sse_consumption_not_guest_execution",
		"input_frames": e.input, "selected_records": e.selected, "retained_records": e.retained, "retained_bytes": len(e.events.Bytes()),
		"truncated": e.events.Truncated(), "final_response_truncated": e.text.Truncated(), "native_observation_enabled": e.enabled, "protocol_succeeded": protocolSucceeded,
		"official_span_collection_enabled": e.officialSpansEnabled,
		"sequence_is_loss_denominator":     false, "producer_coverage": "unknown", "native_process_timing": "unavailable",
	}, "", "  ")
	indexPath := filepath.Join(e.dir, "native-events-index.json")
	writeErr = privatefiles.WriteArtifact(indexPath, index)
	if writeErr == nil {
		report.LogPaths = append(report.LogPaths, indexPath)
	}
	err = errors.Join(err, writeErr)
	if err != nil {
		report.Status = core.StatusFailed
		report.Error = err.Error()
	}
	return report, err
}

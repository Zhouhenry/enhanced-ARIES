// Package jiuwenswarm implements the pinned native AgentServer protocol.
package jiuwenswarm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/hyscale-lab/aries/pkg/performance"
)

const maxFrameBytes = 32 << 20

var (
	ErrIncomplete  = errors.New("Jiuwen stream ended without a successful terminal frame")
	ErrPaused      = errors.New("Jiuwen invocation requires user input")
	ErrAgentFailed = errors.New("Jiuwen agent reported failure")
)

// Frame preserves native fields and bytes. Sequence is not a loss denominator:
// the pinned server does not promise a distinct increasing value for every frame.
type Frame struct {
	ProtocolVersion string          `json:"protocol_version"`
	RequestID       string          `json:"request_id"`
	SessionID       string          `json:"session_id"`
	Sequence        int64           `json:"sequence"`
	IsFinal         bool            `json:"is_final"`
	Status          string          `json:"status"`
	Kind            string          `json:"response_kind"`
	Body            json.RawMessage `json:"body"`
	Raw             json.RawMessage `json:"-"`
	ReceivedAt      time.Time       `json:"-"`
	Source          string          `json:"-"` // chat or push; Raw remains the native wire envelope.
}

// Client owns only HTTP transport. Interrupt and Close do not prove that tools
// have stopped; the harness must confirm its contained process group is empty.
type Client struct {
	mu      sync.RWMutex
	base    string
	headers http.Header
	http    *http.Client
	closed  bool
}

func NewClient(endpoint string, headers http.Header) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("invalid Jiuwen AgentServer endpoint")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &Client{base: strings.TrimRight(endpoint, "/"), headers: headers.Clone(), http: &http.Client{
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

func (c *Client) Close() {
	c.mu.Lock()
	c.closed = true
	c.headers = nil
	c.mu.Unlock()
	c.http.CloseIdleConnections()
}

// Health is service liveness, not Flash, model or sandbox readiness.
func (c *Client) Health(ctx context.Context, requestID string) error {
	var data struct {
		Status string `json:"status"`
	}
	if err := c.unary(ctx, http.MethodGet, "/api/v1/health", requestID, "", nil, &data); err != nil {
		return err
	}
	if data.Status != "ready" {
		return errors.New("Jiuwen service is not ready")
	}
	return nil
}

// CreateSession uses an already registered native project. The server rejects a
// project_dir without project_id; it returns the actual, possibly prewarmed session.
func (c *Client) CreateSession(ctx context.Context, requestID, projectID string) (json.RawMessage, error) {
	var data json.RawMessage
	err := c.unary(ctx, http.MethodPost, "/api/v1/sessions", requestID, "", map[string]any{
		"mode": "agent", "project_id": projectID, "create_token": requestID,
	}, &data)
	return data, err
}

// Interrupt is an acknowledgement only. The caller retains runtime ownership.
func (c *Client) Interrupt(ctx context.Context, requestID, sessionID string) error {
	if sessionID == "" {
		return errors.New("Jiuwen session ID is required")
	}
	return c.unary(ctx, http.MethodPost, "/api/v1/chat/"+url.PathEscape(sessionID)+"/actions/interrupt", requestID, sessionID, map[string]string{"intent": "cancel"}, nil)
}

// Run subscribes to session pushes, then submits once and consumes native E2A SSE.
// onFrame is serialized across both streams and sees
// even failed or foreign frames before validation; evidence storage stays private
// to the harness and observation failures must not change native tool results.
// Push evidence covers frames consumed during Run. Native chat and push have no
// cross-stream drain barrier, so producer coverage remains unknown: chat success
// does not prove that every queued push reached this consumer.
func (c *Client) Run(ctx context.Context, requestID, sessionID, instruction string, onFrame func(Frame)) (err error) {
	if sessionID == "" || strings.TrimSpace(instruction) == "" {
		return errors.New("Jiuwen session and instruction are required")
	}
	ctx, span := performance.Begin(ctx, "jiuwen.chat")
	defer func() { span.End(err) }()
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	var frameMu sync.Mutex
	consume := func(raw []byte, source string) (Frame, error) {
		var frame Frame
		if json.Unmarshal(raw, &frame) != nil {
			return frame, errors.New("invalid Jiuwen SSE JSON")
		}
		frame.Raw = bytes.Clone(raw)
		frame.Source = source
		frameMu.Lock()
		defer frameMu.Unlock()
		frame.ReceivedAt = time.Now()
		if onFrame != nil {
			onFrame(frame)
		}
		return frame, nil
	}
	// Ordinary native subscribers have no ready handshake and do not own reverse RPC.
	push, err := c.request(ctx, http.MethodGet, "/api/v1/events/stream?session_id="+url.QueryEscape(sessionID), requestID, sessionID, nil, true)
	if err != nil {
		return err
	}
	media, _, err := mime.ParseMediaType(push.Header.Get("Content-Type"))
	if err != nil || media != "text/event-stream" {
		push.Body.Close()
		return errors.New("Jiuwen returned a non-SSE push response")
	}
	pushDone := make(chan struct{})
	go func() {
		defer close(pushDone)
		pushErr := readFrames(push.Body, func(raw []byte) error {
			frame, err := consume(raw, "push")
			if err != nil {
				return err
			}
			if frame.SessionID != sessionID {
				return errors.New("Jiuwen push session identity mismatch")
			}
			if frame.ProtocolVersion != "" && frame.ProtocolVersion != "1.0" {
				return errors.New("unsupported Jiuwen push protocol version")
			}
			return nil
		})
		if pushErr == nil {
			pushErr = errors.New("Jiuwen push subscription ended before Run completed")
		}
		cancel(pushErr)
	}()
	defer func() {
		cancel(nil)
		push.Body.Close()
		<-pushDone
		if cause := context.Cause(ctx); cause != context.Canceled && cause != nil {
			err = cause
		}
	}()
	observe := func(operation string) {
		performance.ObserveAt(ctx, performance.Event{Operation: operation, SourceComponent: "jiuwenswarm", MeasurementScope: "controller_http_sse_consumption", RequestInstanceID: requestID, AgentSessionID: sessionID}, time.Now())
	}
	observe("jiuwen.chat.request_started")
	response, err := c.request(ctx, http.MethodPost, "/api/v1/chat/completions", requestID, sessionID, map[string]any{
		"query": instruction, "session_id": sessionID, "mode": "agent", "enable_streaming": true,
	}, true)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	observe("jiuwen.chat.response_headers")
	media, _, err = mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || media != "text/event-stream" {
		return errors.New("Jiuwen returned a non-SSE chat response")
	}
	terminal := false
	firstFrame := true
	err = readFrames(response.Body, func(raw []byte) error {
		if firstFrame {
			observe("jiuwen.chat.first_sse")
			firstFrame = false
		}
		frame, err := consume(raw, "chat")
		if err != nil {
			return err
		}
		if frame.RequestID != requestID || (frame.SessionID != "" && frame.SessionID != sessionID) {
			return errors.New("Jiuwen response identity mismatch")
		}
		if frame.ProtocolVersion != "" && frame.ProtocolVersion != "1.0" {
			return errors.New("unsupported Jiuwen protocol version")
		}
		if terminal {
			return errors.New("Jiuwen sent a frame after the terminal response")
		}
		if frame.Status == "failed" {
			return ErrAgentFailed
		}
		var body struct {
			AwaitingUserInput bool   `json:"awaiting_user_input"`
			EventType         string `json:"event_type"`
		}
		if err := json.Unmarshal(frame.Body, &body); err != nil {
			return errors.New("invalid Jiuwen response body")
		}
		if body.EventType == "chat.error" {
			return ErrAgentFailed
		}
		if body.AwaitingUserInput || body.EventType == "chat.invocation_paused" {
			return ErrPaused
		}
		if frame.IsFinal {
			if frame.Status != "succeeded" || frame.Kind != "e2a.complete" {
				return ErrIncomplete
			}
			terminal = true
			observe("jiuwen.chat.terminal_received")
		}
		return nil
	})
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}
	if err != nil {
		return err
	}
	if !terminal {
		return ErrIncomplete
	}
	observe("jiuwen.chat.stream_drained")
	return nil
}

func (c *Client) unary(ctx context.Context, method, path, requestID, sessionID string, params any, output any) error {
	response, err := c.request(ctx, method, path, requestID, sessionID, params, false)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxFrameBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("read Jiuwen response failed")
	}
	if len(raw) > maxFrameBytes {
		return errors.New("Jiuwen response exceeds 32 MiB")
	}
	var envelope struct {
		RequestID string          `json:"request_id"`
		OK        bool            `json:"ok"`
		Data      json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return errors.New("invalid Jiuwen response envelope")
	}
	if envelope.RequestID != requestID {
		return errors.New("Jiuwen response identity mismatch")
	}
	if !envelope.OK {
		return ErrAgentFailed
	}
	if output == nil {
		return nil
	}
	// HTTP routes wrap the original native payload as data.result.
	var wrapped struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(envelope.Data, &wrapped); err != nil {
		return errors.New("invalid Jiuwen response data")
	}
	if len(wrapped.Result) > 0 {
		envelope.Data = wrapped.Result
	}
	if err := json.Unmarshal(envelope.Data, output); err != nil {
		return errors.New("invalid Jiuwen response result")
	}
	return nil
}

func (c *Client) request(ctx context.Context, method, path, requestID, sessionID string, params any, streaming bool) (*http.Response, error) {
	if requestID == "" {
		return nil, errors.New("Jiuwen request ID is required")
	}
	var body io.Reader
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return nil, errors.New("encode Jiuwen request failed")
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return nil, errors.New("construct Jiuwen request failed")
	}
	c.mu.RLock()
	closed := c.closed
	req.Header = c.headers.Clone()
	c.mu.RUnlock()
	if closed {
		return nil, errors.New("Jiuwen client is closed")
	}
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-Id", requestID)
	req.Header.Set("X-Channel-Id", "web")
	if sessionID != "" {
		req.Header.Set("X-Session-Id", sessionID)
	}
	if streaming {
		req.Header.Set("Accept", "text/event-stream")
	}
	response, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("Jiuwen HTTP transport failed; submission was not retried")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		response.Body.Close()
		return nil, fmt.Errorf("Jiuwen HTTP status %d", response.StatusCode)
	}
	return response, nil
}

func readFrames(reader io.Reader, consume func([]byte) error) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), maxFrameBytes+1)
	var data []byte
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if len(data) > 0 {
				if err := consume(data[:len(data)-1]); err != nil {
					return err
				}
				data = data[:0]
			}
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		if field != "data" {
			continue
		}
		value = strings.TrimPrefix(value, " ")
		if len(data)+len(value)+1 > maxFrameBytes {
			return errors.New("Jiuwen SSE frame exceeds 32 MiB")
		}
		data = append(data, value...)
		data = append(data, '\n')
	}
	if scanner.Err() != nil {
		return errors.New("Jiuwen SSE read failed or line exceeds 32 MiB")
	}
	if len(data) > 0 {
		return errors.New("Jiuwen SSE frame was truncated")
	}
	return nil
}

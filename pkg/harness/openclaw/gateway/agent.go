package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const agentResponseQueueSize = 4
const maxAgentResultBytes = 16 << 20

type AgentRequest struct {
	Message        string
	SessionKey     string
	SessionID      string
	IdempotencyKey string
	Thinking       string
}

type AgentResult struct {
	RunID string
	Text  string
}

func (client *Client) Agent(ctx context.Context, request AgentRequest) (result AgentResult, err error) {
	if strings.TrimSpace(request.Message) == "" || strings.ContainsRune(request.Message, 0) {
		return AgentResult{}, errors.New("gateway agent message is invalid")
	}
	if strings.TrimSpace(request.SessionKey) == "" {
		return AgentResult{}, errors.New("gateway agent session key is required")
	}
	if strings.TrimSpace(request.IdempotencyKey) == "" {
		return AgentResult{}, errors.New("gateway agent idempotency key is required")
	}
	client.mu.Lock()
	summary := client.connected
	transport := client.transport
	generation := client.generation
	readerFatal := client.readerFatal
	readerErr := client.readerErr
	client.mu.Unlock()
	if readerFatal {
		return AgentResult{}, readerErr
	}
	if summary == nil || transport == nil {
		return AgentResult{}, errors.New("gateway agent client is not connected")
	}
	if !summary.HasScope("operator.write") {
		return AgentResult{}, errors.New("gateway agent requires operator.write scope")
	}

	id := client.nextID("agent")
	acceptedRunID := ""
	defer func() { client.observeAgentReturned(ctx, id, request.SessionKey, acceptedRunID, err) }()
	reply := make(chan responseDelivery, agentResponseQueueSize)
	client.mu.Lock()
	if client.transport != transport || client.generation != generation || client.readerFatal {
		fatalErr := client.readerErr
		client.mu.Unlock()
		if fatalErr != nil {
			return AgentResult{}, fatalErr
		}
		return AgentResult{}, errors.New("gateway agent transport changed before submission")
	}
	client.pending[id] = reply
	client.mu.Unlock()
	defer client.removePending(id)

	params := map[string]any{
		"message": request.Message, "sessionKey": request.SessionKey,
		"idempotencyKey": request.IdempotencyKey,
	}
	if request.SessionID != "" {
		params["sessionId"] = request.SessionID
	}
	if request.Thinking != "" {
		params["thinking"] = request.Thinking
	}
	content, err := json.Marshal(Frame{"type": "req", "id": id, "method": "agent", "params": params})
	if err != nil {
		return AgentResult{}, fmt.Errorf("marshal gateway agent request: %w", err)
	}
	client.observeAgent(ctx, "gateway.agent.send_started", id, request.SessionKey, "", time.Now())
	sendErr := transport.Send(ctx, content)
	client.observeAgent(ctx, "gateway.agent.send_returned", id, request.SessionKey, "", time.Now())
	if err := sendErr; err != nil {
		if fatalErr := client.connectionFatal(generation); fatalErr != nil {
			return AgentResult{}, fmt.Errorf("gateway agent delivery is ambiguous after fatal send failure: %w", fatalErr)
		}
		return AgentResult{}, fmt.Errorf("gateway agent delivery is ambiguous after send failure: %w", err)
	}

	defer func() {
		// An acknowledged run remains real even when its terminal outcome is
		// unknown. Retain it so independent event evidence can be correlated.
		if result.RunID == "" {
			result.RunID = acceptedRunID
		}
	}()
	for {
		select {
		case delivery := <-reply:
			client.observeFrame(ctx, "gateway.response.consumed", delivery.frame, delivery.observationID, time.Now())
			if fatalErr := client.connectionFatal(generation); fatalErr != nil {
				if acceptedRunID == "" {
					return AgentResult{}, fmt.Errorf("gateway agent delivery is ambiguous before acknowledgement: %w", fatalErr)
				}
				return AgentResult{}, fmt.Errorf("gateway agent %s was accepted but its outcome is unknown: %w", acceptedRunID, fatalErr)
			}
			if delivery.err != nil {
				if acceptedRunID == "" {
					return AgentResult{}, fmt.Errorf("gateway agent delivery is ambiguous before acknowledgement: %w", delivery.err)
				}
				return AgentResult{}, fmt.Errorf("gateway agent %s was accepted but its outcome is unknown: %w", acceptedRunID, delivery.err)
			}
			response := delivery.frame
			if response.String("id") != id || response.String("type") != "res" {
				return AgentResult{}, errors.New("gateway agent received an uncorrelated response")
			}
			if !response.Bool("ok") {
				return AgentResult{}, ResponseError("agent", response)
			}
			payload := response.Map("payload")
			runID, _ := payload["runId"].(string)
			status, _ := payload["status"].(string)
			if acceptedRunID == "" {
				if status != "accepted" || strings.TrimSpace(runID) == "" {
					return AgentResult{}, errors.New("gateway agent expected accepted response with non-empty runId")
				}
				acceptedRunID = runID
				client.observeAgent(ctx, "gateway.agent.accepted", id, request.SessionKey, runID, time.Now())
				continue
			}
			if strings.TrimSpace(runID) == "" || runID != acceptedRunID {
				return AgentResult{}, errors.New("gateway agent terminal runId did not match accepted runId")
			}
			client.observeAgent(ctx, "gateway.agent.followup_received", id, request.SessionKey, runID, time.Now())
			text, err := parseAgentTerminal(payload)
			if err != nil {
				return AgentResult{}, err
			}
			client.observeAgent(ctx, "gateway.agent.terminal_validated", id, request.SessionKey, runID, time.Now())
			return AgentResult{RunID: runID, Text: text}, nil
		case <-ctx.Done():
			if acceptedRunID == "" {
				return AgentResult{}, fmt.Errorf("gateway agent delivery is ambiguous before acknowledgement: %w", ctx.Err())
			}
			return AgentResult{}, fmt.Errorf("gateway agent %s was accepted but its outcome is unknown: %w", acceptedRunID, ctx.Err())
		}
	}
}

func boundedDiagnostic(value any) string {
	text, _ := value.(string)
	text = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, text)
	text = strings.TrimSpace(text)
	if len(text) > 512 {
		text = text[:512] + "…"
	}
	return text
}

func parseAgentTerminal(payload map[string]any) (string, error) {
	status, _ := payload["status"].(string)
	if status != "ok" {
		return "", fmt.Errorf("gateway agent terminal status is %q, want %q", status, "ok")
	}
	result, ok := payload["result"].(map[string]any)
	if !ok {
		return "", errors.New("gateway agent terminal response missing result")
	}
	rawPayloads, ok := result["payloads"].([]any)
	if !ok {
		return "", errors.New("gateway agent terminal result missing payloads")
	}
	texts := make([]string, 0, len(rawPayloads))
	for _, raw := range rawPayloads {
		payload, ok := raw.(map[string]any)
		if !ok {
			return "", errors.New("gateway agent terminal result contains malformed payload")
		}
		text, ok := payload["text"].(string)
		if !ok {
			return "", errors.New("gateway agent terminal result payload missing text")
		}
		if text != "" {
			texts = append(texts, text)
		}
	}
	if len(texts) == 0 {
		return "", errors.New("gateway agent returned no payload text")
	}
	joined := strings.Join(texts, "\n")
	if len(joined) > maxAgentResultBytes {
		return "", errors.New("gateway agent result exceeded size bound")
	}
	return joined, nil
}

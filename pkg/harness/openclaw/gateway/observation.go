package gateway

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/hyscale-lab/aries/pkg/performance"
)

// Only opaque identities from agent traffic leave the protocol reader. Auth
// replies, challenge material, model text and command output are never copied.
func (client *Client) observedID(value string) string {
	if len(value) == 0 || len(value) > 512 || (client.token != "" && strings.Contains(value, client.token)) {
		return ""
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._:-/|", r)) {
			return ""
		}
	}
	return value
}

func (client *Client) observeAgent(ctx context.Context, operation, requestID, session, runID string, at time.Time) {
	performance.ObserveAt(ctx, performance.Event{Operation: operation,
		SourceComponent: "gateway", MeasurementScope: "controller_observation",
		RequestInstanceID: client.observationID + ":" + requestID,
		AgentSessionID:    client.observedID(session), NativeRunID: client.observedID(runID)}, at)
}

func (client *Client) observeFrame(ctx context.Context, operation string, frame Frame, deliveryID string, at time.Time) {
	if performance.ClockFromContext(ctx) == nil {
		return
	}
	event := performance.Event{GatewayDeliveryID: deliveryID, Operation: operation, SourceComponent: "gateway", MeasurementScope: "controller_observation"}
	if frame.String("type") == "res" && strings.HasPrefix(frame.String("id"), "agent-") {
		id := client.observedID(frame.String("id"))
		if id == "" {
			return
		}
		event.RequestInstanceID = client.observationID + ":" + id
		event.NativeRunID = client.observedID(Frame(frame.Map("payload")).String("runId"))
	} else if frame.String("type") == "event" && frame.String("event") == "agent" {
		p := Frame(frame.Map("payload"))
		event.NativeRunID, event.AgentSessionID = client.observedID(p.String("runId")), client.observedID(p.String("sessionKey"))
		event.ToolCallID = client.observedID(Frame(p.Map("data")).String("toolCallId"))
		if seq, ok := p["seq"].(float64); ok && seq >= 1 && seq <= 9007199254740991 && math.Trunc(seq) == seq {
			n := int64(seq)
			event.NativeSequence = &n
		}
	} else {
		return
	}
	performance.ObserveAt(ctx, event, at)
}

// returned describes the local call, never proof of a remote process exit.
func (client *Client) observeAgentReturned(ctx context.Context, requestID, session, runID string, err error) {
	status := "succeeded"
	if errors.Is(err, context.DeadlineExceeded) {
		status = "timeout"
	} else if errors.Is(err, context.Canceled) {
		status = "canceled"
	} else if err != nil {
		status = "failed"
	}
	performance.ObserveAt(ctx, performance.Event{Operation: "gateway.agent.returned", SourceComponent: "gateway", MeasurementScope: "controller_observation", RequestInstanceID: client.observationID + ":" + requestID, AgentSessionID: client.observedID(session), NativeRunID: client.observedID(runID), Status: status}, time.Now())
}

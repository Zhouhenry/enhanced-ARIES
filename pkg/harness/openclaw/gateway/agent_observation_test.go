package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/performance"
)

func observedAgentClient(t *testing.T, disposition ...EventDisposition) (*Client, *fakeTransport, context.Context, func() ([]performance.Event, []byte)) {
	t.Helper()
	directory := t.TempDir()
	recorder, err := performance.Open(directory, "gateway-observation-test")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(performance.WithRecorder(context.Background(), recorder), 5*time.Second)
	transport := newFakeTransport()
	options := Options{Token: "gateway-private-token"}
	if len(disposition) != 0 {
		options.EventDisposition = disposition[0]
	}
	client, err := New(func(context.Context) (Transport, error) { return transport, nil }, options)
	if err != nil {
		cancel()
		_ = recorder.Close()
		t.Fatal(err)
	}
	client.transport = transport
	client.connected = &ConnectSummary{Role: "operator", Scopes: []string{"operator.write", "operator.read"}}
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		client.readLoop(ctx, transport)
	}()
	var once sync.Once
	finish := func() {
		once.Do(func() {
			_ = client.Close()
			cancel()
			<-readerDone
			if err := recorder.Close(); err != nil {
				t.Errorf("close observation: %v", err)
			}
		})
	}
	t.Cleanup(finish)
	return client, transport, ctx, func() ([]performance.Event, []byte) {
		finish()
		content, err := os.ReadFile(filepath.Join(directory, "performance", "runner-events.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		var events []performance.Event
		for _, line := range bytes.Split(bytes.TrimSpace(content), []byte{'\n'}) {
			if len(line) == 0 {
				continue
			}
			var event performance.Event
			if err := json.Unmarshal(line, &event); err != nil {
				t.Fatal(err)
			}
			events = append(events, event)
		}
		return events, content
	}
}

func TestResponseOnlyObservationDoesNotInventMissingNativeConsumers(t *testing.T) {
	client, transport, ctx, recorded := observedAgentClient(t, EventDispositionResponseOnly)
	done := make(chan error, 1)
	go func() { _, err := client.Agent(ctx, validAgentRequest()); done <- err }()
	id := transport.nextSent(t).String("id")
	transport.deliver(acceptedFrame(id, "run-observed"))
	transport.deliver(Frame{"type": "event", "event": "agent", "payload": map[string]any{"runId": "run-observed", "sessionKey": "native-session", "seq": float64(1)}})
	transport.deliver(terminalFrame(id, "run-observed", "ok", []any{map[string]any{"text": "done"}}))
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	events, _ := recorded()
	received := 0
	for _, event := range events {
		if event.Operation == "gateway.frame.received" {
			received++
			if event.RequestInstanceID == "" {
				t.Fatal("discarded native event represented as an expected delivery")
			}
		}
	}
	if received != 2 {
		t.Fatalf("response deliveries=%d", received)
	}
}

func TestAgentObservationSeparatesDeliveriesAndExcludesPrivatePayloads(t *testing.T) {
	client, transport, ctx, recorded := observedAgentClient(t)
	request := validAgentRequest()
	request.Message = "private-agent-message"
	done := make(chan error, 1)
	go func() {
		_, err := client.Agent(ctx, request)
		done <- err
	}()
	id := transport.nextSent(t).String("id")
	transport.deliver(Frame{"type": "res", "id": "connect-private", "ok": true, "payload": map[string]any{"auth": "private-auth-material", "runId": "private-auth-run"}})
	transport.deliver(Frame{"type": "event", "event": "connect.challenge", "payload": map[string]any{"nonce": "private-challenge"}})
	transport.deliver(acceptedFrame(id, "run-observed"))
	transport.deliver(Frame{"type": "event", "event": "agent", "payload": map[string]any{
		"runId": "run-observed", "sessionKey": request.SessionKey, "seq": float64(7),
		"data": map[string]any{"toolCallId": "tool-observed", "args": map[string]any{"command": "private-tool-command"}, "output": "gateway-private-token"},
	}})
	transport.deliver(terminalFrame(id, "run-observed", "ok", []any{map[string]any{"text": "private-agent-result"}}))
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := client.RecvEvent(ctx); err != nil {
		t.Fatal(err)
	}
	events, content := recorded()
	for _, secret := range []string{"gateway-private-token", "private-auth-material", "private-auth-run", "private-challenge", request.Message, "private-tool-command", "private-agent-result"} {
		if bytes.Contains(content, []byte(secret)) {
			t.Fatalf("observer retained private payload %q", secret)
		}
	}
	deliveries := make(map[string]map[string]performance.Event)
	counts := make(map[string]int)
	for _, event := range events {
		counts[event.Operation]++
		if event.SourceComponent != "gateway" || event.MeasurementScope != "controller_observation" {
			t.Fatalf("wrong observation scope: %+v", event)
		}
		if event.GatewayDeliveryID != "" {
			if deliveries[event.GatewayDeliveryID] == nil {
				deliveries[event.GatewayDeliveryID] = make(map[string]performance.Event)
			}
			if _, exists := deliveries[event.GatewayDeliveryID][event.Operation]; exists {
				t.Fatalf("two frames reused delivery identity %q", event.GatewayDeliveryID)
			}
			deliveries[event.GatewayDeliveryID][event.Operation] = event
		}
		if event.Operation == "gateway.agent.returned" && event.Status != "succeeded" {
			t.Fatalf("local outcome = %q", event.Status)
		}
	}
	if len(deliveries) != 3 || counts["gateway.agent.accepted"] != 1 || counts["gateway.agent.terminal_validated"] != 1 || counts["gateway.agent.returned"] != 1 {
		t.Fatalf("deliveries=%d operations=%v", len(deliveries), counts)
	}
	responseCount, nativeCount := 0, 0
	for deliveryID, stages := range deliveries {
		received, ok := stages["gateway.frame.received"]
		if !ok || len(stages) != 3 {
			t.Fatalf("incomplete delivery %q: %+v", deliveryID, stages)
		}
		prefix := "gateway.response."
		if received.RequestInstanceID == "" {
			prefix = "gateway.event."
			nativeCount++
			if received.ToolCallID != "tool-observed" || received.AgentSessionID != request.SessionKey || received.NativeSequence == nil || *received.NativeSequence != 7 {
				t.Fatalf("native identity lost: %+v", received)
			}
		} else {
			responseCount++
			if received.RequestInstanceID != client.observationID+":"+id {
				t.Fatalf("wrong request instance: %+v", received)
			}
		}
		enqueued, offered := stages[prefix+"enqueue_offer"]
		consumed, taken := stages[prefix+"consumed"]
		if !offered || !taken || received.ObservedOffsetNS > enqueued.ObservedOffsetNS || enqueued.ObservedOffsetNS > consumed.ObservedOffsetNS {
			t.Fatalf("delivery boundaries invalid: %+v", stages)
		}
		for _, stage := range stages {
			if stage.NativeRunID != "run-observed" || stage.ClockDomain != received.ClockDomain || stage.RequestInstanceID != received.RequestInstanceID {
				t.Fatalf("delivery identity changed: %+v", stages)
			}
		}
	}
	if responseCount != 2 || nativeCount != 1 {
		t.Fatalf("responses=%d native=%d", responseCount, nativeCount)
	}
}

func TestAgentObservationDoesNotTreatLocalFailureAsRemoteTerminal(t *testing.T) {
	for _, mode := range []string{"duplicate-accepted", "send-failed", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			client, transport, parent, recorded := observedAgentClient(t)
			ctx, cancel := context.WithCancel(parent)
			defer cancel()
			if mode == "send-failed" {
				transport.sendErr = errors.New("private-transport-diagnostic")
			}
			done := make(chan error, 1)
			go func() {
				_, err := client.Agent(ctx, validAgentRequest())
				done <- err
			}()
			if mode != "send-failed" {
				id := transport.nextSent(t).String("id")
				if mode == "duplicate-accepted" {
					transport.deliver(acceptedFrame(id, "run-accepted"))
					transport.deliver(acceptedFrame(id, "run-accepted"))
				} else {
					cancel()
				}
			}
			if err := <-done; err == nil || mode == "canceled" && !errors.Is(err, context.Canceled) {
				t.Fatalf("Agent error = %v", err)
			}
			events, content := recorded()
			counts := make(map[string]int)
			for _, event := range events {
				counts[event.Operation]++
				if event.Operation == "gateway.agent.returned" {
					wantStatus, wantRun := "failed", ""
					if mode == "canceled" {
						wantStatus = "canceled"
					}
					if mode == "duplicate-accepted" {
						wantRun = "run-accepted"
					}
					if event.Status != wantStatus || event.NativeRunID != wantRun {
						t.Fatalf("local return misreported: %+v", event)
					}
				}
			}
			if counts["gateway.agent.terminal_validated"] != 0 || counts["gateway.agent.returned"] != 1 || counts["gateway.agent.send_started"] != 1 || counts["gateway.agent.send_returned"] != 1 {
				t.Fatalf("wrong lifecycle observations: %v", counts)
			}
			wantAccepted := 0
			if mode == "duplicate-accepted" {
				wantAccepted = 1
			}
			if counts["gateway.agent.accepted"] != wantAccepted || counts["gateway.agent.followup_received"] != wantAccepted || bytes.Contains(content, []byte("private-transport-diagnostic")) {
				t.Fatalf("unvalidated response or error leaked: %v", counts)
			}
		})
	}
}

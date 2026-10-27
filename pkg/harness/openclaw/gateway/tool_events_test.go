package gateway

import (
	"context"
	"testing"
	"time"
)

func TestNativeToolEventCapabilityIsExplicit(t *testing.T) {
	if _, err := New(func(context.Context) (Transport, error) { return nil, nil }, Options{ToolEvents: true, EventDisposition: EventDispositionResponseOnly}); err == nil {
		t.Fatal("requested tool events would be silently discarded")
	}
	for _, enabled := range []bool{false, true} {
		transport := newFakeTransport(Frame{"type": "event", "event": "connect.challenge", "payload": map[string]any{"nonce": "n"}})
		client, err := New(func(context.Context) (Transport, error) { return transport, nil }, Options{ToolEvents: enabled})
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan struct{})
		go func() {
			defer close(done)
			sent := transport.nextSent(t)
			caps, ok := sent.Map("params")["caps"].([]any)
			if !ok || (enabled && (len(caps) != 1 || caps[0] != "tool-events")) || (!enabled && len(caps) != 0) {
				t.Errorf("tool event caps=%v enabled=%v", caps, enabled)
			}
			transport.deliver(Frame{"type": "res", "id": sent["id"], "ok": true, "payload": map[string]any{"auth": map[string]any{"scopes": []any{"operator.write"}}}})
		}()
		_, err = client.Connect(context.Background(), ConnectOptions{Timeout: time.Second})
		<-done
		if err != nil {
			t.Fatal(err)
		}
		if err := client.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFailedNativeAgentRetainsOnlyAcknowledgedRunIdentity(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		transport := newFakeTransport()
		client := connectedAgentClient(t, transport, "operator.write")
		done := make(chan AgentResult, 1)
		go func() {
			result, err := client.Agent(context.Background(), validAgentRequest())
			if err == nil {
				t.Error("terminal error accepted")
			}
			done <- result
		}()
		sent := transport.nextSent(t)
		if accepted {
			transport.deliver(acceptedFrame(sent.String("id"), "actual-native-run"))
		}
		transport.deliver(terminalFrame(sent.String("id"), "different-run", "error", nil))
		result := <-done
		want := ""
		if accepted {
			want = "actual-native-run"
		}
		if result.RunID != want {
			t.Fatalf("actual acknowledged run=%q want=%q", result.RunID, want)
		}
		if err := client.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

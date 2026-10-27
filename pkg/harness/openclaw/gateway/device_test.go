package gateway

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestClientDeviceProofBindsNativeV3FieldsAndFreshChallengeAcrossClose(t *testing.T) {
	device, err := NewDeviceIdentity()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(device.Destroy)
	publicKey, err := base64.RawURLEncoding.DecodeString(device.PublicKey())
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		t.Fatal("device public key is not raw Ed25519 base64url")
	}
	fingerprint := sha256.Sum256(publicKey)
	if device.ID() != hex.EncodeToString(fingerprint[:]) {
		t.Fatal("device ID does not fingerprint the raw public key")
	}
	transports := []*fakeTransport{
		newFakeTransport(Frame{"type": "event", "event": "connect.challenge", "payload": map[string]any{"nonce": "first-challenge"}}),
		newFakeTransport(Frame{"type": "event", "event": "connect.challenge", "payload": map[string]any{"nonce": "second-challenge"}}),
	}
	dials := 0
	client, err := New(func(context.Context) (Transport, error) {
		if dials >= len(transports) {
			return nil, errors.New("unexpected additional dial")
		}
		transport := transports[dials]
		dials++
		return transport, nil
	}, Options{Token: "test-gateway-token", Device: device})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	var firstSignature []byte
	for index, transport := range transports {
		expectedNonce := []string{"first-challenge", "second-challenge"}[index]
		started := time.Now().UnixMilli()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		t.Cleanup(cancel)
		type outcome struct {
			summary ConnectSummary
			err     error
		}
		done := make(chan outcome, 1)
		go func() {
			summary, connectErr := client.Connect(ctx, ConnectOptions{Timeout: time.Second})
			done <- outcome{summary, connectErr}
		}()
		sent := transport.nextSent(t)
		params := sent.Map("params")
		proof := Map(params["device"])
		metadata := Map(params["client"])
		scopes, ok := params["scopes"].([]any)
		if sent.String("method") != "connect" || params["role"] != "operator" || !ok || len(scopes) != 1 || scopes[0] != "operator.write" {
			t.Fatal("device authentication changed the requested method, role, or scopes")
		}
		if metadata["id"] != "gateway-client" || metadata["mode"] != "backend" || metadata["platform"] != "go" {
			t.Fatal("unexpected native client metadata")
		}
		if proof["id"] != device.ID() || proof["publicKey"] != device.PublicKey() || proof["nonce"] != expectedNonce {
			t.Fatal("device proof did not bind the retained identity and current challenge")
		}
		signedAt, ok := proof["signedAt"].(float64)
		if !ok || signedAt != float64(int64(signedAt)) || int64(signedAt) < started || int64(signedAt) > time.Now().UnixMilli() {
			t.Fatal("device proof timestamp is not current Unix milliseconds")
		}
		signatureText, ok := proof["signature"].(string)
		if !ok {
			t.Fatal("device signature has an invalid type")
		}
		signature, err := base64.RawURLEncoding.DecodeString(signatureText)
		if err != nil || len(signature) != ed25519.SignatureSize {
			t.Fatal("device signature is not raw Ed25519 base64url")
		}
		// Reconstruct the native wire contract independently of DeviceIdentity.proof.
		fields := []string{"v3", device.ID(), "gateway-client", "backend", "operator", "operator.write",
			strconv.FormatInt(int64(signedAt), 10), "test-gateway-token", expectedNonce, "go", ""}
		payload := []byte(strings.Join(fields, "|"))
		if !ed25519.Verify(publicKey, payload, signature) {
			t.Fatal("signature does not verify against the native v3 payload")
		}
		for _, field := range []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10} {
			changed := append([]string(nil), fields...)
			changed[field] += "-changed"
			if ed25519.Verify(publicKey, []byte(strings.Join(changed, "|")), signature) {
				t.Fatalf("signature did not bind native payload field %d", field)
			}
		}
		if index == 0 {
			firstSignature = append([]byte(nil), signature...)
		} else if ed25519.Verify(publicKey, payload, firstSignature) {
			t.Fatal("a prior challenge signature authenticated the new connection")
		}
		transport.deliver(Frame{"type": "res", "id": sent.String("id"), "ok": true,
			"payload": map[string]any{"auth": map[string]any{"role": "operator",
				"scopes": []any{"operator.read", "operator.write"}, "deviceToken": "private-returned-token"}}})
		result := <-done
		cancel()
		if result.err != nil || !result.summary.HasScope("operator.read") || !result.summary.HasScope("operator.write") {
			t.Fatalf("paired connect failed: %v", result.err)
		}
		encoded, err := json.Marshal(result.summary)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"test-gateway-token", "private-returned-token", expectedNonce, signatureText} {
			if strings.Contains(string(encoded), secret) {
				t.Fatal("connect summary exposed authentication material")
			}
		}
		if err := client.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if dials != 2 {
		t.Fatal("unexpected connection retry")
	}
}

func TestDeviceDestroyClearsKeyAndPreventsFurtherSigning(t *testing.T) {
	device, err := NewDeviceIdentity()
	if err != nil {
		t.Fatal(err)
	}
	key := device.privateKey
	id, publicKey := device.ID(), device.PublicKey()
	device.Destroy()
	device.Destroy()
	for _, value := range key {
		if value != 0 {
			t.Fatal("destroy left private key bytes in the owned buffer")
		}
	}
	if device.ID() != id || device.PublicKey() != publicKey {
		t.Fatal("destroy changed public pairing metadata")
	}
	for _, identity := range []*DeviceIdentity{device, new(DeviceIdentity)} {
		if _, err := identity.proof(DefaultRole, defaultScopes, "test-token", "test-nonce", 1); err == nil {
			t.Fatal("unavailable identity could still produce a signature")
		}
	}
}

func TestClientPairingErrorExposesOnlyValidatedRequestID(t *testing.T) {
	for _, test := range []struct {
		name       string
		requestID  any
		code       string
		detailCode string
		wantTyped  bool
	}{
		{name: "native-detail", requestID: "8ad5eb11-e541-4cbf-8ae0-1f9e2822c342", detailCode: "PAIRING_REQUIRED", wantTyped: true},
		{name: "legacy-code", requestID: "pair-1", code: "NOT_PAIRED", wantTyped: true},
		{name: "missing", detailCode: "PAIRING_REQUIRED"},
		{name: "empty", requestID: "", detailCode: "PAIRING_REQUIRED"},
		{name: "number", requestID: 123, detailCode: "PAIRING_REQUIRED"},
		{name: "object", requestID: map[string]any{"token": "secret-canary"}, detailCode: "PAIRING_REQUIRED"},
		{name: "newline", requestID: "pair\nsecret-canary", detailCode: "PAIRING_REQUIRED"},
		{name: "oversized", requestID: strings.Repeat("a", 129), detailCode: "PAIRING_REQUIRED"},
		{name: "unrelated-rejection", requestID: "pair-1", code: "UNAUTHORIZED"},
	} {
		t.Run(test.name, func(t *testing.T) {
			transport := newFakeTransport(Frame{"type": "event", "event": "connect.challenge", "payload": map[string]any{"nonce": "test-challenge"}})
			dials := 0
			client, err := New(func(context.Context) (Transport, error) {
				dials++
				return transport, nil
			}, Options{Token: "secret-canary"})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := client.Connect(ctx, ConnectOptions{Timeout: time.Second})
				done <- err
			}()
			sent := transport.nextSent(t)
			transport.deliver(Frame{"type": "res", "id": sent.String("id"), "ok": false,
				"error": map[string]any{"code": test.code, "message": "secret-canary", "details": map[string]any{
					"code": test.detailCode, "requestId": test.requestID, "token": "secret-canary", "signature": "secret-canary"}}})
			err = <-done
			var pairing *PairingRequiredError
			if err == nil || errors.As(err, &pairing) != test.wantTyped {
				t.Fatalf("unexpected pairing error type: %T", err)
			}
			if test.wantTyped && pairing.RequestID != test.requestID {
				t.Fatal("pairing request identity changed")
			}
			if strings.Contains(fmt.Sprint(err), "secret-canary") {
				t.Fatal("pairing error exposed server authentication details")
			}
			if dials != 1 {
				t.Fatal("pairing rejection retried authentication")
			}
		})
	}
}

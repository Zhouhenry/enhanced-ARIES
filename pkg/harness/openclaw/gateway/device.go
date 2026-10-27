package gateway

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"sync"
)

// DeviceIdentity is an attempt-owned, in-memory native Gateway identity.
// Client.Close leaves it available for the reconnect following pairing.
type DeviceIdentity struct {
	id         string
	publicKey  string
	mu         sync.Mutex
	privateKey ed25519.PrivateKey
}

func NewDeviceIdentity() (*DeviceIdentity, error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, errors.New("generate gateway device identity")
	}
	id := sha256.Sum256(publicKey)
	return &DeviceIdentity{id: hex.EncodeToString(id[:]),
		publicKey: base64.RawURLEncoding.EncodeToString(publicKey), privateKey: privateKey}, nil
}

func (device *DeviceIdentity) ID() string        { return device.id }
func (device *DeviceIdentity) PublicKey() string { return device.publicKey }

// Destroy clears the private key after the owning attempt no longer needs to connect.
func (device *DeviceIdentity) Destroy() {
	device.mu.Lock()
	defer device.mu.Unlock()
	clear(device.privateKey)
	device.privateKey = nil
}

type deviceProof struct {
	ID        string `json:"id"`
	PublicKey string `json:"publicKey"`
	Signature string `json:"signature"`
	SignedAt  int64  `json:"signedAt"`
	Nonce     string `json:"nonce"`
}

func (device *DeviceIdentity) proof(role string, scopes []string, token, nonce string, signedAt int64) (deviceProof, error) {
	device.mu.Lock()
	defer device.mu.Unlock()
	if len(device.privateKey) != ed25519.PrivateKeySize {
		return deviceProof{}, errors.New("gateway device identity is unavailable")
	}
	// OpenClaw 2026.7.1 packages/gateway-client/src/device-auth.ts: v3 uses
	// the exact requested scopes, normalized platform, and an empty device family.
	payload := strings.Join([]string{"v3", device.id, nativeClientID, nativeClientMode, role,
		strings.Join(scopes, ","), strconv.FormatInt(signedAt, 10), token, nonce, nativeClientPlatform, ""}, "|")
	signature := ed25519.Sign(device.privateKey, []byte(payload))
	return deviceProof{ID: device.id, PublicKey: device.publicKey,
		Signature: base64.RawURLEncoding.EncodeToString(signature), SignedAt: signedAt, Nonce: nonce}, nil
}

// PairingRequiredError carries only the native request identity needed to
// select and verify the exact pending request. Server auth details stay private.
type PairingRequiredError struct {
	RequestID string
}

func (*PairingRequiredError) Error() string {
	return "connect failed: gateway pairing is required"
}

func pairingError(response Frame) error {
	requestID, ok := Map(response.Map("error")["details"])["requestId"].(string)
	if !ok || requestID == "" || len(requestID) > 128 || strings.IndexFunc(requestID, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_')
	}) >= 0 {
		return errors.New("connect failed: gateway pairing request identity is invalid")
	}
	return &PairingRequiredError{RequestID: requestID}
}

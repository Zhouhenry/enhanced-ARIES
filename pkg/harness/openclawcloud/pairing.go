package openclawcloud

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/harness/openclaw/gateway"
)

type deviceRecord struct {
	RequestID  string   `json:"requestId"`
	DeviceID   string   `json:"deviceId"`
	PublicKey  string   `json:"publicKey"`
	Role       string   `json:"role"`
	Roles      []string `json:"roles"`
	Scopes     []string `json:"scopes"`
	ClientID   string   `json:"clientId"`
	ClientMode string   `json:"clientMode"`
}

type deviceList struct {
	Pending []deviceRecord `json:"pending"`
	Paired  []deviceRecord `json:"paired"`
}

func (d deviceRecord) matches(identity *gateway.DeviceIdentity) bool {
	return d.DeviceID == identity.ID() && d.PublicKey == identity.PublicKey() &&
		d.Role == "operator" && len(d.Roles) == 1 && d.Roles[0] == "operator" &&
		d.ClientID == "gateway-client" && d.ClientMode == "backend" &&
		len(d.Scopes) == 1 && d.Scopes[0] == "operator.write"
}

// The native CLI reads its token from the root-owned launch specification.
// Sending that environment through sandboxprocess.Group.Command would put it in argv.
func (e *Embedded) devices(ctx context.Context, args ...string) (string, error) {
	payload, err := json.Marshal(args)
	if err != nil {
		return "", err
	}
	result, err := e.group.Sandbox().Exec(ctx, core.Command{
		Path: "python3", Args: []string{"-I", "-c", groupScript, "devices", e.group.ControlDir(), e.group.CgroupPath(), string(payload)},
		Dir: "/", User: "root", Timeout: 20 * time.Second, OutputLimitBytes: 1 << 20,
	})
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return "", errors.New("native device control command failed")
	}
	return result.Stdout, nil
}

func (e *Embedded) listDevices(ctx context.Context) (deviceList, error) {
	output, err := e.devices(ctx, "list")
	if err != nil {
		return deviceList{}, err
	}
	var list deviceList
	if err := json.Unmarshal([]byte(output), &list); err != nil {
		return deviceList{}, errors.New("native device list is invalid")
	}
	return list, nil
}

func (e *Embedded) approveDevice(ctx context.Context, requestID string, identity *gateway.DeviceIdentity) error {
	listed, err := e.listDevices(ctx)
	if err != nil {
		return err
	}
	matched := 0
	for _, pending := range listed.Pending {
		if pending.RequestID != requestID {
			continue
		}
		matched++
		if !pending.matches(identity) {
			return errors.New("native pairing request does not match controller identity and scopes")
		}
	}
	if matched != 1 {
		return errors.New("native pairing request is missing or ambiguous")
	}
	output, err := e.devices(ctx, "approve", requestID)
	if err != nil {
		return err
	}
	var approved struct {
		RequestID string       `json:"requestId"`
		Device    deviceRecord `json:"device"`
	}
	// Native CLI fallback may approve a replacement request; never accept that
	// as confirmation of the exact request inspected above.
	if err := json.Unmarshal([]byte(output), &approved); err != nil || approved.RequestID != requestID || !approved.Device.matches(identity) {
		return errors.New("native pairing approval does not match the selected request")
	}
	listed, err = e.listDevices(ctx)
	if err != nil {
		return err
	}
	for _, pending := range listed.Pending {
		if pending.RequestID == requestID || pending.DeviceID == identity.ID() {
			return errors.New("native pairing approval is unconfirmed")
		}
	}
	matched = 0
	for _, paired := range listed.Paired {
		if paired.DeviceID == identity.ID() {
			if !paired.matches(identity) {
				return errors.New("native paired device does not match controller identity and scopes")
			}
			matched++
		}
	}
	if matched != 1 {
		return errors.New("native paired device is missing or ambiguous")
	}
	return nil
}

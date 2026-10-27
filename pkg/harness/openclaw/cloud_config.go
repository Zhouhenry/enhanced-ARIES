package openclaw

import (
	"encoding/json"
	"errors"
	"path"
	"strings"

	"github.com/hyscale-lab/aries/pkg/core"
)

// WorkloadInputs are native inputs for repeated runs in fresh, isolated Gateways.
// They are unrelated to ARIES attempt identities, credentials, and idempotency keys.
type WorkloadInputs struct {
	SessionKey        string
	SessionID         string
	EnvelopeTimestamp string
	SkipBootstrap     bool
	// The candidate image supplies this renderer. This version marker must not
	// change native tool policy, schemas, parameters or background behavior.
	ToolOutputRenderer string
	// Explicit native candidate protocol; omitted inputs preserve native behavior.
	ReplayToolProtocol string
}

// RenderLocalConfig keeps the pinned model/Gateway configuration and moves
// native exec into the Gateway's own Linux environment. The caller must contain
// the Gateway and its descendants before granting it a writable workspace.
func RenderLocalConfig(model core.ModelConfig, workdir string, inputs *WorkloadInputs) ([]byte, error) {
	if inputs != nil && inputs.ReplayToolProtocol != "" {
		return nil, errors.New("OpenClaw replay tool protocol requires external placement")
	}
	if !path.IsAbs(workdir) || path.Clean(workdir) != workdir || strings.ContainsRune(workdir, 0) {
		return nil, errors.New("OpenClaw local workspace must be absolute and clean")
	}
	// The shared renderer validates the SSH endpoint before model rendering;
	// this private placeholder is removed before any configuration is returned.
	configuration, err := renderConfig(model, inputs, core.ToolEndpoint{
		Protocol: "ssh", Username: "aries", Address: "127.0.0.1:1",
		ClientCommand: "/opt/aries/bin/aries-ssh-client", ClientSourceFile: "/unused",
	}, ModeAgent, "", false, false, false, 0)
	if err != nil {
		return nil, err
	}
	var config map[string]any
	if err := json.Unmarshal(configuration, &config); err != nil {
		return nil, err
	}
	defaults := config["agents"].(map[string]any)["defaults"].(map[string]any)
	defaults["sandbox"] = map[string]any{"mode": "off"}
	defaults["workspace"] = workdir
	tools := config["tools"].(map[string]any)
	// The first A/B workload uses exec in both placements. File tools are
	// still denied, so differences are placement/transport rather than tool set.
	tools["exec"] = map[string]any{"host": "gateway", "mode": "full"}
	tools["elevated"] = map[string]any{"enabled": false}
	return json.MarshalIndent(config, "", "  ")
}

// GatewayThinking uses the same task-level setting as the existing harness.
func GatewayThinking(model core.ModelConfig) string { return requestedThinking(model) }

// GatewayScripts returns the existing authenticated loopback Gateway launcher
// and TCP forwarder. Authentication remains in the native Gateway protocol.
func GatewayScripts() (launcher, proxy []byte) {
	launcher = []byte(strings.Replace(string(gatewayLauncherScript()), "openclaw gateway run", `"$ARIES_OPENCLAW_EXECUTABLE" gateway run`, 1))
	return launcher, gatewayProxyScript()
}

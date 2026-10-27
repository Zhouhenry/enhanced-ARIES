package openclawcloud

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/harness/openclaw/gateway"
)

const pairingRequestID = "c8b95bb6-9505-4bdb-9d57-b89f18eab3e2"

type deviceStep struct {
	args   []string
	result core.CommandResult
	err    error
}

func deviceJSON(t *testing.T, value any) core.CommandResult {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return core.CommandResult{Stdout: string(data)}
}

func deviceApproval(t *testing.T, requestID string, paired deviceRecord) core.CommandResult {
	t.Helper()
	return deviceJSON(t, map[string]any{"requestId": requestID, "device": paired})
}

func scriptDevices(t *testing.T, sandbox *fakeSandbox, steps ...deviceStep) {
	t.Helper()
	calls := 0
	sandbox.devices = func(_ context.Context, command core.Command) (core.CommandResult, error) {
		t.Helper()
		if calls >= len(steps) {
			t.Fatal("unexpected native device command or approval retry")
		}
		step := steps[calls]
		calls++
		if command.Path != "python3" || command.User != "root" || len(command.Args) != 7 || len(command.Env) != 0 || len(command.Stdin) != 0 {
			t.Fatal("native device command did not use the private root-owned launch specification")
		}
		var args []string
		if err := json.Unmarshal([]byte(command.Args[6]), &args); err != nil || !reflect.DeepEqual(args, step.args) {
			t.Fatal("native device operation did not use the exact selected request")
		}
		for _, secret := range []string{"session-proxy-secret", "private-traffic-token", "private-device-token"} {
			if strings.Contains(strings.Join(command.Args, "\n"), secret) {
				t.Fatal("native device command leaked a credential in argv")
			}
		}
		return step.result, step.err
	}
	t.Cleanup(func() {
		if calls != len(steps) {
			t.Errorf("native device commands: got %d, want %d", calls, len(steps))
		}
	})
}

func controllerRecord(identity *gateway.DeviceIdentity) deviceRecord {
	return deviceRecord{
		RequestID: pairingRequestID, DeviceID: identity.ID(), PublicKey: identity.PublicKey(),
		Role: "operator", Roles: []string{"operator"}, Scopes: []string{"operator.write"},
		ClientID: "gateway-client", ClientMode: "backend",
	}
}

func TestApproveDeviceRequiresExactPendingAndPairedIdentity(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*deviceRecord)
	}{
		{"device_id", func(d *deviceRecord) { d.DeviceID = "another-device" }},
		{"public_key", func(d *deviceRecord) { d.PublicKey = "another-public-key" }},
		{"role", func(d *deviceRecord) { d.Role = "node" }},
		{"missing_roles", func(d *deviceRecord) { d.Roles = nil }},
		{"additional_role", func(d *deviceRecord) { d.Roles = []string{"operator", "node"} }},
		{"client_id", func(d *deviceRecord) { d.ClientID = "webchat" }},
		{"client_mode", func(d *deviceRecord) { d.ClientMode = "cli" }},
		{"missing_scope", func(d *deviceRecord) { d.Scopes = nil }},
		{"wrong_scope", func(d *deviceRecord) { d.Scopes = []string{"operator.read"} }},
		{"additional_scope", func(d *deviceRecord) { d.Scopes = []string{"operator.write", "operator.admin"} }},
		{"duplicate_scope", func(d *deviceRecord) { d.Scopes = []string{"operator.write", "operator.write"} }},
	}
	for _, phase := range []string{"pending", "approval", "paired"} {
		for _, mutation := range mutations {
			t.Run(phase+"/"+mutation.name, func(t *testing.T) {
				identity, err := gateway.NewDeviceIdentity()
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(identity.Destroy)
				pending, paired := controllerRecord(identity), controllerRecord(identity)
				paired.RequestID = ""
				approved := paired
				if phase == "pending" {
					mutation.mutate(&pending)
				} else if phase == "approval" {
					mutation.mutate(&approved)
				} else {
					mutation.mutate(&paired)
				}
				steps := []deviceStep{{args: []string{"list"}, result: deviceJSON(t, deviceList{Pending: []deviceRecord{pending}})}}
				if phase != "pending" {
					steps = append(steps, deviceStep{args: []string{"approve", pairingRequestID}, result: deviceApproval(t, pairingRequestID, approved)})
					if phase == "paired" {
						steps = append(steps, deviceStep{args: []string{"list"}, result: deviceJSON(t, deviceList{Paired: []deviceRecord{paired}})})
					}
				}
				sandbox := &fakeSandbox{}
				scriptDevices(t, sandbox, steps...)
				e := &Embedded{group: testProcessGroup(t, sandbox, "/workspace")}
				if err := e.approveDevice(context.Background(), pairingRequestID, identity); err == nil {
					t.Fatal("accepted a different native identity, role, client or scope")
				}
			})
		}
	}
}

func TestApproveDeviceRequiresUniqueRequestAndConfirmedReadback(t *testing.T) {
	identity, err := gateway.NewDeviceIdentity()
	if err != nil {
		t.Fatal(err)
	}
	defer identity.Destroy()
	pending, paired := controllerRecord(identity), controllerRecord(identity)
	paired.RequestID = ""
	otherRequest := pending
	otherRequest.RequestID = "different-request"
	for _, test := range []struct {
		name    string
		before  deviceList
		after   deviceList
		approve bool
	}{
		{name: "missing", before: deviceList{}},
		{name: "wrong_request", before: deviceList{Pending: []deviceRecord{otherRequest}}},
		{name: "duplicate_request", before: deviceList{Pending: []deviceRecord{pending, pending}}},
		{name: "still_pending", before: deviceList{Pending: []deviceRecord{pending}}, after: deviceList{Pending: []deviceRecord{pending}, Paired: []deviceRecord{paired}}, approve: true},
		{name: "replacement_pending", before: deviceList{Pending: []deviceRecord{pending}}, after: deviceList{Pending: []deviceRecord{otherRequest}, Paired: []deviceRecord{paired}}, approve: true},
		{name: "missing_paired", before: deviceList{Pending: []deviceRecord{pending}}, after: deviceList{}, approve: true},
		{name: "duplicate_paired", before: deviceList{Pending: []deviceRecord{pending}}, after: deviceList{Paired: []deviceRecord{paired, paired}}, approve: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			sandbox := &fakeSandbox{}
			steps := []deviceStep{{args: []string{"list"}, result: deviceJSON(t, test.before)}}
			if test.approve {
				steps = append(steps, deviceStep{args: []string{"approve", pairingRequestID}, result: deviceApproval(t, pairingRequestID, paired)}, deviceStep{args: []string{"list"}, result: deviceJSON(t, test.after)})
			}
			scriptDevices(t, sandbox, steps...)
			e := &Embedded{group: testProcessGroup(t, sandbox, "/workspace")}
			if err := e.approveDevice(context.Background(), pairingRequestID, identity); err == nil {
				t.Fatal("unconfirmed or ambiguous native pairing was accepted")
			}
		})
	}
}

func TestEmbeddedPairingReconnectsOnceAndPreservesFailures(t *testing.T) {
	for _, name := range []string{"direct", "paired", "initial_auth_failure", "invalid_list", "wrong_request", "approve_timeout", "approve_failure", "approve_invalid", "approve_replacement", "readback_failure", "pairing_again", "reconnect_auth_failure", "missing_write", "wrong_role"} {
		t.Run(name, func(t *testing.T) {
			sandbox := &fakeSandbox{}
			e := embeddedFixture(t, sandbox, nil)
			connectCalls := 0
			e.newGateway = func(_ string, _ http.Header, token []byte, identity *gateway.DeviceIdentity) (gatewayConnection, error) {
				if identity == nil || len(token) == 0 {
					t.Fatal("attempt device identity or shared authentication missing")
				}
				pending, paired := controllerRecord(identity), controllerRecord(identity)
				paired.RequestID = ""
				other := deviceRecord{RequestID: "other-request", DeviceID: "other-device"}
				var steps []deviceStep
				if name != "direct" && name != "initial_auth_failure" {
					listed := deviceJSON(t, deviceList{Pending: []deviceRecord{other, pending}})
					if name == "invalid_list" {
						listed.Stdout = `{"pending":"private-device-token"}`
					} else if name == "wrong_request" {
						listed = deviceJSON(t, deviceList{Pending: []deviceRecord{other}})
					}
					steps = append(steps, deviceStep{args: []string{"list"}, result: listed})
					if name != "invalid_list" && name != "wrong_request" {
						approval := deviceStep{args: []string{"approve", pairingRequestID}, result: deviceApproval(t, pairingRequestID, paired)}
						if name == "approve_timeout" {
							approval.err = context.DeadlineExceeded
						} else if name == "approve_failure" {
							approval.result = core.CommandResult{ExitCode: 1, Stdout: "private-device-token", Stderr: "private-device-token"}
						} else if name == "approve_invalid" {
							approval.result = core.CommandResult{Stdout: `{"requestId":"private-device-token"`}
						} else if name == "approve_replacement" {
							approval.result = deviceApproval(t, "different-request", paired)
						}
						steps = append(steps, approval)
						if name != "approve_timeout" && name != "approve_failure" && name != "approve_invalid" && name != "approve_replacement" {
							readback := deviceStep{args: []string{"list"}, result: deviceJSON(t, deviceList{Pending: []deviceRecord{other}, Paired: []deviceRecord{other, paired}})}
							if name == "readback_failure" {
								readback.result = core.CommandResult{Stdout: `{"paired":`}
							}
							steps = append(steps, readback)
						}
					}
				}
				scriptDevices(t, sandbox, steps...)
				deviceCommands := sandbox.devices
				sandbox.devices = func(ctx context.Context, command core.Command) (core.CommandResult, error) {
					if strings.Contains(strings.Join(command.Args, "\n"), string(token)) {
						t.Fatal("native device command placed the shared gateway token in argv")
					}
					return deviceCommands(ctx, command)
				}
				return &fakeGateway{connect: func(context.Context, gateway.ConnectOptions) (gateway.ConnectSummary, error) {
					connectCalls++
					if connectCalls == 1 && name != "direct" {
						if name == "initial_auth_failure" {
							return gateway.ConnectSummary{}, errors.New("private-traffic-token authentication failed")
						}
						return gateway.ConnectSummary{}, &gateway.PairingRequiredError{RequestID: pairingRequestID}
					}
					if name == "pairing_again" {
						return gateway.ConnectSummary{}, &gateway.PairingRequiredError{RequestID: "second-request"}
					}
					if name == "reconnect_auth_failure" {
						return gateway.ConnectSummary{}, errors.New("private-traffic-token authentication failed")
					}
					summary := gateway.ConnectSummary{Role: "operator", Scopes: []string{"operator.read", "operator.write"}}
					if name == "missing_write" {
						summary.Scopes = []string{"operator.read"}
					} else if name == "wrong_role" {
						summary.Role = "node"
					}
					return summary, nil
				}}, nil
			}
			err := e.Start(context.Background(), core.HarnessRequest{RunID: "run", TaskID: "pairing", Endpoint: core.ToolEndpoint{Protocol: "embedded", Workdir: "/workspace"}, Model: core.ModelConfig{Provider: "deepseek", Model: "explicit-test-model"}})
			wantSuccess := name == "direct" || name == "paired"
			if (err == nil) != wantSuccess {
				t.Fatalf("native connection outcome: %v", err)
			}
			if err != nil && (strings.Contains(err.Error(), "private-traffic-token") || strings.Contains(err.Error(), "private-device-token")) {
				t.Fatal("native pairing failure exposed authentication material")
			}
			if name == "approve_timeout" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("approval timeout classification was lost")
			}
			wantConnects := 1
			if name == "paired" || name == "pairing_again" || name == "reconnect_auth_failure" || name == "missing_write" || name == "wrong_role" {
				wantConnects = 2
			}
			if connectCalls != wantConnects {
				t.Fatalf("connect calls: got %d, want %d", connectCalls, wantConnects)
			}
			if err := e.Stop(context.Background()); err != nil || !e.isolated || sandbox.stopCalls != 1 {
				t.Fatalf("pairing outcome lost contained-process ownership: %v", err)
			}
		})
	}
}

func TestNativeDeviceCLIKeepsCredentialsInEnvironment(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}
	// Exercise the real devices branch without any guest filesystem or privilege effects.
	probe := `import ast, copy, json, sys, types
tree = ast.parse(sys.stdin.read())
branch = next(n for n in ast.walk(tree) if isinstance(n, ast.If) and isinstance(n.test, ast.Compare) and isinstance(n.test.left, ast.Name) and n.test.left.id == "action" and any(isinstance(c, ast.Constant) and c.value == "devices" for c in n.test.comparators))
code = compile(ast.Module(body=branch.body, type_ignores=[]), "devices-branch", "exec")
original = {"path":"/bin/sh", "args":["/run/aries/gateway-launcher"], "dir":"/workspace", "env":{"ARIES_OPENCLAW_EXECUTABLE":"/usr/local/bin/openclaw", "HOME":"/run/aries/home", "OPENCLAW_CONFIG_PATH":"/run/aries/openclaw.json", "OPENCLAW_GATEWAY_TOKEN":"private-gateway-token"}}
for args in (["list"], ["approve", "c8b95bb6-9505-4bdb-9d57-b89f18eab3e2"]):
    spec = copy.deepcopy(original)
    exec(code, {"spec":spec, "json":json, "sys":types.SimpleNamespace(argv=["", "", "", "", json.dumps(args)])})
    assert spec["path"] == "/usr/local/bin/openclaw"
    assert spec["args"] == ["devices"] + args + ["--timeout", "10000", "--json"]
    assert "private-gateway-token" not in json.dumps(spec["args"])
    assert spec["env"]["OPENCLAW_GATEWAY_TOKEN"] == "private-gateway-token"
    assert spec["env"]["HOME"] == original["env"]["HOME"] and spec["env"]["OPENCLAW_CONFIG_PATH"] == original["env"]["OPENCLAW_CONFIG_PATH"]
    assert spec["env"]["OPENCLAW_GATEWAY_URL"] == "ws://127.0.0.1:18790"
    assert spec["env"]["OPENCLAW_DISABLE_BUNDLED_SOURCE_OVERLAYS"] == "1"
for args in (["approve"], ["approve", "--latest"], ["approve", ""], ["list", "--token", "private-gateway-token"], ["clear"]):
    try:
        exec(code, {"spec":copy.deepcopy(original), "json":json, "sys":types.SimpleNamespace(argv=["", "", "", "", json.dumps(args)])})
    except RuntimeError:
        pass
    else:
        raise AssertionError("unexpected native device operation accepted")
`
	command := exec.Command(python, "-I", "-c", probe)
	command.Stdin = strings.NewReader(groupScript)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("native device CLI credentials/arguments: %s %v", output, err)
	}
}

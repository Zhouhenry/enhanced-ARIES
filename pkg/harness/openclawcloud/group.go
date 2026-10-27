// Package openclawcloud binds OpenClaw to an already-owned cloud sandbox.
package openclawcloud

import "github.com/hyscale-lab/aries/pkg/harness/internal/sandboxprocess"

var ErrUnsupported = sandboxprocess.ErrUnsupported

// Native Gateway/device specifications stay root-private and load under the shared admission lock.
const groupScript = sandboxprocess.ScriptLibrary + `
action, control, group = sys.argv[1:4]
if action not in ("gateway", "devices"): raise RuntimeError("invalid native control operation")
def native_spec():
    spec = json.load(open(control + "/gateway.json"))
    if action == "devices":
        args = json.loads(sys.argv[4])
        if args != ["list"] and not (len(args) == 2 and args[0] == "approve" and isinstance(args[1], str) and args[1] and not args[1].startswith("-")):
            raise RuntimeError("invalid device control operation")
        spec["path"] = spec["env"]["ARIES_OPENCLAW_EXECUTABLE"]
        spec["args"] = ["devices"] + args + ["--timeout", "10000", "--json"]
        spec["env"]["OPENCLAW_GATEWAY_URL"] = "ws://127.0.0.1:18790"
        spec["env"]["OPENCLAW_DISABLE_BUNDLED_SOURCE_OVERLAYS"] = "1"
    return spec
group_control("exec", control, group, native_spec)
`

"""Isolated opt-in protocol overlay over the published R01 native image. ECS only."""
import argparse
import hashlib
import json
from pathlib import Path
import sys

BASE_IMAGE = "sha256:4f519985234cc93f653314ea672bcf25c0abcc9d0009597ef82f066c08d02d2d"
PINS = {
    "bash-tools-D7H1rgMh.js": "998e4de388e2e8fdbb9777afa1b39fbbf219af8e64ee32f825c22be8eea33a41",
    "bash-tools.exec-runtime-CHHVz-P3.js": "8657b0be0fbf470c076abcded9584ca465cbf16b27586a34283037835ddc0f91",
}


def once(text, before, after):
    if text.count(before) != 1:
        raise ValueError("native protocol patch anchor is not unique: " + before[:80])
    return text.replace(before, after, 1)


def main():
    if not sys.platform.startswith("linux"):
        raise SystemExit("authorized Linux ECS only")
    parser = argparse.ArgumentParser()
    parser.add_argument("source", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    original = {}
    for name, digest in PINS.items():
        raw = (args.source / name).read_bytes()
        if hashlib.sha256(raw).hexdigest() != digest:
            raise ValueError("native protocol input SHA differs: " + name)
        original[name] = raw.decode()
    args.output.mkdir(mode=0o700, parents=True, exist_ok=False)
    manifest = {}
    for name, text in original.items():
        if name == "bash-tools-D7H1rgMh.js":
            text = 'import { replayToolProtocolEnabled, registerReplayJob, acceptedReplayJob, resolveReplaySession, waitReplayJob } from "./aries-replay-tool-protocol.mjs";\n' + text
            anchor = '\t\t\tconst ariesToolIdentity = sandbox ? createToolIdentity(_toolCallId, defaults?.sessionKey) : void 0;'
            text = once(text, anchor, anchor + '\n\t\t\tconst ariesReplayProtocol = Boolean(sandbox && replayToolProtocolEnabled());')
            anchor = '\t\t\t\t}) : hostEnvResult?.env ?? inheritedBaseEnv;'
            text = once(text, anchor, anchor + '\n\t\t\t\tif (ariesReplayProtocol) delete env.ARIES_REPLAY_TOOL_PROTOCOL;')
            text = once(text, '\t\t\t\t\tariesOutputOrder: !usePty && yieldWindow !== 0,',
                '\t\t\t\t\tariesOutputOrder: !usePty && (ariesReplayProtocol || yieldWindow !== 0),')
            text = once(text, '\t\t\t\t\tnotifyOnExit,\n\t\t\t\t\tnotifyOnExitEmptySuccess,',
                '\t\t\t\t\tnotifyOnExit: ariesReplayProtocol ? false : notifyOnExit,\n\t\t\t\t\tnotifyOnExitEmptySuccess,')
            anchor = '\t\t\t\tconst resolveRunning = () => {\n\t\t\t\t\tcleanupToolRunListeners();'
            text = once(text, anchor, anchor + '\n'
                '\t\t\t\t\tif (ariesReplayProtocol) {\n'
                '\t\t\t\t\t\tconst job = registerReplayJob(run, ariesToolIdentity, defaults?.scopeKey, (outcome) => buildExecForegroundResult({\n'
                '\t\t\t\t\t\t\toutcome: run.ariesTerminalOutcome(outcome), cwd: run.session.cwd, warningText: getWarningText()\n'
                '\t\t\t\t\t\t}));\n'
                '\t\t\t\t\t\tresolve(acceptedReplayJob(job, getWarningText()));\n'
                '\t\t\t\t\t\treturn;\n'
                '\t\t\t\t\t}')
            text = once(text, '\t\tdescription: describeProcessTool({ hasCronTool: defaults?.hasCronTool === true }),',
                '\t\tdescription: describeProcessTool({ hasCronTool: defaults?.hasCronTool === true }) + (replayToolProtocolEnabled() ? " terminal-result-v1: action=wait accepts the logical sessionId from exec and returns its native final result after completion and finalization; optional timeout is milliseconds and stops only waiting. Existing poll/log/control actions retain their native, timing-dependent semantics." : ""),')
            text = once(text, '\t\tparameters: processSchema,',
                '\t\tparameters: replayToolProtocolEnabled() ? { ...processSchema, properties: { ...processSchema.properties,\n'
                '\t\t\taction: { ...processSchema.properties.action, description: processSchema.properties.action.description + "; wait: read the final result of a terminal-result-v1 logical job" },\n'
                '\t\t\ttimeout: { ...processSchema.properties.timeout, description: processSchema.properties.timeout.description + "; for wait: optional positive milliseconds until only the wait is interrupted" }\n'
                '\t\t} } : processSchema,')
            text = once(text, '\t\t\tconst params = args;\n\t\t\tif (params.action === "list") {',
                '\t\t\tif (replayToolProtocolEnabled() && args.action === "wait") return waitReplayJob(args.sessionId, scopeKey, signal, args.timeout, _toolCallId);\n'
                '\t\t\tconst params = replayToolProtocolEnabled() ? { ...args, sessionId: resolveReplaySession(args.sessionId, scopeKey) } : args;\n'
                '\t\t\tif (params.action === "list") {')
        else:
            anchor = '\t\tariesForegroundOutcome: (outcome) => !session.backgrounded && ariesForegroundOutcome ? ariesForegroundOutcome() : outcome,'
            text = once(text, anchor, anchor + '\n\t\tariesTerminalOutcome: (outcome) => ariesForegroundOutcome ? ariesForegroundOutcome() : outcome,')
        raw = text.encode()
        (args.output / name).write_bytes(raw)
        (args.output / name).chmod(0o644)
        manifest[name] = {"input_sha256": PINS[name], "output_sha256": hashlib.sha256(raw).hexdigest()}
    helper = Path(__file__).with_name("replay_tool_protocol.mjs").read_bytes()
    (args.output / "aries-replay-tool-protocol.mjs").write_bytes(helper)
    (args.output / "aries-replay-tool-protocol.mjs").chmod(0o644)
    manifest["aries-replay-tool-protocol.mjs"] = {"output_sha256": hashlib.sha256(helper).hexdigest()}
    (args.output / "Dockerfile").write_text('FROM ' + BASE_IMAGE + '\nCOPY --chown=0:0 *.js *.mjs /app/dist/\n')
    (args.output / "manifest.json").write_text(json.dumps({"schema_version": 1, "protocol": "terminal-result-v1",
        "base_image": BASE_IMAGE, "files": manifest, "activation": "ARIES_REPLAY_TOOL_PROTOCOL=terminal-result-v1; external sandbox only",
        "scope": "independent benchmark variant; native default preserved",
        "unchanged": ["native process creation, execution and finalization", "poll/log/control/PTY", "Strict matcher", "original task and verifier"],
        "limitations": ["native auto-yield timing can still change the call sequence; use explicit background+wait for reproducible long jobs",
            "logical handles are not real PIDs; PID-bearing child output remains real", "poll/log/list results retain real timing and native identifiers"]}, indent=2) + '\n')


if __name__ == "__main__":
    main()

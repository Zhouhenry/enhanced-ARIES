"""Hash-pinned independent final-output experiment; run only on authorized Linux ECS."""
import argparse
import hashlib
import io
import json
from pathlib import Path
import re
import subprocess
import sys
import tarfile

BASE_IMAGE = "ghcr.io/openclaw/openclaw:2026.7.1@sha256:6a31d44b2944e7adcd2b582bf6fb463111264ebca97a0201795b799135bd102c"
IDENTITY_IMAGE = "sha256:feff2cd457d6ea172e3dd5981119f1aa4b6c66b06b1ce4fe70bbd798d77c6988"
PINS = {
    "original": {
        "bash-tools-D7H1rgMh.js": "beded00b62d7d01137226a151fdb551056b1be6c0667b7d28efca292f1fe1bac",
        "bash-tools.exec-runtime-CHHVz-P3.js": "24247e3a3cf9f5e708847afda49104c9821005f2f48a1b0f6550a17eeb73ff3e",
    },
    "r01-poc3": {
        "bash-tools-D7H1rgMh.js": "52a94fd3ed8c79bb9fa768d9576bd348d3442ec57e467137cf08d1d5137fb05d",
        "bash-tools.exec-runtime-CHHVz-P3.js": "f8acc797e3429602a8eeefd04199647a2e198487183d1070e89bb56bc923578a",
    },
}


def once(text, before, after):
    if text.count(before) != 1:
        raise ValueError("native output patch anchor is not unique: " + before[:70])
    return text.replace(before, after, 1)


def main():
    if not sys.platform.startswith("linux"):
        raise SystemExit("execute on authorized Linux ECS only")
    parser = argparse.ArgumentParser()
    parser.add_argument("source", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("--input-version", choices=PINS, default="original")
    parser.add_argument("--base-image", help="optional exact sha256 image ID; modified main/runtime must match pinned input")
    args = parser.parse_args()
    originals = {}
    for name, digest in PINS[args.input_version].items():
        raw = (args.source / name).read_bytes()
        if hashlib.sha256(raw).hexdigest() != digest:
            raise ValueError("native input SHA differs: " + name)
        originals[name] = raw.decode()
    base_image = args.base_image or (BASE_IMAGE if args.input_version == "original" else IDENTITY_IMAGE)
    if args.base_image and not re.fullmatch(r"sha256:[0-9a-f]{64}", base_image):
        raise ValueError("--base-image must be an exact sha256 image ID")
    # Verify the exact layer input. Only create a stopped container; no image code executes.
    container = subprocess.check_output(["docker", "create", "--network=none", "--entrypoint=/bin/true", base_image], text=True).strip()
    try:
        for name, digest in PINS[args.input_version].items():
            archive = subprocess.check_output(["docker", "cp", container + ":/app/dist/" + name, "-"])
            with tarfile.open(fileobj=io.BytesIO(archive)) as copied:
                entries = [entry for entry in copied if entry.isfile()]
                if len(entries) != 1 or hashlib.sha256(copied.extractfile(entries[0]).read()).hexdigest() != digest:
                    raise ValueError("base image differs from pinned source: " + name)
    finally:
        subprocess.run(["docker", "rm", container], check=True, stdout=subprocess.DEVNULL)
    args.output.mkdir(mode=0o700, parents=True, exist_ok=False)
    changes = {}
    for name, text in originals.items():
        if name == "bash-tools-D7H1rgMh.js":
            text = once(text, '\t\t\t\t\tusePty,\n\t\t\t\t\twarnings,',
                '\t\t\t\t\tusePty,\n'
                '\t\t\t\t\tariesOutputOrder: !usePty && yieldWindow !== 0,\n'
                '\t\t\t\t\twarnings,')
            text = once(text, '\t\t\t\t\tif (yielded || run.session.backgrounded) return;\n\t\t\t\t\tresolve(buildExecForegroundResult({\n\t\t\t\t\t\toutcome,',
                '\t\t\t\t\tif (yielded || run.session.backgrounded) return;\n\t\t\t\t\tresolve(buildExecForegroundResult({\n\t\t\t\t\t\toutcome: run.ariesForegroundOutcome(outcome),')
            anchor = 'function buildExecForegroundResult(params) {\n\tconst warningText = params.warningText?.trim() ? `${params.warningText}\\n\\n` : "";'
            text = once(text, anchor, anchor + '\n'
                '\tconst ariesOutputDetails = params.outcome.ariesOutputRenderer ? { outputRenderer: params.outcome.ariesOutputRenderer, outputTruncated: params.outcome.ariesOutputTruncated } : {};\n'
                '\tif (params.outcome.ariesOutputError) return failedTextResult(`${warningText}Output rendering failed: ${params.outcome.ariesOutputError}`, {\n'
                '\t\t...ariesOutputDetails, status: "failed", failureKind: "output-rendering", outputRenderingError: params.outcome.ariesOutputError,\n'
                '\t\texecutionStatus: params.outcome.status, exitCode: params.outcome.exitCode ?? null, exitSignal: params.outcome.exitSignal,\n'
                '\t\texecutionFailureKind: params.outcome.failureKind, executionReason: params.outcome.reason,\n'
                '\t\texitReason: params.outcome.exitReason, durationMs: params.outcome.durationMs, aggregated: `Output rendering failed: ${params.outcome.ariesOutputError}`,\n'
                '\t\ttimedOut: params.outcome.status === "failed" ? params.outcome.timedOut : void 0, noOutputTimedOut: params.outcome.noOutputTimedOut, cwd: params.cwd\n'
                '\t});')
            text = once(text, '\tif (params.outcome.status === "failed") return failedTextResult(`${warningText}${params.outcome.reason}`, {\n',
                '\tif (params.outcome.status === "failed") return failedTextResult(`${warningText}${params.outcome.reason}`, {\n\t\t...ariesOutputDetails,\n')
            text = once(text, '\treturn textResult(`${warningText}${renderExecOutputText(params.outcome.aggregated)}`, {\n',
                '\treturn textResult(`${warningText}${renderExecOutputText(params.outcome.aggregated)}`, {\n\t\t...ariesOutputDetails,\n')
        elif name == "bash-tools.exec-runtime-CHHVz-P3.js":
            text = 'import { createFinalOutput, attachFinalOutput } from "./aries-output-order-poc.mjs";\n' + text
            text = once(text, '\taddSession(session);\n\tlet updatesDisabled = false;',
                '\taddSession(session);\n\tconst ariesFinalOutput = opts.ariesOutputOrder && !opts.usePty ? createFinalOutput(opts.maxOutput) : null;\n'
                '\tlet ariesForegroundOutcome = null;\n'
                '\tlet updatesDisabled = false;')
            text = once(text, '\tconst handleStdout = (data) => {\n\t\tconst raw = data;',
                '\tconst handleStdout = (data) => {\n\t\tariesFinalOutput?.write("stdout", data);\n\t\tconst raw = data;')
            text = once(text, '\tconst handleStderr = (data) => {\n\t\tconst str = sanitizeBinaryOutput(data);',
                '\tconst handleStderr = (data) => {\n\t\tariesFinalOutput?.write("stderr", data);\n\t\tconst str = sanitizeBinaryOutput(data);')
            text = once(text, '\t\tmarkExited(session, exit.exitCode, exit.exitSignal, outcome.status, exit.reason, exit.noOutputTimedOut);',
                '\t\tconst ariesRendered = ariesFinalOutput?.finish();\n'
                '\t\tif (ariesRendered) {\n'
                '\t\t\tconst ariesInput = { exit, aggregated: ariesRendered.text.trim(), durationMs, timeoutSec: opts.timeoutSec };\n'
                '\t\t\tariesForegroundOutcome = () => attachFinalOutput(buildExecExitOutcome(ariesInput), ariesRendered);\n'
                '\t\t}\n'
                '\t\tmarkExited(session, exit.exitCode, exit.exitSignal, outcome.status, exit.reason, exit.noOutputTimedOut);')
            text = once(text, '\t\t\tdurationMs: Date.now() - startedAt\n\t\t});\n\t\temitExecProcessCompleted({',
                '\t\t\tdurationMs: Date.now() - startedAt\n\t\t});\n'
                '\t\tconst ariesRendered = ariesFinalOutput?.finish();\n'
                '\t\tif (ariesRendered) {\n'
                '\t\t\tconst ariesInput = { error: err, aggregated: ariesRendered.text.trim(), durationMs: outcome.durationMs };\n'
                '\t\t\tariesForegroundOutcome = () => attachFinalOutput(buildExecRuntimeErrorOutcome(ariesInput), ariesRendered);\n'
                '\t\t}\n\t\temitExecProcessCompleted({')
            text = once(text, '\t\tpid: session.pid ?? void 0,\n\t\tpromise,',
                '\t\tpid: session.pid ?? void 0,\n\t\tpromise,\n'
                '\t\tariesForegroundOutcome: (outcome) => !session.backgrounded && ariesForegroundOutcome ? ariesForegroundOutcome() : outcome,')
        raw = text.encode()
        (args.output / name).write_bytes(raw)
        changes[name] = {"input_sha256": PINS[args.input_version][name], "output_sha256": hashlib.sha256(raw).hexdigest()}
    helper = Path(__file__).with_name("output_order.mjs").read_bytes()
    (args.output / "aries-output-order-poc.mjs").write_bytes(helper)
    changes["aries-output-order-poc.mjs"] = {"output_sha256": hashlib.sha256(helper).hexdigest()}
    (args.output / "Dockerfile").write_text('FROM ' + base_image + '\nCOPY --chown=0:0 *.js *.mjs /app/dist/\n')
    (args.output / "manifest.json").write_text(json.dumps({
        "schema_version": 1, "scope": "isolated foreground non-PTY output renderer experiment",
        "input_version": args.input_version, "base_image": base_image, "base_main_runtime_sha_verified": True,
        "unmodified_modules": "identity helper/browser and all other modules inherited byte-for-byte from exact base image",
        "output_renderer": "stdout-stderr-v1",
        "files": changes, "activation": "candidate image; actual foreground completion and non-PTY only; original allowBackground/process/yield retained (no environment switch)",
        "final_gate": "apply after original yielded/backgrounded check; freeze view at first native wait/error (finalize failure reuses wait snapshot); reuse original exit and duration",
        "original_runtime_evidence": "registry, diagnostics, notify, poll/log retain original arrival-ordered aggregate; only final foreground ToolResult uses renderer",
        "changed": ["final foreground ToolResult ordering, sanitization chunk independence, truncation selection"],
        "join_rule": "stdout followed by stderr with no inserted separator; UTF16-safe tail to maxOutput then existing trim/exit formatting",
        "unchanged": ["child argv/cwd/env/stdin/stdio", "exit/signal", "live updates", "background/poll/log", "strict matcher"],
        "limits": {"max_ansi_token_utf16": 8192, "max_output_utf16": 200000},
    }, indent=2, sort_keys=True) + '\n')


if __name__ == "__main__":
    main()

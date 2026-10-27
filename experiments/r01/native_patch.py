"""ECS-only, hash-pinned OpenClaw identity PoC; never edits the base image/source."""
import argparse
import hashlib
import json
from pathlib import Path
import sys

PINS = {
    "bash-tools-D7H1rgMh.js": "beded00b62d7d01137226a151fdb551056b1be6c0667b7d28efca292f1fe1bac",
    "bash-tools.exec-runtime-CHHVz-P3.js": "24247e3a3cf9f5e708847afda49104c9821005f2f48a1b0f6550a17eeb73ff3e",
    "browser-bridges-Be07JHOW.js": "ecce9903676afcdc84074aeead784fd27f849bd2987e0e9700f314a315f95b08",
}
BASE_IMAGE = "ghcr.io/openclaw/openclaw:2026.7.1@sha256:6a31d44b2944e7adcd2b582bf6fb463111264ebca97a0201795b799135bd102c"


def once(text, before, after):
    if text.count(before) != 1:
        raise ValueError("pinned native patch anchor is not unique")
    return text.replace(before, after, 1)


def patch_pre_dispatch_rejections(text):
    # This permission gate precedes host resolution. Observe only its existing
    # sandbox context; do not resolve/reorder the target or alter the original error.
    elevated = '\t\t\t\t\tthrow new Error([\n\t\t\t\t\t\t`elevated is not available right now (runtime=${runtime}).`,'
    text = once(text, elevated,
        '\t\t\t\t\ttry {\n'
        '\t\t\t\t\t\tif (defaults?.sandbox) recordToolRejection(createToolIdentity(_toolCallId, defaults?.sessionKey), "elevated_policy_rejected");\n'
        '\t\t\t\t\t} catch {}\n' + elevated)
    return text


def patch_host_policy_rejection(text):
    # The pinned resolver's sole throw is its host-policy refusal. Validation of
    # the requested host stays outside this catch, and the same error is rethrown.
    anchor = ('\t\t\tconst target = resolveExecTarget({\n'
              '\t\t\t\tconfiguredTarget: defaults?.host,\n'
              '\t\t\t\trequestedTarget,\n'
              '\t\t\t\televatedRequested,\n'
              '\t\t\t\tsandboxAvailable: Boolean(defaults?.sandbox)\n'
              '\t\t\t});')
    replacement = ('\t\t\tlet target;\n\t\t\ttry {\n'
                   '\t\t\t\ttarget = resolveExecTarget({\n'
                   '\t\t\t\t\tconfiguredTarget: defaults?.host,\n'
                   '\t\t\t\t\trequestedTarget,\n'
                   '\t\t\t\t\televatedRequested,\n'
                   '\t\t\t\t\tsandboxAvailable: Boolean(defaults?.sandbox)\n'
                   '\t\t\t\t});\n\t\t\t} catch (error) {\n'
                   '\t\t\t\ttry {\n'
                   '\t\t\t\t\tif (defaults?.sandbox) recordToolRejection(createToolIdentity(_toolCallId, defaults?.sessionKey), "host_policy_rejected");\n'
                   '\t\t\t\t} catch {}\n\t\t\t\tthrow error;\n\t\t\t}')
    return once(text, anchor, replacement)


def patch_ssh_pre_dispatch_rejection(text):
    # This actual SSH parser runs before session creation and the identity file.
    anchor = ('\t\t\t\tconst remoteCommand = buildValidatedExecRemoteCommand({\n'
              '\t\t\t\t\tcommand,\n\t\t\t\t\tworkdir: remoteWorkdir,\n'
              '\t\t\t\t\tenv\n\t\t\t\t});')
    replacement = ('\t\t\t\tlet remoteCommand;\n\t\t\t\ttry {\n'
                   '\t\t\t\t\tremoteCommand = buildValidatedExecRemoteCommand({\n'
                   '\t\t\t\t\t\tcommand,\n\t\t\t\t\t\tworkdir: remoteWorkdir,\n'
                   '\t\t\t\t\t\tenv\n\t\t\t\t\t});\n'
                   '\t\t\t\t} catch (error) {\n'
                   '\t\t\t\t\ttry { recordToolRejection(ariesToolIdentity, "shell_preflight_rejected"); } catch {}\n'
                   '\t\t\t\t\tthrow error;\n\t\t\t\t}')
    text = once(text, anchor, replacement)
    return 'import { recordToolRejection } from "./aries-tool-identity-poc.mjs";\n' + text


def patch_script_pre_dispatch_rejection(text):
    # The validator may reject synchronously or after a file read. Its entire
    # lifetime precedes runExecProcess; errors after that boundary are not proof
    # of non-dispatch. Keep native cleanup and the original exception unchanged.
    text = once(text, '\t\t\tlet effectiveTimeout;\n\t\t\ttry {',
                '\t\t\tlet effectiveTimeout;\n\t\t\tlet ariesScriptPreflightActive = false;\n\t\t\ttry {')
    anchor = ('\t\t\t\t})) await validateScriptFileForShellBleed({\n'
              '\t\t\t\t\tcommand: params.command,\n'
              '\t\t\t\t\tworkdir: scriptPreflightCwd\n\t\t\t\t});')
    text = once(text, anchor,
                '\t\t\t\t})) {\n\t\t\t\t\tariesScriptPreflightActive = true;\n'
                '\t\t\t\t\tawait validateScriptFileForShellBleed({\n'
                '\t\t\t\t\t\tcommand: params.command,\n'
                '\t\t\t\t\t\tworkdir: scriptPreflightCwd\n\t\t\t\t\t});\n'
                '\t\t\t\t\tariesScriptPreflightActive = false;\n\t\t\t\t}')
    return once(text, '\t\t\t} catch (error) {\n\t\t\t\tdiscardPreparedSandboxWorkdir?.();',
                '\t\t\t} catch (error) {\n'
                '\t\t\t\tif (ariesScriptPreflightActive) try {\n'
                '\t\t\t\t\trecordToolRejection(ariesToolIdentity, "shell_preflight_rejected");\n'
                '\t\t\t\t} catch {}\n\t\t\t\tdiscardPreparedSandboxWorkdir?.();')


def main():
    if not sys.platform.startswith("linux"):
        raise SystemExit("execute on authorized Linux ECS only")
    parser = argparse.ArgumentParser()
    parser.add_argument("source", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    originals = {}
    for name, digest in PINS.items():
        raw = (args.source / name).read_bytes()
        if hashlib.sha256(raw).hexdigest() != digest:
            raise ValueError("native input SHA differs: " + name)
        originals[name] = raw.decode()
    args.output.mkdir(mode=0o700, parents=True, exist_ok=False)
    changes = {}
    for name, text in originals.items():
        if name == "bash-tools-D7H1rgMh.js":
            text = 'import { createToolIdentity, recordToolRejection } from "./aries-tool-identity-poc.mjs";\n' + text
            text = once(text,
                '\t\t\tconst sandbox = host === "sandbox" ? defaults?.sandbox : void 0;',
                '\t\t\tconst sandbox = host === "sandbox" ? defaults?.sandbox : void 0;\n'
                '\t\t\tconst ariesToolIdentity = sandbox ? createToolIdentity(_toolCallId, defaults?.sessionKey) : void 0;')
            text = patch_pre_dispatch_rejections(text)
            text = patch_host_policy_rejection(text)
            text = patch_script_pre_dispatch_rejection(text)
            anchor = '\t\t\t\trun = await runExecProcess({\n\t\t\t\t\tcommand: params.command,\n\t\t\t\t\texecCommand: execCommandOverride,'
            text = once(text, anchor,
                        anchor.replace('command: params.command,', 'ariesToolIdentity,\n\t\t\t\t\tcommand: params.command,', 1))
        elif name == "bash-tools.exec-runtime-CHHVz-P3.js":
            text = once(text, '\t\t\tconst backendExecSpec = await opts.sandbox.buildExecSpec?.({\n',
                        '\t\t\tconst backendExecSpec = await opts.sandbox.buildExecSpec?.({\n'
                        '\t\t\t\tariesToolIdentity: opts.ariesToolIdentity,\n')
        else:
            text = once(text, 'buildExecSpec: async ({ command, workdir, env, usePty }) => {',
                        'buildExecSpec: async ({ command, workdir, env, usePty, ariesToolIdentity }) => {')
            anchor = '\t\t\t\t\tif (!this.consumeRefreshedSkillsForNextExec(remoteWorkdir)) await this.refreshRemoteSkillsWorkspace(sshSession);'
            text = once(text, anchor, anchor + '\n'
                '\t\t\t\t\tif (ariesToolIdentity) await fs.writeFile(path.join(path.dirname(sshSession.configPath), "aries-tool-identity.json"),\n'
                '\t\t\t\t\t\tJSON.stringify(ariesToolIdentity), { flag: "wx", mode: 256 });')
            text = patch_ssh_pre_dispatch_rejection(text)
        raw = text.encode()
        (args.output / name).write_bytes(raw)
        changes[name] = {"original_sha256": PINS[name], "candidate_sha256": hashlib.sha256(raw).hexdigest()}
    helper = Path(__file__).with_name("tool_identity.mjs").read_bytes()
    (args.output / "aries-tool-identity-poc.mjs").write_bytes(helper)
    changes["aries-tool-identity-poc.mjs"] = {"candidate_sha256": hashlib.sha256(helper).hexdigest()}
    (args.output / "Dockerfile").write_text('FROM ' + BASE_IMAGE + '\nCOPY --chown=0:0 *.js *.mjs /app/dist/\n')
    (args.output / "manifest.json").write_text(json.dumps({
        "schema_version": 1, "scope": "isolated native protocol PoC, not the formal benchmark image",
        "base_image": BASE_IMAGE, "files": changes,
        "unchanged": ["agent shell", "command argv/cwd/env/stdin", "tool result", "strict matcher"],
    }, indent=2, sort_keys=True) + '\n')


if __name__ == "__main__":
    main()

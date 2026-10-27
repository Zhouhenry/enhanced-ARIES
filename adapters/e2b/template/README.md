# One E2B template for both OpenClaw placements

These scripts run on the remote Linux ECS. This revision's post-finalize repair
passed all 8 template tests and native Docker positive/negative probes there;
the probe containers were confirmed absent. The v2 real cloud build and boot
verification passed on October 22, 2026, 16:20:09–16:20:56 UTC: `verify_exit=0`,
`runtime_status=PASS`, cgroup probe `PASS`, and `cleanup_confirmed=true` after
`kill` and control-plane absence confirmation. Full Gateway/Agent lifecycle,
E2B-to-origin TLS, DeepSeek inference, A/B and strict replay remain `UNRUN`.
Imports never execute SDK operations.
They are a template preparation path, not an A/B benchmark or acceptance result.

The build reads the explicit OpenClaw image from the `--versions-file` catalog,
defaulting to [`configs/versions.json`](../../../configs/versions.json). The
checked-in OpenClaw `2026.7.1` reference includes the registry SHA256 verified
on ECS. Version tags without a digest remain accepted. It imports the exact
reference, including its digest when present, through the
official `e2b==2.58.0` Python SDK and retains its Node binary and `/app` runtime.
It does not install a second OpenClaw version with npm. The upstream
[versioned Dockerfile](https://github.com/openclaw/openclaw/blob/v2026.7.1/Dockerfile)
defines `/app/openclaw.mjs`, `/usr/local/bin/openclaw`, and its Node base.
Preparation checks that layout, makes the application tree root-owned and
read-only to the agent, and re-establishes the CLI symlink explicitly.
The pinned base contains 43 dangling links in `/app/node_modules/.bin/`.
Preparation records and removes only dangling links in that directory in
`/usr/local/share/aries-template/pruned-cli-links.txt`; valid links remain and
the strict ownership/target checks are unchanged. A failed runtime probe emits
its structured diagnostic before returning failure.

Both A and B use this same published E2B template for their task and fresh verifier
sandboxes. A's external Docker harness uses the same catalog's OpenClaw image;
B starts that same OpenClaw version in the E2B guest. The template adds Python 3,
Debian pytest, git, the dedicated `aries-agent` UID/GID 60000, and
`/workspace/aries-case`. It does not contain model credentials, hidden tests,
agent configuration, or an automatically running Gateway. The template start
command repairs and checks native permissions, then retains an inert root sleep
process; the harness starts the real Gateway.

E2B's [finalize configuration](https://github.com/e2b-dev/runtime/blob/main/packages/orchestrator/pkg/template/build/phases/finalize/configure.sh)
recursively sets `/usr/local` to mode 0777 after build instructions. The root-owned
`/opt/aries-template/start.sh` runs after that step and removes group/other write
permission from `/usr/local` before executing the unchanged native probe there.
It writes new probe JSON to `/opt/aries-template/ready.json.tmp` and atomically
renames it to read-only `ready.json` only when the probe succeeds. Readiness checks
that dedicated file, never the earlier `image-evidence.json`. Both prepare and
start remove old ready/temporary files; prepare does so before finalize because
start and readiness can run concurrently. Probe failure leaves no ready file.
Under the official [start/snapshot lifecycle](https://docs.e2b.dev/template/start-ready-command),
this runs once before the template snapshot, not on every sandbox creation.
The snapshot preserves the repaired filesystem and sleep process. It does not
prove runtime cgroup capability; the separate fresh-sandbox probe remains required.

The checked-in source image is digest-locked. For archived experiments,
retain the actual catalog used on ECS and use that same locked catalog for both
`build --versions-file` and the A/B profiles' `versions_file`; do not invent a
digest or strip it before use. The build evidence preserves `source_image`
verbatim. Debian package resolution still occurs at build time, so a source
digest alone does not make a later template rebuild byte-identical.
Build once, retain the returned template/build IDs,
and use the immutable `template_id:build_id` reference for both placements. Do not rebuild or
retag it during an experiment. The probe records actual Node, OpenClaw, Python,
pytest and git versions, plus the guest hostname. A's real native version evidence must agree with B's;
matching a configured tag alone does not prove identical binaries.

## Remote build

Install the adapter's pinned requirements on ECS, and supply `E2B_API_KEY` through
the ECS process environment or secret manager. Neither DeepSeek credentials nor
model-proxy tokens are required. Do not pass secrets as arguments, enable shell
tracing, or copy `.env` into a build context. The script creates a private temporary
context containing exactly `prepare.sh`, `check_runtime.py`, and `start.sh`; the SDK
uploads only those three explicitly named files. A separate Dockerfile would duplicate the
SDK definition, so this path does not require Docker on the build client.

From the repository root on ECS, with the pinned Python environment activated:

```sh
python3 -m unittest discover -s adapters/e2b/template -p 'test_*.py'
python3 adapters/e2b/template/build.py build \
  --versions-file runs/versions.lock.json \
  --cpu-count 2 --memory-mb 4096 \
  --output runs/template-build-private.json
```

The locked catalog in this example must first be created from the actual ECS
image pull; this command does not resolve or fabricate registry digests. An
explicit relative `--versions-file` is resolved from the working directory.
The resource values above are an explicit example, not a verified account quota
or performance recommendation. Use the same chosen resources for A and B.
`Template.build_in_background` returns `BuildInfo`; the script records it before
polling `Template.get_build_status` until `ready`. Every invocation chooses a new
name and the `ab-v1` tag. Evidence is created with mode 0600 and cannot overwrite
an existing file. It includes the alias/name, template ID, build ID, script
hashes, source image, resource request, and status. Terminal output omits these
identifiers. SDK logs and raw exceptions are not copied into public output.
Failures retain the exception class and, when available, an integer HTTP status
in `error_http_status`; response bodies and credentials are not recorded.
The script does not modify `.env` or delete the published template.

A build timeout or interruption is not a remote cancellation. Inspect the private
record and the E2B control plane before retrying: the build may still complete,
and a failure before the SDK returns IDs can leave an unknown creation outcome.
The generated requested name is recorded before submission for reconciliation.

## Boot, capability check, and cleanup

```sh
python3 adapters/e2b/template/build.py verify \
  --build-evidence runs/template-build-private.json \
  --output runs/template-verify-private.json
```

This creates one billed sandbox with a 180-second kill timeout, public guest
traffic disabled, and internet access disabled. It executes the trusted root
probe, then calls `kill` and requires `get_info` to return
`SandboxNotFoundException`. Provider cleanup errors or a still-present sandbox
produce failure; a timeout merely limits exposure and is not cleanup proof.
Before create, the private evidence records an allocation handle also sent as
`aries_template_verify_handle` metadata. If the response is lost, reconcile that
exact handle in E2B before another probe; do not delete unrelated resources or
retry an ambiguous creation. This allocation is recorded separately from the
adapter's ownership ledger. Auto-resume is disabled, and normal completion deletes
immediately instead of waiting for the timeout.
The raw sandbox ID is retained only in the private record for recovery.

Before any allocation, verification requires a template ID, a valid build UUID,
and the expected OpenClaw version. It derives and records `template_reference`
as `template_id:build_id` from the existing successful build evidence; older
records need no new field or rebuild. E2B's
[template reference rules](https://docs.e2b.dev/template/tags) make an unqualified
reference select `default`. This recipe publishes only `ab-v1`, so using the bare
template ID would select a tag it did not publish. The build UUID selects the
exact artifact even if tags later move.

`check_runtime.py --check-cgroup` checks the actual guest's cgroup v2 mount,
creates a dedicated group, starts a UID60000 process with `no_new_privs` inside it,
writes `cgroup.kill`, and requires `populated=0` before removing the group. Missing
kernel/delegation capability is `UNSUPPORTED`, not a successful zero measurement.
The image-only build check leaves cgroup capability `UNRUN`: filesystem setup
cannot create a kernel feature or cgroup delegation that the provider does not
expose. Every formal A/B start still performs its own containment checks, and
every formal stop must confirm its own process group is empty. This probe does
not replace those lifecycle proofs or validate Gateway/model connectivity.

After a successful build **and** boot/cleanup check, privately use the returned
`template_reference` as `E2B_TEMPLATE_ID` in both experiment profiles and the
adapter's template allowlist. The full references must match exactly.
Use `/usr/local/bin/openclaw` and the repository version pin for B's native
executable settings. Configuration edits remain explicit; this script performs
none automatically.

API signatures were checked against the installed official
[`e2b==2.58.0` distribution](https://pypi.org/project/e2b/2.58.0/):
`e2b/template/main.py` (`TemplateBase`, `from_image`, `copy`, `run_cmd`,
`set_start_cmd`), `e2b/template_sync/main.py` (`build_in_background`,
`get_build_status`), and `e2b/template/types.py` (`BuildInfo`). The contract test
checks that pinned public surface without making API calls. The separate v2
cloud probe passed boot, cgroup containment and cleanup checks; every formal
task still requires its own Gateway/Agent lifecycle and isolation evidence.

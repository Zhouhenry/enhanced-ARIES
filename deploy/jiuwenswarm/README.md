# JiuwenSwarm images and cloud templates

## Current image7 process observation

The four `jiuwenswarm-*-record.json` profiles now select the published image7
and its verified E2B/Huawei templates, both **2 CPU / 4096 MB**. Image7 adds
native subprocess callback observation without changing commands, outputs,
foreground/background scheduling or cancellation. The shared native shell
creation boundary records creation, first output, direct-child exit and pipe
closure; request/SDK duration is not reported as command execution time.
See [the timing contract and acceptance status](../../docs/jiuwen-process-observation-20261027.md).

`image.lock.json` and `templates.lock.json` pin the registry digest, exact
source overlays and actual receipts. The patch manifest SHA256 is
`122f681cb9519a9e4a7443e3cae56b86fd8200e348f92a0152cef609cf5f0eae`.
Of the original 4480 Swarm source files, 4477 remain unchanged and 3 have
verified overlays; three installed agent-core overlay files are checked
separately. The upstream revision and lock still identify the baseline,
not an unmodified installation. The incremental recipe is in
[process-observation/README.md](process-observation/README.md).

Native producer tests, A remote integration, B real extension/tool integration,
image publication and both cloud capability probes passed. Each probe sandbox
was deleted and independently confirmed absent. These checks contain no model
calls. The subsequent **frozen release23 image7 full Agent A/B matrix passed**
on both clouds: each independent real-model run passed the original 12/12
verifier, all 15 observed command processes and 105 events reconciled, and
all eight task/evaluator sandboxes were independently confirmed absent.
Each run's frozen Snapshot also passed actual Perfetto, Jaeger and Prometheus
queries; disposable backend containers, listeners and cgroups were cleaned.
See [the four-run report and exact source hashes](../../docs/jiuwen-release23-cloud-validation-20261027.md).
This is scoped acceptance: overall coverage remains unknown, Huawei verifier
egress remains assumed, and independent model trajectories are not performance
rankings or same-Tape comparisons.

Previous image5 locks are preserved in `history/image5-*.lock.json` and image4
locks in `history/image4-*.lock.json`. Existing replay profiles retain their
image4 pins and original Tapes. They do not acquire process measurements from
these new-image results.

## Historical image5 acceptance

With frozen release21 and image5, E2B/Huawei A (external) and B (embedded)
real Agent runs each passed the original 12/12 independent verifier; all eight
task/evaluator sandboxes were independently confirmed deleted. The A runs
each have 8 exact native ToolCall IDs and 11 remote invocations. Their process
lifecycle was not instrumented. See [the four-run report](../../docs/jiuwen-release21-cloud-validation-20261027.md).
These independent real model recordings are not same-Tape speed comparisons.
Global coverage remains unknown and Huawei verifier egress remains assumed.

## Incremental image5 recipe and validation

`Dockerfile.patched` and `build_patched.py` build a separate image5 candidate from
the accepted image4 digest. They reuse its dependency layers and do not run
`uv sync`, download dependencies, publish an image, or create cloud resources.
The original upstream commits, `uv.lock`, and `aries-build.json` stay unchanged;
these identify the **upstream baseline**, not an unmodified native installation.
Two reviewed patches add native ToolCall context and remote Glob/session-cancel
support. Their `.patch` files and adjacent JSON manifests record the exact
upstream revisions and each changed file's before/after SHA256.

The root-owned, mode-0444 `/opt/jiuwenswarm/aries-patches.json` records
`native_modified=true`, the base image/build pins, patch file hashes, and actual
installed paths with before/after hashes. Image verification and current
Harness bootstrap validate this overlay separately from the original pins.
Image4 remains available and reports `native_modified=false` with the updated
image verifier. The historical image5 locks pin that published image and its separately
verified cloud templates; their evidence scopes are recorded explicitly.

On ECS, supply a frozen copy of the current harness Python sources:

```sh
python3 build_patched.py --output /opt/aries-artifacts/NEW-IMAGE5-BUILD \
  --runtime-source /root/aries-setup/FROZEN-JIUWEN-RUNTIME
```

Local acceptance runs the UID-60000 image check, original ToolCall context
tests, real native provider tests, and current bootstrap health smoke with no
network or model requests. A deliberately wrong patch hash must prevent Agent
startup. Temporary containers are removed. The separate two-container native
tool validation, cloud containment, real Agent tasks and publication are
additional acceptance steps; local image acceptance alone does not imply those pass.

The 2026-10-27 local candidate passed at
`/opt/aries-artifacts/jiuwen-image5-1/build-2`: image ID
`sha256:b4078957d488e1cb52e9cc85ef5555208f0281a545c580a740a619f52127c6b1`,
246,642 additional bytes with the original image4 layers reused. Its 9 ToolCall
context and 12 provider tests passed, bootstrap reached ready, and the modified
manifest negative test rejected startup. All seven local probe containers were
removed. That receipt covers local acceptance only; the later publication and
cloud probe receipts below establish the current published pins. `build-1`
retains the failed UID-60000 check that found a non-searchable patch directory;
the recipe now sets that directory to 0755 while patch files remain 0444.
The subsequent `/opt/aries-artifacts/jiuwen-image5-1/provider-final-1` regression
passed all 14 provider tests against the same image and the final worker clock,
index and terminal-event instrumentation. That separate receipt pins the final
runtime helpers; it does not rewrite `build-2`'s original 12-test input record.

## Original image4 recipe

This Linux/amd64 recipe installs the native JiuwenSwarm checkout into
`/opt/jiuwenswarm`, with its own `/opt/jiuwenswarm/.venv/bin/python`. It is a new
image for the public `codingcase` B harness. It contains Python, git and pytest;
it does not add Terminal-Bench datasets, task answers or verifier tests.

The native revision is `e2c9d519498080e249e34f16a80c8c97f0cd1962` and the unchanged
`uv.lock` SHA256 is
`f28fbbd9b208bcf4f6e421818969ed2b6570f0648bed25c2ed15ee5bd551e5c9`.
The existing Python 3.12.15 Linux base is pinned by its actual local image ID.
`uv 0.9.5 sync --frozen --offline --no-default-groups` creates a fresh virtual
environment at the final image path, using the existing verified dependency
cache. It never copies the host virtual environment. The three Git dependencies
retain the lock's exact commits; agent-core's cache was seeded through a
same-commit local file mirror because GitCode transport was unreliable.
`pytest-wheels.json` pins the task Python's pytest and its dependencies to wheel
hashes from the same native lock. Debian git/passwd/tini/CA packages resolve at
build time; actual versions are retained in the image. A later rebuild is not
claimed byte-identical. Experiments must use the accepted image digest.

`aries-agent` has UID/GID 60000. Native sources, dependencies, Python and
`/opt/jiuwenswarm/aries-build.json` are root-owned and not agent-writable.
The manifest records the native revision, source archive/tree hashes, lock hash,
base image ID, uv and ENVD hashes, Git dependency commits and recipe hashes.
No `.env`, account, API key, task, project or flash configuration is copied.
The harness creates per-attempt runtime state and injects its flash/model
configuration at bootstrap.

Build only on the authorized ECS; the defaults reuse these existing artifacts:

- `/opt/aries-artifacts/jiuwen-integration-1/source.tar.gz`, `uv-cache`, `tooling/bin/uv`
- `/opt/aries-artifacts/agentsphere-image-1/context/envd` (existing ENVD 0.10.0)
- the pinned local Python base image recorded in `build.py`

Copy this small recipe directory to ECS, then run:

```sh
python3 /opt/aries-artifacts/jiuwen-image-recipe-1/build.py \
  --output /opt/aries-artifacts/jiuwen-image-1
```

The output directory must be new. The build verifies the source archive, all
tracked source file hashes, lock, uv and ENVD before building. The image probe
runs as UID 60000 with a read-only filesystem and no network. It checks real
native imports, Python range 3.11–3.13, task pytest, ownership and build pins.
A second smoke runs the exact frozen Harness bootstrap: root stages its private
files, UID 60000 initializes the native workspace and starts AgentServer. It
checks service health, Flash/external mode, config and canonical `.env` rewrites,
and memory/projects/skills writes. No task or model request is submitted.
The receipt records image ID, size, disk use, logs and the image-only scope.
The source manifest and exact recipe are retained beside it. Retain this small
receipt/recipe on the development machine and the accepted registry digest;
there is no requirement to duplicate a multi-gigabyte Docker tar.

`e2b_template.py` is an inert SDK definition for a later explicitly run E2B
build. Its 2 CPU / 4096 MiB values preserve the existing validated E2B template's
resources; image construction itself does not allocate these resources. E2B
finalize can make `/usr/local` writable, so the root-owned start script repairs
it and writes fresh readiness only after the image probe passes. Huawei can
use the same image's existing `tini`/ENVD 0.10.0 entrypoint on port 49983.
Neither importing this module nor running `build.py` creates cloud templates or
Sandboxes. Provider image pull, fresh guest bootstrap, network policy and
Harness lifecycle remain separate acceptance steps.

The cloud platform must expose usable cgroup v2 delegation and `cgroup.kill`.
An image cannot provide those kernel capabilities. The shared Sandbox process
controller must still enforce root-owned admission, UID 60000, no_new_privs,
controlled descendants and confirmation of `populated=0` before successful
stop. Local image acceptance does not claim cloud containment or an Agent run.


## Current image7 cloud templates

`image.lock.json` records image7 publication, native overlay identity and the
real native-tool integration receipts. `templates.lock.json` records immutable template/build
IDs, registry index/platform/config digests separately, resource readbacks and
accepted guest receipts. The Docker Image ID for this image is the OCI index
digest, not its platform config digest. Both providers use 2 CPU / 4096 MB:

| Provider | Template and build |
| --- | --- |
| E2B | `ka5ej4mq5hsk1kf91gvp:df971e4f-721e-4456-90d0-c6da803db29a` |
| Huawei AgentSphere | `7697f6d1-2b69-4090-88d8-1bb77e8adb44:db05fc28-683a-40a4-9382-d76f5c928775` |

The short real Sandbox probes verify 4477 unchanged upstream source files,
3 source overlays and the three installed agent-core overlays, the root-owned native
paths, pinned Python/native imports, private workspace writes, and the same
shared `sandboxprocess.ControlScript` used by the harness. Execution ran with
UID/GID 60000, no_new_privs and zero effective capabilities, kept a detached
descendant in the owned cgroup, revoked admission, confirmed `populated=0`,
denied subsequent execution and removed its control directory and empty
cgroup. Full source hashing belongs to controlled template acceptance; it is
not an extra required step for every Agent startup.

Image7 used exactly one TTL-300-second Sandbox per cloud. Both passed and
were deleted: E2B returned SDK NotFound after kill; Huawei returned DELETE 204
then GET 404. There were zero model calls in these capability probes.
The separate release23 image7 Agent/Verifier matrix and four actual backend
query validations subsequently passed within the scopes above; capability
probes alone do not establish that result. Huawei egress enforcement remains
assumed in the real runs and was not validated by these probes.

Current receipts and frozen probe inputs are under
`/opt/aries-artifacts/jiuwen-image7-cloud-1`; `closeout.json` links both probes.
The small `evidence.tar.gz` retains receipts, native manifest and exact recipes.
Publication verifies OCI index, its amd64 platform manifest and config digest
separately. Historical image5 evidence remains unchanged under
`/opt/aries-artifacts/jiuwen-image5-cloud-1` and
`/opt/aries-artifacts/jiuwen-image5-1/publication-2`.

## Profile selection and explicit runners

The runtime reads `sandbox.template` from each profile. External placement also
reads `harness.jiuwen_image`; embedded placement uses the native runtime inside
the selected template and must not declare that external image option.
These deployment locks provide evidence and pins; they are not automatically
loaded by the runtime. `configs/versions.json` has no Jiuwen-specific runtime
field, so this update adds none.

The four record profiles cover E2B/Huawei and A/B. The new
`profiles/cloud/jiuwenswarm-huawei-embedded-record.json` explicitly sets
`sandbox.assume_network_isolation=true`: this is the previously authorized
experiment assumption, **not proof of enforced outbound isolation**. Historical
replay pins and Tapes are unchanged. This profile availability does not claim
full image7 A/B execution has passed.

`prepare_e2b.py` and `prepare_huawei.py` are explicitly invoked ECS runners;
importing them does not allocate resources. They use existing private structured
inputs or adapter configuration in memory, never embed credentials in the
recipe, and delete their owned probe Sandbox in `finally`. A deliberately
requested new probe can use the already built Huawei template:

```sh
/opt/aries-artifacts/agentsphere-compatibility-1/venv/bin/python \
  /root/aries-setup/jiuwen-image7-cloud-1/recipe/prepare_huawei.py \
  --output /opt/aries-artifacts/NEW-UNUSED-PROBE-DIRECTORY \
  --bundle /root/aries-setup/jiuwen-image7-cloud-1/bundle \
  --template-reference 7697f6d1-2b69-4090-88d8-1bb77e8adb44:db05fc28-683a-40a4-9382-d76f5c928775
```

The image4 locks and real receipts remain unchanged in the history files and
`/opt/aries-artifacts/jiuwen-image4-release-1`. Historical image3 archives remain
under `/opt/aries-artifacts/jiuwen-image-1` and
`/opt/aries-artifacts/jiuwen-cloud-templates-archive-1`, including its failed
Huawei probes caused by an omitted SDK `cwd` and their later correction.
Neither those old results nor Huawei's reported ENVD version is rewritten.


## Image4 workspace-permission correction

The image3 cloud Agent startup exposed a permissions defect that import-only
acceptance missed. `chmod -R a+rX,a-w` made native template directories `0555`
and regular template files `0444`. Native `prepare_workspace` uses `copytree`
and `copy2`, which preserve these modes in the agent-owned private copy. It
therefore could not create `AGENT.md`; the already copied canonical `.env`
would also have rejected the bootstrap's subsequent write.

Image4 changes that existing install-layer command to
`chmod -R u+rwX,a+rX,go-w /opt/jiuwenswarm`. Sources remain root-owned and
unwritable by UID 60000, while copied private files retain owner write. No native
source, dependency lock, model setting, task or tool behavior changes. This is
part of the original install RUN, not an added layer duplicating the installed
native environment.

The same real UID 60000 bootstrap reproduced the old image's `AGENT.md`
PermissionError and passed on image4, including the subsequent configuration
writes and AgentServer readiness. Source workspace directories now have mode
`0755`, source `.env.template` is `0644`, and the initialized private canonical
`.env` ends at `0600`. `workspace_probe.py` adds the native copy-and-write check
to both cloud guest probes, so template acceptance also exercises this boundary.
Historical image3, template/build IDs, cloud startup failures and the old-image
regression remain archived; their results are not changed by image4 acceptance.

## Model failure evidence

Real local HTTP tests confirmed image5 already emits structured `chat.error`
both on the first model request and after a real Tool call. The old ARIES event
selection omitted this event, and its client ignored it when the native
envelope said `in_progress`. A later generic `succeeded` terminal therefore
made the Harness report success. The fix belongs to ARIES error recognition
and evidence retention; recoverable `chat.tool_result` errors remain separate.

The initially suspected native task-loop patch was not established as necessary.
Its unpromoted image6 candidate, source and counter-evidence are retained under
`/opt/aries-artifacts/jiuwen-image6-1`; the production recipe, locks and templates
remain image5. `review-closeout.json` records this correction and the actual
scope of each check. No image6 registry push or cloud template was performed.

## Native builtin skills

The Harness now registers all 20 skills from the pinned native builtin directory
through `JIUWENSWARM_SHARED_SKILLS_DIRS`. A and B use the same absolute read-only
source paths, so an external Agent does not refer its remote tools to an
Agent-private installation directory. Native skill bodies and scripts are
unchanged. `prepared.json` records manifest hashes; the actual native catalog
was checked under L0 and L1 on ECS. Optional skill-selection remains off while
native `skill_tool` remains available.

Registration does not install every skill’s optional software or credentials.
Image8's presentation toolchain has been built and published for
`llm-development-pptx-v1`; its immutable image, dependency versions and source
evidence are in [presentation/image.lock.json](presentation/image.lock.json).
The UID 60000 offline fixture generated and rendered a generic three-page PPT
through the original native template, LibreOffice and Poppler. This is an
environment fixture, not a scored Agent task.

Docker ENV is not automatically inherited through cloud envd and the contained
launcher. A's `worker.main` and B's `bootstrap.run` restore the native venv PATH
prefix and `NODE_PATH` only when `/opt/aries-presentation/node_modules` exists.
The focused ECS regression passed both actual UID 60000 entrances from a minimal
environment: ordinary native-tool `python3` and `node` resolved the dependencies,
exit status/cancellation behavior remained intact, and image7 kept its original
environment. B's final AgentServer exec was mocked; its configuration, shell
tool, 20-skill catalog and PPT `skill_tool` read were real.

The E2B/Huawei **4 vCPU / 8192 MiB** templates passed actual native runtime
capability probes: both A/B command entrances resolved the presentation
dependencies and preserved exit/cancellation behavior; B also exposed the
20-skill catalog and read PPT through `skill_tool`. Both probe sandboxes were
deleted and independently confirmed absent.
The scope still mocks B's final AgentServer exec and includes no model/task run;
these probes do not establish real PPT task success. The first release24 E2B A
task failed; its recording, capture and collection problems and corrected-run
status are tracked in the [PPT correction record](../../docs/jiuwen-pptx-capacity-20261027.md).
Exact provider receipts are pinned
in [presentation/templates.lock.json](presentation/templates.lock.json). See
[presentation/README.md](presentation/README.md) for the exact validation scope.
Keep earlier images and Tapes unchanged. Registering all 20 skills does not
establish that their external services, credentials or optional dependencies
are available.

For external placement, the native file delivery tool reads local paths directly.
ARIES first materializes files under the bound task workspace using the same
Sandbox FS provider, then calls the native delivery method. Each Run subscribes
to the native session push stream before submitting chat; received `chat.file`
metadata is retained separately from chat completion. The native protocol has no
cross-stream drain acknowledgment: successful chat and a positive native push
enqueue count do not establish that every notification was consumed. Original
PPTX retention and scoring use the independent post-Stop workspace capture,
not push notifications or expiring download URLs.

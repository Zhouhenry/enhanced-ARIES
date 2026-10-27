# Experiment configuration

Start with the [quick start](quick-start.md) for a first run. This reference owns
profile field meanings, defaults, compatibility rules, and advanced examples.
The original `aries PROFILE.json` schema and validation live in
[pkg/config](../pkg/config/config.go) and
[deployment normalization](../pkg/config/deployment.go). The separate
`aries cloud` command uses [CloudConfig](../pkg/config/cloud.go), documented in
[E2B cloud workload profiles](#e2b-cloud-workload-profiles); other sections below
describe the original profile path unless stated otherwise. Available
implementations are summarized in [supported implementations](supported.md).
For the latest accepted runs, stage timings, and evidence, see the
[performance archive](performance-archive.md).
Builds, tests, deployments, and experiments run only on the designated remote
Linux ECS; local work is limited to editing and static review.

## Profile structure

Start from a checked-in [original profile](../profiles). JSON examples below are fragments
unless explicitly identified as complete files. Keep the remaining required
fields from your chosen profile.

| Field | Purpose |
| --- | --- |
| `name` | Experiment name used in run identity. |
| `versions_file` | Pinned benchmark repositories and harness images. |
| `overrides_file` | Optional resource/deadline overrides; `""` disables loading. |
| `benchmark` | Benchmark type, checkout root, selected task occurrences, and supported evaluator options. |
| `harness` | Agent implementation, mode, placement, and optional tool/model settings. |
| `sandbox.deployment` | Task runtime placement. |
| `bridge` | Paired tool adapter, managed deployment, and optional raw evidence retention. |
| `runtime` | Model service backend and external/managed ownership. |
| `model` | Served model ID, endpoint, and credential environment-variable name. |
| `execution` | Concurrency, looping, arrival schedule, optional explicit Goodput deadline. |
| `output_dir` | Private run output root; defaults to `runs`. |

`versions_file`, nonempty `overrides_file`, and managed SGLang's `config.file`
resolve relative to the profile. Run from the repository root for checked-in
profiles: benchmark roots, output paths, and arrival trace paths use the working
directory. Profiles reject unknown fields and trailing JSON; there is no merge
or inheritance layer. Never put credential values in profile JSON.

## E2B cloud workload profiles

JiuwenSwarm Flash now has a separate [embedded candidate configuration](#jiuwenswarm-flash-embedded-candidate).
Its engineering checks do not establish cloud acceptance. Do not substitute its
executable into OpenClaw settings; see the [native design](design/jiuwenswarm-flash-review-20261026.md).

This cloud command reuses Runner's four roles and has its own profile schema.
The codingcase path selects a fixed Python benchmark with `workload` (default
`merge-ranges`), OpenClaw, a cloud ToolSandbox, and the matching external or
embedded ToolBridge. Its independent verifier tests a captured candidate in a
fresh sandbox of the selected provider. The separate [TerminalBench selection](#original-terminalbench-tasks-on-e2b)
preserves that benchmark's original environment and same-Sandbox evaluation.
**E2B A and B have passed
independent acceptance
for live recording and recorded-timing replay in new E2B instances. B's
zero-added-wait replay and the separate model-protocol checks have also passed.**
Each accepted workload passed all 12 verifier tests and confirmed isolation and
cleanup. Additional A C1×2, B C1×2 and B C2×3 replay batches and bounded lifecycle
fault probes have passed; see the [actual results](architecture-guide.md#84-重复并发和故障路径的实际结果).
These small samples do not establish capacity or global critical-path attribution.
Use the
[ECS deployment guide](../deploy/e2b/README.md) for prerequisites, build/check
commands, SDK smoke, the five-run matrix, and cleanup evidence.
Huawei B's real recording and recorded-timing replay have also passed all 12
independent tests, Stop/revocation, and cleanup under an explicit, unverified
verifier-egress assumption. Huawei A recording passed those checks under the
same assumption; its legacy E2B A tape replay diverged on filesystem metadata.
See [Huawei deployment](../deploy/agentsphere/README.md).
Those results use the original `merge-ranges` workload. A separate
`merge-ranges-portable-v1` experiment has now passed one real E2B A recording
and strict recorded-timing replays of its unchanged tape on E2B A and Huawei A:
each has eight model turns, seven real tools, and 12 independent tests, with
confirmed Stop/revocation and cleanup. Huawei retains the verifier-egress
assumption. See the [single-attempt comparison](performance-archive.md#portable-crosscloud);
the legacy failure is preserved and these samples do not establish a provider ranking.

```text
aries cloud [--env-file PATH] [--tape FILE] PROFILE.json
```

Flags precede the profile argument. `cloud` rejects non-Linux execution.
`--tape FILE` replaces a replay profile's placeholder `model.tape_file`; the CLI
path resolves from the working directory. It does not convert a record profile
to replay. Without that override, `versions_file`, `model.tape_file`,
`harness.ssh_client_path`, `benchmark.root`, and `sandbox.binding_file` resolve relative to the profile; `output_dir` uses the
working directory. Unknown fields and trailing JSON are rejected. Legacy
`runtime`, `bridge`, deployment, override, and arrival-trace blocks do not belong
in this schema. Its optional `benchmark` block selects one original TerminalBench
task as described below; it is distinct from the original profile schema.

| Profile under `profiles/cloud/` | Placement and model behavior |
| --- | --- |
| [openclaw-e2b-external-record.json](../profiles/cloud/openclaw-e2b-external-record.json) | A: Agent in an ECS Docker container; real model recording. |
| [openclaw-e2b-embedded-record.json](../profiles/cloud/openclaw-e2b-embedded-record.json) | B: Agent and tools inside E2B; real model recording. |
| [openclaw-e2b-external-replay.json](../profiles/cloud/openclaw-e2b-external-replay.json) | A: strict replay of A's own tape, with recorded frame timing. |
| [openclaw-e2b-embedded-replay.json](../profiles/cloud/openclaw-e2b-embedded-replay.json) | B: strict replay of B's own tape, with recorded frame timing. |
| [openclaw-e2b-embedded-replay-zero.json](../profiles/cloud/openclaw-e2b-embedded-replay-zero.json) | B: strict replay without added model wait; transport and controller overhead remain. |
| [openclaw-agentsphere-external-record.json](../profiles/cloud/openclaw-agentsphere-external-record.json) | Huawei A: Agent in an ECS Docker container; real tools in AgentSphere through SSH/Bridge; real model recording passed 12 independent tests under the verifier-egress assumption. |
| [openclaw-agentsphere-external-replay.json](../profiles/cloud/openclaw-agentsphere-external-replay.json) | Huawei A: strict replay with recorded frame timing and real tools. This original-workload profile's tested E2B A tape diverged on real filesystem metadata. The successful portable-v1 replay uses the separately versioned profile below. |
| [openclaw-agentsphere-embedded-record.json](../profiles/cloud/openclaw-agentsphere-embedded-record.json) | Huawei B: Agent and tools inside AgentSphere; real model recording. Recording and its archived replay passed conditionally under the verifier-egress assumption. |

Each codingcase profile above requests concurrency `1` and one attempt, a `5m` task timeout, and
a `1m` verifier timeout. A and B do not share tapes: placement, model ID, Harness
version, and complete request histories are checked. Replay still runs real
tools. A complete tape is model-protocol evidence, not a successful task score.
The Huawei profiles explicitly use `sandbox.token_env=ARIES_HUAWEI_ADAPTER_TOKEN`
and omit `sandbox.assume_network_isolation`, keeping its default `false`. They do
not silently enable the assumption used by the accepted Huawei experiments. Enabling
it requires both explicit settings described below. A's replay placeholder must
be replaced by a real external tape; an embedded tape is not compatible. Sharing
one tape across providers still requires exact request/history/tool-result
matching. The accepted portable-v1 pair below does not establish that an
arbitrary legacy tape is portable.

For newly prepared Sandbox templates and the current round of cloud comparisons,
the default resource selection is **4 vCPU / 8192 MiB** on both E2B and Huawei.
Apply this when creating and pinning templates; it does not add a profile field
or automatically resize an existing template. Historical templates, existing
profiles, Tapes and reports retain their original resource values. Do not relabel
earlier 2 vCPU / 4096 MiB evidence as 4 vCPU / 8192 MiB or rewrite historical
profiles to match the new selection default.

| Cloud field | Meaning and default |
| --- | --- |
| `agent_type`, `placement`, `sandbox_backend` | Required independent dimensions: `openclaw` or `jiuwenswarm`, each with `external` or `embedded`; backend is `e2b-cloud` or `huawei-agentsphere`. Jiuwen remains codingcase-only. Its B placement has scoped real cloud acceptance; the new A implementation awaits its own engineering and real-cloud acceptance. See [Jiuwen configuration](#jiuwenswarm-flash-embedded-candidate). |
| `benchmark` | Optional `{ "type": "terminalbench2", "root": "...", "task_id": "fix-git" }` selecting one original task from the clean checkout pinned by `versions.terminalbench2`. Requires external placement, `e2b-cloud` or `huawei-agentsphere`, attempts/concurrency `1`, a matching provider template binding, and no `workload` or timeout overrides. Both provider paths have scoped real acceptance; see the [case-by-case Strict and Workload results](performance-archive.md#跨云用例交付进度). |
| `benchmark.variant` | Optional `{ "id": "<distinct-variant-id>", "instruction_appendix_file": "<public-markdown>", "appendix_sha256": "<lowercase-64-hex>" }`. Selects a separately reported instruction variant of the original `task_id`; the appendix path is relative to the profile and its bytes must match the digest. Omission preserves the original task. See [instruction variants](#terminalbench-instruction-variants); candidate engineering and real recording/replay acceptance remain separate. |
| `sandbox.binding_file` | TerminalBench-only JSON binding of the original task environment to a frozen provider template; resolved relative to the profile. An omitted `sandbox.template` comes from this file; an explicit value must match. This mode never falls back to global provider template variables. The binding's provider must match `sandbox_backend`; an omitted binding provider means E2B only. |
| `workload` | Without `benchmark`, top-level string: `merge-ranges` (default), `merge-ranges-portable-v1`, `merge-ranges-multicall-v1`, or `llm-development-pptx-v1`; other values are rejected. The default retains the original behavior. Portable v1 changes only the task instruction given before execution, preserving the buggy source, public/hidden tests and independent verifier. One portable-v1 E2B A recording and its unchanged-tape E2B/Huawei A replays have passed; Huawei's egress assumption remains unverified. Multicall v1 uses a separate initial instruction with the same source and verifier; its real recording/replay acceptance is pending and does not inherit portable-v1 results. |
| `sandbox.adapter_url`, `template`, `token_env` | SDK adapter origin, fixed provider template, and credential variable name. E2B defaults: `ARIES_E2B_ADAPTER_URL` (or `http://127.0.0.1:8787`), `E2B_TEMPLATE_ID`, `ARIES_E2B_ADAPTER_TOKEN`. Huawei defaults: `ARIES_HUAWEI_ADAPTER_URL` (or `http://127.0.0.1:8788`), `HUAWEI_SANDBOX_TEMPLATE_ID`, `ARIES_HUAWEI_ADAPTER_TOKEN`. Explicit profile values take precedence; one provider never falls back to the other's environment. |
| `sandbox.assume_network_isolation` | Huawei-only explicit verifier-egress assumption, default `false`. Must match the Adapter's assumption setting before allocation; results remain labeled assumed/unverified. It does not waive Stop, revocation, independent tests, or cleanup. |
| `sandbox.timeout_seconds` | Provider TTL, defaulting to the configured maximum; this is distinct from task/verifier deadlines and cleanup confirmation. |
| `harness.openclaw_path`, `openclaw_version` | OpenClaw only: absolute native executable and explicit version. Checked-in profiles use `/usr/local/bin/openclaw` and `2026.7.1`; by default the catalog image must be `ghcr.io/openclaw/openclaw:<version>`, optionally followed by a complete lowercase `@sha256:<digest>`. The version tag must match. The explicit experimental options below permit a derived, digest-pinned image; native startup version verification remains required. |
| `harness.jiuwen_source_dir`, `jiuwen_python_path` | Jiuwen only, both required: clean absolute paths to the pinned checkout and prepared Python interpreter. B uses these inside its Sandbox; A requires the same paths in both the Docker Agent image and the tool Sandbox. These are not controller filesystem paths. See [Jiuwen constraints](#jiuwenswarm-flash-embedded-candidate). |
| `harness.jiuwen_image` | Jiuwen A only and required: explicitly tagged native Docker image with a full `@sha256:` pin. B rejects this field and uses its provider template. This is independent of the OpenClaw catalog image. |
| `harness.ssh_client_path` | OpenClaw A only: controller path to the Linux SSH helper. Jiuwen rejects this field. |
| `harness.bridge_host`, `docker_socket` | External placement: private ECS bridge address reachable from the Docker Agent and local Docker socket. Both OpenClaw A and Jiuwen A use them; `bridge_host` falls back to `ARIES_ECS_PRIVATE_IP`. Jiuwen B rejects these fields and creates no Docker Agent runtime. |
| `harness.native_observation_protocol` | OpenClaw only. Omitted or `""`: existing behavior. `aries-ssh-identity-v1` is external-only native ToolCall identity through SSH; `aries-native-process-v1` is embedded-only direct-child callback observation through the native Gateway logger. Each requires its matching digest-pinned candidate image; B also requires its matching template. |
| `harness.tool_output_renderer` | OpenClaw only. Omitted or `""`: existing behavior. The only nonempty value is `stdout-stderr-v1`, accepted for A and B. Records the candidate renderer version and requires its digest-pinned image. It does not change native tool policy, schemas or parameters; `process`, background execution and auto-yield stay available. |
| `harness.replay_tool_protocol` | OpenClaw only. Omitted or `""`: existing behavior. `terminal-result-v1` selects the versioned background-operation/final-result candidate and requires external placement, `aries-ssh-identity-v1`, `stdout-stderr-v1`, and a matching digest-pinned native image. This is a candidate contract, not a guarantee of identical output across runs. |
| `model.mode`, `tape_file`, `timing` | `record` or `replay`; replay requires a nonempty `tape_file` pointing to a complete tape and `recorded`, `zero`, or `synthetic` timing. The CLI `--tape` override is applied after profile validation, so the profile field remains required. Record rejects replay-only fields. |
| `model.replay_policy` | Replay only: omitted or `strict` compares the complete request. Explicit `workload` allows only string-valued `role=tool` message content to differ; IDs, names, arguments, history structure/order and every other request field remain exact. Real tools and independent verification still run. No fallback; results retain their policy. Release12 passed all five ECS gates and same-Tape E2B/Huawei DB and SQLite Workload acceptance, including tool terminals, original scores, dual Trace reconciliation and cleanup. Historical failures remain separate. See [DB evidence](workload-db-performance-20261026.md), [SQLite evidence](workload-sqlite-performance-20261026.md), [policy boundaries](implementation/model-replay.md#workload-replay-policy), and [release12 gates](observability/VALIDATION_AND_REPRODUCE.md#workload-replay-release12). |
| `model.tool_call_id_dialect` | Omitted: exact response/request IDs. The supplied profiles explicitly select `openclaw-2026.7.1`, requiring that harness version. Recording and tape loading verify its bounded native ID conversion; raw requests/SSE and full-request replay comparison stay unchanged. Replay profile and tape must agree before cloud effects. |
| `model.id`, `base_url`, `api_key_env` | Explicit model ID, record upstream API root, and key variable name. Defaults use `DEEPSEEK_MODEL`, `DEEPSEEK_BASE_URL` (or `https://api.deepseek.com`), and `DEEPSEEK_API_KEY`. No model name is guessed. |
| `model.proxy_public_url`, `proxy_listen` | Public HTTPS origin from `ARIES_MODEL_PROXY_PUBLIC_URL`; public IP or DNS name, with no path, query, credentials, or fragment. IP deployment also sets the Caddy-only `ARIES_MODEL_PROXY_TLS_NAME` to the same bare IP for clients without SNI behind ECS NAT; see [deployment](../deploy/e2b/README.md). Listener is fixed to `127.0.0.1:8082` for the supplied Caddy wiring. |
| `model.max_turns`, `reasoning_effort` | Maximum recorded model requests at the proxy (default `32`, range `1..1024`), separate from native Agent iteration limits. OpenClaw accepts an optional validated reasoning setting; Jiuwen currently rejects it. `max_tokens`, `context_length`, and `temperature` overrides are currently rejected for both. |
| `model.synthetic_initial_delay`, `synthetic_frame_interval` | Positive Go durations only with `synthetic`; omitted values add no corresponding delay. Initial delay is at most `1h`, frame interval at most `1m`. |
| `execution.concurrency`, `attempts` | Finite scheduler capacity and planned occurrence count, default `1` each, further limited by the environment guards below. |
| `execution.task_timeout`, `verifier_timeout`, `goodput_deadline` | Positive Go durations; codingcase defaults `5m`, `1m`, and no Goodput SLO. TerminalBench omits the first two and uses its original task budgets. These do not automatically extend provider TTL. |

<a id="jiuwenswarm-flash-embedded-candidate"></a>

### JiuwenSwarm Flash cloud placements

Candidate E2B examples: [record](../profiles/cloud/jiuwenswarm-e2b-embedded-record.json)
and [Strict Replay](../profiles/cloud/jiuwenswarm-e2b-embedded-replay.json).
They explicitly select the new 2 CPU / 4096 MiB template, whose guest capability
probe and deletion have passed. Release18 E2B record7 and Huawei record2 each
completed real native solving and the original 12/12 verifier, with confirmed
Stop/revocation and task/evaluation sandbox deletion. Their nine-turn and seven-turn
Tapes contain different decisions, so this is functional acceptance rather than
a same-Tape comparison. Huawei verifier egress remains explicitly assumed.
Strict replay3 failed at its first request; a complete Tape does not promise
successful Strict replay. For replay, supply the
resulting real Jiuwen `tape.json` with `--tape`; the example placeholder is not
a recording. The adapter must allow this exact template. Image/source pins are
in [the image lock](../deploy/jiuwenswarm/image.lock.json).
These examples request a 900-second sandbox TTL; set the matching private
`ARIES_SANDBOX_TIMEOUT_SECONDS=900` operator ceiling (the default is 600).

`agent_type=jiuwenswarm` selects the native Flash Harness with
`placement=embedded` or the new `external` candidate on either existing cloud
adapter. It accepts codingcase workloads only; original TerminalBench tasks
are rejected before credential lookup or allocation. Real cloud acceptance is
limited to the codingcase and fixed image/source combination linked above;
Strict acceptance and full observability remain incomplete. Validation-mode
discussion is deferred and the fixed-prompt variant is paused.

Supply `harness.jiuwen_source_dir` and `harness.jiuwen_python_path` as absolute
paths inside the selected runtime. The provider template must already contain
the native source and frozen Python environment; A also requires them at the
same paths in its Docker Agent image. Bootstrap checks the pinned source
revision `e2c9d519498080e249e34f16a80c8c97f0cd1962` and `uv.lock` SHA-256
`f28fbbd9b208bcf4f6e421818969ed2b6570f0648bed25c2ed15ee5bd551e5c9`;
it does not install dependencies during a task. `versions_file` remains the
shared catalog, but its OpenClaw image is not selected or compared to a Jiuwen
version. Runtime configuration and evidence are owned by the Jiuwen Harness.

Native builtin skills are now registered by default through Jiuwen’s own
`JIUWENSWARM_SHARED_SKILLS_DIRS` entry point. The pinned source supplies 20 skills,
including `ppt-creation`; A and B resolve the same read-only builtin directory
inside their respective images. `prepared.json` records the directory and
manifest hashes. ECS L0/L1 bootstrap checks discovered all 20 through the actual
FlashAdapter/SkillUseRail catalog. This does not imply that every skill’s
external service credentials or executable dependencies are available.
`flash.skill_selection.enabled=false` disables only the optional installed-skill
retrieval extension; it does not disable native `skill_tool` or this catalog.
The shared catalog changes the available prompt/tool context: record a new Tape
for the new configuration; earlier recordings remain unchanged.

Use the existing per-attempt model handler with `model.mode=record` or
`replay`; Jiuwen replay accepts only omitted/default or explicit `strict` policy
and requires a fresh Jiuwen Tape. Existing OpenClaw tapes are not compatible.
Set `model.session_protocol=jiuwen-tool-branches-v1` for native `task_tool`
subagents. This cloud-only protocol uses explicit native ToolCall IDs and one
immutable v4 Tape containing independent child histories, including nonstreaming
JSON completions. Both A and B stage the same extension through the native loader.
Omission preserves the previous single-trajectory contract; a replay profile must
agree with its Tape. It does not relax Strict matching or fix dynamic prompts.
See [protocol and acceptance](implementation/model-replay.md#jiuwen-子-agent-与非流式-json).
The Tape's harness identity contains `jiuwenswarm:<source-revision>:<lock-sha256>`;
replay rejects a different native source or dependency lock before cloud preparation.
The native recorder also declares the pinned `jiuwenswarm-e2c9d519-request-tail-call-goal-v2`
history dialect. This is selected by the concrete Harness, not a user matching
policy: it recognizes the native request-only user attachment after each new
assistant/tool group. That attachment can update between recording turns;
persistent history and tool causality remain checked. Every replay request still
compares against its complete recorded request, including the attachment. A
replay attachment difference or rewritten persistent history is rejected.
For response-to-history causality, this declaration also applies the pinned
native removal of top-level `call_goal` and `callGoal` display fields. Execution
arguments, nested fields, original SSE and recorded requests remain intact;
Strict replay still compares the complete recorded request. The earlier
`request-tail-v1` and `fixed-tail-v1` declarations retain their original rules.
The standalone model CLI derives the current dialect
from the exact Harness pin when recording and reads the declaration from the Tape
when replaying; it has no new permissive matching flag.
`model.max_turns` bounds proxy model requests, not native Flash iterations.
Explicit `reasoning_effort`, `max_tokens`, `context_length` and `temperature`
overrides are rejected until a supported native mapping exists.

Omit OpenClaw paths/version, SSH client, native observation/output protocol
selectors, and `tool_call_id_dialect`; Jiuwen rejects these options. It retains
native tools and results and does not receive OpenClaw `WorkloadInputs`. In B,
the native AgentServer, Flash and LOCAL tools run inside the exact Runner-owned
Sandbox. Confirmed Harness stop and bridge revocation still precede the
independent codingcase verifier. See [implementation status and evidence](implementation/jiuwenswarm-flash-progress-20261026.md).

<a id="jiuwenswarm-flash-external-candidate"></a>

#### External placement candidate (A)

The new [E2B A record profile](../profiles/cloud/jiuwenswarm-e2b-external-record.json)
and [Huawei A record profile](../profiles/cloud/jiuwenswarm-huawei-external-record.json)
select image4 by digest in `harness.jiuwen_image`. A places native AgentServer and
Flash in an ECS Docker runtime on the existing per-run network; its paired
RemoteBridge grants temporary access to the exact Runner-owned tool Sandbox.
The guest worker uses the pinned native operation implementation. Neither the
Agent nor the worker allocates a cloud Sandbox or receives provider credentials.
The bridge address must be reachable from that Docker network; configure
`ARIES_ECS_PRIVATE_IP` or explicit `harness.bridge_host`, and the local Docker
socket. No OpenClaw SSH helper or tool-output renderer is used.

These profiles retain the 900-second provider TTL and original 5-minute task /
1-minute verifier budgets. Huawei defaults to no verifier-egress assumption;
experiments that require the existing assumption must enable it explicitly in
both profile and adapter. The A candidate is not part of release18 and has no
real-cloud acceptance yet. Its config, native Harness, cloud wiring and app packages
passed targeted ECS unit/race/vet checks, and both public profiles passed
`LoadCloud` validation (`/opt/aries-artifacts/jiuwen-external-wiring-2`, evidence
SHA-256 `0d8eceaf4d95a6264a4d41018a3756a7e2e38107b9d8b543fef2fa5d0b3eefc6`).
These checks did not execute the opt-in native host/Docker fixture or a cloud
attempt. Native ToolCall correlation and full observation coverage must not be
inferred from B's successful results. Model policies and
exact placement/Tape identity checks are unchanged; no Workload extension or
fixed-prompt variant is enabled.

Release18 passed all five ECS engineering gates. Its L1/L2 observations include
official native spans, four cgroup resource boundaries and a sampled SDK producer
audit; L0 does not enable detailed native spans. E2B record7 reconciled 44 started,
ended and exported spans, and Huawei record2 reconciled 35; both had zero missing
or unexpected exports. This scoped coverage is complete, while global coverage
remains unknown. It does not cover unsampled or uninstrumented work, process
timing, cross-host clock alignment or bounded flush. No user matching or
configuration option is added. See [release and cloud evidence](observability/VALIDATION_AND_REPRODUCE.md#jiuwen-release18)
and [earlier targeted checks](observability/VALIDATION_AND_REPRODUCE.md#jiuwen-producer-audit-candidate).

<a id="original-terminalbench-tasks-on-e2b"></a>

### Original TerminalBench tasks on cloud sandboxes

The profiles under [profiles/cloud/terminalbench](../profiles/cloud/terminalbench/)
select one task each. Run them sequentially for the single-task round; this adds
no arrival-rate or concurrent-load policy. Without `benchmark`, cloud codingcase
profiles keep their existing defaults. E2B engineering and individual original-task
results are recorded in the [six-case archive](tb-six-performance-20261026.md).
Huawei A also has scoped real task results; the [current case matrix](performance-archive.md#跨云用例交付进度)
separates Strict acceptance from DB/SQLite Workload acceptance and preserves
failures and unrun cases. Each new provider/template combination still requires
its own preparation and real acceptance; it does not inherit codingcase results.

Prepare the original clean Git checkout at the catalog revision and publish the
task's template for the selected provider on ECS first. The task template contains the original runtime
inputs and declared generic tools; it contains neither private tests nor a
reference solution. A's OpenClaw remains in its separate ECS Docker runtime.
The binding has the following fields:

| Binding field | Meaning |
| --- | --- |
| `schema_version`, `task_id`, `benchmark_revision`, `requested_environment` | Version `1`, exact selected task/commit, and the original `core.Environment`, including image tag, resource requests, network setting and environment variables. The task is not rewritten to hide resource requests. |
| `provider` | Optional `e2b-cloud` or `huawei-agentsphere`. Omission preserves existing E2B bindings; Huawei requires the explicit value. Must match the profile backend before allocation; a binding cannot silently switch cloud providers. |
| `source_image_digest`, `template`, `build_evidence_sha256` | Pinned source image digest, provider `TemplateID:BuildID`, and the hash of the archived build evidence. Keep separate provider bindings even when the source image and original task are shared. |
| `workdir`, `template_config` | Resolved original image working directory and configured template `cpu_count`, `memory_mb`, nullable `storage_mb`, and `gpu_count`. Build configuration is separate from actual creation/SDK/resource readback; unknown storage quota remains unknown. |
| `command_env`, `adaptations` | Explicit environment additions and descriptions of differences such as CPU sizing, UID 60000, generic build tools, or an added writable task install directory in PATH. Per-command native/Verifier environment values take precedence. |
| `initial_directory_mtime_unix_ns` | Optional map of clean absolute directory paths to nonnegative Unix nanoseconds, requiring an adaptation note. For example, `{"/": 1761350400000000000}` initializes only root-directory mtime after sanitization and Bridge workspace setup, before Harness startup. No file contents, undeclared paths, ToolResults or clocks are normalized. |

`task-environment.json` archives the original Task and selected binding. The
optional directory adaptation also saves actual before/after mtime and atime in
the occurrence's `bridge/initial-directory-mtimes.json`. It preserves atime;
the real filesystem update naturally changes ctime. This sets initial state only:
later Agent/filesystem activity may change mtime normally. Failed updates or
readback fail preparation; old bindings without this map perform no update.
Record a new Tape under the new binding; historical Tape and evidence remain intact.

The original task.toml Agent budget reaches the Harness and model proxy; the original
private Verifier budget remains owned by TerminalBench. Omit cloud
`execution.task_timeout` and `verifier_timeout`. Set provider TTL and its existing
environment/Adapter maximum high enough for preparation, both budgets, and
cleanup; a TTL no longer than the Agent budget is rejected before allocation.
Choose `model.max_turns` explicitly for the recorded workload when needed.

The [Huawei C++ recording profile](../profiles/cloud/terminalbench/openclaw-huawei-a-custom-memory-heap-crash-record.json)
selects `custom-memory-heap-crash`, its own Huawei binding, and a 4200-second TTL,
preserving the original 1800-second Agent and 1800-second Verifier budgets.
Provider/account TTL acceptance still needs real validation. This original task
allows network access and uses same-Sandbox private evaluation; the profile does
not enable `assume_network_isolation` or claim a fresh no-egress verifier sandbox.
Use the Huawei adapter and credentials; do not substitute an E2B binding or lower
the task budgets to fit a provider limit.

Original `PrepareSandbox` removes and checks the private verifier paths. After
confirmed Harness stop and Bridge revocation, original `Evaluate` injects tests
and runs them in the same task Sandbox, using the original reward/CTRF checks.
It does not introduce codingcase's fresh no-egress verifier environment. Score,
network conditions, observation coverage and cleanup are separate outcomes.
Without a proven candidate capture, artifact-deadline Goodput remains unknown.

For the adapted SQLite environment, generic gcc/make/gcov may be installed and
declared in `adaptations`; SQLite itself must still be built by the Agent.
Propagate the same declared PATH to tool calls and Verifier execution, keeping
trusted system paths first and task installation paths such as `/app/bin` last.
Template ENV alone is insufficient because native tool containment constructs
its own process environment. Do not alter the original prompt or private tests.

#### TerminalBench instruction variants

The explicit `benchmark.variant` candidate supports the two approved
[large-text and SQLite workloads](design/reproducible-agent-workloads.md).
`benchmark.task_id` still selects the original task in the clean, pinned checkout
and validates its original provider template binding. `variant.id` must be a
different valid task name; scheduling, attempt IDs and operation evidence use
this separate logical identity so variant results cannot silently enter the
original task's statistics. The internal template-task identity remains original.

The loader reads the UTF-8 public appendix once and checks its lowercase SHA-256
before runtime construction. The effective task instruction is exactly the
original loader's `Task.Instruction` (its existing surrounding-whitespace trim),
followed by two LF bytes and the appendix file's **unchanged bytes**, including
its final newline. It does not edit the checkout or replace the original task
instruction file. Environment, Agent/Verifier budgets, private tests, Stop/revocation
and evaluation remain unchanged. This is a declared instruction variant, not an
unmodified official benchmark result or a relaxation of Strict Replay.

`task-environment.json` keeps `original_task` and the original binding. For an
explicit variant only, it also records `effective_task` and `variant` with
`id`, `instruction_appendix_file`, `original_instruction_sha256`,
`appendix_sha256` and `effective_instruction_sha256`. The two instruction hashes
cover the strings actually supplied by the benchmark, not the entire model HTTP
request; the exact full request remains in the model tape. A changed appendix
hash, unreadable/empty appendix or invalid identity fails preparation. No cloud
instance is created by this instruction preparation step.

The [variant profiles](../profiles/cloud/terminalbench/variants/) also select
the explicitly versioned `terminal-result-v1` tool protocol and require its
matching pinned native image. Keep their images, profiles, tapes and results
separate from original runs. The [large-text v2 variant](repro-variants-performance-20261026.md#text-v2-huawei-replay-1)
has same-Tape two-cloud Strict acceptance; [SQLite v3](workload-sqlite-performance-20261026.md)
has two-cloud Workload acceptance with its Strict failure retained. New variants
need their own engineering, real recording and explicitly selected replay acceptance.

### Experimental native observation and output

Native observation and output rendering are independent, explicit experiment
inputs. The replay tool protocol below requires both specified A-mode options.
Omitting these fields preserves existing profiles. Enabling native observation or
output rendering permits a derived OpenClaw image in
`versions_file`, but requires an explicit non-`latest` tag and a complete lowercase
`@sha256:<64-hex-digest>` pin. Archive the actual candidate digest and source
manifest; keep `harness.openclaw_version` at the verified native base version.
Startup still checks that version. Configuration does not install a patch or
prove that the selected image contains the required helper or renderer. B also
needs a template built from the matching candidate.

For `aries-native-process-v1`, embedded staging adds the launch-only
`ARIES_NATIVE_PROCESS_OBSERVATION=1` when observation is L0/L1/L2; baseline
leaves it unset. The candidate factory consumes and deletes this variable before
tools inherit their environment. Existing Gateway logging carries native
run/session/ToolCall IDs, direct-child exit, and natural stdout/stderr EOF
callbacks into the same analysis snapshot. Native ToolResult, background/poll,
PTY, Stop and Verifier behavior remain unchanged. The Node monotonic clock is
unaligned with the controller clock; callback durations must not be summed with
controller receipt intervals. Missing descendants/PTY/coverage evidence remains
unknown. See [current validation](observability/VALIDATION_AND_REPRODUCE.md#r07-native-process-snapshot)
for actual evidence and limitations.

For `aries-ssh-identity-v1`, Runner binds the external harness and Bridge to the
same run/occurrence scope and native Agent session. The candidate reads the
private, read-only `/run/aries/tool-identity-scope.json` mount. Its separate
producer artifact becomes `harness/native-tool-identities.jsonl`; the companion
index reports file integrity, not complete ToolCall coverage or timing. Missing
or invalid evidence is an error. Reconcile it against an independent native
session/Tape denominator before claiming full identity coverage. Auxiliary
commands without native ToolCall metadata remain explicitly unlinked.

`stdout-stderr-v1` records the proposed final-output contract: sanitize each
stream, join stdout followed by stderr without an inserted separator, then apply
bounded truncation and native exit formatting. This does not reconstruct global
write order across streams. The configuration marker changes no native tool
policy, schema or parameter. `process`, explicit background execution and native
auto-yield remain available; disabling them is not an accepted workaround.
Child argv, environment, stdin, stream bytes and exit status remain unchanged.
Earlier foreground-only PoCs do not establish equivalent behavior for background,
poll/log, PTY or live updates. Full native behavior still needs candidate validation.

The changed ToolResult contract requires a fresh real recording and strict
replay of that new tape. The pinned `full-agent-2` E2B A candidate completed both,
with six archived exec calls and 12 independent verifier tests per run. The
`p0-runtime-release-2` Huawei A replay also passed on that unchanged tape, with
confirmed Stop/revocation and cleanup; Huawei verifier egress remains explicitly
assumed/unverified. These runs exercised foreground non-PTY exec, not every
background/process/PTY branch. Preserve earlier tapes and failures; do not
normalize replay requests or relax the matcher. See the
[versioned validation evidence](observability/VALIDATION_AND_REPRODUCE.md#p0-runtime-candidates).

`replay_tool_protocol=terminal-result-v1` is a separate, opt-in candidate for
background operation handles and waiting for the final tool result. It requires
external placement, `native_observation_protocol=aries-ssh-identity-v1`, and
`tool_output_renderer=stdout-stderr-v1`. The controller passes
`ARIES_REPLAY_TOOL_PROTOCOL=terminal-result-v1` to the native harness process;
it does not rewrite the OpenClaw configuration JSON or install the native patch.
Embedded placement rejects this setting. Existing `process`, background and PTY
capabilities remain available; the native candidate has its own validation scope.

The separate private `harness/replay-tool-protocol.jsonl` artifact and companion
index retain native protocol evidence. `file_integrity=retained` means bounded
private bytes were saved; it does not establish event semantics, complete call
coverage or replay success. Missing files are `not_collected`, collection failures
are `incomplete`, and neither changes the Agent's business result. Acceptance must
reconcile those records with the original native calls and ToolResults. Engineering
tests and real acceptance remain separate. [Large-text v2 Strict](repro-variants-performance-20261026.md#text-v2-huawei-replay-1)
and [SQLite v3 Workload](workload-sqlite-performance-20261026.md) each passed two-cloud
job/wait lifecycle checks, with two jobs and two waits per run and the original
verifier passed. These scoped results do not cover every background/PTY branch
or establish alignment of the native background clock to the primary trace.
Create a new recording for this protocol. Strict history matching, original task
and private Verifier remain unchanged; command output can still vary between runs.

### Versioned cloud workloads

`llm-development-pptx-v1` is the user-supplied Jiuwen presentation task: “幻灯片 生成一份关于大模型发展的pptx材料，写5页。” The prompt is unchanged. After confirmed harness stop and bridge revocation, ARIES captures every workspace PPTX and evaluates the immutable files in a fresh verifier sandbox. At least one parseable PPTX must contain exactly five slides, each with nonempty body content; all individual file results remain visible. This is structural validation, not factual or visual-quality scoring. Four separate `jiuwenswarm-{e2b,huawei}-{external,embedded}-pptx-record.json` profiles use real model recording and the dedicated 4 vCPU / 8192 MiB templates on both providers. Release24 passed its engineering gates, but the first E2B A task failed before independent evaluation; the other three were not run on that version. Capacity fixes and new real-run acceptance are tracked in the [PPT correction record](jiuwen-pptx-capacity-20261027.md); earlier coding-task evidence does not cover this workload. Capture retains original files without replacement by a reference solution; the correction record explicitly identifies the first run's failed downloads and lost originals.

Set top-level `"workload": "merge-ranges-portable-v1"` explicitly for the new
portable experiment. Omitting it, or selecting `merge-ranges`, preserves the
existing task instruction and behavior; old profiles do not silently migrate.
The resolved `cloud-profile.json` records the effective selection and must be
archived with the tape and run evidence.

`merge-ranges-multicall-v1` is a separate workload with a new initial instruction
and the same source, tests, and independent verifier. Its real recording and
strict replay have not yet been accepted; the portable-v1 results below do not
cover it. It requires its own new recording and acceptance evidence.

Portable v1 supplies different instructions **before the Agent starts** so the
workload can avoid platform-dependent observations such as directory metadata.
It does not change `buggySource`, public or hidden tests, candidate capture,
verification, or tool results. The Agent still performs real tool calls. A new
instruction is a new experimental workload, not a correction to an old recording.

The accepted experiment used one real E2B A recording followed by E2B A and
Huawei A recorded-timing replays. All three passed 12 independent tests and
the combined 16-check review; each executed eight model turns and seven tools.
The immutable source tape SHA-256 is
`07be09fa95cb3678c94c43accdc4b96ae079d6ae2352505091eeff134e6065bd`.
The original recording's final read-only `cat intervals.py` was wrongly rejected
by the first offline checker; its failed evidence remains preserved. A corrected
review adopted the original files with matching hashes, without another model
call or any tape rewrite. See [results and provenance](performance-archive.md#portable-crosscloud).

| Profile under `profiles/cloud/` | Use and observed acceptance |
| --- | --- |
| [openclaw-e2b-external-portable-record.json](../profiles/cloud/openclaw-e2b-external-portable-record.json) | E2B A: one real-model recording accepted after offline review; the original artifacts were preserved. |
| [openclaw-e2b-external-portable-replay.json](../profiles/cloud/openclaw-e2b-external-portable-replay.json) | E2B A: one strict replay of that unchanged tape with recorded timing passed. |
| [openclaw-agentsphere-external-portable-record.json](../profiles/cloud/openclaw-agentsphere-external-portable-record.json) | Huawei A: separate real-model recording entry; this portable profile has not itself been exercised. The accepted source was recorded on E2B. |
| [openclaw-agentsphere-external-portable-replay.json](../profiles/cloud/openclaw-agentsphere-external-portable-replay.json) | Huawei A: one strict replay of the same E2B source tape passed under the explicit, unverified verifier-egress assumption. |

For a cross-cloud pair, choose one new real recording as the source and pass its
unchanged tape to both replay profiles with `--tape FILE`. Two independently
recorded tapes do not become a fair pair merely because both profiles select
portable v1. Keep placement, versions, dialect, timing, input files, resources
and independent validation conditions aligned; A and B still use separate tapes.

Existing tapes and failed evidence stay immutable. Do not normalize real Tool
Results, omit old history, change recorded SSE or add a fallback to make replay
pass. A divergence remains a failed replay; switching the `workload` field or
renaming a tape cannot convert it to the new workload. New acceptance requires
full strict replay, real tools, independent verification, Stop/revocation and
confirmed cleanup. Huawei verifier-egress assumptions remain separately labeled
and are not enabled implicitly by a portable profile.

### Cloud runtime inputs and credentials

For an archived A/B experiment, point both profiles' `versions_file` and the
[E2B template builder's](../adapters/e2b/template/README.md) `--versions-file` at
the same catalog containing the actual digest resolved on ECS. The external
Docker harness and template builder preserve that full reference. Existing
tag-only catalogs remain valid for the default configuration, but a version tag
alone is not an immutable lock. The experimental native options above require
the digest pin.

The public model API is fixed at `/v1/models` and `/v1/chat/completions`.
Runner generates a high-entropy bearer token per attempt and uses it to select
that attempt's Handler and cursor. Session binding never relies on a prompt or
request arrival order. OpenClaw retains the DeepSeek Chat Completions dialect
while its transport points at the proxy. A model-catalog preflight is not proof
that the real native tool/reasoning protocol has passed. The full-history,
retry, cancellation, and SSE contract is in [strict replay](implementation/model-replay.md).

OpenClaw cloud A/B wiring also supplies native `WorkloadInputs`: the same fixed session key
and UUID for record/replay, `agents.defaults.envelopeTimestamp="off"`, and
`agents.defaults.skipBootstrap=true`. The bootstrap switch prevents automatic
creation of default workspace files and a Git repository; existing bootstrap
files may still be read. The fixed coding benchmark sets only its three public
files and task/parent directory mtimes to `1704067200` (2024-01-01 UTC) during
preparation. It retains atime, ownership, and modes; ctime, subsequent Agent
writes, and time-dependent commands remain outside this stabilization.
Each attempt still receives its own Gateway, private state, model bearer token
and random idempotency key. External placement requests container hostname
`aries-workload`; B's real recording and new-instance replay both observed
`e2b.local`. The implementation does not change guest hostname; subsequent
instances must still satisfy strict request matching. Ordinary
profiles omit these inputs. These are workload inputs, not model-request
normalization; existing tapes need recording again and strict matching is
unchanged. See [native input limits](implementation/model-replay.md#原生输入稳定性与实测边界).

Workstation SSH access is separate from the model proxy. The task-local
`.cache/ecs-ssh.py` can use `ARIES_ECS_SSH_PROXY=socks5://127.0.0.1:7890`, an explicit
existing local SOCKS5 endpoint; only that loopback address and a valid port are
accepted. It retains host-key verification and password askpass, with no
automatic route fallback. The setting is not forwarded to ECS services or
Sandbox/Agent environments. See [SSH and SCP setup](../deploy/e2b/README.md#先准备这些条件).

`--env-file` accepts a private regular file, rejects symlinks and group/other
permissions, and parses literal `KEY=value` assignments with whole-line comments
and matching single/double quotes. It performs no shell evaluation, interpolation,
or process-environment mutation. File keys take precedence over process
environment keys, including explicitly empty values; duplicate keys fail.
Cloud does not discover the legacy `DEEPSEEK_API.key` file. Compose's own
`--env-file` has different interpolation rules; follow the deployment guide.
Never stage the whole private file into a harness: only the SDK adapter needs
`E2B_API_KEY`, only the record proxy needs the upstream key, and the Agent receives
its temporary model-proxy token. Replay constructs no upstream client.

The separate Huawei Adapter consumes `HUAWEI_AGENTSPHERE_API_KEY`,
`HUAWEI_AGENTSPHERE_API_URL`, `HUAWEI_AGENTSPHERE_GATEWAY_URL`, and
`HUAWEI_SANDBOX_TEMPLATE_ID`. `HUAWEI_AGENTSPHERE_GATEWAY_ID` is a template
preparation input, not a Runner credential.
`HUAWEI_VERIFIER_TEMPLATE_ID` is a separate `TemplateID:BuildID` preparation
input for an operator-created VPC template with restricted outbound rules. It is
reserved for network-isolation verification; the current CLI, Adapter and Compose
do not consume it or treat it as proof that no-egress is enforced.
Optional ECS-only cloud-network preparation uses the Huawei SDK credential names
`HUAWEICLOUD_SDK_AK`, `HUAWEICLOUD_SDK_SK`, `HUAWEICLOUD_SDK_SECURITY_TOKEN`,
and `HUAWEICLOUD_SDK_PROJECT_ID`. Temporary AK/SK and the security token must be
provided together for the `cn-southwest-2` project. For permanent AK/SK downloaded
from the console, leave `HUAWEICLOUD_SDK_SECURITY_TOKEN` empty. These are IAM credentials,
not the AgentSphere API Key, and are not passed to Agent or runtime containers.
The current CLI, Adapter and Compose do not consume these preparation fields.
See [VPC authentication](https://support.huaweicloud.com/api-vpc/vpc_api00_0010.html).
The account's confirmed control-plane URL is
`https://agentsandbox.cn-southwest-2.myhuaweicloud.com`; the Gateway URL must
match the gateway bound to its template. Keep this key separate from `E2B_API_KEY`;
the Runner only receives `ARIES_HUAWEI_ADAPTER_TOKEN` for its internal Adapter.
Huawei uses a separate SDK environment and state volume. It checks the Adapter's
provider before allocation and preserves the provider in creation evidence.

The Huawei external and embedded profiles use the same coding case and verifier.
`agentsphere-embedded-record-3` and `agentsphere-embedded-replay-1` each passed
all 12 independent tests, confirmed Stop/revocation before scoring, and confirmed
deletion of both task and verifier sandboxes. Those B runs used the explicit
assumption below. Huawei A recording also passed those checks under the same
assumption; a complete successful Huawei A replay remains unverified.
Effective no-egress networking remains unverified. By default, Huawei rejects
`allow_internet_access=false` before
allocation with `422 network_isolation_unsupported`, including verifier requests.
An explicitly authorized experiment can set `sandbox.assume_network_isolation`
to `true` in its profile and `HUAWEI_AGENTSPHERE_ASSUME_NETWORK_ISOLATION=true`
in the Adapter environment. Both default to `false`; the Adapter environment
accepts only literal `true` or `false`, and its authenticated health response must
match the profile before allocation. Other providers reject this assumption.
The cloud request still contains `allow_internet_access=false`; the assumption
does not change it to true or establish that the platform enforces it.

Adapter responses label this condition `verifier_egress_isolation=assumed-unverified`;
`cloud-profile.json` preserves the profile option and `run-result.json` records
`infrastructure.network_policy=assumed-huawei-verifier-egress-unverified`.
Agent termination, tool revocation, independent hidden tests and cleanup remain
required. The current counters do not automatically exclude assumption-mode
results from validated throughput or Goodput. Analyze them separately by
`infrastructure.network_policy`: their scores and rates are conditional on the
stated assumption and do not prove that egress isolation was tested successfully.
See [result interpretation](run-results.md#attempt-accounting-and-verified-throughput).
Basic compatibility probes alone do not prove a full OpenClaw workload. The
accepted B runs and preserved earlier failures are documented in
[Huawei deployment](../deploy/agentsphere/README.md) and the
[performance archive](performance-archive.md).
The API/runtime/image ENVD version discrepancy is a deferred issue and does not
itself block capability-based integration.

Image publication preparation also reserves `HUAWEI_SWR_REGISTRY` (registry
hostname only) and `HUAWEI_SWR_NAMESPACE` (an existing SWR organization).
Publication first uses existing ECS registry authentication and must verify
push permission for that organization. Alternatively, keep the console-generated
temporary login command in private `HUAWEI_SWR_LOGIN_COMMAND`; publication
preparation parses its login arguments and uses password stdin on ECS, without
executing the field through a shell or exposing the password in process arguments.
The command is a secret, not a registry hostname. An optional
`HUAWEI_SWR_EXPIRES_AT` records the temporary credential's expiry as an ISO 8601
timestamp with timezone. These are not consumed by the CLI, adapter, or Compose.
Keep these settings in the private workstation env file until an ECS publication
step needs them. Never forward registry credentials to Agent or Sandbox
processes. Registry authentication and the AgentSphere API key serve separate APIs. AgentSphere's
permission to pull a private image must be checked separately from ECS push
authentication.

| Environment guard | Default | Enforced by |
| --- | --- | --- |
| `ARIES_MAX_CONCURRENCY` | `1` | Cloud profile validation. |
| `ARIES_MAX_ATTEMPTS` | `2` per run | Cloud profile validation; not a requested attempt count. |
| `ARIES_SANDBOX_TIMEOUT_SECONDS` | `600` | Cloud TTL validation and the deployed SDK adapter's maximum. |
| `ARIES_MAX_SANDBOXES` | `4` live/uncertain | SDK adapter, supplied by Compose. |
| `ARIES_GOODPUT_DEADLINE_SECONDS` | Unset | Optional explicit SLO when the profile omits `execution.goodput_deadline`. |

A completed coding attempt normally uses a task sandbox and an independent
verification sandbox. All five conditions normally require ten creations, plus
the separate SDK smoke and template boot/verification sandboxes. The adapter
persists creation attempts, including failed/uncertain ones, for accounting;
there is no cumulative creation limit. Preserve the ownership ledger across
restarts and resolve uncertain allocations or cleanup before creating more.

Goodput requires an explicit deadline, a stable candidate capture timestamp,
positive isolation, and a final independent verifier PASS. Its deadline uses
scheduled arrival to stable artifact capture; verification may finish later.
The rate denominator includes admission and drain. Cleanup and model-session
outcomes remain separate evidence. Missing artifact evidence, failed/disabled
verification, and absent SLOs do not become successful Goodput samples. Failed,
canceled, timed-out, and unstarted planned attempts remain in the accounting.

Both placements require a dedicated `aries-agent` UID 60000, no preexisting
processes for that UID, root control, and writable root-owned cgroup v2 with
`cgroup.kill`. B also needs the pinned OpenClaw/Node installed outside Agent-writeable
paths; the coding task requires Python 3 and pytest. A generic E2B template is
not presumed to qualify. Missing isolation capabilities fail as unsupported;
transport cancellation or TTL expiry alone does not prove stop/revocation.
See [template and shutdown requirements](../pkg/harness/openclawcloud/README.md#template-and-shutdown-requirements).

## Resource and timeout overrides

The five-task profile additionally references
`configs/runtime-overrides.json` relative to the profile. Its sparse
`harness_resources` and `agent_sandbox_resources` blocks apply only to their
respective containers and may specify different CPU and memory limits. A fresh
evaluation sandbox, such as SWE-bench Pro's, uses the task's own resources.

- In `harness_resources`, an omitted or zero `cpu` or `memory_mb` is unlimited.
- In `agent_sandbox_resources`, an omitted `cpu` or `memory_mb` keeps the task's
  declared value, and a zero one is unlimited.

Neither block inherits from the other.

The independent `agent_timeout_seconds` field changes only the agent deadline.
The independent `verifier_timeout_floor_seconds` field raises a Terminal-Bench
task's evaluation budget to at least that many seconds and never lowers one.
It covers the entire verifier command, including dependency installation and
test execution, and leaves the agent deadline unchanged. Some task test scripts
exhaust a short declared budget while installing dependencies.

For example, an overrides file containing the following sets a 900-second
minimum verifier budget:

```json
{
  "verifier_timeout_floor_seconds": 900
}
```

Report the configured floor with benchmark results and use the same verifier
budget policy across compared runs.

Every checked-in original profile explicitly contains `overrides_file`; the one-task
profile uses `""`, which disables override loading without opening a file.
Profiles and nonempty referenced override files reject unknown fields and
trailing JSON; there is no profile merge or inheritance layer. SGLang is the
exception only for its separate native launch configuration, described below.

### Paper-reproduction profiles

`profiles/experiments/` holds the twenty-task OpenClaw and Hermes profiles for
Terminal-Bench 2, SWE-Bench Pro, and Deep Research Bench used to reproduce the
paper. They run at concurrency 1 and share
`profiles/experiments/no-limits-overrides.json`, which sets `cpu` and
`memory_mb` to `0` in both `harness_resources` and `agent_sandbox_resources`,
so neither container has a CPU or memory limit, and sets
`agent_timeout_seconds` to 3600, a one-hour agent deadline. Run them from the
repository root, as with the other profiles.

## Deployment configuration

Original profiles select harness identity and placement separately. The
`embedded` rejection below concerns legacy `bridge.mode`, not cloud's separate
top-level `placement: "embedded"`:

```json
{
  "harness": {
    "type": "hermes",
    "deployment": {
      "backend": "docker",
      "docker": { "socket": "/var/run/docker.sock" }
    }
  },
  "sandbox": {
    "deployment": {
      "backend": "docker",
      "docker": { "socket": "/var/run/docker.sock" }
    }
  },
  "bridge": {
    "type": "hermes-ssh",
    "mode": "managed",
    "deployment": { "backend": "docker" }
  }
}
```

This is a profile fragment; keep the benchmark, model, runtime, and other
settings from a complete checked-in profile. OpenClaw uses `harness.type:
"openclaw"` with `bridge.type: "openclaw-ssh"`. Pairing is checked independently
of placement.

The separate blocks express independent component placement, so the schema does
not require a future harness and sandbox to share a deployment implementation.
They do not establish support for mixed backends today. A heterogeneous setup
will also need compatible connectivity, identity, isolation, and cleanup semantics;
see the [deployment contract](design/deployment.md).

For Docker placement, both components must use the same local daemon. `socket`
accepts an absolute Unix socket path or `unix:///absolute/path`; normalization
cleans the path before comparing the two settings. Remote Docker endpoints are
rejected. Image preparation and resource monitoring use the selected socket.
The supported topology is native Linux Docker with one network and bridge service
per run. Every live sandbox has its own bridge session/listener and unique service
name, including repeated task IDs. Fresh evaluation sandboxes borrow the same
network. Effective network policy is `shared-egress`, recorded in run infrastructure;
benchmark `AllowNetwork` declarations remain metadata. Docker has no
network-per-task option. See the [bridge contract](design/bridge.md).

Compatibility normalization supplies Docker with `/var/run/docker.sock` for omitted
harness/sandbox deployment blocks. An omitted bridge mode becomes `managed`, and
an omitted bridge deployment becomes `docker`. Explicit `embedded` mode now
fails with a migration message; update it to `managed` and choose placement
explicitly. `external` remains unsupported. Legacy `sandbox.type: "docker"`
still maps to Docker deployment; conflicting explicit settings are rejected.

With Docker placement, `bridge.deployment.backend` is `docker`. Bridges run in a separate container
on the run network and use the same local daemon as the sandbox. Composition
supplies the required Docker socket mount explicitly; component labels do not
grant access. The harness and sandbox never receive that socket. The bridge has daemon authority
like Runner, even though each session's execution adapter validates one fixed target.

The bridge runtime runs `aries-bridge` from the same build as `aries-ssh-client`.
`make build` produces both alongside Runner. `make bridge-image` builds the Docker
image containing only `aries-bridge`, using `bridge.image` and `bridge.base_image`
from the version catalog. Runner stages the separate `aries-ssh-client` binary
directly into the OpenClaw harness; the bridge container does not need that client.
For the Kubernetes backend, `make k8s-images` builds the bridge on
`bridge.kubernetes_base_image` instead, which supplies `kubectl`; bump it with
the Kubernetes version the pinned Invitro installs.
Image preparation uses the selected daemon. Checked-in Docker bridge examples
include [Hermes](../profiles/hermes-tb2-fix-git-deepseek.json) and
[OpenClaw](../profiles/openclaw-tb2-fix-git-deepseek.json).

### Kubernetes deployment

`backend: "kubernetes"` places each runtime as a pod. ARIES reaches the cluster
through `kubectl` and the context it resolves. Harness, sandbox and bridge must
all select this backend and the same namespace; mixed compositions are rejected.

```json
{
  "harness": {
    "type": "hermes",
    "deployment": {
      "backend": "kubernetes",
      "kubernetes": { "namespace": "aries", "images": { "aries-local/hermes": "ghcr.io/hyscale-lab/aries-hermes" } }
    }
  },
  "sandbox": {
    "deployment": {
      "backend": "kubernetes",
      "kubernetes": { "namespace": "aries", "pod_cidr": "192.168.0.0/16", "service_cidr": "10.96.0.0/12" }
    }
  },
  "bridge": {
    "type": "hermes-ssh",
    "deployment": {
      "backend": "kubernetes",
      "kubernetes": { "namespace": "aries", "service_account": "aries-bridge", "images": { "aries-bridge": "ghcr.io/hyscale-lab/aries-bridge" } }
    }
  }
}
```

| Field | Applies to | Meaning |
| --- | --- | --- |
| `namespace` | all | Namespace for every pod and policy. Defaults to `aries`; must match across the three blocks. |
| `node_selector` | all | Labels a node must carry to host the pods, for example `{"loader-nodetype": "worker"}`. Unset leaves pods unpinned. |
| `pod_cidr`, `service_cidr` | sandbox | Cluster networks a network-allowed sandbox is kept off. Unset leaves that egress open. |
| `prometheus_url` | sandbox | Prometheus server that pod resource measurements are read from, reachable from where ARIES runs. Unset records measurements as unsupported. |
| `service_account` | bridge | Account the bridge pod uses to exec into sandbox pods. Defaults to `aries-bridge`. |
| `image_pull_secret` | all | Registry credential Secret in the namespace. |
| `images` | all | Maps an image ARIES builds locally to the reference the cluster pulls. A key without a tag maps the repository and keeps the requested tag. |

The cluster pulls images, so `aries setup` builds nothing for this backend: the
bridge image and the derived Hermes image must already be published;
`make k8s-images` builds and pushes both (see
[publishing the images](../k8s/README.md#3-publish-the-images-the-cluster-pulls)). Each
sandbox gets its own NetworkPolicy and the effective run policy is
`per-sandbox-policy`. Task images that need per-command UID/GID switching, such
as SWE-bench Pro, must start as root and provide `setpriv` from `util-linux`;
the upstream SWE-bench Pro images already do, so it needs no extra step.
Checked-in examples are
[Hermes](../profiles/kubernetes/hermes-tb2-fix-git-deepseek-k8s-pods.json) and
[OpenClaw](../profiles/kubernetes/openclaw-tb2-fix-git-deepseek-k8s-pods.json). See the
[Kubernetes implementation](implementation/kubernetes.md) for its constraints
and [`k8s/`](../k8s/README.md) for cluster setup.

Bridge addresses come from deployment resolution, not profile fields. Plaintext,
unauthenticated gRPC control and harness-facing SSH use separate endpoints.
Control has no TLS certificates or bearer tokens. SSH accepts unauthenticated
clients, retaining one in-memory service host key for the handshake. No SSH client
keys or known-hosts files are staged. The shared service creates a dynamic listener
per sandbox and deployment resolves its harness-reachable endpoint. Runner
explicitly releases each sandbox session; run cleanup removes the service.
There are no leases or renewal timers. Control health alone does not establish
session readiness.

## Preparation and task selection

Running a profile automatically loads `configs/versions.json`, creates or
verifies the pinned Terminal-Bench checkout at `.cache/terminal-bench-2-1`, reads
each selected task's explicit Docker image tag from its `task.toml`, and pulls
only the configured harness image plus those selected images through the
Docker Go SDK for Docker placement. For Hermes it then builds, once, the local image that adds the
`hermes-otel` plugin pinned by `hermes.otel_plugin`. Preparation
happens before the run directory is created, a managed runtime is started,
model weights load, an external endpoint is contacted, or task work is
admitted. The Terminal-Bench Git revision and explicitly tagged OpenClaw image
(with an optional full SHA256 digest) remain in the selected `versions_file`,
alongside the tag-pinned Hermes image and
the `hermes-otel` plugin's repository, release, and 40-character revision; task
image digests are not duplicated there.
Preparation is safe to repeat and refuses to replace a checkout at another
revision.

For an optional prewarm, `setup` performs only profile/backend validation and
the same benchmark/image preparation. It does not contact an external model
service, start managed SGLang, load model weights, create a run directory, or
admit tasks:

```sh
make setup
make setup PROFILE=profiles/openclaw-tb2-five-deepseek.json
make setup PROFILE=profiles/openclaw-tb2-fix-git-sglang.json
```

The normal Make workflow is direct run:

```sh
make run
make run PROFILE=profiles/openclaw-tb2-five-deepseek.json
```

To run another subset from the pinned revision, copy either profile and replace
`benchmark.tasks` with the desired task directory names. ARIES preserves the
listed order and repeated entries. Set positive `execution.concurrency` to
bound parallel occurrences. Add a positive Go duration such as `"30m"` as
`execution.loop_duration` to repeat the list until admissions close; admitted
work is always drained. ARIES loads each task's explicit tagged image directly from that
task's `task.toml`; no version-catalog or Go code change is required.

Set `execution.goodput_deadline` to an explicit positive Go duration (for example
`"30s"`) to calculate SLO Goodput. The deadline runs from scheduled arrival to
proven stable candidate capture, with independent verification required to count
success. It is not inferred from the agent timeout. Omit the field to omit
Goodput. See [attempt accounting](run-results.md#attempt-accounting-and-verified-throughput)
for the distinct throughput counts and evidence requirements.

## Replay task arrivals

An arrival trace schedules task occurrences at offsets from the start of task
scheduling, after benchmark/image preparation and model preflight. Scheduling
happens outside the harness, so it works with any benchmark, harness, and mode
listed in [supported implementations](supported.md). Keep the rest of an
existing profile unchanged.

For example, save this trace as `.cache/arrivals.json` (create `.cache` first):

```json
{
  "trace_id": "fix-git-replay",
  "base_rate_per_min": 1.0,
  "arrivals": [
    {"t": 0, "traj": "fix-git"},
    {"t": 60, "traj": "fix-git"},
    {"t": 180, "traj": "fix-git"}
  ]
}
```

`trace_id` is optional descriptive metadata. `base_rate_per_min` is the positive
reference rate. Each `t` is an offset in seconds, not a delay from the previous
entry; `traj` is the exact logical task ID from `benchmark.tasks`. Selected
offsets must be zero or positive and fit Go's `time.Duration` after scaling. The
trace must contain at least one arrival. Extra metadata fields are accepted in
the trace; the experiment profile still rejects unknown fields.

Copy the one-task profile, replace its `execution` block, and set
`benchmark.tasks` as follows (these are profile fragments):

```sh
cp profiles/openclaw-tb2-fix-git-deepseek.json .cache/arrival-demo.json
```

```json
{
  "execution": {
    "concurrency": 3,
    "arrivals_file": ".cache/arrivals.json",
    "arrival_rate_per_min": 2.0
  },
  "benchmark": {
    "tasks": ["fix-git", "fix-git", "fix-git"]
  }
}
```

Keep the other fields from the copied profile. From the repository root, after
configuring the model credential in [model backends](#model-backends), run:

```sh
./bin/aries .cache/arrival-demo.json
```

Relative `arrivals_file` paths resolve from the command's working directory,
not the profile directory. The copied profile's `../configs/versions.json`
continues to resolve relative to `.cache/arrival-demo.json`. Both
`arrivals_file` and a finite, positive `arrival_rate_per_min` must be set
together; omit `loop_duration` because a trace cannot be combined with looping.

The scheduler uses `offset = t * base_rate_per_min / arrival_rate_per_min`.
The example therefore schedules three independent `fix-git` occurrences at
0, 30, and 90 seconds. It replays the supplied offsets; it does not generate a
Poisson stream or guarantee a measured throughput of two tasks per minute.
For each task, its k-th occurrence in `benchmark.tasks` selects its k-th entry
in trace file order. Missing occurrences fail the run; unused trace entries
are ignored. Selected occurrences are then sorted by scaled offset, preserving
profile order for ties. This schedule order determines occurrence IDs and
result order, regardless of completion order.

`execution.concurrency` still caps active occurrences, including evaluation
and cleanup. A full pool delays admission and can make later arrivals overdue;
they run as capacity becomes available instead of being dropped. Choose enough
capacity for the intended overlap (three guarantees a free slot for each
arrival in this example). Cancellation stops further admissions and drains
admitted work through lifecycle cleanup.

Each task's `started_at` in `run-result.json` records when its Runner task
lifecycle began, after occurrence construction, observer startup, and task
loading, but before sandbox startup. It is neither the planned arrival time
nor the first model request time. Starts can be delayed by capacity or setup;
scheduling is not a hard realtime guarantee. Trace contents are loaded during
`run` after model preflight, so `setup` alone does not validate them.

## Model backends

`runtime.mode` states whether ARIES owns a model-server process: an `external`
endpoint is validated but never started, configured, or stopped, and a
`managed` process is owned for the run. `runtime.backend` names the kind of
service behind the endpoint, which selects the preflight and the provider each
harness renders; it is not a runtime ARIES prepares. The supported combinations
are:

| Backend | Mode | `runtime.config` | Process owner |
| --- | --- | --- | --- |
| `deepseek` | `external` | Must be omitted | DeepSeek |
| `sglang` | `external` | Optional `file` accepted but ignored | User |
| `sglang` | `managed` | `file`, `executable`, `startup_timeout`, `stop_timeout` | ARIES |
| `openai` | `external` | Must be omitted | User |

DeepSeek and `openai` are external only. SGLang supports both modes.

HTTP model endpoints are a trusted-local exception. The checked-in HTTP
examples use the non-secret `unused-local-token` placeholder and are suitable
only when the endpoint and network are under your control. Never send a real
API key over HTTP; use an HTTPS endpoint for a remote or credentialed service.
This is an operator requirement: current URL validation does not enforce
locality. See the [transport-policy gap](implementation/model-runtime.md#implementation-gap-http-transport-policy).

### Reasoning effort

Set `model.reasoning_effort` to control the harness model in either OpenClaw or
Hermes. For example, add `"reasoning_effort": "low"` to the existing `model` block.
Use `"off"` (or the OpenAI spelling `"none"`) to disable reasoning.

| Backend | Explicit values | Request mapping |
| --- | --- | --- |
| `openai`, `sglang` | `off`, `none`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max` | Chat Completions `reasoning_effort`; `off` becomes `none` |
| `deepseek` | `off`, `none`, `low`, `high`, `max` | `thinking.type` disables/enables thinking; enabled requests include the exact effort |

Model support varies: accepting a profile does not prove that an endpoint supports
that effort. ARIES does not silently translate one enabled effort into another.
DeepSeek's noncanonical effort aliases are rejected. Explicit settings conflict
with `thinking`, `reasoning`, or `reasoning_effort` in `harness.hermes.extra_body`.

Omitting the field preserves existing harness behavior. OpenClaw asks its Gateway
for thinking off for the official DeepSeek Flash/V4 models; Hermes leaves reasoning
to its native/provider defaults. Set the field explicitly for comparable runs.
The generated private `openclaw.json` or Hermes `config.yaml` records the request
configuration; it does not prove the provider honored it.

This field controls the main agent model, including voice-transcribed tasks and
OpenClaw realtime agent consultations. `harness.realtime.reasoning_effort` remains
the separate Talk setting. Configure judge reasoning independently under
`benchmark.judge.reasoning_effort`; FACT settings remain unchanged.

Both harnesses currently use Chat Completions. GPT-6 Luna supports tool calls on
that API only with `none`; reasoning with tools requires Responses API support.
See the [OpenAI model documentation](https://developers.openai.com/api/docs/models/gpt-6-luna)
and [DeepSeek thinking guide](https://api-docs.deepseek.com/guides/thinking_mode/).

### External DeepSeek

The checked-in DeepSeek profile uses:

```json
{
  "runtime": {
    "backend": "deepseek",
    "mode": "external"
  },
  "model": {
    "base_url": "https://api.deepseek.com",
    "api_key_env": "DEEPSEEK_API_KEY",
    "id": "deepseek-flash",
    "reasoning_effort": "low"
  }
}
```

Do not add a `runtime.config` object for DeepSeek. ARIES performs model
preflight and configures OpenClaw, but it does not manage the remote service.

The preferred source for `./bin/aries` is the ignored repository-root file
`DEEPSEEK_API.key`:

```sh
umask 077
read -r -s -p "DeepSeek API key: " aries_key; printf '\n'
printf '%s\n' "$aries_key" > DEEPSEEK_API.key
unset aries_key
chmod 600 DEEPSEEK_API.key
```

The file must be a current-user-owned, regular, non-symlink file with owner
read access, no group or world permissions, and one nonempty line. Modes `0400`
and `0600` are both valid. ARIES never writes the value to JSON, logs, Docker
metadata, or results.

If that repository-local file is unavailable, including when the binary is
installed elsewhere, ARIES reads `DEEPSEEK_API_KEY` from the environment. If a
repository-local file exists but is invalid, ARIES fails closed rather than
falling back.

This file convenience is not limited to a DeepSeek-backed harness: it also
applies whenever a benchmark's `judge` block is genuinely official DeepSeek
(`provider: "deepseek"`, `base_url: "https://api.deepseek.com"`, and a
supported model), even if `runtime.backend` is `sglang` — e.g. SWE-Atlas QA's
`profiles/openclaw-sweatlasqa-smoke1-sglang.json`, which runs its agent
against a local SGLang server but grades with a DeepSeek judge. Any other
credential (SGLang's own `api_key_env`, for instance) is always read from the
environment regardless.

Use the model ID in the checked-in profile or one accepted by the selected
backend and returned by its catalog. Preflight requires an exact match; a model
alias is not inferred from a similar name. The [DeepSeek preflight](../internal/app/preflight.go)
defines ARIES's accepted IDs. Provider availability is checked at run time.

### External SGLang

External SGLang is started and configured by the user. The checked-in profile
references `configs/sglang/qwen3-8b-local.yaml`, which can be passed to SGLang's
native `--config` option. ARIES does not read that file in external mode; it
checks the configured endpoint and served model. Native YAML and GPU topology
validation apply only to [managed SGLang](#managed-sglang).

Copy the profile before changing its endpoint:

```sh
mkdir -p .cache
cp profiles/openclaw-tb2-fix-git-sglang.json \
  .cache/openclaw-tb2-fix-git-sglang.json
```

Replace `model.base_url` in the copy with an endpoint ending exactly in `/v1`.
The checked-in `sglang.local` hostname is a placeholder: the configured
hostname or address must resolve and be reachable from both the ARIES host and
OpenClaw containers. Use HTTP only for a trusted local endpoint with the
non-secret placeholder below; use HTTPS for a remote or credentialed service.

The checked-in profile uses the following external runtime and model settings:

```json
{
  "runtime": {
    "backend": "sglang",
    "mode": "external",
    "config": {
      "file": "../configs/sglang/qwen3-8b-local.yaml"
    }
  },
  "model": {
    "base_url": "http://sglang.local:30000/v1",
    "api_key_env": "SGLANG_API_KEY",
    "id": "Qwen/Qwen3-8B"
  }
}
```

External SGLang mode needs no `runtime.config`. An optional `config.file` is
accepted for existing profiles but is not read or validated; `executable`,
`startup_timeout`, and `stop_timeout` are rejected because ARIES does not own
that process. Start SGLang separately; for example, this exposes GPU0 to the
server:

```sh
CUDA_VISIBLE_DEVICES=0 /absolute/path/to/venv/bin/python \
  -m sglang.launch_server \
  --config configs/sglang/qwen3-8b-local.yaml
```

In the shell that runs ARIES, set the environment variable named by
`model.api_key_env`. An unauthenticated endpoint still needs a nonempty
placeholder because OpenClaw and model preflight require the configured
credential:

```sh
export SGLANG_API_KEY=unused-local-token
./bin/aries .cache/openclaw-tb2-fix-git-sglang.json
```

### External OpenAI-compatible server

`runtime.backend: "openai"` accepts any server that speaks the OpenAI chat
completions API and lists its models at `/v1/models`: vLLM, `llama.cpp`, a
gateway, or a hosted endpoint. ARIES never starts, configures, or stops the
server. Before the run it makes one bounded `/v1/models` request and confirms
that `model.id` is served. `runtime.config` must be omitted.

The checked-in profile targets a vLLM server:

```json
{
  "runtime": {
    "backend": "openai",
    "mode": "external"
  },
  "model": {
    "base_url": "http://vllm.local:8000/v1",
    "api_key_env": "VLLM_API_KEY",
    "id": "Qwen/Qwen3.6-35B-A3B-FP8"
  }
}
```

Create a local copy:

```sh
mkdir -p .cache
cp profiles/hermes-tb2-fix-git-vllm.json .cache/hermes-tb2-fix-git-vllm.json
```

In the copy, set `model.base_url` to an endpoint that ends exactly in
`/v1` and `model.id` to the name the server reports. The checked-in
`http://vllm.local:8000/v1` value is a trusted-local placeholder; the address
must resolve from the ARIES host and from the harness containers. Use HTTPS for
a remote or credentialed server. Start the server yourself, for example:

```sh
vllm serve Qwen/Qwen3.6-35B-A3B-FP8 --port 8000 \
  --served-model-name Qwen/Qwen3.6-35B-A3B-FP8
```

For this trusted-local HTTP example, use the non-secret placeholder credential:

```sh
export VLLM_API_KEY=unused-local-token
./bin/aries .cache/hermes-tb2-fix-git-vllm.json
```

Hermes has no `sglang` or plain `openai` provider, so for both backends ARIES
renders Hermes's generic `custom` provider, which routes to `model.base_url`.
OpenClaw receives the server as a `models.providers` entry named `aries`.

### Scripted mock model

`bin/aries-mock-model` serves fixed tool calls and a final response through
`GET /v1/models` and `POST /v1/chat/completions` (JSON or SSE). Use the existing
`openai` backend in `external` mode; omit `runtime.config`. This exercises the
real harness, bridge, sandbox, and benchmark verifier without model inference.
It is a development workload, not a model-capability benchmark.

The [Hermes example profile](../profiles/hermes-tb2-fix-git-mock.json) uses the
[sandbox probe fixture](../configs/mock/hermes-sandbox-probe.json), disables
compaction and subagents, and writes to `runs-mock/`. Its two steps inspect the
environment and exercise file I/O. They do not solve `fix-git` or guarantee reward `1`;
the task's real verifier still runs. Benchmark data, images, and Docker remain
required as in the [quick start](quick-start.md#prerequisites).

Build and inspect the fixture's served model ID:

```sh
make build
./bin/aries-mock-model --fixture configs/mock/hermes-sandbox-probe.json --print-model
```

The default listener is `127.0.0.1:8000`, suitable for host-only requests. For
harness containers, explicitly bind an address they can reach. On the trusted
local host used for the Docker run, start the server in a separate terminal:

```sh
./bin/aries-mock-model \
  --fixture configs/mock/hermes-sandbox-probe.json \
  --listen 0.0.0.0:8000
```

The server does not authenticate requests. Keep it on a trusted private network;
use an HTTPS frontend for access across machines. The placeholder credential
below satisfies ARIES's non-empty credential requirement and provides no access
control. ARIES neither starts nor stops this external process; stop it with
Ctrl-C after the run.

Copy the example profile:

```sh
mkdir -p .cache
cp profiles/hermes-tb2-fix-git-mock.json .cache/hermes-tb2-fix-git-mock.json
```

In the copy, set `model.base_url` to `http://PRIVATE_HOST_IP:8000/v1`, using an
address reachable from both the ARIES host and the harness containers.
`127.0.0.1` inside a container identifies that container, not the mock server.
Set `model.id` to the exact `--print-model` output, then run:

```sh
export ARIES_MOCK_API_KEY=aries-mock
./bin/aries .cache/hermes-tb2-fix-git-mock.json
```

Each fixture defines one workload: `name`, the offered `tool`, a case-sensitive
`prompt_contains` substring, `response_delay_ms`, ordered `steps`, and
`final_response`. Each step supplies the exact tool `arguments` (for the Hermes
terminal tool, an object containing `command`) and an `output_contains` check.
Use 1–48 lowercase letters, digits, or hyphens for `name`, starting with a letter
or digit. `response_delay_ms` is 0–60000 and delays each valid response before
JSON or SSE output; it does not simulate token-by-token generation.
Requests must contain one user task after any initial system/developer messages,
then complete alternating assistant/tool pairs. The endpoint emits one tool call
per step and validates tool names, arguments, call IDs, and expected output. It
derives progress from that history, with no shared session counter. Follow-up
user messages, background-command polling, and auxiliary requests without the
fixture tool are unsupported. Use short synchronous commands and one matching
workload per endpoint. Keep compaction and subagents disabled: fully discarded
history cannot be distinguished from a new request, so the server cannot detect
every history reset.

The model ID is `aries-mock-<name>-<hash>`, where `hash` is the first 12 hex
characters of the SHA-256 of the original fixture bytes. Startup logs include
the full hash. Editing commands, response delay, or even JSON whitespace changes
the ID; rerun `--print-model` and update the profile. Keep the exact fixture and
full hash with the run evidence. The server supplies no synthetic token usage.

For infrastructure comparisons, replace the probe with a fixed representative
workload and hold the fixture, harness version, region, resources, and concurrency
constant. Removing inference does not remove other experimental differences;
see [interpreting mock runs](run-results.md#scripted-mock-runs).
Prefer verifier-based tasks such as Terminal-Bench 2. An LLM judge must use an
explicit independent model, not inherit the mock endpoint; in particular,
Deep Research Bench otherwise defaults its judge identity to the main model.
This model endpoint does not add a cloud or E2B sandbox provider.

### Managed SGLang

To let ARIES own one SGLang process for the entire profile run, use the
following runtime and model settings in the copied profile:

```json
{
  "runtime": {
    "backend": "sglang",
    "mode": "managed",
    "config": {
      "file": "../configs/sglang/qwen3-8b-local.yaml",
      "executable": "/absolute/path/to/venv/bin/python",
      "startup_timeout": "15m",
      "stop_timeout": "1m",
      "gpu_indices": [0]
    }
  },
  "model": {
    "base_url": "http://sglang.local:30000/v1",
    "api_key_env": "SGLANG_API_KEY",
    "id": "Qwen/Qwen3-8B"
  }
}
```

`file`, `executable`, `startup_timeout`, and `stop_timeout` are required. `file` is resolved
relative to the profile, `executable` must identify the Python executable from
the SGLang environment, and both timeout values must be positive Go durations.
`model.base_url` must use the YAML port and end exactly in `/v1`;
`model.id` must equal the YAML `served-model-name`; and `model.api_key_env`
names the environment variable read by ARIES and rendered into OpenClaw.

ARIES passes the native YAML file to SGLang unchanged. It checks the endpoint
settings and GPU topology it consumes, while SGLang validates its other options.
Use full names for these checked fields: for example, `tensor-parallel-size`,
`pipeline-parallel-size`, `data-parallel-size`, and `expert-parallel-size`.
Aliases such as `tp-size` or `tp`, and abbreviations of checked fields, are
rejected before startup so native execution and GPU measurement cannot disagree
because they read different spellings of the same setting.
ARIES also rejects option keys containing `=`. Scalar values and list items
starting with `-` must be numeric, so they cannot introduce command-line options
that override the checked settings.
Negative numeric values such as `chunked-prefill-size: -1` pass through for
SGLang to validate; endpoint and GPU topology limits remain enforced by ARIES.
For example, `enable-metrics`, quantization, context length, memory settings, and
parser options use SGLang's own configuration and defaults.

`runtime.config.gpu_indices` is optional and valid only for managed SGLang.
For YAML `device: cuda`, ARIES derives the number of local workers from the
tensor, pipeline, and multi-node topology and selects physical devices
`[0, ..., N-1]` when the field is omitted.
Ordinary data parallelism replicates workers; DP attention, expert, MoE data,
and attention-context parallelism partition the TP workers instead. An
explicit list must contain exactly `N` unique, non-negative indices. ARIES uses
the resolved list for both the child's `CUDA_VISIBLE_DEVICES` and NVIDIA
sampling. An unsupported or inconsistent topology fails before runtime or
monitor side effects.

Do not start `sglang.launch_server` separately in this mode. ARIES passes the
referenced YAML using the exact arguments
`-m sglang.launch_server --config <file>`, waits up to `startup_timeout` for
`/health`, and then requires exact model discovery at `/v1/models`. It retains
the child output in mode-0600 `sglang/stdout.log` and `sglang/stderr.log`, stops
the process group after all admitted tasks drain, and uses `stop_timeout` as
the graceful TERM budget before forced cleanup.

The configured credential variable is available to ARIES and OpenClaw but is
removed from the managed SGLang child's environment:

```sh
export SGLANG_API_KEY=unused-local-token
./bin/aries .cache/openclaw-tb2-fix-git-sglang.json
```

ARIES does not install SGLang or models or configure the network path shared by
the host and containers.

## Harness settings

The Hermes profile is the same run with a different harness, so it needs the
[DeepSeek credential](#external-deepseek) and no extra setup:

```sh
./bin/aries profiles/hermes-tb2-fix-git-deepseek.json
```

Hermes supports text and [voice-transcribe](voice_mode.md). Preparation builds
a local image from the pinned upstream one with only the `hermes-otel` plugin
added, once per machine (the first build needs GitHub and PyPI access and takes
about 20 seconds), and Hermes runs it. It is paired
with `bridge.type: "hermes-ssh"`; the two values must match, and a crossed pair
is rejected before the run starts. Hermes issues every tool call as `bash -c`,
so the task image must provide `/bin/bash`. Its `~/.hermes` file sync is refused by
the bridge to keep the evaluated sandbox free of harness scaffold and
credentials; Hermes logs one `file_sync: sync failed` warning and continues.

Hermes reserves environment names beginning with `API_SERVER_` for its Gateway.
Do not use them as `model.api_key_env` or MCP `secret_env` source names. MCP
child environment names may still use that prefix with a different source.

Artifacts land under `<run>/<task>/harness/`: the redacted `config.yaml`, the
native Gateway's `gateway-output.txt`, `session-outcome.json`, `container.log`, and the
exported message-level trajectory at `telemetry/sessions.jsonl`, and
`telemetry/otel-spans.jsonl`: tool, model, and API spans with call IDs and wall-clock
start/end times. An image without the plugin leaves no spans file.

### Hermes context window, compaction, and request extra body

Three optional profile blocks reach the rendered Hermes `config.yaml`. Each is
Hermes-only and is rejected under another harness. A profile without them
renders the same file as before.

- `model.context_length`, `model.max_tokens`, and `model.temperature` set the
  window Hermes's compressor reasons about and the request sampling.
  Temperature (including `0.0`) requires the `sglang` or `openai` backend;
  ARIES places it in the custom provider's request `extra_body`, because the
  request body carries the configured temperature explicitly.
  Setting both `model.temperature` and `harness.hermes.extra_body.temperature`
  is rejected. Native DeepSeek temperature is unsupported by this path.
- `harness.compaction.threshold_tokens` is an absolute compaction trigger.
  Hermes applies it after its 64K minimum and its 75% floor for windows under
  512K, so it is the one knob that gives an exact trigger on a large window.
  `harness.compaction.enabled: false` turns compaction off.
- `harness.hermes.extra_body` is a non-empty JSON object. ARIES writes it as
  the `extra_body` of one `custom_providers` entry, and Hermes merges it into
  every chat request. Hermes performs that merge only for its `custom`
  provider, so the block requires the `sglang` or `openai` backend. It sits
  under `harness.hermes` because it is a Hermes escape hatch with no meaning
  for another harness, whereas compaction is a general harness setting.

Hermes expands `${NAME}` references in its configuration from the container
environment. ARIES exports `ARIES_RUN_ID` and `ARIES_TASK_ID` into the Hermes
container, and those two are the only references `harness.hermes.extra_body`
may carry. The object is also rejected when any field at any depth is named
like a credential, such as `api_key`, `authorization`, or `token`: it is
written into the retained `config.yaml` and sent with every request, and model
keys stay out of JSON profiles. Its checked-in HTTP endpoint is intended only
for trusted local use with the non-secret placeholder shown below; use HTTPS
for a remote or credentialed server. The checked-in profile compacts at 65,536
tokens and tags every request with the task through the OpenAI `user` field:

```json
{
  "harness": {
    "type": "hermes",
    "compaction": {
      "enabled": true,
      "threshold_tokens": 65536
    },
    "hermes": {
      "extra_body": {
        "chat_template_kwargs": {
          "preserve_thinking": true
        },
        "user": "${ARIES_RUN_ID}-${ARIES_TASK_ID}"
      }
    }
  },
  "model": {
    "base_url": "http://vllm.local:8000/v1",
    "api_key_env": "VLLM_API_KEY",
    "id": "Qwen/Qwen3.6-35B-A3B-FP8",
    "context_length": 262144,
    "max_tokens": 32768,
    "temperature": 1.0
  }
}
```

```sh
export VLLM_API_KEY=unused-local-token
./bin/aries profiles/hermes-tb2-fix-git-vllm-compaction.json
```

The supported Hermes image is pinned to `v2026.8.31`. Its native Gateway and
absolute compaction threshold are validated together; older image pins are not
covered by this execution path. The rendered `config.yaml` under
`<run>/<task>/harness/` shows the selected settings.
The Gateway reads `agent.max_turns`; `agent_timeout_seconds` in the overrides
file also bounds the wall-clock wait for the run.

### Realtime OpenClaw mode

OpenClaw uses text-agent mode when `harness.mode` is omitted. Set
`harness.mode: "realtime"` to deliver the same task instruction through a
realtime voice session. The checked-in realtime profile does this already.
Keep the configured DeepSeek key file from the text setup and provide the
separate TTS credential named by the realtime profile through a secret manager
or interactive shell, then verify it is nonempty:

```sh
export OPENAI_API_KEY
test -n "${OPENAI_API_KEY:-}"
./bin/aries profiles/openclaw-tb2-fix-git-realtime-deepseek.json
```

This run can incur charges from both the configured model provider and the TTS
provider. The realtime profile keeps model and TTS credentials separate;
neither belongs in profile JSON.

## Benchmark settings

`benchmark.type`, `root`, and `tasks` select the benchmark, checkout, and task
occurrences. Pins come from `versions_file`. Keep evaluator-specific settings in
these guides:

- Terminal-Bench 2 uses pinned task metadata for images and verifier budgets;
  see [overrides](#resource-and-timeout-overrides).
- [Deep Research Bench](benchmarks/deep-research-bench.md) requires an environment
  image with its research services. An omitted `judge` reuses the harness model identity;
  optional `fact` configures citation checking.
- [SWE-Atlas QA](benchmarks/swe-atlas-qa.md) requires a `judge` block, which can
  explicitly disable grading. It rejects `environment` and `fact`.
- [SWE-bench Pro](benchmarks/swe-bench-pro.md) derives environments from dataset
  rows and uses its pinned evaluator in a fresh sandbox from the task image, so
  `agent_sandbox_resources` overrides do not apply to evaluation. It rejects
  `environment`, `judge`, and `fact`.
- [Toolathlon](benchmarks/toolathlon.md) requires `environment.image` (Toolathlon's
  task image) and a harness with an MCP client, Hermes or OpenClaw. The adapter
  adds its gateway to the harness's MCP servers, so `harness.mcp_servers` may
  not name `toolathlon`.
  Tasks backed by Toolathlon's self-hosted applications or by a third-party
  account share state outside the sandbox, so they load only at
  `execution.concurrency` 1. The optional `toolathlon` block sets the gateway
  port, the application host, `max_steps`, which Toolathlon records in its task
  bundle but which does not bound the agent, and `credentials_env` and
  `credential_files_env`, which map the token fields and key files of the
  account-backed tasks to environment variables; the profile names variables,
  never values.

### Judge model settings

`benchmark.judge` embeds the same model settings as the main `model` block.
It additionally declares its own `provider` and optional `enabled` switch:

```json
"judge": {
  "provider": "deepseek",
  "base_url": "https://api.deepseek.com",
  "api_key_env": "DEEPSEEK_API_KEY",
  "id": "deepseek-flash",
  "reasoning_effort": "low"
}
```

**Migration:** rename `judge.model` to `judge.id` in existing profiles. The old
field is rejected. Checked-in profiles use explicit low judge reasoning effort;
other omitted generation settings retain benchmark defaults.

- `reasoning_effort` uses the same [values and provider mapping](#reasoning-effort)
  as the harness model and reaches the judge's Chat Completions request.
- `temperature` and `max_tokens` override benchmark request defaults. Endpoint
  support remains model-dependent. `context_length` is rejected because judges
  do not manage a conversation context window.
- With omitted settings, Deep Research Bench sends temperature zero, while
  SWE-Atlas QA retains its output-token limit of 2048. Explicit enabled reasoning
  suppresses Deep Research Bench's implicit temperature; an explicit temperature
  is forwarded as requested.
- An omitted Deep Research Bench judge block reuses only the main model's
  identity and credential reference, preserving benchmark generation defaults.
  Configure judge generation settings explicitly; SWE-Atlas QA still requires
  a judge block. `enabled: false` requires all other judge fields to be absent.

FACT retains its separate schema, including its `model` field and Jina settings.
Changing the main agent's generation settings does not change FACT requests.

## Other harness and evidence options

- `harness.web_search` and `harness.subagents`: see the
  [research benchmark guide](benchmarks/deep-research-bench.md#web-search-and-fetch).
- `harness.mcp_servers`: see [MCP configuration and boundaries](implementation/harnesses.md#model-context-protocol-mcp).
- `harness.voice_transcribe`: see the [voice guide](voice_mode.md).
- `bridge.retain_raw_log`: defaults to false. Enabling it retains sensitive,
  replayable SSH input; see [run artifacts](run-results.md#bridge-evidence).

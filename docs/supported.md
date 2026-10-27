# Supported implementations

For the original `aries PROFILE.json` command, **Docker harnesses and sandboxes use the same local daemon**, with a separate
managed Docker bridge and shared network per run. Runner stays on the host. Remote Docker
servers and mixed deployment backends are unsupported. **Kubernetes** places the
same three components as pods in one namespace, for a narrower set of runs. See the
[deployment configuration](configuration.md#deployment-configuration).

This page summarizes capabilities and limitations. Use the
[quick start](quick-start.md) for a first run, the
[configuration reference](configuration.md) for profile fields, and
[run results](run-results.md) for artifacts and troubleshooting.
The [performance archive](performance-archive.md) records the latest accepted
experiments, stage timings, and the separate historical evidence.

All builds, tests, deployments, and experiments for this workspace run on the
designated remote Linux ECS. The workstation is for editing and static review.

## Original profile support matrix

JiuwenSwarm Flash A/B have scoped real cloud acceptance on both E2B and Huawei:
release23 passed the five ECS engineering gates and four real model coding-task
runs, each with the original 12/12 independent verifier and confirmed
Stop/revocation/cleanup. Native command observation captured 15 direct child
processes and 105 lifecycle markers. The four corresponding Perfetto, Jaeger
and Prometheus backend validations passed. These are four different real model
trajectories, not a same-Tape cloud speed ranking; Huawei verifier egress remains
assumed and global observation coverage remains unknown. See the
[release23 four-run report](jiuwen-release23-cloud-validation-20261027.md).

The next candidate registers all 20 native builtin skills by default; ECS L0/L1
native catalog checks passed. The user’s five-slide PPTX workload has passed
focused benchmark/verifier tests; its new image and four real-cloud task runs
remain **UNRUN** until their own acceptance evidence is recorded.

| Category | Supported implementation | Guide |
| --- | --- | --- |
| Agent harness | **OpenClaw** — text, realtime, and voice-transcribe modes; web tools and configurable subagent spawning | [Harness configuration](configuration.md), [realtime mode](configuration.md#realtime-openclaw-mode), [voice guide](voice_mode.md) |
| Agent harness | **Hermes** — text and voice-transcribe modes through its native Gateway; web tools, context compaction, and custom request bodies for compatible backends | [Hermes configuration](configuration.md#hermes-context-window-compaction-and-request-extra-body), [voice guide](voice_mode.md) |
| Benchmark | **Terminal-Bench 2** — verifier-based terminal tasks | [Quick start](quick-start.md) |
| Benchmark | **Deep Research Bench** — open-ended research reports with RACE grading and optional FACT citation checking; grading can be disabled | [Benchmark guide](benchmarks/deep-research-bench.md) |
| Benchmark | **SWE-Atlas QA** — codebase Q&A with host-side rubric grading; grading can be disabled; only the QA track is implemented | [Benchmark guide](benchmarks/swe-atlas-qa.md) |
| Benchmark | **SWE-bench Pro** — public issue-resolution split with pinned task scripts and parser | [Benchmark guide](benchmarks/swe-bench-pro.md) |
| Benchmark | **Toolathlon** — tool-use tasks over MCP servers that are local, public, or backed by Toolathlon's self-hosted applications (Canvas, poste.io, WooCommerce), and, given your account credentials, by third-party services; the `k8s` tasks are not supported | [Benchmark guide](benchmarks/toolathlon.md) |
| Tool sandbox and deployment | **Docker** — local containers managed through the Moby Go SDK | [Deployment configuration](configuration.md#deployment-configuration), [Docker implementation](implementation/docker.md) |
| Tool sandbox and deployment | **Kubernetes** — one pod per runtime, driven through `kubectl`; Terminal-Bench 2, SWE-bench Pro and Deep Research Bench with OpenClaw or Hermes in text mode | [Kubernetes deployment](configuration.md#kubernetes-deployment), [Kubernetes implementation](implementation/kubernetes.md), [cluster setup](../k8s/README.md) |
| Tool bridge | **OpenClaw SSH** and **Hermes SSH** — managed Docker container or Kubernetes pod; harness-specific SSH adapters | [SSH bridge implementation](implementation/ssh-bridges.md) |
| Model service | **DeepSeek** — external endpoint | [Model backends](configuration.md#model-backends) |
| Model service | **SGLang** — external endpoint or one ARIES-managed host process per run | [Model backends](configuration.md#model-backends) |
| Model service | **OpenAI-compatible server** — external only, including vLLM, llama.cpp, gateways, and hosted endpoints | [Model backends](configuration.md#model-backends) |

The external `openai` path also supports the bundled
[scripted mock model](configuration.md#scripted-mock-model) for infrastructure
checks. It serves model discovery and Chat Completions with JSON/SSE responses
for one fixed tool-call workload; it is not a general model or a complete API
emulator. Real benchmark verifiers remain in use, and its scores do not measure
LLM capability.

Runs also emit scheduler and Runner phase records. The bundled `aries-perf`
command checks recording completeness and computes single-run descriptive
statistics offline; see [performance records](run-results.md#performance-records-and-offline-summaries).
The separate E2B path below has real A/B acceptance, repeated/concurrent batches
and bounded fault checks. The earlier failed A run remains preserved. Model
first-output events are available, but command first-output attribution and
validated multi-cloud comparisons remain separate capabilities.

Both harnesses and [LLM judges](configuration.md#judge-model-settings) support explicit
[model reasoning effort](configuration.md#reasoning-effort).
Available efforts and tool support depend on the model and API.

Image and dataset revisions are pinned in [versions.json](../configs/versions.json).
Runnable combinations are provided in [profiles/](../profiles/); benchmark setup,
judge settings, and credential requirements are documented in the
[configuration reference](configuration.md#benchmark-settings) and benchmark
guides above.

## Experimental E2B cloud path

`aries cloud` uses a [separate configuration](configuration.md#e2b-cloud-workload-profiles)
and the same Benchmark, AgentHarness, ToolSandbox, and ToolBridge contracts.
Its accepted cloud workloads use OpenClaw `2026.7.1`; the separate TerminalBench selection
retains one original task per run. E2B Cloud has accepted workload baselines; Huawei AgentSphere B has
conditionally accepted real recording and strict replay under an explicit,
unverified verifier-egress assumption. Huawei A recording passed all 12
independent tests under the same assumption; its cross-cloud replay attempt
with the legacy tape failed strict matching on real filesystem metadata.
A separately recorded `merge-ranges-portable-v1` tape has now passed strict
recorded-timing replays on both E2B A and Huawei A. These are single attempts,
with Huawei's verifier-egress assumption still unverified. This path does not
enable every benchmark or harness in the table above.

| Implementation | Boundary and status |
| --- | --- |
| JiuwenSwarm Flash B | Explicit `agent_type=jiuwenswarm`, `placement=embedded`, existing E2B/Huawei adapters and codingcase workloads. Native source/Python guest paths are required; record uses a version-bound native history dialect and Replay retains complete Strict comparison. TerminalBench, Workload Replay and OpenClaw-specific settings remain rejected; the new external candidate is separate below. Release18 passed five engineering gates; E2B record7 and Huawei record2 each passed the original 12/12 verifier with confirmed Stop/revocation/deletion. Their different Tapes are not a workload comparison; Huawei verifier egress is assumed, not proven. Strict replay3 failed; mode discussion is deferred and the prompt variant paused. Native spans and cgroup boundary resources remain scoped, with global coverage unknown. The sampled SDK producer/export sets reconcile at 44/44/44 and 35/35/35, with zero missing or unexpected spans; this scoped coverage is complete, not global coverage or bounded-flush proof. See [configuration](configuration.md#jiuwenswarm-flash-embedded-candidate), [freeze and real acceptance](observability/VALIDATION_AND_REPRODUCE.md#jiuwen-release18), [earlier targeted audit checks](observability/VALIDATION_AND_REPRODUCE.md#jiuwen-producer-audit-candidate), and [cloud evidence](jiuwen-cloud-validation-20261027.md). |
| JiuwenSwarm Flash A candidate | Explicit `agent_type=jiuwenswarm`, `placement=external`, digest-pinned `harness.jiuwen_image`, and the existing local Docker deployment/network/host monitor. Its RemoteBridge targets the exact Runner-owned E2B or Huawei codingcase Sandbox. No OpenClaw SSH/protocol settings and no Workload extension. Not part of release18. The config/native/wiring/app packages passed targeted ECS unit/race/vet and both example profiles passed configuration loading; these checks do not run the opt-in native fixture. Real-cloud functionality, native ToolCall correlation and complete observability are not yet accepted. See [A fields and examples](configuration.md#jiuwenswarm-flash-external-candidate). |
| Original TerminalBench on E2B / Huawei A | External placement, one original task/profile, attempts/concurrency `1`, matching provider template binding, original task resources retained with explicit adaptation evidence, and original Agent/Verifier budgets. An omitted binding provider preserves E2B; Huawei must be explicit. Bindings may initialize declared directory mtimes with actual readback, without normalizing tool output or clocks. Original tests run in the same Sandbox after confirmed Stop/revocation. Both providers have scoped real acceptance; the [current case matrix](performance-archive.md#跨云用例交付进度) separates Strict and Workload results and retains failures. Huawei C++ recordings have passed the original 6/6 verifier, while its replay and newer recording failures remain in the [case evidence](remaining-strict-cases-20261026.md). These results do not claim no-egress verification, exact resource equivalence or candidate-capture Goodput. See [configuration](configuration.md#original-terminalbench-tasks-on-e2b). |
| E2B ToolSandbox + official Python SDK adapter | Real SDK smoke, v2 template startup/cgroup probes, five baselines, A C1×2 / B C1×2 / B C2×3 replay batches and bounded lifecycle fault probes **PASS**, with sandbox absence confirmed. Small batches do not prove capacity or tail accuracy. |
| E2B A: external OpenClaw | Runner creates a Docker Agent container on ECS; its private SSH bridge targets the exact E2B sandbox. Real recording and recorded-timing replay **PASS**: seven model turns, six real tool calls and all 12 verifier tests, with positive isolation and cleanup. The earlier failed run remains preserved. |
| E2B B: embedded OpenClaw | Gateway, Agent Loop, and local tools run inside E2B; the controller stays on ECS and does not create a Docker Agent runtime. Real recording and recorded/zero replay **PASS**: four model turns, three tools, all 12 verifier tests, positive isolation and cleanup. Native events retain tool metadata, not independent full result bodies or per-tool PID snapshots. |
| Record / strict replay | A/B each use their own real tape; real Agent, tools and new E2B instances participate in successful replay. Loopback checks cover full tape validation, zero/recorded SSE, old-history/tool-result/schema rejection, independent sessions and retries. The B C2 batch passed real concurrent cloud routing. Isolated Replay process cost is measured; incremental observer overhead remains unseparated. See [protocol and evidence](implementation/model-replay.md). |
| Explicit workload replay policy | `aries cloud` supports explicit `model.replay_policy=workload`: fixed recorded decisions and request structure, with textual tool-result differences recorded separately. Real tools and the original verifier still run; no automatic fallback from Strict. Release12 passed all five ECS engineering gates and same-Tape E2B/Huawei acceptance for DB (12 turns, 11 tool terminals, original 7/7) and SQLite v3 (41 turns, 40 tool results, original 3/3). Both include exact terminal/identity checks, difference evidence, dual Trace reconciliation and confirmed cleanup. Original failed attempts remain; Huawei SQLite uses a separately pinned shell-profile environment adaptation. See [DB evidence](workload-db-performance-20261026.md), [SQLite evidence](workload-sqlite-performance-20261026.md), [gates](observability/VALIDATION_AND_REPRODUCE.md#workload-replay-release12), and [contract](implementation/model-replay.md#workload-replay-policy). The standalone replay CLI remains Strict; Strict results are counted separately. |
| Jiuwen five-slide PPTX task | `llm-development-pptx-v1` preserves the user’s exact prompt, captures generated PPTX files after confirmed stop/revocation and applies an independent structural verifier (parseable file, exactly five slides, nonempty page content). Four E2B/Huawei A/B profiles are prepared; engineering and real-cloud acceptance are **UNRUN**, pending the new workload and native-skill integration checks. No presentation-quality score is claimed. |
| Versioned cloud workload | Top-level `workload` defaults to `merge-ranges`, preserving existing profiles and task behavior. `merge-ranges-portable-v1` changes only the initial task instruction; the buggy code, public/hidden tests and verifier stay the same. One real E2B A recording and its unchanged-tape E2B/Huawei A recorded replays passed: eight model turns, seven tools and 12 independent tests each; combined review **16/16 PASS**, with Huawei verifier egress explicitly assumed/unverified. The separate Huawei portable recording profile is available but untested. `merge-ranges-multicall-v1` has a separate initial instruction and unchanged source/verifier; its real recording/replay acceptance is pending, and existing portable-v1 evidence does not cover it. See [workload configuration](configuration.md#versioned-cloud-workloads) and [single-attempt results](performance-archive.md#portable-crosscloud). |
| Experimental native ToolCall identity | `harness.native_observation_protocol=aries-ssh-identity-v1` is opt-in and A-only. The pinned E2B A record/replay and merged Huawei A replay each verified 6/6 archived exec identities through native→SSH→Bridge→SDK→guest. Independent verification and cleanup passed; Huawei verifier egress is assumed/unverified. Derived images require a digest pin and base-version check. This is exact identity within the archived exec denominator, not all-tool capture coverage. See [versioned evidence](observability/VALIDATION_AND_REPRODUCE.md#p0-runtime-candidates). |
| Experimental B native process observation | `harness.native_observation_protocol=aries-native-process-v1` is opt-in and embedded-only, using a matching image/template. Five native behavior pairs, the frozen runtime release gates, and a real Huawei B replay of its unchanged Tape passed: 27 callbacks, three direct-child intervals and 12 independent verifier tests. Corrected offline analysis and both clock views passed TraceProcessor reconciliation; full coverage remains unknown. Keeps the native process clock separate; direct-child exit and natural pipe EOF do not prove whole-tree or PTY completion. See [evidence](observability/VALIDATION_AND_REPRODUCE.md#r07-huawei-b-native). |
| Experimental final ToolResult renderer | `harness.tool_output_renderer=stdout-stderr-v1` records the opt-in A/B candidate version and requires its digest-pinned image/template. The E2B A candidate completed a new real recording and unchanged-tape Strict Replay, followed by a merged Huawei A replay, with 12 independent verifier tests each. These runs cover foreground non-PTY exec. Native `process`, background execution and auto-yield remain available; complete regression of those branches is still unverified. See [configuration and boundaries](configuration.md#experimental-native-observation-and-output). |
| Experimental background final-result protocol | `harness.replay_tool_protocol=terminal-result-v1` is opt-in for external placement with SSH identity and `stdout-stderr-v1`. Requires its matching native image and a new tape. [Large-text v2 Strict](repro-variants-performance-20261026.md#text-v2-huawei-replay-1) and [SQLite v3 Workload](workload-sqlite-performance-20261026.md) passed two-cloud checks with two jobs/two waits per run and their original verifiers. These scoped results do not promise cross-run byte equality, every background/PTY branch, or primary-clock alignment of native background intervals. See [configuration and evidence meaning](configuration.md#experimental-native-observation-and-output). |
| Experimental TerminalBench instruction variants | Explicit `benchmark.variant` appends a hash-verified public instruction to one original task, with a separate logical ID and unchanged private evaluation and budgets. Large-text v2 has same-Tape two-cloud Strict acceptance; SQLite v3 has two-cloud Workload acceptance with its Strict failure retained. Their `terminal-result-v1` profiles, pinned provider bindings, original profiles and tapes remain separately identified. New variants require their own acceptance; see [exact instruction and evidence contract](configuration.md#terminalbench-instruction-variants). |
| ECS Compose deployment | Services, trusted public-IP HTTPS and E2B-origin model access **PASS**. Runner `bridge-evidence-1` and source are archived and activated; restoration was checked on the same daemon, not a clean machine. Dynamic E2B sandboxes are not Compose services. |
| Huawei B: embedded OpenClaw | `agentsphere-embedded-record-3` and `agentsphere-embedded-replay-1` **conditionally PASS**: each has four model turns, three real tools, all 12 independent tests, confirmed Stop/revocation before scoring, and deletion of task/verifier sandboxes. Replay matches the full history and 693 recorded SSE frames without a real-model call. Verifier egress remains explicitly assumed/unverified; these are conditional results, not an enforced no-egress or cross-cloud comparison baseline. |
| Huawei A: external OpenClaw | Existing ECS Docker Agent → private SSH/Bridge → Huawei ToolSandbox path. Original-workload real recording **conditionally PASS**: seven model turns, six tools and 12 independent tests. Its legacy E2B tape replay failed at `ls -la` because directory metadata differed; that failure is preserved. The separate [portable-v1 replay](../profiles/cloud/openclaw-agentsphere-external-portable-replay.json) **conditionally PASS** on the exact tape also replayed on E2B: eight turns, seven tools and 12 independent tests, with confirmed Stop/revocation and cleanup. All checked-in Huawei profiles leave the egress assumption disabled; experiments enabled it explicitly. See the [comparison and limits](performance-archive.md#portable-crosscloud). |

Huawei's no-egress requests are rejected before allocation by default. The
Huawei-only profile and Adapter switches must both explicitly enable the
assumption to permit the unchanged request; Stop, revocation, hidden tests, and
cleanup remain required. Analyze assumed/unverified results separately from
enforced isolation. The first two Huawei B runs failed at native plugin discovery
and Gateway authorization respectively; those failures, their fixes, and the
preceding protocol probes remain in the [deployment history](../deploy/agentsphere/README.md).

Both placements require root-controlled, writable cgroup v2 with `cgroup.kill`,
a dedicated `aries-agent` UID 60000, and no preexisting processes for that UID.
B additionally needs the pinned OpenClaw/Node installation in its template.
Missing containment is unsupported, not a successful no-op stop. Only confirmed
harness stop and bridge revocation allow private verification. The coding case
captures a stable candidate and runs hidden tests in a fresh sandbox of the
selected provider.

The [five E2B baseline profiles](../profiles/cloud/) record A/B separately, replay each with
its own tape and recorded timing, and replay B with zero added wait. The directory
also contains Huawei B recording and Huawei A record/replay profiles; the
[Huawei deployment guide](../deploy/agentsphere/README.md) separates their current
acceptance status and required configuration. The E2B
baseline matrix normally needs ten task/verifier sandbox creations plus separate SDK
smoke and template boot/verification sandboxes (one each). The adapter keeps
the cumulative count for accounting, without a cumulative creation limit.
The failed and successful A runs recorded **4/4 under the former configuration**,
with all four sandboxes confirmed absent. Those records remain unchanged.
Preserve the ownership ledger, live-sandbox limits, TTL, and confirmed cleanup.
See [deployment and validation](../deploy/e2b/README.md),
[template isolation](../pkg/harness/openclawcloud/README.md), and
[replay limits](implementation/model-replay.md). Neither a successful model
response nor a complete tape proves the task passed its independent verifier.

## Current limitations

- In the original Docker/Kubernetes path, each SSH bridge supports its corresponding harness. Crossed pairs are rejected
  before execution. Hermes requires `/bin/bash` in the task image; its bridge
  rejects Hermes's private `~/.hermes` file synchronization. OpenClaw requires
  `bin/aries-ssh-client` beside `bin/aries`. Both bridges require the matched Docker bridge image built with
  `make bridge-image`.
- On the Kubernetes backend, a task that executes as a non-root user, as
  SWE-bench Pro does, needs `setpriv` from `util-linux` in its image and a root container
  user for per-command UID/GID switching. The upstream SWE-bench Pro images
  provide both, so running it needs no extra step.
  Images need `/bin/sh`, and the bridge and derived Hermes images must be
  published to a registry the cluster pulls from; `make k8s-images` does that. See the
  [Kubernetes implementation](implementation/kubernetes.md).
- Realtime mode is OpenClaw-only and needs a separate TTS credential. See the
  [realtime setup](configuration.md#realtime-openclaw-mode).
- External model servers are operated separately from ARIES. ARIES does not
  install SGLang or download model weights for managed runs. Endpoint checks
  verify access to the configured model; they do not establish support for every
  server-specific extension.
- SWE-bench Pro has additional image-architecture, isolation, and licensing
  requirements; consult its [benchmark guide](benchmarks/swe-bench-pro.md).
- Toolathlon's application-backed tasks need Toolathlon's own Canvas,
  poste.io and WooCommerce deployments running on the Docker host; consult its
  [benchmark guide](benchmarks/toolathlon.md).
- Measurement availability and deployment portability have known gaps described
  in the [design principles](design.md#measurement-meaning-and-current-gaps). These limits
  matter when comparing runs across implementations.

## Roadmap

Generic deployment providers beyond Docker and Kubernetes are unimplemented.
The E2B cloud slice is a direct ToolSandbox/paired-harness integration, not a
`pkg/deployment` provider or a general E2B-compatible API server. Other cloud
providers beyond the experimental Huawei integration remain future work;
compatibility and complete workload acceptance must be verified per backend.
Shared bridge lifecycle and
deployment contracts accept explicit runtime placement and access settings;
a new provider must implement their execution, connectivity, and cleanup semantics
and be selected in composition wiring.

The original [bridge service](design/bridge.md) hosts independent sessions with one sandbox
ID each and deployment-resolved endpoints. gRPC control and SSH clients are
unauthenticated; one service host key is retained for the SSH handshake.

The cloud SDK adapter's private control protocol is separate from the original
gRPC bridge control transport. External OpenClaw retains its native SSH tool
protocol; embedded OpenClaw executes tools locally in the selected cloud sandbox. Neither
path proves cross-vendor E2B wire compatibility. First-output latency, detailed
Agent/tool resource attribution, and reproducible multi-cloud comparisons need
additional instrumentation and real execution evidence.

See the [deployment contract](design/deployment.md),
[Docker implementation](implementation/docker.md),
[Kubernetes implementation](implementation/kubernetes.md),
[SSH bridge implementation](implementation/ssh-bridges.md) for current boundaries.

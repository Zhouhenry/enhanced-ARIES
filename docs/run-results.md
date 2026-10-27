# Run results and troubleshooting

<a id="r01-real-multicall"></a>

**真实 DeepSeek 同轮多工具自主修复与 Strict Replay 已通过。** 新任务 `merge-ranges-multicall-v1` 只改变题目流程说明，原坏代码、公开/隐藏测试与独立 Verifier 不变。真实模型首轮发出两条独立读取命令，随后自主修复：第一次公开测试失败、中间一次复测仍失败、最后复测成功，失败工具结果均完整保留。录制和严格回放各 7 轮/758 帧/7 次真实 exec，独立 Verifier 均 12/12；14/14 归档调用精确贯通原生→SSH→Bridge→SDK→guest，ToolResult 与候选哈希一致。4 个 Sandbox 均独立 NotFound，无补杀或清理错误。旧受控协议样本的 score 0 保持原值。

两次实际 Perfetto 导入分别 469 区间/50 flow、454 区间/43 flow，起止/时长误差 0ns；对应 OTLP 为 469/454 Span、50/43 parent，均从同一份各自 Snapshot 投影。两份离线对账分别 1924/1864 项通过；完整性 complete、全局 coverage unknown。当前运行的后端发送/UI、华为/B 同 Tape 与观测扰动未验；不外推 P0 完成或生产容量。

[真实版本与复现证据](observability/VALIDATION_AND_REPRODUCE.md#r01-real-multicall)。

<a id="r01-multicall-app"></a>

**真实应用级多调用验收已限定PASS。** 原Tape直接经过真实`app.RunCloud`调度和四角色Runner：1条计划/接纳/执行/完成、1条`verification_failed`、0条验证成功，三次真实exec与原ToolResult严格一致。1010项对账通过：3/3原生→SSH→Bridge→SDK→guest精确身份、真实八阶段、同一Snapshot的221个Perfetto区间/27条flow及221个OTLP Span/27个parent；TraceProcessor导入误差0ns。2个Sandbox独立NotFound，无补杀，既有服务未改。因原录制为OpenAI请求方言，私有验收入口只将`NewAttempt`的Provider恢复为`openai`，并非未经适配的原CLI。此受控模型没有修复题目，原Verifier12项中11失败、score0保持；该历史记录的真实Agent解题验收当时尚未执行；后续成功另见上节，不回填此协议样本。

同一581文件源码`821eefe8…`已完成build/test/race/lint/integration五门禁（`70686b13…`）。本次Snapshot完整性complete、全局coverage仍unknown；后端实际发送/UI、本版扰动及华为/B多调用仍未验。来源、时间与复现见[应用级归档](observability/VALIDATION_AND_REPRODUCE.md#r01-multicall-app)。

<a id="r01-multicall"></a>

当前模型代理新增Tape v2多ToolCall数组与v1原件兼容。ECS定向门禁及E2B A受控模型录制/严格回放通过：两次各3真实exec、6条精确身份链、125项复核、4Sandbox确认删除。该协议fixture未修复题目、独立Verifier原样失败，不计Agent任务成功；直接Runner未生成app Attempt账本，不补造排队或完整Snapshot资格。见[完整证据与复现](observability/VALIDATION_AND_REPRODUCE.md#r01-multicall)和[阶段原值](performance-archive.md#r01-multicall)。


Use this guide after the [quick start](quick-start.md) or a run configured with
the [configuration reference](configuration.md). Artifacts are private and may
contain task or model content.

## Inspect a run

For the one-task profile:

```sh
run_dir="$(ls -1dt runs/*-openclaw-tb2-fix-git-deepseek | head -1)"
cat "$run_dir/live-validation.json"
cat "$run_dir/run-result.json"
task_id="$(jq -er '.tasks[0].task_id' "$run_dir/run-result.json")"
task_dir="$run_dir/$task_id"
cat "$task_dir/evaluation/reward.txt"
cat "$task_dir/bridge/tool-calls.jsonl"
test ! -f "$task_dir/bridge/ssh_raw.log" || cat "$task_dir/bridge/ssh_raw.log"
cat "$task_dir/harness/openclaw.json"
cat "$run_dir/infrastructure/bridge/monitor/index.json"
cat "$run_dir/aries.log"
```

For the SGLang example, select its run and inspect the managed runtime logs
when applicable:

```sh
run_dir="$(ls -1dt runs/*-openclaw-tb2-fix-git-sglang | head -1)"
cat "$run_dir/live-validation.json"
cat "$run_dir/run-result.json"
task_id="$(jq -er '.tasks[0].task_id' "$run_dir/run-result.json")"
task_dir="$run_dir/$task_id"
cat "$run_dir/aries.log"
jq 'select(.component == "gpu")' "$task_dir/monitor/resources.jsonl"
cat "$task_dir/monitor/index.json"
test ! -d "$run_dir/sglang" || ls -l "$run_dir/sglang"
```

For a multi-task profile, `run-result.json` contains one result per task and
each task has its own readable directory:

```sh
run_dir="$(ls -1dt runs/*-openclaw-tb2-five-deepseek | head -1)"
find "$run_dir" -maxdepth 2 -type d | sort
find "$run_dir" -path '*/evaluation/reward.txt' -print -exec cat {} \;
```

A successful task has:

- successful model validation;
- successful harness, confirmed isolation, successful evaluation and cleanup
  outcomes in `run-result.json`; inspect the independent observer status too
  (a disabled observer reports `not_enabled`; an unavailable measurement capability
  reports `unsupported` with a reason and zero samples);
- reward `1`; and
- completed tool calls in `bridge/tool-calls.jsonl`.

When a task fails, its `error` field in `run-result.json` joins every failure in
that task, including a preparation failure that leaves the harness
`not_started`; the task's `task finished` line in `aries.log` carries the same
error. List failed tasks with:

```sh
jq -r '.tasks[] | select(.error) | "\(.task_id): \(.error)"' "$run_dir/run-result.json"
```

Also inspect the run's `infrastructure` object. It records shared bridge and
network identities, the effective network policy, the bridge observer outcome,
startup/cleanup durations, and confirmed shared cleanup. Shared failures remain
visible there even when individual tasks succeed or startup admits no tasks.
These results are finalized before `run-result.json` is written; shared
infrastructure does not appear as a fabricated task or alter task counts.

A benchmark that evaluates in a fresh sandbox, such as SWE-bench Pro, keeps that
sandbox's container logs in the task's `evaluation-sandbox/` directory and its
resource samples under the `evaluation-sandbox` component in
`monitor/resources.jsonl`.

## Scripted mock runs

The [mock example](configuration.md#scripted-mock-model) writes to `runs-mock/`.
Its model ID identifies a fixture rather than an LLM. Inspect the same harness,
tool-call, isolation, evaluation, and cleanup evidence as for other runs. The
example's environment/file-I/O probe does not solve `fix-git`, so a failed task
verifier is compatible with successful execution of the scripted probe. Check
these outcomes separately; do not force the verifier to pass.

A reward from a mock run measures whether the scripted actions satisfied the
real task verifier. It does not measure model capability. The endpoint reports
no synthetic token usage, and model-call timings include the fixture's configured
response delay rather than real inference. Do not present missing usage as zero
inference cost or compare these timings as LLM-serving performance.

Keep the original fixture and its full SHA-256 from the server startup log with
the results; ARIES does not collect that external server log. Record the harness
version, deployment region, effective resource limits, and concurrency alongside
the fixture's response delay. Infrastructure comparisons need the same
representative workload and controlled conditions: eliminating model randomness
alone does not make runs comparable. Keep mock results separate from model
benchmark results. Task-arrival replay changes when tasks start; it is separate
from the mock endpoint's scripted model responses.

## Attempt accounting and verified throughput

Profile scheduling adds optional `attempts` and `attempt_summary` fields. Existing
`tasks` and `summary` still count Runner task results. The attempt ledger instead
retains every planned occurrence, including construction failures with no task
result and arrivals canceled before admission. Repeated logical task IDs have
distinct occurrence/attempt IDs; an Agent session ID is recorded only when the
native harness provides it. A missing or ambiguous task result is `unknown`.

Terminal accounting (`succeeded`, `failed`, `timeout`, `canceled`, `blocked`,
`not_started`, `unknown`) is separate from execution, independent validation and
cleanup. These stage counts overlap. `succeeded` can describe an execution with
evaluation disabled; it never implies a verified result. Independent success
requires both verifier success and confirmed harness-stop/bridge-revocation.

All rates use the measured scheduling-start to final admitted-attempt completion
window, including drain. The load-offering end and drain duration are retained.
`attempts_per_second` counts started occurrence attempts, including build failures;
`executed_throughput` counts calls entering the harness's Run method;
`execution_throughput` counts those whose harness execution succeeded;
`validated_success_throughput` counts independent verified successes. Unstarted
work remains in `planned`/`not_started`; it is not a started attempt or a hidden
success. Blocking admission delays dispatch rather than dropping work.

`execution.goodput_deadline` explicitly sets a positive duration, such as `30s`.
Without it, Goodput fields are omitted. Goodput counts independently verified
PASS results whose stable candidate was captured by the local controller no later
than scheduled arrival plus that deadline. Verifier completion may occur later.
Cleanup failure is recorded separately and does not change a proven, verified
candidate's deadline. Missing artifact capture time, path or SHA-256 evidence
excludes the attempt from Goodput; ordinary artifact logs are not this proof.

Huawei's explicitly enabled network-isolation assumption preserves these
counters. It does not automatically exclude results from
`validated_success_throughput` or `goodput`. A run with
`infrastructure.network_policy=assumed-huawei-verifier-egress-unverified`
reports scores and rates conditional on that assumption. Group analysis by
`infrastructure.network_policy` and keep those runs separate from baselines
with confirmed egress isolation. A verifier PASS and confirmed Agent stop still
do not prove that outbound isolation was tested successfully. See the
[Huawei configuration](configuration.md#e2b-cloud-workload-profiles).

`timing` contains locally observed stage boundaries and monotonic durations.
`harness_run_invoked_at` is distinct from native `agent_started_at`: the latter
and `runtime_execution_duration` remain absent when the harness cannot expose the
native call window. Native timing excludes artifact export but still includes
protocol transport and waiting. UTC timestamps are correlation anchors; remote
wall-clock differences and differences between percentiles are not execution
durations. A readiness timestamp is emitted only after the readiness operation
succeeds. Cleanup completion means the cleanup attempt ended, not that it
succeeded; inspect its status.

`verifier_duration` currently wraps the entire `Benchmark.Evaluate` call. For the
cloud coding case that includes candidate capture, fresh verifier-sandbox
preparation, pytest and report retrieval. `verifier_finished_at` is controller
confirmation of a terminal verifier report, not the guest process's exit clock.
Use the nested evaluation-sandbox and command evidence to inspect those costs;
do not present this outer duration as pure pytest execution time. Artifact
readiness has its own earlier, proven boundary within that call.

The new accounting and fault-matrix checks must be run on the remote ECS before
using these fields for benchmark claims. Unit fixtures do not constitute cloud
or real-model validation.

### Cloud evidence and comparison method

For measured runs and their acceptance limits, see the [performance report](performance-archive.md).
The [stage guide](performance-stages.md) explains each timing boundary, including
A/B placement differences. The table below defines how to interpret evidence;
it is not an additional performance result. Use the [deployment procedure](../deploy/e2b/README.md)
and preserve every failed attempt alongside successful observations.

| Evidence | What it establishes | What it does not establish |
| --- | --- | --- |
| `cloud-profile.json`, template build/probe evidence, native version evidence | Requested placement, pinned inputs, actual versions/capabilities when probed | An image tag alone does not prove immutable binaries, hardware equivalence or a cold host |
| `attempts`, `attempt_summary`, task `timing` | Planning/admission/terminal denominator and controller-local phase boundaries | Missing timestamps are not zero durations; infrastructure/preflight sits outside the attempt window |
| `model_sessions`, per-attempt `model/events.jsonl` and recorded `model/tape.json` | Explicit model-session binding, real HTTP requests/SSE, strict replay status and timing condition | A successful verifier cannot turn an incomplete/diverged replay into a successful replay experiment |
| A bridge audit and SDK operation evidence | Controlled tool path, target identity, transport outcome and available command evidence | One SSH request is not necessarily one native Tool Call; an RPC duration is not pure guest execution |
| B native Gateway tool evidence, when collected | Native tool identity and observed event boundaries | A background tool return is not process termination; SDK controller calls do not enumerate B's local tools |
| `sandbox-metrics.jsonl`, `cloud-observation.json` | Raw provider measurements and distinct external Docker Agent observations | Repeated SDK timestamps are not new independent samples; unavailable disk/network/Agent-Tool attribution stays unsupported |
| Candidate SHA-256, JUnit, isolation and cleanup records | Stable candidate, independent hidden-test score, positive stop confirmation and deletion outcome | An Agent's final answer, failed health probe or issued kill request is not independent proof |

Use run/attempt/occurrence/session/sandbox-handle IDs to join evidence. Keep the
adapter's real provider-ID mapping in its private ownership ledger. Exact IDs
take precedence over command-text or timestamp proximity. If a span cannot be
associated unambiguously, retain it as unassociated and report coverage instead
of assigning a plausible parent. Never subtract UTC timestamps from different
hosts to derive a network duration. A guest-reported wall-clock duration must
also remain labelled as such, separately from a locally monotonic interval.

For each condition, first reconcile the planned terminal count and missing-data
counts; then report verified successes, timeouts, isolation failures, cleanup
failures and model-session failures. Only then report latency distributions with
their sample counts. Keep timeout/cancellation counts separate from completed
latency samples. Do not replace a timeout with a measured duration equal to its
deadline, average worker percentiles, or subtract P95s to estimate a stage cost.

The first matrix is A record, B record, A recorded replay, B recorded replay,
and B zero-wait replay. Use separate real tapes for A/B. A record versus A replay
tests the same placement under a different model timing source; B recorded versus
B zero changes artificial wait while the tool path remains real. A versus B
changes Agent location, resources and the tool channel, and may change prompts,
tool schemas/results and therefore decisions. Describe those differences before
attributing a latency gap to the sandbox. A strict mismatch is a failed control
condition, not a reason to relax matching.

Repeat after a single verified case, with the same published template, actual
Agent version, chosen resource request, controller placement and explicit SLO.
Record region, effective resources and cold/warm condition only when measured or
supplied by an authoritative provider field; otherwise retain `unknown`. Increase
concurrency in bounded steps only within the configured quotas. Randomize or
alternate condition order where possible; report per-sandbox/run samples rather
than treating every command from one sandbox as an independent machine sample.

Examine queue/admission delay, native Agent interval, model requests, tool
intervals, stop, verifier and cleanup separately. Model waiting and tools may
overlap or nest; summing their durations can double-count. The residual of
unmatched intervals is **unattributed time**, not measured Harness CPU overhead.
Use an explicit same-clock critical path only where the IDs and boundary coverage
justify one. Resource sampling can explain sustained pressure but can miss short
peaks; keep its interval and duplicated/absent-sample counts with the analysis.

Model proxy process CPU/RSS and goroutine measurements cover the whole process;
cloud embeds that proxy in Runner and concurrent sessions share it. They cannot
be divided into session-specific inference cost. Check ECS controller CPU, SDK
Adapter saturation and HTTPS buffering before calling a concurrency plateau a
sandbox bottleneck. The current cloud profile does not offer an observation-level
toggle: observation-on/off overhead comparisons remain a separate calibration
experiment, not a claimed implemented benchmark mode.

Model first-output events distinguish `first_sse`, `first_content`,
`first_reasoning`, and `first_tool_call_delta`. A role-only or heartbeat frame is
not a first content token. Record mode's `received_offset_ns` is when the proxy
received the complete SSE event; replay's `actual_offset_ns` is when its local
Write/Flush returned, alongside the configured `target_offset_ns`. Neither is
the time the Agent consumed the data or an observation of internal token
generation. Count `request_retry` separately from new recorded turns; cached
record retries are labelled `cached_retry_<mode>` (zero for the cloud path) and
perform no new upstream call.
Report the chosen first-output definition instead of an unqualified “TTFT”.

The original `aries-perf` summary checks scheduler/Runner records. Snapshot and
Trace modes additionally validate available model and native Tool evidence, but
do not automatically validate every cloud join or infer cross-condition causes. Those checks and their limitations must accompany the experimental
report; a `complete` stage-record summary alone is insufficient.

## Performance records and offline summaries

After preparation and model preflight succeed, `aries` records scheduler and
Runner timing automatically. No profile field is required. The new artifacts
are private files under the run's `performance/` directory:

| File | Meaning |
| --- | --- |
| `manifest.json` | Schema version, run and emitter identity, monotonic clock domain, event filename, scope, Go/platform and available build metadata. |
| `runner-events.jsonl` | Raw schema2: arrival transitions, paired operations and measured instants. One process owns this stream; schema1 remains readable. |
| `index.json` | Finalization status and attempted, written, dropped and failed-write counts; absent after an interrupted finalization. |
| `late-after-close.json` | If present, a producer wrote after closure: the prior index cannot establish final completeness. |

The manifest's scope is `scheduler_and_runner`. It is not yet a complete
experiment manifest: fixture/data hashes, effective cloud resources, template
provenance and controlled workload comparisons are subsequent work. Build
metadata is present only when the Go binary contains it; it is not a fingerprint
of every uncommitted file or external fixture.

`run-result.json.performance` records artifact finalization independently of
task, scoring, resource-monitor and cleanup outcomes. Recording starts before
shared infrastructure creation and closes after shared cleanup, before the run
result is persisted. Initialization failure prevents infrastructure admission;
later recording errors do not skip task or infrastructure cleanup. Buffer
overflow, write, sync or finalization errors fail this observation outcome and
the command. The bounded queue holds 4096 events; core events are not sampled.

To recompute a report after the run has stopped:

```sh
go build -o bin/aries-perf ./cmd/aries-perf
./bin/aries-perf "$run_dir" > "$run_dir/performance/summary.json"
```

The command also accepts the `performance/` directory itself. It does not modify
the input files. Shell redirection creates the derived report; choose another
filename to preserve an earlier report. Incomplete recordings still produce a
partial JSON report, with `status: "incomplete"` and `problems`, and exit nonzero.
A complete report means the recording reconciles, not that tasks passed or the
experiment qualifies for a cloud ranking.

### Viewable offline trace

`aries-perf --trace` exports Chrome Trace Event JSON, readable in a compatible
Chrome Trace or Perfetto viewer. It needs no collector or running service.
Build the analyzer on the development ECS and keep derived files separate from
the immutable run evidence:

```sh
mkdir -p "$run_dir/analysis"
./bin/aries-perf --trace "$run_dir" > "$run_dir/analysis/trace.json"
```

The original summary command and its meanings stay unchanged. The trace exports
validated Runner operation pairs, including failed and timed-out intervals, and
all scheduler transitions, including work that never started. `operation_id`,
`parent_operation_id`, occurrence and arrival IDs preserve recorded relationships.
An unreconciled Runner stream produces a diagnostic and withholds its intervals.
The exporter never manufactures spans from aggregate statistics or residual time.

When `run-result.json` is present, exact attempt/session identities select the
cloud attempt's `model/events.jsonl` and embedded harness native Tool evidence.
Model request intervals use the actual observed start/finish pair; admission
duration is retained as a separate attribute. Retry deliveries remain separate
observations even when they share a turn number. First SSE/content/reasoning/tool
delta and frame-delivery observations retain their explicit timing source and
recorded/synthetic target offsets. They are not individual model-token spans.
Native Tool pairs require the same native run, session and `tool_call_id`, with
the native run also checked against `agent-result.json`. Concurrent Tool calls
get separate lanes. These are start/result receipt intervals; they do not prove
that a launched process or its background children have exited.

**Default Trace contains only the primary Recorder clock domain.** Newly
instrumented model and B observations join it only through an explicit shared
in-process monotonic anchor. Old unaligned domains remain in the snapshot and
`excluded_clock_domains`; export each separately with `--trace-clock <domain>`.
Different process lanes alone would not establish separate time axes. Guest
wall timestamps remain attributes and are never subtracted from controller
observations. No unsupported network or critical-path inference is made.

`--snapshot` exports the versioned analysis result with source SHA, raw ns,
identities, intervals, evidence references and orthogonal integrity/coverage.
Trace is a projection of this same snapshot, with a checked content identity;
it does not parse and correlate the inputs independently. A missing A native
Tool source or unknown expected denominator is explicit, even when legacy
Runner stage records reconcile. See the [measurement contract](observability/OBSERVABILITY_SCHEMA_AND_MEASUREMENT.md)
and [implementation/validation report](observability/OBSERVABILITY_IMPLEMENTATION_REPORT.md).

Trace metadata lists source SHA-256 hashes, clock domains, limitations and
diagnostics. Missing optional native Tool artifacts are explicitly unavailable;
invalid identities, missing event pairs/terminals or malformed evidence produce
`status: "incomplete"` and a nonzero exit, while preserving available observations.
It projects only timing, bounded event fields and IDs; request/response bodies,
commands, raw error messages, credentials and model tapes are not exported.
The file remains an internal experiment artifact, not an automatic public report.
`complete` describes the exported observation checks, not independent task
verification, measured network isolation, complete instrumentation coverage or
benchmark success. Apply the result and network-policy qualifications above.

<a id="p1a-offline-jaeger"></a>

### Offline OTLP and Jaeger

Build and run the analyzer on the development ECS. Both exporters can consume
one frozen snapshot, without reading or correlating the raw logs again:

```sh
./bin/aries-perf --snapshot "$run_dir" > analysis-snapshot.json
./bin/aries-perf --trace --from-snapshot analysis-snapshot.json > trace.json
./bin/aries-perf --otlp --from-snapshot analysis-snapshot.json > otlp-export.json
./bin/aries-perf --otlp-send otlp-export.json http://127.0.0.1:14318/v1/traces > delivery.json
```

Use a fresh output directory and an independently prepared receiver; the
validation backend has been removed. `--trace-clock <domain> --from-snapshot`
also accepts the same snapshot. Loading verifies its original content identity;
the OTLP envelope contains the standard `request` and a separate `manifest` with
version, source hashes, stable IDs and excluded operations. Only real local UTC
anchors yield epoch spans. Unanchored native intervals, instant/SSE events and
duration-only records stay explicit in the snapshot rather than becoming guessed
spans. Coverage can remain unknown while recording integrity is complete.

The sender makes one request with a 30-second timeout and no automatic retry.
`accepted_unverified` requires an independent backend query; timeouts mean
delivery is unknown, and partial-success warnings/rejections are retained.
Fixed Jaeger 2.22 with skew adjustment disabled passed actual A/B/cancellation
queries for 2,424 unique spans, with zero nanosecond error through its v3 API.
Intentional repeat delivery stored duplicate rows: stable IDs do not provide
exactly-once ingestion or additional successful benchmark tasks. Real samples
had parent relations but no Links; a separate, explicitly synthetic fixture
verified same-trace and cross-trace Links. See the
[versioned evidence and reproduction steps](observability/VALIDATION_AND_REPRODUCE.md#p1a-offline-jaeger).
UI rendering, large payloads, authentication, durable storage and realtime
export remain unverified. Scoped Prometheus Metrics validation is described below.

<a id="p1b-offline-prometheus"></a>

### Offline Metrics and Prometheus

The offline accounting and stage metrics passed targeted ECS tests and real
Prometheus 3.15.1 queries for frozen A, B and cancellation snapshots. Those
v1.4 snapshots have no resource samples; the separate resource stream export
is described below. Run the analyzer on ECS, use a fresh output
directory, and keep the same frozen snapshot used for the Trace exports:

```sh
./bin/aries-perf --metrics --from-snapshot analysis-snapshot.json > metrics-export.json
./bin/aries-perf --metrics-send metrics-export.json http://127.0.0.1:9090/api/v1/otlp/v1/metrics > metrics-delivery.json
```

The endpoint must be an independently prepared OTLP-enabled receiver. Use an
isolated TSDB for each immutable batch, because run/attempt IDs are deliberately
absent from metric labels. The `request` contains only low-cardinality
phase/outcome/scope attributes and service name; the `manifest` retains the
snapshot ID, source hashes, original accounting/window, missing fields and
histogram members.

Lifecycle and terminal counts copy the original AttemptSummary into cumulative
Sums. Lifecycle phases overlap and must not be summed as separate tasks. Rate
and Goodput Gauges copy the original values, with no new success calculation.
All points use the original window end; Sums and Histograms also retain its
original start. They are historical batch results, not a live Counter from
which `rate()` or `increase()` can reconstruct throughput. Missing Goodput
values remain absent, including when other verification counts are present.

Stage duration Histograms use seconds and explicit bounds, grouped by phase,
terminal **attempt** outcome and measurement scope. The manifest retains raw
nanoseconds, exact count/sum, each member and non-cumulative OTLP bucket counts.
An actually measured zero is included; an uncollected stage is not. A canceled
`verifier` stage may measure only the Evaluate entry returning immediately, not
executed verifier tests. Resource Gauges remain `NOT_COLLECTED` for those old
frozen snapshots. New snapshots can contain optional resource streams, which
are exported separately from accounting; missing CPU/RSS/IO/network values
are never filled with zero. Missing AttemptSummary yields `NOT_COLLECTED`;
an invalid historical window returns an error.

Each send makes one HTTP request with a 30-second timeout and no automatic
retry. A Prometheus HTTP 200 with an empty body is `accepted_unverified`, not
proof of stored counts or buckets. Query the original history window separately;
preserve partial-success warnings/rejections and unknown timeout delivery.
Manual repeat sends are explicit validation actions, not an exactly-once
guarantee. See the [measurement contract](observability/OBSERVABILITY_SCHEMA_AND_MEASUREMENT.md#p1b-offline-prometheus)
and [acceptance archive](observability/VALIDATION_AND_REPRODUCE.md#p1b-offline-prometheus)
for scoped results, timestamp precision and remaining gaps.

<a id="p1c-resource-prometheus"></a>

### Resource streams and per-owner Metrics

Parser `aries-analysis-v1.5.0-resource-streams` adds optional
`resources.streams` without changing schema 1 or the IDs of existing frozen
snapshots. Old Trace, OTLP and accounting Metrics projections have passed
compatibility checks. Candidate 2 passed ECS unit/race/vet/build; real resource
backend reconciliation passed for the 13 documented owner streams (456 points), including explicit repeats; six unavailable streams were not sent. The first candidate's rejection
of valid zero-based monitor sequences remains archived as a failure.

Select one `stream_id` from the frozen snapshot and export on ECS:

```sh
./bin/aries-perf --resource-metrics STREAM_ID --from-snapshot analysis-snapshot.json > resource-export.json
./bin/aries-perf --metrics-send resource-export.json http://127.0.0.1:9090/api/v1/otlp/v1/metrics > resource-delivery.json
```

Use a separate empty TSDB for each owner stream and for the accounting batch.
Labels contain only scope and optional purpose/component; owner, run, attempt
and Sandbox IDs stay in the manifest. No asynchronous owners are summed or
interpolated. The source stream retains every original receipt, including
repeated provider readings, empty responses and failures.

Controller samples use their own UTC anchor and sampling completion offset;
the start/finish bracket remains visible because MemStats and Rusage are read
sequentially. RSS means process-lifetime peak, not the run's interval peak.
Original controller deltas remain evidence, not fabricated sample points.
A-mode monitor samples retain their source timestamps, cumulative CPU and
memory values; raw `cpu_percent` is not exported because its first zero is a
baseline placeholder. E2B uses the original provider timestamp: identical
timestamp/value points share one exported point while retaining all sample
IDs. This does not establish an independent sample count or a five-second
update guarantee. Huawei `ts` has no verified unit, so its readings and RPC
boundaries remain evidence without resource points; request time is not
substituted for sampling time.

All resource values are Gauges. Cumulative readings have `_observed` names,
with no invented Counter start epoch. Units are `s`, `By`, `%`, or unitless
counts; percentages are not divided by 100. The manifest preserves exact raw
numbers alongside float64 export values. Conflicting values at one timestamp
fail export; unknown time/unit or empty readings produce no fake zeroes.
Disk usage is not disk IO, and Sandbox measurements are not per-Tool costs.
See the [resource contract](observability/OBSERVABILITY_SCHEMA_AND_MEASUREMENT.md#p1c-resource-prometheus)
and [versioned evidence](observability/VALIDATION_AND_REPRODUCE.md#p1c-resource-prometheus).
HTTP acceptance still requires a separate historical query; neither these
exports nor engineering tests establish realtime or full R08 completion.

### Scheduling and operation boundaries

Finite batches and arrival replay record every planned occurrence, including
items canceled before admission. The duration-limited loop records only the
candidates actually generated. Each occurrence has separate logical task,
execution and arrival identifiers. The scheduler emits `arrival_planned`,
`arrival_admitted`, `arrival_dispatched`, and either `arrival_finished` or
`arrival_not_started`. Admission duration measures planned arrival to capacity
acquisition; finish duration measures dispatch through occurrence return.
An item canceled before its scheduled time has no fabricated queue duration.
Admission still blocks when concurrency is full: this is arrival replay with a
capacity limit, not a strict open-loop generator.

Runner operations include `task.e2e`, `sandbox.start`, `sandbox.prepare`,
`bridge.register`, `harness.start`, `agent.run`, `harness.stop`, `bridge.revoke`,
`benchmark.evaluate`, and `sandbox.stop`. `occurrence.build` records construction
before the Runner starts; it does not include resource-observer startup.
Evaluation-sandbox starts and stops use `purpose: "evaluation"`, separately from
the task sandbox. Operation outcomes describe returned errors; a successful
evaluation call does not imply a passing reward. Inspect `run-result.json` and
verifier artifacts for scoring, isolation and cleanup results.

Durations and offsets are nanoseconds measured within one process using Go's
monotonic clock. UTC timestamps are for presentation. Parent intervals include
child intervals; do not add them or subtract independently computed p95 values.
The current spans do not propagate through the Bridge child process or provider
API. `sandbox.start` measures the existing interface call, not physical cold
boot or a new cloud TTI probe. Command first-output timing, file throughput and
provider retry/deletion details remain planned work.

`aries-perf` checks schema/identity, sequence and writer counts, operation pairs
and parent relationships, arrival transitions and terminal records. It groups
operations by logical task, operation and purpose within one run. It reports
success/failure/cancellation/timeout/incomplete counts and nearest-rank duration
quantiles for successful operations only. Failure waits remain in the raw data
and do not enter successful latency distributions. Arrival queue observations
include valid admitted occurrences that reached a terminal outcome, including
failed occurrences.

Missing statistics are null. p99 is shown only with at least 1000 successful
observations in that group; these are descriptive sample quantiles, not evidence
of independent samples or a precision guarantee. Confidence intervals,
correlation-aware resampling, multi-run comparisons and plots are not implemented
in this first batch. Instrumentation overhead has not yet been calibrated on
the remote target workload. See the [performance design](design/sandbox-performance.md)
for the next stages and the [architecture guide](architecture-guide.md) for how
the components fit together.

## Measurement availability

Read observer status and `monitor/index.json` before interpreting resource data.
An initial deployment capability gap is recorded as `unsupported` with a `reason`
and `sample_count: 0`; `resources.jsonl` contains no fabricated zero measurements.
This allows task execution and evaluation to continue. Discovery errors,
permission errors, and later sampling failures remain failures rather than being
reclassified as unsupported. A successful observation whose counters are zero is
still a real sample with status `succeeded`.

Each sample's `second`, and the index's `first_second` and `last_second`, are
whole seconds elapsed since monitoring started, not a count of samples
(`schema_version` 5). Docker is sampled every second. The Kubernetes source
reads Prometheus, whose figures the kubelet refreshes on its housekeeping
interval (10 seconds by default), and records a sample only when the scraped
observation advances. See
[Kubernetes resource measurements](implementation/kubernetes.md#resource-measurements).

Docker sampling can overlap runtime teardown. If a stats response lacks its
observation timestamp, ARIES checks the same runtime again and skips that sample
only when the runtime is confirmed stopped or removed. Invalid stats from a
running runtime, or failure to confirm its state, remain observer errors.

The shared bridge is measured once for the run, including idle periods between
tasks. Its samples and index are under
`infrastructure/bridge/monitor/`, with component `bridge`, `scope: "run"`, and
`run_id`; they contain no `task_id`. Per-task monitor files contain only that
task's harness, sandbox, evaluation sandbox, and configured GPU observations,
with `scope: "task"`, `run_id`, and `task_id`. The index `scope` field distinguishes
the two scopes. CPU percentages use differences between cumulative CPU usage
and observation times; the first sample for each runtime is a zero baseline.
Shared costs are not duplicated or apportioned across tasks. See the
[measurement schema limits](design.md#measurement-meaning-and-current-gaps) for
first CPU baselines and GPU-only fields.

## Hermes Gateway artifacts

Hermes text and voice-transcribe now submit tasks through the native Gateway.
Historical CLI artifacts map as follows:

| Historical CLI artifact | Gateway artifact and meaning |
| --- | --- |
| `hermes_stdout.log` | `gateway-output.txt`: native run `output`, rather than process standard output. |
| `hermes_stderr.log` | Removed; service diagnostics are retained in bounded, redacted `container.log`. |
| `session-outcome.json` with `exit_code` | Same filename, with `run_id`, `session_id`, native `end_reason`, and ARIES `status`; no CLI exit code. Timing covers submission through terminal observation, separately from voice preparation. |
| `telemetry/sessions.jsonl` | Retained native message-level session export. |
| `telemetry/otel-spans.jsonl` | Tool, model, and API timing from the pinned `hermes-otel` plugin; see [collection limits](implementation/harnesses.md#hermes). |
| `config.yaml`, voice artifacts | Retained private configuration, audio, transcript, and voice result. |

A completed native run is a harness outcome. Check isolation, evaluation, and
cleanup separately in `run-result.json`. Historical CLI runs used
`--ignore-rules`; the [compatibility note](implementation/harnesses.md#pinned-gateway-compatibility)
describes the Gateway differences relevant to comparisons.

## Bridge evidence

`bridge/ssh_raw.log` is an opt-in mode-0600 sensitive audit. It is written only
when a profile sets `bridge.retain_raw_log` to `true`; an omitted or `false`
value drops it, so the file is absent by default. When retained it contains
lossless, human-readable text records between full-line
`--- ARIES SSH CALL BEGIN ---` and `--- ARIES SSH CALL END ---` delimiters.
Fixed-order `key=value` lines include the decoded wire command when available,
exact payload and stdin byte counts, and escaped exact payload/stdin. Printable
UTF-8 appears literally; backslash, newline, carriage return, tab, other
controls, and invalid UTF-8 use explicit escapes. The file is neither JSON nor
base64. It may contain exact wire-supplied values; keep the run directory
private and do not publish this artifact without review.

`bridge/tool-calls.jsonl` remains valid line-delimited JSON. Printable Unicode
and HTML characters such as `&&`, `<`, and `>` appear literally, while quotes,
backslashes, and newlines retain required JSON escaping. Printable stdin stays
inline; binary or control-bearing stdin is replaced by a concise
`binary-omitted` marker and exact byte count, with lossless bytes retained only
in `ssh_raw.log`. Structured lifecycle logs and tool-call records continue to
omit environment values and stdout/stderr bodies.

Both bridge logs identify the `sandbox_id` and a sequence number local to that
sandbox's writer, with run/task and exact backend runtime metadata. One harness
tool call may produce several SSH requests. Each session has its own listener
and logs even though the service container is shared. Releasing one sandbox
closes its bridge access and finalizes its evidence without stopping the service
or other sessions. `bridge/runtime.json` records that sandbox's binding;
`infrastructure/bridge/runtime.json` records the shared service runtime.

Each OpenClaw task directory contains the exact placeholder-only rendered
`harness/openclaw.json`, OpenClaw logs and telemetry when available, replayable
SSH tool inputs, Docker sandbox logs, one-second CPU and memory samples,
verifier stdout/stderr, and CTRF output. `aries.log` is the structured Logrus
run log.

OpenClaw text mode writes `harness/agent-result.json`. Realtime mode instead writes the
private `harness/voice-instruction.txt`, `harness/voice-instruction.wav`, its
metadata, and `harness/realtime-result.json`. These mode-specific harness and
bridge artifacts may contain task or model content; review them before sharing.

## Troubleshooting

- **Docker permission or socket error:** run `docker info` against the configured
  daemon. ARIES requires a local Unix socket; the default is `/var/run/docker.sock`.
- **Missing `aries-ssh-client` error:** rebuild with `make build` and keep the helper
  beside the main binary.
- **Credential error:** check ownership, owner read access, absence of group or
  world permissions, and one-line formatting of `DEEPSEEK_API.key`.
- **Model error:** inspect `live-validation.json` for authentication, rate
  limit, connectivity, or missing-model categories.
- **Realtime TTS error:** confirm `OPENAI_API_KEY` is set and that the provider,
  model, and voice in `harness.realtime.tts` are available to that account.
- **OpenClaw Gateway or realtime session error:** inspect the task's
  `harness/gateway.log`, `harness/realtime-result.json`, and
  `harness/telemetry.index.json`, then correlate the harness status in the run's
  `run-result.json`. Keep these private artifacts out of issue reports unless
  their task and model content has been reviewed.
- **Hermes Gateway error:** inspect `harness/session-outcome.json`,
  `harness/container.log`, and `harness/gateway-output.txt`, then compare the
  independent harness, isolation, evaluation, and cleanup outcomes.
- **SGLang configuration error:** confirm that the YAML uses only the supported
  fields and that its served model and port match the profile in managed mode.
  External mode does not validate the native YAML file.
- **SGLang readiness error:** inspect `sglang/stderr.log`, confirm that
  `model.base_url` ends in `/v1`, and test `/health` and `/v1/models` from the
  host. A server reachable only through loopback is not reachable from
  OpenClaw's container.
- **Managed SGLang exits early:** confirm that `runtime.config.executable` is
  the Python executable from the SGLang environment and that the selected GPU
  has enough free memory.
- **GPU monitor error:** confirm `nvidia-smi` is available and every configured
  `runtime.config.gpu_indices` entry exists. GPU indices must be unique and
  non-negative.
- **Unknown task or invalid task image:** choose a task directory from the
  pinned checkout and ensure its `task.toml` declares a valid explicit image
  tag. Digest-bearing or implicit-`latest` task references are rejected.
- **Terminal-Bench revision mismatch:** move the stale checkout aside, then
  rerun the profile command (or the optional setup prewarm). ARIES never
  deletes it automatically.
- **SSH timeout:** confirm the harness and bridge containers share the run
  network and the harness can reach the bridge container's advertised SSH address
  and port. SSH traffic uses that container address; control uses host loopback.
- **Suspected leak:** inspect `docker ps -a --filter label=aries.managed=true`
  and `docker network ls --filter label=aries.managed=true`.

For architecture and security boundaries, see [the architecture guide](design.md).
For the exact implementation matrix and configuration pointers, see
[supported implementations](supported.md).

### Cloud controller resource pair

Cloud runs also write `controller-resources.start.json` and
`controller-resources.end.json`, independently of baseline/L0/L1. The first file
remains incomplete and immutable; the final file contains process SELF CPU and
allocation/GC deltas, instantaneous heap values, lifetime peak RSS, and both
sampling intervals. These are shared Runner/model-proxy/Go-collector totals,
not separate component or Sandbox measurements. A missing final file is not zero
cost. Final report I/O lies outside the sampled pair; external process accounting
is required for the whole invocation. Observation failures do not replace the
workload result. See the [measurement contract](observability/OBSERVABILITY_SCHEMA_AND_MEASUREMENT.md)
for collection boundaries and the separately archived baseline/L0 cloud pairs.

Scoped file API counters are also available in the performance and model boundary
indices, `model/events-file-io.json`, the E2B operations index, and
`cloud-observation.json.sandbox_metrics_file_io` (when resource sampling ran).
They count actual Write/Sync calls and returned bytes, not physical disk writes.
Each scope excludes its metadata/report writes and other processes. Missing or
disabled writers are not measured zeroes. Check the legacy model sidecar's own
status and short-write/error counters even when the model session succeeded.

The metering source has now passed all five ECS Go release gates in
`/opt/aries-artifacts/observability-r15-release-1/evidence.json`
(SHA256 `b0260a73807e0771217010fffd6cfe983a25f8cf6999eaca21394496eb69f2db`).
Its 559-file source manifest is
`f26d2dcd051ad6ed822e381e69f2a3781b825dd72262e5d15d57675db27990e2`.
This covers build, unit, race, lint and Docker integration gates only: no paid
model calls, cloud allocations or production deployment. Source/fixture checks
and container/network inventories were unchanged, with no live gate process
groups. These engineering gates alone do not establish performance overhead or
extend older cloud results; the subsequent real experiment is recorded separately below.

A separate signal-handling candidate was tested by extracting its actual
functions and signal registrations, without running its full main. TERM/HUP
each produced controlled failure evidence and removed the owned sleep process
group. Evidence: `observability-r15-driver-signals-1/evidence.json`, SHA256
`42c0aaf0f177597497a055608ac9e45c4779aece831a9f0a5168a03b2cd5836f`.
This is not a full release-gate or Docker cleanup validation of that candidate.
The initial SCP failure, distinct driver hashes, gate durations and binary hashes
remain in the [validation archive](observability/VALIDATION_AND_REPRODUCE.md#r15-metering-release).

### Current baseline/L0 cloud pairs

Eight fixed-version E2B A Strict Replay tasks (four interleaved baseline/L0 pairs)
passed their declared functional and measurement checks. Each had six exec calls
and twelve independent verifier tests; all sixteen created Sandboxes were confirmed
NotFound and cleanup errors were empty. Evidence:
`/opt/aries-artifacts/observability-r15-paired-2/evidence.json`, SHA256
`3fb5a096b2ee3253757ef93e05a1691d28a32fed37d4272faea1c78ea90df37a`.
The same directory contains `paired-analysis.json`, SHA256
`031ae0159d92c4eb0511bf2ff1b5541f4f561b665876ecfed0b5bddb59999c0d`.
The 394-check independent source/arithmetic review has SHA256
`d182fba9e138b69529ffdff91d12138fa5c822f6045844d9e351d7acfee47286`;
it did not run more tasks, query the provider again, reconstruct HTTP requests or validate UI.

Both conditions retain native identity collection. Baseline model frame/turn
observations and disabled FileIO counters are null; L0 measures four JSONL writers,
while the L1 metrics writer is disabled. SELF CPU/Go allocation/GC is shared process
cost; lifetime peak RSS is not an interval peak. Region and provider cache state
remain unknown. Four pairs without an approved threshold support descriptive
differences only, not acceptable overhead, a speedup, p99 or provider rankings.
See the [eight-stage and resource tables](performance-archive.md#r15-current-paired).

Returned legacy model `events.jsonl` Write/Sync/Close errors now join boundary,
metering-sidecar and native-identity failures in the separate observer outcome.
`Server.Close()` still fails for Strict divergence, incomplete sessions and required
Tape output errors; the CLI logs `observer_status=failed` independently.
[Five Go gates and three actual CLI cases](observability/VALIDATION_AND_REPRODUCE.md#r05-observer-separation)
verify this separation, not cloud execution or bounded filesystem calls.
R01 now also has a [real overlapping background-call PoC](observability/VALIDATION_AND_REPRODUCE.md#r01-live-overlap): two identical commands remain exactly linked when completing in reverse order.
[R07 real SDK checks](observability/VALIDATION_AND_REPRODUCE.md#r07-real-sdk) passed ten old/new semantic pairs (20 executions). Timeout/kill removed the wrapper while its child remained alive in four observations; full-tree termination is still unproven. Both experiments removed their one owned Sandbox.
The direct SDK probe bypasses ARIES' cgroup containment. The current
[A disconnect and B Stop-failure checks](observability/VALIDATION_AND_REPRODUCE.md#r07-current-isolation)
confirmed A group cleanup and B refusal to evaluate, with two Sandboxes independently absent.
B retains its expected task cleanup failure even though final whole-Sandbox removal succeeded.
[Two controlled fsync delays](observability/VALIDATION_AND_REPRODUCE.md#r05-fsync-containment)
also confirmed task Sandboxes were already absent; the model finalizer still held up one shared
network, while Recorder finalization followed shared cleanup. Both runs remain interrupted with
incomplete observations, not successful replay samples. Production bounded filesystem shutdown
and L1 synchronous sampling hangs remain unresolved.

Four L0 traces also passed native TraceProcessor import: 55 intervals, 43 flows
and 19 verified sources each, with 0 ns imported start/duration/end error and no
processor errors. The four baseline exports were skipped because observation was
disabled. Raw files and incomplete coverage remain unchanged; UI interaction for
this matrix is UNRUN. Evidence:
`/opt/aries-artifacts/observability-r15-paired-perfetto-1/evidence.json`, SHA256
`1b4b992f912783f05b9927b4134676f80a1e27cf5b3b30c7e70a44b05e4c826b`.
The earlier paired-1 entry was preflight-only (0 creates, eight NOT_RUN), not
a failed or repeated cloud task; preflight and driver-fixture failures remain
in the [validation archive](observability/VALIDATION_AND_REPRODUCE.md#r15-paired-validation).

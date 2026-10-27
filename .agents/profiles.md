# Profiles and credentials

Start from the relevant checked-in profile; consult the [original schema](../pkg/config/config.go),
[cloud schema](../pkg/config/cloud.go),
[configuration reference](../docs/configuration.md), and [support matrix](../docs/supported.md).
Use `configs/versions.json` as the shared version catalog for cloud and E2B templates,
with verified image digests, never placeholders; do not duplicate the field inventory here.

- Keep role selection separate from placement. Original `aries PROFILE` Docker
  runs share one local daemon, run network, and managed bridge; Kubernetes runs
  share one namespace/backend. Reject unsupported mixed backends and legacy
  `bridge.mode=embedded` before effects. `aries cloud` is a separate bounded
  composition: OpenClaw or JiuwenSwarm `external|embedded`, with Jiuwen limited
  to codingcase and explicit `e2b-cloud` or `huawei-agentsphere`, not a relaxation
  of the original deployment schema. Jiuwen uses pinned native source/lock
  identity and an independently pinned Agent image for external placement.
  Register all pinned native built-in skills by default through Jiuwen's native
  shared skills directory, using the same fixed read-only paths in A and B.
  Registration does not establish external credential or runtime-dependency
  readiness. Reject OpenClaw-only options and unsupported generation overrides. Track implementation
  and real-cloud acceptance separately in the support matrix.
  Huawei uses its own SDK, adapter, credentials and persistent state; never
  reuse or reset E2B state or silently switch a profile's provider.
  Original TerminalBench cloud runs stay external and single-task; require a
  task-template binding matching the selected provider. An omitted binding
  provider means E2B; Huawei must be explicit. Preserve original task budgets
  and same-Sandbox private evaluation after confirmed Stop/revocation.
  Huawei verifier egress assumptions require explicit configuration, default to
  disabled, and must appear in run evidence; reject this option for other providers.
- Keep [TerminalBench instruction variants](../docs/configuration.md#terminalbench-instruction-variants)
  opt-in. Preserve the clean original checkout, environment, budgets and private
  evaluator; verify the public appendix before use. Give variants separate logical
  IDs, profiles, recordings and statistics while retaining the original template
  binding identity. Omission preserves the original task; do not rewrite old tapes.
- Select cloud coding workloads by explicit version. Without a `benchmark` selection, omitted `workload` defaults to
  `merge-ranges` and preserves its original behavior. `merge-ranges-portable-v1`
  changes only the task instruction supplied before execution, not `buggySource`,
  public/hidden tests or verifier semantics. `llm-development-pptx-v1` preserves
  the exact prompt `幻灯片 生成一份关于大模型发展的pptx材料，写5页。`, uses a fresh
  independent structural verifier for a readable five-slide PPTX with nonempty
  slide bodies, and retains all exported decks, including failed intermediates.
  See [versioned cloud workloads](../docs/configuration.md#versioned-cloud-workloads)
  for artifact and verification scope. Keep existing tapes immutable; no
  output normalization or replay fallback. A new workload version needs a fresh
  real recording and cross-cloud acceptance using the same tape before comparison.
- Keep [experimental native observation and output contracts](../docs/configuration.md#experimental-native-observation-and-output)
  opt-in and pin derived images by SHA-256. Preserve default profiles and native
  version checks. Renderer selection must preserve native tool schemas, parameters,
  `process`, background execution and auto-yield. A changed ToolResult requires a
  new real tape and full strict regression; never rewrite an old tape or relax
  matching. Foreground evidence alone cannot establish complete native behavior.
  `terminal-result-v1` requires explicit `harness.replay_tool_protocol`, external
  placement, SSH identity and `stdout-stderr-v1`; omission preserves native
  behavior. Keep its profiles, tapes and statistics separate from the baseline;
  follow the linked native contract rather than assuming cross-run byte equality.
- In original profiles declare `overrides_file` explicitly (`""` disables overrides). Keep harness and
  sandbox resource limits independent; preserve omission/default semantics and
  explicit model request settings without silently changing their meaning. Judges
  share model fields; preserve independent evaluation defaults and validation.
- Match model IDs to the served catalog. Keep credential values out of profiles,
  metadata, logs, and results; use configured environment-variable references.
- Use [scripted mock responses](../docs/configuration.md#scripted-mock-model) only
  for infrastructure checks. Preserve real verifiers, keep LLM judges independent
  of the mock, and do not interpret scripted scores or timing as LLM performance.
- Preserve original [DeepSeek credential-file validation and fallback](../docs/configuration.md#external-deepseek).
  Cloud uses named settings and an optional private literal env file; never
  evaluate it as shell, mutate process-global environment, or stage the whole
  file into children. Keep upstream keys out of Agent/Sandbox configuration.
- Bind cloud model sessions explicitly by per-attempt bearer token at `/v1`.
  Default replay to `strict`, comparing the complete request. Explicit
  `model.replay_policy=workload` permits only string `content` differences between
  corresponding tool messages; all other complete-history fields, IDs, arguments
  and message order stay exact. Preserve placement/version checks, original Tapes,
  real tool results and independent verification; no automatic policy fallback,
  prompt-based routing, silent mapping or scripted mock substituted for a real
  trajectory. Label policies separately; model-session success does not establish
  tool execution, verifier success or a comparable performance sample.
  Apply an explicit `tool_call_id_dialect=openclaw-2026.7.1` only to recording
  and tape-load causal checks; keep raw requests/SSE and Strict full-request
  matching unchanged. Bind Jiuwen's request-tail history dialect to its pinned
  source and dependency lock; recognize its changing request-only attachment
  separately from persistent history, retaining every field for Strict replay.
  The call-goal-v2 declaration applies the pinned native top-level display-field
  removal only when checking response-to-history causality; retain original
  requests/SSE and exact execution arguments. Preserve earlier request-tail and
  fixed-tail declarations' meanings and other dialects.
- For managed SGLang, validate endpoint and measurement settings using full option
  names; reject aliases and abbreviations that bypass those checks, keys containing
  `=`, and scalar values or list items starting with `-` unless numeric. Pass other
  native options through for SGLang to validate.
- Remove model credentials from managed SGLang's child environment. Use HTTPS for
  remote/credentialed endpoints; local HTTP uses a non-secret placeholder. URL
  validation does not enforce this [transport policy](../docs/implementation/model-runtime.md#implementation-gap-http-transport-policy).
- `aries setup PROFILE` only prepares pinned data/images; it must not contact
  model endpoints or start a model runtime. `aries PROFILE` runs the original
  experiment; cloud has its own command and no implicit `setup` equivalent.
- Default newly created and current-run cloud Sandbox comparison templates to
  **4 vCPU / 8192 MiB**, consistently on E2B and Huawei. Preserve the original
  resource values in historical templates, existing profiles, Tapes and reports;
  never backfill the new default into past evidence. Keep cloud admission/TTL
  limits explicit. Preserve the adapter's persistent
  ownership ledger and cumulative creation count across restarts; the count is
  for accounting, not a creation limit. Delete owned E2B sandboxes after use by
  default and confirm absence; resolve unconfirmed cleanup before creating more.
- Follow [ECS deployment](../deploy/e2b/README.md); builds, formatting, tests,
  deployment, and experiments run only on the designated remote Linux ECS.
  Archive runtime images and E2B template recipes, IDs, and evidence on that ECS.

`profiles/experiments/` holds the paper-reproduction profiles; their settings are
documented in [configuration](../docs/configuration.md#paper-reproduction-profiles).

Human references: [configuration](../docs/configuration.md),
[quick start](../docs/quick-start.md),
[runtime](../docs/design/runtime.md), [support matrix](../docs/supported.md).

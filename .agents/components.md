# Component contracts

Follow [Runner interfaces](../pkg/runner/interfaces.go), [design principles](design.md),
and [lifecycle](architecture.md#task-lifecycle).

| Role | Required boundary |
| --- | --- |
| Benchmark | Own tasks, sanitization, private verifiers, and independent upstream evaluation. |
| AgentHarness | Own the agent runtime, model interaction, and private evidence; retain distinct telemetry entries. |
| ToolSandbox | Keep the task environment live through evaluation; create fresh Runner-owned evaluation sandboxes from it on request. |
| ToolBridge | Grant temporary access to one exact sandbox; confirm revocation. |

- Keep implementations independent; a paired bridge may consume a narrow sandbox
  capability. Pair original Hermes/OpenClaw with their corresponding SSH bridges;
  cloud OpenClaw uses the matching external or embedded bridge bound to the exact
  Runner-owned sandbox from the selected backend. Jiuwen embedded placement uses
  its paired bridge and native LOCAL tools inside that same owned sandbox. External
  Jiuwen runs the Agent in Docker and uses its paired remote bridge through the
  native SANDBOX provider interface to that exact Runner-owned sandbox; native
  JiuwenBox is not an additional sandbox owner. The harness owns its Agent runtime,
  while the bridge owns and revokes only guest execution access.
  Configure harness-generated per-user state (such as default `HOME`) outside
  the task workdir; preserve explicit native per-call overrides.
- Preserve shell bodies and native protocol semantics; adapt provider-owned path
  arguments or filesystem mappings without rewriting agent command text. Honor
  benchmark-defined workdirs; resolve unspecified workdirs from the running image
  before exposing access. Never retry ambiguous
  submissions, widen accepted payloads to hide failures, or enable Hermes credential sync.
- Task/evaluation handles borrow run connectivity. Releasing one must not remove
  the network or close peers' transports. Do not require a private network per pair.
- Reuse shared harness ownership and credential mechanics. Preserve native defaults
  and credential fallbacks; OpenClaw bundled plugins use the pinned image's compiled
  packages with source overlays disabled consistently across placements.
  Retain ownership after failed removal. `Stop` prevents admission while preserving
  `Run` snapshots. Redacted errors must not expose secrets through their causes.
- Each original SSH sandbox session has its own listener, immutable target binding, and recorder,
  keyed by a stable sandbox ID. Native tool execution must not call Runner or own
  sandbox lifecycle. Use plaintext unauthenticated gRPC and unauthenticated SSH,
  one in-memory service host key, and no client-key staging; preserve
  target/ownership checks. Cloud external SSH also stays on the explicit private
  path; public model HTTPS uses separate per-attempt bearer authentication.
- Retain tool-operation deadlines and bounded evidence collection. Revocation
  closes access, joins handlers, and finalizes evidence; keep replay inputs private.
  Tool errors alone must not fail revocation. Harness completion is authoritative
  for completed calls; preserve the task environment for independent evaluation.
  Evidence failures are separate observer outcomes; do not rewrite native tool or
  Harness execution outcomes merely because collection failed. Isolation and
  cleanup failures retain their operational meaning.
  Apply the [cancellation/cleanup distinction](design.md).
- Cloud isolation additionally requires closing admission, confirmed termination
  of the contained Agent process group, and joined local calls before verifier
  exposure. Missing cgroup capabilities are unsupported; transport cancellation,
  Gateway health, TTL, and a no-op stop are not confirmation. Preserve provider
  ownership until absence is positively established, including after restart.
  Jiuwen HTTP/SSE completion and interrupt acknowledgements do not prove process
  termination. Native graceful shutdown may drain observation, but must still be
  followed by confirmed group termination and joined calls. Keep per-attempt native state outside the task workdir, and preserve
  native parallel/background tools within the contained runtime.
- By default, reject network policies the selected backend cannot enforce before
  allocating that sandbox. Only explicitly configured Huawei verifier egress
  assumptions may bypass this rejection; preserve the no-egress request and
  expose the assumption separately from enforcement evidence. Do not silently
  permit network access or skip independent verification to make a run pass.
- Preserve benchmark pins, candidate sanitization, verifier privacy, required checks,
  and score meaning. Follow upstream evaluation, including fresh-sandbox methods
  where required; see [benchmark contracts](../docs/design/benchmark.md).
- Keep DRB's [harness task adaptation](../docs/benchmarks/deep-research-bench.md#harness-task-prompt)
  separate from its unchanged dataset question supplied to the judge; record prompt
  changes as experimental method changes.

Details: [harness](../docs/design/harness.md), [bridge](../docs/design/bridge.md),
[sandbox](../docs/design/sandbox.md), [deployment](../docs/design/deployment.md),
[implementations](../docs/implementation/README.md).

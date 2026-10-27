# Architecture

| Ownership | Location |
| --- | --- |
| CLI and implementation selection | `cmd/aries` |
| Profiles, preparation, scheduling, results | `internal/app` |
| Construction, configuration translation, rollback | `internal/app/wiring/<role>` |
| Four role interfaces and task lifecycle | `pkg/runner` |
| Shared data | `pkg/core` |
| Timing records, versioned analysis snapshot and derived exporters | `pkg/performance` |
| Model settings and Chat Completions transport | `pkg/model` |
| Strict model recording/replay and explicit session handler | `internal/modelreplay` |
| Private literal configuration loading | `internal/envfile` |
| Concrete components | `pkg/{benchmark,harness,bridge,sandbox}` |
| Deployment contracts and providers | `pkg/deployment` |

- `cmd` injects `app.Wiring` or `app.CloudWiring`; `internal/app` must not import wiring packages.
- Controllers depend on deployment contracts, not providers. Wiring supplies
  provider choices/defaults, launch settings, and runtime metadata. Keep bridge
  placement and borrowed sandbox execution as separate dependencies. The shared
  deployment package holds contracts; providers own backend-specific operations.
- Deployment owns runtime operations, attachment, publication, and endpoint
  resolution for the consuming process. Forward placement handles opaquely.
  Protocol code must not derive addresses or assume host loopback is reachable.
  See [endpoint handoff](../docs/design/deployment.md#endpoint-handoff).
- Shared harness, bridge, and SSH lifecycle must not import native harnesses,
  dialects, or clients. Wiring injects factories and endpoint/client-staging policy.
  Dialects own grammar/workspace rules; harnesses own native config, readiness,
  protocols, and results. The OpenClaw forwarding client must not import server grammar.
  Embedded harnesses reuse `harness/internal/sandboxprocess` for admission,
  contained execution and confirmed termination; keep native launch configuration
  and protocols in their concrete harness packages.
- Use explicit dependencies, concrete helpers, `context.Context` for external
  work, and Logrus lifecycle logs. Reuse native APIs and standard/existing
  libraries; add dependencies only for missing capabilities, rather than writing
  generic protocols. Compatibility paths need supported versions and actual
  callers. Use Moby for Docker.

## Task lifecycle

Load → start/sanitize sandbox → register bridge → start/run harness → confirm
harness stop → confirm bridge revocation → evaluate → remove evaluation sandboxes
→ remove task sandbox → release attachment. Record every task failure in
`TaskResult.Error`.
Apply explicitly declared initial filesystem metadata after sanitization and
bridge filesystem setup, before Harness startup; preserve requested and actual
metadata evidence without normalizing tool results or execution clocks.

For original profiles the app owns one shared bridge and network per run;
Runner receives per-sandbox sessions. Cloud external placement owns a run network
and per-attempt bridges; embedded placement creates no Docker Agent runtime.
Cloud model handlers surround the same four-role Runner, with one explicit
session per attempt; they are not another Runner role. Close new model-request
admission when the actual `Harness.Run` returns; preserve admitted requests and
their failures, and observe late rejections through teardown. Admission closure
does not replace confirmed harness stop or bridge revocation. Final model-session
close drains handlers and seals evidence at the attempt cleanup boundary.
Model recording validates the complete upstream response before flushing its
terminal DONE. After every frame has flushed successfully, normal client closure
does not revoke that request's success; premature cancellation and transport
errors still fail. Keep receipt and flush clocks distinct; see
[model completion](../docs/implementation/model-replay.md#终帧交付与正常断开).
Start shared infrastructure
after model preflight, before task admission. After task cleanup, remove owned
shared infrastructure, finalize observation, and close transports before persisting
results. Record shared and model-session outcomes separately from task counts.

The app owns the performance recorder and finalizes it after task and shared
cleanup, before persisting results. Each process owns its event stream and clock
domain. Recording failures remain separate from task outcomes; preserve existing
result-field meanings. See [performance artifacts](../docs/run-results.md#performance-records-and-offline-summaries).
All trace/metric exporters consume the validated snapshot, without rereading or
independently correlating evidence. Keep unaligned clocks separate and missing
identities explicit. See [measurement contracts](../docs/observability/OBSERVABILITY_SCHEMA_AND_MEASUREMENT.md).

Load the selected catalog once per run; keep shared metadata immutable and
verifier inputs benchmark-private. Give every occurrence fresh mutable benchmark
state and runtime owners, including repeated task IDs. Runner explicitly revokes
sessions; use fresh bounded cleanup contexts after cancellation. No assignment
leases or controller-liveness supervision.

Details: [design](../docs/design.md), [contracts](components.md).

# OpenClaw cloud placement slice

This package supplies two components for the existing Runner. It does not create
or delete sandboxes, select templates, start a model service, or replace the
benchmark verifier. Cloud integration and native OpenClaw execution remain
unverified until the remote ECS and E2B checks have run.

## External placement (A)

`NewExternalBridge(ExternalBridgeOptions)` returns a `runner.ToolBridge`.
Its `Start` receives the exact Runner-owned sandbox, reuses the existing OpenClaw
SSH dialect and audit recorder, and sends binary command streams through that
sandbox's `runner.StreamExecutor` capability. Use the existing OpenClaw container
harness with the returned endpoint; its Linux forwarding executable still needs
to be supplied as `ClientSourceFile`.

An explicit task-template directory-mtime adaptation runs after process-group
and workspace-alias preparation, before the endpoint is returned to the Harness.
It updates only declared real directories, preserves atime, and retains actual
before/after metadata in `initial-directory-mtimes.json`; a failed update or
readback fails preparation. It does not change tool output, clocks, or later
Agent writes. Bindings without this adaptation keep their existing behavior.

Cloud wiring enables the container harness's optional `ExpectedVersion` check.
Before readiness/admission it executes `/usr/local/bin/openclaw --version` and
`/usr/local/bin/node --version` in the actual container, retains private
`native-version.json`, and rolls back on a missing executable or mismatched
OpenClaw version. Node's observed version is recorded, not inferred from an image
tag. Ordinary profiles leave this option empty and retain their existing behavior.

The SSH listener accepts only an explicit loopback/private bind IP. The external
harness must be able to reach that interface. In particular, Docker container
loopback is not the ECS host's loopback. Wiring must supply the actual private
address and routing; this package does not infer host/container addresses. The
existing SSH dialect is unauthenticated and must stay on this private path.

## Embedded placement (B)

`NewEmbedded(EmbeddedOptions)` returns a `runner.AgentHarness`; use its `Bridge()`
as the matching `runner.ToolBridge` for the same attempt. The bridge binds the
sandbox before the harness starts. The native Gateway and tools execute inside
that sandbox, while the Go controller stays outside.

Required construction inputs are the workspace, private evidence directory,
absolute OpenClaw executable, exact expected native version, external HTTPS
model-proxy URL, scoped proxy access token, and a provider guest-endpoint resolver.
The resolver should adapt `e2b.Sandbox.GuestEndpoint`, carrying its access headers
only in the WebSocket handshake. Raw guest endpoints/credentials are not evidence.

The renderer reuses the existing model/Gateway configuration and selects native
`sandbox.mode=off`, `exec.host=gateway`, and an explicit task workspace. Both
placements initially expose exec and process, retaining the existing file-tool
and subagent deny list. Search, MCP services, voice, and no-tools tasks are not
implemented by this slice. Model routing goes to the external proxy. The guest
receives `ARIES_MODEL_PROXY_ACCESS_TOKEN`; no DeepSeek API-key environment is
looked up or staged by this package.

## Native workload inputs

Cloud wiring passes the same optional `openclaw.WorkloadInputs` to A and B:
session key `agent:main:aries-workload`, session UUID
`8ad5eb11-e541-4cbf-8ae0-1f9e2822c342`, and native
`agents.defaults.envelopeTimestamp="off"`. These inputs are set before both
recording and replay. The Gateway's optional `sessionId` is forwarded unchanged;
no model request or history is rewritten. Ordinary profiles leave this option nil.

Every attempt owns a fresh Gateway and private state directory. Native fixture
IDs can therefore repeat without sharing runtime state; ARIES attempt IDs,
model authentication tokens and Gateway idempotency keys remain independent.
`agent-result.json` records `requested_native_session_id` as request evidence.
A additionally requests native container hostname `aries-workload`. B's actual
hostname and ability to set it remain unverified on E2B; this package does not
change guest hostname or capabilities. These settings remove known input
differences in code, not prove a successful strict replay. Old tapes require
recording again; remote tests and real native request comparison remain required.

## Template and shutdown requirements

Both paths currently require Linux, Python 3, root controller execution, a
dedicated `aries-agent` UID 60000, a task workspace separate from native/control
files, and writable root-owned cgroup v2 supporting `cgroup.kill`. The UID must
have no processes before admission. Template preparation must create that account
and, for B, install the explicitly pinned OpenClaw/Node runtime with root-owned
non-writable installation directories. A generic E2B template is not assumed to
satisfy these requirements. Resource sizes and template publication remain the
E2B adapter/wiring's responsibility.

The controller creates one private cgroup and a root-owned admission lock. Every
agent command enters the group before dropping to UID 60000 with `no_new_privs`.
For B the Gateway and descendants inherit this containment. Stop atomically
closes admission, kills the group, and requires `cgroup.events` to report
`populated=0`. It also joins local calls. Timeouts or missing confirmation return
an error; transport cancellation and an unhealthy Gateway do not prove isolation.
The task sandbox/workspace remains alive for the existing independent verifier.
Missing containment capabilities return `ErrUnsupported`; there is no weaker
fallback or no-op bridge. A snapshot/destroy/fresh-verifier alternative is not
implemented.

## Evidence and validation limits

The existing and embedded OpenClaw harnesses expose the controller-observed
native Agent invocation window separately from total `HarnessResult.Duration`.
That window includes Gateway RPC transport/waiting and is not guest CPU time.
Failed/canceled native invocations retain the window; failures before invocation
leave it absent. Native session keys and Gateway run IDs remain distinct from
ARIES occurrence IDs.

B stores redacted native configuration, a guest-observed PID/UID/executable
snapshot, the correlated native result, and a bounded redacted Gateway log.
The process snapshot shows where observed processes were running; it is not a
guest timing sample. A reuses the existing SSH tool audit, whose command execution
window includes bridge/transport overhead and is not a native tool-call duration.

B advertises the native `tool-events` Gateway capability and continuously drains
events during `Agent`. Private `native-tool-events.jsonl` contains only matching
`sessionKey` and the acknowledged native `runId`; it never substitutes an ARIES
occurrence or idempotency key. The companion index records correlation, terminal
coverage, foreign discards and incomplete observations. Tool `start`/`result`
records retain the native tool-call ID and sanitized command when supplied;
`command_output` retains actual `exitCode`/`durationMs` only when present. Full
results, environment maps and output bodies are not retained. Known credentials
are redacted before persistence. Missing fields are JSON null, not zero.

Host receipt offsets use the controller's monotonic clock. Native `ts` and
`durationMs` are guest wall-clock observations; no host/guest clock subtraction or
monotonic guest claim is made. A tool result, including `phase=end`, does not
prove a background process exited: `process_completion` remains `unknown`.
No tool events means `no_tool_events_observed`, not a measured zero tool count.
Index `complete=true` describes integrity of the received event stream, not proof
that every model-requested tool was covered. Formal coding-case acceptance must
observe real tool events and reconcile native tool-call IDs with the model's
`tool_call_id` history; checking `complete` alone is insufficient. This package
does not yet automate that cross-channel reconciliation.
The collector retains at most 4096 matching-session events / 4 MiB and commands
up to 64 KiB, then reports incomplete evidence. It waits at most two seconds for
a correlated lifecycle terminal after the RPC window; drain and artifact time
are outside `RuntimeExecutionDuration`. Missing terminal/pairs, regressed event
sequence, queue/collector overflow, stream and persistence failures are visible
errors. Transport-close failure remains visible across shutdown retries while
containment cleanup continues.

The source contract was checked against pinned OpenClaw v2026.7.1:
[capability](https://github.com/openclaw/openclaw/blob/v2026.7.1/packages/gateway-protocol/src/client-info.ts),
[recipient registration](https://github.com/openclaw/openclaw/blob/v2026.7.1/src/gateway/server-methods/agent.ts),
[native tool events](https://github.com/openclaw/openclaw/blob/v2026.7.1/src/agents/embedded-agent-subscribe.handlers.tools.ts),
[event timestamp assignment](https://github.com/openclaw/openclaw/blob/v2026.7.1/src/infra/agent-events.ts).
Real wire compatibility, full native trajectory export, Agent-versus-tool resource
attribution, cross-placement aggregate timing comparability, and positive remote
E2B end-to-end validation remain **UNRUN**. SDK execution observations are a
separate channel and must not be counted again as native tool calls.

Run formatting, compilation, tests, deployment, and cloud experiments only on
the user-provided remote ECS. The new tests use fake sandbox/Gateway boundaries
and an actual local SSH handshake on that test host; they do not call E2B or a
model API. They cover binary transport, strict containment failure, cancellation,
shutdown retry, proxy-credential separation, native timing, and private listener
requirements, plus native event correlation/coverage/overflow and actual version
pin failure. The Python test parses guest scripts without executing them.

```sh
gofmt -w pkg/harness/openclawcloud pkg/harness/openclaw/cloud_config.go pkg/harness/openclaw/cloud_config_test.go pkg/harness/openclaw/runtime_timing_test.go pkg/harness/openclaw/native_version.go pkg/harness/openclaw/native_version_test.go pkg/harness/openclaw/harness.go pkg/harness/openclaw/harness_test.go pkg/harness/openclaw/gateway
GOCACHE="$PWD/.cache/go-build" GOMODCACHE="$PWD/.cache/go-mod" TMPDIR="$PWD/.cache/test-tmp" go test -race ./pkg/harness/openclawcloud ./pkg/harness/openclaw/...
```

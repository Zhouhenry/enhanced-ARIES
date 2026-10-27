# E2B SDK adapter

This is an internal, authenticated adapter for the **official Python `e2b==2.58.0` SDK**, published October 20, 2026. It is used by `pkg/sandbox/e2b`; it is not an E2B-compatible public service or a Go SDK. Python 3.11+ is required. `constraints.txt` fixes the dependency versions resolved for this implementation; the ECS container verified Python 3.12.15, SDK 2.58.0, and `pip check`. Run compilation, tests, deployment, and cloud experiments **on the remote ECS only**.

The adapter owns cloud sandbox allocation and cleanup. Go owns each attempt and its handle. E2B remains remote; the adapter's container is not the tested sandbox. External and Embedded OpenClaw can share this data plane, but this package alone does not establish either mode's isolation or end-to-end acceptance.

The [full workload baseline](../../deploy/e2b/README.md#后续真实-ab-验收) has now passed all five conditions once. The smoke and image checks described below retain their narrower scope; lifecycle faults and concurrent cloud workloads still require separate acceptance.

## Remote ECS commands

From the repository root on ECS:

```sh
python3 -m venv adapters/e2b/.venv
adapters/e2b/.venv/bin/python -m pip install -r adapters/e2b/requirements.txt
adapters/e2b/.venv/bin/python -m unittest discover -s adapters/e2b -p 'test_*.py' -v
gofmt -w pkg/sandbox/e2b/*.go
gofmt -w internal/app/wiring/cloud/*.go
GOCACHE=$PWD/.cache/go-build GOMODCACHE=$PWD/.cache/go-mod go test -race ./pkg/sandbox/e2b ./internal/app/wiring/cloud
```

Unit tests use a fake SDK, local HTTP, and a local subprocess to check binary framing. `test_sdk_contract.py` imports the installed SDK and checks its public signatures without contacting E2B. Neither test type establishes cloud behavior.

Supply `E2B_API_KEY`, `E2B_TEMPLATE_ID`, and `ARIES_E2B_ADAPTER_TOKEN` through the deployment environment/secret manager. The token must have at least 32 characters. The repository's private `.env` is configuration input; do not `source`/`eval` arbitrary dotenv files, print values, or pass secrets as CLI arguments. `server.py` does not automatically parse `.env`.

```sh
adapters/e2b/.venv/bin/python adapters/e2b/server.py \
  --state-file /private/aries/e2b-adapter.state.json \
  --max-sandboxes 4 --max-timeout 600
```

The default listener is `127.0.0.1:8787`; Go uses `ARIES_E2B_ADAPTER_URL=http://127.0.0.1:8787`. For containers, use the private Compose network and an explicitly reachable adapter address. Never publish the API unauthenticated or expose it on the public Internet. HTTP is intended for loopback or a controlled private network; use TLS at the boundary if traffic crosses it. The SDK key stays in the adapter. Private guest endpoint responses contain a traffic token: transport only, never evidence/logs.

Container image, built **on ECS**, using this directory as build context:

```sh
docker build -t aries-e2b-adapter:local adapters/e2b
```

Mount a persistent, private, writable volume at `/state`; preserve ownership for UID 10001. Do not run concurrent adapter instances on the same state file. The file lock rejects a second owner. The refreshed image includes the current `smoke.py` and is archived at `/opt/aries-artifacts/aries-e2b-adapter-refresh-1/` (October 22, 2026, 16:43:57 UTC). UID/GID 10001, SDK 2.58.0, `pip check`, three source hashes, smoke help and same-daemon load identity passed; the check container is absent. No cloud/model calls or service startup occurred. The earlier image's 18 Adapter/SDK plus 5 template tests were not rerun (`server.py` is unchanged). Separate post-finalize tests passed all 8 cases and native Docker positive/negative probes. Full service-path acceptance remains unverified.

## Ownership and limits

`--template` may repeat to explicitly allow templates; otherwise the adapter reads `E2B_TEMPLATE_ID`. The adapter does not build templates. Templates must already contain `python3`, the task dependencies, and any Embedded OpenClaw runtime. Go requires explicit benchmark-image-to-template mappings and an absolute workdir. CPU/memory/disk/GPU overrides, global task environment variables, and automatic MCP/search service discovery are currently rejected before cloud allocation; resources must be configured in the template.

Defaults are two live/uncertain sandboxes, eight active/uncertain processes per sandbox, a 900-second maximum requested TTL, and 30-second SDK request timeouts. Cumulative creates count attempts, including failed/uncertain ones, for accounting only; there is no cumulative creation limit. Deleting a sandbox or restarting the adapter preserves this count. Provider errors never trigger automatic resubmission. A cloud quota rejection is returned as a safe allocation failure; the provider's own quotas still apply.

All sandboxes request `on_timeout=kill`, `auto_resume=False`, and private ingress. Outbound Internet access defaults off and follows the explicit task request. SDK/connection failures during create retain an `allocation_unconfirmed` record and increment the creation count. An unknown-ID allocation requires operator reconciliation using the private ledger and E2B metadata `aries_adapter_handle`; automatic retries would risk duplicate paid resources.

The private 0600 state file is written before create and after receiving the cloud ID. On restart, all known remaining cloud IDs are attempted before reporting unresolved entries; an unknown-ID pending allocation or failed deletion blocks startup. A failed deletion retains ownership. Only control-plane `Sandbox.get_info()` raising `SandboxNotFoundException` yields `absence_confirmed=true`; envd health failure is insufficient. Graceful shutdown closes admission and drains accepted HTTP requests before final cleanup. A creation completing during shutdown immediately enters the same deletion path. Cleanup fails if any absence is unresolved. TTL is a backstop, not proof of cleanup. State files contain cloud IDs but no SDK/API/traffic credentials.

Use immediate deletion after exporting needed evidence; do not leave instances running to preserve a template or image archive. Unknown allocation or failed cleanup must be reconciled before another allocation. Live-sandbox/TTL limits are not a dollar spending limit and do not read account balance. Preserve ownership ledgers and account for template builds and separate template/SDK probes. See the [ECS cost and archive procedure](../../deploy/e2b/README.md#费用与残留处理). ECS automated tests passed; real template verification confirmed deletion, but that separate probe does not establish the adapter's cloud shutdown behavior.

A normal deletion in progress still occupies one sandbox slot but allows another creation when capacity remains. Once any deletion request fails to confirm absence, overlapping requests and retries cannot reopen admission for new sandboxes. The failure remains blocking until control-plane absence is confirmed; restarting cannot restore an in-flight permission.

## Internal protocol and semantics

Every route is authenticated `POST /v1/<operation>`, JSON, `Authorization: Bearer ...`. A caller supplies a fresh `sandbox` handle before `create`; duplicates are rejected, never retried.

The internal command wrapper starts with SDK `cwd=/`, so it does not depend on a
default home directory existing in the image. The child command still receives
the caller's exact `argv`, task `cwd` and environment.

Supported operations:

| Operation | Request additions | Result |
| --- | --- | --- |
| `health` | empty object | schema, SDK version, shutdown state |
| `create` | `template`, `timeout_seconds`, `allow_internet_access` | handle, hashed cloud ID, SDK version, template |
| `status`, `kill` | sandbox handle | ledger state / confirmed absence |
| `start` | fresh `process`, `argv`, absolute `cwd`, `env`, `user`, timeout/output limit | managed PID and process handle |
| `events` | process, `cursor` | up to 32 output/terminal records, next cursor, done; at most 10-second long poll |
| `stdin` | process, `data_base64`, `close` | acknowledged bytes/EOF |
| `process`, `kill_process` | process | SDK-managed PID status; descendant absence remains unknown |
| `upload`, `download` | absolute `path`, base64 data / `max_bytes` | size / base64 bytes |
| `guest` | `port` | HTTPS URL and private traffic-token header |
| `metrics` | sandbox handle | E2B timestamped metrics and explicit unsupported fields |

Command stdout/stderr from the SDK are UTF-8 text, so directly forwarding them would corrupt arbitrary bytes. `guest_exec.py` starts the **exact argv** using `subprocess.Popen`, forwards stdin including EOF, and frames raw stdout/stderr as base64. The adapter drains the SDK stream and Go writes decoded bytes to independent writers. It does not rewrite agent shell commands. A negative child exit code retains Python's signal-exit convention. The sandbox requires Python 3; this extra process and framing are part of the measurement overhead and must be reported.

Files are bounded to 64 MiB per upload or download in both E2B and Huawei. The Adapter HTTP request and Go response bounds are 96 MiB to accommodate base64 and JSON metadata. The streamed SDK read enforces the requested download bound while reading, not only through a potentially stale stat. A command defaults to 16 MiB output, maximum 64 MiB, with a 65,536-frame bound and 1,024 managed-command history entries per sandbox. All retained command payloads share a 128 MiB adapter budget, released after confirmed sandbox removal. Output overflow is an explicit failed stream, not a successful truncated command. Input writes are bounded to 1 MiB each. The HTTP server caps concurrent request handlers at 64.

Go `ExecStream` forwards stdin concurrently. As with other borrowed execution transports, cancellation stops client waiting and does not prove guest termination. A caller providing a permanently blocking stdin reader must unblock that reader when its transport closes; the adapter cannot interrupt arbitrary Go `io.Reader` implementations. `Process.Kill` uses the SDK's managed-command signal and list API; **it does not prove all descendants are gone**. Embedded isolation must be confirmed separately, or export a trusted immutable evidence copy before destroying the entire sandbox and independently evaluating that copy.

`core.CommandResult.Duration` retains its existing client wall-time meaning. The terminal frame additionally carries `guest_duration_ns`, measured with Python `time.monotonic_ns()` from immediately before `Popen` until **both the direct-child wait watcher and stdout/stderr pipe draining have completed**. One watcher calls `child.wait()` exactly once, independently of the selector loop that reads both pipes. Pipe EOF can precede or follow the observed child exit; a descendant retaining a pipe can extend the envelope after the direct child has exited.

Optional boundaries share that guest-local anchor: `popen_return_ns` is sampled immediately after `Popen` returns; `first_output_ns` is the first nonempty read from **either** stdout or stderr, and stays null when neither produces data; `child_exit_observed_ns` is sampled after the watcher's `wait()` returns; `pipe_eof_ns` is sampled after both pipes reach EOF. Separate first-stdout/first-stderr times are not collected. These are wrapper observation boundaries, not kernel event timestamps, CPU time, or an exclusive tool duration. The adapter rejects negative or out-of-envelope values and impossible ordering while allowing either ordering of child exit and pipe EOF. Valid boundaries pass through unchanged. `adapter_duration_ns` is a separate adapter-clock interval; no measurement subtracts clocks on different machines.

The ECS old/new-wrapper probe confirmed that terminating the wrapper can leave its direct child present. Its disposable process-group cleanup is a probe mechanism, not a production termination guarantee. An interrupted or failed SDK stream reports `termination_confirmed=false` without an invented child exit or guest duration. Real Sandbox/SDK cancellation, descendant absence and B native-tool termination remain separate acceptance requirements; B native tools do not traverse this wrapper.

Metrics expose the SDK's CPU percentage, memory and disk capacity/usage observations, with its documented nominal five-second cadence. The real SDK smoke returned one such sample; this is not sampling-accuracy or short-peak validation. Missing early samples remain an empty list. Disk IO, network IO, and Agent/Tool attribution are explicitly unsupported; disk usage is not an IO counter.

Trusted Runner file transfers explicitly use SDK `user="root"`; the adapter token must never reach the agent. Agent tools use the contained command path instead. `e2b.Options.Connectivity` passes borrowed harness placement unchanged and never gives the E2B sandbox ownership of a Docker network.

Cloud wiring writes raw SDK snapshots to each occurrence's `sandbox-metrics.jsonl`, with task/evaluation purpose, opaque allocation handle, SDK timestamps and client RPC durations. Repeated provider rows remain raw snapshots and must not be treated as independent samples. `cloud-observation.json` keeps the SDK report separate from the external harness's existing Docker monitor. Empty guest metrics remain unavailable; observation failures do not cancel the workload or pretend cleanup failed. Final sampling occurs before sandbox deletion and is part of cleanup overhead. One fully verified coding attempt can allocate two sandboxes: one task sandbox and one independent verification sandbox.

## Operation observations (W3; cloud calibration UNRUN)

Each occurrence has private `e2b-operations.jsonl` and `e2b-operations-index.json` (0600 files under a 0700 directory). Existing artifacts are never reused or overwritten. The JSONL schema version is 1. Start/end pairs contain `sequence`, `operation_id`, `operation`, `run_id`, `task_id` (occurrence), optional `logical_task_id`, `purpose`, opaque `sandbox_handle`, and `started_at_utc`. End records add `client_duration_ns` from Go's monotonic clock and `status` (`succeeded`, `failed`, `canceled`, or `timeout`). This logs observed operations, including failures; successful measurement does not mean the command succeeded.

- `adapter_rpc.<action>` measures the Go client request through response decoding. Its optional `sdk_calls` list contains only allowlisted official SDK method names, adapter-local monotonic `duration_ns`, and success/failure. SDK file reads include consuming and closing the stream; command submission measures `commands.run`, not the background child's lifetime. An expected `sandbox.get_info` NotFound can be recorded as a failed SDK call while the enclosing kill RPC confirms successful removal. A lost HTTP response leaves SDK-side durations unavailable.
- `exec.wait` measures from the client's background submission until that call to `Wait` ends. It includes `stdout_bytes`, `stderr_bytes`, and, when a validated terminal arrived, `exit_code` and `guest_duration_ns`. An interrupted wait has no invented exit code or guest duration. Background submissions that are never waited on have only their RPC observations. These records are **not** OpenClaw native ToolCall counts; Embedded tool events have a separate collector.
- Successful file RPCs include `file_bytes`; incomplete transfers do not invent a complete byte count. Paths, command bodies, environment values, request headers, keys, guest endpoint credentials, and actual provider sandbox IDs are excluded.
- The create record includes the SDK distribution version read by the adapter's `importlib.metadata.version("e2b")`, its pinned version, the **requested** template, and requested TTL. The public create result exposes no resource allocation, region, resolved build ID, or cold/warm classification here: resource/region fields stay null and `cold_warm` stays `unknown`. Raw resource readings remain in the separate metrics artifact.

RPCs and `exec.wait` overlap, and SDK durations are nested in RPCs. They cannot be added or subtracted to estimate exclusive harness overhead. Serialization, file writes, binary framing and the wrapper process add observation overhead and require remote calibration. These files do not supply TTFB, first useful stdout, agent CPU, or independent samples.

The index starts `incomplete`. `Manager.FinishObservation()` runs after stopping metric sampling and the existing owner cleanup; only matched records with no remaining owned sandboxes or recording failures can become `complete`. It returns an observer result/error separately from cleanup. Interrupted processes, bounded-artifact overflow (64 MiB), write/flush failures and unfinished operations stay incomplete. Inspect the index counts against every JSONL pair and sequence; an absent/truncated index is incomplete. An `e2b-operations-index.json.incomplete` marker invalidates even an earlier complete index if a caller performs an operation after finalization or final index publication fails. A later cleanup retry cannot erase the original incomplete observation. `cloud-observation.json` exposes this report as `sandbox_operations`, independently of `sandbox_sdk` resource snapshots and `external_harness` measurements.

Tests cover binary terminal timing, retained descendant pipes, malformed/negative duration rejection, SDK failures without payload disclosure, JSONL pairing/privacy, transfer bytes, interrupted observations, disk-write failure, preserved cleanup, and finalization order. The ECS build, full unit/race tests, lint and Docker integration checks passed; integration had only 2 explicitly optional skips. These results do not establish real cloud timing accuracy or observation overhead, which remain uncalibrated.

## Opt-in cloud smoke

On ECS with environment secrets already injected:

```sh
adapters/e2b/.venv/bin/python adapters/e2b/smoke.py \
  --confirm-create --state-file /private/aries/e2b-smoke.state.json \
  > /private/aries/e2b-smoke-evidence.json
```

This creates **at most one sandbox**, with a 120-second TTL, tests binary command/stdin/exit and file transfer, starts one bounded background process, checks its SDK-managed status, requests metrics/private guest endpoint, and finally deletes the sandbox with absence confirmation. The evidence has a hashed cloud ID; raw IDs remain only in the private ownership ledger. A read-only probe reports Node/OpenClaw availability and cgroup/no-new-privileges observations. It does not alter cgroups or claim isolation acceptance. The probe's guest URL is resolved without exposing its credentials; Gateway connectivity remains `UNRUN` because no Gateway was started. The smoke is explicitly **not a formal benchmark attempt, real Agent run, DeepSeek recording, or independent verifier acceptance**.

Actual SDK smoke ran on October 22, 2026, 16:05:37–16:05:48 UTC: `PASS` with `cleanup_confirmed=true`. Binary file transfer, stdin/stdout/stderr, expected exit code 7, background process and private endpoint checks passed. Full Gateway/Agent lifecycle acceptance remains `UNRUN`.

The first published template created successfully using `template_id:build_id`, but its strict runtime probe returned `UNSUPPORTED`. The smoke localized writable trusted paths: `/usr/local`, `/usr/local/bin`, the Node binary and template probe were root:root with mode `0777`; `/app` was `0555`, `/run` was `0755`, and `/sys/fs/cgroup` was `0555`. That guest's hostname was `e2b.local`. Cgroup v2 was present, but that smoke did not test child-cgroup creation or `cgroup.kill`. Both completed probes confirmed sandbox deletion.

The corrected v2 cloud build and boot verification passed on October 22, 2026, 16:20:09–16:20:56 UTC: `verify_exit=0`, `runtime_status=PASS`, cgroup probe `PASS`, and `cleanup_confirmed=true`. The probe exercised child-cgroup creation, `cgroup.kill` and an empty-group confirmation; it did not start a real Gateway/Agent. At that checkpoint, service deployment and formal A/B acceptance had not run. Subsequent deployment and one real A task passed, including DeepSeek recording and independent verification; B, E2B-origin model access and real cloud strict replay remain `UNRUN`. See the [deployment evidence summary](../../deploy/e2b/README.md) for current archive and validation boundaries; these probes do not replace formal task lifecycle or verifier acceptance.

## Source verification

- [Official SDK release `2.58.0`](https://pypi.org/project/e2b/2.58.0/) and [official repository](https://github.com/e2b-dev/E2B/tree/main/packages/python-sdk).
- [Create/lifecycle methods](https://github.com/e2b-dev/E2B/blob/main/packages/python-sdk/e2b/sandbox_sync/main.py) and [public command API](https://github.com/e2b-dev/E2B/blob/main/packages/python-sdk/e2b/sandbox_sync/commands/command.py).
- [CommandHandle UTF-8 decoding and stdin](https://github.com/e2b-dev/E2B/blob/main/packages/python-sdk/e2b/sandbox_sync/commands/command_handle.py).
- [Official metrics semantics](https://docs.e2b.dev/sandbox/metrics).

Implementation signatures were checked against the published 2.58.0 wheel, not inferred from method names. The installed-SDK contract test makes subsequent upgrades explicit.

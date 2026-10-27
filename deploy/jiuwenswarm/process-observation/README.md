# Jiuwen native process observer candidate

This directory provides the process observation patch for agent-core commit
`3460150aa5f319b0b8f84a4ed10eec0a08b0feb1` with the existing image5 tool-call
context patch. `apply_image.py` preserves both image5 patch entries and appends
the process patch. `build_image.py` creates a separate image7-process1 image;
image5 remains unchanged. Tests and compilation are ECS-only.

ECS verification on 2026-10-27 passed 14 native producer tests against image5
with two bind mounts, the same 14 tests against installed image7, unprivileged
image verification, all 17 A remote tests, and a real native B ExtensionManager
load / FlashBash foreground exit 7 / background session cancellation / shutdown
test. Evidence is under `/opt/aries-artifacts/jiuwen-process-native-1/` in
`test-1`, `build-1`, `integration-1` (A), and `integration-3` (A provenance + B).
The first two B fixture failures remain archived; they concerned fixture
registry construction and missing LOCAL configuration, not observed producer
failures. Full cloud A/B workload acceptance is separate from these local tests.

Image7 was published as
`swr.cn-southwest-2.myhuaweicloud.com/benchmark-zhoutao/aries-jiuwenswarm:0.2.3-e2c9d519-image7-process1@sha256:ba27c904098a8e41c3efe297e773d16b208c2e77edf38e206b70eb0ee335ce21`.
Its patch manifest SHA-256 is
`122f681cb9519a9e4a7443e3cae56b86fd8200e348f92a0152cef609cf5f0eae`.
The independent E2B and Huawei 2 CPU / 4096 MB template capability probes passed;
each created and deleted one TTL-300-second Sandbox, with provider absence
confirmed and zero model calls. Publication and probe receipts are under
`/opt/aries-artifacts/jiuwen-image7-cloud-1/`. These capability probes verify the
installed source and runtime isolation; they are separate from full Agent runs.

`prepare.py --source PINNED_SHELL_OPERATION_PY --output NEW_DIRECTORY` verifies
the exact source SHA and writes a candidate file, observer module, unified patch
and manifest into a new directory. The supplied native source is never edited.
The native diff adds one import and replaces the two subprocess creation calls.

Install `native_process.py` as
`openjiuwen.core.sys_operation.local.aries_process_observer`. Before tools run:

```python
from openjiuwen.core.sys_operation.local import aries_process_observer as observer
observer.configure({"run_id": run_id, "task_id": task_id}, evidence_path)
# A worker only: use the already validated request identity and invocation ID.
token = observer.set_identity({**request_identity, "invocation_id": invocation_id})
try:
    result = await native_handler(**params)
finally:
    observer.reset_identity(token)
# At the owning runtime's actual stop, after command handling has stopped:
await observer.wait_for_completion(2)
observer.finalize()
```

B captures image5's exact active tool call, request and shell session at creation.
A passes the original native session separately from its registry key. The
snapshot stays attached to the subprocess protocol after a background call
returns. A missing native identity is recorded as unavailable, never inferred.

The unconfigured path calls the original `asyncio.create_subprocess_*` entry.
When configured, a per-process `SubprocessStreamProtocol` subclass delegates all
original methods. It does not add readers, waits, decoders, signal handling or
command transformations. Child exit is the actual protocol callback, independent
of pipe closure and of `Process.wait()`'s possible pipe-related delay.

Events are JSONL, schema 1, source `jiuwen_native_process`, with attempted
`sequence`, `clock_domain`, absolute `monotonic_ns`, UUID-hex `process_id`, `pid`,
and copied `identity` (run/task/session/request/tool/invocation plus identity
status). Event names are `spawn_requested`, `connection_made`, `spawn_return`,
`stdout_first`, `stderr_first`, `child_exit`, `stdout_closed`, `stderr_closed`,
and `spawn_failed`. Exit adds `returncode`; failure/close adds only `error_type`,
never exception text. No argv, command, environment or output bytes are recorded.
`spawn_failed` means the creation await failed; a process can already have been
observed, including when that await is cancelled. Do not require spawn-return
to precede fast output/exit callbacks. Closed pipes are not claimed to be natural
EOF, and direct-child exit does not establish descendant absence.

`evidence_path.with_name('process-index.json')` holds the binding, clock anchor,
independent counters, actual written byte count and SHA-256, and observer-closed
state. Clock fields are `emitter_id`, `clock_domain`, `origin_monotonic_ns`,
`origin_unix_ns`, `anchor_before_monotonic_ns`, `anchor_after_monotonic_ns`.
Counters are `spawn_attempted`, `started` (connection made), `exited` (exit
callback), `events_attempted`, `events_written`, `dropped`, `write_failed`,
`index_write_failed`, `late_events`, `identity_failed`. The index also reports
`pending_processes`. Each event synchronously checkpoints the index. Short writes
retain their actual byte/hash prefix and count as dropped, so a broken artifact
cannot be labelled complete. Finalization closes only the observation sink.

`wait_for_completion(seconds)` waits on the already-observed process instances'
exit and stdout/stderr-close callbacks, using one asyncio Event. It never starts
another process-wait task, reads a stream or sends a signal. A timeout returns
False and retains pending state. Callbacks after finalize increment late/drop/
write-failure counters; they cannot produce a falsely complete ledger.
`pending_sessions()` returns the captured nonempty native session IDs of pending
processes whose direct child has not exited. B shutdown may use the existing
native session registry to cancel them, then wait briefly and finalize. A uses
its existing worker cancellation. Neither operation establishes full descendant
absence; the existing final group stop remains required.

Suggested ECS command, with the pinned openjiuwen environment available:

```sh
python -B -m unittest discover -s deploy/jiuwenswarm/process-observation -p 'test_*.py' -v
```

The tests exercise byte/exit equivalence, fast failure/no output, descendant-held
pipes, background grace, concurrent identity snapshots, native cancellation,
creation failure and sink failure. They create only bounded local subprocesses.
The tests preserve native I/O and cancellation behavior. These timestamps measure
process lifecycle callbacks, not CPU time or an exact kernel exec boundary.

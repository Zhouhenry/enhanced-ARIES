"""Pinned Jiuwen LOCAL subprocess observations; never consume command output.

Installed as openjiuwen.core.sys_operation.local.aries_process_observer.
The observer is disabled until configure() is called by the owning runtime.
"""
import asyncio
import contextvars
import hashlib
import json
import os
from pathlib import Path
import threading
import time
import uuid


SOURCE = "jiuwen_native_process"
_identity = contextvars.ContextVar("aries_process_identity", default=None)
_producer = None
_IDENTITY_FIELDS = ("run_id", "task_id", "session_id", "request_id",
                    "tool_call_id", "invocation_id")


def set_identity(identity):
    """Bind an already validated A request; copy values before child tasks inherit it."""
    return _identity.set({key: value if isinstance(value, str) and value else None
                          for key in _IDENTITY_FIELDS for value in (identity.get(key),)})


def reset_identity(token):
    _identity.reset(token)


def _capture_identity(binding):
    supplied = _identity.get()
    failed = False
    if supplied is None:
        try:
            from openjiuwen.core.foundation.tool.base import get_current_tool_call_id
            from openjiuwen.core.sys_operation.shell_process_registry import resolve_shell_session_id
            from openjiuwen.extensions.observability.span_context import get_current_request_id
            supplied = {"tool_call_id": get_current_tool_call_id(),
                        "request_id": get_current_request_id(),
                        "session_id": resolve_shell_session_id()}
        except Exception:
            supplied = {}
            failed = True
    identity = {key: value if isinstance(value, str) and value else None
                for key in _IDENTITY_FIELDS for value in (supplied.get(key),)}
    identity.update(run_id=binding["run_id"], task_id=binding["task_id"])
    identity["identity_status"] = "known" if identity["tool_call_id"] else "unavailable"
    return identity, failed


class _Producer:
    def __init__(self, binding, evidence_path):
        self.binding = {key: binding[key] for key in ("run_id", "task_id")}
        if not all(isinstance(value, str) and value for value in self.binding.values()):
            raise ValueError("run_id and task_id are required")
        self.path = Path(evidence_path)
        self.index_path = self.path.with_name("process-index.json")
        emitter = uuid.uuid4().hex
        before = time.monotonic_ns()
        unix = time.time_ns()
        after = time.monotonic_ns()
        self.clock = {"emitter_id": emitter, "clock_domain": "jiuwen-process/" + emitter,
                      "origin_monotonic_ns": (before + after) // 2,
                      "origin_unix_ns": unix,
                      "anchor_before_monotonic_ns": before,
                      "anchor_after_monotonic_ns": after}
        self.counters = dict.fromkeys(("spawn_attempted", "started", "exited",
                                      "events_attempted", "events_written", "dropped",
                                      "write_failed", "index_write_failed", "late_events",
                                      "identity_failed"), 0)
        self.lock = threading.Lock()
        self.pending = set()
        self.completion = asyncio.Event()
        self.completion.set()
        self.digest = hashlib.sha256()
        self.written_bytes = 0
        self.observer_closed = False
        self.fd = None
        try:
            self.path.parent.mkdir(parents=True, exist_ok=True)
            self.fd = os.open(self.path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        except OSError:
            self.counters["write_failed"] += 1
        self._checkpoint()

    def _checkpoint(self):
        index = {"schema_version": 1, "source": SOURCE,
                 "binding": self.binding, "clock": self.clock,
                 "counters": dict(self.counters), "ledger_sha256": self.digest.hexdigest(),
                 "written_bytes": self.written_bytes, "observer_closed": self.observer_closed,
                 "pending_processes": len(self.pending)}
        temporary = self.index_path.with_suffix(".json.tmp")
        try:
            with open(temporary, "w", encoding="utf-8") as stream:
                os.chmod(temporary, 0o600)
                json.dump(index, stream, separators=(",", ":"))
            os.replace(temporary, self.index_path)
        except OSError:
            self.counters["index_write_failed"] += 1

    def emit(self, observation, event, stamp=None, **fields):
        # A callback must not turn an observation failure into a tool failure.
        with self.lock:
            self.counters["events_attempted"] += 1
            if self.observer_closed:
                self.counters["late_events"] += 1
            if event == "spawn_requested":
                self.counters["spawn_attempted"] += 1
            elif event == "connection_made":
                self.counters["started"] += 1
            elif event == "child_exit":
                self.counters["exited"] += 1
            record = {"schema_version": 1, "source": SOURCE,
                      "sequence": self.counters["events_attempted"],
                      "clock_domain": self.clock["clock_domain"],
                      "monotonic_ns": time.monotonic_ns() if stamp is None else stamp,
                      "event": event, "process_id": observation.process_id,
                      "pid": observation.pid, "identity": observation.identity, **fields}
            try:
                encoded = (json.dumps(record, separators=(",", ":"), ensure_ascii=True) + "\n").encode()
                if self.fd is None or self.observer_closed:
                    raise OSError("observer unavailable")
                written = os.write(self.fd, encoded)
                self.written_bytes += written
                self.digest.update(encoded[:written])
                if written != len(encoded):
                    raise OSError("short observation write")
                self.counters["events_written"] += 1
            except Exception:
                self.counters["dropped"] += 1
                self.counters["write_failed"] += 1
            self._checkpoint()

    def finalize(self):
        with self.lock:
            self.observer_closed = True
            if self.fd is not None:
                try:
                    os.close(self.fd)
                except OSError:
                    self.counters["write_failed"] += 1
                self.fd = None
            self._checkpoint()


def configure(binding, evidence_path):
    """Configure once per owning process, before executing native tools."""
    global _producer
    if _producer is not None and not _producer.observer_closed:
        raise RuntimeError("process observer already configured")
    _producer = _Producer(binding, evidence_path)


def finalize():
    """Close the sink after owned execution has stopped; does not stop any command."""
    if _producer is not None:
        _producer.finalize()


async def wait_for_completion(timeout_seconds):
    """Wait only for observed direct exit and pipe-close callbacks, never waitpid."""
    producer = _producer
    if producer is None or not producer.pending:
        return True
    try:
        await asyncio.wait_for(producer.completion.wait(), timeout=timeout_seconds)
        return not producer.pending
    except asyncio.TimeoutError:
        return False


def pending_sessions():
    """Native registry session keys for live observed children; never send signals."""
    if _producer is None:
        return set()
    return {item.identity["session_id"] for item in _producer.pending
            if not item.exited and item.pid is not None and item.identity["session_id"]}


class _Observation:
    def __init__(self, producer):
        self.producer = producer
        self.process_id = uuid.uuid4().hex
        self.pid = None
        self.exited = False
        self.open_pipes = set()
        self.identity, failed = _capture_identity(producer.binding)
        if failed:
            producer.counters["identity_failed"] += 1
        producer.pending.add(self)
        producer.completion.clear()

    def emit(self, event, stamp=None, **fields):
        if event == "connection_made":
            self.producer.pending.add(self)
            self.producer.completion.clear()
        elif event == "child_exit":
            self.exited = True
        elif event in ("stdout_closed", "stderr_closed"):
            self.open_pipes.discard(1 if event == "stdout_closed" else 2)
        if (self.exited and not self.open_pipes) or (event == "spawn_failed" and self.pid is None):
            self.producer.pending.discard(self)
            if not self.producer.pending:
                self.producer.completion.set()
        self.producer.emit(self, event, stamp, **fields)


class _Protocol(asyncio.subprocess.SubprocessStreamProtocol):
    def __init__(self, limit, loop, observation):
        super().__init__(limit=limit, loop=loop)
        self.observation = observation
        self.first = set()

    def connection_made(self, transport):
        stamp = time.monotonic_ns()
        self.observation.pid = transport.get_pid()
        self.observation.open_pipes = {fd for fd in (1, 2) if transport.get_pipe_transport(fd) is not None}
        super().connection_made(transport)
        self.observation.emit("connection_made", stamp)

    def pipe_data_received(self, fd, data):
        stamp = time.monotonic_ns()
        first = bool(data) and fd in (1, 2) and fd not in self.first
        if first:
            self.first.add(fd)
        super().pipe_data_received(fd, data)
        if first:
            self.observation.emit("stdout_first" if fd == 1 else "stderr_first", stamp)

    def process_exited(self):
        stamp = time.monotonic_ns()
        returncode = self._transport.get_returncode()
        super().process_exited()
        self.observation.emit("child_exit", stamp, returncode=returncode)

    def pipe_connection_lost(self, fd, exc):
        stamp = time.monotonic_ns()
        super().pipe_connection_lost(fd, exc)
        if fd in (1, 2):
            self.observation.emit("stdout_closed" if fd == 1 else "stderr_closed", stamp,
                                  error_type=type(exc).__name__ if exc is not None else None)


async def _create(kind, args, stdin, stdout, stderr, limit, kwds):
    producer = _producer
    original = getattr(asyncio, "create_subprocess_" + kind)
    if producer is None:
        return await original(*args, stdin=stdin, stdout=stdout, stderr=stderr, limit=limit, **kwds)
    observation = _Observation(producer)
    observation.emit("spawn_requested")
    loop = asyncio.get_running_loop()
    factory = lambda: _Protocol(limit, loop, observation)
    try:
        transport, protocol = await getattr(loop, "subprocess_" + kind)(
            factory, *args, stdin=stdin, stdout=stdout, stderr=stderr, **kwds)
        process = asyncio.subprocess.Process(transport, protocol, loop)
    except BaseException as exc:
        observation.emit("spawn_failed", error_type=type(exc).__name__)
        raise
    observation.emit("spawn_return")
    return process


async def create_subprocess_shell(cmd, stdin=None, stdout=None, stderr=None, limit=2 ** 16, **kwds):
    return await _create("shell", (cmd,), stdin, stdout, stderr, limit, kwds)


async def create_subprocess_exec(program, *args, stdin=None, stdout=None, stderr=None, limit=2 ** 16, **kwds):
    return await _create("exec", (program, *args), stdin, stdout, stderr, limit, kwds)

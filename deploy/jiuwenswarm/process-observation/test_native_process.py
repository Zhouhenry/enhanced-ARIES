"""Run only on ECS with Python 3.12; no cloud or model calls.

Native handler cases require the pinned openjiuwen environment. This file does
not patch the installed native source or change its lifecycle implementations.
"""
import asyncio
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import signal
import sys
import tempfile
import unittest
from unittest import mock


HERE = Path(__file__).resolve().parent


def load_observer():
    spec = importlib.util.spec_from_file_location("candidate_process_observer", HERE / "native_process.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class ProcessObservationTests(unittest.IsolatedAsyncioTestCase):
    async def asyncSetUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.path = Path(self.temp.name) / "process-events.jsonl"
        self.observer = load_observer()
        self.observer.configure({"run_id": "run", "task_id": "task"}, self.path)
        self.token = self.observer.set_identity({"tool_call_id": "call", "request_id": "request",
                                                "session_id": "session", "invocation_id": "invocation"})
        self.processes = []

    async def asyncTearDown(self):
        for process in self.processes:
            if process.returncode is None:
                try:
                    os.killpg(process.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
                await process.communicate()
        self.observer.reset_identity(self.token)
        self.observer.finalize()
        self.temp.cleanup()

    def events(self):
        return [json.loads(line) for line in self.path.read_text().splitlines()]

    async def spawn(self, source):
        process = await self.observer.create_subprocess_exec(
            sys.executable, "-c", source, stdin=asyncio.subprocess.DEVNULL,
            stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.PIPE,
            start_new_session=True)
        self.processes.append(process)
        return process

    async def test_binary_output_exit7_and_ledger_integrity(self):
        source = "import os;os.write(1,b'out\\x00\\xff');os.write(2,b'err\\x00\\xfe');raise SystemExit(7)"
        baseline = await asyncio.create_subprocess_exec(sys.executable, "-c", source,
                    stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.PIPE)
        expected = await baseline.communicate()
        process = await self.spawn(source)
        actual = await process.communicate()
        self.assertEqual(actual, expected)
        self.assertEqual(process.returncode, baseline.returncode)
        events = self.events()
        self.assertEqual([e["returncode"] for e in events if e["event"] == "child_exit"], [7])
        for event in ("spawn_requested", "connection_made", "spawn_return", "stdout_first",
                      "stderr_first", "child_exit", "stdout_closed", "stderr_closed"):
            self.assertEqual(sum(e["event"] == event for e in events), 1, event)
        self.observer.finalize()
        index = json.loads(self.path.with_name("process-index.json").read_text())
        raw = self.path.read_bytes()
        self.assertEqual(index["ledger_sha256"], hashlib.sha256(raw).hexdigest())
        self.assertEqual(index["written_bytes"], len(raw))
        self.assertEqual(index["counters"]["events_attempted"], len(events))
        self.assertEqual(index["counters"]["events_written"], len(events))
        self.assertEqual(index["counters"]["started"], 1)
        self.assertEqual(index["counters"]["exited"], 1)
        self.assertTrue(index["observer_closed"])
        self.assertNotIn("out", {k for e in events for k in e})
        self.assertNotIn(source, raw.decode())

    async def test_fast_exit_no_output_and_shell_branch(self):
        process = await self.observer.create_subprocess_shell("exit 7", stdout=asyncio.subprocess.PIPE,
                        stderr=asyncio.subprocess.PIPE, start_new_session=True)
        self.processes.append(process)
        self.assertEqual(await process.communicate(), (b"", b""))
        events = self.events()
        self.assertEqual([e["returncode"] for e in events if e["event"] == "child_exit"], [7])
        self.assertFalse(any(e["event"] in ("stdout_first", "stderr_first") for e in events))
        # A very fast callback is permitted before the creating await resumes.
        self.assertEqual(len({e["process_id"] for e in events}), 1)

    async def test_direct_exit_precedes_descendant_pipe_close(self):
        process = await self.spawn("import os,time\nif os.fork()==0:\n time.sleep(0.4);os._exit(0)\nos._exit(7)")
        await process.communicate()
        events = {e["event"]: e for e in self.events()}
        exited = events["child_exit"]["monotonic_ns"]
        self.assertEqual(events["child_exit"]["returncode"], 7)
        self.assertGreater(events["stdout_closed"]["monotonic_ns"], exited + 100_000_000)
        self.assertGreater(events["stderr_closed"]["monotonic_ns"], exited + 100_000_000)

    async def test_parallel_and_background_identity_snapshot(self):
        async def launch(call, delay):
            token = self.observer.set_identity({"tool_call_id": call, "session_id": "same",
                                                "request_id": "req-" + call, "invocation_id": "inv-" + call})
            try:
                process = await self.spawn(f"import time;print('ready',flush=True);time.sleep({delay})")
            finally:
                self.observer.reset_identity(token)
            await process.communicate()
            return process.pid
        pids = await asyncio.gather(launch("slow", 0.2), launch("fast", 0.03))
        events = self.events()
        for call, pid in zip(("slow", "fast"), pids):
            child = [e for e in events if e["pid"] == pid]
            self.assertTrue(child)
            self.assertTrue(all(e["identity"]["tool_call_id"] == call for e in child))
            self.assertTrue(all(e["identity"]["invocation_id"] == "inv-" + call for e in child))
        exits = [e["identity"]["tool_call_id"] for e in events if e["event"] == "child_exit"]
        self.assertEqual(exits, ["fast", "slow"])

    async def test_native_background_grace_observes_output_before_return(self):
        from openjiuwen.core.sys_operation.local.utils import AsyncProcessHandler
        process = await self.spawn("import time;print('ready',flush=True);time.sleep(0.25)")
        async with asyncio.timeout(2):
            while not any(e["event"] == "stdout_first" for e in self.events()):
                await asyncio.sleep(0.005)
        handler = AsyncProcessHandler(process)
        pid, error = await handler.background(grace=0.03)
        self.assertEqual(pid, process.pid)
        self.assertIsNone(error)
        names = [e["event"] for e in self.events()]
        self.assertIn("stdout_first", names)
        self.assertNotIn("child_exit", names)
        self.assertEqual(await process.communicate(), (b"ready\n", b""))
        self.assertIn("child_exit", [e["event"] for e in self.events()])

    async def test_native_invoke_cancel_keeps_cancelled_error_and_exit(self):
        from openjiuwen.core.sys_operation.local.utils import AsyncProcessHandler
        process = await self.spawn("import time;print('ready',flush=True);time.sleep(30)")
        task = asyncio.create_task(AsyncProcessHandler(process).invoke())
        async with asyncio.timeout(2):
            while not any(e["event"] == "stdout_first" for e in self.events()):
                await asyncio.sleep(0.005)
        task.cancel()
        with self.assertRaises(asyncio.CancelledError):
            await task
        await process.wait()
        self.assertEqual([e["returncode"] for e in self.events() if e["event"] == "child_exit"], [-signal.SIGKILL])

    async def test_create_failure_has_no_invented_exit(self):
        with self.assertRaises(FileNotFoundError):
            await self.observer.create_subprocess_exec("/aries-no-such-command")
        events = self.events()
        self.assertEqual([e["event"] for e in events], ["spawn_requested", "spawn_failed"])
        self.assertTrue(all(e["pid"] is None for e in events))
        self.assertEqual(events[-1]["error_type"], "FileNotFoundError")

    async def test_sink_failure_does_not_change_command(self):
        with mock.patch.object(self.observer.os, "write", side_effect=OSError("private failure detail")):
            process = await self.spawn("print('intact');raise SystemExit(7)")
            self.assertEqual(await process.communicate(), (b"intact\n", b""))
        self.assertEqual(process.returncode, 7)
        index = json.loads(self.path.with_name("process-index.json").read_text())
        self.assertGreater(index["counters"]["write_failed"], 0)
        self.assertEqual(index["counters"]["events_written"], 0)
        self.assertEqual(index["counters"]["dropped"], index["counters"]["events_attempted"])

    async def test_disabled_uses_original_asyncio_entry(self):
        unconfigured = load_observer()
        result = object()
        with mock.patch.object(asyncio, "create_subprocess_exec", new=mock.AsyncMock(return_value=result)) as original:
            self.assertIs(await unconfigured.create_subprocess_exec("program", "argument", cwd="/tmp"), result)
        original.assert_awaited_once_with("program", "argument", stdin=None, stdout=None,
                                          stderr=None, limit=2 ** 16, cwd="/tmp")

    async def test_completion_timeout_and_native_pending_sessions(self):
        process = await self.spawn("import time;print('ready',flush=True);time.sleep(30)")
        self.assertEqual(self.observer.pending_sessions(), {"session"})
        self.assertFalse(await self.observer.wait_for_completion(0.01))
        os.killpg(process.pid, signal.SIGKILL)
        await process.communicate()
        self.assertTrue(await self.observer.wait_for_completion(1))
        self.assertEqual(self.observer.pending_sessions(), set())
        index = json.loads(self.path.with_name("process-index.json").read_text())
        self.assertEqual(index["pending_processes"], 0)

    async def test_finalize_before_exit_preserves_late_failure(self):
        process = await self.spawn("import time;time.sleep(0.1)")
        self.observer.finalize()
        await process.communicate()
        index = json.loads(self.path.with_name("process-index.json").read_text())
        self.assertTrue(index["observer_closed"])
        self.assertGreater(index["counters"]["late_events"], 0)
        self.assertEqual(index["counters"]["late_events"], index["counters"]["dropped"])
        self.assertGreater(index["counters"]["write_failed"], 0)

    async def test_identity_getter_failure_does_not_block_spawn(self):
        token = self.observer._identity.set(None)
        try:
            # Simulate an unavailable native tool-context module, without touching command I/O.
            with mock.patch.dict(sys.modules, {"openjiuwen.core.foundation.tool.base": None}):
                process = await self.spawn("print('still runs')")
            self.assertEqual(await process.communicate(), (b"still runs\n", b""))
        finally:
            self.observer._identity.reset(token)
        self.assertTrue(all(e["identity"]["identity_status"] == "unavailable" for e in self.events()))
        index = json.loads(self.path.with_name("process-index.json").read_text())
        self.assertEqual(index["counters"]["identity_failed"], 1)

    async def test_patched_native_shell_and_exec_creation_entries(self):
        from openjiuwen.core.sys_operation.local import shell_operation
        from openjiuwen.core.sys_operation.shell import ShellType
        operation = shell_operation.ShellOperation.__new__(shell_operation.ShellOperation)
        with mock.patch.object(shell_operation, "aries_process_observer", self.observer):
            for shell in (ShellType.SH, ShellType.BASH):
                process = await operation._create_subprocess(
                    "printf native-entry; exit 7", Path(self.temp.name), dict(os.environ), shell_type=shell)
                self.processes.append(process)
                self.assertEqual(await process.communicate(), (b"native-entry", b""))
                self.assertEqual(process.returncode, 7)
        self.assertEqual(sum(e["event"] == "child_exit" for e in self.events()), 2)


class PrepareTests(unittest.TestCase):
    def test_wrong_baseline_rejected_without_creating_candidate(self):
        spec = importlib.util.spec_from_file_location("prepare_process_candidate", HERE / "prepare.py")
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        with tempfile.TemporaryDirectory() as directory:
            source = Path(directory) / "shell_operation.py"
            source.write_text("wrong baseline\n")
            output = Path(directory) / "candidate"
            with self.assertRaisesRegex(ValueError, "hash mismatch"):
                module.prepare(source, output)
            self.assertFalse(output.exists())


if __name__ == "__main__":
    unittest.main()

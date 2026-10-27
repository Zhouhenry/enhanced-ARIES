import base64
import copy
from dataclasses import dataclass
import io
import http.client
import hashlib
from http import HTTPStatus
import json
from pathlib import Path
import shlex
import subprocess
import sys
import tempfile
import threading
import unittest
from unittest.mock import patch
import urllib.error
import urllib.request

from server import Adapter, Failure, GUEST_EXEC, Job, create_failure_diagnostic, make_server
from guest_exec import strict_json_loads, validate_observation


def linked_observation(process="process1", invocation="invocation1"):
    return {"schema_version": 1, "native_identity": {
        "schema_version": 1, "scope_id": "attempt1", "agent_session_id": "session1",
        "tool_call_id": "tool1", "native_invocation_id": invocation,
        "ssh_request_id": "request-" + invocation},
        "execution_id": "execution-" + invocation, "process_id": process}


def identity_frame(observation):
    return {"kind": "execution_identity", "phase": "before_popen", "observation": observation}


class Missing(Exception):
    pass


class FileMissing(Exception):
    pass


class ProcessMissing(Exception):
    pass


class Stream:
    def __init__(self, data): self.data = data
    def __enter__(self): return self
    def __exit__(self, *_): pass
    def __iter__(self): yield self.data


class Files:
    def __init__(self): self.data = {}
    def write(self, path, data, **options):
        assert options.get("user") == "root"
        self.data[path] = data
    def read(self, path, **options):
        assert options.get("user") == "root"
        if path not in self.data: raise FileMissing()
        return Stream(self.data[path])


class Native:
    pid = 17
    def __init__(self, frames): self.frames = frames; self.input = b""; self.closed = False
    def __iter__(self):
        data = "".join(json.dumps(f) + "\n" for f in self.frames)
        # SDK callbacks may split frames in the middle of a JSON token.
        for part in (data[:7], data[7:]): yield part, None, None
    def wait(self):
        class Result: exit_code = 0
        return Result()
    def send_stdin(self, data, **_): self.input += data
    def close_stdin(self, **_): self.closed = True
    def kill(self): return True


class Commands:
    def __init__(self): self.native = Native([{"stream": "stdout", "data_base64": "AP8="}, {"exit_code": 7, "guest_duration_ns": 123456}]); self.present = True
    def run(self, cmd, **kwargs): self.command, self.options = cmd, kwargs; return self.native
    def list(self, **_): return [self.native] if self.present else []
    def kill(self, pid, **_): self.present = False; return True


class FakeSandbox:
    sandbox_id = "cloud-private-id"
    traffic_access_token = "private-traffic-token"
    def __init__(self): self.files = Files(); self.commands = Commands()
    def get_host(self, port): return str(port) + "-example.e2b.app"
    def get_metrics(self, **_): return []


class SDK:
    def __init__(self): self.live = None; self.fail_create = False; self.fail_kill = False
    def create(self, **kwargs):
        self.options = kwargs
        if self.fail_create: raise TimeoutError("secret-value")
        self.live = FakeSandbox(); return self.live
    def kill(self, identity, **_):
        if self.fail_kill: raise TimeoutError("secret-value")
        self.live = None; return True
    def get_info(self, identity, **_):
        if self.live is None: raise Missing()
        return object()


class MultiSandboxSDK(SDK):
    def __init__(self):
        super().__init__()
        self.identities = set()
        self.lock = threading.Lock()

    def create(self, **kwargs):
        with self.lock:
            sandbox = super().create(**kwargs)
            sandbox.sandbox_id = kwargs["metadata"]["aries_adapter_handle"]
            self.identities.add(sandbox.sandbox_id)
            return sandbox

    def kill(self, identity, **_):
        with self.lock:
            if self.fail_kill: raise TimeoutError("secret-value")
            self.identities.discard(identity)

    def get_info(self, identity, **_):
        with self.lock:
            if identity not in self.identities: raise Missing()
            return object()


class Tests(unittest.TestCase):
    def setUp(self):
        self.sdk = SDK()
        self.adapter = Adapter(self.sdk, Missing, FileMissing, ProcessMissing, templates=["pinned"], max_sandboxes=1)
        self.body = {"sandbox": "attempt_0123456789", "template": "pinned"}

    def create(self): return self.adapter.create(self.body)

    def test_create_policy_live_quota_and_no_retry(self):
        created = self.create()
        self.assertEqual(self.sdk.options["http_version"], "1.1")
        self.assertEqual(created["sandbox_connection_http_version"], "1.1")
        self.assertEqual(self.sdk.options["network"], {"allow_public_traffic": False})
        self.assertEqual(self.sdk.options["lifecycle"]["on_timeout"], "kill")
        self.assertFalse(self.sdk.options["lifecycle"]["auto_resume"])
        self.assertFalse(self.sdk.options["allow_internet_access"])
        with self.assertRaisesRegex(Failure, "duplicate_create"): self.create()
        with self.assertRaisesRegex(Failure, "local_quota"): self.adapter.create({**self.body, "sandbox": "another_attempt"})
        self.assertTrue(self.adapter.kill(self.body)["absence_confirmed"])
        self.assertTrue(self.adapter.kill(self.body)["absence_confirmed"])

    def test_sequential_creates_have_no_cumulative_limit_and_survive_restart(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "adapter.state.json"
            retained = {}
            for first, last in ((0, 6), (6, 12)):
                adapter = Adapter(self.sdk, Missing, FileMissing, ProcessMissing, templates=["pinned"],
                                  max_sandboxes=1, state_file=path)
                try:
                    self.assertEqual(adapter.creates, first)
                    self.assertEqual(adapter.records, retained)
                    for index in range(first, last):
                        body = {**self.body, "sandbox": "sequential_attempt_" + str(index)}
                        self.assertEqual(adapter.create(body)["state"], "active")
                        self.assertEqual(adapter.creates, index + 1)
                        self.assertTrue(adapter.kill(body)["absence_confirmed"])
                        self.assertIsNone(self.sdk.live)
                    saved = json.loads(path.read_text())
                    self.assertEqual(saved["creates"], last)
                    self.assertEqual(len(saved["records"]), last)
                    self.assertTrue(all(row["state"] == "absent" for row in saved["records"].values()))
                    retained = saved["records"]
                    self.assertEqual(adapter.close(), [])
                finally:
                    adapter.close()
                    adapter.state_lock.close()

    def test_failed_create_stays_owned_and_unknown(self):
        self.sdk.fail_create = True
        with self.assertRaisesRegex(Failure, "allocation_unconfirmed"): self.create()
        diagnostic = self.adapter.records[self.body["sandbox"]]["create_failure"]
        self.assertEqual(diagnostic["exception_type"], "builtins.TimeoutError")
        self.assertIsNone(diagnostic["http_status"])
        self.assertIsNone(diagnostic["request_id"])
        self.assertIsNone(diagnostic["message"])
        self.assertNotIn("secret-value", json.dumps(diagnostic))
        with self.assertRaisesRegex(Failure, "allocation_unconfirmed"): self.adapter.kill(self.body)
        self.assertEqual(len(self.adapter.close()), 1)

    def test_create_failure_persists_safe_sdk_diagnostic_without_changing_ownership(self):
        # Match the pinned SDK's exception contract without importing the SDK.
        sdk_error = type("SandboxException", (Exception,), {"__module__": "e2b.exceptions"})
        error = sdk_error("400: requested timeout exceeds the maximum allowed timeout")
        error.status_code, error.request_id = HTTPStatus.BAD_REQUEST, "req-1234"
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "adapter.state.json"
            adapter = Adapter(self.sdk, Missing, FileMissing, ProcessMissing,
                              templates=["pinned"], state_file=path)
            try:
                with patch.object(self.sdk, "create", side_effect=error) as create:
                    with self.assertRaisesRegex(Failure, "allocation_unconfirmed"):
                        adapter.create(self.body)
                    with self.assertRaisesRegex(Failure, "duplicate_create_no_retry"):
                        adapter.create(self.body)
                    create.assert_called_once()
                saved = json.loads(path.read_text())
                record = saved["records"][self.body["sandbox"]]
                self.assertEqual(saved["creates"], 1)
                self.assertEqual(record["state"], "allocation_unconfirmed")
                self.assertNotIn("cloud_id", record)
                self.assertEqual(record["create_failure"], {
                    "exception_type": "e2b.exceptions.SandboxException", "http_status": 400,
                    "request_id": "req-1234", "message_state": "sdk_message_redacted",
                    "message": "requested timeout exceeds the maximum allowed timeout"})
                self.assertEqual(adapter.close(), [self.body["sandbox"]])
                self.assertEqual(path.stat().st_mode & 0o777, 0o600)
            finally:
                adapter.state_lock.close()

    def test_create_failure_message_redacts_credentials_and_does_not_read_response(self):
        sdk_error = type("SandboxException", (Exception,), {"__module__": "e2b.exceptions"})
        error = sdk_error("400: timeout unsupported; key=unconfigured-key "
                          "Bearer unconfigured-bearer "
                          "https://user:password@provider.invalid/path?token=query-secret "
                          "configured-secret")
        error.status_code = 400
        class UnreadableResponse:
            @property
            def headers(self): raise AssertionError("response headers must not be read")
            @property
            def content(self): raise AssertionError("response body must not be read")
        error.response = UnreadableResponse()
        with patch.dict("os.environ", {"E2B_API_KEY": "configured-secret"}, clear=True):
            diagnostic = create_failure_diagnostic(error)
        self.assertEqual(diagnostic["http_status"], 400)
        self.assertIsNone(diagnostic["request_id"])
        self.assertIn("timeout unsupported", diagnostic["message"])
        for secret in ("unconfigured-key", "unconfigured-bearer", "query-secret",
                       "configured-secret", "user:password", "provider.invalid"):
            self.assertNotIn(secret, json.dumps(diagnostic))

    def test_create_failure_omits_bodies_invalid_metadata_and_generic_messages(self):
        sdk_error = type("SandboxException", (Exception,), {"__module__": "e2b.exceptions"})
        for raw in ("400: b'raw-private-body'", '400: {"message":"private-body"}',
                    "502: <html>private-body</html>", "400: header\nprivate-body"):
            with self.subTest(raw=raw):
                error = sdk_error(raw)
                error.status_code, error.request_id = True, "req\nprivate-body"
                diagnostic = create_failure_diagnostic(error)
                self.assertIsNone(diagnostic["http_status"])
                self.assertIsNone(diagnostic["request_id"])
                self.assertIsNone(diagnostic["message"])
                self.assertEqual(diagnostic["message_state"], "omitted_body_or_nontext_message")
                self.assertNotIn("private-body", json.dumps(diagnostic))

    def test_kill_requires_control_plane_absence(self):
        self.create(); self.sdk.fail_kill = True
        with self.assertRaisesRegex(Failure, "absence_unconfirmed"): self.adapter.kill(self.body)
        self.assertIn(self.body["sandbox"], self.adapter.live)
        self.sdk.fail_kill = False
        self.assertTrue(self.adapter.kill(self.body)["absence_confirmed"])

    def test_unconfirmed_allocation_or_cleanup_blocks_new_spending(self):
        for failed_create in (True, False):
            with self.subTest(failed_create=failed_create):
                sdk = SDK()
                adapter = Adapter(sdk, Missing, FileMissing, ProcessMissing, templates=["pinned"], max_sandboxes=2)
                sdk.fail_create = failed_create
                if failed_create:
                    with self.assertRaisesRegex(Failure, "allocation_unconfirmed"):
                        adapter.create(self.body)
                else:
                    adapter.create(self.body)
                    sdk.fail_kill = True
                    with self.assertRaisesRegex(Failure, "absence_unconfirmed"):
                        adapter.kill(self.body)
                next_body = {**self.body, "sandbox": "next_attempt"}
                with self.assertRaisesRegex(Failure, "cleanup_required_before_create") as blocked:
                    adapter.create(next_body)
                self.assertEqual(blocked.exception.status, 409)
                self.assertEqual(adapter.creates, 1)
                self.assertNotIn(next_body["sandbox"], adapter.records)
                self.assertEqual(sdk.options["metadata"]["aries_adapter_handle"], self.body["sandbox"])
                if not failed_create:
                    sdk.fail_kill = False
                    adapter.kill(self.body)
                    self.assertEqual(adapter.create(next_body)["state"], "active")
                    self.assertEqual(adapter.creates, 2)

    def test_active_and_inflight_allocations_allow_configured_concurrency(self):
        entered, release = threading.Event(), threading.Event()
        sdk = SDK()
        create = sdk.create
        inflight = {**self.body, "sandbox": "inflight_attempt"}

        def delayed_create(**options):
            if options["metadata"]["aries_adapter_handle"] == inflight["sandbox"]:
                entered.set()
                if not release.wait(2):
                    raise TimeoutError("test did not release create")
            return create(**options)

        sdk.create = delayed_create
        adapter = Adapter(sdk, Missing, FileMissing, ProcessMissing, templates=["pinned"], max_sandboxes=3)
        self.assertEqual(adapter.create(self.body)["state"], "active")
        outcomes = []

        def allocate():
            try:
                outcomes.append(adapter.create(inflight)["state"])
            except Failure as error:
                outcomes.append(error.code)

        worker = threading.Thread(target=allocate)
        worker.start()
        try:
            self.assertTrue(entered.wait(2))
            self.assertEqual(adapter.create({**self.body, "sandbox": "third_attempt"})["state"], "active")
        finally:
            release.set()
            worker.join(2)
        self.assertFalse(worker.is_alive())
        self.assertEqual(outcomes, ["active"])
        self.assertEqual(adapter.creates, 3)

    def test_inflight_deletion_allows_spare_capacity_but_still_counts_toward_quota(self):
        for capacity in (1, 2):
            with self.subTest(capacity=capacity):
                sdk = MultiSandboxSDK()
                adapter = Adapter(sdk, Missing, FileMissing, ProcessMissing, templates=["pinned"], max_sandboxes=capacity)
                adapter.create(self.body)
                entered, release = threading.Event(), threading.Event()
                kill, outcomes = sdk.kill, []

                def delayed_kill(identity, **options):
                    entered.set()
                    if not release.wait(2): raise TimeoutError("test did not release deletion")
                    return kill(identity, **options)

                def remove():
                    try: outcomes.append(adapter.kill(self.body))
                    except Exception as error: outcomes.append(error)

                sdk.kill = delayed_kill
                worker = threading.Thread(target=remove)
                worker.start()
                try:
                    self.assertTrue(entered.wait(2))
                    self.assertEqual(adapter.records[self.body["sandbox"]]["state"], "stopping")
                    next_body = {**self.body, "sandbox": "parallel_attempt"}
                    if capacity == 1:
                        with self.assertRaisesRegex(Failure, "local_quota_exceeded"):
                            adapter.create(next_body)
                        self.assertEqual(adapter.creates, 1)
                    else:
                        self.assertEqual(adapter.create(next_body)["state"], "active")
                        self.assertEqual(adapter.creates, 2)
                finally:
                    release.set(); worker.join(2)
                self.assertFalse(worker.is_alive())
                self.assertEqual(outcomes, [{"absence_confirmed": True}])
                self.assertEqual(adapter.records[self.body["sandbox"]]["state"], "absent")
                self.assertEqual(adapter.create({**self.body, "sandbox": "later_attempt"})["state"], "active")

    def test_duplicate_deletion_failure_blocks_creates_through_retry(self):
        sdk = MultiSandboxSDK()
        adapter = Adapter(sdk, Missing, FileMissing, ProcessMissing, templates=["pinned"], max_sandboxes=3)
        adapter.create(self.body)
        entered = [threading.Event(), threading.Event()]
        release = [threading.Event(), threading.Event()]
        outcomes = [None, None]
        kill = sdk.kill

        def remove(index):
            threading.current_thread().deletion_index = index
            try: outcomes[index] = adapter.kill(self.body)
            except Exception as error: outcomes[index] = error

        def failed_kill(identity, **options):
            index = threading.current_thread().deletion_index
            entered[index].set()
            if not release[index].wait(2): raise TimeoutError("test did not release deletion")
            raise TimeoutError("delete remains unconfirmed")

        sdk.kill = failed_kill
        workers = [threading.Thread(target=remove, args=(index,)) for index in range(2)]
        for worker in workers: worker.start()
        try:
            self.assertTrue(all(event.wait(2) for event in entered))
            self.assertEqual(adapter.create({**self.body, "sandbox": "parallel_attempt"})["state"], "active")
            release[0].set(); workers[0].join(2)
            self.assertFalse(workers[0].is_alive())
            self.assertIsInstance(outcomes[0], Failure)
            self.assertEqual(outcomes[0].code, "absence_unconfirmed")
            with self.assertRaisesRegex(Failure, "cleanup_required_before_create"):
                adapter.create({**self.body, "sandbox": "later_attempt"})
        finally:
            for event in release: event.set()
            for worker in workers: worker.join(2)
        self.assertTrue(all(not worker.is_alive() for worker in workers))
        self.assertTrue(all(isinstance(error, Failure) and error.code == "absence_unconfirmed" for error in outcomes))
        with self.assertRaisesRegex(Failure, "cleanup_required_before_create"):
            adapter.create({**self.body, "sandbox": "later_attempt"})
        self.assertEqual(adapter.creates, 2)
        retry_entered, retry_release = threading.Event(), threading.Event()
        retry_outcomes = []

        def delayed_retry(identity, **options):
            retry_entered.set()
            if not retry_release.wait(2): raise TimeoutError("test did not release retry")
            return kill(identity, **options)

        def retry():
            try: retry_outcomes.append(adapter.kill(self.body))
            except Exception as error: retry_outcomes.append(error)

        sdk.kill = delayed_retry
        worker = threading.Thread(target=retry)
        worker.start()
        try:
            self.assertTrue(retry_entered.wait(2))
            with self.assertRaisesRegex(Failure, "cleanup_required_before_create"):
                adapter.create({**self.body, "sandbox": "later_attempt"})
        finally:
            retry_release.set(); worker.join(2)
        self.assertFalse(worker.is_alive())
        self.assertEqual(retry_outcomes, [{"absence_confirmed": True}])
        self.assertEqual(adapter.create({**self.body, "sandbox": "later_attempt"})["state"], "active")

    def test_deletion_permission_unwinds_on_persist_or_lookup_error(self):
        for phase in ("starting_persist", "absent_persist", "get_info"):
            with self.subTest(phase=phase):
                sdk = MultiSandboxSDK()
                adapter = Adapter(sdk, Missing, FileMissing, ProcessMissing, templates=["pinned"], max_sandboxes=2)
                adapter.create(self.body)

                def failed_persist():
                    state = adapter.records[self.body["sandbox"]]["state"]
                    if phase == "starting_persist" or state == "absent":
                        raise OSError("state storage unavailable")

                def failed_lookup(identity, **options):
                    raise TimeoutError("control plane unavailable")

                if phase == "get_info": sdk.get_info = failed_lookup
                else: adapter.persist = failed_persist
                with self.assertRaises((OSError, TimeoutError)): adapter.kill(self.body)
                self.assertEqual(adapter.records[self.body["sandbox"]]["state"], "stopping")
                with self.assertRaisesRegex(Failure, "cleanup_required_before_create"):
                    adapter.create({**self.body, "sandbox": "later_attempt"})
                self.assertEqual(adapter.creates, 1)

    def test_restart_does_not_restore_inflight_deletion_permission(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "adapter.state.json"
            sdk = MultiSandboxSDK()
            sdk.identities.add("known-cloud-id"); sdk.fail_kill = True
            path.write_text(json.dumps({"schema": 1, "creates": 1, "records": {
                self.body["sandbox"]: {"state": "stopping", "cloud_id": "known-cloud-id"}}}))
            with self.assertRaisesRegex(Failure, "recovery_absence_unconfirmed"):
                Adapter(sdk, Missing, FileMissing, ProcessMissing, templates=["pinned"], state_file=path)
            saved = json.loads(path.read_text())
            self.assertEqual(saved["records"][self.body["sandbox"]]["state"], "stopping")
            self.assertEqual(set(saved), {"schema", "creates", "records"})
            sdk.fail_kill = False
            adapter = Adapter(sdk, Missing, FileMissing, ProcessMissing, templates=["pinned"], state_file=path)
            try:
                self.assertEqual(adapter.records[self.body["sandbox"]]["state"], "absent")
                self.assertEqual(adapter.creates, 1)
                self.assertEqual(adapter.create({**self.body, "sandbox": "later_attempt"})["state"], "active")
            finally:
                adapter.close(); adapter.state_lock.close()

    def test_binary_frames_exit_and_managed_process_not_descendants(self):
        self.create()
        process = {**self.body, "process": "process1", "argv": ["printf", "a ' $HOME"], "cwd": "/tmp", "env": {"X": "x"}}
        self.adapter.start(process)
        response = self.adapter.dispatch("events", process)
        while not response["done"]: response = self.adapter.dispatch("events", process)
        self.assertEqual(base64.b64decode(response["events"][0]["data_base64"]), b"\0\xff")
        self.assertEqual(response["events"][-1]["exit_code"], 7)
        self.assertEqual(response["events"][-1]["guest_duration_ns"], 123456)
        self.assertTrue(self.sdk.live.commands.options["stdin"])
        killed = self.adapter.dispatch("kill_process", process)
        self.assertTrue(killed["managed_process_absent"])
        self.assertIsNone(killed["descendants_absent"])

    def test_wrapper_starts_without_default_home_and_preserves_task_cwd(self):
        self.create()
        self.addCleanup(self.adapter.close)
        commands = self.sdk.live.commands
        run = commands.run

        def missing_default_home(command, **options):
            if options.get("cwd") is None:
                raise FileNotFoundError("default /home/user does not exist")
            return run(command, **options)

        commands.run = missing_default_home
        spec = {"argv": ["printf", "literal ' $HOME"],
                "cwd": "/workspace/task with spaces", "env": {"X": "literal"}}
        process = {**self.body, "process": "no_default_home", **spec}
        self.adapter.start(process)
        job = self.adapter.jobs[self.body["sandbox"]][process["process"]]
        with job.condition:
            self.assertTrue(job.condition.wait_for(lambda: job.done, timeout=2))
        self.assertEqual(commands.options["cwd"], "/")
        wrapper = shlex.split(commands.command)
        self.assertEqual(wrapper[:-1], ["python3", "-u", "-c", GUEST_EXEC])
        self.assertEqual(json.loads(base64.b64decode(wrapper[-1])), spec)
        self.assertEqual(job.events[-1]["exit_code"], 7)

    def test_output_limit_terminal_is_failure(self):
        self.create()
        process = {**self.body, "process": "process1", "argv": ["true"], "cwd": "/tmp", "output_limit_bytes": 1}
        self.adapter.start(process)
        job = self.adapter.jobs[self.body["sandbox"]]["process1"]
        with job.condition: job.condition.wait_for(lambda: job.done, timeout=2)
        self.assertEqual(job.events[-1]["error_code"], "output_limit_exceeded")
        self.assertFalse(job.events[-1]["termination_confirmed"])

    def test_guest_duration_is_required_nonnegative_integer_and_not_output(self):
        for duration in (None, -1, True, 1.5, 1 << 63):
            frame = {"exit_code": 0, "guest_duration_ns": duration}
            with self.subTest(duration=duration):
                job = Job(Native([frame]), 1024)
                self.adapter.drain(job)
                self.assertTrue(job.done)
                self.assertEqual(job.events[-1]["error_code"], "invalid_guest_frame")
                self.assertNotIn("guest_duration_ns", job.events[-1])
        for frames in ([{"exit_code": 0}],
                       [{"exit_code": 0, "guest_duration_ns": 0, "command": "secret"}],
                       [{"exit_code": 0, "guest_duration_ns": 0}, {"exit_code": 0, "guest_duration_ns": 0}]):
            job = Job(Native(frames), 1024)
            self.adapter.drain(job)
            self.assertIn("error_code", job.events[-1])

    def test_guest_boundary_values_are_strict_and_nonnegative(self):
        for value in (-1, True, 1.5, 2, 1 << 63):
            with self.subTest(value=value):
                job = Job(Native([{"exit_code": 0, "guest_duration_ns": 1, "pipe_eof_ns": value}]), 1024)
                self.adapter.drain(job)
                self.assertEqual(job.events[-1]["error_code"], "invalid_guest_frame")

    def test_guest_boundary_order_is_checked_without_assuming_exit_before_eof(self):
        for before, after in (("popen_return_ns", "first_output_ns"),
                              ("popen_return_ns", "child_exit_observed_ns"),
                              ("popen_return_ns", "pipe_eof_ns"),
                              ("first_output_ns", "pipe_eof_ns")):
            with self.subTest(before=before, after=after):
                job = Job(Native([{"exit_code": 0, "guest_duration_ns": 3,
                                   before: 2, after: 1}]), 1024)
                self.adapter.drain(job)
                self.assertEqual(job.events[-1]["error_code"], "invalid_guest_frame")
        for child_exit, pipe_eof in ((2, 3), (3, 2)):
            job = Job(Native([{"exit_code": 0, "guest_duration_ns": 4,
                               "popen_return_ns": 1, "first_output_ns": None,
                               "child_exit_observed_ns": child_exit, "pipe_eof_ns": pipe_eof}]), 1024)
            self.adapter.drain(job)
            self.assertEqual(job.events[-1]["exit_code"], 0)

    def test_sdk_measurement_reports_real_boundary_without_payload(self):
        self.adapter.sdk_version = "installed-test-version"
        self.adapter.measurements.calls = []
        created = self.create()
        self.assertEqual(created["sdk_version"], "installed-test-version")
        self.assertIsNone(created["region"])
        self.assertIsNone(created["resources"])
        self.assertEqual(created["cold_warm"], "unknown")
        self.adapter.dispatch("upload", {**self.body, "path": "/secret-path", "data_base64": "AP8="})
        downloaded = self.adapter.dispatch("download", {**self.body, "path": "/secret-path"})
        self.assertEqual(downloaded["bytes"], 2)
        with self.assertRaisesRegex(Failure, "file_not_found"):
            self.adapter.dispatch("download", {**self.body, "path": "/missing"})
        calls = self.adapter.measurements.calls
        self.assertEqual([c["operation"] for c in calls], ["sandbox.create", "files.write", "files.read", "files.read"])
        self.assertEqual(calls[-1]["status"], "failed")
        self.assertTrue(all(type(c["duration_ns"]) is int and c["duration_ns"] >= 0 for c in calls))
        self.assertNotIn("secret", json.dumps(calls))
        self.assertNotIn("cloud-private-id", json.dumps(calls))

    def test_files_enforce_bound_and_missing(self):
        self.create()
        self.adapter.dispatch("upload", {**self.body, "path": "/tmp/x", "data_base64": "AP8="})
        result = self.adapter.dispatch("download", {**self.body, "path": "/tmp/x"})
        self.assertEqual(result["data_base64"], "AP8=")
        with self.assertRaisesRegex(Failure, "file_too_large"): self.adapter.dispatch("download", {**self.body, "path": "/tmp/x", "max_bytes": 1})
        with self.assertRaisesRegex(Failure, "file_not_found"): self.adapter.dispatch("download", {**self.body, "path": "/tmp/missing"})

    def test_http_file_transfer_64_mib_and_explicit_bounds(self):
        self.create()
        content = bytes(range(256)) * ((64 << 20) // 256)
        expected = hashlib.sha256(content).digest()
        server = make_server(self.adapter, "t" * 32, ("127.0.0.1", 0))
        worker = threading.Thread(target=server.serve_forever)
        worker.start()
        try:
            def request(action, body):
                connection = http.client.HTTPConnection("127.0.0.1", server.server_port, timeout=30)
                try:
                    connection.request("POST", "/v1/" + action, json.dumps(body),
                                       {"Authorization": "Bearer " + "t" * 32})
                    response = connection.getresponse()
                    return response.status, json.loads(response.read())
                finally:
                    connection.close()

            status, report = request("upload", {**self.body, "path": "/large.pptx",
                "data_base64": base64.b64encode(content).decode()})
            self.assertEqual((status, report.get("bytes")), (200, 64 << 20))
            status, report = request("download", {**self.body, "path": "/large.pptx", "max_bytes": 64 << 20})
            self.assertEqual((status, report.get("bytes")), (200, 64 << 20))
            actual = base64.b64decode(report.pop("data_base64"), validate=True)
            self.assertEqual(hashlib.sha256(actual).digest(), expected)
            self.assertEqual(len(actual), 64 << 20)
            del actual
            status, report = request("download", {**self.body, "path": "/large.pptx", "max_bytes": (64 << 20)-1})
            self.assertEqual((status, report.get("error_code")), (413, "file_too_large"))
            status, report = request("download", {**self.body, "path": "/large.pptx", "max_bytes": (64 << 20)+1})
            self.assertEqual((status, report.get("error_code")), (400, "invalid_integer"))
            with self.assertRaisesRegex(Failure, "payload_too_large"):
                self.adapter.dispatch("upload", {**self.body, "path": "/too-big",
                    "data_base64": base64.b64encode(content + b"x").decode()})
            connection = http.client.HTTPConnection("127.0.0.1", server.server_port, timeout=5)
            try:
                connection.putrequest("POST", "/v1/upload")
                connection.putheader("Authorization", "Bearer " + "t" * 32)
                connection.putheader("Content-Length", str((96 << 20)+1))
                connection.endheaders()
                response = connection.getresponse()
                self.assertEqual(response.status, 413)
                response.read()
            finally:
                connection.close()
        finally:
            server.shutdown()
            worker.join()
            server.server_close()

    def test_download_failure_keeps_safe_diagnostic_without_retry_or_body(self):
        secret = "fixture-secret-must-not-appear"
        error = RuntimeError("https://user:" + secret + "@provider.invalid/private-file?token=" + secret)
        error.status_code = HTTPStatus.BAD_GATEWAY
        server = make_server(self.adapter, "t" * 32, ("127.0.0.1", 0))
        worker = threading.Thread(target=server.serve_forever)
        worker.start()
        try:
            for failure, status, code, provider in [
                (error, 502, "sdk_operation_failed", 502),
                (TimeoutError(secret), 502, "sdk_operation_failed", None),
                (Failure("file_too_large", 413), 413, "file_too_large", None),
                (ValueError(secret), 400, "invalid_json_request", None),
            ]:
                with self.subTest(code=code), patch.object(self.adapter, "dispatch", side_effect=failure) as dispatch, patch("sys.stderr", new_callable=io.StringIO) as log:
                    connection = http.client.HTTPConnection("127.0.0.1", server.server_port, timeout=5)
                    try:
                        connection.request("POST", "/v1/download", json.dumps({"path": "/private-file", "secret": secret}),
                                           {"Authorization": "Bearer " + "t" * 32})
                        response = connection.getresponse()
                        body = response.read()
                        self.assertEqual((response.status, json.loads(body)["error_code"]), (status, code))
                        dispatch.assert_called_once()
                        self.assertEqual(json.loads(log.getvalue()), {
                            "event": "adapter_download_failed",
                            "exception_type": type(failure).__module__ + "." + type(failure).__name__,
                            "provider_http_status": provider, "adapter_http_status": status, "error_code": code})
                        for private in [secret, "private-file", "provider.invalid", "t" * 32]:
                            self.assertNotIn(private, log.getvalue())
                            self.assertNotIn(private.encode(), body)
                    finally:
                        connection.close()
        finally:
            server.shutdown()
            worker.join()
            server.server_close()

    def test_guest_is_private_and_metrics_missing_not_zero(self):
        self.create()
        self.assertIn("e2b-traffic-access-token", self.adapter.dispatch("guest", {**self.body, "port": 18789})["headers"])
        metrics = self.adapter.dispatch("metrics", self.body)
        self.assertEqual(metrics["metrics"], [])
        self.assertIn("network_io", metrics["unsupported"])

    def test_durable_recovery_and_unknown_block(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "adapter.state.json"
            self.adapter.state_file = path
            self.create()
            self.assertEqual(path.stat().st_mode & 0o777, 0o600)
            fresh = Adapter(self.sdk, Missing, FileMissing, ProcessMissing, templates=["pinned"], state_file=path)
            self.assertEqual(fresh.records[self.body["sandbox"]]["state"], "absent")
            fresh.state_lock.close()
            saved = json.loads(path.read_text()); saved["records"]["pending"] = {"state": "allocating"}; path.write_text(json.dumps(saved))
            with self.assertRaisesRegex(Failure, "manual_reconciliation"): Adapter(self.sdk, Missing, FileMissing, ProcessMissing, templates=["pinned"], state_file=path)

    def test_recovery_cleans_later_ids_even_when_an_earlier_record_is_unresolved(self):
        for unknown in (True, False):
            with self.subTest(unknown=unknown), tempfile.TemporaryDirectory() as directory:
                remaining = {"blocked", "removable"}
                deleted = []

                class RecoverySDK:
                    def kill(self, identity, **_):
                        deleted.append(identity)
                        if identity == "blocked":
                            raise TimeoutError("private failure")
                        remaining.discard(identity)

                    def get_info(self, identity, **_):
                        if identity not in remaining:
                            raise Missing()
                        return object()

                path = Path(directory) / "adapter.state.json"
                first = {"state": "allocation_unconfirmed"} if unknown else {"state": "active", "cloud_id": "blocked"}
                path.write_text(json.dumps({"schema": 1, "creates": 2, "records": {
                    "first": first, "later": {"state": "active", "cloud_id": "removable"}}}))
                code = "manual_reconciliation" if unknown else "recovery_absence_unconfirmed"
                with self.assertRaisesRegex(Failure, code):
                    Adapter(RecoverySDK(), Missing, FileMissing, ProcessMissing, templates=["pinned"], state_file=path)
                saved = json.loads(path.read_text())
                self.assertEqual(saved["creates"], 2)
                self.assertEqual(saved["records"]["later"]["state"], "absent")
                self.assertNotEqual(saved["records"]["first"]["state"], "absent")
                self.assertIn("removable", deleted)

    def test_create_finishing_after_close_is_deleted_or_stays_owned(self):
        for fail_kill in (False, True):
            with self.subTest(fail_kill=fail_kill):
                entered, release = threading.Event(), threading.Event()
                sdk = SDK()
                sdk.fail_kill = fail_kill
                create = sdk.create

                def delayed_create(**options):
                    entered.set()
                    if not release.wait(2):
                        raise TimeoutError("test did not release create")
                    return create(**options)

                sdk.create = delayed_create
                adapter = Adapter(sdk, Missing, FileMissing, ProcessMissing, templates=["pinned"])
                failures = []

                def allocate():
                    try:
                        adapter.create(self.body)
                    except Failure as error:
                        failures.append(error.code)

                worker = threading.Thread(target=allocate)
                worker.start()
                try:
                    self.assertTrue(entered.wait(2))
                    self.assertEqual(adapter.close(), [self.body["sandbox"]])
                finally:
                    release.set()
                    worker.join(2)
                self.assertFalse(worker.is_alive())
                self.assertEqual(adapter.creates, 1)
                state = adapter.records[self.body["sandbox"]]["state"]
                if fail_kill:
                    self.assertEqual(failures, ["absence_unconfirmed"])
                    self.assertEqual(state, "stopping")
                    self.assertIsNotNone(sdk.live)
                    sdk.fail_kill = False
                    self.assertEqual(adapter.close(), [])
                else:
                    self.assertEqual(failures, ["adapter_stopping"])
                    self.assertEqual(state, "absent")
                    self.assertIsNone(sdk.live)

    def test_http_auth_and_redacted_failure(self):
        server = make_server(self.adapter, "t" * 32, ("127.0.0.1", 0))
        worker = threading.Thread(target=server.serve_forever); worker.start()
        try:
            connection = http.client.HTTPConnection("127.0.0.1", server.server_port, timeout=2)
            connection.request("POST", "/v1/create", json.dumps(self.body))
            response = connection.getresponse()
            self.assertEqual(response.status, 401)
            response.read(); connection.close()
            self.sdk.fail_create = True
            connection = http.client.HTTPConnection("127.0.0.1", server.server_port, timeout=2)
            connection.request("POST", "/v1/create", json.dumps(self.body), {"Authorization": "Bearer " + "t" * 32})
            response = connection.getresponse()
            self.assertEqual(response.status, 502)
            content = response.read()
            self.assertNotIn(b"secret-value", content)
            measured = json.loads(content)["sdk_calls"]
            self.assertEqual(measured[0]["operation"], "sandbox.create")
            self.assertEqual(measured[0]["status"], "failed")
            connection.close()
        finally:
            server.shutdown(); worker.join(); server.server_close()

    def test_real_local_guest_wrapper_binary_argv_stdin_and_exit(self):
        argv = [sys.executable, "-c", "import sys;sys.stdout.buffer.write(sys.stdin.buffer.read()+sys.argv[1].encode());sys.stderr.buffer.write(b'\\x00\\xff');sys.exit(9)", "' $HOME `id`"]
        spec = base64.b64encode(json.dumps({"argv": argv, "cwd": "/tmp", "env": {}}).encode()).decode()
        run = subprocess.run([sys.executable, "-c", GUEST_EXEC, spec], input=bytes(range(256)), capture_output=True, timeout=5)
        self.assertEqual(run.returncode, 0, run.stderr)
        frames = [json.loads(line) for line in run.stdout.splitlines()]
        output = b"".join(base64.b64decode(f["data_base64"]) for f in frames if f.get("stream") == "stdout")
        self.assertEqual(output, bytes(range(256)) + argv[-1].encode())
        self.assertEqual(frames[-1]["exit_code"], 9)
        self.assertGreaterEqual(frames[-1]["guest_duration_ns"], 0)
        self.assertGreaterEqual(frames[-1]["popen_return_ns"], 0)
        self.assertGreaterEqual(frames[-1]["first_output_ns"], frames[-1]["popen_return_ns"])
        self.assertGreaterEqual(frames[-1]["child_exit_observed_ns"], 0)
        self.assertGreaterEqual(frames[-1]["pipe_eof_ns"], frames[-1]["first_output_ns"])

    def test_guest_wall_time_includes_descendant_pipe_drain(self):
        # The direct child exits immediately; its descendant retains stdout.
        program = "import os,time; pid=os.fork(); time.sleep(.1) if pid == 0 else None; os._exit(0)"
        spec = base64.b64encode(json.dumps({"argv": [sys.executable, "-c", program], "cwd": "/tmp", "env": {}}).encode()).decode()
        run = subprocess.run([sys.executable, "-c", GUEST_EXEC, spec], capture_output=True, timeout=5)
        self.assertEqual(run.returncode, 0, run.stderr)
        terminal = json.loads(run.stdout.splitlines()[-1])
        self.assertEqual(terminal["exit_code"], 0)
        self.assertGreaterEqual(terminal["guest_duration_ns"], 80_000_000)
        self.assertLess(terminal["child_exit_observed_ns"], terminal["pipe_eof_ns"])
        self.assertLessEqual(terminal["pipe_eof_ns"], terminal["guest_duration_ns"])

    def test_guest_boundaries_mark_no_output_without_inventing_first_byte(self):
        spec = base64.b64encode(json.dumps({"argv": ["true"], "cwd": "/tmp", "env": {}}).encode()).decode()
        run = subprocess.run([sys.executable, "-c", GUEST_EXEC, spec], capture_output=True, timeout=5)
        self.assertEqual(run.returncode, 0, run.stderr)
        terminal = json.loads(run.stdout.splitlines()[-1])
        self.assertIsNone(terminal["first_output_ns"])
        self.assertGreaterEqual(terminal["child_exit_observed_ns"], terminal["popen_return_ns"])

    def test_guest_signal_exit_is_reported_without_rewriting_status(self):
        program = "import os,signal; os.kill(os.getpid(), signal.SIGTERM)"
        spec = base64.b64encode(json.dumps({"argv": [sys.executable, "-c", program], "cwd": "/tmp", "env": {}}).encode()).decode()
        run = subprocess.run([sys.executable, "-c", GUEST_EXEC, spec], capture_output=True, timeout=5)
        self.assertEqual(run.returncode, 0, run.stderr)
        terminal = json.loads(run.stdout.splitlines()[-1])
        self.assertEqual(terminal["exit_code"], -15)
        self.assertGreaterEqual(terminal["guest_duration_ns"], terminal["child_exit_observed_ns"])

    def test_guest_delayed_first_output_and_stdout_stderr_boundaries(self):
        # A delayed first byte must remain observable as a real same-guest
        # boundary.  The wrapper must drain both independent streams without
        # treating the later stderr byte as the first stdout byte.
        program = (
            "import sys,time; time.sleep(.08); "
            "sys.stdout.buffer.write(b'out\\x00'); sys.stdout.flush(); "
            "time.sleep(.04); sys.stderr.buffer.write(b'err\\xff'); "
            "sys.stderr.flush()"
        )
        spec = base64.b64encode(json.dumps({
            "argv": [sys.executable, "-c", program], "cwd": "/tmp", "env": {}
        }).encode()).decode()
        run = subprocess.run([sys.executable, "-c", GUEST_EXEC, spec], capture_output=True, timeout=5)
        self.assertEqual(run.returncode, 0, run.stderr)
        frames = [json.loads(line) for line in run.stdout.splitlines()]
        payload = [(frame["stream"], base64.b64decode(frame["data_base64"]))
                   for frame in frames if "stream" in frame]
        self.assertEqual(b"".join(data for stream, data in payload if stream == "stdout"), b"out\0")
        self.assertEqual(b"".join(data for stream, data in payload if stream == "stderr"), b"err\xff")
        terminal = frames[-1]
        self.assertIsNotNone(terminal["first_output_ns"])
        self.assertGreaterEqual(terminal["first_output_ns"], terminal["popen_return_ns"])
        self.assertGreaterEqual(terminal["pipe_eof_ns"], terminal["first_output_ns"])
        self.assertGreaterEqual(terminal["guest_duration_ns"], terminal["pipe_eof_ns"])

    def test_guest_long_stdout_and_stderr_do_not_deadlock_or_truncate(self):
        # Each stream is larger than a typical OS pipe buffer.  Reading both
        # descriptors through selectors is required; sequential reads would
        # deadlock when the child fills the other descriptor.
        program = (
            "import sys; out=b'A'*(512*1024); err=b'B'*(512*1024); "
            "sys.stdout.buffer.write(out); sys.stdout.flush(); "
            "sys.stderr.buffer.write(err); sys.stderr.flush()"
        )
        spec = base64.b64encode(json.dumps({
            "argv": [sys.executable, "-c", program], "cwd": "/tmp", "env": {}
        }).encode()).decode()
        run = subprocess.run([sys.executable, "-c", GUEST_EXEC, spec], capture_output=True, timeout=10)
        self.assertEqual(run.returncode, 0, run.stderr[:200])
        frames = [json.loads(line) for line in run.stdout.splitlines()]
        stdout = b"".join(base64.b64decode(frame["data_base64"])
                           for frame in frames if frame.get("stream") == "stdout")
        stderr = b"".join(base64.b64decode(frame["data_base64"])
                           for frame in frames if frame.get("stream") == "stderr")
        self.assertEqual(stdout, b"A" * (512 * 1024))
        self.assertEqual(stderr, b"B" * (512 * 1024))
        self.assertEqual(frames[-1]["exit_code"], 0)

    def test_guest_exec_exception_has_no_invented_terminal(self):
        # Popen failure is a wrapper failure.  It must not be turned into a
        # successful or guessed child exit frame; stderr remains diagnostic
        # transport output for the adapter to classify as a failed stream.
        spec = base64.b64encode(json.dumps({
            "argv": ["/definitely/missing/aries-r07-program"],
            "cwd": "/tmp", "env": {}
        }).encode()).decode()
        run = subprocess.run([sys.executable, "-c", GUEST_EXEC, spec], capture_output=True, timeout=5)
        self.assertNotEqual(run.returncode, 0)
        self.assertNotIn(b'"exit_code"', run.stdout)
        self.assertIn(b"No such file", run.stderr)

    def test_adapter_transport_timeout_or_cancel_is_not_guest_completion(self):
        # A lost/timeout SDK stream is classified as transport failure and the
        # adapter explicitly reports termination unconfirmed.  There is no
        # guessed exit code, guest duration, or descendant terminal state.
        class InterruptedNative(Native):
            def __iter__(self):
                raise TimeoutError("command stream deadline exceeded")

        job = Job(InterruptedNative([]), 1024)
        self.adapter.drain(job)
        self.assertTrue(job.done)
        terminal = job.events[-1]
        self.assertEqual(terminal["error_code"], "process_transport_failed")
        self.assertFalse(terminal["termination_confirmed"])
        self.assertNotIn("exit_code", terminal)
        self.assertNotIn("guest_duration_ns", terminal)

    def test_linked_observation_is_strict_and_bound_to_process(self):
        good = linked_observation()
        self.assertEqual(validate_observation(good, "process1"), good)
        invalid = [None, {}, {**good, "extra": 1}, {**good, "schema_version": True},
                   {**good, "schema_version": 1.0}, {**good, "native_identity": None},
                   {**good, "process_id": "another-process"}]
        for field in good["native_identity"]:
            value = copy.deepcopy(good)
            del value["native_identity"][field]
            invalid.append(value)
        for field in ("execution_id", "process_id"):
            for text in ("", " leading", "trailing ", "line\nfeed", "\x85", "x" * 257, "\ud800"):
                invalid.append({**good, field: text})
        for text in (None, True, "", "line\x00feed", "汉" * 86):
            value = copy.deepcopy(good)
            value["native_identity"]["tool_call_id"] = text
            invalid.append(value)
        for value in invalid:
            with self.subTest(value=repr(value)), self.assertRaises((ValueError, TypeError)):
                validate_observation(value, "process1")
        unicode_identity = copy.deepcopy(good)
        unicode_identity["native_identity"]["tool_call_id"] = "汉" * 85
        self.assertEqual(validate_observation(unicode_identity), unicode_identity)

    def test_linked_start_keeps_identity_out_of_argv_env_and_stdin(self):
        self.create()
        observation = linked_observation()
        frames = [identity_frame(observation), {"exit_code": 0, "guest_duration_ns": 1,
                                                "observation": observation}]
        self.sdk.live.commands.native = Native(frames)
        command = {**self.body, "process": "process1", "argv": ["printf", "literal $HOME"],
                   "cwd": "/tmp", "env": {"EXACT": "value"}, "observation": observation}
        response = self.adapter.start(command)
        self.assertEqual(response["observation"], observation)
        wrapper = shlex.split(self.sdk.live.commands.command)
        spec = strict_json_loads(base64.b64decode(wrapper[-1]))
        self.assertEqual(spec, {key: command[key] for key in ("argv", "cwd", "env", "observation")})
        job = self.adapter.jobs[self.body["sandbox"]]["process1"]
        with job.condition:
            self.assertTrue(job.condition.wait_for(lambda: job.done, timeout=2))
        observation["native_identity"]["tool_call_id"] = "mutated-caller"
        self.assertEqual(job.observation["native_identity"]["tool_call_id"], "tool1")
        for action in ("events", "process", "kill_process"):
            self.assertEqual(self.adapter.dispatch(action, command)["observation"], job.observation)
        self.assertEqual(job.events[0]["kind"], "execution_identity")
        self.assertEqual(job.events[-1]["exit_code"], 0)
        self.assertEqual(job.events[-1]["observation"], job.observation)

    def test_invalid_linked_start_fails_before_sdk_submission(self):
        self.create()
        for observation in (None, {}, {**linked_observation(), "process_id": "wrong"}):
            with self.subTest(observation=observation), self.assertRaisesRegex(Failure, "invalid_observation"):
                self.adapter.start({**self.body, "process": "process1", "argv": ["true"],
                                    "cwd": "/tmp", "observation": observation})
        self.assertFalse(hasattr(self.sdk.live.commands, "command"))
        self.assertEqual(self.adapter.jobs[self.body["sandbox"]], {})

    def test_linked_stdin_response_echoes_fixed_job_identity(self):
        self.create()
        observation = linked_observation()
        native = Native([])
        job = Job(native, 1024, observation)
        self.adapter.jobs[self.body["sandbox"]]["process1"] = job
        response = self.adapter.dispatch("stdin", {**self.body, "process": "process1",
            "data_base64": "AP8=", "close": True})
        self.assertEqual(response["observation"], observation)
        self.assertEqual(native.input, b"\0\xff")
        self.assertTrue(native.closed)
        response["observation"]["native_identity"]["tool_call_id"] = "mutated-response"
        self.assertEqual(job.observation, observation)

    def test_only_eof_process_not_found_is_stdin_closed_without_fabricating_terminal(self):
        self.create()
        process = {**self.body, "process": "process1", "close": True}
        cases = [("close_stdin", ProcessMissing()), ("close_stdin", TimeoutError()),
                 ("close_stdin", PermissionError()), ("send_stdin", ProcessMissing())]
        for method, error in cases:
            with self.subTest(method=method, error=type(error).__name__):
                native = Native([])
                job = Job(native, 1024)
                self.adapter.jobs[self.body["sandbox"]]["process1"] = job
                body = {**process, "data_base64": "AP8=" if method == "send_stdin" else ""}
                eof_missing = method == "close_stdin" and isinstance(error, ProcessMissing)
                with patch.object(native, method, side_effect=error), self.assertRaises(Failure if eof_missing else type(error)) as raised:
                    self.adapter.dispatch("stdin", body)
                if eof_missing:
                    self.assertEqual((raised.exception.code, raised.exception.status), ("stdin_closed", 409))
                else:
                    self.assertIs(raised.exception, error)
                self.assertFalse(job.done)
                self.assertFalse(job.stdin_closed)
                self.assertFalse(native.closed)
                self.assertEqual(job.events, [])
                job.finish({"exit_code": 7})
                response = self.adapter.dispatch("events", process)
                self.assertTrue(response["done"])
                self.assertEqual(response["events"][-1]["exit_code"], 7)

    def test_linked_guest_receipt_is_required_once_and_every_frame_is_verified(self):
        observation = linked_observation()
        receipt = identity_frame(observation)
        output = {"stream": "stdout", "data_base64": "AP8=", "observation": observation}
        terminal = {"exit_code": 7, "guest_duration_ns": 1, "observation": observation}
        job = Job(Native([receipt, output, terminal]), 1024, observation)
        self.adapter.drain(job)
        self.assertEqual(job.events[0], receipt)
        self.assertEqual(job.events[1], output)
        self.assertEqual(job.events[-1]["exit_code"], 7)
        invalid_streams = [[output, receipt, terminal], [terminal], [receipt, receipt, terminal],
                           [receipt, {"stream": "stdout", "data_base64": "AP8="}, terminal],
                           [receipt, output, {"exit_code": 7, "guest_duration_ns": 1}],
                           [{**receipt, "phase": "after_popen"}, terminal]]
        for field in ("scope_id", "agent_session_id", "tool_call_id", "native_invocation_id", "ssh_request_id"):
            wrong = copy.deepcopy(observation)
            wrong["native_identity"][field] += "-conflict"
            invalid_streams.append([receipt, {**output, "observation": wrong}, terminal])
        for field in ("execution_id", "process_id"):
            invalid_streams.append([receipt, output, {**terminal, "observation": {**observation, field: "wrong"}}])
        for frames in invalid_streams:
            with self.subTest(frames=frames):
                job = Job(Native(frames), 1024, observation)
                self.adapter.drain(job)
                self.assertIn("error_code", job.events[-1])
                self.assertFalse(job.events[-1]["termination_confirmed"])
                self.assertEqual(job.events[-1]["observation"], observation)
                self.assertNotIn("exit_code", job.events[-1])
                self.assertNotIn("guest_duration_ns", job.events[-1])
        legacy = Job(Native([receipt, output, terminal]), 1024)
        self.adapter.drain(legacy)
        self.assertEqual(legacy.events[-1]["error_code"], "invalid_guest_frame")

    def test_duplicate_json_keys_are_rejected_at_request_spec_and_frame_boundaries(self):
        class RawNative(Native):
            def __iter__(self):
                yield '{"exit_code":7,"exit_code":0,"guest_duration_ns":1}\n', None, None

        job = Job(RawNative([]), 1024)
        self.adapter.drain(job)
        self.assertEqual(job.events[-1]["error_code"], "invalid_guest_frame")
        for raw in ('{"argv":[],"argv":["true"]}',
                    '{"observation":{"schema_version":1,"schema_version":1}}',
                    '{"observation":{"native_identity":{"tool_call_id":"a","tool_call_id":"b"}}}'):
            with self.subTest(raw=raw), self.assertRaisesRegex(ValueError, "duplicate_json_key"):
                strict_json_loads(raw)
        server = make_server(self.adapter, "t" * 32, ("127.0.0.1", 0))
        worker = threading.Thread(target=server.serve_forever)
        worker.start()
        try:
            connection = http.client.HTTPConnection("127.0.0.1", server.server_port, timeout=2)
            connection.request("POST", "/v1/health", '{"observation":null,"observation":{}}',
                               {"Authorization": "Bearer " + "t" * 32})
            response = connection.getresponse()
            self.assertEqual(response.status, 400)
            self.assertEqual(json.loads(response.read())["error_code"], "invalid_json_request")
            connection.close()
        finally:
            server.shutdown(); worker.join(); server.server_close()

    def test_real_guest_identity_frames_preserve_binary_command_semantics(self):
        observation = linked_observation()
        program = "import os,sys;sys.stdout.buffer.write(sys.stdin.buffer.read()+sys.argv[1].encode()+os.environ['EXACT'].encode());sys.stderr.buffer.write(b'\\x00\\xff');sys.exit(9)"
        spec = {"argv": [sys.executable, "-c", program, "literal $HOME ' `id`"],
                "cwd": "/tmp", "env": {"EXACT": "value"}}
        results = []
        for linked in (False, True):
            request = {**spec, "observation": observation} if linked else spec
            encoded = base64.b64encode(json.dumps(request).encode()).decode()
            run = subprocess.run([sys.executable, "-c", GUEST_EXEC, encoded], input=bytes(range(256)),
                                 capture_output=True, timeout=5)
            self.assertEqual(run.returncode, 0, run.stderr)
            frames = [strict_json_loads(line) for line in run.stdout.splitlines()]
            if linked:
                self.assertEqual(frames[0], identity_frame(observation))
                self.assertTrue(all(frame["observation"] == observation for frame in frames))
                job = Job(Native(frames), 1024, observation)
                self.adapter.drain(job)
                self.assertEqual(job.events[0], frames[0])
                self.assertEqual(job.events[-1]["exit_code"], 9)
            results.append(({stream: b"".join(base64.b64decode(frame["data_base64"])
                            for frame in frames if frame.get("stream") == stream)
                            for stream in ("stdout", "stderr")}, frames[-1]["exit_code"]))
        self.assertEqual(results[0], results[1])

    def test_guest_invalid_identity_does_not_launch_child(self):
        with tempfile.TemporaryDirectory() as directory:
            marker = Path(directory) / "launched"
            spec = {"argv": [sys.executable, "-c", "from pathlib import Path;Path('launched').touch()"],
                    "cwd": directory, "env": {}, "observation": None}
            for raw in (json.dumps(spec), json.dumps(spec).replace('"observation": null', '"observation": null,"observation": {}')):
                encoded = base64.b64encode(raw.encode()).decode()
                run = subprocess.run([sys.executable, "-c", GUEST_EXEC, encoded], capture_output=True, timeout=5)
                self.assertNotEqual(run.returncode, 0)
                self.assertEqual(run.stdout, b"")
                self.assertFalse(marker.exists())


if __name__ == "__main__": unittest.main()

"""Small, authenticated, single-owner adapter for the pinned official E2B SDK.

SDK imports are lazy so offline unit tests do not need the SDK or credentials.
No operation retries an ambiguous command or allocation.
"""
import argparse
import base64
from contextlib import contextmanager
import dataclasses
import datetime
import fcntl
import hashlib
import hmac
import json
import os
from pathlib import Path
import re
import shlex
import signal
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from guest_exec import strict_json_loads, validate_observation

SDK_VERSION = "2.58.0"
MAX_BODY = 96 << 20  # Includes base64 expansion and JSON metadata.
MAX_FILE = 64 << 20
GUEST_EXEC = Path(__file__).with_name("guest_exec.py").read_text()


class Failure(Exception):
    def __init__(self, code, status=400):
        self.code, self.status = code, status
        super().__init__(code)


def create_failure_diagnostic(error):
    """Private-ledger diagnostic, never a provider response/header/body dump.

    e2b 2.58.0 uses ``status: message`` for parsed API errors, but embeds the
    entire bytes body when parsing fails. Only its single-line message form is
    eligible for redaction. Other exception messages are not safe to persist.
    """
    status = getattr(error, "status_code", None)
    status = int(status) if isinstance(status, int) and not isinstance(status, bool) and 100 <= status <= 599 else None
    secrets = [value for name, value in os.environ.items() if value and
               re.search(r"KEY|TOKEN|PASSWORD|SECRET|CREDENTIAL", name, re.I)]
    request_id = getattr(error, "request_id", None)
    if (not isinstance(request_id, str) or
            not re.fullmatch(r"[a-zA-Z0-9._:-]{1,128}", request_id) or
            any(value in request_id for value in secrets)):
        request_id = None
    result = {"exception_type": type(error).__module__ + "." + type(error).__name__,
              "http_status": status, "request_id": request_id,
              "message": None, "message_state": "omitted_unstructured_exception"}
    if isinstance(error, Failure):
        result.update(message=error.code, message_state="adapter_error_code")
        return result
    if type(error).__module__ != "e2b.exceptions":
        return result
    raw = error.args[0] if len(error.args) == 1 and isinstance(error.args[0], str) else ""
    # Avoid serialised bodies, byte literals, HTML and multi-line header dumps.
    message = re.sub(r"^\d{3}:\s*", "", raw)
    if (not message or len(message) > 4096 or
            any(c in message for c in "\r\n\x00{}[]<>") or
            message.startswith(("b'", 'b"'))):
        result["message_state"] = "omitted_body_or_nontext_message"
        return result
    for value in sorted(secrets, key=len, reverse=True):
        message = message.replace(value, "[REDACTED]")
    message = re.sub(r"https?://\S+", "[REDACTED_URL]", message, flags=re.I)
    message = re.sub(r"\b(?:Bearer|Basic)\s+[^\s,;]+", "[REDACTED_AUTH]", message, flags=re.I)
    message = re.sub(
        r"\b(?:authorization|[a-z0-9_-]*(?:key|token|password|secret|credential))"
        r"\s*[:=]\s*(?:\"[^\"]*\"|'[^']*'|[^\s,;]+)",
        "[REDACTED_CREDENTIAL]", message, flags=re.I)
    result.update(message=message[:512], message_state="sdk_message_redacted")
    return result


def integer(value, low, high):
    if type(value) is not int or not low <= value <= high:
        raise Failure("invalid_integer")
    return value


def string(value, limit=4096):
    if not isinstance(value, str) or not value or "\0" in value or len(value) > limit:
        raise Failure("invalid_string")
    return value


def decode(value, limit=MAX_FILE):
    if not isinstance(value, str) or len(value) > ((limit + 2) // 3) * 4:
        raise Failure("payload_too_large", 413)
    try:
        result = base64.b64decode(value, validate=True)
    except (ValueError, TypeError):
        raise Failure("invalid_base64") from None
    if len(result) > limit:
        raise Failure("payload_too_large", 413)
    return result


class Job:
    def __init__(self, handle, output_limit, observation=None):
        self.handle = handle
        self.output_limit = output_limit
        self.condition = threading.Condition()
        self.events = []
        self.bytes = 0
        self.done = False
        self.stdin_closed = False
        self.released = False
        self.input_lock = threading.Lock()
        self.started = time.monotonic_ns()
        self.observation = validate_observation(observation) if observation is not None else None

    def binding(self):
        if self.observation is None:
            return {}
        return {"observation": {**self.observation,
                                "native_identity": dict(self.observation["native_identity"])}}

    def append(self, event):
        with self.condition:
            self.events.append(event)
            self.condition.notify_all()

    def finish(self, event):
        with self.condition:
            self.events.append({**event, "adapter_duration_ns": time.monotonic_ns() - self.started})
            self.done = True
            self.condition.notify_all()


class Adapter:
    provider = "e2b-cloud"
    sdk_pinned_version = SDK_VERSION

    def __init__(self, sdk, not_found, file_not_found, process_not_found, *, templates, max_sandboxes=2,
                 max_processes=8, max_timeout=900, request_timeout=30,
                 state_file=None, sdk_version=None):
        self.sdk, self.not_found, self.file_not_found = sdk, not_found, file_not_found
        self.process_not_found = process_not_found
        self.templates = frozenset(templates)
        self.max_sandboxes = max_sandboxes
        self.max_processes, self.max_timeout = max_processes, max_timeout
        self.request_timeout = request_timeout
        self.sdk_version = sdk_version or self.sdk_pinned_version
        self.measurements = threading.local()
        self.lock = threading.RLock()
        self.records = {}
        self.deletions = {}
        self.live = {}
        self.jobs = {}
        self.creates = 0
        self.retained_output_bytes = 0
        self.stopping = False
        self.state_file = Path(state_file) if state_file else None
        self.state_lock = None
        if self.state_file:
            self.state_file.parent.mkdir(parents=True, exist_ok=True)
            self.state_lock = open(str(self.state_file) + ".lock", "a")
            os.chmod(self.state_lock.name, 0o600)
            fcntl.flock(self.state_lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            if self.state_file.exists():
                try:
                    saved = json.loads(self.state_file.read_text())
                    if saved.get("provider", "e2b-cloud") != self.provider:
                        raise Failure("ledger_provider_mismatch", 503)
                    self.records, self.creates = saved["records"], saved["creates"]
                    self.recover()
                except Exception:
                    self.state_lock.close()
                    raise

    @contextmanager
    def sdk_measurement(self, operation):
        started = time.monotonic_ns()
        status = "succeeded"
        try:
            yield
        except BaseException:
            status = "failed"
            raise
        finally:
            calls = getattr(self.measurements, "calls", None)
            if calls is not None:
                calls.append({"operation": operation, "duration_ns": time.monotonic_ns() - started,
                              "status": status})

    def sdk_call(self, operation, function, *args, **kwargs):
        with self.sdk_measurement(operation):
            return function(*args, **kwargs)

    def persist(self):
        if not self.state_file:
            return
        temporary = self.state_file.with_suffix(self.state_file.suffix + ".tmp")
        fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
        with os.fdopen(fd, "w") as out:
            saved = {"schema": 1, "creates": self.creates, "records": self.records}
            # Keep existing E2B ledgers byte-compatible; other providers must
            # carry a discriminator before restart cleanup can touch them.
            if self.provider != "e2b-cloud":
                saved["provider"] = self.provider
            json.dump(saved, out)
            out.flush()
            os.fsync(out.fileno())
        os.replace(temporary, self.state_file)

    def recover(self):
        # Recover known identities by deletion, never by reconnecting/extending TTL.
        unknown = unconfirmed = False
        for handle, rec in self.records.items():
            if rec["state"] == "absent":
                continue
            if not rec.get("cloud_id"):
                unknown = True
                continue
            try:
                self.kill({"sandbox": handle})
            except Exception:
                unconfirmed = True
        if unknown:
            raise Failure("allocation_unconfirmed_manual_reconciliation_required", 503)
        if unconfirmed:
            raise Failure("recovery_absence_unconfirmed", 503)

    def entry(self, body):
        handle = string(body.get("sandbox"), 128)
        with self.lock:
            if handle not in self.records:
                raise Failure("unknown_sandbox", 404)
            if self.records[handle]["state"] != "active" or handle not in self.live:
                raise Failure("sandbox_not_active", 409)
            return handle, self.live[handle]

    def validate_network(self, internet):
        pass

    def remember_cloud_id(self, handle, cloud_id):
        # A provider may need to construct its SDK transport after allocation.
        # Retain deletion ownership before that construction can fail.
        with self.lock:
            self.records[handle]["cloud_id"] = string(cloud_id, 512)
            self.persist()

    def create_sandbox(self, handle, template, timeout, internet):
        return self.sdk_call("sandbox.create", self.sdk.create, template=template, timeout=timeout,
            metadata={"aries_adapter_handle": handle}, allow_internet_access=internet,
            network={"allow_public_traffic": False}, lifecycle={"on_timeout": "kill", "auto_resume": False},
            # The pinned SDK's default transport interrupted real native Trace
            # downloads; its public HTTP/1.1 connection option completed them.
            http_version="1.1", request_timeout=self.request_timeout)

    def runtime_metadata(self):
        metadata = {"provider": self.provider, "sdk_version": self.sdk_version,
                    "sdk_pinned_version": self.sdk_pinned_version}
        if self.provider == "e2b-cloud":
            metadata["sandbox_connection_http_version"] = "1.1"
        return metadata

    def create(self, body):
        handle = string(body.get("sandbox"), 128)
        if not re.fullmatch(r"[a-zA-Z0-9_-]{8,128}", handle):
            raise Failure("invalid_sandbox_handle")
        template = string(body.get("template"))
        if template not in self.templates:
            raise Failure("template_not_allowed", 403)
        timeout = integer(body.get("timeout_seconds", 300), 1, self.max_timeout)
        internet = body.get("allow_internet_access", False)
        if type(internet) is not bool:
            raise Failure("invalid_network_policy")
        self.validate_network(internet)
        with self.lock:
            if handle in self.records:
                raise Failure("duplicate_create_no_retry", 409)
            if any(r["state"] == "allocation_unconfirmed" or
                   (r["state"] == "stopping" and
                    (not self.deletions.get(h) or self.deletions[h]["failed"]))
                   for h, r in self.records.items()):
                raise Failure("cleanup_required_before_create", 409)
            active = sum(r["state"] != "absent" for r in self.records.values())
            if self.stopping or active >= self.max_sandboxes:
                raise Failure("local_quota_exceeded", 429)
            self.creates += 1
            self.records[handle] = {"state": "allocating", "template": template}
            self.persist()
        try:
            sandbox = self.create_sandbox(handle, template, timeout, internet)
        except Exception as error:
            with self.lock:
                known = bool(self.records[handle].get("cloud_id"))
                self.records[handle]["state"] = "stopping" if known else "allocation_unconfirmed"
                self.records[handle]["create_failure"] = create_failure_diagnostic(error)
                self.persist()
            raise Failure("sandbox_connection_failed" if known else "allocation_unconfirmed", 502) from None
        with self.lock:
            self.records[handle].update(state="active", cloud_id=sandbox.sandbox_id)
            self.live[handle] = sandbox
            self.jobs[handle] = {}
            self.persist()
            stopping = self.stopping
        if stopping:
            self.kill({"sandbox": handle})
            raise Failure("adapter_stopping", 503)
        return {"sandbox": handle, "state": "active", **self.runtime_metadata(),
                "cloud_id_sha256": hashlib.sha256(sandbox.sandbox_id.encode()).hexdigest(),
                "template": template, "timeout_seconds": timeout,
                "resources": None, "region": None, "cold_warm": "unknown"}

    def kill(self, body):
        handle = string(body.get("sandbox"), 128)
        registered = False
        try:
            with self.lock:
                rec = self.records.get(handle)
                if rec is None:
                    raise Failure("unknown_sandbox", 404)
                if rec["state"] == "absent":
                    return {"absence_confirmed": True}
                if not rec.get("cloud_id"):
                    raise Failure("allocation_unconfirmed", 409)
                # Only normal in-flight deletion allows spare-capacity creates.
                # A prior failure or recovered stopping record stays blocked
                # through retries until control-plane absence is confirmed.
                deletion = self.deletions.setdefault(handle, {"active": 0, "failed": rec["state"] == "stopping"})
                deletion["active"] += 1
                registered = True
                rec["state"] = "stopping"
                self.persist()
                cloud_id = rec["cloud_id"]
            # A failed delete does not erase ownership. Only control-plane
            # NotFound confirms absence, including when kill itself timed out.
            try:
                self.sdk_call("sandbox.kill", self.sdk.kill, cloud_id, request_timeout=self.request_timeout)
            except Exception:
                pass
            try:
                self.sdk_call("sandbox.get_info", self.sdk.get_info, cloud_id, request_timeout=self.request_timeout)
            except self.not_found:
                with self.lock:
                    rec["state"] = "absent"
                    try:
                        self.persist()
                    except BaseException:
                        rec["state"] = "stopping"
                        raise
                    self.live.pop(handle, None)
                    removed = self.jobs.pop(handle, {})
                    self.retained_output_bytes -= sum(job.bytes for job in removed.values() if job is not None)
                    for job in removed.values():
                        if job is not None:
                            job.released = True
                return {"absence_confirmed": True}
            raise Failure("absence_unconfirmed", 502)
        except BaseException:
            if registered:
                with self.lock:
                    deletion["failed"] = True
            raise
        finally:
            if registered:
                with self.lock:
                    deletion["active"] -= 1
                    if deletion["active"] == 0:
                        del self.deletions[handle]

    def start(self, body):
        handle, sandbox = self.entry(body)
        argv = body.get("argv")
        if not isinstance(argv, list) or not argv or len(argv) > 4096:
            raise Failure("invalid_argv")
        for arg in argv:
            if not isinstance(arg, str) or "\0" in arg:
                raise Failure("invalid_argv")
        string(argv[0])
        cwd = string(body.get("cwd"))
        if not cwd.startswith("/"):
            raise Failure("absolute_cwd_required")
        env = body.get("env", {})
        if not isinstance(env, dict) or any(not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", k) or not isinstance(v, str) or "\0" in v for k, v in env.items()):
            raise Failure("invalid_env")
        user = body.get("user") or None
        if user is not None:
            string(user, 128)
        timeout = integer(body.get("timeout_seconds", 60), 1, self.max_timeout)
        output_limit = integer(body.get("output_limit_bytes", 16 << 20), 1, 64 << 20)
        process = string(body.get("process"), 128)
        observation = None
        if "observation" in body:
            try:
                observation = validate_observation(body["observation"], process)
            except (ValueError, TypeError):
                raise Failure("invalid_observation") from None
        with self.lock:
            jobs = self.jobs[handle]
            if process in jobs:
                raise Failure("duplicate_process_no_retry", 409)
            if sum(j is None or not j.done for j in jobs.values()) >= self.max_processes:
                raise Failure("process_quota_exceeded", 429)
            if len(jobs) >= 1024:
                raise Failure("process_history_limit", 429)
            jobs[process] = None
        spec_body = {"argv": argv, "cwd": cwd, "env": env}
        if observation is not None:
            spec_body["observation"] = observation
        spec = base64.b64encode(json.dumps(spec_body).encode()).decode()
        command = shlex.join(["python3", "-u", "-c", GUEST_EXEC, spec])
        try:
            native = self.sdk_call("commands.run", sandbox.commands.run, command, background=True, stdin=True,
                user=user, cwd="/", timeout=timeout, request_timeout=self.request_timeout)
        except Exception:
            # Unknown submission continues to occupy a slot until sandbox removal.
            raise Failure("process_submission_unconfirmed", 502) from None
        job = Job(native, output_limit, observation)
        with self.lock:
            jobs[process] = job
        threading.Thread(target=self.drain, args=(job,), daemon=True).start()
        return {"process": process, "pid": native.pid, **job.binding()}

    def drain(self, job):
        pending = ""
        terminal = None
        identity_received = False
        try:
            for stdout, stderr, pty in job.handle:
                if stderr or pty:
                    raise Failure("guest_wrapper_failed", 502)
                pending += stdout or ""
                if len(pending) > (128 << 10):
                    raise Failure("invalid_guest_frame", 502)
                while "\n" in pending:
                    line, pending = pending.split("\n", 1)
                    try:
                        event = strict_json_loads(line)
                    except (ValueError, TypeError):
                        raise Failure("invalid_guest_frame", 502) from None
                    if terminal is not None:
                        raise Failure("duplicate_guest_terminal", 502)
                    if not isinstance(event, dict):
                        raise Failure("invalid_guest_frame", 502)
                    if job.observation is not None:
                        try:
                            observed = validate_observation(event.get("observation"))
                        except (ValueError, TypeError):
                            raise Failure("guest_observation_missing_or_invalid", 502) from None
                        if observed != job.observation:
                            raise Failure("guest_observation_mismatch", 502)
                        if event.get("kind") == "execution_identity":
                            if (identity_received or set(event) != {"kind", "phase", "observation"}
                                    or event["phase"] != "before_popen"):
                                raise Failure("invalid_guest_identity_receipt", 502)
                            identity_received = True
                            job.append(event)
                            continue
                        if not identity_received:
                            raise Failure("guest_identity_receipt_missing", 502)
                        payload = {key: value for key, value in event.items() if key != "observation"}
                    else:
                        payload = event
                    event_binding = {"observation": event["observation"]} if job.observation is not None else {}
                    event = payload
                    if "exit_code" in event:
                        boundary_keys = {"popen_return_ns", "first_output_ns",
                                         "child_exit_observed_ns", "pipe_eof_ns"}
                        if (not {"exit_code", "guest_duration_ns"}.issubset(event)
                                or set(event) - {"exit_code", "guest_duration_ns"} - boundary_keys
                                or type(event["exit_code"]) is not int
                                or type(event["guest_duration_ns"]) is not int
                                or not 0 <= event["guest_duration_ns"] <= (1 << 63) - 1):
                            raise Failure("invalid_guest_frame", 502)
                        for key in boundary_keys:
                            value = event.get(key)
                            if value is not None and (type(value) is not int or not 0 <= value <= event["guest_duration_ns"]):
                                raise Failure("invalid_guest_frame", 502)
                        for before, after in (("popen_return_ns", "first_output_ns"),
                                              ("popen_return_ns", "child_exit_observed_ns"),
                                              ("popen_return_ns", "pipe_eof_ns"),
                                              ("first_output_ns", "pipe_eof_ns")):
                            if (event.get(before) is not None and event.get(after) is not None
                                    and event[before] > event[after]):
                                raise Failure("invalid_guest_frame", 502)
                        terminal = {key: event[key] for key in ("exit_code", "guest_duration_ns", *boundary_keys)
                                    if key in event}
                        terminal.update(event_binding)
                    else:
                        if set(event) != {"stream", "data_base64"} or event.get("stream") not in ("stdout", "stderr"):
                            raise Failure("invalid_guest_frame", 502)
                        chunk = decode(event.get("data_base64"), 16384)
                        with self.lock:
                            if job.released:
                                raise Failure("sandbox_removed", 409)
                            if job.bytes + len(chunk) > job.output_limit or len(job.events) >= 65536 or self.retained_output_bytes + len(chunk) > 128 << 20:
                                raise Failure("output_limit_exceeded", 413)
                            job.bytes += len(chunk)
                            self.retained_output_bytes += len(chunk)
                        job.append({"stream": event["stream"], "data_base64": event["data_base64"], **event_binding})
            result = job.handle.wait()
            if pending or terminal is None or result.exit_code != 0:
                raise Failure("guest_terminal_missing", 502)
            job.finish(terminal)
        except Exception as error:
            # SDK errors may include command text or credential-bearing URLs.
            code = error.code if isinstance(error, Failure) else "process_transport_failed"
            try:
                job.handle.kill()
                # Drain the bounded native command stream so its SDK connection
                # is released. The creation-time command deadline still applies.
                job.handle.wait()
            except Exception:
                pass
            job.finish({"error_code": code, "termination_confirmed": False, **job.binding()})

    def job(self, body):
        handle, sandbox = self.entry(body)
        process = string(body.get("process"), 128)
        with self.lock:
            if process not in self.jobs[handle]:
                raise Failure("unknown_process", 404)
            job = self.jobs[handle][process]
            if job is None:
                raise Failure("process_submission_unconfirmed", 409)
        return sandbox, job

    def dispatch(self, action, body):
        if not isinstance(body, dict):
            raise Failure("object_required")
        if action == "health":
            return {"schema": 1, **self.runtime_metadata(), "stopping": self.stopping}
        if action == "create":
            return self.create(body)
        if action == "kill":
            return self.kill(body)
        if action == "status":
            with self.lock:
                rec = self.records.get(body.get("sandbox"))
                if rec is None:
                    raise Failure("unknown_sandbox", 404)
                return {"state": rec["state"], **self.runtime_metadata()}
        if action == "start":
            return self.start(body)
        if action in ("events", "stdin", "process", "kill_process"):
            sandbox, job = self.job(body)
            if action == "events":
                cursor = integer(body.get("cursor", 0), 0, 1 << 31)
                with job.condition:
                    if cursor > len(job.events):
                        raise Failure("invalid_cursor")
                    if cursor == len(job.events) and not job.done:
                        job.condition.wait(timeout=10)
                    events = job.events[cursor:cursor + 32]
                    return {"events": events, "cursor": cursor + len(events),
                            "done": job.done and cursor + len(events) == len(job.events), **job.binding()}
            if action == "stdin":
                data = decode(body.get("data_base64", ""), 1 << 20)
                close = body.get("close", False)
                if type(close) is not bool:
                    raise Failure("invalid_close")
                with job.input_lock:
                    if job.stdin_closed or job.done:
                        raise Failure("stdin_closed", 409)
                    if data:
                        self.sdk_call("commands.send_stdin", job.handle.send_stdin, data, request_timeout=self.request_timeout)
                    if close:
                        try:
                            self.sdk_call("commands.close_stdin", job.handle.close_stdin, request_timeout=self.request_timeout)
                        except self.process_not_found:
                            # The process may exit before EOF arrives; its terminal still comes from drain.
                            raise Failure("stdin_closed", 409) from None
                        job.stdin_closed = True
                return {"accepted": True, **job.binding()}
            if action == "kill_process":
                killed = self.sdk_call("commands.kill", sandbox.commands.kill, job.handle.pid, request_timeout=self.request_timeout)
                listed = self.sdk_call("commands.list", sandbox.commands.list, request_timeout=self.request_timeout)
                present = any(p.pid == job.handle.pid for p in listed)
                return {"signal_sent": killed, "managed_process_absent": not present,
                        "descendants_absent": None, **job.binding()}
            return {"pid": job.handle.pid, "done": job.done,
                    "managed_process_present": any(p.pid == job.handle.pid for p in self.sdk_call("commands.list", sandbox.commands.list, request_timeout=self.request_timeout)),
                    "descendants_absent": None, **job.binding()}
        _, sandbox = self.entry(body)
        if action in ("upload", "download"):
            path = string(body.get("path"))
            if not path.startswith("/"):
                raise Failure("absolute_path_required")
            try:
                if action == "upload":
                    content = decode(body.get("data_base64"))
                    self.sdk_call("files.write", sandbox.files.write, path, content, user="root", request_timeout=self.request_timeout)
                    return {"bytes": len(content)}
                limit = integer(body.get("max_bytes", MAX_FILE), 0, MAX_FILE)
                # A metadata check alone races a writer; enforce the bound while
                # reading the SDK's public streaming API as well.
                with self.sdk_measurement("files.read"), sandbox.files.read(path, format="stream", user="root", request_timeout=self.request_timeout) as stream:
                    content = bytearray()
                    for chunk in stream:
                        content.extend(chunk)
                        if len(content) > limit:
                            raise Failure("file_too_large", 413)
                return {"data_base64": base64.b64encode(content).decode(), "bytes": len(content)}
            except self.file_not_found:
                raise Failure("file_not_found", 404) from None
        if action == "metrics":
            return self.metrics_snapshot(sandbox)
        if action == "guest":
            port = integer(body.get("port"), 1, 65535)
            return self.guest_endpoint(sandbox, port)
        raise Failure("unknown_operation", 404)

    def metrics_snapshot(self, sandbox):
        rows = self.sdk_call("sandbox.get_metrics", sandbox.get_metrics, request_timeout=self.request_timeout)
        return {"source": "e2b_sdk", "nominal_interval_seconds": 5,
                "metrics": [dataclasses.asdict(row) for row in rows],
                "unsupported": ["disk_io", "network_io", "agent_tool_resource_split"]}

    def guest_endpoint(self, sandbox, port):
        token = sandbox.traffic_access_token
        if not token:
            raise Failure("private_ingress_token_missing", 502)
        return {"url": "https://" + sandbox.get_host(port),
                "headers": {"e2b-traffic-access-token": token}}

    def close(self):
        with self.lock:
            self.stopping = True
            handles = list(self.records)
        failed = []
        for handle in handles:
            try:
                self.kill({"sandbox": handle})
            except Exception:
                failed.append(handle)
        return failed


def make_server(adapter, token, address):
    if len(token) < 32:
        raise ValueError("ARIES_E2B_ADAPTER_TOKEN must contain at least 32 characters")

    class Handler(BaseHTTPRequestHandler):
        def setup(self):
            self.request.settimeout(30)
            super().setup()

        def log_message(self, *_):
            pass  # Request paths and SDK failures must not leak credentials.

        def do_POST(self):
            adapter.measurements.calls = []
            if not hmac.compare_digest(self.headers.get("Authorization", ""), "Bearer " + token):
                self.reply(401, {"error_code": "unauthorized"})
                return
            try:
                length = int(self.headers.get("Content-Length", "-1"))
                if not 0 <= length <= MAX_BODY or self.headers.get("Transfer-Encoding"):
                    raise Failure("invalid_body_length", 413)
                body = strict_json_loads(self.rfile.read(length))
                if not self.path.startswith("/v1/"):
                    raise Failure("unknown_operation", 404)
                result = adapter.dispatch(self.path[4:], body)
                self.reply(200, result)
            except Failure as error:
                self.report_download_failure(error, error.status, error.code)
                self.reply(error.status, {"error_code": error.code})
            except (ValueError, TypeError, UnicodeError) as error:
                self.report_download_failure(error, 400, "invalid_json_request")
                self.reply(400, {"error_code": "invalid_json_request"})
            except Exception as error:
                self.report_download_failure(error, 502, "sdk_operation_failed")
                self.reply(502, {"error_code": "sdk_operation_failed"})

        def report_download_failure(self, error, status, code):
            if self.path != "/v1/download":
                return
            diagnostic = create_failure_diagnostic(error)
            # Keep transport failures diagnosable without logging the request,
            # SDK exception message, credential-bearing URLs, or file content.
            print(json.dumps({"event": "adapter_download_failed",
                "exception_type": diagnostic["exception_type"],
                "provider_http_status": diagnostic["http_status"],
                "adapter_http_status": status, "error_code": code}), file=sys.stderr, flush=True)

        def reply(self, status, value):
            value = {**value, "sdk_calls": getattr(adapter.measurements, "calls", [])}
            def encode(obj):
                if isinstance(obj, (datetime.datetime, datetime.date)):
                    return obj.isoformat()
                raise TypeError("unsupported response value")
            content = json.dumps(value, default=encode, ensure_ascii="observation" not in value).encode()
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(content)))
            self.send_header("Cache-Control", "no-store")
            self.end_headers()
            try:
                self.wfile.write(content)
            except (BrokenPipeError, ConnectionResetError):
                pass

    class BoundedServer(ThreadingHTTPServer):
        slots = threading.BoundedSemaphore(64)

        def process_request(self, request, client_address):
            if not self.slots.acquire(blocking=False):
                try:
                    request.sendall(b"HTTP/1.0 503 Service Unavailable\r\nContent-Length: 0\r\n\r\n")
                finally:
                    self.shutdown_request(request)
                return
            try:
                super().process_request(request, client_address)
            except Exception:
                self.slots.release()
                raise

        def process_request_thread(self, request, client_address):
            try:
                super().process_request_thread(request, client_address)
            finally:
                self.slots.release()

    server = BoundedServer(address, Handler)
    # Drain accepted creates before process exit so their cloud IDs can be deleted.
    server.daemon_threads = False
    return server


def serve(adapter, token, address):
    server = make_server(adapter, token, address)
    for sig in (signal.SIGTERM, signal.SIGINT):
        signal.signal(sig, lambda *_: threading.Thread(target=server.shutdown, daemon=True).start())
    try:
        server.serve_forever()
    finally:
        with adapter.lock:
            adapter.stopping = True
        server.server_close()
        failed = adapter.close()
        if failed:
            raise SystemExit("cleanup unconfirmed; inspect private adapter state file")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--listen", default="127.0.0.1:8787")
    parser.add_argument("--template", action="append")
    parser.add_argument("--state-file", required=True)
    parser.add_argument("--max-sandboxes", type=int, default=2)
    parser.add_argument("--max-processes", type=int, default=8)
    parser.add_argument("--max-timeout", type=int, default=900)
    args = parser.parse_args()
    templates = args.template or [os.environ.get("E2B_TEMPLATE_ID", "")]
    if not all(templates):
        parser.error("--template or E2B_TEMPLATE_ID is required")
    from importlib.metadata import version
    installed_version = version("e2b")
    if installed_version != SDK_VERSION:
        parser.error("install the pinned requirements.txt")
    from e2b import Sandbox
    from e2b.exceptions import SandboxNotFoundException, FileNotFoundException, NotFoundException
    if not os.environ.get("E2B_API_KEY"):
        parser.error("E2B_API_KEY is required")
    token = os.environ.get("ARIES_E2B_ADAPTER_TOKEN", "")
    if len(token) < 32:
        parser.error("ARIES_E2B_ADAPTER_TOKEN must contain at least 32 characters")
    for value in (args.max_sandboxes, args.max_processes, args.max_timeout):
        if value < 1:
            parser.error("limits must be positive")
    host, port = args.listen.rsplit(":", 1)
    adapter = Adapter(Sandbox, SandboxNotFoundException, FileNotFoundException, NotFoundException,
        templates=templates, max_sandboxes=args.max_sandboxes,
        max_processes=args.max_processes, max_timeout=args.max_timeout, state_file=args.state_file,
        sdk_version=installed_version)
    serve(adapter, token, (host, int(port)))


if __name__ == "__main__":
    main()

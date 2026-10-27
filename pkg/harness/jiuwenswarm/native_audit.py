"""ID-only audit extension for the pinned Jiuwen native process.

Loaded by the native ExtensionLoader before it accepts agent requests. The
official agent observer is initialized earlier than its usual first-request
boundary, with its unchanged configuration. Export success is deliberately not
inferred from the SDK's flush/shutdown return: the collector must reconcile
these producer IDs with the actual retained native span files after Stop.
"""
import hashlib
import json
import os
from pathlib import Path
import re
import threading

from opentelemetry.sdk.trace import SpanProcessor

MAX_BYTES = 64 << 20
MAX_RECORDS = 262144
PROTOCOL = "jiuwen-native-span-audit-v1"
_ID = re.compile(r"[A-Za-z0-9_.:-]{1,256}\Z")


class AuditProcessor(SpanProcessor):
    def __init__(self, directory, binding, *, max_bytes=MAX_BYTES, max_records=MAX_RECORDS):
        self.directory = Path(directory)
        self.directory.mkdir(mode=0o700, parents=True, exist_ok=True)
        self.lock = threading.RLock()
        self.max_bytes, self.max_records = max_bytes, max_records
        self.digest = hashlib.sha256()
        self.state = dict(binding, protocol=PROTOCOL, pid=os.getpid(), measurement_scope="sampled_sdk_spans",
                          registered_before_provider=False, attached=False,
                          status="incomplete", started_spans=0, ended_spans=0,
                          written_records=0, written_bytes=0, write_failures=0,
                          dropped_records=0, limit_exceeded=False, force_flush_callbacks=0,
                          processor_shutdown_observed=False, extension_shutdown_started=False,
                          extension_shutdown_returned=False, errors=[],
                          exporter_flush="unconfirmed_return_not_proof",
                          max_bytes=max_bytes, max_records=max_records)
        self.fd = os.open(self.directory / "events.jsonl", os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        self._save()

    def _error(self, reason):
        if reason not in self.state["errors"]:
            self.state["errors"].append(reason)

    def _save(self):
        """Atomic status update; failures remain observer outcomes only."""
        self.state["ledger_sha256"] = self.digest.hexdigest()
        try:
            path = self.directory / "status.json.tmp"
            fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
            with os.fdopen(fd, "w") as handle:
                json.dump(self.state, handle, sort_keys=True)
                handle.write("\n")
            os.replace(path, self.directory / "status.json")
        except OSError:
            self.state["write_failures"] += 1
            self._error("status_write_failed")

    def _append(self, event, **fields):
        row = dict(schema_version=1, sequence=self.state["written_records"] + 1,
                   event=event, **fields)
        raw = (json.dumps(row, sort_keys=True, separators=(",", ":")) + "\n").encode()
        if (self.state["written_records"] >= self.max_records or
                self.state["written_bytes"] + len(raw) > self.max_bytes):
            self.state["dropped_records"] += 1
            self.state["limit_exceeded"] = True
            self._error("ledger_limit_exceeded")
            return
        try:
            size = os.write(self.fd, raw)
            self.state["written_bytes"] += size
            self.digest.update(raw[:size])
            if size != len(raw):
                raise OSError("short ledger write")
            self.state["written_records"] += 1
        except OSError:
            self.state["write_failures"] += 1
            self.state["dropped_records"] += 1
            self._error("ledger_write_failed")

    def _span(self, span, event):
        with self.lock:
            try:
                context = span.get_span_context()
                # Match the SDK BatchSpanProcessor export denominator; a
                # RECORD_ONLY span is recorded by the SDK but not exported.
                if not context.trace_flags.sampled:
                    return
                key = "started_spans" if event == "start" else "ended_spans"
                self.state[key] += 1
                parent = span.parent
                fields = dict(trace_id=f"{context.trace_id:032x}", span_id=f"{context.span_id:016x}",
                              parent_span_id=f"{parent.span_id:016x}" if parent else "",
                              start_time_unix_nano=str(span.start_time))
                if event == "end":
                    fields.update(end_time_unix_nano=str(span.end_time),
                                  status_code=span.status.status_code.value)
                tool = (span.attributes or {}).get("gen_ai.tool.call.id")
                if tool is not None:
                    if isinstance(tool, str) and _ID.fullmatch(tool):
                        fields["tool_call_id"] = tool
                    else:
                        self._error("invalid_tool_call_id")
                self._append(event, **fields)
            except Exception:
                # Never serialize exception text: it may contain native payloads.
                self.state["dropped_records"] += 1
                self._error("span_identity_failed")

    def on_start(self, span, parent_context=None):
        self._span(span, "start")

    def on_end(self, span):
        self._span(span, "end")

    def force_flush(self, timeout_millis=30000):
        with self.lock:
            self.state["force_flush_callbacks"] += 1
            self._append("force_flush")
        # This refers only to our synchronous ID writer, never another exporter.
        return not self.state["write_failures"] and not self.state["dropped_records"]

    def shutdown(self):
        with self.lock:
            if self.state["processor_shutdown_observed"]:
                return
            self.state["processor_shutdown_observed"] = True
            self._append("processor_shutdown")
            self._save()

    def finalize(self):
        with self.lock:
            self.state["status"] = (
                "complete" if self.state["attached"] and self.state["processor_shutdown_observed"]
                and not self.state["errors"] else "incomplete")
            try:
                os.fsync(self.fd)
                os.close(self.fd)
            except OSError:
                self.state["write_failures"] += 1
                self._error("ledger_close_failed")
                self.state["status"] = "incomplete"
            self._save()


class NativeAuditExtension:
    def __init__(self, audit):
        self.audit = audit

    async def shutdown(self):
        from jiuwenswarm.agents.harness.agent_observability import shutdown_agent_observability

        self.audit.state["extension_shutdown_started"] = True
        self.audit._save()
        try:
            shutdown_agent_observability()
            self.audit.state["extension_shutdown_returned"] = True
        except Exception:
            self.audit._error("native_shutdown_failed")
        finally:
            self.audit.finalize()


async def register_extensions(registry):
    """Native extension entry; no monkeypatches or alternate tool dispatch."""
    from jiuwenswarm.common.config import get_config
    from opentelemetry import trace
    from openjiuwen.extensions.observability import setup

    config = get_config()
    if not (config.get("agent_observability") or {}).get("enabled", False):
        return []  # L0 retains the original startup and sampling path.
    binding = json.loads(Path(__file__).with_name("binding.json").read_bytes())
    if (set(binding) != {"schema_version", "run_id", "task_id"} or binding["schema_version"] != 1
            or not all(isinstance(binding[k], str) and binding[k] for k in ("run_id", "task_id"))):
        raise RuntimeError("invalid native audit binding")
    audit = AuditProcessor(Path(os.environ["JIUWENSWARM_DATA_DIR"]) / "aries-native-audit", binding)
    audit.state["registered_before_provider"] = (
        not setup.is_initialized() and isinstance(trace.get_tracer_provider(), trace.ProxyTracerProvider))
    if (not audit.state["registered_before_provider"] or
            (config.get("telemetry") or {}).get("enabled", False)):
        audit._error("existing_or_unified_provider_unsupported")
        audit.finalize()
        audit.state["status"] = "unsupported"
        audit._save()
        return []
    try:
        from jiuwenswarm.agents.harness.agent_observability import sync_agent_observability

        # Use the native demand owner so its later first-request sync neither
        # creates a second exporter nor changes trajectory/root-span behavior.
        sync_agent_observability()
        active = setup.get_config()
        if active is None:
            raise RuntimeError("native observer unavailable")
        setup.init_observability(active, additional_span_processors=(audit,))
        audit.state["attached"] = True
        audit.state["status"] = "incomplete"
        audit._save()
        return [NativeAuditExtension(audit)]
    except Exception:
        audit._error("native_registration_failed")
        audit.finalize()
        return []

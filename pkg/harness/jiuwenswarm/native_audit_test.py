"""Run with the pinned native Python on ECS; each provider gets a fresh process."""
import argparse
import asyncio
import concurrent.futures
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import shutil
import signal
import socket
import subprocess
import sys
import time
import unittest


def read(path):
    return json.loads(path.read_bytes())


def module(path):
    spec = importlib.util.spec_from_file_location("audit_candidate", path)
    loaded = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(loaded)
    return loaded


def workspace(root, enabled=True):
    home = root / "home"
    home.mkdir()
    os.environ.update(HOME=str(home), JIUWENSWARM_DATA_DIR=str(home / "data"),
                      JIUWENSWARM_CONFIG_DIR=str(home / "data/config"),
                      PYTHONDONTWRITEBYTECODE="1", AGENT_SERVER_HOST="127.0.0.1")
    os.chdir(home)
    from jiuwenswarm.common.utils import prepare_workspace
    prepare_workspace(overwrite=False)
    import yaml
    config_path = home / "data/config/config.yaml"
    config = yaml.safe_load(config_path.read_text())
    config.setdefault("agent_observability", {}).update(
        enabled=enabled, exporter="file", backend="otlp", sample_rate=1.0 if enabled else 0.0,
        traces_dir=str(home / "data/aries-traces"), file_retention_days=0,
        redact_prompts=True, redact_completions=True)
    config.setdefault("telemetry", {}).update(enabled=False)
    config.setdefault("startup", {}).setdefault("warmup", {}).update(enabled=False)
    config.setdefault("sandbox", {}).update(enabled=False, startup_mode="external")
    config.setdefault("http_server", {}).update(enabled=False)
    config.setdefault("flash", {}).update(enabled=True)
    extensions = root / "extensions"
    extension = extensions / "aries_native_audit"
    extension.mkdir(parents=True)
    shutil.copyfile(Path(__file__).with_name("native_audit.py"), extension / "extension.py")
    (extension / "binding.json").write_text(json.dumps(
        dict(schema_version=1, run_id="native-audit-focused-test", task_id=root.name)))
    config.setdefault("extensions", {})["extension_dirs"] = str(extensions)
    config_path.write_text(yaml.safe_dump(config, allow_unicode=True))
    (config_path.parent / ".env").write_text('API_BASE="http://127.0.0.1:9/v1"\nAPI_KEY="offline-fixture"\nMODEL_NAME="offline-fixture"\nMODEL_PROVIDER="OpenAI"\n')
    from jiuwenswarm.common.config import clear_config_cache
    clear_config_cache()
    return extension, home / "data/aries-native-audit", home / "data/aries-traces"


def exported_ids(traces):
    ids = []
    for path in traces.glob("traces-*.jsonl"):
        for line in path.read_bytes().splitlines():
            for resource in json.loads(line).get("resourceSpans", []):
                for scope in resource.get("scopeSpans", []):
                    ids.extend((s["traceId"], s["spanId"]) for s in scope.get("spans", []))
    return ids


def child(case, root):
    if case == "record_only":
        audit_module = module(Path(__file__).with_name("native_audit.py"))
        audit = audit_module.AuditProcessor(root / "audit", dict(schema_version=1, run_id="r", task_id="t"))
        from opentelemetry.sdk.trace import TracerProvider
        from opentelemetry.sdk.trace.sampling import Sampler, SamplingResult, Decision

        class RecordOnlySampler(Sampler):
            def should_sample(self, *args, **kwargs):
                return SamplingResult(Decision.RECORD_ONLY)

            def get_description(self):
                return "record-only-focused-test"

        provider = TracerProvider(sampler=RecordOnlySampler(), shutdown_on_exit=False)
        provider.add_span_processor(audit)
        with provider.get_tracer("test").start_as_current_span("record-only") as span:
            assert span.is_recording() and not span.get_span_context().trace_flags.sampled
        provider.shutdown()
        audit.finalize()
        status = read(root / "audit/status.json")
        assert status["started_spans"] == status["ended_spans"] == 0
        rows = [json.loads(line) for line in (root / "audit/events.jsonl").read_bytes().splitlines()]
        assert all(row["event"] not in ("start", "end") for row in rows)
        (root / "result.json").write_text(json.dumps(dict(status="PASS_RECORD_ONLY_EXCLUDED", audit=status)))
        return
    if case in ("limits", "byte_limit", "write_failure"):
        audit_module = module(Path(__file__).with_name("native_audit.py"))
        audit = audit_module.AuditProcessor(root / "audit", dict(schema_version=1, run_id="r", task_id="t"),
                                            max_records=2 if case == "limits" else 100,
                                            max_bytes=128 if case == "byte_limit" else audit_module.MAX_BYTES)
        from opentelemetry.sdk.trace import TracerProvider
        provider = TracerProvider(shutdown_on_exit=False)
        provider.add_span_processor(audit)
        if case == "write_failure":
            os.close(audit.fd)
            audit.fd = 999999  # Real EBADF, not fsync's argument-validation ValueError.
        for _ in range(5):
            with provider.get_tracer("test").start_as_current_span("private-test-name"):
                pass
        provider.shutdown()
        audit.finalize()
        status = read(root / "audit/status.json")
        assert status["started_spans"] == status["ended_spans"] == 5
        assert status["status"] == "incomplete" and status["dropped_records"] > 0
        if case in ("limits", "byte_limit"):
            assert status["limit_exceeded"]
            if case == "limits":
                assert status["written_records"] == 2
            else:
                assert status["written_bytes"] <= 128 and status["dropped_records"] >= 10
        else:
            assert status["write_failures"] > 0 and not status["limit_exceeded"]
        (root / "result.json").write_text(json.dumps(status))
        return
    extension, output, traces = workspace(root, enabled=case != "l0")
    if case == "agentserver":
        # The actual pinned entry point loads and shuts down the extension.
        # It accepts no agent request and has no reachable model endpoint.
        with socket.socket() as probe:
            probe.bind(("127.0.0.1", 0))
            port = probe.getsockname()[1]
        with (root / "agentserver.log").open("wb") as log:
            process = subprocess.Popen([sys.executable, "-m", "jiuwenswarm.server.app_agentserver", "--port", str(port)],
                                       stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
            try:
                deadline = time.monotonic() + 90
                while True:
                    assert process.poll() is None, "native server exited before readiness"
                    try:
                        with socket.create_connection(("127.0.0.1", port), timeout=0.1):
                            break
                    except OSError:
                        assert time.monotonic() < deadline, "native server readiness deadline"
                        time.sleep(0.2)
                attached = read(output / "status.json")
                assert attached["attached"] and attached["registered_before_provider"]
                assert not attached["processor_shutdown_observed"]
                # The pinned server opens its socket before installing signal
                # handlers. Wait for actual SIGTERM readiness, not just TCP.
                while True:
                    assert process.poll() is None
                    fields = dict(line.split(":", 1) for line in Path(f"/proc/{process.pid}/status").read_text().splitlines())
                    if int(fields["SigCgt"].strip(), 16) & (1 << (signal.SIGTERM - 1)):
                        break
                    assert time.monotonic() < deadline, "native signal readiness deadline"
                    time.sleep(0.05)
                os.killpg(process.pid, signal.SIGTERM)
                assert process.wait(timeout=45) == 0
                final = read(output / "status.json")
                assert final["status"] == "complete", final
                assert final["extension_shutdown_started"] and final["extension_shutdown_returned"]
                assert final["processor_shutdown_observed"]
            finally:
                if process.poll() is None:
                    os.killpg(process.pid, signal.SIGKILL)
                    process.wait(timeout=10)
                try:
                    os.killpg(process.pid, 0)
                except ProcessLookupError:
                    pass
                else:
                    os.killpg(process.pid, signal.SIGKILL)
                    raise AssertionError("native descendant remained")
        with socket.socket() as probe:
            probe.bind(("127.0.0.1", port))
        (root / "result.json").write_text(json.dumps(dict(status="PASS_NATIVE_ENTRY_LOAD_SHUTDOWN", pid=process.pid,
                                                         process_group_absent=True, port_released=True, audit=final)))
        return
    from jiuwenswarm.extensions.loader import ExtensionLoader
    from openjiuwen.extensions.observability import setup
    if case == "preexisting":
        from openjiuwen.extensions.observability.config import ObservabilityConfig
        setup.init_observability(ObservabilityConfig(exporter="file", backend="otlp", traces_dir=str(traces)))
    elif case == "external_provider":
        from opentelemetry import trace
        from opentelemetry.sdk.trace import TracerProvider
        trace.set_tracer_provider(TracerProvider())
    elif case == "unified":
        import yaml
        config_path = root / "home/data/config/config.yaml"
        config = yaml.safe_load(config_path.read_text())
        config["telemetry"]["enabled"] = True
        config_path.write_text(yaml.safe_dump(config))
        from jiuwenswarm.common.config import clear_config_cache
        clear_config_cache()
    audit_module = ExtensionLoader._import_module(extension)
    registered = asyncio.run(audit_module.register_extensions(None))
    if case in ("preexisting", "external_provider", "unified"):
        assert registered == []
        status = read(output / "status.json")
        assert status["status"] == "unsupported" and not status["attached"]
        assert status["registered_before_provider"] == (case == "unified")
        setup.shutdown_observability()
        (root / "result.json").write_text(json.dumps(status))
        return
    if case == "l0":
        assert registered == [] and not setup.is_initialized() and not output.exists()
        (root / "result.json").write_text(json.dumps(dict(status="PASS_L0_UNTOUCHED")))
        return
    assert len(registered) == 1
    audit = registered[0].audit
    assert audit.state["registered_before_provider"] and audit.state["attached"]
    if case == "export_failure":
        # Real pinned exporter reports SUCCESS despite this write error. The
        # independent producer ledger must retain the missing exported IDs.
        traces.rmdir()
        traces.write_text("force exporter ENOTDIR")
    marker = "MUST_NOT_APPEAR_IN_ID_AUDIT"
    tracer = setup.get_tracer("native-audit-focused")

    def emit(number):
        from opentelemetry.trace import Status, StatusCode
        with tracer.start_as_current_span(marker, attributes={"private.input": marker}) as parent:
            with tracer.start_as_current_span(marker, attributes={"gen_ai.tool.call.id": "call_" + str(number), "private.output": marker}) as span:
                if number == 0:
                    span.set_status(Status(StatusCode.ERROR, marker))

    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
        list(pool.map(emit, range(20 if case == "complete" else 1)))
    if case == "abrupt":
        # Simulate a killed process after real span end and before lifecycle
        # shutdown. No finalization flag may be manufactured by atexit.
        os._exit(0)
    asyncio.run(registered[0].shutdown())
    status = read(output / "status.json")
    rows = [json.loads(line) for line in (output / "events.jsonl").read_bytes().splitlines()]
    assert [row["sequence"] for row in rows] == list(range(1, len(rows) + 1))
    assert marker not in (output / "events.jsonl").read_text()
    assert status["written_records"] == len(rows)
    assert status["ledger_sha256"] == hashlib.sha256((output / "events.jsonl").read_bytes()).hexdigest()
    starts = [(row["trace_id"], row["span_id"]) for row in rows if row["event"] == "start"]
    ends = [(row["trace_id"], row["span_id"]) for row in rows if row["event"] == "end"]
    assert len(starts) == len(set(starts)) == status["started_spans"]
    assert len(ends) == len(set(ends)) == status["ended_spans"]
    assert set(starts) == set(ends)
    assert any(row.get("status_code") == 2 for row in rows)
    assert status["status"] == "complete" and status["exporter_flush"] == "unconfirmed_return_not_proof"
    exported = exported_ids(traces)
    if case == "export_failure":
        assert len(ends) == 2 and not exported
    else:
        assert len(exported) == len(set(exported)) == len(ends)
        assert set(exported) == set(ends)
    (root / "result.json").write_text(json.dumps(dict(status="PASS", producer=status, exported_ids=len(exported),
                                                     ended_ids=len(ends), sets_equal=set(exported) == set(ends))))


class NativeAuditTests(unittest.TestCase):
    def run_case(self, case):
        directory = ROOT / case
        directory.mkdir()
        with (directory / "child.log").open("wb") as log:
            result = subprocess.run([sys.executable, __file__, "--child", case, "--root", str(directory)],
                                    stdout=log, stderr=subprocess.STDOUT, timeout=160)
        self.assertEqual(result.returncode, 0, f"{case}: {directory / 'child.log'}")
        return directory

    def test_exact_ids_concurrent_native_provider(self):
        self.run_case("complete")

    def test_native_entry_load_and_shutdown(self):
        self.run_case("agentserver")

    def test_exporter_write_failure_not_claimed_success(self):
        self.run_case("export_failure")

    def test_quota_preserves_denominator(self):
        self.run_case("limits")

    def test_byte_limit_preserves_denominator(self):
        self.run_case("byte_limit")

    def test_preexisting_provider_is_unsupported(self):
        self.run_case("preexisting")

    def test_unified_provider_is_unsupported(self):
        self.run_case("unified")

    def test_record_only_matches_exporter_denominator(self):
        self.run_case("record_only")

    def test_external_provider_is_unsupported(self):
        self.run_case("external_provider")

    def test_write_failure_preserves_denominator(self):
        self.run_case("write_failure")

    def test_l0_untouched(self):
        self.run_case("l0")

    def test_abrupt_exit_is_incomplete(self):
        directory = self.run_case("abrupt")
        status = read(directory / "home/data/aries-native-audit/status.json")
        self.assertEqual(status["status"], "incomplete")
        self.assertFalse(status["processor_shutdown_observed"])
        self.assertFalse(status["extension_shutdown_returned"])


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--root", type=Path, required=True)
    parser.add_argument("--child")
    args, rest = parser.parse_known_args()
    assert sys.platform.startswith("linux"), "tests execute on ECS only"
    ROOT = args.root.resolve()
    if args.child:
        child(args.child, ROOT)
    else:
        unittest.main(argv=[sys.argv[0]] + rest, verbosity=2)

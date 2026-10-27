"""Offline contract checks. Execute on ECS; these never call the E2B API."""
import importlib.util
import inspect
from io import StringIO
import json
import os
from pathlib import Path
import subprocess
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location("aries_template_build", Path(__file__).with_name("build.py"))
module = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(module)
BUILD_ID = "f47ac10b-58cc-4372-a567-0e02b2c3d479"


class Definition:
    def __init__(self, file_context_path):
        self.context = Path(file_context_path)
        self.steps = []

    def __getattr__(self, method):
        def call(*args, **kwargs):
            self.steps.append((method, args, kwargs))
            return self
        return call


class NotFound(Exception):
    pass


class CommandExit(Exception):
    def __init__(self):
        self.exit_code = 2
        self.stdout = json.dumps({"status": "UNSUPPORTED", "reason": "cgroup v2 is unavailable"})


class FakeSandbox:
    sandbox_id = "private-sandbox-id"

    def __init__(self, unsupported=False, remains=False):
        self.unsupported, self.remains = unsupported, remains
        self.commands = self
        self.killed = False

    def run(self, command, **options):
        assert options["user"] == "root"
        assert "--check-cgroup" in command
        if self.unsupported:
            raise CommandExit()
        return SimpleNamespace(exit_code=0, stdout=json.dumps({"status": "PASS", "cgroup": {"status": "PASS"}}))

    def kill(self, **options):
        self.killed = True

    def get_info(self, **options):
        assert self.killed
        if not self.remains:
            raise NotFound()
        return object()


class TemplateTests(unittest.TestCase):
    def test_build_uses_pin_allowlisted_context_and_private_evidence(self):
        seen = []
        class Template(Definition):
            @staticmethod
            def build_in_background(template, **options):
                seen.append(template)
                self.assertEqual(set(p.name for p in template.context.iterdir()), set(module.CONTEXT_FILES))
                self.assertEqual(options["api_key"], "secret-not-a-build-arg")
                self.assertFalse(options["debug"])
                self.assertNotIn("on_build_logs", options)
                return SimpleNamespace(template_id="private-id", build_id=BUILD_ID, name=options["name"], alias=options["name"], tags=["ab-v1"])

            @staticmethod
            def get_build_status(info, **options):
                return SimpleNamespace(status=SimpleNamespace(value="ready"))

        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "build.json"
            versions_file = Path(directory) / "versions.lock.json"
            image = "ghcr.io/openclaw/openclaw:2026.7.1@sha256:" + "a" * 64
            versions_file.write_text(json.dumps({"openclaw": {"image": image}}))
            evidence = module.Evidence(path, {"status": "FAIL"})
            args = SimpleNamespace(cpu_count=2, memory_mb=4096, wait_seconds=10, versions_file=versions_file)
            with patch.dict(os.environ, {"DEEPSEEK_API_KEY": "never-in-context"}):
                module.build(Template, args, "secret-not-a-build-arg", evidence)
            evidence.close()
            record = json.loads(path.read_text())
            self.assertEqual(record["status"], "PASS")
            self.assertEqual(record["runtime_capabilities"], "UNRUN")
            self.assertEqual(record["source_image"], image)
            self.assertEqual(path.stat().st_mode & 0o777, 0o600)
            self.assertNotIn("secret-not-a-build-arg", path.read_text())
            self.assertNotIn("never-in-context", path.read_text())
            self.assertIn(("from_image", (image,), {}), seen[0].steps)
            self.assertIn(("copy", ("start.sh", "/opt/aries-template/start.sh"),
                           {"user": "root", "mode": 0o500}), seen[0].steps)
            self.assertEqual(seen[0].steps[-2], ("set_user", ("root",), {}))
            self.assertEqual(seen[0].steps[-1], ("set_start_cmd",
                             ("exec /bin/sh /opt/aries-template/start.sh 2026.7.1",
                              "test -f /opt/aries-template/ready.json"), {}))
            self.assertNotIn("secret-not-a-build-arg", repr(seen[0].steps))
            self.assertFalse(seen[0].context.exists())
            with self.assertRaises(FileExistsError):
                module.Evidence(path, {})

    def test_start_repairs_permissions_and_publishes_only_successful_probe(self):
        # Run the shell logic only against a temporary tree with fake identity/probe/sleep.
        for probe_exit in (0, 2):
            with self.subTest(probe_exit=probe_exit), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                local = root / "local"
                probe = local / "share/aries-template/check_runtime.py"
                probe.parent.mkdir(parents=True)
                protected = root / "protected"
                protected.mkdir()
                ready, pending = protected / "ready.json", protected / "ready.json.tmp"
                ready.write_text('{"status":"STALE"}')
                pending.write_text("stale partial data")
                probe.write_text("import json, pathlib, sys\n"
                    + "assert not pathlib.Path(" + repr(str(ready)) + ").exists()\n"
                    + "assert pathlib.Path(" + repr(str(local)) + ").stat().st_mode & 0o022 == 0\n"
                    + "assert pathlib.Path(__file__).stat().st_mode & 0o022 == 0\n"
                    + "assert sys.argv[1:] == ['--expected-version', '2026.7.1']\n"
                    + "print(json.dumps({'status': " + repr("PASS" if probe_exit == 0 else "UNSUPPORTED") + "}))\n"
                    + "sys.exit(" + str(probe_exit) + ")\n")
                local.chmod(0o777)
                probe.chmod(0o777)
                identity, sleeper = root / "identity", root / "sleeper"
                identity.write_text("#!/bin/sh\nprintf '0\\n'\n")
                sleeper.write_text("#!/bin/sh\nprintf 'SLEEP_REACHED\\n'\n")
                identity.chmod(0o700)
                sleeper.chmod(0o700)
                script = (module.HERE / "start.sh").read_text()
                script = script.replace("/usr/local", str(local)).replace("/opt/aries-template", str(protected))
                script = script.replace("/usr/bin/id", str(identity)).replace("/bin/sleep", str(sleeper))
                self.assertNotIn("/usr/local", script)
                self.assertNotIn("/opt/aries-template", script)
                staged = root / "start.sh"
                staged.write_text(script)
                result = subprocess.run(["/bin/sh", str(staged), "2026.7.1"],
                                        capture_output=True, text=True, timeout=10)
                self.assertEqual(result.returncode, probe_exit, result.stderr)
                self.assertFalse(pending.exists())
                if probe_exit == 0:
                    self.assertEqual(json.loads(ready.read_text()), {"status": "PASS"})
                    self.assertEqual(ready.stat().st_mode & 0o777, 0o444)
                    self.assertIn("SLEEP_REACHED", result.stdout)
                else:
                    self.assertFalse(ready.exists())
                    self.assertNotIn("SLEEP_REACHED", result.stdout)
                    self.assertEqual(json.loads(result.stderr), {"status": "UNSUPPORTED"})

    def test_image_pin_accepts_tag_or_complete_digest_and_rejects_invalid_references(self):
        tag = "ghcr.io/openclaw/openclaw:2026.7.1"
        digest = "@sha256:" + "a" * 64
        with tempfile.TemporaryDirectory() as directory:
            versions = Path(directory) / "versions.json"
            for image in (tag, tag + digest):
                with self.subTest(image=image):
                    versions.write_text(json.dumps({"openclaw": {"image": image}}))
                    self.assertEqual(module.read_pin(versions), (image, "2026.7.1"))
            for image in (tag + "@sha256:short", tag + "@sha256:" + "A" * 64,
                          tag + "@sha512:" + "a" * 128, tag + "@", tag + digest + "\n",
                          "example.invalid/openclaw:2026.7.1" + digest,
                          "ghcr.io/openclaw/openclaw" + digest):
                with self.subTest(image=image):
                    versions.write_text(json.dumps({"openclaw": {"image": image}}))
                    with self.assertRaises(ValueError):
                        module.read_pin(versions)

    def test_boot_requires_capability_and_confirmed_absence(self):
        for unsupported, remains, expected in [(False, False, "PASS"), (True, False, "UNSUPPORTED"), (False, True, "FAIL")]:
            with self.subTest(unsupported=unsupported, remains=remains), tempfile.TemporaryDirectory() as directory:
                source = Path(directory) / "build.json"
                source.write_text(json.dumps({"status": "PASS", "phase": "template_published", "template_id": "private-id", "build_id": BUILD_ID, "expected_openclaw_version": "2026.7.1"}))
                sandbox = FakeSandbox(unsupported, remains)
                class Sandbox:
                    @staticmethod
                    def create(**options):
                        self.assertFalse(options["allow_internet_access"])
                        self.assertFalse(options["network"]["allow_public_traffic"])
                        self.assertNotIn("envs", options)
                        saved = json.loads((Path(directory) / "verify.json").read_text())
                        self.assertEqual(options["template"], "private-id:" + BUILD_ID)
                        self.assertEqual(options["template"], saved["template_reference"])
                        self.assertEqual(options["metadata"], {"aries_template_verify_handle": saved["allocation_handle"]})
                        self.assertEqual(options["timeout"], saved["requested_timeout_seconds"])
                        self.assertEqual(options["lifecycle"], {"on_timeout": "kill", "auto_resume": False})
                        return sandbox
                evidence = module.Evidence(Path(directory) / "verify.json", {"status": "FAIL"})
                try:
                    module.verify(Sandbox, NotFound, CommandExit, SimpleNamespace(build_evidence=source), "secret", evidence)
                finally:
                    evidence.close()
                self.assertEqual(evidence.document["status"], expected)
                self.assertEqual(evidence.document["cleanup_confirmed"], not remains)
                self.assertTrue(sandbox.killed)

    def test_missing_or_invalid_build_identity_fails_before_allocation(self):
        source_record = {"status": "PASS", "phase": "template_published",
                         "template_id": "private-id", "build_id": BUILD_ID,
                         "expected_openclaw_version": "2026.7.1"}
        for field, value in (("template_id", None), ("build_id", None),
                             ("build_id", ""), ("build_id", "ab-v1"),
                             ("template_id", "private-id:ab-v1"),
                             ("expected_openclaw_version", None)):
            with self.subTest(field=field, value=value), tempfile.TemporaryDirectory() as directory:
                source = Path(directory) / "build.json"
                record = dict(source_record)
                if value is None:
                    record.pop(field)
                else:
                    record[field] = value
                source.write_text(json.dumps(record))
                evidence = module.Evidence(Path(directory) / "verify.json", {"status": "FAIL"})
                try:
                    with self.assertRaises(ValueError):
                        module.verify(object(), NotFound, CommandExit,
                                      SimpleNamespace(build_evidence=source), "secret", evidence)
                finally:
                    evidence.close()
                self.assertNotIn("allocation_handle", evidence.document)

    def test_error_evidence_keeps_safe_http_status_without_response_or_credentials(self):
        class ProviderFailure(Exception):
            pass
        for status in (404, "secret-response", True):
            with self.subTest(status=status), tempfile.TemporaryDirectory() as directory:
                path = Path(directory) / "failure.json"
                failure = ProviderFailure("secret-response")
                failure.status_code = status
                output = StringIO()
                args = ["build.py", "verify", "--build-evidence", "unused", "--output", str(path)]
                with patch.object(module.sys, "argv", args), patch.object(module.sys, "platform", "linux"), \
                     patch.dict(os.environ, {"E2B_API_KEY": "secret-api-key"}), \
                     patch.object(module, "version", return_value=module.SDK_VERSION), \
                     patch.object(module, "verify", side_effect=failure), patch.object(module.sys, "stdout", output):
                    self.assertEqual(module.main(), 1)
                record = json.loads(path.read_text())
                self.assertEqual(record["error_class"], "ProviderFailure")
                if status == 404:
                    self.assertEqual(record["error_http_status"], 404)
                else:
                    self.assertNotIn("error_http_status", record)
                for secret in ("secret-response", "secret-api-key"):
                    self.assertNotIn(secret, path.read_text() + output.getvalue())

    def test_pinned_sdk_public_signature_and_definition(self):
        # Construction records build instructions; it does not build or create a sandbox.
        from e2b import Template
        self.assertEqual(module.version("e2b"), module.SDK_VERSION)
        for name in ("name", "tags", "cpu_count", "memory_mb"):
            self.assertIn(name, inspect.signature(Template.build_in_background).parameters)
        self.assertIn("build_info", inspect.signature(Template.get_build_status).parameters)
        image, native_version = module.read_pin()
        for reference in (image, image.partition("@")[0] + "@sha256:" + "a" * 64):
            self.assertIsNotNone(module.definition(Template, module.HERE, reference, native_version))

    def test_probe_transport_failure_still_confirms_cleanup(self):
        with tempfile.TemporaryDirectory() as directory:
            source = Path(directory) / "build.json"
            source.write_text(json.dumps({"status": "PASS", "phase": "template_published", "template_id": "private-id", "build_id": BUILD_ID, "expected_openclaw_version": "2026.7.1"}))
            sandbox = FakeSandbox()
            class Sandbox:
                @staticmethod
                def create(**options):
                    return sandbox
            evidence = module.Evidence(Path(directory) / "verify.json", {"status": "FAIL"})
            try:
                with patch.object(sandbox, "run", side_effect=ConnectionError("injected")):
                    with self.assertRaises(ConnectionError):
                        module.verify(Sandbox, NotFound, CommandExit, SimpleNamespace(build_evidence=source), "secret", evidence)
            finally:
                evidence.close()
            self.assertTrue(sandbox.killed)
            self.assertTrue(evidence.document["cleanup_confirmed"])
            self.assertEqual(evidence.document["status"], "FAIL")


if __name__ == "__main__":
    unittest.main()

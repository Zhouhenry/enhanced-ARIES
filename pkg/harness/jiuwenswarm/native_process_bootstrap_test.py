"""Focused bootstrap capability/registration checks; no native process is started."""
import ast
from contextlib import redirect_stdout
import hashlib
import io
import json
import os
from pathlib import Path
import re
import sys
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch


DIRECTORY = Path(__file__).resolve().parent
tree = ast.parse((DIRECTORY / "bootstrap.py").read_text())
tree.body = [node for node in tree.body if not isinstance(node, ast.If)]
bootstrap = {}
exec(compile(tree, str(DIRECTORY / "bootstrap.py"), "exec"), bootstrap)
verify = bootstrap["verify_process_observer"]
prepared_script = re.search(r"const preparedScript = `(.*?)`", (DIRECTORY / "embedded.go").read_text(), re.S).group(1)


class NativeProcessCapabilityTests(unittest.TestCase):
    def fixture(self, root):
        observer_path = root / "aries_process_observer.py"
        observer_path.write_text("# fixture observer\n")
        shell_path = root / "shell_operation.py"
        shell_path.write_text("await aries_process_observer.create_subprocess_shell()\n"
                              "await aries_process_observer.create_subprocess_exec()\n")
        observer = SimpleNamespace(__file__=str(observer_path))
        shell = SimpleNamespace(__file__=str(shell_path), aries_process_observer=observer)
        files = {p.name: dict(path=str(p), sha256=hashlib.sha256(p.read_bytes()).hexdigest())
                 for p in (observer_path, shell_path)}
        return observer, shell, files

    def test_legacy_absence_is_unsupported_but_unverified_module_is_error(self):
        with patch("importlib.util.find_spec", return_value=None):
            self.assertFalse(verify({}))
            with self.assertRaises(RuntimeError):
                verify({"aries_process_observer.py": {}})
        with patch("importlib.util.find_spec", return_value=object()):
            with self.assertRaises(RuntimeError):
                verify({})

    def test_verified_modules_and_hook_are_required(self):
        for case in ("valid", "partial_manifest", "bad_hash", "missing_hook", "import_failure", "shadow_module"):
            with self.subTest(case=case), tempfile.TemporaryDirectory() as directory:
                root = Path(directory).resolve()
                observer, shell, files = self.fixture(root)
                if case == "partial_manifest":
                    del files["shell_operation.py"]
                elif case == "bad_hash":
                    Path(observer.__file__).write_text("# changed\n")
                elif case == "missing_hook":
                    Path(shell.__file__).write_text("await asyncio.create_subprocess_shell()\n")
                    files["shell_operation.py"]["sha256"] = hashlib.sha256(Path(shell.__file__).read_bytes()).hexdigest()
                elif case == "shadow_module":
                    shadow = root / "other"
                    shadow.mkdir()
                    shadow = shadow / "aries_process_observer.py"
                    shadow.write_bytes(Path(observer.__file__).read_bytes())
                    observer.__file__ = str(shadow)
                imports = ImportError("broken native import") if case == "import_failure" else [observer, shell]
                with patch("importlib.util.find_spec", return_value=object()), \
                        patch("importlib.import_module", side_effect=imports):
                    if case == "valid":
                        self.assertTrue(verify(files))
                    else:
                        with self.assertRaises((RuntimeError, ImportError)):
                            verify(files)

    @unittest.skipUnless(sys.platform.startswith("linux"), "native readiness proof uses Linux /proc")
    def test_ready_proof_requires_registered_observer_only_when_supported(self):
        for case in ("legacy", "registered", "missing_index", "write_failure", "wrong_binding"):
            with self.subTest(case=case), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                home = root / "home"
                home.mkdir()
                group = root / "group"
                group.mkdir()
                (group / "cgroup.procs").write_text(str(os.getpid()))
                binding = dict(run_id="run", task_id="task")
                (root / "spec.json").write_text(json.dumps(dict(observation_binding=binding)))
                proof = dict(uid=60000, sandbox_enabled=False, sandbox_startup_mode="external",
                             skill_selection=False, pid=os.getpid(),
                             native_process_observation_supported=case != "legacy")
                (home / "prepared.json").write_text(json.dumps(proof))
                if case not in ("legacy", "missing_index"):
                    process = home / "data/aries-process"
                    process.mkdir(parents=True)
                    counters = dict.fromkeys(("dropped", "write_failed", "index_write_failed", "late_events", "identity_failed"), 0)
                    counters["write_failed"] = int(case == "write_failure")
                    index = dict(schema_version=1, source="jiuwen_native_process", binding=binding,
                                 observer_closed=False, counters=counters)
                    if case == "wrong_binding":
                        index["binding"] = dict(run_id="other", task_id="task")
                    (process / "process-index.json").write_text(json.dumps(index))
                    (process / "process-events.jsonl").write_bytes(b"")
                with patch.object(sys, "argv", ["proof", str(root), str(group)]), redirect_stdout(io.StringIO()):
                    if case in ("legacy", "registered"):
                        exec(prepared_script, {})
                    else:
                        with self.assertRaises((AssertionError, FileNotFoundError)):
                            exec(prepared_script, {})


if __name__ == "__main__":
    unittest.main()

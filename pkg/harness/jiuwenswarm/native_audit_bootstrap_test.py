"""ECS-only actual bootstrap configuration/extension discovery regression.

The run branch's UID gate and terminal exec are mocked. This checks the native
configuration path, not process containment or execution as UID 60000.
"""
import argparse
import asyncio
import hashlib
import json
import os
from pathlib import Path
import runpy
import subprocess
import sys
from unittest.mock import patch


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--root", type=Path, required=True)
    parser.add_argument("--native", type=Path, required=True)
    parser.add_argument("--enabled", choices=("yes", "no"), required=True)
    parser.add_argument("--process-supported", choices=("yes", "no"))
    args = parser.parse_args()
    assert sys.platform.startswith("linux") and os.geteuid() == 0
    root, native = args.root.resolve(), args.native.resolve()
    root.mkdir()
    work = root / "work"
    work.mkdir()
    runtime = root / "runtime"
    bootstrap = Path(__file__).with_name("bootstrap.py")
    audit = Path(__file__).with_name("native_audit.py")
    enabled = args.enabled == "yes"
    binding = dict(schema_version=1, run_id="bootstrap-config-focused", task_id="l1" if enabled else "l0")
    spec = dict(source_dir=str(native), python_path=str(native / ".venv/bin/python"),
                bootstrap=bootstrap.read_text(), native_audit=audit.read_text(),
                observation_binding=binding, native_observation=enabled,
                native_process_observation=enabled,
                native_process_extension=bootstrap.with_name("native_process_extension.py").read_text(),
                model_proxy_url="http://127.0.0.1:9/v1", proxy_token="offline-fixture",
                model="offline-fixture", workdir=str(work))
    subprocess.run([sys.executable, str(bootstrap), "stage", str(runtime)],
                   input=json.dumps(spec).encode(), check=True, timeout=20)
    process_supported = json.loads((runtime / "spec.json").read_text())["native_process_observation_supported"]
    if args.process_supported is not None:
        assert process_supported == (enabled and args.process_supported == "yes")
    process_extension = runtime / "extensions/aries_native_process"
    assert process_extension.exists() == process_supported
    extension = runtime / "extensions/aries_native_audit"
    if enabled:
        for name in ("extension.py", "binding.json"):
            info = (extension / name).stat()
            assert info.st_uid == 0 and info.st_mode & 0o222 == 0
        assert json.loads((extension / "binding.json").read_bytes()) == binding
    else:
        assert not extension.exists()
    os.environ.update(HOME=str(runtime / "home"), JIUWENSWARM_DATA_DIR=str(runtime / "home/data"),
                      JIUWENSWARM_CONFIG_DIR=str(runtime / "home/data/config"), PYTHONDONTWRITEBYTECODE="1")
    os.chdir(runtime / "home")
    from jiuwenswarm.common.utils import prepare_workspace
    prepare_workspace(overwrite=False)
    import yaml
    config_path = runtime / "home/data/config/config.yaml"
    config = yaml.safe_load(config_path.read_text())
    config.setdefault("extensions", {})["extension_dirs"] = str(root / "existing-a") + ";" + str(root / "existing-b")
    config_path.write_text(yaml.safe_dump(config, allow_unicode=True))
    with patch.object(sys, "argv", [str(bootstrap), "run", str(runtime)]), \
            patch("os.geteuid", return_value=60000), patch("os.execv") as terminal_exec:
        runpy.run_path(str(bootstrap), run_name="__main__")
    terminal_exec.assert_called_once()
    proof = json.loads((runtime / "home/prepared.json").read_text())
    assert proof["native_process_observation_supported"] == process_supported
    assert terminal_exec.call_args.args[1][1:3] == ["-m", "jiuwenswarm.server.app_agentserver"]
    from jiuwenswarm.common.utils import get_builtin_skills_dir, resolve_agent_registered_skill_dirs
    from jiuwenswarm.common.config import get_config
    from jiuwenswarm.server.runtime.agent_adapter.interface_flash import JiuwenSwarmFlashAdapter
    from jiuwenswarm.server.runtime.skill.skill_manager import SkillManager
    builtin = get_builtin_skills_dir().resolve()
    expected_skills = {p.parent.name: hashlib.sha256(p.read_bytes()).hexdigest()
                       for p in builtin.glob("*/SKILL.md")}
    assert "ppt-creation" in expected_skills
    assert resolve_agent_registered_skill_dirs() == [builtin]
    assert proof["native_skills"]["manifests"] == expected_skills
    assert proof["native_skills"]["directory"] == str(builtin)
    adapter = JiuwenSwarmFlashAdapter()
    adapter._skill_manager = SkillManager()
    rail = adapter._build_skill_rail(get_config().get("react", {}), include_tools=False)
    assert rail is not None
    asyncio.run(rail.reload_skills())
    actual_skills = {skill.name: str(skill.directory) for skill in rail.skills_meta}
    assert actual_skills == {name: str(builtin / name) for name in expected_skills}
    assert 'JIUWENSWARM_SHARED_SKILLS_DIRS=' + json.dumps(str(builtin)) in (config_path.parent / ".env").read_text()
    config = yaml.safe_load(config_path.read_text())
    directories = config["extensions"]["extension_dirs"]
    assert isinstance(directories, str)
    assert directories.split(";")[:2] == [str(root / "existing-a"), str(root / "existing-b")]
    assert (str(runtime / "extensions") in directories.split(";")) == enabled
    from jiuwenswarm.extensions.manager import ExtensionManager
    manager = ExtensionManager(registry=None)
    discovered = manager.loader.discover_extension_roots()
    assert (extension in discovered) == enabled
    assert (process_extension in discovered) == process_supported
    status = None
    if enabled:
        loaded = asyncio.run(manager.loader.load_extension(extension))
        assert len(loaded) == 1
        if process_supported:
            process_loaded = asyncio.run(manager.loader.load_extension(process_extension))
            assert len(process_loaded) == 1
            asyncio.run(process_loaded[0].shutdown())
            process_status = json.loads((runtime / "home/data/aries-process/process-index.json").read_text())
            assert process_status["observer_closed"] and process_status["pending_processes"] == 0
        asyncio.run(loaded[0].shutdown())
        status = json.loads((runtime / "home/data/aries-native-audit/status.json").read_bytes())
        assert status["attached"] and status["registered_before_provider"] and status["status"] == "complete"
    else:
        assert config["agent_observability"]["sample_rate"] == 0
        assert not (runtime / "home/data/aries-native-audit").exists()
    result = dict(status="PASS_ACTUAL_BOOTSTRAP_CONFIG_AND_NATIVE_DISCOVERY", enabled=enabled,
                  process_supported=process_supported,
                  bootstrap_sha256=hashlib.sha256(bootstrap.read_bytes()).hexdigest(),
                  audit_sha256=hashlib.sha256(audit.read_bytes()).hexdigest(),
                  extension_dirs_type="str", previous_directories_preserved=True,
                  registered_native_skills=actual_skills,
                  extension_discovered=enabled, audit=status,
                  scope="Actual stage/run/config/project/native extension-loader path; run UID gate and final exec mocked; no cloud/model/listener/UID-containment claim")
    (root / "result.json").write_text(json.dumps(result, indent=2) + "\n")


if __name__ == "__main__":
    main()

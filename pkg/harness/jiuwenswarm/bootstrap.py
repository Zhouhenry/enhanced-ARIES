"""Pinned native bootstrap; placement selects LOCAL or registered SANDBOX tools."""
import hashlib
import importlib
import importlib.util
import json
import os
from pathlib import Path
import socket
import sys

REVISION = "e2c9d519498080e249e34f16a80c8c97f0cd1962"
LOCK_SHA256 = "f28fbbd9b208bcf4f6e421818969ed2b6570f0648bed25c2ed15ee5bd551e5c9"


def trusted(path):
    for item in [path] + list(path.parents):
        info = item.stat()
        if info.st_uid != 0 or info.st_mode & 0o022:
            raise RuntimeError("native installation must be root owned")


def write(path, content, mode=0o600, owner=60000):
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, mode)
    with os.fdopen(fd, "w") as handle:
        os.fchmod(handle.fileno(), mode)
        handle.write(content)
    os.chown(path, owner, owner)


def verify_process_observer(files):
    name = "openjiuwen.core.sys_operation.local.aries_process_observer"
    found = importlib.util.find_spec(name)
    if found is None and not files:
        return False
    if found is None or set(files) != {"aries_process_observer.py", "shell_operation.py"}:
        raise RuntimeError("native process observer lacks verified patch evidence")
    observer = importlib.import_module(name)
    shell = importlib.import_module("openjiuwen.core.sys_operation.local.shell_operation")
    for module in (observer, shell):
        installed = Path(module.__file__).resolve(strict=True)
        evidence = files.get(installed.name)
        if (evidence is None or str(installed) != evidence["path"]
                or hashlib.sha256(installed.read_bytes()).hexdigest() != evidence["sha256"]):
            raise RuntimeError("loaded native process module differs from verified patch")
    hook = Path(shell.__file__).read_text()
    if (getattr(shell, "aries_process_observer", None) is not observer
            or not all("await aries_process_observer.create_subprocess_" + kind + "(" in hook
                       for kind in ("shell", "exec"))):
        raise RuntimeError("native process creation hook is unavailable")
    return True


def stage():
    if os.geteuid() != 0:
        raise RuntimeError("root staging required")
    spec = json.load(sys.stdin)
    source = Path(spec["source_dir"])
    executable = Path(spec["python_path"])
    if source.resolve(strict=True) != source:
        raise RuntimeError("native source path must be canonical")
    # Keep the venv executable path for launch: resolving its symlink would lose
    # the venv. Validate both paths and the root-owned build receipt instead.
    trusted(source)
    trusted(executable)
    trusted(executable.resolve(strict=True))
    receipt = source / "aries-build.json"
    trusted(receipt)
    build = json.loads(receipt.read_text())
    lock = source / "uv.lock"
    trusted(lock)
    if build.get("revision") != REVISION or build.get("uv_lock_sha256") != LOCK_SHA256:
        raise RuntimeError("native build pin mismatch")
    if hashlib.sha256(lock.read_bytes()).hexdigest() != LOCK_SHA256:
        raise RuntimeError("native dependency lock mismatch")
    patches = source / "aries-patches.json"
    process_files = {}
    if patches.exists():
        trusted(patches)
        overlay = json.loads(patches.read_text())
        if (overlay.get("schema_version") != 1 or not overlay.get("native_modified")
                or overlay.get("revision") != REVISION or overlay.get("uv_lock_sha256") != LOCK_SHA256
                or overlay.get("base_build_sha256") != hashlib.sha256(receipt.read_bytes()).hexdigest()):
            raise RuntimeError("native patch baseline mismatch")
        for patch in overlay["patches"]:
            patch_file = Path(patch["patch_file"])
            trusted(patch_file)
            if hashlib.sha256(patch_file.read_bytes()).hexdigest() != patch["patch_sha256"]:
                raise RuntimeError("native patch evidence mismatch")
            for name, digests in patch["files"].items():
                installed = Path(name)
                trusted(installed)
                if hashlib.sha256(installed.read_bytes()).hexdigest() != digests["after_sha256"]:
                    raise RuntimeError("native patched installation mismatch")
            if patch.get("name") == "agent-core-process-observation":
                if process_files:
                    raise RuntimeError("duplicate native process patch evidence")
                process_files = {Path(name).name: {"path": name, "sha256": digests["after_sha256"]}
                                 for name, digests in patch["files"].items()
                                 if name.endswith(("/openjiuwen/core/sys_operation/local/aries_process_observer.py",
                                                   "/openjiuwen/core/sys_operation/local/shell_operation.py"))}
                if set(process_files) != {"aries_process_observer.py", "shell_operation.py"}:
                    raise RuntimeError("native process patch is missing its observer or creation hook")
    spec["native_process_observation_files"] = process_files
    spec["native_process_observation_supported"] = bool(spec.get("native_process_observation") and process_files)
    for port in (18865, 18866):
        with socket.socket() as probe:
            probe.bind(("0.0.0.0", port))
    runtime = Path(sys.argv[2])
    runtime.mkdir(mode=0o755)
    runtime.chmod(0o755)
    home = runtime / "home"
    home.mkdir(mode=0o700)
    os.chown(home, 60000, 60000)
    write(runtime / "launch.py", spec.pop("bootstrap"), 0o555, 0)
    audit = spec.pop("native_audit")
    process_extension = spec.pop("native_process_extension", None)
    model_extension = spec.pop("model_branches_extension", None)
    branch_protocol = spec.get("model_session_protocol", "")
    if branch_protocol not in ("", "jiuwen-tool-branches-v1"):
        raise RuntimeError("unsupported model session protocol")
    remote_sources = spec.pop("remote_sources", {})
    if spec["native_observation"] or spec.get("remote_binding") or branch_protocol:
        extensions = runtime / "extensions"
        extensions.mkdir(mode=0o755)
        extensions.chmod(0o755)
    if branch_protocol:
        extension = extensions / "aries_model_branches"
        extension.mkdir(mode=0o755)
        extension.chmod(0o755)
        write(extension / "extension.py", model_extension, 0o444, 0)
    if spec["native_observation"]:
        extension = extensions / "aries_native_audit"
        extension.mkdir(mode=0o755)
        extension.chmod(0o755)
        write(extension / "extension.py", audit, 0o444, 0)
        write(extension / "binding.json", json.dumps(spec["observation_binding"]), 0o444, 0)
    if spec["native_process_observation_supported"]:
        if spec.get("remote_binding") or not process_extension:
            raise RuntimeError("native process observation requires a LOCAL runtime extension")
        extension = extensions / "aries_native_process"
        extension.mkdir(mode=0o755)
        extension.chmod(0o755)
        write(extension / "extension.py", process_extension, 0o444, 0)
        write(extension / "binding.json", json.dumps(spec["observation_binding"]), 0o444, 0)
    if spec.get("remote_binding"):
        extension = extensions / "aries_remote"
        extension.mkdir(mode=0o755)
        extension.chmod(0o755)
        for name in ("extension.py", "provider.py", "identity.py", "wire.py"):
            write(extension / name, remote_sources[name], 0o444, 0)
        write(extension / "binding.json", json.dumps(spec["remote_binding"]), 0o400)
        # Native ProjectStore requires a directory on the Agent host. Its tools
        # use the remote provider; this directory contains no task files.
        workspace = Path(spec["workdir"])
        workspace.mkdir(parents=True, exist_ok=True)
        os.chown(workspace, 60000, 60000)
    write(runtime / "spec.json", json.dumps(spec), 0o400)


def run():
    if os.geteuid() != 60000:
        raise RuntimeError("dedicated native agent identity required")
    runtime = Path(sys.argv[2])
    spec = json.loads((runtime / "spec.json").read_text())
    home = runtime / "home"
    data = home / "data"
    config_dir = data / "config"
    os.environ.update(HOME=str(home), JIUWENSWARM_DATA_DIR=str(data),
                      JIUWENSWARM_CONFIG_DIR=str(config_dir),
                      PYTHONDONTWRITEBYTECODE="1", PYTHONUNBUFFERED="1")
    # Cloud envd and the contained launcher do not inherit Docker image ENV.
    if Path("/opt/aries-presentation/node_modules").is_dir():
        os.environ["PATH"] = str(Path(spec["python_path"]).parent) + os.pathsep + os.environ["PATH"]
        os.environ["NODE_PATH"] = "/opt/aries-presentation/node_modules"
    # SDK logging may initialize on the first native import. Project tool paths
    # remain bound to the separately registered task workdir below.
    os.chdir(home)
    import jiuwenswarm
    if Path(jiuwenswarm.__file__).resolve().parent.parent != Path(spec["source_dir"]):
        raise RuntimeError("loaded native package differs from pinned installation")
    from jiuwenswarm.common.utils import get_builtin_skills_dir, prepare_workspace
    # Native shared skills keep the same immutable paths in Agent and ToolSandbox.
    skills_dir = get_builtin_skills_dir().resolve(strict=True)
    os.environ["JIUWENSWARM_SHARED_SKILLS_DIRS"] = str(skills_dir)
    prepare_workspace(overwrite=False)
    import yaml
    config_file = config_dir / "config.yaml"
    config = yaml.safe_load(config_file.read_text()) or {}
    config.setdefault("flash", {}).update(enabled=True)
    config["flash"].setdefault("skill_selection", {}).update(enabled=False)
    config.setdefault("http_server", {}).update(enabled=True, host="0.0.0.0", port=18866)
    config.setdefault("startup", {}).setdefault("warmup", {}).update(enabled=False)
    # Linux internal startup can enable JiuwenBox even when enabled=False.
    config.setdefault("sandbox", {}).update(enabled=False, startup_mode="external")
    if spec.get("remote_binding"):
        config["sandbox"].update(enabled=True, type="aries", url=spec["remote_binding"]["base_url"])
    # Native evolution can demand the provider even when enabled=False. Sampling
    # zero keeps L0 off without changing skills, tools or their execution policy.
    observation = bool(spec["native_observation"])
    config.setdefault("agent_observability", {}).update(
        enabled=observation, exporter="file", backend="otlp",
        sample_rate=1.0 if observation else 0.0,
        redact_prompts=True, redact_completions=True,
        traces_dir=str(data / "aries-traces"), file_retention_days=0)
    if observation or spec.get("remote_binding") or spec.get("model_session_protocol"):
        extensions = config.setdefault("extensions", {})
        directories = extensions.get("extension_dirs", "")
        extensions["extension_dirs"] = ";".join(
            value for value in (directories if isinstance(directories, str) else "",
                                str(runtime / "extensions")) if value)
    config_file.write_text(yaml.safe_dump(config, allow_unicode=True, sort_keys=False))
    config_file.chmod(0o600)
    # AgentServer reloads this canonical .env with override=True. A second file
    # passed only through --dotenv is not sufficient for the pinned version.
    env_file = config_dir / ".env"
    model_env = {"API_BASE": spec["model_proxy_url"], "API_KEY": spec["proxy_token"],
                 "MODEL_NAME": spec["model"], "MODEL_PROVIDER": "OpenAI",
                 "AGENT_SERVER_HOST": "127.0.0.1", "AGENT_SERVER_PORT": "18865",
                 "JIUWENSWARM_SHARED_SKILLS_DIRS": str(skills_dir)}
    env_file.write_text("".join(k + "=" + json.dumps(v) + "\n" for k, v in model_env.items()))
    env_file.chmod(0o600)
    from jiuwenswarm.common.config import clear_config_cache, get_config
    clear_config_cache()
    process_supported = False
    if spec.get("native_process_observation"):
        process_supported = verify_process_observer(spec["native_process_observation_files"])
        if process_supported != spec["native_process_observation_supported"]:
            raise RuntimeError("native process capability changed after staging")
    from jiuwenswarm.server.handlers._shared import resolve_agent_request_mode
    resolved = resolve_agent_request_mode("agent")
    if resolved[0] != "flash":
        raise RuntimeError("native agent mode did not resolve to Flash")
    from jiuwenswarm.server.runtime.session.project_store import create_or_restore_project
    project, restored = create_or_restore_project("ARIES task", spec["workdir"], work_mode="work")
    if restored:
        raise RuntimeError("per-attempt project was unexpectedly reused")
    proof = {"schema_version": 1, "revision": REVISION, "uv_lock_sha256": LOCK_SHA256,
             "project_id": project.project_id, "workdir": spec["workdir"], "mode": resolved[0],
             "skill_selection": get_config()["flash"]["skill_selection"]["enabled"],
             "native_skills": {"directory": str(skills_dir),
                               "manifests": {p.parent.name: hashlib.sha256(p.read_bytes()).hexdigest()
                                             for p in sorted(skills_dir.glob("*/SKILL.md"))},
                               "scope": "configured_builtin_catalog_before_agent_start"},
             "sandbox_enabled": get_config()["sandbox"]["enabled"],
             "sandbox_type": get_config()["sandbox"].get("type"),
             "sandbox_startup_mode": get_config()["sandbox"]["startup_mode"],
             "native_observation": {key: get_config()["agent_observability"][key] for key in
                                    ("enabled", "exporter", "sample_rate", "redact_prompts",
                                     "redact_completions", "traces_dir")},
             "native_process_observation_supported": process_supported,
             "config_sha256": hashlib.sha256(config_file.read_bytes()).hexdigest(),
             "uid": os.geteuid(), "pid": os.getpid(),
             "measurement_scope": "configuration_before_agent_start"}
    write(home / "prepared.json", json.dumps(proof))
    os.execv(spec["python_path"], [spec["python_path"], "-m",
             "jiuwenswarm.server.app_agentserver", "--port", "18865", "--dotenv", str(env_file)])


if sys.argv[1] == "stage":
    stage()
elif sys.argv[1] == "run":
    run()
else:
    raise RuntimeError("invalid native bootstrap operation")

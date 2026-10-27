"""ECS-only E2B template build/boot checks; imports never perform cloud operations."""
import argparse
from datetime import datetime, timezone
import hashlib
from importlib.metadata import version
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import sys
import tempfile
import time
import uuid

SDK_VERSION = "2.58.0"
HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[2]
CONTEXT_FILES = ("prepare.sh", "check_runtime.py", "start.sh")


def utc_now():
    return datetime.now(timezone.utc).isoformat()


def read_pin(versions_file=ROOT / "configs/versions.json"):
    image = json.loads(Path(versions_file).read_text())["openclaw"]["image"]
    match = re.fullmatch(r"ghcr\.io/openclaw/openclaw:(\d{4}\.\d+\.\d+)(?:@sha256:[0-9a-f]{64})?", image)
    if not match:
        raise ValueError("template requires the explicit published OpenClaw version pin")
    return image, match.group(1)


def definition(template_class, context, image, native_version):
    return (template_class(file_context_path=str(context))
            .from_image(image)
            .set_user("root")
            .run_cmd("install -d -m 0755 -o root -g root /usr/local/share/aries-template /opt/aries-template", user="root")
            .copy("prepare.sh", "/tmp/aries-template-prepare.sh", user="root", mode=0o500)
            .copy("check_runtime.py", "/usr/local/share/aries-template/check_runtime.py", user="root", mode=0o555)
            .copy("start.sh", "/opt/aries-template/start.sh", user="root", mode=0o500)
            .run_cmd("/bin/sh /tmp/aries-template-prepare.sh " + shlex.quote(native_version), user="root")
            .run_cmd("rm /tmp/aries-template-prepare.sh", user="root")
            .set_workdir("/workspace/aries-case")
            .set_user("root")
            .set_start_cmd("exec /bin/sh /opt/aries-template/start.sh " + shlex.quote(native_version),
                           "test -f /opt/aries-template/ready.json"))


class Evidence:
    def __init__(self, path, document):
        self.path = Path(path).absolute()
        # A build context is assembled separately, but keep evidence away from sources too.
        if self.path == HERE or HERE in self.path.parents:
            raise ValueError("private evidence must be outside the template source directory")
        for parent in self.path.parents:
            if parent.is_symlink():
                raise ValueError("private evidence must not traverse symlinks")
        self.path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        descriptor = os.open(self.path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        self.file = os.fdopen(descriptor, "w")
        self.document = document
        self.update()

    def update(self, **fields):
        self.document.update(fields)
        self.file.seek(0)
        json.dump(self.document, self.file, sort_keys=True, indent=2)
        self.file.write("\n")
        self.file.truncate()
        self.file.flush()
        os.fsync(self.file.fileno())

    def close(self):
        self.file.close()


def build(template_class, args, key, evidence):
    image, native_version = read_pin(args.versions_file)
    name = "aries-openclaw-" + native_version.replace(".", "-") + "-" + uuid.uuid4().hex[:12]
    context_hashes = {name: hashlib.sha256((HERE / name).read_bytes()).hexdigest() for name in CONTEXT_FILES}
    evidence.update(source_image=image, expected_openclaw_version=native_version,
                    requested_name=name, context_sha256=context_hashes,
                    resources={"cpu_count": args.cpu_count, "memory_mb": args.memory_mb},
                    phase="build_requested", runtime_capabilities="UNRUN")
    with tempfile.TemporaryDirectory(prefix="aries-e2b-template-") as context:
        for filename in CONTEXT_FILES:
            shutil.copyfile(HERE / filename, Path(context) / filename)
        template = definition(template_class, context, image, native_version)
        # No logger callback: SDK build logs can contain identifiers and remote URLs.
        info = template_class.build_in_background(template, name=name, tags=["ab-v1"],
                cpu_count=args.cpu_count, memory_mb=args.memory_mb,
                api_key=key, debug=False, request_timeout=60)
        evidence.update(template_id=info.template_id, build_id=info.build_id,
                        name=info.name, alias=info.alias, tags=info.tags, phase="build_submitted")
        deadline = time.monotonic() + args.wait_seconds
        while time.monotonic() < deadline:
            status = template_class.get_build_status(info, api_key=key, debug=False, request_timeout=30)
            state = status.status.value
            if state == "ready":
                evidence.update(status="PASS", phase="template_published", finished_at=utc_now())
                return
            if state == "error":
                raise RuntimeError("remote template build failed")
            if state not in ("building", "waiting"):
                raise RuntimeError("unknown template build state")
            time.sleep(2)
    raise TimeoutError("template build may still be running; inspect the private build identifiers")


def verify(sandbox_class, not_found, command_exit, args, key, evidence):
    source = json.loads(Path(args.build_evidence).read_text())
    if source.get("status") != "PASS" or source.get("phase") != "template_published":
        raise ValueError("verify requires a successful template build record")
    if (not isinstance(source.get("template_id"), str)
            or not re.fullmatch(r"[a-z0-9_-]+", source["template_id"])
            or not isinstance(source.get("build_id"), str)
            or not source.get("expected_openclaw_version")):
        raise ValueError("template identity is absent")
    # Bare IDs select the default tag; a build UUID pins the published artifact.
    template_reference = source["template_id"] + ":" + str(uuid.UUID(source["build_id"]))
    allocation = uuid.uuid4().hex
    evidence.update(template_id=source["template_id"], build_id=source["build_id"],
                    template_reference=template_reference,
                    allocation_handle=allocation, requested_timeout_seconds=180,
                    phase="sandbox_create_requested", cleanup_confirmed=False)
    sandbox = None
    try:
        sandbox = sandbox_class.create(template=template_reference, timeout=180,
                    metadata={"aries_template_verify_handle": allocation},
                    allow_internet_access=False, network={"allow_public_traffic": False},
                    lifecycle={"on_timeout": "kill", "auto_resume": False}, api_key=key, debug=False, request_timeout=60)
        evidence.update(sandbox_id=sandbox.sandbox_id, phase="sandbox_created")
        command = "python3 -I /usr/local/share/aries-template/check_runtime.py --check-cgroup --expected-version " + shlex.quote(source["expected_openclaw_version"])
        try:
            result = sandbox.commands.run(command, user="root", timeout=90)
        except command_exit as result_error:
            result = result_error
        probe = json.loads(result.stdout)
        if result.exit_code != 0 or probe.get("status") != "PASS" or probe.get("cgroup", {}).get("status") != "PASS":
            evidence.update(status="UNSUPPORTED", runtime=probe, phase="runtime_probe_completed")
            return
        evidence.update(status="PASS", runtime=probe, phase="runtime_probe_completed")
    finally:
        if sandbox is not None:
            try:
                sandbox.kill(request_timeout=20)
            finally:
                try:
                    sandbox.get_info(request_timeout=20)
                except not_found:
                    evidence.update(cleanup_confirmed=True)
                else:
                    evidence.update(status="FAIL", cleanup_error_class="SandboxStillPresent")
        evidence.update(finished_at=utc_now())


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="operation", required=True)
    create = subparsers.add_parser("build")
    create.add_argument("--versions-file", type=Path, default=ROOT / "configs/versions.json",
                        help="same image catalog as the A/B profiles; relative to the working directory")
    create.add_argument("--cpu-count", type=int, required=True)
    create.add_argument("--memory-mb", type=int, required=True)
    create.add_argument("--wait-seconds", type=int, default=1800)
    check = subparsers.add_parser("verify")
    check.add_argument("--build-evidence", required=True)
    for subparser in (create, check):
        subparser.add_argument("--output", required=True, help="new private evidence file, never overwritten")
    args = parser.parse_args()
    if args.operation == "build" and min(args.cpu_count, args.memory_mb, args.wait_seconds) <= 0:
        parser.error("resource sizes and wait timeout must be positive")
    if sys.platform != "linux":
        parser.error("execute this tool on the remote Linux ECS")
    key = os.environ.get("E2B_API_KEY", "")
    if not key or version("e2b") != SDK_VERSION:
        parser.error("ECS must provide E2B_API_KEY and the pinned e2b==2.58.0 SDK")
    from e2b import CommandExitException, Sandbox, SandboxNotFoundException, Template
    evidence = Evidence(args.output, {"kind": "aries_template_" + args.operation,
                    "sdk_version": SDK_VERSION, "started_at": utc_now(), "status": "FAIL"})
    try:
        if args.operation == "build":
            build(Template, args, key, evidence)
        else:
            verify(Sandbox, SandboxNotFoundException, CommandExitException, args, key, evidence)
    except (Exception, KeyboardInterrupt) as exc:
        evidence.update(status="FAIL", error_class=type(exc).__name__, finished_at=utc_now())
        http_status = getattr(exc, "status_code", None)
        if type(http_status) is int and 100 <= http_status <= 599:
            evidence.update(error_http_status=http_status)
    finally:
        evidence.close()
    ok = evidence.document["status"] == "PASS" and (args.operation == "build" or evidence.document.get("cleanup_confirmed") is True)
    print(json.dumps({"operation": args.operation, "status": evidence.document["status"],
                      "cleanup_confirmed": evidence.document.get("cleanup_confirmed"),
                      "identifiers": "private evidence only"}))
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main())

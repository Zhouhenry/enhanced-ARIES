"""Trusted guest probe; --check-cgroup creates and removes one isolated cgroup."""
import argparse
import ctypes
import json
import os
from pathlib import Path
import pwd
import re
import stat
import subprocess
import sys
import time
import uuid


class Unsupported(RuntimeError):
    pass


def trusted_path(path):
    resolved = Path(path).resolve(strict=True)
    for item in [resolved, *resolved.parents]:
        info = item.stat()
        if info.st_uid != 0 or info.st_mode & 0o022:
            raise Unsupported("native installation is writable by an untrusted user")
    return str(resolved)


def version(argv):
    result = subprocess.run(argv, capture_output=True, text=True, timeout=20,
                            env={"PATH": "/usr/local/bin:/usr/bin:/bin"})
    if result.returncode != 0:
        raise Unsupported("required executable version check failed")
    return result.stdout.strip()


def probe_cgroup():
    base = Path("/sys/fs/cgroup")
    trusted_path(base)
    if not (base / "cgroup.controllers").is_file():
        raise Unsupported("cgroup v2 is unavailable")
    group = base / ("aries-template-probe-" + uuid.uuid4().hex)
    child = None
    try:
        group.mkdir(mode=0o700)
        if not (group / "cgroup.kill").is_file():
            raise Unsupported("cgroup.kill is unavailable")

        def enter():
            (group / "cgroup.procs").write_text(str(os.getpid()))
            libc = ctypes.CDLL(None, use_errno=True)
            if libc.prctl(38, 1, 0, 0, 0) != 0:
                raise OSError(ctypes.get_errno(), "no_new_privs failed")
            os.setgroups([])
            os.setgid(60000)
            os.setuid(60000)

        child = subprocess.Popen(["/bin/sleep", "30"], preexec_fn=enter,
                                 stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
                                 stderr=subprocess.DEVNULL, env={"PATH": "/usr/bin:/bin"})
        if str(child.pid) not in (group / "cgroup.procs").read_text().split():
            raise Unsupported("probe process did not enter its dedicated cgroup")
        status = dict(line.split(":", 1) for line in Path(f"/proc/{child.pid}/status").read_text().splitlines() if ":" in line)
        if ([int(uid) for uid in status["Uid"].split()] != [60000] * 4
                or status["NoNewPrivs"].strip() != "1"
                or int(status["CapEff"].strip(), 16) != 0):
            raise Unsupported("contained process retained unexpected privileges")
        (group / "cgroup.kill").write_text("1")
        child.wait(timeout=5)
        deadline = time.monotonic() + 5
        while time.monotonic() < deadline:
            events = dict(line.split() for line in (group / "cgroup.events").read_text().splitlines())
            if events.get("populated") == "0":
                return {"status": "PASS", "uid": 60000, "no_new_privs": 1,
                        "effective_capabilities": 0, "cgroup_kill": True, "populated_after_kill": 0}
            time.sleep(0.05)
        raise Unsupported("cgroup did not confirm emptiness")
    except (OSError, subprocess.SubprocessError) as exc:
        raise Unsupported("cgroup delegation or process containment failed") from exc
    finally:
        if child is not None and child.poll() is None:
            child.kill()
            child.wait(timeout=5)
        if group.exists():
            group.rmdir()


def inspect(expected_version, check_cgroup=False):
    if os.geteuid() != 0:
        raise Unsupported("probe requires the trusted root controller")
    account = pwd.getpwuid(60000)
    if account.pw_name != "aries-agent" or account.pw_gid != 60000:
        raise Unsupported("dedicated agent identity is unavailable")
    executable = trusted_path("/usr/local/bin/openclaw")
    if executable != "/app/openclaw.mjs":
        raise Unsupported("OpenClaw entrypoint does not match the published image layout")
    trusted_path("/usr/local/bin/node")
    # Entry-point checks alone do not protect imported JS or Node dependencies.
    for directory, directories, files in os.walk("/app", followlinks=False):
        for name in [directory, *(str(Path(directory) / item) for item in directories + files)]:
            info = os.lstat(name)
            if info.st_uid != 0 or (not stat.S_ISLNK(info.st_mode) and info.st_mode & 0o222):
                raise Unsupported("OpenClaw application tree is not root-owned and read-only")
            if stat.S_ISLNK(info.st_mode):
                trusted_path(name)
    native = version(["/usr/local/bin/openclaw", "--version"])
    if not re.fullmatch(r"(?:OpenClaw )?" + re.escape(expected_version) + r"(?: \([0-9a-f]+\))?", native):
        raise Unsupported("OpenClaw runtime version differs from the repository pin")
    evidence = {"status": "PASS", "openclaw": native, "openclaw_entrypoint": executable,
                "node": version(["/usr/local/bin/node", "--version"]),
                "python": version(["python3", "--version"]),
                "pytest": version(["python3", "-I", "-m", "pytest", "--version"]),
                "git": version(["git", "--version"]), "agent_uid": account.pw_uid,
                "kernel": os.uname().release, "hostname": os.uname().nodename,
                "cgroup": {"status": "UNRUN", "reason": "image build is not runtime containment proof"}}
    if check_cgroup:
        evidence["cgroup"] = probe_cgroup()
    return evidence


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--expected-version", required=True)
    parser.add_argument("--check-cgroup", action="store_true")
    args = parser.parse_args()
    try:
        evidence = inspect(args.expected_version, args.check_cgroup)
    except Exception as exc:
        # No raw subprocess/SDK output or inherited environment is serialized.
        evidence = {"status": "UNSUPPORTED", "error_class": type(exc).__name__}
        if isinstance(exc, Unsupported):
            evidence["reason"] = str(exc)
    print(json.dumps(evidence, sort_keys=True))
    return 0 if evidence["status"] == "PASS" else 2


if __name__ == "__main__":
    sys.exit(main())

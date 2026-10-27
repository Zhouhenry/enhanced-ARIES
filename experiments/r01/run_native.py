"""Run the isolated native identity PoC on the authorized ECS; no cloud APIs."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import sys
import time

BASE = "ghcr.io/openclaw/openclaw:2026.7.1@sha256:6a31d44b2944e7adcd2b582bf6fb463111264ebca97a0201795b799135bd102c"


def main():
    if not sys.platform.startswith("linux"):
        raise SystemExit("authorized ECS only")
    parser = argparse.ArgumentParser()
    parser.add_argument("--source", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    root = args.output.resolve()
    root.mkdir(mode=0o700, exist_ok=False)
    here = Path(__file__).resolve().parent
    source = args.source.resolve()
    env = dict(os.environ, GOMODCACHE="/opt/aries-agent-bench/.cache/go-mod",
               GOCACHE="/tmp/aries-current-source-validation-go-build-2", GOPROXY="off",
               GOSUMDB="off", GOTOOLCHAIN="local", GOMAXPROCS="2", GOFLAGS="-p=2",
               CGO_ENABLED="0", DOCKER_BUILDKIT="0")
    evidence = {"schema_version": 1, "status": "FAIL", "base_image": BASE,
                "scope": "native createExecTool through actual SSH Bridge to Docker target",
                "cloud_created": 0, "model_calls": 0, "formal_image_changed": False,
                "steps": [], "cleanup": []}

    def run(argv, name, timeout=180, check=True, cwd=None):
        with (root / (name + ".log")).open("wb") as log:
            result = subprocess.run(argv, stdout=log, stderr=subprocess.STDOUT,
                                    env=env, timeout=timeout, cwd=cwd)
        evidence["steps"].append({"name": name, "exit_code": result.returncode})
        if check and result.returncode:
            raise RuntimeError(name + " failed; see private log")
        return result

    try:
        run(["python3", str(here / "native_patch.py"),
             "/opt/aries-artifacts/observability-r01-native-source-1", str(root / "image")], "patch")
        candidate = "aries-openclaw:" + root.name
        run(["docker", "build", "--pull=false", "--network=none", "-t", candidate,
             str(root / "image")], "image-build")
        candidate = subprocess.check_output(["docker", "image", "inspect", "--format={{.Id}}", candidate], text=True).strip()
        evidence["candidate_image_id"] = candidate
        fixture = source / "experiments/r01/bridge"
        fixture.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(here / "bridge/main.go", fixture / "main.go")
        run(["gofmt", "-w", str(fixture / "main.go")], "gofmt")
        source_files = sorted(p for p in source.rglob("*") if p.is_file() and
                              not any(part.startswith('.') or part in ('bin', 'runs') for part in p.relative_to(source).parts) and
                              (p.suffix == '.go' or p.name in ('go.mod', 'go.sum')))
        (root / "go-source-manifest.json").write_text(json.dumps({str(p.relative_to(source)):
            hashlib.sha256(p.read_bytes()).hexdigest() for p in source_files}, indent=2, sort_keys=True) + '\n')
        evidence["input_sources"] = {str(p.relative_to(here)): hashlib.sha256(p.read_bytes()).hexdigest()
                                      for p in here.rglob('*') if p.is_file()}
        run(["go", "build", "-o", str(root / "bridge"), "./experiments/r01/bridge"], "bridge-build", cwd=source)
        run(["go", "build", "-o", str(root / "aries-ssh-client"), "./cmd/aries-ssh-client"], "client-build", cwd=source)
        (root / "aries-ssh-client").chmod(0o555)
        for name, image in [("baseline", BASE), ("candidate", candidate)]:
            case = root / name
            case.mkdir(mode=0o700)
            guest = root.name + "-" + name + "-guest"
            native = root.name + "-" + name + "-native"
            process = None
            bridge_log = None
            try:
                run(["docker", "run", "-d", "--name", guest, "--network=none", "--pids-limit=128",
                     "--memory=256m", "--cpus=1", "--entrypoint=/bin/sh", "--user=0:0",
                     BASE, "-c", "mkdir -p /work; chown 1000:1000 /work; exec sleep 600"], name + "-guest-start")
                bridge_log = (case / "bridge.log").open("wb")
                process = subprocess.Popen([str(root / "bridge"), "--container", guest,
                    "--output", str(case), "--scope", "r01-poc-" + name, "--session", "r01-session"],
                    stdout=bridge_log, stderr=subprocess.STDOUT, env=env)
                deadline = time.monotonic() + 30
                while not (case / "ready.json").exists():
                    if process.poll() is not None or time.monotonic() > deadline:
                        raise RuntimeError("Bridge readiness failed: " + name)
                    time.sleep(0.05)
                (case / "ready.json").chmod(0o444)
                (case / "scope.json").write_text(json.dumps({"scope_id": "r01-poc-" + name,
                                                             "agent_session_id": "r01-session"}))
                (case / "scope.json").chmod(0o444)
                run(["docker", "run", "--name", native, "--network=host", "--read-only",
                     "--cap-drop=ALL", "--pids-limit=128", "--memory=1g", "--cpus=1",
                     "--tmpfs=/tmp:rw,nosuid,nodev,size=64m", "--user=1000:1000",
                     "-e", "HOME=/tmp", "-e", "OPENCLAW_STATE_DIR=/tmp/openclaw-state",
                     "-v", str(case / "ready.json") + ":/run/aries/probe.json:ro",
                     "-v", str(case / "scope.json") + ":/run/aries/tool-identity-scope.json:ro",
                     "-v", str(root / "aries-ssh-client") + ":/opt/aries/bin/aries-ssh-client:ro",
                     "-v", str(here / "native_probe.mjs") + ":/run/aries/native_probe.mjs:ro",
                     "--entrypoint=node", image, "/run/aries/native_probe.mjs"], name + "-native", timeout=90)
            finally:
                # These names belong only to this fresh experiment output directory.
                run(["docker", "rm", "-f", native], name + "-native-remove", check=False)
                if process is not None:
                    process.send_signal(signal.SIGTERM)
                    try:
                        process.wait(timeout=25)
                    except subprocess.TimeoutExpired:
                        process.kill()
                        process.wait(timeout=5)
                    evidence["cleanup"].append({"case": name, "bridge_exit": process.returncode,
                                                  "bridge_stop_confirmed": (case / "stopped").exists()})
                if bridge_log:
                    bridge_log.close()
                run(["docker", "rm", "-f", guest], name + "-guest-remove", check=False)
                names = subprocess.run(["docker", "container", "ls", "-a", "--format={{.Names}}"],
                                       capture_output=True, text=True, timeout=15, env=env)
                absent = []
                for owned in (guest, native):
                    confirmed = names.returncode == 0 and owned not in names.stdout.splitlines()
                    absent.append(confirmed)
                    evidence["cleanup"].append({"container": owned, "absent_confirmed": confirmed})
                if not all(absent):
                    raise RuntimeError("owned container absence not confirmed")
        run(["python3", str(here / "reconcile.py"), str(root)], "reconcile")
        assert all(row.get("bridge_exit", 0) == 0 and row.get("bridge_stop_confirmed", True) and
                   row.get("absent_confirmed", True) for row in evidence["cleanup"])
        evidence["status"] = "PASS_NATIVE_DOCKER_POC_ONLY"
    except Exception as exc:
        evidence["error_type"] = type(exc).__name__
        evidence["error"] = str(exc)
    finally:
        evidence["files"] = {str(path.relative_to(root)): hashlib.sha256(path.read_bytes()).hexdigest()
                             for path in root.rglob("*") if path.is_file() and path.name != "evidence.json"}
        (root / "evidence.json").write_text(json.dumps(evidence, indent=2, sort_keys=True) + "\n")
        print(json.dumps({"status": evidence["status"], "root": str(root), "cleanup": evidence["cleanup"]}))
    if evidence["status"] == "FAIL":
        raise SystemExit(1)


if __name__ == "__main__":
    main()

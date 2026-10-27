"""ECS-only image7-process1 build from a passing, separately generated candidate."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import time


HERE = Path(__file__).resolve().parent
BASE_IMAGE = "swr.cn-southwest-2.myhuaweicloud.com/benchmark-zhoutao/aries-jiuwenswarm:0.2.3-e2c9d519-image5@sha256:b4078957d488e1cb52e9cc85ef5555208f0281a545c580a740a619f52127c6b1"
NATIVE_PYTHON = "/opt/jiuwenswarm/.venv/bin/python"


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--candidate", type=Path, required=True)
    parser.add_argument("--test-evidence", type=Path, required=True)
    parser.add_argument("--verify", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--tag", default="aries-jiuwenswarm:e2c9d519-image7-process1")
    args = parser.parse_args()
    if sys.platform != "linux" or os.geteuid() != 0:
        parser.error("authorized Linux ECS root only")
    if subprocess.run(["docker", "image", "inspect", args.tag], capture_output=True).returncode == 0:
        parser.error("candidate image tag already exists")
    tested = json.loads(args.test_evidence.read_text())
    assert tested["status"] == "PASS_IMAGE5_BIND_MOUNTED_NATIVE_PROCESS_TESTS"
    assert tested["inputs"]["native_process.py"] == sha(HERE / "native_process.py")
    assert tested["inputs"]["test_native_process.py"] == sha(HERE / "test_native_process.py")
    assert tested["candidate_manifest"] == json.loads((args.candidate / "manifest.json").read_text())
    out = args.output.absolute()
    out.mkdir(parents=True, exist_ok=False)
    out.chmod(0o755)
    evidence = {"schema_version": 1, "status": "FAIL", "started_unix_ns": time.time_ns(),
                "scope": "local image7-process1 only", "base_image": BASE_IMAGE,
                "registry_push": "UNRUN", "cloud_created": 0, "model_calls": 0,
                "containers_removed": [], "test_evidence_sha256": sha(args.test_evidence)}
    def write():
        (out / "evidence.json").write_text(json.dumps(evidence, indent=2) + "\n")
    def run(command, log, timeout=300):
        result = subprocess.run(command, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=timeout)
        (out / log).write_bytes(result.stdout)
        if result.returncode:
            raise RuntimeError(log + " exit " + str(result.returncode))
        return result.stdout.decode()
    def container(suffix, command, *, root=False, mounts=()):
        name = "aries-process-" + out.name + "-" + suffix
        options = ["docker", "run", "--name", name, "--label", "aries.owner=jiuwen-process-native-1",
                   "--network=none", "--read-only", "--workdir=/tmp", "--cpus=2", "--memory=2g",
                   "--tmpfs=/tmp:rw,nosuid,nodev,size=256m", "-e", "HOME=/tmp", "-e", "PYTHONDONTWRITEBYTECODE=1"]
        if not root:
            options += ["--user=60000:60000"]
        for mount in mounts:
            options += ["-v", mount]
        try:
            return run(options + ["--entrypoint", command[0], evidence["image_id"], *command[1:]], suffix + ".log")
        finally:
            checked = subprocess.run(["docker", "inspect", name], capture_output=True, text=True)
            if checked.returncode == 0:
                info = json.loads(checked.stdout)[0]
                assert info["Image"] == evidence["image_id"]
                assert info["Config"]["Labels"].get("aries.owner") == "jiuwen-process-native-1"
                subprocess.run(["docker", "rm", "-f", name], check=True, stdout=subprocess.DEVNULL)
            assert not subprocess.check_output(["docker", "ps", "-aq", "--filter", "name=^/" + name + "$"], text=True).strip()
            evidence["containers_removed"].append(name)
            write()
    try:
        base = json.loads(run(["docker", "image", "inspect", BASE_IMAGE], "base-image.json"))[0]
        evidence["base_image_id"] = base["Id"]
        context = out / "context"
        context.mkdir()
        shutil.copytree(args.candidate, context / "candidate")
        for filename in ("Dockerfile", "apply_image.py"):
            shutil.copyfile(HERE / filename, context / filename)
        shutil.copyfile(args.verify, context / "verify.py")
        evidence["context_sha256"] = {str(path.relative_to(context)): sha(path)
                                     for path in sorted(context.rglob("*")) if path.is_file()}
        write()
        run(["docker", "buildx", "build", "--load", "--pull=false", "--network=none", "--progress=plain",
             "--label", "aries.owner=jiuwen-process-native-1", "--build-arg", "BASE_IMAGE=" + BASE_IMAGE,
             "-t", args.tag, str(context)], "build.log", timeout=600)
        image = json.loads(run(["docker", "image", "inspect", args.tag], "image.json"))[0]
        assert image["RootFS"]["Layers"][:len(base["RootFS"]["Layers"])] == base["RootFS"]["Layers"]
        evidence.update(image_id=image["Id"], image_tag=args.tag, image_size_bytes=image["Size"],
                        added_size_bytes=image["Size"] - base["Size"], base_layers_reused=True)
        write()
        probe = json.loads(container("verify", ["python3", "-B", "-I", "/opt/aries-template/verify.py"]))
        assert probe["status"] == "PASS_IMAGE_ONLY" and probe["native_modified"] is True
        evidence["image_probe"] = probe
        overlay = json.loads(container("manifest", ["python3", "-B", "-I", "-c",
            "from pathlib import Path;print(Path('/opt/jiuwenswarm/aries-patches.json').read_text())"]))
        (out / "aries-patches.json").write_text(json.dumps(overlay, indent=2, sort_keys=True) + "\n")
        assert [p["name"] for p in overlay["patches"]] == ["agent-core-tool-context", "jiuwenswarm-remote-tools", "agent-core-process-observation"]
        container("tests", [NATIVE_PYTHON, "-B", "-I", "/input/test_native_process.py", "-v"],
                  mounts=[str(HERE) + ":/input:ro"])
        after = json.loads(run(["docker", "image", "inspect", BASE_IMAGE], "base-after.json"))[0]
        assert after["Id"] == base["Id"]
        evidence.update(status="PASS_IMAGE7_PROCESS1_LOCAL_ONLY", base_image_unchanged=True,
                        native_patch_manifest_sha256=probe["native_patch_manifest_sha256"],
                        tests_log_sha256=sha(out / "tests.log"))
    except BaseException as exc:
        evidence["error_type"] = type(exc).__name__
        raise
    finally:
        evidence["finished_unix_ns"] = time.time_ns()
        write()
        print(json.dumps({key: evidence[key] for key in ("status", "image_id", "containers_removed") if key in evidence}))


if __name__ == "__main__":
    main()

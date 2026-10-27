"""ECS-only linked guest-frame evidence; no SDK/cloud or native-Agent acceptance."""
import argparse
import base64
import ctypes
import hashlib
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import time

from guest_exec import strict_json_loads
from server import Adapter, GUEST_EXEC, Job


class FrameHandle:
    def __init__(self, data):
        self.data = data

    def __iter__(self):
        for offset in range(0, len(self.data), 113):
            yield self.data[offset:offset + 113], None, None

    def wait(self):
        return type("Result", (), {"exit_code": 0})()

    def kill(self):
        raise AssertionError("valid guest frames must not invoke kill")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--evidence-dir", type=Path, required=True)
    args = parser.parse_args()
    if not sys.platform.startswith("linux"):
        parser.error("run only on the authorized remote Linux ECS")
    os.umask(0o077)
    root = args.evidence_dir.resolve()
    root.mkdir(parents=True, exist_ok=False)
    if ctypes.CDLL(None, use_errno=True).prctl(36, 1, 0, 0, 0) != 0:
        raise RuntimeError("probe descendant ownership unavailable")
    program = (
        "import json,os,pathlib,sys;"
        "status=pathlib.Path('/proc/self/stat').read_text().rsplit(')',1)[1].split();"
        "pathlib.Path(str(os.getpid())+'.json').write_text(json.dumps({'pid':os.getpid(),"
        "'ppid':os.getppid(),'start_ticks':int(status[19]),'uid':os.getuid(),'gid':os.getgid(),"
        "'cgroup':pathlib.Path('/proc/self/cgroup').read_text(),'cwd':os.getcwd(),'argv':sys.argv}));"
        "data=sys.stdin.buffer.read();sys.stdout.buffer.write(data);"
        "sys.stderr.buffer.write(b'err\\x00\\xff');sys.exit(7)"
    )
    processes, observations, wrappers = {}, {}, {}
    result = {"schema": 1, "status": "RUNNING", "scope": __doc__, "cloud_creates": 0,
              "samples": [], "completion_order": ["second", "first", "legacy"]}
    sha = lambda data: hashlib.sha256(data).hexdigest()
    result["sources"] = {name: sha(Path(__file__).with_name(name).read_bytes())
                         for name in ("guest_exec.py", "server.py", "identity_probe.py")}
    try:
        for name in ("first", "second", "legacy"):
            spec = {"argv": [sys.executable, "-c", program], "cwd": str(root), "env": {}}
            if name != "legacy":
                observation = {"schema_version": 1, "native_identity": {
                    "schema_version": 1, "scope_id": "attempt1", "agent_session_id": "session1",
                    "tool_call_id": "tool-" + name, "native_invocation_id": "invocation-" + name,
                    "ssh_request_id": "request-" + name},
                    "execution_id": "execution-" + name, "process_id": "process-" + name}
                spec["observation"] = observation
                observations[name] = observation
            encoded = base64.b64encode(json.dumps(spec).encode()).decode()
            process = subprocess.Popen([sys.executable, "-c", GUEST_EXEC, encoded],
                                       stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                       start_new_session=True)
            processes[name] = process
            fields = Path(f"/proc/{process.pid}/stat").read_text().rsplit(")", 1)[1].split()
            wrappers[name] = {"pid": process.pid, "start_ticks": int(fields[19])}
        for name in result["completion_order"]:
            process = processes[name]
            stdout, stderr = process.communicate(input=bytes(range(256)), timeout=5)
            assert process.returncode == 0 and not stderr
            frames = [strict_json_loads(line) for line in stdout.splitlines()]
            observation = observations.get(name)
            job = Job(FrameHandle(stdout.decode()), 1024, observation)
            adapter = Adapter(None, Exception, Exception, LookupError, templates=[])
            adapter.drain(job)
            assert job.done and job.events[-1]["exit_code"] == 7
            if observation is not None:
                assert frames[0] == {"kind": "execution_identity", "phase": "before_popen", "observation": observation}
                assert all(frame["observation"] == observation for frame in frames)
                assert job.events[0] == frames[0]
            payload = {stream: b"".join(base64.b64decode(frame["data_base64"])
                                      for frame in frames if frame.get("stream") == stream)
                       for stream in ("stdout", "stderr")}
            assert payload["stdout"] == bytes(range(256)) and payload["stderr"] == b"err\0\xff"
            raw_path = root / (name + "-guest-frames.jsonl")
            raw_path.write_bytes(stdout)
            child = [strict_json_loads(path.read_text()) for path in root.glob("*.json")
                     if path.stem.isdigit()]
            child = [row for row in child if row["ppid"] == process.pid]
            assert len(child) == 1
            assert not Path(f"/proc/{process.pid}").exists() and not Path(f"/proc/{child[0]['pid']}").exists()
            result["samples"].append({"name": name, "wrapper_identity": wrappers[name], "child_identity": child[0],
                "observation": observation, "raw_guest_frames": str(raw_path), "raw_frames_sha256": sha(stdout),
                "guest_receipt": frames[0] if observation is not None else None,
                "adapter_receipt": job.events[0] if observation is not None else None,
                "stdout_sha256": sha(payload["stdout"]), "stderr_sha256": sha(payload["stderr"]),
                "exit_code": job.events[-1]["exit_code"], "wrapper_and_child_absent": True})
        for sample in result["samples"][1:]:
            for field in ("uid", "gid", "cgroup", "cwd", "argv"):
                assert sample["child_identity"][field] == result["samples"][0]["child_identity"][field]
        result["status"] = "PASS_LINKED_GUEST_SCOPE"
    except BaseException as error:
        result.update(status="FAIL", error_class=type(error).__name__, error=str(error))
        raise
    finally:
        for process in processes.values():
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
            process.communicate(timeout=5)
        deadline = time.monotonic() + 5
        while True:
            try:
                pid, _ = os.waitpid(-1, os.WNOHANG)
            except ChildProcessError:
                break
            if pid == 0:
                if time.monotonic() >= deadline:
                    raise AssertionError("probe descendant reap timeout")
                time.sleep(.01)
        (root / "evidence.json").write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps({"status": result["status"], "evidence": str(root / "evidence.json"),
                      "sha256": sha((root / "evidence.json").read_bytes())}))


if __name__ == "__main__":
    main()

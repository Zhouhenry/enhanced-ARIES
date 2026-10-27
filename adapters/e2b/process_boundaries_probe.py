"""ECS-only R07 semantic comparison of archived and current guest wrappers.

This creates no cloud resources and does not exercise SDK cancellation.  The
subreaper belongs to the probe alone so every test descendant can be reaped.
"""
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


FIXTURE = r'''
import json, os, pathlib, signal, sys, time
def identity(name):
    status = pathlib.Path('/proc/self/stat').read_text().rsplit(')', 1)[1].split()
    destination = pathlib.Path(name + '.json')
    temporary = pathlib.Path(name + '.tmp')
    temporary.write_text(json.dumps({
        'pid': os.getpid(), 'start_ticks': int(status[19]),
        'ppid': os.getppid(), 'uid': os.getuid(), 'gid': os.getgid(),
        'cwd': os.getcwd(), 'argv': sys.argv, 'test_env': os.environ['ARIES_R07_VALUE'],
        'cgroup': pathlib.Path('/proc/self/cgroup').read_text(),
        'isatty': [os.isatty(fd) for fd in (0, 1, 2)]}))
    os.replace(temporary, destination)
identity('child')
case = sys.argv[1]
if case == 'no_output':
    pass
elif case == 'split_binary':
    sys.stdout.buffer.write(sys.stdin.buffer.read() + b'out\x00\xff')
    sys.stderr.buffer.write(b'err\x00\xff')
    sys.exit(7)
elif case == 'delayed':
    time.sleep(.08)
    sys.stdout.buffer.write(b'out'); sys.stdout.flush()
    time.sleep(.04)
    sys.stderr.buffer.write(b'err'); sys.stderr.flush()
elif case == 'long_output':
    for i in range(8):
        sys.stdout.buffer.write(bytes(range(256)) * 256); sys.stdout.flush()
        sys.stderr.buffer.write(bytes(reversed(range(256))) * 256); sys.stderr.flush()
elif case == 'descendant_pipe':
    pid = os.fork()
    if pid == 0:
        identity('descendant')
        time.sleep(.15)
        os.write(1, b'grandchild'); os.write(2, b'err-grandchild')
    os._exit(0)
elif case == 'signal':
    os.kill(os.getpid(), signal.SIGTERM)
elif case == 'exception':
    raise RuntimeError('r07-controlled-fixture-exception')
elif case in ('timeout', 'cancel'):
    time.sleep(30)
else:
    raise AssertionError(case)
'''


def sha(data):
    return hashlib.sha256(data).hexdigest()


def identity(pid):
    try:
        fields = Path(f"/proc/{pid}/stat").read_text().rsplit(")", 1)[1].split()
        return {"pid": pid, "start_ticks": int(fields[19]), "state": fields[0]}
    except FileNotFoundError:
        return None


def same_process(saved):
    current = identity(saved["pid"])
    return current is not None and current["start_ticks"] == saved["start_ticks"]


def write_json(path, value):
    path.write_text(json.dumps(value, indent=2) + "\n")


def run_one(wrapper, case, work):
    for name in ("child.json", "descendant.json"):
        (work / name).unlink(missing_ok=True)
    argv = [sys.executable, str(work / "fixture.py"), case]
    if case == "exec_missing":
        argv = [str(work / "definitely-missing-command")]
    spec = {"argv": argv, "cwd": str(work), "env": {"ARIES_R07_VALUE": "literal ' $HOME `id`"}}
    encoded = base64.b64encode(json.dumps(spec).encode()).decode()
    process = subprocess.Popen([sys.executable, str(wrapper), encoded], stdin=subprocess.PIPE,
                               stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
    wrapper_id = identity(process.pid)
    interruption = None
    known = []
    try:
        if case in ("timeout", "cancel"):
            deadline = time.monotonic() + 5
            while not (work / "child.json").exists():
                if process.poll() is not None or time.monotonic() >= deadline:
                    raise AssertionError("child readiness missing")
                time.sleep(.01)
            child_id = json.loads((work / "child.json").read_text())
            if case == "timeout":
                try:
                    process.communicate(timeout=.05)
                    raise AssertionError("expected orchestration timeout")
                except subprocess.TimeoutExpired:
                    interruption = "probe_wait_timeout_then_sigterm_wrapper"
            else:
                interruption = "probe_cancel_sigterm_wrapper"
            process.terminate()
            stdout, stderr = process.communicate(timeout=5)
            child_present_after_cancel = same_process(child_id)
            child_after_cancel = identity(child_id["pid"])
        else:
            stdout, stderr = process.communicate(input=bytes(range(256)), timeout=10)
            child_present_after_cancel = None
            child_after_cancel = None
        frames = [json.loads(line) for line in stdout.splitlines()]
        payload = {stream: b"".join(base64.b64decode(frame["data_base64"])
                                  for frame in frames if frame.get("stream") == stream)
                   for stream in ("stdout", "stderr")}
        identities = {name: json.loads((work / (name + ".json")).read_text())
                      for name in ("child", "descendant") if (work / (name + ".json")).exists()}
        known = list(identities.values())
        terminal = [frame for frame in frames if "exit_code" in frame]
        event_sequence = []
        for number, frame in enumerate(frames, 1):
            event = {"sequence": number, **frame}
            if "data_base64" in event:
                data = base64.b64decode(event.pop("data_base64"))
                event.update(bytes=len(data), sha256=sha(data))
            event_sequence.append(event)
        result = {"case": case, "wrapper": wrapper.name, "wrapper_identity": wrapper_id,
                  "process_identities": identities, "wrapper_exit_code": process.returncode,
                  "wrapper_stderr_bytes": len(stderr), "wrapper_stderr_sha256": sha(stderr),
                  "output": {stream: {"bytes": len(data), "sha256": sha(data)}
                             for stream, data in payload.items()},
                  "events": event_sequence, "terminal": terminal,
                  "interruption": interruption,
                  "direct_child_present_after_wrapper_cancel": child_present_after_cancel,
                  "direct_child_identity_after_cancel": child_after_cancel}
        if case in ("timeout", "cancel"):
            assert process.returncode == -signal.SIGTERM and not terminal
        elif case == "exec_missing":
            assert process.returncode != 0 and not terminal
        else:
            assert process.returncode == 0 and len(terminal) == 1
        return result
    finally:
        # This is probe-owned cleanup, never a claimed production kill/absence
        # capability.  Preserve the observed cancellation outcome above it.
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        if process.poll() is None:
            process.wait(timeout=5)
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
        for saved in [wrapper_id, *known]:
            assert saved is not None and not same_process(saved), "owned process remains"
        for stream in (process.stdin, process.stdout, process.stderr):
            if stream is not None:
                stream.close()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--baseline", type=Path, required=True)
    parser.add_argument("--current", type=Path, required=True)
    parser.add_argument("--evidence-dir", type=Path, required=True)
    args = parser.parse_args()
    if not sys.platform.startswith("linux"):
        parser.error("run only on the authorized remote Linux ECS")
    os.umask(0o077)
    root = args.evidence_dir.resolve()
    root.mkdir(parents=True, exist_ok=False)
    work = root / "work"
    work.mkdir()
    (work / "fixture.py").write_text(FIXTURE)
    wrappers = {}
    for kind, source in (("baseline", args.baseline), ("current", args.current)):
        target = root / (kind + "-guest-exec.py")
        target.write_bytes(source.read_bytes())
        wrappers[kind] = target
    # Adopt only our disposable test descendants for explicit reaping.  This
    # does not alter child argv, env, cwd, UID, cgroup, signals or output bytes.
    libc = ctypes.CDLL(None, use_errno=True)
    if libc.prctl(36, 1, 0, 0, 0) != 0:  # PR_SET_CHILD_SUBREAPER
        raise OSError(ctypes.get_errno(), "could not own probe descendants")
    cases = ("no_output", "split_binary", "delayed", "long_output", "descendant_pipe",
             "signal", "exception", "exec_missing", "timeout", "cancel")
    evidence = {"schema": 1, "kind": "ecs_r07_wrapper_semantic_probe", "status": "RUNNING",
                "sources": {kind: {"path": str(source), "sha256": sha(source.read_bytes())}
                            for kind, source in wrappers.items()},
                "fixture_sha256": sha(FIXTURE.encode()), "samples": [],
                "scope": "ECS subprocesses; not Sandbox/SDK/native-B acceptance",
                "cloud_creates": 0, "cleanup_scope": "probe-owned process group and adopted descendants",
                "limitations": ["wait return is observed direct-child exit, not kernel event time",
                                "cancel/timeout are probe transport interruption; SDK semantics remain UNRUN",
                                "pipe chunk boundaries can vary; compare each decoded stream's bytes",
                                "first_output_ns is combined stdout/stderr, not separate first-byte fields",
                                "no claim of production descendant absence"]}
    write_json(root / "evidence.json", evidence)
    try:
        for case in cases:
            pair = {}
            for kind, wrapper in wrappers.items():
                fd_before = len(list(Path("/proc/self/fd").iterdir()))
                sample = run_one(wrapper, case, work)
                fd_after = len(list(Path("/proc/self/fd").iterdir()))
                sample["probe_open_fds"] = {"before": fd_before, "after": fd_after}
                assert fd_before == fd_after, "probe leaked descriptors"
                sample["kind"] = kind
                evidence["samples"].append(sample)
                pair[kind] = sample
                write_json(root / "evidence.json", evidence)
            before, after = pair["baseline"], pair["current"]
            for field in ("wrapper_exit_code", "output", "direct_child_present_after_wrapper_cancel"):
                assert before[field] == after[field], (case, field)
            assert [row["exit_code"] for row in before["terminal"]] == [row["exit_code"] for row in after["terminal"]]
            for name in before["process_identities"]:
                for field in ("uid", "gid", "cwd", "argv", "test_env", "cgroup", "isatty"):
                    assert before["process_identities"][name][field] == after["process_identities"][name][field], (case, name, field)
            if after["terminal"]:
                terminal = after["terminal"][0]
                assert 0 <= terminal["popen_return_ns"] <= terminal["child_exit_observed_ns"] <= terminal["guest_duration_ns"]
                assert terminal["popen_return_ns"] <= terminal["pipe_eof_ns"] <= terminal["guest_duration_ns"]
                if terminal["first_output_ns"] is not None:
                    assert terminal["popen_return_ns"] <= terminal["first_output_ns"] <= terminal["pipe_eof_ns"]
                if case == "descendant_pipe":
                    assert terminal["child_exit_observed_ns"] < terminal["pipe_eof_ns"]
                if case == "delayed":
                    assert terminal["first_output_ns"] >= 70_000_000
                if case == "no_output":
                    assert terminal["first_output_ns"] is None
        evidence.update(status="PASS", cases=len(cases), runs=len(evidence["samples"]),
                        probe_owned_processes_absent=True)
    except BaseException as error:
        evidence.update(status="FAIL", error_class=type(error).__name__, error=str(error))
        raise
    finally:
        write_json(root / "evidence.json", evidence)
    print(json.dumps({"status": evidence["status"], "cases": evidence["cases"],
                      "runs": evidence["runs"], "evidence": str(root / "evidence.json"),
                      "sha256": sha((root / "evidence.json").read_bytes())}))


if __name__ == "__main__":
    main()

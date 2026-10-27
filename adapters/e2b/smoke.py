"""Opt-in cloud connectivity smoke, not a benchmark attempt or A/B acceptance."""
import argparse
import base64
import json
import os
import sys
import time
import uuid

from server import Adapter, SDK_VERSION


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--confirm-create", action="store_true", help="acknowledge one billed cloud sandbox")
    parser.add_argument("--state-file", required=True)
    args = parser.parse_args()
    if not args.confirm_create:
        parser.error("cloud creation requires --confirm-create")
    if not os.environ.get("E2B_API_KEY") or not os.environ.get("E2B_TEMPLATE_ID"):
        parser.error("E2B_API_KEY and E2B_TEMPLATE_ID must be set, never pass their values as CLI arguments")
    from importlib.metadata import version
    if version("e2b") != SDK_VERSION:
        parser.error("install the pinned SDK")
    from e2b import Sandbox
    from e2b.exceptions import SandboxNotFoundException, FileNotFoundException, NotFoundException
    adapter = Adapter(Sandbox, SandboxNotFoundException, FileNotFoundException, NotFoundException,
        templates=[os.environ["E2B_TEMPLATE_ID"]], max_sandboxes=1,
        max_timeout=120, state_file=args.state_file)
    handle = uuid.uuid4().hex
    evidence = {"kind": "non_benchmark_cloud_smoke", "sdk_version": SDK_VERSION,
                "started_at": time.time(), "checks": {}, "cleanup_confirmed": False}
    try:
        evidence["sandbox"] = adapter.dispatch("create", {"sandbox": handle,
            "template": os.environ["E2B_TEMPLATE_ID"], "timeout_seconds": 120})
        path = "/tmp/aries-sdk-smoke-" + handle
        payload = bytes(range(256))
        adapter.dispatch("upload", {"sandbox": handle, "path": path,
            "data_base64": base64.b64encode(payload).decode()})
        downloaded = adapter.dispatch("download", {"sandbox": handle, "path": path})
        assert base64.b64decode(downloaded["data_base64"]) == payload
        evidence["checks"]["binary_file_roundtrip"] = "PASS"
        process = uuid.uuid4().hex
        adapter.dispatch("start", {"sandbox": handle, "process": process,
            "argv": ["python3", "-c", "import sys; d=sys.stdin.buffer.read();sys.stdout.buffer.write(d);sys.stderr.buffer.write(b'err');sys.exit(7)"], "cwd": "/tmp", "timeout_seconds": 20})
        adapter.dispatch("stdin", {"sandbox": handle, "process": process,
            "data_base64": base64.b64encode(payload).decode(), "close": True})
        cursor, output, errors, exit_code = 0, bytearray(), bytearray(), None
        while True:
            response = adapter.dispatch("events", {"sandbox": handle, "process": process, "cursor": cursor})
            cursor = response["cursor"]
            for event in response["events"]:
                assert "error_code" not in event, event.get("error_code")
                if "exit_code" in event:
                    exit_code = event["exit_code"]
                else:
                    (output if event["stream"] == "stdout" else errors).extend(base64.b64decode(event["data_base64"]))
            if response["done"]:
                break
        assert output == payload and errors == b"err" and exit_code == 7
        evidence["checks"]["binary_exec_stdin_exit"] = "PASS"
        probe = uuid.uuid4().hex
        probe_code = """import importlib.metadata,json,os,pathlib,pwd,shutil,stat,subprocess
p=pathlib.Path('/sys/fs/cgroup')
status=pathlib.Path('/proc/self/status').read_text()
def path_info(path):
    result={'path':str(path)}
    try:
        info=path.lstat()
        kind='symlink' if stat.S_ISLNK(info.st_mode) else 'directory' if stat.S_ISDIR(info.st_mode) else 'file' if stat.S_ISREG(info.st_mode) else 'other'
        result.update(uid=info.st_uid,gid=info.st_gid,mode=format(stat.S_IMODE(info.st_mode),'04o'),type=kind)
    except OSError as error:result['error_class']=type(error).__name__
    return result
def path_chain(value):
    path=pathlib.Path(value)
    result={'requested_path':value,'raw_chain':[path_info(item) for item in [path,*path.parents]]}
    try:
        resolved=path.resolve(strict=True)
        result.update(resolved_path=str(resolved),resolved_chain=[path_info(item) for item in [resolved,*resolved.parents]])
    except (OSError,RuntimeError) as error:result['resolve_error_class']=type(error).__name__
    return result
def version(command):
    executable=shutil.which(command)
    if not executable:return {'available':False}
    result=subprocess.run([executable,'--version'],capture_output=True,text=True,timeout=10)
    return {'available':True,'exit_code':result.returncode,'version':result.stdout.strip()[:128]}
try:
    agent=pwd.getpwuid(60000); agent_account={'available':True,'name':agent.pw_name,'uid':agent.pw_uid,'gid':agent.pw_gid}
except KeyError:agent_account={'available':False}
try:pytest_version=importlib.metadata.version('pytest')
except importlib.metadata.PackageNotFoundError:pytest_version=None
print(json.dumps({'uid':os.getuid(),'hostname':os.uname().nodename,
 'paths':[path_chain(path) for path in ('/','/run','/app','/app/openclaw.mjs','/usr/local/bin/openclaw',
 '/usr/local/bin/node','/usr/local/share/aries-template/check_runtime.py','/sys/fs/cgroup')],
 'cgroup_v2':(p/'cgroup.controllers').exists(),
 'cgroup_root_writable':os.access(p,os.W_OK),'cgroup_kill_present':(p/'cgroup.kill').exists(),
 'process_status':[line for line in status.splitlines() if line.startswith(('NoNewPrivs:','CapEff:'))],
 'python':version('python3'),'pytest_version':pytest_version,'agent_account':agent_account,
 'node':version('node'),'openclaw':version('openclaw')}))"""
        adapter.dispatch("start", {"sandbox": handle, "process": probe,
            "argv": ["python3", "-c", probe_code], "cwd": "/tmp", "user": "root", "timeout_seconds": 30})
        adapter.dispatch("stdin", {"sandbox": handle, "process": probe, "close": True})
        cursor, probe_output = 0, bytearray()
        while True:
            response = adapter.dispatch("events", {"sandbox": handle, "process": probe, "cursor": cursor})
            cursor = response["cursor"]
            for event in response["events"]:
                assert "error_code" not in event
                if event.get("stream") == "stdout":
                    probe_output.extend(base64.b64decode(event["data_base64"]))
                if "exit_code" in event:
                    assert event["exit_code"] == 0
            if response["done"]:
                break
        evidence["read_only_capability_probe"] = json.loads(probe_output)
        # Existence/access checks do not prove writable, isolated cgroup delegation.
        evidence["read_only_capability_probe"]["isolation_acceptance"] = "UNRUN"
        background = uuid.uuid4().hex
        adapter.dispatch("start", {"sandbox": handle, "process": background,
            "argv": ["python3", "-c", "import time; time.sleep(60)"], "cwd": "/tmp", "timeout_seconds": 90})
        status = adapter.dispatch("process", {"sandbox": handle, "process": background})
        assert status["managed_process_present"]
        evidence["checks"]["background_status"] = "PASS"
        evidence["process_kill"] = adapter.dispatch("kill_process", {"sandbox": handle, "process": background})
        evidence["metrics"] = adapter.dispatch("metrics", {"sandbox": handle})
        guest = adapter.dispatch("guest", {"sandbox": handle, "port": 18789})
        evidence["checks"]["private_guest_endpoint_resolved"] = bool(guest["url"].startswith("https://") and guest["headers"].get("e2b-traffic-access-token"))
        # No listener was started on this port; this does not claim Gateway connectivity.
        evidence["checks"]["gateway_connectivity"] = "UNRUN"
        evidence["result"] = "PASS"
    except Exception:
        evidence["result"] = "FAIL"
        # SDK errors and command output may contain confidential data.
    finally:
        failures = adapter.close()
        evidence["cleanup_confirmed"] = not failures
        evidence["finished_at"] = time.time()
        print(json.dumps(evidence, default=lambda value: value.isoformat()))
    return 0 if evidence.get("result") == "PASS" and evidence["cleanup_confirmed"] else 1


if __name__ == "__main__":
    sys.exit(main())

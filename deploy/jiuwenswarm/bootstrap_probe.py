"""Run the exact Harness bootstrap as UID 60000 in an isolated local container."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import signal
import subprocess
import time
import urllib.request


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--bootstrap', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    assert os.geteuid() == 0
    args.output.mkdir(parents=True, exist_ok=True)
    runtime = Path('/run/aries-native-bootstrap-probe')
    source = Path('/opt/jiuwenswarm')
    native_python = str(source / '.venv/bin/python')
    record = {'status': 'FAIL_NATIVE_BOOTSTRAP_SMOKE', 'started_at': datetime.now(timezone.utc).isoformat(),
        'bootstrap_sha256': hashlib.sha256(args.bootstrap.read_bytes()).hexdigest(),
        'scope': 'exact stage/run bootstrap, private initialization and AgentServer health; no task/model request',
        'task_requests_submitted': 0, 'network': 'docker --network=none', 'uid': 60000}
    child = None
    log = None
    def unprivileged():
        os.setgroups([])
        os.setgid(60000)
        os.setuid(60000)
    try:
        spec = {'source_dir': str(source), 'python_path': native_python,
            'bootstrap': args.bootstrap.read_text(), 'workdir': '/workspace/aries-case',
            'model_proxy_url': 'http://127.0.0.1:9/v1', 'proxy_token': 'public-unused-probe-token',
            'model': 'no-model-request', 'native_observation': False, 'native_audit': ''}
        subprocess.run(['python3', '-I', str(args.bootstrap), 'stage', str(runtime)],
            input=json.dumps(spec), text=True, check=True, timeout=15)
        log = (args.output / 'agentserver.log').open('w')
        child = subprocess.Popen([native_python, '-I', str(runtime / 'launch.py'), 'run', str(runtime)],
            stdout=log, stderr=subprocess.STDOUT, preexec_fn=unprivileged, start_new_session=True)
        deadline = time.monotonic() + 90
        while True:
            if child.poll() is not None:
                raise RuntimeError('native_bootstrap_exited_before_health')
            try:
                with urllib.request.urlopen('http://127.0.0.1:18866/api/v1/health', timeout=1) as response:
                    health = json.load(response)
                if health.get('ok') and health.get('data', {}).get('status') == 'ready':
                    break
            except OSError:
                pass
            assert time.monotonic() < deadline, 'native_bootstrap_health_timeout'
            time.sleep(.25)
        proof = json.loads((runtime / 'home/prepared.json').read_text())
        assert proof['uid'] == 60000 and proof['mode'] == 'flash'
        assert proof['skill_selection'] is False and proof['sandbox_enabled'] is False
        assert proof['sandbox_startup_mode'] == 'external'
        # These assertions run as the same UID, after all production bootstrap
        # writes completed. Same-content rewrites leave the running config intact.
        code = r'''
import json,os,pathlib,stat
home=pathlib.Path('/run/aries-native-bootstrap-probe/home');data=home/'data'
configs=[data/'config/config.yaml',data/'config/.env']
for path in configs:
    content=path.read_bytes();path.write_bytes(content)
    assert path.stat().st_uid==60000 and path.stat().st_mode&0o200
agents=list(data.rglob('AGENT.md'));assert len(agents)==1
workspace=agents[0].parent
for name in ('memory','projects','skills'):
    folder=workspace/name;assert folder.is_dir()
    marker=folder/'aries-permission-probe';marker.write_text('public probe');marker.unlink()
for path in workspace.rglob('*'):
    assert path.stat().st_uid==60000
    assert path.stat().st_mode&0o200
source=pathlib.Path('/opt/jiuwenswarm/jiuwenswarm/resources')
for path in (source/'agent/workspace',source/'agent/workspace/AGENT_EN.md',source/'.env.template'):
    s=path.stat();assert s.st_uid==0 and not s.st_mode&0o022
    assert s.st_mode&0o200 and not os.access(path,os.W_OK)
print(json.dumps({'private_config_rewrite':True,'canonical_env_rewrite':True,'agent_file_created':True,
 'memory_projects_skills_writable':True,'source_not_agent_writable':True,
 'source_workspace_mode':oct(stat.S_IMODE((source/'agent/workspace').stat().st_mode)),
 'private_workspace_mode':oct(stat.S_IMODE(workspace.stat().st_mode)),
 'source_env_template_mode':oct(stat.S_IMODE((source/'.env.template').stat().st_mode)),
 'private_env_mode':oct(stat.S_IMODE(configs[1].stat().st_mode))}))
'''
        checks = json.loads(subprocess.check_output([native_python, '-I', '-c', code],
            text=True, preexec_fn=unprivileged, timeout=15))
        record.update(status='PASS_NATIVE_BOOTSTRAP_SMOKE_NO_TASK', health_status='ready',
            native_mode=proof['mode'], sandbox_startup_mode=proof['sandbox_startup_mode'], checks=checks)
    except BaseException as error:
        record.update(error_class=type(error).__name__, reason=str(error))
    finally:
        if child is not None:
            try:
                os.killpg(child.pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
            try:
                child.wait(timeout=10)
            except subprocess.TimeoutExpired:
                os.killpg(child.pid, signal.SIGKILL)
                child.wait(timeout=5)
            record['native_process_reaped'] = child.poll() is not None
        if log is not None:
            log.close()
        record['finished_at'] = datetime.now(timezone.utc).isoformat()
        (args.output / 'evidence.json').write_text(json.dumps(record,sort_keys=True,indent=2)+'\n')
        print(json.dumps(record))
    return 0 if record['status']=='PASS_NATIVE_BOOTSTRAP_SMOKE_NO_TASK' else 1


if __name__ == '__main__':
    raise SystemExit(main())

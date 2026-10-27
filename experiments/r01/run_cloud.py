"""One disposable E2B sandbox, isolated adapter, native identity PoC on ECS."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import secrets
import signal
import socket
import subprocess
import sys
import time
import urllib.request

SDK_IMAGE = 'sha256:69ffe66403a83d5956a887e838138b1b9fd50f1e3040313d076aefba0a257a11'
PYTHON_SOURCE = Path('/opt/aries-artifacts/observability-r01-python-binding-1/source/adapters/e2b')
GO_RELEASE = Path('/opt/aries-artifacts/observability-r01-fixture-cloud-1')
NATIVE_RELEASE = Path('/opt/aries-artifacts/observability-r01-native-poc-3')
NATIVE_INPUTS = Path('/opt/aries-artifacts/observability-r01-native-inputs-3')


def main():
    if not sys.platform.startswith('linux'):
        raise SystemExit('authorized ECS only')
    parser = argparse.ArgumentParser()
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    root = args.output.resolve()
    root.mkdir(mode=0o700, exist_ok=False)
    case = root / 'candidate'
    case.mkdir(mode=0o700)
    state = root / 'state'
    state.mkdir(mode=0o700)
    os.chown(state, 10001, 10001)
    adapter = root.name + '-adapter'
    native = root.name + '-native'
    envfile, configfile = root / 'adapter.env', root / 'cloud.json'
    process = None
    bridge_log = None
    record = {'schema_version': 1, 'status': 'FAIL', 'planned_cloud_creates': 1,
              'max_sandboxes': 1, 'ttl_seconds': 300, 'model_calls': 0,
              'formal_image_changed': False, 'steps': [], 'cleanup': {}}

    def run(argv, name, timeout=30, check=True):
        with (root / (name + '.log')).open('wb') as out:
            result = subprocess.run(argv, stdout=out, stderr=subprocess.STDOUT, timeout=timeout)
        record['steps'].append({'name': name, 'exit_code': result.returncode})
        if check and result.returncode:
            raise RuntimeError(name + ' failed')
        return result

    try:
        # Existing service is a credential source only; never mutate it or its ledger.
        service = json.loads(subprocess.check_output(['docker', 'inspect', 'aries-e2b-e2b-adapter-1']))[0]
        existing = dict(item.split('=', 1) for item in service['Config']['Env'])
        key, template = existing['E2B_API_KEY'], existing['E2B_TEMPLATE_ID']
        assert key and template and all(c not in key + template for c in '\r\n\0')
        token = secrets.token_urlsafe(48)
        with socket.socket() as available:
            available.bind(('127.0.0.1', 0))
            port = available.getsockname()[1]
        endpoint = 'http://127.0.0.1:' + str(port)
        fd = os.open(envfile, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, 'w') as out:
            out.write('E2B_API_KEY=' + key + '\nE2B_TEMPLATE_ID=' + template + '\nARIES_E2B_ADAPTER_TOKEN=' + token + '\n')
        fd = os.open(configfile, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, 'w') as out:
            json.dump({'endpoint': endpoint, 'token': token, 'template': template, 'timeout_seconds': 300}, out)
        record['template_sha256'] = hashlib.sha256(template.encode()).hexdigest()
        record['sdk_image_id'] = SDK_IMAGE
        record['native_image_id'] = json.loads((NATIVE_RELEASE / 'evidence.json').read_text())['candidate_image_id']
        record['sources'] = {str(p): hashlib.sha256(p.read_bytes()).hexdigest() for p in
            [PYTHON_SOURCE / 'server.py', PYTHON_SOURCE / 'guest_exec.py', GO_RELEASE / 'bridge-cloud',
             GO_RELEASE / 'aries-ssh-client', NATIVE_INPUTS / 'native_probe.mjs', Path(__file__)]}
        run(['docker', 'run', '-d', '--name', adapter, '--network=host', '--read-only',
             '--cap-drop=ALL', '--pids-limit=128', '--memory=512m', '--cpus=1',
             '--tmpfs=/tmp:rw,nosuid,nodev,size=32m', '--env-file', str(envfile),
             '-v', str(state) + ':/state', '-v', str(PYTHON_SOURCE) + ':/app:ro',
             '--entrypoint=python', SDK_IMAGE, '/app/server.py', '--listen', '127.0.0.1:' + str(port),
             '--state-file', '/state/adapter.state.json', '--max-sandboxes', '1',
             '--max-timeout', '300'], 'adapter-start')
        envfile.unlink()
        deadline = time.monotonic() + 20
        while True:
            request = urllib.request.Request(endpoint + '/v1/health', data=b'{}',
                headers={'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json'})
            try:
                with urllib.request.urlopen(request, timeout=2) as response:
                    health = json.load(response)
                assert health['provider'] == 'e2b-cloud'
                record['health'] = health
                break
            except OSError:
                if time.monotonic() >= deadline:
                    raise RuntimeError('isolated adapter readiness failed')
                time.sleep(0.1)
        bridge_log = (case / 'bridge.log').open('wb')
        process = subprocess.Popen([str(GO_RELEASE / 'bridge-cloud'), '--cloud-config', str(configfile),
            '--output', str(case), '--scope', 'r01-poc-candidate', '--session', 'r01-session'],
            stdout=bridge_log, stderr=subprocess.STDOUT)
        deadline = time.monotonic() + 90
        while not (case / 'ready.json').exists():
            if process.poll() is not None or time.monotonic() > deadline:
                raise RuntimeError('cloud Bridge readiness failed')
            time.sleep(0.1)
        configfile.unlink()
        (case / 'ready.json').chmod(0o444)
        (case / 'scope.json').write_text(json.dumps({'scope_id': 'r01-poc-candidate', 'agent_session_id': 'r01-session'}))
        (case / 'scope.json').chmod(0o444)
        run(['docker', 'run', '--name', native, '--network=host', '--read-only', '--cap-drop=ALL',
             '--pids-limit=128', '--memory=1g', '--cpus=1', '--tmpfs=/tmp:rw,nosuid,nodev,size=64m',
             '--user=1000:1000', '-e', 'HOME=/tmp', '-e', 'OPENCLAW_STATE_DIR=/tmp/openclaw-state',
             '-v', str(case / 'ready.json') + ':/run/aries/probe.json:ro',
             '-v', str(case / 'scope.json') + ':/run/aries/tool-identity-scope.json:ro',
             '-v', str(GO_RELEASE / 'aries-ssh-client') + ':/opt/aries/bin/aries-ssh-client:ro',
             '-v', str(NATIVE_INPUTS / 'native_probe.mjs') + ':/run/aries/native_probe.mjs:ro',
             '--entrypoint=node', record['native_image_id'], '/run/aries/native_probe.mjs'], 'candidate-native', timeout=100)
        record['status'] = 'EXECUTED_PENDING_RECONCILIATION'
    except Exception as exc:
        record['error_type'] = type(exc).__name__
        # No arbitrary SDK/HTTP exception text is published.
    finally:
        run(['docker', 'rm', '-f', native], 'native-remove', check=False)
        if process is not None:
            if process.poll() is None:
                process.send_signal(signal.SIGTERM)
            try:
                process.wait(timeout=80)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=5)
            record['cleanup']['bridge_exit'] = process.returncode
        if bridge_log:
            bridge_log.close()
        run(['docker', 'logs', adapter], 'adapter-output', check=False)
        run(['docker', 'stop', '--time=90', adapter], 'adapter-stop', timeout=100, check=False)
        ledger = state / 'adapter.state.json'
        if ledger.exists():
            saved = json.loads(ledger.read_text())
            record['actual_cloud_creates'] = saved['creates']
            record['cleanup']['ledger_states'] = [r['state'] for r in saved['records'].values()]
            record['cleanup']['cloud_absence_confirmed'] = all(r['state'] == 'absent' for r in saved['records'].values())
        else:
            record['cleanup']['cloud_absence_confirmed'] = process is None
        run(['docker', 'rm', adapter], 'adapter-remove', check=False)
        names = subprocess.run(['docker', 'container', 'ls', '-a', '--format={{.Names}}'], capture_output=True, text=True, timeout=15)
        record['cleanup']['owned_containers_absent'] = names.returncode == 0 and not ({adapter, native} & set(names.stdout.splitlines()))
        for secret in (envfile, configfile):
            secret.unlink(missing_ok=True)
        record['cleanup']['temporary_credentials_removed'] = not envfile.exists() and not configfile.exists()
        if not record['cleanup'].get('cloud_absence_confirmed') or not record['cleanup']['owned_containers_absent']:
            record['status'] = 'FAIL_CLEANUP_UNCONFIRMED'
        record['files'] = {str(p.relative_to(root)): hashlib.sha256(p.read_bytes()).hexdigest()
                           for p in root.rglob('*') if p.is_file() and p.name != 'evidence.json'}
        (root / 'evidence.json').write_text(json.dumps(record, indent=2, sort_keys=True) + '\n')
        print(json.dumps({'status': record['status'], 'actual_cloud_creates': record.get('actual_cloud_creates', 0),
                          'cleanup': record['cleanup'], 'root': str(root)}))
    if record['status'] != 'EXECUTED_PENDING_RECONCILIATION':
        raise SystemExit(1)


if __name__ == '__main__':
    main()

#!/usr/bin/env python3
"""Build one AgentSphere Jiuwen template and delete its short capability Sandbox."""
import argparse
from datetime import datetime, timezone
import hashlib
from importlib.metadata import version
import json
import os
from pathlib import Path
import re
import shlex
import signal
import sys
import time
import urllib.parse
import uuid


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--bundle', type=Path, required=True)
    parser.add_argument('--template-reference', help='Probe an existing ready template without rebuilding it')
    args = parser.parse_args()
    assert os.geteuid() == 0 and version('e2b') == '2.42.0' and version('agentsphere-e2b-patch') == '0.1.0'
    import agentsphere_e2b_patch  # noqa: F401; existing pinned transport patch
    import httpx
    from e2b import Sandbox, CommandExitException
    sys.path.insert(0, '/opt/aries-artifacts/workload-replay-release-14/source/adapters/agentsphere')
    from agentsphere import Connection
    from huawei_template import create_body, build_body
    os.umask(0o077)
    root = args.output
    root.mkdir(mode=0o700, parents=True, exist_ok=False)
    (root / 'api').mkdir()
    pins = json.loads((args.bundle / 'probe-pins.json').read_text())
    image = pins['image_reference']
    assert re.fullmatch(r'swr\.cn-southwest-2\.myhuaweicloud\.com/benchmark-zhoutao/aries-jiuwenswarm:[A-Za-z0-9._-]+@sha256:[0-9a-f]{64}', image)
    if pins.get('publication_receipt_sha256'):
        publication_file = args.bundle / 'publication.json'
        assert hashlib.sha256(publication_file.read_bytes()).hexdigest() == pins['publication_receipt_sha256']
        publication = json.loads(publication_file.read_text())
        assert publication['status'] == 'PASS_PUBLISHED_IMAGE_ONLY'
        assert publication['reference'] == image and publication['image_id'] == pins['image_id']
    record = {'status': 'RUNNING', 'provider': 'huawei-agentsphere', 'started_at': datetime.now(timezone.utc).isoformat(),
        'image_reference': image, 'resources': {'cpu_count': 2, 'memory_mb': 4096},
        'template_create_requests': 0, 'template_build_requests': 0, 'sandbox_create_requests': 0,
        'model_calls': 0, 'cloud_harness_status': 'UNRUN', 'cleanup_errors': [], 'cleanup_confirmed': False,
        'api_calls': [], 'recipe_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
        'probe_files_sha256': {p.name: hashlib.sha256(p.read_bytes()).hexdigest() for p in args.bundle.iterdir() if p.is_file()}}
    private = {}
    sensitive = []
    connection = None
    sandbox_id = None
    handle = uuid.uuid4().hex
    def redact(value):
        if isinstance(value, dict):
            return {k: '[REDACTED]' if re.search(r'token|password|secret|credential|authorization|cookie|api.?key|access.?key', k, re.I) else redact(v) for k, v in value.items()}
        if isinstance(value, list):
            return [redact(v) for v in value]
        if isinstance(value, str):
            for secret in sensitive:
                value = value.replace(secret, '[REDACTED]')
        return value
    def save():
        (root / 'evidence.json').write_text(json.dumps(redact(record), sort_keys=True, indent=2) + '\n')
    def api(method, path, payload=None, allowed=(200,)):
        nonlocal sandbox_id
        call = {'method': method, 'path': path, 'started_at': datetime.now(timezone.utc).isoformat()}
        record['api_calls'].append(call)
        save()
        started = time.monotonic_ns()
        try:
            status, value = connection.request(method, path, payload=payload, timeout=30)
            call['http_status'] = status
            if method == 'POST' and path == '/sandboxes' and isinstance(value, dict):
                candidate = value.get('sandboxID')
                if isinstance(candidate, str) and re.fullmatch('[A-Za-z0-9_-]+', candidate):
                    sandbox_id = candidate
                    record['sandbox_id'] = candidate
                sensitive.extend(v for k, v in value.items() if 'token' in k.lower() and isinstance(v, str))
            if method == 'POST' and path == '/v3/templates' and isinstance(value, dict):
                record.update(template_id=value.get('templateID'), build_id=value.get('buildID'))
            receipt = root / 'api' / ('%03d.json' % len(record['api_calls']))
            receipt.write_text(json.dumps(redact({'method': method, 'path': path, 'request': payload,
                'http_status': status, 'response': value}), sort_keys=True, indent=2)+'\n')
            call['receipt_sha256'] = hashlib.sha256(receipt.read_bytes()).hexdigest()
            assert status in allowed, 'controlplane_http_' + str(status)
            return status, value
        finally:
            call['duration_ns'] = time.monotonic_ns() - started
            save()
    def interrupted(*_):
        raise InterruptedError('template_probe_interrupted')
    for sig in (signal.SIGINT, signal.SIGTERM, signal.SIGHUP):
        signal.signal(sig, interrupted)
    try:
        private = json.loads(Path('/root/aries-setup/tb-huawei-cpp-1/input.private.json').read_text())
        assert private['api_url'] == 'https://agentsandbox.cn-southwest-2.myhuaweicloud.com'
        sensitive = [v for k, v in private.items() if isinstance(v, str) and v and (k in ('ak', 'sk') or re.search('key|token|password|secret', k, re.I))]
        connection = Connection(Sandbox, httpx.Client(), api_url=private['api_url'],
            api_key=private['api_key'], gateway_url=private['gateway_url'])
        started = time.monotonic_ns()
        if args.template_reference:
            template, build = args.template_reference.split(':')
            assert re.fullmatch('[A-Za-z0-9_-]+', template) and str(uuid.UUID(build)) == build
            record.update(template_id=template, build_id=build, template_reference=args.template_reference,
                phase='verify_existing_template')
        else:
            name = 'aries-jiuwen-' + uuid.uuid4().hex[:12]
            create = create_body(name, private['gateway_id'])
            record.update(phase='template_create', template_create_requests=1, template_name=name)
            save()
            api('POST', '/v3/templates', create, (202,))
            template, build = record['template_id'], record['build_id']
            assert re.fullmatch('[A-Za-z0-9_-]+', template) and str(uuid.UUID(build)) == build
            record['template_reference'] = template + ':' + build
            record.update(phase='building', template_build_requests=1)
            save()
            api('POST', '/v2/templates/' + template + '/builds/' + build, build_body(image), (202,))
        deadline = time.monotonic() + 1800
        while True:
            _, detail = api('GET', '/templates/' + template + '/builds/' + build)
            record['build_state'] = detail.get('status')
            if detail.get('status') == 'ready':
                break
            assert detail.get('status') in ('building', 'pending', 'waiting'), 'template_build_failed'
            assert time.monotonic() < deadline, 'template_build_deadline'
            time.sleep(3)
        assert detail.get('fromImage') == image
        assert detail.get('cpuCount') == 2 and detail.get('memoryMB') == 4096
        _, template_detail = api('GET', '/templates/' + template)
        matched = [item for item in template_detail.get('builds', []) if item.get('buildID') == build]
        assert len(matched) == 1 and matched[0]['cpuCount'] == 2 and matched[0]['memoryMB'] == 4096
        assert template_detail.get('arch') == 'x86_64'
        record.update(template_status='PASS_TEMPLATE_BUILD_ONLY', build_duration_ns=time.monotonic_ns()-started,
            phase='sandbox_create', sandbox_create_requests=1, allocation_handle=handle, sandbox_ttl_seconds=300,
            network_scope='public template; probe requests internet=false, provider egress enforcement remains unverified')
        save()
        _, created = api('POST', '/sandboxes', {'templateID': record['template_reference'], 'timeout': 300,
            'autoPause': False, 'autoResume': {'enabled': False}, 'secure': True,
            'allow_internet_access': False, 'metadata': {'aries_jiuwen_probe': handle}, 'envVars': {}}, (201,))
        assert sandbox_id
        _, actual = api('GET', '/sandboxes/' + urllib.parse.quote(sandbox_id, safe=''))
        assert actual['templateID'] == template and actual['buildID'] == build
        assert actual['timeout'] == 300 and actual['lifecycle']['onTimeout'] == 'kill' and actual['lifecycle']['autoResume'] is False
        sandbox = connection.connect(created, 30)
        record['phase'] = 'guest_probe'
        save()
        guest = '/run/aries-jiuwen-template-probe'
        record['sdk_stage'] = 'prepare_guest_directory'
        save()
        sandbox.commands.run('install -d -m 0700 ' + guest, user='root', cwd='/', timeout=20)
        for p in args.bundle.iterdir():
            if p.is_file():
                record['sdk_stage'] = 'upload_' + p.name
                save()
                sandbox.files.write(guest + '/' + p.name, p.read_bytes(), user='root', request_timeout=30)
        record['sdk_stage'] = 'run_guest_probe'
        save()
        try:
            result = sandbox.commands.run('python3 -I ' + shlex.quote(guest + '/guest_probe.py') + ' ' + shlex.quote(guest), user='root', cwd='/', timeout=120, request_timeout=150)
        except CommandExitException as error:
            result = error
        (root / 'guest-result.json').write_text(json.dumps({'exit_code': result.exit_code, 'stdout': result.stdout, 'stderr': result.stderr}, indent=2)+'\n')
        assert result.exit_code == 0, 'guest_probe_failed'
        probe = json.loads(result.stdout)
        assert probe['status'] == 'PASS_GUEST_CAPABILITIES_ONLY'
        record.update(guest_probe=probe, status='PASS_TEMPLATE_AND_GUEST_CAPABILITIES_ONLY')
    except BaseException as error:
        record.update(status='FAIL_TEMPLATE_OR_GUEST_PROBE', error_class=type(error).__name__)
        record['reason'] = redact(str(error))[:2048]
        context = error.__cause__ or error.__context__
        if context is not None:
            record['error_context'] = {'class': type(context).__name__, 'message': redact(str(context))[:2048]}
    finally:
        for sig in (signal.SIGINT, signal.SIGTERM, signal.SIGHUP):
            signal.signal(sig, signal.SIG_IGN)
        ids = [sandbox_id] if sandbox_id else []
        if not ids and record['sandbox_create_requests'] and private:
            try:
                pages = Sandbox.list(query={'metadata': {'aries_jiuwen_probe': handle}}, limit=100,
                    api_key=private['api_key'], api_url=private['api_url'], validate_api_key=False, debug=False, request_timeout=30)
                while pages.has_next:
                    ids.extend(item.sandbox_id for item in pages.next_items())
            except BaseException as error:
                record['cleanup_errors'].append({'stage': 'create_reconciliation', 'error_class': type(error).__name__})
        for sid in ids:
            try:
                path = '/sandboxes/' + urllib.parse.quote(sid, safe='')
                api('DELETE', path, allowed=(200, 202, 204, 404))
                deadline = time.monotonic() + 60
                while True:
                    status, _ = api('GET', path, allowed=(200, 404))
                    if status == 404:
                        break
                    assert time.monotonic() < deadline, 'sandbox_still_present'
                    time.sleep(2)
            except BaseException as error:
                record['cleanup_errors'].append({'stage': 'owned_sandbox_delete', 'sandbox_id': sid, 'error_class': type(error).__name__})
        record['cleanup_confirmed'] = not record['cleanup_errors']
        if record['cleanup_errors']:
            record['status'] = 'FAIL_CLEANUP'
        record.update(finished_at=datetime.now(timezone.utc).isoformat(), template_retained=bool(record.get('template_id')))
        save()
        print(json.dumps({k: record.get(k) for k in ('status', 'template_reference', 'cleanup_confirmed', 'reason')}))
    return 0 if record['status'] == 'PASS_TEMPLATE_AND_GUEST_CAPABILITIES_ONLY' else 1


if __name__ == '__main__':
    raise SystemExit(main())

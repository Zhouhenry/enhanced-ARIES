#!/usr/bin/env python3
"""Build one E2B Jiuwen template and delete its one short capability Sandbox."""
import argparse
from datetime import datetime, timezone
import hashlib
import hmac
from importlib.metadata import version
import json
import os
import re
from pathlib import Path
import shlex
import signal
import subprocess
import time
import uuid


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--bundle', type=Path, required=True)
    args = parser.parse_args()
    assert os.geteuid() == 0 and version('e2b') == '2.58.0'
    from e2b import Template, Sandbox, SandboxNotFoundException, CommandExitException
    os.umask(0o077)
    root = args.output
    root.mkdir(mode=0o700, parents=True, exist_ok=False)
    pins = json.loads((args.bundle / 'probe-pins.json').read_text())
    image = pins['image_reference']
    assert re.fullmatch(r'swr\.cn-southwest-2\.myhuaweicloud\.com/benchmark-zhoutao/aries-jiuwenswarm:[A-Za-z0-9._-]+@sha256:[0-9a-f]{64}', image)
    if pins.get('publication_receipt_sha256'):
        publication_file = args.bundle / 'publication.json'
        assert hashlib.sha256(publication_file.read_bytes()).hexdigest() == pins['publication_receipt_sha256']
        publication = json.loads(publication_file.read_text())
        assert publication['status'] == 'PASS_PUBLISHED_IMAGE_ONLY'
        assert publication['reference'] == image and publication['image_id'] == pins['image_id']
    record = {'status': 'RUNNING', 'provider': 'e2b', 'started_at': datetime.now(timezone.utc).isoformat(),
        'image_reference': image, 'resources': {'cpu_count': 2, 'memory_mb': 4096},
        'template_build_requests': 0, 'sandbox_create_requests': 0, 'model_calls': 0,
        'cloud_harness_status': 'UNRUN', 'cleanup_errors': [], 'cleanup_confirmed': False,
        'recipe_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
        'probe_files_sha256': {p.name: hashlib.sha256(p.read_bytes()).hexdigest() for p in args.bundle.iterdir() if p.is_file()}}
    def save():
        (root / 'evidence.json').write_text(json.dumps(record, sort_keys=True, indent=2) + '\n')
    def interrupted(*_):
        raise InterruptedError('template_probe_interrupted')
    for sig in (signal.SIGINT, signal.SIGTERM, signal.SIGHUP):
        signal.signal(sig, interrupted)
    sandbox = None
    handle = uuid.uuid4().hex
    key = None
    try:
        inspected = json.loads(subprocess.check_output(['docker', 'inspect', 'aries-e2b-e2b-adapter-1']))[0]
        env = dict(v.split('=', 1) for v in inspected['Config']['Env'] if '=' in v)
        key = env['E2B_API_KEY']
        assert key
        del inspected, env
        settings = json.loads(Path('/root/aries-setup/tb-huawei-cpp-1/input.private.json').read_text())
        username = 'cn-southwest-2@' + settings['ak']
        password = hmac.new(settings['sk'].encode(), settings['ak'].encode(), hashlib.sha256).hexdigest()
        private_values = [key, settings['ak'], settings['sk'], username, password]
        del settings
        template = (Template().from_image(image, username=username, password=password)
            .set_user('root').set_workdir('/workspace/aries-case')
            .set_start_cmd('exec /bin/sh /opt/aries-template/start.sh', 'test -f /opt/aries-template/ready.json'))
        name = 'aries-jiuwen-' + uuid.uuid4().hex[:12]
        record.update(phase='build_requested', template_name=name, template_build_requests=1)
        save()
        started = time.monotonic_ns()
        info = Template.build_in_background(template, name=name, tags=['jiuwen-' + image.split('@', 1)[0].rsplit(':', 1)[1]],
            cpu_count=2, memory_mb=4096, api_key=key, debug=False, request_timeout=60)
        record.update(template_id=info.template_id, build_id=info.build_id,
            template_reference=info.template_id + ':' + str(uuid.UUID(info.build_id)), phase='building')
        save()
        deadline = time.monotonic() + 1800
        while True:
            status = Template.get_build_status(info, api_key=key, debug=False, request_timeout=30)
            record['build_state'] = status.status.value
            save()
            if status.status.value == 'ready':
                break
            if status.status.value not in ('building', 'waiting'):
                reason = str(getattr(getattr(status, 'reason', None), 'message', ''))
                for value in private_values:
                    reason = reason.replace(value, '[REDACTED]')
                record['build_failure_reason'] = reason[:1500]
                raise RuntimeError('template_build_failed')
            assert time.monotonic() < deadline, 'template_build_deadline'
            time.sleep(3)
        record.update(template_status='PASS_TEMPLATE_BUILD_ONLY', build_duration_ns=time.monotonic_ns()-started,
            phase='sandbox_create', sandbox_create_requests=1, allocation_handle=handle, sandbox_ttl_seconds=300)
        save()
        sandbox = Sandbox.create(template=record['template_reference'], timeout=300,
            metadata={'aries_jiuwen_probe': handle}, allow_internet_access=False,
            network={'allow_public_traffic': False}, lifecycle={'on_timeout': 'kill', 'auto_resume': False},
            api_key=key, debug=False, request_timeout=60)
        record.update(sandbox_id=sandbox.sandbox_id, phase='guest_probe')
        save()
        actual = sandbox.get_info(request_timeout=30)
        assert actual.template_id == info.template_id and actual.cpu_count == 2 and actual.memory_mb == 4096
        record['sandbox_readback'] = {'template_id': actual.template_id, 'cpu_count': actual.cpu_count,
            'memory_mb': actual.memory_mb, 'envd_version': actual.envd_version,
            'allow_internet_access': actual.allow_internet_access}
        guest = '/run/aries-jiuwen-template-probe'
        sandbox.commands.run('install -d -m 0700 ' + guest, user='root', cwd='/', timeout=20)
        for p in args.bundle.iterdir():
            if p.is_file():
                sandbox.files.write(guest + '/' + p.name, p.read_bytes(), user='root', request_timeout=30)
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
        if isinstance(error, (RuntimeError, AssertionError)):
            record['reason'] = str(error)
    finally:
        for sig in (signal.SIGINT, signal.SIGTERM, signal.SIGHUP):
            signal.signal(sig, signal.SIG_IGN)
        ids = [sandbox.sandbox_id] if sandbox else []
        if not ids and record['sandbox_create_requests'] and key:
            try:
                pages = Sandbox.list(query={'metadata': {'aries_jiuwen_probe': handle}}, limit=100, api_key=key, debug=False, request_timeout=30)
                while pages.has_next:
                    ids.extend(item.sandbox_id for item in pages.next_items())
            except BaseException as error:
                record['cleanup_errors'].append({'stage': 'create_reconciliation', 'error_class': type(error).__name__})
        for sid in ids:
            try:
                Sandbox.kill(sid, api_key=key, debug=False, request_timeout=30)
                try:
                    Sandbox.get_info(sid, api_key=key, debug=False, request_timeout=30)
                except SandboxNotFoundException:
                    continue
                raise RuntimeError('sandbox_still_present')
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

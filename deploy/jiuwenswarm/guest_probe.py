"""Short guest capability probe using the unchanged shared Sandbox ControlScript."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import uuid


def digest(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def verify_native_source(bundle, pins):
    assert digest(bundle / 'source-manifest.json') == pins['source_manifest_sha256']
    assert digest('/opt/jiuwenswarm/aries-build.json') == pins['aries_build_sha256']
    files = json.loads((bundle / 'source-manifest.json').read_text())
    overrides = {}
    patch_manifest_sha = pins.get('native_patch_manifest_sha256')
    if patch_manifest_sha:
        manifest = Path('/opt/jiuwenswarm/aries-patches.json')
        assert digest(manifest) == patch_manifest_sha, 'native_patch_manifest_changed'
        overlay = json.loads(manifest.read_text())
        assert overlay['base_build_sha256'] == pins['aries_build_sha256']
        assert overlay['native_modified'] is True
        for patch in overlay['patches']:
            assert digest(patch['patch_file']) == patch['patch_sha256'], 'native_patch_file_changed'
            for path, item in patch['files'].items():
                target = Path(path)
                relative = str(target.relative_to('/opt/jiuwenswarm')) if target.is_relative_to('/opt/jiuwenswarm') else None
                if relative in files:
                    assert files[relative] == item['before_sha256'], 'native_patch_original_mismatch'
                    assert relative not in overrides, 'native_patch_duplicate_source'
                    overrides[relative] = item['after_sha256']
                else:
                    assert path in {
                        '/opt/jiuwenswarm/.venv/lib/python3.12/site-packages/openjiuwen/core/foundation/tool/base.py',
                        '/opt/jiuwenswarm/.venv/lib/python3.12/site-packages/openjiuwen/core/sys_operation/local/shell_operation.py',
                        '/opt/jiuwenswarm/.venv/lib/python3.12/site-packages/openjiuwen/core/sys_operation/local/aries_process_observer.py',
                        '/opt/aries-template/verify.py',
                    }, 'unexpected_native_patch_target'
                assert digest(path) == item['after_sha256'], 'native_patch_installed_mismatch'
        assert len(overrides) == 3, 'native_source_overlay_scope_mismatch'
    for name, expected in files.items():
        assert digest(Path('/opt/jiuwenswarm') / name) == overrides.get(name, expected), 'native_source_file_changed'
    return files, overrides


def main():
    bundle = Path(sys.argv[1])
    pins = json.loads((bundle / 'probe-pins.json').read_text())
    script = (bundle / 'control.py').read_text()
    assert digest(bundle / 'control.py') == pins['control_script_sha256']
    files, overrides = verify_native_source(bundle, pins)
    patch_manifest_sha = pins.get('native_patch_manifest_sha256')
    def unprivileged():
        os.setgroups([])
        os.setgid(60000)
        os.setuid(60000)
    native = subprocess.run(['python3', '-I', '/opt/aries-template/verify.py'],
        capture_output=True, text=True, preexec_fn=unprivileged, timeout=60)
    assert native.returncode == 0, 'unprivileged_image_probe_failed'
    image = json.loads(native.stdout)
    assert image['status'] == 'PASS_IMAGE_ONLY'
    assert image.get('native_patch_manifest_sha256') == patch_manifest_sha
    workspace = subprocess.run(['/opt/jiuwenswarm/.venv/bin/python', '-I', '-c',
        (bundle / 'workspace_probe.py').read_text()], capture_output=True, text=True,
        preexec_fn=unprivileged, timeout=60)
    assert workspace.returncode == 0, 'native_workspace_permissions_failed'
    workspace_proof = json.loads(workspace.stdout.splitlines()[-1])
    assert workspace_proof['status'] == 'PASS_NATIVE_WORKSPACE_WRITES_ONLY'
    identifier = uuid.uuid4().hex
    control = '/run/aries-cloud-probe-' + identifier
    group = '/sys/fs/cgroup/aries-probe-' + identifier
    workdir = '/workspace/aries-case'
    result = {'status': 'FAIL', 'image': image, 'native_source_files_verified': len(files),
              'source_manifest_sha256': pins['source_manifest_sha256'],
              'native_source_unmodified_verified': len(files) - len(overrides),
              'native_source_overlay_verified': len(overrides),
              'native_patch_manifest_sha256': patch_manifest_sha,
              'native_workspace': workspace_proof,
              'control_script_sha256': pins['control_script_sha256'], 'model_calls': 0}
    stopped = False
    def call(action, extra=None):
        args = ['python3', '-I', '-c', script, action, control, group]
        if extra is not None:
            args.append(extra)
        return subprocess.run(args, capture_output=True, text=True, timeout=20)
    try:
        prepared = call('prepare', workdir)
        assert prepared.returncode == 0, 'shared_group_prepare_failed'
        code = ('import json,os,pathlib,subprocess; '
                'p=subprocess.Popen(["/bin/sleep","120"],start_new_session=True,stdin=subprocess.DEVNULL,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL); '
                's=dict(x.split(":",1) for x in pathlib.Path("/proc/self/status").read_text().splitlines() if ":" in x); '
                'print(json.dumps({"uid":os.getuid(),"gid":os.getgid(),"child_pid":p.pid,"no_new_privs":s["NoNewPrivs"].strip(),"cap_eff":s["CapEff"].strip()}))')
        executed = call('exec', json.dumps({'path': 'python3', 'args': ['-I', '-c', code], 'dir': workdir}))
        assert executed.returncode == 0, 'shared_group_exec_failed'
        process = json.loads(executed.stdout)
        assert process['uid'] == process['gid'] == 60000
        assert process['no_new_privs'] == '1' and int(process['cap_eff'], 16) == 0
        assert str(process['child_pid']) in (Path(group) / 'cgroup.procs').read_text().split()
        assert Path(control).stat().st_uid == 0 and Path(control).stat().st_mode & 0o077 == 0
        stop = call('stop')
        assert stop.returncode == 0 and stop.stdout.strip() == 'ARIES_GROUP_EMPTY', 'shared_group_stop_unconfirmed'
        stopped = True
        events = dict(line.split() for line in (Path(group) / 'cgroup.events').read_text().splitlines())
        assert events['populated'] == '0'
        denied = call('exec', json.dumps({'path': '/bin/true', 'args': [], 'dir': workdir}))
        assert denied.returncode != 0 and 'agent admission revoked' in denied.stderr
        result.update(status='PASS_GUEST_CAPABILITIES_ONLY', cgroup={
            'uid': 60000, 'gid': 60000, 'no_new_privs': True, 'effective_capabilities': 0,
            'detached_descendant_joined': True, 'root_private_admission': True,
            'populated_after_stop': 0, 'revoked_exec_denied': True})
    finally:
        if Path(group).exists() and Path(control, 'lock').exists() and not stopped:
            stop = call('stop')
            stopped = stop.returncode == 0 and stop.stdout.strip() == 'ARIES_GROUP_EMPTY'
        if stopped:
            Path(group).rmdir()
            shutil.rmtree(control)
        result['probe_group_removed'] = not Path(group).exists()
        result['probe_control_removed'] = not Path(control).exists()
        print(json.dumps(result, sort_keys=True))
    assert result['probe_group_removed'] and result['probe_control_removed']


if __name__ == '__main__':
    main()

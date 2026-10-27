"""Image-only checks; no cloud, model call, configuration or cgroup mutation."""
import hashlib
import json
import os
from pathlib import Path
import pwd
import stat
import subprocess
import sys

ROOT = Path('/opt/jiuwenswarm')
REVISION = 'e2c9d519498080e249e34f16a80c8c97f0cd1962'
LOCK_SHA = 'f28fbbd9b208bcf4f6e421818969ed2b6570f0648bed25c2ed15ee5bd551e5c9'


def main():
    manifest = ROOT / 'aries-build.json'
    info = manifest.lstat()
    assert stat.S_ISREG(info.st_mode) and info.st_uid == info.st_gid == 0
    assert info.st_mode & 0o022 == 0
    build = json.loads(manifest.read_text())
    assert build['revision'] == REVISION and build['uv_lock_sha256'] == LOCK_SHA
    assert hashlib.sha256((ROOT / 'uv.lock').read_bytes()).hexdigest() == LOCK_SHA
    patches = ROOT / 'aries-patches.json'
    overlay = None
    if patches.exists():
        overlay = json.loads(patches.read_text())
        assert overlay['schema_version'] == 1 and overlay['kind'] == 'aries_native_patch_overlay'
        assert overlay['native_modified'] is True and overlay['revision'] == REVISION
        assert overlay['uv_lock_sha256'] == LOCK_SHA
        assert overlay['base_build_sha256'] == hashlib.sha256(manifest.read_bytes()).hexdigest()
        patch_names = [patch['name'] for patch in overlay['patches']]
        image5_patches = ['agent-core-tool-context', 'jiuwenswarm-remote-tools']
        assert patch_names in (image5_patches, image5_patches + ['agent-core-process-observation'])
        for patch in overlay['patches']:
            assert hashlib.sha256(Path(patch['patch_file']).read_bytes()).hexdigest() == patch['patch_sha256']
            for path, pins in patch['files'].items():
                assert hashlib.sha256(Path(path).read_bytes()).hexdigest() == pins['after_sha256'], path
    agent = pwd.getpwnam('aries-agent')
    assert (agent.pw_uid, agent.pw_gid) == (60000, 60000)
    for root in [ROOT, Path('/opt/aries-template'), Path('/opt/aries-envd'), Path('/usr/local')]:
        for directory, dirs, files in os.walk(root):
            for name in [directory] + [str(Path(directory) / n) for n in dirs + files]:
                p = Path(name)
                s = p.lstat()
                assert s.st_uid == s.st_gid == 0, str(p)
                if not p.is_symlink():
                    assert not s.st_mode & 0o022, str(p)
                elif p.exists():
                    target = p.resolve().stat()
                    assert target.st_uid == target.st_gid == 0 and not target.st_mode & 0o022, str(p)
    assert not (ROOT / '.env').exists()
    native_python = ROOT / '.venv/bin/python'
    native = json.loads(subprocess.check_output([str(native_python), '-B', '-I', '-c',
        'import importlib.metadata as m,json,sys; import jiuwenswarm,openjiuwen; '
        'assert (3,11)<=sys.version_info[:2]<(3,14); '
        'print(json.dumps({"python":sys.version.split()[0],"jiuwenswarm":m.version("jiuwenswarm"),"openjiuwen":m.version("openjiuwen")}))'], text=True, timeout=60))
    import pytest
    print(json.dumps({'status': 'PASS_IMAGE_ONLY', 'revision': REVISION,
        'uv_lock_sha256': LOCK_SHA, 'python_path': str(native_python), 'native': native,
        'task_python': sys.version.split()[0], 'pytest': pytest.__version__,
        'agent_uid': agent.pw_uid, 'agent_gid': agent.pw_gid,
        'native_modified': overlay is not None,
        'native_patch_manifest_sha256': hashlib.sha256(patches.read_bytes()).hexdigest() if overlay else None,
        'root_owned_not_agent_writable': True, 'cloud_template': 'UNRUN',
        'cgroup_delegation': 'UNRUN_PROVIDER_RUNTIME_REQUIRED', 'model_calls': 0}, sort_keys=True))


if __name__ == '__main__':
    main()

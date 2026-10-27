"""Apply the two reviewed image5 patches to the exact image4 installation."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess

ROOT = Path('/opt/jiuwenswarm')
PATCHES = Path('/opt/aries-template/native-patches')
REVISION = 'e2c9d519498080e249e34f16a80c8c97f0cd1962'
CORE_REVISION = '3460150aa5f319b0b8f84a4ed10eec0a08b0feb1'
LOCK_SHA = 'f28fbbd9b208bcf4f6e421818969ed2b6570f0648bed25c2ed15ee5bd551e5c9'


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-image', required=True)
    args = parser.parse_args()
    destination = ROOT / 'aries-patches.json'
    assert not destination.exists(), 'native_patch_overlay_already_applied'
    build = json.loads((ROOT / 'aries-build.json').read_text())
    assert build['revision'] == REVISION
    assert build['git_dependencies']['agent-core'] == CORE_REVISION
    assert sha(ROOT / 'uv.lock') == build['uv_lock_sha256'] == LOCK_SHA
    purelib = Path(subprocess.check_output([str(ROOT / '.venv/bin/python'), '-B', '-I', '-c',
        'import sysconfig;print(sysconfig.get_paths()["purelib"])'], text=True).strip())
    assert purelib.is_relative_to(ROOT / '.venv')
    record = {'schema_version': 1, 'kind': 'aries_native_patch_overlay', 'native_modified': True,
        'base_image_reference': args.base_image, 'base_build_sha256': sha(ROOT / 'aries-build.json'),
        'revision': REVISION, 'uv_lock_sha256': LOCK_SHA, 'patches': []}
    for name, root, revision in [('agent-core-tool-context', purelib, CORE_REVISION),
                                 ('jiuwenswarm-remote-tools', ROOT, REVISION)]:
        patch = PATCHES / (name + '.patch')
        manifest = json.loads((PATCHES / (name + '.json')).read_text())
        assert manifest['upstream_revision'] == revision and manifest['patch_sha256'] == sha(patch)
        files = {}
        for relative, pins in manifest['files'].items():
            target = root / relative
            assert target.is_relative_to(root) and '..' not in Path(relative).parts
            assert sha(target) == pins['before_sha256'], 'native_patch_input_mismatch: ' + str(target)
            files[str(target)] = pins
        assert files, 'empty_native_patch'
        subprocess.run(['git', 'apply', '--check', str(patch)], cwd=root, check=True)
        subprocess.run(['git', 'apply', str(patch)], cwd=root, check=True)
        for target, pins in files.items():
            assert sha(target) == pins['after_sha256'], 'native_patch_output_mismatch: ' + target
        record['patches'].append({'name': name, 'upstream_revision': revision,
            'patch_file': str(patch), 'patch_sha256': sha(patch), 'files': files})
    assert sha(ROOT / 'uv.lock') == LOCK_SHA
    assert sha(ROOT / 'aries-build.json') == record['base_build_sha256']
    destination.write_text(json.dumps(record, indent=2, sort_keys=True) + '\n')
    destination.chmod(0o444)
    print(json.dumps(record, sort_keys=True))


if __name__ == '__main__':
    main()

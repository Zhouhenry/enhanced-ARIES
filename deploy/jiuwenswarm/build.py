#!/usr/bin/env python3
"""ECS-only pinned local image build. No cloud template or Sandbox creation."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile
import urllib.request

REVISION = 'e2c9d519498080e249e34f16a80c8c97f0cd1962'
LOCK_SHA = 'f28fbbd9b208bcf4f6e421818969ed2b6570f0648bed25c2ed15ee5bd551e5c9'
SOURCE_ARCHIVE_SHA = '7f844734561d86d8257af323377b7623a82669c2941788c56d9776c3b51d9c39'
SOURCE_TREE_SHA = 'd72c6ac035f954eb5b6d89ca999598e7374382e2af82c6ca0e96dcfb6b0ca934'
UV_SHA = 'd3dc3ca8e29337dd602a9a4df9e6edb15ebc52aed91461debeedb96939dc4ce0'
ENVD_SHA = '0287fa6b6b1e727ad4b420a102fe79618b8cd8248806d9cef49af5fab2ca0587'
BASE_IMAGE = 'sha256:6a0e06f5ba0b373d051aabd3fffb9b873f27f9a2a3a14bcbe37dee6f0e3bbc70'
BASE_TAG = 'aries-local/jiuwen-python:base-6a0e06f5ba0b'
HERE = Path(__file__).resolve().parent
INTEGRATION = Path('/opt/aries-artifacts/jiuwen-integration-1')


def sha(path):
    h = hashlib.sha256()
    with Path(path).open('rb') as f:
        for block in iter(lambda: f.read(1024 * 1024), b''):
            h.update(block)
    return h.hexdigest()


def write(path, value):
    Path(path).write_text(json.dumps(value, sort_keys=True, indent=2) + '\n')


def run(args, *, log=None, timeout=1800):
    if log:
        with Path(log).open('w') as output:
            subprocess.run(args, check=True, stdout=output, stderr=subprocess.STDOUT, timeout=timeout)
        return ''
    return subprocess.check_output(args, text=True, timeout=timeout)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True, help='new independent artifact directory')
    parser.add_argument('--tag', default='aries-jiuwenswarm:e2c9d519-image4')
    args = parser.parse_args()
    if sys.platform != 'linux' or os.geteuid() != 0:
        parser.error('build only as root on the authorized Linux ECS')
    out = args.output.absolute()
    out.mkdir(mode=0o700, parents=True, exist_ok=False)
    evidence = {'status': 'FAIL', 'started_at': datetime.now(timezone.utc).isoformat(),
        'scope': 'Linux image build, local native import, exact bootstrap initialization and ENVD smoke; no task/model request',
        'cloud_templates_created': 0, 'sandboxes_created': 0, 'model_calls': 0,
        'resources_for_future_template': {'cpu_count': 2, 'memory_mb': 4096},
        'cgroup_delegation': 'UNRUN_PROVIDER_RUNTIME_REQUIRED', 'registry_push': 'UNRUN'}
    try:
        base = json.loads(run(['docker', 'image', 'inspect', BASE_IMAGE]))[0]
        assert base['Id'] == BASE_IMAGE and base['Os'] == 'linux' and base['Architecture'] == 'amd64'
        run(['docker', 'tag', BASE_IMAGE, BASE_TAG])
        assert json.loads(run(['docker', 'image', 'inspect', BASE_TAG]))[0]['Id'] == BASE_IMAGE
        inputs = {'source.tar.gz': (INTEGRATION / 'source.tar.gz', SOURCE_ARCHIVE_SHA),
            'uv': (INTEGRATION / 'tooling/bin/uv', UV_SHA),
            'envd': (Path('/opt/aries-artifacts/agentsphere-image-1/context/envd'), ENVD_SHA)}
        for path, expected in inputs.values():
            assert sha(path) == expected, str(path)
        context = out / 'context'
        context.mkdir()
        source = context / 'source'
        source.mkdir()
        with tarfile.open(inputs['source.tar.gz'][0]) as archive:
            archive.extractall(source, filter='data')
        native_files = {str(p.relative_to(source)): sha(p) for p in source.rglob('*') if p.is_file()}
        write(out / 'source-manifest.json', native_files)
        assert sha(out / 'source-manifest.json') == SOURCE_TREE_SHA
        assert sha(source / 'uv.lock') == LOCK_SHA
        for name in ('uv', 'envd'):
            shutil.copyfile(inputs[name][0], context / name)
        recipe_files = ('Dockerfile', 'verify.py', 'start.sh', 'build.py', 'e2b_template.py', 'pytest-wheels.json', 'README.md', 'bootstrap_probe.py')
        recipe = out / 'recipe'
        recipe.mkdir()
        for name in recipe_files:
            shutil.copyfile(HERE / name, recipe / name)
        for name in ('Dockerfile', 'verify.py', 'start.sh'):
            shutil.copyfile(HERE / name, context / name)
        wheel_root = context / 'pytest-wheels'
        wheel_root.mkdir()
        wheels = json.loads((HERE / 'pytest-wheels.json').read_text())
        for wheel in wheels:
            target = wheel_root / wheel['url'].rsplit('/', 1)[1]
            urllib.request.urlretrieve(wheel['url'], target)
            assert sha(target) == wheel['hash'].removeprefix('sha256:')
        build = {'schema_version': 1, 'revision': REVISION, 'uv_lock_sha256': LOCK_SHA,
            'source_archive_sha256': SOURCE_ARCHIVE_SHA, 'source_manifest_sha256': SOURCE_TREE_SHA,
            'base_image_id': BASE_IMAGE, 'platform': 'linux/amd64', 'uv_version': '0.9.5',
            'uv_sha256': UV_SHA, 'envd_sha256': ENVD_SHA,
            'envd_revision': '3172e41266d408b462b0b5e8dc764d80f6dc2675',
            'git_dependencies': {'agent-core': '3460150aa5f319b0b8f84a4ed10eec0a08b0feb1',
                'agent-runtime': '503700c0e93ef54cb45a41474ae36f104c1642e7',
                'deepsearch': '05d846214487c603a31022c3d46782675d34d4f9'},
            'recipe_sha256': {name: sha(HERE / name) for name in recipe_files},
            'python_path': '/opt/jiuwenswarm/.venv/bin/python',
            'dependency_install': 'uv sync --frozen --offline --no-default-groups',
            'agent_core_transport': 'same-commit file mirror seeded existing uv cache'}
        write(context / 'aries-build.json', build)
        evidence.update(build=build, aries_build_sha256=sha(context / 'aries-build.json'),
            disk_free_before=shutil.disk_usage(out).free, phase='docker_build')
        write(out / 'evidence.json', evidence)
        run(['docker', 'buildx', 'build', '--load', '--pull=false', '--progress=plain',
             '--build-context', 'uv_cache=' + str(INTEGRATION / 'uv-cache'),
             '--build-arg', 'BASE_IMAGE=' + BASE_TAG, '-t', args.tag, str(context)], log=out / 'build.log')
        image = json.loads(run(['docker', 'image', 'inspect', args.tag]))[0]
        evidence.update(image_id=image['Id'], image_tag=args.tag, image_size_bytes=image['Size'], phase='local_probes')
        write(out / 'evidence.json', evidence)
        probe = json.loads(run(['docker', 'run', '--rm', '--network=none', '--read-only',
            '--tmpfs=/tmp:rw,nosuid,nodev,size=64m', '--cpus=2', '--memory=1g',
            '--user=60000:60000', '--entrypoint=python3', image['Id'], '-I', '/opt/aries-template/verify.py']))
        assert probe['status'] == 'PASS_IMAGE_ONLY'
        write(out / 'image-probe.json', probe)
        envd_version = run(['docker', 'run', '--rm', '--network=none', '--read-only',
            '--entrypoint=/opt/aries-envd/envd', image['Id'], '-version']).strip()
        envd_commit = run(['docker', 'run', '--rm', '--network=none', '--read-only',
            '--entrypoint=/opt/aries-envd/envd', image['Id'], '-commit']).strip()
        assert '0.10.0' in envd_version and '3172e41266d408b462b0b5e8dc764d80f6dc2675' in envd_commit
        bootstrap = Path('/opt/aries-artifacts/release15/source/pkg/harness/jiuwenswarm/bootstrap.py')
        assert bootstrap.is_file() and sha(bootstrap) == '31c757f856a4a42ef85e38873d988ba25c8aba2ac67572c70cfe0920c342c694'
        shutil.copyfile(bootstrap, out / 'bootstrap.py')
        smoke = out / 'bootstrap-smoke'
        smoke.mkdir()
        container_name = 'aries-jiuwen-bootstrap-' + out.name
        try:
            run(['docker', 'run', '--name', container_name, '--label', 'aries.owner=jiuwen-image4',
                '--network=none', '--read-only', '--cpus=2', '--memory=2g',
                '--tmpfs=/tmp:rw,nosuid,nodev,size=256m', '--tmpfs=/run:rw,nosuid,nodev,size=256m',
                '--tmpfs=/workspace/aries-case:rw,nosuid,nodev,uid=60000,gid=60000,mode=0700,size=64m',
                '-v', str(recipe / 'bootstrap_probe.py') + ':/probe.py:ro',
                '-v', str(out / 'bootstrap.py') + ':/bootstrap.py:ro',
                '-v', str(smoke) + ':/probe-output:rw', '--entrypoint=python3', image['Id'],
                '-I', '/probe.py', '--bootstrap', '/bootstrap.py', '--output', '/probe-output'],
                log=out / 'bootstrap-smoke.log', timeout=150)
            bootstrap_probe = json.loads((smoke / 'evidence.json').read_text())
            assert bootstrap_probe['status'] == 'PASS_NATIVE_BOOTSTRAP_SMOKE_NO_TASK'
            evidence['native_bootstrap_smoke'] = bootstrap_probe
        finally:
            owned = json.loads(run(['docker', 'inspect', container_name]))[0]
            assert owned['Image'] == image['Id'] and owned['Config']['Labels']['aries.owner'] == 'jiuwen-image4'
            run(['docker', 'rm', '-f', container_name])
            evidence['bootstrap_container_removed'] = not run(['docker', 'ps', '-aq', '--filter', 'name=^/' + container_name + '$']).strip()
        evidence.update(status='PASS_PINNED_IMAGE_LOCAL_ONLY', finished_at=datetime.now(timezone.utc).isoformat(),
            image_probe=probe, envd_version=envd_version, envd_commit=envd_commit,
            disk_free_after=shutil.disk_usage(out).free, phase='local_acceptance_complete')
    except BaseException as exc:
        evidence.update(error_class=type(exc).__name__, finished_at=datetime.now(timezone.utc).isoformat())
        raise
    finally:
        write(out / 'evidence.json', evidence)
    print(json.dumps({k: evidence[k] for k in ['status', 'image_id', 'image_size_bytes']}))


if __name__ == '__main__':
    main()

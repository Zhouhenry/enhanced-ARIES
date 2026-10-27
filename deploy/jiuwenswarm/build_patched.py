#!/usr/bin/env python3
"""ECS-only incremental image5 build and local checks; no push or cloud calls."""
import argparse
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys

from build import HERE, sha, write, run

BASE_IMAGE = ('swr.cn-southwest-2.myhuaweicloud.com/benchmark-zhoutao/'
              'aries-jiuwenswarm:0.2.3-e2c9d519-image4@sha256:'
              'f0a82b0e258a0e763defa922c7a01410a40698b92967d03d7279e7976fdf6489')
NATIVE_PYTHON = '/opt/jiuwenswarm/.venv/bin/python'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--runtime-source', type=Path, required=True,
                        help='frozen pkg/harness/jiuwenswarm directory containing bootstrap.py and remote/')
    parser.add_argument('--tag', default='aries-jiuwenswarm:e2c9d519-image5')
    args = parser.parse_args()
    if sys.platform != 'linux' or os.geteuid() != 0:
        parser.error('build only as root on the authorized Linux ECS')
    if subprocess.run(['docker', 'image', 'inspect', args.tag], stdout=subprocess.DEVNULL,
                      stderr=subprocess.DEVNULL).returncode == 0:
        parser.error('candidate tag already exists; preserve previous evidence and choose a new tag')
    out = args.output.absolute()
    out.mkdir(mode=0o700, parents=True, exist_ok=False)
    evidence = {'status': 'FAIL', 'started_at': datetime.now(timezone.utc).isoformat(),
        'scope': 'image4 plus exact ARIES native patches; local image/bootstrap/Tool tests only',
        'base_image_reference': BASE_IMAGE, 'registry_push': 'UNRUN', 'cloud_templates_created': 0,
        'sandboxes_created': 0, 'model_calls': 0, 'native_modified': True,
        'native_two_container_tools': 'SEPARATE_VALIDATION_REQUIRED',
        'cgroup_delegation': 'UNRUN_PROVIDER_RUNTIME_REQUIRED', 'containers_removed': []}
    def container(name, image, command, *, mounts=(), root=False, bootstrap=False, log=None):
        label = 'aries.owner=jiuwen-image5'
        owned_name = 'aries-image5-' + out.name + '-' + name
        options = ['docker', 'run', '--name', owned_name, '--label', label,
            '--network=none', '--read-only', '--workdir=/tmp', '--cpus=2', '--memory=2g',
            '--tmpfs=/tmp:rw,nosuid,nodev,size=256m', '-e', 'HOME=/tmp',
            '-e', 'PYTHONDONTWRITEBYTECODE=1']
        if not root:
            options += ['--user=60000:60000']
        if bootstrap:
            options += ['--tmpfs=/run:rw,nosuid,nodev,size=256m',
                '--tmpfs=/workspace/aries-case:rw,nosuid,nodev,uid=60000,gid=60000,mode=0700,size=64m']
        for mount in mounts:
            options += ['-v', mount]
        try:
            return run([*options, '--entrypoint', command[0], image, *command[1:]], log=log, timeout=300)
        finally:
            inspect = subprocess.run(['docker', 'inspect', owned_name], capture_output=True, text=True)
            if inspect.returncode == 0:
                owned = json.loads(inspect.stdout)[0]
                assert owned['Image'] == image and owned['Config']['Labels'].get('aries.owner') == 'jiuwen-image5'
                run(['docker', 'rm', '-f', owned_name])
            assert not run(['docker', 'ps', '-aq', '--filter', 'name=^/' + owned_name + '$']).strip()
            evidence['containers_removed'].append(owned_name)
            write(out / 'evidence.json', evidence)
    try:
        base = json.loads(run(['docker', 'image', 'inspect', BASE_IMAGE]))[0]
        assert base['Os'] == 'linux' and base['Architecture'] == 'amd64'
        evidence.update(base_image_id=base['Id'], base_layers=base['RootFS']['Layers'],
                        disk_free_before=shutil.disk_usage(out).free)
        recipe = out / 'recipe'
        recipe.mkdir()
        names = ['Dockerfile.patched', 'apply_patches.py', 'verify.py', 'build_patched.py',
                 'build.py', 'bootstrap_probe.py', 'tests/test_tool_context.py']
        names += ['patches/' + stem + suffix for stem in ('agent-core-tool-context', 'jiuwenswarm-remote-tools')
                  for suffix in ('.patch', '.json')]
        for name in names:
            target = recipe / name
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(HERE / name, target)
        evidence['recipe_sha256'] = {name: sha(recipe / name) for name in names}
        runtime = out / 'runtime'
        runtime.mkdir()
        for origin in [args.runtime_source / 'bootstrap.py', *sorted((args.runtime_source / 'remote').glob('*.py'))]:
            target = runtime / origin.relative_to(args.runtime_source)
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(origin, target)
        evidence['runtime_sha256'] = {str(path.relative_to(runtime)): sha(path)
                                      for path in sorted(runtime.rglob('*.py'))}
        context = out / 'context'
        context.mkdir()
        for name in ['Dockerfile.patched', 'apply_patches.py', 'verify.py', *[n for n in names if n.startswith('patches/')]]:
            target = context / ('Dockerfile' if name == 'Dockerfile.patched' else name)
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(recipe / name, target)
        write(out / 'evidence.json', evidence)
        run(['docker', 'buildx', 'build', '--load', '--pull=false', '--network=none', '--progress=plain',
             '--build-arg', 'BASE_IMAGE=' + BASE_IMAGE, '-t', args.tag, str(context)], log=out / 'build.log')
        image = json.loads(run(['docker', 'image', 'inspect', args.tag]))[0]
        assert image['RootFS']['Layers'][:len(base['RootFS']['Layers'])] == base['RootFS']['Layers']
        evidence.update(image_id=image['Id'], image_tag=args.tag, image_size_bytes=image['Size'],
                        added_size_bytes=image['Size'] - base['Size'], base_layers_reused=True)
        write(out / 'evidence.json', evidence)
        original = json.loads(container('baseline', base['Id'], ['python3', '-B', '-I', '/opt/aries-template/verify.py'],
            mounts=[str(recipe / 'verify.py') + ':/opt/aries-template/verify.py:ro']))
        assert original['status'] == 'PASS_IMAGE_ONLY' and original['native_modified'] is False
        write(out / 'baseline-probe.json', original)
        probe = json.loads(container('verify', image['Id'], ['python3', '-B', '-I', '/opt/aries-template/verify.py']))
        assert probe['status'] == 'PASS_IMAGE_ONLY' and probe['native_modified'] is True
        write(out / 'image-probe.json', probe)
        overlay = json.loads(container('manifest', image['Id'], ['python3', '-B', '-I', '-c',
            'from pathlib import Path;print(Path("/opt/jiuwenswarm/aries-patches.json").read_text())']))
        write(out / 'aries-patches.json', overlay)
        evidence['native_patch_manifest_sha256'] = probe['native_patch_manifest_sha256']
        container('tool-context', image['Id'], [NATIVE_PYTHON, '-B', '-I', '/tests/test_tool_context.py'],
            mounts=[str(recipe / 'tests') + ':/tests:ro'], log=out / 'tool-context-tests.log')
        container('remote', image['Id'], [NATIVE_PYTHON, '-B', '-I', '/runtime/remote/test_remote.py'],
            mounts=[str(runtime) + ':/runtime:ro'], log=out / 'remote-tests.log')
        smoke = out / 'bootstrap-smoke'
        smoke.mkdir()
        container('bootstrap', image['Id'], ['python3', '-B', '-I', '/probe.py', '--bootstrap', '/bootstrap.py',
            '--output', '/probe-output'], root=True, bootstrap=True,
            mounts=[str(recipe / 'bootstrap_probe.py') + ':/probe.py:ro',
                    str(runtime / 'bootstrap.py') + ':/bootstrap.py:ro', str(smoke) + ':/probe-output:rw'],
            log=out / 'bootstrap-smoke.log')
        bootstrap = json.loads((smoke / 'evidence.json').read_text())
        assert bootstrap['status'] == 'PASS_NATIVE_BOOTSTRAP_SMOKE_NO_TASK'
        bad_overlay = json.loads(json.dumps(overlay))
        bad_overlay['patches'][0]['patch_sha256'] = '0' * 64
        bad_manifest = out / 'tampered-patches.json'
        write(bad_manifest, bad_overlay)
        bad_manifest.chmod(0o444)
        negative = out / 'bootstrap-negative'
        negative.mkdir()
        try:
            container('bad-manifest', image['Id'], ['python3', '-B', '-I', '/probe.py',
                '--bootstrap', '/bootstrap.py', '--output', '/probe-output'], root=True, bootstrap=True,
                mounts=[str(recipe / 'bootstrap_probe.py') + ':/probe.py:ro',
                        str(runtime / 'bootstrap.py') + ':/bootstrap.py:ro', str(negative) + ':/probe-output:rw',
                        str(bad_manifest) + ':/opt/jiuwenswarm/aries-patches.json:ro'],
                log=out / 'bootstrap-negative.log')
        except subprocess.CalledProcessError as exc:
            assert exc.returncode == 1
        else:
            raise AssertionError('tampered_native_manifest_was_accepted')
        assert 'native patch evidence mismatch' in (out / 'bootstrap-negative.log').read_text()
        assert not (negative / 'agentserver.log').exists()
        evidence['tampered_manifest_rejected_before_agent_start'] = True
        assert json.loads(run(['docker', 'image', 'inspect', BASE_IMAGE]))[0]['Id'] == base['Id']
        evidence.update(status='PASS_PATCHED_IMAGE_LOCAL_ONLY', image_probe=probe,
                        native_bootstrap_smoke=bootstrap, base_image_unchanged=True,
                        disk_free_after=shutil.disk_usage(out).free)
    except BaseException as exc:
        evidence.update(error_class=type(exc).__name__)
        raise
    finally:
        evidence['finished_at'] = datetime.now(timezone.utc).isoformat()
        write(out / 'evidence.json', evidence)
    print(json.dumps({name: evidence[name] for name in ('status', 'image_id', 'added_size_bytes')}))


if __name__ == '__main__':
    main()

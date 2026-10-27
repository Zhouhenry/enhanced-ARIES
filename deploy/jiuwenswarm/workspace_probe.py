"""Exercise native workspace copying and private writes as the actual agent UID."""
import json
import os
from pathlib import Path
import stat
import tempfile


def main():
    assert os.geteuid() == os.getegid() == 60000
    with tempfile.TemporaryDirectory(prefix='aries-native-workspace-') as temporary:
        home = Path(temporary)
        data = home / 'data'
        config = data / 'config'
        os.environ.update(HOME=str(home), JIUWENSWARM_DATA_DIR=str(data),
            JIUWENSWARM_CONFIG_DIR=str(config), PYTHONDONTWRITEBYTECODE='1')
        from jiuwenswarm.common.utils import prepare_workspace
        prepare_workspace(overwrite=False, preferred_language='en')
        import yaml
        config_file = config / 'config.yaml'
        settings = yaml.safe_load(config_file.read_text()) or {}
        settings.setdefault('flash', {})['enabled'] = True
        config_file.write_text(yaml.safe_dump(settings))
        env_file = config / '.env'
        assert env_file.exists()
        env_file.write_text('ARIES_PUBLIC_PERMISSION_PROBE=1\n')
        agents = list(data.rglob('AGENT.md'))
        assert len(agents) == 1
        workspace = agents[0].parent
        for name in ('memory', 'projects', 'skills'):
            folder = workspace / name
            assert folder.is_dir()
            marker = folder / 'aries-permission-probe'
            marker.write_text('public probe')
            marker.unlink()
        for path in workspace.rglob('*'):
            assert path.stat().st_uid == 60000 and path.stat().st_mode & 0o200
        source = Path('/opt/jiuwenswarm/jiuwenswarm/resources')
        source_paths = (source/'agent/workspace', source/'agent/workspace/AGENT_EN.md', source/'.env.template')
        for path in source_paths:
            info = path.stat()
            assert info.st_uid == 0 and not info.st_mode & 0o022 and info.st_mode & 0o200
            assert not os.access(path, os.W_OK)
        result = {'status': 'PASS_NATIVE_WORKSPACE_WRITES_ONLY', 'uid': 60000,
            'config_yaml_rewritten': True, 'copied_env_rewritten': True,
            'agent_file_created': True, 'memory_projects_skills_writable': True,
            'source_not_agent_writable': True,
            'source_workspace_mode': oct(stat.S_IMODE(source_paths[0].stat().st_mode)),
            'private_workspace_mode': oct(stat.S_IMODE(workspace.stat().st_mode)),
            'source_env_mode': oct(stat.S_IMODE(source_paths[2].stat().st_mode)),
            'copied_env_mode': oct(stat.S_IMODE(env_file.stat().st_mode)), 'model_calls': 0}
    result['temporary_workspace_removed'] = not home.exists()
    print(json.dumps(result, sort_keys=True))


if __name__ == '__main__':
    main()

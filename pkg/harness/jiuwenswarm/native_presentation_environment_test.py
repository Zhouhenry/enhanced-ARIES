"""ECS/cloud fixture: real staged entrances and native tools, no model calls.

Root only stages trusted files and starts UID 60000 children. A runs worker.main;
B runs bootstrap.run with only its final AgentServer exec mocked. Neither shell
command receives a fixture-supplied PATH/NODE_PATH or an explicit venv executable.
"""
import argparse
import asyncio
import hashlib
import json
import os
from pathlib import Path
import re
import runpy
import shlex
import signal
import socket
import subprocess
import sys
import time
from unittest.mock import patch


HERE = Path(__file__).resolve().parent
MIN_PATH = '/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin'
NODE_MODULES = '/opt/aries-presentation/node_modules'


def minimal_env(home):
    return dict(PATH=MIN_PATH, HOME=str(home), LANG='C.UTF-8',
                PYTHONDONTWRITEBYTECODE='1', PYTHONUNBUFFERED='1')


def commands(enabled):
    imports = 'import pptx,fitz,PIL,lxml;' if enabled else ''
    python = imports + "import os,sys,json;print('ARIES_ENV='+json.dumps(dict(uid=os.getuid(),python=sys.executable,path=os.environ['PATH'],node_path=os.getenv('NODE_PATH'))))"
    result = {'python': 'python3 -c ' + shlex.quote(python)}
    if enabled:
        node = "console.log('ARIES_ENV='+JSON.stringify({module:require.resolve('pptxgenjs'),node_path:process.env.NODE_PATH,uid:process.getuid()}))"
        result['node'] = 'node -e ' + shlex.quote(node)
    return result


def check_output(text, kind, enabled, native):
    value = json.loads(re.search(r'ARIES_ENV=(\{[^\n]+\})', text).group(1))
    assert value['uid'] == 60000, value
    if kind == 'python':
        expected_path = str(native / '.venv/bin') + ':' + MIN_PATH if enabled else MIN_PATH
        assert value['path'] == expected_path, value
        assert value['node_path'] == (NODE_MODULES if enabled else None), value
        if enabled:
            assert Path(value['python']).parent == native / '.venv/bin', value
    else:
        assert value['node_path'] == NODE_MODULES and value['module'].startswith(NODE_MODULES + '/'), value
    return value


def alive(pid):
    try:
        return (Path('/proc') / str(pid) / 'stat').read_text().split(') ', 1)[1].split()[0] != 'Z'
    except FileNotFoundError:
        return False


async def wait_for(check, seconds=30):
    async with asyncio.timeout(seconds):
        while not check():
            await asyncio.sleep(.02)


async def check_worker(runtime, work, executable, native, enabled):
    import aiohttp
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        port = sock.getsockname()[1]
    binding = json.loads((runtime / 'binding.json').read_text())
    with (runtime / 'worker.log').open('wb') as log:
        proc = subprocess.Popen([str(executable), '-I', str(runtime / 'worker.py'),
                                 '--binding', str(runtime / 'binding.json'),
                                 '--listen', '127.0.0.1:' + str(port)],
                                env=minimal_env(runtime / 'home'), user=60000, group=60000,
                                extra_groups=[], start_new_session=True, stdout=log, stderr=subprocess.STDOUT)
        output = {}
        try:
            async with aiohttp.ClientSession(headers={'X-Aries-Worker-Token': binding['worker_token']}) as client:
                url = 'http://127.0.0.1:' + str(port)
                async with asyncio.timeout(40):
                    while True:
                        assert proc.poll() is None, 'worker exited; inspect worker.log'
                        try:
                            async with client.get(url + '/health') as response:
                                assert response.status == 200
                                break
                        except aiohttp.ClientConnectionError:
                            await asyncio.sleep(.05)
                def body(command, invocation):
                    return dict(schema_version=1, binding_id=binding['binding_id'],
                                invocation_id=invocation, identity=dict(run_id=binding['run_id'],
                                task_id=binding['task_id'], session_id='environment-fixture',
                                request_id='environment-request', tool_call_id=invocation),
                                operation=dict(kind='shell', method='execute_cmd'), stream=False,
                                params=dict(command=command), params_bytes=[], cwd=str(work))
                async def call(command, invocation):
                    async with client.post(url + '/call', json=body(command, invocation)) as response:
                        assert response.status == 200
                        return [json.loads(line) for line in (await response.text()).splitlines()]
                for kind, command in commands(enabled).items():
                    frames = await call(command, kind)
                    result = frames[0]['result']
                    assert result['code'] == 0 and result['data']['exit_code'] == 0, result
                    assert result['data']['command'] == command
                    assert frames[-1]['event'] == 'end'
                    output[kind] = check_output(result['data']['stdout'], kind, enabled, native)
                frames = await call("printf native-out; printf native-err >&2; exit 7", 'exit-seven')
                result = frames[0]['result']
                assert result['code'] == 0 and result['data']['exit_code'] == 7
                assert result['data']['stdout'] == 'native-out' and result['data']['stderr'] == 'native-err'
                pidfile = work / 'a-cancel.pid'
                pending = asyncio.create_task(call('echo $$ > ' + shlex.quote(str(pidfile)) + '; sleep 60', 'cancel'))
                await wait_for(pidfile.exists)
                pid = int(pidfile.read_text())
                async with client.post(url + '/cancel', json=dict(schema_version=1,
                        binding_id=binding['binding_id'], invocation_id='cancel')) as response:
                    cancellation = await response.json()
                    assert cancellation['invocations_found'] == 1 and cancellation['active_calls_joined']
                await asyncio.gather(pending, return_exceptions=True)
                await wait_for(lambda: not alive(pid))
                output.update(exit_seven_preserved=True, cancellation_joined=True)
        finally:
            if proc.poll() is None:
                proc.terminate()
                try:
                    await asyncio.to_thread(proc.wait, 15)
                except subprocess.TimeoutExpired:
                    os.killpg(proc.pid, signal.SIGKILL)
                    await asyncio.to_thread(proc.wait, 10)
            assert proc.returncode == 0, proc.returncode
        output['worker_main_uid'] = 60000
        return output


async def check_embedded(runtime, native, enabled):
    from openjiuwen.core.sys_operation import LocalWorkConfig, OperationMode, SysOperation, SysOperationCard
    from openjiuwen.core.sys_operation.cwd import init_cwd
    from jiuwenswarm.agents.harness.flash.tools.flash_bash_tool import FlashBashTool
    spec = json.loads((runtime / 'spec.json').read_text())
    work = Path(spec['workdir'])
    op = SysOperation(SysOperationCard(id='presentation-environment', mode=OperationMode.LOCAL,
                     work_config=LocalWorkConfig(work_dir=str(work), shell_allowlist=None, restrict_to_sandbox=False)))
    init_cwd(str(work))
    tool = FlashBashTool(op, language='en')
    output = {}
    for kind, command in commands(enabled).items():
        result = await tool.invoke({'command': command, 'workdir': str(work)})
        assert result.success, result
        output[kind] = check_output(result.data['content'], kind, enabled, native)
    result = await tool.invoke({'command': 'printf native-out; printf native-err >&2; exit 7'})
    assert not result.success and 'native-out' in result.data['content'] and 'native-err' in result.data['content']
    pidfile = work / 'b-cancel.pid'
    pending = asyncio.create_task(op.shell().execute_cmd('echo $$ > ' + shlex.quote(str(pidfile)) + '; sleep 60'))
    await wait_for(pidfile.exists)
    pid = int(pidfile.read_text())
    pending.cancel()
    try:
        await pending
        raise AssertionError('native cancellation was swallowed')
    except asyncio.CancelledError:
        pass
    await wait_for(lambda: not alive(pid))
    output.update(exit_seven_preserved=True, cancellation_joined=True)
    from jiuwenswarm.common.config import get_config
    from jiuwenswarm.common.utils import get_builtin_skills_dir, get_agent_workspace_dir
    from jiuwenswarm.server.runtime.agent_adapter.interface_flash import JiuwenSwarmFlashAdapter
    from jiuwenswarm.server.runtime.skill.skill_manager import SkillManager
    from openjiuwen.core.foundation.llm.model import init_model
    from openjiuwen.core.single_agent.schema.agent_card import AgentCard
    from openjiuwen.core.runner.runner import Runner
    from openjiuwen.harness.factory import create_deep_agent
    adapter = JiuwenSwarmFlashAdapter()
    adapter._skill_manager = SkillManager()
    rail = adapter._build_skill_rail(get_config().get('react', {}), include_tools=False)
    await rail.reload_skills()
    builtin = get_builtin_skills_dir()
    expected = {p.parent.name: str(p.parent) for p in builtin.glob('*/SKILL.md')}
    catalog = {skill.name: str(skill.directory) for skill in rail.skills_meta}
    assert catalog == expected and len(catalog) == 20
    model = init_model(provider='OpenAI', model_name='never-called', api_key='offline-fixture', api_base='http://127.0.0.1:9/v1')
    agent = create_deep_agent(model=model, card=AgentCard(name='environment_fixture'),
                             workspace=get_agent_workspace_dir(), sys_operation=op,
                             enable_task_loop=False, enable_security_rail=False, enable_llm_retry_rail=False)
    await agent.register_rail(rail)
    card = next(card for card in agent.ability_manager.list() if card.name == 'skill_tool')
    skill = await Runner.resource_mgr.get_tool(card.id).invoke({'skill_name': 'ppt-creation'})
    assert skill.success and 'PPT Creation' in skill.data['content']
    await agent.unregister_rail(rail)
    output.update(skill_catalog=catalog, ppt_skill_tool_read=True)
    return output


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--root', type=Path, required=True)
    parser.add_argument('--native', type=Path, default=Path('/opt/jiuwenswarm'))
    parser.add_argument('--expected', choices=('enabled', 'legacy'), required=True)
    parser.add_argument('--embedded-child', action='store_true', help=argparse.SUPPRESS)
    args = parser.parse_args()
    enabled = args.expected == 'enabled'
    assert Path(NODE_MODULES).is_dir() == enabled
    assert os.environ['PATH'] == MIN_PATH and 'NODE_PATH' not in os.environ
    root, native = args.root.resolve(), args.native.resolve()
    executable = native / '.venv/bin/python'
    if args.embedded_child:
        assert os.geteuid() == 60000
        runtime = root / 'embedded'
        with patch.object(sys, 'argv', [str(runtime / 'launch.py'), 'run', str(runtime)]), patch('os.execv') as final_exec:
            runpy.run_path(str(runtime / 'launch.py'), run_name='__main__')
        final_exec.assert_called_once()
        assert final_exec.call_args.args[1][1:3] == ['-m', 'jiuwenswarm.server.app_agentserver']
        result = asyncio.run(check_embedded(runtime, native, enabled))
        result.update(uid=60000, final_agent_server_exec='MOCKED; actual bootstrap configuration and native tools exercised')
        (runtime / 'home/environment-result.json').write_text(json.dumps(result, indent=2) + '\n')
        return
    assert os.geteuid() == 0 and sys.platform.startswith('linux')
    root.mkdir(mode=0o755)
    work = root / 'work'
    work.mkdir(mode=0o700)
    os.chown(work, 60000, 60000)
    result = dict(status='FAIL', model_calls=0, expected=args.expected,
                  initial_path=MIN_PATH, initial_node_path=None,
                  source_sha256={str(p.relative_to(HERE)): hashlib.sha256(p.read_bytes()).hexdigest()
                                 for p in [Path(__file__), HERE / 'bootstrap.py', HERE / 'remote/worker.py', HERE / 'remote/wire.py', HERE / 'remote_bridge.go']})
    try:
        build = json.loads((native / 'aries-build.json').read_text())
        remote = root / 'remote'
        binding = dict(schema_version=1, binding_id='presentation-environment', run_id='fixture', task_id='environment',
                       source_dir=str(native), python_path=str(executable), revision=build['revision'],
                       uv_lock_sha256=build['uv_lock_sha256'], worker_token='offline-fixture', workdir=str(work),
                       home=str(remote / 'home'), evidence_path=str(remote / 'home/events.jsonl'), process_observation=False,
                       files={name: (HERE / 'remote' / name).read_text() for name in ('worker.py', 'wire.py')})
        stage = re.search(r'const remoteStageScript = `(.*?)`', (HERE / 'remote_bridge.go').read_text(), re.S).group(1)
        subprocess.run([str(executable), '-I', '-c', stage, str(remote)], input=json.dumps(binding).encode(), check=True, timeout=30)
        result['A'] = asyncio.run(check_worker(remote, work, executable, native, enabled))
        embedded = root / 'embedded'
        spec = dict(source_dir=str(native), python_path=str(executable), bootstrap=(HERE / 'bootstrap.py').read_text(),
                    native_audit=(HERE / 'native_audit.py').read_text(), native_observation=False,
                    native_process_observation=False, model_proxy_url='http://127.0.0.1:9/v1', proxy_token='offline-fixture',
                    model='never-called', workdir=str(work))
        subprocess.run([str(executable), '-I', str(HERE / 'bootstrap.py'), 'stage', str(embedded)],
                       input=json.dumps(spec).encode(), check=True, timeout=30)
        with (root / 'embedded.log').open('wb') as log:
            subprocess.run([str(executable), '-I', str(Path(__file__)), '--root', str(root), '--native', str(native),
                            '--expected', args.expected, '--embedded-child'], env=minimal_env(embedded / 'home'),
                           user=60000, group=60000, extra_groups=[], stdout=log, stderr=subprocess.STDOUT,
                           check=True, timeout=120)
        result['B'] = json.loads((embedded / 'home/environment-result.json').read_text())
        result['status'] = 'PASS_NATIVE_PRESENTATION_ENTRANCES' if enabled else 'PASS_LEGACY_ENVIRONMENT_UNCHANGED'
    finally:
        (root / 'evidence.json').write_text(json.dumps(result, indent=2) + '\n')
    print(json.dumps(result))


if __name__ == '__main__':
    main()

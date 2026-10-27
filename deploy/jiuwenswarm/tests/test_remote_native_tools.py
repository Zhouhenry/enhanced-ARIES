"""Pinned native handler/Glob against a separate real guest worker; no model/cloud.

Run inside image4 plus the recorded Swarm overlay (or image5). ARIES_REMOTE_SOURCE
contains the production Python remote package. ARIES_GUEST_URL points at another
container. Only the upstream AgentManager/active-round collaborators are fixtures;
Glob, interrupt dispatch, native adapter routes, provider, worker and processes run.
"""
import asyncio
import importlib.util
import json
import logging
import os
from pathlib import Path
import sys
from types import SimpleNamespace
import unittest
from unittest.mock import AsyncMock, patch

import aiohttp
from aiohttp import web

remote = Path(os.environ['ARIES_REMOTE_SOURCE'])
spec = importlib.util.spec_from_file_location('aries_remote', remote / 'extension.py',
                                             submodule_search_locations=[str(remote)])
package = importlib.util.module_from_spec(spec)
sys.modules['aries_remote'] = package
# Provider-only fixture: extension loading is tested independently with image5.
from aries_remote import provider
from openjiuwen.core.runner.callback.framework import AsyncCallbackFramework
from openjiuwen.core.sys_operation import SysOperation, SysOperationCard, OperationMode
from openjiuwen.core.sys_operation.config import SandboxGatewayConfig, PreDeployLauncherConfig
from openjiuwen.core.sys_operation.cwd import init_cwd
from openjiuwen.core.sys_operation.sandbox.gateway.gateway import SandboxGateway
from openjiuwen.core.sys_operation.shell_process_registry import set_shell_session_id, reset_shell_session_id
from jiuwenswarm.agents.harness.flash.tools.flash_glob_tool import FlashGlobTool
from jiuwenswarm.common.schema.agent import AgentRequest, AgentResponse
from jiuwenswarm.common.schema.message import ReqMethod
from jiuwenswarm.extensions.hook_event import AgentServerHookEvents
from jiuwenswarm.extensions.registry import ExtensionRegistry
from jiuwenswarm.server.handlers import chat
from jiuwenswarm.server.runtime.agent_adapter.interface_deep import JiuWenSwarmDeepAdapter

TOKEN = 'controlled-fixture-token-not-a-credential'


class NativeRemoteTests(unittest.IsolatedAsyncioTestCase):
    async def asyncSetUp(self):
        self.sid = 'fixture-' + self._testMethodName
        self.work = Path('/workspace/aries-case') / self._testMethodName
        self.work.mkdir(parents=True, exist_ok=True)
        self.events = []
        self.fail_cancel = False
        self.cancel_gate = None
        self.cancel_entered = asyncio.Event()
        self.guest = os.environ['ARIES_GUEST_URL'].rstrip('/')
        self.http = aiohttp.ClientSession()

        async def forward(request):
            body = await request.json()
            self.events.append(('bridge', request.path, body))
            if request.path == '/cancel' and self.cancel_gate is not None:
                self.cancel_entered.set()
                await self.cancel_gate.wait()
            if request.path == '/cancel' and self.fail_cancel:
                return web.Response(status=503, text='controlled cancel failure')
            async with self.http.post(self.guest + request.path, json=body,
                    headers={'X-Aries-Worker-Token': TOKEN}) as response:
                return web.Response(status=response.status, body=await response.read(),
                                    content_type=response.content_type)
        app = web.Application(client_max_size=0)
        app.router.add_post('/call', forward)
        app.router.add_post('/cancel', forward)
        self.bridge = web.AppRunner(app)
        await self.bridge.setup()
        site = web.TCPSite(self.bridge, '127.0.0.1', 0)
        await site.start()
        base = 'http://127.0.0.1:' + str(site._server.sockets[0].getsockname()[1])
        provider._binding = None
        provider.configure({'schema_version': 1, 'binding_id': 'native-gaps',
            'run_id': 'native-gaps', 'task_id': 'native-gaps', 'workdir': str(self.work),
            'base_url': base, 'access_token': TOKEN})
        SandboxGateway._instance = None
        self.op = SysOperation(SysOperationCard(id=self.sid, mode=OperationMode.SANDBOX,
            gateway_config=SandboxGatewayConfig(launcher_config=PreDeployLauncherConfig(
                base_url=base, sandbox_type='aries', on_stop='keep'))))
        init_cwd('/workspace/aries-case')
        self.session_token = set_shell_session_id(self.sid)
        await self.command('mkdir -p ' + str(self.work))
        init_cwd(str(self.work))
        ExtensionRegistry.reset_instance()
        self.registry = ExtensionRegistry.create_instance(AsyncCallbackFramework(), {}, logging.getLogger(__name__))
        self.registry.register(AgentServerHookEvents.AFTER_SESSION_CANCEL, provider.cancel_session_tree)

    async def asyncTearDown(self):
        self.fail_cancel = False
        await provider.cancel_session_tree(SimpleNamespace(session_id=self.sid, params={'intent': 'cancel'}))
        await self.bridge.cleanup()
        await self.http.close()
        reset_shell_session_id(self.session_token)
        ExtensionRegistry.reset_instance()

    async def command(self, command):
        result = await self.op.shell().execute_cmd(command)
        self.assertEqual(result.code, 0, result.message)
        self.assertEqual(result.data.exit_code, 0, result.data.stderr)
        return result.data.stdout

    async def guest_alive(self, pid):
        # Actual guest procfs observation, not an interrupt acknowledgement.
        text = await self.command(f"if test -e /proc/{int(pid)}/stat; then cat /proc/{int(pid)}/stat; fi")
        return bool(text) and text.split(') ', 1)[1].split()[0] != 'Z'

    async def assert_stopped(self, pid):
        async with asyncio.timeout(10):
            while await self.guest_alive(pid):
                await asyncio.sleep(.05)

    async def background(self, sid):
        token = set_shell_session_id(sid)
        try:
            result = await self.op.shell().execute_cmd_background('sleep 120 & wait', grace=.02)
        finally:
            reset_shell_session_id(token)
        self.assertEqual(result.data.status, 'started')
        self.assertTrue(await self.guest_alive(result.data.pid))
        children = await self.command(f'cat /proc/{result.data.pid}/task/{result.data.pid}/children')
        self.assertTrue(children.strip(), 'background child must be live')
        return [result.data.pid, *map(int, children.split())]

    async def dispatch(self, intent='cancel', mode='idle', callback=None):
        request = AgentRequest(request_id='request-' + self.sid, channel_id='fixture',
            session_id=self.sid, req_method=ReqMethod.CHAT_CANCEL,
            params={'intent': intent, 'project_dir': str(self.work)})
        agent = None
        if mode == 'interaction':
            adapter = object.__new__(JiuWenSwarmDeepAdapter)
            adapter._is_session_scoped_adapter = True
            adapter._instance = SimpleNamespace(cancel_round=AsyncMock(return_value=False))
            adapter._instance_interaction_started = lambda: True
            adapter._get_goal_manager = lambda: None
            adapter._collect_cancelled_tools_for_session = lambda *a, **kw: []
            adapter._revoke_main_skill_authorization = lambda *a, **kw: None
            adapter._drop_pending_pure_ask_user_round = AsyncMock()
            adapter._clear_session_persisted_interrupt_state = AsyncMock()
            adapter._clear_pending_skill_turbo_hitl = AsyncMock()
            agent = SimpleNamespace(process_message=adapter.process_interrupt)
        elif mode == 'noninteraction':
            async def process(request):
                # Exercise the actual native non-interaction stop helper.
                adapter = object.__new__(JiuWenSwarmDeepAdapter)
                adapter._cancel_session_agent_tasks = AsyncMock(return_value=0)
                adapter._collect_cancelled_tools_for_session = lambda *a, **kw: []
                await adapter._stop_session_interrupt_work(request.session_id, intent=intent)
                return AgentResponse(request_id=request.request_id, channel_id=request.channel_id,
                                     payload={'event_type': 'chat.interrupt_result', 'success': True})
            agent = SimpleNamespace(process_message=process)
        wires = []
        async def send(wire):
            if callback is not None:
                await callback(wire)
            self.events.append(('ack', wire))
            wires.append(wire)
        manager = SimpleNamespace(get_agent_nowait=lambda *a, **kw: agent)
        ctx = SimpleNamespace(request=request, sink=SimpleNamespace(send_wire=send),
            services=SimpleNamespace(agent_manager=manager, session_stream_tasks={}))
        await chat.handle_chat_cancel_dispatch(ctx)
        self.assertEqual(len(wires), 1)
        return wires[0]

    async def test_glob_uses_guest_ignore_not_agent_file(self):
        (self.work / '.gitignore').write_text('keep.txt\n')
        await self.op.fs().write_file(str(self.work / '.gitignore'), b'# comment\nignored.txt\n\xff\n', mode='bytes')
        for name in ('keep.txt', 'ignored.txt', 'extra.txt'):
            await self.op.fs().write_file(str(self.work / name), 'content', prepend_newline=False)
        result = await FlashGlobTool(self.op, language='en').invoke({'pattern': '*.txt', 'path': str(self.work),
                                                                  'exclude_patterns': ['extra.txt']})
        self.assertTrue(result.success, result.error)
        self.assertEqual(result.data['filenames'], ['keep.txt'])
        self.assertTrue(result.data['gitignore_filtered'])
        reads = [e[2] for e in self.events if e[:2] == ('bridge', '/call') and
                 e[2]['operation'] == {'kind': 'fs', 'method': 'read_file'}]
        self.assertTrue(any(r['params'].get('path') == str(self.work / '.gitignore') and
                            r['params'].get('mode') == 'bytes' for r in reads))
        self.assertEqual((self.work / '.gitignore').read_text(), 'keep.txt\n')

    async def test_glob_missing_unreadable_ignore_and_sort_truncate(self):
        (self.work / '.gitignore').write_text('*.txt\n')
        await self.command("printf a > a.txt; printf b > b.txt; printf c > c.txt; "
                           "touch -t 202001010100 a.txt; touch -t 202001010200 b.txt; touch -t 202001010300 c.txt")
        tool = FlashGlobTool(self.op, language='en')
        tool.DEFAULT_MAX_RESULTS = 2
        result = await tool.invoke({'pattern': '*.txt', 'path': str(self.work)})
        self.assertEqual(result.data['filenames'], ['c.txt', 'b.txt'])
        self.assertTrue(result.data['truncated'])
        await self.op.fs().write_file(str(self.work / '.gitignore'), '*.txt', prepend_newline=False)
        await self.command('chmod 000 .gitignore')
        try:
            result = await tool.invoke({'pattern': '*.txt', 'path': str(self.work)})
            self.assertEqual(result.data['filenames'], ['c.txt', 'b.txt'])
            self.assertFalse(result.data['gitignore_filtered'])
        finally:
            await self.command('chmod 600 .gitignore')

    async def check_cancel(self, mode, intent='cancel'):
        target = await self.background(self.sid)
        child = await self.background(self.sid + '_subagent')
        other_sid = self.sid + '-other'
        other = await self.background(other_sid)
        try:
            async def before_ack(_):
                cancels = [e for e in self.events if e[:2] == ('bridge', '/cancel')]
                self.assertTrue(any(e[2].get('session_id') == self.sid for e in cancels))
            wire = await self.dispatch(intent=intent, mode=mode, callback=before_ack)
            self.assertNotIn('session cancellation extension failed', json.dumps(wire))
            for pid in target + child:
                await self.assert_stopped(pid)
            for pid in other:
                self.assertTrue(await self.guest_alive(pid), 'another native session was killed')
        finally:
            await provider.cancel_session_tree(SimpleNamespace(session_id=other_sid, params={'intent': 'cancel'}))
            for pid in other:
                await self.assert_stopped(pid)

    async def test_idle_cancel_actual_guest_parent_child_and_other_session(self):
        await self.check_cancel('idle')

    async def test_interaction_cancel_actual_guest(self):
        await self.check_cancel('interaction')

    async def test_noninteraction_supplement_actual_guest(self):
        await self.check_cancel('noninteraction', intent='supplement')

    async def test_pause_resume_do_not_cancel_guest(self):
        pids = await self.background(self.sid)
        for intent in ('pause', 'resume'):
            await self.dispatch(intent=intent)
            for pid in pids:
                self.assertTrue(await self.guest_alive(pid))
        self.assertFalse(any(e[:2] == ('bridge', '/cancel') for e in self.events))

    async def test_cancel_failure_ack_is_failure_guest_still_alive(self):
        pids = await self.background(self.sid)
        self.fail_cancel = True
        wire = await self.dispatch()
        self.assertIn('session cancellation extension failed', json.dumps(wire))
        for pid in pids:
            self.assertTrue(await self.guest_alive(pid))
        self.fail_cancel = False
        await provider.cancel_session_tree(SimpleNamespace(session_id=self.sid, params={'intent': 'cancel'}))
        for pid in pids:
            await self.assert_stopped(pid)

    async def test_interrupt_ack_waits_for_remote_cancel_completion(self):
        pids = await self.background(self.sid)
        self.cancel_gate = asyncio.Event()
        pending = asyncio.create_task(self.dispatch())
        async with asyncio.timeout(10):
            await self.cancel_entered.wait()
        self.assertFalse(pending.done())
        self.assertFalse(any(e[0] == 'ack' for e in self.events))
        for pid in pids:
            self.assertTrue(await self.guest_alive(pid))
        self.cancel_gate.set()
        await pending
        for pid in pids:
            await self.assert_stopped(pid)
        self.cancel_gate = None

    async def test_native_without_remote_hook_keeps_local_result(self):
        self.registry.unregister(AgentServerHookEvents.AFTER_SESSION_CANCEL, provider.cancel_session_tree)
        wire = await self.dispatch()
        self.assertNotIn('session cancellation extension failed', json.dumps(wire))
        self.assertFalse(any(e[:2] == ('bridge', '/cancel') for e in self.events))


if __name__ == '__main__':
    unittest.main(verbosity=2)

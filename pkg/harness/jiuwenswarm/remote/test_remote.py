"""Real pinned native operations behind the official SANDBOX gateway, no cloud/model."""
import asyncio
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import sys
import tempfile
import shutil
import socket
import subprocess
import signal
import unittest
from unittest import mock
from types import SimpleNamespace

ROOT = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('aries_remote', ROOT / 'extension.py',
                                             submodule_search_locations=[str(ROOT)])
package = importlib.util.module_from_spec(spec)
sys.modules['aries_remote'] = package
spec.loader.exec_module(package)
from aries_remote import provider
from aries_remote.identity import current_identity
sys.path.insert(0, str(ROOT))
from worker import Worker

import aiohttp
from aiohttp import web
from openjiuwen.core.sys_operation import SysOperation, SysOperationCard, OperationMode
from openjiuwen.core.sys_operation.config import SandboxGatewayConfig, PreDeployLauncherConfig
from openjiuwen.core.sys_operation.cwd import init_cwd
from openjiuwen.core.sys_operation.sandbox.gateway.gateway import SandboxGateway
from openjiuwen.core.sys_operation.shell_process_registry import set_shell_session_id, reset_shell_session_id


def alive(pid):
    stat = Path('/proc') / str(pid) / 'stat'
    try:
        return stat.read_text().split(') ', 1)[1].split()[0] != 'Z'
    except FileNotFoundError:
        return False


async def wait_for(check):
    async with asyncio.timeout(10):
        while not check():
            await asyncio.sleep(.01)


class RemoteTests(unittest.IsolatedAsyncioTestCase):
    async def asyncSetUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name)
        self.binding = {'schema_version': 1, 'binding_id': 'fixture-binding',
            'run_id': 'fixture-run', 'task_id': 'fixture-task',
            'workdir': str(self.root), 'home': str(self.root), 'worker_token': 'fixture-token',
            'evidence_path': str(self.root / 'events.jsonl'),
            'process_observation': getattr(self, 'process_observation', False)}
        if self.binding['process_observation']:
            self.binding['process_observation_files'] = {}
            for name in ('aries_process_observer', 'shell_operation'):
                module = importlib.util.find_spec('openjiuwen.core.sys_operation.local.' + name)
                if module is not None:
                    self.binding['process_observation_files'][module.origin] = hashlib.sha256(Path(module.origin).read_bytes()).hexdigest()
            if len(self.binding['process_observation_files']) != 2:
                self.binding['process_observation_files'] = {}
        self.worker = Worker(self.binding)
        @web.middleware
        async def bridge_headers(request, handler):
            # The Go bridge authenticates its client then adds the guest token.
            if request.headers.get('Authorization') == 'Bearer fixture-token':
                headers = request.headers.copy()
                headers['X-Aries-Worker-Token'] = 'fixture-token'
                request = request.clone(headers=headers)
            return await handler(request)
        app = self.worker.app()
        app.middlewares.append(bridge_headers)
        self.server = web.AppRunner(app, handler_cancellation=True)
        await self.server.setup()
        self.site = web.TCPSite(self.server, '127.0.0.1', 0)
        await self.site.start()
        self.url = 'http://127.0.0.1:' + str(self.site._server.sockets[0].getsockname()[1])
        provider._binding = None
        provider.configure({**self.binding, 'base_url': self.url, 'access_token': 'fixture-token'})
        SandboxGateway._instance = None
        self.op = SysOperation(SysOperationCard(id='fixture', mode=OperationMode.SANDBOX,
            gateway_config=SandboxGatewayConfig(launcher_config=PreDeployLauncherConfig(
                base_url=self.url, sandbox_type='aries', on_stop='keep'))))
        init_cwd(str(self.root))
        self.token = set_shell_session_id('native-session')

    async def asyncTearDown(self):
        await self.server.cleanup()
        self.assertFalse(self.worker.active)
        reset_shell_session_id(self.token)
        self.tmp.cleanup()

    async def test_official_fs_binary_text_errors_search_and_lists(self):
        fs = self.op.fs()
        data = b'\x00\xff\x80\n' * 100000
        written = await fs.write_file('binary.dat', data, mode='bytes')
        self.assertEqual(written.code, 0)
        result = await fs.read_file('binary.dat', mode='bytes')
        self.assertEqual(result.data.content, data)
        chunks = [x async for x in fs.read_file_stream('binary.dat', mode='bytes', chunk_size=8192)]
        self.assertEqual(b''.join(x.data.chunk_content for x in chunks if x.data), data)
        await fs.write_file('test.txt', 'α\nβ\nγ\n', prepend_newline=False)
        self.assertEqual((await fs.read_file('test.txt', line_range=(2, 2))).data.content, 'β\n')
        self.assertEqual((await fs.search_files('.', '*.txt')).data.total_matches, 1)
        self.assertEqual((await fs.list_files('.')).code, 0)
        self.assertEqual((await fs.list_directories('.')).code, 0)
        remote = await fs.read_file('missing')
        local = await self.worker.operations['fs'].read_file('missing')
        self.assertEqual(remote.model_dump(), local.model_dump())
        self.assertNotEqual(remote.code, 0)

    async def test_native_file_delivery_materializes_remote_bytes_and_never_sends_stale_copy(self):
        from jiuwenswarm.agents.harness.common.tools.send_file_to_user import (
            SendFileToolkit, clear_sent_files_for_session)
        from jiuwenswarm.server.agent_ws_server import AgentWebSocketServer

        destination = self.root / 'agent' / 'result.pptx'
        remote = self.root / 'sandbox-result.pptx'
        content = b'PK\x03\x04\x00\xff\x80\n' * 10000
        remote.write_bytes(content)
        local_fs = self.worker.operations['fs']

        async def sandbox_file(path, **kwargs):
            # Different files stand in for the same path in the two hosts.
            self.assertEqual(path, str(destination))
            async for result in local_fs.read_file_stream(str(remote), **kwargs):
                yield result

        self.worker.operations['fs'] = SimpleNamespace(read_file_stream=sandbox_file)
        push = mock.AsyncMock(return_value=1)
        sid = 'delivery-' + self.root.name
        toolkit = SendFileToolkit('delivery-request', sid, 'web')
        download = dict(size=len(content), mime_type='application/vnd.openxmlformats-officedocument.presentationml.presentation',
                        download_url='/fixture-download', download_token='fixture-token')
        try:
            with mock.patch.object(AgentWebSocketServer, 'get_instance', return_value=SimpleNamespace(send_push=push)), \
                    mock.patch.object(SendFileToolkit, '_should_use_obs_download', return_value=False), \
                    mock.patch('jiuwenswarm.common.file_transfer_config.get_file_transfer_config', return_value=SimpleNamespace(enabled=False)), \
                    mock.patch('jiuwenswarm.agents.harness.common.tools.web_file_download.build_file_download_info', return_value=download), \
                    mock.patch('jiuwenswarm.server.runtime.session.session_history.append_history_record'):
                self.assertFalse(destination.exists())
                result = await toolkit.send_file(json.dumps([str(destination)]))
                self.assertEqual(result, '成功发送 1 个文件')
                self.assertEqual(destination.read_bytes(), content)
                self.assertEqual(push.await_args.args[0]['payload']['files'][0]['path'], str(destination))
                remote.unlink()
                with self.assertRaises(OSError):
                    await toolkit.send_file([str(destination)])
                self.assertEqual(push.await_count, 1)
                self.assertFalse(list(destination.parent.glob('.aries-delivery-*')))
        finally:
            clear_sent_files_for_session(sid)

    async def test_shell_preserves_original_command_and_terminal(self):
        command = "printf 'native-out\\n'; printf 'native-err\\n' >&2; exit 7"
        result = await self.op.shell().execute_cmd(command)
        self.assertEqual(result.data.command, command)
        self.assertEqual(result.data.stdout, 'native-out\n')
        self.assertEqual(result.data.stderr, 'native-err\n')
        self.assertEqual(result.data.exit_code, 7)
        self.assertEqual(result.code, 0)  # Native LOCAL distinguishes operation success from exit status.

    async def test_stream_is_live_and_native_frames_survive(self):
        command = "printf 'first\\n'; while test ! -f release; do sleep 0.01; done; printf 'last\\n'; exit 3"
        chunks = []
        async with asyncio.timeout(10):
            async for chunk in self.op.shell().execute_cmd_stream(command):
                chunks.append(chunk)
                if chunk.data and 'first' in chunk.data.text:
                    (self.root / 'release').write_text('release')
        self.assertIn('first\n', ''.join(x.data.text for x in chunks if x.data))
        self.assertIn('last\n', ''.join(x.data.text for x in chunks if x.data))
        self.assertEqual(chunks[-1].data.exit_code, 3)

    async def test_background_native_launch_and_worker_shutdown(self):
        result = await self.op.shell().execute_cmd_background('sleep 60', grace=.01)
        self.assertEqual(result.data.status, 'started')
        self.assertIsNone(result.data.exit_code)
        self.assertTrue(alive(result.data.pid))
        await self.worker.shutdown(None)
        await wait_for(lambda: not alive(result.data.pid))

    async def test_cancel_foreground_keeps_cancelled_outcome_and_joins(self):
        pending = asyncio.create_task(self.op.shell().execute_cmd('echo $$ > child.pid; sleep 60'))
        await wait_for(lambda: (self.root / 'child.pid').exists())
        pid = int((self.root / 'child.pid').read_text())
        pending.cancel()
        with self.assertRaises(asyncio.CancelledError):
            await pending
        await wait_for(lambda: not alive(pid) and not self.worker.active)

    async def test_session_cancel_targets_actual_guest_background_registry(self):
        result = await self.op.shell().execute_cmd_background('sleep 60', grace=.01)
        async with aiohttp.ClientSession(headers={'Authorization': 'Bearer fixture-token'}) as client:
            async with client.post(self.url + '/cancel', json={
                'schema_version': 1, 'binding_id': 'fixture-binding',
                'scope': 'session_tree', 'session_id': 'native-session',
            }) as response:
                reply = await response.json()
                self.assertEqual(reply['invocations_found'], 1)
                self.assertEqual(reply['native_processes_signalled'], 1)
                self.assertEqual(reply['process_tree_absence'], 'unconfirmed_until_group_stop')
        await wait_for(lambda: not alive(result.data.pid))

    async def test_actual_same_name_flash_identity_parallel_and_unavailable(self):
        from jiuwenswarm.agents.harness.flash.tools.flash_bash_tool import FlashBashTool
        from openjiuwen.extensions.observability.span_context import (
            set_current_request_id, clear_current_request_id)
        tool = FlashBashTool(self.op, language='en')
        set_current_request_id('original-request')
        try:
            results = await asyncio.gather(*[tool.invoke(
                {'command': 'printf identity'}, tool_call_id=call)
                for call in ('original-call-1', 'original-call-2')])
            self.assertTrue(all(result.success for result in results))
            self.assertEqual(current_identity()['identity_status'], 'unavailable')
            await self.op.fs().list_files('.')
        finally:
            clear_current_request_id()
        rows = [json.loads(x) for x in (self.root / 'events.jsonl').read_text().splitlines()]
        starts = [x for x in rows if x['event'] == 'invocation_start']
        self.assertEqual({x['identity']['tool_call_id'] for x in starts},
                         {'original-call-1', 'original-call-2', None})
        self.assertTrue(all(x['identity']['request_id'] == 'original-request' for x in starts))
        self.assertTrue(all(x['identity']['session_id'] == 'native-session' for x in starts))
        self.assertEqual(len({x['invocation_id'] for x in starts}), len(starts))
        self.assertTrue(all('params' not in x and 'result' not in x for x in rows))

    async def test_same_lane_cancelled_flash_then_auxiliary_has_no_stale_identity(self):
        from jiuwenswarm.agents.harness.flash.tools.flash_bash_tool import FlashBashTool
        tool = FlashBashTool(self.op, language='en')
        with self.assertRaises(TimeoutError):
            async with asyncio.timeout(.5):
                await tool.invoke({'command': 'echo $$ > identity-cancel.pid; sleep 60'},
                                  tool_call_id='cancelled-native-tool')
        self.assertEqual(current_identity()['identity_status'], 'unavailable')
        await wait_for(lambda: not self.worker.active)
        pid = int((self.root / 'identity-cancel.pid').read_text())
        await wait_for(lambda: not alive(pid))
        await self.op.fs().list_files('.')
        result = await tool.invoke({'command': 'printf next'}, tool_call_id='next-native-tool')
        self.assertTrue(result.success)
        rows = [json.loads(x) for x in (self.root / 'events.jsonl').read_text().splitlines()]
        starts = [x for x in rows if x['event'] == 'invocation_start']
        self.assertEqual([x['identity']['tool_call_id'] for x in starts],
                         ['cancelled-native-tool', None, 'next-native-tool'])

    async def test_six_native_flash_tools_use_unmodified_renderers(self):
        from openjiuwen.harness.tools.filesystem import WriteFileTool, GrepTool
        from jiuwenswarm.agents.harness.flash.tools.flash_bash_tool import FlashBashTool
        from jiuwenswarm.agents.harness.flash.tools.flash_edit_tool import FlashEditFileTool
        from jiuwenswarm.agents.harness.flash.tools.flash_glob_tool import FlashGlobTool
        from jiuwenswarm.agents.harness.flash.tools.flash_read_tool import FlashReadFileTool
        path = str(self.root / 'flash.txt')
        cases = [
            (WriteFileTool, {'file_path': path, 'content': 'alpha beta\n'}),
            (FlashReadFileTool, {'file_path': path}),
            (FlashEditFileTool, {'file_path': path, 'old_string': 'beta', 'new_string': 'gamma'}),
            (FlashGlobTool, {'path': str(self.root), 'pattern': '*.txt'}),
            (GrepTool, {'path': str(self.root), 'pattern': 'gamma', 'output_mode': 'content'}),
            (FlashBashTool, {'command': 'cat flash.txt'}),
        ]
        for cls, inputs in cases:
            with self.subTest(tool=cls.__name__):
                result = await cls(self.op, language='en').invoke(inputs, tool_call_id=cls.__name__)
                self.assertTrue(result.success, result.error)
        self.assertIn('gamma', (self.root / 'flash.txt').read_text())
        rows = [json.loads(x) for x in (self.root / 'events.jsonl').read_text().splitlines()]
        starts = [x for x in rows if x['event'] == 'invocation_start']
        self.assertEqual({x['identity']['tool_call_id'] for x in starts},
                         {cls.__name__ for cls, _ in cases})
        self.assertTrue(all(x['identity']['identity_status'] == 'known' for x in starts))

    async def test_provider_native_sdk_parent_ids_phases_and_nonzero_result(self):
        from unittest.mock import patch
        from opentelemetry import trace
        from opentelemetry.sdk.trace import TracerProvider
        from opentelemetry.sdk.trace.export import SimpleSpanProcessor
        from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter
        tp = TracerProvider(shutdown_on_exit=False)
        exporter = InMemorySpanExporter()
        tp.add_span_processor(SimpleSpanProcessor(exporter))
        tracer = tp.get_tracer('fixture')
        with patch.object(provider, 'get_tracer', return_value=tracer):
            with tracer.start_as_current_span('original-parent') as parent:
                results = await asyncio.gather(*[self.op.shell().execute_cmd('printf exact; exit 7') for _ in range(2)])
                self.assertEqual(trace.get_current_span(), parent)
        self.assertTrue(all((r.data.stdout, r.data.exit_code) == ('exact', 7) for r in results))
        spans = [s for s in exporter.get_finished_spans() if s.name == 'aries.provider.request']
        starts = [json.loads(x) for x in (self.root / 'events.jsonl').read_text().splitlines() if json.loads(x)['event'] == 'invocation_start']
        self.assertEqual(len(spans), 2)
        observed = []
        for span in spans:
            self.assertEqual(span.parent.span_id, parent.get_span_context().span_id)
            data = json.loads(span.attributes['aries.provider.observation'])
            self.assertEqual(data['outcome'], 'returned')
            self.assertEqual([p['name'] for p in data['phases']], ['http_submit', 'response_headers', 'first_frame', 'terminal_frame'])
            offsets = [data['start_offset_ns']] + [p['offset_ns'] for p in data['phases']] + [data['end_offset_ns']]
            self.assertEqual(offsets, sorted(offsets))
            self.assertEqual(span.attributes['openjiuwen.session.id'], 'native-session')
            self.assertNotIn('printf', json.dumps(data))
            observed.append(data['invocation_id'])
        self.assertEqual(set(observed), {r['invocation_id'] for r in starts})
        self.assertEqual(len(set(observed)), 2)
        tp.shutdown()

    async def test_provider_stream_cancel_ends_span_and_preserves_current_context(self):
        from unittest.mock import patch
        from opentelemetry import trace
        from opentelemetry.sdk.trace import TracerProvider
        from opentelemetry.sdk.trace.export import SimpleSpanProcessor
        from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter
        tp = TracerProvider(shutdown_on_exit=False)
        exporter = InMemorySpanExporter(); tp.add_span_processor(SimpleSpanProcessor(exporter))
        tracer = tp.get_tracer('fixture')
        with patch.object(provider, 'get_tracer', return_value=tracer):
            with tracer.start_as_current_span('original-parent') as parent:
                async def consume():
                    async for result in self.op.shell().execute_cmd_stream('printf visible; sleep 60'):
                        self.assertEqual(trace.get_current_span(), parent)
                task = asyncio.create_task(consume())
                await wait_for(lambda: bool(self.worker.active))
                task.cancel()
                with self.assertRaises(asyncio.CancelledError): await task
                await wait_for(lambda: not self.worker.active)
        spans = [s for s in exporter.get_finished_spans() if s.name == 'aries.provider.request']
        self.assertEqual(len(spans), 1)
        data = json.loads(spans[0].attributes['aries.provider.observation'])
        self.assertEqual(data['outcome'], 'cancelled')
        self.assertGreaterEqual(data['end_offset_ns'], data['start_offset_ns'])
        tp.shutdown()

    async def test_native_extension_loader_registers_the_actual_staged_files(self):
        from jiuwenswarm.extensions.loader import ExtensionLoader
        from openjiuwen.core.sys_operation.sandbox.sandbox_registry import SandboxRegistry
        target = self.root / 'aries_remote_fixture'
        target.mkdir()
        for name in ('extension.py', 'provider.py', 'identity.py', 'wire.py'):
            shutil.copy2(ROOT / name, target / name)
        (target / 'binding.json').write_text(json.dumps({**self.binding,
            'base_url': self.url, 'access_token': 'fixture-token'}))
        import logging
        from openjiuwen.core.runner import Runner
        from jiuwenswarm.extensions.registry import ExtensionRegistry
        registry = ExtensionRegistry(Runner.callback_framework, {}, logging.getLogger('fixture'))
        await ExtensionLoader(registry).load_extension(target)
        cls = SandboxRegistry.get_provider_cls('aries', 'fs')
        self.assertEqual(cls.__module__, 'jiuwenswarm.loaded_extension.aries_remote_fixture.provider')
        SandboxGateway._instance = None
        operation = SysOperation(SysOperationCard(id='real-extension', mode=OperationMode.SANDBOX,
            gateway_config=SandboxGatewayConfig(launcher_config=PreDeployLauncherConfig(
                base_url=self.url, sandbox_type='aries', on_stop='keep'))))
        result = await operation.fs().write_file('extension.txt', 'loaded', prepend_newline=False)
        self.assertEqual(result.code, 0)
        self.assertEqual((self.root / 'extension.txt').read_text(), 'loaded')

    async def test_python_isolated_worker_entry_and_sigterm_cleanup(self):
        binding = self.root / 'worker-binding.json'
        binding.write_text(json.dumps(self.binding))
        with socket.socket() as sock:
            sock.bind(('127.0.0.1', 0))
            port = sock.getsockname()[1]
        with (self.root / 'worker.log').open('wb') as log:
            proc = subprocess.Popen([sys.executable, '-I', str(ROOT / 'worker.py'),
                '--binding', str(binding), '--listen', '127.0.0.1:' + str(port)],
                cwd=self.root, stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
            try:
                async with aiohttp.ClientSession(headers={'X-Aries-Worker-Token': 'fixture-token',
                                                         'Authorization': 'Bearer provider-access'}) as client:
                    async with asyncio.timeout(20):
                        while True:
                            if proc.poll() is not None:
                                self.fail('isolated worker exited before readiness')
                            try:
                                async with client.get(f'http://127.0.0.1:{port}/health') as response:
                                    health = await response.json()
                                    self.assertEqual(health['binding_id'], 'fixture-binding')
                                    break
                            except aiohttp.ClientConnectionError:
                                await asyncio.sleep(.05)
                    async with client.get(f'http://127.0.0.1:{port}/health',
                                          headers={'X-Aries-Worker-Token': '',
                                                   'Authorization': 'Bearer fixture-token'}) as response:
                        self.assertEqual(response.status, 401)
                proc.terminate()
                await asyncio.to_thread(proc.wait, 15)
                self.assertEqual(proc.returncode, 0)
            finally:
                if proc.poll() is None:
                    os.killpg(proc.pid, signal.SIGKILL)
                    await asyncio.to_thread(proc.wait, 10)

    async def test_worker_clock_and_terminal_receipt_are_independent_of_shutdown(self):
        result = await self.op.shell().execute_cmd('printf receipt')
        self.assertEqual(result.data.stdout, 'receipt')
        index = json.loads((self.root / 'worker-index.json').read_text())
        self.assertFalse(index['closing'])
        self.assertEqual((index['admitted'], index['terminal'], index['active']), (1, 1, 0))
        self.assertEqual((index['attempted'], index['written'], index['dropped'], index['write_failures']), (3, 3, 0, 0))
        clock = index['clock']
        self.assertLessEqual(clock['anchor_before_monotonic_ns'], clock['origin_monotonic_ns'])
        self.assertLessEqual(clock['origin_monotonic_ns'], clock['anchor_after_monotonic_ns'])
        rows = [json.loads(x) for x in (self.root / 'events.jsonl').read_text().splitlines()]
        self.assertEqual([r['sequence'] for r in rows], [1, 2, 3])
        self.assertEqual(rows[-1]['event'], 'invocation_terminal')
        self.assertTrue(all(r['clock_domain'] == clock['clock_domain'] for r in rows))
        self.assertGreaterEqual(rows[-1]['monotonic_ns'], rows[0]['monotonic_ns'])

    async def test_observer_drop_preserves_real_native_tool_result(self):
        self.worker.ledger.bytes = 64 * 1024 * 1024
        result = await self.op.shell().execute_cmd('printf unchanged; exit 7')
        self.assertEqual((result.data.stdout, result.data.exit_code), ('unchanged', 7))
        index = json.loads((self.root / 'worker-index.json').read_text())
        self.assertEqual((index['admitted'], index['terminal'], index['dropped']), (1, 1, 3))
        self.assertEqual(index['written'], 0)

    async def test_auth_binding_and_unknown_method_are_not_dispatched(self):
        async with aiohttp.ClientSession() as client:
            async with client.get(self.url + '/health') as response:
                self.assertEqual(response.status, 401)
            body = {'schema_version': 1, 'binding_id': 'other'}
            async with client.post(self.url + '/call', json=body,
                                   headers={'Authorization': 'Bearer fixture-token'}) as response:
                self.assertEqual(response.status, 409)
            body.update(binding_id='fixture-binding', invocation_id='unsupported',
                        identity={'run_id': 'fixture-run', 'task_id': 'fixture-task'},
                        operation={'kind': 'shell', 'method': 'arbitrary_method'})
            async with client.post(self.url + '/call', json=body,
                                   headers={'Authorization': 'Bearer fixture-token'}) as response:
                self.assertEqual(response.status, 400)
        self.assertFalse(self.worker.invocations)


if __name__ == '__main__':
    unittest.main()

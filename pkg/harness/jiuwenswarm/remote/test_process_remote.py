"""Run the native remote contract with process observation enabled, on ECS."""
import asyncio
import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock

import aiohttp
import test_remote


class ProcessRemoteTests(test_remote.RemoteTests):
    process_observation = True

    async def test_quiesce_preserves_original_identity_and_observed_exit(self):
        from jiuwenswarm.agents.harness.flash.tools.flash_bash_tool import FlashBashTool
        from openjiuwen.extensions.observability.span_context import (
            set_current_request_id, clear_current_request_id)
        tool = FlashBashTool(self.op, language='en')
        set_current_request_id('request-process')
        try:
            results = await asyncio.gather(*[
                tool.invoke({'command': 'printf process'}, tool_call_id=identity)
                for identity in ('process-call-1', 'process-call-2')])
            self.assertTrue(all(result.success for result in results))
        finally:
            clear_current_request_id()
        async with aiohttp.ClientSession(headers={'Authorization': 'Bearer fixture-token'}) as client:
            async with client.get(self.url + '/health') as response:
                health = await response.json()
                self.assertIs(health['process_observation_enabled'], True)
                self.assertEqual(health['process_observation_status'], 'enabled')
            async with client.post(self.url + '/quiesce', json={
                'schema_version': 1, 'binding_id': 'fixture-binding'}) as response:
                self.assertEqual(response.status, 200)
                receipt = await response.json()
                self.assertTrue(receipt['process_observation_complete'])
                self.assertEqual(receipt['process_tree_absence'], 'unconfirmed_until_group_stop')
        raw = (self.root / 'process-events.jsonl').read_bytes()
        index = json.loads((self.root / 'process-index.json').read_bytes())
        events = [json.loads(line) for line in raw.splitlines()]
        self.assertTrue(index['observer_closed'])
        self.assertEqual(index['ledger_sha256'], hashlib.sha256(raw).hexdigest())
        self.assertEqual(index['pending_processes'], 0)
        for name in ('dropped', 'write_failed', 'index_write_failed', 'late_events', 'identity_failed'):
            self.assertEqual(index['counters'][name], 0, name)
        self.assertEqual(index['counters']['started'], 2)
        self.assertEqual(index['counters']['exited'], 2)
        requests = [json.loads(line) for line in (self.root / 'events.jsonl').read_text().splitlines()]
        starts = {row['invocation_id']: row for row in requests if row['event'] == 'invocation_start'}
        for event in events:
            identity = event['identity']
            source = starts[identity['invocation_id']]['identity']
            self.assertTrue(all(identity[key] == source[key] for key in
                ('run_id', 'task_id', 'session_id', 'request_id', 'tool_call_id')))
            self.assertEqual(identity['session_id'], 'native-session')
            self.assertEqual(identity['request_id'], 'request-process')
        self.assertEqual({row['identity']['tool_call_id'] for row in events},
                         {'process-call-1', 'process-call-2'})
        self.assertEqual(len({row['process_id'] for row in events}), 2)
        self.assertTrue(self.worker.closing)


class UnsupportedProcessRemoteTests(unittest.IsolatedAsyncioTestCase):
    process_observation = True

    async def asyncSetUp(self):
        if importlib.util.find_spec('openjiuwen.core.sys_operation.local.aries_process_observer') is not None:
            self.skipTest('requires an older native image without the process hook')
        await test_remote.RemoteTests.asyncSetUp(self)

    async def asyncTearDown(self):
        await test_remote.RemoteTests.asyncTearDown(self)

    async def test_old_image_preserves_native_shell_and_reports_unsupported(self):
        result = await self.op.shell().execute_cmd("printf old-image-out; printf old-image-err >&2; exit 7")
        self.assertEqual(result.data.exit_code, 7)
        self.assertEqual(result.data.stdout, 'old-image-out')
        self.assertEqual(result.data.stderr, 'old-image-err')
        async with aiohttp.ClientSession(headers={'Authorization': 'Bearer fixture-token'}) as client:
            async with client.get(self.url + '/health') as response:
                health = await response.json()
        self.assertIs(health['process_observation_enabled'], False)
        self.assertEqual(health['process_observation_status'], 'unsupported_native_process_hook')
        self.assertIsNone(self.worker.process_observer)
        self.assertFalse((self.root / 'process-index.json').exists())
        self.assertFalse((self.root / 'process-events.jsonl').exists())


class InvalidProcessCapabilityTests(unittest.TestCase):
    def test_existing_module_error_is_not_unsupported(self):
        module = importlib.util.find_spec('openjiuwen.core.sys_operation.local.aries_process_observer')
        if module is None:
            self.skipTest('requires process-capable native image')
        from openjiuwen.core.sys_operation.local import aries_process_observer
        with tempfile.TemporaryDirectory() as directory:
            binding = {'run_id': 'run', 'task_id': 'task', 'workdir': directory,
                       'evidence_path': str(Path(directory) / 'worker-events.jsonl'),
                       'process_observation': True, 'process_observation_files': {}}
            for name in ('aries_process_observer', 'shell_operation'):
                spec = importlib.util.find_spec('openjiuwen.core.sys_operation.local.' + name)
                binding['process_observation_files'][spec.origin] = hashlib.sha256(Path(spec.origin).read_bytes()).hexdigest()
            with mock.patch.object(aries_process_observer, 'configure', side_effect=ImportError('broken observer dependency')):
                with self.assertRaisesRegex(ImportError, 'broken observer dependency'):
                    test_remote.Worker(binding)
            binding['process_observation_files'][module.origin] = '0' * 64
            with self.assertRaisesRegex(RuntimeError, 'observer changed'):
                test_remote.Worker(binding)
            binding['process_observation_files'] = {}
            with self.assertRaisesRegex(RuntimeError, 'lacks a verified creation hook'):
                test_remote.Worker(binding)


if __name__ == '__main__':
    unittest.main()

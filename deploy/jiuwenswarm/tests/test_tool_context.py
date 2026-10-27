"""Run in the fixed native runtime with agent-core-tool-context.patch applied."""
import asyncio
import unittest

from openjiuwen.core.foundation.tool.base import Tool, ToolCard, get_current_tool_call_id


class ProbeTool(Tool):
    def __init__(self, invoke=None, stream=None):
        super().__init__(ToolCard(id='shared-card-id', name='same-name', description='identity fixture'))
        self.invoke_body = invoke
        self.stream_body = stream

    async def invoke(self, inputs, **kwargs):
        return await self.invoke_body(inputs, **kwargs)

    async def stream(self, inputs, **kwargs):
        async for value in self.stream_body(inputs, **kwargs):
            yield value


class ToolContextTests(unittest.IsolatedAsyncioTestCase):
    async def asyncTearDown(self):
        self.assertIsNone(get_current_tool_call_id())

    async def test_same_name_parallel_calls_keep_original_ids_and_results(self):
        async def invoke(inputs, **kwargs):
            first = get_current_tool_call_id()
            await asyncio.sleep(0)
            self.assertEqual(get_current_tool_call_id(), first)
            self.assertEqual(first, kwargs['tool_call_id'])
            return inputs
        tool = ProbeTool(invoke)
        payloads = [b'original\x00\xff', {'original': ['\n', 3]}]
        results = await asyncio.gather(*[tool.invoke(value, tool_call_id=f'call-{i}')
                                        for i, value in enumerate(payloads)])
        self.assertEqual(results, payloads)

    async def test_nested_calls_restore_outer_and_missing_id_masks_outer(self):
        async def child(inputs, **kwargs):
            self.assertEqual(get_current_tool_call_id(), kwargs.get('tool_call_id'))
            return inputs
        inner = ProbeTool(child)
        async def parent(inputs, **kwargs):
            self.assertEqual(await inner.invoke('nested', tool_call_id='inner-call'), 'nested')
            self.assertEqual(get_current_tool_call_id(), 'outer-call')
            self.assertEqual(await inner.invoke('no-id'), 'no-id')
            self.assertEqual(get_current_tool_call_id(), 'outer-call')
            return inputs
        self.assertEqual(await ProbeTool(parent).invoke('native', tool_call_id='outer-call'), 'native')

    async def test_same_task_cancel_then_auxiliary_and_next_invocation_are_clean(self):
        async def invoke(inputs, **kwargs):
            if inputs == 'cancel':
                self.assertEqual(get_current_tool_call_id(), 'cancel-call')
                asyncio.current_task().cancel()
                await asyncio.sleep(0)
            return get_current_tool_call_id()
        tool = ProbeTool(invoke)
        with self.assertRaises(asyncio.CancelledError):
            await tool.invoke('cancel', tool_call_id='cancel-call')
        asyncio.current_task().uncancel()
        self.assertIsNone(get_current_tool_call_id())
        self.assertIsNone(await tool.invoke('without-id'))
        self.assertEqual(await tool.invoke('next', tool_call_id='next-call'), 'next-call')
        self.assertIsNone(get_current_tool_call_id())

    async def test_exception_identity_and_late_child_do_not_leak(self):
        error = ValueError('original error')
        release = asyncio.Event()
        late = None
        async def invoke(inputs, **kwargs):
            nonlocal late
            async def child():
                self.assertEqual(get_current_tool_call_id(), 'origin')
                await release.wait()
                return get_current_tool_call_id()
            late = asyncio.create_task(child())
            await asyncio.sleep(0)
            raise error
        try:
            await ProbeTool(invoke).invoke(None, tool_call_id='origin')
        except ValueError as caught:
            self.assertIs(caught, error)
        else:
            self.fail('original error lost')
        release.set()
        self.assertIsNone(await late)

    async def test_stream_identity_is_active_only_while_producing(self):
        async def stream(inputs, **kwargs):
            for value in ('first', b'\x00last'):
                self.assertEqual(get_current_tool_call_id(), 'stream-call')
                yield value
        values = []
        async for value in ProbeTool(stream=stream).stream(None, tool_call_id='stream-call'):
            self.assertIsNone(get_current_tool_call_id())
            values.append(value)
        self.assertEqual(values, ['first', b'\x00last'])

    async def test_stream_close_from_other_task_does_not_leak(self):
        release = asyncio.Event()
        late = None
        async def stream(inputs, **kwargs):
            nonlocal late
            async def child():
                await release.wait()
                return get_current_tool_call_id()
            late = asyncio.create_task(child())
            yield 'native first'
            yield 'native second'
        iterator = ProbeTool(stream=stream).stream(None, tool_call_id='close-call')
        self.assertEqual(await anext(iterator), 'native first')
        self.assertIsNone(get_current_tool_call_id())
        await asyncio.create_task(iterator.aclose())
        release.set()
        self.assertIsNone(await late)
        with self.assertRaises(StopAsyncIteration):
            await anext(iterator)

    async def test_cancel_stream_next_then_continue_same_task(self):
        async def stream(inputs, **kwargs):
            yield 'first'
            asyncio.current_task().cancel()
            await asyncio.sleep(0)
        iterator = ProbeTool(stream=stream).stream(None, tool_call_id='cancel-stream')
        self.assertEqual(await anext(iterator), 'first')
        with self.assertRaises(asyncio.CancelledError):
            await anext(iterator)
        asyncio.current_task().uncancel()
        self.assertIsNone(get_current_tool_call_id())
        await iterator.aclose()
        self.assertIsNone(get_current_tool_call_id())

    async def test_coroutine_stream_preserves_original_unsupported_exception(self):
        # Native RestfulApi/MCP stream methods are coroutines that raise, not generators.
        error = NotImplementedError('original unsupported stream')
        class NoStreamTool(ProbeTool):
            async def stream(self, inputs, **kwargs):
                self_id = get_current_tool_call_id()
                if self_id != 'unsupported-call':
                    raise AssertionError('missing coroutine identity')
                raise error
        with self.assertRaises(NotImplementedError) as caught:
            await NoStreamTool().stream(None, tool_call_id='unsupported-call')
        self.assertIs(caught.exception, error)

    async def test_nested_stream_yield_restores_consumer_identity(self):
        async def stream(inputs, **kwargs):
            self.assertEqual(get_current_tool_call_id(), 'inner-stream')
            yield 'original'
        async def parent(inputs, **kwargs):
            async for value in ProbeTool(stream=stream).stream(None, tool_call_id='inner-stream'):
                self.assertEqual(get_current_tool_call_id(), 'outer-tool')
                self.assertEqual(value, 'original')
            return inputs
        self.assertEqual(await ProbeTool(parent).invoke('done', tool_call_id='outer-tool'), 'done')


if __name__ == '__main__':
    unittest.main(verbosity=2)

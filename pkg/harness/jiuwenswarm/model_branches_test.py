"""Context isolation contract, run on ECS; no cloud or model calls."""
import asyncio
import importlib.util
import json
import logging
import os
from pathlib import Path
from types import SimpleNamespace
import unittest

spec = importlib.util.spec_from_file_location("model_branches", Path(__file__).with_name("model_branches.py"))
extension = importlib.util.module_from_spec(spec)
spec.loader.exec_module(extension)


async def observed_branch():
    context = SimpleNamespace(headers={"Authorization": "unchanged"}, extra_body={"unchanged": True})
    await extension.before_model_http(context)
    assert context.headers["Authorization"] == "unchanged" and context.extra_body == {"unchanged": True}
    return context.headers[extension.BRANCH_HEADER]


class BranchTests(unittest.IsolatedAsyncioTestCase):
    async def test_parallel_nested_exception_and_cancellation_keep_native_results(self):
        class TaskTool:
            async def invoke(self, inputs, **kwargs):
                await asyncio.sleep(0)
                if inputs == "nested":
                    return await self.invoke("value", tool_call_id="nested", untouched=kwargs["untouched"])
                if inputs == "error":
                    raise ValueError("original failure")
                if inputs == "cancel":
                    raise asyncio.CancelledError()
                return {"branch": await observed_branch(), "input": inputs, "kwargs": kwargs}

        extension.install_task_branch(TaskTool)
        tool = TaskTool()
        results = await asyncio.gather(tool.invoke("value", tool_call_id="one", untouched=42),
                                       tool.invoke("nested", tool_call_id="two", untouched=43))
        self.assertEqual([x["branch"] for x in results], ["main/one", "main/two/nested"])
        self.assertEqual([x["kwargs"]["untouched"] for x in results], [42, 43])
        self.assertEqual(await observed_branch(), "main")
        for kind, error in (("error", ValueError), ("cancel", asyncio.CancelledError)):
            with self.assertRaises(error):
                await tool.invoke(kind, tool_call_id="failed")
            self.assertEqual(await observed_branch(), "main")
        with self.assertRaises(RuntimeError):
            await tool.invoke("value")

    @unittest.skipUnless(os.getenv("ARIES_JIUWEN_NATIVE_FIXTURE") == "1", "requires pinned native image on ECS")
    async def test_pinned_task_tool_and_native_http_clients(self):
        import httpx
        from openai import AsyncOpenAI
        from jiuwenswarm.extensions.registry import ExtensionRegistry
        from jiuwenswarm.llm_sse_patch import apply_before_model_http_hook_patch
        from openjiuwen.core.runner.callback.framework import AsyncCallbackFramework
        from openjiuwen.core.foundation.llm.model_clients.openai_model_client import OpenAIModelClient
        from openjiuwen.core.foundation.llm.schema.config import ModelClientConfig, ModelRequestConfig
        from openjiuwen.core.session.agent import Session
        from openjiuwen.harness.tools.subagent.task_tool import create_task_tool

        observed = []

        def respond(request):
            body = json.loads(request.content)
            observed.append((request.headers[extension.BRANCH_HEADER], body))
            self.assertEqual(request.headers["authorization"], "Bearer fixture-key")
            message = {"role": "assistant", "content": "fixture response"}
            if body["stream"]:
                chunk = {"id": "fixture", "object": "chat.completion.chunk", "created": 1,
                         "model": "fixture-model", "choices": [{"index": 0, "delta": message, "finish_reason": "stop"}]}
                return httpx.Response(200, headers={"content-type": "text/event-stream"},
                                      content="data: " + json.dumps(chunk) + "\n\ndata: [DONE]\n\n")
            return httpx.Response(200, json={"id": "fixture", "object": "chat.completion", "created": 1,
                "model": "fixture-model", "choices": [{"index": 0, "message": message, "finish_reason": "stop"}],
                "usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}})

        ExtensionRegistry.reset_instance()
        registry = ExtensionRegistry.create_instance(AsyncCallbackFramework(), {}, logging.getLogger("branch-fixture"))
        from openjiuwen.harness.tools.subagent.task_tool import TaskTool
        original = TaskTool.invoke
        await extension.register_extensions(registry)
        apply_before_model_http_hook_patch()
        client = OpenAIModelClient(ModelRequestConfig(model="fixture-model"),
            ModelClientConfig(client_provider="OpenAI", api_key="fixture-key", api_base="https://fixture.invalid/v1"))
        sdk = AsyncOpenAI(api_key="fixture-key", base_url="https://fixture.invalid/v1",
                          http_client=httpx.AsyncClient(transport=httpx.MockTransport(respond)))
        client._create_async_openai_client = lambda **kwargs: sdk

        class Child:
            card = SimpleNamespace(id="child-fixture", name="child")

            async def invoke(self, inputs):
                answer = await client.invoke([{"role": "user", "content": inputs["query"]}])
                return {"output": answer.content}

        parent = SimpleNamespace(create_subagent=lambda *args, **kwargs: Child())
        tool = create_task_tool(parent, "general-purpose")[0]
        try:
            results = await asyncio.gather(*[tool.invoke(
                {"subagent_type": "general-purpose", "task_description": label},
                session=Session("fixture-root"), tool_call_id=label) for label in ("child-one", "child-two")])
            self.assertTrue(all(result.success and result.data["output"] == "fixture response" for result in results))
            self.assertEqual({branch for branch, body in observed}, {"main/child-one", "main/child-two"})
            self.assertTrue(all(body["stream"] is False and body["messages"] == [{"role": "user", "content": branch.split("/")[-1]}]
                                for branch, body in observed))
            chunks = [chunk async for chunk in client.stream([{"role": "user", "content": "root"}])]
            self.assertTrue(chunks)
            self.assertEqual(observed[-1][0], "main")
            self.assertIs(observed[-1][1]["stream"], True)
        finally:
            TaskTool.invoke = original
            ExtensionRegistry.reset_instance()
            await sdk.close()


if __name__ == "__main__":
    unittest.main()

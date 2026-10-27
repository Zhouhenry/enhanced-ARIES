"""Bind model history branches to native task_tool IDs; preserve native payloads."""
from contextvars import ContextVar
from functools import wraps
import re

BRANCH_HEADER = "X-Aries-Model-Branch"
_branch = ContextVar("aries_model_branch", default="main")


def install_task_branch(task_tool):
    original = task_tool.invoke

    @wraps(original)
    async def invoke(self, inputs, **kwargs):
        call_id = kwargs.get("tool_call_id")
        if not isinstance(call_id, str) or not re.fullmatch(r"[a-zA-Z0-9_.:-]+", call_id):
            raise RuntimeError("model branch requires the native task_tool call ID")
        token = _branch.set(_branch.get() + "/" + call_id)
        try:
            return await original(self, inputs, **kwargs)
        finally:
            _branch.reset(token)

    task_tool.invoke = invoke


async def before_model_http(context):
    context.headers[BRANCH_HEADER] = _branch.get()


async def register_extensions(registry):
    from jiuwenswarm.extensions.hook_event import AgentServerHookEvents
    from openjiuwen.harness.tools.subagent.task_tool import TaskTool
    install_task_branch(TaskTool)
    registry.register(AgentServerHookEvents.BEFORE_MODEL_HTTP, before_model_http)

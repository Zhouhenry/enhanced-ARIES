"""Exact native invocation context; neither ToolCard IDs nor span matching."""
from openjiuwen.core.foundation.tool.base import get_current_tool_call_id
from openjiuwen.core.sys_operation.shell_process_registry import resolve_shell_session_id
from openjiuwen.extensions.observability.span_context import get_current_request_id


def current_identity():
    tool_call_id = get_current_tool_call_id()
    return {'identity_status': 'known' if tool_call_id else 'unavailable',
            'tool_call_id': tool_call_id,
            'request_id': get_current_request_id() or None,
            'session_id': resolve_shell_session_id()}

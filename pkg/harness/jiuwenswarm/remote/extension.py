"""Native extension entry point; no agent loop or cloud lifecycle ownership."""
import json
from pathlib import Path

from jiuwenswarm.extensions.hook_event import AgentServerHookEvents

from .provider import configure, cancel_session_tree


async def register_extensions(registry):
    configure(json.loads(Path(__file__).with_name('binding.json').read_text()))
    registry.register(AgentServerHookEvents.AFTER_SESSION_CANCEL, cancel_session_tree)
    return None

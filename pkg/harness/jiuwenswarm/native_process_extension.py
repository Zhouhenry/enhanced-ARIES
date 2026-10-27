"""Observe LOCAL command processes through the pinned native creation hook."""
import json
import os
from pathlib import Path


class NativeProcessExtension:
    async def shutdown(self):
        from openjiuwen.core.sys_operation.local import aries_process_observer as observer
        from openjiuwen.core.sys_operation.shell_process_registry import kill_shell_processes_for_session

        # Stop owns cancellation. Preserve the native registry and let callbacks
        # report actual exits before the outer owner confirms cgroup emptiness.
        try:
            for session in observer.pending_sessions():
                kill_shell_processes_for_session(session)
            await observer.wait_for_completion(2)
        finally:
            observer.finalize()


async def register_extensions(registry):
    from openjiuwen.core.sys_operation.local import aries_process_observer as observer

    binding = json.loads(Path(__file__).with_name("binding.json").read_bytes())
    observer.configure(binding, Path(os.environ["JIUWENSWARM_DATA_DIR"]) / "aries-process/process-events.jsonl")
    return [NativeProcessExtension()]

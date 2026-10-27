"""Official SANDBOX providers forwarding native operations, without rendering tools."""
import asyncio
import functools
import json
import os
from pathlib import Path
import tempfile
import time
import uuid
from contextlib import suppress

import aiohttp
from openjiuwen.core.runner.callback.errors import AbortError
from openjiuwen.core.sys_operation.cwd import get_cwd
from openjiuwen.core.sys_operation.sandbox.providers.base_provider import BaseFSProvider, BaseShellProvider
from openjiuwen.core.sys_operation.sandbox.sandbox_registry import SandboxRegistry
from openjiuwen.core.sys_operation.sandbox.gateway.gateway import SandboxEndpoint
from openjiuwen.core.sys_operation.shell_process_registry import resolve_shell_session_id

from opentelemetry.trace import Status, StatusCode
from openjiuwen.extensions.observability.setup import get_tracer

from .identity import current_identity
from .wire import METHODS, PROTOCOL, encode_fields, native_result

_binding = None


_provider_clock = None


class _RequestObservation:
    """Use the native SDK/export audit; never attach a context across generator yields."""
    def __init__(self, identity, invocation, kind, method):
        self.identity, self.invocation = identity, invocation
        self.kind, self.method = kind, method
        self.span, self.data, self.failed = None, None, False

    def __enter__(self):
        global _provider_clock
        try:
            attributes = {'gen_ai.operation.name': 'aries_provider_request'}
            for source, target in [('session_id', 'openjiuwen.session.id'),
                                   ('request_id', 'openjiuwen.request.id'),
                                   ('tool_call_id', 'gen_ai.tool.call.id')]:
                if self.identity.get(source):
                    attributes[target] = self.identity[source]
            self.span = get_tracer('aries.jiuwen.remote').start_span(
                'aries.provider.request', attributes=attributes)
            if self.span.is_recording():
                if _provider_clock is None:
                    before = time.monotonic_ns()
                    unix = time.time_ns()
                    after = time.monotonic_ns()
                    emitter = uuid.uuid4().hex
                    _provider_clock = dict(emitter_id=emitter,
                        clock_domain='jiuwen-provider/' + emitter,
                        origin_monotonic_ns=(before + after) // 2, origin_unix_ns=unix,
                        anchor_before_monotonic_ns=before, anchor_after_monotonic_ns=after)
                self.data = dict(schema_version=1, invocation_id=self.invocation,
                    operation=dict(kind=self.kind, method=self.method), clock=_provider_clock,
                    start_offset_ns=time.monotonic_ns() - _provider_clock['origin_monotonic_ns'],
                    phases=[])
        except Exception:
            self.failed = True  # Missing or malformed spans fail evidence reconciliation, not the Tool.
        return self

    def mark(self, name):
        if self.data is not None:
            try:
                self.data['phases'].append(dict(name=name,
                    offset_ns=time.monotonic_ns() - self.data['clock']['origin_monotonic_ns']))
            except Exception:
                self.failed = True

    def __exit__(self, error_type, error, traceback):
        if self.span is None:
            return False
        try:
            if self.data is not None and not self.failed:
                self.data['end_offset_ns'] = time.monotonic_ns() - self.data['clock']['origin_monotonic_ns']
                self.data['outcome'] = ('returned' if error_type is None else
                    'cancelled' if issubclass(error_type, asyncio.CancelledError) else
                    'consumer_closed' if issubclass(error_type, GeneratorExit) else 'error')
                self.span.set_attribute('aries.provider.observation', json.dumps(self.data, separators=(',', ':')))
            self.span.set_status(Status(StatusCode.ERROR if error_type else StatusCode.OK))
        except Exception:
            pass  # The declared observer and accepted-call denominator expose missing evidence.
        finally:
            with suppress(Exception):
                self.span.end()
        return False


def configure(binding):
    global _binding
    if binding.get('schema_version') != PROTOCOL:
        raise ValueError('unsupported remote binding schema')
    for field in ('binding_id', 'run_id', 'task_id', 'base_url', 'access_token', 'workdir'):
        if not isinstance(binding.get(field), str) or not binding[field]:
            raise ValueError('missing remote binding field: ' + field)
    if _binding is not None and _binding != binding:
        raise ValueError('native process already bound to another attempt')
    _binding = dict(binding)
    SandboxRegistry.register_provider('aries', 'fs', AriesFSProvider)
    SandboxRegistry.register_provider('aries', 'shell', AriesShellProvider)
    install_remote_file_delivery()


def install_remote_file_delivery():
    """The native delivery tool reads local paths instead of its FS provider."""
    from jiuwenswarm.agents.harness.common.tools.send_file_to_user import SendFileToolkit
    original = SendFileToolkit.send_file
    if getattr(original, '_aries_remote_file_delivery', False):
        return

    @functools.wraps(original)
    async def send_file(self, abs_file_path_list, *args, **kwargs):
        paths = abs_file_path_list
        if isinstance(paths, str):
            try:
                parsed = json.loads(paths)
                paths = parsed if isinstance(parsed, (str, list)) else paths
            except (TypeError, ValueError):
                pass
        if not isinstance(paths, list):
            paths = [paths]
        workdir = Path(_binding['workdir']).resolve()
        fs = AriesFSProvider(SandboxEndpoint(base_url=_binding['base_url']))
        for item in paths:
            destination = Path(str(item).strip())
            if not destination.is_absolute() or not destination.resolve().is_relative_to(workdir):
                continue
            destination.parent.mkdir(parents=True, exist_ok=True)
            temporary = None
            try:
                with tempfile.NamedTemporaryFile(dir=destination.parent, prefix='.aries-delivery-', delete=False) as stream:
                    temporary = Path(stream.name)
                    total = 0
                    async for result in fs.read_file_stream(str(destination), mode='bytes', chunk_size=65536):
                        if result.code != 0:
                            raise OSError('sandbox delivery file could not be read')
                        if result.data is None:
                            continue
                        content = result.data.chunk_content
                        if not isinstance(content, bytes):
                            raise TypeError('sandbox delivery file was not returned as bytes')
                        total += len(content)
                        if total > 64 << 20:
                            raise ValueError('sandbox delivery file exceeds 64 MiB')
                        stream.write(content)
                os.replace(temporary, destination)
            finally:
                if temporary is not None:
                    temporary.unlink(missing_ok=True)
        return await original(self, abs_file_path_list, *args, **kwargs)

    send_file._aries_remote_file_delivery = True
    SendFileToolkit.send_file = send_file


class RemoteOperationError(RuntimeError):
    def __init__(self, error_type, message):
        self.remote_type = error_type
        super().__init__(message)


async def cancel_session_tree(context):
    """Forward the native cancel/supplement after host tasks have been joined."""
    if context.params.get('intent', 'cancel') not in ('cancel', 'supplement'):
        return
    try:
        sid = context.session_id
        if not isinstance(sid, str) or not sid:
            raise ValueError('session cancellation requires the native session id')
        if _binding is None:
            raise RuntimeError('aries remote provider is not configured')
        body = {'schema_version': PROTOCOL, 'binding_id': _binding['binding_id'],
                'scope': 'session_tree', 'session_id': sid}
        headers = {'Authorization': 'Bearer ' + _binding['access_token']}
        async with aiohttp.ClientSession(headers=headers, timeout=aiohttp.ClientTimeout(total=15)) as client:
            async with client.post(_binding['base_url'].rstrip('/') + '/cancel', json=body) as response:
                response.raise_for_status()
                result = await response.json()
        if (result.get('schema_version') != PROTOCOL or
                result.get('binding_id') != _binding['binding_id'] or
                result.get('active_calls_joined') is not True):
            raise ValueError('invalid remote session cancellation receipt')
        # Acknowledgement means native session cancellation, not cgroup absence.
    except Exception as exc:
        # The official callback dispatcher swallows ordinary errors; AbortError propagates.
        raise AbortError("remote session cancellation failed", cause=exc) from exc



class _Remote:
    kind = ''

    def __init__(self, endpoint, config=None):
        super().__init__(endpoint, config)
        if _binding is None:
            raise RuntimeError('aries remote provider is not configured')
        if endpoint.base_url.rstrip('/') != _binding['base_url'].rstrip('/'):
            raise ValueError('provider endpoint differs from attempt binding')
        self.binding = dict(_binding)

    async def _frames(self, method, params):
        invocation = uuid.uuid4().hex
        identity = current_identity()
        identity.update(run_id=self.binding['run_id'], task_id=self.binding['task_id'])
        if not identity.get('session_id'):
            identity['session_id'] = resolve_shell_session_id()
        params, binary = encode_fields(params, ('content',))
        body = {'schema_version': PROTOCOL, 'binding_id': self.binding['binding_id'],
                'invocation_id': invocation, 'identity': identity,
                'operation': {'kind': self.kind, 'method': method},
                'params': params, 'params_bytes': binary,
                'stream': method.endswith('_stream'), 'cwd': get_cwd()}
        base = self.binding['base_url'].rstrip('/')
        headers = {'Authorization': 'Bearer ' + self.binding['access_token']}
        with _RequestObservation(identity, invocation, self.kind, method) as observation:
            ended = False
            async with aiohttp.ClientSession(headers=headers, timeout=aiohttp.ClientTimeout(total=None)) as client:
                try:
                    observation.mark('http_submit')
                    async with client.post(base + '/call', json=body) as response:
                        observation.mark('response_headers')
                        response.raise_for_status()
                        pending = b''
                        first_frame = True
                        async for chunk in response.content.iter_any():
                            pending += chunk
                            while b'\n' in pending:
                                line, pending = pending.split(b'\n', 1)
                                frame = json.loads(line)
                                if frame.get('invocation_id') != invocation:
                                    raise ValueError('response invocation mismatch')
                                if first_frame:
                                    observation.mark('first_frame')
                                    first_frame = False
                                event = frame.get('event')
                                if event == 'end':
                                    observation.mark('terminal_frame')
                                    ended = True
                                    if pending.strip():
                                        raise ValueError('data after native end')
                                    return
                                if event == 'error':
                                    error = frame['error']
                                    raise RemoteOperationError(error['type'], error['message'])
                                if event != 'result':
                                    raise ValueError('unsupported native frame')
                                yield native_result(self.kind, method, frame)
                        raise EOFError('native operation ended without terminal frame')
                finally:
                    if not ended:
                        # No retries: an interrupted submission may already be executing.
                        async def cancel():
                            async with client.post(base + '/cancel', json={
                                'schema_version': PROTOCOL, 'binding_id': self.binding['binding_id'],
                                'invocation_id': invocation,
                            }, timeout=aiohttp.ClientTimeout(total=15)) as response:
                                response.raise_for_status()
                                await response.read()
                        task = asyncio.create_task(cancel())
                        try:
                            await asyncio.shield(task)
                        except BaseException:
                            with suppress(BaseException):
                                await task
                            # Containment/absence remains the bridge's Stop responsibility.

    async def _call(self, method, params):
        results = []
        async for result in self._frames(method, params):
            results.append(result)
        if len(results) != 1:
            raise ValueError('ordinary native call must return one result')
        return results[0]


class AriesFSProvider(_Remote, BaseFSProvider):
    kind = 'fs'

    async def read_file(self, path, **kwargs):
        return await self._call('read_file', dict(path=path, **kwargs))

    async def read_file_stream(self, path, **kwargs):
        async for item in self._frames('read_file_stream', dict(path=path, **kwargs)):
            yield item

    async def write_file(self, path, content, **kwargs):
        return await self._call('write_file', dict(path=path, content=content, **kwargs))

    async def list_files(self, path, **kwargs):
        return await self._call('list_files', dict(path=path, **kwargs))

    async def list_directories(self, path, **kwargs):
        return await self._call('list_directories', dict(path=path, **kwargs))

    async def search_files(self, path, pattern, **kwargs):
        return await self._call('search_files', dict(path=path, pattern=pattern, **kwargs))


class AriesShellProvider(_Remote, BaseShellProvider):
    kind = 'shell'

    async def execute_cmd(self, command, **kwargs):
        return await self._call('execute_cmd', dict(command=command, **kwargs))

    async def execute_cmd_stream(self, command, **kwargs):
        async for item in self._frames('execute_cmd_stream', dict(command=command, **kwargs)):
            yield item

    async def execute_cmd_background(self, command, **kwargs):
        return await self._call('execute_cmd_background', dict(command=command, **kwargs))

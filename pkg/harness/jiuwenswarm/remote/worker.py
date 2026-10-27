"""Run pinned LOCAL operations inside the Runner-owned guest process group."""
import argparse
import asyncio
import hmac
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import sys
import time
import uuid

# The harness invokes this trusted staged script with Python -I.
sys.path.insert(0, str(Path(__file__).resolve().parent))
from wire import METHODS, PROTOCOL, decode_fields, result_frame


class Ledger:
    def __init__(self, path):
        self.path = path
        self.bytes = 0
        self.failures = 0
        self.dropped = 0
        self.attempted = self.written = 0
        before = time.monotonic_ns()
        unix = time.time_ns()
        after = time.monotonic_ns()
        self.clock = {'emitter_id': uuid.uuid4().hex,
                      'origin_monotonic_ns': (before + after) // 2,
                      'origin_unix_ns': unix,
                      'anchor_before_monotonic_ns': before,
                      'anchor_after_monotonic_ns': after}
        self.clock['clock_domain'] = 'jiuwen-worker/' + self.clock['emitter_id']
        try:
            Path(path).touch(mode=0o600)
        except OSError:
            self.failures += 1

    def emit(self, event, request, **fields):
        self.attempted += 1
        row = {'schema_version': PROTOCOL, 'event': event,
               'sequence': self.attempted, 'clock_domain': self.clock['clock_domain'],
               'invocation_id': request.get('invocation_id'),
               'identity': request.get('identity'), 'operation': request.get('operation'),
               'monotonic_ns': time.monotonic_ns(), 'unix_ns': time.time_ns(), **fields}
        data = (json.dumps(row, separators=(',', ':')) + '\n').encode()
        if self.bytes + len(data) > 64 * 1024 * 1024:
            self.dropped += 1
            return
        try:
            with open(self.path, 'ab', buffering=0) as out:
                count = out.write(data)
            self.bytes += count
            if count != len(data):
                self.failures += 1
            else:
                self.written += 1
        except OSError:
            self.failures += 1


class Worker:
    def __init__(self, binding):
        from openjiuwen.core.sys_operation.base import OperationMode
        from openjiuwen.core.sys_operation.config import LocalWorkConfig
        from openjiuwen.core.sys_operation.local.fs_operation import FsOperation
        from openjiuwen.core.sys_operation.local.shell_operation import ShellOperation
        self.binding = binding
        self.process_observer = None
        self.process_observation_status = 'not_enabled'
        if binding.get('process_observation'):
            module = importlib.util.find_spec('openjiuwen.core.sys_operation.local.aries_process_observer')
            verified = binding.get('process_observation_files', {})
            if module is None:
                if verified:
                    raise RuntimeError('verified native process observer module is missing')
                self.process_observation_status = 'unsupported_native_process_hook'
            else:
                shell = importlib.util.find_spec('openjiuwen.core.sys_operation.local.shell_operation')
                if len(verified) != 2 or {module.origin, shell.origin} != set(verified):
                    raise RuntimeError('native process observer lacks a verified creation hook')
                for filename, digest in verified.items():
                    if hashlib.sha256(Path(filename).read_bytes()).hexdigest() != digest:
                        raise RuntimeError('verified native process observer changed')
                from openjiuwen.core.sys_operation.local import aries_process_observer
                aries_process_observer.configure(binding, Path(binding['evidence_path']).with_name('process-events.jsonl'))
                self.process_observer = aries_process_observer
                self.process_observation_status = 'enabled'
        # This is the same unrestricted LOCAL config used by the native harness.
        config = LocalWorkConfig(shell_allowlist=None, restrict_to_sandbox=False)
        self.operations = {
            'fs': FsOperation('fs', OperationMode.LOCAL, 'Native filesystem', config),
            'shell': ShellOperation('shell', OperationMode.LOCAL, 'Native shell', config),
        }
        self.active = {}
        self.invocations = {}
        self.ledger = Ledger(binding['evidence_path'])
        self.closing = False
        self.terminal = 0
        self.index_failures = 0
        self.checkpoint()

    def checkpoint(self):
        index = {'schema_version': 1, 'run_id': self.binding['run_id'],
                 'task_id': self.binding['task_id'], 'binding_id': self.binding['binding_id'],
                 'clock': self.ledger.clock, 'admitted': len(self.invocations),
                 'terminal': self.terminal, 'active': len(self.active), 'closing': self.closing,
                 'attempted': self.ledger.attempted, 'written': self.ledger.written,
                 'dropped': self.ledger.dropped, 'write_failures': self.ledger.failures,
                 'index_write_failures': self.index_failures,
                 'measurement_scope': 'worker_request_prepare_to_native_result_write_not_process'}
        path = Path(self.binding['evidence_path']).with_name('worker-index.json')
        temporary = path.with_suffix('.tmp')
        try:
            temporary.write_text(json.dumps(index, separators=(',', ':')) + '\n')
            temporary.replace(path)
        except OSError:
            self.index_failures += 1

    def app(self):
        from aiohttp import web
        app = web.Application(client_max_size=0)
        app.router.add_get('/health', self.health)
        app.router.add_post('/call', self.call)
        app.router.add_post('/cancel', self.cancel)
        app.router.add_post('/quiesce', self.quiesce)
        app.on_shutdown.append(self.shutdown)
        return app

    def authorize(self, request):
        from aiohttp import web
        expected = self.binding['worker_token']
        if not hmac.compare_digest(request.headers.get('X-Aries-Worker-Token', ''), expected):
            raise web.HTTPUnauthorized()

    def check_binding(self, body):
        from aiohttp import web
        if body.get('schema_version') != PROTOCOL or body.get('binding_id') != self.binding['binding_id']:
            raise web.HTTPConflict(text='binding mismatch')

    async def health(self, request):
        from aiohttp import web
        self.authorize(request)
        return web.json_response({'schema_version': PROTOCOL,
                                  'binding_id': self.binding['binding_id'],
                                  'closing': self.closing,
                                  'process_observation_enabled': self.process_observer is not None,
                                  'process_observation_status': self.process_observation_status,
                                  'observer_write_failures': self.ledger.failures,
                                  'observer_dropped': self.ledger.dropped})

    async def call(self, request):
        from aiohttp import web
        from openjiuwen.core.sys_operation.cwd import init_cwd
        from openjiuwen.core.sys_operation.shell_process_registry import (
            set_shell_session_id, reset_shell_session_id, kill_shell_processes_for_session,
        )
        self.authorize(request)
        body = await request.json()
        self.check_binding(body)
        identity = body.get('identity')
        if not isinstance(identity, dict) or any(
            identity.get(k) != self.binding[k] for k in ('run_id', 'task_id')
        ):
            raise web.HTTPConflict(text='attempt mismatch')
        invocation = body.get('invocation_id')
        if not isinstance(invocation, str) or not invocation or len(invocation) > 128:
            raise web.HTTPBadRequest(text='invocation required')
        if self.closing or invocation in self.invocations:
            raise web.HTTPConflict(text='closed or duplicate invocation')
        operation = body.get('operation', {})
        kind, method = operation.get('kind'), operation.get('method')
        if kind not in METHODS or method not in METHODS[kind]:
            raise web.HTTPBadRequest(text='unsupported native operation')
        streaming = method.endswith('_stream')
        if body.get('stream') is not streaming:
            raise web.HTTPBadRequest(text='stream mismatch')
        params = decode_fields(body.get('params', {}), body.get('params_bytes', []), ('content',))
        # Registry keys distinguish concurrent invocations; the native session is retained separately.
        registry_key = 'aries_' + invocation
        init_cwd(body.get('cwd') or self.binding['workdir'])
        self.invocations[invocation] = (registry_key, identity.get('session_id'))
        self.active[invocation] = asyncio.current_task()
        token = set_shell_session_id(registry_key)
        process_token = (self.process_observer.set_identity(dict(identity, invocation_id=invocation))
                         if self.process_observer is not None else None)
        response = web.StreamResponse(headers={'Content-Type': 'application/x-ndjson'})
        self.ledger.emit('invocation_start', body)
        self.checkpoint()
        outcome = 'unknown'
        try:
            await response.prepare(request)
            async def send(frame):
                await response.write((json.dumps(frame, ensure_ascii=False) + '\n').encode())
            handler = getattr(self.operations[kind], method)
            if streaming:
                async for result in handler(**params):
                    await send(result_frame(result, invocation))
            else:
                result = await handler(**params)
                await send(result_frame(result, invocation))
            # A background 'started' result is a launch result, never a process exit.
            fields = {'native_code': result.code} if 'result' in locals() else {}
            data = getattr(result, 'data', None) if 'result' in locals() else None
            for key in ('exit_code', 'pid', 'status'):
                value = getattr(data, key, None)
                if value is not None:
                    fields[key] = value
            self.ledger.emit('invocation_returned', body, **fields)
            await send({'event': 'end', 'invocation_id': invocation})
            outcome = 'returned'
            return response
        except asyncio.CancelledError:
            outcome = 'canceled'
            kill_shell_processes_for_session(registry_key)
            self.ledger.emit('invocation_cancelled', body)
            raise
        except (ConnectionError, ConnectionResetError):
            outcome = 'disconnected'
            kill_shell_processes_for_session(registry_key)
            self.ledger.emit('transport_disconnected', body)
            raise
        except Exception as exc:
            outcome = 'error'
            self.ledger.emit('invocation_error', body, error_type=type(exc).__name__)
            await response.write((json.dumps({'event': 'error', 'invocation_id': invocation,
                'error': {'type': type(exc).__name__, 'message': str(exc)}}) + '\n').encode())
            await response.write((json.dumps({'event': 'end', 'invocation_id': invocation}) + '\n').encode())
            return response
        finally:
            self.ledger.emit('invocation_terminal', body, outcome=outcome)
            self.active.pop(invocation, None)
            self.terminal += 1
            self.checkpoint()
            reset_shell_session_id(token)
            if process_token is not None:
                self.process_observer.reset_identity(process_token)

    async def cancel(self, request):
        from aiohttp import web
        self.authorize(request)
        body = await request.json()
        self.check_binding(body)
        scope = body.get('scope', 'invocation')
        if scope == 'session_tree':
            sid = body.get('session_id')
            if not isinstance(sid, str) or not sid:
                raise web.HTTPBadRequest(text='session required')
            ids = [key for key, (_, native_sid) in self.invocations.items()
                   if native_sid == sid or (isinstance(native_sid, str) and native_sid.startswith(sid + '_'))]
        elif scope == 'invocation':
            ids = [body.get('invocation_id')]
        else:
            raise web.HTTPBadRequest(text='unsupported cancellation scope')
        known, signalled = await self.cancel_ids(ids)
        return web.json_response({'schema_version': PROTOCOL, 'binding_id': self.binding['binding_id'],
            'invocations_found': known, 'native_processes_signalled': signalled,
            'active_calls_joined': True, 'process_tree_absence': 'unconfirmed_until_group_stop'})

    async def cancel_ids(self, ids):
        from openjiuwen.core.sys_operation.shell_process_registry import kill_shell_processes_for_session
        known = signalled = 0
        tasks = []
        for invocation in ids:
            entry = self.invocations.get(invocation)
            if entry is None:
                continue
            known += 1
            signalled += kill_shell_processes_for_session(entry[0])
            task = self.active.get(invocation)
            if task is not None and task is not asyncio.current_task():
                task.cancel()
                tasks.append(task)
        if tasks:
            await asyncio.gather(*tasks, return_exceptions=True)
        return known, signalled

    async def shutdown(self, app):
        self.closing = True
        await self.cancel_ids(list(self.invocations))
        await self.finish_process_observation()
        self.checkpoint()

    async def finish_process_observation(self):
        if self.process_observer is None:
            return None
        try:
            return await self.process_observer.wait_for_completion(2)
        finally:
            self.process_observer.finalize()

    async def quiesce(self, request):
        from aiohttp import web
        self.authorize(request)
        body = await request.json()
        self.check_binding(body)
        self.closing = True
        await self.cancel_ids(list(self.invocations))
        complete = await self.finish_process_observation()
        self.checkpoint()
        return web.json_response({'schema_version': PROTOCOL, 'binding_id': self.binding['binding_id'],
                                  'process_observation_complete': complete,
                                  'process_tree_absence': 'unconfirmed_until_group_stop'})


def main():
    from aiohttp import web
    parser = argparse.ArgumentParser()
    parser.add_argument('--binding', required=True)
    parser.add_argument('--listen', required=True)
    args = parser.parse_args()
    binding = json.loads(Path(args.binding).read_text())
    if binding['schema_version'] != PROTOCOL:
        raise ValueError('unsupported binding schema')
    os.environ['HOME'] = binding['home']
    # Restore the presentation image's native tools after contained launch.
    if Path('/opt/aries-presentation/node_modules').is_dir():
        os.environ['PATH'] = str(Path(sys.executable).parent) + os.pathsep + os.environ['PATH']
        os.environ['NODE_PATH'] = '/opt/aries-presentation/node_modules'
    os.chdir(binding['home'])
    host, port = args.listen.rsplit(':', 1)
    web.run_app(Worker(binding).app(), host=host, port=int(port),
                access_log=None, print=None, handler_cancellation=True)


if __name__ == '__main__':
    main()

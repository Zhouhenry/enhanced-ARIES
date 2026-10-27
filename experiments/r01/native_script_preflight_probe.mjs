// ECS-only real native validator and local child probe. No model, SSH or cloud.
import {r as createExecTool, t as createProcessTool} from '/app/dist/bash-tools-D7H1rgMh.js';
import {readFileSync, writeFileSync} from 'node:fs';
import assert from 'node:assert/strict';

assert.equal(process.platform, 'linux');
const candidate = process.argv[2] === 'candidate';
assert.ok(candidate || process.argv[2] === 'baseline');
const scope = JSON.parse(readFileSync('/run/aries/tool-identity-scope.json', 'utf8'));
const protocol = process.env.ARIES_REPLAY_TOOL_PROTOCOL === 'terminal-result-v1';
const backendFailure = new Error('fixture: buildExecSpec failed after script preflight');
let backendCalls = 0;
const sandbox = {containerWorkdir: '/tmp', workspaceDir: '/tmp', buildExecSpec: async ({command}) => {
  backendCalls++;
  if (command === 'python3 /tmp/aries-valid.py') throw backendFailure;
  return {argv: ['/bin/sh', '-c', command], env: process.env, stdinMode: 'pipe-closed'};
}};
const defaults = {host: 'sandbox', cwd: '/tmp', sessionKey: scope.agent_session_id,
  scopeKey: 'script-preflight-probe', allowBackground: true, notifyOnExit: false, sandbox};
const exec = createExecTool(defaults), processTool = createProcessTool({scopeKey: defaults.scopeKey});
const originalError = globalThis.Error;
const createdErrors = [], errors = [];
const keepAlive = setInterval(() => {}, 1000);
writeFileSync('/tmp/aries-shell-bleed.py', 'print($HOME)\n');
writeFileSync('/tmp/aries-shell-header.js', 'NODE\n');
writeFileSync('/tmp/aries-valid.py', 'print(1)\n');
try {
  // Capture native-created error objects without changing their prototypes.
  // Object identity proves the existing catch rethrows, rather than replacing.
  globalThis.Error = function (...args) {
    const error = new originalError(...args);
    if (String(args[0]).startsWith('exec preflight:')) createdErrors.push(error);
    return error;
  };
  globalThis.Error.prototype = originalError.prototype;
  for (const [id, command, message] of [
    ['complex-interpreter', "python3 <(printf 'print(1)')", 'exec preflight: complex interpreter invocation detected; refusing to run without script preflight validation. Use a direct `python <file>.py` or `node <file>.js` command.'],
    ['shell-bleed', 'python3 /tmp/aries-shell-bleed.py', 'exec preflight: detected likely shell variable injection ($HOME) in python script: aries-shell-bleed.py:1.'],
    ['shell-header', 'node /tmp/aries-shell-header.js', 'exec preflight: JS file starts with shell syntax (NODE). This looks like a shell command, not JavaScript.'],
  ]) {
    let caught;
    try { await exec.execute(id, {command, workdir: '/tmp'}); } catch (error) { caught = error; }
    assert.ok(caught, id);
    assert.equal(caught, createdErrors.at(-1), id + ': same native exception object');
    assert.equal(caught.name, 'Error');
    assert.ok(caught.message.startsWith(message), id);
    errors.push({id, name: caught.name, message: caught.message, same_error_object: true});
    assert.equal(backendCalls, 0, 'validator rejection must precede buildExecSpec');
  }
  globalThis.Error = originalError;
  let caught;
  try { await exec.execute('backend-failure', {command: 'python3 /tmp/aries-valid.py', workdir: '/tmp'}); }
  catch (error) { caught = error; }
  assert.equal(caught, backendFailure, 'post-validator backend error must be unchanged');
  assert.equal(backendCalls, 1);
  const normal = await exec.execute('foreground', {command: 'printf normal', workdir: '/tmp', timeout: 5});
  assert.equal(normal.details.status, 'completed'); assert.equal(normal.details.exitCode, 0);
  assert.equal(normal.content[0].text, 'normal');
  const background = await exec.execute('background', {command: 'sleep 0.05; printf background', background: true, workdir: '/tmp', timeout: 5});
  assert.equal(background.details.status, protocol ? 'accepted' : 'running');
  let backgroundFinal;
  do {
    backgroundFinal = await processTool.execute('background-wait', {action: protocol ? 'wait' : 'poll', sessionId: background.details.sessionId, timeout: 1000});
  } while (backgroundFinal.details.status === 'running');
  assert.equal(backgroundFinal.details.exitCode, 0);
  if (protocol) assert.equal(backgroundFinal.content[0].text, 'background');
  else assert.equal(backgroundFinal.details.aggregated, 'background');
  const direct = createExecTool({host: 'gateway', security: 'full', ask: 'off', cwd: '/tmp', allowBackground: true, notifyOnExit: false});
  const pty = await direct.execute('pty', {command: 'printf pty', pty: true, timeout: 5});
  assert.equal(pty.details.status, 'completed'); assert.equal(pty.details.exitCode, 0);
  assert.equal(pty.content[0].text, 'pty');
  assert.equal(backendCalls, 3);
  const records = readFileSync('/tmp/aries-native-tool-identities.jsonl', 'utf8').trimEnd().split('\n').map(JSON.parse);
  const starts = records.filter(row => row.phase === 'invocation');
  const rejected = records.filter(row => row.phase === 'pre_dispatch');
  assert.equal(starts.length, 6);
  assert.equal(new Set(starts.map(row => row.native_invocation_id)).size, 6);
  assert.equal(rejected.length, candidate ? 3 : 0);
  for (const row of rejected) {
    assert.equal(row.reason, 'shell_preflight_rejected'); assert.equal(row.dispatched, false);
    assert.equal(row.outcome, 'rejected');
    const start = starts.find(value => value.native_invocation_id === row.native_invocation_id);
    assert.ok(errors.some(value => value.id === row.tool_call_id));
    assert.equal(start.tool_call_id, row.tool_call_id);
    assert.equal(start.scope_id, row.scope_id); assert.equal(start.agent_session_id, row.agent_session_id);
  }
  assert.ok(!rejected.some(row => ['backend-failure', 'foreground', 'background', 'pty'].includes(row.tool_call_id)));
  const health = JSON.parse(readFileSync('/tmp/aries-native-tool-identities.status.json', 'utf8'));
  assert.deepEqual([health.attempted, health.written, health.write_failed, health.failed_latched], [records.length, records.length, 0, false]);
  console.log(JSON.stringify({status: candidate ? 'PASS_SCRIPT_PREFLIGHT_REJECTION' : 'REPRODUCED_SCRIPT_PREFLIGHT_OBSERVATION_GAP',
    protocol, errors, backend_calls: backendCalls, same_backend_error_object: true,
    normal, background_status: background.details.status, background_final: backgroundFinal, pty,
    records, health, limits: ['local native child backend; no SSH, Bridge, model or cloud', 'other pre-dispatch paths remain outside this change']}));
} finally {
  globalThis.Error = originalError;
  clearInterval(keepAlive);
}

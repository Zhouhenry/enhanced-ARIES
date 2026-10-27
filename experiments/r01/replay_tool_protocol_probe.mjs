// ECS: real pinned native exec/process tools -> SSH -> isolated Bridge -> real guest.
import {r as createExecTool, t as createProcessTool} from '/app/dist/bash-tools-D7H1rgMh.js';
import {c as backendFactory} from '/app/dist/browser-bridges-Be07JHOW.js';
import {s as getSession} from '/app/dist/bash-process-registry-17q1dHVV.js';
import {resolveReplaySession} from '/app/dist/aries-replay-tool-protocol.mjs';
import {readFileSync} from 'node:fs';
import assert from 'node:assert/strict';

const keepAlive = setInterval(() => {}, 1000);
const cfg = JSON.parse(readFileSync('/run/aries/probe.json', 'utf8'));
const backend = await backendFactory('ssh')({cfg: {docker: {env: {HOME: '/tmp'}}, workspaceAccess: 'rw',
  ssh: {target: 'aries@' + cfg.address, command: '/opt/aries/bin/aries-ssh-client', workspaceRoot: '/aries/openclaw',
    strictHostKeyChecking: false, updateHostKeys: false}}, scopeKey: 'shared', workspaceDir: '/tmp', agentWorkspaceDir: '/tmp'});
const defaults = {host: 'sandbox', sandbox: {...backend, containerWorkdir: backend.workdir, workspaceDir: '/tmp'},
  cwd: '/tmp', sessionKey: cfg.agent_session_id, scopeKey: 'replay-protocol', allowBackground: true, notifyOnExit: true};
const exec = createExecTool(defaults), proc = createProcessTool({scopeKey: defaults.scopeKey});
const results = [];
const start = async (id, command, options = {}, tool = exec) => {
  const result = await tool.execute(id, {command, workdir: backend.workdir, background: true, timeout: 10, ...options});
  assert.equal(result.details.status, 'accepted', id);
  assert.ok(!Object.hasOwn(result.details, 'pid'), id);
  assert.match(result.details.sessionId, /^job-/);
  results.push({id, start: result});
  return result.details.sessionId;
};
const wait = (id, handle, signal, timeout) => proc.execute(id, {action: 'wait', sessionId: handle, ...(timeout === undefined ? {} : {timeout})}, signal);
try {
  const normal = await start('ordered', 'printf err >&2; sleep 0.08; printf out; exit 7');
  const normalResult = await wait('ordered-wait', normal);
  assert.equal(normalResult.details.exitCode, 7);
  assert.equal(normalResult.details.status, 'completed'); // native completed-process meaning, not task success
  assert.equal(normalResult.content[0].text, 'outerr\n\n(Command exited with code 7)');
  assert.equal(normalResult.details.outputRenderer, 'stdout-stderr-v1');
  assert.deepEqual(await wait('ordered-wait', normal), normalResult);
  // Legacy poll/log are still real native views, and do not consume the stable final view.
  const poll = await proc.execute('ordered-poll', {action: 'poll', sessionId: normal, timeout: 1000});
  assert.equal(poll.details.exitCode, 7);
  assert.equal(poll.details.aggregated, 'errout');
  const log = await proc.execute('ordered-log', {action: 'log', sessionId: normal});
  assert.equal(log.content[0].text, 'errout');
  assert.deepEqual(await wait('ordered-after-log', normal), normalResult);
  results.at(-1).final = normalResult;

  const fast = await start('fast', 'printf fast');
  assert.equal((await wait('fast-wait', fast)).content[0].text, 'fast');
  let releaseYield;
  const yieldGate = new Promise(resolve => {releaseYield = resolve;});
  const yieldingTool = createExecTool({...defaults, sandbox: {...defaults.sandbox, finalizeExec: () => yieldGate}});
  const yielded = await start('auto-yield', 'printf yielded', {background: false, yieldMs: 10}, yieldingTool);
  releaseYield();
  assert.equal((await wait('auto-yield-wait', yielded)).content[0].text, 'yielded');
  const failed = await start('exit127', 'printf failure >&2; exit 127');
  const failure = await wait('exit127-wait', failed);
  assert.equal(failure.details.status, 'failed'); assert.equal(failure.details.exitCode, 127);

  const [one, two] = await Promise.all([start('parallel-one', 'sleep 0.2; printf one'), start('parallel-two', 'printf two')]);
  assert.notEqual(one, two);
  assert.deepEqual((await Promise.all([wait('one-wait', one), wait('two-wait', two)])).map(r => r.content[0].text), ['one','two']);
  const retry1 = await start('same-call', 'printf first'), retry2 = await start('same-call', 'printf second');
  assert.notEqual(retry1, retry2);
  assert.deepEqual((await Promise.all([wait('retry1', retry1), wait('retry2', retry2)])).map(r => r.content[0].text), ['first','second']);

  let finalizeFinished = false;
  const slowFinalize = createExecTool({...defaults, sandbox: {...defaults.sandbox, finalizeExec: async () => {
    await new Promise(resolve => setTimeout(resolve, 300)); finalizeFinished = true;
  }}});
  const finalized = await start('slow-finalize', 'printf finalized', {}, slowFinalize);
  const finalizedResult = await wait('slow-finalize-wait', finalized);
  assert.equal(finalizeFinished, true); assert.equal(finalizedResult.content[0].text, 'finalized');
  const failedFinalize = createExecTool({...defaults, sandbox: {...defaults.sandbox, finalizeExec: async () => {throw new Error('injected-finalize-failure');}}});
  const failedFinalizeJob = await start('failed-finalize', 'printf done', {}, failedFinalize);
  const finalizedFailure = await wait('failed-finalize-wait', failedFinalizeJob);
  assert.equal(finalizedFailure.details.status, 'failed');
  assert.match(finalizedFailure.content[0].text, /injected-finalize-failure/);

  const drain = await start('pipe-drain', 'printf before; (sleep 0.2; printf after) &');
  assert.equal((await wait('pipe-drain-wait', drain)).content[0].text, 'beforeafter');
  const cap = await start('cap', 'python3 -c \'import os; os.write(2,b"err"); os.write(1,b"o"*201000)\'');
  const capResult = await wait('cap-wait', cap);
  assert.equal(capResult.details.outputTruncated, true);
  assert.equal(capResult.content[0].text, 'o'.repeat(199997) + 'err');

  const canceled = await start('wait-cancel', 'sleep 0.35; printf survived-wait-cancel');
  const controller = new AbortController();
  const canceledWait = wait('wait-cancel-wait', canceled, controller.signal);
  controller.abort(new Error('cancel-only-wait'));
  await assert.rejects(canceledWait, /cancel-only-wait/);
  await assert.rejects(wait('wait-deadline', canceled, undefined, 5), /Wait timed out/);
  assert.equal((await wait('wait-cancel-final', canceled)).content[0].text, 'survived-wait-cancel');
  const kill = await start('kill', 'printf ready; sleep 5');
  assert.equal(getSession(resolveReplaySession(kill, defaults.scopeKey)).notifyOnExit, false);
  const killReady = await proc.execute('kill-ready', {action: 'poll', sessionId: kill, timeout: 1000});
  assert.match(killReady.content.map(item => item.text ?? '').join('\n'), /ready/);
  await proc.execute('kill-control', {action: 'kill', sessionId: kill});
  assert.equal((await wait('kill-final', kill)).details.status, 'failed');
  const timeout = await start('timeout', 'printf ready; sleep 5', {timeout: 1});
  const timedOut = await wait('timeout-wait', timeout);
  assert.equal(timedOut.details.status, 'failed'); assert.equal(timedOut.details.timedOut, true);
  const env = await start('controller-env', 'python3 -c \'import os; print("ARIES_REPLAY_TOOL_PROTOCOL" in os.environ)\'', {env: {ARIES_REPLAY_TOOL_PROTOCOL: 'must-not-reach-guest'}});
  assert.equal((await wait('controller-env-wait', env)).content[0].text, 'False');

  const raw = readFileSync('/tmp/aries-replay-tool-protocol.jsonl');
  const records = raw.toString().trim().split('\n').map(JSON.parse);
  const accepted = records.filter(row => row.event === 'accepted'), terminal = records.filter(row => row.event === 'terminal_result');
  assert.equal(accepted.length, results.length);
  assert.equal(terminal.length, results.length);
  assert.deepEqual(records.map(row => row.sequence), records.map((_,i) => i+1));
  assert.equal(new Set(records.map(row => row.emitter_id)).size, 1);
  assert.equal(new Set(records.map(row => row.clock_id)).size, 1);
  assert.equal(new Set(records.map(row => row.clock_origin_monotonic_ns)).size, 1);
  assert.ok(records.every(row => row.clock_domain === 'native_replay_tool_monotonic' &&
    row.emitter_pid === process.pid && row.previous_emit_failed === false &&
    BigInt(row.observed_monotonic_ns) >= BigInt(row.clock_origin_monotonic_ns)));
  assert.ok(records.every((row, index) => index === 0 ||
    BigInt(row.observed_monotonic_ns) >= BigInt(records[index - 1].observed_monotonic_ns)));
  assert.ok(accepted.every(row => row.native_session_id && Number.isInteger(row.native_pid)));
  assert.ok(records.filter(row => row.event.startsWith('wait_')).every(row => typeof row.wait_tool_call_id === 'string' && row.wait_tool_call_id));
  const starts = records.filter(row => row.event === 'wait_started');
  const ends = records.filter(row => row.event === 'wait_finished');
  assert.equal(new Set(starts.map(row => row.wait_invocation_id)).size, starts.length);
  assert.equal(starts.filter(row => row.wait_tool_call_id === 'ordered-wait').length, 2);
  assert.equal(ends.length, starts.length);
  for (const started of starts) {
    const finished = ends.filter(row => row.wait_invocation_id === started.wait_invocation_id);
    assert.equal(finished.length, 1);
    assert.equal(finished[0].wait_tool_call_id, started.wait_tool_call_id);
    assert.equal(finished[0].logical_handle, started.logical_handle);
    assert.deepEqual(finished[0].identity, started.identity);
  }
  // These assertions cover this probe's actual calls, not arbitrary Agent trace coverage.
  console.log(JSON.stringify({schema_version: 1, status: 'PASS_NATIVE_REPLAY_TOOL_PROTOCOL_PROBE',
    actual_exec_invocations: results.length, accepted: accepted.length, terminal_results: terminal.length,
    real_native_wait_and_finalize: true, original_poll_log_retained: true, native_control_kill_retained: true,
    strict_matcher_changed: false, cloud_created: 0, model_calls: 0,
    results, evidence_base64: raw.toString('base64')}));
} finally { clearInterval(keepAlive); }

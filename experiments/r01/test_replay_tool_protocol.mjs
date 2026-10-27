// Run with node --test inside the isolated ECS candidate image.
import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {registerReplayJob, acceptedReplayJob, resolveReplaySession, waitReplayJob, replayToolProtocolEnabled} from '/app/dist/aries-replay-tool-protocol.mjs';

let invocation = 0;
const identity = id => ({schema_version: 1, scope_id: 'unit', agent_session_id: 'session', tool_call_id: id, native_invocation_id: `invocation-${++invocation}`});
const deferred = () => { let resolve, reject; const promise = new Promise((a, b) => {resolve = a; reject = b;}); return {promise, resolve, reject}; };
const render = outcome => ({content: [{type: 'text', text: outcome.text}], details: {status: outcome.status, exitCode: outcome.exitCode}});
function job(scope, id, pending = deferred()) {
  const run = {session: {id: `random-native-${invocation}`, pid: 100 + invocation}, promise: pending.promise};
  return {entry: registerReplayJob(run, identity(id), scope, render), pending};
}

test('activation is exact and does not infer from a native job', () => {
  const original = process.env.ARIES_REPLAY_TOOL_PROTOCOL;
  delete process.env.ARIES_REPLAY_TOOL_PROTOCOL;
  assert.equal(replayToolProtocolEnabled(), false);
  process.env.ARIES_REPLAY_TOOL_PROTOCOL = 'terminal-result-v1';
  assert.equal(replayToolProtocolEnabled(), true);
  process.env.ARIES_REPLAY_TOOL_PROTOCOL = 'terminal-result-v2';
  assert.equal(replayToolProtocolEnabled(), false);
  if (original === undefined) delete process.env.ARIES_REPLAY_TOOL_PROTOCOL; else process.env.ARIES_REPLAY_TOOL_PROTOCOL = original;
});

test('logical handle is independent of random native identifiers and retains real final result', async () => {
  const a = job('run-a', 'call_1'), b = job('run-b', 'call_1');
  assert.equal(a.entry.handle, b.entry.handle);
  assert.notEqual(a.entry.identity.native_invocation_id, b.entry.identity.native_invocation_id);
  assert.equal(acceptedReplayJob(a.entry).details.status, 'accepted');
  assert.ok(!acceptedReplayJob(a.entry).content[0].text.includes('random-native'));
  a.pending.resolve({text: 'real-output', status: 'completed', exitCode: 7});
  b.pending.resolve({text: 'different-real-output', status: 'completed', exitCode: 7});
  const result = await waitReplayJob(a.entry.handle, 'run-a');
  assert.equal(result.content[0].text, 'real-output');
  assert.equal(result.details.exitCode, 7);
  assert.deepEqual(await waitReplayJob(a.entry.handle, 'run-a'), result);
  assert.equal((await waitReplayJob(b.entry.handle, 'run-b')).content[0].text, 'different-real-output');
});

test('concurrent jobs, repeated invocation identity and scope lookup stay distinct', async () => {
  const a = job('concurrent', 'same'), b = job('concurrent', 'same'), c = job('concurrent', 'other');
  assert.equal(new Set([a.entry.handle, b.entry.handle, c.entry.handle]).size, 3);
  assert.equal(resolveReplaySession(a.entry.handle, 'concurrent'), a.entry.run.session.id);
  assert.equal(resolveReplaySession(a.entry.handle, 'wrong'), a.entry.handle);
  assert.equal((await waitReplayJob(a.entry.handle, 'wrong')).details.status, 'failed');
  for (const item of [a,b,c]) item.pending.resolve({text: item.entry.handle, status: 'completed', exitCode: 0});
  const outputs = await Promise.all([a,b,c].map(item => waitReplayJob(item.entry.handle, 'concurrent')));
  assert.deepEqual(outputs.map(x => x.content[0].text), [a,b,c].map(x => x.entry.handle));
});

test('wait cancellation and timeout do not synthesize a terminal result or cancel the native promise', async () => {
  const item = job('wait', 'unfinished'), controller = new AbortController();
  const waiting = waitReplayJob(item.entry.handle, 'wait', controller.signal);
  controller.abort(new Error('test-wait-canceled'));
  await assert.rejects(waiting, /test-wait-canceled/);
  await assert.rejects(waitReplayJob(item.entry.handle, 'wait', undefined, 5), /Wait timed out/);
  item.pending.resolve({text: 'completed-later', status: 'completed', exitCode: 0});
  assert.equal((await waitReplayJob(item.entry.handle, 'wait')).content[0].text, 'completed-later');
});

test('native failure, forced-kill result and promise rejection remain failures', async () => {
  const failed = job('failure', 'native-failure');
  failed.pending.resolve({text: 'real execution failure', status: 'failed', exitCode: null});
  assert.equal((await waitReplayJob(failed.entry.handle, 'failure')).details.status, 'failed');
  const rejected = job('failure', 'native-rejected');
  rejected.pending.reject(new Error('native-finalize-rejected'));
  await assert.rejects(waitReplayJob(rejected.entry.handle, 'failure'), /native-finalize-rejected/);
});

test('repeated wait ToolCall ID has distinct invocation identities on one real monotonic clock', async () => {
  const item = job('wait-identity', 'source-exec');
  item.pending.resolve({text: 'finished', status: 'completed', exitCode: 0});
  await Promise.all([1,2].map(() => waitReplayJob(item.entry.handle, 'wait-identity', undefined, undefined, 'repeated-wait-call')));
  const rows = readFileSync('/tmp/aries-replay-tool-protocol.jsonl','utf8').trim().split('\n').map(JSON.parse)
    .filter(row => row.wait_tool_call_id === 'repeated-wait-call');
  const starts = rows.filter(row => row.event === 'wait_started'), ends = rows.filter(row => row.event === 'wait_finished');
  assert.equal(starts.length, 2); assert.equal(ends.length, 2);
  assert.equal(new Set(starts.map(row => row.wait_invocation_id)).size, 2);
  assert.equal(new Set(rows.map(row => row.clock_id)).size, 1);
  assert.equal(new Set(rows.map(row => row.emitter_id)).size, 1);
  for (const started of starts) {
    const finished = ends.find(row => row.wait_invocation_id === started.wait_invocation_id);
    assert.equal(finished.outcome, 'returned');
    assert.equal(finished.identity.tool_call_id, 'source-exec');
    assert.ok(BigInt(finished.observed_monotonic_ns) >= BigInt(started.observed_monotonic_ns));
  }
});

// Opt-in benchmark view over native jobs; the native promise still owns completion.
import { appendFileSync } from 'node:fs';
import { randomUUID } from 'node:crypto';

export const REPLAY_TOOL_PROTOCOL = 'terminal-result-v1';
const scopes = new Map();
const emitter = randomUUID(), clock = randomUUID(), origin = process.hrtime.bigint();
let sequence = 0, evidenceWarning = false;

export function replayToolProtocolEnabled() {
  return process.env.ARIES_REPLAY_TOOL_PROTOCOL === REPLAY_TOOL_PROTOCOL;
}

function scopeState(scopeKey) {
  const key = scopeKey ?? '';
  if (!scopes.has(key)) scopes.set(key, {jobs: new Map(), ordinals: new Map()});
  return scopes.get(key);
}

function record(event, job, extra = {}) {
  const row = {schema_version: 1, protocol: REPLAY_TOOL_PROTOCOL, sequence: ++sequence,
    event, emitter_id: emitter, emitter_pid: process.pid, clock_id: clock,
    clock_domain: 'native_replay_tool_monotonic', clock_origin_monotonic_ns: origin.toString(),
    observed_monotonic_ns: process.hrtime.bigint().toString(), previous_emit_failed: evidenceWarning,
    identity: job.identity,
    logical_handle: job.handle, native_session_id: job.run.session.id,
    native_pid: job.run.session.pid ?? null, ...extra};
  try {
    appendFileSync('/tmp/aries-replay-tool-protocol.jsonl', JSON.stringify(row) + '\n', {mode: 0o600});
  } catch {
    // Evidence collection is independent of the native execution result.
    if (!evidenceWarning) process.emitWarning('ARIES replay tool protocol evidence could not be written');
    evidenceWarning = true;
  }
}

export function registerReplayJob(run, identity, scopeKey, render) {
  if (!identity?.tool_call_id || !identity.native_invocation_id) throw new Error('Replay job requires native ToolCall identity');
  const state = scopeState(scopeKey);
  const ordinal = (state.ordinals.get(identity.tool_call_id) ?? 0) + 1;
  state.ordinals.set(identity.tool_call_id, ordinal);
  // ToolCall identity is recorded by the model Tape; native PID and random slug are not.
  const handle = `job-${Buffer.from(identity.tool_call_id).toString('base64url')}-${ordinal}`;
  const job = {handle, identity, run};
  state.jobs.set(handle, job);
  record('accepted', job);
  job.result = run.promise.then(outcome => {
    const result = render(outcome);
    record('terminal_result', job, {status: result.details?.status ?? null,
      exit_code: result.details?.exitCode ?? null, exit_signal: result.details?.exitSignal ?? null,
      output_renderer: result.details?.outputRenderer ?? null,
      output_truncated: result.details?.outputTruncated ?? null});
    return {...result, details: {...result.details, protocol: REPLAY_TOOL_PROTOCOL, sessionId: handle}};
  });
  // A background task can fail before the Agent asks for its result.
  job.result.catch(() => record('terminal_error', job));
  return job;
}

export function acceptedReplayJob(job, warningText = '') {
  return {content: [{type: 'text', text: `${warningText}Command accepted (session ${job.handle}). Use process action=wait with this sessionId for the final result.`}],
    details: {status: 'accepted', protocol: REPLAY_TOOL_PROTOCOL, sessionId: job.handle}};
}

export function resolveReplaySession(sessionId, scopeKey) {
  return scopes.get(scopeKey ?? '')?.jobs.get(sessionId)?.run.session.id ?? sessionId;
}

export async function waitReplayJob(sessionId, scopeKey, signal, timeoutMs, waitToolCallId) {
  const job = scopes.get(scopeKey ?? '')?.jobs.get(sessionId);
  if (!job) return {content: [{type: 'text', text: `No replay job found for ${sessionId}`}], details: {status: 'failed'}};
  if (timeoutMs !== undefined && (!Number.isFinite(timeoutMs) || timeoutMs <= 0)) throw new Error('wait timeout must be a positive number of milliseconds');
  let timer, onAbort, outcome = 'returned';
  const waitIdentity = {wait_tool_call_id: waitToolCallId ?? null, wait_invocation_id: randomUUID()};
  record('wait_started', job, waitIdentity);
  try {
    return await new Promise((resolve, reject) => {
      onAbort = () => reject(signal?.reason ?? new DOMException('Wait canceled; the background job was not canceled.', 'AbortError'));
      if (signal?.aborted) return onAbort();
      signal?.addEventListener('abort', onAbort, {once: true});
      if (timeoutMs !== undefined) timer = setTimeout(() => reject(new Error('Wait timed out; the background job was not canceled.')), timeoutMs);
      job.result.then(resolve, reject);
    });
  } catch (error) {
    outcome = 'interrupted';
    throw error;
  } finally {
    if (timer) clearTimeout(timer);
    if (onAbort) signal?.removeEventListener('abort', onAbort);
    record('wait_finished', job, {...waitIdentity, outcome});
  }
}

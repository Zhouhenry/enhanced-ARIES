// Loaded only by the isolated, hash-pinned R01 candidate image.
import { readFileSync, lstatSync, openSync, fstatSync, writeSync, closeSync, renameSync, unlinkSync, constants } from 'node:fs';
import { randomUUID } from 'node:crypto';
import { channel } from 'node:diagnostics_channel';

const scopePath = '/run/aries/tool-identity-scope.json';
const logPath = '/tmp/aries-native-tool-identities.jsonl';
const statusPath = '/tmp/aries-native-tool-identities.status.json';
const evidenceLimit = 4 * 1024 * 1024;
const producerInstanceId = randomUUID();
const statusKeys = 'agent_session_id,attempted,failed_latched,producer_instance_id,schema_version,scope_id,status_write_failed,write_failed,written';
let producerStatus;
let lastPublishedStatus;
const opaque = value => typeof value === 'string' && value.length > 0 &&
  value.length <= 256 && /^[A-Za-z0-9._:/|-]+$/.test(value);

function checkPrivateFile(fd, limit) {
  const info = fstatSync(fd);
  if (!info.isFile() || info.nlink !== 1 || info.uid !== process.geteuid() || (info.mode & 0o777) !== 0o600 || info.size > limit) {
    throw new Error('R01 producer evidence file rejected');
  }
  return info;
}

function checkPreviousStatus() {
  let fd;
  try {
    fd = openSync(statusPath, constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK);
    checkPrivateFile(fd, 4096);
    const previous = JSON.parse(readFileSync(fd, 'utf8'));
    if (!previous || Object.keys(previous).sort().join(',') !== statusKeys || previous.schema_version !== 1 ||
        previous.scope_id !== producerStatus.scope_id || previous.agent_session_id !== producerStatus.agent_session_id ||
        previous.producer_instance_id !== producerInstanceId || previous.failed_latched !== false ||
        !['attempted','written','write_failed','status_write_failed'].every(key => Number.isSafeInteger(previous[key]) && previous[key] >= 0) ||
        previous.attempted !== previous.written + previous.write_failed ||
        lastPublishedStatus && !Object.entries(lastPublishedStatus).every(([key,value]) => previous[key] === value)) producerStatus.failed_latched = true;
  } catch (error) {
    if (error.code !== 'ENOENT' || producerStatus.attempted > 1) producerStatus.failed_latched = true;
  } finally {
    if (fd !== undefined) {
      try { closeSync(fd); } catch { producerStatus.failed_latched = true; }
    }
  }
}

function persistStatus() {
  checkPreviousStatus();
  const temporary = `${statusPath}.${producerInstanceId}.${producerStatus.attempted}.tmp`;
  let fd;
  try {
    fd = openSync(temporary, constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL | constants.O_NOFOLLOW | constants.O_NONBLOCK, 0o600);
    checkPrivateFile(fd, 4096);
    const bytes = Buffer.from(JSON.stringify(producerStatus) + '\n');
    if (writeSync(fd, bytes) !== bytes.length) throw new Error('R01 producer status short write');
    const closing = fd; fd = undefined;
    closeSync(closing);
    renameSync(temporary, statusPath);
    lastPublishedStatus = {...producerStatus};
  } catch {
    producerStatus.status_write_failed++;
    producerStatus.failed_latched = true;
    // A later successful publication exposes the latch; total storage failure cannot be made durable here.
  } finally {
    if (fd !== undefined) {
      try { closeSync(fd); } catch { producerStatus.failed_latched = true; }
    }
    try { unlinkSync(temporary); } catch {}
  }
}

function recordIdentity(identity) {
  producerStatus ??= {schema_version: 1, scope_id: identity.scope_id, agent_session_id: identity.agent_session_id,
    producer_instance_id: producerInstanceId, attempted: 0, written: 0, write_failed: 0, status_write_failed: 0, failed_latched: false};
  producerStatus.attempted++;
  let fd, failed = false;
  try {
    const line = Buffer.from(JSON.stringify(identity) + '\n');
    fd = openSync(logPath, constants.O_WRONLY | constants.O_APPEND | constants.O_CREAT | constants.O_NOFOLLOW | constants.O_NONBLOCK, 0o600);
    if (checkPrivateFile(fd, evidenceLimit).size + line.length > evidenceLimit) throw new Error('R01 producer evidence limit');
    if (writeSync(fd, line) !== line.length) throw new Error('R01 producer evidence short write');
  } catch { failed = true; }
  finally {
    if (fd !== undefined) {
      try { closeSync(fd); } catch { failed = true; }
    }
  }
  if (failed) { producerStatus.write_failed++; producerStatus.failed_latched = true; }
  else producerStatus.written++;
  persistStatus();
}

export function createToolIdentity(toolCallId, sessionKey) {
  const stat = lstatSync(scopePath);
  if (!stat.isFile() || stat.isSymbolicLink() || (stat.mode & 0o222) !== 0 || stat.size > 4096) {
    throw new Error('R01 PoC scope must be an immutable regular file');
  }
  const scope = JSON.parse(readFileSync(scopePath, 'utf8'));
  if (!opaque(toolCallId) || !opaque(sessionKey) || !opaque(scope.scope_id) ||
      sessionKey !== scope.agent_session_id || Object.keys(scope).sort().join(',') !== 'agent_session_id,scope_id') {
    throw new Error('R01 PoC identity scope mismatch');
  }
  const identity = Object.freeze({schema_version: 1, scope_id: scope.scope_id,
    agent_session_id: sessionKey, tool_call_id: toolCallId, native_invocation_id: randomUUID()});
  if (producerStatus && (producerStatus.scope_id !== scope.scope_id || producerStatus.agent_session_id !== sessionKey)) {
    throw new Error('R01 PoC producer identity scope changed');
  }
  recordIdentity({...identity, schema_version: 2, phase: 'invocation', outcome: 'started', dispatched: null, reason: ''});
  channel('aries.tool.identity.v1').publish(identity);
  return identity;
}

// Called only at explicitly instrumented native rejection branches before dispatch.
export function recordToolRejection(identity, reason = 'shell_preflight_rejected') {
  if (identity) recordIdentity({...identity, schema_version: 2, phase: 'pre_dispatch',
    outcome: 'rejected', dispatched: false, reason});
}

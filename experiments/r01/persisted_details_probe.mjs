// ECS-only real native SessionManager/guard persistence; no Agent or model calls.
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { mkdirSync, readFileSync } from 'node:fs';
import { z as guardSessionManager } from '/app/dist/attempt.model-diagnostic-events-Dg8sP6iR.js';
import { t as SessionManager } from '/app/dist/session-manager-BKz2VYho.js';

assert.equal(process.platform, 'linux');
const mode = process.argv[2];
assert.ok(['baseline', 'candidate'].includes(mode));
const candidate = mode === 'candidate';
const bytes = value => Buffer.byteLength(JSON.stringify(value));
const sha = value => createHash('sha256').update(JSON.stringify(value)).digest('hex');
const fields = ['protocol', 'outputRenderer', 'outputTruncated'];
const metadata = {status: 'completed', sessionId: 'job-fixture', cwd: '/work', exitCode: 0,
  exitSignal: null, protocol: 'terminal-result-v1', outputRenderer: 'stdout-stderr-v1', outputTruncated: false};
const sized = target => {
  const details = {...metadata, aggregated: ''};
  details.aggregated = 'x'.repeat(target - bytes(details));
  assert.equal(bytes(details), target);
  return details;
};
const fallback = {...metadata, aggregated: 'x'.repeat(12000)};
for (const key of ['status', 'sessionId', 'pid', 'startedAt', 'endedAt', 'cwd', 'name',
  'retryInMs', 'total', 'totalLines', 'totalChars', 'fullOutputPath']) fallback[key] = 'q'.repeat(2000);
const absent = {status: 'completed', exitCode: 0, aggregated: 'x'.repeat(12000)};
const cases = [
  ['below-cap', sized(8191)], ['at-cap', sized(8192)], ['over-cap', sized(8193)],
  ['large-aggregate-false', sized(14000)], ['large-aggregate-true', {...sized(14000), outputTruncated: true}],
  ['fallback', fallback], ['fields-absent', absent],
  ['large-content', sized(14000), 'public-output\n'.repeat(20000)],
];
const dir = '/tmp/aries-persisted-details-' + mode;
mkdirSync(dir, {recursive: true});
const manager = SessionManager.create('/tmp', dir);
guardSessionManager(manager, {allowedToolNames: new Set(['process']), allowSyntheticToolResults: false});
const report = [];
for (const [name, details, text = 'public result'] of cases) {
  const id = 'details-' + name;
  manager.appendMessage({role: 'assistant', content: [{type: 'toolCall', id, name: 'process', arguments: {action: 'wait', sessionId: 'job-fixture'}}],
    api: 'openai-completions', provider: 'openai', model: 'public-fixture', stopReason: 'toolUse', timestamp: 1,
    usage: {input: 0, output: 0, cacheRead: 0, cacheWrite: 0, totalTokens: 0,
      cost: {input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0}}});
  const original = {role: 'toolResult', toolCallId: id, toolName: 'process', content: [{type: 'text', text}], details,
    isError: false, timestamp: 2};
  const before = structuredClone(original);
  manager.appendMessage(original);
  assert.deepEqual(original, before, name + ': live result mutated');
  const entries = readFileSync(manager.getSessionFile(), 'utf8').trimEnd().split('\n').map(JSON.parse);
  const matches = entries.filter(row => row.type === 'message' && row.message?.role === 'toolResult' && row.message.toolCallId === id);
  assert.equal(matches.length, 1, name + ': expected one real JSONL result');
  const stored = matches[0].message;
  assert.ok(bytes(stored.details) <= 8192, name + ': cap exceeded');
  const truncated = bytes(details) > 8192;
  assert.equal(stored.details.persistedDetailsTruncated === true, truncated, name + ': threshold changed');
  if (name === 'fallback') assert.equal(stored.details.finalDetailsTruncated, true);
  if (truncated) assert.equal(Object.hasOwn(stored.details, 'aggregated'), false, name + ': aggregate enlarged');
  for (const key of fields) {
    if (Object.hasOwn(details, key) && (candidate || !truncated)) assert.equal(stored.details[key], details[key], name + ': ' + key);
    else assert.equal(Object.hasOwn(stored.details, key), false, name + ': unexpected ' + key);
  }
  if (name !== 'large-content') assert.deepEqual(stored.content, original.content);
  else assert.ok(JSON.stringify(stored.content).length < JSON.stringify(original.content).length, 'native content cap not exercised');
  const projection = structuredClone(stored);
  for (const key of fields) delete projection.details[key];
  delete projection.details.sanitizedDetailsBytes;
  report.push({name, original_details_bytes: bytes(details), persisted_details_bytes: bytes(stored.details),
    truncated, fallback: stored.details.finalDetailsTruncated === true, content_sha256: sha(stored.content),
    original_unchanged: true, unchanged_fields_sha256: sha(projection),
    sanitized_details_bytes: stored.details.sanitizedDetailsBytes ?? null,
    retained_field_bytes: bytes(Object.fromEntries(fields.filter(key => Object.hasOwn(stored.details, key)).map(key => [key, stored.details[key]]))) - 1,
    retained_fields: fields.filter(key => Object.hasOwn(stored.details, key)),
    output_truncated: stored.details.outputTruncated ?? null});
}
manager.flushPendingToolResults();
console.log(JSON.stringify({status: 'PASS_NATIVE_PERSISTENCE_' + mode.toUpperCase(), cases: report,
  native_session_jsonl_sha256: createHash('sha256').update(readFileSync(manager.getSessionFile())).digest('hex'),
  cloud_calls: 0, model_calls: 0, limitations: ['fixture ToolResults through real native persistence; no Agent end-to-end run']}));

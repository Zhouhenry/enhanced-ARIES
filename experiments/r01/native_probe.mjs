// Executes the real pinned createExecTool and SSH backend, without an LLM.
import { r as createExecTool } from '/app/dist/bash-tools-D7H1rgMh.js';
import { c as backendFactory } from '/app/dist/browser-bridges-Be07JHOW.js';
import { readFileSync, readdirSync, existsSync } from 'node:fs';
import assert from 'node:assert/strict';
import { AsyncLocalStorage } from 'node:async_hooks';
import { channel } from 'node:diagnostics_channel';

const cfg = JSON.parse(readFileSync('/run/aries/probe.json', 'utf8'));
const backend = await backendFactory('ssh')({
  cfg: {docker: {env: {HOME: '/tmp'}}, workspaceAccess: 'rw',
    ssh: {target: 'aries@' + cfg.address, command: '/opt/aries/bin/aries-ssh-client',
      workspaceRoot: '/aries/openclaw', strictHostKeyChecking: false, updateHostKeys: false}},
  scopeKey: 'shared', workspaceDir: '/tmp', agentWorkspaceDir: '/tmp',
});
const tool = createExecTool({host: 'sandbox', sandbox: {...backend,
  containerWorkdir: backend.workdir, workspaceDir: '/tmp'},
  cwd: '/tmp', sessionKey: cfg.agent_session_id, allowBackground: false, notifyOnExit: false});
const results = [];
const calls = new AsyncLocalStorage();
channel('aries.tool.identity.v1').subscribe(identity => {
  const call = calls.getStore();
  assert.ok(call);
  assert.equal(identity.tool_call_id, call.tool_call_id);
  assert.equal(call.native_identity, undefined);
  call.native_identity = identity;
});
let caseNumber = 0;
const invoke = (id, command) => calls.run({case_id: ++caseNumber, tool_call_id: id}, async () => {
  const call = calls.getStore();
  call.response = await tool.execute(id, {command, workdir: backend.workdir, timeout: 10});
  results.push(call);
  return call.response;
});
for (const id of ['repeat-1', 'repeat-2']) await invoke(id, 'printf same');
await Promise.all(['parallel-1','parallel-2'].map(id => invoke(id, 'sleep 0.1; printf concurrent')));
for (let i = 0; i < 2; i++) await invoke('retry-tool', 'printf retry');
await Promise.all([invoke('slow','sleep 0.3; printf slow'), invoke('fast','printf fast')]);
await invoke('nonzero','printf out; printf err >&2; exit 7');
assert.equal(results.length, 9);
for (const record of results) assert.ok(record.response.content?.length > 0);
const leftovers = ['/tmp/openclaw', '/tmp/openclaw-1000'].flatMap(root =>
  existsSync(root) ? readdirSync(root, {withFileTypes: true})
    .filter(entry => entry.name.startsWith('openclaw-sandbox-ssh-')).map(entry => root + '/' + entry.name) : []);
assert.deepEqual(leftovers, []);
console.log(JSON.stringify({schema_version: 1, scope: 'native createExecTool + native SSH backend',
  results, metadata_directories_remaining: leftovers.length}));

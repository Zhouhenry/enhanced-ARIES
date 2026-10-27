// ECS-only native parser/policy probe; no SSH child may be dispatched.
import childProcess from 'node:child_process';
import { syncBuiltinESMExports } from 'node:module';
import { readFileSync, existsSync } from 'node:fs';
import { channel } from 'node:diagnostics_channel';
import assert from 'node:assert/strict';

assert.equal(process.platform, 'linux');
const candidate = process.argv[2] === 'candidate';
assert.ok(candidate || process.argv[2] === 'baseline');
const scope = JSON.parse(readFileSync('/run/aries/tool-identity-scope.json', 'utf8'));
const witnessed = [];
channel('aries.tool.identity.v1').subscribe(identity => witnessed.push(identity));
let backendCalls = 0, returnedExecSpecs = 0, sshDispatches = 0;
const originalSpawn = childProcess.spawn;
childProcess.spawn = () => { sshDispatches++; throw new Error('unexpected child dispatch in pre-dispatch probe'); };
syncBuiltinESMExports();
try {
  const { r: createExecTool } = await import('/app/dist/bash-tools-D7H1rgMh.js');
  const { i: getSandboxBackendFactory, p: buildValidatedExecRemoteCommand } = await import('/app/dist/browser-bridges-Be07JHOW.js');
  const factory = getSandboxBackendFactory('ssh');
  assert.equal(typeof factory, 'function');
  const backend = await factory({scopeKey: 'native-rejection-probe', workspaceDir: '/tmp', agentWorkspaceDir: '/tmp',
    cfg: {docker: {env: {}}, ssh: {target: 'probe@127.0.0.1', workspaceRoot: '/tmp/aries-native-rejection'}, workspaceAccess: 'none'}});
  assert.equal(backend.id, 'ssh');
  // A static local workdir isolates command validation from remote-workdir setup.
  // The execution specification itself is the original factory's SSH function.
  const sandbox = {containerWorkdir: '/tmp', workspaceDir: '/tmp',
      buildExecSpec: async params => {
        backendCalls++;
        const spec = await backend.buildExecSpec(params);
        returnedExecSpecs++;
        return spec;
      }};
  const tool = createExecTool({host: 'sandbox', cwd: '/tmp', sessionKey: scope.agent_session_id,
    allowBackground: true, notifyOnExit: false, sandbox});
  async function thrown(call) {
    try { await call(); } catch (error) { return error; }
    throw new Error('expected original native rejection');
  }
  const originalErrors = [];
  for (const [name, command] of [
    ['heredoc', "cat > /tmp/aries-heredoc-rejection <<'VIMEOF'\nprobe\nWIMEOF"],
    ['here-string', "tclsh <<< 'puts $tcl_version'"]
  ]) {
    const parser = await thrown(() => buildValidatedExecRemoteCommand({command, workdir: '/tmp', env: {}}));
    const rejected = await thrown(() => tool.execute(name, {command, workdir: '/tmp'}));
    assert.equal(rejected.name, parser.name);
    assert.equal(rejected.message, parser.message);
    assert.equal(rejected.message, name === 'heredoc'
      ? 'Malformed SSH/OpenShell exec command: unterminated here-doc VIMEOF.'
      : 'Malformed SSH/OpenShell exec command: unterminated here-doc puts $tcl_version.');
    originalErrors.push({case: name, name: rejected.name, message: rejected.message});
  }
  const elevated = await thrown(() => tool.execute('elevated-disabled', {command: 'true', elevated: true}));
  assert.ok(elevated.message.startsWith('elevated is not available right now (runtime=sandboxed).'));
  originalErrors.push({case: 'elevated-disabled', name: elevated.name, message: elevated.message});
  const autoTool = createExecTool({host: 'auto', cwd: '/tmp', sessionKey: scope.agent_session_id,
    allowBackground: true, notifyOnExit: false, sandbox});
  for (const host of ['gateway', 'node']) {
    const rejected = await thrown(() => autoTool.execute('host-' + host, {command: 'true', host}));
    assert.equal(rejected.message, `exec host not allowed (requested ${host}; configured host is auto; set tools.exec.host=${host} to allow this override).`);
    originalErrors.push({case: 'host-' + host, name: rejected.name, message: rejected.message});
  }
  const directTool = createExecTool({host: 'gateway', sessionKey: scope.agent_session_id});
  const direct = await thrown(() => directTool.execute('direct-host-node', {command: 'true', host: 'node'}));
  assert.equal(direct.message, 'exec host not allowed (requested node; configured host is gateway; set tools.exec.host=node or auto to allow this override).');
  originalErrors.push({case: 'direct-host-node', name: direct.name, message: direct.message});
  const invalid = await thrown(() => autoTool.execute('invalid-host', {command: 'true', host: 'invalid-host'}));
  originalErrors.push({case: 'invalid-host', name: invalid.name, message: invalid.message});
  const missing = await thrown(() => tool.execute('missing-command', {command: '', workdir: '/tmp'}));
  assert.equal(missing.message, 'Provide a command to start.');
  originalErrors.push({case: 'missing-command', name: missing.name, message: missing.message});
  assert.equal(backendCalls, 2);
  assert.equal(returnedExecSpecs, 0);
  assert.equal(sshDispatches, 0);
  const logPath = '/tmp/aries-native-tool-identities.jsonl';
  let records = null, health = null;
  if (candidate) {
    records = readFileSync(logPath, 'utf8').trimEnd().split('\n').map(line => JSON.parse(line));
    health = JSON.parse(readFileSync('/tmp/aries-native-tool-identities.status.json', 'utf8'));
    assert.equal(witnessed.length, 6);
    assert.equal(records.length, 11);
    assert.deepEqual(records.map(record => [record.phase, record.outcome, record.dispatched]),
      [['invocation', 'started', null], ['pre_dispatch', 'rejected', false],
        ['invocation', 'started', null], ['pre_dispatch', 'rejected', false],
        ['invocation', 'started', null], ['pre_dispatch', 'rejected', false],
        ['invocation', 'started', null], ['pre_dispatch', 'rejected', false],
        ['invocation', 'started', null], ['pre_dispatch', 'rejected', false], ['invocation', 'started', null]]);
    assert.deepEqual(records.filter(record => record.phase === 'pre_dispatch').map(record => record.reason),
      ['shell_preflight_rejected', 'shell_preflight_rejected', 'elevated_policy_rejected', 'host_policy_rejected', 'host_policy_rejected']);
    assert.equal(new Set(witnessed.map(identity => identity.native_invocation_id)).size, 6);
    assert.deepEqual(records.map(record => record.tool_call_id),
      ['heredoc', 'heredoc', 'here-string', 'here-string', 'elevated-disabled', 'elevated-disabled',
        'host-gateway', 'host-gateway', 'host-node', 'host-node', 'missing-command']);
    for (const [index, identity] of witnessed.entries()) {
      assert.equal(identity.schema_version, 1);
      assert.deepEqual(Object.keys(identity).sort(), ['agent_session_id', 'native_invocation_id', 'schema_version', 'scope_id', 'tool_call_id']);
      for (const key of ['scope_id', 'agent_session_id', 'tool_call_id', 'native_invocation_id']) {
        assert.equal(records[index * 2][key], identity[key]);
        if (index < 5) assert.equal(records[index * 2 + 1][key], identity[key]);
      }
    }
    assert.deepEqual([health.attempted, health.written, health.write_failed, health.failed_latched], [11, 11, 0, false]);
  } else {
    assert.equal(witnessed.length, 0);
    assert.equal(existsSync(logPath), false);
  }
  console.log(JSON.stringify({status: 'PASS_NATIVE_PRE_DISPATCH_ONLY', backend_calls: backendCalls,
    tool_ssh_dispatches: sshDispatches, returned_exec_specs: returnedExecSpecs, original_errors: originalErrors,
    records, health, limits: ['native SSH factory with static workdir fixture; no remote-workdir setup',
      'no Agent/model/cloud run', 'other pre-dispatch exits remain unknown', 'original here-string parser behavior unchanged',
      'no guest execution or completion claim']}));
} finally {
  childProcess.spawn = originalSpawn;
  syncBuiltinESMExports();
}

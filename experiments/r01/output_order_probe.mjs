// Real pinned createExecTool + SSH backend + Bridge; no model or simulated tool result.
import { r as createExecTool, t as createProcessTool } from '/app/dist/bash-tools-D7H1rgMh.js';
import { c as backendFactory } from '/app/dist/browser-bridges-Be07JHOW.js';
import { readFileSync, openSync, fstatSync, closeSync, constants } from 'node:fs';
import { AsyncLocalStorage } from 'node:async_hooks';
import { channel } from 'node:diagnostics_channel';
import assert from 'node:assert/strict';

// The real harness has server handles; native poll timers intentionally do not keep Node alive.
const harnessHandle=setInterval(()=>{},1000);
const invocationContext=new AsyncLocalStorage(), execInvocations=[], nativeIdentityWitnesses=[], toolOptions=new WeakMap();
channel('aries.tool.identity.v1').subscribe(identity=>nativeIdentityWitnesses.push({
  call_index:invocationContext.getStore()?.call_index??null,identity}));
function trackedExecTool(options) {
  const result=createExecTool(options); toolOptions.set(result,options); return result;
}
function executeNative(tool,caseId,...args) {
  const options=toolOptions.get(tool), host=args[1]?.host??options.host;
  const invocation={call_index:execInvocations.length,case_id:caseId,tool_call_id:args[0],host,
    identity_expected:host==='sandbox'&&Boolean(options.sandbox),pty_requested:args[1]?.pty===true};
  execInvocations.push(invocation);
  return invocationContext.run(invocation,()=>Reflect.apply(tool.execute,tool,args));
}
function readProducerEvidence(path,limit) {
  let fd, result;
  try {
    fd=openSync(path,constants.O_RDONLY|constants.O_NOFOLLOW|constants.O_NONBLOCK);
    const info=fstatSync(fd);
    if(!info.isFile()||info.nlink!==1||info.uid!==process.geteuid()||(info.mode&0o777)!==0o600||info.size>limit) throw new Error('private evidence rejected');
    result={status:'captured',bytes_base64:readFileSync(fd).toString('base64')};
  } catch(error) { result={status:error.code==='ENOENT'?'absent':'error',error_code:error.code??error.name}; }
  finally { if(fd!==undefined) try { closeSync(fd); } catch(error) { result={status:'error',error_code:error.code??error.name}; } }
  return result;
}
const cfg = JSON.parse(readFileSync('/run/aries/probe.json','utf8'));
const backend = await backendFactory('ssh')({cfg:{docker:{env:{HOME:'/tmp'}},workspaceAccess:'rw',
  ssh:{target:'aries@'+cfg.address,command:'/opt/aries/bin/aries-ssh-client',workspaceRoot:'/aries/openclaw',
    strictHostKeyChecking:false,updateHostKeys:false}},scopeKey:'shared',workspaceDir:'/tmp',agentWorkspaceDir:'/tmp'});
const defaults = {host:'sandbox',sandbox:{...backend,containerWorkdir:backend.workdir,workspaceDir:'/tmp'},
  cwd:'/tmp',sessionKey:cfg.agent_session_id,scopeKey:'output-order',allowBackground:true,notifyOnExit:false};
const tool = trackedExecTool(defaults);
const results = [];
const cases = [
  ['forward','printf out; sleep 0.08; printf err >&2; exit 7','outerr\n\n(Command exited with code 7)'],
  ['reverse','printf err >&2; sleep 0.08; printf out; exit 7','outerr\n\n(Command exited with code 7)'],
  ['interleave','printf o; sleep 0.04; printf e >&2; sleep 0.04; printf ut; sleep 0.04; printf rr >&2; exit 7','outerr\n\n(Command exited with code 7)'],
  ['empty','true',''],
  ['ansi-whole',"printf '\\033[31mred\\033[0m'; printf '\\033]title\\007err' >&2",'rederr'],
  ['ansi-split',"printf '\\033['; sleep 0.08; printf '31mred\\033'; sleep 0.08; printf '[0m'; printf '\\033]ti' >&2; sleep 0.08; printf 'tle\\007err' >&2",'rederr'],
  ['unicode',"python3 -c 'import os,time; b=\"中文🙂\".encode(); [ (os.write(1,bytes([x])), time.sleep(.01)) for x in b ]; os.write(2,\"后𝌆\".encode())'",'中文🙂后𝌆'],
  ['binary',"printf '\\001\\000ok'; printf '\\177err' >&2",'\\x01\\x00ok\\x7ferr'],
  ['cap',"python3 -c 'import os; os.write(2,b\"err\"); os.write(1,b\"o\"*201000)'",'o'.repeat(199997)+'err'],
  ['ansi-limit',"python3 -c 'import os; os.write(1,b\"\\x1b]\"+b\"a\"*8192+b\"\\x07\")'",null],
  ['ansi-limit-126',"python3 -c 'import os; os.write(1,b\"\\x1b]\"+b\"a\"*8192+b\"\\x07\"); raise SystemExit(126)'",null],
  ['126','printf err >&2; sleep 0.08; printf out; exit 126',null],
  ['127','printf err >&2; sleep 0.08; printf out; exit 127',null],
];
async function invoke(id, command, options = {}) {
  const updates=[];
  const controller=new AbortController();
  const response=await executeNative(tool,id,id,{command,workdir:backend.workdir,timeout:options.timeout || 15,
    ...(options.yieldMs===undefined?{}:{yieldMs:options.yieldMs})},controller.signal,update=>{
    updates.push(update);
    if(options.cancel && update.content?.some(x=>x.text?.includes('ready'))) controller.abort();
  });
  const result={id,command,response,updates}; results.push(result); return result;
}
for(const [id,command,expected] of cases) {
  const row=await invoke(id,command); row.expected=expected;
}
const beforeYield=await invoke('before-yield','printf err >&2; sleep 0.08; printf out',{yieldMs:1000});
assert.equal(beforeYield.response.details.status,'completed');
beforeYield.expected='outerr';
await invoke('timeout','printf err >&2; sleep 0.08; printf out; sleep 3',{timeout:1});
await invoke('cancel','printf ready; sleep 3',{cancel:true});
const finalizeErrorTool=trackedExecTool({...defaults,sandbox:{...defaults.sandbox,finalizeExec:async()=>{throw new Error('injected-finalize-error');}}});
const finalizeCommand='printf err >&2; sleep 0.08; printf out; true';
const finalizeResponse=await executeNative(finalizeErrorTool,'finalize-error','finalize-error',{command:finalizeCommand,workdir:backend.workdir,timeout:10});
assert.equal(finalizeResponse.details.status,'failed');
assert.ok(finalizeResponse.content[0].text.includes('injected-finalize-error'));
results.push({id:'finalize-error',command:finalizeCommand,response:finalizeResponse});
const backgroundTool=trackedExecTool({...defaults,allowBackground:true});
const processTool=createProcessTool({scopeKey:defaults.scopeKey});
const background=await executeNative(backgroundTool,'background','background',{command:'sleep 0.2; printf err >&2; sleep 0.08; printf out',workdir:backend.workdir,background:true,timeout:10});
assert.equal(background.details.status,'running');
const sessionId=background.details.sessionId;
await new Promise(resolve=>setTimeout(resolve,500));
const poll=await processTool.execute('poll',{action:'poll',sessionId,timeout:1000});
assert.equal(poll.details.status,'completed');
assert.equal(poll.details.exitCode,0);
assert.equal(poll.details.aggregated,'errout');
const backgroundLog=await processTool.execute('background-log',{action:'log',sessionId});
assert.equal(backgroundLog.content[0].text,'errout');
results.push({id:'background',response:background,poll,log:backgroundLog});
for(let n=0;n<4;n++) {
  const id='background-fast-'+n, output='fast'+n, exitCode=n%2?7:0;
  const response=await executeNative(backgroundTool,id,id,{command:`printf ${output}; exit ${exitCode}`,workdir:backend.workdir,background:true,timeout:10});
  assert.equal(response.details.status,'running');
  await new Promise(resolve=>setTimeout(resolve,250));
  const poll=await processTool.execute(id+'-poll',{action:'poll',sessionId:response.details.sessionId,timeout:2000});
  assert.notEqual(poll.details.status,'running');
  assert.equal(poll.details.exitCode,exitCode);
  assert.equal(poll.details.aggregated,output);
  const log=await processTool.execute(id+'-log',{action:'log',sessionId:response.details.sessionId});
  assert.equal(log.content[0].text,output);
  results.push({id,response,poll,log});
}
const autoYield=await executeNative(backgroundTool,'auto-yield','yield',{command:'sleep 0.2; printf yielded',workdir:backend.workdir,yieldMs:10,timeout:10});
assert.equal(autoYield.details.status,'running');
await new Promise(resolve=>setTimeout(resolve,400));
const yieldPoll=await processTool.execute('yield-poll',{action:'poll',sessionId:autoYield.details.sessionId,timeout:1000});
assert.equal(yieldPoll.details.status,'completed');
assert.equal(yieldPoll.details.exitCode,0);
assert.equal(yieldPoll.details.aggregated,'yielded');
const yieldLog=await processTool.execute('yield-log',{action:'log',sessionId:autoYield.details.sessionId});
assert.equal(yieldLog.content[0].text,'yielded');
results.push({id:'auto-yield',response:autoYield,poll:yieldPoll,log:yieldLog});
const zeroYield=await executeNative(backgroundTool,'zero-yield','zero-yield',{command:'sleep 0.2; printf zero',workdir:backend.workdir,yieldMs:0,timeout:10});
assert.equal(zeroYield.details.status,'running');
const zeroPoll=await processTool.execute('zero-poll',{action:'poll',sessionId:zeroYield.details.sessionId,timeout:2000});
assert.equal(zeroPoll.details.status,'completed');
assert.equal(zeroPoll.details.exitCode,0);
assert.equal(zeroPoll.details.aggregated,'zero');
const zeroLog=await processTool.execute('zero-log',{action:'log',sessionId:zeroYield.details.sessionId});
assert.equal(zeroLog.content[0].text,'zero');
results.push({id:'zero-yield',response:zeroYield,poll:zeroPoll,log:zeroLog});
let finalizeStarted=false,finalizeFinished=false;
const slowFinalizeTool=trackedExecTool({...defaults,sandbox:{...defaults.sandbox,finalizeExec:async()=>{
  finalizeStarted=true; await new Promise(resolve=>setTimeout(resolve,1500)); finalizeFinished=true;
}}});
const finalizeYield=await executeNative(slowFinalizeTool,'finalize-yield','finalize-yield',{command:'printf finalized',workdir:backend.workdir,yieldMs:1000,timeout:10});
assert.equal(finalizeYield.details.status,'running');
assert.ok(finalizeStarted && !finalizeFinished);
await new Promise(resolve=>setTimeout(resolve,1000));
assert.ok(finalizeFinished);
const finalizePoll=await processTool.execute('finalize-poll',{action:'poll',sessionId:finalizeYield.details.sessionId,timeout:1000});
const finalizeLog=await processTool.execute('finalize-log',{action:'log',sessionId:finalizeYield.details.sessionId});
results.push({id:'finalize-yield',response:finalizeYield,poll:finalizePoll,log:finalizeLog,
  diagnostic_only:true,finalize_started_before_yield:true,finalize_finished:true});
const ptyTool=trackedExecTool({host:'gateway',cwd:'/tmp',security:'full',ask:'off',allowBackground:true,notifyOnExit:false});
const pty=await executeNative(ptyTool,'pty','pty',{command:'test -t 1 && printf pty',pty:true,timeout:5});
assert.equal(pty.details.exitCode,0);
assert.ok(!pty.content[0].text.includes('PTY spawn failed'));
results.push({id:'pty',response:pty});
console.log(JSON.stringify({schema_version:1,scope:'real native SSH Bridge output renderer',allow_background:true,process_enabled:true,results,
  exec_invocations:execInvocations,native_identity_witnesses:nativeIdentityWitnesses,
  producer_evidence:{log:readProducerEvidence('/tmp/aries-native-tool-identities.jsonl',4*1024*1024),
    status:readProducerEvidence('/tmp/aries-native-tool-identities.status.json',4096)}}));
clearInterval(harnessHandle);

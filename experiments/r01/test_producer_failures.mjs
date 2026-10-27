// Independent ECS-only fault experiments. The business child is real; filesystem faults are explicitly labelled.
import fs from 'node:fs';
import { syncBuiltinESMExports } from 'node:module';
import { spawnSync } from 'node:child_process';
import { channel } from 'node:diagnostics_channel';
import { createHash } from 'node:crypto';
import { pathToFileURL, fileURLToPath } from 'node:url';
import assert from 'node:assert/strict';

if (process.platform !== 'linux') throw new Error('run on authorized Linux ECS only');
const helper = process.argv[3] || '/run/aries/tool_identity.mjs';
const name = process.argv[2];
const logPath = '/tmp/aries-native-tool-identities.jsonl';
const statusPath = '/tmp/aries-native-tool-identities.status.json';
const cases = ['healthy','enospc','short-write','close-error','log-mode','log-symlink','log-hardlink','log-fifo','log-fifo-reader','log-cap',
  'status-short-write','status-close-error','status-rename-error','status-always-fails','status-symlink','status-fifo',
  'producer-conflict','same-instance-rollback','stale-clean-status','bad-session','bad-tool-id'];

if (!name) {
  const results = cases.map(test => {
    const child = spawnSync(process.execPath, [fileURLToPath(import.meta.url), test, helper], {encoding:'utf8',timeout:15000});
    const parsed = child.status === 0 ? JSON.parse(child.stdout) : undefined;
    return {case:test, exit_code:child.status, signal:child.signal, stderr:child.stderr, result:parsed,
      ...(child.status !== 0 ? {stdout:child.stdout,error:child.error?.message} : {})};
  });
  const status = results.every(row => row.exit_code === 0 && row.stderr === '' && row.result.status === 'PASS_EXPECTED_BEHAVIOR') ? 'PASS_PRODUCER_FAULT_HELPER_ONLY' : 'FAIL';
  console.log(JSON.stringify({schema_version:1,status,cases:results,
    limits:['synchronous regular-file write/close latency unbounded','all storage failure may leave stale clean status',
      'single producer per harness; multi-process aggregate unsupported','not a full native Agent/cloud replay']}, null, 2));
  process.exitCode = status === 'FAIL' ? 1 : 0;
} else {
  assert.ok(cases.includes(name) || name === 'real-enospc' || name === 'legacy-enospc');
  const real = Object.fromEntries(['openSync','writeSync','closeSync','renameSync','readFileSync','fstatSync','unlinkSync'].map(key => [key,fs[key].bind(fs)]));
  for (const entry of fs.readdirSync('/tmp')) if (entry.startsWith('aries-native-tool-identities') || entry === 'producer-test-sentinel' || entry === 'producer-test-fill') fs.rmSync('/tmp/'+entry,{force:true});
  const sentinel='/tmp/producer-test-sentinel';
  const previous={schema_version:1,scope_id:'producer-fault-scope',agent_session_id:'producer-fault-session',
    producer_instance_id:'00000000-0000-4000-8000-000000000000',attempted:0,written:0,write_failed:0,status_write_failed:0,failed_latched:false};
  let reader;
  if (name === 'log-mode') fs.writeFileSync(logPath,'preserve',{mode:0o644});
  if (name === 'log-cap') fs.writeFileSync(logPath,Buffer.alloc(4*1024*1024,120),{mode:0o600});
  if (name.endsWith('symlink')) { fs.writeFileSync(sentinel,'preserve',{mode:0o600}); fs.symlinkSync(sentinel,name.startsWith('log')?logPath:statusPath); }
  if (name === 'log-hardlink') { fs.writeFileSync(sentinel,'preserve',{mode:0o600}); fs.linkSync(sentinel,logPath); }
  if (name.includes('fifo')) {
    const target=name.startsWith('log')?logPath:statusPath;
    const result=spawnSync('mkfifo',['-m','600',target]); assert.equal(result.status,0);
    if (name === 'log-fifo-reader') reader=real.openSync(target,fs.constants.O_RDONLY|fs.constants.O_NONBLOCK);
  }
  if (name === 'producer-conflict') fs.writeFileSync(statusPath,JSON.stringify(previous),{mode:0o600});
  let realEnospc=false;
  if (name === 'real-enospc') {
    const fd=real.openSync('/tmp/producer-test-fill','w',0o600);
    try { const bytes=Buffer.alloc(4096); for (let n=0;n<65536;n++) real.writeSync(fd,bytes); }
    catch(error) { assert.equal(error.code,'ENOSPC'); realEnospc=true; }
    finally { real.closeSync(fd); }
    assert.ok(realEnospc,'real-enospc must run with a bounded private tmpfs');
  }
  const paths=new Map(), faults=[];
  let calls=0, used=false;
  const failure = code => Object.assign(new Error('injected producer '+code),{code});
  fs.openSync=(path,...args)=>{ const fd=real.openSync(path,...args); paths.set(fd,String(path)); return fd; };
  fs.writeSync=(fd,bytes,...args)=>{
    const path=paths.get(fd), log=path===logPath, status=path?.startsWith(statusPath+'.');
    const unavailable=name==='status-always-fails'&&status || name==='stale-clean-status'&&calls===2&&(log||status);
    if (unavailable || !used && log && (name==='enospc'||name==='legacy-enospc')) { used=true; faults.push({operation:'write',target:log?'log':'status',error:'ENOSPC'}); throw failure('ENOSPC'); }
    if (!used && (name==='short-write'&&log || name==='status-short-write'&&status)) {
      used=true; const written=real.writeSync(fd,bytes.subarray(0,Math.floor(bytes.length/2)));
      faults.push({operation:'write',target:log?'log':'status',short_bytes:written,requested_bytes:bytes.length}); return written;
    }
    return real.writeSync(fd,bytes,...args);
  };
  fs.closeSync=fd=>{
    const path=paths.get(fd); paths.delete(fd); real.closeSync(fd);
    if (!used && (name==='close-error'&&path===logPath || name==='status-close-error'&&path?.startsWith(statusPath+'.'))) {
      used=true; faults.push({operation:'close',target:name==='close-error'?'log':'status',error:'EIO',underlying_fd_closed:true}); throw failure('EIO');
    }
  };
  fs.renameSync=(source,target)=>{
    if (name==='status-rename-error'&&!used) { used=true; faults.push({operation:'rename',target:'status',error:'EIO'}); throw failure('EIO'); }
    return real.renameSync(source,target);
  };
  syncBuiltinESMExports();
  const emitted=[]; channel('aries.tool.identity.v1').subscribe(identity=>emitted.push(identity));
  const {createToolIdentity}=await import(pathToFileURL(helper));
  const business=[];
  const execute=()=>{
    calls++;
    const identity=createToolIdentity(name==='bad-tool-id'?'bad id':'tool-'+calls,name==='bad-session'?'wrong-session':'producer-fault-session');
    assert.deepEqual(Object.keys(identity).sort(),['agent_session_id','native_invocation_id','schema_version','scope_id','tool_call_id']);
    assert.equal(identity.scope_id,'producer-fault-scope'); assert.equal(identity.agent_session_id,'producer-fault-session');
    const result=spawnSync('/bin/sh',['-c','printf business-out; printf business-err >&2; exit 7'],{env:{PATH:'/usr/bin:/bin',HOME:'/tmp'}});
    assert.equal(result.stdout.toString(),'business-out'); assert.equal(result.stderr.toString(),'business-err'); assert.equal(result.status,7);
    business.push({stdout_base64:result.stdout.toString('base64'),stderr_base64:result.stderr.toString('base64'),exit_code:result.status});
  };
  const began=performance.now();
  if (name==='bad-session'||name==='bad-tool-id'||name==='legacy-enospc') {
    assert.throws(execute,name==='legacy-enospc'?/injected producer ENOSPC/:/identity scope mismatch/);
    assert.equal(business.length,0); assert.equal(emitted.length,0);
  } else {
    execute();
    if (name==='same-instance-rollback') {
      const old=JSON.parse(real.readFileSync(statusPath,'utf8'));
      old.attempted=0; old.written=0;
      fs.writeFileSync(statusPath,JSON.stringify(old));
      execute();
    }
    if (['healthy','enospc','status-short-write','status-close-error','status-rename-error','stale-clean-status'].includes(name)) execute();
    assert.equal(emitted.length,business.length);
  }
  const elapsed=performance.now()-began;
  if (reader!==undefined) real.closeSync(reader);
  const statusExists=fs.existsSync(statusPath)&&fs.lstatSync(statusPath).isFile();
  const status=statusExists?JSON.parse(real.readFileSync(statusPath,'utf8')):null;
  const logExists=fs.existsSync(logPath)&&fs.lstatSync(logPath).isFile();
  const bytes=logExists?real.readFileSync(logPath):null;
  if (name==='healthy') assert.deepEqual([status.attempted,status.written,status.write_failed,status.status_write_failed,status.failed_latched],[2,2,0,0,false]);
  else if (name==='stale-clean-status') {
    assert.equal(business.length,2); assert.deepEqual([status.attempted,status.written,status.failed_latched],[1,1,false]);
  } else if (name==='same-instance-rollback') {
    assert.deepEqual([status.attempted,status.written,status.write_failed,status.status_write_failed,status.failed_latched],[2,2,0,0,true]);
  } else if (['status-always-fails','real-enospc','bad-session','bad-tool-id','legacy-enospc'].includes(name)) assert.equal(status,null);
  else {
    assert.ok(status.failed_latched);
    assert.equal(status.attempted,status.written+status.write_failed);
    if (['enospc','short-write','close-error','log-mode','log-cap','log-symlink','log-hardlink','log-fifo','log-fifo-reader'].includes(name)) assert.equal(status.write_failed,1);
    if (['status-short-write','status-close-error','status-rename-error'].includes(name)) assert.equal(status.status_write_failed,1);
  }
  if (status) { assert.equal(fs.statSync(statusPath).mode&0o777,0o600); assert.equal(status.schema_version,1); }
  if (name.endsWith('symlink') || name==='log-hardlink') assert.equal(real.readFileSync(sentinel,'utf8'),'preserve');
  if (name.includes('fifo')) assert.ok(elapsed<5000,'FIFO must not block before fstat');
  assert.equal(fs.readdirSync('/tmp').filter(x=>x.startsWith('aries-native-tool-identities.status.json.')&&x.endsWith('.tmp')).length,0);
  console.log(JSON.stringify({case:name,status:'PASS_EXPECTED_BEHAVIOR',calls,business,producer_status:status,identities:emitted,
    faults,real_enospc:realEnospc,elapsed_ms:elapsed,log_bytes:bytes?.length??null,log_sha256:bytes?createHash('sha256').update(bytes).digest('hex'):null,
    log_text:bytes&&bytes.length<8192?bytes.toString():null,
    limitation:name==='stale-clean-status'?'STALE_CLEAN_STATUS_REQUIRES_INDEPENDENT_CALL_DENOMINATOR':name==='status-always-fails'||name==='real-enospc'?'STATUS_NOT_DURABLE':null}));
}

// Standalone original-native B candidate: real local process tools, no SSH identity patch.
import { r as createExecTool } from '/app/dist/bash-tools-D7H1rgMh.js';
import assert from 'node:assert/strict';
const tool=createExecTool({host:'gateway',cwd:'/tmp',security:'full',ask:'off',allowBackground:true,notifyOnExit:false});
const cases=[['forward','printf out; sleep .05; printf err >&2; exit 7','outerr\n\n(Command exited with code 7)'],
  ['reverse','printf err >&2; sleep .05; printf out; exit 7','outerr\n\n(Command exited with code 7)'],
  ['empty','true',''],['unicode','printf 中文🙂; printf 后𝌆 >&2','中文🙂后𝌆']];
const results=await Promise.all(cases.map(async([id,command,expected])=>{
  const response=await tool.execute(id,{command,timeout:5});
  assert.equal(response.details.outputRenderer,'stdout-stderr-v1');
  assert.equal(response.details.aggregated,expected);
  assert.equal(response.content[0].text,expected||'(no output)');
  assert.equal(response.details.exitCode,id==='forward'||id==='reverse'?7:0);
  return {id,response};
}));
console.log(JSON.stringify({status:'PASS_ORIGINAL_NATIVE_B_FOREGROUND',allow_background:true,results}));

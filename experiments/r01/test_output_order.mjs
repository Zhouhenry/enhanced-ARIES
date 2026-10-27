import assert from 'node:assert/strict';
import { test } from 'node:test';
import { createFinalOutput, OUTPUT_RENDERER, MAX_ANSI_TOKEN } from '/app/dist/aries-output-order-poc.mjs';
import { o as sanitizeBinaryOutput } from '/app/dist/shell-utils-D5xjUWUG.js';
import { p as tail } from '/app/dist/bash-process-registry-17q1dHVV.js';

function render(events, max = 1000) {
  const view = createFinalOutput(max);
  for (const [stream, chunk] of events) {
    view.write(stream, chunk);
    for (const retained of Object.values(view.retained())) {
      assert.ok(retained.text_chars <= max + 2);
      assert.ok(retained.pending_chars <= MAX_ANSI_TOKEN);
    }
  }
  const result = view.finish();
  assert.equal(result.renderer, OUTPUT_RENDERER);
  assert.strictEqual(view.finish(), result);
  view.write('stdout', 'late');
  assert.strictEqual(view.finish(), result);
  return result;
}

test('stdout/stderr cross-stream order changes cannot change final text', () => {
  for (const events of [
    [['stdout','out'],['stderr','err']], [['stderr','err'],['stdout','out']],
    [['stderr','e'],['stdout','o'],['stderr','rr'],['stdout','ut']],
  ]) assert.equal(render(events).text, 'outerr');
});

test('all split positions match real native full-stream sanitizer, including incomplete ANSI', () => {
  const inputs = ['', 'ordinary\r\n\t text', '中文🙂𝌆', '\u0000\u0001\u001f\u007f\u009f\u200b',
    '\x1b[31mred\x1b[0m', '\x1b]title\x07ok', '\x1b]title\x1b\\ok',
    '\u009dtitle\u009cOK', '\u009b31mred\u009bm', '\x1b[', '\x1b]', '\x1b]unfinished\x1b',
    'a\x1b]bad\x1b[32mgood', '\x1b[12\x00m', 'x\u009b\u0001y', '\ud800', '\udc00'];
  for (const input of inputs) {
    const expected = sanitizeBinaryOutput(input);
    for (let split = 0; split <= input.length; split++) {
      const result = render([['stdout', input.slice(0,split)], ['stdout', input.slice(split)]]);
      assert.equal(result.error, null, JSON.stringify({input,split}));
      assert.equal(result.text, expected, JSON.stringify({input,split}));
    }
    assert.equal(render(Array.from({length: input.length}, (_,i) => ['stdout',input[i]])).text, expected);
  }
});

test('same per-stream text with every Unicode split and deterministic capped tail', () => {
  const stdout = '前🙂'.repeat(100), stderr = '后𝌆'.repeat(100);
  for (const max of [1,2,3,7,100,401,403,450,799,1000]) {
    const expected = tail(sanitizeBinaryOutput(stdout) + sanitizeBinaryOutput(stderr), max);
    const events = [];
    for (let i=0;i<Math.max(stdout.length,stderr.length);i++) {
      events.push(['stderr',stderr[i] || ''], ['stdout',stdout[i] || '']);
    }
    const result = render(events,max);
    assert.equal(result.text,expected);
    assert.equal(result.truncated,stdout.length+stderr.length>max);
  }
});

test('ANSI token bound has identical failure for complete, split and bytewise input', () => {
  for (const token of ['\x1b]'+'a'.repeat(MAX_ANSI_TOKEN)+'\x07', '\x1b['+'1'.repeat(MAX_ANSI_TOKEN)+'m',
    '\x1b]'+'a'.repeat(MAX_ANSI_TOKEN)+'\x1b']) {
    const variants = [[['stdout',token]], [['stdout',token.slice(0,4000)],['stdout',token.slice(4000)]],
      [...token].map(c=>['stdout',c])];
    for (const events of variants) {
      const result = render(events);
      assert.equal(result.error,'ansi_token_limit');
      assert.equal(result.text,'');
    }
  }
  assert.equal(render([['stdout','\x1b]'+'a'.repeat(MAX_ANSI_TOKEN-3)+'\x07']]).error,null);
  for (const n of [8191,8192,8193]) {
    for (const suffix of ['\x07','','\x1b']) {
      const token='\x1b]'+'x'.repeat(n-2-suffix.length)+suffix;
      for (const split of [0,1,8190,8191,8192,8193]) {
        const result=render([['stdout',token.slice(0,split)],['stdout',token.slice(split)]]);
        assert.equal(result.error,n>MAX_ANSI_TOKEN?'ansi_token_limit':null,JSON.stringify({n,suffix,split}));
        if (!result.error) assert.equal(result.text,tail(sanitizeBinaryOutput(token),1000));
      }
    }
  }
});

test('large ordinary output is bounded and exactly matches declared final cap', () => {
  const view = createFinalOutput(1000);
  for (let i=0;i<2000;i++) {
    view.write('stdout','a'.repeat(500)); view.write('stderr','b'.repeat(500));
    for (const r of Object.values(view.retained())) assert.ok(r.text_chars <= 1002 && r.pending_chars <= 8192);
  }
  assert.deepEqual(view.finish(), {renderer:OUTPUT_RENDERER,text:'b'.repeat(1000),error:null,truncated:true});
});

// Independent final-result view; native live output and process control stay untouched.
import { o as sanitizeBinaryOutput } from './shell-utils-D5xjUWUG.js';
import { p as tail } from './bash-process-registry-17q1dHVV.js';

export const OUTPUT_RENDERER = 'stdout-stderr-v1';
export const MAX_ANSI_TOKEN = 8192;

function streamView(maxChars) {
  let pending = '', value = '', error = null, characters = 0;
  const emit = text => {
    const clean = sanitizeBinaryOutput(text);
    characters += clean.length;
    // Two extra code units keep a surrogate pair at the final truncation boundary.
    value = tail(value + clean, maxChars + 2);
  };
  const write = (chunk, eof = false) => {
    if (error) return;
    const text = pending + chunk;
    pending = '';
    let i = 0, plain = 0;
    const hold = () => {
      emit(text.slice(plain, i));
      pending = text.slice(i);
    };
    while (i < text.length) {
      const first = text.charCodeAt(i);
      if (!eof && i === text.length - 1 && first >= 0xd800 && first <= 0xdbff) {
        hold();
        return;
      }
      let body = i, kind = '';
      if (first === 0x1b) {
        if (i + 1 === text.length && !eof) { hold(); return; }
        if (text[i + 1] === '[') { body = i + 2; kind = 'csi'; }
        if (text[i + 1] === ']') { body = i + 2; kind = 'osc'; }
      } else if (first === 0x9b) { body = i + 1; kind = 'c1-csi'; }
      else if (first === 0x9d) { body = i + 1; kind = 'osc'; }
      if (!kind) { i++; continue; }
      let end = body, complete = false, unfinished = false;
      if (kind.endsWith('csi')) {
        while (end < text.length && text.charCodeAt(end) >= 0x20 && text.charCodeAt(end) <= 0x3f) end++;
        unfinished = end === text.length;
        complete = !unfinished && text.charCodeAt(end) >= 0x40 && text.charCodeAt(end) <= 0x7e &&
          (kind !== 'c1-csi' || end > body);
        if (complete) end++;
      } else {
        while (end < text.length && ![0x07, 0x1b, 0x9c].includes(text.charCodeAt(end))) end++;
        unfinished = end === text.length || (text.charCodeAt(end) === 0x1b && end + 1 === text.length);
        if (!unfinished) {
          complete = text.charCodeAt(end) !== 0x1b || text[end + 1] === '\\';
          if (complete) end += text.charCodeAt(end) === 0x1b ? 2 : 1;
        }
      }
      // The same limit applies to complete tokens and pending tokens, regardless of chunking.
      if ((unfinished ? text.length : end) - i > MAX_ANSI_TOKEN) {
        error = 'ansi_token_limit'; pending = ''; value = ''; return;
      }
      if (unfinished && !eof) { hold(); return; }
      if (complete) {
        emit(text.slice(plain, i));
        emit(text.slice(i, end)); // The pinned native sanitizer owns character/ANSI semantics.
        i = end; plain = end;
      } else {
        // Invalid/incomplete sequences remain literal, as in the pinned sanitizer.
        i = Math.max(body, end);
      }
    }
    emit(text.slice(plain));
  };
  return {
    write,
    finish() { write('', true); return {text: value, error, characters}; },
    retained() { return {text_chars: value.length, pending_chars: pending.length}; },
  };
}

export function createFinalOutput(maxChars) {
  if (!Number.isInteger(maxChars) || maxChars < 1 || maxChars > 200000) throw new Error('invalid output limit');
  const streams = {stdout: streamView(maxChars), stderr: streamView(maxChars)};
  let finished;
  return {
    write(stream, data) {
      // Native wait defines the final snapshot; late callbacks still follow its unchanged live path.
      if (finished) return;
      if (!Object.hasOwn(streams, stream) || typeof data !== 'string') throw new Error('invalid output stream');
      streams[stream].write(data);
    },
    finish() {
      if (finished) return finished;
      const stdout = streams.stdout.finish(), stderr = streams.stderr.finish();
      const error = stdout.error || stderr.error;
      return finished = {renderer: OUTPUT_RENDERER, text: error ? '' : tail(stdout.text + stderr.text, maxChars),
        error, truncated: stdout.characters + stderr.characters > maxChars};
    },
    retained() { return {stdout: streams.stdout.retained(), stderr: streams.stderr.retained()}; },
  };
}

export function attachFinalOutput(outcome, rendered) {
  if (!rendered) return outcome;
  outcome.ariesOutputRenderer = rendered.renderer;
  outcome.ariesOutputTruncated = rendered.truncated;
  if (rendered.error) outcome.ariesOutputError = rendered.error;
  return outcome;
}

"""Add first nonempty pipe observations to the pinned native process candidate.

The original data callbacks still own decoding and delivery; no extra reader,
listener, shell wrapper or ToolResult transformation is introduced.
"""
import argparse
import difflib
import hashlib
import json
from pathlib import Path

BASE_MANIFEST = '712eedb551156cbd49c066ae0ce595ac7d66920d0f662575db80b93cec32853d'
SUPERVISOR = 'supervisor-AWzFqBCH.js'


def sha(data):
    return hashlib.sha256(data).hexdigest()


def once(text, before, after):
    assert text.count(before) == 1, 'pinned native source anchor changed'
    return text.replace(before, after, 1)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--source', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    raw_manifest = (args.source / 'manifest.json').read_bytes()
    assert sha(raw_manifest) == BASE_MANIFEST
    prior = json.loads(raw_manifest)
    args.output.mkdir(mode=0o700, parents=True, exist_ok=False)
    records, differences = {}, []
    for name, row in prior['files'].items():
        original = (args.source / name).read_bytes()
        assert sha(original) == row['output_sha256']
        text = original.decode()
        if name == SUPERVISOR:
            text = once(text, 'observation.sink.emit({ schema_version: 1,',
                        'observation.sink.emit({ schema_version: 2,')
            for stream in ('stdout', 'stderr'):
                declaration = '\tconst on' + stream.capitalize() + ' = (listener) => {\n'
                # There is also a PTY onStdout. Limit edits to the child adapter.
                child, pty = text.split('//#region src/process/supervisor/adapters/pty.ts', 1)
                child = once(child, declaration,
                             '\tlet aries' + stream.capitalize() + 'Observed = false;\n' + declaration)
                callback = '\t\tchild.' + stream + '.on("data", (chunk) => {\n'
                child = once(child, callback, callback +
                    '\t\t\tif (!aries' + stream.capitalize() + 'Observed && chunk.length > 0) {\n' +
                    '\t\t\t\taries' + stream.capitalize() + 'Observed = true;\n' +
                    '\t\t\t\tariesObserve("' + stream + '_first_nonempty");\n' +
                    '\t\t\t}\n')
                text = child + '//#region src/process/supervisor/adapters/pty.ts' + pty
            assert text.split('//#region src/process/supervisor/adapters/pty.ts', 1)[1] == original.decode().split('//#region src/process/supervisor/adapters/pty.ts', 1)[1]
        value = text.encode()
        (args.output / name).write_bytes(value)
        records[name] = {'input_sha256': sha(original), 'output_sha256': sha(value),
                         'restore_source': str(args.source / name)}
        differences.extend(difflib.unified_diff(original.decode().splitlines(True), text.splitlines(True),
                           fromfile='schema1/' + name, tofile='schema2/' + name))
    (args.output / 'manifest.json').write_text(json.dumps({
        'status': 'PRIVATE_NATIVE_FIRST_OUTPUT_CANDIDATE_UNTESTED', 'files': records,
        'prior_manifest_sha256': BASE_MANIFEST, 'schema_version': 2,
        'scope': 'first nonempty raw data callback per stdout/stderr before native decoding',
        'pty_adapter_byte_identical': True,
        'unchanged': ['data listeners', 'decoding', 'ToolResult', 'argv/env/stdio', 'signals', 'background/poll/PTY']
    }, indent=2) + '\n')
    (args.output / 'changes.diff').write_text(''.join(differences))


if __name__ == '__main__':
    main()

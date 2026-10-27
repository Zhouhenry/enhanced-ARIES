"""Reconcile the independent producer witness using IDs, never command similarity."""
from collections import Counter
from copy import deepcopy
import hashlib
import json
from pathlib import Path
import sys


def lines(path):
    return [json.loads(line) for line in path.read_text().splitlines() if line.strip()]


def reconcile(root):
    native = {name: lines(root / (name + '-native.log'))[-1] for name in ('baseline', 'candidate')}
    target = {name: lines(root / name / 'target-events.jsonl') for name in native}
    audit = {name: lines(root / name / 'tool-calls.jsonl') for name in native}
    expected = [(b'same', b'', 0), (b'same', b'', 0), (b'concurrent', b'', 0),
                (b'concurrent', b'', 0), (b'retry', b'', 0), (b'retry', b'', 0),
                (b'slow', b'', 0), (b'fast', b'', 0), (b'out', b'err', 7)]
    for name in native:
        assert len(native[name]['results']) == len(expected)
        assert native[name]['metadata_directories_remaining'] == 0
        assert {r['case_id'] for r in native[name]['results']} == set(range(1, 10))
    assert all('native_identity' not in r for r in native['baseline']['results'])
    assert all(not r['linked'] for r in target['baseline'])
    assert all(r['identity_status'] == 'unlinked' and not r.get('native_identity') for r in audit['baseline'])
    by_native = {}
    for row in target['candidate']:
        if row['linked']:
            key = row['native_identity']['native_invocation_id']
            assert key not in by_native
            by_native[key] = row
        else:
            assert not row['native_identity'].get('native_invocation_id')
    assert len(by_native) == 9
    by_execution = {}
    for row in audit['candidate']:
        if row.get('execution_id'):
            assert row['execution_id'] not in by_execution
            by_execution[row['execution_id']] = row
        if not row.get('native_identity'):
            assert row['identity_status'] == 'unlinked'
    mappings = []
    producer_ids = set()
    ssh_ids = set()
    for call in native['candidate']['results']:
        identity = call['native_identity']
        assert identity['schema_version'] == 1
        assert identity['scope_id'] == 'r01-poc-candidate'
        assert identity['agent_session_id'] == 'r01-session'
        assert identity['tool_call_id'] == call['tool_call_id']
        key = identity['native_invocation_id']
        assert key not in producer_ids
        producer_ids.add(key)
        row = by_native[key]
        wire = dict(row['native_identity'])
        ssh_id = wire.pop('ssh_request_id')
        assert ssh_id and ssh_id not in ssh_ids
        ssh_ids.add(ssh_id)
        assert wire == identity
        bridge = by_execution[row['execution_id']]
        assert bridge['identity_status'] == 'exact_channel'
        assert bridge['native_identity'] == row['native_identity']
        stdout, stderr, code = expected[call['case_id'] - 1]
        assert row['stdout_sha256'] == hashlib.sha256(stdout).hexdigest()
        assert row['stderr_sha256'] == hashlib.sha256(stderr).hexdigest()
        assert not row['transport_error']
        assert row['exit_code'] == bridge['exit_code'] == call['response']['details']['exitCode'] == code
        assert bridge['stdout_bytes'] == len(stdout) and bridge['stderr_bytes'] == len(stderr)
        assert bridge['status'] == 'completed'
        mappings.append({'case_id': call['case_id'], 'native_identity': row['native_identity'],
                         'execution_id': row['execution_id'], 'join': 'exact_id', 'exit_code': code})
    assert producer_ids == set(by_native)
    assert sum(r['identity_status'] == 'exact_channel' for r in audit['candidate']) == 9
    # Fixture case IDs compare two controlled runs, never associate transport events.
    baseline_calls = {r['case_id']: r for r in native['baseline']['results']}
    result_differences = []
    for call in native['candidate']['results']:
        one = deepcopy(call['response'])
        two = deepcopy(baseline_calls[call['case_id']]['response'])
        for response in (one, two):
            assert response['details'].pop('durationMs') >= 0
        if one != two:
            result_differences.append({'case_id': call['case_id'],
                'baseline_sha256': hashlib.sha256(json.dumps(two, sort_keys=True).encode()).hexdigest(),
                'candidate_sha256': hashlib.sha256(json.dumps(one, sort_keys=True).encode()).hexdigest()})
    # This multiset only tests unchanged command/IO semantics; no identity joins use it.
    def semantics(rows):
        fields = ('command', 'exit_code', 'transport_error', 'stdout_sha256', 'stderr_sha256')
        return Counter(json.dumps({k: r[k] for k in fields}, sort_keys=True) for r in rows)
    assert semantics(target['baseline']) == semantics(target['candidate'])
    return {'schema_version': 1, 'status': 'FAIL_NATIVE_RESULT_EQUIVALENCE' if result_differences else 'PASS_NATIVE_DOCKER_POC_ONLY',
            'expected_native_calls': 9, 'exact_mappings': mappings,
            'unlinked_auxiliary_target_calls': len(target['candidate']) - 9,
            'baseline_candidate_native_semantics_equal': not result_differences,
            'native_result_differences': result_differences,
            'identity_and_split_stream_io_exit_checks': 'PASS_9_OF_9',
            'target_command_io_multiset_equal': True,
            'timing_field_excluded_from_semantic_equality': 'ToolResult.details.durationMs',
            'not_covered': ['LLM/Gateway dispatch', 'real E2B/Huawei guest', 'A Strict Replay and independent Verifier',
                            'native cancellation and native malformed-metadata fault injection']}


if __name__ == '__main__':
    root = Path(sys.argv[1])
    output = root / (sys.argv[2] if len(sys.argv) > 2 else 'reconciliation.json')
    result = reconcile(root)
    with output.open('x') as file:
        json.dump(result, file, indent=2, sort_keys=True)
        file.write('\n')
    if result['status'].startswith('FAIL'):
        raise SystemExit(1)

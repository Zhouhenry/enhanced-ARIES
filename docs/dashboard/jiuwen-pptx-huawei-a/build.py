"""Assemble this fixed, evidence-bound dashboard on ECS; never run an Agent."""
from pathlib import Path
import argparse
import gzip
import hashlib
import json
import re
import shlex
import shutil
import sys
from collections import defaultdict

assert sys.platform.startswith('linux'), 'Build only on the authorized ECS.'
p = argparse.ArgumentParser()
p.add_argument('--artifact-root', type=Path, default=Path('/opt/aries-artifacts'))
p.add_argument('--analysis', type=Path, required=True)
p.add_argument('--comparison', type=Path, required=True)
p.add_argument('--recorded-summary', type=Path)
p.add_argument('--recorded-summary-sha')
p.add_argument('--recorded-comparison', type=Path)
p.add_argument('--recorded-comparison-sha')
p.add_argument('--e2b-recorded-summary', type=Path)
p.add_argument('--e2b-recorded-summary-sha')
args = p.parse_args()
out = Path(__file__).resolve().parent
artifacts = args.artifact_root
backend = artifacts / 'jiuwen-pptx-huawei-a-workload-backends-1'
sha = lambda b: hashlib.sha256(b).hexdigest()
read = lambda path: json.loads(Path(path).read_bytes())
write = lambda path, obj: Path(path).write_text(json.dumps(obj, ensure_ascii=False, indent=2) + '\n')
for name in ('traces', 'downloads', 'evidence'):
    (out / name).mkdir(exist_ok=True)
inputs = {}

def verified(path, expected=None):
    b = path.read_bytes()
    h = sha(b)
    if expected:
        assert h == expected, path.name + ': source SHA mismatch'
    inputs[str(path)] = {'sha256': h, 'bytes': len(b)}
    return json.loads(b)

specs = [
    ('huawei', '华为云', 'jiuwen-huawei-external-pptx-workload-replay-1', 'a8d59d52471b20a5f858bf9c1de7cccc85b8eda7be983f7abb89825b9c5d0d9a'),
    ('e2b', 'E2B', 'jiuwen-e2b-external-pptx-workload-replay-1', 'da9991606c791e84707792869382da524eae377673fd853bd0bfe2520dd345b7'),
]
summaries = []
runs = []
for key, label, name, expected in specs:
    root = artifacts / name
    source = root / 'acceptance-summary.json'
    d = verified(source, expected)
    summaries.append(d)
    assert d['verifier']['score'] == 1 and d['cleanup']['provider_absence_confirmed']
    assert d['tape']['source']['sha256'] == '6a15976b408e008a238c34e60e417006a989963b42e4f4db1ea042bd13df9c17'
    stages = {x['source_field']: x['duration_ns'] for x in d['stages']}
    queue = stages.pop('queue_duration')
    streams = {s['actor']: s for s in d['observation']['resources']['streams']}
    controller, agent, provider = (streams[k] for k in ('controller_shared_process', 'agent_container', 'task_sandbox'))
    metrics = lambda s: {m['name']: m for m in s['metrics']}
    cm, am, pm = metrics(controller), metrics(agent), metrics(provider)
    cpu_delta = sum(x['value'] for x in controller['snapshot_boundary_deltas'] if x['name'] in ('cpu_user_ns', 'cpu_system_ns'))
    resources = {
        'controller_cpu_s': cpu_delta / 1e9,
        'controller_rss_mib': cm['process_lifetime_peak_rss_bytes']['maximum_returned'] / 1048576,
        'agent_cpu_s': (am['cpu_usage_nanoseconds']['last_returned'] - am['cpu_usage_nanoseconds']['first_returned']) / 1e9,
        'agent_memory_mib': am['memory_usage_bytes']['maximum_returned'] / 1048576,
        'agent_samples': agent['receipt_count'],
        'provider_memory_mib': pm['mem_used_mib']['maximum_returned'] if 'mem_used_mib' in pm else pm['mem_used']['maximum_returned'] / 1048576,
        'provider_cpu_pct': pm['cpu_used_pct']['maximum_returned'],
        'provider_receipts': provider['receipt_count'],
        'provider_repeats': provider['repeated_timestamp_and_value_receipts'],
        'provider_counts': provider.get('provider_counts'),
        'scope': 'summary aggregates only, no synthetic time series',
    }
    runs.append({'id': key, 'label': label, 'run_id': d['run_id'], 'summary_sha': expected,
                 'runtime_ns': stages['runtime_execution_duration'], 'attempt_ns': d['attempt_duration_ns'],
                 'driver_s': d['driver_duration_s'], 'queue_ns': queue, 'stages': stages,
                 'remainder_ns': d['attempt_duration_ns'] - sum(stages.values()), 'resources': resources})
    shutil.copyfile(source, out / 'evidence' / (key + '-acceptance-summary.json'))
    for deck in d['verifier']['pptx']['decks']:
        filename = Path(deck['path']).name
        if filename not in ('final.pptx', 'content.pptx'):
            continue
        origin = root / deck['path']
        raw = origin.read_bytes()
        assert sha(raw) == deck['sha256']
        (out / 'downloads' / (key + '-' + filename)).write_bytes(raw)
        inputs[str(origin)] = {'sha256': sha(raw), 'bytes': len(raw)}

analysis_doc = verified(args.analysis, 'd48c03a911661c744684836cfdfda8ad3e6f2c919efb521a3f106deae755a0f9')
analysis = next(a for a in analysis_doc['results'] if a['summary_sha256'] == specs[0][3])
write(out / 'evidence/runtime-breakdown.json', analysis)
comparison = verified(args.comparison)
assert comparison['source_summary_sha256'] == specs[0][3] and comparison['replay_summary_sha256'] == specs[1][3]
assert not comparison['returncode_differences'] and len(comparison['pairs']) == 32
pairs = {x['tool_call_id']: x for x in comparison['pairs']}
native = [{a['tool_call_id']: a for a in s['tool_actions']['actions']} for s in summaries]
tools = []
for item in summaries[0]['tape']['actions']:
    call = item['tool_call_id']
    assert call in native[0] and call in native[1]
    spans = [m[call]['native_tool_spans'] for m in native]
    assert all(len(a) == 1 for a in spans)
    pair = pairs.get(call)
    tools.append({'tool_call_id': call, 'tool_name': item['tool_name'], 'action': item['action'],
                  'turn': item['turn_index'] + 1, 'ordinal': item['tool_index'] + 1,
                  'locator': item['evidence_locator'], 'command_sha256': item.get('command_sha256'),
                  'returncode': pair['source_returncode'] if pair else None,
                  'huawei_command_ns': pair['source_direct_child_duration_ns'] if pair else None,
                  'e2b_command_ns': pair['replay_direct_child_duration_ns'] if pair else None,
                  'huawei_native_ns': spans[0][0]['wall_duration_ns'], 'e2b_native_ns': spans[1][0]['wall_duration_ns'],
                  'huawei_operation_id': spans[0][0]['operation_id'], 'e2b_operation_id': spans[1][0]['operation_id']})
assert len(tools) == 78 and sum(x['tool_name'] == 'bash' for x in tools) == 33

# Read the existing Tape in ECS memory. Export only program names and safe file
# basenames, never commands, inline scripts, prompts, or tool-result bodies.
tape_source = summaries[0]['tape']['source']
tape = verified(Path(tape_source['source_root']) / tape_source['path'], tape_source['sha256'])
by_id = {x['tool_call_id']: x for x in tools}
known_programs = {'python','python3','node','ls','find','cat','head','tail','sed','cp','mv','mkdir','wc','unzip','zip','libreoffice','soffice','pdftoppm','pdftotext','test','du','file','pwd'}
for turn in tape['turns']:
    for call in turn.get('tool_calls', []):
        if call.get('name') != 'bash' or call['id'] not in by_id:
            continue
        body = json.loads(call['arguments']) if isinstance(call['arguments'], str) else call['arguments']
        command = body.get('command', body.get('cmd', ''))
        if not isinstance(command, str):
            continue
        programs = []
        for fragment in re.split(r'&&|\|\||;|\n', command):
            match = re.match(r'\s*([A-Za-z0-9_./+-]+)(?:\s|$)', fragment)
            name = Path(match[1]).name if match else ''
            if name in known_programs and name not in programs:
                programs.append(name)
        try:
            tokens = shlex.split(command)
        except ValueError:
            tokens = []
        files = []
        for token in tokens:
            if re.fullmatch(r'[A-Za-z0-9_./+-]+\.(?:py|js|mjs|cjs|pptx|pdf|md)', token):
                name = Path(token).name
                if name not in files:
                    files.append(name)
        item = by_id[call['id']]
        item['command_details'] = {'programs': programs, 'files': files[:8], 'scope': 'static command program/file tokens only; no inline code or command body'}
        item['archive_action_category'] = item['action']
        scripts = [f for f in files if f.endswith(('.py','.js','.mjs','.cjs'))]
        if set(programs) and set(programs) <= {'ls','find','pwd'}:
            item['action'] = '查看目录与查找文件'
        elif 'soffice' in programs and 'pdftotext' in programs:
            item['action'] = '转换 PPT 为 PDF，并提取文本检查'
        elif 'cp' in programs:
            item['action'] = '复制项目文件并检查目标'
        elif set(programs) == {'mkdir','ls'}:
            item['action'] = '建立输出目录并检查文件'
        elif set(programs) == {'tail'}:
            item['action'] = '读取文件末尾'
        elif scripts and 'finalize_deck.py' in scripts:
            item['action'] = '运行演示稿定稿脚本'
        elif 'node' in programs:
            item['action'] = '运行 Node 脚本处理演示项目'
        elif 'python3' in programs or 'python' in programs:
            item['action'] = '运行 Python 检查或处理演示文件'
        if scripts:
            item['action'] += ' · ' + ' / '.join(scripts[:2])
        elif files:
            item['action'] += ' · ' + ' / '.join(files[:2])

backend_file = backend / 'evidence.json'
backend_proof = verified(backend_file, '5ba72bf48657dfb09f483e336c2e060e82c08a7d1ad515a20c0ef154e7b7126a')
assert str(backend_proof['status']).startswith('PASS'), 'Actual backend receipt must pass'
shutil.copyfile(backend_file, out / 'evidence/backend-evidence.json')
for filename, expected in (
    ('metrics-evidence.json', 'fc53062ada3129317e06fe638b98df9bfc6c23aebf92a38b06b4e8ccd2a4457d'),
    ('postflight.json', '945701aa1d91c8694529d7470e365ac1fe65c48f3bffd1114e921c5d607caa09'),
):
    proof = verified(backend / filename, expected)
    assert proof['status'].startswith('PASS')
    shutil.copyfile(backend / filename, out / 'evidence' / filename)
trace_specs = [
 ('controller', 'Controller · 模型 / Bridge / 生命周期', 'controller-trace.json', 'a8f196f5cc146801fcb2c58a075c5625a05160f75b85fc73110d1f2a908d5a98'),
 ('native', 'Agent 原生 · 工具与调用父子关系', 'native-trace.json', '98d09466d2e910565967ab127df8e6313733d7bef581f4ef7af6c6dded6317b9'),
 ('process', 'Sandbox 命令 · 创建 / 直接子进程 / 管道', 'process-0.json', 'bdfa58514a09bd17807e83591c362619bd9061ebcaacc30dfecd162e6a2a6c08'),
 ('worker', 'Sandbox Worker · 原生操作请求', 'worker-0.json', '2d6d85f6b68463d9584765cc76ba0cd2b9a61bf755537c7688fc8b53ee6b57f3'),
 ('provider', 'Agent Provider · 传输请求', 'provider-0.json', 'dcecca03206b80c2c416850955604af7321b5f77ac580bb15045dbc5a16e3f6a'),
]
safe_args = {'operation_id','tool_call_id','clock_domain','status','duration_ns','measurement_scope','process_id','native_trace_id','native_span_id','native_parent_span_id','start_time_unix_nano','end_time_unix_nano','native_status_code'}
traces = []
for key, label, filename, expected in trace_specs:
    source = backend / filename
    trace = verified(source, expected)
    raw = source.read_bytes()
    zipped = gzip.compress(raw, compresslevel=6, mtime=0)
    target = 'traces/' + filename + '.gz'
    (out / target).write_bytes(zipped)
    assert sha(gzip.decompress(zipped)) == expected
    spans = []
    for e in trace['traceEvents']:
        if e.get('ph') != 'X':
            continue
        spans.append({**{k:e[k] for k in ('name','ts','dur','pid','tid') if k in e},
                      'args': {k:v for k,v in e.get('args',{}).items() if k in safe_args}})
    spans.sort(key=lambda e:(e['ts'],e.get('tid',0)))
    for n, span in enumerate(spans):
        span['preview_index'] = n
    assert spans and all(x['dur'] >= 0 for x in spans)
    traces.append({'id': key, 'label': label, 'file': target, 'bytes': len(raw), 'gzip_bytes': len(zipped),
                   'sha256': expected, 'gzip_sha256': sha(zipped), 'clock_domain': trace['metadata']['clock_domain'],
                   'snapshot_id': trace['metadata']['snapshot_id'], 'spans': spans,
                   'first_ts': min(x['ts'] for x in spans), 'last_ts': max(x['ts']+x['dur'] for x in spans),
                   'scope': 'preview is original complete intervals with selected identity fields; gzip is full original export'})
data = {'schema_version': 2, 'tape_sha': summaries[0]['tape']['source']['sha256'],
        'runs': runs, 'tools': tools, 'analysis': analysis,
        'backend': {'status': 'PASS', 'sha256': inputs[str(backend_file)]['sha256']}, 'traces': traces}
if args.recorded_summary:
    assert args.recorded_summary_sha and args.recorded_comparison and args.recorded_comparison_sha
    recorded = verified(args.recorded_summary, args.recorded_summary_sha)
    timing_comparison = verified(args.recorded_comparison, args.recorded_comparison_sha)
    detail = next(r for r in timing_comparison['results'] if r['timing'] == 'recorded')
    assert detail['summary_sha256'] == args.recorded_summary_sha
    assert recorded['tape']['source']['sha256'] == data['tape_sha'] == timing_comparison['same_tape_sha256']
    assert recorded['replay']['timing'] == 'recorded' and recorded['verifier']['score'] == 1
    assert recorded['cleanup']['provider_absence_confirmed']
    recorded_stages = {x['source_field']: x['duration_ns'] for x in recorded['stages']}
    recorded_queue = recorded_stages.pop('queue_duration')
    recorded_streams = {s['actor']: s for s in recorded['observation']['resources']['streams']}
    rc, ra, rp = (recorded_streams[k] for k in ('controller_shared_process', 'agent_container', 'task_sandbox'))
    metric = lambda s: {m['name']: m for m in s['metrics']}
    rcm, ram, rpm = metric(rc), metric(ra), metric(rp)
    recorded_resources = {
        'controller_cpu_s': sum(x['value'] for x in rc['snapshot_boundary_deltas'] if x['name'] in ('cpu_user_ns', 'cpu_system_ns')) / 1e9,
        'controller_rss_mib': rcm['process_lifetime_peak_rss_bytes']['maximum_returned'] / 1048576,
        'agent_cpu_s': (ram['cpu_usage_nanoseconds']['last_returned'] - ram['cpu_usage_nanoseconds']['first_returned']) / 1e9,
        'agent_memory_mib': ram['memory_usage_bytes']['maximum_returned'] / 1048576,
        'agent_samples': ra['receipt_count'],
        'provider_memory_mib': rpm['mem_used_mib']['maximum_returned'] if 'mem_used_mib' in rpm else rpm['mem_used']['maximum_returned'] / 1048576,
        'provider_cpu_pct': rpm['cpu_used_pct']['maximum_returned'],
        'provider_receipts': rp['receipt_count'],
        'provider_repeats': rp['repeated_timestamp_and_value_receipts'],
        'provider_counts': rp.get('provider_counts'),
    }
    recorded_native = {a['tool_call_id']: a for a in recorded['tool_actions']['actions']}
    recorded_commands = [i for i in recorded['observation']['process']['intervals'] if i['name'] == 'jiuwen.process.command']
    commands_by_id = {i['tool_call_id']: i for i in recorded_commands}
    assert len(commands_by_id) == len(recorded_commands)
    assert set(recorded_native) == {t['tool_call_id'] for t in tools}
    native_groups = defaultdict(list)
    recorded_tools = []
    operation_ids = set()
    for source_tool in tools:
        call_id = source_tool['tool_call_id']
        spans = recorded_native[call_id]['native_tool_spans']
        assert len(spans) == 1
        span = spans[0]
        assert span['operation_id'] not in operation_ids
        operation_ids.add(span['operation_id'])
        native_groups[source_tool['tool_name']].append(span['wall_duration_ns'])
        command = commands_by_id.get(call_id)
        item = {k: v for k, v in source_tool.items() if not k.startswith(('huawei_', 'e2b_')) and k != 'returncode'}
        item.update(huawei_native_ns=span['wall_duration_ns'], huawei_operation_id=span['operation_id'],
                    huawei_command_ns=command['duration_ns'] if command else None,
                    returncode=command['attributes'].get('returncode') if command else None)
        recorded_tools.append(item)
    native_aggregates = [{'tool_name': name, 'count': len(values), 'sum_ns': sum(values), 'maximum_ns': max(values)} for name, values in sorted(native_groups.items())]
    # The existing small summary preserves native durations, not native start/end.
    # Do not fabricate a same-clock union from the cumulative duration.
    recorded_analysis = {
        'summary_sha256': args.recorded_summary_sha,
        'models': [{'name': 'model.http_request', 'count': detail['model']['requests'],
                    'sum_ns': detail['model']['http_handler_duration_sum_ns']}],
        'authoritative_native_tools': native_aggregates,
        'authoritative_native_tools_total': {'count': len(recorded_tools), 'sum_ns': sum(x['sum_ns'] for x in native_aggregates), 'same_clock_unions': []},
        'command_intervals': {'count': len(recorded_commands), 'sum_ns': sum(i['duration_ns'] for i in recorded_commands)},
        'scope': 'Exact current-attempt native ToolCall and process durations; no union or missing model subwindows inferred',
    }
    assert recorded_analysis['authoritative_native_tools_total']['sum_ns'] == detail['native_tool_duration_sum_ns']
    assert recorded_analysis['command_intervals']['sum_ns'] == detail['command_duration_sum_ns']
    data['recorded'] = {
        'run': {'id': 'huawei-recorded', 'label': '华为云 recorded', 'run_id': recorded['run_id'], 'summary_sha': args.recorded_summary_sha,
                'runtime_ns': recorded_stages['runtime_execution_duration'], 'attempt_ns': recorded['attempt_duration_ns'],
                'driver_s': recorded['driver_duration_s'], 'queue_ns': recorded_queue, 'stages': recorded_stages,
                'remainder_ns': recorded['attempt_duration_ns'] - sum(recorded_stages.values()), 'resources': recorded_resources},
        'analysis': recorded_analysis, 'tools': recorded_tools, 'ppt': 'downloads/huawei-recorded-final.pptx',
        'backend_status': 'UNRUN', 'model_timing': detail['model'], 'environment_note': timing_comparison['environment_note'],
    }
    for deck in recorded['verifier']['pptx']['decks']:
        filename = Path(deck['path']).name
        if filename in ('final.pptx', 'content.pptx'):
            origin = args.recorded_summary.parent / deck['path']
            raw = origin.read_bytes()
            assert sha(raw) == deck['sha256']
            (out / 'downloads' / ('huawei-recorded-' + filename)).write_bytes(raw)
            inputs[str(origin)] = {'sha256': sha(raw), 'bytes': len(raw)}
    assert (out / data['recorded']['ppt']).exists()
    shutil.copyfile(args.recorded_summary, out / 'evidence/huawei-recorded-acceptance-summary.json')
    shutil.copyfile(args.recorded_comparison, out / 'evidence/zero-recorded-comparison.json')
    write(out / 'evidence/recorded-runtime-breakdown.json', recorded_analysis)
from groups import build_groups
build_groups(out, artifacts, data, verified, args.e2b_recorded_summary, args.e2b_recorded_summary_sha)
svg=(out/'time-boundaries.svg').read_text()
html=(out/'index.html').read_text()
html,n=re.subn(r'<!-- time-boundaries:start -->.*?<!-- time-boundaries:end -->',lambda _: '<!-- time-boundaries:start -->'+svg+'<!-- time-boundaries:end -->',html,flags=re.S)
assert n==1
(out/'index.html').write_text(html)
write(out / 'data.json', data)
(out / 'data.js').write_text('window.DASHBOARD=' + json.dumps(data, ensure_ascii=False, separators=(',',':')).replace('</',r'<\/') + ';\n')
receipt = {'status': 'BUILT_FROM_VERIFIED_EXISTING_EVIDENCE', 'scope': 'static presentation; zero Agent/cloud/model runs',
           'inputs': inputs, 'outputs': {}, 'preview': 'Only original X intervals and safe identity fields; no clock alignment or fabricated ranges',
           'original_traces': 'Full gzip bytes decompress to original backend-validated SHA; no spans removed'}
for path in sorted(out.rglob('*')):
    if path.is_file() and path.name not in ('build-receipt.json', 'delivery-manifest.json') and not path.name.startswith('browser-validation') and path.suffix not in ('.png', '.zip') and '__pycache__' not in str(path):
        receipt['outputs'][str(path.relative_to(out))] = {'sha256': sha(path.read_bytes()), 'bytes': path.stat().st_size}
write(out / 'build-receipt.json', receipt)
print(json.dumps({'status': receipt['status'], 'tools': len(tools), 'traces': len(traces), 'output_files': len(receipt['outputs'])}))

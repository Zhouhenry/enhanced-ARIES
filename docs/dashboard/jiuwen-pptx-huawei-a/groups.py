"""Safe presentation summaries for six explicitly selected A-placement runs."""
from collections import defaultdict
from pathlib import Path
import hashlib
import json
import re
import shlex
import shutil

LIVE = [
 ('hw-live', '01', '华为云', 'jiuwen-huawei-external-pptx-record-3', '0e745240b94692de92a73b4bc6db8b721e4bf80dc693f36344714e155ad6f91f'),
 ('e2b-live', '02', 'E2B', 'jiuwen-e2b-external-pptx-record-3', '878a7ed021cf2e960696b39e3798715baaebc27eb44494b55a6783488ac6c653'),
]
MODE_LABEL = {'live':'实时 DeepSeek', 'zero':'回放 · 去除模型等待', 'recorded':'回放 · 保留模型节奏'}
PROGRAMS = {'python','python3','node','ls','find','cat','head','tail','sed','cp','mv','mkdir','wc','unzip','zip','libreoffice','soffice','pdftoppm','pdftotext','test','du','file','pwd','grep','rg'}

def summarize_intervals(rows):
    domains=defaultdict(list)
    for row in rows:
        assert row['duration_ns']==row['end_offset_ns']-row['start_offset_ns']>=0
        domains[row['clock_domain']].append((row['start_offset_ns'],row['end_offset_ns']))
    unions=[]
    for domain,ranges in sorted(domains.items()):
        total=0;start=end=None
        for left,right in sorted(ranges):
            if start is None:start,end=left,right
            elif left<=end:end=max(end,right)
            else:total+=end-start;start,end=left,right
        if start is not None:total+=end-start
        unions.append({'clock_domain':domain,'union_ns':total})
    return {'count':len(rows),'sum_ns':sum(r['duration_ns'] for r in rows),
      'maximum_ns':max((r['duration_ns'] for r in rows),default=None),'same_clock_unions':unions}

def snapshot_layers(group,source,verified):
    summary=verified(source,group['run']['summary_sha'])
    exported=next(x for x in summary['observation']['exports'] if x['name']=='snapshot.json')
    assert exported['exit_code']==0 and exported['same_snapshot']
    snap=verified(source.parent/exported['source']['path'],exported['source']['sha256'])
    intervals=snap['intervals']
    rows=[x for x in intervals if x['name']=='jiuwen.worker.native_operation_request']
    assert len({r['attributes']['invocation_id'] for r in rows})==len(rows)
    assert all(r['measurement_scope']=='worker_request_prepare_to_native_result_write_not_process' for r in rows)
    coverage=[c for c in snap['coverage'] if c['source']=='jiuwen_remote_worker']
    assert rows and coverage and all(c['integrity']=='complete' for c in coverage)
    assert sum(c['paired'] for c in coverage)==len(rows)
    by_kind=defaultdict(list);by_method=defaultdict(list);by_tool=defaultdict(list)
    safe=[]
    for r in rows:
        attrs=r['attributes'];kind=attrs['operation_kind'];method=attrs['operation_method']
        by_kind[kind].append(r);by_method[(kind,method)].append(r)
        if r.get('tool_call_id'):by_tool[r['tool_call_id']].append(r)
        safe.append({k:r[k] for k in ('operation_id','tool_call_id','clock_domain','start_offset_ns','end_offset_ns','duration_ns','outcome') if k in r}|{'invocation_id':attrs['invocation_id'],'kind':kind,'method':method})
    worker=summarize_intervals(rows)|{'measurement_scope':'worker_request_prepare_to_native_result_write_not_process',
      'by_kind':[{'kind':kind}|summarize_intervals(v) for kind,v in sorted(by_kind.items())],
      'by_method':[{'kind':kind,'method':method}|summarize_intervals(v) for (kind,method),v in sorted(by_method.items())],
      'intervals':safe,'coverage_integrity':'complete','tool_call_ids':len(by_tool),
      'identity_unavailable_count':sum(not r.get('tool_call_id') for r in rows)}
    group['worker']=worker
    for tool in group['tools']:
        linked=by_tool.get(tool['tool_call_id'],[])
        tool['worker']=summarize_intervals(linked) if linked else None
    models=[]
    for name in ('model.http_request','model.admission','model.body_read','model.parse','model.match'):
        values=[i for i in intervals if i['name']==name]
        if values:models.append({'name':name}|summarize_intervals(values))
    # These are retained actual controller intervals, not Runtime minus tools.
    if models:group['analysis']['models']=models
    group['snapshot_sha256']=exported['source']['sha256']
    return {'group':group['id'],'summary_sha256':group['run']['summary_sha'],
      'snapshot_sha256':exported['source']['sha256'],'worker':worker,'models':models}

def historical_backend(out,artifacts,data,verified):
    group=next(g for g in data['groups'] if g['id']=='hw-live')
    assert group['snapshot_sha256']=='89f37420ecd667c4886543274ddaf1a577bef9aaa86b6ce0025603802ae8c154'
    base=artifacts/'jiuwen-pptx-huawei-a-backends-1'
    for name,digest in [('evidence.json','faf485d6167838c1d669bd0f851366b222787c64d8dbd3685bb9d912e298fe0f'),
      ('metrics-evidence.json','6efefa2e39064d6403dc3bdae4bdfd11e31707cc8f24ef32a84e63b8cb50c906'),
      ('postflight.json','301d4aff171b22ec130dff3ef99233b0923fa9adce165681868bcf2a0fd78bbf')]:
        proof=verified(base/name,digest)
        assert proof['status'].startswith('PASS')
        shutil.copyfile(base/name,out/'evidence'/('hw-live-backend-'+name))
    group['backend_status']='PASS'
    group['backend_receipt']='evidence/hw-live-backend-evidence.json'
    group['backend_receipt_sha256']='faf485d6167838c1d669bd0f851366b222787c64d8dbd3685bb9d912e298fe0f'
    group['backend_note']='Original summary UNRUN preserved; existing later query receipt validates the identical snapshot.'

def safe_commands(tape, tools):
    by_id = {t['tool_call_id']: t for t in tools}
    for turn in tape['turns']:
        for call in turn.get('tool_calls', []):
            if call['id'] not in by_id:
                continue
            raw = json.loads(call['arguments']) if isinstance(call['arguments'], str) else call['arguments']
            command = raw.get('command', raw.get('cmd', ''))
            if not isinstance(command, str) or not command:
                continue
            programs = []
            for fragment in re.split(r'&&|\|\||;|\n', command):
                m = re.match(r'\s*([A-Za-z0-9_./+-]+)(?:\s|$)', fragment)
                name = Path(m[1]).name if m else ''
                if name in PROGRAMS and name not in programs:
                    programs.append(name)
            try:
                tokens = shlex.split(command)
            except ValueError:
                tokens = []
            files = list(dict.fromkeys(Path(t).name for t in tokens if re.fullmatch(r'[A-Za-z0-9_./+-]+\.(?:py|js|mjs|cjs|pptx|pdf|md)',t)))[:8]
            item = by_id[call['id']]
            item['command_details'] = {'programs':programs,'files':files,'scope':'static program/file basenames only; no command or inline code'}
            if programs:
                item['action'] += ' · ' + ' / '.join(programs[:3])
            if files:
                item['action'] += ' · ' + ' / '.join(files[:2])

def from_summary(key, code, provider, mode, source, expected, out, verified, model=None, metadata=None):
    d = verified(source, expected)
    root = source.parent
    fields = {x['source_field']:x['duration_ns'] for x in d['stages']}
    queue = fields.pop('queue_duration')
    streams = {s['actor']:s for s in d['observation']['resources']['streams']}
    controller,agent,cloud = (streams[k] for k in ('controller_shared_process','agent_container','task_sandbox'))
    metrics = lambda s:{m['name']:m for m in s['metrics']}
    cm,am,pm = metrics(controller),metrics(agent),metrics(cloud)
    resources = {
      'controller_cpu_s':sum(x['value'] for x in controller['snapshot_boundary_deltas'] if x['name'] in ('cpu_user_ns','cpu_system_ns'))/1e9,
      'controller_rss_mib':cm['process_lifetime_peak_rss_bytes']['maximum_returned']/1048576,
      'agent_cpu_s':(am['cpu_usage_nanoseconds']['last_returned']-am['cpu_usage_nanoseconds']['first_returned'])/1e9,
      'agent_memory_mib':am['memory_usage_bytes']['maximum_returned']/1048576,'agent_samples':agent['receipt_count'],
      'provider_memory_mib':pm['mem_used_mib']['maximum_returned'] if 'mem_used_mib' in pm else pm['mem_used']['maximum_returned']/1048576,
      'provider_cpu_pct':pm['cpu_used_pct']['maximum_returned'],'provider_receipts':cloud['receipt_count'],
      'provider_repeats':cloud['repeated_timestamp_and_value_receipts'],
      'provider_counts':cloud.get('provider_counts'),
    }
    commands = [x for x in d['observation']['process']['intervals'] if x['name']=='jiuwen.process.command']
    command_ids = {x['tool_call_id']:x for x in commands}
    assert len(command_ids)==len(commands), 'Duplicate command ToolCall identity'
    native = {x['tool_call_id']:x for x in d['tool_actions']['actions']}
    inventory = d['tape']['actions']
    assert len(native)==len(d['tool_actions']['actions'])
    tools=[]
    native_groups=defaultdict(list)
    operations=set()
    for item in inventory:
        call=item['tool_call_id']; spans=native[call]['native_tool_spans']
        assert len(spans)==1 and spans[0]['operation_id'] not in operations
        span=spans[0];operations.add(span['operation_id']);native_groups[item['tool_name']].append(span['wall_duration_ns'])
        cmd=command_ids.get(call)
        extra=(metadata or {}).get(call,{})
        tools.append({'tool_call_id':call,'tool_name':item['tool_name'],'action':extra.get('action',item['action']),
          'turn':item['turn_index']+1,'ordinal':item['tool_index']+1,'locator':item['evidence_locator'],
          'command_sha256':item.get('command_sha256'),'command_details':extra.get('command_details'),
          'returncode':cmd['attributes'].get('returncode') if cmd else None,
          'command_ns':cmd['duration_ns'] if cmd else None,'native_ns':span['wall_duration_ns'],
          'operation_id':span['operation_id'],
          'command_observation':'observed' if cmd else 'not_observed' if item['tool_name']=='bash' else 'not_applicable'})
    assert set(command_ids)<=set(native)
    if mode=='live' and not metadata:
        tape_source=d['tape']['source']
        tape=verified(root/tape_source['path'],tape_source['sha256'])
        safe_commands(tape,tools)
    aggregates=[{'tool_name':name,'count':len(v),'sum_ns':sum(v),'maximum_ns':max(v)} for name,v in sorted(native_groups.items())]
    analysis={'models':model or [],'authoritative_native_tools':aggregates,
      'authoritative_native_tools_total':{'count':len(tools),'sum_ns':sum(x['sum_ns'] for x in aggregates),'same_clock_unions':[]},
      'command_intervals':{'count':len(commands),'sum_ns':sum(x['duration_ns'] for x in commands),'maximum_ns':max(x['duration_ns'] for x in commands)},
      'scope':'Current-attempt exact ToolCall process durations; no absent model windows or native unions inferred'}
    ppts=[]
    for deck in d['verifier']['pptx']['decks']:
        raw=(root/deck['path']).read_bytes()
        assert hashlib.sha256(raw).hexdigest()==deck['sha256']
        target='downloads/'+key+'-'+Path(deck['path']).name
        assert target not in ppts
        (out/target).write_bytes(raw);ppts.append(target)
    shutil.copyfile(source,out/'evidence'/f'{key}-acceptance-summary.json')
    return {'id':key,'code':code,'provider':provider,'mode':mode,'label':provider+' · '+MODE_LABEL[mode],
      'status':'PASS' if d['verifier']['score']==1 and d['cleanup']['provider_absence_confirmed'] else 'FAIL',
      'run':{'id':key,'label':provider,'run_id':d['run_id'],'summary_sha':expected,'runtime_ns':fields['runtime_execution_duration'],
        'attempt_ns':d['attempt_duration_ns'],'driver_s':d['driver_duration_s'],'queue_ns':queue,'stages':fields,
        'remainder_ns':d['attempt_duration_ns']-sum(fields.values()),'resources':resources},
      'analysis':analysis,'tools':tools,'ppt':next(p for p in ppts if p.endswith('-final.pptx')),
      'backend_status':d['observation']['backends'],'projection_status':d['observation']['projection_status'],
      'release':Path(d['source_release']).name,'summary_file':f'evidence/{key}-acceptance-summary.json',
      'tape_sha':d['tape']['source']['sha256'],'decisions':d['tape']['tool_call_decisions'],
      'comparability':'end-to-end reference; distinct live decisions' if mode=='live' else 'same A Tape within identical timing only'}

def build_groups(out, artifacts, data, verified, e2b_recorded=None, e2b_sha=None):
    metadata={t['tool_call_id']:t for t in data['tools']}
    groups=[from_summary(key,code,provider,'live',artifacts/name/'acceptance-summary.json',digest,out,verified,
                         metadata=metadata if provider=='华为云' else None) for key,code,provider,name,digest in LIVE]
    for i,(key,code,provider,name) in enumerate((('hw-zero','03','华为云','jiuwen-huawei-external-pptx-workload-replay-1'),('e2b-zero','04','E2B','jiuwen-e2b-external-pptx-workload-replay-1'))):
        g=from_summary(key,code,provider,'zero',artifacts/name/'acceptance-summary.json',data['runs'][i]['summary_sha'],out,verified,metadata=metadata)
        if i==0:
            g['analysis']=data['analysis'];g['backend_status']='PASS';g['backend_receipt']='evidence/backend-evidence.json'
        groups.append(g)
    if data.get('recorded'):
        r=data['recorded'];g=dict(r)
        g['tools']=[{**{k:v for k,v in t.items() if not k.startswith('huawei_')},
          'command_ns':t['huawei_command_ns'],'native_ns':t['huawei_native_ns'],'operation_id':t['huawei_operation_id'],
          'command_observation':'observed' if t['huawei_command_ns'] is not None else 'not_observed' if t['tool_name']=='bash' else 'not_applicable'} for t in r['tools']]
        g.update(id='hw-recorded',code='05',provider='华为云',mode='recorded',label='华为云 · '+MODE_LABEL['recorded'],status='PASS',release='release29',
          summary_file='evidence/huawei-recorded-acceptance-summary.json',tape_sha=data['tape_sha'],decisions=len(r['tools']),comparability='same A Tape within identical timing only')
        groups.append(g)
    else:
        groups.append({'id':'hw-recorded','code':'05','provider':'华为云','mode':'recorded','label':'华为云 · '+MODE_LABEL['recorded'],'status':'UNRUN'})
    if e2b_recorded:
        assert e2b_sha
        g=from_summary('e2b-recorded','06','E2B','recorded',e2b_recorded,e2b_sha,out,verified,metadata=metadata)
        assert g['tape_sha']==data['tape_sha']
        groups.append(g)
    else:
        groups.append({'id':'e2b-recorded','code':'06','provider':'E2B','mode':'recorded','label':'E2B · '+MODE_LABEL['recorded'],'status':'RUNNING'})
    sources={key:artifacts/name/'acceptance-summary.json' for key,_,_,name,_ in LIVE}
    sources.update({'hw-zero':artifacts/'jiuwen-huawei-external-pptx-workload-replay-1/acceptance-summary.json',
      'e2b-zero':artifacts/'jiuwen-e2b-external-pptx-workload-replay-1/acceptance-summary.json',
      'hw-recorded':artifacts/'jiuwen-huawei-external-pptx-workload-replay-2/acceptance-summary.json',
      'e2b-recorded':e2b_recorded})
    aggregate={'scope':'Existing validated Snapshot intervals only; no new cloud work, no cross-clock subtraction',
      'results':[snapshot_layers(g,sources[g['id']],verified) for g in groups if g.get('run')]}
    (out/'evidence/six-group-snapshot-layers.json').write_text(json.dumps(aggregate,ensure_ascii=False,indent=2)+'\n')
    data['groups']=groups
    historical_backend(out,artifacts,data,verified)

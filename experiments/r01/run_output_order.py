"""ECS-only isolated native final-output regression; no cloud/LLM/production mutation."""
import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import re
import signal
import subprocess
import sys
import time

BASE = "ghcr.io/openclaw/openclaw:2026.7.1@sha256:6a31d44b2944e7adcd2b582bf6fb463111264ebca97a0201795b799135bd102c"
PRIOR = Path('/opt/aries-artifacts/observability-r01-native-poc-3')
EXPECTED_CASES = {
    'forward','reverse','interleave','empty','ansi-whole','ansi-split','unicode','binary','cap',
    'ansi-limit','ansi-limit-126','126','127','before-yield','timeout','cancel','finalize-error',
    'background','background-fast-0','background-fast-1','background-fast-2','background-fast-3',
    'auto-yield','zero-yield','finalize-yield','pty',
}


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError('duplicate producer JSON key: ' + key)
        result[key] = value
    return result


def validate_producer(data, targets, directory):
    captured = {}
    for key, filename, limit in [('log','native-tool-identities.jsonl',4*1024*1024),
                                 ('status','native-tool-identities.status.json',4096)]:
        item = data['producer_evidence'][key]
        assert item['status'] == 'captured', ('producer evidence',key,item['status'])
        raw = base64.b64decode(item['bytes_base64'], validate=True)
        assert 0 < len(raw) <= limit, ('producer size',key)
        with (directory/filename).open('xb') as out:
            os.fchmod(out.fileno(),0o600)
            out.write(raw)
        captured[key] = raw
    calls = data['exec_invocations']
    assert len(calls) == len(EXPECTED_CASES)
    assert {call['case_id'] for call in calls} == EXPECTED_CASES
    assert [call['call_index'] for call in calls] == list(range(len(calls)))
    assert len({call['tool_call_id'] for call in calls}) == len(calls)
    for call in calls:
        assert type(call['identity_expected']) is bool and type(call['pty_requested']) is bool
        assert call['host'] in ('sandbox','gateway')
        assert call['identity_expected'] == (call['host'] == 'sandbox')
    expected = {call['call_index']:call for call in calls if call['identity_expected']}
    witnessed = data['native_identity_witnesses']
    assert len(witnessed) == len(expected)
    assert {row['call_index'] for row in witnessed} == set(expected)
    keys = {'schema_version','scope_id','agent_session_id','tool_call_id','native_invocation_id'}
    identities = []
    for row in witnessed:
        identity = row['identity']
        assert set(identity) == keys and type(identity['schema_version']) is int and identity['schema_version'] == 1
        assert identity['scope_id'] == 'output-order-candidate'
        assert identity['agent_session_id'] == 'output-order-session'
        assert identity['tool_call_id'] == expected[row['call_index']]['tool_call_id']
        assert all(isinstance(identity[key],str) and 0 < len(identity[key].encode()) <= 256 for key in keys-{'schema_version'})
        identities.append(identity)
    assert len({item['native_invocation_id'] for item in identities}) == len(identities)
    assert captured['log'].endswith(b'\n')
    lines = captured['log'].decode('utf8').splitlines()
    logged = [json.loads(line,object_pairs_hook=unique_object) for line in lines]
    # This existing execution matrix has no pre-dispatch rejection. V2 keeps
    # producer lifecycle fields out of the unchanged v1 wire publication.
    logged_identities = []
    for record in logged:
        if record.get('schema_version') == 2:
            assert set(record) == keys | {'phase','outcome','dispatched','reason'}
            assert (record['phase'],record['outcome'],record['dispatched'],record['reason']) == ('invocation','started',None,'')
            logged_identities.append(dict({key:record[key] for key in keys},schema_version=1))
        else:
            logged_identities.append(record)
    assert logged_identities == identities, 'producer invocation log must equal actual native identity publications in order'
    health = json.loads(captured['status'],object_pairs_hook=unique_object)
    assert set(health) == {'schema_version','scope_id','agent_session_id','producer_instance_id','attempted','written',
                           'write_failed','status_write_failed','failed_latched'}
    assert type(health['schema_version']) is int and health['schema_version'] == 1
    assert health['scope_id'] == 'output-order-candidate' and health['agent_session_id'] == 'output-order-session'
    assert isinstance(health['producer_instance_id'],str) and 0 < len(health['producer_instance_id']) <= 256
    assert all(type(health[key]) is int and health[key] >= 0 for key in ('attempted','written','write_failed','status_write_failed'))
    assert health['attempted'] == health['written'] + health['write_failed'] == len(identities)
    assert health['written'] == len(logged) and health['write_failed'] == health['status_write_failed'] == 0
    assert health['failed_latched'] is False
    linked = [row for row in targets if row.get('linked')]
    assert len(linked) == len(identities), 'all linked target executions must have one native invocation'
    for identity in identities:
        matches = [row for row in linked if row.get('native_identity',{}).get('native_invocation_id') == identity['native_invocation_id']]
        assert len(matches) == 1 and matches[0]['execution_id']
        assert {key:matches[0]['native_identity'][key] for key in keys} == identity
    return {'status':'PASS_THIS_NATIVE_PROBE_ONLY','native_exec_calls':len(calls),'identity_expected_calls':len(expected),
            'native_identity_publications':len(identities),'linked_target_executions':len(linked),'health':health,
            'denominator':'actual native execute invocations plus ALS-bound native identity publications; no inferred fixed count'}


def main():
    if not sys.platform.startswith('linux'):
        raise SystemExit('authorized Linux ECS only')
    parser = argparse.ArgumentParser()
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--candidate-image', help='Already built immutable Docker image ID; defaults to the historical isolated overlay')
    parser.add_argument('--transport-dir', type=Path, default=PRIOR,
                        help='Directory containing the isolated bridge fixture and aries-ssh-client')
    args = parser.parse_args()
    root = args.output.resolve()
    root.mkdir(mode=0o700, exist_ok=False)
    here = Path(__file__).resolve().parent
    evidence = dict(schema_version=1, status='FAIL', scope='native foreground non-PTY renderer PoC',
                    cloud_created=0, model_calls=0, production_changed=False, steps=[], cleanup=[])
    env = dict(os.environ, DOCKER_BUILDKIT='0')
    transport = args.transport_dir.resolve()
    owned = [root.name+'-helper',root.name+'-b-native'] + [root.name+'-'+kind+'-'+role
             for kind in ('baseline','candidate') for role in ('guest','native')]
    cleanup_enabled = False
    def interrupted(signum, frame):
        signal.signal(signal.SIGTERM,signal.SIG_IGN)
        raise InterruptedError('controlled signal '+str(signum))
    signal.signal(signal.SIGTERM,interrupted)

    def remove_container(name):
        try:
            result = subprocess.run(['docker','rm','-f',name],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,timeout=5)
            return result.returncode
        except Exception as exc:
            evidence['cleanup'].append({'container':name,'remove_error':type(exc).__name__})
            return None

    def run(argv, name, timeout=180, check=True):
        with (root/(name+'.log')).open('wb') as log:
            result = subprocess.run(argv, stdout=log, stderr=subprocess.STDOUT, env=env, timeout=timeout)
        evidence['steps'].append(dict(name=name, exit_code=result.returncode))
        if check and result.returncode:
            raise RuntimeError(name+' failed; see log')
        return result

    try:
        existing = subprocess.check_output(['docker','container','ls','-a','--format={{.Names}}'],text=True,timeout=10).splitlines()
        assert not set(owned).intersection(existing), 'owned test container name already exists'
        cleanup_enabled = True
        evidence['owned_containers'] = owned
        transport_hashes = {}
        for binary, digest in [('bridge','91dcfef71b44562bd9fa336cc4be00bdf6ef31a202350c56a211c3fdd329355e'),
                               ('aries-ssh-client','a3a46fef7668338eb3364bd13e2f505175d4070c0f23df793d9a30569946ea47')]:
            transport_hashes[binary] = hashlib.sha256((transport/binary).read_bytes()).hexdigest()
            if transport == PRIOR:
                assert transport_hashes[binary] == digest
        evidence['transport'] = {'path':str(transport),'sha256':transport_hashes}
        if args.candidate_image:
            assert re.fullmatch(r'sha256:[0-9a-f]{64}', args.candidate_image), 'immutable image ID required'
            candidate=subprocess.check_output(['docker','image','inspect','--format={{.Id}}',args.candidate_image],text=True).strip()
            assert candidate == args.candidate_image
        else:
            run(['python3',str(here/'output_order_patch.py'),str(PRIOR/'image'),str(root/'image'),'--input-version','r01-poc3'],'patch')
            tag='aries-openclaw:'+root.name
            run(['docker','build','--pull=false','--network=none','-t',tag,str(root/'image')],'image-build')
            candidate=subprocess.check_output(['docker','image','inspect','--format={{.Id}}',tag],text=True).strip()
        evidence['candidate_image_id']=candidate
        run(['docker','run','--rm','--name',root.name+'-helper','--network=none','--read-only','--cap-drop=ALL','--memory=512m','--cpus=1',
             '-v',str(here/'test_output_order.mjs')+':/run/tests.mjs:ro','--entrypoint=node',candidate,
             '--test','/run/tests.mjs'],'helper-tests')
        run(['python3',str(here/'output_order_patch.py'),'/opt/aries-artifacts/observability-r01-native-source-1',str(root/'image-b')],'patch-b')
        btag='aries-openclaw:'+root.name+'-b'
        run(['docker','build','--pull=false','--network=none','-t',btag,str(root/'image-b')],'image-b-build')
        bimage=subprocess.check_output(['docker','image','inspect','--format={{.Id}}',btag],text=True).strip()
        evidence['original_native_b_image_id']=bimage
        run(['docker','run','--rm','--name',root.name+'-b-native','--network=none','--read-only','--cap-drop=ALL','--pids-limit=128','--memory=1g','--cpus=1',
             '--tmpfs=/tmp:rw,nosuid,nodev,size=64m','--user=1000:1000','-e','HOME=/tmp','-e','OPENCLAW_STATE_DIR=/tmp/openclaw-state',
             '-v',str(here/'output_order_b_probe.mjs')+':/run/b-probe.mjs:ro','--entrypoint=node',bimage,
             '/run/b-probe.mjs'],'original-native-b')
        for name,image in [('baseline',BASE),('candidate',candidate)]:
            case=root/name;case.mkdir(mode=0o700)
            guest=root.name+'-'+name+'-guest';native=root.name+'-'+name+'-native'
            process=None;bridge_log=None
            try:
                run(['docker','run','-d','--name',guest,'--network=none','--pids-limit=128','--memory=256m','--cpus=1',
                     '--entrypoint=/bin/sh','--user=0:0',BASE,'-c','mkdir -p /work; chown 1000:1000 /work; exec sleep 600'],name+'-guest-start')
                bridge_log=(case/'bridge.log').open('wb')
                process=subprocess.Popen([str(transport/'bridge'),'--container',guest,'--output',str(case),
                    '--scope','output-order-'+name,'--session','output-order-session'],stdout=bridge_log,stderr=subprocess.STDOUT,env=env)
                deadline=time.monotonic()+30
                while not (case/'ready.json').exists():
                    if process.poll() is not None or time.monotonic()>deadline: raise RuntimeError('Bridge readiness')
                    time.sleep(.05)
                (case/'ready.json').chmod(0o444)
                (case/'scope.json').write_text(json.dumps(dict(scope_id='output-order-'+name,agent_session_id='output-order-session')))
                (case/'scope.json').chmod(0o444)
                run(['docker','run','--name',native,'--network=host','--read-only','--cap-drop=ALL','--pids-limit=128',
                     '--memory=1g','--cpus=1','--tmpfs=/tmp:rw,nosuid,nodev,size=64m','--user=1000:1000',
                     '-e','HOME=/tmp','-e','OPENCLAW_STATE_DIR=/tmp/openclaw-state',
                     '-v',str(case/'ready.json')+':/run/aries/probe.json:ro',
                     '-v',str(case/'scope.json')+':/run/aries/tool-identity-scope.json:ro',
                     '-v',str(transport/'aries-ssh-client')+':/opt/aries/bin/aries-ssh-client:ro',
                     '-v',str(here/'output_order_probe.mjs')+':/run/aries/probe.mjs:ro','--entrypoint=node',image,
                     '/run/aries/probe.mjs'],name+'-native',timeout=180)
            finally:
                remove_container(native)
                if process is not None:
                    try:
                        process.send_signal(signal.SIGTERM)
                        try:process.wait(timeout=25)
                        except subprocess.TimeoutExpired:process.kill();process.wait(timeout=5)
                    except Exception as exc:
                        evidence['cleanup'].append({'bridge_cleanup_error':type(exc).__name__})
                    finally:
                        evidence['cleanup'].append(dict(bridge_exit=process.returncode,bridge_stopped=(case/'stopped').exists()))
                if bridge_log:bridge_log.close()
                remove_container(guest)
                names=subprocess.check_output(['docker','container','ls','-a','--format={{.Names}}'],text=True,timeout=10).splitlines()
                for container_name in (guest,native):
                    evidence['cleanup'].append(dict(container=container_name,absent=container_name not in names))
                    assert container_name not in names
        outputs={};raw_outputs={}
        for name in ('baseline','candidate'):
            lines=(root/(name+'-native.log')).read_text().splitlines()
            data=next(json.loads(line) for line in reversed(lines) if line.startswith('{"schema_version":'))
            raw_outputs[name]=data
            outputs[name]={x['id']:x for x in data['results']}
            assert len(data['results'])==len(outputs[name])==len(EXPECTED_CASES),(name,'case denominator')
            assert set(outputs[name])==EXPECTED_CASES,(name,'case identity set')
            assert data['allow_background'] and data['process_enabled']
        baseline=outputs['baseline'];candidate=outputs['candidate']
        targets={name:[json.loads(line) for line in (root/name/'target-events.jsonl').read_text().splitlines()]
                 for name in ('baseline','candidate')}
        if args.candidate_image:
            evidence['producer_validation'] = validate_producer(raw_outputs['candidate'],targets['candidate'],root/'candidate')
            assert all(item['status']=='absent' for item in raw_outputs['baseline']['producer_evidence'].values())
        else:
            evidence['producer_validation'] = {'status':'HISTORICAL_NOT_REQUIRED','reason':'historical helper has no status contract'}
        byte_checks=[]
        assert baseline['forward']['response']['details']['aggregated'] != baseline['reverse']['response']['details']['aggregated']
        for key,row in candidate.items():
            details=row['response']['details']
            if key=='finalize-yield':
                assert 'outputRenderer' not in details
                for name in ('baseline','candidate'):
                    injected=outputs[name][key]
                    assert injected['finalize_started_before_yield'] and injected['finalize_finished']
                    assert injected['poll']['content'][0]['text']=='No session found for '+injected['response']['details']['sessionId']
                    assert 'outputRenderer' not in injected['poll']['details']
                evidence['injected_finalize_yield']={'status':'KNOWN_ORIGINAL_LIFECYCLE_DEFECT_REPRODUCED',
                    'normal_lifecycle_pass':False,'reason':'original markExited removes foreground session before delayed finalize; later yield has no poll session',
                    'candidate_legacy_behavior_preserved':True}
                continue
            if key.startswith('background') or key in ('auto-yield','zero-yield'):
                assert 'outputRenderer' not in details and 'outputRenderer' not in row['poll']['details'] and 'outputRenderer' not in row['log']['details']
                for field in ('aggregated','status','exitCode','exitSignal'):
                    assert row['poll']['details'].get(field)==baseline[key]['poll']['details'].get(field),(key,field)
                assert row['log']['content']==baseline[key]['log']['content'],key
                continue
            if key=='pty':
                assert 'outputRenderer' not in details
                assert details['aggregated']==baseline[key]['response']['details']['aggregated']=='pty'
                continue
            assert details['outputRenderer']=='stdout-stderr-v1'
            if row.get('expected') is not None:
                assert details['aggregated']==row['expected'],key
                assert row['response']['content'][0]['text']==(row['expected'] or '(no output)'),key
            if key.startswith('ansi-limit'):
                assert details['outputRenderingError']=='ansi_token_limit' and details['exitCode']==(126 if key.endswith('126') else 0)
                assert details['aggregated']==row['response']['content'][0]['text']
                if key.endswith('126'):
                    assert details['executionFailureKind']==baseline[key]['response']['details']['failureKind']
            if key in ('126','127','timeout','finalize-error'):
                assert details['aggregated'].startswith('outerr'),key
                assert row['response']['content'][0]['text'].startswith('outerr'),key
                assert 'errout' not in row['response']['content'][0]['text'],key
            for field in ('exitCode','exitSignal','exitReason','timedOut','noOutputTimedOut','failureKind'):
                if field=='failureKind' and key.startswith('ansi-limit'):continue
                assert details.get(field)==baseline[key]['response']['details'].get(field),(key,field)
            # Known unique experiment inputs select baseline cases; this is not a causal-ID fallback.
            expected_args=['-c',"cd '/work' && "+row['command']]
            left=[r for r in targets['baseline'] if r['command'].get('args')==expected_args]
            right=[r for r in targets['candidate'] if r.get('native_identity',{}).get('tool_call_id')==key]
            assert len(left)==len(right)==1,(key,'target cardinality')
            assert right[0]['linked'] and right[0]['execution_id']
            assert right[0]['native_identity']['agent_session_id']=='output-order-session'
            assert right[0]['native_identity']['scope_id']=='output-order-candidate'
            for field in ('command','stdout_sha256','stderr_sha256','exit_code','transport_error'):
                assert left[0][field]==right[0][field],(key,field)
            byte_checks.append(key)
        assert candidate['forward']['response']['details']['aggregated']==candidate['reverse']['response']['details']['aggregated']
        assert candidate['ansi-whole']['response']['details']['aggregated']==candidate['ansi-split']['response']['details']['aggregated']
        assert all(x.get('bridge_exit',0)==0 and x.get('bridge_stopped',True) and x.get('absent',True) for x in evidence['cleanup'])
        evidence['checks']={'baseline_cross_stream_negative':True,'candidate_final_positive':True,'case_count':len(candidate),
            'preregistered_case_ids':sorted(EXPECTED_CASES),'expected_behavior_cases':25,'known_native_limitation_cases':1,
            'original_exit_metadata_preserved':True,'background_and_yield_legacy':True,'strict_matcher_changed':False,
            'actual_pty_confirmed':True,'target_byte_command_env_checks':byte_checks,
            'allow_background_true':True,'process_poll_and_log_retained':True,'fast_background_cases':4,
            'final_before_yield_and_zero_yield_tested':True,'background_replay_determinism_claimed':False,
            'injected_finalize_error_tested':True,'injected_finalize_yield_excluded_from_lifecycle_pass':True}
        evidence['checks']['original_native_b_parallel_cases']=4
        evidence['status']='PASS_NATIVE_OUTPUT_RENDERER_POC_ONLY'
    except Exception as exc:
        evidence['error_type']=type(exc).__name__;evidence['error']=str(exc)
    finally:
        signal.signal(signal.SIGTERM,signal.SIG_IGN)
        if cleanup_enabled:
            for name in owned:
                remove_container(name)
            try:
                remaining = subprocess.check_output(['docker','container','ls','-a','--format={{.Names}}'],text=True,timeout=10).splitlines()
                for name in owned:
                    evidence['cleanup'].append({'container':name,'absent':name not in remaining})
                if set(owned).intersection(remaining):evidence['status']='FAIL'
            except Exception as exc:
                evidence['status']='FAIL';evidence['cleanup_inventory_error']=type(exc).__name__
            if any('remove_error' in item or 'bridge_cleanup_error' in item for item in evidence['cleanup']):evidence['status']='FAIL'
        evidence['inputs']={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in here.glob('*output_order*') if p.is_file()}
        evidence['files']={str(p.relative_to(root)):hashlib.sha256(p.read_bytes()).hexdigest() for p in root.rglob('*') if p.is_file() and p.name!='evidence.json'}
        (root/'evidence.json').write_text(json.dumps(evidence,indent=2,sort_keys=True)+'\n')
        print(json.dumps(dict(status=evidence['status'],root=str(root))))
    if evidence['status']=='FAIL':raise SystemExit(1)


if __name__=='__main__':main()

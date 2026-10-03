"""Verify raw evidence of the sealed no-primary workload; missing samples fail."""
import datetime as dt
import json
import re
import sys
from pathlib import Path

root = Path(sys.argv[1])
c = json.loads((root / 'contract.json').read_text())
errors = []
def gate(condition, description):
    if not condition:
        errors.append(description)
def at(value):
    return dt.datetime.fromisoformat(value)
def rows(name):
    p = root / 'metrics' / name
    if not p.is_file():
        errors.append('missing ' + name)
        return []
    out = []
    for line in p.read_text().splitlines():
        stamp, body = line.split('\t', 1)
        out.append((at(stamp), json.loads(body)))
    return out
def window(stamps, name):
    times = sorted(set(stamps))
    gate(len(times) >= c['minimum_complete_samples'], name + ': sample count')
    if len(times) > 1:
        gate((times[-1] - times[0]).total_seconds() >= c['minimum_sampling_span_seconds'], name + ': span')
        gate(max((b - a).total_seconds() for a, b in zip(times, times[1:])) <= c['maximum_sampling_gap_seconds'], name + ': gap')
    return {'count':len(times),'span_seconds':(times[-1]-times[0]).total_seconds() if times else None}
def memory(value):
    m=re.fullmatch(r'([\d.]+)(B|KiB|MiB|GiB|kB|MB|GB)',value.strip())
    if not m: raise ValueError('unknown Docker memory unit')
    return float(m[1])*{'B':1,'KiB':1024,'MiB':1024**2,'GiB':1024**3,'kB':1000,'MB':1000**2,'GB':1000**3}[m[2]]
try:
    log=(root/'primary-loss.log').read_text()
    gate((root/'primary-loss.exit-code').read_text().strip()=='0','actual script exit')
    gate('--- PASS: TestM407NewAnswerSheetBatchThroughStandardProfile' in log,'actual target test PASS')
    gate('--- SKIP:' not in log,'target was skipped')
    diag=re.search(r'standard full chain batch diagnostic: count=(\d+) spacing=([^ ]+) submit_return_rate_per_sec=([\d.]+) manifest_sha256=([0-9a-f]+) nsq_delivery=(\d+) nsq_fin_samples=(\d+)',log)
    gate(diag is not None,'full final diagnostic missing')
    if diag:
        gate(int(diag[1])==c['batch_count'] and diag[2]=='120ms','original fixed workload')
        gate(float(diag[3])>=c['minimum_input_rate_per_second'],'input rate')
        gate(int(diag[5])>=c['batch_count'] and diag[5]==diag[6],'every physical delivery FINed')
    timeline=re.findall(r'm4_07_timeline stream=new_mongo event_id=([^ ]+) submit_returned_at=([^ ]+) nsqd_message_at=([^ ]+) outbox_transport_confirmed_at=([^ ]+) handler_done_at=([^\s]+)',log)
    gate(len(timeline)==c['batch_count'] and len({x[0] for x in timeline})==c['batch_count'],'unique complete original identity timeline')
    submit_times=[at(x[1]) for x in timeline]
    if submit_times:
        gate((max(submit_times)-min(submit_times)).total_seconds()>=c['minimum_input_span_seconds'],'input span')
    started=re.search(r'm4_primary_loss phase=request_started old_primary=([^ ]+) no_primary_at=([^ ]+) request_at=([^ ]+) sheet_id=(\d+) event_id=([^\s]+)',log)
    returned=re.search(r'm4_primary_loss phase=request_returned event_id=([^ ]+) request_at=([^ ]+) returned_at=([^ ]+) elected_at=([^ ]+) new_primary=([^ ]+) original_call_count=(\d+)',log)
    roles=re.search(r'm4_primary_loss phase=roles timeline=(\[[^\n]+\])',log)
    recovered=re.search(r'm4_primary_loss phase=prefix_recovered original_call_count=(\d+) prefix=(\d+) event_id=([^ ]+) no_primary_at=([^ ]+) request_started_at=([^ ]+) request_returned_at=([^ ]+) elected_at=([^ ]+) prefix_recovered_at=([^ ]+) fault_to_prefix_ms=(\d+) restored_to_prefix_ms=(\d+) continued_input_total=(\d+)',log)
    gate(all((started,returned,roles,recovered)),'actual no-primary/recovery proof missing')
    recovery = None
    if all((started,returned,roles,recovered)):
        begin,end=at(returned[2]),at(returned[3])
        gate(started[5]==returned[1]==recovered[3],'unchanged overlapping event identity')
        gate(returned[6]==recovered[1]=='1','one original business request call')
        gate(started[1]!=returned[5],'different primary actually observed')
        role_samples=json.loads(roles[1]);no_primary=[];elected=[]
        for sample in role_samples:
            stamp=at(sample['at']);members=sample['members']
            gate({m['member'] for m in members}=={'mongo:27017','mongo2:27017','mongo3:27017'},'complete role members')
            gate(not any(m['error'] for m in members),'actual role reply missing')
            if not any(m['writable'] or m['error'] for m in members):no_primary.append(stamp)
            elected.extend((stamp,m['member']) for m in members if m['writable'] and not m['error'])
        overlap=[x for x in no_primary if begin<=x<=end]
        gate(bool(overlap) and (max(overlap)-begin).total_seconds()>=c['minimum_proven_no_primary_overlap_seconds'],'original request overlaps at least10s of actual all-member no-primary replies')
        gate(bool(elected) and elected[0][1]==returned[5],'actual elected role matches request proof')
        gate(int(recovered[2])==c['primary_loss_after']+1 and int(recovered[11])==c['batch_count'],'original recovery watermark and continued workload')
        fault_ms=int(recovered[9]);restored_ms=int(recovered[10])
        gate(fault_ms>=0 and fault_ms<=1000*c['maximum_recovery_seconds'],'fault to original watermark exceeds120s')
        gate(restored_ms>=0 and restored_ms<=1000*c['maximum_recovery_seconds'],'restored primary to original watermark exceeds120s')
        gate(abs((at(recovered[8])-at(recovered[4])).total_seconds()*1000-fault_ms)<2,'fault duration origin')
        gate(abs((at(recovered[8])-at(recovered[7])).total_seconds()*1000-restored_ms)<2,'restoration duration origin')
        recovery={'fault_to_prefix_ms':fault_ms,'restored_to_prefix_ms':restored_ms,'overlapping_request_seconds':(end-begin).total_seconds(),'observed_no_primary_groups':len(no_primary)}
    stats=rows('docker-stats.jsonl');per_member={k:[] for k in c['resource_members']}
    for stamp,row in stats:
        for member in per_member:
            if row['Name'].endswith('-'+member+'-1'):per_member[member].append((stamp,row))
    resources={}
    for member,values in per_member.items():
        result=window([s for s,_ in values],member)
        cpu=max((float(row['CPUPerc'].rstrip('%')) for _,row in values),default=None)
        mem=max((memory(row['MemUsage'].split('/')[0]) for _,row in values),default=None)
        if member!='nsqd':
            gate(cpu is not None and cpu<=c['cpu_max_percent_mysql_and_mongo'],member+': CPU')
            gate(mem is not None and mem<=c['memory_max_bytes_mysql_and_mongo'],member+': memory')
        result.update(cpu_max_percent=cpu,memory_max_bytes=mem);resources[member]=result
    sql={}
    for line in (root/'metrics/mysql-status.tsv').read_text().splitlines():
        stamp,key,value=line.split('\t');sql.setdefault(at(stamp),{})[key]=int(value)
    complete={s:x for s,x in sql.items() if {'Threads_connected','Innodb_row_lock_waits'}<=x.keys()}
    gate(len(complete)==len(sql),'incomplete actual SQL status group')
    window(complete,'mysql status')
    gate(max((x['Threads_connected'] for x in complete.values()),default=10**9)<=c['mysql_connections_max'],'SQL connections')
    if complete:
        stamps=sorted(complete);delta=complete[stamps[-1]]['Innodb_row_lock_waits']-complete[stamps[0]]['Innodb_row_lock_waits']
        gate(0<=delta<=c['mysql_row_lock_waits_delta_max'],'SQL row-lock wait delta')
    for member in ['mongo','mongo2','mongo3']:
        status=rows(member+'-status.jsonl');window([s for s,_ in status],member+' status')
        connections=max((x['connections']['current'] for _,x in status),default=10**9)
        gate(connections<=c['mongo_connections_max'],member+': connections')
        resources[member]['connections_max']=connections
    driver=json.loads((root/'metrics/driver-container.json').read_text())
    gate(driver['nano_cpus']==0 and driver['memory_limit_bytes']==0,'driver resources artificially capped')
    driver_rows=rows('driver-stats.jsonl');window([s for s,_ in driver_rows],'independent driver')
    containers=[json.loads(x) for x in (root/'metrics/containers.jsonl').read_text().splitlines()]
    gate(len(containers)==5 and all(x['nano_cpus']==0 and x['memory_limit_bytes']==0 for x in containers),'original DB images/resources altered')
    report={'passed':not errors,'errors':errors,'runtime_head':c['runtime_head'],'recovery':recovery,'resources':resources,'manifest_sha256':diag[4] if diag else None,'normal_report_sla_proven':False,'production_capacity_proven':False,'formal_acceptance_closed':False}
except Exception as error:
    errors.append('missing or malformed actual evidence: '+type(error).__name__)
    report={'passed':False,'errors':errors,'formal_acceptance_closed':False}
(root/'verification.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps(report))
sys.exit(0 if report['passed'] else 1)

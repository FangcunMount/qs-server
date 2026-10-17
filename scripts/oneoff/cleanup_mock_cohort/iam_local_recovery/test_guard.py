"""Pure control-flow models; no PID1, SQL, Docker, SSH, or production qualification."""
import importlib.util,pathlib,tempfile,unittest,datetime as dt,threading,contextlib
from unittest import mock
S=importlib.util.spec_from_file_location('guard',pathlib.Path(__file__).with_name('guard.py'));g=importlib.util.module_from_spec(S);S.loader.exec_module(g)
class FakeOps:
 def __init__(self):self.units={};self.events=[];self.fail_start=False;self.not_empty=False;self.gate_known=True;self.runtime_count=0;self.gc_after_stop=False
 def info(self,u):return dict(self.units.get(u,{'LoadState':'not-found'}))
 def start(self,u,props,args,timer=None):
  self.events.append(('start',u));self.units[u]={'LoadState':'loaded','ActiveState':'active','MainPID':'77','ControlPID':'0','ControlGroup':'/system.slice/'+u,'Description':next(x.split('=',1)[1] for x in props if x.startswith('Description=')),'FragmentPath':'/run/systemd/transient/'+u,'KillMode':'control-group','Delegate':'no','Result':'success','ExecMainStatus':'0'}
  if self.fail_start:raise RuntimeError('start_response_unknown')
 def stop(self,u):
  self.events.append(('stop',u));self.units[u].update(ActiveState='inactive',MainPID='0',ControlPID='0')
  if self.gc_after_stop:del self.units[u]
 def empty(self,u,r):
  self.events.append(('reaped',u));g.need(not self.not_empty,'owned_cgroup_not_empty');return True
 def bind_processes(self,u,before):return {'unit':u,'control_group':'/system.slice/'+u,'processes':[{'pid':77,'startticks':123,'absent_before_stop':False}]}
 def original_gone(self,u,binding):self.events.append(('reaped_gc',u));g.need(not self.not_empty,'owned_cgroup_not_empty');return True
 def gate(self,c):
  self.events.append(('gate',None));g.need(self.gate_known,'gate_unknown');return {'complete':True,'maintenance_gate_free':True}
 def runtime(self,c,op):
  self.events.append(('runtime',op))
  if op=='recover':self.runtime_count+=1
  if op=='stop':return {'stopped':True,'exit_code':0,'finished_at':'fixture'}
  return {'ready':True}
class GuardTests(unittest.TestCase):
 def setUp(self):
  self.tmp=tempfile.TemporaryDirectory();self.base=pathlib.Path(self.tmp.name).resolve();self.window=self.base/'rule-window-00718';self.window.mkdir(mode=0o700);self.control=self.window/'local-recovery';self.control.mkdir(mode=0o700)
  self.fixture_batches={};self.batch_patch=mock.patch.object(g,'FIXED_BATCHES',self.fixture_batches);self.batch_patch.start()
  now=g.now();self.d={'window_directory':str(self.window),'window_started_at':now.isoformat(),'data_deadline':(now+dt.timedelta(minutes=25)).isoformat(),'hard_deadline':(now+dt.timedelta(minutes=30)).isoformat(),'iam_tool_source':'a'*40,'iam_tool_sha256':g.sha(b'tool')}
  self.configure_batch('next-batch-0001',(1,1,1,1))
  self.group_patch=mock.patch.object(g,'GROUP_SHA',g.sha(self.group));self.group_patch.start()
  binary=self.window/'iam-cleanup';binary.write_bytes(b'tool');binary.chmod(0o500)
  g.save(self.window/'writers-stopped.json',g.wire({'host':'serverB','all_stopped':True,'count':1,'forced_kill_used':False,'instances':[{'cid':g.CID}]}))
  self.ops=FakeOps();self.c=g.Control(self.control,self.d,self.ops);self.run=self.c.run;self.run.mkdir(mode=0o700,parents=True);self.patch=mock.patch.object(g,'BASE',self.base);self.patch.start()
  g.save(self.control/'runtime-stop-ACK.json',g.wire({'actual_stop_complete':True,'CID':g.CID,'descriptor_sha256':g.sha(g.wire(self.d)),'writers_stopped':{'file':str(self.window/'writers-stopped.json'),'sha256':g.sha(g.read(self.window/'writers-stopped.json'))}}))
  g.save(self.control/'armed.json',g.wire({'descriptor_sha256':g.sha(g.wire(self.d)),'only_guard_stop_permitted_after_this_ACK':True}))
  g.save(self.control/'runtime-before.json',b'[]')
  self.ops.units[self.c.prefix+'-recovery.timer']={'LoadState':'loaded','ActiveState':'active'}
 def tearDown(self):self.group_patch.stop();self.batch_patch.stop();self.patch.stop();self.tmp.cleanup()
 def configure_batch(self,name,counts):
  roots,users,profiles,logins=[list(map(str,range(base,base+count))) for base,count in zip([1000,2000,3000,4000],counts)]
  scope={'user_ids':users,'profile_ids':profiles,'login_identity_ids':logins}
  selector={'operation_id':'daily-simulation-cohort-20261016','protected_manifest_sha256':g.PROTECTION_SHA,'testee_ids':roots,'selector':{'source':'synthetic_only'},'binding':{'synthetic':True}}
  sr,ir=g.wire(selector),g.wire(scope);self.fixture_batches[name]=(g.sha(sr),g.sha(ir),counts)
  self.d.update(logical_batch=name,selector_sha256=g.sha(sr),iam_scope_sha256=g.sha(ir))
  for file,raw in [('fixed-batch-selector.json',sr),('fixed-batch-iam-scope.json',ir)]:
   p=self.window/file
   if p.exists():p.unlink()
   g.save(p,raw)
  self.group=g.wire({'operation_id':selector['operation_id'],'protected_manifest_sha256':g.PROTECTION_SHA,'iam':scope,'qs':{'graph':{'selector_sha256':g.sha(sr),'root_testee_ids':roots},'selector':selector['selector'],'binding':selector['binding']}})
 def future_batch(self,counts=(4,2,3,2)):
  self.configure_batch('next-batch-0011',counts);self.c=g.Control(self.control,self.d,self.ops);self.run=self.c.run;self.run.mkdir(mode=0o700,parents=True)
  p=self.control/'runtime-stop-ACK.json';p.unlink();g.save(p,g.wire({'actual_stop_complete':True,'CID':g.CID,'descriptor_sha256':g.sha(g.wire(self.d)),'writers_stopped':{'file':str(self.window/'writers-stopped.json'),'sha256':g.sha(g.read(self.window/'writers-stopped.json'))}}))
 def argv(self,action):
  names=g.COMMON|(g.EXTRA if action in ['apply','verify'] else set())
  if action=='verify':names-= {'qs-clearance','qs-clearance-execution'}
  argv=[str(self.window/'iam-cleanup'),'--action='+action,'--deadline=60s']
  for name in sorted(names):
   p=self.window/(name+'.json')
   if not p.exists():g.save(p,self.group if name=='group' else b'{}')
   digest='' if name=='archive' and action in ['plan','capture'] or name=='journal' else g.sha(p.read_bytes())
   argv+=['--'+name+'-file='+str(p),'--'+name+'-sha256='+digest]
  return argv
 def unit(self,stage):
  self.ops.start(self.c.unit(stage),['Description='+self.c.prefix+':'+stage],[])
 def test_client_exit_does_not_cancel_owned_stage_and_deadline_stops_before_runtime(self):
  self.c.submit('plan',self.argv('plan'))
  # No parent SSH object participates in the dispatch/recovery state machine.
  fresh=g.Control(self.control,self.d,self.ops)
  result=fresh.recover('normal');names=[e[0] for e in self.ops.events]
  self.assertLess(names.index('stop'),names.index('reaped'));self.assertLess(names.index('reaped'),names.index('gate'))
  self.assertLess(names.index('gate'),max(i for i,e in enumerate(self.ops.events) if e==('runtime','recover')))
  self.assertFalse(result['IAM_delete_completion_asserted']);self.assertTrue((self.control/'recovery-fence.json').exists())
 def test_all_four_declared_stages_are_reaped_and_no_foreign_unit_touched(self):
  for a in g.ACTIONS:self.unit(a)
  self.ops.units['business.service']={'ActiveState':'active'}
  self.c.recover('normal');stopped=[u for e,u in self.ops.events if e=='stop']
  self.assertEqual(stopped,[self.c.unit(a) for a in g.ACTIONS]);self.assertEqual(self.ops.units['business.service']['ActiveState'],'active')
 def test_fence_prevents_every_subsequent_stage(self):
  self.c.recover('normal')
  for a in g.ACTIONS:
   with self.subTest(stage=a),self.assertRaisesRegex(RuntimeError,'recovery_fence'):self.c.submit(a,self.argv(a))
 def test_unknown_start_is_not_retried_and_recovery_does_not_forge_exit_proof(self):
  self.ops.fail_start=True
  with self.assertRaisesRegex(RuntimeError,'unknown'):self.c.submit('plan',self.argv('plan'))
  with self.assertRaisesRegex(RuntimeError,'already_dispatched'):self.c.submit('plan',self.argv('plan'))
  self.assertEqual(sum(e[0]=='start' for e in self.ops.events),1)
  self.c.recover('normal');self.assertFalse((self.run/'plan-execution.json').exists())
 def test_live_owned_processes_block_gate_and_runtime(self):
  self.unit('apply');self.ops.not_empty=True
  with self.assertRaisesRegex(RuntimeError,'not_empty'):self.c.recover('normal')
  self.assertFalse(any(e[0]=='gate' for e in self.ops.events));self.assertEqual(self.ops.runtime_count,0)
 def test_unknown_gate_blocks_runtime_and_preserves_unknown(self):
  self.unit('apply');self.ops.gate_known=False
  with self.assertRaisesRegex(RuntimeError,'gate_unknown'):self.c.recover('normal')
  self.assertEqual(self.ops.runtime_count,0);self.assertFalse((self.control/'service-recovery.json').exists())
 def test_reentry_preserves_original_recovery_receipt_and_no_new_delete(self):
  first=self.c.recover('normal');raw=g.read(self.control/'service-recovery.json');second=self.c.recover('normal')
  self.assertEqual(g.read(self.control/'service-recovery.json'),raw);self.assertEqual(second['existing_recovery_original_sha256'],g.sha(raw));self.assertFalse(first['IAM_delete_completion_asserted'])
  self.assertFalse(any(e[0]=='start' for e in self.ops.events))
 def test_foreign_unit_metadata_and_lost_dispatched_unit_fail_closed(self):
  self.unit('apply');self.ops.units[self.c.unit('apply')]['Description']='unrelated'
  with self.assertRaisesRegex(RuntimeError,'identity'):self.c.recover('normal')
  self.assertEqual(self.ops.runtime_count,0)
 def test_delayed_worker_cannot_extend_original_window(self):
  self.d['data_deadline']=(g.now()-dt.timedelta(seconds=1)).isoformat()
  with self.assertRaisesRegex(RuntimeError,'deadline_expired'):self.c.submit('plan',self.argv('plan'))
  self.assertFalse(any(e[0]=='start' for e in self.ops.events))
 def test_actual_forward_runtime_recovery_failure_is_not_completed(self):
  with mock.patch.object(self.ops,'runtime',side_effect=RuntimeError('runtime_unknown')):
   with self.assertRaisesRegex(RuntimeError,'runtime_unknown'):self.c.recover('normal')
  self.assertFalse((self.control/'service-recovery.json').exists());self.assertTrue((self.control/'recovery-fence.json').exists())
 def test_successful_phase_metadata_is_retained_and_recovery_accepts_stop_gc(self):
  self.c.submit('plan',self.argv('plan'))
  self.ops.units[self.c.unit('plan')].update(ActiveState='active',SubState='exited',MainPID='0')
  self.ops.gc_after_stop=True
  result=self.c.recover('normal')
  self.assertTrue(any(x[0]=='reaped_gc' for x in self.ops.events));self.assertFalse(result['IAM_delete_completion_asserted'])
  old=g.read(self.control/'plan-unit-stop-proof.json');self.c.recover('normal');self.assertEqual(g.read(self.control/'plan-unit-stop-proof.json'),old)
 def test_missing_unit_before_stop_with_dispatch_request_is_not_inferred_reaped(self):
  g.save(self.control/'plan-request.json',b'{}')
  with self.assertRaisesRegex(RuntimeError,'metadata_lost'):self.c.recover('normal')
  self.assertEqual(self.ops.runtime_count,0)
 def test_late_stop_after_deadline_recovery_is_rejected_before_runtime(self):
  self.c.recover('normal')
  before=len(self.ops.events)
  with self.assertRaisesRegex(RuntimeError,'recovery_fence'):self.c.stop()
  self.assertNotIn(('runtime','stop'),self.ops.events[before:])
 def test_stop_and_recovery_share_the_same_local_lock(self):
  (self.control/'runtime-stop-ACK.json').unlink();(self.window/'writers-stopped.json').unlink()
  entered=threading.Event();release=threading.Event();attempted=threading.Event();acquired=threading.Event();errors=[]
  original_runtime=self.ops.runtime;original_lock=g.locked
  def runtime(c,op):
   if op=='stop':entered.set();g.need(release.wait(3),'test_release_deadline');return {'stopped':True,'exit_code':0,'finished_at':'fixture'}
   return original_runtime(c,op)
  @contextlib.contextmanager
  def observe_lock(directory):
   if threading.current_thread().name=='recovery':attempted.set()
   with original_lock(directory):
    if threading.current_thread().name=='recovery':acquired.set()
    yield
  def invoke(fn):
   try:fn()
   except BaseException as e:errors.append(e)
  with mock.patch.object(self.ops,'runtime',side_effect=runtime),mock.patch.object(g,'locked',side_effect=observe_lock):
   stop=threading.Thread(target=lambda:invoke(self.c.stop),name='stop');stop.start();self.assertTrue(entered.wait(3))
   recovery=threading.Thread(target=lambda:invoke(lambda:self.c.recover('normal')),name='recovery');recovery.start();self.assertTrue(attempted.wait(3));self.assertFalse(acquired.is_set())
   release.set();stop.join(3);recovery.join(3)
   self.assertFalse(stop.is_alive());self.assertFalse(recovery.is_alive());self.assertTrue(acquired.is_set());self.assertEqual(errors,[])
   self.assertTrue((self.control/'runtime-stop-ACK.json').exists());self.assertTrue((self.control/'recovery-fence.json').exists())
   proof=g.decode(g.read(self.window/'writers-stopped.json'))
   self.assertEqual(proof['window_started_at'],self.d['window_started_at'])
   self.assertGreaterEqual(g.stamp(proof['actual_B_stop_started_at']),g.stamp(self.d['window_started_at']))
 def test_late_stop_then_unknown_start_can_be_observed_without_repeating_stop(self):
  (self.control/'runtime-stop-ACK.json').unlink()
  g.save(self.control/'runtime-stop-intent.json',g.wire({'CID':g.CID,'descriptor_sha256':g.sha(g.wire(self.d))}))
  actual={'State':{'Running':True,'Pid':77,'Status':'running','FinishedAt':'','ExitCode':0}};attempts=[]
  def runtime(c,op):
   self.assertEqual(op,'recover');attempts.append(op);g.observe_original_stop(c,actual)
   marker=self.window/'first-runtime-start-intent.json'
   if not marker.exists():
    g.save(marker,b'{"native_fake_marker":true}');actual['State'].update(Running=True,Pid=77,Status='running');raise RuntimeError('start_response_unknown')
   return {'ready':True}
  with mock.patch.object(self.ops,'runtime',side_effect=runtime):
   with self.assertRaisesRegex(RuntimeError,'inflight_original_stop_unknown'):self.c.recover('normal')
   self.assertFalse((self.control/'original-runtime-stopped.json').exists());self.assertFalse((self.control/'service-recovery.json').exists())
   actual['State'].update(Running=False,Pid=0,Status='exited',FinishedAt='fixed-finished')
   with self.assertRaisesRegex(RuntimeError,'start_response_unknown'):self.c.recover('normal')
   witness=g.read(self.control/'original-runtime-stopped.json');self.assertFalse((self.control/'service-recovery.json').exists())
   result=self.c.recover('normal');self.assertEqual(g.read(self.control/'original-runtime-stopped.json'),witness)
   self.assertFalse(result['IAM_delete_completion_asserted']);self.assertEqual(attempts,['recover']*3)
   self.assertFalse(any(e[0]=='stop' for e in self.ops.events))
 def test_running_without_original_forward_marker_cannot_reuse_stopped_witness(self):
  g.save(self.control/'runtime-stop-intent.json',g.wire({'CID':g.CID,'descriptor_sha256':g.sha(g.wire(self.d))}))
  actual={'State':{'Running':False,'Pid':0,'Status':'exited','FinishedAt':'fixed-finished','ExitCode':0}}
  g.observe_original_stop(self.c,actual);actual['State'].update(Running=True,Pid=77,Status='running')
  with self.assertRaisesRegex(RuntimeError,'forward_marker'):g.observe_original_stop(self.c,actual)
 def test_unknown_batch_or_wrong_fixed_original_SHA_is_rejected(self):
  for change in [{'logical_batch':'next-batch-0099'},{'selector_sha256':'0'*64},{'iam_scope_sha256':'0'*64}]:
   with self.subTest(change=change),self.assertRaisesRegex(RuntimeError,'fixed_batch'):g.Control(self.control,{**self.d,**change},self.ops)
 def test_full_IAM_and_QS_sets_are_checked_not_only_counts(self):
  self.future_batch((2,2,2,2));argv=self.argv('plan');path=self.window/'group.json';base=g.decode(g.read(path))
  for change in ['foreign_user','foreign_profile','foreign_login','foreign_root','wrong_selector']:
   value=g.decode(g.wire(base))
   if change.startswith('foreign_'):
    kind=change[len('foreign_'):]
    if kind=='root':value['qs']['graph']['root_testee_ids'][0]='9999'
    else:value['iam'][{'user':'user_ids','profile':'profile_ids','login':'login_identity_ids'}[kind]][0]='9999'
   else:value['qs']['graph']['selector_sha256']='0'*64
   raw=g.wire(value);path.unlink();g.save(path,raw)
   args=[('--group-sha256='+g.sha(raw)) if x.startswith('--group-sha256=') else x for x in argv]
   with self.subTest(change=change),self.assertRaisesRegex(RuntimeError,'scope_differs'):self.c.submit('plan',args)
   self.assertFalse((self.control/'group-binding.json').exists())
 def test_plan_seals_one_group_and_refuses_changed_same_scope_group(self):
  self.future_batch();argv=self.argv('plan');self.c.submit('plan',argv);original=g.read(self.control/'group-binding.json')
  value=g.decode(self.group);value['owner_confirmation_sha256']='b'*64;raw=g.wire(value);path=self.window/'group.json';path.unlink();g.save(path,raw)
  args=[('--group-sha256='+g.sha(raw)) if x.startswith('--group-sha256=') else x for x in argv]
  with self.assertRaisesRegex(RuntimeError,'sealed_group_original_changed'):self.c.submit('plan',args)
  self.assertEqual(g.read(self.control/'group-binding.json'),original)
  self.assertEqual(sum(e[0]=='start' for e in self.ops.events),1)
 def test_missing_seal_or_changed_external_original_refuses_work_admission(self):
  argv=self.argv('plan')
  with self.assertRaisesRegex(RuntimeError,'sealed_group_required'):self.c.validate_argv('plan',argv)
  p=self.window/'fixed-batch-iam-scope.json';p.unlink();g.save(p,b'{}')
  with self.assertRaisesRegex(RuntimeError,'fixed_batch_original_changed'):self.c.submit('plan',argv)
 def test_first_historical_group_stays_exact(self):
  argv=self.argv('plan');value=g.decode(self.group);value['owner_confirmation_sha256']='b'*64;raw=g.wire(value);p=self.window/'group.json';p.unlink();g.save(p,raw)
  args=[('--group-sha256='+g.sha(raw)) if x.startswith('--group-sha256=') else x for x in argv]
  with self.assertRaisesRegex(RuntimeError,'fixed_first_group_required'):self.c.submit('plan',args)
 def test_non6000_batch_prior_uses_exact_independent_IAM_counts(self):
  self.future_batch();self.c.submit('plan',self.argv('plan'))
  row={'format_version':1,'kind':'mock_cohort_iam_receipt','compiled_source':self.d['iam_tool_source'],'action':'plan','group_manifest_sha256':g.sha(self.group),'iam_counts':dict(users=2,profiles=3,login_identities=2),'error_category':'','complete':True,'sql_closed':True,'redis_closed':True,'gate_released':True}
  out=self.run/'plan-stdout.json';proof=self.run/'plan-execution.json'
  def receipt(counts):
   row['iam_counts']=counts;raw=g.wire(row)
   for p in [out,proof]:
    if p.exists():p.unlink()
   g.save(out,raw);g.save(proof,g.wire({'format_version':1,'kind':'mock_cohort_iam_plan_execution','receipt_file':str(out),'receipt_sha256':g.sha(raw),'termination':'exited','exit_code':0}))
  receipt(dict(users=6000,profiles=6000,login_identities=6000))
  with self.assertRaisesRegex(RuntimeError,'prior_stage_release_unproven'):self.c.prior('capture')
  receipt(dict(users=2,profiles=3,login_identities=2));self.c.prior('capture')
  self.assertEqual(self.c.run,self.window/'iam-batches/window00718-batch0001/iam-run')
if __name__=='__main__':unittest.main()

"""CI-only actual PID1 unit/cgroup lifecycle; gate/runtime callbacks are file-only stubs.
No IAM binary, credentials, SQL, Redis, Docker, SSH, or production paths are used.
Run explicitly as root on Ubuntu/systemd with IAM_LOCAL_RECOVERY_SYSTEMD_NATIVE_REQUIRED=1.
"""
import importlib.util,json,os,pathlib,secrets,shutil,subprocess,sys,time,unittest
SPEC=importlib.util.spec_from_file_location('guard',pathlib.Path(__file__).with_name('guard.py'));g=importlib.util.module_from_spec(SPEC);SPEC.loader.exec_module(g)
HERE=pathlib.Path(__file__).resolve()
class FileCallbacks(g.Ops):
 def gate(self,c):
  g.need(g.decode(g.read(c.control/'native-fixture.json'))['gate_known'] is True,'native_fake_gate_unknown')
  return {'kind':'native_file_only_fake_gate','complete':True,'maintenance_gate_free':True,'not_actual_SQL_qualification':True}
 def runtime(self,c,op):
  if op=='stop':
   if (c.control/'native-delay-stop').exists():
    g.save(c.control/'native-stop-entered.json',b'{}');end=time.monotonic()+10
    while not (c.control/'native-release-stop.json').exists():
     g.need(time.monotonic()<end,'native_stop_release_deadline');time.sleep(.02)
   g.save(c.control/'fake-service-stopped.json',g.wire({'native_file_callback_only':True}));return {'stopped':True,'exit_code':0,'finished_at':'fixture'}
  g.need(op=='recover','native_only_recovery_callback')
  if (c.control/'native-late-stop-mode.json').exists():
   sequence=len(list(c.control.glob('native-runtime-attempt-*.json')))+1;g.save(c.control/('native-runtime-attempt-'+str(sequence)+'.json'),b'{}')
   actual=g.decode(g.read(c.control/'native-service-state.json'))
   g.observe_original_stop(c,actual)
   if not (c.window/'first-runtime-start-intent.json').exists():g.save(c.window/'first-runtime-start-intent.json',b'{"file_callback_marker_only":true}')
  p=c.control/'fake-service-restored.json'
  if not p.exists():g.save(p,g.wire({'fake_service_ready':True,'no_IAM_or_business_service_touched':True}))
  return {'ready':True,'native_file_callback_only':True}
def fixture(path):
 p=pathlib.Path(path);g.need(os.getuid()==0 and sys.platform=='linux' and p.parent==pathlib.Path('/var/tmp') and __import__('re').fullmatch('iam-local-recovery-native-[0-9a-f]{16}',p.name),'fixed_native_fixture_only')
 d=g.decode(g.read(p/'local-recovery/native-descriptor.json'));return g.Control(p/'local-recovery',d,FileCallbacks())
def child_entry(action,path):
 c=fixture(path)
 if action=='native-stop':
  print(json.dumps(c.stop(),separators=(',',':')));return 0
 if action in ['native-recover','native-normal-recover']:
  if action=='native-normal-recover':g.save(c.control/'native-recovery-attempt.json',b'{}')
  try:r=c.recover('deadline' if action=='native-recover' else 'normal');print(json.dumps(r,separators=(',',':')));return 0
  except RuntimeError:
   if not (c.control/'native-recovery-refused.json').exists():g.save(c.control/'native-recovery-refused.json',g.wire({'recovery_refused':True,'fake_service_not_started':not (c.control/'fake-service-restored.json').exists()}))
   return 1
 g.need(action=='native-client','closed_native_entry')
 dummy="import os,signal,subprocess,sys,time;signal.signal(signal.SIGTERM,signal.SIG_IGN);p=subprocess.Popen([sys.executable,'-c','import signal,time;signal.signal(signal.SIGTERM,signal.SIG_IGN);time.sleep(600)']);open(sys.argv[1],'w').write(str(os.getpid())+' '+str(p.pid));time.sleep(600)"
 g.save(c.control/'plan-request.json',g.wire({'native_dummy_only':True}))
 properties=['Description='+c.prefix+':plan','Type=exec','RemainAfterExit=yes','KillMode=control-group','Delegate=no','User=root','MemoryMax=64M','TasksMax=16','Restart=no','TimeoutStopSec=1s','SendSIGKILL=yes']
 c.ops.start(c.unit('plan'),properties,['/usr/bin/python3','-I','-B','-S','-c',dummy,str(c.control/'native-pids')])
 print(json.dumps({'client_submission_complete':True,'PID1_unit':c.unit('plan')}));return 0
class PID1Native(unittest.TestCase):
 @classmethod
 def setUpClass(cls):
  g.need(os.getenv('IAM_LOCAL_RECOVERY_SYSTEMD_NATIVE_REQUIRED')=='1' and os.getuid()==0 and sys.platform=='linux' and pathlib.Path('/proc/1/comm').read_text().strip()=='systemd' and pathlib.Path('/run/systemd/system').is_dir(),'explicit_native_root_PID1_lane_required')
 def setUp(self):
  self.window=pathlib.Path('/var/tmp')/('iam-local-recovery-native-'+secrets.token_hex(8));self.window.mkdir(mode=0o700)
  self.control=self.window/'local-recovery';self.control.mkdir(mode=0o700)
  self.d={'window_directory':str(self.window),'window_started_at':g.now().isoformat(),'data_deadline':(g.now()+g.dt.timedelta(seconds=2)).isoformat(),'hard_deadline':(g.now()+g.dt.timedelta(minutes=5,seconds=2)).isoformat(),'iam_tool_source':'0'*40,'logical_batch':'next-batch-0001','selector_sha256':g.FIXED_BATCHES['next-batch-0001'][0],'iam_scope_sha256':g.FIXED_BATCHES['next-batch-0001'][1]}
  self.c=g.Control(self.control,self.d,FileCallbacks());g.save(self.control/'native-descriptor.json',g.wire(self.d));g.save(self.control/'native-fixture.json',g.wire({'gate_known':True}))
 def tearDown(self):
  # Only names derived from this random fixture descriptor may be stopped/reset.
  units=[self.c.unit(x) for x in g.ACTIONS]+[self.c.prefix+'-recovery.timer',self.c.prefix+'-recovery.service']
  for u in units:
   subprocess.run(['/usr/bin/systemctl','stop',u],stdin=subprocess.DEVNULL,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=10)
   subprocess.run(['/usr/bin/systemctl','reset-failed',u],stdin=subprocess.DEVNULL,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=10)
  for u in units:
   for _ in range(30):
    info=self.c.ops.info(u)
    if info.get('LoadState')=='not-found':break
    time.sleep(.1)
   g.need(info.get('LoadState')=='not-found','native_owned_unit_cleanup_unproven')
  shutil.rmtree(self.window)
 def wait(self,predicate,seconds=20):
  end=time.monotonic()+seconds
  while time.monotonic()<end:
   if predicate():return
   time.sleep(.05)
  self.fail('fixed native observation deadline')
 def client(self):
  # A separate submitting process exits. PID1 must retain both dummy processes.
  r=subprocess.run(['/usr/bin/python3','-I','-B','-S',str(HERE),'native-client',str(self.window)],capture_output=True,timeout=15)
  self.assertEqual(r.returncode,0);self.wait(lambda:(self.control/'native-pids').exists())
  info=self.c.ops.info(self.c.unit('plan'));self.c.own(self.c.unit('plan'),info);self.assertEqual(info['ActiveState'],'active')
  pids=[int(p) for p in (self.control/'native-pids').read_text().split()];self.assertEqual(len(pids),2)
  self.assertTrue(all(pathlib.Path('/proc',str(p)).exists() for p in pids));return info
 def timer(self,retry=False):
  # Actual PID1 timer invokes the production recovery coordinator with only
  # explicit file-only callbacks in this test module, never production CLI flags.
  self.c.ops.start(self.c.prefix+'-recovery.service',['Description='+self.c.prefix+':native-recovery','Type=oneshot','MemoryMax=64M','TasksMax=16','TimeoutStartSec=20s']+(['Restart=on-failure','RestartSec=1s','StartLimitIntervalSec=0'] if retry else []),['/usr/bin/python3','-I','-B','-S',str(HERE),'native-recover',str(self.window)],timer=3)
 def test_client_exit_deadline_forced_group_reap_gc_and_reentry(self):
  before=self.client();self.timer();self.wait(lambda:(self.control/'service-recovery.json').exists())
  proof=g.decode(g.read(self.control/'plan-unit-stop-proof.json'));self.assertTrue(proof['stop_ACK']);self.assertTrue(proof['all_original_processes_gone']);self.assertGreaterEqual(len(proof['process_binding']['processes']),2)
  self.c.ops.original_gone(self.c.unit('plan'),proof['process_binding'])
  # systemd retains a failed unit after forcibly reaping its dummy group.
  # Simulate later GC only after retaining and validating the actual stop proof.
  subprocess.run(['/usr/bin/systemctl','reset-failed',self.c.unit('plan')],check=True,capture_output=True,timeout=10)
  self.wait(lambda:self.c.ops.info(self.c.unit('plan')).get('LoadState')=='not-found')
  original=g.read(self.control/'service-recovery.json');again=self.c.recover('normal');self.assertEqual(g.read(self.control/'service-recovery.json'),original);self.assertFalse(again['IAM_delete_completion_asserted'])
  self.assertTrue((self.control/'fake-service-restored.json').exists());self.assertFalse((self.control/'plan-execution.json').exists())
 def test_completed_success_phase_retains_metadata_until_recovery(self):
  unit=self.c.unit('plan');self.c.ops.start(unit,['Description='+self.c.prefix+':plan','Type=exec','RemainAfterExit=yes','KillMode=control-group','Delegate=no'],['/usr/bin/true'])
  self.wait(lambda:self.c.ops.info(unit).get('SubState')=='exited')
  info=self.c.ops.info(unit);self.c.own(unit,info);self.assertEqual(info['MainPID'],'0');g.save(self.control/'plan-request.json',b'{}')
  self.c.recover('normal');self.assertTrue((self.control/'service-recovery.json').exists());self.wait(lambda:self.c.ops.info(unit).get('LoadState')=='not-found');self.c.recover('normal')
 def test_actual_group_reaped_but_unknown_fake_gate_refuses_runtime(self):
  self.client();p=self.control/'native-fixture.json';p.unlink();g.save(p,g.wire({'gate_known':False}))
  self.timer();self.wait(lambda:(self.control/'native-recovery-refused.json').exists())
  self.assertFalse((self.control/'fake-service-restored.json').exists());self.assertFalse((self.control/'service-recovery.json').exists());self.assertTrue((self.control/'recovery-fence.json').exists())
 def test_late_stop_after_timer_recovery_never_stops_fake_service(self):
  self.timer();self.wait(lambda:(self.control/'service-recovery.json').exists())
  with self.assertRaisesRegex(RuntimeError,'recovery_fence'):self.c.stop()
  self.assertFalse((self.control/'fake-service-stopped.json').exists());self.assertTrue((self.control/'fake-service-restored.json').exists())
 def test_guard_stop_ACK_precedes_concurrent_local_recovery(self):
  self.d['data_deadline']=(g.now()+g.dt.timedelta(minutes=10)).isoformat();self.d['hard_deadline']=(g.now()+g.dt.timedelta(minutes=15)).isoformat();self.c=g.Control(self.control,self.d,FileCallbacks())
  p=self.control/'native-descriptor.json';p.unlink();g.save(p,g.wire(self.d))
  g.save(self.control/'armed.json',g.wire({'descriptor_sha256':g.sha(g.wire(self.d)),'only_guard_stop_permitted_after_this_ACK':True}))
  g.save(self.control/'runtime-before.json',b'[]');g.save(self.control/'native-delay-stop',b'{}')
  self.c.ops.start(self.c.prefix+'-recovery.service',['Description='+self.c.prefix+':native-recovery','Type=oneshot'],['/usr/bin/true'],timer=600)
  stop=subprocess.Popen(['/usr/bin/python3','-I','-B','-S',str(HERE),'native-stop',str(self.window)],stdout=subprocess.PIPE,stderr=subprocess.PIPE)
  self.wait(lambda:(self.control/'native-stop-entered.json').exists())
  recovery=subprocess.Popen(['/usr/bin/python3','-I','-B','-S',str(HERE),'native-normal-recover',str(self.window)],stdout=subprocess.PIPE,stderr=subprocess.PIPE)
  self.wait(lambda:(self.control/'native-recovery-attempt.json').exists());self.assertFalse((self.control/'fake-service-restored.json').exists())
  g.save(self.control/'native-release-stop.json',b'{}')
  _,stop_error=stop.communicate(timeout=15);_,recover_error=recovery.communicate(timeout=15)
  self.assertEqual(stop.returncode,0,stop_error.decode()[-200:]);self.assertEqual(recovery.returncode,0,recover_error.decode()[-200:])
  self.assertLessEqual((self.control/'runtime-stop-ACK.json').stat().st_mtime_ns,(self.control/'fake-service-restored.json').stat().st_mtime_ns)
 def test_actual_PID1_on_failure_retries_after_late_stop_without_new_delete(self):
  g.save(self.control/'runtime-before.json',b'[]')
  g.save(self.control/'runtime-stop-intent.json',g.wire({'CID':g.CID,'descriptor_sha256':g.sha(g.wire(self.d))}))
  g.save(self.control/'native-late-stop-mode.json',b'{}')
  state=self.control/'native-service-state.json';g.save(state,g.wire({'State':{'Running':True,'Pid':77,'Status':'running','FinishedAt':'','ExitCode':0}}))
  self.timer(retry=True);self.wait(lambda:(self.control/'native-recovery-refused.json').exists())
  self.assertFalse((self.control/'service-recovery.json').exists());self.assertTrue((self.control/'recovery-fence.json').exists())
  replacement=self.control/'native-state-replacement.json';g.save(replacement,g.wire({'State':{'Running':False,'Pid':0,'Status':'exited','FinishedAt':'fixed-finished','ExitCode':0}}));os.replace(replacement,state)
  self.wait(lambda:(self.control/'service-recovery.json').exists())
  self.assertGreaterEqual(len(list(self.control.glob('native-runtime-attempt-*.json'))),2)
  self.assertTrue((self.control/'original-runtime-stopped.json').exists());self.assertTrue((self.control/'fake-service-restored.json').exists())
  self.assertFalse((self.control/'plan-request.json').exists());self.assertFalse(g.decode(g.read(self.control/'service-recovery.json'))['IAM_delete_completion_asserted'])
if __name__=='__main__':
 if len(sys.argv)==3 and sys.argv[1] in ['native-client','native-recover','native-stop','native-normal-recover']:sys.exit(child_entry(sys.argv[1],sys.argv[2]))
 g.need(os.getenv('IAM_LOCAL_RECOVERY_SYSTEMD_NATIVE_REQUIRED')=='1' and os.getuid()==0 and sys.platform=='linux' and pathlib.Path('/proc/1/comm').read_text().strip()=='systemd' and pathlib.Path('/run/systemd/system').is_dir(),'explicit_native_root_PID1_lane_required')
 unittest.main()

#!/usr/bin/env python3
"""One fixed IAM-only window: PID1 owns cleanup units and an independent recovery timer."""
import contextlib,datetime as dt,fcntl,hashlib,json,math,os,pathlib,re,socket,stat,subprocess,sys,time
BASE=pathlib.Path('/var/tmp/qs-mock-direct-rule-20261016-v1')
MEMBER='window00518-batch0001'
CID='b4aa3ebe562c912dcf80392a5197acb63a91a1832fabc26e53c198bd397638c0'
RUNTIME_SHA='f4f2d6f39c25a57219c44e80a65ad487968639d652eb38b894c5dfcae42b69d1'
GROUP_SHA='84314eea560b97c8af6ccdce0df9b62628b67cf8d542e0384e442f4b88ad862b'
GATE_SHA='a9b452db9c7360e39a59bf9642113233fc439851151f3acec37bd9fe9d0f4a27'
MASTER_SHA='2960dd93051c1c9d50554b75b2e0ff64f426fdf9153e9bbba6908c7d830d0674'
PROTECTION_SHA='0a1222db9a85c30d3be94900b4ebd5c5d3c9b2ac41775c8aff108355e768e7c8'
# Only non-sensitive digests/counts are embedded; fixed private originals stay outside Git.
FIXED_BATCHES={
 'next-batch-0001':('0440d70763de04ac218e421361a37ee0d0a0dc2493fb15665d5a2f6a3c87415e','0bbad888f27f85260c4dc202030b69e2a80344ceb6c91efa42e1bc860a6df51b',(6000, 6000, 6000, 6000)),
 'next-batch-0002':('87f971061fa1b6a439bc469dbed2889276891ec918dcb0e6f051d880f67ed5e5','06670bd3c3c4994ca699d867ab69d6b869619ebc19dbc05c279d5483eda13517',(6000, 6000, 6000, 6000)),
 'next-batch-0003':('0b3c9e15d73ac675a5eafd3f77734e7ad19ffa143b1ed520a9fcff893dddf62b','aa10a86ab0250726c61a566bb8745f0117596dad03e5d45b17fc256ddaa217a9',(6000, 6000, 6000, 6000)),
 'next-batch-0004':('55f8ac1ce4e5a2e4c67ea7affbb50453f5e4d66b9c804426e23c3d787bce8bb3','01ec2aee0ec9db0a9d3e5eabbe1d9d156286e0b19ba16690052768c78be82834',(6000, 6000, 6000, 6000)),
 'next-batch-0005':('950a58fe3d57ebdbe49ee8e7332f6090e98bef84b7892b835d24d1b6c412eed7','0d1f4b5623ea6830a6fbb3191c4f69141d68d5bc390afd437f27ec0dfee1de5b',(6000, 6000, 6000, 6000)),
 'next-batch-0006':('bcd6c680fdb6ce50b91a053e35cf698353319787b37bda3ad46f5afdb5e93ee2','49b3b93332216a7a25d4ec2e8c27e4edd9afd452d5519b80dc7561374afc862b',(6000, 6000, 6000, 6000)),
 'next-batch-0007':('baf1b27c93c382f48490db16d0e65b5ed30e8d7614f74ff89c9acd9938882c16','af04e9a5552968c52019981231955cd4e74080c09fd76ce40bc8a20d57ced0f3',(6000, 6000, 6000, 6000)),
 'next-batch-0008':('d7ff9457f404f542b9ba7a4bef056d749cd302eddd67cea22e82df62b27387cf','75ec7a09f9ef4070b6a19d7720561e813cc5253fb53ccfc8cefddd550b54088c',(6000, 6000, 6000, 6000)),
 'next-batch-0009':('d4ea0adbfff62d2e3048a7c4c00461542f7210086a4eea69132a251ca449bb38','dfda4109543d19aacab88e4f3d2d235ade084c1946c655e88230fa00d9eb9ac6',(6000, 6000, 6000, 6000)),
 'next-batch-0010':('cc35f9351cefc472ac480fe180a92194a45d79d1ec8f5d97672d6127f61a2ca8','b932fbc1d6deb607e0722daec9b3fe726e762400715bc1c9d563e15dc03ccb18',(5985, 4456, 5985, 4456)),
 'next-batch-0011':('d4fa23108baf302c56f9e03e589a2b5524bbf03451198e8b5daf32f3c4e689de','fedd5021507b2b9c4abdfd2c13a5a04a6c167dca4a783c543e68d44d6789f5ef',(5947, 295, 5947, 295)),
 'next-batch-0012':('164c90d8c11173bffc7c7af8fab891be778ae3a664fefaee9faec63fbfce3ee1','61b192311ba2fb90a34313e41917a505aa2f8e602e75c04a75298654017e73af',(5936, 1402, 5942, 1402)),
 'next-batch-0013':('5b1d5e8cb3796b84196609f39d9541db0efc588ccead0020d447721844b7e3a1','f951d8cd15c9553223ba2c1360a3981bc145015e91fbbb3330a3d54fc813ab3d',(5999, 5612, 6000, 5612)),
 'next-batch-0014':('b10cd8b6296bf671087e50e7ff89d8656a16bc0529172bc04b657f3383882b52','d4d8e52fe0e459d5e71022834bf165ad2bb442f4e94e2a6da989b6045f90765b',(921, 849, 921, 849)),
}
ACTIONS=('plan','capture','apply','verify')
ENV_KEYS={'IAM_MOCK_CLEANUP_MYSQL_DSN','IAM_MOCK_CLEANUP_REDIS_URL'}
COMMON={'group','owner-confirmation','protected','frozen','event-catalog','archive'}
EXTRA={'qs-handoff','qs-capture','qs-apply','qs-clearance','qs-clearance-execution','messages-settled','approval','journal'}
def need(x,code):
 if not x:raise RuntimeError(code)
def sha(b):return hashlib.sha256(b).hexdigest()
def wire(x):return json.dumps(x,separators=(',',':'),sort_keys=True).encode()
def decode(raw):
 def pairs(items):
  out={}
  for k,v in items:need(k not in out,'duplicate_json_key');out[k]=v
  return out
 return json.loads(raw,object_pairs_hook=pairs)
def fixed_batch(d):
 name=d.get('logical_batch');need(isinstance(name,str) and name in FIXED_BATCHES,'fixed_batch_required')
 selector,scope,counts=FIXED_BATCHES[name]
 need(d.get('selector_sha256')==selector and d.get('iam_scope_sha256')==scope,'fixed_batch_original_SHA_required')
 return name,counts
def batch_originals(window,d):
 name,counts=fixed_batch(d)
 selector_raw=read(window/'fixed-batch-selector.json',16<<20);scope_raw=read(window/'fixed-batch-iam-scope.json',16<<20)
 need(sha(selector_raw)==d['selector_sha256'] and sha(scope_raw)==d['iam_scope_sha256'],'fixed_batch_original_changed')
 selector,scope=decode(selector_raw),decode(scope_raw)
 need(set(scope)=={'user_ids','profile_ids','login_identity_ids'},'fixed_IAM_scope_fields_required')
 ids=[selector.get('testee_ids'),scope['user_ids'],scope['profile_ids'],scope['login_identity_ids']]
 for values,count in zip(ids,counts):
  need(isinstance(values,list) and len(values)==count and all(isinstance(x,str) and re.fullmatch('[1-9][0-9]*',x) for x in values) and len(set(values))==count,'fixed_batch_typed_identity_required')
 need(selector.get('operation_id')=='daily-simulation-cohort-20261016' and selector.get('protected_manifest_sha256')==PROTECTION_SHA,'fixed_batch_protection_required')
 return selector,scope
def stamp(s):
 value=dt.datetime.fromisoformat(s.replace('Z','+00:00'));need(value.tzinfo is not None,'timezone_required');return value
def now():return dt.datetime.now(dt.timezone.utc)
def canonical(path):
 p=pathlib.Path(path);need(p.is_absolute() and str(p)==os.path.normpath(str(p)),'absolute_path_required')
 for q in (p,*p.parents):need(not q.is_symlink(),'symlink_refused')
 return p
def read(path,limit=256<<20,mode=0o600):
 p=canonical(path);fd=os.open(p,os.O_RDONLY|os.O_NOFOLLOW)
 try:
  s=os.fstat(fd);need(stat.S_ISREG(s.st_mode) and s.st_uid==os.geteuid() and stat.S_IMODE(s.st_mode)==mode and s.st_nlink==1 and s.st_size<=limit,'private_original_required')
  with os.fdopen(fd,'rb',closefd=False) as f:b=f.read(limit+1)
  signature=lambda z:(z.st_dev,z.st_ino,z.st_size,z.st_mtime_ns,z.st_ctime_ns,z.st_mode,z.st_uid,z.st_nlink)
  need(len(b)==s.st_size and signature(s)==signature(os.fstat(fd))==signature(p.lstat()),'original_changed');return b
 finally:os.close(fd)
def save(path,raw):
 p=canonical(path);fd=os.open(p,os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW|os.O_WRONLY,0o600)
 with os.fdopen(fd,'wb') as f:f.write(raw);f.flush();os.fsync(f.fileno())
 fd=os.open(p.parent,os.O_RDONLY|os.O_DIRECTORY)
 try:os.fsync(fd)
 finally:os.close(fd)
 need(read(p,max(1,len(raw)))==raw,'durable_readback_failed');return {'file':str(p),'sha256':sha(raw)}
@contextlib.contextmanager
def locked(directory):
 p=canonical(directory)/'coordinator.lock';fd=os.open(p,os.O_CREAT|os.O_NOFOLLOW|os.O_RDWR,0o600)
 try:
  s=os.fstat(fd);need(stat.S_ISREG(s.st_mode) and s.st_uid==os.geteuid() and stat.S_IMODE(s.st_mode)==0o600 and s.st_nlink==1,'private_lock_required')
  fcntl.flock(fd,fcntl.LOCK_EX);yield
 finally:fcntl.flock(fd,fcntl.LOCK_UN);os.close(fd)
def observe_original_stop(c,actual):
 # A Docker stop client timeout is not a daemon stop ACK. Only a fixed-CID
 # stopped observation may close that uncertainty; the old intent is untouched.
 intent_path=c.control/'runtime-stop-intent.json';intent_raw=read(intent_path,65536);intent=decode(intent_raw)
 need(intent.get('CID')==CID and intent.get('descriptor_sha256')==sha(wire(c.d)),'original_stop_intent_binding')
 before_sha=sha(read(c.control/'runtime-before.json',4<<20));path=c.control/'original-runtime-stopped.json'
 if path.exists():
  witness=decode(read(path,65536))
  need(witness.get('kind')=='fixed_original_IAM_stopped_observation' and witness.get('CID')==CID and witness.get('descriptor_sha256')==sha(wire(c.d)) and witness.get('runtime_before_sha256')==before_sha and witness.get('stop_intent_sha256')==sha(intent_raw) and witness.get('state_verified_stopped') is True,'original_stopped_observation_binding')
  if actual['State']['Running']:need(os.path.lexists(c.window/'first-runtime-start-intent.json'),'running_requires_original_forward_marker')
  return witness
 state=actual['State']
 need(state.get('Running') is False and state.get('Pid')==0 and state.get('Status')=='exited' and state.get('Restarting',False) is False and state.get('Dead',False) is False,'inflight_original_stop_unknown')
 witness={'kind':'fixed_original_IAM_stopped_observation','CID':CID,'descriptor_sha256':sha(wire(c.d)),'runtime_before_sha256':before_sha,'stop_intent_sha256':sha(intent_raw),'state_verified_stopped':True,'observed_at':now().isoformat(),'finished_at':state['FinishedAt'],'exit_code':state['ExitCode'],'no_Docker_stop_reissued':True}
 save(path,wire(witness));return witness
class Ops:
 def command(self,args,seconds=20):
  r=subprocess.run(args,stdin=subprocess.DEVNULL,capture_output=True,timeout=seconds,env={'PATH':'/usr/bin:/bin','LANG':'C','TZ':'UTC'})
  need(len(r.stdout)<=1<<20 and len(r.stderr)<=65536,'command_output_bound');need(r.returncode==0,'fixed_command_failed');return r.stdout
 def info(self,unit):
  fields=['LoadState','ActiveState','SubState','MainPID','ControlPID','ControlGroup','Description','FragmentPath','Result','ExecMainStatus','KillMode','Delegate']
  raw=self.command(['/usr/bin/systemctl','show',unit,'--no-pager']+['--property='+k for k in fields],10)
  return dict(line.split('=',1) for line in raw.decode().splitlines() if '=' in line)
 def start(self,unit,properties,args,timer=None):
  argv=['/usr/bin/systemd-run','--quiet','--unit='+unit]
  if timer is not None:argv+=['--on-active='+str(timer)+'s','--timer-property=AccuracySec=1s','--timer-property=RandomizedDelaySec=0']
  for value in properties:argv+=['--property='+value]
  self.command(argv+['--']+args,20)
 def stop(self,unit):self.command(['/usr/bin/systemctl','stop',unit],25)
 def empty(self,unit,info):
  need(info.get('ActiveState') in ['inactive','failed'] and info.get('MainPID')=='0' and info.get('ControlPID')=='0','unit_processes_not_reaped')
  group=info.get('ControlGroup','')
  if not group:return True
  need(group=='/system.slice/'+unit,'unexpected_owned_cgroup')
  candidates=[pathlib.Path('/sys/fs/cgroup')/group.lstrip('/'),pathlib.Path('/sys/fs/cgroup/systemd')/group.lstrip('/')]
  for parent in candidates:
   if parent.exists():
    for p in parent.rglob('cgroup.procs'):need(not p.read_text().strip(),'owned_cgroup_not_empty')
  return True
 def cgroup_files(self,unit,group):
  need(not group or group=='/system.slice/'+unit,'unexpected_owned_cgroup')
  if not group:return []
  out=[]
  for root in ['/sys/fs/cgroup','/sys/fs/cgroup/systemd']:
   parent=pathlib.Path(root)/group.lstrip('/')
   if parent.exists():out.extend(parent.rglob('cgroup.procs'))
  return out
 def process_ticks(self,pid):
  try:
   raw=pathlib.Path('/proc',str(pid),'stat').read_bytes();head,sep,tail=raw.rpartition(b') ')
   need(sep and int(head.split(b' ',1)[0])==pid,'process_identity_unreadable');fields=tail.split();need(len(fields)>19,'process_identity_unreadable');return int(fields[19])
  except (FileNotFoundError,ProcessLookupError):return None
 def bind_processes(self,unit,info):
  group=info.get('ControlGroup','');pids={int(info.get(k,'0')) for k in ['MainPID','ControlPID']}-{0}
  files=self.cgroup_files(unit,group)
  for path in files:pids.update(int(x) for x in path.read_text().split())
  if not group:need(not pids,'processes_without_bound_cgroup')
  bound=[]
  for pid in sorted(pids):
   ticks=self.process_ticks(pid)
   if ticks is not None:
    try:membership=pathlib.Path('/proc',str(pid),'cgroup').read_text()
    except (FileNotFoundError,ProcessLookupError):ticks=None
    else:need(any(line.rsplit(':',1)[-1]==group or line.rsplit(':',1)[-1].startswith(group+'/') for line in membership.splitlines()),'PID_not_in_owned_cgroup')
   bound.append({'pid':pid,'startticks':ticks,'absent_before_stop':ticks is None})
  return {'unit':unit,'control_group':group,'processes':bound}
 def original_gone(self,unit,binding):
  need(binding.get('unit')==unit,'foreign_process_binding')
  for path in self.cgroup_files(unit,binding['control_group']):need(not path.read_text().strip(),'owned_cgroup_not_empty')
  for item in binding['processes']:
   current=self.process_ticks(item['pid'])
   need(item['absent_before_stop'] is True or current is None or current!=item['startticks'],'original_PID_generation_still_alive')
  return True
 def runtime(self,c,operation):
  raw=read(c.window/'iam-runtime-recovery.py',128<<10);need(sha(raw)==RUNTIME_SHA,'runtime_source_changed')
  n={'__name__':'fixed_IAM_recovery_library'};exec(compile(raw,'fixed-IAM-runtime-recovery','exec'),n)
  n.update(ROOT=c.window,HOST='serverB',END=time.monotonic()+180,BEFORE_RAW=read(c.control/'runtime-before.json',4<<20))
  n['BEFORE']=decode(n['BEFORE_RAW']);need(len(n['BEFORE'])==1 and n['BEFORE'][0]['Id']==CID,'fixed_original_CID_required')
  if operation=='snapshot':
   actual=n['inspect'](CID);need(actual['State']['Running'],'original_runtime_not_running');return actual
  n['targets']() # Complete original Config/HostConfig/mount/path/args + seed=false.
  if operation=='stop':
   need(n['inspect'](CID)['State']['Running'],'original_IAM_not_running_before_stop')
   n['run'](n['D']+['update','--restart=no',CID],15)
   n['run'](n['D']+['stop','--time=-1',CID],120)
   actual=n['inspect'](CID);need(not actual['State']['Running'],'original_IAM_stop_unproven')
   return {'stopped':True,'exit_code':actual['State']['ExitCode'],'finished_at':actual['State']['FinishedAt'],'oom_killed':actual['State']['OOMKilled']}
  if operation=='stopped':
   need(not n['inspect'](CID)['State']['Running'],'original_IAM_must_be_stopped');return {'stopped':True}
  if operation=='verify':return n['verify']()
  need(operation=='recover','fixed_runtime_action_required')
  actual=n['inspect'](CID)
  if not (c.control/'runtime-stop-intent.json').exists():
   need(actual['State']['Running'],'unexpected_stop_without_guard_intent');return n['verify']()
  observe_original_stop(c,actual)
  n['forward']() # Existing exact marker is reused; never erase/rebuild it.
  if not n['inspect'](CID)['State']['Running']:n['start']()
  else:n['policy'](CID,n['BEFORE'][0]) # Covers start ACK followed by lost policy ACK.
  return n['verify']()
 def gate(self,c):
  raw=self.command([str(c.window/'iam-gate-observer')],10);r=decode(raw)
  need(r.get('kind')=='fixed_IAM_maintenance_gate_observer' and all(r.get(k) is True for k in ['complete','sql_identity_matches','maintenance_gate_free','sql_closed','connection_closed']),'maintenance_gate_release_unproven')
  return r
class Control:
 def __init__(self,control,d,ops=None):
  self.control=pathlib.Path(control);self.d=d;self.window=pathlib.Path(d['window_directory']);self.ops=ops or Ops()
  self.batch,self.batch_counts=fixed_batch(d)
  self.member=MEMBER if self.batch=='next-batch-0001' else 'window'+self.window.name[len('rule-window-'):]+'-batch0001'
  self.prefix='qs-iam-'+sha(wire(d))[:16];self.run=self.window/'iam-batches'/self.member/'iam-run'
  self.guard=self.window/'iam-local-guard.py'
 def unit(self,stage):need(stage in ACTIONS,'unknown_stage');return self.prefix+'-'+stage+'.service'
 def own(self,unit,info):
  need(unit in [self.unit(x) for x in ACTIONS],'foreign_unit_refused')
  need(info.get('Description')==self.prefix+':'+unit.split('-')[-1].split('.')[0] and info.get('FragmentPath')=='/run/systemd/transient/'+unit and info.get('KillMode')=='control-group' and info.get('Delegate')=='no','owned_unit_identity_unproven')
 def before_cutoff(self):need(now()<stamp(self.d['data_deadline']),'original_data_deadline_expired')
 def unfenced(self):need(not os.path.lexists(self.control/'recovery-fence.json'),'recovery_fence_blocks_all_stages')
 def group_authority(self,values):
  selector,scope=batch_originals(self.window,self.d);raw=read(values['group-file']);need(sha(raw)==values['group-sha256'],'group_original_changed');group=decode(raw)
  need(group.get('iam')==scope,'group_IAM_scope_differs_from_fixed_batch')
  qs=group.get('qs',{});graph=qs.get('graph',{})
  need(group.get('operation_id')==selector['operation_id'] and group.get('protected_manifest_sha256')==PROTECTION_SHA and graph.get('selector_sha256')==self.d['selector_sha256'] and graph.get('root_testee_ids')==selector['testee_ids'] and qs.get('selector')==selector.get('selector') and qs.get('binding')==selector.get('binding'),'group_QS_scope_differs_from_fixed_batch')
  if self.batch=='next-batch-0001':need(values['group-sha256']==GROUP_SHA,'fixed_first_group_required')
  return {'kind':'fixed_IAM_batch_group_binding','master_sha256':MASTER_SHA,'logical_batch':self.batch,'file':values['group-file'],'sha256':values['group-sha256'],'selector_sha256':self.d['selector_sha256'],'iam_scope_sha256':self.d['iam_scope_sha256']}
 def group_binding(self):return decode(read(self.control/'group-binding.json',65536))
 def validate_argv(self,stage,argv,allow_unsealed_plan=False):
  need(stage in ACTIONS and isinstance(argv,list) and argv and argv[0]==str(self.window/'iam-cleanup'),'fixed_binary_and_stage_required')
  values={}
  for arg in argv[1:]:
   need(isinstance(arg,str) and arg.startswith('--') and '=' in arg,'fixed_literal_flags_required');k,v=arg[2:].split('=',1);need(k not in values,'duplicate_flag');values[k]=v
  need(values.get('action')==stage and re.fullmatch('[1-9][0-9]*s',values.get('deadline','')),'fixed_action_deadline_required')
  seconds=int(values['deadline'][:-1]);need(0<seconds<=1500,'stage_extends_original_deadline')
  names=COMMON|(EXTRA if stage in ['apply','verify'] else set())
  if stage=='verify':names=names-{'qs-clearance','qs-clearance-execution'}
  need(set(values)=={'action','deadline'}|{name+suffix for name in names for suffix in ['-file','-sha256']},'fixed_stage_flag_set_required')
  for name in names:
   path=canonical(values[name+'-file']);need(path.is_relative_to(BASE),'input_outside_fixed_root')
   digest=values[name+'-sha256']
   empty=(name=='archive' and stage in ['plan','capture']) or (name=='journal' and stage in ['apply','verify'])
   need((digest=='' and empty) or re.fullmatch('[a-f0-9]{64}',digest),'input_SHA_required')
   if digest:need(sha(read(path))==digest,'input_original_changed')
  authority=self.group_authority(values);seal=self.control/'group-binding.json'
  if os.path.lexists(seal):need(decode(read(seal,65536))==authority,'sealed_group_original_changed')
  else:need(stage=='plan' and allow_unsealed_plan,'sealed_group_required')
  return values
 def prior(self,stage):
  if stage=='plan':return
  previous=ACTIONS[ACTIONS.index(stage)-1];p=decode(read(self.run/(previous+'-execution.json'),65536));out=read(self.run/(previous+'-stdout.json'),65536)
  need(p=={'format_version':1,'kind':'mock_cohort_iam_'+previous+'_execution','receipt_file':str(self.run/(previous+'-stdout.json')),'receipt_sha256':sha(out),'termination':'exited','exit_code':0} and type(p['exit_code']) is int and type(p['format_version']) is int,'prior_stage_not_known_success')
  r=decode(out);need(type(r.get('format_version')) is int and r['format_version']==1 and r.get('kind')=='mock_cohort_iam_receipt' and r.get('compiled_source')==self.d['iam_tool_source'] and r.get('action')==previous and r.get('group_manifest_sha256')==self.group_binding()['sha256'] and r.get('iam_counts')==dict(zip(['users','profiles','login_identities'],self.batch_counts[1:])) and r.get('error_category')=='' and all(r.get(k) is True for k in ['complete','sql_closed','redis_closed','gate_released']),'prior_stage_release_unproven')
 def stop(self):
  with locked(self.control):
   self.unfenced();self.before_cutoff()
   armed=decode(read(self.control/'armed.json',65536))
   need(armed.get('descriptor_sha256')==sha(wire(self.d)) and armed.get('only_guard_stop_permitted_after_this_ACK') is True,'guard_arm_ACK_required')
   timer=self.ops.info(self.prefix+'-recovery.timer');need(timer.get('LoadState')=='loaded' and timer.get('ActiveState')=='active','PID1_timer_not_armed')
   need((stamp(self.d['data_deadline'])-now()).total_seconds()>=180,'insufficient_original_stop_budget')
   need(not (self.control/'runtime-stop-intent.json').exists(),'original_stop_already_dispatched_no_retry')
   started=now().isoformat()
   save(self.control/'runtime-stop-intent.json',wire({'CID':CID,'descriptor_sha256':sha(wire(self.d)),'started_at':started,'only_guard_stop_permitted':True}))
   before=read(self.control/'runtime-before.json',4<<20)
   save(self.window/'writers-before.json',before)
   save(self.window/'stop-started.json',wire({'started':started}))
   actual=self.ops.runtime(self,'stop');need(actual.get('stopped') is True,'actual_original_stop_required')
   proof={'kind':'authorized_mock_writers_stopped','host':'serverB','window_started_at':self.d['window_started_at'],'actual_B_stop_started_at':started,'window_expires_at':self.d['hard_deadline'],'all_stopped':True,'count':1,'forced_kill_used':False,'instances':[{'cid':CID,'exit_code':actual['exit_code'],'finished_at':actual['finished_at'],'oom_killed':actual.get('oom_killed',False)}]}
   ref=save(self.window/'writers-stopped.json',wire(proof))
   save(self.control/'runtime-stop-ACK.json',wire({'kind':'fixed_IAM_guard_stop_ACK','CID':CID,'descriptor_sha256':sha(wire(self.d)),'writers_stopped':ref,'actual_stop_complete':True}))
   return proof
 def stopped(self):
  ack=decode(read(self.control/'runtime-stop-ACK.json',65536));need(ack.get('actual_stop_complete') is True and ack.get('CID')==CID and ack.get('descriptor_sha256')==sha(wire(self.d)) and ack.get('writers_stopped')=={'file':str(self.window/'writers-stopped.json'),'sha256':sha(read(self.window/'writers-stopped.json',65536))},'guard_stop_ACK_required')
  r=decode(read(self.window/'writers-stopped.json',65536));need(r.get('host')=='serverB' and r.get('all_stopped') is True and r.get('count')==1 and r.get('forced_kill_used') is False and [x['cid'] for x in r['instances']]==[CID],'actual_B_stop_required')
  self.ops.runtime(self,'stopped')
 def submit(self,stage,argv):
  with locked(self.control):
   self.unfenced();self.before_cutoff();values=self.validate_argv(stage,argv,allow_unsealed_plan=True);self.prior(stage);self.stopped()
   need(sha(read(self.window/'iam-cleanup',32<<20,0o500))==self.d['iam_tool_sha256'],'tool_binding_changed')
   need(self.ops.info(self.unit(stage)).get('LoadState')=='not-found','stage_already_dispatched_no_retry')
   if not os.path.lexists(self.control/'group-binding.json'):save(self.control/'group-binding.json',wire(self.group_authority(values)))
   save(self.control/(stage+'-request.json'),wire({'stage':stage,'argv':argv}))
   save(self.run/(stage+'-intent.json'),wire({'action':stage,'compiled_source':self.d['iam_tool_source'],'window_expires_at':self.d['data_deadline']}))
   remaining=max(1,math.floor((stamp(self.d['data_deadline'])-now()).total_seconds()))
   props=['Description='+self.prefix+':'+stage,'Type=exec','RemainAfterExit=yes','User=root','KillMode=control-group','Delegate=no','MemoryMax=512M','MemorySwapMax=0','CPUQuota=100%','TasksMax=64','OOMPolicy=stop','Restart=no','TimeoutStopSec=10s','RuntimeMaxSec='+str(remaining+15)+'s']
   self.ops.start(self.unit(stage),props,['/usr/bin/python3','-I','-B','-S',str(self.guard),'work',str(self.control),stage])
  return {'kind':'fixed_IAM_stage_dispatched','stage':stage,'unit':self.unit(stage),'no_completion_asserted':True}
 def work(self,stage):
  self.own(self.unit(stage),self.ops.info(self.unit(stage)))
  with locked(self.control):
   self.unfenced();self.before_cutoff();r=decode(read(self.control/(stage+'-request.json'),256<<10));need(r['stage']==stage,'stage_request_mismatch');self.validate_argv(stage,r['argv']);self.prior(stage);self.stopped()
   secret=decode(read('/dev/shm/qs-mock-direct-rule-20261016-v1/iam-env.json',1<<20))
   need(set(secret)==ENV_KEYS and all(isinstance(x,str) and x for x in secret.values()),'fixed_secret_ENV_required')
   streams=[]
   for name in [stage+'-stdout.json',stage+'-stderr.bin']:streams.append(os.fdopen(os.open(self.run/name,os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW|os.O_WRONLY,0o600),'wb'))
   seconds=min(int(next(x.split('=',1)[1][:-1] for x in r['argv'] if x.startswith('--deadline='))),math.floor((stamp(self.d['data_deadline'])-now()).total_seconds()))
   need(seconds>=1,'original_data_deadline_expired')
   argv=[('--deadline='+str(seconds)+'s') if x.startswith('--deadline=') else x for x in r['argv']]
   proc=subprocess.Popen(argv,stdin=subprocess.DEVNULL,stdout=streams[0],stderr=streams[1],env={'PATH':'/usr/bin:/bin','LANG':'C','TZ':'UTC','GOMAXPROCS':'1','GOMEMLIMIT':'384MiB',**secret},close_fds=True)
   save(self.control/(stage+'-child.json'),wire({'pid':proc.pid,'stage':stage,'unit':self.unit(stage),'tool_sha256':self.d['iam_tool_sha256']}))
  # PID1 owns this Python + Go cgroup. If the unit is killed before this wait
  # returns, no execution proof is invented by any collector/recovery path.
  code=proc.wait()
  for stream in streams:stream.flush();os.fsync(stream.fileno());stream.close()
  out=read(self.run/(stage+'-stdout.json'),65536);err=read(self.run/(stage+'-stderr.bin'),65536)
  save(self.run/(stage+'-execution.json'),wire({'format_version':1,'kind':'mock_cohort_iam_'+stage+'_execution','receipt_file':str(self.run/(stage+'-stdout.json')),'receipt_sha256':sha(out),'termination':'exited' if code>=0 else 'signaled','exit_code':code}))
  save(self.run/(stage+'-diagnostic.json'),wire({'stderr_sha256':sha(err),'stdout_bytes':len(out),'stderr_bytes':len(err),'child_reaped':True,'PID1_owned_unit':self.unit(stage)}))
  return code
 def recover(self,reason):
  need(reason in ['normal','deadline'],'fixed_recovery_reason')
  if reason=='deadline':need(now()>=stamp(self.d['data_deadline']),'deadline_recovery_too_early')
  with locked(self.control):
   fence=self.control/'recovery-fence.json'
   if not fence.exists():save(fence,wire({'kind':'fixed_IAM_forward_recovery_fence','descriptor_sha256':sha(wire(self.d)),'entered_at':now().isoformat(),'reason':reason,'all_subsequent_CLI_stages_forbidden':True}))
   else:need(decode(read(fence,65536))['descriptor_sha256']==sha(wire(self.d)),'foreign_recovery_fence')
   if now()>stamp(self.d['hard_deadline']) and not (self.control/'recovery-window-exceeded.json').exists():save(self.control/'recovery-window-exceeded.json',wire({'original_window_started_at':self.d['window_started_at'],'original_data_deadline':self.d['data_deadline'],'original_hard_deadline':self.d['hard_deadline'],'observed_at':now().isoformat(),'maintenance_30_minute_target_met':False,'data_deadline_not_extended':True,'service_recovery_only_may_continue':True}))
   stopped=[]
   for stage in ACTIONS:
    unit=self.unit(stage);before=self.ops.info(unit)
    stop_path=self.control/(stage+'-unit-stop-proof.json')
    if before.get('LoadState')=='not-found':
     if not (self.control/(stage+'-request.json')).exists():continue
     need(stop_path.exists(),'dispatched_unit_metadata_lost')
     prior=decode(read(stop_path,256<<10));need(prior.get('unit')==unit and prior.get('stop_ACK') is True and prior.get('all_original_processes_gone') is True,'owned_stop_original_required')
     self.ops.original_gone(unit,prior['process_binding']);stopped.append(prior);continue
    self.own(unit,before)
    binding=self.ops.bind_processes(unit,before)
    ownership=self.control/(stage+'-unit-before-stop.json')
    if not ownership.exists():save(ownership,wire({'unit':unit,'metadata':before,'process_binding':binding}))
    self.ops.stop(unit);after=self.ops.info(unit)
    if after.get('LoadState')!='not-found':self.own(unit,after);self.ops.empty(unit,after)
    self.ops.original_gone(unit,binding)
    proof={'stage':stage,'unit':unit,'stop_ACK':True,'process_binding':binding,'all_original_processes_gone':True,'result':after.get('Result'),'exit_status':after.get('ExecMainStatus'),'IAM_execution_outcome_not_reclassified':True}
    if not stop_path.exists():save(stop_path,wire(proof))
    stopped.append(proof)
   gate=self.ops.gate(self) # A free advisory gate never substitutes for reaping.
   ready=self.ops.runtime(self,'recover')
   receipt={'kind':'fixed_IAM_local_forward_service_recovery','source_runtime':'c8fc1fe61a32056368148e232e96d32763c15ba5','CID':CID,'recovered_at':now().isoformat(),'original_window_started_at':self.d['window_started_at'],'original_data_deadline':self.d['data_deadline'],'original_hard_deadline':self.d['hard_deadline'],'iam_ready_before_original_hard_deadline':now()<=stamp(self.d['hard_deadline']),'iam_ready_within_60_minutes_of_original_start':now()<=stamp(self.d['window_started_at'])+dt.timedelta(minutes=60),'whole_business_maintenance_and_recovery_targets_not_evaluated':True,'data_deadline_not_extended':True,'stopped_owned_units':stopped,'maintenance_gate':gate,'runtime':ready,'IAM_delete_completion_asserted':False,'unknown_action_results_not_reclassified':True,'archive_restore_performed':False,'entry_opened':False}
   path=self.control/'service-recovery.json'
   if not path.exists():save(path,wire(receipt))
   else:receipt['existing_recovery_original_sha256']=sha(read(path,1<<20));receipt['fresh_runtime_verified_without_rewriting_original']=True
   return receipt

def load(control):
 control=canonical(control);need(control.parent.parent==BASE and control.name=='local-recovery' and re.fullmatch('rule-window-[0-9]{5}',control.parent.name),'fixed_control_path_required')
 d=decode(read(control/'descriptor.json',65536));need(sha(wire(d))==decode(read(control/'descriptor-sha.json',65536))['sha256'],'descriptor_changed')
 validate(d);need(read(control/'boot-id',128).decode()==pathlib.Path('/proc/sys/kernel/random/boot_id').read_text().strip(),'boot_identity_changed');need(pathlib.Path(d['window_directory'])==control.parent,'window_binding_changed');return Control(control,d)
def validate(d):
 fields={'format_version','kind','window_directory','window_started_at','data_deadline','hard_deadline','iam_tool_source','iam_tool_sha256','guard_sha256','gate_observer_sha256','logical_batch','selector_sha256','iam_scope_sha256'}
 need(set(d)==fields and type(d['format_version']) is int and d['format_version']==1 and d['kind']=='fixed_batch_IAM_only_local_recovery','fixed_descriptor_required')
 fixed_batch(d)
 w=canonical(d['window_directory']);need(w.parent==BASE and re.fullmatch('rule-window-[0-9]{5}',w.name) and w.name not in ['rule-window-00518','rule-window-00618'],'new_IAM_only_window_required')
 need(re.fullmatch('[a-f0-9]{40}',d['iam_tool_source']),'source_SHA_required')
 for key in ['iam_tool_sha256','guard_sha256','gate_observer_sha256']:need(re.fullmatch('[a-f0-9]{64}',d[key]),'artifact_SHA_required')
 start,cutoff,hard=[stamp(d[k]) for k in ['window_started_at','data_deadline','hard_deadline']]
 need(cutoff-start==dt.timedelta(minutes=25) and hard-start==dt.timedelta(minutes=30),'original_25_30_window_required')
 need(sha(read(w/'iam-local-guard.py',128<<10,0o500))==d['guard_sha256'],'guard_source_changed')
 need(sha(read(w/'iam-cleanup',32<<20,0o500))==d['iam_tool_sha256'],'IAM_binary_changed')
 need(d['gate_observer_sha256']==GATE_SHA and sha(read(w/'iam-gate-observer',32<<20,0o500))==GATE_SHA,'gate_observer_changed')
 need(sha(read(w/'iam-runtime-recovery.py',128<<10))==RUNTIME_SHA,'runtime_library_changed')
def arm(path,digest):
 raw=read(path,65536);need(sha(raw)==digest,'descriptor_input_SHA_mismatch');d=decode(raw);validate(d);need(stamp(d['window_started_at'])<=now()<stamp(d['data_deadline'])-dt.timedelta(seconds=60),'arm_within_original_window_required')
 w=pathlib.Path(d['window_directory']);control=w/'local-recovery';control.mkdir(mode=0o700);save(control/'descriptor.json',wire(d));save(control/'descriptor-sha.json',wire({'sha256':sha(wire(d))}));save(control/'boot-id',pathlib.Path('/proc/sys/kernel/random/boot_id').read_text().strip().encode());c=Control(control,d);batch_originals(w,d)
 # Snapshot the one original container before B is stopped; no new runtime.
 actual=decode(c.ops.command(['/usr/bin/docker','--host','unix:///var/run/docker.sock','inspect','--type','container',CID],10))[0]
 need(actual['Id']==CID and actual['State']['Running'],'original_IAM_running_required');save(control/'runtime-before.json',wire([actual]));c.ops.runtime(c,'verify')
 for stage in ACTIONS:need(c.ops.info(c.unit(stage)).get('LoadState')=='not-found','preexisting_owned_unit_refused')
 remaining=math.ceil((stamp(d['data_deadline'])-now()).total_seconds());need(remaining>=1,'arm_deadline_expired')
 unit=c.prefix+'-recovery.service'
 need(c.ops.info(unit).get('LoadState')=='not-found' and c.ops.info(c.prefix+'-recovery.timer').get('LoadState')=='not-found','preexisting_recovery_unit_refused')
 properties=['Description='+c.prefix+':recovery','Type=oneshot','User=root','KillMode=control-group','Delegate=no','MemoryMax=192M','MemorySwapMax=0','CPUQuota=100%','TasksMax=64','Restart=on-failure','RestartSec=3s','StartLimitIntervalSec=0','TimeoutStartSec=240s']
 c.ops.start(unit,properties,['/usr/bin/python3','-I','-B','-S',str(c.guard),'recover',str(control),'deadline'],timer=remaining)
 timer=c.ops.info(c.prefix+'-recovery.timer');need(timer.get('LoadState')=='loaded' and timer.get('ActiveState')=='active','PID1_timer_not_armed')
 result={'kind':'fixed_IAM_local_recovery_armed','descriptor_sha256':sha(wire(d)),'timer':c.prefix+'-recovery.timer','data_deadline':d['data_deadline'],'only_guard_stop_permitted_after_this_ACK':True,'no_IAM_data_action_dispatched':True};save(control/'armed.json',wire(result));return result

def main():
 need(os.getuid()==0 and socket.gethostname().lower().split('.')[0]=='serverb','fixed_B_root_required')
 action=sys.argv[1]
 if action=='arm':need(len(sys.argv)==4,'fixed_arm_args');result=arm(sys.argv[2],sys.argv[3])
 else:
  need(action in ['stop','submit','work','recover','status'],'fixed_control_action');c=load(sys.argv[2])
  if action=='stop':need(len(sys.argv)==3,'fixed_stop_args');result=c.stop()
  elif action=='submit':need(len(sys.argv)==4,'fixed_submit_args');result=c.submit(sys.argv[3],decode(sys.stdin.buffer.read(256<<10)))
  elif action=='work':need(len(sys.argv)==4,'fixed_worker_args');return c.work(sys.argv[3])
  elif action=='recover':need(len(sys.argv)==4,'fixed_recovery_args');result=c.recover(sys.argv[3])
  else:result={'fenced':(c.control/'recovery-fence.json').exists(),'units':{x:c.ops.info(c.unit(x)) for x in ACTIONS},'service_recovery_original_exists':(c.control/'service-recovery.json').exists()}
 print(json.dumps(result,separators=(',',':')));return 0
if __name__=='__main__':
 try:sys.exit(main())
 except Exception as e:
  category=e.args[0] if type(e) is RuntimeError and e.args and isinstance(e.args[0],str) and re.fullmatch('[A-Za-z0-9_]{1,96}',e.args[0]) else 'unexpected_failure'
  print(json.dumps({'kind':'fixed_IAM_local_guard_failure','complete':False,'safe_error_type':type(e).__name__,'error_category':category,'no_delete_or_recovery_completion_asserted':True},separators=(',',':')));sys.exit(1)

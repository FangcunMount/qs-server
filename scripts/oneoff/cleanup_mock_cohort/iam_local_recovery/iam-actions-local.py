"""Private fixed IAM dispatcher seam; old admission stays in the pinned helper.
Root imports load_actions(control), then calls its original four public functions.
No source/credentials are accepted from a remote payload by this module.
"""
import importlib.util,pathlib,time
OLD_SHA='cb11661863d350491bb1d39395d2220aed41b1154596bfe32563dc607952e302'
def load_actions(control):
 control=pathlib.Path(control);path=control.parent/'iam-local-guard.py'
 spec=importlib.util.spec_from_file_location('fixed_local_IAM_guard',path);g=importlib.util.module_from_spec(spec);spec.loader.exec_module(g)
 g.need(g.os.getuid()==0 and g.socket.gethostname().lower().split('.')[0]=='serverb','fixed_B_root_required')
 c=g.load(control);raw=g.read(c.window/'iam-actions-v2.py',64<<10);g.need(g.sha(raw)==OLD_SHA,'original_dispatcher_changed')
 n={'__name__':'original_IAM_actions_with_PID1_backend'};exec(compile(raw,'pinned-IAM-action-admission','exec'),n)
 n['IAM_SOURCE']=c.d['iam_tool_source'] # Exact newly built tool source, never a receipt rewrite.
 def call(state,action,secret_env):
  g.need(action in g.ACTIONS and set(secret_env)==g.ENV_KEYS,'fixed_action_ENV_required')
  g.need(state['binary']==str(c.window/'iam-cleanup') and state['binary_sha256']==c.d['iam_tool_sha256'] and pathlib.Path(state['directory'])==c.run,'fixed_stage_state_required')
  g.need(secret_env==g.decode(g.read('/dev/shm/qs-mock-direct-rule-20261016-v1/iam-env.json',1<<20)),'original_private_ENV_changed')
  remaining=(min(n['_time'](state['window_expires_at']),g.stamp(c.d['data_deadline']))-g.now()).total_seconds()
  if action=='apply':remaining=min(remaining,(n['_time'](state['apply_expires_at'])-g.now()).total_seconds())
  g.need(remaining>=1,'original_window_expired');seconds=min(1500,int(remaining))
  argv=[state['binary'],'--action='+action,'--deadline='+str(seconds)+'s']
  for name,ref in state['inputs'].items():
   if action=='verify' and name in ['qs-clearance','qs-clearance-execution']:continue
   if ref[1]:n['_read'](ref)
   argv+=['--'+name+'-file='+ref[0],'--'+name+'-sha256='+ref[1]]
  c.submit(action,argv) # One intent/unit; a second call never re-dispatches.
  while g.now()<g.stamp(c.d['data_deadline']):
   if (c.run/(action+'-execution.json')).exists():break
   status=c.ops.info(c.unit(action))
   if status.get('ActiveState') in ['failed','inactive']:break
   time.sleep(.1)
  out=g.read(c.run/(action+'-stdout.json'),65536)
  proof=g.decode(g.read(c.run/(action+'-execution.json'),65536))
  g.need(proof=={'format_version':1,'kind':'mock_cohort_iam_'+action+'_execution','receipt_file':str(c.run/(action+'-stdout.json')),'receipt_sha256':g.sha(out),'termination':'exited','exit_code':0} and type(proof['exit_code']) is int,'stage_unknown_or_failed_no_retry')
  receipt=n['_receipt'](out,action,state);state[action+'_receipt']=(str(c.run/(action+'-stdout.json')),g.sha(out));return receipt
 n['_call']=call
 return n

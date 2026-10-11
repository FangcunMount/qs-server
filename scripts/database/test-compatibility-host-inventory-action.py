import ast
import base64
import copy
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import shutil
import stat
import struct
import subprocess
import sys
import tempfile
import time
import unittest
from unittest import mock

sys.dont_write_bytecode = True

SOURCE=Path(__file__).with_name('compatibility-host-inventory-action.py')
ROOT=SOURCE.parents[2]
WORKFLOW=ROOT/'.github/workflows/compatibility-host-inventory.yml'
FROZEN=SOURCE.parent
TRANSPORT=ROOT/'scripts/dbops/receipt-transport.py'
def load(path,name):
 s=importlib.util.spec_from_file_location(name,path);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m
m=load(SOURCE,'readonly_inventory_action')
f=load(FROZEN/'test-compatibility-retirement-host-inventory.py','readonly_inventory_fixture')

class Tests(unittest.TestCase):
 def setUp(self):
  self.tmp=tempfile.TemporaryDirectory(prefix='qs-host-inventory-action-unit-')
  self.addCleanup(self.tmp.cleanup);self.root=Path(self.tmp.name).resolve();self.root.chmod(0o700)
  self.matches=m.canonical([{'user':'deploy','host':'runner','addr':'127.0.0.1','laddr':'127.0.0.1','lport':'22'}])
  self.a={'protocol':m.PROTOCOL,'source_sha':'a'*40,'operation_id':'123-1','host_class':'server_a','route':'pinned_svra_retirement','inventory_sha256':m.INVENTORY_SHA,'transport_sha256':m.TRANSPORT_SHA,'wrapper_sha256':m.sha(SOURCE.read_bytes()),'matches_sha256':m.sha(self.matches),'run_binding':'current_workflow_run','total_seconds':120}
  self.approved=m.sha(m.canonical(self.a));self.run='456-1';self.req=m.request(self.a,self.matches,self.run)
  self.env={'GITHUB_SHA':self.a['source_sha'],'GITHUB_REF':'refs/heads/main','GITHUB_REPOSITORY':'FangcunMount/qs-server','GITHUB_RUN_ID':'456','GITHUB_RUN_ATTEMPT':'1','HOST_INVENTORY_VALIDATED_MAIN_SHA':self.a['source_sha'],'HOST_INVENTORY_APPROVAL_JSON':m.canonical(self.a).decode().rstrip('\n'),'HOST_INVENTORY_APPROVAL_SHA256':self.approved,'RUNNER_TEMP':str(self.root),'HOST_INVENTORY_SSH_HOST':'PRIVATE_HOST','HOST_INVENTORY_SSH_USERNAME':'PRIVATE_USER','HOST_INVENTORY_SSH_PORT':'22','HOST_INVENTORY_SSH_KEY':'-----BEGIN PRIVATE KEY-----\nPRIVATE_SECRET_KEY_SENTINEL\n-----END PRIVATE KEY-----\n','HOST_INVENTORY_SSH_FINGERPRINT':'SHA256:'+'A'*43}
 def rejected(self,category,fn):
  with self.assertRaises(m.Rejected) as e:fn()
  self.assertEqual(str(e.exception),category)
 def put(self,path,raw):
  path=Path(path);path.parent.mkdir(parents=True,exist_ok=True,mode=0o700);path.write_bytes(raw);path.chmod(0o600);return path
 def setup_local(self):
  home=self.root/'home';home.mkdir(mode=0o700)
  self.put(home/'.local/state/qs-host-inventory-approvals'/self.approved/'matches.json',self.matches)
  repo=self.root/'repo';self.put(repo/'scripts/database/compatibility-retirement-host-inventory.py',(FROZEN/'compatibility-retirement-host-inventory.py').read_bytes());self.put(repo/'scripts/dbops/receipt-transport.py',TRANSPORT.read_bytes())
  return home,repo
 def report(self):
  host=self.root/'host';host.mkdir(mode=0o700)
  fixture=f.Fixture(host);fixture.request=m.decode(self.req)
  runner=f.FakeRunner(fixture)
  v=f.m._collect(fixture.request,m.sha(self.req),f.m._Files(str(host)),runner,fixture.identity)
  for r in v['read_only_command_receipts']:r['executable_sha256']='e'*64
  v['tool_sha256']=m.INVENTORY_SHA
  return v
 def node(self,a=None,inputs=None,ctx=None,main=None,workflow_ref=None):
  # Executes the actual github-script body, not a mirror validator.
  text=WORKFLOW.read_text();start=text.index("            const reject =");end=text.index('\n  inventory:',start)
  script='\n'.join(line[12:] for line in text[start:end].splitlines())
  value=a or self.a
  raw=m.canonical(value).decode().rstrip('\n')
  default={'approval_json':raw,'approval_sha256':m.sha((raw+'\n').encode())}
  context={'payload':{'inputs':inputs if inputs is not None else default},'eventName':'workflow_dispatch','ref':'refs/heads/main','sha':self.a['source_sha'],'repo':{'owner':'FangcunMount','repo':'qs-server'}}
  if ctx:context.update(ctx)
  js="const context="+json.dumps(context)+";const core={setOutput:(k,v)=>{}};const github={rest:{repos:{getBranch:async()=>({data:{commit:{sha:"+json.dumps(main or self.a['source_sha'])+"}}})}}};(async()=>{try{"+script+";process.stdout.write('accepted')}catch(e){process.stdout.write('rejected');process.exitCode=1}})();"
  node=shutil.which('node');self.assertIsNotNone(node)
  env={'PATH':os.environ.get('PATH','/usr/bin:/bin'),'GITHUB_WORKFLOW_REF':workflow_ref or 'FangcunMount/qs-server/.github/workflows/compatibility-host-inventory.yml@refs/heads/main'}
  r=subprocess.run([node,'-e',js],stdout=subprocess.PIPE,stderr=subprocess.PIPE,env=env,timeout=10)
  self.assertEqual(r.stderr,b'');return r.returncode,r.stdout
 def service_projection(self):
  host=self.root/'service-host';host.mkdir(mode=0o700)
  suite=f.InventoryTests();suite.f=f.Fixture(host)
  runner,rows=suite.service_runner()
  suite.f.request=m.decode(self.req)
  report=f.m._collect(suite.f.request,m.sha(self.req),f.m._Files(str(host)),runner,suite.f.identity)
  for r in report['read_only_command_receipts']:r['executable_sha256']='e'*64
  report['tool_sha256']=m.INVENTORY_SHA
  raw=m.canonical(report);m.validate_report(raw,self.a,self.approved,self.req,self.run)
  value=m.projection(self.a,self.approved,self.run,self.req,raw,report,cleanup='verified')
  value['diagnostics']={'execution_stage':'complete','remote_cleanup':'verified','local_cleanup':'verified','registration_cleanup':'verified','cleanup_failure_stage':'none','cleanup_error_category':'none'}
  return value
 def test_service_artifact_preserves_exact_fields_hash_and_closed_armor_binding(self):
  value=self.service_projection();self.assertIsNotNone(value['qs_services'])
  m.validate_projection(value,self.a,self.approved,self.run,self.req)
  output=self.root/'github-output';output.write_text('')
  env={'RUNNER_TEMP':str(self.root),'GITHUB_OUTPUT':str(output)}
  closed=m.service_artifact(value,env)
  name='qs-service-observation-'+self.a['operation_id']+'-'+self.run
  path=self.root/name/'service-observation.private.json'
  raw,stamp=m.read_private(path);payload=m.decode(raw)
  self.assertEqual(closed['qs_service_artifact_sha256'],m.sha(raw))
  self.assertEqual(payload['service_observation'],value['qs_services'])
  self.assertEqual(payload['source_sha'],self.a['source_sha'])
  before=dict(closed);before.pop('qs_service_artifact_sha256')
  self.assertEqual(payload['actual_projection_sha256'],m.sha(m.canonical(before)))
  self.assertFalse(any(payload['capabilities'].values()))
  self.assertNotIn('qs_services',closed)
  transport=load(TRANSPORT,'service_artifact_transport')
  armor=transport.encode_armored_receipt(closed,schema=m.PROJECTION_SCHEMA)
  self.assertEqual(json.loads(transport.decode_armored_receipt(armor)),closed)
  self.assertIn('service_observation_artifact='+name,output.read_text())
  self.assertEqual(stamp[2]&0o777,0o600)
  self.rejected('private_namespace_conflict',lambda:m.service_artifact(value,env))
 def test_service_decoder_rejects_cross_host_bad_first_row_and_forged_hash(self):
  value=self.service_projection()
  for key,changed in [('host_role','server-d'),('observation_sha256','f'*64),('recheck_equal',False)]:
   bad=copy.deepcopy(value);bad['qs_services'][key]=changed
   self.rejected('transport_output_rejected',lambda:m.validate_projection(bad,self.a,self.approved,self.run,self.req))
  bad=copy.deepcopy(value);bad['qs_services']['containers'][0]['command']=['--password=PRIVATE_SENTINEL']
  body={k:v for k,v in bad['qs_services'].items() if k not in ('observation_sha256','recheck_equal')}
  bad['qs_services']['observation_sha256']=m.sha(m.canonical(body));bad['qs_service_observation_sha256']=bad['qs_services']['observation_sha256']
  self.rejected('transport_output_rejected',lambda:m.validate_projection(bad,self.a,self.approved,self.run,self.req))

 def test_descriptor_independent_sha_and_current_run_derivation(self):
  a=m.approval(m.canonical(self.a),self.approved,self.a['source_sha'],self.run)
  self.assertEqual(a,self.a);self.assertNotEqual(self.approved,m.sha(self.req))
  self.assertEqual(m.decode(self.req)['run_id'],'456-1');self.assertNotEqual(m.sha(self.req),m.sha(m.request(a,self.matches,'456-2')))
 def test_actual_node_accepts_closed_current_main_descriptor(self):self.assertEqual(self.node(),(0,b'accepted'))
 def test_actual_node_rejects_source_ref_main_and_unknown_inputs(self):
  cases=[{'ctx':{'ref':'refs/heads/old'}},{'ctx':{'sha':'b'*40}},{'main':'b'*40},{'workflow_ref':'FangcunMount/qs-server/.github/workflows/compatibility-host-inventory.yml@old'},{'inputs':{'approval_json':'{}','approval_sha256':'a'*64,'extra':'PRIVATE_SENTINEL'}}]
  for args in cases:
   with self.subTest(args=list(args)):self.assertEqual(self.node(**args),(1,b'rejected'))
 def test_actual_node_rejects_mixed_types_scope_budget_and_hash(self):
  for key,value in [('host_class',['worker']),('operation_id',['123-1']),('total_seconds',True),('route','arbitrary_host'),('wrapper_sha256','bad'),('extra','PRIVATE_SECRET')]:
   a=copy.deepcopy(self.a);a[key]=value
   with self.subTest(key=key):self.assertEqual(self.node(a=a),(1,b'rejected'))
 def test_duplicate_noncanonical_and_wrong_descriptor_hash_rejected(self):
  self.rejected('action_input_rejected',lambda:m.decode(b'{"x":1,"x":1}\n'))
  self.rejected('action_input_rejected',lambda:m.decode(b'{"x": 1}\n'))
  self.rejected('approval_rejected',lambda:m.approval(m.canonical(self.a),'f'*64,self.a['source_sha'],self.run))
 def test_source_binding_never_accepts_old_ref_or_other_repository(self):
  for k,v in [('GITHUB_REF','refs/tags/old'),('GITHUB_SHA','b'*40),('HOST_INVENTORY_VALIDATED_MAIN_SHA','b'*40),('GITHUB_REPOSITORY','other/repo')]:
   e=dict(self.env);e[k]=v;self.rejected('source_binding_rejected',lambda:m.source_binding(e,self.a))
 def test_collection_uses_server_a_worker_uses_server_d(self):
  for host,route,role in [('collection','pinned_svra_retirement','server_a'),('worker','pinned_svrd_m5_postcheck','server_d')]:
   a=dict(self.a,host_class=host,route=route);self.assertEqual(m.decode(m.request(a,self.matches,self.run))['host_role'],role)
 def test_matches_required_not_auto_discovered_or_caller_complete(self):
  self.rejected('matches_rejected',lambda:m.request(self.a,m.canonical([]),self.run))
  bad=m.decode(self.matches);bad[0]['complete']=True;a=dict(self.a,matches_sha256=m.sha(m.canonical(bad)))
  self.rejected('matches_rejected',lambda:m.request(a,m.canonical(bad),self.run))
  self.rejected('matches_rejected',lambda:m.request(self.a,m.canonical([{'user':'other'}]),self.run))
 def test_private_file_actual_fifo_symlink_hardlink_mode_and_parent_rejected(self):
  p=self.root/'asset';self.put(p,b'private')
  self.assertEqual(m.read_private(p)[0],b'private')
  os.link(p,self.root/'other');self.rejected('private_file_rejected',lambda:m.read_private(p));os.unlink(self.root/'other')
  p.chmod(0o644);self.rejected('private_file_rejected',lambda:m.read_private(p));p.chmod(0o600)
  link=self.root/'symlink';link.symlink_to(p);self.rejected('private_file_rejected',lambda:m.read_private(link))
  fifo=self.root/'fifo';os.mkfifo(fifo,0o600);before=time.monotonic();self.rejected('private_file_rejected',lambda:m.read_private(fifo));self.assertLess(time.monotonic()-before,1)
  child=self.root/'writable';child.mkdir(mode=0o700);self.put(child/'x',b'x');child.chmod(0o777);self.rejected('private_file_rejected',lambda:m.read_private(child/'x'))
 def test_private_fd_named_replacement_same_bytes_is_rejected(self):
  p=self.put(self.root/'asset',b'original');original=os.read;changed=False
  def read(fd,n):
   nonlocal changed
   raw=original(fd,n)
   if raw and not changed:
    changed=True;p.rename(self.root/'old');self.put(p,b'original')
   return raw
  with mock.patch.object(m.os,'read',side_effect=read):self.rejected('private_file_changed',lambda:m.read_private(p))
 def test_real_cli_fifo_does_not_hang_or_echo_private_path(self):
  fifo=self.root/'fifo_PRIVATE_SENTINEL';os.mkfifo(fifo,0o600)
  r=subprocess.run([sys.executable,str(SOURCE),'known-host','--route-file',str(fifo),'--key-type','ssh-ed25519','--key-blob','NONE'],stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=2,env={'PATH':'/usr/bin:/bin'})
  self.assertEqual(r.returncode,1);self.assertEqual(r.stderr,b'');self.assertNotIn(b'PRIVATE_SENTINEL',r.stdout);self.assertIn(b'private_file_rejected',r.stdout)
 def test_ssh_pin_is_standard_binary_wire_hash_not_text(self):
  wire=struct.pack('>I',11)+b'ssh-ed25519'+struct.pack('>I',32)+b'x'*32;blob=base64.b64encode(wire).decode();pin='SHA256:'+base64.b64encode(hashlib.sha256(wire).digest()).decode().rstrip('=')
  self.assertEqual(m.verify_handshake_key('ssh-ed25519',blob,pin),'qs-host-inventory ssh-ed25519 '+blob+'\n')
  text_pin='SHA256:'+base64.b64encode(hashlib.sha256(('ssh-ed25519 '+blob).encode()).digest()).decode().rstrip('=')
  self.rejected('host_key_rejected',lambda:m.verify_handshake_key('ssh-ed25519',blob,text_pin))
  self.rejected('host_key_rejected',lambda:m.verify_handshake_key('ssh-rsa',blob,pin))
 def test_actual_known_hosts_command_private_pin_no_original_target_output(self):
  wire=struct.pack('>I',11)+b'ssh-ed25519'+struct.pack('>I',32)+b'z'*32;blob=base64.b64encode(wire).decode();pin='SHA256:'+base64.b64encode(hashlib.sha256(wire).digest()).decode().rstrip('=')
  route=self.put(self.root/'route.json',m.canonical({'host':'PRIVATE_HOST','username':'PRIVATE_USER','port':'22','fingerprint':pin}))
  r=subprocess.run([sys.executable,str(SOURCE),'known-host','--route-file',str(route),'--key-type','ssh-ed25519','--key-blob',blob],stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=2)
  self.assertEqual(r.returncode,0);self.assertEqual(r.stderr,b'');self.assertNotIn(b'PRIVATE',r.stdout);self.assertEqual(r.stdout,('qs-host-inventory ssh-ed25519 '+blob+'\n').encode())
 def test_known_host_order_selects_only_the_actual_approved_algorithm(self):
  keys=[]
  for kind in ('ssh-rsa','ecdsa-sha2-nistp256','ssh-ed25519'):
   wire=struct.pack('>I',len(kind))+kind.encode()+struct.pack('>I',32)+b'x'*32
   keys.append((kind,base64.b64encode(wire).decode(),'SHA256:'+base64.b64encode(hashlib.sha256(wire).digest()).decode().rstrip('=')))
  route={'host':'PRIVATE_HOST','username':'PRIVATE_USER','port':'2222','fingerprint':keys[1][2]}
  raw=''.join('# PRIVATE_HOST:2222 SSH-2.0-fixed\n[PRIVATE_HOST]:2222 '+kind+' '+blob+'\n' for kind,blob,pin in keys).encode()
  with mock.patch.object(m,'capture',return_value=(0,raw,b'')) as run:
   self.assertEqual(m.prepare_pinned_host_order(route),'qs-host-inventory '+keys[1][0]+' '+keys[1][1]+'\n')
  run.assert_called_once_with(['/usr/bin/ssh-keyscan','-T','5','-p','2222','-t','rsa,ecdsa,ed25519','PRIVATE_HOST'],timeout=10,cap=65536)
 def test_known_host_order_rejects_unknown_missing_duplicate_failed_and_budget(self):
  kind='ssh-ed25519';wire=struct.pack('>I',len(kind))+kind.encode()+struct.pack('>I',32)+b'x'*32
  blob=base64.b64encode(wire).decode();pin='SHA256:'+base64.b64encode(hashlib.sha256(wire).digest()).decode().rstrip('=')
  route={'host':'PRIVATE_HOST','username':'PRIVATE_USER','port':'22','fingerprint':pin};line=('PRIVATE_HOST '+kind+' '+blob+'\n').encode()
  for code,out,err in [(0,b'',b''),(0,line+line,b''),(0,b'OTHER_HOST '+line.split(b' ',1)[1],b''),(0,b'PRIVATE_HOST unknown '+blob.encode()+b'\n',b''),(1,line,b''),(0,line,b'PRIVATE_ERROR'),(0,b'PRIVATE_HOST ssh-ed25519 invalid\n',b'')]:
   with self.subTest(code=code,length=len(out)),mock.patch.object(m,'capture',return_value=(code,out,err)):
    self.rejected('host_key_rejected',lambda:m.prepare_pinned_host_order(route))
  with mock.patch.object(m,'capture',return_value=(0,line,b'')):
   self.rejected('host_key_rejected',lambda:m.prepare_pinned_host_order(dict(route,fingerprint='SHA256:'+'A'*43)))
  with mock.patch.object(m,'capture',side_effect=m.Rejected('transport_budget_exceeded')):
   self.rejected('transport_budget_exceeded',lambda:m.prepare_pinned_host_order(route))
 def test_known_host_order_main_uses_private_pin_and_hostname_keeps_strict_check(self):
  kind='ssh-ed25519';wire=struct.pack('>I',len(kind))+kind.encode()+struct.pack('>I',32)+b'z'*32
  blob=base64.b64encode(wire).decode();pin='SHA256:'+base64.b64encode(hashlib.sha256(wire).digest()).decode().rstrip('=')
  route=self.put(self.root/'route-order.json',m.canonical({'host':'PRIVATE_HOST','username':'PRIVATE_USER','port':'22','fingerprint':pin}))
  import io
  stdout=io.StringIO()
  with mock.patch.object(sys,'argv',[str(SOURCE),'known-host','--route-file',str(route),'--key-type','NONE','--key-blob','NONE']),mock.patch.object(m,'capture',return_value=(0,('PRIVATE_HOST '+kind+' '+blob+'\n').encode(),b'')),mock.patch.object(sys,'stdout',stdout):
   self.assertEqual(m.main(),0)
  self.assertEqual(stdout.getvalue(),'qs-host-inventory '+kind+' '+blob+'\n');self.assertNotIn('PRIVATE',stdout.getvalue())
  self.rejected('host_key_rejected',lambda:m.verify_handshake_key('ssh-rsa',blob,pin))
 def test_closed_private_full_report_accepts_real_files_fake_host_reads(self):
  report=self.report();raw=m.canonical(report);self.assertEqual(m.validate_report(raw,self.a,self.approved,self.req,self.run),report)
  p=m.projection(self.a,self.approved,self.run,self.req,raw,report);self.assertFalse(any(p['capabilities'].values()));self.assertGreater(p['unknown_count'],0)
  receipt=load(TRANSPORT,'transport_fixture');encoded=receipt.encode_armored_receipt(p,schema=m.PROJECTION_SCHEMA);decoded=json.loads(receipt.decode_armored_receipt(encoded))
  self.assertEqual(decoded,p);self.assertNotIn('PRIVATE',str(decoded));self.assertNotIn('observations',decoded)
 def test_full_report_rejects_nested_secret_extra_field(self):
  v=self.report();v['observations']['docker']['containers'][0]['Env']=['PASSWORD=PRIVATE_SECRET']
  self.rejected('inventory_report_rejected',lambda:m.validate_report(m.canonical(v),self.a,self.approved,self.req,self.run))
 def test_full_report_rejects_secret_in_existing_digest_or_unknown(self):
  for mutate in [lambda v:v['identity'].__setitem__('os_sha256','PRIVATE_SECRET'),lambda v:v['unknown'].append('PASSWORD=PRIVATE_SECRET')]:
   # Each fixture report uses its own actual namespace.
   host=self.root/'host'
   if host.exists():shutil.rmtree(host)
   v=self.report();mutate(v)
   self.rejected('inventory_report_rejected',lambda:m.validate_report(m.canonical(v),self.a,self.approved,self.req,self.run))
 def test_full_report_rejects_wrong_source_request_tool_and_caps(self):
  v=self.report()
  for key,value in [('requested_source_sha','b'*40),('request_sha256','f'*64),('tool_sha256','f'*64),('host_role','server_d')]:
   b=copy.deepcopy(v);b[key]=value;self.rejected('inventory_report_rejected',lambda:m.validate_report(m.canonical(b),self.a,self.approved,self.req,self.run))
  v['capabilities']['drop_ready']=True;self.rejected('inventory_report_rejected',lambda:m.validate_report(m.canonical(v),self.a,self.approved,self.req,self.run))
 def test_full_report_rejects_all_match_complete_budget_or_missing_eof(self):
  v=self.report()
  for mutate in [lambda b:b['observations']['ssh'].__setitem__('all_match_contexts_complete',True),lambda b:b['observed_budgets'].__setitem__('elapsed_seconds',121),lambda b:b['end_rechecks'].pop(),lambda b:b.__setitem__('unknown',[])]:
   b=copy.deepcopy(v);mutate(b);self.rejected('inventory_report_rejected',lambda:m.validate_report(m.canonical(b),self.a,self.approved,self.req,self.run))
 def test_capture_real_secret_stdout_stderr_stays_private(self):
  code,out,err=m.capture([sys.executable,'-c','import os,sys;print("PRIVATE_SECRET");sys.stderr.write("PRIVATE_ERROR");print(os.getenv("MYSQL_PASSWORD"))'],env={'PATH':'/usr/bin:/bin'},timeout=2)
  self.assertEqual(code,0);self.assertIn(b'PRIVATE_SECRET',out);self.assertIn(b'None',out);self.assertEqual(err,b'PRIVATE_ERROR')
 def test_capture_real_timeout_reaps_owned_process(self):
  before=time.monotonic();self.rejected('transport_timeout',lambda:m.capture([sys.executable,'-c','import time;time.sleep(20)'],timeout=.1));self.assertLess(time.monotonic()-before,2)
 def test_capture_output_cap_not_public_diagnostics(self):
  self.rejected('transport_budget_exceeded',lambda:m.capture([sys.executable,'-c','print("PRIVATE_SECRET"*10000)'],cap=100,timeout=2))
 def test_exact_cleanup_refuses_unknown_file_changed_field_and_existing_output(self):
  d=self.root/'namespace';d.mkdir(mode=0o700);m.write_exclusive(d,'asset',b'body');records=m.records_for(d)
  self.rejected('private_namespace_conflict',lambda:m.write_exclusive(d,'asset',b'body'))
  self.put(d/'unknown',b'unknown');self.rejected('private_cleanup_unknown',lambda:m.clean_namespace(d,records));self.assertTrue((d/'unknown').exists());os.unlink(d/'unknown')
  self.put(d/'asset',b'changed');self.rejected('private_cleanup_unknown',lambda:m.clean_namespace(d,records));self.assertTrue(d.exists())
 def test_exact_cleanup_real_readback_zero(self):
  d=self.root/'namespace';d.mkdir(mode=0o700);m.write_exclusive(d,'asset',b'body');m.clean_namespace(d,m.records_for(d));self.assertFalse(d.exists())
 def test_parallel_exclusive_writers_only_one_wins(self):
  d=self.root/'namespace';d.mkdir(mode=0o700)
  from concurrent.futures import ThreadPoolExecutor
  def writer(unused):
   try:m.write_exclusive(d,'request.json',b'body');return 'created'
   except m.Rejected as e:return str(e)
  with ThreadPoolExecutor(max_workers=8) as pool:out=list(pool.map(writer,range(8)))
  self.assertEqual(out.count('created'),1);self.assertEqual(out.count('private_namespace_conflict'),7);self.assertEqual(m.read_private(d/'request.json')[0],b'body')
 def test_remote_cleanup_no_arbitrary_path_or_unknown_output_adoption(self):
  self.rejected('private_file_rejected',lambda:m.remote_cleanup(str(self.root),'a'*64))
 def test_mac_runner_refuses_without_matches_credentials_or_transport(self):
  a=dict(self.a,host_class='runner',route='runner_no_linux_channel');e=dict(self.env,HOST_INVENTORY_APPROVAL_JSON=m.canonical(a).decode().rstrip('\n'),HOST_INVENTORY_APPROVAL_SHA256=m.sha(m.canonical(a)))
  for k in list(e):
   if k.startswith('HOST_INVENTORY_SSH_'):del e[k]
  with mock.patch.object(Path,'home',side_effect=AssertionError('Match must not be accessed')):
   v=m.run_action(e,self.root,capture_fn=lambda *x,**kw:(_ for _ in ()).throw(AssertionError('SSH must not run')))
  self.assertEqual(v['error_category'],'linux_host_required');self.assertFalse(v['derived_request_created']);self.assertNotIn('derived_request_sha256',v);self.assertFalse(any(v['capabilities'].values()))
 def test_no_prepositioned_matches_does_not_connect_or_register(self):
  home=self.root/'home';home.mkdir(mode=0o700)
  with mock.patch.object(Path,'home',return_value=home):self.rejected('private_asset_missing',lambda:m.run_action(self.env,self.root,capture_fn=lambda *x,**kw:(_ for _ in ()).throw(AssertionError('SSH must not run'))))
  self.assertEqual(sorted(x.name for x in self.root.iterdir()),['home'])
 def test_fake_transport_private_config_and_clean_result(self):
  home,repo=self.setup_local();report=self.report();report_raw=m.canonical(report);calls=[]
  expected_key=self.env['HOST_INVENTORY_SSH_KEY'].encode()
  def transport(argv,**kw):
   calls.append((argv,kw))
   self.assertNotIn('PRIVATE_HOST',' '.join(argv));self.assertNotIn('PRIVATE_USER',' '.join(argv));self.assertNotIn('PRIVATE_SECRET',' '.join(argv))
   key=Path(argv[2]).parent/'ssh.key'
   self.assertEqual(key.read_bytes(),expected_key);self.assertEqual(stat.S_IMODE(key.stat().st_mode),0o600)
   if 'python3 -' in argv:return 0,b'{"created":true}\n',b''
   if argv[0]=='/usr/bin/scp':return 0,b'',b''
   if ' cleanup ' in argv[-1]:return 0,b'{"cleanup":"verified"}\n',b''
   return 0,m.canonical(m.projection(self.a,self.approved,self.run,self.req,report_raw,report)),b''
  for variant,key in [('lf',expected_key.decode()),('no_final_lf',expected_key[:-1].decode()),('crlf',expected_key.decode().replace('\n','\r\n'))]:
   with self.subTest(variant=variant):
    calls=[];env=dict(self.env,HOST_INVENTORY_SSH_KEY=key)
    with mock.patch.object(Path,'home',return_value=home):v=m.run_action(env,repo,capture_fn=transport)
    self.assertEqual(v['cleanup'],'verified');self.assertEqual(v['status'],'observed');self.assertFalse(any(v['capabilities'].values()));self.assertEqual(len(calls),4)
    self.assertFalse(any(x.name.startswith('qs-host-inventory-registration') for x in self.root.iterdir()))
    self.assertEqual(v['diagnostics'],{'execution_stage':'complete','remote_cleanup':'verified','local_cleanup':'verified','registration_cleanup':'verified','cleanup_failure_stage':'none','cleanup_error_category':'none'});self.assert_closed_diagnostics(v)
  self.rejected('route_rejected',lambda:m.route_file(dict(self.env,HOST_INVENTORY_SSH_KEY=expected_key.decode().replace('\n','\r'))))
 def test_transport_stderr_secret_is_not_published_unknown_remote_not_deleted(self):
  import io
  home,repo=self.setup_local();calls=[]
  def transport(argv,**kw):
   calls.append(argv)
   if 'python3 -' in argv:return 0,b'{"created":true}\n',b''
   return 1,b'PRIVATE_SECRET',b'PRIVATE_PASSWORD'
  output=io.StringIO()
  with mock.patch.object(Path,'home',return_value=home),mock.patch.object(sys,'stderr',output):v=m.run_action(self.env,repo,capture_fn=transport)
  self.assertEqual(v['cleanup'],'unknown');self.assertEqual(v['error_category'],'transport_failed');self.assertEqual(v['diagnostics']['execution_stage'],'asset_upload');self.assertEqual(v['diagnostics']['remote_cleanup'],'not_attempted');self.assertEqual(v['diagnostics']['local_cleanup'],'verified');self.assertEqual(v['diagnostics']['registration_cleanup'],'not_attempted');self.assertNotIn('PRIVATE',json.dumps(v));self.assertFalse(any('cleanup' in a[-1] for a in calls));self.assertTrue(any(x.name.startswith('qs-host-inventory-registration') for x in self.root.iterdir()))
  prefix='QS_HOST_INVENTORY_TRANSPORT_DIAGNOSTIC '
  self.assertEqual(output.getvalue(),prefix+m.canonical({'execution_stage':'asset_upload','exit_code':1,'stdout_bytes':14,'stderr_bytes':16,'stderr_nonempty':True,'reason':'other_or_unknown'}).decode())
  self.assert_closed_diagnostics(v)
  original_root=self.root
  cases=[(0,b'{"created":true}\n',b'PRIVATE_WARNING','stderr_on_zero_exit'),
         (255,b'PRIVATE_SECRET',b'PRIVATE_USER@PRIVATE_HOST: Permission denied (publickey).\n','publickey_authentication_denied'),
         (255,b'',b'Host key verification failed.\n','host_key_rejected'),
         (255,b'',b'ssh: connect to host PRIVATE_HOST port 22: Connection timed out\n','connection_timeout'),
         (255,b'',b'ssh: connect to host PRIVATE_HOST port 22: Connection refused\n','connection_refused'),
         (255,b'',b'ssh: Could not resolve hostname PRIVATE_HOST: Name or service not known\n','name_resolution_failed'),
         (1,b'',b'Traceback (most recent call last):\n  File "<stdin>", line 2, in <module>\nFileExistsError: PRIVATE_PATH\n','remote_python_failed'),
         (-15,b'PRIVATE_SECRET',b'PRIVATE_UNKNOWN Permission denied (publickey). trailing','other_or_unknown'),
         (m.Rejected('transport_timeout'),None,None,'other_or_unknown'),
         (OSError('PRIVATE_CAPTURE'),None,None,'other_or_unknown')]
  for index,(code,out,err,reason) in enumerate(cases):
   with self.subTest(reason=reason,exit_type=type(code).__name__):
    self.root=original_root/('case-'+str(index));self.root.mkdir(mode=0o700)
    home,repo=self.setup_local();env=dict(self.env,RUNNER_TEMP=str(self.root));calls=[];output=io.StringIO()
    def transport(argv,**kw):
     calls.append(argv)
     if isinstance(code,Exception):raise code
     return code,out,err
    with mock.patch.object(Path,'home',return_value=home),mock.patch.object(sys,'stderr',output):
     if isinstance(code,OSError):
      with self.assertRaises(OSError) as raised:m.run_action(env,repo,capture_fn=transport)
      self.assertIs(raised.exception,code)
     else:
      v=m.run_action(env,repo,capture_fn=transport)
      self.assertEqual(v['error_category'],'transport_timeout' if isinstance(code,m.Rejected) else 'transport_failed')
      self.assertEqual(v['cleanup'],'unknown');self.assertEqual(v['diagnostics']['execution_stage'],'remote_bootstrap');self.assertEqual(v['diagnostics']['remote_cleanup'],'not_attempted');self.assert_closed_diagnostics(v)
    lines=output.getvalue().splitlines();self.assertEqual(len(lines),1);self.assertTrue(lines[0].startswith(prefix))
    diagnostic=json.loads(lines[0][len(prefix):]);self.assertEqual(set(diagnostic),{'execution_stage','exit_code','stdout_bytes','stderr_bytes','stderr_nonempty','reason'})
    self.assertEqual(diagnostic,{'execution_stage':'remote_bootstrap','exit_code':'unknown' if isinstance(code,Exception) else code,'stdout_bytes':'unknown' if out is None else len(out),'stderr_bytes':'unknown' if err is None else len(err),'stderr_nonempty':'unknown' if err is None else bool(err),'reason':reason})
    self.assertNotIn('PRIVATE',output.getvalue());self.assertEqual(len(calls),1);self.assertFalse(any('cleanup' in a[-1] for a in calls));self.assertTrue(any(x.name.startswith('qs-host-inventory-registration') for x in self.root.iterdir()))
 def test_same_run_registration_is_never_overwritten(self):
  home,repo=self.setup_local()
  with mock.patch.object(Path,'home',return_value=home):
   v=m.run_action(self.env,repo,capture_fn=lambda *a,**kw:(1,b'',b'PRIVATE'))
   self.assertEqual(v['cleanup'],'unknown')
   self.rejected('private_namespace_conflict',lambda:m.run_action(self.env,repo,capture_fn=lambda *a,**kw:(_ for _ in ()).throw(AssertionError())))
 def test_cross_report_projection_binding_rejected(self):
  v=m.projection(self.a,self.approved,self.run,self.req,category='inventory_process_failed');v['run_id']='456-2'
  self.rejected('transport_output_rejected',lambda:m.validate_projection(v,self.a,self.approved,self.run,self.req))
 def remote_assets(self):
  name='qs-host-inventory-'+self.a['operation_id']+'-'+self.run+'-'+'a'*24
  d=self.root/name;d.mkdir(mode=0o700)
  bodies={'action.py':SOURCE.read_bytes(),'inventory.py':(FROZEN/'compatibility-retirement-host-inventory.py').read_bytes(),'receipt.py':TRANSPORT.read_bytes(),'approval.json':m.canonical(self.a),'request.json':self.req}
  for name,raw in bodies.items():m.write_exclusive(d,name,raw)
  manifest=m.canonical({name:m.sha(raw) for name,raw in bodies.items()});m.write_exclusive(d,'manifest.json',manifest)
  return d,m.sha(manifest)
 def test_remote_actual_files_registry_and_complete_projection_then_cleanup(self):
  d,pkg=self.remote_assets();raw=m.canonical(self.report())
  def child(argv,**kw):
   self.assertEqual(kw['timeout'],120);self.assertNotIn('MYSQL_PASSWORD',kw.get('env',{}));self.assertEqual(Path(argv[1]).name,'inventory.py')
   return 0,raw,b''
  with mock.patch.object(m,'remote_directory',return_value=d),mock.patch.object(m,'capture',side_effect=child):
   v=m.remote(d,self.approved,self.run,pkg)
   self.assertEqual(v['status'],'observed');self.assertEqual(v['cleanup'],'unknown');self.assertEqual(m.read_private(d/'report.private.json')[0],raw)
   self.assertEqual(m.remote_cleanup(d,pkg),{'cleanup':'verified'})
  self.assertFalse(d.exists())
 def test_remote_unregistered_report_cannot_be_adopted_or_deleted(self):
  d,pkg=self.remote_assets()
  with mock.patch.object(m,'remote_directory',return_value=d),mock.patch.object(m,'capture',return_value=(1,b'PRIVATE_SECRET',b'PRIVATE_ERROR')):
   v=m.remote(d,self.approved,self.run,pkg);self.assertEqual(v['error_category'],'inventory_process_failed')
   self.put(d/'report.private.json',b'PRIVATE_UNREGISTERED')
   self.rejected('private_cleanup_unknown',lambda:m.remote_cleanup(d,pkg))
  self.assertTrue((d/'report.private.json').exists())
 def test_remote_registered_report_replacement_is_blocked(self):
  d,pkg=self.remote_assets();raw=m.canonical(self.report())
  with mock.patch.object(m,'remote_directory',return_value=d),mock.patch.object(m,'capture',return_value=(0,raw,b'')):
   m.remote(d,self.approved,self.run,pkg)
   old=d/'report.private.json';old.rename(d/'old.private');self.put(old,raw)
   self.rejected('private_cleanup_unknown',lambda:m.remote_cleanup(d,pkg))
  self.assertTrue(old.exists())
 def test_remote_package_wrong_hash_unknown_member_and_cross_run_rejected(self):
  d,pkg=self.remote_assets()
  with mock.patch.object(m,'remote_directory',return_value=d):
   self.rejected('package_rejected',lambda:m.remote(d,self.approved,self.run,'f'*64))
   self.put(d/'extra.env',b'PRIVATE_PASSWORD')
   self.rejected('package_rejected',lambda:m.remote(d,self.approved,self.run,pkg));os.unlink(d/'extra.env')
   self.rejected('package_rejected',lambda:m.remote(d,self.approved,'456-2',pkg))
 def test_remote_cleanup_interruption_preserves_durable_registry(self):
  d,pkg=self.remote_assets()
  with mock.patch.object(m,'remote_directory',return_value=d),mock.patch.object(m,'capture',side_effect=m.Rejected('transport_timeout')):
   self.rejected('transport_timeout',lambda:m.remote(d,self.approved,self.run,pkg))
  self.assertTrue((d/'registry.json').exists());self.assertFalse((d/'report.private.json').exists())
 def test_runtime_private_report_plaintext_never_sent_to_controller(self):
  d,pkg=self.remote_assets();raw=m.canonical(self.report())
  with mock.patch.object(m,'remote_directory',return_value=d),mock.patch.object(m,'capture',return_value=(0,raw,b'')):
   v=m.remote(d,self.approved,self.run,pkg)
  public=m.canonical(v);self.assertNotIn(b'observations',public);self.assertNotIn(b'PRIVATE',public);self.assertNotIn(b'raw_',public);self.assertEqual(v['report_sha256'],m.sha(raw))
 def test_cleanup_race_captures_unknown_inode_without_deleting_it(self):
  d=self.root/'namespace';d.mkdir(mode=0o700);m.write_exclusive(d,'asset',b'body');records=m.records_for(d);original=os.rename
  def replace(src,dst,**kwargs):
   if src=='asset':
    original(d/'asset',self.root/'original.private')
    self.put(d/'asset',b'PRIVATE_UNKNOWN')
   return original(src,dst,**kwargs)
  with mock.patch.object(m.os,'rename',side_effect=replace):self.rejected('private_cleanup_unknown',lambda:m.clean_namespace(d,records))
  unknown=list(d.glob('.cleanup-*/asset'));self.assertEqual(len(unknown),1);self.assertEqual(unknown[0].read_bytes(),b'PRIVATE_UNKNOWN');self.assertTrue((self.root/'original.private').exists())
 def assert_closed_diagnostics(self,value):
  self.assertFalse(any(value['capabilities'].values()))
  self.assertNotIn('PRIVATE',json.dumps(value))
  transport=load(TRANSPORT,'closed_cleanup_diagnostics_transport')
  closed={k:v for k,v in value.items() if k!='qs_services'}
  armor=transport.encode_armored_receipt(closed,schema=m.PROJECTION_SCHEMA)
  self.assertEqual(json.loads(transport.decode_armored_receipt(armor)),closed)
  self.assertEqual(m.validate_projection(value,self.a,self.approved,self.run,self.req),value)
 def test_early_package_rejection_preserves_primary_without_claiming_cleanup(self):
  home,repo=self.setup_local();self.put(repo/'scripts/database/compatibility-retirement-host-inventory.py',b'PRIVATE_BAD_PACKAGE')
  with mock.patch.object(Path,'home',return_value=home):
   v=m.run_action(self.env,repo,capture_fn=lambda *a,**kw:(_ for _ in ()).throw(AssertionError('no transport')))
  self.assertEqual(v['error_category'],'package_rejected');self.assertEqual(v['cleanup'],'unknown')
  self.assertEqual(v['diagnostics'],{'execution_stage':'asset_prepare','remote_cleanup':'not_attempted','local_cleanup':'not_attempted','registration_cleanup':'not_attempted','cleanup_failure_stage':'none','cleanup_error_category':'none'})
  self.assertTrue(any(x.name.startswith('qs-host-inventory-registration') for x in self.root.iterdir()))
  self.assert_closed_diagnostics(v)
 def test_real_remote_cleanup_rejection_preserves_stage_and_unknown_asset(self):
  home,repo=self.setup_local();raw=m.canonical(self.report());d,pkg=self.remote_assets()
  def transport(argv,**kw):
   if 'python3 -' in argv:return 0,b'{"created":true}\n',b''
   if argv[0]=='/usr/bin/scp':return 0,b'',b''
   if ' cleanup ' in argv[-1]:
    self.put(d/'unknown.private',b'PRIVATE_UNKNOWN_REMOTE')
    try:m.remote_cleanup(d,pkg)
    except m.Rejected as e:
     self.assertEqual(str(e),'private_cleanup_unknown')
     return 1,m.canonical({'error_category':str(e)}),b''
    self.fail('unknown remote file must reject cleanup')
   return 0,m.canonical(m.remote(d,self.approved,self.run,pkg)),b''
  with mock.patch.object(Path,'home',return_value=home),mock.patch.object(m,'remote_directory',return_value=d),mock.patch.object(m,'capture',return_value=(0,raw,b'')):
   v=m.run_action(self.env,repo,capture_fn=transport)
  self.assertEqual(v['error_category'],'transport_failed');self.assertEqual(v['status'],'failed');self.assertEqual(v['cleanup'],'unknown')
  self.assertEqual(v['diagnostics']['execution_stage'],'complete');self.assertEqual(v['diagnostics']['remote_cleanup'],'unknown');self.assertEqual(v['diagnostics']['local_cleanup'],'verified');self.assertEqual(v['diagnostics']['registration_cleanup'],'not_attempted')
  self.assertEqual(v['diagnostics']['cleanup_failure_stage'],'remote_cleanup');self.assertEqual(v['diagnostics']['cleanup_error_category'],'transport_failed')
  self.assertEqual((d/'unknown.private').read_bytes(),b'PRIVATE_UNKNOWN_REMOTE');self.assertTrue((d/'registry.json').exists())
  self.assert_closed_diagnostics(v)
 def test_remote_primary_failure_survives_additional_cleanup_failure(self):
  home,repo=self.setup_local();d,pkg=self.remote_assets()
  def transport(argv,**kw):
   if 'python3 -' in argv:return 0,b'{"created":true}\n',b''
   if argv[0]=='/usr/bin/scp':return 0,b'',b''
   if ' cleanup ' in argv[-1]:
    self.put(d/'unknown.private',b'PRIVATE_UNKNOWN_REMOTE')
    try:m.remote_cleanup(d,pkg)
    except m.Rejected:return 1,b'',b''
    self.fail('unknown remote file must reject cleanup')
   return 0,m.canonical(m.remote(d,self.approved,self.run,pkg)),b''
  with mock.patch.object(Path,'home',return_value=home),mock.patch.object(m,'remote_directory',return_value=d),mock.patch.object(m,'capture',return_value=(1,b'PRIVATE_REPORT_ERROR',b'PRIVATE_STDERR')):
   v=m.run_action(self.env,repo,capture_fn=transport)
  self.assertEqual(v['error_category'],'inventory_process_failed');self.assertEqual(v['diagnostics']['execution_stage'],'remote_inventory')
  self.assertEqual(v['diagnostics']['cleanup_error_category'],'transport_failed');self.assertTrue((d/'unknown.private').exists())
  self.assert_closed_diagnostics(v)
 def test_real_local_cleanup_rejection_retains_unknown_assets_and_observation(self):
  home,repo=self.setup_local();report=self.report();raw=m.canonical(report);local=None
  def transport(argv,**kw):
   nonlocal local
   if 'python3 -' in argv:
    local=Path(argv[argv.index('-F')+1]).parent
    self.put(local/'unknown.private',b'PRIVATE_UNKNOWN_LOCAL')
    return 0,b'{"created":true}\n',b''
   if argv[0]=='/usr/bin/scp':return 0,b'',b''
   if ' cleanup ' in argv[-1]:return 0,b'{"cleanup":"verified"}\n',b''
   return 0,m.canonical(m.projection(self.a,self.approved,self.run,self.req,raw,report)),b''
  with mock.patch.object(Path,'home',return_value=home):v=m.run_action(self.env,repo,capture_fn=transport)
  self.assertEqual(v['error_category'],'private_cleanup_unknown');self.assertEqual(v['status'],'failed');self.assertIn('report_sha256',v)
  self.assertEqual(v['diagnostics']['remote_cleanup'],'verified');self.assertEqual(v['diagnostics']['local_cleanup'],'unknown');self.assertEqual(v['diagnostics']['registration_cleanup'],'not_attempted')
  self.assertEqual(v['diagnostics']['cleanup_failure_stage'],'local_cleanup');self.assertEqual(v['diagnostics']['cleanup_error_category'],'private_cleanup_unknown')
  self.assertEqual((local/'unknown.private').read_bytes(),b'PRIVATE_UNKNOWN_LOCAL');self.assertTrue((local.parent/'registration.json').exists())
  self.assert_closed_diagnostics(v)
 def test_registration_identity_drift_is_separate_unknown_cleanup(self):
  home,repo=self.setup_local();report=self.report();raw=m.canonical(report);state=None
  def transport(argv,**kw):
   nonlocal state
   if 'python3 -' in argv:return 0,b'{"created":true}\n',b''
   if argv[0]=='/usr/bin/scp':return 0,b'',b''
   if ' cleanup ' in argv[-1]:
    state=Path(argv[argv.index('-F')+1]).parent.parent
    p=state/'registration.json';body=p.read_bytes();p.rename(state/'original.private');self.put(p,body)
    return 0,b'{"cleanup":"verified"}\n',b''
   return 0,m.canonical(m.projection(self.a,self.approved,self.run,self.req,raw,report)),b''
  with mock.patch.object(Path,'home',return_value=home):v=m.run_action(self.env,repo,capture_fn=transport)
  self.assertEqual(v['error_category'],'private_cleanup_unknown');self.assertEqual(v['cleanup'],'unknown')
  self.assertEqual(v['diagnostics']['remote_cleanup'],'verified');self.assertEqual(v['diagnostics']['local_cleanup'],'verified');self.assertEqual(v['diagnostics']['registration_cleanup'],'unknown')
  self.assertEqual(v['diagnostics']['cleanup_failure_stage'],'registration_cleanup');self.assertTrue((state/'original.private').exists());self.assertTrue((state/'registration.json').exists())
  self.assert_closed_diagnostics(v)
 def test_closed_cleanup_diagnostics_reject_unknown_keys_false_verified_and_category(self):
  v=m.projection(self.a,self.approved,self.run,self.req,category='transport_failed')
  v['diagnostics']={'execution_stage':'remote_bootstrap','remote_cleanup':'not_attempted','local_cleanup':'verified','registration_cleanup':'not_attempted','cleanup_failure_stage':'none','cleanup_error_category':'none'}
  self.assert_closed_diagnostics(v)
  changes=[lambda x:x['diagnostics'].__setitem__('unknown','PRIVATE_SECRET'),lambda x:x['diagnostics'].__setitem__('execution_stage','PRIVATE_ACCOUNT'),lambda x:x['diagnostics'].__setitem__('registration_cleanup','verified'),lambda x:x['diagnostics'].__setitem__('cleanup_error_category','PRIVATE_STDERR'),lambda x:x['diagnostics'].__setitem__('remote_cleanup','unknown'),lambda x:x.__setitem__('cleanup','verified')]
  for change in changes:
   bad=copy.deepcopy(v);change(bad)
   self.rejected('transport_output_rejected',lambda:m.validate_projection(bad,self.a,self.approved,self.run,self.req))
 def test_emit_pinned_transport_projection_only(self):
  import io
  v=m.projection(self.a,self.approved,self.run,self.req,category='inventory_process_failed')
  output=io.StringIO()
  with mock.patch.object(sys,'stdout',output):m.emit(v,TRANSPORT)
  transport=load(TRANSPORT,'decode_projection_test');self.assertEqual(json.loads(transport.decode_armored_receipt(output.getvalue())),v)
  wrong=self.put(self.root/'wrong_receipt.py',b'print("PRIVATE_SECRET")')
  self.rejected('package_rejected',lambda:m.emit(v,wrong))
 def test_private_match_registration_leaf_directory_must_be_exact_700(self):
  home,repo=self.setup_local();(home/'.local/state/qs-host-inventory-approvals'/self.approved).chmod(0o755)
  with mock.patch.object(Path,'home',return_value=home):self.rejected('private_file_rejected',lambda:m.run_action(self.env,repo,capture_fn=lambda *a,**kw:(_ for _ in ()).throw(AssertionError('SSH must not run'))))
  self.assertFalse(any(x.name.startswith('qs-host-inventory-registration') for x in self.root.iterdir()))
 def test_workflow_source_mappings_lock_and_no_write_permissions(self):
  w=WORKFLOW.read_text();self.assertIn('group: production-deploy',w);self.assertIn('cancel-in-progress: false',w);self.assertIn('vars.SVRD_SSH_FINGERPRINT',w);self.assertIn('vars.SVRA_SSH_FINGERPRINT',w)
  self.assertNotIn('SUDO_PASSWORD',w);self.assertNotIn('MYSQL_',w);self.assertNotIn('MONGODB_',w);self.assertNotIn('id-token:',w);self.assertNotIn('accept-new',w);self.assertNotIn('appleboy',w);self.assertIn('may appear in',w);self.assertIn('entire GitHub run log',w)
  runner=w[w.index('  unsupported-runner:'):];self.assertNotIn('secrets.',runner);self.assertNotIn('HOST_INVENTORY_SSH_',runner)
 def test_implementation_never_installs_or_executes_arbitrary_shell(self):
  tree=ast.parse(SOURCE.read_text())
  for n in ast.walk(tree):
   if isinstance(n,ast.Call):
    for kw in n.keywords:self.assertFalse(kw.arg=='shell' and isinstance(kw.value,ast.Constant) and kw.value.value is True)
  strings=[n.value for n in ast.walk(tree) if isinstance(n,ast.Constant) and isinstance(n.value,str)]
  self.assertFalse(any(x.startswith(('sudo python','sshd -','docker ')) for x in strings))

class ObservationActionTests(unittest.TestCase):
 setUp=Tests.setUp
 rejected=Tests.rejected
 put=Tests.put
 node=Tests.node
 def descriptor(self):
  a=dict(self.a,protocol=m.OBSERVATION_PROTOCOL,context_mode=m.CONTEXT_MODE)
  del a['matches_sha256'];return a
 def bind2(self):
  self.a=self.descriptor();self.approved=m.sha(m.canonical(self.a));self.seed=m.request(self.a,None,self.run)
  api=m.inventory_api((FROZEN/'compatibility-retirement-host-inventory.py').read_bytes())
  with mock.patch.dict(os.environ,{'SSH_CONNECTION':'192.0.2.10 45001 192.0.2.20 22'},clear=True):
   self.req,self.session=m.observed_request(m.decode(self.seed),api)
  self.env.update(HOST_INVENTORY_APPROVAL_JSON=m.canonical(self.a).decode().rstrip('\n'),HOST_INVENTORY_APPROVAL_SHA256=self.approved)
 def report2(self, usedns='yes'):
  host=self.root/'host';host.mkdir(mode=0o700)
  fixture=f.Fixture(host)
  case=f.ObservationTests('test_v2_public_inventory_consumes_request_not_private_bypass');case.f=fixture;case.fixture2()
  fixture.request=m.decode(self.req);fixture.identity.update(uid=self.session['uid'],euid=self.session['euid'])
  fixture.put('/proc/124/status',('Uid:\t'+str(self.session['uid'])+'\t'+str(self.session['euid'])+'\t0\t0\n').encode())
  result=f.m._collect(fixture.request,m.sha(self.req),case.files(),f.ObservationRunner(fixture,usedns),fixture.identity)
  result['session_observation']['identity_connection_rechecked']=True
  for row in result['read_only_command_receipts']:row['executable_sha256']='e'*64
  result['tool_sha256']=m.INVENTORY_SHA
  return result
 def assets2(self):
  d=self.root/('qs-host-inventory-'+self.a['operation_id']+'-'+self.run+'-'+'a'*24);d.mkdir(mode=0o700)
  bodies={'action.py':SOURCE.read_bytes(),'inventory.py':(FROZEN/'compatibility-retirement-host-inventory.py').read_bytes(),'receipt.py':TRANSPORT.read_bytes(),'approval.json':m.canonical(self.a),'request.json':self.seed}
  for n,v in bodies.items():m.write_exclusive(d,n,v)
  manifest=m.canonical({n:m.sha(v) for n,v in bodies.items()});m.write_exclusive(d,'manifest.json',manifest)
  return d,m.sha(manifest)
 def test_actual_node_v2_accepts_explicit_mode_without_matches_hash(self):
  a=self.descriptor();self.assertEqual(self.node(a=a),(0,b'accepted'))
  self.assertEqual(m.approval(m.canonical(a),m.sha(m.canonical(a)),a['source_sha'],self.run),a)
  for key,value in [('context_mode','all_matches'),('matches_sha256','a'*64),('complete',True),('protocol',m.PROTOCOL),('context_mode',True)]:
   bad=dict(a);bad[key]=value
   with self.subTest(key=key):self.assertEqual(self.node(a=bad),(1,b'rejected'))
 def test_actual_node_v2_rejects_old_ref_head_budget_and_wrong_independent_sha(self):
  a=self.descriptor()
  for args in ({'ctx':{'ref':'refs/tags/old'}},{'main':'b'*40},{'ctx':{'sha':'b'*40}},{'inputs':{'approval_json':m.canonical(a).decode().rstrip('\n'),'approval_sha256':'f'*64}}):
   self.assertEqual(self.node(a=a,**args),(1,b'rejected'))
  bad=dict(a,total_seconds=121);self.assertEqual(self.node(a=bad),(1,b'rejected'))
  bad=dict(a,run_binding='independently_approved_runtime');self.assertEqual(self.node(a=bad),(1,b'rejected'))
 def test_v2_seed_actual_request_matches_and_approval_hash_are_distinct(self):
  self.bind2()
  self.assertNotIn('matches',m.decode(self.seed));self.assertNotIn('session',m.decode(self.seed))
  self.assertEqual(len({self.approved,m.sha(self.seed),m.sha(self.req)}),3)
  self.rejected('matches_rejected',lambda:m.request(self.a,self.matches,self.run))
  self.assertNotEqual(m.request(self.a,None,'456-2'),self.seed)
 def test_v2_full_closed_report_partial_projection_and_transport(self):
  self.bind2();v=self.report2();raw=m.canonical(v)
  self.assertEqual(m.validate_report(raw,self.a,self.approved,self.req,self.run),v)
  value=m.projection(self.a,self.approved,self.run,self.req,raw,v)
  self.assertEqual(value['protocol'],'qs_host_inventory_observation_v2');self.assertEqual(value['seed_request_sha256'],m.sha(self.seed))
  self.assertEqual(value['derived_request_sha256'],m.sha(self.req));self.assertEqual(value['host_status'],'host_unobserved')
  self.assertFalse(value['session_origin_proven']);self.assertTrue(value['partial']);self.assertFalse(any(value['capabilities'].values()))
  self.assertEqual(m.validate_projection(value,self.a,self.approved,self.run,self.seed),value)
  transport=load(TRANSPORT,'observed_transport');armor=transport.encode_armored_receipt(value,schema=m.PROJECTION_SCHEMA)
  self.assertEqual(json.loads(transport.decode_armored_receipt(armor)),value)
  for hidden in ('192.0.2','username','PRIVATE','observations','ssh_connection'):self.assertNotIn(hidden,json.dumps(value))
 def test_v2_v1_report_origin_true_unknown_host_match_or_cross_run_rejected(self):
  self.bind2();v=self.report2()
  changes=[lambda x:x.__setitem__('protocol',m.INVENTORY_PROTOCOL),lambda x:x['session_observation'].__setitem__('origin_proven',True),lambda x:x['session_observation'].__setitem__('identity_connection_rechecked',False),lambda x:x['session_observation'].__setitem__('usedns','no'),lambda x:x['session_observation'].__setitem__('derived_matches_sha256','f'*64),lambda x:x.__setitem__('run_id','456-2'),lambda x:x['session_observation'].__setitem__('extra','PRIVATE_SECRET')]
  for change in changes:
   bad=copy.deepcopy(v);change(bad);self.rejected('inventory_report_rejected',lambda:m.validate_report(m.canonical(bad),self.a,self.approved,self.req,self.run))
 def test_v2_projection_cannot_claim_independent_match_approval_or_full_origin(self):
  self.bind2();v=self.report2();value=m.projection(self.a,self.approved,self.run,self.req,m.canonical(v),v)
  for change in [lambda x:x.__setitem__('matches_sha256',m.sha(self.matches)),lambda x:x.__setitem__('session_origin_proven',True),lambda x:x.__setitem__('partial',False),lambda x:x.__setitem__('seed_request_sha256','f'*64),lambda x:x.__setitem__('derived_request_created',False)]:
   bad=copy.deepcopy(value);change(bad);self.rejected('transport_output_rejected',lambda:m.validate_projection(bad,self.a,self.approved,self.run,self.seed))
 def test_v2_remote_final_session_actual_identity_beforeafter_private_request(self):
  self.bind2();raw=m.canonical(self.report2());d,pkg=self.assets2();calls=[]
  def child(argv,**kw):
   calls.append(argv);self.assertEqual(Path(argv[3]).name,'observed-request.json')
   self.assertEqual(set(kw['env']),{'PATH','LANG','LC_ALL','SSH_CONNECTION'})
   self.assertEqual(kw['env']['SSH_CONNECTION'],self.session['ssh_connection'])
   self.assertEqual(m.read_private(d/'observed-request.json')[0],self.req)
   return 0,raw,b''
  with mock.patch.dict(os.environ,{'SSH_CONNECTION':self.session['ssh_connection'],'USER':'FORGED'},clear=True),mock.patch.object(m,'remote_directory',return_value=d),mock.patch.object(m,'capture',side_effect=child):
   result=m.remote(d,self.approved,self.run,pkg)
   self.assertEqual(result['status'],'observed');self.assertEqual(len(calls),1)
   self.assertNotIn('FORGED',json.dumps(result));self.assertEqual(m.remote_cleanup(d,pkg),{'cleanup':'verified'})
  self.assertFalse(d.exists())
 def test_v2_remote_connection_changed_or_missing_refuses_without_adoption(self):
  self.bind2();raw=m.canonical(self.report2());d,pkg=self.assets2()
  with mock.patch.dict(os.environ,{},clear=True),mock.patch.object(m,'remote_directory',return_value=d):
   self.rejected('session_observation_rejected',lambda:m.remote(d,self.approved,self.run,pkg))
  self.assertFalse((d/'registry.json').exists())
  def child(argv,**kw):
   os.environ['SSH_CONNECTION']='192.0.2.11 45001 192.0.2.20 22';return 0,raw,b''
  with mock.patch.dict(os.environ,{'SSH_CONNECTION':self.session['ssh_connection']},clear=True),mock.patch.object(m,'remote_directory',return_value=d),mock.patch.object(m,'capture',side_effect=child):
   self.rejected('session_observation_changed',lambda:m.remote(d,self.approved,self.run,pkg))
  self.assertTrue((d/'registry.json').exists());self.assertFalse((d/'report.private.json').exists())
 def test_v2_no_match_asset_access_and_fake_transport_current_run_source_pinned(self):
  self.bind2();repo=self.root/'repo';self.put(repo/'scripts/database/compatibility-retirement-host-inventory.py',(FROZEN/'compatibility-retirement-host-inventory.py').read_bytes());self.put(repo/'scripts/dbops/receipt-transport.py',TRANSPORT.read_bytes())
  raw=m.canonical(self.report2());calls=[]
  def transport(argv,**kw):
   calls.append(argv)
   if 'python3 -' in argv:return 0,b'{"created":true}\n',b''
   if argv[0]=='/usr/bin/scp':return 0,b'',b''
   if ' cleanup ' in argv[-1]:return 0,b'{"cleanup":"verified"}\n',b''
   return 0,m.canonical(m.projection(self.a,self.approved,self.run,self.req,raw,m.decode(raw))),b''
  with mock.patch.object(Path,'home',side_effect=AssertionError('v2 must not discover registered Matches')):
   result=m.run_action(self.env,repo,capture_fn=transport)
  self.assertEqual(result['cleanup'],'verified');self.assertEqual(result['status'],'observed');self.assertEqual(len(calls),4)
  self.assertEqual(m.validate_projection(result,self.a,self.approved,self.run,self.seed),result)
  self.assertFalse(any(x.name.startswith('qs-host-inventory-registration') for x in self.root.iterdir()))
 def test_v2_mac_runner_refuses_without_session_or_credentials(self):
  self.bind2();a=dict(self.a,host_class='runner',route='runner_no_linux_channel');env=dict(self.env,HOST_INVENTORY_APPROVAL_JSON=m.canonical(a).decode().rstrip('\n'),HOST_INVENTORY_APPROVAL_SHA256=m.sha(m.canonical(a)))
  for k in list(env):
   if k.startswith('HOST_INVENTORY_SSH_'):del env[k]
  with mock.patch.object(Path,'home',side_effect=AssertionError('no matches')),mock.patch.dict(os.environ,{},clear=True):
   result=m.run_action(env,self.root,capture_fn=lambda *a,**k:(_ for _ in ()).throw(AssertionError('no SSH')))
  self.assertEqual(result['status'],'unsupported');self.assertFalse(result['derived_request_created']);self.assertFalse(any(result['capabilities'].values()))
 def test_v2_inventory_approved_source_fd_same_bytes_replacement_rejected(self):
  original=os.read;path=SOURCE.with_name('compatibility-retirement-host-inventory.py');real=path.read_bytes();changed=False
  # Replace an independent copy and redirect only the fixed __file__ test seam.
  d=self.root/'source';d.mkdir(mode=0o700);self.put(d/path.name,real)
  old=m.__file__;m.__file__=str(d/SOURCE.name)
  def read(fd,n):
   nonlocal changed
   raw=original(fd,n)
   if raw and not changed:
    changed=True;(d/path.name).rename(d/'original.private');self.put(d/path.name,real)
   return raw
  try:
   with mock.patch.object(m.os,'read',side_effect=read):self.rejected('package_rejected',m.INVENTORY_SOURCE_BYTES)
  finally:m.__file__=old

if __name__=='__main__':unittest.main(verbosity=2)

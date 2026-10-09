#!/usr/bin/env python3
"""Offline metadata/current-run derivation only. No sudo/DB/Docker/SSH proof."""
import ast
import copy
import importlib.util
import json
from pathlib import Path
import unittest

spec=importlib.util.spec_from_file_location('window_tool',Path(__file__).with_name('compatibility-window-tool.py'))
tool=importlib.util.module_from_spec(spec);spec.loader.exec_module(tool)

class WindowToolMetadata(unittest.TestCase):
    def request(self):
        return {'format_version':1,'kind':'compatibility_retirement_lifecycle_request','tool_source_sha':'a'*40,'original_source_sha':'b'*40,
                'operation_id':'12-1','actual_run_id':'','manifest_sha256':'c'*64,'archive_directory':'/approved/archive','window_directory':'/approved/window','journal_directory':'/approved/journal',
                'archive_approval':{'InventorySHA256':'1'*64,'SQLMetadataSHA256':'2'*64,'MongoMetadataSHA256':'3'*64,'OrderedMongoSchemaSHA256':'4'*64,'SourceSHA':'b'*40,'OperationID':'12-1','RunID':'11-1','RequestHash':'5'*64},
                'recovery':{'source_sha':'b'*40,'operation_id':'12-1','original_run_id':'11-1','actual_run_id':'','manifest_sha256':'c'*64,'archive_sha256':'','mysql_non_target_sha256':'','mongodb_non_target_sha256':'6'*64,'mysql_head':99,'mongodb_head':38}}
    def approval(self,r,stage='prepare'):
        value = {'format_version':1,'kind':'independent_compatibility_window_tool_approval','dispatcher_source_sha':'d'*40,'tool_source_sha':'a'*40,
                'original_source_sha':'b'*40,'operation_id':'12-1','original_run_id':'11-1','stage':stage,'target_hash':tool.TARGET,'manifest_sha256':'c'*64,
                'request_template_sha256':tool.digest(tool.canonical(r)),'tool_binary_sha256':{'amd64':'7'*64,'arm64':'8'*64},'b_image_id':'','b_program_sha256':''}
        if stage != 'prepare':
            scope = {k:value[k] for k in ('dispatcher_source_sha','tool_source_sha','original_source_sha','operation_id','original_run_id','manifest_sha256')}
            scope.update(format_version=1,kind='approved_runner_workflow_quarantine_scope',repository_id='21',owner_id='22',actor_id='23',workflow_id=24,workflow_ids=[24,25],job_name='Retire exact private lifecycle stage with workflow quarantine',runner_id=26)
            value['workflow_scope'] = scope
            r.setdefault('writer_control',dict(workflow_scope_sha256=tool.digest(tool.canonical(scope))))
            value['request_template_sha256']=tool.digest(tool.canonical(r))
        return value
    def approve(self,a):
        raw=tool.canonical(a)
        return tool.approve(raw[:-1].decode(),tool.digest(raw),'d'*40,a['stage'],'12-1','c'*64,a['request_template_sha256'])
    def test_independent_dispatcher_tool_original_facts_have_distinct_bindings(self):
        r=self.request();a=self.approval(r)
        self.assertEqual(self.approve(a),a)
        self.assertNotEqual(a['dispatcher_source_sha'],a['tool_source_sha'])
        self.assertNotEqual(a['original_source_sha'],a['tool_source_sha'])
    def test_current_main_a_prepare_can_use_same_tool_without_b_image(self):
        r=self.request();r['tool_source_sha']='d'*40;a=self.approval(r);a['tool_source_sha']='d'*40
        self.assertEqual(self.approve(a),a)
    def test_unknown_run_derives_only_two_new_current_fields_and_keeps_original(self):
        r=self.request();raw=tool.canonical(r);a=self.approval(r)
        derived=tool.derive_request(raw,a,'22-3');value=tool.decode(derived)
        expected=copy.deepcopy(r);expected['actual_run_id']='22-3';expected['recovery']['actual_run_id']='22-3'
        self.assertEqual(value,expected);self.assertEqual(tool.decode(raw),r)
        self.assertNotEqual(tool.digest(derived),a['request_template_sha256'])
        self.assertEqual(value['archive_approval'],r['archive_approval']);self.assertEqual(value['recovery']['original_run_id'],'11-1')
    def test_prepare_forbids_effect_fields_including_null(self):
        for name in ('resume','resume_kind','service_control','deployment_control','final_history'):
            with self.subTest(name=name):
                r=self.request();r[name]=None;a=self.approval(r)
                with self.assertRaises(tool.Refused):tool.derive_request(tool.canonical(r),a,'22-3')
    def final_history(self):
        root='/opt/backups/qs-server/compatibility-retirement/12-1/'
        return dict(assets_directory='/opt/qs-server/retirement-assets',runtime_source_sha='a'*40,image_id='sha256:'+'b'*64,container_id='c'*64,runtime_binding_sha256='d'*64,ai_bounds=dict(path=root+'ai.json',sha256='1'*64),peer_bounds=dict(path=root+'peer.json',sha256='2'*64),protection=dict(path=root+'protection.json',sha256='3'*64))
    def test_final_history_has_only_inputs_and_current_run_derivation_preserves_them(self):
        r=self.request();r['final_history']=self.final_history();a=self.approval(r,'apply')
        out=tool.decode(tool.derive_request(tool.canonical(r),a,'22-3'))
        self.assertEqual(out['final_history'],r['final_history']);self.assertEqual(r['actual_run_id'],'')
        for key,value in (('drop_ready',True),('q_complete',True),('run_id','22'),('PeerConnection',{})):
            with self.subTest(key=key):
                v=self.final_history();v[key]=value
                with self.assertRaises(tool.Refused):tool.validate_final_history(v,'12-1')
    def test_final_history_rejects_null_alias_other_operation_and_nonexact_runtime(self):
        with self.assertRaises(tool.Refused):tool.validate_final_history(None,'12-1')
        for mutate in (lambda v:v.update(image_id='qs-ai:latest'),lambda v:v['ai_bounds'].update(path='/tmp/ai.json'),lambda v:v['ai_bounds'].update(path='/opt/backups/qs-server/compatibility-retirement/13-1/ai.json'),lambda v:v.update(protection=v['ai_bounds']),lambda v:v.update(runtime_source_sha='main')):
            v=self.final_history();mutate(v)
            with self.assertRaises(tool.Refused):tool.validate_final_history(v,'12-1')
    def test_writer_control_preserves_expected_hash_only_and_rejects_prepare_even_null(self):
        for value in (None,dict(workflow_scope_sha256='a'*64)):
            r=self.request();r['writer_control']=value;a=self.approval(r)
            with self.assertRaises(tool.Refused):tool.derive_request(tool.canonical(r),a,'22-3')
        r=self.request();a=self.approval(r,'apply')
        out=tool.decode(tool.derive_request(tool.canonical(r),a,'22-3'))
        self.assertEqual(out['writer_control'],r['writer_control'])
        for value in (None,dict(workflow_scope_sha256='main'),dict(workflow_scope_sha256='a'*64,drop_ready=True)):
            r['writer_control']=value;a=self.approval(r,'apply')
            with self.assertRaises(tool.Refused):tool.derive_request(tool.canonical(r),a,'22-3')
    def test_prepare_retains_twelve_credentials_and_effects_use_private_read_token(self):
        self.assertEqual(tool.credential_names('prepare'),tool.CREDENTIALS)
        self.assertEqual(tool.credential_names('apply'),tool.CREDENTIALS+('GITHUB_READ_TOKEN',))
        with self.assertRaises(tool.Refused):tool.credential_names('other')
        ast.parse(tool.ROOT_BOOTSTRAP)
        self.assertIn("names=namespace['credential_names'](stage)",tool.ROOT_BOOTSTRAP)

    def test_mixed_unknown_uppercase_and_proof_fields_reject(self):
        for name in ('drop_ready','whole_writer_fence','SourceSHA','tool_sha','window_lease'):
            r=self.request();r[name]=True;a=self.approval(r)
            with self.subTest(name=name),self.assertRaises(tool.Refused):tool.derive_request(tool.canonical(r),a,'22-3')
    def test_nonempty_future_run_cannot_overwrite_or_rebind(self):
        for where in ('actual_run_id','recovery'):
            r=self.request()
            if where=='recovery':r['recovery']['actual_run_id']='20-1'
            else:r['actual_run_id']='20-1'
            with self.subTest(where=where),self.assertRaises(tool.Refused):tool.derive_request(tool.canonical(r),self.approval(r),'22-3')
    def test_original_identity_schema_and_source_changes_reject(self):
        for key,value in (('source_sha','e'*40),('original_run_id','23-1'),('mysql_head',100),('mongodb_head',39),('mysql_head',True)):
            r=self.request();a=self.approval(r);r['recovery'][key]=value;a['request_template_sha256']=tool.digest(tool.canonical(r))
            with self.subTest(key=key,value=value),self.assertRaises(tool.Refused):tool.derive_request(tool.canonical(r),a,'22-3')
    def test_effects_require_approved_exact_image_and_program(self):
        a=self.approval(self.request(),'apply')
        with self.assertRaises(tool.Refused):self.approve(a)
        a['b_image_id']='sha256:'+'9'*64;a['b_program_sha256']='a'*64
        self.assertEqual(self.approve(a),a)
        for value in ('latest','repo:tag','sha256:invalid'):
            q=copy.deepcopy(a);q['b_image_id']=value
            with self.assertRaises(tool.Refused):self.approve(q)
    def test_resume_changes_only_outer_and_current_run_not_original_recovery_request(self):
        r=self.request();r['recovery']['actual_run_id']='15-1';r['recovery']['archive_sha256']='f'*64
        r['resume_kind']='b_complete';r['resume']={'Recovery':{'Original':copy.deepcopy(r['recovery']),'CurrentRunID':'','JournalSHA256':'1'*64,'WindowStartSHA256':'2'*64},'ApprovedBSourceSHA':'a'*40,'MigrationIntentSHA256':'3'*64,'MigrationResultSHA256':'4'*64}
        a=self.approval(r,'recover');a['b_image_id']='sha256:'+'9'*64;a['b_program_sha256']='a'*64
        out=tool.decode(tool.derive_request(tool.canonical(r),a,'22-3'))
        expected=copy.deepcopy(r);expected['actual_run_id']='22-3';expected['resume']['Recovery']['CurrentRunID']='22-3'
        self.assertEqual(out,expected);self.assertEqual(out['recovery']['actual_run_id'],'15-1')
    def test_duplicate_noncanonical_or_false_dispatcher_cannot_select_another_tool(self):
        a=self.approval(self.request());raw=tool.canonical(a)
        for bad in (raw[:-1].decode()+' ',raw[:-1].decode().replace('{','{"drop_ready":true,',1),raw[:-1].decode().replace('{','{"format_version":1,',1)):
            with self.assertRaises(tool.Refused):tool.approve(bad,tool.digest((bad+'\n').encode()),'d'*40,'prepare','12-1','c'*64,a['request_template_sha256'])
        with self.assertRaises(tool.Refused):tool.approve(raw[:-1].decode(),tool.digest(raw),'e'*40,'prepare','12-1','c'*64,a['request_template_sha256'])
    def test_root_bootstrap_is_fixed_parseable_and_has_no_fence_or_budget_constructor(self):
        ast.parse(tool.ROOT_BOOTSTRAP)
        self.assertIn("source_uid=int(os.environ['SUDO_UID'])",tool.ROOT_BOOTSTRAP)
        self.assertIn("if 'SUDO_UID' in os.environ: raise ValueError()",tool.ROOT_BOOTSTRAP)
        for denied in ('docker','MaintenanceWindow','Permit','fence=true','recovery_complete'):
            self.assertNotIn(denied,tool.ROOT_BOOTSTRAP)

    def native(self, stage="prepare"):
        r=self.request();a=self.approval(r,stage)
        return a,dict(format_version=1,kind='compatibility_retirement_lifecycle_result',operation=stage,source_sha=a['tool_source_sha'],original_source_sha=a['original_source_sha'],operation_id='12-1',run_id='22-3',manifest_sha256=a['manifest_sha256'],request_sha256='e'*64,archive_sha256='f'*64,target_hash=tool.TARGET,target_count=4,complete=True,execution_allowed=False,drop_ready=False,archive_binding_complete=True,recovery_attempted=False,recovery_complete=False,acceptance_complete=False,purge_complete=False,error_category='none',required_adapters=[],isolated_content_restore_complete=True,restore_elapsed_millis=599999)
    def test_native_receipt_rejects_extra_fields_body_boolean_upgrade_and_wrong_derived_hash(self):
        a,n=self.native()
        for key,value in (('credentials',{'password':'private'}),('drop_ready',True),('target_count',True),('request_sha256','d'*64),('restore_elapsed_millis',600001),('original_source_sha','c'*40)):
            q=copy.deepcopy(n);q[key]=value
            with self.subTest(key=key),self.assertRaises(tool.Refused):tool.validate_native(tool.canonical(q),0,a,'22-3','e'*64)
    def test_native_preflight_rejection_has_no_original_facts_and_never_success(self):
        a,n=self.native('apply');n.update(complete=False,archive_binding_complete=False,original_source_sha='',manifest_sha256='',archive_sha256='',isolated_content_restore_complete=False,restore_elapsed_millis=0,error_category='lifecycle_actual_host_adapters_missing')
        self.assertEqual(tool.validate_native(tool.canonical(n),1,a,'22-3','e'*64),n)
        for mutate in ({'complete':True},{'archive_binding_complete':True},{'original_source_sha':'b'*40}):
            q=dict(n,**mutate)
            with self.subTest(mutate=mutate),self.assertRaises(tool.Refused):tool.validate_native(tool.canonical(q),1,a,'22-3','e'*64)
    def test_receipt_normalizes_unrecognized_native_error_without_public_body(self):
        a,n=self.native();n.update(complete=False,error_category='unrecognized_native_detail')
        clean=tool.validate_native(tool.canonical(n),1,a,'22-3','e'*64)
        self.assertEqual(clean['error_category'],'lifecycle_native_operation_failed')
        self.assertNotIn('unrecognized_native_detail',tool.canonical(clean).decode())
    def test_receipt_preserves_actual_budget_and_requires_native_prepare_restore(self):
        a,n=self.native();self.assertEqual(tool.validate_native(tool.canonical(n),0,a,'22-3','e'*64),n)
        for change in ({'isolated_content_restore_complete':False},{'error_category':'other_error'}, {'restore_elapsed_millis':True}):
            with self.subTest(change=change),self.assertRaises(tool.Refused):tool.validate_native(tool.canonical(dict(n,**change)),0,a,'22-3','e'*64)
    def test_nonfinite_json_never_enters_a_template(self):
        with self.assertRaises(tool.Refused):tool.decode('{"test":NaN}')
    def test_owned_file_bounded_exact_mode_and_name_guard(self):
        import os,tempfile
        with tempfile.TemporaryDirectory() as name:
            directory=Path(name).resolve();directory.chmod(0o700);p=directory/'input';p.write_bytes(b'approved');p.chmod(0o600)
            expected=tool.digest(b'approved')
            self.assertEqual(tool.read_owned(p,os.getuid(),expected,8),b'approved')
            with self.assertRaises(tool.Refused):tool.read_owned(p,os.getuid(),expected,7)
            p.chmod(0o500)
            with self.assertRaises(tool.Refused):tool.read_owned(p,os.getuid(),expected,8)
            self.assertEqual(tool.read_owned(p,os.getuid(),expected,8,mode=0o500),b'approved')
            p.unlink();p.symlink_to(directory/'absent')
            with self.assertRaises(OSError):tool.read_owned(p,os.getuid(),expected,8,mode=0o500)

    def test_actual_receipt_transport_roundtrip_has_only_validated_native_fields(self):
        import contextlib,io,shutil,tempfile
        with tempfile.TemporaryDirectory() as name:
            directory=Path(name).resolve();script=Path(__file__).with_name('compatibility-window-tool.py')
            shutil.copyfile(script,directory/script.name)
            shutil.copyfile(Path(__file__).resolve().parents[1]/'dbops/receipt-transport.py',directory/'receipt-transport.py')
            spec=importlib.util.spec_from_file_location('packaged_window_tool',directory/script.name);module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
            a,n=self.native();native=module.validate_native(module.canonical(n),0,a,'22-3','e'*64)
            result=dict(format_version=1,kind='independent_window_tool_call_result',dispatcher_source_sha='d'*40,tool_source_sha='a'*40,approved_template_sha256='c'*64,derived_request_sha256='e'*64,native_result=native)
            output=io.StringIO()
            with contextlib.redirect_stdout(output):module.emit(result,('private-credential-value',))
            spec=importlib.util.spec_from_file_location('packaged_transport',directory/'receipt-transport.py');transport=importlib.util.module_from_spec(spec);spec.loader.exec_module(transport)
            self.assertEqual(json.loads(transport.decode_armored_receipt(output.getvalue())),result)
            self.assertNotIn('private-credential-value',output.getvalue())
            self.assertNotIn('source_sha',output.getvalue())

class WorkflowWindowTool(unittest.TestCase):
    request=WindowToolMetadata.request
    approval=WindowToolMetadata.approval
    # Run the actual checked-in github-script offline with read-only metadata
    # responses. These are selector contracts, never a fence/restore proof.
    def run_script(self, inputs, *, branch='refs/heads/main', main='d'*40, ci='success', selected='d'*40):
        import subprocess
        workflow=(Path(__file__).resolve().parents[2]/'.github/workflows/compatibility-retirement.yml').read_text()
        start=workflow.index('          script: |')+len('          script: |\n')
        end=workflow.index('      - name: Require successful final source CI',start)
        script='\n'.join(line[12:] for line in workflow[start:end].splitlines())
        runner=r"""
const fs=require('fs');const input=JSON.parse(fs.readFileSync(0,'utf8'));
const AsyncFunction=Object.getPrototypeOf(async function(){}).constructor;
const calls=[],outputs={};
const context={payload:{inputs:input.inputs},repo:{owner:'fixed',repo:'fixed'},ref:input.branch,sha:'d'.repeat(40),runId:22};
const github={rest:{repos:{getCommit:async v=>{calls.push(['getCommit',v.ref]);return{data:{sha:v.ref==='main'?input.main:input.selected}}}},actions:{listWorkflowRuns:async v=>{calls.push(['listWorkflowRuns',v.head_sha]);return{data:{workflow_runs:[{id:3,head_sha:v.head_sha,path:'.github/workflows/ci.yml',event:'push',status:input.ci==='running'?'in_progress':'completed',conclusion:input.ci}]}}}}}};
const core={setOutput:(k,v)=>outputs[k]=v};process.env.GITHUB_RUN_ATTEMPT='1';
(async()=>{try{await new AsyncFunction('context','github','core','require',input.script)(context,github,core,require);console.log(JSON.stringify({accepted:true,calls,outputs}));}catch(e){console.log(JSON.stringify({accepted:false,calls,category:e.message}));}})();
"""
        child=subprocess.run(['node','-e',runner],input=json.dumps(dict(inputs=inputs,branch=branch,main=main,ci=ci,selected=selected,script=script)),text=True,capture_output=True,timeout=10,check=True)
        return json.loads(child.stdout)
    def inputs(self,stage='prepare'):
        r=self.request();r['tool_source_sha']='d'*40
        a=self.approval(r,stage);a['tool_source_sha']='d'*40
        if stage!='prepare':a.update(b_image_id='sha256:'+'9'*64,b_program_sha256='a'*64)
        raw=tool.canonical(a)
        return dict(operation=stage,database='mysql-and-mongodb',approved_source_sha='d'*40,operation_id='12-1',manifest_sha256='c'*64,inventory_request_sha256=a['request_template_sha256'],prepare_mode='window-tool',identity_request_sha256='',bootstrap_approval_json=raw[:-1].decode(),bootstrap_approval_sha256=tool.digest(raw))
    def test_workflow_has_same_ten_declared_inputs_and_shared_production_lock(self):
        workflow=(Path(__file__).resolve().parents[2]/'.github/workflows/compatibility-retirement.yml').read_text()
        input_block=workflow.split('    inputs:\n',1)[1].split('\npermissions:',1)[0]
        import re
        self.assertEqual(len(re.findall(r'^      [a-z0-9_]+:',input_block,re.M)),10)
        self.assertIn('group: production-deploy',workflow)
        self.assertNotIn('workflow_dispatch:',workflow.split('  controlled-stage:',1)[1])
    def test_workflow_selects_same_checked_source_and_actual_main(self):
        value=self.run_script(self.inputs())
        self.assertTrue(value['accepted']);self.assertEqual(value['outputs'],{'tool_source_sha':'d'*40})
        self.assertIn(['getCommit','main'],value['calls']);self.assertIn(['listWorkflowRuns','d'*40],value['calls'])
    def test_workflow_only_admits_prepare_and_never_b_effectful_stages(self):
        for stage in sorted(tool.STAGES):
            with self.subTest(stage=stage):
                value=self.run_script(self.inputs(stage))
                self.assertEqual(value['accepted'],stage=='prepare')
                if stage!='prepare':self.assertEqual(value['calls'],[])
    def test_workflow_refuses_old_independent_tool_source_before_metadata_reads(self):
        q=self.inputs();a=tool.decode(q['bootstrap_approval_json']);a['tool_source_sha']='a'*40
        raw=tool.canonical(a);q.update(bootstrap_approval_json=raw[:-1].decode(),bootstrap_approval_sha256=tool.digest(raw))
        value=self.run_script(q)
        self.assertFalse(value['accepted']);self.assertEqual(value['calls'],[])
    def test_workflow_refuses_b_image_or_program_fields_for_prepare(self):
        for field,value in (('b_image_id','sha256:'+'9'*64),('b_program_sha256','a'*64),('b_image_id',None)):
            with self.subTest(field=field,value=value):
                q=self.inputs();a=tool.decode(q['bootstrap_approval_json']);a[field]=value
                raw=tool.canonical(a);q.update(bootstrap_approval_json=raw[:-1].decode(),bootstrap_approval_sha256=tool.digest(raw))
                result=self.run_script(q)
                self.assertFalse(result['accepted']);self.assertEqual(result['calls'],[])
    def test_workflow_current_main_a_prepare_requires_no_b_image(self):
        q=self.inputs();a=tool.decode(q['bootstrap_approval_json']);a['tool_source_sha']='d'*40
        raw=tool.canonical(a);q.update(bootstrap_approval_json=raw[:-1].decode(),bootstrap_approval_sha256=tool.digest(raw))
        self.assertTrue(self.run_script(q,selected='d'*40)['accepted'])
    def test_workflow_ref_main_advance_selected_mismatch_and_failed_ci_refuse(self):
        for values in ({'branch':'refs/heads/branch'},{'main':'e'*40},{'selected':'e'*40},{'ci':'failure'},{'ci':'running'}):
            with self.subTest(values=values):self.assertFalse(self.run_script(self.inputs(),**values)['accepted'])
    def test_workflow_missing_hash_unknown_input_capability_and_duplicate_key_refuse(self):
        q=self.inputs()
        for mutation in ({'inventory_request_sha256':''},{'identity_request_sha256':'a'*64},{'execution_allowed':True}):
            with self.subTest(mutation=mutation):self.assertFalse(self.run_script(dict(q,**mutation))['accepted'])
        a=tool.decode(q['bootstrap_approval_json']);a['drop_ready']=True;raw=tool.canonical(a)
        self.assertFalse(self.run_script(dict(q,bootstrap_approval_json=raw[:-1].decode(),bootstrap_approval_sha256=tool.digest(raw)))['accepted'])
        raw=q['bootstrap_approval_json'].replace('{','{"format_version":1,',1)
        self.assertFalse(self.run_script(dict(q,bootstrap_approval_json=raw,bootstrap_approval_sha256=tool.digest((raw+'\n').encode())))['accepted'])
    def test_workflow_existing_inventory_defaults_and_source_binding_remain(self):
        q=dict(operation='prepare',database='mysql-and-mongodb',approved_source_sha='d'*40,operation_id='12-1',inventory_request_sha256='c'*64)
        value=self.run_script(q)
        self.assertTrue(value['accepted']);self.assertEqual(value['outputs'],{})


class ActualOwnedLocalProcess(unittest.TestCase):
    # Real unprivileged local processes/Unix pipes only. No root capability,
    # sudo, SSH, Docker, native Window or database fixture is mocked here.
    def fixture(self):
        return r'''
import os,signal,subprocess,sys,time
child=subprocess.Popen([sys.executable,'-c','import time;time.sleep(30)'])
def stopped(n,f):
 child.terminate();child.wait();raise SystemExit(1)
signal.signal(signal.SIGTERM,stopped)
print('local-fixture-ready',flush=True)
while True:time.sleep(.01)
'''
    def test_actual_root_native_supervisor_replaces_exec_and_packet_control_remains_open(self):
        script=Path(__file__).with_name('compatibility-window-tool.py').read_text()
        self.assertIn('owner = LinuxChildOwner()',script)
        self.assertIn('control=sys.stdin.fileno(), owner=owner',script)
        self.assertNotIn('os.execve(',script)
        self.assertIn('raw=sys.stdin.buffer.readline(32769)',tool.ROOT_BOOTSTRAP)
        self.assertNotIn('subprocess.run(command, env=environment, input=packet',script)
    def test_real_child_with_grandchild_timeout_has_no_group_left(self):
        import os,subprocess,sys
        holder=[];original=tool.subprocess.Popen
        def save(*args,**kwargs):
            child=original(*args,**kwargs);holder.append(child);return child
        tool.subprocess.Popen=save
        try:
            with self.assertRaises(tool.Refused):tool.owned_process([sys.executable,'-c',self.fixture()],None,timeout=.12)
        finally:tool.subprocess.Popen=original
        self.assertEqual(len(holder),1);self.assertIsNotNone(holder[0].returncode)
        self.assertFalse(tool.group_present(holder[0].pid))
    def test_control_loss_terminates_real_child_and_grandchild(self):
        import os,sys,threading
        read,write=os.pipe();holder=[];original=tool.subprocess.Popen
        def save(*args,**kwargs):
            child=original(*args,**kwargs);holder.append(child);return child
        tool.subprocess.Popen=save
        closer=threading.Timer(.12,lambda:os.close(write));closer.start()
        try:
            with self.assertRaises(tool.Refused):tool.owned_process([sys.executable,'-c',self.fixture()],None,control=read,timeout=2)
        finally:
            closer.join();os.close(read);tool.subprocess.Popen=original
        self.assertIsNotNone(holder[0].returncode);self.assertFalse(tool.group_present(holder[0].pid))
    def test_closed_control_or_regular_file_refuses_before_start(self):
        import os,sys,tempfile
        with tempfile.TemporaryFile() as f:
            with self.assertRaises(tool.Refused):tool.owned_process([sys.executable,'-c','raise SystemExit(0)'],None,control=f.fileno())
        read,write=os.pipe();os.close(write)
        try:
            with self.assertRaises(tool.Refused):tool.owned_process([sys.executable,'-c','raise SystemExit(0)'],None,control=read)
        finally:os.close(read)
    def test_plain_actual_child_known_exit_returns_actual_bytes(self):
        import sys
        code,raw=tool.owned_process([sys.executable,'-c','print("actual-local-terminal")'],None,timeout=2)
        self.assertEqual(code,0);self.assertEqual(raw,b'actual-local-terminal\n')
    def test_manager_keeps_control_writer_open_until_helper_terminal(self):
        import sys
        command=[sys.executable,'-c','import select,sys;line=sys.stdin.buffer.readline();p=select.poll();p.register(0,select.POLLHUP|select.POLLIN);assert not p.poll(10);print("packet-live")']
        code,raw=tool.owned_process(command,None,packet=b'{"private":"input"}\n',timeout=2)
        self.assertEqual(code,0);self.assertEqual(raw,b'packet-live\n')

    def test_catchable_hup_terminates_actual_local_child_and_grandchild(self):
        import os,subprocess,sys
        script=r"""
import importlib.util,os,signal,sys,threading
spec=importlib.util.spec_from_file_location('owned',sys.argv[1]);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
children=[];original=m.subprocess.Popen
def save(*a,**k):
 p=original(*a,**k);children.append(p);return p
m.subprocess.Popen=save
t=threading.Timer(.12,lambda:os.kill(os.getpid(),signal.SIGHUP));t.start()
try:m.owned_process([sys.executable,'-c',sys.argv[2]],None,timeout=2)
except m.Refused:pass
else:raise SystemExit(4)
t.join()
assert len(children)==1 and children[0].returncode is not None and not m.group_present(children[0].pid)
print('actual-hup-local-terminal')
"""
        result=subprocess.run([sys.executable,'-c',script,str(Path(__file__).with_name('compatibility-window-tool.py')),self.fixture()],capture_output=True,timeout=8,check=True)
        self.assertEqual(result.stdout,b'actual-hup-local-terminal\n')

if __name__=='__main__':unittest.main()

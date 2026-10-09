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
        return {'format_version':1,'kind':'compatibility_retirement_lifecycle_request','tool_source_sha':'d'*40,'original_source_sha':'b'*40,
                'operation_id':'12-1','actual_run_id':'','manifest_sha256':'c'*64,'archive_directory':'/approved/archive','window_directory':'/approved/window','journal_directory':'/approved/journal',
                'archive_approval':{'InventorySHA256':'1'*64,'SQLMetadataSHA256':'2'*64,'MongoMetadataSHA256':'3'*64,'OrderedMongoSchemaSHA256':'4'*64,'SourceSHA':'b'*40,'OperationID':'12-1','RunID':'11-1','RequestHash':'5'*64},
                'recovery':{'source_sha':'b'*40,'operation_id':'12-1','original_run_id':'11-1','actual_run_id':'','manifest_sha256':'c'*64,'archive_sha256':'','mysql_non_target_sha256':'','mongodb_non_target_sha256':'6'*64,'mysql_head':99,'mongodb_head':38}}
    def approval(self,r,stage='prepare'):
        return {'format_version':1,'kind':'independent_compatibility_window_tool_approval','dispatcher_source_sha':'d'*40,'tool_source_sha':'d'*40,
                'original_source_sha':'b'*40,'operation_id':'12-1','original_run_id':'11-1','stage':stage,'target_hash':tool.TARGET,'manifest_sha256':'c'*64,
                'request_template_sha256':tool.digest(tool.canonical(r)),'tool_binary_sha256':{'amd64':'7'*64,'arm64':'8'*64},'b_image_id':'','b_program_sha256':''}
    def approve(self,a):
        raw=tool.canonical(a)
        return tool.approve(raw[:-1].decode(),tool.digest(raw),'d'*40,a['stage'],'12-1','c'*64,a['request_template_sha256'])
    def test_a_dispatcher_and_tool_match_but_original_producer_stays_separate(self):
        r=self.request();a=self.approval(r)
        self.assertEqual(self.approve(a),a)
        self.assertEqual(a['dispatcher_source_sha'],a['tool_source_sha'])
        self.assertNotEqual(a['original_source_sha'],a['tool_source_sha'])
        a['tool_source_sha']='a'*40
        with self.assertRaises(tool.Refused):self.approve(a)
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
        for name in ('resume','resume_kind','service_control','deployment_control'):
            with self.subTest(name=name):
                r=self.request();r[name]=None;a=self.approval(r)
                with self.assertRaises(tool.Refused):tool.derive_request(tool.canonical(r),a,'22-3')
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
    def test_a_refuses_all_effects_even_with_exact_b_image_and_program(self):
        for stage in ('apply','verify','recover','purge'):
            a=self.approval(self.request(),stage)
            a['b_image_id']='sha256:'+'9'*64;a['b_program_sha256']='a'*64
            with self.subTest(stage=stage),self.assertRaises(tool.Refused):self.approve(a)
        for key,value in (('b_image_id','sha256:'+'9'*64),('b_program_sha256','a'*64)):
            a=self.approval(self.request());a[key]=value
            with self.subTest(key=key),self.assertRaises(tool.Refused):self.approve(a)
    def test_original_run_and_cross_operation_template_cannot_be_reused(self):
        r=self.request();a=self.approval(r)
        with self.assertRaises(tool.Refused):tool.derive_request(tool.canonical(r),a,'11-1')
        for key,value in (('operation_id','99-1'),('original_source_sha','e'*40),('manifest_sha256','e'*64)):
            q=copy.deepcopy(r);q[key]=value;a['request_template_sha256']=tool.digest(tool.canonical(q))
            with self.subTest(key=key),self.assertRaises(tool.Refused):tool.derive_request(tool.canonical(q),a,'22-3')
    def test_a_resume_is_refused_even_if_nested_original_recovery_matches(self):
        r=self.request();r['recovery']['actual_run_id']='15-1';r['recovery']['archive_sha256']='f'*64
        r['resume_kind']='b_complete';r['resume']={'Recovery':{'Original':copy.deepcopy(r['recovery']),'CurrentRunID':'','JournalSHA256':'1'*64,'WindowStartSHA256':'2'*64},'ApprovedBSourceSHA':'a'*40,'MigrationIntentSHA256':'3'*64,'MigrationResultSHA256':'4'*64}
        a=self.approval(r,'recover');a['b_image_id']='sha256:'+'9'*64;a['b_program_sha256']='a'*64
        with self.assertRaises(tool.Refused):tool.derive_request(tool.canonical(r),a,'22-3')
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
    def test_native_effect_receipt_is_refused_even_if_native_failed_before_inputs(self):
        a,n=self.native('apply');n.update(complete=False,archive_binding_complete=False,original_source_sha='',manifest_sha256='',archive_sha256='',isolated_content_restore_complete=False,restore_elapsed_millis=0,error_category='lifecycle_actual_host_adapters_missing')
        with self.assertRaises(tool.Refused):tool.validate_native(tool.canonical(n),1,a,'22-3','e'*64)
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
            result=dict(format_version=1,kind='independent_window_tool_call_result',dispatcher_source_sha='d'*40,tool_source_sha='d'*40,approved_template_sha256='c'*64,derived_request_sha256='e'*64,native_result=native)
            output=io.StringIO()
            with contextlib.redirect_stdout(output):module.emit(result,('private-credential-value',))
            spec=importlib.util.spec_from_file_location('packaged_transport',directory/'receipt-transport.py');transport=importlib.util.module_from_spec(spec);spec.loader.exec_module(transport)
            self.assertEqual(json.loads(transport.decode_armored_receipt(output.getvalue())),result)
            self.assertNotIn('private-credential-value',output.getvalue())
            self.assertNotIn('source_sha',output.getvalue())

class RootTemplateStaging(unittest.TestCase):
    request=WindowToolMetadata.request
    approval=WindowToolMetadata.approval
    def test_exact_per_actual_run_intents_derive_only_current_run_and_preserve_originals(self):
        import io,os,subprocess,tarfile,tempfile
        from unittest import mock
        original_uid=os.getuid()
        with tempfile.TemporaryDirectory() as name:
            local=Path(name).resolve();local.chmod(0o700)
            prefix='/opt/backups/qs-server/'
            def mapped(value):
                value=str(value)
                return Path(local/value[len(prefix):]) if value.startswith(prefix) else Path(value)
            original=mapped(prefix+'compatibility-retirement/12-1');original.mkdir(parents=True,mode=0o700)
            original.parent.chmod(0o700)
            r=self.request();raw=tool.canonical(r);manifest=b'{"frozen":"manifest"}\n';r['manifest_sha256']=tool.digest(manifest);r['recovery']['manifest_sha256']=tool.digest(manifest)
            raw=tool.canonical(r);a=self.approval(r);a['manifest_sha256']=tool.digest(manifest)
            binary=b'private-offline-native-placeholder';a['tool_binary_sha256']={arch:tool.digest(binary) for arch in ('amd64','arm64')}
            (original/'lifecycle-request-template.json').write_bytes(raw);(original/'lifecycle-request-template.json').chmod(0o600)
            (original/'manifest.json').write_bytes(manifest);(original/'manifest.json').chmod(0o600)
            producer_files={f'source-{i}':str(i).encode() for i in range(7)}
            for asset,body in producer_files.items():
                (original/asset).write_bytes(body);(original/asset).chmod(0o600)
            before={p.name:p.read_bytes() for p in original.iterdir()}
            archive=io.BytesIO()
            with tarfile.open(fileobj=archive,mode='w:gz') as tar:
                for member in ('compatibility-window-tool.py','receipt-transport.py','inventory-linux-amd64','inventory-linux-arm64'):
                    body=binary if member.startswith('inventory-') else b'packaged source'
                    info=tarfile.TarInfo(member);info.size=len(body);tar.addfile(info,io.BytesIO(body))
            archive=archive.getvalue();approval=tool.canonical(a)
            packet={'approval':approval[:-1].decode(),'credentials':{k:'' for k in tool.CREDENTIALS},'tool_directory':'/tmp/qs-independent-window-tool.ABCDEF','tool_program_sha256':'e'*64}
            arguments=['prepare','12-1','22-3','d'*40,tool.digest(approval),tool.digest(archive),tool.digest(manifest),tool.digest(raw)]
            native_calls=[]
            def protected(path,*,create=False):
                if create:path.mkdir(mode=0o700)
                self.assertTrue(path.is_dir());self.assertEqual(path.stat().st_mode & 0o777,0o700)
            def execve(native,argv,env):
                invocation=mapped(prefix+'compatibility-retirement-invocations/12-1-'+arguments[2])
                intent=tool.decode((invocation/'native-call.intent.private.json').read_bytes())
                derived=(invocation/'lifecycle-request.json').read_bytes()
                self.assertEqual(intent['derived_request_sha256'],tool.digest(derived))
                self.assertEqual(intent['approved_template_sha256'],tool.digest(raw))
                self.assertEqual(intent['original_run_id'],'11-1');self.assertEqual(intent['source_uid'],original_uid)
                self.assertEqual(intent['drop_authority'],False)
                self.assertEqual(argv[2],'lifecycle-prepare-root-once')
                self.assertEqual(argv[4],str(invocation/'lifecycle-request.json'))
                self.assertEqual(argv[6],tool.digest(derived))
                self.assertEqual(env['QS_RETIREMENT_SOURCE_UID'],str(original_uid))
                value=tool.decode(derived);expected=copy.deepcopy(r);expected['actual_run_id']=arguments[2];expected['recovery']['actual_run_id']=arguments[2]
                self.assertEqual(value,expected);native_calls.append((arguments[2],intent,derived))
                raise ChildProcessError('mocked native boundary')
            # This test owns real temp files only; privilege and native owner
            # are explicitly mocked, and never provide Linux/root proof.
            owner=object();check=[(0,b'd'*40+b'\n')]
            def run_owned(command,environment,**options):
                self.assertIs(options['owner'],owner)
                self.assertEqual(options['control'],tool.sys.stdin.fileno())
                if command[1]=='--source-sha':return check[0]
                return execve(command[0],command,environment)
            with mock.patch.object(tool,'Path',side_effect=mapped),mock.patch.object(tool.os,'getuid',return_value=0),mock.patch.object(tool.os,'geteuid',return_value=0),mock.patch.object(tool.platform,'system',return_value='Linux'),mock.patch.object(tool.platform,'machine',return_value='aarch64'),mock.patch.object(tool,'protected_directory',side_effect=protected),mock.patch.object(tool,'LinuxChildOwner',return_value=owner),mock.patch.object(tool,'live_control'),mock.patch.object(tool,'owned_process',side_effect=run_owned):
                arguments[2]='21-1';check[0]=(0,b'e'*40+b'\n')
                with self.assertRaises(tool.Refused):tool.root_execute(arguments,packet,original_uid,archive)
                self.assertEqual(native_calls,[])
                rejected=mapped(prefix+'compatibility-retirement-invocations/12-1-21-1')
                self.assertTrue((rejected/'native-call.intent.private.json').is_file())
                self.assertFalse((rejected/'lifecycle-request.json').exists())
                arguments[2]='22-3';check[0]=(0,b'd'*40+b'\n')
                with self.assertRaises(ChildProcessError):tool.root_execute(arguments,packet,original_uid,archive)
                first=native_calls[0][2]
                with self.assertRaises(FileExistsError):tool.root_execute(arguments,packet,original_uid,archive)
                self.assertEqual(len(native_calls),1)
                arguments[2]='23-1'
                with self.assertRaises(ChildProcessError):tool.root_execute(arguments,packet,original_uid,archive)
                self.assertEqual(len(native_calls),2)
                self.assertEqual((mapped(prefix+'compatibility-retirement-invocations/12-1-22-3')/'lifecycle-request.json').read_bytes(),first)
            self.assertEqual(before,{p.name:p.read_bytes() for p in original.iterdir()})
    def test_real_exclusive_write_never_overwrites_an_existing_receipt(self):
        import tempfile
        with tempfile.TemporaryDirectory() as name:
            path=Path(name)/'request'
            tool.write_new(path,b'first')
            with self.assertRaises(FileExistsError):tool.write_new(path,b'second')
            self.assertEqual(path.read_bytes(),b'first');self.assertEqual(path.stat().st_mode & 0o777,0o600)

class ActualACLI(unittest.TestCase):
    """Compile this source and exercise its public gates; the test SHA is metadata.

    No root, DB, Docker or production provenance is inferred from this fixture.
    """
    @classmethod
    def setUpClass(cls):
        import os,subprocess,tempfile
        cls.directory=tempfile.TemporaryDirectory()
        cls.addClassCleanup(cls.directory.cleanup)
        cls.native=Path(cls.directory.name)/'retirement-test-cli'
        cls.environment=dict(os.environ)
        cls.environment.pop('QS_RETIREMENT_SOURCE_UID',None)
        cls.environment.update(MYSQL_HOST='invalid-no-connection',MONGODB_HOST='invalid-no-connection',GOWORK='off')
        subprocess.run(['go','build','-buildvcs=false','-trimpath','-ldflags','-X main.sourceSHA='+('d'*40),'-o',str(cls.native),'./cmd/qs-compatibility-retirement'],cwd=Path(__file__).resolve().parents[2],env=cls.environment,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=180,check=True)
    def test_actual_current_a_cli_source_sha_flag_reports_its_compiled_test_binding(self):
        import subprocess
        result=subprocess.run([str(self.native),'--source-sha'],env=self.environment,capture_output=True,timeout=5,check=True)
        self.assertEqual(result.stdout,b'd'*40+b'\n');self.assertEqual(result.stderr,b'')
    def test_actual_a_effectful_modes_refuse_before_request_or_database_access(self):
        import subprocess
        directory=Path(self.directory.name)
        for stage in ('apply','verify','recover','purge'):
            with self.subTest(stage=stage):
                result=subprocess.run([str(self.native),'--mode','lifecycle-'+stage,'--request',str(directory/'nonexistent'),'--request-hash','a'*64,'--operation-id','12-1','--run-id','22-3'],env=self.environment,capture_output=True,timeout=5,check=False)
                self.assertEqual(result.returncode,1)
                receipt=tool.decode(result.stdout)
                self.assertEqual(receipt['operation'],stage);self.assertEqual(receipt['error_category'],'lifecycle_actual_host_adapters_missing')
                for flag in ('complete','execution_allowed','drop_ready','archive_binding_complete','recovery_attempted','recovery_complete','acceptance_complete','purge_complete','isolated_content_restore_complete'):
                    self.assertEqual(receipt[flag],False)
                self.assertEqual(receipt['original_source_sha'],'');self.assertEqual(receipt['manifest_sha256'],'')
                self.assertFalse((directory/'nonexistent').exists())

class WorkflowWindowTool(unittest.TestCase):
    request=WindowToolMetadata.request
    approval=WindowToolMetadata.approval
    # Run the actual checked-in github-script offline with read-only metadata
    # responses. These are selector contracts, never a fence/restore proof.
    def run_script(self, inputs, *, branch='refs/heads/main', main='d'*40, ci='success', selected='a'*40):
        import subprocess
        workflow=(Path(__file__).resolve().parents[2]/'.github/workflows/compatibility-retirement.yml').read_text()
        start=workflow.index('          script: |')+len('          script: |\n')
        end=workflow.index('      - name: Require successful final source CI',start)
        script='\n'.join(line[12:] for line in workflow[start:end].splitlines())
        ci_start=workflow.index('          script: |\n',end)+len('          script: |\n')
        ci_end=workflow.index('  controlled-stage:',ci_start)
        ci_script='\n'.join(line[12:] for line in workflow[ci_start:ci_end].splitlines() if line)
        runner=r"""
const fs=require('fs');const input=JSON.parse(fs.readFileSync(0,'utf8'));
const AsyncFunction=Object.getPrototypeOf(async function(){}).constructor;
const calls=[],outputs={};
const context={payload:{inputs:input.inputs},repo:{owner:'fixed',repo:'fixed'},ref:input.branch,sha:'d'.repeat(40),runId:22};
const github={rest:{repos:{getCommit:async v=>{calls.push(['getCommit',v.ref]);return{data:{sha:v.ref==='main'?input.main:input.selected}}}},actions:{listWorkflowRuns:async v=>{calls.push(['listWorkflowRuns',v.head_sha]);return{data:{workflow_runs:[{id:3,head_sha:v.head_sha,path:'.github/workflows/ci.yml',event:'push',head_branch:'main',status:input.ci==='running'?'in_progress':'completed',conclusion:input.ci}]}}}}}};
const core={setOutput:(k,v)=>outputs[k]=v};process.env.GITHUB_RUN_ATTEMPT='1';
(async()=>{try{for(const script of input.scripts){await new AsyncFunction('context','github','core','require',script)(context,github,core,require);}console.log(JSON.stringify({accepted:true,calls,outputs}));}catch(e){console.log(JSON.stringify({accepted:false,calls,category:e.message}));}})();
"""
        child=subprocess.run(['node','-e',runner],input=json.dumps(dict(inputs=inputs,branch=branch,main=main,ci=ci,selected=selected,scripts=[script,ci_script])),text=True,capture_output=True,timeout=10,check=True)
        return json.loads(child.stdout)
    def inputs(self,stage='prepare'):
        a=self.approval(self.request(),stage)
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
    def test_workflow_uses_current_main_tool_and_binds_actual_main_ci(self):
        value=self.run_script(self.inputs())
        self.assertTrue(value['accepted']);self.assertEqual(value['outputs'],{})
        self.assertIn(['getCommit','main'],value['calls']);self.assertIn(['listWorkflowRuns','d'*40],value['calls'])
        self.assertNotIn(['getCommit','a'*40],value['calls'])
    def test_workflow_refuses_all_effectful_stages_before_ci_or_source_selection(self):
        for stage in ('apply','verify','recover','purge'):
            with self.subTest(stage=stage):
                value=self.run_script(self.inputs(stage));self.assertFalse(value['accepted']);self.assertEqual(value['calls'],[])
    def test_workflow_refuses_foreign_tool_and_every_b_field_in_a_prepare(self):
        for key,value in (('tool_source_sha','a'*40),('b_image_id','sha256:'+'9'*64),('b_program_sha256','a'*64),('original_run_id','22-1')):
            q=self.inputs();a=tool.decode(q['bootstrap_approval_json']);a[key]=value
            raw=tool.canonical(a);q.update(bootstrap_approval_json=raw[:-1].decode(),bootstrap_approval_sha256=tool.digest(raw))
            with self.subTest(key=key):self.assertFalse(self.run_script(q)['accepted'])
    def test_workflow_ref_main_advance_and_failed_ci_refuse(self):
        for values in ({'branch':'refs/heads/branch'},{'main':'e'*40},{'ci':'failure'},{'ci':'running'}):
            with self.subTest(values=values):self.assertFalse(self.run_script(self.inputs(),**values)['accepted'])
    def test_workflow_missing_hash_unknown_input_capability_and_duplicate_key_refuse(self):
        q=self.inputs()
        for mutation in ({'inventory_request_sha256':''},{'identity_request_sha256':'a'*64},{'execution_allowed':True}):
            with self.subTest(mutation=mutation):self.assertFalse(self.run_script(dict(q,**mutation))['accepted'])
        a=tool.decode(q['bootstrap_approval_json']);a['drop_ready']=True;raw=tool.canonical(a)
        self.assertFalse(self.run_script(dict(q,bootstrap_approval_json=raw[:-1].decode(),bootstrap_approval_sha256=tool.digest(raw)))['accepted'])
        raw=q['bootstrap_approval_json'].replace('{','{"format_version":1,',1)
        self.assertFalse(self.run_script(dict(q,bootstrap_approval_json=raw,bootstrap_approval_sha256=tool.digest((raw+'\n').encode())))['accepted'])
    def test_existing_prepare_facts_ten_input_descriptor_stays_readonly(self):
        descriptor=dict(format_version=1,kind='readonly_prepare_facts_observation_descriptor',prepare_mode='prepare-facts',source_sha='d'*40,operation_id='12-1',target_hash=tool.TARGET,database_scope='mysql-and-mongodb',inventory_report=dict(operation_id='12-1',run_id='11-1',source_sha='b'*40,sha256='1'*64,request_sha256='2'*64),restore_engines=dict(mysql_image_id='sha256:'+'3'*64,mongodb_image_id='sha256:'+'4'*64,architecture='amd64'),archive_directory='/opt/backups/qs-server/compatibility-retirement/12-1/archive')
        raw=tool.canonical(descriptor)
        q=dict(operation='prepare',database='mysql-and-mongodb',approved_source_sha='d'*40,operation_id='12-1',manifest_sha256='',inventory_request_sha256='',identity_request_sha256='',prepare_mode='prepare-facts',bootstrap_approval_json=raw[:-1].decode(),bootstrap_approval_sha256=tool.digest(raw))
        self.assertTrue(self.run_script(q)['accepted'])
        for mutation in (dict(operation='apply'),dict(manifest_sha256='a'*64),dict(inventory_request_sha256='a'*64)):
            with self.subTest(mutation=mutation):self.assertFalse(self.run_script(dict(q,**mutation))['accepted'])
    def test_workflow_keeps_exact_package_current_checkout_and_pinned_ssh_prepare(self):
        workflow=(Path(__file__).resolve().parents[2]/'.github/workflows/compatibility-retirement.yml').read_text()
        checkout=workflow.split('      - name: Checkout immutable approved source\n',1)[1].split('      - name:',1)[0]
        self.assertIn('ref: ${{ github.sha }}',checkout)
        package=workflow.split('          if [[ "$RETIREMENT_PACKAGE_MODE" == "window-tool" ]]; then\n',1)[1].split('          elif ',1)[0]
        self.assertIn('[[ "$(git rev-parse HEAD)" == "$RETIREMENT_TOOL_SOURCE_SHA" ]]',package)
        self.assertIn('compatibility-window-tool.py receipt-transport.py inventory-linux-amd64 inventory-linux-arm64',package)
        self.assertIn('-buildvcs=false -trimpath',package)
        ssh=workflow.split('      - name: Invoke approved independent window tool on the still-A database\n',1)[1]
        self.assertIn("if: inputs.prepare_mode == 'window-tool' && inputs.operation == 'prepare'",ssh)
        self.assertIn('fingerprint: ${{ vars.SVRA_SSH_FINGERPRINT }}',ssh)
        self.assertIn('[[ "$RETIREMENT_OPERATION" == "prepare" ]]',ssh)
        for denied in ('docker pull','docker run','workflow_dispatch','lifecycle-apply','lifecycle-recover'):
            self.assertNotIn(denied,ssh)
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

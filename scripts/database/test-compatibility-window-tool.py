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
                'request_template_sha256':tool.digest(tool.canonical(r)),'tool_binary_sha256':{'amd64':'7'*64,'arm64':'8'*64},'b_image_id':'sha256:'+'9'*64,'b_program_sha256':'a'*64}
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
    def test_current_main_prepare_same_tool_requires_actual_b_image(self):
        r=self.request();r['tool_source_sha']='d'*40;a=self.approval(r);a['tool_source_sha']='d'*40
        self.assertEqual(self.approve(a),a)
        a['b_image_id']='';a['b_program_sha256']=''
        with self.assertRaises(tool.Refused):self.approve(a)
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
    def test_original_run_and_cross_operation_template_cannot_be_reused(self):
        r=self.request();a=self.approval(r)
        with self.assertRaises(tool.Refused):tool.derive_request(tool.canonical(r),a,'11-1')
        for key,value in (('operation_id','99-1'),('original_source_sha','e'*40),('manifest_sha256','e'*64)):
            q=copy.deepcopy(r);q[key]=value;a['request_template_sha256']=tool.digest(tool.canonical(q))
            with self.subTest(key=key),self.assertRaises(tool.Refused):tool.derive_request(tool.canonical(q),a,'22-3')
    def final_history(self):
        root='/opt/backups/qs-server/compatibility-retirement/12-1/'
        return dict(assets_directory='/opt/qs-server/retirement-assets',runtime_source_sha='a'*40,image_id='sha256:'+'b'*64,container_id='c'*64,runtime_binding_sha256='d'*64,ai_bounds=dict(path=root+'ai.json',sha256='1'*64),peer_bounds=dict(path=root+'peer.json',sha256='2'*64),protection=dict(path=root+'protection.json',sha256='3'*64),stop_constraints=dict(settings_sha256='4'*64,network_id='5'*64))
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
        for mutate in (lambda v:v.update(stop_constraints=None),lambda v:v['stop_constraints'].update(network_id='infra-network'),lambda v:v['stop_constraints'].update(settings_sha256=''),lambda v:v['stop_constraints'].update(drop_ready=True),lambda v:v.update(image_id='qs-ai:latest'),lambda v:v['ai_bounds'].update(path='/tmp/ai.json'),lambda v:v['ai_bounds'].update(path='/opt/backups/qs-server/compatibility-retirement/13-1/ai.json'),lambda v:v.update(protection=v['ai_bounds']),lambda v:v.update(runtime_source_sha='main')):
            v=self.final_history();mutate(v)
            with self.assertRaises(tool.Refused):tool.validate_final_history(v,'12-1')
    def test_writer_control_preserves_expected_hash_only_and_rejects_prepare_even_null(self):
        for value in (None,dict(workflow_scope_sha256='a'*64)):
            r=self.request();r['writer_control']=value;a=self.approval(r)
            with self.assertRaises(tool.Refused):tool.derive_request(tool.canonical(r),a,'22-3')
    def test_source_copy_intent_is_only_approved_original_path_and_hash(self):
        value = dict(path='/opt/backups/qs-server/compatibility-retirement-root-prepare/12-1-20-1/source-copy.intent.private.json',sha256='7'*64)
        r=self.request();r['source_copy_intent']=value;a=self.approval(r,'apply')
        out=tool.decode(tool.derive_request(tool.canonical(r),a,'22-3'))
        self.assertEqual(out['source_copy_intent'],value)
        r=self.request();r['source_copy_intent']=value;a=self.approval(r,'prepare')
        with self.assertRaises(tool.Refused):tool.derive_request(tool.canonical(r),a,'22-3')
        for changed in (None,dict(value,complete=True),dict(value,sha256='main'),dict(value,path='/tmp/source-copy.intent.private.json'),dict(value,path=value['path'].replace('12-1-20-1','13-1-20-1')),dict(value,path=value['path'].replace('20-1','22-3')),dict(value,path=value['path'].replace('20-1','11-1')),dict(value,path=value['path'].replace('source-copy','other'))):
            r=self.request();r['source_copy_intent']=changed;a=self.approval(r,'apply')
            with self.subTest(value=changed),self.assertRaises(tool.Refused):tool.derive_request(tool.canonical(r),a,'22-3')

    def test_prepare_source_copy_receipt_keeps_only_digest_and_rejects_other_stages(self):
        a,n=self.native('prepare');n['source_copy_intent_sha256']='7'*64
        self.assertEqual(tool.validate_native(tool.canonical(n),0,a,'22-3','e'*64)['source_copy_intent_sha256'],'7'*64)
        for stage,value in (('prepare','main'),('apply','7'*64),('prepare',None)):
            a,n=self.native(stage);n['source_copy_intent_sha256']=value
            with self.subTest(stage=stage,value=value),self.assertRaises(tool.Refused):tool.validate_native(tool.canonical(n),0,a,'22-3','e'*64)

    def test_preparation_restore_zero_is_only_same_approved_prepare_root(self):
        intent=dict(path='/opt/backups/qs-server/compatibility-retirement-root-prepare/12-1-20-1/source-copy.intent.private.json',sha256='7'*64)
        value=dict(path=str(Path(intent['path']).parent/'lifecycle-restore-20-1.zero.private.json'),sha256='8'*64)
        r=self.request();r['source_copy_intent']=intent;r['preparation_restore_zero']=value;a=self.approval(r,'apply')
        self.assertEqual(tool.decode(tool.derive_request(tool.canonical(r),a,'22-3'))['preparation_restore_zero'],value)
        for stage,bad in (('prepare',value),('apply',None),('apply',dict(value,complete=True)),('apply',dict(value,sha256='main')),('apply',dict(value,path=value['path'].replace('20-1.zero','21-1.zero')))):
            r=self.request();r['source_copy_intent']=intent;r['preparation_restore_zero']=bad;a=self.approval(r,stage)
            with self.subTest(stage=stage,bad=bad),self.assertRaises(tool.Refused):tool.derive_request(tool.canonical(r),a,'22-3')
        r=self.request();r['preparation_restore_zero']=value;a=self.approval(r,'apply')
        with self.assertRaises(tool.Refused):tool.derive_request(tool.canonical(r),a,'22-3')

    def test_prepare_restore_zero_transport_is_digest_only(self):
        a,n=self.native('prepare');n['preparation_restore_zero_sha256']='7'*64
        self.assertEqual(tool.validate_native(tool.canonical(n),0,a,'22-3','e'*64)['preparation_restore_zero_sha256'],'7'*64)
        for stage,value in (('prepare','main'),('apply','7'*64),('prepare',None)):
            a,n=self.native(stage);n['preparation_restore_zero_sha256']=value
            with self.subTest(stage=stage,value=value),self.assertRaises(tool.Refused):tool.validate_native(tool.canonical(n),0,a,'22-3','e'*64)

    def test_historical_material_report_preserves_exact_original_reference_only(self):
        value=dict(path='/opt/backups/qs-server/compatibility-retirement/12-1/history-write-20-1/history.write.json',sha256='7'*64)
        r=self.request();r['historical_write_report']=value;a=self.approval(r,'apply')
        self.assertEqual(tool.decode(tool.derive_request(tool.canonical(r),a,'22-3'))['historical_write_report'],value)
        r=self.request();r['historical_write_report']=value;a=self.approval(r,'prepare')
        with self.assertRaises(tool.Refused):tool.derive_request(tool.canonical(r),a,'22-3')
        for bad in (None,dict(value,complete=True),dict(value,sha256='main'),dict(value,path='/tmp/history.write.json'),dict(value,path=value['path'].replace('/12-1/','/13-1/')),dict(value,path=value['path'].replace('20-1','22-3'))):
            r=self.request();r['historical_write_report']=bad;a=self.approval(r,'apply')
            with self.subTest(value=bad),self.assertRaises(tool.Refused):tool.derive_request(tool.canonical(r),a,'22-3')
        r=self.request();a=self.approval(r,'apply')
        out=tool.decode(tool.derive_request(tool.canonical(r),a,'22-3'))
        self.assertEqual(out['writer_control'],r['writer_control'])
        for value in (None,dict(workflow_scope_sha256='main'),dict(workflow_scope_sha256='a'*64,drop_ready=True)):
            r['writer_control']=value;a=self.approval(r,'apply')
            with self.assertRaises(tool.Refused):tool.derive_request(tool.canonical(r),a,'22-3')
    def test_prepare_retains_twelve_credentials_and_effects_use_private_read_token(self):
        self.assertEqual(tool.credential_names('prepare'),tool.CREDENTIALS)
        self.assertEqual(tool.credential_names('apply'),tool.CREDENTIALS+('GITHUB_READ_TOKEN',)+tool.SERVICE_CREDENTIALS)
        with self.assertRaises(tool.Refused):tool.credential_names('other')
        ast.parse(tool.ROOT_BOOTSTRAP)
        self.assertIn("names=namespace['credential_names'](stage)",tool.ROOT_BOOTSTRAP)

    def test_database_writer_expected_bytes_are_bound_without_importing_isolation(self):
        r=self.request();a=self.approval(r,'apply')
        file=dict(path='/opt/backups/qs-server/compatibility-retirement/12-1/database-writers.private.json',sha256='a'*64)
        r['writer_control']['database_input']=file
        a['request_template_sha256']=tool.digest(tool.canonical(r))
        out=tool.decode(tool.derive_request(tool.canonical(r),a,'22-3'))
        self.assertEqual(out['writer_control']['database_input'],file)
        for mutate in (lambda v:v.update(path='/tmp/writer.json'),lambda v:v.update(path='/opt/backups/qs-server/compatibility-retirement/13-1/writer.json'),lambda v:v.update(complete=True),lambda v:v.update(sha256='main')):
            bad=copy.deepcopy(r);mutate(bad['writer_control']['database_input']);a['request_template_sha256']=tool.digest(tool.canonical(bad))
            with self.assertRaises(tool.Refused):tool.derive_request(tool.canonical(bad),a,'22-3')
    def test_service_credentials_are_private_and_only_key_allows_newlines(self):
        credentials={k:'' for k in tool.credential_names('apply')}
        credentials[tool.SERVICE_KEY]='-----BEGIN OPENSSH PRIVATE KEY-----\nprivate\n-----END OPENSSH PRIVATE KEY-----\n'
        tool.validate_credentials(credentials,'apply')
        for name in ('MYSQL_HOST','GITHUB_READ_TOKEN','RETIREMENT_SERVICE_SSH_HOST'):
            v=dict(credentials);v[name]='x\ny'
            with self.subTest(name=name),self.assertRaises(tool.Refused):tool.validate_credentials(v,'apply')
        with self.assertRaises(tool.Refused):tool.validate_credentials(credentials,'prepare')

    def test_preparation_inventory_hash_never_becomes_an_effectful_approval(self):
        a=self.approval(self.request());a['local_descriptor_sha256']='e'*64
        self.assertEqual(self.approve(a),a)
        a['local_descriptor_sha256']='main'
        with self.assertRaises(tool.Refused):self.approve(a)
        a=self.approval(self.request(),'apply');a.update(local_descriptor_sha256='e'*64,b_image_id='sha256:'+'9'*64,b_program_sha256='a'*64)
        with self.assertRaises(tool.Refused):self.approve(a)

    def test_service_known_host_uses_observed_key_with_exact_existing_pin(self):
        import base64,hashlib
        blob=b'offline public-key parser fixture only'
        fingerprint='SHA256:'+base64.b64encode(hashlib.sha256(blob).digest()).decode().rstrip('=')
        raw=b'[server-d.example]:2222 ssh-ed25519 '+base64.b64encode(blob)+b'\n'
        self.assertEqual(tool.pinned_service_known_host(raw,'server-d.example',2222,fingerprint),raw)
        for changed in (raw.replace(b'server-d.example',b'server-a.example'),raw.replace(b'2222',b'22'),raw.replace(b'ssh-ed25519',b'@cert-authority'),raw.replace(base64.b64encode(blob),b'invalid')):
            with self.subTest(changed=changed),self.assertRaises(tool.Refused):tool.pinned_service_known_host(changed,'server-d.example',2222,fingerprint)
        with self.assertRaises(tool.Refused):tool.pinned_service_known_host(raw,'server-d.example',2222,'SHA256:'+'a'*43)

    def test_database_writer_native_census_reference_is_exact_and_not_authority(self):
        r=self.request();a=self.approval(r,'apply')
        file=dict(path='/opt/backups/qs-server/compatibility-retirement-root-prepare/12-1-18-1/db-writer-census.private.json',sha256='a'*64)
        r['writer_control']['database_input']=file
        a['request_template_sha256']=tool.digest(tool.canonical(r))
        self.assertEqual(tool.decode(tool.derive_request(tool.canonical(r),a,'22-3'))['writer_control']['database_input'],file)
        for path in (file['path'].replace('12-1','13-1',1),file['path'].replace('18-1','future',1),file['path'].replace('db-writer-census.private.json','other.json'),file['path']+'/../db-writer-census.private.json'):
            bad=copy.deepcopy(r);bad['writer_control']['database_input']['path']=path;a['request_template_sha256']=tool.digest(tool.canonical(bad))
            with self.subTest(path=path),self.assertRaises(tool.Refused):tool.derive_request(tool.canonical(bad),a,'22-3')

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
        a=self.approval(self.request(),'apply');a['b_image_id']='';a['b_program_sha256']=''
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
    def test_workflow_refuses_missing_or_unbound_b_image_pair_for_prepare(self):
        for field,value in (('b_image_id',''),('b_program_sha256',''),('b_image_id',None),('b_image_id','latest')):
            with self.subTest(field=field,value=value):
                q=self.inputs();a=tool.decode(q['bootstrap_approval_json']);a[field]=value
                raw=tool.canonical(a);q.update(bootstrap_approval_json=raw[:-1].decode(),bootstrap_approval_sha256=tool.digest(raw))
                result=self.run_script(q)
                self.assertFalse(result['accepted']);self.assertEqual(result['calls'],[])
    def test_workflow_same_main_prepare_requires_actual_image_even_same_original_source(self):
        q=self.inputs();a=tool.decode(q['bootstrap_approval_json']);a['tool_source_sha']='d'*40;a['original_source_sha']='d'*40
        raw=tool.canonical(a);q.update(bootstrap_approval_json=raw[:-1].decode(),bootstrap_approval_sha256=tool.digest(raw))
        self.assertTrue(self.run_script(q,selected='d'*40)['accepted'])
        a['b_image_id']='';a['b_program_sha256']=''
        raw=tool.canonical(a);q.update(bootstrap_approval_json=raw[:-1].decode(),bootstrap_approval_sha256=tool.digest(raw))
        self.assertFalse(self.run_script(q,selected='d'*40)['accepted'])
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

class ImagePreloadBoundary(unittest.TestCase):
    def test_closed_observation_keeps_actual_source_and_no_runtime_authority(self):
        r=WindowToolMetadata().request();a=WindowToolMetadata().approval(r)
        value=dict(kind="native_cached_api_image_observation",tool_source_sha=a["tool_source_sha"],original_source_sha=a["original_source_sha"],operation_id=a["operation_id"],actual_run_id="22-3",image_archive_sha256="1"*64,image_id="sha256:"+"2"*64,os="linux",architecture="amd64",revision=a["tool_source_sha"],program_sha256="3"*64,probe_id="4"*64,probe_absent=True,temporary_files_zero=True,capabilities=dict(deployment=False,writer_fence=False,drop=False))
        self.assertEqual(tool.validate_image_preload(value,a,"22-3"),value)
        for key,changed in (("image_id","qs-apiserver:latest"),("architecture","arm64"),("revision","b"*40),("tool_source_sha","b"*40),("actual_run_id","11-1"),("probe_absent",False),("temporary_files_zero",False),("capabilities",dict(deployment=True,writer_fence=False,drop=False)),("extra",True)):
            bad=copy.deepcopy(value);bad[key]=changed
            with self.subTest(key=key),self.assertRaises(tool.Refused):tool.validate_image_preload(bad,a,"22-3")
    def test_image_preparation_is_only_existing_prepare_and_never_start_or_deploy(self):
        import inspect
        source=inspect.getsource(tool.preload_api_image)
        self.assertIn('approval["stage"] != "prepare"',source)
        self.assertIn('"--network","none","--read-only"',source)
        self.assertIn('command("cp",cid+":/app/qs-apiserver"',source)
        self.assertIn('second != first',source)
        self.assertNotIn('command("start"',source)
        self.assertNotIn('remote-deploy',source)
        self.assertNotIn('prune',source)
        workflow=Path(__file__).resolve().parents[2]/'.github/workflows/compatibility-retirement.yml'
        body=workflow.read_text()
        self.assertIn("if: inputs.prepare_mode == 'prepare-facts' && inputs.operation == 'prepare'",body)
        self.assertIn('bash scripts/cd/build-image.sh',body)
        self.assertIn('preload-image.tar.gz',body)

class RootTemplateStaging(unittest.TestCase):
    request=WindowToolMetadata.request
    approval=WindowToolMetadata.approval
    def test_preparation_rejects_missing_runtime_source_before_native_side_effect(self):
        from unittest import mock
        a=self.approval(self.request())
        basis={k:a[k] for k in ('tool_source_sha','operation_id','original_run_id','manifest_sha256')}
        basis.update(source_sha=a['original_source_sha'],host_role='server-a')
        for runtime in (None,'','A'*40,'f'*39):
            descriptor=dict(basis)
            if runtime is not None:descriptor['runtime_source_sha']=runtime
            raw=tool.canonical(descriptor);a['local_descriptor_sha256']=tool.digest(raw)
            with mock.patch.object(tool,'read_owned',return_value=raw),mock.patch.object(tool,'owned_process') as native:
                with self.assertRaises(tool.Refused):tool.prepare_budget_key(Path('/unused-source'),Path('/unused-prepare'),Path('/unused-native'),1001,a,'22-3',object())
                native.assert_not_called()
    def test_preparation_creates_seed_once_then_removes_only_original_inputs_after_native_terminal(self):
        import os,tempfile
        from unittest import mock
        uid=os.getuid()
        with tempfile.TemporaryDirectory() as name:
            local=Path(name).resolve();local.chmod(0o700)
            prefix='/opt/qs-server/qs-apiserver/compatibility-retirement'
            parent=local/'retirement';parent.parent.chmod(0o700)
            original=local/'original';original.mkdir(mode=0o700)
            batch=local/'prepare';batch.mkdir(mode=0o700)
            native=batch/'restore-native';native.write_bytes(b'offline native placeholder');native.chmod(0o700)
            a=self.approval(self.request())
            descriptor={k:a[k] for k in ('tool_source_sha','operation_id','original_run_id','manifest_sha256')}
            descriptor.update(source_sha=a['original_source_sha'],runtime_source_sha='f'*40,host_role='server-a')
            raw=tool.canonical(descriptor);a['local_descriptor_sha256']=tool.digest(raw)
            (original/'budget-key.descriptor.private.json').write_bytes(raw);(original/'budget-key.descriptor.private.json').chmod(0o600)
            expected_seed=os.urandom(32);calls=[];exit_code=[0]
            def mapped(value):
                value=str(value)
                return parent/ value[len(prefix)+1:] if value.startswith(prefix+'/') else parent if value==prefix else Path(value)
            def protected(path,*,create=False):
                if create:path.mkdir(mode=0o700)
                self.assertTrue(path.is_dir());self.assertEqual(path.stat().st_mode&0o777,0o700)
            real_read=tool.read_owned
            def read(path,owner,expected,maximum,mode=0o600):
                # Files are real, but this offline test explicitly substitutes
                # its own UID. It does not supply production root/native proof.
                return real_read(path,uid,expected,maximum,mode)
            def native_call(command,environment,**options):
                self.assertEqual(command[1:3],['--mode','host-budget-key-create'])
                self.assertEqual(environment,{'PATH':'/usr/bin:/bin'})
                session=tool.decode(Path(command[4]).read_bytes())
                self.assertEqual(session['actual_run_id'],command[-1]);self.assertEqual(session['descriptor_sha256'],a['local_descriptor_sha256'])
                seed=Path(command[4]).parent/'issuer-seed';tool.write_new(seed,expected_seed)
                calls.append(command)
                return exit_code[0],tool.canonical(dict(kind='qs_native_temporary_budget_key_result',tool_source_sha=a['tool_source_sha'],operation_id=command[-3],actual_run_id=command[-1],public_key='e'*64,key_available=True,whole_writer_fence_proven=False,drop_ready=False,error_category='none' if not exit_code[0] else 'lifecycle_actual_budget_issuer_missing'))
            with mock.patch.object(tool,'Path',side_effect=mapped),mock.patch.object(tool,'protected_directory',side_effect=protected),mock.patch.object(tool,'read_owned',side_effect=read),mock.patch.object(tool,'owned_process',side_effect=native_call):
                result_hash=tool.prepare_budget_key(original,batch,native,uid,a,'22-3',object())
                directory=parent/'12-1'/'budget-issuer'
                self.assertEqual(set(p.name for p in directory.iterdir()),{'issuer-seed'})
                self.assertEqual((directory/'issuer-seed').read_bytes(),expected_seed)
                self.assertEqual((original/'budget-key.descriptor.private.json').read_bytes(),raw)
                self.assertEqual((batch/'budget-key.basis.private.json').read_bytes(),raw)
                self.assertEqual(result_hash,tool.digest((batch/'budget-key.result.private.json').read_bytes()))
                self.assertEqual(tool.decode((batch/'budget-key.result.private.json').read_bytes())['public_key'],'e'*64)
                with self.assertRaises(tool.Refused):tool.prepare_budget_key(original,batch,native,uid,a,'23-1',object())
                self.assertEqual(len(calls),1)
                # A nonterminal/native failure retains its actual uncertain
                # seed+inputs. It cannot use the preceding successful receipt.
                a['operation_id']='13-1';descriptor['operation_id']='13-1';changed=tool.canonical(descriptor);a['local_descriptor_sha256']=tool.digest(changed)
                (original/'budget-key.descriptor.private.json').write_bytes(changed);exit_code[0]=1
                next_batch=local/'prepare-2';next_batch.mkdir(mode=0o700)
                with self.assertRaises(tool.Refused):tool.prepare_budget_key(original,next_batch,native,uid,a,'24-1',object())
                self.assertEqual(set(p.name for p in (parent/'13-1'/'budget-issuer').iterdir()),{'issuer-seed','approved-services.json','service-session.json'})
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
            r=self.request();r['tool_source_sha']='d'*40;raw=tool.canonical(r);manifest=b'{"frozen":"manifest"}\n';r['manifest_sha256']=tool.digest(manifest);r['recovery']['manifest_sha256']=tool.digest(manifest)
            raw=tool.canonical(r);a=self.approval(r);a['tool_source_sha']='d'*40;a['manifest_sha256']=tool.digest(manifest)
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
            owner=object();check=[(0,b'd'*40+b'\n')];image_reads=[]
            def run_owned(command,environment,**options):
                self.assertIs(options['owner'],owner)
                self.assertEqual(options['control'],tool.sys.stdin.fileno())
                if command[1]=='--source-sha':return check[0]
                if command[0]=='/usr/bin/docker':
                    self.assertEqual(command[1:6],['--host','unix:///run/docker.sock','image','inspect','--format'])
                    self.assertEqual(command[6],'{"id":{{json .Id}},"os":{{json .Os}},"architecture":{{json .Architecture}},"revision":{{json (index .Config.Labels "org.opencontainers.image.revision")}}}')
                    self.assertEqual(command[7:], [a['b_image_id']])
                    self.assertEqual(environment,{'PATH':'/usr/bin:/bin'});self.assertEqual(options['timeout'],10)
                    image_reads.append(arguments[2])
                    return 0,tool.canonical({'id':a['b_image_id'],'os':'linux','architecture':'arm64','revision':a['tool_source_sha']})
                self.assertEqual(command[1:3],['--mode','lifecycle-prepare-root-once'])
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
            self.assertEqual(image_reads,['22-3','22-3','23-1','23-1'])
            self.assertEqual(before,{p.name:p.read_bytes() for p in original.iterdir()})
    def test_real_exclusive_write_never_overwrites_an_existing_receipt(self):
        import tempfile
        with tempfile.TemporaryDirectory() as name:
            path=Path(name)/'request'
            tool.write_new(path,b'first')
            with self.assertRaises(FileExistsError):tool.write_new(path,b'second')
            self.assertEqual(path.read_bytes(),b'first');self.assertEqual(path.stat().st_mode & 0o777,0o600)

class ActualRetirementCLI(unittest.TestCase):
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
    def test_actual_cli_source_sha_flag_reports_its_compiled_test_binding(self):
        import subprocess
        result=subprocess.run([str(self.native),'--source-sha'],env=self.environment,capture_output=True,timeout=5,check=True)
        self.assertEqual(result.stdout,b'd'*40+b'\n');self.assertEqual(result.stderr,b'')
    def test_actual_effectful_modes_refuse_before_request_or_database_access(self):
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

if __name__=='__main__':unittest.main()

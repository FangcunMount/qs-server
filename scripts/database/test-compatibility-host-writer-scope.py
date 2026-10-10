#!/usr/bin/env python3
"""Offline schema/pipe fixtures; no actual Linux root, SSH or host observation."""
import argparse
import copy
import hashlib
import importlib.util
import json
import io
import contextlib
from pathlib import Path
import subprocess
import unittest
from unittest import mock
spec=importlib.util.spec_from_file_location('retirement',Path(__file__).with_name('compatibility-retirement.py'))
api=importlib.util.module_from_spec(spec);spec.loader.exec_module(api)
class HostScopeTest(unittest.TestCase):
    def args(self):
        approval={'format_version':1,'kind':'readonly_host_writer_scope_descriptor','prepare_mode':'host-writer-scope','source_sha':'a'*40,'operation_id':'123-1','target_hash':api.TARGET_HASH,'host_role':'server_a'}
        raw=api.canonical_bytes(approval)
        return argparse.Namespace(operation='prepare',prepare_mode='host-writer-scope',operation_id='123-1',run_id='124-1',actual_source_sha='a'*40,approved_source_sha='a'*40,bootstrap_approval_json=raw.decode().strip(),bootstrap_approval_hash=hashlib.sha256(raw).hexdigest(),manifest_hash='',identity_request_hash='',inventory_request_hash='',lifecycle_request_hash='')
    def receipt(self,args,request,request_hash):
        return {'format_version':1,'kind':'readonly_host_writer_scope_observation','operation':'prepare','prepare_mode':'host-writer-scope','source_sha':args.actual_source_sha,'operation_id':args.operation_id,'run_id':args.run_id,'host_role':'server_a','source_uid':1001,'request_sha256':request_hash,'observation_approval_sha256':args.bootstrap_approval_hash,'target_hash':api.TARGET_HASH,'complete':False,'diagnostic_only':True,'execution_allowed':False,'drop_ready':False,'host_observation_complete':True,'writer_scope_complete':False,'observed_machine_id_sha256':'b'*64,'observed_boot_id_sha256':'c'*64,'observed_namespace_sha256':'d'*64,'host_scope_private_observation_sha256':'e'*64,'host_scope_catalog_sha256':'f'*64,'process_count':3,'file_count':4,'entry_count':5,'observation_elapsed_millis':1,'observed_scopes':[{'name':n,'enumeration_complete':True,'recheck_equal':True,'items':1,'catalog_sha256':'a'*64,'unknown':[]} for n in sorted(api.HOST_SCOPE_NAMES)],'unknown':['external_database_and_qs_ai_writers_not_observed'],'error_category':'none'}
    def test_closed_request_and_distinct_approval(self):
        args=self.args();request=api.host_scope_request(args)
        self.assertEqual(request['actual_run_id'],'124-1')
        args.bootstrap_approval_json=args.bootstrap_approval_json.replace('server_a','server_d')
        with self.assertRaises(api.Blocked):api.host_scope_request(args)
        args=self.args();v=json.loads(args.bootstrap_approval_json);v['complete']=False;args.bootstrap_approval_json=api.canonical_bytes(v).decode().strip();args.bootstrap_approval_hash=hashlib.sha256(api.canonical_bytes(v)).hexdigest()
        with self.assertRaises(api.Blocked):api.host_scope_request(args)
    def test_scope_never_mints_authority_or_false_code(self):
        args=self.args();request=api.host_scope_request(args);rh=hashlib.sha256(api.canonical_bytes(request)).hexdigest();r=self.receipt(args,request,rh)
        accepted=api.validate_host_scope_result(copy.deepcopy(r),args,request,rh,0)
        self.assertFalse(any(accepted['capabilities'].values()))
        for field in ('complete','writer_scope_complete','execution_allowed','drop_ready'):
            changed=copy.deepcopy(r);changed[field]=True
            with self.assertRaises(api.Blocked):api.validate_host_scope_result(changed,args,request,rh,0)
        with self.assertRaises(api.Blocked):api.validate_host_scope_result(r,args,request,rh,1)
    def test_partial_native_and_unknown_are_retained(self):
        args=self.args();request=api.host_scope_request(args);rh=hashlib.sha256(api.canonical_bytes(request)).hexdigest();r=self.receipt(args,request,rh)
        r['host_observation_complete']=False;r['error_category']='host_scope_observation_incomplete';r['observed_scopes'][0]['enumeration_complete']=False;r['observed_scopes'][0]['unknown']=['source_sshd_configuration_unread']
        accepted=api.validate_host_scope_result(copy.deepcopy(r),args,request,rh,1);self.assertFalse(accepted['host_observation_complete']);self.assertEqual(accepted['host_scope_private_observation_sha256'],'e'*64)
        r['unknown']=['raw-secret-or-unregistered-category']
        with self.assertRaises(api.Blocked):api.validate_host_scope_result(r,args,request,rh,1)
    def test_root_once_uses_same_package_binding_and_no_db_credentials(self):
        args=self.args();args.host_scope_request_hash='d'*64
        with mock.patch.dict(api.os.environ,{'RETIREMENT_PACKAGE_SHA256':'e'*64,'MYSQL_PASSWORD':'must-not-forward'},clear=True),mock.patch.object(api.os,'getuid',return_value=0),mock.patch.object(api.os,'geteuid',return_value=0),mock.patch.object(api.subprocess,'run',return_value=subprocess.CompletedProcess([],1,b'{}')) as run:
            api.root_once_lifecycle_prepare(args)
        call=run.call_args;self.assertEqual(json.loads(call.kwargs['input']),{});self.assertEqual(call.kwargs['env'],{'PATH':'/usr/bin:/bin'});self.assertIn('root-direct',call.args[0]);self.assertEqual(call.args[0][-1],'host-writer-scope');self.assertIn('d'*64,call.args[0]);self.assertIn('e'*64,call.args[0])
    def test_nonroot_uses_only_installed_fixed_entry_without_password_or_code(self):
        args=self.args();args.host_scope_request_hash='d'*64
        fixed=Path('/usr/local/libexec/qs-retirement')/('f'*64+'.py')
        with mock.patch.dict(api.os.environ,{'RETIREMENT_PACKAGE_SHA256':'e'*64,'PATH':'/attacker/path','SUDO_UID':'fake','SUDO_PASSWORD':'private-password'},clear=True),mock.patch.object(api.os,'getuid',return_value=1001),mock.patch.object(api.os,'geteuid',return_value=1001),mock.patch.object(api,'installed_fixed_host_entry',return_value=fixed),mock.patch.object(api,'root_askpass_environment') as askpass,mock.patch.object(api.subprocess,'run',return_value=subprocess.CompletedProcess([],1,b'{}')) as run:
            api.root_once_lifecycle_prepare(args)
        call=run.call_args;self.assertEqual(call.args[0],['/usr/bin/sudo','-n','--','/usr/bin/python3','-I',str(fixed)]);self.assertEqual(call.kwargs['env'],{'PATH':'/usr/bin:/bin'});self.assertEqual(json.loads(call.kwargs['input']),{'operation_id':'123-1','run_id':'124-1','source_sha':'a'*40,'request_sha256':'d'*64,'package_sha256':'e'*64});askpass.assert_not_called()
        self.assertNotIn('private-password',repr(call));self.assertNotIn('-c',call.args[0])
    def test_nonroot_missing_installation_rejects_before_sudo_without_fallback(self):
        args=self.args();args.host_scope_request_hash='d'*64
        with mock.patch.dict(api.os.environ,{'RETIREMENT_PACKAGE_SHA256':'e'*64},clear=True),mock.patch.object(api.os,'getuid',return_value=1001),mock.patch.object(api.os,'geteuid',return_value=1001),mock.patch.object(api.subprocess,'run') as run,self.assertRaises(api.Blocked):
            api.root_once_lifecycle_prepare(args)
        run.assert_not_called()
    def test_fixed_policy_refusal_remains_native_failure_without_authority(self):
        args=self.args();args.host_scope_request_hash='d'*64
        raw=b'{"format_version":1,"complete":false,"execution_allowed":false,"drop_ready":false,"error_category":"fixed_host_entry_rejected"}\n'
        with mock.patch.dict(api.os.environ,{'RETIREMENT_PACKAGE_SHA256':'e'*64},clear=True),mock.patch.object(api.os,'getuid',return_value=1001),mock.patch.object(api.os,'geteuid',return_value=1001),mock.patch.object(api,'installed_fixed_host_entry',return_value=Path('/usr/local/libexec/qs-retirement')/('f'*64+'.py')),mock.patch.object(api.subprocess,'run',return_value=subprocess.CompletedProcess([],1,raw)),self.assertRaises(api.NativeReceiptBlocked) as error:
            api.root_once_lifecycle_prepare(args)
        self.assertEqual(str(error.exception),'fixed_host_entry_rejected');self.assertTrue(error.exception.native_diagnostic['process_completed']);self.assertEqual(error.exception.native_diagnostic['exit_code'],1)
    def test_actual_transport_fixed_a_role_early_refusal_keeps_no_observation(self):
        args=self.args();request=api.host_scope_request(args);rh=hashlib.sha256(api.canonical_bytes(request)).hexdigest();r=self.receipt(args,request,rh)
        r.update(host_observation_complete=False,error_category='host_scope_root_once_required',source_uid=0,process_count=0,file_count=0,entry_count=0,observed_scopes=[],unknown=[],observed_machine_id_sha256='',observed_boot_id_sha256='',observed_namespace_sha256='',host_scope_private_observation_sha256='',host_scope_catalog_sha256='',observation_approval_sha256='',observation_elapsed_millis=2)
        r=api.validate_host_scope_result(r,args,request,rh,1);output=io.StringIO()
        with mock.patch.object(api,'execute',return_value=r),mock.patch.dict(api.os.environ,{},clear=True),contextlib.redirect_stdout(output):
            code=api.main(['--operation','prepare','--operation-id',args.operation_id,'--approved-source-sha',args.actual_source_sha,'--actual-source-sha',args.actual_source_sha,'--run-id',args.run_id,'--prepare-mode','host-writer-scope'])
        self.assertEqual(code,42);body=json.loads(api.transport().decode_armored_receipt(output.getvalue()));self.assertEqual(body['error_category'],'host_scope_root_once_required');self.assertEqual(body['host_role'],'server_a');self.assertFalse(body['host_observation_complete']);self.assertFalse(any(body['capabilities'].values()));self.assertEqual(body['observed_machine_id_sha256'],'');self.assertEqual(body['host_scope_private_observation_sha256'],'')
    def test_empty_unread_approval_is_only_valid_for_exact_early_refusal_profile(self):
        args=self.args();request=api.host_scope_request(args);rh=hashlib.sha256(api.canonical_bytes(request)).hexdigest()
        early=self.receipt(args,request,rh)
        early.update(host_observation_complete=False,error_category='host_scope_root_once_required',source_uid=0,process_count=0,file_count=0,entry_count=0,observed_scopes=[],unknown=[],observed_machine_id_sha256='',observed_boot_id_sha256='',observed_namespace_sha256='',host_scope_private_observation_sha256='',host_scope_catalog_sha256='',observation_approval_sha256='',observation_elapsed_millis=2)
        for error in api.HOST_SCOPE_EARLY_ERRORS:
            with self.subTest(error=error):
                actual=copy.deepcopy(early);actual['error_category']=error
                accepted=api.validate_host_scope_result(actual,args,request,rh,1)
                self.assertEqual(accepted['observation_approval_sha256'],'');self.assertEqual(accepted['error_category'],error)
                self.assertFalse(any(accepted['capabilities'].values()))
        for field,value in [('host_observation_complete',True),('source_uid',1001),('process_count',1),('file_count',1),('entry_count',1),
            ('observed_machine_id_sha256','a'*64),('observed_boot_id_sha256','a'*64),('observed_namespace_sha256','a'*64),
            ('host_scope_private_observation_sha256','a'*64),('host_scope_catalog_sha256','a'*64),
            ('observed_scopes',[{'name':'ssh_configuration_sources','enumeration_complete':False,'recheck_equal':False,'items':0,'catalog_sha256':'a'*64,'unknown':[]}]),
            ('unknown',['external_database_and_qs_ai_writers_not_observed']),('complete',True),('execution_allowed',True),('drop_ready',True),('writer_scope_complete',True),('diagnostic_only',False),
            ('error_category','host_scope_observation_incomplete'),('error_category','none'),('observation_approval_sha256','f'*64),('source_uid',False)]:
            with self.subTest(field=field,value=value):
                changed=copy.deepcopy(early);changed[field]=value
                with self.assertRaises(api.Blocked):api.validate_host_scope_result(changed,args,request,rh,1)
        for code in (0,2,True):
            with self.subTest(code=code),self.assertRaises(api.Blocked):api.validate_host_scope_result(copy.deepcopy(early),args,request,rh,code)
        success=self.receipt(args,request,rh)
        for approval in ('','f'*64):
            success['observation_approval_sha256']=approval
            with self.subTest(approval=approval),self.assertRaises(api.Blocked):api.validate_host_scope_result(copy.deepcopy(success),args,request,rh,0)

    def test_effectful_input_rejected_before_root_call(self):
        args=self.args();args.operation='apply'
        with mock.patch.object(api,'live_host_scope') as live:
            with self.assertRaises(api.Blocked):api.execute(args)
            live.assert_not_called()
if __name__=='__main__':unittest.main()

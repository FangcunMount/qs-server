#!/usr/bin/env python3
"""Offline request/strict public receipt fixtures. No native DB/root observation."""
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
class DBCensusTest(unittest.TestCase):
    def args(self):
        reference={'operation_id':'123-1','run_id':'120-2','source_sha':'b'*40,'sha256':'c'*64,'request_sha256':'d'*64}
        approval={'format_version':1,'kind':'readonly_db_writer_census_descriptor','prepare_mode':'db-writer-census','source_sha':'a'*40,'operation_id':'123-1','target_hash':api.TARGET_HASH,'database_scope':'mysql-and-mongodb','identity_report':reference}
        raw=api.canonical_bytes(approval)
        return argparse.Namespace(operation='prepare',prepare_mode='db-writer-census',operation_id='123-1',run_id='124-1',actual_source_sha='a'*40,approved_source_sha='a'*40,bootstrap_approval_json=raw.decode().strip(),bootstrap_approval_hash=hashlib.sha256(raw).hexdigest(),manifest_hash='',identity_request_hash='',inventory_request_hash='',lifecycle_request_hash='')
    def receipt(self,args,request,rh):
        return {'format_version':1,'kind':'readonly_db_writer_census_observation','operation':'prepare','prepare_mode':'db-writer-census','source_sha':args.actual_source_sha,'operation_id':args.operation_id,'run_id':args.run_id,'source_uid':1001,'target_hash':api.TARGET_HASH,'request_sha256':rh,'observation_approval_sha256':args.bootstrap_approval_hash,'observed_identity_producer':request['identity_report'],'mysql_identity_sha256':'1'*64,'mongodb_identity_sha256':'2'*64,'mongodb_namespace_anchor_sha256':'3'*64,'mysql_migration_version':99,'mongodb_migration_version':38,'complete':False,'diagnostic_only':True,'execution_allowed':False,'drop_ready':False,'db_census_observation_complete':True,'writer_scope_complete':False,'mysql_all_connections_permission_proven':True,'mongodb_local_all_sessions_permission_proven':True,'all_nodes_sessions_coverage_complete':False,'external_writer_coverage_complete':False,'db_census_private_catalog_sha256':'4'*64,'db_census_catalog_sha256':'5'*64,'observed_sections':[{'name':n,'enumeration_complete':True,'recheck_equal':False,'items':0,'sha256':'6'*64,'error_category':'none'} for n in sorted(api.DB_CENSUS_SECTION_NAMES)],'unknown':['mongodb_other_nodes_sessions_and_external_authentication_unobserved'],'observation_elapsed_millis':12,'error_category':'none'}
    def prepared(self):
        args=self.args();request=api.db_census_request(args);rh=hashlib.sha256(api.canonical_bytes(request)).hexdigest();return args,request,rh
    def test_actual_original_producer_is_not_relabelled(self):
        args,request,_=self.prepared();self.assertEqual(request['identity_report']['source_sha'],'b'*40);self.assertEqual(request['source_sha'],'a'*40)
        for key,value in [('run_id',args.run_id),('source_sha','invalid'),('account','must-not-accept')]:
            a=json.loads(args.bootstrap_approval_json);a['identity_report'][key]=value;raw=api.canonical_bytes(a);args.bootstrap_approval_json=raw.decode().strip();args.bootstrap_approval_hash=hashlib.sha256(raw).hexdigest()
            with self.assertRaises(api.Blocked):api.db_census_request(args)
            args=self.args()
    def test_eof_observation_does_not_mint_full_coverage_or_fence(self):
        args,request,rh=self.prepared();r=self.receipt(args,request,rh);accepted=api.validate_db_census_result(copy.deepcopy(r),args,request,rh,0);self.assertFalse(any(accepted['capabilities'].values()));self.assertTrue(accepted['db_census_observation_complete']);self.assertFalse(accepted['writer_scope_complete'])
        for key in ('writer_scope_complete','complete','drop_ready','execution_allowed'):
            bad=copy.deepcopy(r);bad[key]=True
            with self.assertRaises(api.Blocked):api.validate_db_census_result(bad,args,request,rh,0)
    def test_session_permission_failure_remains_incomplete(self):
        args,request,rh=self.prepared();r=self.receipt(args,request,rh);r['db_census_observation_complete']=False;r['error_category']='db_census_catalog_or_session_permissions_incomplete';r['observed_sections'][0]['enumeration_complete']=False;r['observed_sections'][0]['error_category']='db_census_mongo_query_failed_or_bounded'
        accepted=api.validate_db_census_result(r,args,request,rh,1);self.assertFalse(accepted['db_census_observation_complete']);self.assertEqual(accepted['db_census_private_catalog_sha256'],'4'*64)
        with self.assertRaises(api.Blocked):api.validate_db_census_result(self.receipt(args,request,rh),args,request,rh,1)
    def test_closed_partial_pre_identity_fields_and_credential_pipe(self):
        args,request,rh=self.prepared();r=self.receipt(args,request,rh);r.update(db_census_observation_complete=False,mysql_all_connections_permission_proven=False,mongodb_local_all_sessions_permission_proven=False,error_category='db_census_original_identity_read_rejected',observed_sections=[],observed_identity_producer={k:'' for k in request['identity_report']},mysql_migration_version=0,mongodb_migration_version=0)
        self.assertFalse(api.validate_db_census_result(r,args,request,rh,1)['db_census_observation_complete'])
        args.db_census_request_hash='7'*64
        with mock.patch.dict(api.os.environ,{'RETIREMENT_PACKAGE_SHA256':'8'*64,'MYSQL_PASSWORD':'pipe-only'},clear=True),mock.patch.object(api.os,'getuid',return_value=1001),mock.patch.object(api.os,'geteuid',return_value=1001),mock.patch.object(api.subprocess,'run',return_value=subprocess.CompletedProcess([],1,b'{}')) as run:
            api.root_once_lifecycle_prepare(args)
        call=run.call_args;self.assertEqual(call.args[0][:4],['/usr/bin/sudo','-n','--','/usr/bin/python3']);self.assertEqual(call.kwargs['env'],{'PATH':'/usr/bin:/bin'});self.assertEqual(json.loads(call.kwargs['input'])['MYSQL_PASSWORD'],'pipe-only');self.assertNotIn('pipe-only',str(call.args[0]));self.assertEqual(call.args[0][-1],'db-writer-census')
    def test_actual_transport_retains_partial_original_read_category(self):
        args,request,rh=self.prepared();r=self.receipt(args,request,rh)
        r.update(db_census_observation_complete=False,mysql_all_connections_permission_proven=False,mongodb_local_all_sessions_permission_proven=False,error_category='db_census_original_identity_read_rejected',observed_sections=[],observed_identity_producer={k:'' for k in request['identity_report']},mysql_identity_sha256='',mongodb_identity_sha256='',mongodb_namespace_anchor_sha256='',db_census_private_catalog_sha256='',db_census_catalog_sha256='',observation_approval_sha256='',mysql_migration_version=0,mongodb_migration_version=0)
        validated=api.validate_db_census_result(r,args,request,rh,1);output=io.StringIO()
        with mock.patch.object(api,'execute',return_value=validated),mock.patch.dict(api.os.environ,{},clear=True),contextlib.redirect_stdout(output):
            code=api.main(['--operation','prepare','--operation-id',args.operation_id,'--approved-source-sha',args.actual_source_sha,'--actual-source-sha',args.actual_source_sha,'--run-id',args.run_id,'--prepare-mode','db-writer-census'])
        self.assertEqual(code,42)
        decoded=json.loads(api.transport().decode_armored_receipt(output.getvalue()));self.assertEqual(decoded['error_category'],'db_census_original_identity_read_rejected');self.assertEqual(decoded['observed_identity_producer'],{k:'' for k in request['identity_report']});self.assertFalse(any(decoded['capabilities'].values()))
    def test_effectful_and_raw_dynamic_categories_rejected(self):
        args=self.args();args.operation='apply'
        with mock.patch.object(api,'live_db_census') as live:
            with self.assertRaises(api.Blocked):api.execute(args)
            live.assert_not_called()
        args,request,rh=self.prepared();r=self.receipt(args,request,rh);r['unknown']=['private_account_name']
        with self.assertRaises(api.Blocked):api.validate_db_census_result(r,args,request,rh,0)
if __name__=='__main__':unittest.main()

#!/usr/bin/env python3
"""Offline synthetic report inputs; never production or inventory execution proof."""
import argparse
import copy
import contextlib
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import re
import subprocess
import textwrap
import unittest
from unittest import mock

BASE_SCRIPT = Path(__file__).with_name('test-compatibility-retirement.py')
spec = importlib.util.spec_from_file_location('existing_boundary_contracts', BASE_SCRIPT)
base = importlib.util.module_from_spec(spec)
spec.loader.exec_module(base)
tool = base.tool


class InventoryReportDiagnosticContracts(unittest.TestCase):
    write = base.ReportDiagnosticSafetyContracts.write
    approve = base.ReportDiagnosticSafetyContracts.approve
    inventory = base.ReportDiagnosticSafetyContracts.inventory
    assertBlocked = base.ReportDiagnosticSafetyContracts.assertBlocked
    tearDown = base.ReportDiagnosticSafetyContracts.tearDown

    def setUp(self):
        base.ReportDiagnosticSafetyContracts.setUp(self)
        self.output = self.directory / ('inventory-' + base.DIAGNOSTIC_RUN)
        self.output.mkdir(mode=0o700)
        self.request.update(kind='readonly_inventory_request', boundary_run_id='701-1', boundary_report_hash='5'*64,
                            approved_boundaries=[dict(zip(('database','name','kind'), target), present=False, empty=False,
                            pk_type='', upper_token='', schema_hash='6'*64, identity_hash='7'*64) for target in tool.TARGETS])
        self.request_hash = self.write(self.directory/'inventory-request.json', self.request)
        self.report.update(kind='readonly_compatibility_inventory', request_hash=self.request_hash,
                           boundary_report_hash=self.request['boundary_report_hash'],
                           source_bytes_protocol='mysql_cast_binary_columns_pk_order_v2+mongodb_server_bson_pk_order_v2',
                           consistency_semantics='two_equal_complete_passes_within_independently_approved_upper;sql_same_readonly_snapshot;mongo_homogeneous_bson_id_simple_collation;after_upper_next_cycle_not_fenced')
        self.report['database_bindings']['mongodb']['error_category']='mongo_target_read_failed_or_timed_out'
        self.report['targets']=[dict(zip(('database','name','kind'), target), present=True, complete=False,
                    records=0, bytes=0, pages=0, equal_full_passes=0, next_cycle_required=False,
                    schema_hash='', data_hash='', identity_hash='', classification={}, error_category='none') for target in tool.TARGETS]
        self.report_hash=self.write(self.output/'inventory.private.json',self.report)
        self.approval['kind']='readonly_existing_inventory_report_approval'
        self.approval['inventory_report']=self.approval.pop('boundary_report')
        self.approval['inventory_report'].update(sha256=self.report_hash,request_sha256=self.request_hash)
        self.approve()

    def update_report(self):
        self.approval['inventory_report']['sha256']=self.write(self.output/'inventory.private.json',self.report)
        self.approve()

    def test_exact_original_files_read_only_no_lock_no_child_no_credentials(self):
        before=self.inventory()
        with mock.patch.object(tool,'live_inventory',side_effect=AssertionError('DB rescan')),             mock.patch.object(tool,'capture_fixed',side_effect=AssertionError('child')),             mock.patch.object(tool,'inventory_connection_values',side_effect=AssertionError('credentials')),             mock.patch.object(tool,'locked_operation',side_effect=AssertionError('operation lock')),             mock.patch.object(tool,'validate_source_asset',side_effect=AssertionError('source body')):
            receipt=tool.execute(self.args)
        self.assertEqual(receipt['inventory_database_error_categories']['mongodb'],'mongo_target_read_failed_or_timed_out')
        self.assertTrue(receipt['report_diagnostic_complete']);self.assertFalse(receipt['inventory_complete'])
        self.assertFalse(receipt['complete']);self.assertFalse(receipt['execution_allowed']);self.assertFalse(receipt['drop_ready'])
        self.assertTrue(all(value is False for value in receipt['capabilities'].values()))
        self.assertEqual(receipt['observed_inventory_report']['source_sha'],base.DIAGNOSTIC_ORIGINAL)
        self.assertEqual(receipt['source_sha'],base.DIAGNOSTIC_SOURCE)
        self.assertNotIn('observed_boundary_report',receipt)
        self.assertEqual(before,self.inventory());self.assertFalse((self.directory/'operation.lock').exists())

    def test_every_new_category_is_an_actual_literal_native_producer_category(self):
        native_dir=base.SCRIPT.parents[2]/'cmd/qs-compatibility-retirement'
        literals=set(re.findall(r'category\("([a-z_]+)"\)', ''.join((native_dir/name).read_text() for name in ('main.go','paging.go'))))
        for database in ('mysql','mongodb'):
            additions=tool.INVENTORY_REPORT_DIAGNOSTIC_ERRORS[database]-tool.INVENTORY_DIAGNOSTIC_ERRORS[database]
            self.assertTrue(additions <= literals,repr(additions-literals))
            for category in additions:
                with self.subTest(database=database,category=category):
                    self.report['database_bindings'][database]['error_category']=category;self.update_report()
                    self.assertEqual(tool.execute(self.args)['inventory_database_error_categories'][database],category)

    def test_raw_driver_or_wrong_database_categories_reduce_to_fixed_unknown(self):
        for private in ('PRIVATE_URI_AND_BODY','mongo_target_read_failed_or_timed_out',{'private':'PRIVATE_BODY'}):
            with self.subTest(private=private):
                self.report['database_bindings']['mysql']['error_category']=private;self.update_report()
                receipt=tool.execute(self.args)
                self.assertEqual(receipt['inventory_database_error_categories']['mysql'],'inventory_database_error_unrecognized')
                self.assertNotIn('PRIVATE_',json.dumps(receipt))

    def test_boundary_reference_cannot_select_inventory_and_both_refs_rejected(self):
        original=copy.deepcopy(self.approval)
        self.approval['boundary_report']=self.approval.pop('inventory_report');self.approve()
        self.assertBlocked('evidence_fields_invalid')
        self.approval=copy.deepcopy(original);self.approval['boundary_report']=copy.deepcopy(self.approval['inventory_report']);self.approve()
        self.assertBlocked('evidence_fields_invalid')

    def test_source_run_operation_request_target_protocol_and_boundary_bindings(self):
        original=copy.deepcopy(self.report)
        for key,value in [('source_sha',base.DIAGNOSTIC_SOURCE),('run_id','704-1'),('operation_id','124-1'),
                ('request_hash','0'*64),('target_hash','0'*64),('boundary_report_hash','0'*64),
                ('kind','readonly_inventory_boundaries'),('format_version',1),('drop_ready',True),
                ('diagnostic_only',False),('source_bytes_protocol','no_source_body_copy'),('consistency_semantics','unknown')]:
            with self.subTest(key=key):
                self.report=dict(original,**{key:value});self.update_report();self.assertBlocked('report_diagnostic_report_binding_invalid')

    def test_request_source_hash_and_report_hash_are_exact(self):
        path=self.output/'inventory.private.json';raw=path.read_bytes();path.write_bytes(raw+b' ')
        self.assertBlocked('evidence_hash_mismatch');path.write_bytes(raw)
        path=self.directory/'inventory-request.json';path.write_bytes(path.read_bytes()+b' ')
        self.assertBlocked('evidence_hash_mismatch')

    def test_inventory_path_is_not_boundary_path(self):
        (self.output/'inventory.private.json').unlink()
        self.assertBlocked('evidence_unavailable')

    def test_original_request_source_is_not_relabelled_as_tool_source(self):
        self.request['source_sha']=base.DIAGNOSTIC_SOURCE
        self.approval['inventory_report']['request_sha256']=self.write(self.directory/'inventory-request.json',self.request);self.approve()
        self.assertBlocked('evidence_binding_mismatch')

    def test_target_scope_duplicates_unknown_fields_and_invalid_uint_rejected(self):
        original=copy.deepcopy(self.report)
        for mutation in ('duplicate','unknown','negative','extra'):
            self.report=copy.deepcopy(original)
            if mutation=='duplicate': self.report['targets'][1]=copy.deepcopy(self.report['targets'][0])
            elif mutation=='unknown':self.report['targets'][0]['name']='PRIVATE_OTHER_TABLE'
            elif mutation=='negative':self.report['targets'][0]['records']=-1
            else:self.report['private_extra']='PRIVATE_BODY'
            self.update_report()
            with self.subTest(mutation=mutation),self.assertRaises(tool.Blocked):tool.execute(self.args)

    def test_incomplete_scan_never_becomes_success_even_when_categories_none(self):
        self.report['database_bindings']['mongodb']['error_category']='none';self.update_report()
        self.assertFalse(tool.execute(self.args)['inventory_complete'])
        self.report['complete']=True;self.update_report();self.assertBlocked('database_anchor_missing')

    def test_original_file_recheck_detects_drift_and_symlink_hardlink_fifo_reject(self):
        read=tool.read_private;count=0
        def changing(directory,filename,expected_hash=None):
            nonlocal count
            result=read(directory,filename,expected_hash)
            if filename=='inventory.private.json':
                count+=1
                if count==1:
                    path=directory/filename;path.write_bytes(path.read_bytes()+b' ')
            return result
        with mock.patch.object(tool,'read_private',side_effect=changing):self.assertBlocked('evidence_hash_mismatch')
        self.update_report()
        path=self.output/'inventory.private.json';raw=path.read_bytes();path.unlink()
        peer=self.output/'peer.json';peer.write_bytes(raw);peer.chmod(0o600)
        path.symlink_to(peer);self.assertBlocked('evidence_unavailable');path.unlink()
        os.link(peer,path);self.assertBlocked('evidence_not_private');path.unlink();peer.unlink()
        os.mkfifo(path,0o600);self.assertBlocked('evidence_not_private')

    def test_actual_cli_armored_transport_fixed_categories_only(self):
        args=['--operation','prepare','--root',str(self.root),'--operation-id',base.DIAGNOSTIC_OP,
              '--approved-source-sha',base.DIAGNOSTIC_SOURCE,'--actual-source-sha',base.DIAGNOSTIC_SOURCE,
              '--run-id',base.DIAGNOSTIC_OBSERVE,'--prepare-mode','report-diagnostic',
              '--bootstrap-approval-json',self.args.bootstrap_approval_json,'--bootstrap-approval-hash',self.args.bootstrap_approval_hash]
        result=subprocess.run(['python3','-B',str(base.SCRIPT),*args],capture_output=True,timeout=5)
        self.assertEqual(result.returncode,0,result.stderr.decode());self.assertEqual(result.stderr,b'')
        receipt=json.loads(tool.transport().decode_armored_receipt(result.stdout.decode().strip()))
        self.assertEqual(receipt['inventory_database_error_categories']['mongodb'],'mongo_target_read_failed_or_timed_out')
        self.assertEqual(receipt['inventory_private_report_hash'],self.report_hash)
        self.assertNotIn('boundary_private_report_hash',receipt);self.assertNotIn('targets',receipt)
        self.assertNotIn(str(self.root),json.dumps(receipt));self.assertFalse(receipt['inventory_complete'])

    def test_actual_node_validator_accepts_inventory_reference_only_and_keeps_ten_inputs(self):
        script=textwrap.dedent(base.DIAGNOSTIC_WORKFLOW.read_text().split('          script: |\n',1)[1].split('      - name:',1)[0])
        supplied={'operation':'prepare','database':'mysql-and-mongodb','approved_source_sha':base.DIAGNOSTIC_SOURCE,
                  'operation_id':base.DIAGNOSTIC_OP,'prepare_mode':'report-diagnostic',
                  'bootstrap_approval_json':self.args.bootstrap_approval_json,'bootstrap_approval_sha256':self.args.bootstrap_approval_hash}
        cases=[('valid',supplied,True),('eleventh_input',dict(supplied,unknown=''),False),
               ('mutation',dict(supplied,operation='apply'),False),('mixed_request',dict(supplied,inventory_request_sha256='1'*64),False)]
        for mutation in ('boundary_key','both_keys','same_run','wrong_tool_source','bad_ref_hash','unknown_kind'):
            descriptor=copy.deepcopy(self.approval)
            if mutation=='boundary_key':descriptor['boundary_report']=descriptor.pop('inventory_report')
            elif mutation=='both_keys':descriptor['boundary_report']=copy.deepcopy(descriptor['inventory_report'])
            elif mutation=='same_run':descriptor['inventory_report']['run_id']=base.DIAGNOSTIC_OBSERVE
            elif mutation=='wrong_tool_source':descriptor['source_sha']=base.DIAGNOSTIC_ORIGINAL
            elif mutation=='bad_ref_hash':descriptor['inventory_report']['sha256']='0'*63
            else:descriptor['kind']='readonly_existing_unknown_report_approval'
            raw=tool.canonical_bytes(descriptor)
            cases.append((mutation,dict(supplied,bootstrap_approval_json=raw[:-1].decode(),bootstrap_approval_sha256=hashlib.sha256(raw).hexdigest()),False))
        for name,inputs,allowed in cases:
            context={'payload':{'inputs':inputs},'ref':'refs/heads/main','sha':base.DIAGNOSTIC_SOURCE,'runId':703,'repo':{}}
            program=('const script='+json.dumps(script)+';const context='+json.dumps(context)+';const current='+json.dumps(base.DIAGNOSTIC_SOURCE)
                     +';const github={rest:{repos:{getCommit:async()=>({data:{sha:current}})}}};'
                     +"new (Object.getPrototypeOf(async function(){}).constructor)('context','github',script)(context,github).catch(()=>{process.exitCode=1;});")
            result=subprocess.run(['node','-e',program],capture_output=True,env=dict(os.environ,GITHUB_RUN_ATTEMPT='1'),timeout=5)
            with self.subTest(name=name):self.assertEqual(result.returncode,0 if allowed else 1,result.stderr.decode())
        source=base.DIAGNOSTIC_WORKFLOW.read_text()
        keys=re.findall(r'^      ([a-z_0-9]+):$',source.split('jobs:',1)[0],re.M)
        self.assertEqual(len(keys),10,keys)


class FailedInventoryCleanupContracts(unittest.TestCase):
    write = base.ReportDiagnosticSafetyContracts.write
    approve = base.ReportDiagnosticSafetyContracts.approve
    tearDown = base.ReportDiagnosticSafetyContracts.tearDown

    def setUp(self):
        base.ReportDiagnosticSafetyContracts.setUp(self)
        self.actual_reference = tool.FAILED_INVENTORY_REFERENCE.copy()
        self.directory.rename(self.root/tool.FAILED_INVENTORY_OPERATION)
        self.directory = self.root/tool.FAILED_INVENTORY_OPERATION
        self.args.operation_id = tool.FAILED_INVENTORY_OPERATION
        self.output = self.directory/('inventory-'+self.actual_reference['run_id']); self.output.mkdir(mode=0o700)
        self.request.update(kind='readonly_inventory_request', source_sha=self.actual_reference['source_sha'],
            operation_id=tool.FAILED_INVENTORY_OPERATION, limits=tool.FAILED_INVENTORY_LIMITS.copy(),
            boundary_run_id='701-1', boundary_report_hash='5'*64,
            approved_boundaries=[dict(zip(('database','name','kind'),t),present=False,empty=False,
                pk_type='',upper_token='',schema_hash='6'*64,identity_hash='7'*64) for t in tool.TARGETS])
        request_hash=self.write(self.directory/'inventory-request.json',self.request)
        self.report.update(kind='readonly_compatibility_inventory',source_sha=self.actual_reference['source_sha'],
            operation_id=tool.FAILED_INVENTORY_OPERATION,run_id=self.actual_reference['run_id'],request_hash=request_hash,
            targets=[dict(zip(('database','name','kind'),t),complete=i<3,equal_full_passes=2 if i<3 else 0,
                pages=2 if i<3 else 0,records=1 if i<3 else 0,error_category='none') for i,t in enumerate(tool.TARGETS)])
        report_hash=self.write(self.output/'inventory.private.json',self.report)
        self.reference=dict(self.actual_reference,request_sha256=request_hash,sha256=report_hash)
        self.patch=mock.patch.object(tool,'FAILED_INVENTORY_REFERENCE',self.reference);self.patch.start();self.addCleanup(self.patch.stop)
        self.approval.update(kind='cleanup_only_failed_inventory_baseline_approval',operation_id=tool.FAILED_INVENTORY_OPERATION)
        self.approval['inventory_report']=self.reference.copy();self.approval.pop('boundary_report');self.approve()
        for i,(target,filename) in enumerate(tool.SOURCE_FILENAMES.items()):
            path=self.output/filename;path.write_bytes(b'SYNTHETIC_BODY');path.chmod(0o600)
            self.write(self.output/(filename+'.asset.json'),dict(format_version=1,kind='temporary_inventory_source_copy',
                filename=filename,source_sha=self.reference['source_sha'],operation_id=tool.FAILED_INVENTORY_OPERATION,
                run_id=self.reference['run_id'],request_hash=request_hash,
                protocol='mysql_cast_binary_columns_pk_order_v2' if target[0]=='mysql' else 'mongodb_server_bson_pk_order_v2',
                boundary=self.request['approved_boundaries'][i],contains_original_body=True,retirement_proof=False,
                purge_required_after_acceptance=True,resume_existing_file_allowed=False))
        self.write(self.output/'entrypoints.private.json',dict(format_version=1,kind='source_only_production_entrypoint_catalog',
            source_sha=self.reference['source_sha'],operation_id=tool.FAILED_INVENTORY_OPERATION,run_id=self.reference['run_id'],
            request_hash=request_hash,catalog_hash='a'*64,live_fence_proven=False,catalog={}))
        self.write(self.output/'mysql-metadata.private.json',{'synthetic_schema_only':True})
        _,_,_,_,checkpoints=tool.failed_inventory_inputs(self.args)
        for name,(pass_id,page) in checkpoints.items():
            self.write(self.output/name,dict(format_version=1,kind='readonly_inventory_page_checkpoint',
                source_sha=self.reference['source_sha'],**{'pass':pass_id},page=page,cursor_token='',records=0,
                source_bytes=0,prefix_hash='a'*64,diagnostic_only=True,resume_existing_file_allowed=False))

    def execute(self):
        with mock.patch.object(tool,'capture_fixed',return_value=(0,b'')) as capture, \
             mock.patch.object(tool,'live_inventory',side_effect=AssertionError('DB rescan')), \
             mock.patch.object(tool,'inventory_connection_values',side_effect=AssertionError('credentials')):
            receipt=tool.execute(self.args)
        self.assertEqual(capture.call_count,4)
        for call in capture.call_args_list:
            self.assertEqual(call.args[0][:6],['sudo','-n','docker','container','ls','--all'])
        return receipt

    def test_exact_fd_hash_baseline_does_not_remove_or_certify_original_content(self):
        names=set(p.name for p in self.output.iterdir());receipt=self.execute()
        baseline_path=self.directory/tool.FAILED_INVENTORY_BASELINE;raw=baseline_path.read_bytes();baseline=json.loads(raw)
        self.assertEqual(receipt['cleanup_baseline_sha256'],hashlib.sha256(raw).hexdigest())
        self.assertEqual(set(baseline['files']),names);self.assertEqual(set(p.name for p in self.output.iterdir()),names)
        self.assertEqual(receipt['cleanup_file_count'],len(names));self.assertFalse(receipt['inventory_complete'])
        self.assertFalse(receipt['complete']);self.assertFalse(receipt['execution_allowed']);self.assertFalse(receipt['drop_ready'])
        self.assertFalse(receipt['original_content_verified']);self.assertFalse(receipt['purge_executed'])
        self.assertNotIn('SYNTHETIC_BODY',raw.decode());self.assertTrue(all(v is False for v in receipt['capabilities'].values()))
        source=next(iter(tool.SOURCE_FILENAMES.values()));self.assertEqual(baseline['files'][source]['stat'][1],(self.output/source).stat().st_ino)
        with self.assertRaises(tool.Blocked):self.execute()  # O_EXCL, no overwrite/resume

    def main_arguments(self):
        return ['--operation','prepare','--root',str(self.root),'--operation-id',tool.FAILED_INVENTORY_OPERATION,
            '--approved-source-sha',base.DIAGNOSTIC_SOURCE,'--actual-source-sha',base.DIAGNOSTIC_SOURCE,
            '--run-id',base.DIAGNOSTIC_OBSERVE,'--prepare-mode','report-diagnostic',
            '--bootstrap-approval-json',self.args.bootstrap_approval_json,
            '--bootstrap-approval-hash',self.args.bootstrap_approval_hash]

    def test_real_main_executes_baseline_and_encodes_closed_armored_receipt(self):
        out,err=io.StringIO(),io.StringIO();transport=tool.transport()
        with mock.patch.object(tool,'capture_fixed',return_value=(0,b'')), \
             mock.patch.object(tool,'live_inventory',side_effect=AssertionError('DB rescan')), \
             mock.patch.object(tool,'transport',return_value=transport), \
             mock.patch.object(transport,'encode_armored_receipt',wraps=transport.encode_armored_receipt) as encode, \
             contextlib.redirect_stdout(out),contextlib.redirect_stderr(err):
            code=tool.main(self.main_arguments())
        self.assertEqual(code,0,err.getvalue());self.assertEqual(err.getvalue(),'');encode.assert_called_once()
        receipt=json.loads(transport.decode_armored_receipt(out.getvalue().strip()))
        baseline=(self.directory/tool.FAILED_INVENTORY_BASELINE).read_bytes()
        self.assertEqual(receipt['cleanup_baseline_sha256'],hashlib.sha256(baseline).hexdigest())
        self.assertTrue(receipt['cleanup_only']);self.assertTrue(receipt['cleanup_baseline_complete'])
        self.assertGreater(receipt['cleanup_file_count'],0);self.assertGreater(receipt['cleanup_source_file_bytes'],0)
        for key in ('complete','execution_allowed','drop_ready','inventory_complete','original_content_verified','purge_executed'):
            self.assertIs(receipt[key],False)
        self.assertTrue(all(v is False for v in receipt['capabilities'].values()))
        self.assertNotIn('SYNTHETIC_BODY',json.dumps(receipt));self.assertNotIn(str(self.root),json.dumps(receipt))
        self.assertNotIn('files',receipt);self.assertNotIn('directory_identity',receipt)

    def test_real_main_rejects_wrong_cleanup_field_type_and_false_content_claim(self):
        execute=tool.execute;transport=tool.transport()
        for key,value in (('cleanup_file_count',True),('original_content_verified',True)):
            with self.subTest(key=key):
                def changed(args):
                    receipt=execute(args);receipt[key]=value;return receipt
                out,err=io.StringIO(),io.StringIO()
                with mock.patch.object(tool,'capture_fixed',return_value=(0,b'')), \
                     mock.patch.object(tool,'execute',side_effect=changed), \
                     contextlib.redirect_stdout(out),contextlib.redirect_stderr(err):
                    self.assertEqual(tool.main(self.main_arguments()),42)
                if key=='cleanup_file_count':
                    self.assertEqual(out.getvalue(),'');self.assertIn('receipt_transport_failed',err.getvalue())
                else:
                    receipt=json.loads(transport.decode_armored_receipt(out.getvalue().strip()))
                    self.assertIs(receipt['inventory_complete'],False);self.assertIs(receipt['drop_ready'],False)
                (self.directory/tool.FAILED_INVENTORY_BASELINE).unlink()

    def test_unknown_member_missing_metadata_and_wrong_checkpoint_rejected(self):
        unknown=self.output/'foreign.json';self.write(unknown,{});
        with self.assertRaisesRegex(tool.Blocked,'failed_inventory_unknown_or_missing_member'):self.execute()
        unknown.unlink();self.write(self.output/'mongodb-metadata.private.json',{})
        with self.assertRaisesRegex(tool.Blocked,'failed_inventory_unknown_or_missing_member'):self.execute()
        (self.output/'mongodb-metadata.private.json').unlink()
        name='mongodb-domain_event_outbox-pass-1-page-000001.checkpoint.json'
        value=json.loads((self.output/name).read_bytes());value['source_sha']='c'*40;self.write(self.output/name,value)
        with self.assertRaisesRegex(tool.Blocked,'failed_inventory_checkpoint_mismatch'):self.execute()
        self.assertFalse((self.directory/tool.FAILED_INVENTORY_BASELINE).exists())

    def test_active_or_unknown_container_never_mints_baseline(self):
        for result in ((0,b'c'*64),(1,b'')):
            with self.subTest(result=result),mock.patch.object(tool,'capture_fixed',return_value=result),self.assertRaisesRegex(tool.Blocked,'failed_inventory_producer_not_absent'):
                tool.execute(self.args)
        self.assertFalse((self.directory/tool.FAILED_INVENTORY_BASELINE).exists())

    def test_symlink_hardlink_fifo_and_changed_source_actual_fd_rejected(self):
        path=self.output/next(iter(tool.SOURCE_FILENAMES.values()));raw=path.read_bytes();path.unlink()
        peer=self.directory/'peer';peer.write_bytes(raw);peer.chmod(0o600)
        path.symlink_to(peer)
        with self.assertRaises(tool.Blocked):self.execute()
        path.unlink();os.link(peer,path)
        with self.assertRaises(tool.Blocked):self.execute()
        path.unlink();os.mkfifo(path,0o600)
        with self.assertRaisesRegex(tool.Blocked,'failed_inventory_file_not_private'):self.execute()
        path.unlink();path.write_bytes(raw);path.chmod(0o600)
        read=tool.os.read;mutated=False
        def changing(fd,size):
            nonlocal mutated
            chunk=read(fd,size)
            if os.fstat(fd).st_ino==path.stat().st_ino and not mutated:
                mutated=True;path.write_bytes(raw+b'changed')
            return chunk
        with mock.patch.object(tool.os,'read',side_effect=changing),self.assertRaisesRegex(tool.Blocked,'failed_inventory_file_changed'):self.execute()

    def test_cleanup_scope_rejects_unapproved_limits_or_other_run(self):
        unapproved=copy.deepcopy(self.request);unapproved['limits']['page_size']=10000
        with self.assertRaisesRegex(tool.Blocked,'inventory_request_limits_invalid'):
            tool.validate_v2_request(unapproved,tool.FAILED_INVENTORY_OPERATION,self.reference['source_sha'],boundary=False)
        self.approval['inventory_report']['run_id']='38025045552-1';self.approve()
        with self.assertRaisesRegex(tool.Blocked,'failed_inventory_cleanup_approval_invalid'):self.execute()

    def test_real_action_validator_accepts_only_exact_failed_cleanup_tuple(self):
        value=dict(self.approval,inventory_report=self.actual_reference.copy())
        script=textwrap.dedent(base.DIAGNOSTIC_WORKFLOW.read_text().split('          script: |\n',1)[1].split('      - name:',1)[0])
        for mutation in (None,'run_id','request_sha256','source_sha','sha256'):
            changed=copy.deepcopy(value)
            if mutation:changed['inventory_report'][mutation]='999-1' if mutation=='run_id' else '0'*(40 if mutation=='source_sha' else 64)
            raw=tool.canonical_bytes(changed)
            supplied={'operation':'prepare','database':'mysql-and-mongodb','approved_source_sha':base.DIAGNOSTIC_SOURCE,
                'operation_id':tool.FAILED_INVENTORY_OPERATION,'prepare_mode':'report-diagnostic',
                'bootstrap_approval_json':raw[:-1].decode(),'bootstrap_approval_sha256':hashlib.sha256(raw).hexdigest()}
            context={'payload':{'inputs':supplied},'ref':'refs/heads/main','sha':base.DIAGNOSTIC_SOURCE,'runId':703,'runAttempt':1,'repo':{}}
            program=('const script='+json.dumps(script)+';const context='+json.dumps(context)+';const current='+json.dumps(base.DIAGNOSTIC_SOURCE)
                +';const github={rest:{repos:{getCommit:async()=>({data:{sha:current}})}}};'
                +"new (Object.getPrototypeOf(async function(){}).constructor)('context','github',script)(context,github).catch(()=>{process.exitCode=1;});")
            result=subprocess.run(['node','-e',program],capture_output=True,env=dict(os.environ,GITHUB_RUN_ATTEMPT='1'),timeout=5)
            self.assertEqual(result.returncode,0 if mutation is None else 1,result.stderr.decode())


if __name__=='__main__':
    unittest.main()

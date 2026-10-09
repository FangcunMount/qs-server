#!/usr/bin/env python3
"""Offline transport boundaries only; never invokes sudo/Docker/database."""
import argparse
import ast
import copy

import hashlib

import io

import shutil

import textwrap

import tempfile

import contextlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import stat
import sys
import unittest
from types import SimpleNamespace
from unittest import mock

spec=importlib.util.spec_from_file_location('root_prepare_tool',Path(__file__).with_name('compatibility-retirement.py'))
tool=importlib.util.module_from_spec(spec);spec.loader.exec_module(tool)

class RootPrepareOnceTransport(unittest.TestCase):
    def args(self):
        return argparse.Namespace(operation='prepare',prepare_mode='lifecycle',operation_id='123-1',run_id='456-1',actual_source_sha='a'*40,lifecycle_request_hash='b'*64,manifest_hash='c'*64)
    def test_same_existing_bindings_bounded_private_credentials_and_fixed_root_once(self):
        args=self.args();marker='PRIVATE_CREDENTIAL_NEVER_IN_ARGV'
        with mock.patch.dict(os.environ,{'RETIREMENT_PACKAGE_SHA256':'d'*64,'MYSQL_PASSWORD':marker},clear=True),mock.patch.object(tool.os,'getuid',return_value=501),mock.patch.object(tool.os,'geteuid',return_value=501),mock.patch.object(tool.subprocess,'run',return_value=subprocess.CompletedProcess([],1,b'fixed-failure')) as run:
            code,raw=tool.root_once_lifecycle_prepare(args)
        self.assertEqual((code,raw),(1,b'fixed-failure'));command=run.call_args.args[0]
        self.assertEqual(command[:5],['sudo','-n','python3','-I','-c'])
        self.assertEqual(command[6:],[args.operation_id,args.run_id,args.actual_source_sha,args.lifecycle_request_hash,'d'*64,args.manifest_hash,'sudo-user'])
        self.assertNotIn(marker,str(command));packet=json.loads(run.call_args.kwargs['input']);self.assertEqual(packet['MYSQL_PASSWORD'],marker)
        self.assertEqual(run.call_args.kwargs['stderr'],subprocess.DEVNULL)
        self.assertEqual(run.call_args.kwargs['timeout'],91*60)
    def test_real_root_uses_same_once_helper_with_clean_environment_and_private_pipe(self):
        args=self.args();marker='PRIVATE_ROOT_SECRET_NEVER_IN_ARGV'
        with mock.patch.dict(os.environ,{'RETIREMENT_PACKAGE_SHA256':'d'*64,'MYSQL_PASSWORD':marker,'SUDO_UID':'999','LD_PRELOAD':'untrusted','PYTHONPATH':'untrusted'},clear=True),mock.patch.object(tool.os,'getuid',return_value=0),mock.patch.object(tool.os,'geteuid',return_value=0),mock.patch.object(tool.subprocess,'run',return_value=subprocess.CompletedProcess([],1,b'fixed-failure')) as run:
            tool.root_once_lifecycle_prepare(args)
        command=run.call_args.args[0]
        self.assertEqual(command[:3],['/usr/bin/python3','-I','-c'])
        self.assertEqual(command[3],tool.ROOT_PREPARE_ONCE)
        self.assertEqual(command[4:],[args.operation_id,args.run_id,args.actual_source_sha,args.lifecycle_request_hash,'d'*64,args.manifest_hash,'root-direct'])
        self.assertEqual(run.call_args.kwargs['env'],{'PATH':'/usr/bin:/bin'})
        self.assertNotIn(marker,str(command));self.assertEqual(json.loads(run.call_args.kwargs['input'])['MYSQL_PASSWORD'],marker)
        self.assertNotIn('SUDO_UID',json.loads(run.call_args.kwargs['input']))
    def test_mixed_privilege_identity_denies_before_helper(self):
        for uid,euid in ((501,0),(0,501)):
            with mock.patch.dict(os.environ,{'RETIREMENT_PACKAGE_SHA256':'d'*64},clear=True),mock.patch.object(tool.os,'getuid',return_value=uid),mock.patch.object(tool.os,'geteuid',return_value=euid),mock.patch.object(tool.subprocess,'run') as run,self.assertRaises(tool.Blocked):
                tool.root_once_lifecycle_prepare(self.args())
            run.assert_not_called()
    def test_helper_identity_preserves_sudo_and_rejects_inherited_root_uid(self):
        tree=ast.parse(tool.ROOT_PREPARE_ONCE)
        branch=next(node for node in ast.walk(tree) if isinstance(node,ast.If) and ast.unparse(node.test)=="source_channel == 'sudo-user'")
        code=compile(ast.Module(body=[branch],type_ignores=[]),'fixed-helper-identity','exec')
        def stop():raise ValueError('rejected')
        with mock.patch.object(os,'getuid',return_value=0):
            clean={'source_channel':'root-direct','os':os,'stop':stop}
            with mock.patch.dict(os.environ,{},clear=True):exec(code,clean)
            self.assertEqual(clean['source_uid'],0)
            with mock.patch.dict(os.environ,{'SUDO_UID':'999'},clear=True),self.assertRaises(ValueError):exec(code,{'source_channel':'root-direct','os':os,'stop':stop})
            sudo={'source_channel':'sudo-user','os':os,'stop':stop}
            with mock.patch.dict(os.environ,{'SUDO_UID':'501'},clear=True):exec(code,sudo)
            self.assertEqual(sudo['source_uid'],501)
            for inherited in ('0','-1'):
                with mock.patch.dict(os.environ,{'SUDO_UID':inherited},clear=True),self.assertRaises(ValueError):exec(code,{'source_channel':'sudo-user','os':os,'stop':stop})
    def test_both_live_privilege_branches_route_once_and_never_call_unstaged_mode(self):
        for uid in (0,501):
            args=self.args();args.inventory_binary='/tool/private-native'
            request={'format_version':1,'kind':'compatibility_retirement_lifecycle_request','tool_source_sha':args.actual_source_sha,'original_source_sha':'e'*40,
                     'operation_id':args.operation_id,'actual_run_id':args.run_id,'manifest_sha256':args.manifest_hash,'archive_directory':'/archive',
                     'window_directory':'/window','journal_directory':'/journal','archive_approval':{},'recovery':{}}
            metadata=SimpleNamespace(st_mode=stat.S_IFREG|0o700,st_nlink=1,st_uid=uid)
            with mock.patch.object(tool.os,'getuid',return_value=uid),mock.patch.object(tool.os,'geteuid',return_value=uid),mock.patch.object(tool,'read_private',side_effect=[(request,'b'*64),({},'c'*64)]),mock.patch.object(tool,'validate_manifest'),mock.patch.object(Path,'lstat',return_value=metadata),mock.patch.object(tool,'capture_fixed',return_value=(0,b'a'*40+b'\n')) as capture,mock.patch.object(tool,'locked_operation',return_value=contextlib.nullcontext()),mock.patch.object(tool,'root_once_lifecycle_prepare',return_value=(1,b'fixed-failure')) as once,mock.patch.object(tool,'decode',side_effect=tool.Blocked('test_stop_after_actual_caller_route')),self.assertRaises(tool.Blocked):
                tool.live_lifecycle(args,Path('/approved-operation'))
            once.assert_called_once_with(args)
            capture.assert_called_once_with(['/tool/private-native','--source-sha'],timeout=5,maximum=128)
    def test_apply_cannot_borrow_pre_window_once_authority(self):
        args=self.args();args.operation='apply'
        with mock.patch.object(tool.subprocess,'run') as run,self.assertRaises(tool.Blocked):tool.root_once_lifecycle_prepare(args)
        run.assert_not_called()
    def test_invalid_package_hash_and_oversized_packet_refuse_before_sudo(self):
        for environment in ({'RETIREMENT_PACKAGE_SHA256':'tag'},{'RETIREMENT_PACKAGE_SHA256':'d'*64,'MYSQL_PASSWORD':'x'*40000}):
            with mock.patch.dict(os.environ,environment,clear=True),mock.patch.object(tool.subprocess,'run') as run,self.assertRaises(tool.Blocked):tool.root_once_lifecycle_prepare(self.args())
            run.assert_not_called()
    def test_a_effectful_modes_refuse_before_private_reads_or_sudo(self):
        for operation in ('apply','verify','recover','purge'):
            args=self.args();args.operation=operation
            with mock.patch.object(tool,'read_private') as read,mock.patch.object(tool.subprocess,'run') as run,self.assertRaises(tool.Blocked) as rejected:
                tool.live_lifecycle(args,Path('/nonexistent'))
            self.assertEqual(str(rejected.exception),'lifecycle_actual_host_adapters_missing')
            read.assert_not_called();run.assert_not_called()
    def test_a_prepare_resume_fields_refuse_before_native_bootstrap(self):
        for extra in ('resume','resume_kind','migration_intent_sha256','drop_ready'):
            args=self.args()
            value={key:'' for key in ('format_version','kind','tool_source_sha','original_source_sha','operation_id','actual_run_id','manifest_sha256','archive_directory','window_directory','journal_directory','archive_approval','recovery')}
            value[extra]=None
            with mock.patch.object(tool,'read_private',return_value=(value,'b'*64)),mock.patch.object(tool.subprocess,'run') as run,self.assertRaises(tool.Blocked):
                tool.live_lifecycle(args,Path('/nonexistent'))
            run.assert_not_called()
    def test_tool_intent_is_durable_before_native_copy(self):
        program=tool.ROOT_PREPARE_ONCE
        intent=program.index("batch/'tool.intent.private.json'")
        copy=program.index('fd=os.open(native,')
        self.assertLess(intent,copy)
        between=program[intent:copy]
        self.assertIn('os.fsync(f.fileno())',between)
        self.assertIn('os.fsync(fd);os.close(fd)',between)
    def test_inline_root_bootstrap_is_fixed_parseable_and_denies_nonroot(self):
        ast.parse(tool.ROOT_PREPARE_ONCE)
        if os.getuid()==0:return # no privileged execution in this offline suite
        result=subprocess.run([sys.executable,'-I','-c',tool.ROOT_PREPARE_ONCE],input=b'{}',stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=5,check=False)
        self.assertEqual(result.returncode,1);self.assertEqual(result.stderr,b'');receipt=json.loads(result.stdout)
        self.assertFalse(receipt['complete']);self.assertFalse(receipt['drop_ready']);self.assertFalse(receipt['execution_allowed'])

class PrepareFactsBoundaries(unittest.TestCase):
    def args(self):
        value = {'format_version':1,'kind':'readonly_prepare_facts_observation_descriptor','prepare_mode':'prepare-facts',
            'source_sha':'a'*40,'operation_id':'123-1','target_hash':tool.TARGET_HASH,'database_scope':'mysql-and-mongodb',
            'inventory_report':{'operation_id':'123-1','run_id':'456-1','source_sha':'b'*40,'sha256':'c'*64,'request_sha256':'d'*64},
            'restore_engines':{'mysql_image_id':'sha256:'+'1'*64,'mongodb_image_id':'sha256:'+'2'*64,'architecture':'amd64'},
            'archive_directory':'/opt/backups/qs-server/compatibility-retirement/123-1/temporary-archive'}
        args=argparse.Namespace(operation='prepare',prepare_mode='prepare-facts',operation_id='123-1',run_id='789-1',actual_source_sha='a'*40,
            approved_source_sha='a'*40,manifest_hash='',identity_request_hash='',inventory_request_hash='',lifecycle_request_hash='',root='/opt/backups/qs-server/compatibility-retirement')
        self.set_descriptor(args,value)
        return args,value
    def set_descriptor(self,args,value):
        raw=tool.canonical_bytes(value);args.bootstrap_approval_json=raw[:-1].decode('ascii');args.bootstrap_approval_hash=hashlib.sha256(raw).hexdigest()
    def receipt(self,args,request,request_hash):
        return {'format_version':1,'kind':'readonly_prepare_facts_observation','operation':'prepare','prepare_mode':'prepare-facts','source_sha':args.actual_source_sha,
            'operation_id':args.operation_id,'run_id':args.run_id,'request_sha256':request_hash,'observation_approval_sha256':args.bootstrap_approval_hash,
            'target_hash':tool.TARGET_HASH,'complete':False,'prepare_facts_observation_complete':True,'diagnostic_only':True,'execution_allowed':False,'drop_ready':False,
            'observed_inventory_producer':request['inventory_report'],'prepare_source_files':[{'name':name,'sha256':request['inventory_report']['sha256'] if i==0 else 'e'*64,'bytes':0} for i,name in enumerate(tool.PREPARE_SOURCE_NAMES)],
            'observed_ordered_mongo_schema_sha256':'f'*64,'observed_restore_engines':request['restore_engines'],
            'observed_filesystems':[{'scope':scope,'path_sha256':'0'*64,'total_bytes':100,'available_bytes':50,'free_bytes':60} for scope in ('source','staging','archive','docker')],
            'observed_socket_kind':'fixed_root_owned_unix_docker','observation_elapsed_millis':123,'error_category':'none','prepare_facts_private_observation_sha256':'9'*64}
    def test_separate_original_producer_request_does_not_approve_ordered_facts(self):
        args,value=self.args();request=tool.prepare_facts_request(args)
        self.assertEqual(request['source_sha'],'a'*40);self.assertEqual(request['inventory_report']['source_sha'],'b'*40)
        self.assertEqual(request['observation_approval_sha256'],args.bootstrap_approval_hash)
        self.assertEqual(request['actual_run_id'],args.run_id)
        self.assertNotIn('ordered_mongo_schema_sha256',request);self.assertNotIn('manifest_sha256',request)
        for key in ('ordered_mongo_schema_sha256','source_file_sha256','drop_ready','execution_allowed','capacity_ready'):
            invalid=copy.deepcopy(value);invalid[key]=True;self.set_descriptor(args,invalid)
            with self.assertRaises(tool.Blocked):tool.prepare_facts_request(args)
    def test_tags_same_images_wrong_origin_and_source_path_refuse_without_native(self):
        args,value=self.args()
        mutations=(lambda v:v['restore_engines'].update(mysql_image_id='mysql:8.0'),
            lambda v:v['restore_engines'].update(mysql_image_id=v['restore_engines']['mongodb_image_id']),
            lambda v:v['inventory_report'].update(run_id=args.run_id),
            lambda v:v['inventory_report'].update(operation_id='999-1'),
            lambda v:v.update(archive_directory='/opt/backups/qs-server/compatibility-retirement/123-1/inventory-456-1/nested'))
        for mutate in mutations:
            invalid=copy.deepcopy(value);mutate(invalid);self.set_descriptor(args,invalid)
            with mock.patch.object(tool.subprocess,'run') as native,self.assertRaises(tool.Blocked):tool.prepare_facts_request(args)
            native.assert_not_called()
    def test_existing_once_channel_uses_new_fixed_mode_without_manifest_or_fake_secret(self):
        args,_=self.args();args.prepare_facts_request_hash='7'*64
        with mock.patch.dict(os.environ,{'RETIREMENT_PACKAGE_SHA256':'8'*64},clear=True),mock.patch.object(tool.os,'getuid',return_value=501),mock.patch.object(tool.os,'geteuid',return_value=501),mock.patch.object(tool.subprocess,'run',return_value=subprocess.CompletedProcess([],1,b'fixed')) as run:
            tool.root_once_lifecycle_prepare(args)
        self.assertEqual(run.call_args.args[0][6:],[args.operation_id,args.run_id,args.actual_source_sha,'7'*64,'8'*64,'','sudo-user','prepare-facts'])
        self.assertEqual(json.loads(run.call_args.kwargs['input'])['MONGODB_PASSWORD'],'')
    def test_actual_receipt_requires_seven_files_real_ordered_hash_and_all_false_capabilities(self):
        args,_=self.args();request=tool.prepare_facts_request(args);request_hash='7'*64
        original=self.receipt(args,request,request_hash)
        result=tool.validate_prepare_facts_result(copy.deepcopy(original),args,request,request_hash,0)
        self.assertFalse(result['complete']);self.assertFalse(result['drop_ready']);self.assertTrue(result['prepare_facts_observation_complete'])
        self.assertTrue(all(v is False for v in result['capabilities'].values()))
        self.assertEqual(result['observed_restore_engines']['mysql_image_id_sha256'],'1'*64)
        mutations=(lambda r:r.update(drop_ready=True),lambda r:r.update(complete=True),lambda r:r.update(observed_ordered_mongo_schema_sha256=''),
            lambda r:r['prepare_source_files'].pop(),lambda r:r['observed_inventory_producer'].update(source_sha=args.actual_source_sha),
            lambda r:r.update(prepare_facts_private_observation_sha256=''),lambda r:r.update(observed_filesystems=[]))
        for mutate in mutations:
            invalid=copy.deepcopy(original);mutate(invalid)
            with self.assertRaises(tool.Blocked):tool.validate_prepare_facts_result(invalid,args,request,request_hash,0)
    def test_execute_facts_refuses_effectful_or_mixed_ten_input_classes(self):
        for key,value in (('operation','apply'),('manifest_hash','1'*64),('inventory_request_hash','2'*64),('identity_request_hash','3'*64)):
            args,_=self.args();setattr(args,key,value)
            with mock.patch.object(tool,'live_prepare_facts') as native,self.assertRaises(tool.Blocked):tool.execute(args)
            native.assert_not_called()
    def test_existing_armored_transport_emits_bodyfree_facts_without_authority(self):
        args,_=self.args();request=tool.prepare_facts_request(args)
        result=tool.validate_prepare_facts_result(self.receipt(args,request,'7'*64),args,request,'7'*64,0)
        output=io.StringIO()
        argv=['--operation','prepare','--operation-id',args.operation_id,'--approved-source-sha',args.actual_source_sha,
            '--actual-source-sha',args.actual_source_sha,'--run-id',args.run_id,'--prepare-mode','prepare-facts']
        with mock.patch.object(tool,'execute',return_value=result),mock.patch.dict(os.environ,{},clear=True),contextlib.redirect_stdout(output):
            code=tool.main(argv)
        self.assertEqual(code,0)
        decoded=json.loads(tool.transport().decode_armored_receipt(output.getvalue().strip()))
        self.assertEqual(decoded['observed_inventory_producer']['source_sha'],'b'*40)
        self.assertEqual(decoded['observed_ordered_mongo_schema_sha256'],'f'*64)
        self.assertEqual(decoded['prepare_source_files'][0]['name'],'inventory_private_json')
        self.assertFalse(decoded['complete']);self.assertFalse(decoded['execution_allowed']);self.assertFalse(decoded['drop_ready'])
    def test_actual_action_ten_input_validator_accepts_only_observation_descriptor(self):
        node=shutil.which('node')
        if node is None:self.skipTest('node required for actual Action validator')
        workflow=Path(__file__).parents[2]/'.github/workflows/compatibility-retirement.yml'
        block=workflow.read_text().split('      - name: Reject unknown inputs and stale source before production credentials\n',1)[1].split('      - name: Require successful final source CI\n',1)[0]
        script=textwrap.dedent(block.split('          script: |\n',1)[1])
        args,value=self.args()
        input={'operation':'prepare','database':'mysql-and-mongodb','approved_source_sha':args.actual_source_sha,'operation_id':args.operation_id,
            'manifest_sha256':'','inventory_request_sha256':'','prepare_mode':'prepare-facts','identity_request_sha256':'',
            'bootstrap_approval_json':args.bootstrap_approval_json,'bootstrap_approval_sha256':args.bootstrap_approval_hash}
        cases=[{'input':input,'accepted':True}]
        for field in ('drop_ready','ordered_mongo_schema_sha256'):
            changed=copy.deepcopy(value);changed[field]=True;self.set_descriptor(args,changed)
            invalid=copy.deepcopy(input);invalid.update(bootstrap_approval_json=args.bootstrap_approval_json,bootstrap_approval_sha256=args.bootstrap_approval_hash)
            cases.append({'input':invalid,'accepted':False})
        for key,content in (('operation','apply'),('manifest_sha256','4'*64),('eleventh_input','unexpected')):
            invalid=copy.deepcopy(input);invalid[key]=content;cases.append({'input':invalid,'accepted':False})
        program="""const fs=require('fs'); const x=JSON.parse(fs.readFileSync(0,'utf8'));
const AsyncFunction=Object.getPrototypeOf(async function(){}).constructor;
(async()=>{for(const c of x.cases){let accepted=false;try{await new AsyncFunction('context','github','require','process',x.script)(
{payload:{inputs:c.input},sha:'a'.repeat(40),ref:'refs/heads/main',runId:789,repo:{}},
{rest:{repos:{getCommit:async()=>({data:{sha:'a'.repeat(40)}})}}},require,{env:{GITHUB_RUN_ATTEMPT:'1'}});accepted=true;}catch{}
if(accepted!==c.accepted)throw new Error('offline_action_validation_mismatch');}process.stdout.write(JSON.stringify({cases:x.cases.length,complete:true})+'\\n');})().catch(()=>process.exit(1));"""
        done=subprocess.run([node,'-e',program],input=json.dumps({'script':script,'cases':cases}).encode(),stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=10,check=False)
        self.assertEqual(done.returncode,0,done.stderr.decode())
        self.assertEqual(json.loads(done.stdout),{'cases':6,'complete':True})
    def test_failed_observation_then_new_run_keeps_old_request_and_same_inventory(self):
        args,_=self.args()
        with tempfile.TemporaryDirectory(prefix='prepare-facts-offline-') as temporary:
            directory=Path(temporary).resolve();directory.chmod(0o700)
            # Only the host/DB caller is mocked. These requests, the operation
            # lock and durable exclusive publication use actual local files.
            attempts=[]
            def native(current):
                path=directory/('prepare-facts-request-'+current.run_id+'.json')
                raw=path.read_bytes();request=json.loads(raw)
                self.assertEqual(hashlib.sha256(raw).hexdigest(),current.prepare_facts_request_hash)
                self.assertEqual(request['actual_run_id'],current.run_id)
                result=self.receipt(current,request,current.prepare_facts_request_hash)
                attempts.append(request)
                if len(attempts)==1:
                    result.update(prepare_facts_observation_complete=False,error_category='prepare_facts_ordered_schema_read_failed',
                        prepare_source_files=[],observed_filesystems=[],observed_ordered_mongo_schema_sha256='',observed_socket_kind='')
                    result.pop('observed_restore_engines');result.pop('prepare_facts_private_observation_sha256')
                    return 1,tool.canonical_bytes(result)
                return 0,tool.canonical_bytes(result)
            with mock.patch.object(tool,'operation_directory',return_value=directory),mock.patch.object(tool,'root_once_lifecycle_prepare',side_effect=native) as caller:
                first=tool.live_prepare_facts(args)
                self.assertFalse(first['prepare_facts_observation_complete'])
                old_path=directory/'prepare-facts-request-789-1.json';old_bytes=old_path.read_bytes()
                args.run_id='790-1'
                second=tool.live_prepare_facts(args)
                self.assertTrue(second['prepare_facts_observation_complete'])
                self.assertEqual(old_path.read_bytes(),old_bytes)
                self.assertTrue((directory/'prepare-facts-request-790-1.json').is_file())
                self.assertEqual(attempts[0]['inventory_report'],attempts[1]['inventory_report'])
                self.assertEqual(attempts[0]['operation_id'],attempts[1]['operation_id'])
                self.assertFalse(any(value is True for value in second['capabilities'].values()))
                self.assertEqual(caller.call_count,2)
                new_bytes=(directory/'prepare-facts-request-790-1.json').read_bytes()
                with self.assertRaises(tool.Blocked):tool.live_prepare_facts(args)
                self.assertEqual(caller.call_count,2)
                self.assertEqual((directory/'prepare-facts-request-790-1.json').read_bytes(),new_bytes)
                self.assertEqual(old_path.read_bytes(),old_bytes)
    def test_fixed_root_helper_intent_and_exec_use_exact_actual_run_path(self):
        tree=ast.parse(tool.ROOT_PREPARE_ONCE)
        assignments=[node for node in ast.walk(tree) if isinstance(node,ast.Assign)]
        names={node.targets[0].id:node for node in assignments if len(node.targets)==1 and isinstance(node.targets[0],ast.Name)}
        for stage,expected in (('prepare-facts','prepare-facts-request-790-1.json'),('lifecycle','lifecycle-request.json')):
            values={'stage':stage,'operation':'123-1','run':'790-1'}
            for key in ('request_name','request_path'):
                exec(compile(ast.Module(body=[names[key]],type_ignores=[]),'fixed-root-path','exec'),values)
            self.assertEqual(values['request_path'],'/opt/backups/qs-server/compatibility-retirement/123-1/'+expected)
        registry=names['registry'].value
        self.assertIsInstance(registry,ast.Dict)
        keys=[key.value for key in registry.keys]
        self.assertEqual(ast.unparse(registry.values[keys.index('request_path')]),'request_path')
        self.assertEqual(ast.unparse(registry.values[keys.index('request_sha256')]),'request_hash')
        execution=next(node for node in ast.walk(tree) if isinstance(node,ast.Call) and isinstance(node.func,ast.Attribute) and node.func.attr=='execve')
        argv=execution.args[1].elts
        position=next(i for i,item in enumerate(argv) if isinstance(item,ast.Constant) and item.value=='--request')
        self.assertEqual(ast.unparse(argv[position+1]),'request_path')
    def test_actual_run_path_tokens_are_checked_before_any_private_publication(self):
        args,_=self.args();args.run_id='../790-1'
        with mock.patch.object(tool,'operation_directory') as directory,mock.patch.object(tool,'create_bootstrap_file') as publish,self.assertRaises(tool.Blocked):
            tool.live_prepare_facts(args)
        directory.assert_not_called();publish.assert_not_called()


if __name__=='__main__':unittest.main()

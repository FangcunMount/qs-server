#!/usr/bin/env python3
"""Offline transport boundaries only; never invokes sudo/Docker/database."""
import argparse
import ast
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

if __name__=='__main__':unittest.main()

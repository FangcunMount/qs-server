#!/usr/bin/env python3
"""Real private temporary-file effects; no production, Docker or network."""
import hashlib
import importlib.util
import os
import json
import subprocess
import sys
from pathlib import Path
import tempfile
import time
import unittest
from unittest.mock import patch

spec=importlib.util.spec_from_file_location('s0',Path(__file__).with_name('compatibility-s0-exact-exit.py'))
s0=importlib.util.module_from_spec(spec);spec.loader.exec_module(s0)


class ExactExit(unittest.TestCase):
    def setUp(self):
        self.temp=tempfile.TemporaryDirectory(prefix='qs-owned-s0-exit-')
        self.parent=Path(self.temp.name);self.parent.chmod(0o700)
        self.output=self.parent/'inventory-38025045551-1';self.output.mkdir(mode=0o700)
        self.files={'mysql-domain_event_outbox.source.ndjson':b'private fixture bytes\n',
                    'inventory.private.json':b'{"fixture":true}\n'}
        for name,body in self.files.items():
            p=self.output/name;p.write_bytes(body);p.chmod(0o600)
        st=self.output.stat()
        self.baseline={'directory_identity':[st.st_dev,st.st_ino,st.st_uid,0o700],
                       'files':{name:{'sha256':hashlib.sha256(body).hexdigest(),'stat':s0.stamp((self.output/name).stat())} for name,body in self.files.items()}}
        self.fd=os.open(self.parent,os.O_RDONLY|os.O_DIRECTORY)
        self.authority={'actual_run_id':'99999999999-1','baseline_sha256':'1'*64}
        self.absences=0

    def tearDown(self):
        os.close(self.fd)
        self.temp.cleanup()

    def absent(self,deadline):
        self.absences+=1

    def call(self):
        return s0.purge_open(self.fd,self.output.name,self.baseline,set(self.files),self.authority,self.absent,time.monotonic()+10)

    def reject(self,category='s0_predelete_refused'):
        with self.assertRaisesRegex(s0.Rejected,'^'+category+'$'):self.call()

    def test_actual_closed_files_and_exact_directory_exit(self):
        result=self.call()
        self.assertTrue(result['complete']);self.assertEqual(result['removed_file_count'],2)
        self.assertFalse(self.output.exists());self.assertEqual(self.absences,2)
        self.assertEqual(set(p.name for p in self.parent.iterdir()),{'s0-exit-99999999999-1.intent.json','s0-exit-99999999999-1.zero.json'})
        self.assertFalse(result['original_content_verified']);self.assertFalse(result['drop_authority'])

    def test_extra_file_blocks_all_effects(self):
        p=self.output/'unknown.json';p.write_bytes(b'x');p.chmod(0o600)
        self.reject();self.assertTrue(all((self.output/n).exists() for n in self.files))

    def test_content_change_same_length_rejected(self):
        name=next(iter(self.files));(self.output/name).write_bytes(b'x'*len(self.files[name]))
        self.reject();self.assertEqual(set(p.name for p in self.output.iterdir()),set(self.files))

    def test_inode_replacement_rejected(self):
        name=next(iter(self.files));p=self.output/name;p.unlink();p.write_bytes(self.files[name]);p.chmod(0o600)
        self.reject()

    def test_symlink_rejected(self):
        name=next(iter(self.files));p=self.output/name;p.unlink();p.symlink_to(self.parent/'missing')
        self.reject()

    def test_multiple_hard_links_rejected(self):
        name=next(iter(self.files));os.link(self.output/name,self.parent/'owned-copy')
        self.reject()

    def test_second_actual_absence_failure_prevents_intent_and_unlink(self):
        def check(deadline):
            self.absences+=1
            if self.absences==2:raise s0.Rejected('producer')
        self.absent=check;self.reject();self.assertEqual(set(self.parent.iterdir()),{self.output})

    def test_change_after_intent_blocks_remaining_unlink(self):
        original=s0.os.unlink;counter=0
        def unlink(name,*,dir_fd):
            nonlocal counter
            original(name,dir_fd=dir_fd);counter+=1
            if counter==1:
                remaining=next(n for n in self.files if (self.output/n).exists())
                (self.output/remaining).write_bytes(b'changed')
        with patch.object(s0.os,'unlink',unlink):self.reject('s0_physical_exit_unknown')
        self.assertEqual(len(list(self.output.iterdir())),1)

    def test_partial_unlink_unknown_no_automatic_resume(self):
        original=s0.os.unlink;counter=0
        def unlink(name,*,dir_fd):
            nonlocal counter
            counter+=1
            if counter==2:raise OSError('fixture')
            original(name,dir_fd=dir_fd)
        with patch.object(s0.os,'unlink',unlink):self.reject('s0_physical_exit_unknown')
        self.assertEqual(len(list(self.output.iterdir())),1)
        self.reject();self.assertEqual(len(list(self.output.iterdir())),1)

    def test_close_after_unlink_unknown(self):
        original=s0.close;raised=False
        def close(fd):
            nonlocal raised
            info=os.fstat(fd)
            original(fd)
            if not raised and info.st_nlink==0 and not os.path.isdir('/dev/fd/'+str(fd)):
                raised=True;raise OSError('fixture close unknown')
        with patch.object(s0,'close',close):self.reject('s0_physical_exit_unknown')
        self.assertTrue(raised)

    def test_fsync_after_unlink_unknown(self):
        original=s0.os.fsync
        def fsync(fd):
            if os.fstat(fd).st_ino==self.baseline['directory_identity'][1] and len(list(self.output.iterdir()))==1:raise OSError('fixture fsync unknown')
            original(fd)
        with patch.object(s0.os,'fsync',fsync):self.reject('s0_physical_exit_unknown')


    def test_real_armored_completed_owner_and_origin_binding(self):
        transport=s0.load(Path(__file__).parents[1]/'dbops/receipt-transport.py','s0_test_transport')
        a={'completion_run_id':'77777777777-1','completion_source_sha':'a'*40,'tool_source_sha':'b'*40,
           'completion_operation_id':'66666666666-1','workflow_scope_sha256':'c'*64,'native_stdout_sha256':'d'*64,
           'producer_workflow_id':123,'classifier_sha256':'e'*64}
        f={'protocol':'runner_platform_window_owner_v1','dispatcher_source_sha':a['completion_source_sha'],
           'tool_source_sha':a['tool_source_sha'],'operation_id':a['completion_operation_id'],
           'actual_run_id':a['completion_run_id'],'workflow_scope_sha256':a['workflow_scope_sha256'],
           'platform_installed':True,'platform_restored':True,'native_channel_terminal':True,
           'native_disposition':'native_completed','native_stdout_sha256':a['native_stdout_sha256'],
           'whole_writer_fence_proven':False,'drop_ready':False}
        schema={k:('bool' if type(v) is bool else 'sha40' if k.endswith('source_sha') else 'hash64' if k.endswith('sha256') else 'run_id' if k in ('operation_id','actual_run_id') else frozenset({v})) for k,v in f.items()}
        frame=transport.encode_armored_receipt(f,schema=schema)
        a['completion_footer_sha256']=s0.sha(s0.canonical(f))
        p={'id':38025045551,'run_attempt':1,'status':'completed','conclusion':'failure','head_sha':s0.OLD_SOURCE,
           'event':'workflow_dispatch','path':'.github/workflows/compatibility-retirement.yml','workflow_id':123}
        c=dict(p,id=77777777777,conclusion='success',head_sha=a['completion_source_sha'])
        proof={'producer':p,'completion':c,'workflow':{'id':123,'path':p['path'],'state':'disabled_manually'},
               'active_runs':[],'footer':'timestamp '+frame,'classifier_sha256':'e'*64}
        s0.validate_authority(a,proof,transport)
        for key,value in [('native_disposition','native_recovered'),('native_channel_terminal',False)]:
            altered=dict(f,**{key:value});changed=dict(proof,footer=transport.encode_armored_receipt(altered,schema={**schema,key:frozenset({value}) if type(value) is str else 'bool'}))
            with self.assertRaises(s0.Rejected):s0.validate_authority(a,changed,transport)
        for field,value in [('state','active'),('path','.github/workflows/other.yml')]:
            changed=dict(proof,workflow=dict(proof['workflow'],**{field:value}))
            with self.assertRaises(s0.Rejected):s0.validate_authority(a,changed,transport)
        with self.assertRaises(s0.Rejected):s0.validate_authority(a,dict(proof,active_runs=[1]),transport)
        with self.assertRaises(s0.Rejected):s0.validate_authority(a,dict(proof,completion=dict(c,head_sha='f'*40)),transport)

    def test_empty_real_approval_is_rejected_before_effect(self):
        with self.assertRaises(s0.Rejected):s0.approved(b'{}\n',s0.sha(b'{}\n'),'a'*40)
        self.assertEqual(len(list(self.output.iterdir())),2)

    def test_finite_packet_rejection_does_not_wait_for_control_eof(self):
        # The real caller keeps stdin open as its native cancellation channel.
        # Only the fixture child's UID probes are replaced; no remote call runs.
        program='import os\nos.getuid=lambda:0\nos.geteuid=lambda:0\n'+s0.BOOTSTRAP
        child=subprocess.Popen([sys.executable,'-I','-c',program],stdin=subprocess.PIPE,
                               stdout=subprocess.PIPE,stderr=subprocess.PIPE)
        try:
            child.stdin.write(b'{}\n');child.stdin.flush()
            self.assertEqual(child.wait(timeout=3),1)
            self.assertIn(b'unknown_or_refused',child.stdout.read())
        finally:
            if child.poll() is None:child.kill();child.wait(timeout=3)
            for stream in (child.stdin,child.stdout,child.stderr):stream.close()


if __name__=='__main__':unittest.main()

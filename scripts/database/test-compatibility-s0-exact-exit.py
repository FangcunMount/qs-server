#!/usr/bin/env python3
"""Real private temporary-file effects; no production, Docker or network."""
import base64
import hashlib
import importlib.util
import os
import json
import subprocess
import select
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
        a={'failed_inventory_operation_id':s0.OLD_OP,'completion_run_id':'77777777777-1','completion_source_sha':'a'*40,'tool_source_sha':'b'*40,
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
        child=subprocess.Popen([sys.executable,'-I','-c',program,'root-direct','0'],stdin=subprocess.PIPE,
                               stdout=subprocess.PIPE,stderr=subprocess.PIPE)
        try:
            child.stdin.write(b'{}\n');child.stdin.flush()
            self.assertEqual(child.wait(timeout=3),1)
            self.assertIn(b'unknown_or_refused',child.stdout.read())
        finally:
            if child.poll() is None:child.kill();child.wait(timeout=3)
            for stream in (child.stdin,child.stdout,child.stderr):stream.close()


class FixedSudoRoute(unittest.TestCase):
    """Actual local child/pipe/script effects, never production root authority."""
    def setUp(self):
        self.temp=tempfile.TemporaryDirectory(prefix='qs-owned-s0-root-route-')
        self.parent=Path(self.temp.name);self.parent.chmod(0o700)
        self.helper=s0.load(Path(__file__).with_name('compatibility-retirement.py'),'s0_local_sudo_helper')
        self.control,self.writer=os.pipe()
        self.packet={'approval':{},'proof':{},'actual_run_id':'99999999999-1','sources':{'fixture':'x'*100000}}
        self.calls=[];self.children=[];self.askpass=[]
        self.real_popen=subprocess.Popen
        self.real_mkdtemp=tempfile.mkdtemp

    def tearDown(self):
        for child in self.children:
            if child.poll() is None:child.kill();child.wait(timeout=3)
        os.close(self.control)
        if self.writer is not None:os.close(self.writer)
        self.temp.cleanup()

    def invoke(self,password,*,askpass=True,exit_code=0,empty=False,root=False,deny=False):
        expected=s0.sha(s0.canonical(self.packet))
        program=("import hashlib,os,select,stat,subprocess,sys\n"
                 "raw=sys.stdin.buffer.readline(2097153)\n"
                 "assert hashlib.sha256(raw).hexdigest()=="+repr(expected)+"\n"
                 "assert b'sudo_password' not in raw and b'fixture-sudo-password' not in raw\n"
                 "assert not select.select([sys.stdin.fileno()],[],[],0.05)[0]\n")
        if password and not root and askpass:
            program+=("path=os.environ['SUDO_ASKPASS']\n"
                      "assert stat.S_IMODE(os.stat(path).st_mode)==0o700\n"
                      "assert stat.S_IMODE(os.stat(os.path.dirname(path)).st_mode)==0o700\n"
                      "assert b'fixture-sudo-password' not in open(path,'rb').read()\n"
                      "p=subprocess.run([path],stdout=subprocess.PIPE,stderr=subprocess.DEVNULL,check=True)\n"
                      "assert p.stdout==b'fixture-sudo-password\\n'\n")
        if root or not password:
            program+="assert 'SUDO_PASSWORD' not in os.environ and 'SUDO_ASKPASS' not in os.environ\n"
        if not empty:program+="sys.stdout.write('fixture terminal\\n');sys.stdout.flush()\n"
        program+='raise SystemExit('+str(exit_code)+')\n'
        def popen(command,**kwargs):
            self.calls.append((command,dict(kwargs['env'])))
            if 'SUDO_ASKPASS' in kwargs['env']:self.askpass.append(Path(kwargs['env']['SUDO_ASKPASS']))
            if deny:raise OSError('fixture launcher denied')
            child=self.real_popen([sys.executable,'-I','-c',program],**kwargs)
            self.children.append(child);return child
        def directory(**kwargs):return self.real_mkdtemp(dir=self.parent,**kwargs)
        with patch.dict(os.environ,{'SUDO_PASSWORD':'dirty-inherited-value','SUDO_ASKPASS':'/unapproved/askpass'},clear=True), \
             patch.object(s0.subprocess,'Popen',popen),patch.object(self.helper.tempfile,'mkdtemp',directory):
            if root:
                with patch.object(s0.os,'getuid',return_value=0),patch.object(s0.os,'geteuid',return_value=0):
                    return s0.root_packet_once(self.packet,self.helper,self.control,password)
            return s0.root_packet_once(self.packet,self.helper,self.control,password)

    def assert_clean(self):
        self.assertTrue(all(not path.exists() and not path.parent.exists() for path in self.askpass))
        self.assertEqual(list(self.parent.iterdir()),[])
        self.assertEqual(len(self.calls),1)

    def test_actual_fixed_askpass_separate_large_stdin_and_terminal_cleanup(self):
        code,output=self.invoke('fixture-sudo-password')
        self.assertEqual((code,output),(0,b'fixture terminal\n'))
        self.assertEqual(self.calls[0][0][:4],['/usr/bin/sudo','-A','--','/usr/bin/python3'])
        self.assertNotIn('fixture-sudo-password',repr(self.calls[0][0]))
        self.assert_clean()

    def test_nopasswd_does_not_consume_original_native_stdin(self):
        self.assertEqual(self.invoke('fixture-sudo-password',askpass=False)[0],0)
        self.assert_clean()

    def test_absent_secret_retains_fixed_sudo_n(self):
        self.assertEqual(self.invoke('')[0],0)
        self.assertEqual(self.calls[0][0][:4],['/usr/bin/sudo','-n','--','/usr/bin/python3'])
        self.assertEqual(self.calls[0][1],{'PATH':'/usr/bin:/bin'})
        self.assert_clean()

    def test_root_direct_scrubs_inherited_secret_without_creating_askpass(self):
        self.assertEqual(self.invoke('fixture-sudo-password',root=True)[0],0)
        self.assertEqual(self.calls[0][0][:3],['/usr/bin/python3','-I','-c'])
        self.assertEqual(self.calls[0][1],{'PATH':'/usr/bin:/bin'})
        self.assert_clean()

    def test_actual_child_failure_is_not_native_success_and_cleans_owned_script(self):
        self.assertEqual(self.invoke('fixture-sudo-password',exit_code=1)[0],1)
        self.assert_clean()

    def test_askpass_launch_failure_cleans_original_private_script(self):
        with self.assertRaisesRegex(OSError,'fixture launcher denied'):self.invoke('fixture-sudo-password',deny=True)
        self.assert_clean()

    def test_original_caller_eof_is_unknown_without_retry(self):
        os.close(self.writer);self.writer=None
        with self.assertRaisesRegex(s0.Rejected,'s0_physical_exit_unknown'):self.invoke('fixture-sudo-password')
        self.assert_clean()

    def test_empty_child_output_cannot_be_completion(self):
        with self.assertRaisesRegex(s0.Rejected,'s0_physical_exit_unknown'):self.invoke('fixture-sudo-password',empty=True)
        self.assert_clean()

    def test_password_bounds_reject_before_child_or_private_script(self):
        for value in (None,False,'bad\nline','bad\rline','bad\x00line','é'*2049):
            with self.subTest(value_type=type(value).__name__):
                with self.assertRaises(s0.Rejected):s0.sudo_password(value)
        self.assertEqual(s0.sudo_password(''),'');self.assertEqual(self.calls,[])
        self.assertEqual(list(self.parent.iterdir()),[])

    def test_source_bound_supervisor_finite_lf_import_and_rejection(self):
        operator=b'def root_packet_once(packet,helper,control,password):\n return 0,b"fixture source-only terminal\\n"\n'
        helper=b'fixture_import_only=True\n'
        packet={'approval':{'operator_sha256':s0.sha(operator),'helper_sha256':s0.sha(helper)},
                'proof':{},'actual_run_id':'99999999999-1','sources':{'operator':base64.b64encode(operator).decode(),'helper':base64.b64encode(helper).decode()}}
        valid={'root_packet':packet,'sudo_password':'fixture-sudo-password'}
        for mutate,expected in ((lambda v:None,0),(lambda v:v['root_packet']['approval'].update(helper_sha256='0'*64),1),
                                (lambda v:v.update(sudo_password='bad\nline'),1),(lambda v:v.update(extra=True),1)):
            with self.subTest(expected=expected):
                envelope=json.loads(json.dumps(valid));mutate(envelope)
                child=self.real_popen([sys.executable,'-I','-c',s0.SUDO_BOOTSTRAP],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
                self.children.append(child)
                try:
                    child.stdin.write(s0.canonical(envelope));child.stdin.flush()  # original caller stays open
                    self.assertEqual(child.wait(timeout=3),expected)
                    output=child.stdout.read();self.assertNotIn(b'fixture-sudo-password',output)
                    self.assertNotIn(b'complete":true',output)
                finally:
                    if child.poll() is None:child.kill();child.wait(timeout=3)
                    for stream in (child.stdin,child.stdout,child.stderr):stream.close()

    def test_current_source_pure_import_reaches_only_fixed_launch_and_cleans_on_refusal(self):
        operator=Path(__file__).with_name('compatibility-s0-exact-exit.py').read_bytes()
        helper=Path(__file__).with_name('compatibility-retirement.py').read_bytes()
        packet={'approval':{'operator_sha256':s0.sha(operator),'helper_sha256':s0.sha(helper)},
                'proof':{},'actual_run_id':'99999999999-1','sources':{'operator':base64.b64encode(operator).decode(),'helper':base64.b64encode(helper).decode()}}
        # The real dynamic imports and private constructor run. The audit hook
        # rejects the only fixed launch before a sudo process can be created.
        program=("import os,sys,tempfile\n"
                 "base="+repr(str(self.parent))+"\n"
                 "original=tempfile.mkdtemp\n"
                 "tempfile.mkdtemp=lambda **kw:original(dir=base,**kw)\n"
                 "def audit(event,args):\n"
                 " if event=='subprocess.Popen':\n"
                 "  command=args[1]\n"
                 "  assert command[:4]==['/usr/bin/sudo','-A','--','/usr/bin/python3']\n"
                 "  assert len(command)==9 and command[4:6]==['-I','-c']\n"
                 "  assert 'fixture-sudo-password' not in repr(command)\n"
                 "  sys.stdout.write('fixture fixed launch refused\\n');sys.stdout.flush()\n"
                 "  raise RuntimeError('fixture pre-launch refusal')\n"
                 "sys.addaudithook(audit)\n"+s0.SUDO_BOOTSTRAP)
        child=self.real_popen([sys.executable,'-I','-c',program],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
        self.children.append(child)
        try:
            child.stdin.write(s0.canonical({'root_packet':packet,'sudo_password':'fixture-sudo-password'}));child.stdin.flush()
            self.assertEqual(child.wait(timeout=3),1)
            output=child.stdout.read()
            self.assertIn(b'fixture fixed launch refused',output)
            self.assertIn(b'unknown_or_refused',output)
            self.assertNotIn(b'fixture-sudo-password',output)
            self.assertEqual(child.stderr.read(),b'')
            self.assertEqual(list(self.parent.iterdir()),[])
        finally:
            if child.poll() is None:child.kill();child.wait(timeout=3)
            for stream in (child.stdin,child.stdout,child.stderr):stream.close()

    def test_original_root_bootstrap_scrubs_both_secret_names(self):
        operator=b'import os\ndef remote(*args):\n assert "SUDO_PASSWORD" not in os.environ and "SUDO_ASKPASS" not in os.environ\n return {"fixture":"root environment scrubbed"}\n'
        helper=b'fixture_import_only=True\n';transport=b'fixture_import_only=True\n'
        packet={'approval':{'operator_sha256':s0.sha(operator),'helper_sha256':s0.sha(helper)},'proof':{},'actual_run_id':'99999999999-1',
                'sources':{k:base64.b64encode(v).decode() for k,v in {'operator':operator,'helper':helper,'transport':transport}.items()}}
        # Only fixture transport digest and UID probes change; no native purge runs.
        program='import os\nos.getuid=lambda:0\nos.geteuid=lambda:0\n'+s0.BOOTSTRAP.replace(s0.TRANSPORT_SHA,s0.sha(transport))
        with patch.dict(os.environ,{'SUDO_PASSWORD':'fixture-sudo-password','SUDO_ASKPASS':'/unapproved'},clear=True):
            child=self.real_popen([sys.executable,'-I','-c',program,'root-direct','0'],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
            self.children.append(child)
            try:
                child.stdin.write(s0.canonical(packet));child.stdin.flush()
                self.assertEqual(child.wait(timeout=3),0)
                self.assertIn(b'root environment scrubbed',child.stdout.read())
                self.assertEqual(child.stderr.read(),b'')
            finally:
                if child.poll() is None:child.kill();child.wait(timeout=3)
                for stream in (child.stdin,child.stdout,child.stderr):stream.close()


class OriginalSourceOwner(unittest.TestCase):
    def setUp(self):
        # Use existing platform ancestry, resolving macOS symlink aliases.
        temporary_root=Path(tempfile.gettempdir()).resolve(strict=True)
        self.assertTrue(temporary_root.is_dir())
        with patch.object(tempfile,'gettempdir',return_value=str(temporary_root)):
            ExactExit.setUp(self)
    tearDown=ExactExit.tearDown
    absent=ExactExit.absent
    """Original local inode ownership; UID probes never grant real root."""
    def test_actual_original_owner_with_authenticated_root_origin(self):
        owner=os.getuid()
        with patch.object(s0.os,'getuid',return_value=0),patch.object(s0.os,'geteuid',return_value=0):
            result=s0.purge_open(self.fd,self.output.name,self.baseline,set(self.files),self.authority,self.absent,time.monotonic()+10,source_uid=owner)
        self.assertTrue(result['complete']);self.assertFalse(self.output.exists())

    def test_source_owner_mismatch_refuses_before_effect(self):
        owner=os.getuid()
        with patch.object(s0.os,'getuid',return_value=0),patch.object(s0.os,'geteuid',return_value=0):
            with self.assertRaises(s0.Rejected):s0.purge_open(self.fd,self.output.name,self.baseline,set(self.files),self.authority,self.absent,time.monotonic()+10,source_uid=owner+1)
        self.assertEqual(set(p.name for p in self.output.iterdir()),set(self.files))
        self.assertEqual(set(self.parent.iterdir()),{self.output})

    def test_original_private_reader_does_not_weaken_global_helper(self):
        helper=s0.load(Path(__file__).with_name('compatibility-retirement.py'),'s0_owner_read_helper')
        owner=os.getuid();name='inventory.private.json';expected=self.baseline['files'][name]['sha256']
        with patch.object(s0.os,'getuid',return_value=0),patch.object(s0.os,'geteuid',return_value=0):
            self.assertEqual(helper.failed_inventory_source_directory(self.output,owner),self.output)
            self.assertEqual(helper.failed_inventory_read_private(self.output,name,expected,owner),({'fixture':True},expected))
            with self.assertRaises(helper.Blocked):helper.private_directory(self.output)
            with self.assertRaises(helper.Blocked):helper.failed_inventory_source_directory(self.output,owner+1)
            with self.assertRaises(helper.Blocked):helper.failed_inventory_read_private(self.output,name,expected,owner+1)
            with self.assertRaises(helper.Blocked):helper.failed_inventory_read_private(self.output,'unapproved.json',expected,owner)
        self.assertEqual(self.output.stat().st_uid,owner)
        self.assertEqual(set(p.name for p in self.output.iterdir()),set(self.files))

    def test_nonroot_cannot_supply_another_owner_to_narrow_reader(self):
        helper=s0.load(Path(__file__).with_name('compatibility-retirement.py'),'s0_owner_nonroot_helper')
        with self.assertRaises(helper.Blocked):helper.failed_inventory_source_uid(os.getuid())
        self.assertEqual(helper.failed_inventory_source_uid(None),os.getuid())

    def test_actual_fixed_root_bootstrap_requires_sudo_origin_uid_and_not_json_owner(self):
        owner=os.getuid()
        operator=('import os\ndef remote(a,p,r,h,t,source_uid):\n assert source_uid=='+str(owner)+'\n return {"fixture":"sudo origin bound"}\n').encode()
        helper=b'fixture_import_only=True\n';transport=b'fixture_import_only=True\n'
        packet={'approval':{'operator_sha256':s0.sha(operator),'helper_sha256':s0.sha(helper)},'proof':{},'actual_run_id':'99999999999-1',
                'sources':{k:base64.b64encode(v).decode() for k,v in {'operator':operator,'helper':helper,'transport':transport}.items()}}
        program='import os\nos.getuid=lambda:0\nos.geteuid=lambda:0\n'+s0.BOOTSTRAP.replace(s0.TRANSPORT_SHA,s0.sha(transport))
        for channel,argument,sudo_uid,extra,expected in (('sudo-user',str(owner),str(owner),False,0),('sudo-user',str(owner+1),str(owner),False,1),
                ('sudo-user',str(owner),'',False,1),('sudo-user',str(owner),str(owner),True,1),('root-direct','0',str(owner),False,1)):
            with self.subTest(channel=channel,extra=extra,expected=expected):
                env={'PATH':'/usr/bin:/bin'}
                if sudo_uid:env['SUDO_UID']=sudo_uid
                body=dict(packet)
                if extra:body['source_uid']=owner
                child=subprocess.Popen([sys.executable,'-I','-c',program,channel,argument],env=env,stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
                try:
                    child.stdin.write(s0.canonical(body));child.stdin.flush()
                    self.assertEqual(child.wait(timeout=3),expected)
                    output=child.stdout.read();self.assertEqual(child.stderr.read(),b'')
                    if expected:self.assertNotIn(b'sudo origin bound',output)
                    else:self.assertIn(b'sudo origin bound',output)
                finally:
                    if child.poll() is None:child.kill();child.wait(timeout=3)
                    for stream in (child.stdin,child.stdout,child.stderr):stream.close()


class FixedFailedInventorySelection(unittest.TestCase):
    def approval(self,operation):
        a={key:'a'*64 for key in s0.REQUIRED}
        a.update(protocol='qs-s0-exact-physical-exit/v1',failed_inventory_operation_id=operation,
            source_sha='a'*40,baseline_observing_source_sha='b'*40,completion_source_sha='c'*40,
            tool_source_sha='d'*40,baseline_observing_run_id='77777777777-1',completion_run_id='88888888888-1',
            completion_operation_id='99999999999-1',completion_job_id=123,producer_workflow_id=456,
            pin_sha256=s0.PIN_SHA,total_seconds=600)
        return a

    def test_approval_only_two_exact_origins_without_default(self):
        for operation,origin in s0.FAILED_INVENTORY_ORIGINS.items():
            a=self.approval(operation);raw=s0.canonical(a)
            self.assertEqual(s0.approved(raw,s0.sha(raw),'a'*40),a)
            self.assertEqual(s0.failed_inventory_origin(a),(operation,*origin))
            a['completion_run_id']=origin[0];raw=s0.canonical(a)
            with self.assertRaises(s0.Rejected):s0.approved(raw,s0.sha(raw),'a'*40)
        for operation in ('38019009876-3','../38019009876-2',None,True):
            a=self.approval(operation);raw=s0.canonical(a)
            with self.assertRaises(s0.Rejected):s0.approved(raw,s0.sha(raw),'a'*40)
        a=self.approval(s0.OLD_OP);del a['failed_inventory_operation_id'];raw=s0.canonical(a)
        with self.assertRaises(s0.Rejected):s0.approved(raw,s0.sha(raw),'a'*40)

    def authority(self,operation):
        transport=s0.load(Path(__file__).parents[1]/'dbops/receipt-transport.py','s0_selection_transport')
        a=self.approval(operation)
        f={'protocol':'runner_platform_window_owner_v1','dispatcher_source_sha':a['completion_source_sha'],
           'tool_source_sha':a['tool_source_sha'],'operation_id':a['completion_operation_id'],
           'actual_run_id':a['completion_run_id'],'workflow_scope_sha256':a['workflow_scope_sha256'],
           'platform_installed':True,'platform_restored':True,'native_channel_terminal':True,
           'native_disposition':'native_completed','native_stdout_sha256':a['native_stdout_sha256'],
           'whole_writer_fence_proven':False,'drop_ready':False}
        schema={k:('bool' if type(v) is bool else 'sha40' if k.endswith('source_sha') else 'hash64' if k.endswith('sha256') else 'run_id' if k in ('operation_id','actual_run_id') else frozenset({v})) for k,v in f.items()}
        a['completion_footer_sha256']=s0.sha(s0.canonical(f))
        original_run,original_source=s0.FAILED_INVENTORY_ORIGINS[operation]
        p={'id':int(original_run.split('-')[0]),'run_attempt':1,'status':'completed','conclusion':'failure','head_sha':original_source,
           'event':'workflow_dispatch','path':'.github/workflows/compatibility-retirement.yml','workflow_id':456}
        c=dict(p,id=88888888888,conclusion='success',head_sha=a['completion_source_sha'])
        proof={'producer':p,'completion':c,'workflow':{'id':456,'path':p['path'],'state':'disabled_manually'},
            'active_runs':[],'footer':transport.encode_armored_receipt(f,schema=schema),'classifier_sha256':a['classifier_sha256']}
        return a,proof,transport

    def test_selected_second_producer_cannot_be_first_or_unknown(self):
        a,proof,transport=self.authority('38019009876-2');p=proof['producer']
        s0.validate_authority(a,proof,transport)
        for changes in ({'id':38025045551,'head_sha':s0.OLD_SOURCE},{'run_attempt':2},{'status':'in_progress'},{'head_sha':'0'*40}):
            with self.subTest(changes=changes),self.assertRaises(s0.Rejected):
                s0.validate_authority(a,dict(proof,producer=dict(p,**changes)),transport)
        with self.assertRaises(s0.Rejected):s0.validate_authority(dict(a,failed_inventory_operation_id=s0.OLD_OP),proof,transport)

    def test_real_existing_action_only_exact_selected_producer(self):
        import textwrap
        repo=Path(__file__).parents[2]
        script=textwrap.dedent((repo/'.github/workflows/compatibility-s0-exact-exit.yml').read_text().split('          script: |\n',1)[1].split('      - name:',1)[0])
        for operation in s0.FAILED_INVENTORY_ORIGINS:
            a,proof,_=self.authority(operation)
            for filename,key in (('compatibility-s0-exact-exit.py','operator_sha256'),('compatibility-retirement.py','helper_sha256')):
                a[key]=s0.sha((repo/'scripts/database'/filename).read_bytes())
            classifier=(repo/'scripts/database/compatibility-platform-window-action.py').read_bytes()
            a['classifier_sha256']=s0.sha(classifier)
            for change in ('none','different_selector','wrong_producer'):
                with self.subTest(operation=operation,change=change),tempfile.TemporaryDirectory(prefix='qs-owned-s0-action-choice-') as temporary:
                    producer=dict(proof['producer'])
                    if change=='wrong_producer':producer['id']+=1
                    fixture={'producer':producer,'completion':proof['completion'],'workflow':proof['workflow'],
                        'jobs':[{'id':123,'name':'Retire exact private lifecycle stage with workflow quarantine','status':'completed','conclusion':'success'}],
                        'footer':proof['footer'],'classifier':base64.b64encode(classifier).decode()}
                    context={'eventName':'workflow_dispatch','ref':'refs/heads/main','sha':a['source_sha'],'runId':555,
                        'repo':{'owner':'FangcunMount','repo':'qs-server'}}
                    program=('const fixture='+json.dumps(fixture)+';const context='+json.dumps(context)+';'
                        +"const github={rest:{repos:{getBranch:async()=>({data:{commit:{sha:context.sha}}}),getContent:async()=>({data:{type:'file',encoding:'base64',content:fixture.classifier}})},actions:{getWorkflowRun:async()=>({data:fixture.producer}),getWorkflowRunAttempt:async()=>({data:fixture.completion}),getWorkflow:async()=>({data:fixture.workflow}),listWorkflowRuns:async()=>({data:{workflow_runs:[]}}),listJobsForWorkflowRunAttempt:async()=>({data:{jobs:fixture.jobs}}),downloadJobLogsForWorkflowRun:async()=>({data:fixture.footer})}}};const core={setOutput:()=>{}};"
                        +"new (Object.getPrototypeOf(async function(){}).constructor)('context','github','core','require',"
                        +json.dumps(script)+")(context,github,core,require).catch(()=>{process.exitCode=1;});")
                    env=dict(os.environ,GITHUB_WORKFLOW_REF='FangcunMount/qs-server/.github/workflows/compatibility-s0-exact-exit.yml@refs/heads/main',
                        GITHUB_WORKSPACE=str(repo),RUNNER_TEMP=temporary,S0_APPROVAL_JSON=s0.canonical(a)[:-1].decode(),
                        S0_APPROVAL_SHA256=s0.sha(s0.canonical(a)),S0_FAILED_INVENTORY_OPERATION_ID=s0.OLD_OP if change=='different_selector' and operation!=s0.OLD_OP else '38019009876-2' if change=='different_selector' else operation)
                    result=subprocess.run(['node','-e',program],capture_output=True,env=env,timeout=5)
                    self.assertEqual(result.returncode,0 if change=='none' else 1,result.stderr.decode())
                    paths=list(Path(temporary).iterdir())
                    self.assertEqual(len(paths),1 if change=='none' else 0)
                    if paths:self.assertEqual({p.name for p in paths[0].iterdir()},{'approval.json','authority.json'})

    def test_real_cli_rejects_missing_and_unknown_operation_before_effect(self):
        path=Path(__file__).with_name('compatibility-s0-exact-exit.py')
        for values in ([],['--failed-inventory-operation-id','38019009876-3']):
            result=subprocess.run([sys.executable,str(path),*values],capture_output=True,timeout=3)
            self.assertEqual(result.returncode,2);self.assertEqual(result.stdout,b'')
        for operation in s0.FAILED_INVENTORY_ORIGINS:
            result=subprocess.run([sys.executable,str(path),'--failed-inventory-operation-id',operation,
                '--repo','/nonexistent','--approval','/nonexistent','--proof','/nonexistent','--approval-sha256','a'*64],capture_output=True,timeout=3)
            self.assertEqual(result.returncode,1);self.assertIn(b'unknown_or_refused',result.stdout)
            self.assertEqual(result.stderr,b'')


if __name__=='__main__':unittest.main()

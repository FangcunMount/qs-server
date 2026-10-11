#!/usr/bin/env python3
"""Temporary, source-reviewed physical exit of either of two fixed failed producers."""
import argparse
import base64
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import resource
import select
import selectors
import signal
import subprocess
import stat
import sys
import time

sys.dont_write_bytecode = True
ROOT = '/opt/backups/qs-server/compatibility-retirement'
OLD_OP = '38019009876-1'
OLD_RUN = '38025045551-1'
OLD_SOURCE = 'ae807219ffee37c1f060f1bfca25dd7c408ef9fd'
FAILED_INVENTORY_ORIGINS = {
    OLD_OP: (OLD_RUN, OLD_SOURCE),
    '38019009876-2': ('38037670416-1', 'd638638865c08b2b77a28ecc4920fb3f679ca798'),
}
PIN_SHA = '984b437e6cf15c9a80f1633cc9dddcb59f8443c818e1a0ab6f54e47b7e3314b1'
TRANSPORT_SHA = '5f88b51b14771960a46670a4e32353547bb2da5c0e303c4d3b27b4f58cc2e92a'
HASH = re.compile(r'^[0-9a-f]{64}$')
SHA = re.compile(r'^[0-9a-f]{40}$')
RUN = re.compile(r'^[1-9][0-9]{0,19}-[1-9][0-9]{0,3}$')
REQUIRED = ('protocol', 'failed_inventory_operation_id', 'source_sha', 'operator_sha256', 'helper_sha256', 'pin_sha256',
            'baseline_sha256', 'baseline_observing_source_sha', 'baseline_observing_run_id',
            'completion_source_sha', 'tool_source_sha', 'completion_run_id', 'completion_operation_id',
            'completion_job_id', 'completion_footer_sha256', 'classifier_sha256',
            'native_stdout_sha256', 'workflow_scope_sha256', 'manifest_sha256', 'request_sha256',
            'independent_acceptance_sha256', 'producer_workflow_id', 'total_seconds')


class Rejected(Exception):
    pass


def fail(category):
    raise Rejected(category)


def canonical(value):
    return (json.dumps(value, sort_keys=True, separators=(',', ':'), ensure_ascii=True)+'\n').encode('ascii')


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def load(path, name):
    spec = importlib.util.spec_from_file_location(name, path)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def failed_inventory_origin(a):
    operation = a.get('failed_inventory_operation_id')
    if type(operation) is not str or operation not in FAILED_INVENTORY_ORIGINS:
        fail('s0_approval_rejected')
    return operation, *FAILED_INVENTORY_ORIGINS[operation]


def approved(raw, expected, source):
    try: a = json.loads(raw)
    except (ValueError, TypeError): fail('s0_approval_rejected')
    if type(a) is not dict or set(a) != set(REQUIRED) or canonical(a) != raw or sha(raw) != expected:
        fail('s0_approval_rejected')
    _, original_run, _ = failed_inventory_origin(a)
    if a['protocol'] != 'qs-s0-exact-physical-exit/v1' or a['source_sha'] != source or not SHA.fullmatch(source):
        fail('s0_approval_rejected')
    for key in REQUIRED:
        if key.endswith('_sha256') and (type(a[key]) is not str or not HASH.fullmatch(a[key])): fail('s0_approval_rejected')
        if key.endswith('_source_sha') and (type(a[key]) is not str or not SHA.fullmatch(a[key])): fail('s0_approval_rejected')
        if key.endswith('_run_id') and (type(a[key]) is not str or not RUN.fullmatch(a[key])): fail('s0_approval_rejected')
    if not RUN.fullmatch(a['completion_operation_id']) or a['completion_run_id'] == original_run or a['pin_sha256'] != PIN_SHA:
        fail('s0_approval_rejected')
    for key in ('completion_job_id', 'producer_workflow_id'):
        if type(a[key]) is not int or not 0 < a[key] < 2**64: fail('s0_approval_rejected')
    if type(a['total_seconds']) is not int or a['total_seconds'] != 600: fail('s0_approval_rejected')
    return a


def validate_authority(a, proof, transport):
    # This is operator authorization provenance, never a native capability or
    # an acceptedMaterials constructor. The fixed workflow freshly obtains it
    # from GitHub, and the independent approval binds its original footer.
    if type(proof) is not dict or set(proof) != {'producer', 'completion', 'workflow', 'active_runs', 'footer', 'classifier_sha256'}:
        fail('s0_authority_rejected')
    _, original_run, original_source = failed_inventory_origin(a)
    original_id, original_attempt = map(int, original_run.split('-'))
    p, c, w = proof['producer'], proof['completion'], proof['workflow']
    common = {'id', 'run_attempt', 'status', 'conclusion', 'head_sha', 'event', 'path', 'workflow_id'}
    if any(type(v) is not dict or set(v) != common for v in (p,c)): fail('s0_authority_rejected')
    run, attempt = map(int, a['completion_run_id'].split('-'))
    if p['id'] != original_id or p['run_attempt'] != original_attempt or p['head_sha'] != original_source or p['status'] != 'completed' or p['event'] != 'workflow_dispatch':
        fail('s0_producer_not_terminal')
    if c['id'] != run or c['run_attempt'] != attempt or c['head_sha'] != a['completion_source_sha'] or c['status'] != 'completed' or c['conclusion'] != 'success' or c['event'] != 'workflow_dispatch' or c['path'] != '.github/workflows/compatibility-retirement.yml':
        fail('s0_completion_not_bound')
    if type(w) is not dict or set(w) != {'id','path','state'} or w['id'] != a['producer_workflow_id'] or p['workflow_id'] != w['id'] or p['path'] != w['path'] or w['state'] != 'disabled_manually' or proof['active_runs'] != []:
        fail('s0_producer_isolation_unproven')
    if proof['classifier_sha256'] != a['classifier_sha256']: fail('s0_classifier_not_bound')
    raw = transport.decode_armored_receipt(proof['footer'])
    try: f = json.loads(raw)
    except ValueError: fail('s0_completion_not_bound')
    keys = {'protocol','dispatcher_source_sha','tool_source_sha','operation_id','actual_run_id','workflow_scope_sha256','platform_installed','platform_restored','native_channel_terminal','native_disposition','native_stdout_sha256','whole_writer_fence_proven','drop_ready'}
    if type(f) is not dict or set(f) != keys or sha(canonical(f)) != a['completion_footer_sha256']:
        fail('s0_completion_not_bound')
    if f['protocol'] != 'runner_platform_window_owner_v1' or f['dispatcher_source_sha'] != a['completion_source_sha'] or f['tool_source_sha'] != a['tool_source_sha'] or f['operation_id'] != a['completion_operation_id'] or f['actual_run_id'] != a['completion_run_id'] or f['workflow_scope_sha256'] != a['workflow_scope_sha256'] or f['native_stdout_sha256'] != a['native_stdout_sha256'] or f['native_disposition'] != 'native_completed':
        fail('s0_completion_not_bound')
    if any(f[k] is not True for k in ('platform_installed','platform_restored','native_channel_terminal')) or f['whole_writer_fence_proven'] is not False or f['drop_ready'] is not False:
        fail('s0_completion_not_bound')


def stamp(st):
    return [st.st_dev, st.st_ino, st.st_size, st.st_mtime_ns, st.st_ctime_ns, st.st_uid, stat.S_IMODE(st.st_mode), st.st_nlink]


def close(fd):
    os.close(fd)


def held_hash(fd, expected, deadline):
    if stamp(os.fstat(fd)) != expected['stat']: fail('s0_file_changed')
    os.lseek(fd, 0, os.SEEK_SET)
    h = hashlib.sha256()
    count = 0
    while True:
        if time.monotonic() >= deadline: fail('s0_timeout')
        part = os.read(fd, 131072)
        if not part: break
        count += len(part)
        if count > expected['stat'][2]: fail('s0_file_changed')
        h.update(part)
    if count != expected['stat'][2] or h.hexdigest() != expected['sha256'] or stamp(os.fstat(fd)) != expected['stat']:
        fail('s0_file_changed')


def directory_matches(parentfd, name, dirfd, identity):
    st = os.fstat(dirfd)
    visible = os.stat(name, dir_fd=parentfd, follow_symlinks=False)
    actual = [st.st_dev, st.st_ino, st.st_uid, stat.S_IMODE(st.st_mode)]
    if not stat.S_ISDIR(st.st_mode) or not stat.S_ISDIR(visible.st_mode) or actual != identity or actual != [visible.st_dev,visible.st_ino,visible.st_uid,stat.S_IMODE(visible.st_mode)]:
        fail('s0_directory_changed')


def purge_open(parentfd, name, baseline, names, authority, absence, deadline, *, source_uid=None):
    """Actual FD owner. Only the fixed production caller can reach this leaf."""
    owner = os.getuid() if source_uid is None else source_uid
    if type(owner) is not int or not 0 <= owner < 2**32 or (source_uid is not None and (os.getuid()!=0 or os.geteuid()!=0)): fail('s0_source_owner_unbound')
    if set(baseline['files']) != set(names): fail('s0_scope_changed')
    fds = {}; removed = []; dirfd = None; known = False
    intent_name = 's0-exit-'+authority['actual_run_id']+'.intent.json'
    try:
        dirfd = os.open(name, os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW, dir_fd=parentfd)
        identity = baseline['directory_identity']
        directory_matches(parentfd,name,dirfd,identity)
        if identity[2:] != [owner,0o700] or set(os.listdir(dirfd)) != set(names): fail('s0_scope_changed')
        soft, hard = resource.getrlimit(resource.RLIMIT_NOFILE)
        needed = len(names)+64
        if needed > 8192 or needed > hard: fail('s0_fd_budget_unavailable')
        if soft < needed: resource.setrlimit(resource.RLIMIT_NOFILE,(needed,hard))
        absence(deadline)
        for member in sorted(names):
            if not re.fullmatch(r'[a-z][a-z0-9_.-]{0,120}',member): fail('s0_scope_changed')
            expected = baseline['files'][member]
            if type(expected) is not dict or set(expected) != {'sha256','stat'} or not HASH.fullmatch(expected['sha256']) or type(expected['stat']) is not list or len(expected['stat']) != 8: fail('s0_baseline_rejected')
            fd = os.open(member,os.O_RDONLY|os.O_NOFOLLOW|os.O_NONBLOCK,dir_fd=dirfd); fds[member]=fd
            st = os.fstat(fd)
            if not stat.S_ISREG(st.st_mode) or st.st_uid != owner or stat.S_IMODE(st.st_mode) != 0o600 or st.st_nlink != 1 or stamp(st) != expected['stat'] or stamp(os.stat(member,dir_fd=dirfd,follow_symlinks=False)) != expected['stat']: fail('s0_file_changed')
            held_hash(fd,expected,deadline)
        directory_matches(parentfd,name,dirfd,identity)
        if set(os.listdir(dirfd)) != set(names): fail('s0_scope_changed')
        absence(deadline)
        intent = canonical({'protocol':'s0-exact-exit-intent/v1','authority':authority,'baseline_sha256':authority['baseline_sha256'],'directory_identity':identity,'files':baseline['files']})
        journal = os.open(intent_name,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600,dir_fd=parentfd)
        try:
            os.fchmod(journal,0o600)
            pos=0
            while pos<len(intent):
                n=os.write(journal,intent[pos:])
                if n <= 0: fail('s0_intent_unknown')
                pos+=n
            os.fsync(journal)
        finally: close(journal)
        os.fsync(parentfd)
        for member in sorted(names):
            directory_matches(parentfd,name,dirfd,identity)
            if set(os.listdir(dirfd)) != set(names)-set(removed): fail('s0_scope_changed')
            expected=baseline['files'][member]; fd=fds[member]
            held_hash(fd,expected,deadline)
            if stamp(os.stat(member,dir_fd=dirfd,follow_symlinks=False)) != expected['stat']: fail('s0_file_changed')
            os.unlink(member,dir_fd=dirfd); removed.append(member)
            os.fsync(dirfd)
            close(fd); del fds[member]
        directory_matches(parentfd,name,dirfd,identity)
        if os.listdir(dirfd) or fds: fail('s0_zero_unproven')
        os.fsync(dirfd)
        # Keep the original directory FD until exact rmdir and absent readback.
        st=os.stat(name,dir_fd=parentfd,follow_symlinks=False)
        if [st.st_dev,st.st_ino,st.st_uid,stat.S_IMODE(st.st_mode)] != identity: fail('s0_directory_changed')
        os.rmdir(name,dir_fd=parentfd); os.fsync(parentfd)
        if name in os.listdir(parentfd) or os.listdir(dirfd): fail('s0_zero_unproven')
        close(dirfd); dirfd=None
        result={'protocol':'s0_exact_exit_result_v1','actual_run_id':authority['actual_run_id'],'baseline_sha256':authority['baseline_sha256'],'complete':True,'disposition':'owned_namespace_absent','removed_file_count':len(removed),'remaining_owned_files':0,'original_content_verified':False,'retirement_proof':False,'drop_authority':False}
        zero_name='s0-exit-'+authority['actual_run_id']+'.zero.json'
        zero=canonical(result)
        zero_fd=os.open(zero_name,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600,dir_fd=parentfd)
        try:
            os.fchmod(zero_fd,0o600);pos=0
            while pos<len(zero):
                n=os.write(zero_fd,zero[pos:])
                if n<=0:fail('s0_zero_unproven')
                pos+=n
            os.fsync(zero_fd)
        finally:close(zero_fd)
        os.fsync(parentfd)
        known=True
        return result
    except (OSError,ValueError,Rejected):
        fail('s0_physical_exit_unknown' if removed else 's0_predelete_refused')
    finally:
        unknown=False
        for fd in list(fds.values()):
            try: close(fd)
            except OSError: unknown=True
        if dirfd is not None:
            try: close(dirfd)
            except OSError: unknown=True
        if unknown or (known and fds): fail('s0_physical_exit_unknown')


def remote(a, proof, actual_run, helper, transport, source_uid):
    validate_authority(a,proof,transport)
    operation, original_run, original_source = failed_inventory_origin(a)
    ref, _, _ = helper.failed_inventory_profile(operation)
    if ref['run_id'] != original_run or ref['source_sha'] != original_source: fail('s0_source_rejected')
    if type(source_uid) is not int or not 0 <= source_uid < 2**32: fail('s0_source_owner_unbound')
    if os.getuid()!=0 or os.geteuid()!=0 or not RUN.fullmatch(actual_run) or actual_run in (original_run,a['completion_run_id']): fail('s0_host_or_run_rejected')
    args=argparse.Namespace(root=ROOT,operation_id=operation)
    directory,output,request,names,_=helper.failed_inventory_inputs(args,source_uid=source_uid)
    # Existing lock only: never create or adopt a missing lock.
    parentfd=os.open(directory,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW)
    lockfd=None
    try:
        lockfd=os.open('operation.lock',os.O_RDWR|os.O_NOFOLLOW,dir_fd=parentfd)
        st=os.fstat(lockfd)
        if not stat.S_ISREG(st.st_mode) or st.st_uid!=source_uid or stat.S_IMODE(st.st_mode)!=0o600 or st.st_nlink!=1: fail('s0_lock_rejected')
        import fcntl
        fcntl.flock(lockfd,fcntl.LOCK_EX|fcntl.LOCK_NB)
        parent=os.fstat(parentfd)
        if not stat.S_ISDIR(parent.st_mode) or parent.st_uid!=source_uid or stat.S_IMODE(parent.st_mode)!=0o700: fail('s0_source_owner_unbound')
        identity=lambda v:(v.st_dev,v.st_ino,v.st_uid,stat.S_IMODE(v.st_mode))
        parent_identity=identity(parent)
        def actual_absence(deadline):
            visible=os.stat(directory,follow_symlinks=False)
            if identity(visible)!=parent_identity or identity(os.fstat(parentfd))!=parent_identity or stamp(os.fstat(lockfd))!=stamp(st) or stamp(os.stat('operation.lock',dir_fd=parentfd,follow_symlinks=False))!=stamp(st):fail('s0_lock_rejected')
            helper.failed_inventory_container_absent(deadline,operation)
        baselinefd=os.open(helper.FAILED_INVENTORY_BASELINE,os.O_RDONLY|os.O_NOFOLLOW|os.O_NONBLOCK,dir_fd=parentfd)
        try:
            bs=os.fstat(baselinefd)
            if not stat.S_ISREG(bs.st_mode) or bs.st_uid!=source_uid or stat.S_IMODE(bs.st_mode)!=0o600 or bs.st_nlink!=1 or not 0<bs.st_size<=helper.FAILED_INVENTORY_BASELINE_MAX: fail('s0_baseline_rejected')
            with os.fdopen(baselinefd,'rb',closefd=False) as stream: baseline_raw=stream.read(helper.FAILED_INVENTORY_BASELINE_MAX+1)
            if sha(baseline_raw)!=a['baseline_sha256'] or stamp(os.fstat(baselinefd))!=stamp(bs) or stamp(os.stat(helper.FAILED_INVENTORY_BASELINE,dir_fd=parentfd,follow_symlinks=False))!=stamp(bs): fail('s0_baseline_rejected')
            baseline=helper.decode(baseline_raw)
        finally: close(baselinefd)
        keys={'format_version','kind','original_operation_id','original_inventory_report','observing_source_sha','observing_run_id','approval_sha256','directory_identity','files','inventory_complete','retirement_proof','drop_authority','producer_container_absent'}
        if set(baseline)!=keys or baseline['format_version']!=1 or type(baseline['format_version']) is not int or baseline['kind']!='cleanup_only_failed_inventory_baseline' or baseline['original_operation_id']!=operation or baseline['original_inventory_report']!=ref or baseline['observing_source_sha']!=a['baseline_observing_source_sha'] or baseline['observing_run_id']!=a['baseline_observing_run_id'] or any(baseline[k] is not False for k in ('inventory_complete','retirement_proof','drop_authority')) or baseline['producer_container_absent'] is not True: fail('s0_baseline_rejected')
        authority={k:a[k] for k in REQUIRED if k not in ('protocol','pin_sha256','helper_sha256','total_seconds')}
        authority['actual_run_id']=actual_run
        return purge_open(parentfd,output.name,baseline,names,authority,actual_absence,time.monotonic()+a['total_seconds'],source_uid=source_uid)
    finally:
        if lockfd is not None: close(lockfd)
        close(parentfd)


# Root receives one finite source-bound packet over the original strict-pinned
# SSH channel. No uploaded remote directory, source file or key is created.
BOOTSTRAP = r'''
import base64,hashlib,json,os,re,sys,types
try:
 if os.getuid()!=0 or os.geteuid()!=0 or len(sys.argv)!=3: raise ValueError()
 channel,claimed=sys.argv[1:]
 if re.fullmatch(r'0|[1-9][0-9]{0,9}',claimed) is None or int(claimed)>=2**32: raise ValueError()
 if channel=='sudo-user':
  actual=os.environ.get('SUDO_UID','')
  if re.fullmatch(r'[1-9][0-9]{0,9}',actual) is None or actual!=claimed: raise ValueError()
 elif channel=='root-direct':
  if claimed!='0' or 'SUDO_UID' in os.environ: raise ValueError()
 else: raise ValueError()
 source_uid=int(claimed)
 os.environ.pop('SUDO_PASSWORD',None);os.environ.pop('SUDO_ASKPASS',None)
 raw=sys.stdin.buffer.readline(2097153)
 if len(raw)>2097152 or not raw.endswith(b'\n'): raise ValueError()
 p=json.loads(raw)
 if set(p)!={'approval','proof','actual_run_id','sources'}: raise ValueError()
 a=p['approval']; modules={}
 for name,key in (('operator','operator_sha256'),('helper','helper_sha256'),('transport',None)):
  body=base64.b64decode(p['sources'][name],validate=True)
  expected=a[key] if key else '5f88b51b14771960a46670a4e32353547bb2da5c0e303c4d3b27b4f58cc2e92a'
  if hashlib.sha256(body).hexdigest()!=expected: raise ValueError()
  m=types.ModuleType('s0_'+name);m.__file__='/nonexistent/s0-'+name+'.py';sys.modules[m.__name__]=m
  exec(compile(body,m.__file__,'exec'),m.__dict__);modules[name]=m
 result=modules['operator'].remote(a,p['proof'],p['actual_run_id'],modules['helper'],modules['transport'],source_uid)
 print(json.dumps(result,sort_keys=True,separators=(',',':')))
except BaseException:
 print('{"protocol":"s0_exact_exit_result_v1","complete":false,"disposition":"unknown_or_refused"}')
 raise SystemExit(1)
'''


def sudo_password(value):
    if type(value) is not str or len(value.encode('utf-8')) > 4096 or any(c in value for c in ('\x00', '\r', '\n')):
        fail('s0_root_channel_refused')
    return value


def root_packet_once(packet, helper, control, password):
    """Only this S0 root program; retain finite stdin until actual EOF/wait.

    Losing the original SSH control pipe makes the result unknown. Closing
    our pipe is not proof that a root descendant or an unlink completed.
    """
    import contextlib
    uid, euid = os.getuid(), os.geteuid()
    if uid != euid: fail('s0_root_channel_refused')
    raw = canonical(packet)
    if len(raw) > 2097152: fail('s0_packet_rejected')
    os.environ.clear(); os.environ['PATH'] = '/usr/bin:/bin'
    if uid and password: os.environ['SUDO_PASSWORD'] = sudo_password(password)
    environment = contextlib.nullcontext(None) if uid == 0 else helper.root_askpass_environment()
    child = None; streams = None; previous = {}; cancelled = [False]
    try:
        for number in (signal.SIGINT, signal.SIGTERM, signal.SIGHUP):
            previous[number] = signal.getsignal(number)
            signal.signal(number, lambda _n, _f: cancelled.__setitem__(0, True))
        with environment as private:
            command = ['/usr/bin/python3', '-I', '-c', BOOTSTRAP, 'sudo-user' if uid else 'root-direct', str(uid)]
            if uid: command = ['/usr/bin/sudo', '-A' if private else '-n', '--', *command]
            child = subprocess.Popen(command, env=private or {'PATH':'/usr/bin:/bin'},
                stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                start_new_session=True, bufsize=0)
            if os.getpgid(child.pid) != child.pid: fail('s0_physical_exit_unknown')
            streams = selectors.DefaultSelector()
            os.set_blocking(child.stdin.fileno(), False)
            streams.register(child.stdin, selectors.EVENT_WRITE, 'stdin')
            streams.register(child.stdout, selectors.EVENT_READ, 'stdout')
            offset = 0; output = bytearray(); deadline = time.monotonic()+660
            try:
                while streams.get_map() or child.poll() is None:
                    if cancelled[0] or time.monotonic() >= deadline or select.select([control], [], [], 0)[0]:
                        fail('s0_physical_exit_unknown')
                    for key, _ in streams.select(0.1):
                        if key.data == 'stdin':
                            offset += os.write(child.stdin.fileno(), raw[offset:offset+8192])
                            if offset == len(raw): streams.unregister(child.stdin)  # live root control pipe
                        else:
                            body = os.read(child.stdout.fileno(), 8192)
                            if not body: streams.unregister(child.stdout)
                            else: output.extend(body)
                            if len(output) > 32768: fail('s0_physical_exit_unknown')
                code = child.wait()
                if offset != len(raw) or not output: fail('s0_physical_exit_unknown')
                # The reaped leader cannot authorize signals to a reused PGID.
                try: os.killpg(child.pid, 0)
                except ProcessLookupError: return code, bytes(output)
                except PermissionError: pass
                fail('s0_physical_exit_unknown')
            except BaseException:
                child.stdin.close()
                try: child.wait(timeout=25)
                except subprocess.TimeoutExpired: pass  # root work remains unknown; never retry
                raise
    finally:
        if streams is not None: streams.close()
        if child is not None:
            for stream in (child.stdin, child.stdout):
                if stream is not None and not stream.closed: stream.close()
        for number, handler in previous.items(): signal.signal(number, handler)
        os.environ.pop('SUDO_PASSWORD', None); os.environ.pop('SUDO_ASKPASS', None)


# Nonroot owns the original SSH control FD. It imports only the two approved
# source bytes; the password is never part of the unchanged root packet.
SUDO_BOOTSTRAP = r'''
import base64,hashlib,json,os,sys,types
def unique(pairs):
 d={}
 for k,v in pairs:
  if k in d: raise ValueError()
  d[k]=v
 return d
try:
 if os.getuid()!=os.geteuid(): raise ValueError()
 raw=sys.stdin.buffer.readline(2097153)
 if len(raw)>2097152 or not raw.endswith(b'\n'): raise ValueError()
 envelope=json.loads(raw,object_pairs_hook=unique)
 if type(envelope)!=dict or set(envelope)!={'root_packet','sudo_password'}: raise ValueError()
 packet=envelope['root_packet'];password=envelope['sudo_password']
 if type(password)!=str or len(password.encode('utf-8'))>4096 or any(c in password for c in ('\x00','\r','\n')): raise ValueError()
 if type(packet)!=dict or set(packet)!={'approval','proof','actual_run_id','sources'}: raise ValueError()
 a=packet['approval'];modules={}
 for name,key in (('operator','operator_sha256'),('helper','helper_sha256')):
  body=base64.b64decode(packet['sources'][name],validate=True)
  if hashlib.sha256(body).hexdigest()!=a[key]: raise ValueError()
  m=types.ModuleType('s0_private_'+name);m.__file__='/nonexistent/s0-'+name+'.py';sys.modules[m.__name__]=m
  exec(compile(body,m.__file__,'exec'),m.__dict__);modules[name]=m
 code,output=modules['operator'].root_packet_once(packet,modules['helper'],sys.stdin.fileno(),password)
 sys.stdout.buffer.write(output);sys.stdout.buffer.flush()
 raise SystemExit(code)
except SystemExit: raise
except BaseException:
 print('{"protocol":"s0_exact_exit_result_v1","complete":false,"disposition":"unknown_or_refused"}')
 raise SystemExit(1)
'''


def run(repo, approval_path, proof_path, approved_hash, operation_id):
    platform=load(Path(repo)/'scripts/database/compatibility-platform-window-action.py','s0_platform')
    pin=load(Path(repo)/'scripts/database/compatibility-host-inventory-action.py','s0_pin')
    transport=load(Path(repo)/'scripts/dbops/receipt-transport.py','s0_transport')
    source=os.environ.get('GITHUB_SHA',''); run_id=os.environ.get('GITHUB_RUN_ID','')+'-'+os.environ.get('GITHUB_RUN_ATTEMPT','')
    if os.environ.get('GITHUB_REF')!='refs/heads/main' or os.environ.get('GITHUB_WORKFLOW_REF')!='FangcunMount/qs-server/.github/workflows/compatibility-s0-exact-exit.yml@refs/heads/main' or not RUN.fullmatch(run_id): fail('s0_source_rejected')
    a=approved(Path(approval_path).read_bytes(),approved_hash,source)
    if a['failed_inventory_operation_id']!=operation_id:fail('s0_approval_rejected')
    proof=json.loads(Path(proof_path).read_bytes()); validate_authority(a,proof,transport)
    sources={}
    for name,path,key in (('operator','scripts/database/compatibility-s0-exact-exit.py','operator_sha256'),('helper','scripts/database/compatibility-retirement.py','helper_sha256'),('pin','scripts/database/compatibility-host-inventory-action.py','pin_sha256'),('transport','scripts/dbops/receipt-transport.py',None)):
        raw=platform.source_bytes(repo,source,path)
        if sha(raw)!=(a[key] if key else TRANSPORT_SHA): fail('s0_source_rejected')
        sources[name]=raw
    # The approved ORDER helper is a dependency, not copied into this leaf.
    route=pin.route_file(dict(os.environ))
    import uuid,shlex
    state=Path(os.environ['RUNNER_TEMP'])/('s0-exact-exit-'+uuid.uuid4().hex)
    state.mkdir(mode=0o700)
    assets=[]; terminal=False; started=False
    try:
        for name,body in (('route.json',canonical({k:v for k,v in route.items() if k!='key'})),('ssh.key',route['key'].encode('ascii')),('pin.py',sources['pin'])):
            assets.append(platform.IdentityAsset(state/name,body))
        known='/usr/bin/python3 '+str(state/'pin.py')+' known-host --route-file '+str(state/'route.json')+' --key-type %t --key-blob %K'
        config=('Host qs-host-inventory\n HostName '+route['host']+'\n User '+route['username']+'\n Port '+route['port']+'\n IdentityFile '+str(state/'ssh.key')+'\n IdentitiesOnly yes\n PreferredAuthentications publickey\n BatchMode yes\n StrictHostKeyChecking yes\n CheckHostIP no\n HostKeyAlias qs-host-inventory\n UserKnownHostsFile /dev/null\n GlobalKnownHostsFile /dev/null\n KnownHostsCommand '+known+'\n ControlMaster no\n RequestTTY no\n ConnectTimeout 10\n LogLevel ERROR\n').encode('ascii')
        if any(c.isspace() or c in '\"\'`$\\' for c in str(state)): fail('s0_route_rejected')
        assets.append(platform.IdentityAsset(state/'ssh.config',config))
        for item in assets:item.check()
        password=sudo_password(os.environ.get('SUDO_PASSWORD',''))
        root_packet={'approval':a,'proof':proof,'actual_run_id':run_id,'sources':{k:base64.b64encode(v).decode('ascii') for k,v in sources.items() if k!='pin'}}
        packet=canonical({'root_packet':root_packet,'sudo_password':password})
        if len(packet)>2097152: fail('s0_packet_rejected')
        started=True
        code,output=platform.collect_owned(['/usr/bin/ssh','-F',str(state/'ssh.config'),'qs-host-inventory','/usr/bin/python3 -I -c '+shlex.quote(SUDO_BOOTSTRAP)],packet=packet,timeout=660)
        terminal=True
        for item in assets:item.check()
        result=json.loads(output)
        if code or result.get('complete') is not True or result.get('actual_run_id')!=run_id or result.get('baseline_sha256')!=a['baseline_sha256'] or result.get('disposition')!='owned_namespace_absent' or result.get('remaining_owned_files')!=0: fail('s0_physical_exit_unknown')
        schema={'protocol':frozenset({'s0_exact_exit_result_v1'}),'actual_run_id':'run_id','baseline_sha256':'hash64','complete':'bool','disposition':frozenset({'owned_namespace_absent'}),'removed_file_count':'uint','remaining_owned_files':'uint','original_content_verified':'bool','retirement_proof':'bool','drop_authority':'bool'}
        print(transport.encode_armored_receipt(result,schema=schema,secrets=(password,)))
    finally:
        if terminal or not started:
            for item in assets:item.remove()
        for item in assets:item.close()
        if (terminal or not started) and not list(state.iterdir()):state.rmdir()


def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--failed-inventory-operation-id',choices=tuple(FAILED_INVENTORY_ORIGINS),required=True)
    p.add_argument('--repo',required=True);p.add_argument('--approval',required=True)
    p.add_argument('--proof',required=True);p.add_argument('--approval-sha256',required=True)
    args=p.parse_args()
    try:run(args.repo,args.approval,args.proof,args.approval_sha256,args.failed_inventory_operation_id)
    except BaseException:
        print('{"protocol":"s0_exact_exit_result_v1","complete":false,"disposition":"unknown_or_refused"}')
        raise SystemExit(1)


if __name__=='__main__':main()

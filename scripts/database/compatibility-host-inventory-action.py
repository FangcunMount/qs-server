#!/usr/bin/env python3
"""Closed readonly inventory transport. No install, DB, sudo-Python or fence authority.

The independent descriptor approves only a deterministic current-workflow-run
request. V1 strictly requires prepositioned Match registration; explicit v2
observes only the final execute SSH session, never an absent-file fallback.
OpenSSH receives target values only through a private config; its handshake key
is checked against the existing private pinned SHA256 fingerprint. GitHub may
show the existing non-sensitive route vars in its step environment header.
This script projects no route values, private key, Match or inventory body;
it does NOT claim the whole GitHub run log contains no route metadata.
"""
import argparse
import base64
import hashlib
import hmac
import json
import math
import os
from pathlib import Path
import re
import selectors
import signal
import stat
import struct
import subprocess
import sys
import time
import uuid

sys.dont_write_bytecode = True

PROTOCOL = 'qs-host-inventory-action-approval/v1'
OBSERVATION_PROTOCOL = 'qs-host-inventory-action-approval/v2'
CONTEXT_MODE = 'current_ssh_session_observation'
INVENTORY_PROTOCOL = 'qs-retirement-linux-host-inventory/v1'
INVENTORY_OBSERVATION_PROTOCOL = 'qs-retirement-linux-host-inventory/v2'
AUDITED_SOURCE = '0d5d75046328e6d4a415380f4ed249a6d3c9714c'
INVENTORY_SHA = '73b76f309cbdec1524c83624e7b0eb784864713eeb6418589357f880a212922b'
TRANSPORT_SHA = '5f88b51b14771960a46670a4e32353547bb2da5c0e303c4d3b27b4f58cc2e92a'
SOURCE = re.compile(r'[0-9a-f]{40}')
HASH = re.compile(r'[0-9a-f]{64}')
RUN = re.compile(r'[1-9][0-9]{0,19}-[1-9][0-9]{0,3}')
CAPS = ('management_channel_authenticated', 'production_installed', 'sshd_reloaded', 'writer_fence_proven', 'historical_rerun_denied', 'execution_authority', 'cas_ready', 'drop_ready')
ROUTES = {'server_a': ('pinned_svra_retirement', 'server_a'), 'collection': ('pinned_svra_retirement', 'server_a'), 'worker': ('pinned_svrd_m5_postcheck', 'server_d'), 'runner': ('runner_no_linux_channel', 'runner')}
ASSETS = ('action.py', 'inventory.py', 'receipt.py', 'approval.json', 'request.json')
MAX_REPORT, MAX_PRIVATE, TOTAL_SECONDS = 16 << 20, 1 << 20, 240
ERRORS = frozenset(('action_input_rejected', 'approval_rejected', 'source_binding_rejected', 'private_file_rejected', 'private_file_changed', 'private_asset_missing', 'private_namespace_conflict', 'private_cleanup_unknown', 'private_manifest_rejected', 'route_rejected', 'host_key_rejected', 'matches_rejected', 'package_rejected', 'inventory_report_rejected', 'inventory_process_failed', 'transport_failed', 'transport_output_rejected', 'transport_timeout', 'transport_budget_exceeded', 'linux_host_required', 'session_observation_rejected', 'session_observation_changed', 'action_fixed_failure'))
EXECUTION_STAGES = frozenset(('asset_prepare', 'remote_bootstrap', 'asset_upload', 'remote_inventory', 'projection_validate', 'complete'))
CLEANUP_STATES = frozenset(('not_attempted', 'unknown', 'verified'))
CLEANUP_FAILURE_STAGES = frozenset(('none', 'remote_cleanup', 'local_cleanup', 'registration_cleanup'))
DIAGNOSTIC_SCHEMA = {'execution_stage':EXECUTION_STAGES, 'remote_cleanup':CLEANUP_STATES, 'local_cleanup':CLEANUP_STATES, 'registration_cleanup':CLEANUP_STATES, 'cleanup_failure_stage':CLEANUP_FAILURE_STAGES, 'cleanup_error_category':ERRORS | frozenset(('none',))}
SSH_FIELDS = ('authenticationmethods', 'authorizedkeysfile', 'authorizedkeyscommand', 'authorizedkeyscommanduser', 'authorizedprincipalsfile', 'authorizedprincipalscommand', 'trustedusercakeys', 'passwordauthentication', 'kbdinteractiveauthentication', 'hostbasedauthentication', 'gssapiauthentication', 'pubkeyauthentication', 'permituserenvironment', 'permituserrc', 'permittty', 'disableforwarding', 'forcecommand', 'strictmodes', 'acceptenv', 'allowusers', 'allowgroups', 'denyusers', 'denygroups')
ALWAYS_UNKNOWN = frozenset(('management_channel_and_request_origin_unproven', 'all_match_contexts_not_enumerated', 'nss_external_subject_coverage_unknown', 'all_writers_and_external_services_unproven', 'historical_refs_reruns_queues_approvals_not_fenced', 'local_runner_bypass_not_fenced', 'existing_sessions_not_drained', 'effective_acl_visibility_unknown', 'systemd_user_socket_and_transient_activation_coverage_unknown', 'ssh_key_option_semantics_and_authentication_unproven'))
INVENTORY_ERRORS = frozenset(('account_schema_unknown', 'command_denied_or_failed', 'command_executable_changed', 'command_executable_unprotected', 'command_not_in_closed_read_set', 'command_output_budget_exceeded', 'command_timeout', 'command_unavailable', 'cron_directory_budget_exceeded', 'directory_changed_during_read', 'directory_missing', 'directory_permission_unknown', 'directory_read_unknown', 'docker_mount_schema_unknown', 'docker_projection_schema_unknown', 'docker_roster_schema_or_budget_unknown', 'file_budget_exceeded', 'file_changed_during_read', 'file_link_or_type_unsupported', 'file_missing', 'file_path_unsupported', 'file_permission_unknown', 'file_read_unknown', 'file_content_forbidden', 'inventory_deadline_exceeded', 'inventory_input_rejected', 'inventory_request_rejected', 'inventory_fixed_failure', 'linux_host_required', 'process_budget_exceeded', 'process_link_visibility_unknown', 'process_schema_unknown', 'property_projection_schema_unknown', 'session_listing_schema_unknown', 'ssh_authorized_key_schema_unknown', 'ssh_config_syntax_unknown', 'ssh_effective_duplicate_key', 'ssh_effective_projection_incomplete', 'ssh_include_cycle_or_depth_unknown', 'ssh_include_directory_changed', 'ssh_include_path_unsupported', 'systemd_listing_schema_unknown', 'systemd_unit_budget_exceeded'))
UNKNOWN_EXTRA = frozenset(('ssh_daemon_original_argv_unknown', 'ssh_daemon_cli_override_unproven', 'ssh_actual_daemon_config_unknown', 'dynamic_ssh_key_or_principal_provider_unknown', 'ssh_context_subject_not_in_local_accounts', 'ssh_key_path_expansion_unsupported', 'ssh_public_key_file_name_unsupported', 'additional_ssh_ca_or_principal_source_unproven', 'systemd_execution_and_secret_environment_not_read', 'cron_contents_and_indirect_scripts_not_read', 'session_environment_origin_unproven', 'ssh_daemon_loaded_configuration_unproven', 'ssh_session_daemon_binding_unknown', 'ssh_session_host_unobserved'))
ATTEMPTS = frozenset(('accounts_visibility', 'process_visibility', 'ssh_config_visibility', 'ssh_effective_visibility', 'ssh_authorized_key_visibility', 'container_writable_path_visibility', 'systemd_writable_path_visibility', 'docker_visibility', 'systemd_visibility', 'sessions_visibility', 'cron_visibility', 'sudo_list_visibility', 'docker_socket_visibility', 'host_identity_visibility', 'namespace_visibility', 'process_end_recheck', 'docker_end_recheck', 'sessions_end_recheck', 'systemd_end_recheck', 'ssh_end_recheck', 'boot_end_recheck', 'files_end_recheck', 'session_context_visibility', 'qs_service_visibility'))
COMMANDS = frozenset(('sudo_list', 'docker_list', 'docker_inspect', 'sessions', 'units', 'unit_files', 'timers', 'sshd', 'unit', 'session','sshd_global', 'qs_service_inspect', 'qs_restore_image_present', 'qs_restore_image_inspect'))

class Rejected(Exception):
    def __init__(self, category):
        super().__init__(category if category in ERRORS else 'action_fixed_failure')

def reject(category):
    raise Rejected(category)

def canonical(value):
    return (json.dumps(value, sort_keys=True, separators=(',', ':'), ensure_ascii=True, allow_nan=False) + '\n').encode('ascii')

def sha(raw):
    return hashlib.sha256(raw).hexdigest()

def decode(raw, cap=MAX_PRIVATE):
    def pairs(items):
        result = {}
        for k, v in items:
            if k in result:
                reject('action_input_rejected')
            result[k] = v
        return result
    try:
        if type(raw) is not bytes or len(raw) > cap:
            reject('action_input_rejected')
        result = json.loads(raw, object_pairs_hook=pairs, parse_constant=lambda unused: reject('action_input_rejected'))
        if canonical(result) != raw:
            reject('action_input_rejected')
        return result
    except (ValueError, TypeError, UnicodeError, RecursionError):
        reject('action_input_rejected')

def exact(value, keys, category='action_input_rejected'):
    if type(value) is not dict or set(value) != set(keys):
        reject(category)

def digest(value):
    if type(value) is not str or not HASH.fullmatch(value):
        reject('inventory_report_rejected')

def integer(value, limit=(1 << 64)-1):
    if type(value) is not int or not 0 <= value <= limit:
        reject('inventory_report_rejected')

def boolean(value):
    if type(value) is not bool:
        reject('inventory_report_rejected')

def array(value, cap):
    if type(value) is not list or len(value) > cap:
        reject('inventory_report_rejected')

def hashes(value, keys):
    exact(value, keys, 'inventory_report_rejected')
    for v in value.values():
        digest(v)

def caps(value):
    exact(value, CAPS, 'inventory_report_rejected')
    if any(v is not False for v in value.values()):
        reject('inventory_report_rejected')

def stamp(s):
    return (s.st_dev, s.st_ino, s.st_mode, s.st_uid, s.st_gid, s.st_nlink, s.st_size, s.st_mtime_ns, s.st_ctime_ns)

def read_private(path, cap=MAX_PRIVATE):
    """Actual dirfds, no-follow/nonblock, private regular singlelink and EOF."""
    path = Path(path)
    if not path.is_absolute() or str(path) != os.path.normpath(str(path)):
        reject('private_file_rejected')
    fds, parents = [], []
    flags = os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK
    try:
        parent = os.open('/', flags | os.O_DIRECTORY); fds.append(parent)
        for name in path.parts[1:-1]:
            before = os.stat(name, dir_fd=parent, follow_symlinks=False)
            if not stat.S_ISDIR(before.st_mode):
                reject('private_file_rejected')
            child = os.open(name, flags | os.O_DIRECTORY, dir_fd=parent); fds.append(child)
            identity = stamp(os.fstat(child))[:5]
            if before.st_uid not in (0, os.geteuid()) or (before.st_mode & 0o022 and not (before.st_uid == 0 and before.st_mode & stat.S_ISVTX)):
                reject('private_file_rejected')
            if identity != stamp(before)[:5]:
                reject('private_file_changed')
            parents.append((parent, name, child, identity)); parent = child
        before = os.stat(path.name, dir_fd=parent, follow_symlinks=False)
        if (not stat.S_ISREG(before.st_mode) or before.st_nlink != 1 or before.st_size > cap or before.st_uid != os.geteuid() or stat.S_IMODE(before.st_mode) != 0o600):
            reject('private_file_rejected')
        fd = os.open(path.name, flags, dir_fd=parent); fds.append(fd)
        if stamp(os.fstat(fd)) != stamp(before): reject('private_file_changed')
        chunks, count = [], 0
        while True:
            b = os.read(fd, min(65536, cap+1-count))
            if not b: break
            chunks.append(b); count += len(b)
            if count > cap: reject('private_file_rejected')
        if count != before.st_size or stamp(os.fstat(fd)) != stamp(before) or stamp(os.stat(path.name, dir_fd=parent, follow_symlinks=False)) != stamp(before): reject('private_file_changed')
        for p, name, child, identity in parents:
            if stamp(os.fstat(child))[:5] != identity or stamp(os.stat(name, dir_fd=p, follow_symlinks=False))[:5] != identity: reject('private_file_changed')
        return b''.join(chunks), stamp(before)
    except FileNotFoundError: reject('private_asset_missing')
    except OSError: reject('private_file_rejected')
    finally:
        for fd in reversed(fds): os.close(fd)

def private_dir(path):
    path = Path(path)
    try:
        before = os.stat(path, follow_symlinks=False)
        if not stat.S_ISDIR(before.st_mode) or before.st_uid != os.geteuid() or stat.S_IMODE(before.st_mode) != 0o700:
            reject('private_file_rejected')
        fd = os.open(path, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW | os.O_NONBLOCK)
        if stamp(os.fstat(fd))[:5] != stamp(before)[:5]:
            os.close(fd); reject('private_file_changed')
        return fd, stamp(before)[:5]
    except FileNotFoundError:reject('private_asset_missing')
    except OSError:reject('private_file_rejected')

def write_exclusive(directory, name, raw):
    if not re.fullmatch(r'[A-Za-z0-9_.-]{1,80}', name) or type(raw) is not bytes:
        reject('private_file_rejected')
    d, identity = private_dir(directory)
    try:
        fd = os.open(name, os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW|os.O_NONBLOCK, 0o600, dir_fd=d)
        try:
            os.fchmod(fd, 0o600)
            pos = 0
            while pos < len(raw): pos += os.write(fd, raw[pos:])
            os.fsync(fd)
        finally: os.close(fd)
        os.fsync(d)
        if stamp(os.stat(directory, follow_symlinks=False))[:5] != identity: reject('private_file_changed')
    except FileExistsError: reject('private_namespace_conflict')
    except OSError: reject('private_file_rejected')
    finally: os.close(d)

def approval(raw, approved, source, actual_run):
    a = decode(raw)
    observation = type(a) is dict and a.get('protocol') == OBSERVATION_PROTOCOL
    keys = ('protocol', 'source_sha', 'operation_id', 'host_class', 'route', 'inventory_sha256', 'transport_sha256', 'wrapper_sha256', 'run_binding', 'total_seconds') + (('context_mode',) if observation else ('matches_sha256',))
    exact(a, keys, 'approval_rejected')
    if (type(approved) is not str or not HASH.fullmatch(approved) or sha(raw) != approved or a['protocol'] not in (PROTOCOL, OBSERVATION_PROTOCOL) or type(a['source_sha']) is not str or not SOURCE.fullmatch(a['source_sha']) or a['source_sha'] != source or type(a['operation_id']) is not str or not RUN.fullmatch(a['operation_id']) or type(a['host_class']) is not str or a['host_class'] not in ROUTES or a['route'] != ROUTES[a['host_class']][0] or a['run_binding'] != 'current_workflow_run' or type(a['total_seconds']) is not int or a['total_seconds'] != 120 or a['inventory_sha256'] != INVENTORY_SHA or a['transport_sha256'] != TRANSPORT_SHA or type(actual_run) is not str or not RUN.fullmatch(actual_run)):
        reject('approval_rejected')
    if observation and a['context_mode'] != CONTEXT_MODE: reject('approval_rejected')
    for k in (('wrapper_sha256',) if observation else ('wrapper_sha256','matches_sha256')):
        if type(a[k]) is not str or not HASH.fullmatch(a[k]): reject('approval_rejected')
    return a

def request(a, match_raw, run):
    if a['protocol'] == OBSERVATION_PROTOCOL:
        if match_raw is not None: reject('matches_rejected')
        # This is a seed only. The independently approved mode derives the real
        # request on the final SSH execution session, not on bootstrap/Runner.
        return canonical({'protocol':INVENTORY_OBSERVATION_PROTOCOL,'source_sha':a['source_sha'],'operation_id':a['operation_id'],'run_id':run,'host_role':ROUTES[a['host_class']][1],'context_mode':CONTEXT_MODE})
    matches = decode(match_raw)
    if sha(match_raw) != a['matches_sha256'] or type(matches) is not list or not 1 <= len(matches) <= 32:
        reject('matches_rejected')
    seen = set()
    for m in matches:
        exact(m, ('user', 'host', 'addr', 'laddr', 'lport'), 'matches_rejected')
        if any(type(v) is not str or not re.fullmatch(r'[A-Za-z0-9_.:-]{1,128}', v) for v in m.values()) or not m['lport'].isdigit() or not 1 <= int(m['lport']) <= 65535 or canonical(m) in seen:
            reject('matches_rejected')
        seen.add(canonical(m))
    return canonical({'protocol':INVENTORY_PROTOCOL, 'source_sha':a['source_sha'], 'operation_id':a['operation_id'], 'run_id':run, 'host_role':ROUTES[a['host_class']][1], 'matches':matches})

def inventory_api(raw):
    if sha(raw) != INVENTORY_SHA: reject('package_rejected')
    api = {'__name__':'pinned_readonly_inventory'}
    exec(compile(raw,'<pinned-readonly-inventory>','exec'),api)
    return api

def INVENTORY_SOURCE_BYTES():
    # Fixed sibling only; checkout files need not be chmod 0600. Exact approved
    # source bytes, finite EOF and named/FD identity are mandatory before exec.
    name='inventory.py' if Path(__file__).name=='action.py' else 'compatibility-retirement-host-inventory.py'
    path=Path(__file__).absolute().with_name(name)
    fd=None
    try:
        before=os.stat(path,follow_symlinks=False)
        if not stat.S_ISREG(before.st_mode) or before.st_nlink!=1 or before.st_size>MAX_PRIVATE or before.st_mode&0o022:reject('package_rejected')
        fd=os.open(path,os.O_RDONLY|os.O_NOFOLLOW|os.O_NONBLOCK)
        if stamp(os.fstat(fd))!=stamp(before):reject('package_rejected')
        chunks=[];count=0
        while True:
            raw=os.read(fd,min(65536,MAX_PRIVATE+1-count))
            if not raw:break
            chunks.append(raw);count+=len(raw)
            if count>MAX_PRIVATE:reject('package_rejected')
        if count!=before.st_size or stamp(os.fstat(fd))!=stamp(before) or stamp(os.stat(path,follow_symlinks=False))!=stamp(before):reject('package_rejected')
        raw=b''.join(chunks)
        if sha(raw)!=INVENTORY_SHA:reject('package_rejected')
        return raw
    except OSError:reject('package_rejected')
    finally:
        if fd is not None:os.close(fd)

def observed_request(seed, api):
    """Public inventory v2 is consumed by its CLI, never a private collector."""
    try:
        session = api['observe_current_session']()
        value = dict(seed,session=session)
        raw = canonical(value)
        api['request'](raw,sha(raw))
        return raw,session
    except api['Unknown']:
        reject('session_observation_rejected')

def file_record(v):
    if type(v) is not dict:reject('inventory_report_rejected')
    basic = ('path_sha256','stat_sha256','uid','gid','mode','is_link','current_user_write_access','acl_complete')
    content = ('path_sha256','raw_sha256','bytes','stat_sha256','uid','gid','mode','single_link','root_mode_protected','ancestor_root_mode_protected','acl_complete')
    if set(v) == set(basic):
        exact(v,basic,'inventory_report_rejected')
        for k in ('is_link','current_user_write_access'): boolean(v[k])
    elif set(v) == set(content):
        exact(v,content,'inventory_report_rejected'); digest(v['raw_sha256']); integer(v['bytes'],2<<20)
        for k in ('single_link','root_mode_protected','ancestor_root_mode_protected'): boolean(v[k])
        if v['single_link'] is not True: reject('inventory_report_rejected')
    else: reject('inventory_report_rejected')
    for k in ('path_sha256','stat_sha256'): digest(v[k])
    for k in ('uid','gid'): integer(v[k],(1<<32)-1)
    integer(v['mode'],0o7777)
    if v['acl_complete'] is not False: reject('inventory_report_rejected')

def validate_report(raw, a, approved, req_raw, run):
    if sha(canonical(a))!=approved:reject('inventory_report_rejected')
    v = decode(raw, MAX_REPORT)
    keys=('protocol','audited_source_sha','requested_source_sha','operation_id','run_id','request_sha256','host_role','identity','observations','end_rechecks','unknown','read_only_command_receipts','observed_budgets','required_readonly_host_visibility','root_python_or_permission_changes_requested','raw_credentials_or_configuration_output','capabilities','tool_sha256')
    req = decode(req_raw)
    observation = a['protocol'] == OBSERVATION_PROTOCOL
    exact(v,keys+(('context_mode','session_observation') if observation else ()),'inventory_report_rejected')
    for k, val in {'protocol':INVENTORY_OBSERVATION_PROTOCOL if observation else INVENTORY_PROTOCOL,'audited_source_sha':AUDITED_SOURCE,'requested_source_sha':a['source_sha'],'operation_id':a['operation_id'],'run_id':run,'request_sha256':sha(req_raw),'host_role':ROUTES[a['host_class']][1],'tool_sha256':INVENTORY_SHA}.items():
        if v[k] != val: reject('inventory_report_rejected')
    caps(v['capabilities'])
    if v['root_python_or_permission_changes_requested'] is not False or v['raw_credentials_or_configuration_output'] is not False: reject('inventory_report_rejected')
    i=v['identity']; exact(i,('uid','euid','groups_sha256','os_sha256','boot_id_sha256','namespaces'),'inventory_report_rejected')
    integer(i['uid'],(1<<32)-1); integer(i['euid'],(1<<32)-1)
    digest(i['groups_sha256']);digest(i['os_sha256'])
    if i['boot_id_sha256'] is not None: digest(i['boot_id_sha256'])
    if type(i['namespaces']) is not dict or not set(i['namespaces']) <= {'mnt','pid','user'}: reject('inventory_report_rejected')
    for x in i['namespaces'].values(): digest(x)
    o=v['observations'];exact(o,('accounts','processes','ssh','docker','systemd','sessions','cron','sudo_list','files','qs_services'),'inventory_report_rejected')
    services=o['qs_services']
    if services is not None:
        api=inventory_api(INVENTORY_SOURCE_BYTES())
        try:api['validate_qs_service_observation'](services,ROUTES[a['host_class']][1])
        except api['Unknown']:reject('inventory_report_rejected')
    accounts=o['accounts']
    if accounts is not None:
        exact(accounts,('local_subjects','passwd_raw_sha256','group_raw_sha256','nss_complete'),'inventory_report_rejected');digest(accounts['passwd_raw_sha256']);digest(accounts['group_raw_sha256'])
        if accounts['nss_complete'] is not False: reject('inventory_report_rejected')
        array(accounts['local_subjects'],32768)
        for r in accounts['local_subjects']:
            exact(r,('subject_sha256','uid','gid','home_sha256','shell_sha256','interactive_shell_candidate'),'inventory_report_rejected')
            for k in ('subject_sha256','home_sha256','shell_sha256'):digest(r[k])
            integer(r['uid'],(1<<32)-1);integer(r['gid'],(1<<32)-1);boolean(r['interactive_shell_candidate'])
    processes=o['processes']
    if type(processes) is not dict:reject('inventory_report_rejected')
    if set(processes)=={'observed','set_sha256'}:
        if processes != {'observed':0,'set_sha256':''}:reject('inventory_report_rejected')
    else:
        exact(processes,('rows','set_sha256','observed'),'inventory_report_rejected');array(processes['rows'],32768);digest(processes['set_sha256']);integer(processes['observed'],32768)
        if processes['observed'] != len(processes['rows']) or processes['set_sha256'] != sha(canonical(processes['rows'])):reject('inventory_report_rejected')
        for r in processes['rows']:
            expected={'pid','ppid','start_ticks','uid','euid','exe_sha256','cgroup_sha256'}
            if type(r) is not dict or set(r) not in (expected,expected|{'sshd','sshd_original_argv_sha256'}):reject('inventory_report_rejected')
            for k in ('pid','ppid','uid','euid'):integer(r[k],(1<<32)-1)
            if type(r['start_ticks']) is not str or not re.fullmatch(r'[0-9]{1,24}',r['start_ticks']):reject('inventory_report_rejected')
            digest(r['exe_sha256']);digest(r['cgroup_sha256'])
            if 'sshd' in r:
                if r['sshd'] is not True:reject('inventory_report_rejected')
                digest(r['sshd_original_argv_sha256'])
    ssh=o['ssh'];exact(ssh,('include_records','matches','all_match_contexts_complete'),'inventory_report_rejected')
    if ssh['all_match_contexts_complete'] is not False:reject('inventory_report_rejected')
    array(ssh['include_records'],1024);array(ssh['matches'],1024)
    for r in ssh['include_records']:
        if type(r) is not dict:reject('inventory_report_rejected')
        if set(r)=={'path_sha256','raw_sha256','sequence'}:
            digest(r['path_sha256']);digest(r['raw_sha256']);integer(r['sequence'],1024)
        elif set(r)=={'pattern_sha256','directory_listing_sha256','matched_files'}:
            digest(r['pattern_sha256']);digest(r['directory_listing_sha256']);integer(r['matched_files'],512)
        else:reject('inventory_report_rejected')
    if observation:
        api=inventory_api(INVENTORY_SOURCE_BYTES())
        try: api['request'](req_raw,sha(req_raw))
        except api['Unknown']: reject('inventory_report_rejected')
        session=req['session']; fields=api['validate_session'](session)
        so=v['session_observation']
        exact(so,('identity_sha256','connection_sha256','request_session_sha256','uid','euid','origin_proven','partial','host_status','usedns','daemon_binding_sha256','derived_matches_sha256','match_context_count','identity_connection_rechecked'),'inventory_report_rejected')
        integer(so['uid'],(1<<32)-1);integer(so['euid'],(1<<32)-1)
        for k in ('identity_sha256','connection_sha256'):
            if so[k]!=session[k]:reject('inventory_report_rejected')
        if v['context_mode']!=CONTEXT_MODE or so['request_session_sha256']!=sha(canonical(session)) or so['uid']!=session['uid'] or so['euid']!=session['euid'] or so['origin_proven'] is not False or so['partial'] is not True or so['identity_connection_rechecked'] is not True or so['usedns'] not in ('no','yes','unknown'):reject('inventory_report_rejected')
        if i['uid']!=session['uid'] or i['euid']!=session['euid']:reject('inventory_report_rejected')
        if so['daemon_binding_sha256'] is not None:digest(so['daemon_binding_sha256'])
        if so['host_status']=='numeric_peer_from_usedns_no':
            if so['usedns']!='no' or so['daemon_binding_sha256'] is None:reject('inventory_report_rejected')
            derived=[{'user':session['username'],'host':fields[0],'addr':fields[0],'laddr':fields[2],'lport':fields[3]}]
        elif so['host_status']=='host_unobserved':
            if so['usedns']=='no':reject('inventory_report_rejected')
            derived=[]
        else:reject('inventory_report_rejected')
        if so['derived_matches_sha256']!=sha(canonical(derived)) or type(so['match_context_count']) is not int or so['match_context_count']!=len(derived):reject('inventory_report_rejected')
        contexts={sha(canonical(x)) for x in derived}
    else:contexts={sha(canonical(x)) for x in req['matches']}
    for r in ssh['matches']:
        exact(r,('context_sha256','raw_sha256','projection','key_files'),'inventory_report_rejected');digest(r['context_sha256']);digest(r['raw_sha256'])
        if r['context_sha256'] not in contexts or type(r['projection']) is not dict or not set(SSH_FIELDS[:18]) <= set(r['projection']) <= set(SSH_FIELDS):reject('inventory_report_rejected')
        for p in r['projection'].values():
            exact(p,('sha256','state'),'inventory_report_rejected');digest(p['sha256'])
            if p['state'] not in ('yes','no','none','other'):reject('inventory_report_rejected')
        array(r['key_files'],512)
        for f in r['key_files']:
            exact(f,('path_sha256','raw_sha256','keys'),'inventory_report_rejected');digest(f['path_sha256']);digest(f['raw_sha256']);array(f['keys'],32768)
            for k in f['keys']:
                exact(k,('wire_sha256','options_sha256','certificate'),'inventory_report_rejected');digest(k['wire_sha256']);digest(k['options_sha256'])
                if k['certificate'] is not False:reject('inventory_report_rejected')
    docker=o['docker']
    if docker is not None:
        exact(docker,('containers',),'inventory_report_rejected');array(docker['containers'],256);seen=set()
        for r in docker['containers']:
            exact(r,('id','image_config_digest','inspect_sha256','status_sha256','running','privileged','readonly_rootfs','mounts','project_sha256','service_sha256'),'inventory_report_rejected');digest(r['id'])
            if r['id'] in seen or type(r['image_config_digest']) is not str or not re.fullmatch(r'sha256:[0-9a-f]{64}',r['image_config_digest']):reject('inventory_report_rejected')
            seen.add(r['id'])
            for k in ('inspect_sha256','status_sha256','project_sha256','service_sha256'):digest(r[k])
            for k in ('running','privileged','readonly_rootfs'):boolean(r[k])
            array(r['mounts'],512)
            for mount in r['mounts']:
                exact(mount,('source_sha256','target_sha256','rw','type_sha256'),'inventory_report_rejected');boolean(mount['rw'])
                for k in ('source_sha256','target_sha256','type_sha256'):digest(mount[k])
    systemd=o['systemd']
    if systemd is not None:
        exact(systemd,('rows','listing_hashes'),'inventory_report_rejected');hashes(systemd['listing_hashes'],('units','unit_files','timers'));array(systemd['rows'],256)
        for r in systemd['rows']:hashes(r,('unit_sha256','raw_sha256','property_sha256'))
    sessions=o['sessions']
    if sessions is not None:
        exact(sessions,('listing_sha256','rows'),'inventory_report_rejected');digest(sessions['listing_sha256']);array(sessions['rows'],256)
        for r in sessions['rows']:hashes(r,('session_sha256','properties_sha256'))
    array(o['files'],4096)
    for r in o['files']:file_record(r)
    array(o['cron'],2048)
    for r in o['cron']:
        if type(r) is not dict:reject('inventory_report_rejected')
        if set(r)=={'directory_sha256','listing_sha256'}:hashes(r,('directory_sha256','listing_sha256'))
        else:file_record(r)
    if o['sudo_list'] is not None:
        exact(o['sudo_list'],('raw_sha256','actual_command_denial_proven'),'inventory_report_rejected');digest(o['sudo_list']['raw_sha256'])
        if o['sudo_list']['actual_command_denial_proven'] is not False:reject('inventory_report_rejected')
    array(v['unknown'],256)
    if any(type(x) is not str for x in v['unknown']):reject('inventory_report_rejected')
    if v['unknown'] != sorted(set(v['unknown'])) or not ALWAYS_UNKNOWN <= set(v['unknown']):reject('inventory_report_rejected')
    if observation and not {'session_environment_origin_unproven','ssh_daemon_loaded_configuration_unproven'} <= set(v['unknown']):reject('inventory_report_rejected')
    for x in v['unknown']:
        if type(x) is not str:reject('inventory_report_rejected')
        if x in ALWAYS_UNKNOWN|UNKNOWN_EXTRA:continue
        parts=x.split(':')
        if len(parts)!=2 or parts[0] not in ATTEMPTS or parts[1] not in INVENTORY_ERRORS|{'observation_schema_unknown','end_recheck_unproven'}:reject('inventory_report_rejected')
    array(v['end_rechecks'],7);kinds=set()
    for r in v['end_rechecks']:
        exact(r,('kind','unchanged'),'inventory_report_rejected');boolean(r['unchanged'])
        if r['kind'] not in ATTEMPTS or not r['kind'].endswith('_end_recheck') or r['kind'] in kinds:reject('inventory_report_rejected')
        kinds.add(r['kind'])
        if r['unchanged'] is False and r['kind']+':end_recheck_unproven' not in v['unknown']:reject('inventory_report_rejected')
    required={'process_end_recheck','ssh_end_recheck','boot_end_recheck','files_end_recheck'}
    required|={k+'_end_recheck' for k in ('docker','sessions','systemd') if o[k] is not None}
    if kinds != required:reject('inventory_report_rejected')
    array(v['read_only_command_receipts'],8192)
    for r in v['read_only_command_receipts']:
        exact(r,('kind','argv_sha256','raw_sha256','bytes','executable_sha256'),'inventory_report_rejected')
        if r['kind'] not in COMMANDS:reject('inventory_report_rejected')
        for k in ('argv_sha256','raw_sha256','executable_sha256'):digest(r[k])
        integer(r['bytes'],2<<20)
    b=v['observed_budgets'];exact(b,('read_calls','content_bytes','binary_hash_bytes','distinct_content_files','elapsed_seconds'),'inventory_report_rejected')
    integer(b['read_calls'],8192);integer(b['content_bytes'],32<<20);integer(b['binary_hash_bytes'],512<<20);integer(b['distinct_content_files'],512)
    if b['read_calls'] != len(v['read_only_command_receipts']) or type(b['elapsed_seconds']) not in (int,float) or not math.isfinite(b['elapsed_seconds']) or not 0<=b['elapsed_seconds']<=120:reject('inventory_report_rejected')
    expected_visibility=['effective_sshd_config_and_protected_include_key_metadata','root_cron_and_other_subject_sessions','full_proc_visibility_and_acl_metadata','docker_socket_and_selected_inspect']
    if v['required_readonly_host_visibility'] != expected_visibility:reject('inventory_report_rejected')
    return v

def projection(a, approved, run, req_raw=None, report_raw=None, report=None, category=None, cleanup='unknown'):
    v={'protocol':'qs_host_inventory_observation_v1','source_sha':a['source_sha'],'approval_sha256':approved,'operation_id':a['operation_id'],'run_id':run,'host_class':a['host_class'],'inventory_sha256':a['inventory_sha256'],'inventory_audited_source_sha':AUDITED_SOURCE,'wrapper_sha256':a['wrapper_sha256'],'derived_request_created':req_raw is not None,'status':'observed' if report is not None else 'failed','cleanup':cleanup,'capabilities':dict.fromkeys(CAPS,False)}
    if req_raw is not None:v['derived_request_sha256']=sha(req_raw)
    if a['protocol']==OBSERVATION_PROTOCOL:
        v['protocol']='qs_host_inventory_observation_v2';v['context_mode']=CONTEXT_MODE
        v['session_origin_proven']=False;v['partial']=True
        if req_raw is not None:
            request_value=decode(req_raw);seed=canonical({k:x for k,x in request_value.items() if k!='session'})
            v['seed_request_sha256']=sha(seed)
            if 'session' not in request_value:
                v['derived_request_created']=False;v.pop('derived_request_sha256',None)
    if report is not None:
        o=report['observations'];v.update(report_sha256=sha(report_raw),unknown_count=len(report['unknown']),unknown_sha256=sha(canonical(report['unknown'])),recheck_failed_count=sum(r['unchanged'] is False for r in report['end_rechecks']),counts={'accounts':len(o['accounts']['local_subjects']) if o['accounts'] is not None else 0,'processes':o['processes']['observed'],'ssh_contexts':len(o['ssh']['matches']),'containers':len(o['docker']['containers']) if o['docker'] is not None else 0,'units':len(o['systemd']['rows']) if o['systemd'] is not None else 0,'sessions':len(o['sessions']['rows']) if o['sessions'] is not None else 0,'files':len(o['files']),'read_calls':len(report['read_only_command_receipts'])})
        if o['qs_services'] is not None:v['qs_services']=o['qs_services']
        v['qs_service_observation_sha256']=o['qs_services']['observation_sha256'] if o['qs_services'] is not None else ''
        if a['protocol']==OBSERVATION_PROTOCOL:
            so=report['session_observation']
            v.update(observed_matches_sha256=so['derived_matches_sha256'],session_identity_sha256=so['identity_sha256'],session_connection_sha256=so['connection_sha256'],host_status=so['host_status'],usedns=so['usedns'])
    if category is not None:
        if category not in ERRORS:reject('transport_output_rejected')
        v['error_category']=category
        if category=='linux_host_required':
            v['status']='unsupported';v['visibility_gap']='runner_management_channel_unknown'
    return v

PROJECTION_SCHEMA={'protocol':frozenset(('qs_host_inventory_observation_v1','qs_host_inventory_observation_v2')),'source_sha':'sha40','approval_sha256':'hash64','operation_id':'run_id','run_id':'run_id','host_class':frozenset(ROUTES),'inventory_sha256':'hash64','inventory_audited_source_sha':'sha40','wrapper_sha256':'hash64','derived_request_created':'bool','derived_request_sha256':'hash64','status':frozenset(('observed','failed','unsupported')),'cleanup':frozenset(('verified','unknown')),'capabilities':dict.fromkeys(CAPS,'bool'),'report_sha256':'hash64','unknown_count':'uint','unknown_sha256':'hash64','recheck_failed_count':'uint','counts':dict.fromkeys(('accounts','processes','ssh_contexts','containers','units','sessions','files','read_calls'),'uint'),'error_category':ERRORS,'visibility_gap':frozenset(('runner_management_channel_unknown',))}

PROJECTION_SCHEMA.update({'context_mode':frozenset((CONTEXT_MODE,)),'seed_request_sha256':'hash64','session_origin_proven':'bool','partial':'bool','observed_matches_sha256':'hash64','session_identity_sha256':'hash64','session_connection_sha256':'hash64','host_status':frozenset(('numeric_peer_from_usedns_no','host_unobserved')),'usedns':frozenset(('no','yes','unknown')),'diagnostics':DIAGNOSTIC_SCHEMA, 'qs_service_observation_sha256':'hash64_or_empty', 'qs_service_artifact_sha256':'hash64'})

def validate_diagnostics(value, cleanup):
    exact(value, DIAGNOSTIC_SCHEMA, 'transport_output_rejected')
    for key, allowed in DIAGNOSTIC_SCHEMA.items():
        if type(value[key]) is not str or value[key] not in allowed:reject('transport_output_rejected')
    stages=('remote_cleanup','local_cleanup','registration_cleanup')
    verified=all(value[k]=='verified' for k in stages)
    if (cleanup=='verified') != verified:reject('transport_output_rejected')
    if value['registration_cleanup']!='not_attempted' and any(value[k]!='verified' for k in stages[:2]):reject('transport_output_rejected')
    failure=value['cleanup_failure_stage'];category=value['cleanup_error_category']
    if (failure=='none') != (category=='none'):reject('transport_output_rejected')
    if failure=='none':
        if any(value[k]=='unknown' for k in stages):reject('transport_output_rejected')
    elif value[failure]!='unknown':reject('transport_output_rejected')
    return value

def validate_projection(v,a,approved,run,req_raw):
    base={'protocol','source_sha','approval_sha256','operation_id','run_id','host_class','inventory_sha256','inventory_audited_source_sha','wrapper_sha256','derived_request_created','status','cleanup','capabilities','derived_request_sha256'}
    observed={'report_sha256','unknown_count','unknown_sha256','recheck_failed_count','counts','qs_service_observation_sha256'}
    if type(v) is dict and 'qs_services' in v:observed.add('qs_services')
    observation=a['protocol']==OBSERVATION_PROTOCOL
    if observation:
        base|={'context_mode','seed_request_sha256','session_origin_proven','partial'}
        observed|={'observed_matches_sha256','session_identity_sha256','session_connection_sha256','host_status','usedns'}
        if type(v) is dict and v.get('derived_request_created') is False:base.discard('derived_request_sha256')
    diagnosed=type(v) is dict and 'diagnostics' in v
    if diagnosed:base.add('diagnostics')
    shapes=(base|observed,base|{'error_category'})
    if diagnosed:shapes+=(base|observed|{'error_category'},)
    if type(v) is not dict or set(v) not in shapes:reject('transport_output_rejected')
    expected=projection(a,approved,run,req_raw)
    compare=('protocol','source_sha','approval_sha256','operation_id','run_id','host_class','inventory_sha256','inventory_audited_source_sha','wrapper_sha256')
    compare+=(('context_mode','seed_request_sha256','session_origin_proven','partial') if observation else ('derived_request_created','derived_request_sha256'))
    for k in compare:
        if v[k]!=expected[k]:reject('transport_output_rejected')
    if observation:
        if type(v['derived_request_created']) is not bool or v['session_origin_proven'] is not False or v['partial'] is not True:reject('transport_output_rejected')
        if v['derived_request_created']:digest(v['derived_request_sha256'])
        if v['status']=='observed' and v['derived_request_created'] is not True:reject('transport_output_rejected')
    caps(v['capabilities'])
    if v['status'] not in ('observed','failed') or v['cleanup'] not in ('unknown','verified'):reject('transport_output_rejected')
    if diagnosed:
        validate_diagnostics(v['diagnostics'],v['cleanup'])
        if v['status']=='observed' and (v['diagnostics']['execution_stage']!='complete' or v['cleanup']!='verified'):reject('transport_output_rejected')
    elif v['cleanup']!='unknown':reject('transport_output_rejected')
    if v['status']=='observed' and set(v)!=base|observed:reject('transport_output_rejected')
    if v['status']=='failed' and 'error_category' not in v:reject('transport_output_rejected')
    if 'error_category' in v and v['error_category'] not in ERRORS:reject('transport_output_rejected')
    if 'qs_service_observation_sha256' in v and (v['qs_service_observation_sha256']!='') != ('qs_services' in v):reject('transport_output_rejected')
    if 'qs_services' in v:
        services=v['qs_services']
        if services is not None:
            api=inventory_api(INVENTORY_SOURCE_BYTES())
            try:api['validate_qs_service_observation'](services,ROUTES[a['host_class']][1])
            except api['Unknown']:reject('transport_output_rejected')
        if v['qs_service_observation_sha256'] != (services['observation_sha256'] if services is not None else ''):reject('transport_output_rejected')
    if 'counts' in v:
        exact(v['counts'],PROJECTION_SCHEMA['counts'],'transport_output_rejected')
        for val in v['counts'].values():integer(val,32768)
        for k in ('report_sha256','unknown_sha256'):digest(v[k])
        integer(v['unknown_count'],256);integer(v['recheck_failed_count'],7)
        if observation:
            for k in ('observed_matches_sha256','session_identity_sha256','session_connection_sha256'):digest(v[k])
            if v['host_status'] not in ('numeric_peer_from_usedns_no','host_unobserved') or v['usedns'] not in ('no','yes','unknown') or (v['host_status']=='numeric_peer_from_usedns_no')!=(v['usedns']=='no'):reject('transport_output_rejected')
            if v['host_status']=='host_unobserved' and (v['counts']['ssh_contexts']!=0 or v['observed_matches_sha256']!=sha(canonical([]))):reject('transport_output_rejected')
            if v['counts']['ssh_contexts']>1:reject('transport_output_rejected')
    return v

def capture(argv, *, stdin=None, timeout=30, cap=MAX_PRIVATE, env=None):
    """Bounded private buffers, process-group kill/reap; never diagnostics output."""
    if type(argv) is not list or not argv or any(type(x) is not str for x in argv):reject('transport_failed')
    child=subprocess.Popen(argv,stdin=subprocess.PIPE if stdin is not None else subprocess.DEVNULL,stdout=subprocess.PIPE,stderr=subprocess.PIPE,env=env or {'PATH':'/usr/bin:/bin','LANG':'C','LC_ALL':'C'},start_new_session=True)
    sel=selectors.DefaultSelector();buffers=[bytearray(),bytearray()];deadline=time.monotonic()+timeout
    try:
        if stdin is not None:
            # All fixed input scripts are bounded to 8 KiB, below pipe capacity.
            if len(stdin)>8192:reject('transport_budget_exceeded')
            child.stdin.write(stdin);child.stdin.close()
        for index,stream in enumerate((child.stdout,child.stderr)):
            os.set_blocking(stream.fileno(),False);sel.register(stream,selectors.EVENT_READ,index)
        while sel.get_map():
            if time.monotonic()>deadline:reject('transport_timeout')
            for key,unused in sel.select(min(.1,max(0,deadline-time.monotonic()))):
                b=os.read(key.fileobj.fileno(),65536)
                if not b:sel.unregister(key.fileobj);continue
                buffers[key.data].extend(b)
                if len(buffers[0])+len(buffers[1])>cap:reject('transport_budget_exceeded')
        code=child.wait(timeout=max(.01,deadline-time.monotonic()))
        return code,bytes(buffers[0]),bytes(buffers[1])
    except (OSError,subprocess.TimeoutExpired):reject('transport_failed')
    finally:
        if child.poll() is None:
            os.killpg(child.pid,signal.SIGKILL);child.wait()
        sel.close()
        for stream in (child.stdin,child.stdout,child.stderr):
            if stream is not None:stream.close()

def verify_handshake_key(kind,blob,fingerprint):
    if kind not in ('ssh-ed25519','ssh-rsa','ecdsa-sha2-nistp256') or type(blob) is not str or len(blob)>16384 or type(fingerprint) is not str or not re.fullmatch(r'SHA256:[A-Za-z0-9+/]{43}',fingerprint):reject('host_key_rejected')
    try:
        wire=base64.b64decode(blob,validate=True)
        size=struct.unpack('>I',wire[:4])[0]
        if size>128 or wire[4:4+size].decode('ascii')!=kind:reject('host_key_rejected')
    except (ValueError,UnicodeError,struct.error):reject('host_key_rejected')
    actual='SHA256:'+base64.b64encode(hashlib.sha256(wire).digest()).decode().rstrip('=')
    if not hmac.compare_digest(actual,fingerprint):reject('host_key_rejected')
    return 'qs-host-inventory '+kind+' '+blob+'\n'

def clean_namespace(directory,records):
    """Delete only the exact caller-owned registered files after all checks."""
    d,identity=private_dir(directory)
    try:
        if set(os.listdir(d))!=set(records):reject('private_cleanup_unknown')
        for name,(expected_sha,expected_stamp) in records.items():
            raw,actual=read_private(Path(directory)/name,MAX_REPORT)
            if sha(raw)!=expected_sha or actual!=tuple(expected_stamp):reject('private_cleanup_unknown')
        quarantine='.cleanup-'+uuid.uuid4().hex
        os.mkdir(quarantine,0o700,dir_fd=d)
        q=os.open(quarantine,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW|os.O_NONBLOCK,dir_fd=d)
        try:
            for name in records:
                raw,current=read_private(Path(directory)/name,MAX_REPORT)
                if sha(raw)!=records[name][0] or current!=tuple(records[name][1]):reject('private_cleanup_unknown')
                # Move first, then check the actual captured inode before deletion.
                # A race-replaced unknown file is retained in the quarantine.
                os.rename(name,name,src_dir_fd=d,dst_dir_fd=q)
                raw,moved=read_private(Path(directory)/quarantine/name,MAX_REPORT)
                # Rename legitimately changes ctime; every other field and bytes
                # must still match the originally registered object.
                if sha(raw)!=records[name][0] or moved[:-1]!=tuple(records[name][1])[:-1]:reject('private_cleanup_unknown')
                os.unlink(name,dir_fd=q);os.fsync(q)
            if os.listdir(q):reject('private_cleanup_unknown')
        finally:os.close(q)
        os.rmdir(quarantine,dir_fd=d)
        os.fsync(d)
        if os.listdir(d) or stamp(os.stat(directory,follow_symlinks=False))[:5]!=identity:reject('private_cleanup_unknown')
    except OSError:reject('private_cleanup_unknown')
    finally:os.close(d)
    os.rmdir(directory)
    if os.path.lexists(directory):reject('private_cleanup_unknown')

def records_for(directory):
    result={}
    for name in sorted(os.listdir(directory)):
        raw,s=read_private(Path(directory)/name,MAX_REPORT);result[name]=(sha(raw),s)
    return result

def remote_directory(directory):
    directory=Path(directory)
    if directory.parent!=Path('/tmp') or not re.fullmatch(r'qs-host-inventory-[1-9][0-9]{0,19}-[1-9][0-9]{0,3}-[1-9][0-9]{0,19}-[1-9][0-9]{0,3}-[0-9a-f]{24}',directory.name):reject('private_file_rejected')
    return directory

def remote(directory,approved,run,package_hash):
    directory=remote_directory(directory)
    raw_manifest,unused=read_private(directory/'manifest.json')
    if sha(raw_manifest)!=package_hash:reject('package_rejected')
    manifest=decode(raw_manifest);exact(manifest,ASSETS,'package_rejected')
    if set(os.listdir(directory))!=set(ASSETS)|{'manifest.json'}:reject('package_rejected')
    bodies={}
    for name in ASSETS:
        bodies[name],unused=read_private(directory/name)
        if type(manifest[name]) is not str or not HASH.fullmatch(manifest[name]) or sha(bodies[name])!=manifest[name]:reject('package_rejected')
    a=approval(bodies['approval.json'],approved,decode(bodies['request.json'])['source_sha'],run)
    if sha(bodies['action.py'])!=a['wrapper_sha256'] or sha(bodies['inventory.py'])!=INVENTORY_SHA or sha(bodies['receipt.py'])!=TRANSPORT_SHA:reject('package_rejected')
    req=decode(bodies['request.json'])
    if not directory.name.startswith('qs-host-inventory-'+a['operation_id']+'-'+run+'-'):reject('package_rejected')
    observation=a['protocol']==OBSERVATION_PROTOCOL
    if request(a,None if observation else canonical(req.get('matches')),run)!=bodies['request.json']:reject('package_rejected')
    actual_request=bodies['request.json'];request_name='request.json';session=None;api=None
    if observation:
        api=inventory_api(bodies['inventory.py'])
        actual_request,session=observed_request(req,api)
        request_name='observed-request.json'
        write_exclusive(directory,request_name,actual_request)
    # Register initial assets before starting the readonly child. Report is only private.
    initial=records_for(directory);write_exclusive(directory,'registry.json',canonical({k:[v[0],list(v[1])] for k,v in initial.items()}))
    child_env={'PATH':'/usr/bin:/bin','LANG':'C','LC_ALL':'C'}
    if observation:child_env['SSH_CONNECTION']=session['ssh_connection']
    code,out,err=capture([sys.executable,str(directory/'inventory.py'),'--request-file',str(directory/request_name),'--request-sha256',sha(actual_request)],timeout=120,cap=MAX_REPORT,env=child_env)
    if err or code!=0:
        return projection(a,approved,run,actual_request,category='inventory_process_failed')
    if observation:
        unused,current=observed_request(req,api)
        if current!=session:reject('session_observation_changed')
    for name,(expected_hash,expected_stamp) in initial.items():
        reread,current=read_private(directory/name)
        if sha(reread)!=expected_hash or current!=expected_stamp:reject('package_rejected')
    report=validate_report(out,a,approved,actual_request,run)
    write_exclusive(directory,'report.private.json',out)
    report_bytes,report_stamp=read_private(directory/'report.private.json',MAX_REPORT)
    write_exclusive(directory,'result-registry.json',canonical({'package_sha256':package_hash,'report_sha256':sha(report_bytes),'report_stamp':list(report_stamp)}))
    return projection(a,approved,run,actual_request,out,report)

def remote_cleanup(directory,package_hash):
    directory=remote_directory(directory)
    raw,unused=read_private(Path(directory)/'manifest.json')
    if sha(raw)!=package_hash:reject('private_cleanup_unknown')
    registry,unused=read_private(Path(directory)/'registry.json');v=decode(registry)
    seed_raw,unused=read_private(Path(directory)/'request.json')
    seed=decode(seed_raw)
    allowed=set(ASSETS)|{'manifest.json'}
    if seed.get('protocol')==INVENTORY_OBSERVATION_PROTOCOL:allowed.add('observed-request.json')
    if set(v)!=allowed:reject('private_cleanup_unknown')
    # No auto-adopt unknown output. Only this fixed readonly report may be present.
    expected=set(v)|{'registry.json'}
    if (Path(directory)/'result-registry.json').exists():
        result_raw,unused=read_private(Path(directory)/'result-registry.json')
        result=decode(result_raw)
        exact(result,('package_sha256','report_sha256','report_stamp'),'private_cleanup_unknown')
        report_raw,report_stamp=read_private(Path(directory)/'report.private.json',MAX_REPORT)
        if result['package_sha256']!=package_hash or result['report_sha256']!=sha(report_raw) or result['report_stamp']!=list(report_stamp):reject('private_cleanup_unknown')
        expected|={'report.private.json','result-registry.json'}
    if set(os.listdir(directory))!=expected:reject('private_cleanup_unknown')
    records={}
    for name,val in v.items():
        if type(val) is not list or len(val)!=2 or type(val[1]) is not list or len(val[1])!=9:reject('private_cleanup_unknown')
        raw,s=read_private(Path(directory)/name,MAX_REPORT)
        if sha(raw)!=val[0] or list(s)!=val[1]:reject('private_cleanup_unknown')
        records[name]=(val[0],s)
    for name in expected-set(v):
        raw,s=read_private(Path(directory)/name,MAX_REPORT);records[name]=(sha(raw),s)
    clean_namespace(directory,records)
    return {'cleanup':'verified'}

def route_file(env):
    required=('HOST_INVENTORY_SSH_HOST','HOST_INVENTORY_SSH_USERNAME','HOST_INVENTORY_SSH_PORT','HOST_INVENTORY_SSH_KEY','HOST_INVENTORY_SSH_FINGERPRINT')
    if any(type(env.get(k)) is not str or not env[k] for k in required):reject('route_rejected')
    host,user,port,key,pin=(env[k] for k in required)
    if not re.fullmatch(r'[A-Za-z0-9_.:-]{1,253}',host) or host.startswith('-') or not re.fullmatch(r'[A-Za-z0-9_.-]{1,64}',user) or not port.isdigit() or not 1<=int(port)<=65535 or not re.fullmatch(r'SHA256:[A-Za-z0-9+/]{43}',pin) or len(key)>65536 or '\x00' in key:reject('route_rejected')
    if not key.startswith('-----BEGIN ') or not key.rstrip().endswith('-----'):reject('route_rejected')
    return {'host':host,'username':user,'port':port,'key':key,'fingerprint':pin}

def source_binding(env,a):
    if env.get('GITHUB_REF')!='refs/heads/main' or env.get('GITHUB_SHA')!=a['source_sha'] or env.get('HOST_INVENTORY_VALIDATED_MAIN_SHA')!=a['source_sha'] or env.get('GITHUB_REPOSITORY')!='FangcunMount/qs-server':reject('source_binding_rejected')

def run_action(env,repo,capture_fn=capture):
    run=env.get('GITHUB_RUN_ID','')+'-'+env.get('GITHUB_RUN_ATTEMPT','')
    raw=(env.get('HOST_INVENTORY_APPROVAL_JSON','')+'\n').encode('ascii')
    approved=env.get('HOST_INVENTORY_APPROVAL_SHA256','')
    a=approval(raw,approved,env.get('GITHUB_SHA'),run);source_binding(env,a)
    script=Path(__file__).absolute()
    if sha(script.read_bytes())!=a['wrapper_sha256']:reject('source_binding_rejected')
    if a['host_class']=='runner':return projection(a,approved,run,category='linux_host_required',cleanup='verified')
    matchraw=None
    if a['protocol']==PROTOCOL:
        # V1 remains an explicit independent Match prerequisite; never fallback.
        matchfile=Path.home()/'.local/state/qs-host-inventory-approvals'/approved/'matches.json'
        match_dir,match_identity=private_dir(matchfile.parent)
        try:
            matchraw,unused=read_private(matchfile)
            if stamp(os.fstat(match_dir))[:5]!=match_identity or stamp(os.stat(matchfile.parent,follow_symlinks=False))[:5]!=match_identity:reject('private_file_changed')
        finally:os.close(match_dir)
    reqraw=request(a,matchraw,run)
    route=route_file(env)
    base=Path(env.get('RUNNER_TEMP',''))
    if not base.is_absolute() or not base.is_dir():reject('private_file_rejected')
    state=base/('qs-host-inventory-registration-'+a['operation_id']+'-'+run)
    try:state.mkdir(mode=0o700)
    except FileExistsError:reject('private_namespace_conflict')
    os.chmod(state,0o700)
    directory=state/'assets'
    directory.mkdir(mode=0o700);os.chmod(directory,0o700)
    remote_name='/tmp/qs-host-inventory-'+a['operation_id']+'-'+run+'-'+uuid.uuid4().hex[:24]
    started=time.monotonic();local_records=None;result=None
    diagnostics={'execution_stage':'asset_prepare','remote_cleanup':'not_attempted','local_cleanup':'not_attempted','registration_cleanup':'not_attempted','cleanup_failure_stage':'none','cleanup_error_category':'none'}
    def cleanup_failure(stage,category):
        diagnostics[stage]='unknown'
        if diagnostics['cleanup_failure_stage']=='none':
            diagnostics['cleanup_failure_stage']=stage;diagnostics['cleanup_error_category']=category
    registration=state/'registration.json'
    write_exclusive(state,registration.name,canonical({'protocol':'qs_host_inventory_private_registration_v1','source_sha':a['source_sha'],'approval_sha256':approved,'run_id':run,'operation_id':a['operation_id'],'local_namespace':str(directory),'remote_namespace':remote_name,'cleanup':'unknown'}))
    registration_raw,registration_stamp=read_private(registration)
    def call(argv,stdin=None,cap=MAX_PRIVATE):
        remaining=TOTAL_SECONDS-(time.monotonic()-started)
        if remaining<=0:reject('transport_timeout')
        code,out,err=capture_fn(argv,stdin=stdin,timeout=min(remaining,150),cap=cap)
        if err or code!=0:reject('transport_failed')
        return out
    try:
        bodies={'action.py':script.read_bytes(),'inventory.py':(Path(repo)/'scripts/database/compatibility-retirement-host-inventory.py').read_bytes(),'receipt.py':(Path(repo)/'scripts/dbops/receipt-transport.py').read_bytes(),'approval.json':raw,'request.json':reqraw}
        if sha(bodies['inventory.py'])!=INVENTORY_SHA or sha(bodies['receipt.py'])!=TRANSPORT_SHA:reject('package_rejected')
        for name,body in bodies.items():write_exclusive(directory,name,body)
        manifest=canonical({k:sha(v) for k,v in bodies.items()});write_exclusive(directory,'manifest.json',manifest);package_hash=sha(manifest)
        private_route={k:v for k,v in route.items() if k!='key'};write_exclusive(directory,'route.json',canonical(private_route));write_exclusive(directory,'ssh.key',route['key'].encode())
        command='/usr/bin/python3 '+str(directory/'action.py')+' known-host --route-file '+str(directory/'route.json')+' --key-type %t --key-blob %K'
        # Quoted private paths only; no interpolation of untrusted source into commands.
        if any(c.isspace() or c in '\"\'`$\\' for c in str(directory)):reject('private_file_rejected')
        config=('Host qs-host-inventory\n HostName '+route['host']+'\n User '+route['username']+'\n Port '+route['port']+'\n IdentityFile '+str(directory/'ssh.key')+'\n IdentitiesOnly yes\n PreferredAuthentications publickey\n BatchMode yes\n StrictHostKeyChecking yes\n CheckHostIP no\n HostKeyAlias qs-host-inventory\n UserKnownHostsFile /dev/null\n GlobalKnownHostsFile /dev/null\n KnownHostsCommand '+command+'\n ControlMaster no\n RequestTTY no\n ConnectTimeout 10\n LogLevel ERROR\n').encode();write_exclusive(directory,'ssh.config',config)
        local_records=records_for(directory)
        ssh=['/usr/bin/ssh','-F',str(directory/'ssh.config'),'qs-host-inventory']
        bootstrap=('import os,stat,json\np='+repr(remote_name)+'\nos.mkdir(p,0o700)\nos.chmod(p,0o700)\ns=os.stat(p,follow_symlinks=False)\nassert stat.S_ISDIR(s.st_mode) and s.st_uid==os.geteuid() and stat.S_IMODE(s.st_mode)==0o700\nprint("{\\\"created\\\":true}")\n').encode()
        diagnostics['execution_stage']='remote_bootstrap'
        if decode(call(ssh+['python3 -'],stdin=bootstrap))!={'created':True}:reject('transport_output_rejected')
        diagnostics['execution_stage']='asset_upload'
        call(['/usr/bin/scp','-F',str(directory/'ssh.config'),*[str(directory/n) for n in (*ASSETS,'manifest.json')],'qs-host-inventory:'+remote_name+'/'])
        remote_command='python3 '+remote_name+'/action.py remote --asset-dir '+remote_name+' --approval-sha '+approved+' --run '+run+' --package-sha '+package_hash
        diagnostics['execution_stage']='remote_inventory'
        out=call(ssh+[remote_command],cap=MAX_PRIVATE)
        diagnostics['execution_stage']='projection_validate'
        result=validate_projection(decode(out),a,approved,run,reqraw)
        diagnostics['execution_stage']='complete' if result['status']=='observed' else 'remote_inventory'
        diagnostics['remote_cleanup']='unknown'
        cleanup_out=call(ssh+['python3 '+remote_name+'/action.py cleanup --asset-dir '+remote_name+' --package-sha '+package_hash])
        if decode(cleanup_out)!={'cleanup':'verified'}:reject('private_cleanup_unknown')
        diagnostics['remote_cleanup']='verified'
    except Rejected as e:
        primary=result.get('error_category') if result is not None else None
        result=projection(a,approved,run,reqraw,category=primary or str(e))
        if diagnostics['remote_cleanup']=='unknown':cleanup_failure('remote_cleanup',str(e))
    finally:
        # No name-based remote rm fallback after interrupted upload or absent registry.
        if local_records is not None:
            diagnostics['local_cleanup']='unknown'
            try:
                clean_namespace(directory,local_records)
                diagnostics['local_cleanup']='verified'
            except Rejected as e:cleanup_failure('local_cleanup',str(e))
            except OSError:cleanup_failure('local_cleanup','private_cleanup_unknown')
    if result is None:reject('action_fixed_failure')
    if diagnostics['remote_cleanup']=='verified' and diagnostics['local_cleanup']=='verified':
        diagnostics['registration_cleanup']='unknown'
        try:
            current,current_stamp=read_private(registration)
            if current!=registration_raw or current_stamp!=registration_stamp:reject('private_cleanup_unknown')
            os.unlink(registration)
            state_fd=os.open(state,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW)
            try:os.fsync(state_fd)
            finally:os.close(state_fd)
            if os.path.lexists(registration):reject('private_cleanup_unknown')
            os.rmdir(state)
            base_fd=os.open(base,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW)
            try:os.fsync(base_fd)
            finally:os.close(base_fd)
            if os.path.lexists(state):reject('private_cleanup_unknown')
            diagnostics['registration_cleanup']='verified'
        except Rejected as e:cleanup_failure('registration_cleanup',str(e))
        except OSError:cleanup_failure('registration_cleanup','private_cleanup_unknown')
    result['cleanup']='verified' if all(diagnostics[k]=='verified' for k in ('remote_cleanup','local_cleanup','registration_cleanup')) else 'unknown'
    result['diagnostics']=diagnostics
    if result['cleanup']!='verified':
        result['status']='failed'
        result.setdefault('error_category','private_cleanup_unknown')
        # Preserve the primary fixed error. Cleanup facts never create authority.
    return result

def service_artifact(value, env):
    # Only this closed public service projection survives the old raw/private
    # report cleanup. It is input metadata, not an approval, lease or fence.
    closed=dict(value); services=closed.pop('qs_services',None)
    if services is None or closed.get('status')!='observed' or closed.get('cleanup')!='verified':return closed
    api=inventory_api(INVENTORY_SOURCE_BYTES())
    try:api['validate_qs_service_observation'](services,ROUTES[closed['host_class']][1])
    except api['Unknown']:reject('inventory_report_rejected')
    payload={'protocol':'qs_service_descriptor_observation_v1',
        **{k:closed[k] for k in ('source_sha','operation_id','run_id','host_class','inventory_sha256','wrapper_sha256','approval_sha256','report_sha256')},
        'actual_projection_sha256':sha(canonical(closed)), 'service_observation':services,
        'capabilities':dict.fromkeys(CAPS,False)}
    raw=canonical(payload); base=Path(env.get('RUNNER_TEMP',''))
    if not base.is_absolute() or not base.is_dir():reject('private_file_rejected')
    name='qs-service-observation-'+closed['operation_id']+'-'+closed['run_id']
    directory=base/name
    try:directory.mkdir(mode=0o700)
    except FileExistsError:reject('private_namespace_conflict')
    fd,_=private_dir(directory);os.close(fd)
    write_exclusive(directory,'service-observation.private.json',raw)
    path=directory/'service-observation.private.json'
    actual,_=read_private(path)
    if actual!=raw:reject('private_file_changed')
    closed['qs_service_artifact_sha256']=sha(raw)
    output=env.get('GITHUB_OUTPUT')
    if type(output) is not str or not output:reject('action_input_rejected')
    with open(output,'a',encoding='utf-8') as handle:
        handle.write('service_observation_path='+str(path)+'\nservice_observation_artifact='+name+'\n')
    return closed

def emit(v,transport):
    v=dict(v)
    if v.pop("qs_services",None) is not None:reject("transport_output_rejected")
    raw=Path(transport).read_bytes()
    if sha(raw)!=TRANSPORT_SHA:reject('package_rejected')
    # Compile the exact pinned bytes, not a second path-based import.
    namespace={'__name__':'owned_receipt_transport'}
    exec(compile(raw,'<pinned-receipt-transport>','exec'),namespace)
    print(namespace['encode_armored_receipt'](v,schema=PROJECTION_SCHEMA),end='\n')

def main():
    class Parser(argparse.ArgumentParser):
        def error(self,unused):reject('action_input_rejected')
    try:
        p=Parser();s=p.add_subparsers(dest='command',required=True)
        run=s.add_parser('run');run.add_argument('--repo',required=True)
        r=s.add_parser('remote');r.add_argument('--asset-dir',required=True);r.add_argument('--approval-sha',required=True);r.add_argument('--run',required=True);r.add_argument('--package-sha',required=True)
        c=s.add_parser('cleanup');c.add_argument('--asset-dir',required=True);c.add_argument('--package-sha',required=True)
        h=s.add_parser('known-host');h.add_argument('--route-file',required=True);h.add_argument('--key-type',required=True);h.add_argument('--key-blob',required=True)
        args=p.parse_args()
        if args.command=='known-host':
            raw,unused=read_private(args.route_file);route=decode(raw);exact(route,('host','username','port','fingerprint'),'route_rejected')
            if args.key_blob=='NONE' and args.key_type=='NONE':return 0
            print(verify_handshake_key(args.key_type,args.key_blob,route['fingerprint']),end='');return 0
        if args.command=='remote':print(canonical(remote(args.asset_dir,args.approval_sha,args.run,args.package_sha)).decode(),end='');return 0
        if args.command=='cleanup':print(canonical(remote_cleanup(args.asset_dir,args.package_sha)).decode(),end='');return 0
        value=service_artifact(run_action(dict(os.environ),args.repo),dict(os.environ));emit(value,Path(args.repo)/'scripts/dbops/receipt-transport.py');return 0 if value['status']=='observed' and value['cleanup']=='verified' else 1
    except Rejected as e:
        # Fixed category only. No argv, file paths, route, captured output or input.
        print(canonical({'protocol':'qs_host_inventory_action_error_v1','error_category':str(e),'capabilities':dict.fromkeys(CAPS,False)}).decode(),end='');return 1
    except (OSError,ValueError,TypeError,KeyError,UnicodeError,RecursionError):
        print(canonical({'protocol':'qs_host_inventory_action_error_v1','error_category':'action_fixed_failure','capabilities':dict.fromkeys(CAPS,False)}).decode(),end='');return 1

if __name__=='__main__':sys.exit(main())

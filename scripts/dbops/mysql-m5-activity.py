"""Separate bounded M5 activity observation; never schema100 readiness/freeze."""
import importlib.util
from pathlib import Path

def load_profile():
    spec=importlib.util.spec_from_file_location('m5_activity_profile',Path(__file__).with_name('mysql-metadata-profile.py'))
    module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module);return module

profile=load_profile()
ACTIVITY=True
SHA,RUN,HASH=profile.SHA,profile.RUN,profile.HASH
PROFILE_KEYS=profile.PROFILE_KEYS
runtime_module=profile.runtime_module
receipt_transport_module=profile.receipt_transport_module
capture_private=profile.capture_private
COUNTS=frozenset(('qs_threads','qs_active_threads','qs_transactions','qs_modifying_transactions','qs_metadata_locks_granted','qs_metadata_locks_pending','qs_enabled_events'))
STAGES=frozenset(('input','connect','identity_before','identity_after','global_grants','ledger','instrument','thread_binding','sample_time','none','connection_release','database_release','transport'))|COUNTS
ERRORS=frozenset(profile.ERRORS|{'query_failed','query_server_timeout','query_server_interrupted','query_permission_denied','query_deadline_exceeded','query_canceled','query_mysql_error','query_network_timeout','query_network_error','query_invalid_connection','query_bad_connection','query_unexpected_eof','query_eof','metadata_invalid','metadata_truncated','metadata_release_failed','storage_observation_unavailable','grants_unverified'})
RECEIPT_SCHEMA={
    'format_version':'uint','source_sha':'sha40','run_id':'run_id','expected_target_hash':'hash64','source_target_hash':'hash64','mysql_identity_sha256':'hash64',
    'read_only':'bool','m5_activity_only':'bool','freeze_verified':'bool','observations_atomic':'bool','complete':'bool','query_count':'uint',
    'connection_released':'nullable_bool','database_released':'nullable_bool','error_category':ERRORS,'failure_stage':STAGES,'query_mysql_error_number':'uint',
    'measurements':{'mysql_version':'uint','mysql_dirty':'bool','counts':{k:'uint'for k in COUNTS},'sampled_at_epoch_microseconds':'uint'},
}
FAILURE_RECEIPT=dict(format_version=1,read_only=True,m5_activity_only=True,freeze_verified=False,observations_atomic=False,complete=False,query_count=0,connection_released=None,database_released=None,error_category='profile_transport_or_receipt_failed',failure_stage='transport')

def stage_failure(env,category):
    value=dict(FAILURE_RECEIPT);value['error_category']=category if category in ERRORS else 'profile_transport_or_receipt_failed'
    for key,source,expr in [('source_sha','PROFILE_SOURCE_SHA',SHA),('run_id','PROFILE_RUN_ID',RUN),('expected_target_hash','PROFILE_EXPECTED_TARGET_HASH',HASH)]:
        if expr.fullmatch(env.get(source,'')):value[key]=env[source]
    return value

def safe_receipt(module,raw,code,sha,run,expected):
    value=module.load_json_bytes(raw)
    receipt_transport_module().encode_armored_receipt(value,schema=RECEIPT_SCHEMA)
    required=set(RECEIPT_SCHEMA)-{'source_target_hash','mysql_identity_sha256','measurements','query_mysql_error_number'}
    if not required<=set(value) or value['format_version']!=1 or value['source_sha']!=sha or value['run_id']!=run or value['expected_target_hash']!=expected:
        raise ValueError('m5_activity_binding')
    if value['read_only'] is not True or value['m5_activity_only'] is not True or value['freeze_verified'] is not False or value['observations_atomic'] is not False or value['query_count']>14:
        raise ValueError('m5_activity_scope')
    if value['complete']:
        if code!=0 or value['error_category']!='none' or value['failure_stage']!='none' or value['source_target_hash']!=expected or 'mysql_identity_sha256'not in value or value['query_count']!=14 or any(value[k] is not True for k in ['connection_released','database_released']) or 'query_mysql_error_number'in value:
            raise ValueError('m5_activity_incomplete')
        m=value['measurements']
        if set(m)!=set(RECEIPT_SCHEMA['measurements']) or set(m['counts'])!=COUNTS or m['sampled_at_epoch_microseconds']==0:
            raise ValueError('m5_activity_invalid')
    elif code==0 or value['error_category']=='none' or value['failure_stage']=='none' or 'measurements'in value:
        raise ValueError('m5_activity_failure_retains_measurements')
    return value

def execute_native(binary,env,module):
    sha,run,expected=(env.get(k,'')for k in ['PROFILE_SOURCE_SHA','PROFILE_RUN_ID','PROFILE_EXPECTED_TARGET_HASH'])
    module.validate_source(env)
    if not SHA.fullmatch(sha) or not RUN.fullmatch(run) or not HASH.fullmatch(expected):raise ValueError('input_invalid')
    path=Path(binary)
    if not path.is_absolute() or path.is_symlink() or not path.is_file() or env.get('MYSQL_DATABASE')!='qs':raise ValueError('m5_activity_binary_or_database')
    private={k:env.get(k,'')for k in PROFILE_KEYS};private['MYSQL_PORT']=private['MYSQL_PORT']or'3306'
    code,raw=capture_private([str(path),'--m5-activity'],env=private)
    return code,safe_receipt(module,raw,code,sha,run,expected)

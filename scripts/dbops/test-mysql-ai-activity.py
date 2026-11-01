import importlib.util
import json
from pathlib import Path
import unittest

HERE=Path(__file__).parent
def load(name,file):
    spec=importlib.util.spec_from_file_location(name,HERE/file);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m
activity=load('ai_activity_test','mysql-ai-activity.py')
bridge=load('ai_activity_bridge_test','mysql-metadata-profile-ssh.py')
SHA,RUN,HASH='a'*40,'12345-1','b'*64
def receipt():
    return dict(format_version=1,source_sha=SHA,run_id=RUN,expected_target_hash=HASH,source_target_hash=HASH,mysql_identity_sha256='04f119402a8b7704f84df45bd3e3fb182eaf1ea05febe7fbad38f66c76740fc3',
                read_only=True,ai_activity_only=True,freeze_verified=False,observations_atomic=False,complete=True,query_count=24,
                connection_released=True,database_released=True,error_category='none',failure_stage='none',
                measurements=dict(schema_head='0043_native_transaction_limit',target_present=True,target_rows=7,target_ddl_sha256='c'*64,global_metadata_grants_verified=True,counts={k:1 for k in activity.COUNTS},sampled_at_epoch_microseconds=1793466000000000))
class AIActivityTest(unittest.TestCase):
    def check(self,value,code=0):return activity.safe_receipt(activity.runtime_module(),json.dumps(value),code,SHA,RUN,HASH)
    def test_busy_and_dirty_observation_never_claims_freeze_or_schema100(self):
        x=receipt();x['measurements']['counts']['ai_modifying_transactions']=3
        # Sequential metadata reads are not a distributed/atomic snapshot;
        # a later count can exceed an earlier total. Preserve actual observations.
        safe=self.check(x);self.assertTrue(safe['complete']);self.assertFalse(safe['freeze_verified']);self.assertEqual(safe['measurements']['target_rows'],7)
        self.assertEqual(safe['measurements']['counts']['ai_modifying_transactions'],3)
    def test_scope_partial_unknown_and_private_fields_rejected(self):
        for changes in [dict(source_sha='d'*40),dict(freeze_verified=True),dict(observations_atomic=True),dict(query_count=14),dict(connection_released=False),dict(database_released=None),dict(raw_sql='private')]:
            x=receipt();x.update(changes)
            with self.assertRaises((ValueError,KeyError)):self.check(x)
        for key in ['mysql_identity_sha256','measurements']:
            x=receipt();del x[key]
            with self.assertRaises((ValueError,KeyError)):self.check(x)
        x=receipt();del x['measurements']['counts']['ai_metadata_locks_pending']
        with self.assertRaises(ValueError):self.check(x)
    def test_failed_query_cannot_keep_partial_counts(self):
        x=receipt();x.update(complete=False,error_category='query_permission_denied',failure_stage='ai_metadata_locks_pending',query_count=10,query_mysql_error_number=1142)
        with self.assertRaises(ValueError):self.check(x,1)
        del x['measurements'];safe=self.check(x,1);self.assertFalse(safe['complete']);self.assertNotIn('measurements',safe)
    def test_explicit_separate_member_and_mode(self):
        self.assertEqual(bridge.mode_members(ai_activity=True),bridge.MEMBERS|{'mysql-ai-activity.py'})
        self.assertNotIn('mysql-ai-activity.py',bridge.mode_members())
        for kwargs in [dict(ai_activity=True,readiness=True),dict(ai_activity=True,inventory=True),dict(ai_activity=True,activity=True)]:
            with self.assertRaises(ValueError):bridge.mode_members(**kwargs)
    def test44_absence_is_explicit_and_not_a_fake_zero(self):
        x=receipt();x['query_count']=22;m=x['measurements'];m.update(schema_head='0044_empty_publisher',target_present=False)
        del m['target_rows'];del m['target_ddl_sha256']
        self.assertTrue(self.check(x)['complete'])
        for changes in [dict(target_rows=0),dict(target_present=True),dict(global_metadata_grants_verified=False)]:
            bad=json.loads(json.dumps(x));bad['measurements'].update(changes)
            with self.assertRaises((ValueError,KeyError)):self.check(bad)
    def test_source_and_scope_transport_keeps_credentials_in_stdin(self):
        import io,tarfile,hashlib,tempfile
        from unittest.mock import patch
        private='fixture-not-real-credential'
        env=dict(MYSQL_HOST='fixture.invalid',MYSQL_PORT='3306',MYSQL_USERNAME='fixture',MYSQL_PASSWORD=private,MYSQL_DATABASE='ai',
                 PROFILE_SOURCE_SHA=SHA,PROFILE_RUN_ID=RUN,PROFILE_EXPECTED_TARGET_HASH=HASH,PROFILE_OPERATION='ai-activity-admin',PROFILE_DATABASE='mysql',PROFILE_ARCHIVE_INPUT='')
        raw=io.BytesIO()
        with tarfile.open(fileobj=raw,mode='w:gz') as t:
            for name in bridge.AI_ACTIVITY_MEMBERS:
                item=tarfile.TarInfo(name);item.size=1;t.addfile(item,io.BytesIO(b'x'))
        env['PROFILE_PACKAGE_SHA256']=hashlib.sha256(raw.getvalue()).hexdigest()
        def capture(args,**kw):
            self.assertNotIn(private,str(args));self.assertNotIn(private,str(kw['env']))
            header=json.loads(kw['payload'].split(b'\n',1)[0]);self.assertEqual(header['env']['MYSQL_PASSWORD'],private)
            return 0,activity.receipt_transport_module().encode_armored_receipt(receipt(),schema=activity.RECEIPT_SCHEMA).encode()
        with tempfile.TemporaryDirectory() as d:
            path=Path(d)/'package.tar.gz';path.write_bytes(raw.getvalue());path.chmod(0o600)
            with patch.object(bridge,'trusted_ssh',return_value=(['ssh'],{})),patch.object(activity,'capture_private',side_effect=capture):
                code,sealed=bridge.dispatch(path,env,activity,Path(d),ai_activity=True)
            self.assertEqual(code,0);self.assertNotIn(private,sealed)
            for changes in [dict(PROFILE_OPERATION='m5-activity-admin'),dict(PROFILE_DATABASE='all')]:
                with patch.object(bridge,'trusted_ssh') as ssh:
                    with self.assertRaises(Exception):bridge.dispatch(path,dict(env,**changes),activity,Path(d),ai_activity=True)
                    ssh.assert_not_called()
if __name__=='__main__':unittest.main()

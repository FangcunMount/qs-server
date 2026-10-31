import importlib.util
import json
from pathlib import Path
import unittest

HERE=Path(__file__).parent
def load(name,file):
    spec=importlib.util.spec_from_file_location(name,HERE/file);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m
activity=load('m5_activity_test','mysql-m5-activity.py')
bridge=load('m5_activity_bridge_test','mysql-metadata-profile-ssh.py')
SHA,RUN,HASH='a'*40,'12345-1','b'*64
def receipt():
    return dict(format_version=1,source_sha=SHA,run_id=RUN,expected_target_hash=HASH,source_target_hash=HASH,mysql_identity_sha256='c'*64,
                read_only=True,m5_activity_only=True,freeze_verified=False,observations_atomic=False,complete=True,query_count=14,
                connection_released=True,database_released=True,error_category='none',failure_stage='none',
                measurements=dict(mysql_version=111,mysql_dirty=True,counts={k:1 for k in activity.COUNTS},sampled_at_epoch_microseconds=1793466000000000))
class ActivityTest(unittest.TestCase):
    def check(self,value,code=0):return activity.safe_receipt(activity.runtime_module(),json.dumps(value),code,SHA,RUN,HASH)
    def test_busy_and_dirty_observation_never_claims_freeze_or_schema100(self):
        x=receipt();x['measurements']['counts']['qs_modifying_transactions']=3
        # Sequential metadata reads are not a distributed/atomic snapshot;
        # a later count can exceed an earlier total. Preserve actual observations.
        safe=self.check(x);self.assertTrue(safe['complete']);self.assertFalse(safe['freeze_verified']);self.assertTrue(safe['measurements']['mysql_dirty'])
        self.assertEqual(safe['measurements']['counts']['qs_modifying_transactions'],3)
    def test_scope_partial_unknown_and_private_fields_rejected(self):
        for changes in [dict(source_sha='d'*40),dict(freeze_verified=True),dict(observations_atomic=True),dict(query_count=12),dict(connection_released=False),dict(database_released=None),dict(raw_sql='private')]:
            x=receipt();x.update(changes)
            with self.assertRaises((ValueError,KeyError)):self.check(x)
        for key in ['mysql_identity_sha256','measurements']:
            x=receipt();del x[key]
            with self.assertRaises((ValueError,KeyError)):self.check(x)
        x=receipt();del x['measurements']['counts']['qs_metadata_locks_pending']
        with self.assertRaises(ValueError):self.check(x)
    def test_failed_query_cannot_keep_partial_counts(self):
        x=receipt();x.update(complete=False,error_category='query_permission_denied',failure_stage='qs_metadata_locks_pending',query_count=10,query_mysql_error_number=1142)
        with self.assertRaises(ValueError):self.check(x,1)
        del x['measurements'];safe=self.check(x,1);self.assertFalse(safe['complete']);self.assertNotIn('measurements',safe)
    def test_explicit_separate_member_and_mode(self):
        self.assertEqual(bridge.mode_members(activity=True),bridge.MEMBERS|{'mysql-m5-activity.py'})
        self.assertNotIn('mysql-m5-activity.py',bridge.mode_members())
        for kwargs in [dict(activity=True,readiness=True),dict(activity=True,inventory=True)]:
            with self.assertRaises(ValueError):bridge.mode_members(**kwargs)
if __name__=='__main__':unittest.main()

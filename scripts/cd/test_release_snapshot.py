"""Exercise the no-write inventory boundary and reject changing host inventory."""
import importlib.util
import json
from pathlib import Path
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('release_snapshot', Path(__file__).with_name('release-snapshot.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class SnapshotTests(unittest.TestCase):
    def test_inventory_exports_only_selected_fields_and_never_invokes_writes(self):
        calls = []
        def command(*args):
            calls.append(args)
            if args[1] == 'version': return '29.0'
            if args[1] == 'ps': return 'abc qs-apiserver\ndef mysql\n'
            if args[1] == 'inspect':
                return json.dumps({'Image':'sha256:'+'a'*64, 'Config': {'Image':'qs-api:'+'b'*40, 'Env':['PASSWORD=private']}, 'State':{'Running':True}, 'Mounts':[]})
            if args[1:3] == ('image','inspect'):
                return json.dumps({'Os':'linux','Architecture':'amd64','RepoDigests':['qs-api@sha256:'+'c'*64],'Config':{'Labels':{'org.opencontainers.image.revision':'b'*40},'Env':['PASSWORD=private']}})
            self.fail('Unapproved command')
        with patch.object(module, 'command', command), patch.object(module.Path,'is_file',return_value=False):
            result = module.collect()
        self.assertEqual(len(result['containers']),1)
        self.assertEqual(result['containers'][0]['source_sha'],'b'*40)
        self.assertNotIn('private',json.dumps(result))
        self.assertTrue(result['read_only'])
        self.assertFalse(result['business_acceptance_verified'])
        self.assertTrue(all(x[1] in {'version','ps','inspect','image'} for x in calls))
        self.assertTrue(all('.Env' not in ' '.join(x) for x in calls))

    def test_changed_inventory_is_not_a_rollback_baseline(self):
        n = 0
        def command(*args):
            nonlocal n
            if args[1]=='version': return '29.0'
            if args[1]=='ps':
                n+=1
                return '' if n==1 else 'abc qs-apiserver'
            self.fail('Unexpected access')
        with patch.object(module,'command',command):
            with self.assertRaisesRegex(ValueError,'Runtime changed'):
                module.collect()


if __name__=='__main__': unittest.main()

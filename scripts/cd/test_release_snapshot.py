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
            if args[1] == 'exec':
                if '--version=true' in args: return 'gitCommit: '+ 'b'*40 +'\n'
                if 'sha256sum' in args: return 'c'*64+'  /proc/1/exe\n'+'c'*64+'  /app/qs-apiserver\n'
                if 'wget' in args: return ''
                self.fail('Unexpected exec')
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
        self.assertEqual(result['containers'][0]['reported_git_commit'],'b'*40)
        self.assertTrue(result['containers'][0]['ready'])
        self.assertTrue(result['read_only'])
        self.assertFalse(result['business_acceptance_verified'])
        self.assertTrue(all(x[1] in {'version','ps','inspect','image','exec'} for x in calls))
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



    def test_sudo_mode_elevates_only_docker_and_not_python(self):
        def command(*args):
            self.assertEqual(args[:3], ('sudo','-n','docker'))
            if args[3]=='version': return '29.0'
            if args[3]=='ps': return ''
            self.fail('Unexpected command')
        with patch.object(module,'command',command):
            with self.assertRaisesRegex(ValueError,'No release containers'):
                module.collect(sudo_docker=True)

    def test_unreadable_bind_is_unknown_and_keeps_image_identity(self):
        def command(*args):
            if args[1]=='version': return '29.0'
            if args[1]=='ps': return 'abc nginx'
            if args[1]=='inspect': return json.dumps({'Image':'sha256:'+'a'*64,'Config':{'Image':'nginx'},'State':{'Running':True},'Mounts':[{'Source':'/private/config.yaml','Destination':'/config.yaml','Type':'bind','RW':False}]})
            if args[1]=='image': return json.dumps({'Os':'linux','Architecture':'amd64','RepoDigests':[],'Config':{}})
            self.fail('Unexpected command')
        with patch.object(module,'command',command), patch.object(module.Path,'is_file',side_effect=PermissionError):
            result=module.collect()
        self.assertFalse(result['containers'][0]['mounts'][0]['fingerprint_available'])
        self.assertFalse(result['ai_release_state_available'])
        self.assertTrue(result['containers'][0]['running'])

    def test_loaded_image_mapping_requires_every_existing_release_binding(self):
        import tempfile
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            identity = {'config_sha256': 'sha256:'+'a'*64, 'private': 'not-exported'}
            manifest = dict(revision='b'*40, image_id='sha256:'+'a'*64, archive_sha256='c'*64,
                            digest='sha256:'+'d'*64, image_identity=identity)
            receipt = dict(version=1, revision=manifest['revision'], source_image_id=manifest['image_id'],
                           loaded_image_id='sha256:'+'e'*64, archive_sha256=manifest['archive_sha256'],
                           registry_digest=manifest['digest'], identity=identity)
            (root/'manifest.json').write_text(json.dumps(manifest))
            (root/'image.env').write_text('QS_AI_IMAGE='+receipt['loaded_image_id']+'\n')
            (root/'loaded-image.json').write_text(json.dumps(receipt))
            result = module.ai_release_metadata(root)
            self.assertTrue(result['loaded_scalar_binding_valid'])
            self.assertNotIn('not-exported', json.dumps(result))
            receipt['archive_sha256']='f'*64
            (root/'loaded-image.json').write_text(json.dumps(receipt))
            self.assertFalse(module.ai_release_metadata(root)['loaded_scalar_binding_valid'])
            (root/'loaded-image.json').unlink()
            self.assertFalse(module.ai_release_metadata(root)['loaded_receipt_exists'])
            (root/'image.env').write_text('PASSWORD=private\n')
            result=module.ai_release_metadata(root)
            self.assertFalse(result['image_env_reference_valid'])
            self.assertNotIn('private',json.dumps(result))

    def test_material_read_uses_only_the_installed_digest_selected_verifier(self):
        tool='/opt/fangcun-m4/material-tools/'+module.MATERIAL_TOOL_SHA+'.py'
        value=dict(status='verified', read_only=True, activated=False, role='ai', host='serverA',
                   revision='m4-readable-20261028-v1', material_root='/var/lib/fangcun-m4-materials', receipt_sha256='a'*64)
        with patch.object(module,'verify_material_tool',return_value=tool), patch.object(module.subprocess,'run',return_value=module.subprocess.CompletedProcess([],0,json.dumps(value),'')) as run:
            self.assertTrue(module.material_observation('ai',value['revision'],'serverA')['verified'])
            self.assertEqual(run.call_args.args[0][:7],['sudo','-n','/usr/bin/python3','-I',tool,'--verify-installed','--host'])
            value['activated']=True
            run.return_value=module.subprocess.CompletedProcess([],0,json.dumps(value),'')
            self.assertFalse(module.material_observation('ai',value['revision'],'serverA')['verified'])
            run.return_value=module.subprocess.CompletedProcess([],1,'','sudo: a password is required')
            result=module.material_observation('ai',value['revision'],'serverA')
            self.assertFalse(result['verified'])
            self.assertEqual(result['error'],'password_required')


if __name__=='__main__': unittest.main()

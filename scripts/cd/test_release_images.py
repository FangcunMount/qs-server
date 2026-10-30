import importlib.util
import json
from pathlib import Path
import unittest

spec=importlib.util.spec_from_file_location('release_images',Path(__file__).with_name('verify-release-images.py'))
module=importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class ReleaseImageTests(unittest.TestCase):
    def test_all_six_images_use_only_fixed_digest_pulls_and_inspection(self):
        images={k:dict(sha='a'*40,digest='sha256:'+'b'*64,config='sha256:'+'c'*64) for k in module.ALLOWED}
        calls=[]
        def run(args, **kwargs):
            calls.append(args)
            if args[1]=='pull': return b''
            return json.dumps([{'Id':'sha256:'+'c'*64,'Os':'linux','Architecture':'amd64','Config':{'Labels':{'org.opencontainers.image.revision':'a'*40},'Env':['PASSWORD=private']}}]).encode()
        result=module.verify(images,'registry.example','namespace',run)
        self.assertEqual(len(result['images']),6)
        self.assertTrue(result['verified'])
        self.assertFalse(result['deployment_verified'])
        self.assertNotIn('private',json.dumps(result))
        self.assertTrue(all(a[1:2]==['pull'] or a[1:3]==['image','inspect'] for a in calls))
        self.assertTrue(all('@sha256:' in a[-1] for a in calls))
        images['iam']['digest']='latest'
        with self.assertRaises(ValueError):module.verify(images,'registry.example','namespace',run)
        self.assertEqual(len(calls),12)

    def test_image_config_mismatch_is_not_accepted(self):
        images={k:dict(sha='a'*40,digest='sha256:'+'b'*64,config='sha256:'+'c'*64) for k in module.ALLOWED}
        def run(args, **kwargs):
            return b'' if args[1]=='pull' else json.dumps([{'Id':'sha256:'+'d'*64,'Os':'linux','Architecture':'amd64','Config':{}}]).encode()
        with self.assertRaisesRegex(ValueError,'configuration'):module.verify(images,'registry.example','namespace',run)


if __name__=='__main__': unittest.main()

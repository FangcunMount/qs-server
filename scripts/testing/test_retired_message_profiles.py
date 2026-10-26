"""The retired process proof cannot allocate containers, builds or temp state."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT=Path(__file__).resolve().parents[2]

class RetiredProfile(unittest.TestCase):
    def test_old_process_proof_refuses_before_external_commands(self):
        for arguments in ([],['/unused-sdk']):
            with self.subTest(arguments=arguments), tempfile.TemporaryDirectory() as raw:
                root=Path(raw);marker=root/'external-call'
                for command in ('docker','go','git','mktemp'):
                    p=root/command;p.write_text('#!/bin/sh\nprintf invoked >> "$PROFILE_MARKER"\nexit 99\n');p.chmod(0o700)
                result=subprocess.run(['bash',str(ROOT/'scripts/testing/m4-standard-process-proof.sh'),*arguments],capture_output=True,text=True,timeout=5,env={**os.environ,'PATH':str(root)+os.pathsep+os.environ['PATH'],'PROFILE_MARKER':str(marker)})
                self.assertEqual(result.returncode,2,result.stderr)
                self.assertIn('retired',result.stderr)
                self.assertFalse(marker.exists())

if __name__=='__main__':unittest.main()

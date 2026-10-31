"""Stage hash-bound files; only existing ServerD admin authority can install/verify."""
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import subprocess

HELPER_SHA = '53ec1f1e093aca37ad671a50d37203b87757f2daea7f78d03e96df7c51474324'
INSTALL_SHA = 'b05a3ce9146828e5cbfc7f5fae3fb949a1356ee85b9861c5a0a21f8bd15b1b92'
operation = os.environ['RETENTION_OPERATION']
host = os.environ['DEPLOY_NODE_HOSTNAME']
run = os.environ['GITHUB_RUN_ID']
assert operation in ('stage', 'install-worker', 'verify-worker')
assert host in ('serverA', 'serverB', 'serverD') and run.isdecimal()
assert operation == 'stage' or host == 'serverD'
directory = '/tmp/qlume-retention-install-' + run + '-' + host
ssh = ['ssh', '-F', os.environ['RUNNER_SSH_CONFIG'], '-o', 'ConnectTimeout=20', os.environ['RUNNER_SSH_ALIAS']]


def remote(code, *args):
    result = subprocess.run([*ssh, shlex.join(['python3', '-c', code, *args])],
        text=True, capture_output=True, timeout=90)
    if result.returncode:
        raise SystemExit('Bounded retention operation failed; inspect its current phase, no raw stderr exported')
    return result.stdout


helper = Path('.retention-qs/scripts/cd/image-retention.py')
installer = Path('.retention-infra/scripts/ops/install-image-retention.sh')
assert hashlib.sha256(helper.read_bytes()).hexdigest() == HELPER_SHA
assert hashlib.sha256(installer.read_bytes()).hexdigest() == INSTALL_SHA
remote('import os,sys;os.mkdir(sys.argv[1],0o700)', directory)
for source, name in ((helper, 'image-retention.py'), (installer, 'install-image-retention.sh')):
    subprocess.run(['scp', '-F', os.environ['RUNNER_SSH_CONFIG'], str(source),
        os.environ['RUNNER_SSH_ALIAS'] + ':' + directory + '/' + name], check=True,
        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=30)
receipt = json.loads(remote('''import hashlib,json,os,pathlib,pwd,socket,stat,sys
p=pathlib.Path(sys.argv[1]); info=p.lstat()
assert stat.S_ISDIR(info.st_mode) and stat.S_IMODE(info.st_mode)==0o700 and info.st_uid==os.getuid()
assert socket.gethostname().split('.')[0]==sys.argv[2]
hashes={n:hashlib.sha256((p/n).read_bytes()).hexdigest() for n in ('image-retention.py','install-image-retention.sh')}
print(json.dumps(dict(host=sys.argv[2],directory=str(p),deploy_account=pwd.getpwuid(os.getuid()).pw_name,sha256=hashes)))
''', directory, host))
assert receipt['sha256'] == {'image-retention.py': HELPER_SHA, 'install-image-retention.sh': INSTALL_SHA}
assert re.fullmatch(r'[a-z_][a-z0-9_-]{0,31}', receipt['deploy_account'])
receipt.update(operation=operation, business_database_operations=False, application_restarted=False,
               installed=False, production_cleanup_executed=False)
receipt['administrator_command'] = shlex.join(['sudo', 'bash', directory + '/install-image-retention.sh',
    host, receipt['deploy_account'], directory + '/image-retention.py', '--apply'])
if operation == 'install-worker':
    remote('''import hashlib,pathlib,subprocess,sys
p=pathlib.Path(sys.argv[1]);assert hashlib.sha256((p/'install-image-retention.sh').read_bytes()).hexdigest()==sys.argv[3]
subprocess.run(['sudo','-n','/bin/bash',str(p/'install-image-retention.sh'),'serverD',sys.argv[2],str(p/'image-retention.py'),'--apply'],check=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,timeout=70)
''', directory, receipt['deploy_account'], INSTALL_SHA)
    receipt['installed'] = True
elif operation == 'verify-worker':
    receipt['cleanup'] = json.loads(remote('''import fcntl,hashlib,json,pathlib,subprocess
tool=pathlib.Path('/usr/local/libexec/fangcun/image-retention.py')
assert hashlib.sha256(tool.read_bytes()).hexdigest()=='53ec1f1e093aca37ad671a50d37203b87757f2daea7f78d03e96df7c51474324'
lock=pathlib.Path('/var/lib/fangcun-image-retention/deploy.lock');inode=lock.stat().st_ino
with lock.open('r') as stream:
 fcntl.flock(stream,fcntl.LOCK_EX|fcntl.LOCK_NB);assert lock.stat().st_ino==inode
 current='sha256:511d750af70bcca1206c691d57317a5d88a782b25a940df6f0d70b7c124d49dc'
 fallback='sha256:b637bd1a84a4657b8452332d72629288c54665d9e0bd7faf5d1a1757dd9a72b6'
 command=['sudo','-n','/usr/bin/python3','-I',str(tool),'--service','qs-worker','--image-ref',current,'--protect-image-id',fallback,'--deployment-locked']
 preview=json.loads(subprocess.check_output(command,text=True,timeout=45))
 assert not preview['candidates'] and {current,fallback}<=set(preview['keep'])
 def runtime():
  docker=['sudo','-n','/usr/bin/docker'];ids=sorted(subprocess.check_output(docker+['ps','-aq'],text=True,timeout=20).split())
  return subprocess.check_output(docker+['inspect','--format','{{.Id}} {{.Image}} {{.State.Status}} {{.State.StartedAt}} {{.RestartCount}}',*ids],text=True,timeout=20)
 before=runtime()
 result=json.loads(subprocess.check_output(command+['--apply'],text=True,timeout=45));assert result['status']=='success' and not result['removed_tags']
 assert runtime()==before
 for image in (current,fallback):
  assert subprocess.check_output(['sudo','-n','/usr/bin/docker','image','inspect','--format','{{.Id}}',image],text=True,timeout=20).strip()==image
 result.update(preview_candidates=0,original_lock_inode_unchanged=lock.stat().st_ino==inode,current_and_fallback_present=True)
 print(json.dumps(result))
'''))
    receipt['production_cleanup_executed'] = True
Path('retention-closeout.json').write_text(json.dumps(receipt, indent=2)+'\n')

"""One-variable import control in the fixed image: no network, mounts or app startup."""
import json
import subprocess

IMAGE = 'sha256:18a39e862a82bfb9b503a3dd93639688bc45016caa2c7f1bf9081953b4523fa0'
SOURCE = 'f9f24abdf98535295c4e3d223675ef6be62115e2'

def command(*args):
    return subprocess.check_output(args, text=True, timeout=40, stderr=subprocess.DEVNULL)

def current():
    return command('sudo','-n','docker','inspect','qs-ai','--format','{{.Id}} {{.Image}} {{.State.Health.Status}}').strip()

before = current()
assert before.endswith('sha256:a14f5fd4823cfda040de3aff9b1de6327df0644666758e924db2364393c31734 healthy')
source = command('sudo','-n','docker','image','inspect',IMAGE,'--format','{{index .Config.Labels "org.opencontainers.image.revision"}} {{.Config.User}}').strip()
assert source == SOURCE+' app'
code = '''import json,os
r=dict(home=os.path.expanduser('~'), imported=False)
try:
 from rocketmq.v5.client import ClientConfiguration, Credentials
 r['imported']=True
except Exception as e:
 r.update(error_type=type(e).__name__, errno=getattr(e,'errno',None))
print(json.dumps(r))
'''
results = {}
for label, extra in [('default_home',[]),('writable_home',['--env','HOME=/tmp'])]:
    raw=command('sudo','-n','docker','run','--rm','--pull','never','--network','none','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges:true','--tmpfs','/tmp:rw,noexec,nosuid,size=64m',*extra,'--entrypoint','/app/.venv/bin/python',IMAGE,'-c',code)
    result=json.loads(raw)
    assert set(result) <= {'home','imported','error_type','errno'}
    assert result['home'] in ('/home/app','/tmp')
    assert result.get('error_type') in (None,'OSError','PermissionError','FileNotFoundError')
    results[label]=result
unchanged=before == current()
report=dict(production_services_changed=not unchanged, starts_application=False, network='none', database_access=False, image=IMAGE, source=SOURCE, results=results)
print(json.dumps(report,indent=2))
assert unchanged and results['default_home']['imported'] is False and results['default_home']['errno'] in (13,30) and results['writable_home']['imported'] is True

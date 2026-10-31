"""Remote, unprivileged coordinator for the installed hash-bound retention CLI.

Plan is read-only. Verify applies only an empty deletion plan, under the original
host lock, preserving the actual pre-release images and every container reference.
"""
import fcntl
import hashlib
import json
import os
from pathlib import Path
import socket
import stat
import subprocess
import sys

SHA = '53ec1f1e093aca37ad671a50d37203b87757f2daea7f78d03e96df7c51474324'
PINS = {
    'serverA': {
        'iam': ('46930b3677e96a7de18b907ae3e1a9e6fe1559f71398e78af33f023a4039c3fd', '450f5c5872a247f04053116fbfb04ccd440315c4f3ed0fe9bd0353b2ed25b04b', ['iam-apiserver']),
        'qs-apiserver': ('821cfa3613e12d6ab39c469c45c9d959e1184a8277efb61070c9fcfc369df47d', '5940b75bae8eff01b6649584450fe86665ac4e185a643aae17539d7ea05de4e2', ['qs-apiserver']),
        'qs-collection-server': ('174bdb37268121ed81b317722869b5b5fca39d61988d8331e4d114596c03c056', 'd139bfe81bba7fce5af87a69444d0b24fed0848aa66d3dff2251f6dd32fd0846', ['qs-collection-server-1', 'qs-collection-server-2']),
        'qs-ai': ('18a39e862a82bfb9b503a3dd93639688bc45016caa2c7f1bf9081953b4523fa0', 'a14f5fd4823cfda040de3aff9b1de6327df0644666758e924db2364393c31734', ['qs-ai']),
    },
    'serverB': {
        'qs-operating-system': ('6cb564aab2c1961129051ebea4a82c81e89b0b694165f7473092b6a9b62433bd', '9133db6a3838693c3e09627371582fa021915ebc9226c456f47c092fe2492263', ['qs-operating-system']),
    },
}
INODES = {'serverA': 820503, 'serverB': 263196}
DOCKER = ['sudo', '-n', '/usr/bin/docker']


def run(args):
    return subprocess.check_output(args, text=True, stderr=subprocess.DEVNULL, timeout=40).strip()


def runtime():
    ids = sorted(run(DOCKER + ['ps', '-aq']).split())
    template = '{"id":{{json .Id}},"name":{{json .Name}},"image":{{json .Image}},"status":{{json .State.Status}},"started":{{json .State.StartedAt}},"restarts":{{json .RestartCount}},"health":{{if .State.Health}}{{json .State.Health.Status}}{{else}}null{{end}}}'
    return [json.loads(line) for line in run(DOCKER + ['inspect', '--format', template, *ids]).splitlines()]


def main():
    operation, host = sys.argv[1:]
    report = dict(operation=operation, host=host, status='in_progress', read_only=operation=='plan-installed',
                  business_database_operations=False, application_restarted=False, roles={})
    phase = 'arguments'
    try:
        assert operation in ('plan-installed', 'verify-installed') and host in PINS
        assert socket.gethostname().split('.')[0] == host
        phase = 'installed_helper_trust'
        tool = Path('/usr/local/libexec/fangcun/image-retention.py')
        assert tool.resolve() == tool
        for parent in tool.parents:
            info = parent.stat()
            assert stat.S_ISDIR(info.st_mode) and info.st_uid == 0 and not info.st_mode & 0o022
        info = tool.lstat()
        assert stat.S_ISREG(info.st_mode) and info.st_uid == 0 and stat.S_IMODE(info.st_mode) == 0o755
        assert hashlib.sha256(tool.read_bytes()).hexdigest() == SHA
        report['helper_sha256'] = SHA
        phase = 'original_host_lock'
        lock = Path('/var/lib/fangcun-image-retention/deploy.lock')
        assert lock.resolve() == lock and lock.is_file() and not lock.is_symlink()
        info = lock.stat()
        assert info.st_uid == 0 and stat.S_IMODE(info.st_mode) == 0o666 and info.st_ino == INODES[host]
        info_dir = lock.parent.stat()
        assert info_dir.st_uid == 0 and stat.S_IMODE(info_dir.st_mode) == 0o755
        identity = (info.st_dev, info.st_ino)
        with lock.open('r') as stream:
            fcntl.flock(stream, fcntl.LOCK_EX | fcntl.LOCK_NB)
            assert (lock.stat().st_dev, lock.stat().st_ino) == identity
            before = runtime()
            all_ids = sorted(set(run(DOCKER + ['image', 'ls', '-aq', '--no-trunc']).split()))
            live = {row['name'].lstrip('/'): row for row in before}
            commands = {}
            # Validate every role and complete every preview before any apply.
            for service, (current, fallback, names) in PINS[host].items():
                phase = 'preview_' + service
                current, fallback = 'sha256:' + current, 'sha256:' + fallback
                assert {current, fallback} <= set(all_ids)
                for name in names:
                    row = live[name]
                    assert row['image'] == current and row['status'] == 'running' and row['health'] == 'healthy'
                command = ['sudo', '-n', '/usr/bin/python3', '-I', str(tool), '--service', service,
                           '--image-ref', current, '--protect-image-id', fallback, '--deployment-locked']
                preview = json.loads(run(command))
                protected = {current, fallback} | {row['image'] for row in before}
                assert protected <= set(preview['keep'])
                report['roles'][service] = dict(current=current, protected_previous=fallback,
                    preview=preview, fixed_command_authorized=True,
                    fallback_local_present=True, current_running_healthy=True)
                commands[service] = command
            if operation == 'verify-installed':
                phase = 'reviewed_empty_deletion_plan'
                assert all(not row['preview']['candidates'] for row in report['roles'].values())
                for service, command in commands.items():
                    phase = 'apply_' + service
                    # Reconfirm the empty candidate set immediately before apply.
                    preview = json.loads(run(command))
                    assert not preview['candidates']
                    assert set(preview['keep']) == set(report['roles'][service]['preview']['keep'])
                    audit = json.loads(run(command + ['--apply']))
                    assert audit['status'] == 'success' and not audit['candidates'] and not audit['removed_tags']
                    report['roles'][service]['audit'] = audit
            phase = 'runtime_and_all_image_preservation'
            assert runtime() == before
            assert sorted(set(run(DOCKER + ['image', 'ls', '-aq', '--no-trunc']).split())) == all_ids
            assert (lock.stat().st_dev, lock.stat().st_ino) == identity
            report.update(status='success', all_container_states_unchanged=True, all_images_preserved=True,
                          original_lock_inode_unchanged=True, removed_tags=0)
    except Exception as error:
        report.update(status='failed', phase=phase, error_type=type(error).__name__)
    print(json.dumps(report))
    return 0 if report['status'] == 'success' else 1


if __name__ == '__main__':
    raise SystemExit(main())

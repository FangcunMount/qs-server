"""Read-only release and rollback inventory. Never export container environment values."""
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import socket
import stat
import subprocess
import sys

NAMES = re.compile(r"(?:qs-(?:apiserver|collection.*|worker.*|ai|operating-system)|iam-apiserver|qs-ops|ops|prometheus|grafana|nginx)(?:-\d+)?$")
MATERIAL_TOOL_SHA = '36f3046ed453dfbf8beae684269c94d1a6db0abaa23de51e09d41e462c6a3d5c'



def ai_release_metadata(directory):
    manifest = json.loads((directory / 'manifest.json').read_text())
    result = {k: manifest.get(k) for k in ('revision', 'image_id', 'image_ref', 'schema_policy', 'digest', 'provenance')}
    receipt_path = directory / 'loaded-image.json'
    result['loaded_receipt_exists'] = receipt_path.is_file()
    image_env = (directory / 'image.env').read_text()
    if re.fullmatch(r'QS_AI_IMAGE=(?:sha256:[0-9a-f]{64}|qs-ai:[0-9a-f]{40})\n?', image_env):
        result['image_env_reference'] = image_env.strip().split('=', 1)[1]
    else:
        result['image_env_reference_valid'] = False
    if result['loaded_receipt_exists']:
        receipt = json.loads(receipt_path.read_text())
        result['loaded_image'] = {k: receipt.get(k) for k in ('version', 'revision', 'source_image_id', 'loaded_image_id', 'archive_sha256', 'registry_digest')}
        identity = receipt.get('identity', {})
        identity_matches = manifest.get('image_identity') == identity if manifest.get('image_identity') is not None else manifest['image_id'] == identity.get('config_sha256')
        result['loaded_scalar_binding_valid'] = (receipt.get('version') == 1 and identity_matches
            and receipt.get('revision') == manifest['revision'] and receipt.get('source_image_id') == manifest['image_id']
            and receipt.get('archive_sha256') == manifest.get('archive_sha256') and receipt.get('registry_digest') == manifest.get('digest')
            and image_env == 'QS_AI_IMAGE=' + receipt.get('loaded_image_id', '') + '\n')
    return result


def verify_material_tool():
    path = Path('/opt/fangcun-m4/material-tools') / (MATERIAL_TOOL_SHA + '.py')
    if path.resolve() != path:
        raise ValueError('Material tool path changed')
    for parent in path.parents:
        info = parent.stat()
        if info.st_uid != 0 or info.st_mode & 0o022:
            raise ValueError('Material tool parent changed')
    info = path.stat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != 0 or stat.S_IMODE(info.st_mode) != 0o555 or not 0 < info.st_size <= 65536 or hashlib.sha256(path.read_bytes()).hexdigest() != MATERIAL_TOOL_SHA:
        raise ValueError('Material tool identity changed')
    return str(path)


def material_observation(role, revision, host):
    result = dict(role=role, revision=revision, host=host, verified=False)
    stage = 'tool_identity'
    try:
        if role not in ('iam', 'qs-api', 'ai', 'worker') or not re.fullmatch(r'm4-[a-z0-9][a-z0-9-]{0,59}', revision):
            raise ValueError('Invalid material binding')
        tool = verify_material_tool()
        stage = 'installed_verification'
        password = os.environ.get('SUDO_PASSWORD', '')
        sudo = ['sudo', '-S', '-p', ''] if password else ['sudo', '-n']
        args = sudo + ['/usr/bin/python3', '-I', tool, '--verify-installed', '--host', host, '--expect-role', role, '--expect-revision', revision]
        response = subprocess.run(args, input=password + '\n' if password else None, capture_output=True, text=True, timeout=30)
        if response.returncode:
            result['returncode'] = response.returncode
            result['error_stage'] = stage
            result['error'] = ('password_required' if 'password is required' in response.stderr else
                               'command_not_permitted' if 'not allowed to execute' in response.stderr else
                               'authentication_failed' if 'incorrect password' in response.stderr else
                               'installed_verifier_refused')
            return result
        value = json.loads(response.stdout)
        result['verified'] = (value.get('status') == 'verified' and value.get('read_only') is True and value.get('activated') is False
            and (value.get('role'), value.get('revision'), value.get('host')) == (role, revision, host)
            and value.get('material_root') == '/var/lib/fangcun-m4-materials'
            and isinstance(value.get('receipt_sha256'), str) and re.fullmatch(r'[0-9a-f]{64}', value['receipt_sha256']) is not None)
        if result['verified']:
            result['receipt_sha256'] = value['receipt_sha256']
    except (OSError, ValueError, subprocess.SubprocessError) as error:
        result['error'] = 'verification_unavailable'
        result['error_stage'] = stage
        result['error_type'] = type(error).__name__
        if isinstance(error, ValueError) and str(error) in ('Material tool path changed', 'Material tool parent changed', 'Material tool identity changed'):
            result['reason'] = str(error)
    return result


def command(*args):
    return subprocess.check_output(args, text=True, timeout=30, stderr=subprocess.DEVNULL)


def collect(sudo_docker=False):
    docker = ("sudo", "-n", "docker") if sudo_docker else ("docker",)
    command(*docker, "version", "--format", "{{.Server.Version}}")
    ids = command(*docker, "ps", "--no-trunc", "--format", "{{.ID}} {{.Names}}")
    containers = []
    materials = {}
    material_auth_failed = False
    for row in ids.splitlines():
        cid, name = row.split(maxsplit=1)
        if not NAMES.fullmatch(name):
            continue
        # Fixed projections exclude credentials, Env, logs and application data.
        data = json.loads(command(*docker, "inspect", "--format", '{"Image":{{json .Image}},"Config":{"Image":{{json .Config.Image}}},"State":{"Running":{{json .State.Running}},"HealthStatus":{{if .State.Health}}{{json .State.Health.Status}}{{else}}null{{end}}},"Mounts":{{json .Mounts}}}', cid))
        image = json.loads(command(*docker, "image", "inspect", "--format", '{"Os":{{json .Os}},"Architecture":{{json .Architecture}},"RepoDigests":{{json .RepoDigests}},"RepoTags":{{json .RepoTags}},"Config":{"Labels":{{json .Config.Labels}}}}', data["Image"]))
        mounts = []
        for mount in data.get("Mounts", []):
            item = {k.lower(): mount.get(k) for k in ("Type", "Source", "Destination", "RW")}
            source = Path(mount["Source"])
            item["fingerprint_available"] = False
            try:
                if source.is_file() and source.stat().st_size <= 4 * 1024 * 1024:
                    item["file_sha256"] = hashlib.sha256(source.read_bytes()).hexdigest()
                    item["fingerprint_available"] = True
                if source.is_dir() and "config" in mount["Destination"].lower():
                    def unreadable(error):
                        raise error
                    config_files = sorted(Path(directory) / name for directory, _, names in os.walk(source, onerror=unreadable)
                                          for name in names if Path(name).suffix in {'.yaml', '.yml', '.json', '.toml', '.env'})
                    if len(config_files) > 256 or any(f.stat().st_size > 4 * 1024 * 1024 for f in config_files):
                        raise ValueError("Configuration fingerprint exceeds bounds")
                    fingerprints = {str(f.relative_to(source)): hashlib.sha256(f.read_bytes()).hexdigest() for f in config_files}
                    item["config_files_sha256"] = fingerprints
                    item["fingerprint_available"] = True
            except PermissionError:
                item["fingerprint_available"] = False
            mounts.append(item)
        labels = image.get("Config", {}).get("Labels") or {}
        containers.append({"name": name, "container_id": cid, "image_ref": data["Config"]["Image"],
                           "image_id": data["Image"], "repo_digests": image.get("RepoDigests", []), "repo_tags": image.get("RepoTags", []),
                           "source_sha": labels.get("org.opencontainers.image.revision"),
                           "platform": image["Os"] + "/" + image["Architecture"],
                           "running": data["State"]["Running"],
                           "health": data["State"].get("HealthStatus"),
                           "mounts": mounts})
        material_role = 'iam' if name == 'iam-apiserver' else 'qs-api' if name == 'qs-apiserver' else 'ai' if name == 'qs-ai' else 'worker' if name.startswith('qs-worker') else None
        revisions = {part for mount in mounts for part in mount['source'].split('/') if re.fullmatch(r'm4-[a-z0-9][a-z0-9-]{0,59}', part) and mount['source'].startswith('/var/lib/fangcun-m4-materials/')}
        if material_role and len(revisions) == 1 and material_role not in materials:
            revision = revisions.pop()
            host = 'serverD' if material_role == 'worker' else 'serverA'
            if material_auth_failed:
                materials[material_role] = dict(role=material_role, revision=revision, host=host, verified=False, error='authentication_not_retried')
            else:
                materials[material_role] = material_observation(material_role, revision, host)
                material_auth_failed = materials[material_role].get('error') == 'authentication_failed'
        if name == 'iam-apiserver':
            containers[-1].update(reported_git_commit=None, ready=False)
            try:
                version = command(*docker, 'exec', cid, 'curl', '-fsS', '--max-time', '5', 'http://127.0.0.1:9080/version')
                commits = re.findall(r'"gitCommit"\s*:\s*"([0-9a-f]{40})"', version)
                if len(commits) == 1:
                    containers[-1]['reported_git_commit'] = commits[0]
                command(*docker, 'exec', cid, 'curl', '-fsS', '--max-time', '5', '-o', '/dev/null', 'http://127.0.0.1:9080/readyz')
                containers[-1]['ready'] = True
            except subprocess.SubprocessError:
                pass
        if name == 'qs-ai':
            projection = '{"user":{{json .Config.User}},"health":{{if .State.Health}}{{json .State.Health.Status}}{{else}}null{{end}},"project":{{json (index .Config.Labels "com.docker.compose.project")}},"service":{{json (index .Config.Labels "com.docker.compose.service")}},"compose_files":{{json (index .Config.Labels "com.docker.compose.project.config_files")}}}'
            containers[-1]['handoff_runtime'] = json.loads(command(*docker, 'inspect', '--format', projection, cid))
            containers[-1]['handoff_runtime']['effective_uid'] = command(*docker, 'exec', cid, 'id', '-u').strip()
            native = containers[-1]['handoff_runtime']
            native['reader_uid'] = os.geteuid()
            native['configuration_files'] = []
            files = [Path(p) for p in native['compose_files'].split(',')]
            for path in [*files, files[0].parent / 'runtime.env']:
                item = dict(path=str(path), readable=os.access(path, os.R_OK))
                try:
                    info = path.stat()
                    item.update(uid=info.st_uid, mode=oct(stat.S_IMODE(info.st_mode)), size=info.st_size)
                except OSError as error:
                    item['error_type'] = type(error).__name__
                native['configuration_files'].append(item)
            native['rendered_configuration_available'] = False
            try:
                if any(not p.is_relative_to('/opt/fangcun-readable-names/releases/readable-20261028-v2') or '..' in p.parts for p in files):
                    raise ValueError('Unexpected native profile namespace')
                args = [*docker, 'compose', '-p', 'qs-ai', '--env-file', str(files[0].parent / 'runtime.env')]
                for path in files:
                    args += ['-f', str(path)]
                config = json.loads(command(*args, 'config', '--format', 'json'))
                service = config['services']['qs-ai']
                projection = '{"Env":{{json .Config.Env}},"User":{{json .Config.User}},"Cmd":{{json .Config.Cmd}},"Entrypoint":{{json .Config.Entrypoint}}}'
                actual = json.loads(command(*docker, 'inspect', '--format', projection, cid))
                base = json.loads(command(*docker, 'image', 'inspect', '--format', projection, data['Image']))
                desired_env = dict(item.split('=', 1) for item in base['Env'])
                for key, value in service.get('environment', {}).items():
                    if value is None:
                        desired_env.pop(key, None)
                    else:
                        desired_env[key] = str(value).replace('$$', '$')
                actual_env = dict(item.split('=', 1) for item in actual['Env'])
                native.update(rendered_configuration_available=True,
                    rendered_configuration_sha256=hashlib.sha256(json.dumps(config, sort_keys=True, separators=(',', ':')).encode()).hexdigest(),
                    rendered_service_names=sorted(config['services']), image_matches=service['image'] == data['Image'],
                    environment_matches=desired_env == actual_env,
                    environment_mismatched_keys=sorted(k for k in set(desired_env) | set(actual_env) if desired_env.get(k) != actual_env.get(k)),
                    configured_user=service.get('user'), command_is_null=service.get('command') is None,
                    entrypoint_is_null=service.get('entrypoint') is None,
                    command_matches=(service.get('command') if service.get('command') is not None else base['Cmd']) == actual['Cmd'],
                    entrypoint_matches=(service.get('entrypoint') if service.get('entrypoint') is not None else base['Entrypoint']) == actual['Entrypoint'])
            except (OSError, ValueError, KeyError, TypeError, subprocess.SubprocessError) as error:
                native['render_error_type'] = type(error).__name__
            containers[-1].update(reported_git_commit=None, ready=False)
            try:
                version = command(*docker, 'exec', cid, '/app/.venv/bin/python', '-c', 'import os; print(os.environ.get("QS_AI_RELEASE_SHA", "unknown"))').strip()
                if re.fullmatch(r'[0-9a-f]{40}', version):
                    containers[-1]['reported_git_commit'] = version
                command(*docker, 'exec', cid, '/app/.venv/bin/python', '-c', 'import urllib.request; urllib.request.urlopen("http://127.0.0.1:8000/readyz", timeout=5).close()')
                containers[-1]['ready'] = True
            except subprocess.SubprocessError:
                pass
        # The same fixed early-exit version and readiness commands used by runtime-evidence.py.
        role = "worker" if name.startswith("qs-worker") else "collection-server" if name.startswith("qs-collection") else "apiserver" if name == "qs-apiserver" else None
        if role:
            binary = "/app/" + ("collection-server" if role == "collection-server" else "qs-" + role)
            container = containers[-1]
            container.update(reported_git_commit=None, binary_matches_process=False, ready=False)
            try:
                raw = command(*docker, "exec", cid, binary, "--version=true", "--config=/app/configs/" + role + ".prod.yaml")
                commits = re.findall(r"^\s*gitCommit:\s*([0-9a-f]{40})\s*$", raw, re.MULTILINE)
                hashes = command(*docker, "exec", cid, "sha256sum", "/proc/1/exe", binary).splitlines()
                container["binary_matches_process"] = len(hashes) == 2 and all(re.fullmatch(r"[0-9a-f]{64}\s+\S+", x) for x in hashes) and hashes[0].split()[0] == hashes[1].split()[0]
                if len(commits) == 1 and container["binary_matches_process"]:
                    container["reported_git_commit"] = commits[0]
            except subprocess.SubprocessError:
                pass
            port, ready_path = (9092, "/readyz") if role == "worker" else (8080, "/serve-readyz") if role == "collection-server" else (8080, "/readyz")
            try:
                command(*docker, "exec", cid, "wget", "-q", "-O", "/dev/null", "-T", "5", "http://127.0.0.1:" + str(port) + ready_path)
                container["ready"] = True
            except subprocess.SubprocessError:
                pass
    if command(*docker, "ps", "--no-trunc", "--format", "{{.ID}} {{.Names}}") != ids:
        raise ValueError("Runtime changed during snapshot")
    if not containers:
        raise ValueError("No release containers found")
    result = {"host": socket.gethostname(), "observed_at": datetime.datetime.now(datetime.timezone.utc).isoformat(),
              "read_only": True, "business_acceptance_verified": False, "containers": containers, "installed_materials": materials}
    state = Path('/opt/qs-ai/state.json')
    result["ai_release_state_available"] = False
    try:
        if state.is_file():
            values = json.loads(state.read_text())
            result["ai_release_state"] = {k: values.get(k) for k in ("current", "previous")}
            result["ai_release_manifests"] = {}
            for key in ("current", "previous"):
                release = values.get(key)
                if release and re.fullmatch(r"[0-9a-f]{40}-[0-9]+-[0-9]+", release):
                    manifest = Path('/opt/qs-ai/releases') / release / 'manifest.json'
                    if manifest.is_file():
                        result["ai_release_manifests"][key] = ai_release_metadata(manifest.parent)
            result["ai_release_state_available"] = True
        followup = Path('/opt/qs-ai/schema-maintenance/active-v2.json')
        result['ai_active_schema_followup_exists'] = followup.is_file()
        if result['ai_active_schema_followup_exists']:
            value = json.loads(followup.read_text())
            result['ai_active_schema_followup'] = {k: value.get(k) for k in ('format', 'context_id', 'executor_revision', 'helper_sha256')}
        operation = Path('/opt/qs-ai/schema-maintenance/operation-v2.json')
        result['ai_schema_operation_exists'] = operation.is_file()
        if result['ai_schema_operation_exists']:
            value = json.loads(operation.read_text())
            result['ai_schema_operation'] = {k: value.get(k) for k in ('phase', 'kind', 'context_id', 'operation_id')}
    except PermissionError:
        pass
    return result


if __name__ == "__main__":
    try:
        if sys.argv[1:] not in ([], ["--sudo-docker"]):
            raise ValueError("Unsupported snapshot mode")
        print(json.dumps(collect(sudo_docker=bool(sys.argv[1:])), sort_keys=True, indent=2))
    except Exception:
        raise SystemExit("Read-only release snapshot failed; no deployment performed") from None

"""Runner-only immutable image diagnostics; no SSH, container start or secret output."""
import base64, hashlib, json, os, pathlib, subprocess


LAST = None

def call(args):
    global LAST
    LAST = subprocess.run(args, capture_output=True, text=True, timeout=120)
    return LAST


def required(value):
    if not value:
        raise ValueError('binding_check_failed')


result = {'format': 'frozen-image-runner-diagnostic/v1', 'production_changed': False, 'images': {}}
result['isolated_buildx_available_before'] = call(['docker', 'buildx', 'version']).returncode == 0
profile = pathlib.Path.home() / '.docker'
lookup = call(['docker', '--config', str(profile), 'info', '--format', '{{json .ClientInfo.Plugins}}'])
plugins = json.loads(lookup.stdout) if lookup.returncode == 0 else []
paths = [pathlib.Path(p['Path']) for p in (plugins or []) if p.get('Name') == 'buildx' and p.get('Path')]
result['default_buildx_plugin_available'] = any(p.is_file() and os.access(p, os.X_OK) for p in paths)
if not result['isolated_buildx_available_before'] and result['default_buildx_plugin_available']:
    directory = pathlib.Path(os.environ['DOCKER_CONFIG']) / 'cli-plugins'
    directory.mkdir(mode=0o700, exist_ok=True)
    target = next(p for p in paths if p.is_file() and os.access(p, os.X_OK))
    (directory / 'docker-buildx').symlink_to(target)
result['isolated_buildx_available_after'] = call(['docker', 'buildx', 'version']).returncode == 0
images = json.loads(os.environ['CANDIDATE_IMAGES'])
for name in ['iam', 'qs-operating-system']:
    expected = images[name]
    repo = os.environ['ALIYUN_ACR_REGISTRY'] + '/' + os.environ['ALIYUN_ACR_NAMESPACE'] + '/' + name
    ref = repo + '@' + expected['digest']
    row = result['images'][name] = {'phase': 'pull', 'passed': False}
    try:
        required(call(['docker', 'pull', '--platform', 'linux/amd64', ref]).returncode == 0)
        row['phase'] = 'registry_manifest'
        response = call(['docker', 'buildx', 'imagetools', 'inspect', '--raw', ref])
        required(response.returncode == 0)
        raw = response.stdout.encode();doc = json.loads(raw)
        row['raw_sha256'] = 'sha256:' + hashlib.sha256(raw).hexdigest()
        row['trimmed_sha256'] = 'sha256:' + hashlib.sha256(raw[:-1] if raw.endswith(b'\n') else raw).hexdigest()
        row['raw_digest_matches'] = expected['digest'] in [row['raw_sha256'], row['trimmed_sha256']]
        graph = {expected['digest']}
        if 'manifests' in doc:
            selected = [v for v in doc['manifests'] if v.get('platform', {}).get('os') == 'linux' and v.get('platform', {}).get('architecture') == 'amd64']
            required(len(selected) == 1);graph.add(selected[0]['digest'])
            response = call(['docker', 'buildx', 'imagetools', 'inspect', '--raw', repo + '@' + selected[0]['digest']]);required(response.returncode == 0)
            doc = json.loads(response.stdout)
        row['registry_config_digest'] = doc['config']['digest']
        row['config_matches'] = doc['config']['digest'] == expected['config']
        row['phase'] = 'local_identity'
        local = json.loads(subprocess.check_output(['docker', 'image', 'inspect', ref], text=True))[0]
        row['local_id'] = local['Id'];row['platform'] = local['Os'] + '/' + local['Architecture']
        row['local_id_in_graph'] = local['Id'] in graph | {expected['config']}
        row['repo_digest_matches'] = bool(set(local.get('RepoDigests') or []) & {repo + '@' + d for d in graph})
        descriptor = (local.get('Descriptor') or {}).get('digest');row['descriptor'] = descriptor
        row['descriptor_in_graph'] = descriptor is None or descriptor in graph
        revision = (local.get('Config', {}).get('Labels') or {}).get('org.opencontainers.image.revision')
        row['revision_matches'] = revision is None or revision == expected['sha']
        row['passed'] = all(row[key] for key in ['raw_digest_matches', 'config_matches', 'local_id_in_graph', 'repo_digest_matches', 'descriptor_in_graph', 'revision_matches']) and row['platform'] == 'linux/amd64'
        row['phase'] = 'complete'
    except Exception as error:
        row['error_type'] = type(error).__name__
        if LAST is not None:
            row['command_returncode'] = LAST.returncode
            row['error_categories'] = [v for v in (
                'permission denied','unauthorized','denied','not found','no such','no builder',
                'failed to resolve','manifest unknown','no such manifest','certificate','tls',
                'context','keychain','error getting credentials','cannot connect','connection refused',
                'timeout','unknown flag','buildx','missing','reference does not exist',
                'manifest does not exist','proxy','service unavailable','requested access',
                'forbidden','unauthenticated','failed to initialize','failed to read',
                'configuration file','docker-credential','resource temporarily','x509','eof',
                'no space left','unsupported','invalid reference','429','403','401','404',
                'no output','failed to create','invalid character','unexpected status',
            ) if v in LAST.stderr.lower()]
pathlib.Path('image-binding-diagnostics.json').write_text(json.dumps(result, indent=2) + '\n')
print(json.dumps({'isolated_buildx_available_before': result['isolated_buildx_available_before'], 'isolated_buildx_available_after': result['isolated_buildx_available_after'], 'passed': {k:v['passed'] for k,v in result['images'].items()}}))

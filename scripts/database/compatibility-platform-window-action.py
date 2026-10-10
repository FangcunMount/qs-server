#!/usr/bin/env python3
"""Same-run workflow owner and fixed pinned window-tool caller.

Only the dedicated effectful retirement job calls this producer. Credentials
stay in memory or the existing ephemeral SSH identity file. A saved receipt,
caller callback or JSON outcome cannot reconstruct this live owner. Unknown
platform/native results retain exact intents and prohibit automatic restore.
"""
import argparse
import hashlib
import importlib.util
import os
from pathlib import Path
import re
import selectors
import shlex
import signal
import stat
import subprocess
import sys
import tarfile
import time
import uuid

sys.dont_write_bytecode = True
JOB_NAME = "Retire exact private lifecycle stage with workflow quarantine"
ASSETS = ("compatibility-window-tool.py", "receipt-transport.py", "inventory-linux-amd64", "inventory-linux-arm64")
LIMIT = 128 << 20
OUTPUT_LIMIT = 256 << 10
TIMEOUT = 114 * 60


class Rejected(Exception):
    """A fixed category only, never remote stderr, credentials or route."""


def fail(category):
    raise Rejected(category)


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def load(path, name):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def source_bytes(repo, source, relative):
    """Actual immutable checkout bytes, independently checked against Git."""
    if not re.fullmatch(r"[0-9a-f]{40}", source):
        fail("platform_window_source_rejected")
    repo = Path(repo).resolve(strict=True)
    if b"" != subprocess.check_output(["/usr/bin/git", "-C", str(repo), "status", "--porcelain", "--untracked-files=no"], stderr=subprocess.DEVNULL):
        fail("platform_window_source_rejected")
    head = subprocess.check_output(["/usr/bin/git", "-C", str(repo), "rev-parse", "HEAD"], stderr=subprocess.DEVNULL).strip()
    if head != source.encode("ascii"):
        fail("platform_window_source_rejected")
    expected = subprocess.check_output(["/usr/bin/git", "-C", str(repo), "show", source + ":" + relative], stderr=subprocess.DEVNULL)
    path = repo / relative
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_nlink != 1 or info.st_uid != os.geteuid() or info.st_mode & 0o022 or not 0 < info.st_size <= 2 << 20:
        fail("platform_window_source_rejected")
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
    try:
        actual = os.read(fd, (2 << 20) + 1)
        after = os.fstat(fd)
        identity = lambda s: (s.st_dev, s.st_ino, s.st_uid, s.st_mode, s.st_nlink, s.st_size, s.st_mtime_ns, s.st_ctime_ns)
        if actual != expected or identity(info) != identity(after) or identity(after) != identity(path.lstat()):
            fail("platform_window_source_rejected")
    finally:
        os.close(fd)
    return actual


def read_binary(path, expected):
    info = path.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != os.geteuid() or info.st_nlink != 1 or info.st_mode & 0o022 or not 0 < info.st_size <= LIMIT:
        fail("platform_window_package_rejected")
    fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
    try:
        with os.fdopen(fd, "rb", closefd=False) as stream:
            raw = stream.read(LIMIT + 1)
        after = os.fstat(fd)
        identity = lambda s: (s.st_dev, s.st_ino, s.st_uid, s.st_mode, s.st_nlink, s.st_size, s.st_mtime_ns, s.st_ctime_ns)
        if sha(raw) != expected or identity(info) != identity(after) or identity(after) != identity(path.lstat()):
            fail("platform_window_package_rejected")
        return raw
    finally:
        os.close(fd)


def write_private(path, raw):
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    try:
        with os.fdopen(fd, "wb", closefd=False) as stream:
            stream.write(raw); stream.flush(); os.fsync(fd)
    finally:
        os.close(fd)
    fd = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)


class IdentityAsset:
    """Original FD for the exact ephemeral SSH identity/config asset only."""
    def __init__(self, path, raw):
        write_private(path, raw)
        self.path, self.expected = path, sha(raw)
        self.fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
        self.parent_fd = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        self.parent = self.parent_stamp(os.fstat(self.parent_fd))
        self.identity = self.stamp(os.fstat(self.fd))
        try: self.check()
        except BaseException:
            self.close(); raise

    @staticmethod
    def stamp(value):
        return (value.st_dev, value.st_ino, value.st_uid, value.st_mode, value.st_nlink, value.st_size, value.st_mtime_ns, value.st_ctime_ns)

    @staticmethod
    def parent_stamp(value):
        return (value.st_dev, value.st_ino, value.st_uid, value.st_mode)

    def check(self):
        if self.fd is None: fail("platform_window_identity_cleanup_unknown")
        if self.parent_stamp(os.fstat(self.parent_fd)) != self.parent or self.parent_stamp(self.path.parent.lstat()) != self.parent or self.parent[2] != os.geteuid() or stat.S_IMODE(self.parent[3]) != 0o700 or not stat.S_ISDIR(self.parent[3]):
            fail("platform_window_identity_cleanup_unknown")
        before = os.fstat(self.fd)
        raw = os.pread(self.fd, 2 << 20, 0)
        if self.stamp(before) != self.identity or self.stamp(os.fstat(self.fd)) != self.identity or self.stamp(self.path.lstat()) != self.identity or sha(raw) != self.expected or not stat.S_ISREG(before.st_mode) or before.st_uid != os.geteuid() or stat.S_IMODE(before.st_mode) != 0o600 or before.st_nlink != 1:
            fail("platform_window_identity_cleanup_unknown")

    def remove(self):
        self.check()
        os.unlink(self.path)
        fd = os.open(self.path.parent, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        try: os.fsync(fd)
        finally: os.close(fd)
        if os.path.lexists(self.path) or os.fstat(self.fd).st_nlink != 0:
            fail("platform_window_identity_cleanup_unknown")

    def close(self):
        if self.fd is not None: os.close(self.fd); self.fd = None
        if self.parent_fd is not None: os.close(self.parent_fd); self.parent_fd = None


def group_present(group):
    try:
        os.killpg(group, 0)
        return True
    except ProcessLookupError:
        return False


def collect_owned(command, *, packet=None, timeout=TIMEOUT):
    """Own the actual local SSH session, its EOF and terminal process group.

    Connection loss closes the live remote control pipe. Even a reaped local
    SSH process does not prove a remote daemon operation completed; callers
    accept only its exact bound native result. No unknown outcome is retried.
    """
    child = None
    streams = None
    previous = {}
    cancelled = [False]
    output = bytearray()
    try:
        for number in (signal.SIGINT, signal.SIGTERM, signal.SIGHUP):
            previous[number] = signal.getsignal(number)
            signal.signal(number, lambda _n, _f: cancelled.__setitem__(0, True))
        # The token is sent in the private packet; never inherited by ssh/scp.
        environment = {"PATH": "/usr/bin:/bin", "LC_ALL": "C"}
        child = subprocess.Popen(command, env=environment, stdin=subprocess.PIPE if packet is not None else subprocess.DEVNULL,
            stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True, bufsize=0)
        if os.getpgid(child.pid) != child.pid:
            fail("platform_window_native_unknown")
        streams = selectors.DefaultSelector()
        streams.register(child.stdout, selectors.EVENT_READ, "stdout")
        streams.register(child.stderr, selectors.EVENT_READ, "stderr")
        packet_offset = 0
        if packet is not None:
            os.set_blocking(child.stdin.fileno(), False)
            streams.register(child.stdin, selectors.EVENT_WRITE, "stdin")
        deadline = time.monotonic() + timeout
        stderr_bytes = 0
        while streams.get_map() or child.poll() is None:
            if cancelled[0] or time.monotonic() >= deadline:
                fail("platform_window_native_unknown")
            for key, _ in streams.select(0.1):
                if key.data == "stdin":
                    packet_offset += os.write(child.stdin.fileno(), packet[packet_offset:packet_offset + 8192])
                    if packet_offset == len(packet):
                        streams.unregister(child.stdin)  # keep real control pipe open
                    continue
                raw = os.read(key.fileobj.fileno(), 8192)
                if not raw:
                    streams.unregister(key.fileobj)
                elif key.data == "stdout":
                    output.extend(raw)
                else:
                    stderr_bytes += len(raw)  # never persisted or projected
                if len(output) > OUTPUT_LIMIT or stderr_bytes > OUTPUT_LIMIT:
                    fail("platform_window_native_unknown")
        code = child.wait()
        if group_present(child.pid):
            fail("platform_window_native_unknown")
        return code, bytes(output)
    except BaseException:
        if child is not None:
            if child.stdin is not None:
                try: child.stdin.close()
                except OSError: pass
            try: child.wait(timeout=25)
            except subprocess.TimeoutExpired: pass
            for number, delay in ((signal.SIGTERM, 2), (signal.SIGKILL, 5)):
                # Once the Popen leader has been reaped, this numeric PGID can
                # no longer authorize a signal to a possibly reused group.
                # Descendant/remote work then remains unknown and retained.
                if child.poll() is not None: break
                try:
                    if os.getpgid(child.pid) != child.pid: break
                except ProcessLookupError: break
                try: os.killpg(child.pid, number)
                except ProcessLookupError: pass
                try: child.wait(timeout=delay)
                except subprocess.TimeoutExpired: pass
            # Unknown remote work and exact assets are retained regardless of
            # the local process group's final state. Never manufacture success.
        raise
    finally:
        if streams is not None: streams.close()
        if child is not None:
            for stream in (child.stdin, child.stdout, child.stderr):
                if stream is not None and not stream.closed: stream.close()
        for number, handler in previous.items(): signal.signal(number, handler)


# Fixed remote bootstrap. Credentials travel only through its real stdin, then
# the existing root supervisor's private pipe. No shell env, token file,
# arbitrary command, capability flag, saved receipt or completion callback.
REMOTE = r'''
import os,sys,json,stat,hashlib,tarfile,types,io,select
def bad(): raise ValueError()
def unique(pairs):
 d={}
 for k,v in pairs:
  if k in d: bad()
  d[k]=v
 return d
try:
 if os.getuid()!=os.geteuid(): bad()
 raw=sys.stdin.buffer.readline(65537)
 if len(raw)>65536 or not raw.endswith(b'\n'): bad()
 packet=json.loads(raw,object_pairs_hook=unique)
 if type(packet)!=dict or set(packet)!={'bindings','approval','approval_sha256','package_sha256','tool_directory','credentials'}: bad()
 bindings=packet['bindings']
 if type(bindings)!=list or len(bindings)!=6 or any(type(v)!=str for v in bindings): bad()
 stage,operation,run,dispatcher,manifest,template=bindings
 import re
 if stage not in ('apply','verify','recover','purge') or re.fullmatch(r'[1-9][0-9]{0,19}-[1-9][0-9]{0,3}',run) is None: bad()
 for value in (packet['approval_sha256'],packet['package_sha256']):
  if type(value)!=str or re.fullmatch(r'[0-9a-f]{64}',value) is None: bad()
 directory=packet['tool_directory']
 if type(directory)!=str or re.fullmatch(r'/tmp/qs-independent-window-tool\.[0-9a-f]{12}',directory) is None: bad()
 info=os.lstat(directory)
 if not stat.S_ISDIR(info.st_mode) or info.st_uid!=os.getuid() or stat.S_IMODE(info.st_mode)!=0o700: bad()
 source=directory+'/qs-compatibility-retirement-'+run+'.tar.gz'
 archive='/tmp/qs-compatibility-retirement-'+run+'.tar.gz'
 fd=os.open(source,os.O_RDONLY|os.O_NOFOLLOW)
 with os.fdopen(fd,'rb') as f:
  before=os.fstat(f.fileno())
  if not stat.S_ISREG(before.st_mode) or before.st_uid!=os.getuid() or before.st_nlink!=1 or stat.S_IMODE(before.st_mode)!=0o600 or not 0<before.st_size<=134217728: bad()
  body=f.read(134217729);after=os.fstat(f.fileno())
  identity=lambda s:(s.st_dev,s.st_ino,s.st_uid,s.st_mode,s.st_nlink,s.st_size,s.st_mtime_ns,s.st_ctime_ns)
  if identity(before)!=identity(after) or identity(after)!=identity(os.lstat(source)) or hashlib.sha256(body).hexdigest()!=packet['package_sha256']: bad()
 # The fixed batch archive is O_EXCL: never overwrite/adopt an earlier run.
 fd=os.open(archive,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
 with os.fdopen(fd,'wb') as f: f.write(body);f.flush();os.fsync(f.fileno())
 with tarfile.open(fileobj=io.BytesIO(body),mode='r:gz') as package:
  entries=package.getmembers()
  allowed={'compatibility-window-tool.py','receipt-transport.py','inventory-linux-amd64','inventory-linux-arm64'}
  if len(entries)!=4 or {v.name for v in entries}!=allowed or any(not v.isfile() or v.size<=0 or v.size>134217728 for v in entries): bad()
  for name in ('compatibility-window-tool.py','receipt-transport.py'):
   member=package.getmember(name)
   if member.size>2097152: bad()
   content=package.extractfile(member).read(2097153)
   if len(content)!=member.size: bad()
   p=directory+'/'+name
   fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600)
   with os.fdopen(fd,'wb') as f: f.write(content);f.flush();os.fsync(f.fileno())
 fd=os.open(directory,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW);os.fsync(fd);os.close(fd)
 # Only our exact transferred copy is no longer needed. Unknown source or
 # fixed-archive work above never reaches this cleanup.
 if identity(before)!=identity(os.lstat(source)): bad()
 os.unlink(source)
 fd=os.open(directory,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW);os.fsync(fd);os.close(fd)
 path=directory+'/compatibility-window-tool.py'
 with open(path,'rb') as f: program=f.read(2097153)
 namespace={'__name__':'owned_window_tool','__file__':path}
 exec(compile(program,'<approved-window-tool>','exec'),namespace)
 args=types.SimpleNamespace(operation=stage,operation_id=operation,run_id=run,dispatcher_sha=dispatcher,manifest_hash=manifest,template_hash=template)
 # No credential env inheritance. The open SSH stdin is native cancellation.
 os.environ.clear();os.environ['PATH']='/usr/bin:/bin'
 code=namespace['run_window_call'](args,packet['approval'],packet['approval_sha256'],packet['package_sha256'],packet['credentials'],control=sys.stdin.fileno())
 raise SystemExit(code)
except SystemExit: raise
except BaseException:
 # No raw input, body, traceback, URI or token is emitted.
 raise SystemExit(125)
'''


def disposition(window, transport, raw, exit_code, approval, run):
    """Validate only a result from this call's actual captured native channel.

    This pure classifier is not an authority constructor. The live runner
    method below is the sole production caller, after actual EOF/Wait/reap.
    """
    try:
        if type(exit_code) is not int or exit_code not in (0, 1):
            fail("platform_window_native_unknown")
        text = raw.decode("utf-8", "strict")
        decoded = transport.decode_armored_receipt(text)
        value = window.decode(decoded)
        expected = {"format_version", "kind", "dispatcher_source_sha", "tool_source_sha", "approved_template_sha256", "derived_request_sha256", "native_result"}
        if type(value) is not dict or set(value) != expected or type(value["format_version"]) is not int or value["format_version"] != 1 or value["kind"] != "independent_window_tool_call_result" or value["dispatcher_source_sha"] != approval["dispatcher_source_sha"] or value["tool_source_sha"] != approval["tool_source_sha"] or value["approved_template_sha256"] != approval["request_template_sha256"]:
            fail("platform_window_native_unknown")
        window.token(value["derived_request_sha256"], window.HASH)
        native = window.validate_native(window.canonical(value["native_result"]), exit_code, approval, run, value["derived_request_sha256"])
        if native.get("kind") != "compatibility_retirement_lifecycle_result":
            fail("platform_window_native_unknown")
        if native["error_category"] == "lifecycle_actual_host_adapters_missing":
            if any(native[k] for k in window.BOOLS) or exit_code != 1:
                fail("platform_window_native_unknown")
            return "native_preflight_refused", native
        if native["complete"] and exit_code == 0:
            if approval["stage"] in ("apply", "purge") and native["acceptance_complete"] and native["purge_complete"]:
                return "native_completed", native
            if approval["stage"] == "recover" and native["recovery_attempted"] and native["recovery_complete"]:
                return "native_recovered", native
        # A failed forward call can finish recovery before deferred host/window
        # close fails. Its recovery booleans do not prove terminal release; even
        # a normalized generic error must retain quarantine and identity assets.
        return "native_unknown_or_not_released", native
    except (ValueError, UnicodeError, window.Refused, KeyError):
        fail("platform_window_native_unknown")


def run(dispatcher_repo, tool_repo, binary_directory):
    os.umask(0o077)
    env = dict(os.environ)
    token = env.pop("RETIREMENT_PLATFORM_TOKEN", "")
    os.environ.pop("RETIREMENT_PLATFORM_TOKEN", None)
    env.pop("GITHUB_TOKEN", None); os.environ.pop("GITHUB_TOKEN", None)
    dispatcher, run_id = env.get("GITHUB_SHA", ""), env.get("GITHUB_RUN_ID", "") + "-" + env.get("GITHUB_RUN_ATTEMPT", "")
    source_bytes(dispatcher_repo, dispatcher, "scripts/database/compatibility-platform-window-action.py")
    for relative in ("scripts/database/compatibility-platform-fence.py", "scripts/database/compatibility-host-inventory-action.py", "scripts/dbops/receipt-transport.py"):
        source_bytes(dispatcher_repo, dispatcher, relative)
    if Path(__file__).resolve() != Path(dispatcher_repo).resolve() / "scripts/database/compatibility-platform-window-action.py":
        fail("platform_window_source_rejected")
    platform = load(Path(dispatcher_repo) / "scripts/database/compatibility-platform-fence.py", "owned_platform")
    pin = load(Path(dispatcher_repo) / "scripts/database/compatibility-host-inventory-action.py", "owned_pin")
    transport = load(Path(dispatcher_repo) / "scripts/dbops/receipt-transport.py", "owned_transport")
    tool_sha = env.get("RETIREMENT_TOOL_SOURCE_SHA", "")
    window_raw = source_bytes(tool_repo, tool_sha, "scripts/database/compatibility-window-tool.py")
    source_bytes(tool_repo, tool_sha, "scripts/dbops/receipt-transport.py")
    window = load(Path(tool_repo) / "scripts/database/compatibility-window-tool.py", "owned_window")
    approval_raw, approval_sha = env.get("RETIREMENT_BOOTSTRAP_APPROVAL_JSON", ""), env.get("RETIREMENT_BOOTSTRAP_APPROVAL_SHA256", "")
    approval = window.approve(approval_raw, approval_sha, dispatcher, env.get("RETIREMENT_OPERATION", ""), env.get("RETIREMENT_OPERATION_ID", ""), env.get("RETIREMENT_MANIFEST_SHA256", ""), env.get("RETIREMENT_TEMPLATE_SHA256", ""))
    if approval["stage"] == "prepare" or approval["tool_source_sha"] != tool_sha or approval["workflow_scope"]["job_name"] != JOB_NAME:
        fail("platform_window_scope_rejected")
    scope = approval["workflow_scope"]; platform.validate_scope(scope)
    route = pin.route_file(env)
    base = Path(env.get("RUNNER_TEMP", "")).resolve(strict=True)
    state = base / ("qs-retirement-platform-owner-" + approval["operation_id"] + "-" + run_id)
    state.mkdir(mode=0o700); state.chmod(0o700)
    assets = state / "assets"; assets.mkdir(mode=0o700)
    registration = platform._Store(state)
    quarantine = None
    identity_assets = []
    installed = restored = native_terminal = False
    try:
        remote_directory = "/tmp/qs-independent-window-tool." + uuid.uuid4().hex[:12]
        registration.save("runner-owner.intent.json", {"dispatcher_source_sha": dispatcher, "tool_source_sha": tool_sha, "operation_id": approval["operation_id"], "actual_run_id": run_id, "approval_sha256": approval_sha, "workflow_scope_sha256": platform.digest(platform.canonical(scope)), "remote_directory": remote_directory})
        binaries = {arch: read_binary(Path(binary_directory) / ("inventory-linux-" + arch), approval["tool_binary_sha256"][arch]) for arch in ("amd64", "arm64")}
        body = {"compatibility-window-tool.py": window_raw, "receipt-transport.py": source_bytes(tool_repo, tool_sha, "scripts/dbops/receipt-transport.py"), **{"inventory-linux-" + k: v for k, v in binaries.items()}}
        archive = assets / ("qs-compatibility-retirement-" + run_id + ".tar.gz")
        with tarfile.open(archive, "x:gz") as package:
            import io
            for name in ASSETS:
                item = tarfile.TarInfo(name); item.mode = 0o600; item.size = len(body[name]); package.addfile(item, io.BytesIO(body[name]))
        archive.chmod(0o600)
        package_sha = sha(archive.read_bytes())
        fd = os.open(archive, os.O_RDONLY | os.O_NOFOLLOW); os.fsync(fd); os.close(fd)
        private_route = {k: v for k, v in route.items() if k != "key"}
        identity_assets.append(IdentityAsset(assets / "route.json", platform.canonical(private_route)))
        identity_assets.append(IdentityAsset(assets / "ssh.key", route["key"].encode("ascii")))
        # Existing pin helper, copied from the actual dispatcher Git object.
        identity_assets.append(IdentityAsset(assets / "pin.py", source_bytes(dispatcher_repo, dispatcher, "scripts/database/compatibility-host-inventory-action.py")))
        if any(c.isspace() or c in "\"'`$\\" for c in str(assets)): fail("platform_window_route_rejected")
        known = "/usr/bin/python3 " + str(assets / "pin.py") + " known-host --route-file " + str(assets / "route.json") + " --key-type %t --key-blob %K"
        config = ("Host qs-host-inventory\n HostName " + route["host"] + "\n User " + route["username"] + "\n Port " + route["port"] + "\n IdentityFile " + str(assets / "ssh.key") + "\n IdentitiesOnly yes\n PreferredAuthentications publickey\n BatchMode yes\n StrictHostKeyChecking yes\n CheckHostIP no\n HostKeyAlias qs-host-inventory\n UserKnownHostsFile /dev/null\n GlobalKnownHostsFile /dev/null\n KnownHostsCommand " + known + "\n ControlMaster no\n RequestTTY no\n ConnectTimeout 10\n LogLevel ERROR\n").encode("ascii")
        identity_assets.append(IdentityAsset(assets / "ssh.config", config))
        registration.save("ssh-identity-assets.intent.json", {asset.path.name: {"sha256": asset.expected, "identity": list(asset.identity)} for asset in identity_assets})
        for asset in identity_assets: asset.check()
        registration.save("package-transfer.intent.json", {"package_sha256": package_sha, "actual_run_id": run_id, "remote_directory": remote_directory})
        # Reserve an exact private remote namespace before uploading. SCP only
        # writes into this new directory; the fixed native archive is created
        # later with O_EXCL by the source-bound remote bootstrap.
        reserve = "import os,stat; p=" + repr(remote_directory) + "; os.mkdir(p,0o700); s=os.lstat(p); assert stat.S_ISDIR(s.st_mode) and s.st_uid==os.getuid() and stat.S_IMODE(s.st_mode)==0o700; print('reserved')"
        code, reserved = collect_owned(["/usr/bin/ssh", "-F", str(assets / "ssh.config"), "qs-host-inventory", "/usr/bin/python3 -I -c " + shlex.quote(reserve)], timeout=30)
        if code or reserved != b"reserved\n": fail("platform_window_transfer_unknown")
        code, unused = collect_owned(["/usr/bin/scp", "-F", str(assets / "ssh.config"), str(archive), "qs-host-inventory:" + remote_directory + "/" + archive.name], timeout=120)
        if code: fail("platform_window_transfer_unknown")
        for asset in identity_assets: asset.check()
        registration.save("package-transfer.result.json", {"exit_code": code, "eof_and_wait_complete": True})
        credentials = {key: env.get(key, "") for key in window.CREDENTIALS + window.SERVICE_CREDENTIALS}
        credentials[window.SERVICE_KEY] = credentials[window.SERVICE_KEY].replace("\r", "")
        credentials[window.READ_TOKEN] = token
        window.validate_credentials(credentials, approval["stage"])
        root_packet = platform.canonical({"approval": approval_raw, "credentials": credentials, "tool_directory": remote_directory, "tool_program_sha256": sha(window_raw)})
        if len(root_packet) > 32768: fail("platform_window_packet_rejected")
        fence_dir = state / "workflow-lease"; fence_dir.mkdir(mode=0o700)
        quarantine = platform.open_native_workflow_quarantine(scope, platform.digest(platform.canonical(scope)), fence_dir, token.encode("ascii"), total_seconds=TIMEOUT)
        quarantine.install(); installed = True; quarantine.check()
        packet = platform.canonical({"bindings": [approval["stage"], approval["operation_id"], run_id, dispatcher, approval["manifest_sha256"], approval["request_template_sha256"]], "approval": approval_raw, "approval_sha256": approval_sha, "package_sha256": package_sha, "tool_directory": remote_directory, "credentials": credentials})
        if len(packet) > 65536: fail("platform_window_packet_rejected")
        registration.save("native-window.intent.json", {"actual_run_id": run_id, "package_sha256": package_sha, "approval_sha256": approval_sha, "remote_directory": remote_directory})
        command = "/usr/bin/python3 -I -c " + shlex.quote(REMOTE)
        code, output = collect_owned(["/usr/bin/ssh", "-F", str(assets / "ssh.config"), "qs-host-inventory", command], packet=packet)
        for asset in identity_assets: asset.check()
        # This local physical EOF/Wait owner reads only its actual fixed SSH
        # invocation. Nothing accepts an imported stdout/receipt as a caller.
        native_terminal = True
        reason, native = disposition(window, transport, output, code, approval, run_id)
        registration.save("native-window.result.json", {"actual_run_id": run_id, "native_stdout_sha256": sha(output), "exit_code": code, "eof_and_wait_complete": True, "disposition": reason, "native_error_category": native["error_category"]})
        if reason in ("native_preflight_refused", "native_completed", "native_recovered"):
            # The known native terminal result and actual local group EOF/reap
            # end this SSH identity's responsibility. Remove original inodes
            # before reopening ordinary workflow entrypoints.
            for asset in identity_assets: asset.remove()
            registration.save("ssh-identity-assets.result.json", {"remaining_owned_identity_assets": 0})
            quarantine.restore(); restored = True
        else:
            fail("platform_window_native_unknown")
        return {"protocol": "runner_platform_window_owner_v1", "dispatcher_source_sha": dispatcher, "tool_source_sha": tool_sha, "operation_id": approval["operation_id"], "actual_run_id": run_id, "workflow_scope_sha256": platform.digest(platform.canonical(scope)), "platform_installed": installed, "platform_restored": restored, "native_channel_terminal": native_terminal, "native_disposition": reason, "native_stdout_sha256": sha(output), "whole_writer_fence_proven": False, "drop_ready": False}, code
    finally:
        # Unknown native/PUT/cleanup retains all registered assets and original
        # states. No generic finally enables workflows or adopts an old owner.
        token = ""
        env.clear()
        if quarantine is not None: quarantine.close()
        for asset in identity_assets: asset.close()
        registration.close()


def emit(result, secrets):
    transport = load(Path(__file__).resolve().parents[1] / "dbops/receipt-transport.py", "runner_receipt")
    errors = frozenset({"platform_window_source_rejected", "platform_window_package_rejected", "platform_window_native_unknown", "platform_window_scope_rejected", "platform_window_route_rejected", "platform_window_transfer_unknown", "platform_window_packet_rejected", "platform_window_identity_cleanup_unknown", "platform_window_operation_unknown"})
    schema = {"protocol": frozenset({"runner_platform_window_owner_v1"}), "dispatcher_source_sha": "sha40", "tool_source_sha": "sha40", "operation_id": "run_id", "actual_run_id": "run_id", "workflow_scope_sha256": "hash64", "platform_installed": "bool", "platform_restored": "bool", "native_channel_terminal": "bool", "native_disposition": frozenset({"native_preflight_refused", "native_completed", "native_recovered"}), "native_stdout_sha256": "hash64", "whole_writer_fence_proven": "bool", "drop_ready": "bool", "error_category": errors}
    print(transport.encode_armored_receipt(result, schema=schema, secrets=secrets))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dispatcher-repo", required=True)
    parser.add_argument("--tool-repo", required=True)
    parser.add_argument("--binary-directory", required=True)
    args = parser.parse_args()
    secrets = tuple(os.environ.get(k, "") for k in ("RETIREMENT_PLATFORM_TOKEN", "HOST_INVENTORY_SSH_KEY", "MYSQL_HOST", "MYSQL_PORT", "MYSQL_USERNAME", "MYSQL_PASSWORD", "MYSQL_DATABASE", "MONGODB_HOST", "MONGODB_PORT", "MONGODB_USERNAME", "MONGODB_PASSWORD", "MONGODB_DBNAME", "MONGODB_METADATA_ADMIN_USERNAME", "MONGODB_METADATA_ADMIN_PASSWORD"))
    try:
        result, code = run(args.dispatcher_repo, args.tool_repo, args.binary_directory)
        emit(result, secrets)
        return code
    except BaseException as error:
        category = str(error) if isinstance(error, Rejected) else "platform_window_operation_unknown"
        emit({"protocol": "runner_platform_window_owner_v1", "error_category": category, "whole_writer_fence_proven": False, "drop_ready": False}, secrets)
        return 1


if __name__ == "__main__":
    sys.exit(main())

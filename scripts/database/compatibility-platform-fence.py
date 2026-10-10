#!/usr/bin/env python3
"""Same-run runner workflow quarantine, without host/DB writer authority.

The public producer uses fixed native HTTPS endpoints and a short-lived token
kept only in memory. It never adopts saved success or retries an unknown PUT.
Restoration is explicit; Close only releases resources and clears credentials.
"""
import copy
import fcntl
import hashlib
import json
import os
import platform
from pathlib import Path
import re
import resource
import stat
import time
import urllib.error
import urllib.request

API = "https://api.github.com"
REPO = "FangcunMount/qs-server"
WORKFLOW = ".github/workflows/compatibility-retirement.yml"
SHA = re.compile(r"[0-9a-f]{40}")
HASH = re.compile(r"[0-9a-f]{64}")
DECIMAL = re.compile(r"[1-9][0-9]{0,19}")
RUN = re.compile(r"[1-9][0-9]{0,19}-[1-9][0-9]{0,3}")
LIMIT = 2 << 20
STATES = {"active", "disabled_manually", "disabled_inactivity", "disabled_fork"}


class Refused(Exception):
    """Only a fixed category leaves the producer."""


def fail(category):
    raise Refused(category)


def canonical(value):
    return (json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True) + "\n").encode("ascii")


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def decode(raw):
    def unique(items):
        result = {}
        for key, value in items:
            if key in result:
                fail("platform_fence_schema_rejected")
            result[key] = value
        return result
    try:
        return json.loads(raw, object_pairs_hook=unique, parse_constant=lambda _: fail("platform_fence_schema_rejected"))
    except Refused:
        raise
    except (ValueError, UnicodeError):
        fail("platform_fence_schema_rejected")


def validate_scope(scope):
    fields = {"format_version", "kind", "dispatcher_source_sha", "tool_source_sha", "original_source_sha",
              "operation_id", "original_run_id", "manifest_sha256", "repository_id", "owner_id", "actor_id",
              "workflow_id", "workflow_ids", "job_name", "runner_id"}
    if type(scope) is not dict or set(scope) != fields or type(scope["format_version"]) is not int or scope["format_version"] != 1 or scope["kind"] != "approved_runner_workflow_quarantine_scope":
        fail("platform_fence_scope_rejected")
    for key in ("dispatcher_source_sha", "tool_source_sha", "original_source_sha"):
        if type(scope[key]) is not str or SHA.fullmatch(scope[key]) is None:
            fail("platform_fence_scope_rejected")
    for key in ("operation_id", "original_run_id"):
        if type(scope[key]) is not str or RUN.fullmatch(scope[key]) is None:
            fail("platform_fence_scope_rejected")
    if type(scope["manifest_sha256"]) is not str or HASH.fullmatch(scope["manifest_sha256"]) is None:
        fail("platform_fence_scope_rejected")
    for key in ("repository_id", "owner_id", "actor_id"):
        if type(scope[key]) is not str or DECIMAL.fullmatch(scope[key]) is None:
            fail("platform_fence_scope_rejected")
    for key in ("workflow_id", "runner_id"):
        if type(scope[key]) is not int or not 0 < scope[key] < 1 << 63:
            fail("platform_fence_scope_rejected")
    ids = scope["workflow_ids"]
    if type(ids) is not list or not 1 <= len(ids) <= 1000 or any(type(i) is not int or not 0 < i < 1 << 63 for i in ids) or len(set(ids)) != len(ids) or ids != sorted(ids) or scope["workflow_id"] not in ids:
        fail("platform_fence_scope_rejected")
    if type(scope["job_name"]) is not str or re.fullmatch(r"[A-Za-z0-9 ()_.-]{1,200}", scope["job_name"]) is None:
        fail("platform_fence_scope_rejected")


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        fail("platform_fence_redirect_rejected")


class _NativeAPI:
    def __init__(self, token, deadline):
        if type(token) is not bytes or not 1 <= len(token) <= 8192 or any(c < 33 or c > 126 for c in token):
            fail("platform_fence_credential_rejected")
        self._token = bytearray(token)
        self._deadline = deadline
        self._opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), _NoRedirect())

    def call(self, method, path, expected=200):
        if method not in ("GET", "PUT") or path != "/repos/" + REPO and not path.startswith("/repos/" + REPO + "/") or not re.fullmatch(r"/[A-Za-z0-9_./?=&-]+", path) or ".." in path:
            fail("platform_fence_endpoint_rejected")
        if method == "PUT" and re.fullmatch(r"/repos/" + REPO + r"/actions/workflows/[1-9][0-9]*/(?:disable|enable)", path) is None:
            fail("platform_fence_endpoint_rejected")
        remaining = self._deadline - time.monotonic()
        if remaining <= 0:
            fail("platform_fence_deadline_exceeded")
        address = API + path
        request = urllib.request.Request(address, method=method, data=b"" if method == "PUT" else None,
            headers={"Authorization": "Bearer " + self._token.decode("ascii"), "Accept": "application/vnd.github+json", "X-GitHub-Api-Version": "2022-11-28"})
        try:
            with self._opener.open(request, timeout=min(15, remaining)) as response:
                raw = response.read(LIMIT + 1)
                if response.status != expected or response.geturl() != address or len(raw) > LIMIT or time.monotonic() >= self._deadline:
                    fail("platform_fence_remote_unknown")
                if expected == 204:
                    if raw:
                        fail("platform_fence_remote_unknown")
                    return None
                decoded = decode(raw)
                if type(decoded) is not dict:
                    fail("platform_fence_schema_rejected")
                return decoded
        except Refused:
            raise
        except (OSError, urllib.error.URLError, TimeoutError, ValueError):
            fail("platform_fence_remote_unknown")

    def close(self):
        self._token[:] = b"\x00" * len(self._token)


class _Store:
    def __init__(self, path):
        self.path = Path(path)
        if not self.path.is_absolute() or str(self.path) != os.path.normpath(str(self.path)) or self.path.resolve(strict=True) != self.path:
            fail("platform_fence_store_rejected")
        self.fd = os.open(self.path, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        self.stamp = os.fstat(self.fd)
        self.files = {}
        self.ancestors = []
        if self.stamp.st_uid != os.geteuid() or stat.S_IMODE(self.stamp.st_mode) != 0o700 or not stat.S_ISDIR(self.stamp.st_mode):
            self.close(); fail("platform_fence_store_rejected")
        try:
            fcntl.flock(self.fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except OSError:
            self.close(); fail("platform_fence_store_busy")
        try:
            for parent in self.path.parents:
                fd = os.open(parent, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
                info = os.fstat(fd)
                self.ancestors.append((parent, fd, info))
                if info.st_uid not in (0, os.geteuid()) or stat.S_IMODE(info.st_mode) & 0o022 and not info.st_mode & stat.S_ISVTX:
                    fail("platform_fence_store_rejected")
        except Exception:
            self.close()
            fail("platform_fence_store_rejected")

    def check(self):
        if self.fd is None:
            fail("platform_fence_store_rejected")
        for path, fd, expected in self.ancestors:
            identity = lambda info: (info.st_dev, info.st_ino, info.st_uid, info.st_gid, info.st_mode)
            if identity(os.fstat(fd)) != identity(expected) or identity(path.lstat()) != identity(expected):
                fail("platform_fence_store_changed")
        actual, named = os.fstat(self.fd), self.path.lstat()
        for value in (actual, named):
            if (value.st_dev, value.st_ino, value.st_uid, stat.S_IMODE(value.st_mode)) != (self.stamp.st_dev, self.stamp.st_ino, self.stamp.st_uid, 0o700) or not stat.S_ISDIR(value.st_mode):
                fail("platform_fence_store_changed")
        for name, (identity, raw, fd) in self.files.items():
            info = os.fstat(fd)
            if (info.st_dev, info.st_ino, info.st_uid, stat.S_IMODE(info.st_mode), info.st_nlink, info.st_size) != identity or os.pread(fd, LIMIT + 1, 0) != raw:
                fail("platform_fence_store_changed")
            after = os.fstat(fd)
            named = os.stat(name, dir_fd=self.fd, follow_symlinks=False)
            for value in (after, named):
                if (value.st_dev, value.st_ino, value.st_uid, stat.S_IMODE(value.st_mode), value.st_nlink, value.st_size) != identity or not stat.S_ISREG(value.st_mode):
                    fail("platform_fence_store_changed")

    def save(self, name, value):
        self.check()
        raw = canonical(value)
        if len(raw) > LIMIT or re.fullmatch(r"[a-z0-9.-]+\.json", name) is None:
            fail("platform_fence_store_rejected")
        try:
            fd = os.open(name, os.O_RDWR | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600, dir_fd=self.fd)
        except FileExistsError:
            fail("platform_fence_existing_or_unknown")
        try:
            if os.write(fd, raw) != len(raw):
                fail("platform_fence_store_unknown")
            os.fsync(fd)
            info = os.fstat(fd)
            identity = (info.st_dev, info.st_ino, info.st_uid, stat.S_IMODE(info.st_mode), info.st_nlink, info.st_size)
            if identity[2:] != (os.geteuid(), 0o600, 1, len(raw)) or not stat.S_ISREG(info.st_mode):
                fail("platform_fence_store_unknown")
            self.files[name] = (identity, raw, fd)
        finally:
            if name not in self.files:
                os.close(fd)
        os.fsync(self.fd)
        self.check()
        return digest(raw)

    def close(self):
        if self.fd is not None:
            for _, _, fd in self.files.values():
                os.close(fd)
            self.files.clear()
            for _, fd, _ in self.ancestors:
                os.close(fd)
            self.ancestors.clear()
            os.close(self.fd)
            self.fd = None


def _listed(api, suffix, key, maximum=1000):
    seen, total = {}, None
    for page in range(1, 12):
        value = api.call("GET", "/repos/" + REPO + suffix + ("&" if "?" in suffix else "?") + "per_page=100&page=" + str(page))
        if type(value) is not dict or type(value.get("total_count")) is not int or type(value.get(key)) is not list:
            fail("platform_fence_coverage_rejected")
        if total is None:
            total = value["total_count"]
        rows = value[key]
        if value["total_count"] != total or not 0 <= total <= maximum or len(rows) > 100:
            fail("platform_fence_coverage_rejected")
        for row in rows:
            if type(row) is not dict or type(row.get("id")) is not int or row["id"] <= 0 or row["id"] in seen:
                fail("platform_fence_coverage_rejected")
            seen[row["id"]] = row
        if len(rows) < 100:
            if len(seen) != total:
                fail("platform_fence_coverage_rejected")
            return seen
    fail("platform_fence_coverage_rejected")


def _snapshot(api, scope, run, quarantined):
    base = "/repos/" + REPO
    repository = api.call("GET", base)
    if str(repository.get("id")) != scope["repository_id"] or repository.get("full_name") != REPO or str(repository.get("owner", {}).get("id")) != scope["owner_id"]:
        fail("platform_fence_identity_rejected")
    if api.call("GET", base + "/commits/main").get("sha") != scope["dispatcher_source_sha"]:
        fail("platform_fence_old_ref_rejected")
    rid, attempt = run.split("-")
    current = api.call("GET", base + "/actions/runs/" + rid + "/attempts/" + attempt)
    if (str(current.get("id")), str(current.get("run_attempt")), current.get("workflow_id"), current.get("head_sha"), current.get("head_branch"), current.get("event"), current.get("status"), current.get("path")) != (rid, attempt, scope["workflow_id"], scope["dispatcher_source_sha"], "main", "workflow_dispatch", "in_progress", WORKFLOW):
        fail("platform_fence_identity_rejected")
    for key in ("actor", "triggering_actor"):
        if str(current.get(key, {}).get("id")) != scope["actor_id"]:
            fail("platform_fence_identity_rejected")
    for key in ("repository", "head_repository"):
        if str(current.get(key, {}).get("id")) != scope["repository_id"] or current[key].get("full_name") != REPO:
            fail("platform_fence_identity_rejected")
    workflows = _listed(api, "/actions/workflows", "workflows")
    if sorted(workflows) != scope["workflow_ids"]:
        fail("platform_fence_coverage_rejected")
    for wid, workflow in workflows.items():
        if type(workflow.get("path")) is not str or workflow.get("state") not in STATES:
            fail("platform_fence_coverage_rejected")
        if wid == scope["workflow_id"]:
            if workflow["path"] != WORKFLOW or workflow["state"] != "active":
                fail("platform_fence_identity_rejected")
        elif quarantined and workflow["state"] == "active":
            fail("platform_fence_entrypoint_open")
    queue = []
    for status in ("queued", "in_progress", "waiting", "pending", "requested"):
        for row in _listed(api, "/actions/runs?status=" + status, "workflow_runs").values():
            if (str(row.get("id")), str(row.get("run_attempt")), row.get("workflow_id"), row.get("status"), row.get("head_sha")) != (rid, attempt, scope["workflow_id"], "in_progress", scope["dispatcher_source_sha"]) or status != "in_progress":
                fail("platform_fence_other_active_run")
            if str(row.get("repository", {}).get("id")) != scope["repository_id"] or row.get("repository", {}).get("full_name") != REPO:
                fail("platform_fence_identity_rejected")
            queue.append(row["id"])
    if queue != [int(rid)]:
        fail("platform_fence_coverage_rejected")
    jobs = _listed(api, "/actions/runs/" + rid + "/attempts/" + attempt + "/jobs", "jobs")
    matched = []
    for job in jobs.values():
        if str(job.get("run_id")) != rid or job.get("head_sha") != scope["dispatcher_source_sha"]:
            fail("platform_fence_identity_rejected")
        if job.get("name") == scope["job_name"]:
            if job.get("status") != "in_progress" or job.get("runner_id") != scope["runner_id"]:
                fail("platform_fence_identity_rejected")
            matched.append(job["id"])
        elif job.get("status") != "completed":
            fail("platform_fence_other_active_job")
    if len(matched) != 1:
        fail("platform_fence_coverage_rejected")
    return {"workflows": {str(wid): {k: w[k] for k in ("id", "path", "state")} for wid, w in sorted(workflows.items())}, "job_id": matched[0], "actual_run_id": run}


class WorkflowQuarantine:
    """One live runner lease only; receipts never reconstruct it."""
    def __init__(self, api, store, scope, run):
        self._self = self
        self._api, self._store, self._scope, self._run = api, store, copy.deepcopy(scope), run
        self._original = None
        self._attempted = self._unknown = self._installed = self._restore_attempted = self._restored = self._closed = False

    def _live(self):
        if self._self is not self or self._closed or self._unknown:
            fail("platform_fence_live_lease_rejected")
        try:
            self._store.check()
        except OSError:
            fail("platform_fence_store_changed")

    def _expected_installed(self):
        expected = copy.deepcopy(self._original)
        for wid, row in expected["workflows"].items():
            if int(wid) != self._scope["workflow_id"] and row["state"] == "active":
                row["state"] = "disabled_manually"
        return expected

    def install(self):
        self._live()
        if self._attempted:
            fail("platform_fence_existing_or_unknown")
        original = _snapshot(self._api, self._scope, self._run, False)
        if original != _snapshot(self._api, self._scope, self._run, False):
            fail("platform_fence_snapshot_changed")
        self._attempted = True
        try:
            self._store.save("platform-install.intent.json", {"scope": self._scope, "actual_run_id": self._run, "original": original})
            self._original = original
            for wid, row in original["workflows"].items():
                if int(wid) == self._scope["workflow_id"] or row["state"] != "active":
                    continue
                self._store.save("workflow-" + wid + ".disable.intent.json", row)
                self._api.call("PUT", "/repos/" + REPO + "/actions/workflows/" + wid + "/disable", 204)
                current = self._api.call("GET", "/repos/" + REPO + "/actions/workflows/" + wid)
                if {k: current.get(k) for k in ("id", "path", "state")} != {**row, "state": "disabled_manually"}:
                    fail("platform_fence_mutation_unknown")
                self._store.save("workflow-" + wid + ".disable.result.json", {"id": row["id"], "path": row["path"], "state": "disabled_manually"})
            observed = _snapshot(self._api, self._scope, self._run, True)
            if observed != self._expected_installed() or observed != _snapshot(self._api, self._scope, self._run, True):
                fail("platform_fence_snapshot_changed")
            self._store.save("platform-install.result.json", observed)
            self._installed = True
        except Exception as error:
            self._unknown = True
            if isinstance(error, Refused):
                raise
            fail("platform_fence_operation_unknown")

    def check(self):
        self._live()
        if not self._installed or self._restored:
            fail("platform_fence_live_lease_rejected")
        observed = _snapshot(self._api, self._scope, self._run, True)
        if observed != self._expected_installed() or observed != _snapshot(self._api, self._scope, self._run, True):
            fail("platform_fence_snapshot_changed")
        self._store.check()
        return {"protocol": "runner-workflow-quarantine/v1", "observed_sha256": digest(canonical(observed)), "workflow_count": len(observed["workflows"]), "actual_run_id": self._run, "whole_writer_fence_proven": False, "drop_ready": False}

    def restore(self):
        self.check()
        if self._restore_attempted:
            fail("platform_fence_existing_or_unknown")
        self._restore_attempted = True
        try:
            self._store.save("platform-restore.intent.json", {"actual_run_id": self._run, "original_sha256": digest(canonical(self._original))})
            for wid, row in self._original["workflows"].items():
                if int(wid) == self._scope["workflow_id"] or row["state"] != "active":
                    continue
                self._store.save("workflow-" + wid + ".enable.intent.json", row)
                self._api.call("PUT", "/repos/" + REPO + "/actions/workflows/" + wid + "/enable", 204)
                current = self._api.call("GET", "/repos/" + REPO + "/actions/workflows/" + wid)
                if {k: current.get(k) for k in ("id", "path", "state")} != row:
                    fail("platform_fence_mutation_unknown")
                self._store.save("workflow-" + wid + ".enable.result.json", row)
            observed = _snapshot(self._api, self._scope, self._run, False)
            if observed != self._original or observed != _snapshot(self._api, self._scope, self._run, False):
                fail("platform_fence_snapshot_changed")
            self._store.save("platform-restore.result.json", observed)
            self._restored = True
        except Exception as error:
            self._unknown = True
            if isinstance(error, Refused):
                raise
            fail("platform_fence_operation_unknown")

    def close(self):
        if self._self is not self or self._closed:
            return
        self._closed = True
        self._api.close()
        self._store.close()  # No implicit enable, journal removal or adoption.


def open_native_workflow_quarantine(scope, approved_sha256, directory, short_lived_token, *, total_seconds=1800):
    """Expected bindings only; actual API reads and native file writes follow.

    The host owns this live object for the entire controlled run, including
    unknown outcomes. It must explicitly restore only after verified acceptance
    or completed recovery, never merely in a generic finally block.
    """
    validate_scope(scope)
    # This runner API owner spans prewindow preparation and the original native
    # window; it is not a boot-clock Window and grants no maintenance extension.
    if type(approved_sha256) is not str or HASH.fullmatch(approved_sha256) is None or digest(canonical(scope)) != approved_sha256 or type(total_seconds) is not int or not 1 <= total_seconds <= 115 * 60:
        fail("platform_fence_scope_rejected")
    rid, attempt = os.environ.get("GITHUB_RUN_ID", ""), os.environ.get("GITHUB_RUN_ATTEMPT", "")
    run = rid + "-" + attempt
    if RUN.fullmatch(run) is None or os.environ.get("GITHUB_SHA") != scope["dispatcher_source_sha"] or os.environ.get("GITHUB_REPOSITORY") != REPO or os.environ.get("GITHUB_EVENT_NAME") != "workflow_dispatch" or os.environ.get("GITHUB_REF") != "refs/heads/main":
        fail("platform_fence_runner_binding_rejected")
    soft, _ = resource.getrlimit(resource.RLIMIT_NOFILE)
    system = platform.system()
    descriptor_path = {"Linux": "/proc/self/fd", "Darwin": "/dev/fd"}.get(system)
    if soft != resource.RLIM_INFINITY:
        if descriptor_path is None:
            fail("platform_fence_descriptor_budget_unproven")
        # Fixed kernel descriptor directory for the actual runner platform.
        # No caller-supplied count or host/root capability is accepted.
        try:
            current = len(os.listdir(descriptor_path))
        except OSError:
            fail("platform_fence_descriptor_budget_unproven")
        if current + 4 * len(scope["workflow_ids"]) + 64 >= soft:
            fail("platform_fence_descriptor_budget_unproven")
    api = _NativeAPI(short_lived_token, time.monotonic() + total_seconds)
    try:
        store = _Store(directory)
    except Exception as error:
        api.close()
        if isinstance(error, Refused):
            raise
        fail("platform_fence_store_rejected")
    return WorkflowQuarantine(api, store, scope, run)

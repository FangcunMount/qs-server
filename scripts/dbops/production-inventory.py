#!/usr/bin/env python3
"""Read namespace/index/storage metadata only; never print raw client output."""
import contextlib
import datetime
import json
import os
from pathlib import Path
import re
import selectors
import signal
import subprocess
import sys
import tempfile
import time
import uuid

NAME = re.compile(r"^[A-Za-z0-9_.$:*-]{1,256}$")
SHA = re.compile(r"^[0-9a-f]{40}$")
MAX_ROWS = {"server": 1, "tables": 1000, "columns": 10000, "indexes": 10000, "migration": 2}


class InventoryError(Exception):
    pass


def fail(category):
    raise InventoryError(category)


def name(value):
    if not isinstance(value, str) or not NAME.fullmatch(value):
        fail("invalid_metadata_name")
    return value


def integer(value, nullable=False):
    if value is None or value == "NULL":
        if nullable:
            return None
        fail("invalid_metadata_number")
    if isinstance(value, bool) or not re.fullmatch(r"[0-9]{1,20}", str(value)):
        fail("invalid_metadata_number")
    return int(value)


def parse_mysql(raw, database):
    result = {"database": database, "tables": [], "columns": [], "indexes": [], "migration_state": []}
    counts = dict.fromkeys(MAX_ROWS, 0)
    for line in raw.splitlines():
        values = line.split("\t")
        kind = values.pop(0)
        if kind not in counts:
            fail("unexpected_mysql_output")
        counts[kind] += 1
        if counts[kind] > MAX_ROWS[kind]:
            fail("mysql_metadata_limit_exceeded")
        if kind == "server" and len(values) == 2:
            if values[0] != database or not re.fullmatch(r"[0-9][A-Za-z0-9.+~_-]{0,99}", values[1]):
                fail("mysql_database_identity_mismatch")
            result["server_version"] = values[1]
        elif kind == "tables" and len(values) == 6:
            table, table_type, engine, rows, size, index_size = values
            if table_type not in ("BASE TABLE", "VIEW"):
                fail("invalid_mysql_table_type")
            result["tables"].append({"name": name(table), "type": table_type, "engine": None if engine == "NULL" else name(engine), "estimated_rows": integer(rows, True), "data_bytes": integer(size, True), "index_bytes": integer(index_size, True)})
        elif kind == "columns" and len(values) == 7:
            table, position, field, data_type, nullable, char_length, numeric_precision = values
            if nullable not in ("YES", "NO"):
                fail("invalid_mysql_nullable")
            result["columns"].append({"table": name(table), "position": integer(position), "name": name(field), "data_type": name(data_type), "nullable": nullable == "YES", "character_maximum_length": integer(char_length, True), "numeric_precision": integer(numeric_precision, True)})
        elif kind == "indexes" and len(values) == 8:
            table, index, non_unique, position, field, prefix_length, index_type, visible = values
            if non_unique not in ("0", "1") or visible not in ("YES", "NO"):
                fail("invalid_mysql_index_flags")
            result["indexes"].append({"table": name(table), "name": name(index), "unique": non_unique == "0", "position": integer(position), "field": None if field == "NULL" else name(field), "prefix_length": integer(prefix_length, True), "type": name(index_type), "visible": visible == "YES"})
        elif kind == "migration" and len(values) == 2 and values[1] in ("0", "1"):
            result["migration_state"].append({"version": integer(values[0]), "dirty": values[1] == "1"})
        else:
            fail("unexpected_mysql_output_shape")
    if counts["server"] != 1 or counts["migration"] != 1 or counts["tables"] < 1:
        fail("mysql_metadata_incomplete")
    return result


def parse_mongo(raw, database):
    try:
        result = json.loads(raw)
    except (ValueError, TypeError):
        fail("unexpected_mongo_output")
    if not isinstance(result, dict) or set(result) != {"database", "namespaces", "migration_state"} or result["database"] != database:
        fail("unexpected_mongo_output_shape")
    if not isinstance(result["namespaces"], list) or len(result["namespaces"]) > 1000:
        fail("mongo_metadata_limit_exceeded")
    total_indexes = 0
    total_fields = 0
    for item in result["namespaces"]:
        required = {"name", "type", "validator_fields", "storage", "indexes"}
        if not isinstance(item, dict) or set(item) != required or item["type"] not in ("collection", "view", "timeseries"):
            fail("unexpected_mongo_namespace_shape")
        name(item["name"])
        if not isinstance(item["validator_fields"], list) or len(item["validator_fields"]) > 1000:
            fail("mongo_metadata_limit_exceeded")
        total_fields += len(item["validator_fields"])
        if total_fields > 10000:
            fail("mongo_metadata_limit_exceeded")
        for field in item["validator_fields"]:
            name(field)
        if item["storage"] is not None:
            if set(item["storage"]) != {"estimated_documents", "data_bytes", "storage_bytes", "index_bytes"}:
                fail("unexpected_mongo_storage_shape")
            item["storage"] = {key: integer(value, True) for key, value in item["storage"].items()}
        if not isinstance(item["indexes"], list) or len(item["indexes"]) > 1000:
            fail("mongo_metadata_limit_exceeded")
        total_indexes += len(item["indexes"])
        if total_indexes > 10000:
            fail("mongo_metadata_limit_exceeded")
        for index in item["indexes"]:
            if not isinstance(index, dict) or set(index) != {"name", "keys", "unique", "sparse", "hidden", "partial", "expire_after_seconds"}:
                fail("unexpected_mongo_index_shape")
            name(index["name"])
            if not isinstance(index["keys"], dict) or len(index["keys"]) > 1000:
                fail("unexpected_mongo_index_keys")
            for field, direction in index["keys"].items():
                name(field)
                if isinstance(direction, bool) or direction not in (1, -1, "text", "hashed", "2d", "2dsphere", "geoHaystack", "columnstore"):
                    fail("invalid_mongo_index_direction")
            for flag in ("unique", "sparse", "hidden", "partial"):
                if not isinstance(index[flag], bool):
                    fail("invalid_mongo_index_flags")
            index["expire_after_seconds"] = integer(index["expire_after_seconds"], True)
    heads = result["migration_state"]
    if not isinstance(heads, list) or len(heads) != 1 or not isinstance(heads[0], dict) or set(heads[0]) != {"version", "dirty"} or not isinstance(heads[0]["dirty"], bool):
        fail("mongo_metadata_incomplete")
    heads[0]["version"] = integer(heads[0]["version"])
    return result


def capture(args, *, input_text=None, timeout=15):
    # Enforce the host-side budget while reading, including discarded stderr.
    # Docker's container memory limit does not bound subprocess pipe capture.
    process = None
    stdout = bytearray()
    total = 0
    deadline = time.monotonic() + timeout
    if input_text is not None and len(input_text.encode()) > 4096:
        fail("client_input_limit_exceeded")
    try:
        process = subprocess.Popen(args, stdin=subprocess.PIPE if input_text is not None else subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
        if input_text is not None:
            try:
                process.stdin.write(input_text.encode())
                process.stdin.close()
            except BrokenPipeError:
                pass
        with selectors.DefaultSelector() as streams:
            streams.register(process.stdout, selectors.EVENT_READ, "stdout")
            streams.register(process.stderr, selectors.EVENT_READ, "stderr")
            while streams.get_map():
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    fail("client_execution_timed_out")
                for key, _ in streams.select(min(remaining, 1)):
                    chunk = os.read(key.fileobj.fileno(), 65536)
                    if not chunk:
                        streams.unregister(key.fileobj)
                        continue
                    total += len(chunk)
                    if total > 4 * 1024 * 1024:
                        fail("client_output_limit_exceeded")
                    if key.data == "stdout":
                        stdout.extend(chunk)
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            fail("client_execution_timed_out")
        return process.wait(timeout=remaining), stdout.decode("utf-8")
    except (OSError, subprocess.TimeoutExpired, UnicodeError):
        fail("client_execution_failed_or_timed_out")
    finally:
        if process is not None:
            # Always terminate this isolated group, even if its leader exited
            # first and left a descendant holding a pipe. sudo gets a TERM
            # forwarding window before KILL; owned-container cleanup follows.
            with contextlib.suppress(ProcessLookupError, PermissionError):
                os.killpg(process.pid, signal.SIGTERM)
            with contextlib.suppress(subprocess.TimeoutExpired):
                process.wait(timeout=2)
            with contextlib.suppress(ProcessLookupError, PermissionError):
                os.killpg(process.pid, signal.SIGKILL)
            if process.poll() is None:
                process.kill()
            with contextlib.suppress(subprocess.TimeoutExpired):
                process.wait(timeout=2)
            for stream in (process.stdin, process.stdout, process.stderr):
                if stream is not None:
                    stream.close()


def invoke(args, *, input_text=None, timeout=15):
    status, output = capture(args, input_text=input_text, timeout=timeout)
    if status != 0:
        fail("client_execution_failed")
    return output


def validate_binding(env, kind):
    prefix = "MYSQL" if kind == "mysql" else "MONGODB"
    database_key = "MYSQL_DATABASE" if kind == "mysql" else "MONGODB_DBNAME"
    for key in (prefix + "_HOST", prefix + "_USERNAME", prefix + "_PASSWORD", database_key):
        if not env.get(key):
            fail("database_environment_missing")
        if any(char in env[key] for char in ("\x00", "\r", "\n")):
            fail("database_environment_control_character")
    database = name(env[database_key])
    if database.lower() in ("admin", "config", "local", "mysql", "sys", "information_schema", "performance_schema"):
        fail("system_database_not_allowed")
    if not re.fullmatch(r"[A-Za-z0-9_.:-]+", env[prefix + "_HOST"]):
        fail("database_host_invalid")
    port = env.get(prefix + "_PORT", "") or ("3306" if kind == "mysql" else "27017")
    if not re.fullmatch(r"[0-9]{1,5}", port) or not 1 <= int(port) <= 65535:
        fail("database_port_invalid")
    return database, port


def mysql_option(value):
    # MySQL quoted option values: escape backslashes before quote/control escapes.
    return '"' + value.replace("\\", "\\\\").replace('"', '\\"').replace("\t", "\\t").replace("\b", "\\b") + '"'


@contextlib.contextmanager
def connection_file(kind, env, database, port):
    # Docker reads these files itself, so sudo env_reset cannot lose credentials.
    # Neither password nor username appears in Docker/client argv or public output.
    with tempfile.TemporaryDirectory(prefix="qs-db-inventory-connection-") as directory:
        root = Path(directory)
        path = root / ("mysql.cnf" if kind == "mysql" else "mongodb.env")
        if kind == "mysql":
            values = {"host": env["MYSQL_HOST"], "port": port, "user": env["MYSQL_USERNAME"], "password": env["MYSQL_PASSWORD"], "database": database}
            payload = "[client]\n" + "".join(key + "=" + mysql_option(value) + "\n" for key, value in values.items())
        else:
            values = {key: env[key] for key in ("MONGODB_HOST", "MONGODB_USERNAME", "MONGODB_PASSWORD", "MONGODB_DBNAME")}
            values["MONGODB_PORT"] = port
            payload = "".join(key + "=" + value + "\n" for key, value in values.items())
        descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(descriptor, "w") as stream:
            stream.write(payload)
        yield root, path


def cleanup_client(docker, owner):
    # The random label is this operation's ownership proof, independent of sudo
    # umask and shared names. Only exact IDs are ever passed to rm.
    query = [*docker, "ps", "--all", "--quiet", "--no-trunc", "--filter", "label=com.fangcunmount.qs-server.metadata-inventory-owner=" + owner]
    try:
        status, output = capture(query, timeout=8)
        ids = output.splitlines()
        if status != 0 or len(ids) > 1 or any(not re.fullmatch(r"[0-9a-f]{64}", value) for value in ids):
            fail("client_cleanup_unconfirmed")
        if ids:
            capture([*docker, "rm", "--force", ids[0]], timeout=8)
            status, output = capture(query, timeout=8)
            if status != 0 or output.strip():
                fail("client_cleanup_unconfirmed")
    except InventoryError:
        fail("client_cleanup_unconfirmed")


def collect(kind, env, docker, script_dir, run_id):
    database, port = validate_binding(env, kind)
    image = "mysql:8.0" if kind == "mysql" else "mongo:7.0"
    try:
        invoke([*docker, "image", "inspect", "--format", "{{.Id}}", image])
    except InventoryError:
        fail("client_image_unavailable")
    container = "qs-db-inventory-" + run_id + "-" + kind
    # Refuse collision instead of cleaning a container owned by another operation.
    try:
        present, _ = capture([*docker, "container", "inspect", "--format", "{{.Id}}", container], timeout=10)
    except InventoryError:
        fail("container_preflight_failed")
    if present == 0:
        fail("inventory_container_name_conflict")
    common = [*docker, "run", "--rm", "--name", container, "--pull=never", "--network", "infra-network", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--cpus=0.5", "--memory=512m", "--pids-limit=64", "--user", str(os.getuid()) + ":" + str(os.getgid()), "--tmpfs", "/tmp:rw,noexec,nosuid,size=16m,mode=1777", "--mount", "type=bind,source=" + str(script_dir) + ",target=/audit,readonly", "--env", "HOME=/tmp"]
    with connection_file(kind, env, database, port) as (connection_dir, config):
        owner = uuid.uuid4().hex
        common += ["--label", "com.fangcunmount.qs-server.metadata-inventory-owner=" + owner]
        try:
            if kind == "mysql":
                command = [*common, "-i", "--mount", "type=bind,source=" + str(connection_dir) + ",target=/connection,readonly", image, "mysql", "--defaults-file=/connection/mysql.cnf", "--protocol=tcp", "--connect-timeout=5", "--batch", "--skip-column-names"]
                raw = invoke(command, input_text=(script_dir / "production-inventory.sql").read_text(), timeout=75)
                return parse_mysql(raw, database)
            command = [*common, "--env-file", str(config), image, "mongosh", "--nodb", "--quiet", "--file", "/audit/production-inventory.js"]
            return parse_mongo(invoke(command, timeout=75), database)
        finally:
            cleanup_client(docker, owner)


def run(env):
    source_sha = env.get("INVENTORY_SOURCE_SHA", "")
    scope = env.get("INVENTORY_DATABASE", "")
    run_id = env.get("INVENTORY_RUN_ID", "")
    if not SHA.fullmatch(source_sha) or scope not in ("mysql", "mongodb", "all") or not re.fullmatch(r"[0-9]{1,20}-[0-9]{1,4}", run_id):
        fail("invalid_inventory_binding")
    kinds = ("mysql", "mongodb") if scope == "all" else (scope,)
    for kind in kinds:
        validate_binding(env, kind)
    docker = ["sudo", "-n", "docker"]
    invoke([*docker, "network", "inspect", "--format", "{{.Name}}", "infra-network"])
    result = {"read_only": True, "source_sha": source_sha, "observed_at": datetime.datetime.now(datetime.timezone.utc).isoformat(), "scope": scope, "complete": True, "count_semantics": "metadata estimate; not an emptiness or deletion proof", "databases": {}}
    script_dir = Path(__file__).resolve().parent
    for kind in kinds:
        try:
            result["databases"][kind] = collect(kind, env, docker, script_dir, run_id)
        except InventoryError as error:
            result["complete"] = False
            result["databases"][kind] = {"error": str(error)}
    return result


def main():
    try:
        result = run(dict(os.environ))
    except InventoryError as error:
        result = {"read_only": True, "complete": False, "error": str(error)}
    except Exception:
        result = {"read_only": True, "complete": False, "error": "unexpected_inventory_failure"}
    print("QS_DB_INVENTORY_BEGIN")
    print(json.dumps(result, sort_keys=True, ensure_ascii=True, indent=2))
    print("QS_DB_INVENTORY_END")
    return 0 if result["complete"] else 1


if __name__ == "__main__":
    sys.exit(main())

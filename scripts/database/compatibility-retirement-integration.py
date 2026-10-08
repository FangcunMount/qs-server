#!/usr/bin/env python3
"""Disposable local MySQL 8 / Mongo 7 tests; never accepts a remote endpoint."""
import json
import hashlib
import importlib.util
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import time
import uuid
from unittest import mock


def command(args, *, timeout=60, check=True, env=None):
    result = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                            timeout=timeout, env=env)
    if check and result.returncode:
        raise RuntimeError("local_integration_command_failed")
    return result


def bootstrap_native(repository, names, password, env):
    """Validate Python bootstrap against real Go identity/boundary reports."""
    spec = importlib.util.spec_from_file_location("retirement", repository / "scripts/database/compatibility-retirement.py")
    tool = importlib.util.module_from_spec(spec); spec.loader.exec_module(tool)
    import argparse
    source = "a" * 40
    sql = ["CREATE TABLE schema_migrations(version BIGINT NOT NULL,dirty BOOL NOT NULL)", "INSERT INTO schema_migrations VALUES(95,FALSE)",
           "CREATE TABLE kept_fact(id BIGINT PRIMARY KEY,note TEXT)",
           "CREATE TABLE domain_event_outbox(id BIGINT UNSIGNED PRIMARY KEY,event_type VARCHAR(128),status VARCHAR(32),payload_json LONGTEXT)",
           "CREATE TABLE ai_bridge_commands(command_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,kind VARCHAR(32),delivered BOOL,payload JSON)",
           "CREATE TABLE ai_messaging_legacy_commands(command_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,source_kind VARCHAR(32),source_payload MEDIUMBLOB)",
           "INSERT INTO domain_event_outbox VALUES(1,'evaluation.outcome.committed','published','{}')",
           "INSERT INTO ai_bridge_commands VALUES('00000000-0000-0000-0000-000000000001','start',TRUE,'{}')"]
    def sql_command(statement):
        command(["docker", "exec", names[0], "mysql", "-uroot", "-p"+password, "qs_retirement_inventory_test", "--execute", statement])
    for statement in sql:sql_command(statement)
    def mongo_command(statement):
        command(["docker", "exec", names[1], "mongosh", "--quiet", "--username", "local_inventory_root", "--password", password,
                 "--authenticationDatabase", "admin", "--eval", "db=db.getSiblingDB('qs_retirement_inventory_test');"+statement])
    mongo_command("db.schema_migrations.insertOne({version:NumberLong(36),dirty:false});db.domain_event_outbox.insertOne({_id:ObjectId('000000000000000000000001'),event_type:'answersheet.submitted',status:'published'});")
    with tempfile.TemporaryDirectory(prefix="qs-compat-bootstrap-native-") as temporary:
        base = Path(temporary).resolve(); base.chmod(0o700)
        binary = base / "inventory-tool"
        command(["go", "build", "-ldflags=-X main.sourceSHA="+source, "-o", str(binary), "./cmd/qs-compatibility-retirement"], env=env, timeout=240)
        root = base / "backups/qs-server/compatibility-retirement"; root.mkdir(parents=True, mode=0o700)
        for case, index in (("all_present", 1), ("partially_absent", 2), ("all_absent", 3)):
            if case == "partially_absent":
                sql_command("DROP TABLE ai_bridge_commands"); mongo_command("db.domain_event_outbox.drop();")
            elif case == "all_absent":
                sql_command("DROP TABLE domain_event_outbox,ai_messaging_legacy_commands")
            op = "900-"+str(index); directory = root / op; directory.mkdir(mode=0o700)
            def native(mode, run_id, filename, raw):
                path = directory / filename
                if not path.exists():path.write_bytes(raw); path.chmod(0o600)
                output = directory / (mode+"-"+run_id); output.mkdir(mode=0o700)
                digest = hashlib.sha256(raw).hexdigest()
                command([str(binary), "--mode", mode, "--request", str(path), "--request-hash", digest,
                         "--operation-id", op, "--run-id", run_id, "--output-directory", str(output)], env=env)
                return output
            identity_run = "901-"+str(index)
            identity_dir = native("identity", identity_run, "identity-request.json", tool.identity_request_bytes(op, source))
            identity, identity_hash = tool.read_private(identity_dir, "identity.private.json")
            if identity["complete"] is not True:raise RuntimeError("native_identity_incomplete")
            approval = {"format_version": 1, "kind": "readonly_request_bootstrap_approval", "prepare_mode": "bootstrap-bounds",
                        "operation_id": op, "source_sha": source, "target_hash": tool.TARGET_HASH, "database_scope": "mysql-and-mongodb",
                        "identity_report": {"run_id": identity_run, "source_sha": source, "sha256": identity_hash},
                        "identity_hashes": {db: state["identity_hash"] for db, state in identity["database_states"].items()},
                        "expected_migrations": {db: state["migration_version"] for db, state in identity["database_states"].items()},
                        "limits": tool.INVENTORY_V2_LIMITS.copy()}
            args = argparse.Namespace(operation="prepare", root=str(root), operation_id=op, approved_source_sha=source, actual_source_sha=source,
                                      run_id="902-"+str(index), manifest_hash="", inventory_request_hash="", identity_request_hash="", prepare_mode="bootstrap-bounds")
            def approve():
                args.bootstrap_approval_json = tool.canonical_bytes(approval)[:-1].decode("ascii")
                args.bootstrap_approval_hash = hashlib.sha256(tool.canonical_bytes(approval)).hexdigest()
            approve()
            with mock.patch.object(tool, "live_inventory", side_effect=AssertionError("bootstrap_called_DB")), mock.patch.object(tool, "capture_fixed", side_effect=AssertionError("bootstrap_called_runtime")):
                bounds_receipt = tool.execute(args)
            request, request_hash = tool.read_private(directory, "boundary-request.json", bounds_receipt["derived_request_sha256"])
            bounds_run = "903-"+str(index)
            bounds_dir = native("bounds", bounds_run, "boundary-request.json", tool.canonical_bytes(request))
            bounds, bounds_hash = tool.read_private(bounds_dir, "boundary.private.json")
            if bounds["complete"] is not True:raise RuntimeError("native_boundary_incomplete")
            approval.update(prepare_mode="bootstrap-inventory", boundary_report={"run_id": bounds_run, "source_sha": source, "sha256": bounds_hash})
            args.prepare_mode = "bootstrap-inventory"; args.run_id = "904-"+str(index); approve()
            with mock.patch.object(tool, "live_inventory", side_effect=AssertionError("bootstrap_called_DB")), mock.patch.object(tool, "capture_fixed", side_effect=AssertionError("bootstrap_called_runtime")):
                receipt = tool.execute(args)
            inventory, inventory_hash = tool.read_private(directory, "inventory-request.json", receipt["derived_request_sha256"])
            tool.validate_v2_request(inventory, op, source, boundary=False)
            tool.validate_approved_boundary_file(inventory, directory)
            if inventory["approved_boundaries"] != [target["boundary"] for target in bounds["targets"]] or inventory_hash == args.bootstrap_approval_hash:
                raise RuntimeError("native_bootstrap_binding_mismatch")
            if any(key in json.dumps(receipt) for key in ("upper_token", "approved_boundaries", "MYSQL_PASSWORD")):
                raise RuntimeError("native_bootstrap_private_receipt_leak")
            # Mutating a copied actual report and reapproving its hash must not
            # make a dirty head acceptable. The original source stays intact.
            altered = dict(identity, database_states={db: dict(state) for db, state in identity["database_states"].items()})
            altered["database_states"]["mysql"]["migration_dirty"] = True
            (identity_dir / "identity.private.json").write_bytes(tool.canonical_bytes(altered))
            approval["identity_report"]["sha256"] = hashlib.sha256(tool.canonical_bytes(altered)).hexdigest(); approve()
            try:tool.execute(args)
            except tool.Blocked:pass
            else:raise RuntimeError("native_dirty_report_accepted")
    print(json.dumps({"local_only": True, "production_operations": False, "bootstrap_native_passed": True,
                      "actual_go_reports": ["identity", "bounds"], "presence_cases": ["all_present", "partially_absent", "all_absent"],
                      "present_empty_proven": True, "actual_bson_upper_preserved": True, "bootstrap_database_calls": 0,
                      "separate_approval_and_request_hashes": True, "dirty_report_rejected": True, "drop_ready": False}))
    return 0


def main():
    bootstrap_only = sys.argv[1:] == ["--bootstrap"]
    if sys.argv[1:] and not bootstrap_only:
        print("local_integration_mode_rejected", file=sys.stderr); return 1
    owner = "qs-compat-inventory-test-" + uuid.uuid4().hex[:12]
    names = [owner + "-mysql", owner + "-mongo"]
    synthetic_password = uuid.uuid4().hex
    repository = Path(__file__).resolve().parents[2]
    try:
        for name, image, port, envs in (
            (names[0], "mysql:8.0", "3306", {"MYSQL_ROOT_PASSWORD": synthetic_password,
             "MYSQL_DATABASE": "qs_retirement_inventory_test"}),
            (names[1], "mongo:7", "27017", {"MONGO_INITDB_ROOT_USERNAME": "local_inventory_root",
             "MONGO_INITDB_ROOT_PASSWORD": synthetic_password}),
        ):
            args = ["docker", "run", "--detach", "--pull=never", "--name", name,
                    "--label", "qs.compatibility-retirement.local-test=" + owner,
                    "--publish", "127.0.0.1::" + port, "--memory=1g", "--cpus=1"]
            for key, value in envs.items():
                args += ["--env", key + "=" + value]
            command([*args, image], timeout=30)
        ports = []
        for name, port in zip(names, ("3306", "27017")):
            output = command(["docker", "port", name, port + "/tcp"]).stdout.decode().strip()
            if not re.fullmatch(r"127\.0\.0\.1:[0-9]{1,5}", output):
                raise RuntimeError("non_loopback_test_port_rejected")
            ports.append(output.split(":")[-1])
        limit = time.monotonic() + 90
        while True:
            # The image's temporary bootstrap server answers socket pings but
            # cannot serve the host TCP endpoint; require the final TCP server
            # and an authenticated query before starting native inventory.
            mysql = command(["docker", "exec", names[0], "mysql", "--protocol=TCP", "--host=127.0.0.1",
                             "--batch", "--skip-column-names", "-uroot", "-p" + synthetic_password,
                             "--execute", "SELECT 1"], check=False)
            mongo = command(["docker", "exec", names[1], "mongosh", "--quiet", "--host", "127.0.0.1",
                             "--username", "local_inventory_root", "--password", synthetic_password,
                             "--authenticationDatabase", "admin", "--eval", "if(db.runCommand({ping:1}).ok!==1)quit(1)"], check=False)
            if mysql.returncode == mongo.returncode == 0 and mysql.stdout.strip() == b"1":
                break
            if time.monotonic() >= limit:
                raise RuntimeError("local_database_readiness_timeout")
            time.sleep(1)
        env = os.environ.copy()
        env.update({"QS_RETIREMENT_LOCAL_INTEGRATION": "1", "MYSQL_HOST": "127.0.0.1",
                    "QS_RETIREMENT_SCALE_ROWS": "650123",
                    "MYSQL_PORT": ports[0], "MYSQL_USERNAME": "root", "MYSQL_PASSWORD": synthetic_password,
                    "MYSQL_DATABASE": "qs_retirement_inventory_test", "MONGODB_HOST": "127.0.0.1",
                    "MONGODB_PORT": ports[1], "MONGODB_USERNAME": "local_inventory_root",
                    "MONGODB_PASSWORD": synthetic_password, "MONGODB_DBNAME": "qs_retirement_inventory_test"})
        if bootstrap_only:
            return bootstrap_native(repository, names, synthetic_password, env)
        with tempfile.TemporaryDirectory(prefix="qs-compat-integration-cache-") as cache:
            env["GOCACHE"] = os.environ.get("GOCACHE", cache)
            result = command(["go", "test", "-tags=integration", "-count=1", "./cmd/qs-compatibility-retirement"],
                             timeout=900, check=False, env=env)
        if result.returncode:
            # The fixed test's output contains synthetic evidence only. Redact
            # its random local password before reporting any debugging text.
            output = (result.stdout + result.stderr).decode(errors="replace").replace(synthetic_password, "[local-test-secret]")
            sys.stderr.write(output)
            return 1
        print(json.dumps({"local_only": True, "mysql_major": 8, "mongodb_major": 7,
                          "presence_cases": ["all_present", "partially_absent", "all_absent"],
                          "production_profile": {"rows_per_sql_and_mongo_target": 650123, "equal_passes": 2,
                                                 "fixed_upper": True, "post_upper_next_cycle": True,
                                                 "temporary_assets_registered": True, "drop_ready": False},
                          "reject_cases": ["dirty_head", "wrong_database_identity", "mysql_view", "mongodb_view", "discovery_dirty_mysql", "discovery_dirty_mongodb", "discovery_absent_database_uuid", "histogram_excess_buckets",
                                           "partial_mysql_metadata_permission", "mixed_bson_types", "unsupported_bson_type",
                                           "page_cap", "row_cap", "byte_cap", "timeout", "overwrite_or_resume"], "passed": True}))
        return 0
    except Exception as error:
        print(type(error).__name__ + ":local_integration_failed", file=sys.stderr)
        return 1
    finally:
        for name in names:
            result = command(["docker", "inspect", name, "--format",
                              '{{index .Config.Labels "qs.compatibility-retirement.local-test"}}'], check=False)
            if result.returncode == 0 and result.stdout.decode().strip() == owner:
                command(["docker", "rm", "--force", "--volumes", name], timeout=30, check=False)


if __name__ == "__main__":
    sys.exit(main())

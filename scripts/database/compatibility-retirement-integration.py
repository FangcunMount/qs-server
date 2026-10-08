#!/usr/bin/env python3
"""Disposable local MySQL 8 / Mongo 7 tests; never accepts a remote endpoint."""
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import time
import uuid


def command(args, *, timeout=60, check=True, env=None):
    result = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                            timeout=timeout, env=env)
    if check and result.returncode:
        raise RuntimeError("local_integration_command_failed")
    return result


def main():
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

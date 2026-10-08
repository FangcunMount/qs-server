#!/usr/bin/env python3
"""Run B migration contracts only in disposable loopback MySQL 8 / Mongo 7."""
import json
import os
from pathlib import Path
import re
import subprocess
import stat
import sys
import tempfile
import time
import uuid

DOCKER_CONTEXT = None


def command(args, *, timeout=30, check=True, env=None, cwd=None):
    if args[0] == "docker" and DOCKER_CONTEXT is not None:
        args = ["docker", "--context", DOCKER_CONTEXT, *args[1:]]
    result = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                            timeout=timeout, env=env, cwd=cwd)
    if check and result.returncode:
        raise RuntimeError("local_b_integration_command_failed")
    return result


def local_docker_context():
    override = os.environ.get("DOCKER_HOST", "")
    if override and not override.startswith("unix:///"):
        raise RuntimeError("remote_docker_override_rejected")
    name = command(["docker", "context", "show"]).stdout.decode().strip()
    contexts = json.loads(command(["docker", "context", "inspect", name]).stdout)
    if len(contexts) != 1 or contexts[0]["Name"] != name:
        raise RuntimeError("docker_context_identity_unknown")
    endpoint = contexts[0].get("Endpoints", {}).get("docker", {}).get("Host", "")
    if not endpoint.startswith("unix:///"):
        raise RuntimeError("nonlocal_docker_context_rejected")
    socket = Path(endpoint[len("unix://"):])
    if not socket.is_absolute() or not stat.S_ISSOCK(socket.stat().st_mode):
        raise RuntimeError("local_docker_socket_unknown")
    return name


def main():
    global DOCKER_CONTEXT
    DOCKER_CONTEXT = local_docker_context()
    owner = "qs-compat-b-native-" + uuid.uuid4().hex[:12]
    names = [owner + "-mysql", owner + "-mongo"]
    password = uuid.uuid4().hex
    repository = Path(__file__).resolve().parents[2]
    key_directory = tempfile.TemporaryDirectory(prefix="qs-compat-b-key-")
    key = Path(key_directory.name) / "key"
    key.write_text(os.urandom(64).hex(), encoding="ascii")
    key.chmod(0o600)
    try:
        command(["docker", "run", "--detach", "--pull=never", "--name", names[0],
                 "--label", "qs.compatibility-retirement-b.local-test=" + owner,
                 "--publish", "127.0.0.1::3306", "--memory=1g", "--cpus=1",
                 "--env", "MYSQL_ROOT_PASSWORD=" + password, "mysql:8.0"])
        command(["docker", "run", "--detach", "--pull=never", "--name", names[1],
                 "--label", "qs.compatibility-retirement-b.local-test=" + owner,
                 "--publish", "127.0.0.1::27017", "--memory=1g", "--cpus=1",
                 "--mount", "type=bind,src=" + str(key) + ",dst=/run/qs-test-key,readonly",
                 "--user", "0", "--entrypoint", "mongod", "mongo:7",
                 "--replSet", "qs-compat-b-local", "--bind_ip_all", "--auth",
                 "--keyFile", "/run/qs-test-key"])
        ports = []
        for name, port in zip(names, ("3306", "27017")):
            output = command(["docker", "port", name, port + "/tcp"]).stdout.decode().strip()
            if not re.fullmatch(r"127\.0\.0\.1:[0-9]{1,5}", output):
                raise RuntimeError("non_loopback_b_test_port_rejected")
            ports.append(output.split(":")[-1])
        limit = time.monotonic() + 90
        initialized = False
        authenticated = False
        while True:
            sql_ready = command(["docker", "exec", names[0], "mysqladmin", "ping", "--silent",
                                 "-uroot", "-p" + password], check=False).returncode == 0
            if not initialized:
                result = command(["docker", "exec", names[1], "mongosh", "--quiet", "--host", "127.0.0.1",
                                  "--eval", "rs.initiate({_id:'qs-compat-b-local',members:[{_id:0,host:'127.0.0.1:27017'}]})"], check=False)
                initialized = result.returncode == 0
            if initialized and not authenticated:
                result = command(["docker", "exec", names[1], "mongosh", "--quiet", "--host", "127.0.0.1",
                                  "--eval", "if(!db.hello().isWritablePrimary)quit(2);"
                                  "db.getSiblingDB('admin').createUser({user:'b_native_root',pwd:'" + password + "',roles:['root']})"], check=False)
                authenticated = result.returncode == 0
            mongo_ready = False
            if authenticated:
                mongo_ready = command(["docker", "exec", names[1], "mongosh", "--quiet", "--host", "127.0.0.1",
                                       "--username", "b_native_root", "--password", password,
                                       "--authenticationDatabase", "admin", "--eval",
                                       "if(!db.hello().isWritablePrimary)quit(2)"], check=False).returncode == 0
            if sql_ready and mongo_ready:
                break
            if time.monotonic() >= limit:
                raise RuntimeError("local_b_database_readiness_timeout")
            time.sleep(1)
        env = os.environ.copy()
        env.update({"QS_COMPAT_B_LOCAL_ONLY": "1", "QS_COMPAT_REQUIRE_DATABASE": "1",
                    "MYSQL_DSN": "root:" + password + "@tcp(127.0.0.1:" + ports[0] + ")/?multiStatements=true&parseTime=true",
                    "QS_SERVER_TEST_MONGO_URI": "mongodb://b_native_root:" + password + "@127.0.0.1:" + ports[1] + "/?authSource=admin&directConnection=true",
                    "QS_SERVER_TEST_MONGO_DB_PREFIX": "qs_compat_b_test"})
        with tempfile.TemporaryDirectory(prefix="qs-compat-b-go-cache-") as cache:
            env.setdefault("GOCACHE", cache)
            result = command(["go", "test", "-json", "-race", "-p=1", "-tags=integration", "-count=1", "-timeout=8m",
                              "-run", "^TestCompatibilityRetirementB", "./internal/pkg/migration", "./internal/apiserver/bootstrap"],
                             timeout=540, check=False, env=env, cwd=repository)
        if result.returncode:
            try:
                events = [json.loads(line) for line in result.stdout.splitlines()]
                failed = {event.get("Test") for event in events if event.get("Action") == "fail"}
                output = "".join(event.get("Output", "") for event in events
                                 if event.get("Action") in ("output", "build-output")
                                 and (event.get("Test") in failed or not event.get("Test")))
            except (ValueError, TypeError):
                output = result.stdout.decode(errors="replace")
            output = (output + result.stderr.decode(errors="replace")).replace(password, "[local-test-secret]")
            sys.stderr.write(output)
            return 1
        passed = set()
        for line in result.stdout.splitlines():
            event = json.loads(line)
            if event.get("Action") == "pass" and event.get("Test"):
                passed.add(event["Test"])
        required = {
            "TestCompatibilityRetirementBInstalledSixteenPresenceCombinations",
            "TestCompatibilityRetirementBPartialForwardAndRestart",
            "TestCompatibilityRetirementBRejectsInvalidInstalledPairWithoutWrites",
            "TestCompatibilityRetirementBStandaloneMigratorCannotCrossTail",
            "TestCompatibilityRetirementBLowLevelForceAndDownCannotRewriteRetirementHeads",
            "TestCompatibilityRetirementBOrderAndMetadataOverridesRefuseBeforeWrites",
            "TestCompatibilityRetirementBLateNamespaceReappearanceRefuses",
            "TestCompatibilityRetirementBPairCannotAuthorizeDifferentConnectionsOrNames",
            "TestCompatibilityRetirementBPristinePairNeedsHashBoundAuthorization",
            "TestCompatibilityRetirementBAuthorizedColdFullHistoryAndRestart",
            "TestCompatibilityRetirementBHeadlessOrMixedPairIsNotPristine",
            "TestCompatibilityRetirementBIncompleteVisibilityRefusesWithoutWrites",
            "TestCompatibilityRetirementBPristineAuthorizationRejectsTamperingWithoutConsumption",
            "TestCompatibilityRetirementBPristineProfilingVisibilityIsReadOnlyAndRequired",
            "TestCompatibilityRetirementBNativeMongoWrapperBoundaries",
            "TestCompatibilityRetirementBNativeMongoUUIDReplacementBeforeDropRefuses",
            "TestCompatibilityRetirementBDisabledRecoveryBootstrapDoesNotWrite",
        }
        required.update("TestCompatibilityRetirementBInstalledSixteenPresenceCombinations/mask_%02d" % number
                        for number in range(16))
        required.update("TestCompatibilityRetirementBDisabledRecoveryBootstrapDoesNotWrite/sql_dirty_%s_mongo_dirty_%s" % (sql_dirty, mongo_dirty)
                        for sql_dirty in ("false", "true") for mongo_dirty in ("false", "true"))
        required.update("TestCompatibilityRetirementBLowLevelForceAndDownCannotRewriteRetirementHeads/%s_%s_dirty_%s_proof_%s" % (backend, action, dirty, proof)
                        for backend in ("mysql", "mongodb") for action in ("force_low", "force_current", "force_unknown", "down")
                        for dirty in ("false", "true") for proof in ("false", "true"))
        required.update("TestCompatibilityRetirementBPristineProfilingVisibilityIsReadOnlyAndRequired/" + scenario
                        for scenario in ("fresh_read_only", "enabled_profile", "disabled_profile_with_filter", "catalog_visibility_without_profile_permission"))
        required.update("TestCompatibilityRetirementBPristineAuthorizationRejectsTamperingWithoutConsumption/" + scenario
                        for scenario in ("wrong_approved_source", "expected_source_empty", "expected_source_unknown", "expected_source_invalid", "expected_source_mismatch"))
        if not required.issubset(passed):
            raise RuntimeError("local_b_native_case_set_incomplete")
        print(json.dumps({"local_only": True, "docker_local_unix_socket": True, "mysql_major": 8, "mongodb_major": 7,
                          "mongodb_replica_set": True, "presence_combinations": 16,
                          "race": True,
                          "disabled_recovery_head_combinations": 4,
                          "low_level_force_down_refusals": 32,
                          "pristine_profiling_cases": 4,
                          "source_identity_refusals": 5,
                          "passed_cases": len(passed),
                          "passed": True}))
        return 0
    except Exception as error:
        print(type(error).__name__ + ":local_b_integration_failed", file=sys.stderr)
        return 1
    finally:
        for name in names:
            result = command(["docker", "inspect", name, "--format",
                              '{{index .Config.Labels "qs.compatibility-retirement-b.local-test"}}'], check=False)
            if result.returncode == 0 and result.stdout.decode().strip() == owner:
                command(["docker", "rm", "--force", "--volumes", name], check=False)
        key_directory.cleanup()


if __name__ == "__main__":
    sys.exit(main())

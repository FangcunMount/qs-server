import copy
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

import m5_maintenance_container as m


def source():
    return dict(Id="a" * 64, Name="/qs-apiserver", Image="sha256:" + "b" * 64,
                Config=dict(Env=["QS_APISERVER_MYSQL_DATABASE=qs",
                                 "QS_APISERVER_MYSQL_PASSWORD=owned=canary#value",
                                 "QS_APISERVER_MONGODB_DATABASE=qs", "JWT_SECRET=not-copied"]),
                NetworkSettings=dict(Networks={"infra-network": {}}),
                HostConfig={}, Mounts=[], State=dict(Running=False))


class MaintenanceTransportTest(unittest.TestCase):
    def test_default_preflight_never_creates_container_and_private_file_is_removed(self):
        calls = []
        files = []

        def invoke(args, **kwargs):
            calls.append(args)
            if args[:2] == ["docker", "inspect"]:
                if args[2] == m.SOURCE:
                    return subprocess.CompletedProcess(args, 0, json.dumps([source()]).encode())
                return subprocess.CompletedProcess(args, 1, b"")
            self.assertEqual(args[:4], ["sudo", "-n", "-l", "--"])
            if "--env-file" in args:
                path = Path(args[args.index("--env-file") + 1])
                files.append(path)
                self.assertEqual(path.stat().st_mode & 0o777, 0o600)
                text = path.read_text()
                self.assertIn("owned=canary#value", text)
                self.assertNotIn("JWT_SECRET", text)
            return subprocess.CompletedProcess(args, 0, b"")

        with patch.object(m.subprocess, "run", side_effect=invoke):
            receipt = m.prepare(source()["Image"], "123-1", docker_prefix=["docker"])
        self.assertFalse(receipt["container_created"])
        self.assertFalse(receipt["database_queried"])
        self.assertTrue(receipt["read_only_preflight"])
        self.assertTrue(files and all(not p.exists() for p in files))
        self.assertFalse(any(a[:2] in [["docker", "create"], ["docker", "start"]] for a in calls))

    def test_password_mode_is_private_stdin_and_permissions_are_list_only(self):
        secret = "private-sudo-canary"
        calls = []

        def invoke(args, **kwargs):
            calls.append(args)
            self.assertNotIn(secret, " ".join(args))
            self.assertEqual(kwargs.get("input"), (secret + "\n").encode())
            self.assertEqual(args[:4], ["sudo", "-S", "-p", ""])
            if "inspect" in args:
                if args[-1] == m.SOURCE:
                    return subprocess.CompletedProcess(args, 0, json.dumps([source()]).encode())
                return subprocess.CompletedProcess(args, 1, b"[]")
            self.assertEqual(args[4:6], ["-l", "--"])
            return subprocess.CompletedProcess(args, 0, b"")

        with patch.dict(os.environ, {"SUDO_PASSWORD": secret}), patch.object(m.subprocess, "run", side_effect=invoke):
            receipt = m.prepare(source()["Image"], "123-1")
        self.assertFalse(receipt["container_created"])
        self.assertFalse(receipt["database_queried"])
        self.assertNotIn(secret, json.dumps(receipt))
        for bad in ("secret\nsecond", "secret\r", "secret\0", "x" * 4097):
            with patch.object(m.os, "environ", {"SUDO_PASSWORD": bad}), patch.object(m.subprocess, "run") as run:
                with self.assertRaisesRegex(m.Refused, "sudo_credential_invalid"):
                    m.prepare(source()["Image"], "123-1")
                run.assert_not_called()

    def test_stopped_source_fixed_entrypoint_and_private_env_only(self):
        s = source()
        name, network, env = m.source_configuration(s, s["Image"], "123-1")
        self.assertEqual(name, "qs-m5-maintenance-123-1")
        self.assertEqual(network, "infra-network")
        self.assertNotIn("JWT_SECRET", env)
        args = m.create_arguments(s, s["Image"], "123-1", "/private/env")
        self.assertEqual(args[-3:], [s["Image"], "-c", m.SLEEP])
        self.assertEqual(args[args.index("--entrypoint") + 1], "/bin/sh")
        self.assertIn("--no-healthcheck", args)
        self.assertEqual(args[args.index("--volumes-from") + 1], s["Id"] + ":ro")
        self.assertFalse(any("canary" in x or "JWT_SECRET" in x for x in args))

    def test_foreign_scope_ambiguous_network_or_lost_env_bytes_refused(self):
        mutations = [lambda s: s.update(Name="/iam-apiserver"),
                     lambda s: s.update(Image="sha256:" + "c" * 64),
                     lambda s: s["NetworkSettings"]["Networks"].update(other={}),
                     lambda s: s["Mounts"].append(dict(Destination="/tmp")),
                     lambda s: s["Config"]["Env"].append("QS_APISERVER_MYSQL_PASSWORD=duplicate"),
                     lambda s: s["Config"]["Env"].append("QS_APISERVER_MYSQL_HOST=bad\nvalue")]
        for mutate in mutations:
            s = copy.deepcopy(source())
            mutate(s)
            with self.subTest(change=mutate), self.assertRaises(ValueError):
                m.source_configuration(s, source()["Image"], "123-1")
        with self.assertRaises(ValueError):
            m.source_configuration(source(), source()["Image"], "../other")

    def test_inventory_override_requires_exact_idle_owned_image(self):
        name = "qs-m5-maintenance-123-1"
        actual = dict(Name="/" + name, Image=source()["Image"], State=dict(Running=True),
                      Config=dict(User="2000:2000", Entrypoint=["/bin/sh"], Cmd=["-c", m.SLEEP], Healthcheck=dict(Test=["NONE"]),
                                  Labels={m.LABEL: "123-1"}), Mounts=[])
        env = dict(QS_MAINTENANCE_CONTAINER=name, QS_MAINTENANCE_IMAGE_ID=source()["Image"],
                   RUNNER_SSH_CONFIG="/private/config", RUNNER_SSH_ALIAS="serverA")
        with patch.dict(os.environ, env, clear=True):
            with patch.object(m.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, json.dumps([actual]).encode())):
                self.assertEqual(m.resolve_inventory_target("qs.mysql", "qs-apiserver"), name)
                self.assertEqual(m.resolve_inventory_target("iam.mysql", "iam-apiserver"), "iam-apiserver")
                for change in (dict(User="0"), dict(Entrypoint=["/app/qs-apiserver"]),
                               dict(Labels={m.LABEL: "456-1"}), dict(Healthcheck=dict(Test=["CMD", "application"]))):
                    bad = copy.deepcopy(actual)
                    bad["Config"].update(change)
                    with patch.object(m.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, json.dumps([bad]).encode())):
                        with self.assertRaises(ValueError):
                            m.resolve_inventory_target("qs.mysql", "qs-apiserver")


def native():
    """Owned Docker lifecycle/mount/env proof; no database or application runs."""
    def docker(*args, allow_failure=False):
        result = subprocess.run(["docker", *args], stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                timeout=120, check=False)
        if result.returncode and not allow_failure:
            raise RuntimeError("owned Docker smoke failed")
        if result.returncode:
            return ""
        return result.stdout.decode().strip()

    operation = os.environ.get("GITHUB_RUN_ID", str(os.getpid())) + "-" + os.environ.get("GITHUB_RUN_ATTEMPT", "1")
    if not m.OPERATION.fullmatch(operation):
        raise RuntimeError("owned native operation required")
    # The fixture gets its own source namespace; the production CLI still has
    # no source override and remains fixed to qs-apiserver.
    m.SOURCE = "qs-m5-native-api-" + operation
    if docker("inspect", m.SOURCE, allow_failure=True):
        raise RuntimeError("source name already occupied")
    name = "qs-m5-maintenance-" + operation
    if docker("inspect", name, allow_failure=True):
        raise RuntimeError("maintenance name already occupied")
    network = "qs-m5-native-" + operation
    tag = "qs-m5-owned-image:" + operation
    made_source = made_network = made_image = False
    source_id = ""
    with tempfile.TemporaryDirectory(prefix="qs-m5-native-") as private:
        directory = Path(private)
        directory.chmod(0o755)
        marker = directory / "marker"
        marker.mkdir(mode=0o777)
        marker.chmod(0o777)
        try:
            docker("pull", "alpine:3.22.1")
            base = json.loads(docker("image", "inspect", "alpine:3.22.1"))[0]["RepoDigests"][0]
            (directory / "Dockerfile").write_text("FROM " + base + '\nENTRYPOINT ["/bin/sh","-c","touch /marker/application-started; exec sleep 2147483647"]\nHEALTHCHECK --interval=1s CMD touch /marker/image-health-command\n')
            docker("build", "--network=none", "-t", tag, str(directory))
            made_image = True
            image = json.loads(docker("image", "inspect", tag))[0]["Id"]
            docker("network", "create", network)
            made_network = True
            source_id = docker("create", "--name", m.SOURCE, "--network", network,
                   "-v", str(marker) + ":/marker",
                   "-e", "QS_APISERVER_MYSQL_DATABASE=qs",
                   "-e", "QS_APISERVER_MYSQL_PASSWORD=owned=canary#value",
                   "-e", "QS_APISERVER_MONGODB_DATABASE=qs",
                   "-e", "JWT_SECRET=must-not-copy", "--entrypoint", "/bin/sh", image, "-c", m.SLEEP)
            made_source = True
            docker("start", source_id)
            docker("stop", "--time", "1", source_id)
            assert json.loads(docker("inspect", source_id))[0]["State"]["Status"] == "exited"
            receipt = m.prepare(image, operation, stage=True, docker_prefix=["docker"])
            assert not receipt["source_was_running"] and receipt["container_created"]
            assert not receipt["application_started"] and not receipt["database_queried"]
            actual = json.loads(docker("inspect", name))[0]
            assert actual["Path"] == "/bin/sh" and actual["Args"] == ["-c", m.SLEEP]
            assert actual["Config"]["Healthcheck"]["Test"] == ["NONE"]
            env = docker("exec", name, "sh", "-c", 'printf "%s" "$QS_APISERVER_MYSQL_PASSWORD"')
            assert env == "owned=canary#value"
            assert docker("exec", name, "sh", "-c", 'printf "%s" "${JWT_SECRET:-}"') == ""
            assert docker("exec", name, "id", "-u") == "2000"
            assert not (marker / "application-started").exists()
            assert all(not mount["RW"] for mount in actual["Mounts"])
            denied = subprocess.run(["docker", "exec", name, "sh", "-c", "touch /marker/forbidden"],
                                    stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=10)
            assert denied.returncode != 0 and not (marker / "forbidden").exists()
            docker("exec", name, "sh", "-c", "printf owned-marker > /tmp/original-consumed-marker")
            try:
                m.prepare(image, operation, stage=True, docker_prefix=["docker"])
            except ValueError:
                pass
            else:
                raise AssertionError("existing operation was reused")
            assert docker("exec", name, "cat", "/tmp/original-consumed-marker") == "owned-marker"
            result = dict(fixture_only=True, source_sha=os.environ.get("GITHUB_SHA", "unknown"),
                          base_image=base, source_image_id=image, stopped_source_supported=True,
                          application_not_started=True, private_env_preserved=True,
                          image_healthcheck_disabled=True, original_private_marker_preserved=True,
                          unrelated_secret_not_copied=True, borrowed_mounts_read_only=True,
                          existing_operation_refused=True, no_database_connections=True)
        finally:
            if made_source:
                raw = docker("inspect", name, allow_failure=True)
                if raw:
                    owned = json.loads(raw)[0]
                    labels = owned["Config"].get("Labels", {})
                    assert labels.get(m.LABEL) == operation
                    assert labels.get("com.fangcunmount.qs-m5.source") == source_id
                    docker("rm", "-f", owned["Id"])
                docker("rm", "-f", source_id)
            if made_network:
                docker("network", "rm", network)
            if made_image:
                docker("image", "rm", tag)
        assert docker("inspect", name, allow_failure=True) == ""
        assert docker("inspect", m.SOURCE, allow_failure=True) == ""
        result["owned_containers_removed"] = True
        Path("m5-maintenance-transport-native.json").write_text(json.dumps(result, indent=2) + "\n")
        print("PASS owned stopped-source maintenance transport; no DB or application runtime")


if __name__ == "__main__":
    if sys.argv[1:] == ["--native"]:
        native()
    else:
        unittest.main()

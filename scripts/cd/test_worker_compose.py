"""Required CI: real worker package and Compose model, without starting containers."""

import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]


class WorkerCompose(unittest.TestCase):
    def test_worker_keeps_original_per_instance_budget_and_has_no_publisher(self):
        with tempfile.TemporaryDirectory(prefix="qs-m4-worker-") as temporary:
            root = Path(temporary)
            package = root / "package"
            environment = {
                **os.environ, "SERVICE": "worker", "DEPLOY_PACKAGE_DIR": str(package),
                "DEPLOY_PACKAGE": str(root / "package.tar.gz"), "MESSAGING_PROVIDER": "rocketmq",
                "ROCKETMQ_ENDPOINT": "broker.invalid:8081", "ROCKETMQ_PUBLISH_ENABLED": "false",
                "ROCKETMQ_WORKER_BINDING_REVISION": "m4-worker-fixture",
                "MYSQL_TLS_MODE": "VERIFY_IDENTITY", "MYSQL_HOST": "mysql.invalid",
                "MYSQL_USERNAME": "synthetic", "MYSQL_PASSWORD": "synthetic", "MYSQL_DATABASE": "qs",
                "MONGODB_HOST": "mongo.invalid", "MONGODB_USERNAME": "synthetic",
                "MONGODB_PASSWORD": "synthetic", "MONGODB_DBNAME": "qs",
                "REDIS_HOST": "redis.invalid", "REDIS_PORT": "6379",
                "DOCKER_IMAGE_QS_WORKER": "qs-worker:" + "a" * 40,
            }
            subprocess.run(["bash", "scripts/cd/prepare-package.sh"], cwd=ROOT, env=environment,
                           check=True, capture_output=True, text=True)
            env_file = package / "configs/env/config.prod.env"
            # Relocate only Compose's host env-file references into this disposable
            # directory. Keep the actual packaged topology and all service options.
            text = (package / "docker-compose.prod.yml").read_text()
            empty = root / "other-role.env"
            empty.write_text("")
            for role in ("qs-apiserver", "qs-collection-server", "qs-worker"):
                source = f"/opt/qs-server/{role}/configs/env/config.prod.env"
                self.assertEqual(text.count(source), 1)
                text = text.replace(source, str(env_file if role == "qs-worker" else empty))
            compose = root / "compose.yml"
            compose.write_text(text)
            result = subprocess.run(["docker", "compose", "--env-file", str(env_file), "-f",
                                     str(compose), "config", "--format", "json"], env=environment,
                                    check=True, capture_output=True, text=True)
            services = json.loads(result.stdout)["services"]
            self.assertEqual(set(services), {"qs-apiserver", "server", "runtime"})
            worker = services["runtime"]
            self.assertEqual(int(worker["mem_limit"]), 768 * 1024 * 1024)
            self.assertEqual(float(worker["cpus"]), 1)
            self.assertEqual(worker["stop_grace_period"], "8m30s")
            self.assertEqual(worker["environment"]["QS_WORKER_MESSAGING_PROVIDER"], "rocketmq")
            self.assertEqual(worker["environment"]["QS_WORKER_MESSAGING_ROCKETMQ_PUBLISH_ENABLED"], "false")
            self.assertFalse(any("NSQ_" in k for k in worker["environment"]))
            ca_path = worker["environment"]["QS_WORKER_MYSQL_TLS_CA_FILE"]
            mysql_ca = [m for m in worker["volumes"] if m["target"] == ca_path]
            self.assertEqual(len(mysql_ca), 1)
            self.assertEqual(mysql_ca[0]["source"], "/data/infra/ssl/mysql/ca-chain.pem")
            self.assertTrue(mysql_ca[0]["read_only"])
            self.assertFalse(mysql_ca[0]["bind"].get("create_host_path", False))
            mounts = [m for m in worker["volumes"] if m["target"].startswith("/etc/qs-worker/rocketmq/")]
            self.assertEqual({m["target"].rsplit("/", 1)[1] for m in mounts},
                             {"credentials.json", "ca.crt", "routes.json"})
            for mount in mounts:
                self.assertTrue(mount["read_only"])
                self.assertFalse(mount["bind"].get("create_host_path", False))
                self.assertEqual(mount["source"], "/var/lib/fangcun-m4-materials/qs-worker-messaging/versions/m4-worker-fixture/rocketmq/" + mount["target"].rsplit("/", 1)[1])
            self.assertEqual(int(services["qs-apiserver"]["mem_limit"]), 1024 * 1024 * 1024)
            self.assertEqual(float(services["qs-apiserver"]["cpus"]), 2)
            self.assertEqual(int(services["server"]["mem_limit"]), 320 * 1024 * 1024)


if __name__ == "__main__":
    unittest.main()

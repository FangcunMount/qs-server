#!/usr/bin/env python3
"""Owned loopback-only Docker fixtures; refuses remote endpoints and existing roles."""
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import uuid

PATH = Path(__file__).with_name("runtime-evidence.py")
SPEC = importlib.util.spec_from_file_location("runtime_evidence_native", PATH)
tool = importlib.util.module_from_spec(SPEC); SPEC.loader.exec_module(tool)
SOURCE = "a"*40


def main():
    if sys.argv[1:]:print("local_runtime_test_input_rejected", file=sys.stderr); return 1
    endpoint = os.environ.get("DOCKER_HOST", "")
    if endpoint and not endpoint.startswith("unix://"):
        print("local_runtime_test_endpoint_rejected", file=sys.stderr); return 1
    owner = "qs-runtime-proof-"+uuid.uuid4().hex[:12]
    names = []; image = owner+":"+SOURCE
    label = "qs.cd.runtime-evidence.test-owner"
    with tempfile.TemporaryDirectory(prefix="qs-runtime-evidence-native-") as temporary:
        private = Path(temporary); private.chmod(0o700)
        log = private/"native.private.log"
        log.touch(mode=0o600)
        def run(args, timeout=60, check=True, env=None):
            result = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=timeout, env=env)
            with log.open("ab") as stream:stream.write(result.stdout); stream.write(result.stderr)
            if check and result.returncode:raise RuntimeError("local_runtime_command_failed")
            return result
        try:
            if any(row["project"] == "qs-worker" or row["name"] == "qs-worker" for row in tool.Docker().listing()):
                raise RuntimeError("existing_local_role_refused")
            architecture = run(["docker", "info", "--format", "{{.Architecture}} {{.Name}}"], timeout=15).stdout.decode().strip().split()
            if len(architecture) != 2 or architecture[1] != "docker-desktop" or architecture[0] not in ("aarch64", "arm64", "x86_64", "amd64"):
                raise RuntimeError("non_local_docker_daemon_refused")
            arch = "arm64" if architecture[0] in ("aarch64", "arm64") else "amd64"
            # Use the real repository version formatter; fixture HTTP is local
            # and never initializes business, persistence or a broker.
            (private/"main.go").write_text('package main\nimport ("fmt";"os";"net/http";"github.com/FangcunMount/qs-server/pkg/version")\nfunc main(){if len(os.Args)==3&&os.Args[1]=="--version=true"&&os.Args[2]=="--config=/app/configs/worker.prod.yaml" {fmt.Printf("%s\\n",version.Get());return};http.HandleFunc("/readyz",func(w http.ResponseWriter,r *http.Request){w.WriteHeader(200)});if http.ListenAndServe("127.0.0.1:9092",nil)!=nil {os.Exit(1)}}\n')
            env = os.environ.copy(); env.update({"GOOS":"linux", "GOARCH":arch, "CGO_ENABLED":"0"})
            flags = "-X github.com/FangcunMount/qs-server/pkg/version.GitVersion=v1.0.0 -X github.com/FangcunMount/qs-server/pkg/version.GitCommit="+SOURCE+" -X github.com/FangcunMount/qs-server/pkg/version.GitTreeState=clean -X github.com/FangcunMount/qs-server/pkg/version.BuildDate=2026-10-08T12:00:00Z"
            run(["go", "build", "-ldflags", flags, "-o", str(private/"qs-worker"), str(private/"main.go")], timeout=180, env=env)
            (private/"Dockerfile").write_text('FROM alpine:3.21\nCOPY qs-worker /app/qs-worker\nENTRYPOINT ["/app/qs-worker"]\n')
            run(["docker", "build", "--pull=false", "--label", label+"="+owner, "-t", image, str(private)], timeout=120)
            for index in range(1, 4):
                name = owner+"-"+str(index); names.append(name)
                run(["docker", "run", "--detach", "--pull=never", "--network", "none", "--memory", "128m", "--cpus", "0.5", "--name", name,
                     "--label", label+"="+owner, "--label", "com.docker.compose.project=qs-worker", "--label", "com.docker.compose.service=runtime",
                     "--label", "com.docker.compose.container-number="+str(index), "--label", "com.docker.compose.oneoff=False", image])
            deadline=time.monotonic()+30
            while True:
                result=run(["docker", "exec", names[0], "wget", "-q", "-O", "/dev/null", "http://127.0.0.1:9092/readyz"], check=False, timeout=3)
                if result.returncode == 0:break
                if time.monotonic()>deadline:raise RuntimeError("native_readiness_timeout")
                time.sleep(0.2)
            receipt=tool.collect(tool.Docker(), "worker", 3, SOURCE, "99001", "1", SOURCE)
            if not receipt["complete"] or len({i["container_id"] for i in receipt["instances"]}) != 3 or receipt["business_acceptance_verified"]:
                raise RuntimeError("native_receipt_invalid")
            # Existing stopped replicas must be counted, rather than hidden by
            # --status running. No production/deployed container is touched.
            extra=owner+"-stopped"; names.append(extra)
            run(["docker", "create", "--network", "none", "--name", extra, "--label", label+"="+owner,
                 "--label", "com.docker.compose.project=qs-worker", "--label", "com.docker.compose.service=runtime", image])
            try:tool.collect(tool.Docker(), "worker", 3, SOURCE, "99001", "1", SOURCE)
            except tool.Refused as error:
                if str(error)!="instance_set_invalid":raise
            else:raise RuntimeError("native_stopped_extra_accepted")
            run(["docker", "rm", extra]); names.remove(extra)
            # Real writable-layer modification is detected before running the
            # changed version command. PID1 continues executing the old ELF.
            run(["docker", "exec", "--user", "0", names[0], "sh", "-c", "cp /bin/busybox /app/replacement; mv /app/replacement /app/qs-worker"])
            try:tool.collect(tool.Docker(), "worker", 3, SOURCE, "99001", "1", SOURCE)
            except tool.Refused as error:
                if str(error)!="binary_modified":raise
            else:raise RuntimeError("native_changed_binary_accepted")
            print(json.dumps({"local_only":True,"production_operations":False,"native_instances":3,"actual_docker_projection":True,
                              "real_repository_version_formatter":True,"proc_executable_sha_verified":True,"image_config_digest_verified":True,
                              "loopback_readiness_verified":True,"stopped_extra_rejected":True,"modified_binary_rejected":True,
                              "business_acceptance_verified":False,"private_native_log_sha256":hashlib.sha256(log.read_bytes()).hexdigest(),"passed":True}))
            return 0
        except Exception as error:
            category=str(error) if type(error) is tool.Refused else "local_runtime_native_failed"
            print("runtime_evidence_native_refused:"+category,file=sys.stderr)
            return 1
        finally:
            for name in names:
                result=run(["docker", "inspect", "--format", '{{index .Config.Labels "'+label+'"}}', name], check=False)
                if result.returncode==0 and result.stdout.decode().strip()==owner:
                    run(["docker", "rm", "--force", name], check=False)
            result=run(["docker", "image", "inspect", "--format", '{{index .Config.Labels "'+label+'"}}', image], check=False)
            if result.returncode==0 and result.stdout.decode().strip()==owner:
                run(["docker", "image", "rm", image], check=False)


if __name__ == "__main__":sys.exit(main())

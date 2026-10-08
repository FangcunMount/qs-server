#!/usr/bin/env python3
"""Owned loopback-only Docker fixtures; refuses remote endpoints and existing roles."""
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import stat
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
    containers = {}; images = {}; result_payload = None; failure = None
    label = "qs.cd.runtime-evidence.test-owner"
    with tempfile.TemporaryDirectory(prefix="qs-runtime-evidence-native-") as temporary:
        private = Path(temporary); private.chmod(0o700)
        log = private/"native.private.log"; log.touch(mode=0o600)
        def run(args, timeout=60, check=True, env=None):
            result = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=timeout, env=env)
            with log.open("ab") as stream:stream.write(result.stdout); stream.write(result.stderr)
            if check and result.returncode:raise RuntimeError("local_runtime_command_failed")
            return result
        def image_id(reference):
            raw=run(["docker", "image", "inspect", "--format", '{{json .Id}}|{{json (index .Config.Labels "'+label+'")}}', reference]).stdout.decode().strip().split("|")
            cid, observed_owner = [json.loads(item) for item in raw]
            if not tool.IMAGE.fullmatch(cid) or observed_owner != owner:raise RuntimeError("native_image_owner_unproven")
            return cid
        def owned_container(cid):
            if cid not in containers or not tool.HASH.fullmatch(cid):return False
            raw=run(["docker", "inspect", "--type", "container", "--format", '{{json .Id}}|{{json .Image}}|{{json (index .Config.Labels "'+label+'")}}|{{json .HostConfig.NetworkMode}}|{{json .HostConfig.PortBindings}}|{{json .Mounts}}', cid], check=False)
            if raw.returncode:return False
            actual, image, observed_owner, network, ports, mounts = [json.loads(item) for item in raw.stdout.decode().strip().split("|")]
            return actual == cid and image == containers[cid] and observed_owner == owner and network == "none" and ports in (None,{}) and mounts == []
        def remove_container(cid):
            if not owned_container(cid):raise RuntimeError("native_container_owner_unproven")
            run(["docker", "rm", "--force", cid]); del containers[cid]
        def create_container(name, role, reference, *, stopped=False):
            arguments=["docker", "create" if stopped else "run"]
            if not stopped:arguments += ["--detach", "--pull=never", "--memory", "128m", "--cpus", "0.5"]
            arguments += ["--network", "none", "--name", name, "--label", label+"="+owner,
                          "--label", "com.docker.compose.project="+tool.ROLES[role][0],
                          "--label", "com.docker.compose.service="+tool.ROLES[role][1],
                          "--label", "com.docker.compose.container-number="+str(len(containers)+1),
                          "--label", "com.docker.compose.oneoff=False", reference]
            cid=run(arguments).stdout.decode().strip()
            if not tool.HASH.fullmatch(cid):raise RuntimeError("native_created_id_unproven")
            containers[cid]=images[reference]
            if not owned_container(cid):raise RuntimeError("native_created_owner_unproven")
            return cid
        def wait_ready(cid, role):
            _, _, _, port, path=tool.ROLES[role]
            deadline=time.monotonic()+30
            while True:
                result=run(["docker", "exec", cid, "wget", "-q", "-O", "/dev/null", "http://127.0.0.1:"+str(port)+path], check=False, timeout=3)
                if result.returncode == 0:return
                if time.monotonic()>deadline:raise RuntimeError("native_readiness_timeout")
                time.sleep(0.2)
        def refused(category, callback):
            try:callback()
            except tool.Refused as error:
                if str(error) != category:raise
            else:raise RuntimeError("native_required_refusal_missing")
        try:
            # A named Docker context can override DOCKER_HOST. Resolve its
            # actual endpoint before any daemon request, then pin this child
            # process to that Unix socket for every subsequent command.
            context=run(["docker","context","show"],timeout=15).stdout.decode().strip()
            local_endpoint=json.loads(run(["docker","context","inspect","--format","{{json .Endpoints.docker.Host}}",context],timeout=15).stdout.decode())
            if type(local_endpoint) is not str or not local_endpoint.startswith("unix://"):
                raise RuntimeError("non_local_docker_endpoint_refused")
            os.environ["DOCKER_HOST"]=local_endpoint if os.environ.get("DOCKER_CONTEXT") else endpoint or local_endpoint
            os.environ.pop("DOCKER_CONTEXT",None)
            # Read all role containers before any fixture mutation, including
            # stopped/legacy roles. Never replace a pre-existing API name.
            for row in tool.Docker().listing():
                if row["project"] in {spec[0] for spec in tool.ROLES.values()} or row["name"] in {"qs-apiserver","qs-worker","qs-collection-server"} or row["service"] == "qs-apiserver" or row["name"].startswith(("qs-deploy-worker-","qs-deploy-collection-")):
                    raise RuntimeError("existing_local_role_refused")
            architecture = run(["docker", "info", "--format", "{{.Architecture}} {{.Name}}"], timeout=15).stdout.decode().strip().split()
            if len(architecture) != 2 or architecture[1] != "docker-desktop" or architecture[0] not in ("aarch64", "arm64", "x86_64", "amd64"):
                raise RuntimeError("non_local_docker_daemon_refused")
            arch = "arm64" if architecture[0] in ("aarch64", "arm64") else "amd64"
            # Actual repository formatter with an explicitly synthetic fixture
            # build SHA; no business/DB/broker initialization in these programs.
            (private/"main.go").write_text('package main\nimport ("fmt";"os";"net/http";"path/filepath";"github.com/FangcunMount/qs-server/pkg/version")\nfunc main(){role:=filepath.Base(os.Args[0]);config:="/app/configs/worker.prod.yaml";port:="9092";if role=="qs-apiserver"{config="/app/configs/apiserver.prod.yaml";port="8080"};if len(os.Args)==3&&os.Args[1]=="--version=true"&&os.Args[2]=="--config="+config {fmt.Printf("%s\\n",version.Get());return};http.HandleFunc("/readyz",func(w http.ResponseWriter,r *http.Request){w.WriteHeader(200)});if http.ListenAndServe("127.0.0.1:"+port,nil)!=nil {os.Exit(1)}}\n')
            env = os.environ.copy(); env.update({"GOOS":"linux", "GOARCH":arch, "CGO_ENABLED":"0"})
            flags = "-X github.com/FangcunMount/qs-server/pkg/version.GitVersion=v1.0.0 -X github.com/FangcunMount/qs-server/pkg/version.GitCommit="+SOURCE+" -X github.com/FangcunMount/qs-server/pkg/version.GitTreeState=clean -X github.com/FangcunMount/qs-server/pkg/version.BuildDate=2026-10-08T12:00:00Z"
            run(["go", "build", "-ldflags", flags, "-o", str(private/"fixture"), str(private/"main.go")], timeout=180, env=env)
            worker_image=owner+"-worker:"+SOURCE
            (private/"Dockerfile").write_text('FROM alpine:3.21\nCOPY fixture /app/qs-worker\nENTRYPOINT ["/app/qs-worker"]\n')
            run(["docker", "build", "--pull=false", "--label", label+"="+owner, "-t", worker_image, str(private)], timeout=120)
            images[worker_image]=image_id(worker_image)
            workers=[create_container(owner+"-"+str(index),"worker",worker_image) for index in range(1,4)]
            for cid in workers:wait_ready(cid,"worker")
            receipt=tool.collect(tool.Docker(),"worker",3,SOURCE,"99001","1",SOURCE)
            bindings=dict(role="worker",expected=3,source=SOURCE,run="99001",attempt="1",tool_sha256=hashlib.sha256(PATH.read_bytes()).hexdigest(),expected_container_ids=sorted(workers))
            tool.receive_armored_receipt(tool.transport().encode_armored_receipt(receipt,schema=tool.SCHEMA),**bindings)
            extra=create_container(owner+"-stopped","worker",worker_image,stopped=True)
            refused("instance_set_invalid",lambda:tool.collect(tool.Docker(),"worker",3,SOURCE,"99001","1",SOURCE))
            remove_container(extra)
            run(["docker","exec","--user","0",workers[0],"sh","-c","cp /bin/busybox /app/replacement; mv /app/replacement /app/qs-worker"])
            refused("binary_modified",lambda:tool.collect(tool.Docker(),"worker",3,SOURCE,"99001","1",SOURCE))

            api_image=owner+"-api:"+SOURCE
            (private/"Dockerfile").write_text('FROM alpine:3.21\nCOPY fixture /app/qs-apiserver\nENTRYPOINT ["/app/qs-apiserver"]\n')
            run(["docker","build","--pull=false","--label",label+"="+owner,"-t",api_image,str(private)],timeout=120)
            config_id=image_id(api_image); images[api_image]=config_id; images[config_id]=config_id
            api=create_container("qs-apiserver","apiserver",config_id)
            wait_ready(api,"apiserver")
            actual=tool.Docker().inspect(api)
            if actual["image_id"] != config_id or actual["image_reference"] != config_id:raise RuntimeError("native_exact_config_id_not_observed")
            api_receipt=tool.collect(tool.Docker(),"apiserver",1,SOURCE,"99002","1",SOURCE,config_id)
            api_bindings=dict(role="apiserver",expected=1,source=SOURCE,run="99002",attempt="1",tool_sha256=hashlib.sha256(PATH.read_bytes()).hexdigest(),expected_image_config_id=config_id,expected_container_ids=[api])
            tool.receive_armored_receipt(tool.transport().encode_armored_receipt(api_receipt,schema=tool.SCHEMA),**api_bindings)
            refused("image_tag_mismatch",lambda:tool.collect(tool.Docker(),"apiserver",1,SOURCE,"99002","1",SOURCE))
            wrong="sha256:"+("0"*64 if config_id != "sha256:"+"0"*64 else "1"*64)
            refused("image_config_binding_mismatch",lambda:tool.collect(tool.Docker(),"apiserver",1,SOURCE,"99002","1",SOURCE,wrong))
            refused("version_mismatch",lambda:tool.collect(tool.Docker(),"apiserver",1,"b"*40,"99002","1","b"*40,config_id))
            run(["docker","exec","--user","0",api,"sh","-c","cp /bin/busybox /app/replacement; mv /app/replacement /app/qs-apiserver"])
            refused("binary_modified",lambda:tool.collect(tool.Docker(),"apiserver",1,SOURCE,"99002","1",SOURCE,config_id))
            result_payload={"local_only":True,"production_operations":False,"native_worker_instances":3,"native_api_instances":1,"actual_docker_projection":True,"real_repository_version_formatter":True,"fixture_build_source_is_synthetic":True,"proc_executable_sha_verified":True,"image_config_digest_verified":True,"loopback_readiness_verified":True,"stopped_extra_rejected":True,"modified_worker_binary_rejected":True,"exact_api_config_image_id_observed":True,"missing_and_wrong_api_config_id_rejected":True,"wrong_actual_compiled_version_rejected":True,"modified_api_binary_rejected":True,"semantic_v2_receiver_verified":True,"business_acceptance_verified":False}
        except Exception as error:
            safe_failures={"existing_local_role_refused","non_local_docker_endpoint_refused","non_local_docker_daemon_refused"}
            failure=str(error) if type(error) is tool.Refused or type(error) is RuntimeError and str(error) in safe_failures else "local_runtime_native_failed"
        finally:
            for cid in list(containers):
                try:remove_container(cid)
                except Exception:failure="local_runtime_cleanup_unproven"
            for reference,cid in list(images.items()):
                if reference == cid:continue
                try:
                    if image_id(reference) != cid:raise RuntimeError("native_image_replaced")
                    run(["docker","image","rm",reference])
                except Exception:failure="local_runtime_cleanup_unproven"
        # Optional caller-owned private log retention, never raw public output.
        log_directory=os.environ.get("QS_RUNTIME_EVIDENCE_NATIVE_LOG_DIR")
        if log_directory:
            try:
                parent=Path(log_directory); header=parent.lstat()
                if not stat.S_ISDIR(header.st_mode) or parent.resolve()!=parent.absolute() or header.st_uid!=os.getuid() or header.st_mode & 0o077:raise ValueError("private_log_directory_required")
                destination=parent/(owner+".log")
                fd=os.open(destination,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
                with os.fdopen(fd,"wb") as stream:stream.write(log.read_bytes());stream.flush();os.fsync(stream.fileno())
            except Exception:failure="local_runtime_private_log_failed"
        if failure:
            print("runtime_evidence_native_refused:"+failure,file=sys.stderr);return 1
        result_payload.update(private_native_log_sha256=hashlib.sha256(log.read_bytes()).hexdigest(),owned_resources_cleaned=True,passed=True)
        print(json.dumps(result_payload));return 0


if __name__ == "__main__":sys.exit(main())

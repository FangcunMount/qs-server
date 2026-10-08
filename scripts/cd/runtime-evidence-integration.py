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
    containers = {}; container_mounts = {}; images = {}; result_payload = None; failure = None
    label = "qs.cd.runtime-evidence.test-owner"
    with tempfile.TemporaryDirectory(prefix="qs-runtime-evidence-native-") as temporary:
        private = Path(temporary); private.chmod(0o700)
        log = private/"native.private.log"; log.touch(mode=0o600)
        manifest = private/"owned-resources.jsonl"; manifest.touch(mode=0o600)
        def record_resource(value):
            with manifest.open("a") as stream:
                stream.write(json.dumps(dict(value, owner=owner, label=label), sort_keys=True)+"\n")
                stream.flush(); os.fsync(stream.fileno())
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
            if type(mounts) is not list:return False
            observed = {}
            for mount in mounts:
                if type(mount) is not dict or mount.get("Destination") in observed:return False
                observed[mount.get("Destination")] = {key:mount.get(key) for key in ("Type", "Source", "RW")}
            return actual == cid and image == containers[cid] and observed_owner == owner and network == "none" and ports in (None,{}) and observed == container_mounts[cid]
        def remove_container(cid):
            if not owned_container(cid):raise RuntimeError("native_container_owner_unproven")
            run(["docker", "rm", "--force", cid])
            record_resource({"kind":"container_removed", "container_id":cid})
            del containers[cid]; del container_mounts[cid]
        def create_container(name, role, reference, *, stopped=False, mounts=None):
            arguments=["docker", "create" if stopped else "run"]
            if not stopped:arguments += ["--detach", "--pull=never", "--memory", "128m", "--cpus", "0.5"]
            arguments += ["--network", "none", "--name", name, "--label", label+"="+owner,
                          "--label", "com.docker.compose.project="+tool.ROLES[role][0],
                          "--label", "com.docker.compose.service="+tool.ROLES[role][1],
                          "--label", "com.docker.compose.container-number="+str(len(containers)+1),
                          "--label", "com.docker.compose.oneoff=False"]
            expected_mounts = {}
            for destination, source in (mounts or {}).items():
                arguments += ["--mount", "type=bind,src="+str(source)+",dst="+destination+",readonly"]
                expected_mounts[destination] = {"Type":"bind", "Source":str(source), "RW":False}
            arguments += [reference]
            cid=run(arguments).stdout.decode().strip()
            if not tool.HASH.fullmatch(cid):raise RuntimeError("native_created_id_unproven")
            containers[cid]=images[reference]; container_mounts[cid]=expected_mounts
            record_resource({"kind":"container_created", "container_id":cid, "image_id":images[reference], "network":"none", "ports":{}, "mounts":expected_mounts})
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
            (private/"Dockerfile").write_text('FROM alpine:3.21\nCOPY fixture /app/qs-worker\nUSER 10001:10001\nENTRYPOINT ["/app/qs-worker"]\n')
            run(["docker", "build", "--pull=false", "--label", label+"="+owner, "-t", worker_image, str(private)], timeout=120)
            images[worker_image]=image_id(worker_image)
            record_resource({"kind":"image_created", "reference":worker_image, "image_id":images[worker_image]})
            mount_sources = {"/app/configs":private/"configs-ro", "/data/logs/qs":private/"logs-ro"}
            for path in mount_sources.values():path.mkdir(mode=0o755)
            workers=[create_container(owner+"-"+str(index),"worker",worker_image,mounts=mount_sources) for index in range(1,4)]
            for cid in workers:wait_ready(cid,"worker")
            # Reproduce the real non-root image boundary. The same user can
            # read its executable; container root has no ptrace capability.
            same_user=run(["docker","exec",workers[0],"sha256sum","/proc/1/exe","/app/qs-worker"])
            root_user=run(["docker","exec","--user","0",workers[0],"sha256sum","/proc/1/exe","/app/qs-worker"],check=False)
            expected_lines=same_user.stdout.decode().splitlines()
            if len(expected_lines)!=2 or expected_lines[0].split()[0]!=expected_lines[1].split()[0] or root_user.returncode==0 or b"Permission denied" not in root_user.stderr:
                raise RuntimeError("native_nonroot_process_boundary_not_observed")
            receipt=tool.collect(tool.Docker(),"worker",3,SOURCE,"99001","1",SOURCE)
            bindings=dict(role="worker",expected=3,source=SOURCE,run="99001",attempt="1",tool_sha256=hashlib.sha256(PATH.read_bytes()).hexdigest(),expected_container_ids=sorted(workers))
            tool.receive_armored_receipt(tool.transport().encode_armored_receipt(receipt,schema=tool.SCHEMA),**bindings)
            # Real two-bind native projections and hashes. Current daemons may
            # already sort Mounts; controlled reversal below is explicitly a
            # transport permutation, never production-root-cause evidence.
            observed_orders = set()
            for _ in range(24):
                projection = tool.Docker().inspect(workers[0])
                if len(projection["mount_destinations"]) != 2:raise RuntimeError("native_two_mount_projection_missing")
                observed_orders.add(tuple(projection["mount_destinations"]))
            for instance in receipt["instances"]:
                current = tool.Docker().inspect(instance["container_id"])
                if instance["state_sha256"] != tool.digest(tool.canonical_projection(current)):raise RuntimeError("native_canonical_state_hash_mismatch")
            class ReorderedDocker(tool.Docker):
                def __init__(self):super().__init__(); self.inspects=0
                def inspect(self, cid):
                    value=super().inspect(cid); self.inspects+=1
                    if self.inspects > len(workers):value["mount_destinations"].reverse()
                    return value
            reordered=tool.collect(ReorderedDocker(),"worker",3,SOURCE,"99001","1",SOURCE)
            if reordered != receipt:raise RuntimeError("native_order_only_receipt_changed")
            tool.receive_armored_receipt(tool.transport().encode_armored_receipt(reordered,schema=tool.SCHEMA),**bindings)
            class RestartedDocker(tool.Docker):
                def __init__(self):super().__init__(); self.ready_reads=0
                def capture(self, args, **kwargs):
                    result=super().capture(args, **kwargs)
                    if "wget" in args:
                        self.ready_reads+=1
                        if self.ready_reads == len(workers):
                            if not owned_container(workers[0]):raise RuntimeError("native_restart_owner_unproven")
                            run(["docker","restart","--time","1",workers[0]])
                            wait_ready(workers[0],"worker")
                    return result
            try:tool.collect(RestartedDocker(),"worker",3,SOURCE,"99001","1",SOURCE)
            except tool.Refused as error:
                diagnostic=tool.runtime_change_diagnostic(error)
                if str(error)!="runtime_changed" or "started_at" not in diagnostic["changed_fields"]:raise
            else:raise RuntimeError("native_real_restart_change_not_refused")
            extra=create_container(owner+"-stopped","worker",worker_image,stopped=True)
            refused("instance_set_invalid",lambda:tool.collect(tool.Docker(),"worker",3,SOURCE,"99001","1",SOURCE))
            remove_container(extra)
            run(["docker","exec","--user","0",workers[0],"sh","-c","cp /bin/busybox /app/replacement; mv /app/replacement /app/qs-worker"])
            refused("binary_modified",lambda:tool.collect(tool.Docker(),"worker",3,SOURCE,"99001","1",SOURCE))

            api_image=owner+"-api:"+SOURCE
            (private/"Dockerfile").write_text('FROM alpine:3.21\nCOPY fixture /app/qs-apiserver\nUSER 10001:10001\nENTRYPOINT ["/app/qs-apiserver"]\n')
            run(["docker","build","--pull=false","--label",label+"="+owner,"-t",api_image,str(private)],timeout=120)
            config_id=image_id(api_image); images[api_image]=config_id; images[config_id]=config_id
            record_resource({"kind":"image_created", "reference":api_image, "image_id":config_id})
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
            result_payload={"local_only":True,"production_operations":False,"native_worker_instances":3,"native_api_instances":1,"actual_docker_projection":True,"real_repository_version_formatter":True,"fixture_build_source_is_synthetic":True,"proc_executable_sha_verified":True,"nonroot_entrypoint_user":True,"root_proc_access_denied_without_extra_capabilities":True,"same_user_proc_hash_read_verified":True,"image_config_digest_verified":True,"loopback_readiness_verified":True,"stopped_extra_rejected":True,"modified_worker_binary_rejected":True,"exact_api_config_image_id_observed":True,"missing_and_wrong_api_config_id_rejected":True,"wrong_actual_compiled_version_rejected":True,"modified_api_binary_rejected":True,"semantic_v2_receiver_verified":True,"business_acceptance_verified":False,"native_worker_two_readonly_bind_mounts":True,"native_canonical_state_hash_verified":True,"controlled_actual_projection_permutation_verified":True,"native_mount_order_variants":len(observed_orders),"production_failure_root_cause_verified":False,"real_owned_container_restart_refused":True}
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
                    record_resource({"kind":"image_removed", "reference":reference, "image_id":cid})
                except Exception:failure="local_runtime_cleanup_unproven"
            try:
                if not manifest.read_text():raise LookupError("no_owned_resources_created")
                remaining_containers=run(["docker","ps","--all","--no-trunc","--filter","label="+label+"="+owner,"--format","{{.ID}}"],check=False)
                remaining_images=run(["docker","image","ls","--no-trunc","--filter","label="+label+"="+owner,"--format","{{.ID}}"],check=False)
                if remaining_containers.returncode or remaining_images.returncode or remaining_containers.stdout.strip() or remaining_images.stdout.strip():failure="local_runtime_cleanup_unproven"
                record_resource({"kind":"cleanup_readback", "remaining_containers":0 if not remaining_containers.stdout.strip() else None, "remaining_images":0 if not remaining_images.stdout.strip() else None})
            except LookupError:pass
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
                destination=parent/(owner+".resources.jsonl")
                fd=os.open(destination,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
                with os.fdopen(fd,"wb") as stream:stream.write(manifest.read_bytes());stream.flush();os.fsync(stream.fileno())
            except Exception:failure="local_runtime_private_log_failed"
        if failure:
            print("runtime_evidence_native_refused:"+failure,file=sys.stderr);return 1
        result_payload.update(private_native_log_sha256=hashlib.sha256(log.read_bytes()).hexdigest(),private_ownership_manifest_sha256=hashlib.sha256(manifest.read_bytes()).hexdigest(),owned_resources_cleaned=True,owned_containers_remaining=0,owned_images_remaining=0,passed=True)
        print(json.dumps(result_payload));return 0


if __name__ == "__main__":sys.exit(main())

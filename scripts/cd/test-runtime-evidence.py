#!/usr/bin/env python3
"""Synthetic failure tests: no Docker or production access in this suite."""
import contextlib
import copy
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import unittest
from unittest import mock

PATH = Path(__file__).with_name("runtime-evidence.py")
SPEC = importlib.util.spec_from_file_location("runtime_evidence", PATH)
tool = importlib.util.module_from_spec(SPEC); SPEC.loader.exec_module(tool)
SOURCE = "a" * 40
VERSION = "gitVersion: v1.0.0\ngitCommit: "+SOURCE+"\ngitTreeState: clean\nbuildDate: 2026-10-08T12:00:00Z\ngoVersion: go1.25.12\ncompiler: gc\nplatform: linux/arm64\n"
SECRET = "SYNTHETIC_PRIVATE_OUTPUT_DO_NOT_PRINT"


class FakeDocker:
    def __init__(self, role="worker", count=3):
        project, service, binary, _, _ = tool.ROLES[role]
        self.rows = []; self.values = {}; self.calls = []
        self.lists = 0; self.inspects = 0; self.after = None
        self.version = VERSION; self.changes = ""; self.hashes = ["b"*64, "b"*64]; self.ready = True
        self.image_entrypoint = [binary]
        for i in range(1, count+1):
            cid = format(i, "064x"); name = "qs-apiserver" if role == "apiserver" else project+"-"+service+"-"+str(i)
            row = {"container_id": cid, "name": name, "project": project, "service": service}; self.rows.append(row)
            self.values[cid] = dict(row, name="/"+name, image_id="sha256:"+"f"*64, image_reference="registry/repo:"+SOURCE,
                entrypoint=[binary], path=binary, status="running", running=True, dead=False, restarting=False,
                started_at="2026-10-08T12:00:00.000000000Z", finished_at="0001-01-01T00:00:00Z", restart_count=0,
                number=str(i), oneoff="False", mount_destinations=["/app/configs", "/data/logs/qs"])

    def listing(self):
        self.lists += 1
        return copy.deepcopy(self.rows if self.lists == 1 or self.after is None else self.after)

    def inspect(self, cid):
        self.inspects += 1
        value = copy.deepcopy(self.values[cid])
        if self.inspects > len(self.rows) and hasattr(self, "change"):
            self.change(value)
        return value

    def capture(self, args, **kwargs):
        self.calls.append(args)
        if args[:2] == ["image", "inspect"]:return json.dumps({"image_id": args[-1], "entrypoint": self.image_entrypoint})
        if args[0] == "diff":return self.changes
        if "sha256sum" in args:return self.hashes[0]+"  /proc/1/exe\n"+self.hashes[1]+"  "+args[-1]+"\n"
        if "--version=true" in args:return self.version
        if "wget" in args:
            if not self.ready:tool.refuse("readiness_failed")
            return ""
        raise AssertionError("unexpected fixed Docker command")


class RuntimeEvidenceTests(unittest.TestCase):
    def collect(self, docker=None, role="worker", count=3, **override):
        params = dict(role=role, expected=count, source=SOURCE, run="12345", attempt="1", image_tag=SOURCE); params.update(override)
        return tool.collect(docker or FakeDocker(role, count), **params)

    def blocked(self, category, docker, **params):
        with self.assertRaisesRegex(tool.Refused, "^"+category+"$"):self.collect(docker, **params)

    def test_all_roles_prove_real_independent_instance_identity(self):
        for role, count in (("apiserver", 1), ("collection", 2), ("worker", 3)):
            docker = FakeDocker(role, count); receipt = self.collect(docker, role, count)
            self.assertIs(receipt["complete"], True); self.assertIs(receipt["health_verified"], True)
            self.assertIs(receipt["business_acceptance_verified"], False)
            self.assertEqual(docker.lists, 2); self.assertEqual(docker.inspects, count*2)
            self.assertEqual(len({i["container_id"] for i in receipt["instances"]}), count)
            self.assertEqual([args[-1] for args in docker.calls if "--version=true" in args], ["--config="+tool.VERSION_CONFIGS[role]]*count)
            armor = tool.transport().encode_armored_receipt(receipt, schema=tool.SCHEMA)
            self.assertEqual(json.loads(tool.transport().decode_armored_receipt(armor)), receipt)

    def test_input_requires_real_full_source_and_run(self):
        for change in ({"source":"abc"}, {"image_tag":"b"*40}, {"run":""}, {"run":"001"}, {"attempt":"0"}, {"expected":0}, {"expected":33}):
            docker = FakeDocker(); self.blocked("input_binding_invalid", docker, **change)
            self.assertFalse(docker.calls); self.assertEqual(docker.lists, 0)

    def test_old_tag_refused(self):
        docker=FakeDocker(); docker.values[docker.rows[0]["container_id"]]["image_reference"]="registry/repo:"+"b"*40
        self.blocked("image_tag_mismatch", docker)

    def test_tag_is_not_image_or_manifest_identity(self):
        receipt = self.collect(); self.assertEqual(receipt["instances"][0]["image_config_sha256"], "f"*64)
        self.assertNotIn("manifest", json.dumps(receipt))

    def test_wrong_compiled_version_refused(self):
        docker=FakeDocker(); docker.version=VERSION.replace(SOURCE, "b"*40)
        self.blocked("version_mismatch", docker)

    def test_incomplete_duplicate_or_poisoned_version_block_refused(self):
        for output in (VERSION+"gitCommit: "+SOURCE+"\n", VERSION+VERSION, VERSION.replace("compiler: gc", "password: "+SECRET), "gitCommit: "+SOURCE+"\n"):
            docker=FakeDocker(); docker.version=output; self.blocked("version_output_rejected", docker)

    def test_extra_sensitive_app_output_is_discarded_and_cannot_pollute_proof(self):
        docker=FakeDocker(); docker.version="INFO WorkingDir: /app\nDEBUG FLAG: password="+SECRET+"\n"+VERSION.replace("compiler: gc","compiler: "+SECRET)+SECRET+"\n"
        receipt=self.collect(docker)
        self.assertNotIn(SECRET,json.dumps(receipt)); self.assertEqual(receipt["instances"][0]["build_git_commit"],SOURCE)
        self.assertNotIn(SECRET,tool.transport().encode_armored_receipt(receipt,schema=tool.SCHEMA))

    def test_current_docker_branch_version_and_empty_tree_state_are_valid(self):
        docker=FakeDocker(); docker.version=VERSION.replace("gitVersion: v1.0.0", "gitVersion: main").replace("gitTreeState: clean", "gitTreeState: ")
        docker.version="\n".join(" "*(12-len(line.split(":",1)[0]))+line for line in docker.version.splitlines())+"\n"
        self.assertTrue(self.collect(docker)["complete"])

    def test_replaced_binary_refused_even_when_tag_and_version_match(self):
        docker=FakeDocker(); docker.changes="C /app/qs-worker\n"; self.blocked("binary_modified", docker)
        docker=FakeDocker(); docker.hashes[0]="c"*64; self.blocked("binary_hash_mismatch", docker)

    def test_mounts_cannot_shadow_binary_or_proc(self):
        for destination in ("/", "/app", "/app/qs-worker", "/proc", "/proc/1", "/other/../app", "//app/qs-worker"):
            docker=FakeDocker(); docker.values[docker.rows[0]["container_id"]]["mount_destinations"].append(destination)
            self.blocked("binary_shadowed", docker)

    def test_configuration_mount_and_unrelated_changes_allowed(self):
        docker=FakeDocker(); docker.changes="C /app\nA /tmp/request-buffer\n"
        self.assertTrue(self.collect(docker)["complete"])

    def test_entrypoint_and_image_configuration_both_fixed(self):
        docker=FakeDocker(); docker.values[docker.rows[0]["container_id"]]["entrypoint"]=["/bin/sh"]
        self.blocked("entrypoint_mismatch", docker)
        docker=FakeDocker(); docker.image_entrypoint=["/bin/sh"]
        self.blocked("entrypoint_mismatch", docker)

    def test_not_running_and_restarting_refused(self):
        for changes in ({"running":False}, {"restarting":True}, {"dead":True}, {"status":"exited"}):
            docker=FakeDocker(); docker.values[docker.rows[0]["container_id"]].update(changes)
            self.blocked("container_not_running", docker)

    def test_stopped_extra_and_missing_replicas_refused(self):
        docker=FakeDocker(); docker.rows.append(dict(docker.rows[0], container_id="9"*64, name="qs-worker-runtime-4"))
        self.blocked("instance_set_invalid", docker)
        docker=FakeDocker(); docker.rows.pop(); self.blocked("instance_set_invalid", docker)

    def test_legacy_fixed_name_is_extra_instance(self):
        docker=FakeDocker(); docker.rows.append({"container_id":"9"*64,"name":"qs-worker","project":"","service":""})
        self.blocked("instance_set_invalid", docker)

    def test_duplicate_replica_number_and_oneoff_refused(self):
        docker=FakeDocker(); docker.values[docker.rows[1]["container_id"]]["number"]="1"
        self.blocked("instance_set_invalid", docker)
        docker=FakeDocker(); docker.values[docker.rows[0]["container_id"]]["oneoff"]="True"
        self.blocked("topology_mismatch", docker)

    def test_api_name_and_service_both_required(self):
        docker=FakeDocker("apiserver",1); docker.rows[0]["service"]="server"
        self.blocked("topology_mismatch",docker,role="apiserver",count=1)

    def test_ready_failure_refused_and_collection_has_no_fallback(self):
        docker=FakeDocker(); docker.ready=False; self.blocked("readiness_failed",docker)
        docker=FakeDocker("collection",2); self.collect(docker,"collection",2)
        urls=[args[-1] for args in docker.calls if "wget" in args]
        self.assertEqual(urls,["http://127.0.0.1:8080/serve-readyz"]*2)

    def test_restart_image_and_started_time_changes_refused(self):
        for changes in ({"restart_count":1}, {"started_at":"2026-10-08T12:01:00Z"}, {"image_id":"sha256:"+"e"*64}, {"status":"exited"}):
            docker=FakeDocker(); docker.change=lambda v, c=changes:v.update(c)
            self.blocked("runtime_changed",docker)

    def test_complete_role_listing_reread_refuses_new_stopped_container(self):
        docker=FakeDocker(); docker.after=copy.deepcopy(docker.rows)+[dict(docker.rows[0],container_id="9"*64,name="qs-worker-runtime-4")]
        self.blocked("instance_set_invalid",docker)

    def test_projection_never_requests_env_network_or_logs(self):
        for field in (".Config.Env",".NetworkSettings",".Mounts.Source",".Args"):
            self.assertNotIn(field,tool.INSPECT_TEMPLATE)
        docker=FakeDocker(); self.collect(docker)
        for args in docker.calls:self.assertNotIn("logs",args)

    def test_process_failure_never_exposes_secret_stdout_or_stderr(self):
        result=subprocess.CompletedProcess([],1,SECRET.encode(),SECRET.encode())
        listing=["ps","--all","--no-trunc","--format",tool.LIST_TEMPLATE]
        with mock.patch.object(tool.subprocess,"run",return_value=result):
            with self.assertRaisesRegex(tool.Refused,"^docker_command_failed$"):tool.Docker().capture(listing)
        result.returncode=0; result.stdout=VERSION.encode()
        with mock.patch.object(tool.subprocess,"run",return_value=result):
            self.assertEqual(tool.Docker(True).capture(["exec","1"*64,"/app/qs-worker","--version=true","--config=/app/configs/worker.prod.yaml"],discard_stderr=True),VERSION)
        result.returncode=0
        with mock.patch.object(tool.subprocess,"run",return_value=result):
            with self.assertRaisesRegex(tool.Refused,"^docker_command_failed$"):tool.Docker(True).capture(listing)

    def test_docker_prefix_is_exact_boolean_and_never_privileged_python(self):
        command=["ps","--all","--no-trunc","--format",tool.LIST_TEMPLATE]
        result=subprocess.CompletedProcess([],0,b"",b"")
        for flag, prefix in ((False,["docker"]),(True,["sudo","-n","docker"])):
            with mock.patch.dict(tool.os.environ,{},clear=True), mock.patch.object(tool.subprocess,"run",return_value=result) as run:
                self.assertEqual(tool.Docker(flag).capture(command),"")
                self.assertEqual(run.call_args.args[0],prefix+command)
                self.assertNotIn("shell",run.call_args.kwargs)
        for value in ("sudo -S",["sudo","python3"],1,None):
            with self.assertRaisesRegex(tool.Refused,"^input_binding_invalid$"):tool.Docker(value)

    def test_password_sudo_uses_private_stdin_and_fixed_docker_only(self):
        command=["ps","--all","--no-trunc","--format",tool.LIST_TEMPLATE]
        result=subprocess.CompletedProcess([],0,b"",b"")
        with mock.patch.dict(tool.os.environ,{"SUDO_PASSWORD":SECRET}), mock.patch.object(tool.subprocess,"run",return_value=result) as run:
            self.assertEqual(tool.Docker(True).capture(command),"")
            self.assertEqual(run.call_args.args[0],["sudo","-S","-p","","docker",*command])
            self.assertEqual(run.call_args.kwargs["input"],SECRET.encode()+b"\n")
            self.assertNotIn(SECRET,repr(run.call_args.args[0]))
            self.assertNotIn("shell",run.call_args.kwargs)
            self.assertEqual(tool.Docker(False).capture(command),"")
            self.assertEqual(run.call_args.args[0],["docker",*command])
            self.assertNotIn("input",run.call_args.kwargs)

    def test_invalid_password_input_is_refused_before_any_command(self):
        command=["ps","--all","--no-trunc","--format",tool.LIST_TEMPLATE]
        for password in (SECRET+"\n",SECRET+"\r",SECRET+"\x00","x"*4097,"\ud800"):
            with mock.patch.object(tool.os,"environ",{"SUDO_PASSWORD":password}), mock.patch.object(tool.subprocess,"run") as run:
                with self.assertRaisesRegex(tool.Refused,"^input_binding_invalid$"):tool.Docker(True).capture(command)
                run.assert_not_called()

    def test_password_failure_does_not_echo_credentials_or_process_output(self):
        command=["ps","--all","--no-trunc","--format",tool.LIST_TEMPLATE]
        result=subprocess.CompletedProcess([],1,SECRET.encode(),SECRET.encode())
        out=io.StringIO();err=io.StringIO()
        with mock.patch.dict(tool.os.environ,{"SUDO_PASSWORD":SECRET}), mock.patch.object(tool.subprocess,"run",return_value=result), contextlib.redirect_stdout(out),contextlib.redirect_stderr(err):
            with self.assertRaisesRegex(tool.Refused,"^docker_command_failed$"):tool.Docker(True).capture(command)
        self.assertEqual(out.getvalue()+err.getvalue(),"")

    def test_executor_refuses_mutation_arbitrary_exec_and_free_sudo_arguments(self):
        commands=(["restart","1"*64],["rm","1"*64],["exec","1"*64,"sh","-c",SECRET],
                  ["exec","1"*64,"env"],["exec","1"*64,"wget","http://external/"],
                  ["inspect","1"*64],["sudo","-S","docker","ps"])
        with mock.patch.object(tool.subprocess,"run") as run:
            for command in commands:
                with self.assertRaisesRegex(tool.Refused,"^docker_command_refused$"):tool.Docker(True).capture(command)
            run.assert_not_called()

    def test_unknown_prefix_cli_is_refused_without_echoing_sensitive_input(self):
        for extra in (["--sudo-prefix",SECRET],["--sudo-docker="+SECRET]):
            out=io.StringIO();err=io.StringIO()
            with mock.patch.object(tool.subprocess,"run") as run, contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
                self.assertEqual(tool.main(["--role","worker","--expected-instances","3","--source-sha",SOURCE,"--image-tag",SOURCE,"--run-id","12345","--run-attempt","1",*extra]),1)
                run.assert_not_called()
            self.assertEqual(out.getvalue(),"");self.assertNotIn(SECRET,err.getvalue())
            self.assertEqual(err.getvalue(),"runtime_instance_evidence_refused:input_binding_invalid\n")

    def test_cli_prints_only_safe_receipt_or_fixed_failure_category(self):
        out=io.StringIO(); err=io.StringIO()
        with mock.patch.object(tool,"Docker",return_value=FakeDocker()), contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            self.assertEqual(tool.main(["--role","worker","--expected-instances","3","--source-sha",SOURCE,"--image-tag",SOURCE,"--run-id","12345","--run-attempt","1"]),0)
        self.assertTrue(json.loads(tool.transport().decode_armored_receipt(out.getvalue()))["health_verified"])
        docker=FakeDocker(); docker.version=SECRET; out=io.StringIO(); err=io.StringIO()
        with mock.patch.object(tool,"Docker",return_value=docker), contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            self.assertEqual(tool.main(["--role","worker","--expected-instances","3","--source-sha",SOURCE,"--image-tag",SOURCE,"--run-id","12345","--run-attempt","1"]),1)
        self.assertEqual(out.getvalue(),""); self.assertNotIn(SECRET,err.getvalue())

    def test_cd_binding_and_packaged_transport_are_explicit(self):
        root=PATH.resolve().parents[2]
        workflow=(root/".github/workflows/cd.yml").read_text()
        self.assertEqual(workflow.count("CD_SOURCE_SHA: ${{ env.DEPLOY_SHA }}"),3)
        self.assertEqual(workflow.count("CD_RUN_ID: ${{ github.run_id }}"),3)
        self.assertEqual(workflow.count("CD_RUN_ATTEMPT: ${{ github.run_attempt }}"),3)
        runner=(root/"scripts/cd/runner-upload-and-deploy.sh").read_text()
        for key in ("CD_SOURCE_SHA","CD_RUN_ID","CD_RUN_ATTEMPT"):
            self.assertIn("emit_export "+key,runner)
        remote=(root/"scripts/cd/remote-deploy.sh").read_text()
        self.assertNotIn('$SUDO python3 "$DEPLOY_TMP/scripts/cd/runtime-evidence.py"',remote)
        self.assertIn('if [ -n "${SUDO:-}" ]; then runtime_docker_args=(--sudo-docker); fi',remote)
        self.assertLess(remote.index('python3 "$DEPLOY_TMP/scripts/cd/runtime-evidence.py"'),remote.index('retain_successful_image "$(resolve_compose_image_ref)"'))


if __name__ == "__main__":unittest.main()

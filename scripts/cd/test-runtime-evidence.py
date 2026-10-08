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

    def pinned_api(self):
        docker = FakeDocker("apiserver", 1)
        value = docker.values[docker.rows[0]["container_id"]]
        value["image_reference"] = value["image_id"]
        return docker, value["image_id"]

    def receipt_bindings(self, role="worker", count=3, config=None):
        return dict(role=role, expected=count, source=SOURCE, run="12345", attempt="1",
                    tool_sha256=tool.hashlib.sha256(PATH.read_bytes()).hexdigest(), expected_image_config_id=config)

    def test_exact_api_preflight_image_config_binding(self):
        docker, config = self.pinned_api()
        receipt = self.collect(docker, role="apiserver", count=1, expected_image_config_id=config)
        self.assertEqual(receipt["format_version"], 2)
        self.assertEqual(receipt["image_binding_kind"], "preflight_config_id")
        self.assertEqual(receipt["expected_image_config_sha256"], config[7:])
        self.assertEqual(receipt["instances"][0]["image_config_sha256"], config[7:])
        self.assertIs(tool.receive_armored_receipt(tool.transport().encode_armored_receipt(receipt, schema=tool.SCHEMA), **self.receipt_bindings("apiserver", 1, config))["complete"], True)

    def test_preflight_config_input_only_api_count_one(self):
        for role, count in (("collection", 2), ("worker", 3), ("apiserver", 2)):
            docker = FakeDocker(role, count)
            self.blocked("input_binding_invalid", docker, role=role, count=count, expected_image_config_id="sha256:"+"f"*64)
            self.assertEqual(docker.lists, 0)
        for config in ("", "f"*64, "sha256:"+"F"*64, "registry@sha256:"+"f"*64, 1, SECRET):
            docker, _ = self.pinned_api()
            self.blocked("input_binding_invalid", docker, role="apiserver", count=1, expected_image_config_id=config)
            self.assertEqual(docker.lists, 0)

    def test_missing_wrong_default_and_registry_digest_bindings_refused(self):
        docker, config = self.pinned_api()
        self.blocked("image_tag_mismatch", docker, role="apiserver", count=1)
        docker, config = self.pinned_api()
        self.blocked("image_config_binding_mismatch", docker, role="apiserver", count=1, expected_image_config_id="sha256:"+"e"*64)
        for reference in ("registry@sha256:"+"f"*64, "registry/repo:"+SOURCE, "sha256:"+"e"*64):
            docker, config = self.pinned_api()
            docker.values[docker.rows[0]["container_id"]]["image_reference"] = reference
            self.blocked("image_config_binding_mismatch", docker, role="apiserver", count=1, expected_image_config_id=config)
        docker = FakeDocker(); docker.values[docker.rows[0]["container_id"]]["image_reference"] = "registry@sha256:"+"f"*64
        self.blocked("image_tag_mismatch", docker)
        docker, config = self.pinned_api(); docker.values[docker.rows[0]["container_id"]]["image_id"] = "sha256:"+"e"*64
        self.blocked("image_config_binding_mismatch", docker, role="apiserver", count=1, expected_image_config_id=config)

    def test_pinned_api_still_proves_version_binary_and_reread(self):
        for category, change in (("version_mismatch", lambda d:setattr(d,"version",VERSION.replace(SOURCE,"b"*40))),
                                 ("binary_modified", lambda d:setattr(d,"changes","C /app/qs-apiserver\n")),
                                 ("binary_hash_mismatch", lambda d:setattr(d,"hashes",["e"*64,"b"*64])),
                                 ("runtime_changed", lambda d:setattr(d,"change",lambda v:v.update(image_id="sha256:"+"e"*64))),
                                 ("readiness_failed", lambda d:setattr(d,"ready",False))):
            docker, config = self.pinned_api(); change(docker)
            self.blocked(category, docker, role="apiserver", count=1, expected_image_config_id=config)

    def test_semantic_receiver_requires_every_field_not_schema_only(self):
        receipt = self.collect()
        for key in receipt:
            changed = copy.deepcopy(receipt); del changed[key]
            armor = tool.transport().encode_armored_receipt(changed, schema=tool.SCHEMA)
            with self.assertRaisesRegex(tool.Refused,"^receipt_binding_invalid$"):
                tool.receive_armored_receipt(armor, **self.receipt_bindings())
        for key in tool.INSTANCE_SCHEMA:
            changed = copy.deepcopy(receipt); del changed["instances"][0][key]
            armor = tool.transport().encode_armored_receipt(changed, schema=tool.SCHEMA)
            with self.assertRaisesRegex(tool.Refused,"^receipt_binding_invalid$"):
                tool.receive_armored_receipt(armor, **self.receipt_bindings())

    def test_semantic_receiver_binds_source_run_tool_mode_instances_and_body(self):
        receipt = self.collect()
        changes = ({"format_version":1}, {"source_sha":"b"*40}, {"image_tag":"b"*40}, {"run_id":"12345-2"},
                   {"role":"collection"}, {"expected_instances":2}, {"observed_instances":2}, {"tool_sha256":"b"*64},
                   {"topology_sha256":"b"*64}, {"complete":False}, {"health_verified":False},
                   {"business_acceptance_verified":True}, {"error_category":"runtime_changed"},
                   {"image_binding_kind":"preflight_config_id"}, {"expected_image_config_sha256":"f"*64})
        for change in changes:
            changed = copy.deepcopy(receipt); changed.update(change)
            changed["receipt_body_sha256"] = tool.digest({k:v for k,v in changed.items() if k!="receipt_body_sha256"})
            with self.assertRaisesRegex(tool.Refused,"^receipt_binding_invalid$"):
                tool.validate_receipt(changed, **self.receipt_bindings())
        for kind in ("duplicate_id", "wrong_version", "not_ready", "unknown_field", "bad_body_hash"):
            changed = copy.deepcopy(receipt)
            if kind == "duplicate_id":changed["instances"][1]["container_id"] = changed["instances"][0]["container_id"]
            elif kind == "wrong_version":changed["instances"][0]["build_git_commit"] = "b"*40
            elif kind == "not_ready":changed["instances"][0]["ready"] = False
            elif kind == "unknown_field":changed["instances"][0]["credentials"] = SECRET
            else:changed["receipt_body_sha256"] = "b"*64
            if kind != "bad_body_hash":changed["receipt_body_sha256"] = tool.digest({k:v for k,v in changed.items() if k!="receipt_body_sha256"})
            with self.assertRaisesRegex(tool.Refused,"^receipt_binding_invalid$"):
                tool.validate_receipt(changed, **self.receipt_bindings())

    def test_semantic_receiver_can_bind_independent_actual_instance_and_state(self):
        receipt = self.collect()
        bindings = dict(self.receipt_bindings(), expected_container_ids=[i["container_id"] for i in receipt["instances"]],
                        expected_state_sha256=[i["state_sha256"] for i in receipt["instances"]],
                        expected_instance_set_sha256=receipt["instance_set_sha256"])
        self.assertIs(tool.validate_receipt(receipt, **bindings), receipt)
        for key in ("expected_container_ids", "expected_state_sha256", "expected_instance_set_sha256"):
            changed = copy.deepcopy(bindings)
            changed[key] = "e"*64 if type(changed[key]) is str else ["e"*64]*3
            with self.assertRaisesRegex(tool.Refused,"^receipt_binding_invalid$"):
                tool.validate_receipt(receipt, **changed)
        docker, config = self.pinned_api(); pinned = self.collect(docker, role="apiserver", count=1, expected_image_config_id=config)
        for expected_config in (None, "sha256:"+"e"*64):
            with self.assertRaisesRegex(tool.Refused,"^receipt_binding_invalid$"):
                tool.validate_receipt(pinned, **self.receipt_bindings("apiserver",1,expected_config))
        pinned["instances"][0]["image_config_sha256"] = "e"*64
        pinned["receipt_body_sha256"] = tool.digest({k:v for k,v in pinned.items() if k!="receipt_body_sha256"})
        with self.assertRaisesRegex(tool.Refused,"^receipt_binding_invalid$"):
            tool.validate_receipt(pinned, **self.receipt_bindings("apiserver",1,config))

    def test_producer_applies_semantic_receiver_before_emit(self):
        incomplete = self.collect(); del incomplete["image_binding_kind"]
        out=io.StringIO(); err=io.StringIO()
        with mock.patch.object(tool,"collect",return_value=incomplete), contextlib.redirect_stdout(out),contextlib.redirect_stderr(err):
            self.assertEqual(tool.main(["--role","worker","--expected-instances","3","--source-sha",SOURCE,"--image-tag",SOURCE,"--run-id","12345","--run-attempt","1"]),1)
        self.assertEqual(out.getvalue(),"");self.assertEqual(err.getvalue(),"runtime_instance_evidence_refused:receipt_binding_invalid\n")

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

    def test_mount_permutation_is_canonical_for_every_role_and_state_hash(self):
        for role, count in (("apiserver", 1), ("collection", 2), ("worker", 3)):
            baseline = self.collect(FakeDocker(role, count), role, count)
            docker = FakeDocker(role, count)
            for value in docker.values.values():value["mount_destinations"].reverse()
            docker.change = lambda value:value["mount_destinations"].reverse()
            changed = self.collect(docker, role, count)
            self.assertEqual(changed, baseline)
            for value in docker.values.values():
                self.assertEqual(tool.canonical_projection(value)["mount_destinations"], sorted(value["mount_destinations"]))
                self.assertEqual(value["mount_destinations"], ["/data/logs/qs", "/app/configs"])

    def test_mount_add_remove_and_exact_spelling_change_still_refused(self):
        for change in (lambda value:value["mount_destinations"].append("/fixture/new"),
                       lambda value:value["mount_destinations"].pop(),
                       lambda value:value.update(mount_destinations=["/app//configs", "/data/logs/qs"])):
            docker = FakeDocker(); docker.change = change
            with self.assertRaises(tool.Refused) as caught:self.collect(docker)
            self.assertEqual(str(caught.exception), "runtime_changed")
            self.assertEqual(tool.runtime_change_diagnostic(caught.exception), {"changed_fields":["mount_destinations"]})

    def test_mount_projection_rejects_duplicates_invalid_paths_and_types(self):
        for mounts in (("/a",), None, ["/a", "/a"], ["relative"], [""], [1], [True], ["/nul\x00path"], ["/newline\npath"], ["/bad\ud800path"]):
            docker = FakeDocker(); docker.values[docker.rows[0]["container_id"]]["mount_destinations"] = mounts
            self.blocked("container_projection_invalid", docker)
            self.assertFalse(docker.calls)
        value = FakeDocker().values[format(1,"064x")]
        value["mount_destinations"] = ["/z", "/a//b", "/a/b"]
        canonical = tool.canonical_projection(value)
        self.assertEqual(canonical["mount_destinations"], ["/a//b", "/a/b", "/z"])
        self.assertEqual(len(canonical["mount_destinations"]), 3)

    def test_runtime_change_diagnostics_are_fixed_fields_and_keep_category(self):
        docker = FakeDocker(); docker.change = lambda value:value.update(restart_count=1, started_at="2026-10-08T12:01:00Z")
        with self.assertRaises(tool.Refused) as caught:self.collect(docker)
        self.assertEqual(str(caught.exception), "runtime_changed")
        self.assertEqual(tool.runtime_change_diagnostic(caught.exception), {"changed_fields":["restart_count", "started_at"]})
        docker = FakeDocker(); docker.after = copy.deepcopy(docker.rows)
        docker.after[0]["container_id"] = "9"*64
        with self.assertRaises(tool.Refused) as caught:self.collect(docker)
        self.assertEqual(str(caught.exception), "runtime_changed")
        self.assertEqual(tool.runtime_change_diagnostic(caught.exception), {"changed_fields":[], "role_inventory_change":"instance_set_changed"})
        forged = tool.Refused("runtime_changed", changed_fields=(SECRET,), role_inventory_changed=SECRET)
        self.assertEqual(tool.runtime_change_diagnostic(forged), {"changed_fields":[]})
        err = tool.Refused("runtime_changed", changed_fields=("image_id", "image_id"))
        self.assertEqual(tool.runtime_change_diagnostic(err), {"changed_fields":["image_id"]})

    def test_runtime_change_cli_diagnostic_never_outputs_values_or_ids(self):
        docker = FakeDocker(); docker.change = lambda value:value.update(image_reference="registry/"+SECRET)
        output, errors = io.StringIO(), io.StringIO()
        args = ["--role","worker","--expected-instances","3","--source-sha",SOURCE,"--run-id","12345","--run-attempt","1","--image-tag",SOURCE]
        with mock.patch.object(tool,"Docker",return_value=docker), contextlib.redirect_stdout(output), contextlib.redirect_stderr(errors):
            self.assertEqual(tool.main(args), 1)
        self.assertEqual(output.getvalue(), "")
        self.assertEqual(errors.getvalue(), 'runtime_instance_evidence_refused:runtime_changed\nruntime_instance_evidence_change:{"changed_fields":["image_reference"]}\n')
        self.assertNotIn(SECRET, errors.getvalue())
        for row in docker.rows:self.assertNotIn(row["container_id"], errors.getvalue())

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

    def test_process_hash_read_uses_configured_entrypoint_user(self):
        for role, count in (("apiserver", 1), ("collection", 2), ("worker", 3)):
            docker = FakeDocker(role, count)
            self.collect(docker, role=role, expected=count)
            reads = [args for args in docker.calls if "sha256sum" in args]
            self.assertEqual(len(reads), count)
            for args in reads:
                self.assertEqual(args, ["exec", args[1], "sha256sum", "/proc/1/exe", tool.ROLES[role][2]])
                self.assertTrue(tool.readonly_docker_command(args))
                self.assertFalse(tool.readonly_docker_command(["exec", "--user", "0", *args[1:]]))

    def test_process_hash_failure_has_fixed_category_without_private_output(self):
        command = ["exec", "1"*64, "sha256sum", "/proc/1/exe", "/app/qs-worker"]
        result = subprocess.CompletedProcess([], 1, SECRET.encode(), SECRET.encode())
        with mock.patch.object(tool.subprocess, "run", return_value=result):
            with self.assertRaisesRegex(tool.Refused, "^process_executable_read_failed$"):
                tool.Docker().capture(command, failure="process_executable_read_failed")

    def test_fixed_projection_failures_identify_step_without_private_output(self):
        result = subprocess.CompletedProcess([], 1, SECRET.encode(), SECRET.encode())
        with mock.patch.object(tool.subprocess, "run", return_value=result):
            with self.assertRaisesRegex(tool.Refused, "^docker_inventory_read_failed$"):
                tool.Docker().listing()
            with self.assertRaisesRegex(tool.Refused, "^container_projection_read_failed$"):
                tool.Docker().inspect("1"*64)
        for prefix, category in ((["image", "inspect"], "image_metadata_read_failed"), (["diff"], "container_changes_read_failed")):
            docker = FakeDocker()
            original = docker.capture
            def fail_selected(args, **kwargs):
                if args[:len(prefix)] == prefix:
                    with mock.patch.object(tool.subprocess, "run", return_value=result):
                        return tool.Docker().capture(args, **kwargs)
                return original(args, **kwargs)
            docker.capture = fail_selected
            self.blocked(category, docker)

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
        self.assertLess(remote.index('\nverify_running_image\n'),remote.index('runtime_docker_args+=(--expected-image-config-id "$MQ_IMAGE_ID")'))
        self.assertIn('if [ "$SERVICE" != "apiserver" ] || ! [[ "$MQ_IMAGE_ID" =~ ^sha256:[0-9a-f]{64}$ ]]',remote)


class NativeRuntimeSafetyTests(unittest.TestCase):
    def native(self):
        path=PATH.with_name("runtime-evidence-integration.py")
        spec=importlib.util.spec_from_file_location("runtime_native_safety",path)
        module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
        return module

    def test_existing_role_is_refused_before_any_fixture_mutation(self):
        native=self.native();out=io.StringIO();err=io.StringIO()
        def command(args, **kwargs):
            if args==["docker","context","show"]:raw=b"synthetic\n"
            elif args==["docker","context","inspect","--format","{{json .Endpoints.docker.Host}}","synthetic"]:raw=b'"unix:///tmp/synthetic-docker.sock"\n'
            else:raise AssertionError("mutation or unexpected daemon command")
            return subprocess.CompletedProcess(args,0,raw,b"")
        with mock.patch.dict(native.os.environ,{},clear=True), mock.patch.object(native.sys,"argv",[str(PATH)]), mock.patch.object(native.subprocess,"run",side_effect=command), mock.patch.object(native.tool,"Docker") as docker, contextlib.redirect_stdout(out),contextlib.redirect_stderr(err):
            docker.return_value.listing.return_value=[{"container_id":"e"*64,"name":"qs-apiserver","project":"existing","service":"qs-apiserver"}]
            self.assertEqual(native.main(),1)
        self.assertEqual(out.getvalue(),"");self.assertEqual(err.getvalue(),"runtime_evidence_native_refused:existing_local_role_refused\n")

    def test_remote_named_context_is_refused_before_daemon_or_fixture_calls(self):
        native=self.native();out=io.StringIO();err=io.StringIO()
        def command(args, **kwargs):
            if args==["docker","context","show"]:raw=b"synthetic\n"
            elif args==["docker","context","inspect","--format","{{json .Endpoints.docker.Host}}","synthetic"]:raw=b'"ssh://synthetic.invalid"\n'
            else:raise AssertionError("mutation or unexpected daemon command")
            return subprocess.CompletedProcess(args,0,raw,b"")
        with mock.patch.dict(native.os.environ,{"DOCKER_CONTEXT":"synthetic"},clear=True), mock.patch.object(native.sys,"argv",[str(PATH)]), mock.patch.object(native.subprocess,"run",side_effect=command), mock.patch.object(native.tool,"Docker") as docker, contextlib.redirect_stdout(out),contextlib.redirect_stderr(err):
            self.assertEqual(native.main(),1)
            docker.assert_not_called()
        self.assertEqual(out.getvalue(),"");self.assertEqual(err.getvalue(),"runtime_evidence_native_refused:non_local_docker_endpoint_refused\n")


if __name__ == "__main__":unittest.main()

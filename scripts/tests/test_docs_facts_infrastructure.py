from __future__ import annotations

import copy
import json
import tempfile
import unittest
from collections.abc import Callable
from datetime import date, datetime, timedelta
from pathlib import Path
from unittest import mock

from scripts import check_docs_facts


class FactSourcePathspecTest(unittest.TestCase):
    def test_source_baseline_covers_runtime_delivery_and_packaging(self) -> None:
        with mock.patch.object(check_docs_facts, "git_output", return_value="a" * 40) as git_output:
            self.assertEqual(check_docs_facts.current_source_baseline(), "a" * 40)
        args = set(git_output.call_args.args)
        for required in (".github/workflows", "build/docker", "scripts/cd", "Makefile", "scripts/oneoff"):
            self.assertIn(required, args)

    def test_double_star_matches_zero_or_more_directories(self) -> None:
        scope = "internal/pkg/grpc/**/*.go"
        self.assertTrue(check_docs_facts.path_matches_scope("internal/pkg/grpc/server.go", scope))
        self.assertTrue(check_docs_facts.path_matches_scope("internal/pkg/grpc/nested/server.go", scope))
        self.assertFalse(check_docs_facts.path_matches_scope("internal/pkg/http/server.go", scope))

    def test_fact_source_existence_uses_the_same_double_star_semantics(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "internal/pkg/grpc").mkdir(parents=True)
            direct = root / "internal/pkg/grpc/server.go"
            direct.write_text("package grpc\n", encoding="utf-8")
            with mock.patch.object(check_docs_facts, "ROOT", root):
                matches = check_docs_facts.manifest_source_matches("internal/pkg/grpc/**/*.go")
            self.assertEqual(matches, [direct])

    def test_tmp_never_satisfies_existence_or_freshness(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "tmp/facts").mkdir(parents=True)
            (root / "tmp/facts/source.go").write_text("package facts\n", encoding="utf-8")
            with mock.patch.object(check_docs_facts, "ROOT", root):
                self.assertEqual(check_docs_facts.manifest_source_matches("tmp/**/*.go"), [])
            self.assertFalse(check_docs_facts.path_matches_scope("tmp/facts/source.go", "tmp/**/*.go"))

    def test_generated_config_path_is_bound_to_its_generator(self) -> None:
        self.assertTrue(check_docs_facts.source_path_exists("configs/env/config.prod.env"))

    def test_freshness_includes_dirty_and_untracked_files_but_excludes_tmp(self) -> None:
        outputs = [
            "internal/committed.go\n",
            "configs/dirty.yaml\ntmp/noise.txt\n",
            "api/staged.proto\n",
            "scripts/new.sh\ntmp/untracked.txt\n",
        ]
        with mock.patch.object(check_docs_facts, "git_output", side_effect=outputs):
            changed = check_docs_facts.changed_paths_since("0" * 40, {})
        self.assertEqual(
            changed,
            {"internal/committed.go", "configs/dirty.yaml", "api/staged.proto", "scripts/new.sh"},
        )


class StrictEvidenceTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.head = check_docs_facts.current_git_head()
        cls.today = date.today().isoformat()

    def selector_evidence(self) -> dict[str, object]:
        return {
            "kind": "source_selector",
            "path": "scripts/check_docs_facts.py",
            "selector": "def main() -> int:",
            "result": "matched",
            "source_sha": self.head,
            "verified_on": self.today,
        }

    def command_evidence(self) -> dict[str, object]:
        return {
            "kind": "command",
            "command": "make docs-check",
            "result": "passed",
            "source_sha": self.head,
            "verified_on": self.today,
        }

    def test_source_selector_and_known_make_target_are_verifiable(self) -> None:
        self.assertEqual(check_docs_facts.validate_evidence_item(self.selector_evidence(), "selector", self.head), [])
        command = {
            "kind": "command",
            "command": "make docs-check",
            "result": "passed",
            "source_sha": self.head,
            "verified_on": self.today,
        }
        self.assertEqual(check_docs_facts.validate_evidence_item(command, "command", self.head), [])

    def test_arbitrary_text_wrong_result_and_extra_fields_fail(self) -> None:
        arbitrary = {
            "kind": "semantic_audit",
            "ref": "reviewed by somebody",
            "result": "passed",
            "source_sha": self.head,
            "verified_on": self.today,
        }
        self.assertIn(
            "closure-evidence-kind",
            {issue.kind for issue in check_docs_facts.validate_evidence_item(arbitrary, "arbitrary", self.head)},
        )
        wrong = self.selector_evidence()
        wrong["result"] = "passed"
        wrong["note"] = "trust me"
        kinds = {issue.kind for issue in check_docs_facts.validate_evidence_item(wrong, "wrong", self.head)}
        self.assertEqual(kinds, {"closure-evidence-schema"})

    def test_unverifiable_command_and_missing_selector_fail(self) -> None:
        self.assertFalse(check_docs_facts.command_is_verifiable("echo passed"))
        self.assertTrue(check_docs_facts.command_is_verifiable("bash scripts/cd/test-wait-worker-readiness.sh"))
        evidence = self.selector_evidence()
        evidence["selector"] = "this selector does not exist"
        self.assertIn(
            "closure-evidence-selector",
            {issue.kind for issue in check_docs_facts.validate_evidence_item(evidence, "selector", self.head)},
        )

    def test_infrastructure_go_test_evidence_must_be_full_and_non_cached(self) -> None:
        self.assertTrue(
            check_docs_facts.command_is_non_cached_test(
                "go test -count=1 ./internal/pkg/grpc"
            )
        )
        for command in (
            "go test ./internal/pkg/grpc",
            "go test -count=0 ./internal/pkg/grpc",
            "go test -count=2 ./internal/pkg/grpc",
            "go test -count=1 -count=1 ./internal/pkg/grpc",
            "go test -count 1 ./internal/pkg/grpc",
            "go test -run=NoSuchTest -count=1 ./internal/pkg/grpc",
            "go test -run NoSuchTest -count=1 ./internal/pkg/grpc",
            "go test -count=1 -list=. ./internal/pkg/grpc",
            "go test -count=1 -list . ./internal/pkg/grpc",
            "go test -count=1 -skip=.* ./internal/pkg/grpc",
            "go test -count=1 -skip .* ./internal/pkg/grpc",
            "go test -count=1 -exec=/bin/true ./internal/pkg/grpc",
            "go test -count=1 -exec /bin/true ./internal/pkg/grpc",
            "go test -count=1 -c ./internal/pkg/grpc",
            "go test -count=1 ./internal/pkg/grpc -args -test.run=NoSuchTest",
            "go test -count=1 ./internal/pkg/grpc -- -test.run=.*",
        ):
            with self.subTest(command=command):
                self.assertFalse(check_docs_facts.command_is_non_cached_test(command))


class InfrastructureParserTest(unittest.TestCase):
    @staticmethod
    def issues_after_mutation(
        path: Path,
        old: str,
        new: str,
        validator: Callable[[], list[check_docs_facts.Issue]],
    ) -> list[check_docs_facts.Issue]:
        original_read_text = Path.read_text
        original = original_read_text(path, encoding="utf-8")
        if old not in original:
            raise AssertionError(f"mutation selector not found in {path}: {old!r}")

        def mutated_read_text(current: Path, *args: object, **kwargs: object) -> str:
            text = original_read_text(current, *args, **kwargs)
            return text.replace(old, new, 1) if current == path else text

        with mock.patch.object(Path, "read_text", mutated_read_text):
            return validator()

    def test_yaml_duplicate_key_is_not_silently_overwritten(self) -> None:
        text = "root:\n  enabled: true\n  enabled: false\n"
        values, duplicates = check_docs_facts.yaml_scalar_inventory(text)
        self.assertEqual(values[("root", "enabled")], "false")
        self.assertEqual(duplicates, {("root", "enabled")})

    def test_yaml_sequence_items_do_not_create_false_duplicates(self) -> None:
        _, duplicates = check_docs_facts.yaml_scalar_inventory(
            check_docs_facts.GRPC_ACL_CONFIG.read_text(encoding="utf-8")
        )
        self.assertEqual(duplicates, set())

    def test_event_and_signal_contracts_have_no_ambiguous_yaml_keys(self) -> None:
        for path in (check_docs_facts.EVENTS, check_docs_facts.SIGNALS):
            self.assertEqual(
                check_docs_facts.yaml_duplicate_issues(path, path.read_text(encoding="utf-8")),
                [],
            )

    def test_scheduler_inventory_is_exact_and_ordered(self) -> None:
        source = check_docs_facts.SCHEDULER_BOOTSTRAP.read_text(encoding="utf-8")
        self.assertEqual(
            check_docs_facts.scheduler_registrations(source),
            list(check_docs_facts.EXPECTED_SCHEDULER_REGISTRATIONS),
        )
        mutated = source.replace("NewMongoConsistencyAuditRunner", "NewUnexpectedRunner", 1)
        self.assertNotEqual(
            check_docs_facts.scheduler_registrations(mutated),
            list(check_docs_facts.EXPECTED_SCHEDULER_REGISTRATIONS),
        )

    def test_locklease_inventory_includes_auto_renewal(self) -> None:
        source = check_docs_facts.LOCKLEASE_CATALOG_SOURCE.read_text(encoding="utf-8")
        inventory = check_docs_facts.locklease_inventory(source)
        self.assertEqual(inventory, list(check_docs_facts.EXPECTED_LOCKLEASE_INVENTORY))
        self.assertEqual({entry[4] for entry in inventory}, {"auto"})
        mutated = source.replace('RenewalMode = "auto"', 'RenewalMode = "manual"', 1)
        self.assertNotEqual(
            check_docs_facts.locklease_inventory(mutated),
            list(check_docs_facts.EXPECTED_LOCKLEASE_INVENTORY),
        )

    def test_signal_topology_is_exact(self) -> None:
        source = check_docs_facts.SIGNALS.read_text(encoding="utf-8")
        self.assertEqual(check_docs_facts.signal_topology(source), check_docs_facts.EXPECTED_SIGNAL_TOPOLOGY)
        mutated = source.replace("publisher: apiserver, worker", "publisher: apiserver", 1)
        self.assertNotEqual(check_docs_facts.signal_topology(mutated), check_docs_facts.EXPECTED_SIGNAL_TOPOLOGY)

    def test_process_stage_inventory_is_exact(self) -> None:
        for process, (runner, _) in check_docs_facts.PROCESS_SOURCES.items():
            self.assertEqual(
                check_docs_facts.process_stage_names(runner.read_text(encoding="utf-8")),
                check_docs_facts.EXPECTED_PROCESS_STAGES[process],
            )

    def test_acl_and_worker_log_mutations_are_rejected(self) -> None:
        acl_issues = self.issues_after_mutation(
            check_docs_facts.PRODUCTION_CONFIG,
            "  acl:\n    enabled: true",
            "  acl:\n    enabled: false",
            check_docs_facts.acl_contract_issues,
        )
        self.assertIn("grpc-acl-production-config-drift", {issue.kind for issue in acl_issues})

        log_issues = self.issues_after_mutation(
            check_docs_facts.WORKER_PRODUCTION_CONFIG,
            "  level: warn",
            "  level: debug",
            check_docs_facts.worker_log_contract_issues,
        )
        self.assertIn("worker-production-log-source-drift", {issue.kind for issue in log_issues})

    def test_stage_and_shutdown_mutations_are_rejected(self) -> None:
        stage_issues = self.issues_after_mutation(
            check_docs_facts.PROCESS_SOURCES["apiserver"][0],
            'Name() string { return "initialize transports" }',
            'Name() string { return "initialize transport" }',
            check_docs_facts.process_lifecycle_contract_issues,
        )
        self.assertIn("process-stage-source-drift", {issue.kind for issue in stage_issues})

        shutdown_issues = self.issues_after_mutation(
            check_docs_facts.PROCESS_SOURCES["apiserver"][1],
            "if deps.transport.closeGRPC != nil {\n\t\tdeps.transport.closeGRPC()\n\t}",
            "if deps.transport.terminateGRPC != nil {\n\t\tdeps.transport.terminateGRPC()\n\t}",
            check_docs_facts.process_lifecycle_contract_issues,
        )
        self.assertIn("process-shutdown-source-drift", {issue.kind for issue in shutdown_issues})

    def test_probe_and_deployment_config_mutations_are_rejected(self) -> None:
        probe_issues = self.issues_after_mutation(
            check_docs_facts.COLLECTION_HEALTH_SOURCE,
            "serveReady := controlSynchronized",
            "serveReady := false",
            check_docs_facts.probe_contract_issues,
        )
        self.assertIn("probe-source-contract-drift", {issue.kind for issue in probe_issues})

        compose_issues = self.issues_after_mutation(
            check_docs_facts.PRODUCTION_COMPOSE,
            "/opt/qs-server/qs-worker/configs/env/config.prod.env",
            "/tmp/missing-worker-env",
            check_docs_facts.docker_config_contract_issues,
        )
        self.assertIn("docker-required-config-mount-drift", {issue.kind for issue in compose_issues})

    def test_9091_remains_an_unused_compatibility_field(self) -> None:
        self.assertEqual(check_docs_facts.grpc_healthz_port_source_issues(), [])
        correct = "9091 当前无独立 listener，因此不得作为探针或可达性验收目标。"
        self.assertEqual(check_docs_facts.grpc_healthz_port_doc_issues(correct), [])
        stale = "配置另有 9091 gRPC health；9091 是否对监控可达需目标网络另验。"
        kinds = {issue.kind for issue in check_docs_facts.grpc_healthz_port_doc_issues(stale)}
        self.assertIn("grpc-healthz-port-doc-listener-drift", kinds)
        self.assertIn("grpc-healthz-port-doc-current-listener-claim", kinds)

    def test_infrastructure_go_test_paths_reject_retired_testeeaccess_package(self) -> None:
        correct = "go test -count=1 ./internal/collection-server/application/testeeaccess"
        self.assertEqual(
            check_docs_facts.markdown_go_test_packages(correct),
            ["./internal/collection-server/application/testeeaccess"],
        )
        self.assertTrue(check_docs_facts.source_path_exists(check_docs_facts.markdown_go_test_packages(correct)[0]))

        issues = self.issues_after_mutation(
            check_docs_facts.INFRA_SECURITY_CANONICAL_DOC,
            "./internal/collection-server/application/testeeaccess",
            "./internal/pkg/testeeaccess",
            check_docs_facts.infrastructure_non_cached_test_path_issues,
        )
        canonical = check_docs_facts.INFRA_SECURITY_CANONICAL_DOC.relative_to(check_docs_facts.ROOT).as_posix()
        details = {(issue.kind, issue.detail) for issue in issues}
        self.assertIn(
            ("infra-security-retired-test-package", f"{canonical}: ./internal/pkg/testeeaccess"),
            details,
        )
        self.assertTrue(
            any(issue.kind == "infra-non-cached-test-package-missing" and canonical in issue.detail for issue in issues)
        )

    def test_all_current_infrastructure_contracts_pass(self) -> None:
        self.assertEqual(check_docs_facts.infrastructure_contract_issues(), [])

    def test_oneoff_cleanup_must_remain_blocked_and_audit_only(self) -> None:
        text = check_docs_facts.ONEOFF_README.read_text(encoding="utf-8")
        self.assertEqual(check_docs_facts.oneoff_safety_contract_issues(text), [])
        mutated = text.replace("**blocked / audit-only**", "ready for apply", 1)
        self.assertIn(
            "oneoff-orphan-cleanup-safety-doc-drift",
            {issue.kind for issue in check_docs_facts.oneoff_safety_contract_issues(mutated)},
        )

        residual_issues = self.issues_after_mutation(
            check_docs_facts.INFRA_MIGRATION_RECOVERY_DOC,
            "不是二进制 fail-closed",
            "二进制已经 fail-closed",
            check_docs_facts.oneoff_safety_contract_issues,
        )
        self.assertIn(
            "oneoff-orphan-cleanup-residual-capability-undisclosed",
            {issue.kind for issue in residual_issues},
        )


class InfrastructureSignoffTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.head = check_docs_facts.current_git_head()
        cls.today = date.today().isoformat()
        cls.primary_paths = set(check_docs_facts.INFRASTRUCTURE_TOPICS.values())

    def selector_evidence(self) -> dict[str, object]:
        return {
            "kind": "source_selector",
            "path": "scripts/check_docs_facts.py",
            "selector": "def main() -> int:",
            "result": "matched",
            "source_sha": self.head,
            "verified_on": self.today,
        }

    def command_evidence(self) -> dict[str, object]:
        return {
            "kind": "command",
            "command": "make docs-check",
            "result": "passed",
            "source_sha": self.head,
            "verified_on": self.today,
        }

    def current_production_entry(self, topic: str) -> dict[str, object]:
        observed_at = datetime.now().astimezone() - timedelta(days=1)
        return {
            "status": "current",
            "topics": [topic],
            "result": {"status": "passed", "measurements": {"readyz_status": 200}},
            "observed_at": observed_at.isoformat(),
            "expires_on": (observed_at.date() + timedelta(days=30)).isoformat(),
            "deployed_sha": self.head,
            "source_baseline_sha": self.head,
            "effective_config_hash": "sha256:" + "1" * 64,
            "effective_config_hash_limitation": None,
            "evidence": [
                {
                    "kind": "github_run",
                    "ref": "https://github.com/FangcunMount/qs-server/actions/runs/123456789",
                }
            ],
        }

    @staticmethod
    def unsigned_dimension() -> dict[str, object]:
        return {
            "review_state": "conditional",
            "rationale": "repository evidence only",
            "evidence": [],
            "gaps": [],
            "signoff": {"state": "unsigned", "by": None, "at": None, "source_sha": None},
        }

    def valid_manifest(self) -> dict[str, object]:
        topics = []
        for topic, canonical_doc in check_docs_facts.INFRASTRUCTURE_TOPICS.items():
            required_sources = [
                anchors[0]
                for anchors in check_docs_facts.INFRASTRUCTURE_REQUIRED_SOURCE_SCOPES[topic].values()
            ]
            topics.append(
                {
                    "topic": topic,
                    "canonical_doc": canonical_doc,
                    "fact_sources": [
                        {"kind": "source", "path": path}
                        for path in ["scripts/check_docs_facts.py", *required_sources]
                    ],
                    "non_cached_tests": [self.command_evidence()],
                    "environment_tests": [
                        {
                            "kind": "environment_test",
                            "environment": "production",
                            "planned_command": "run topic-specific production check",
                            "status": "not_run",
                            "gap_id": f"{topic}-env-not-run",
                        }
                    ],
                    "production_evidence": ["historical-prod"],
                    "gaps": [
                        {
                            "id": f"{topic}-env-not-run",
                            "kind": "environment_evidence",
                            "severity": "medium",
                            "summary": "environment test has not run",
                            "exit_criteria": "run and retain the planned environment test",
                        }
                    ],
                    "dimensions": {
                        dimension: self.unsigned_dimension()
                        for dimension in check_docs_facts.MODULE_SIGNOFF_DIMENSIONS
                    },
                }
            )
        return {
            "source_baseline": {"sha": self.head, "verified_on": self.today},
            "infrastructure_signoff": {
                "dimensions": check_docs_facts.MODULE_SIGNOFF_DIMENSIONS,
                "topics": topics,
            }
        }

    def validate(
        self,
        manifest: dict[str, object],
        production_entries: dict[str, dict[str, object]] | None = None,
        changed_paths: set[str] | None = None,
    ) -> list[check_docs_facts.Issue]:
        if production_entries is None:
            production_entries = {
                "historical-prod": {
                    "status": "historical",
                    "topics": list(check_docs_facts.INFRASTRUCTURE_TOPICS),
                }
            }
        with mock.patch.object(
            check_docs_facts,
            "validate_infrastructure_production_evidence",
            return_value=(production_entries, []),
        ), mock.patch.object(
            check_docs_facts,
            "changed_paths_since",
            return_value=changed_paths or set(),
        ):
            return check_docs_facts.validate_infrastructure_signoff(manifest, self.primary_paths, self.head)

    def test_honest_unsigned_ten_topic_skeleton_passes(self) -> None:
        self.assertEqual(self.validate(self.valid_manifest()), [])

    def test_missing_topic_and_wrong_canonical_doc_fail(self) -> None:
        manifest = self.valid_manifest()
        topics = manifest["infrastructure_signoff"]["topics"]
        topics.pop()
        self.assertIn("infrastructure-topic-inventory", {issue.kind for issue in self.validate(manifest)})

        manifest = self.valid_manifest()
        first, second = manifest["infrastructure_signoff"]["topics"][:2]
        first["canonical_doc"] = second["canonical_doc"]
        kinds = {issue.kind for issue in self.validate(manifest)}
        self.assertIn("infrastructure-topic-canonical-doc", kinds)
        self.assertIn("infrastructure-topic-canonical-duplicate", kinds)

    def test_arbitrary_dimension_evidence_cannot_self_certify(self) -> None:
        manifest = self.valid_manifest()
        dimension = manifest["infrastructure_signoff"]["topics"][0]["dimensions"]["runtime"]
        dimension["evidence"] = [{"kind": "semantic_audit", "ref": "looks good"}]
        self.assertIn("closure-evidence-kind", {issue.kind for issue in self.validate(manifest)})

    def test_trivial_source_selector_cannot_upgrade_dimension(self) -> None:
        trivial_selectors = (
            ("scripts/check_docs_facts.py", "#!/usr/bin/env python3"),
            ("internal/apiserver/process/runner.go", "package process"),
            ("configs/apiserver.prod.yaml", "mongo_consistency_audit:"),
        )
        for path, selector in trivial_selectors:
            with self.subTest(selector=selector):
                manifest = self.valid_manifest()
                runtime = manifest["infrastructure_signoff"]["topics"][0]["dimensions"]["runtime"]
                runtime["review_state"] = "ready"
                runtime["evidence"] = [
                    {
                        "kind": "source_selector",
                        "path": path,
                        "selector": selector,
                        "result": "matched",
                        "source_sha": self.head,
                        "verified_on": self.today,
                    }
                ]
                kinds = {issue.kind for issue in self.validate(manifest)}
                self.assertIn("infrastructure-dimension-trivial-selector", kinds)

    def test_deleting_any_required_topic_source_scope_fails(self) -> None:
        for topic_id, scopes in check_docs_facts.INFRASTRUCTURE_REQUIRED_SOURCE_SCOPES.items():
            for scope_name, anchors in scopes.items():
                with self.subTest(topic=topic_id, scope=scope_name):
                    manifest = self.valid_manifest()
                    topic = next(
                        item
                        for item in manifest["infrastructure_signoff"]["topics"]
                        if item["topic"] == topic_id
                    )
                    topic["fact_sources"] = [
                        source for source in topic["fact_sources"] if source["path"] not in anchors
                    ]
                    issues = self.validate(manifest)
                    required_scope_issues = [
                        issue
                        for issue in issues
                        if issue.kind == "infrastructure-topic-required-source-scope"
                    ]
                    self.assertTrue(
                        any(issue.detail.startswith(f"{topic_id}.{scope_name}:") for issue in required_scope_issues),
                        required_scope_issues,
                    )

    def test_non_cached_test_mutations_fail_infrastructure_schema(self) -> None:
        for command in (
            "go test ./internal/pkg/grpc",
            "go test -count=0 ./internal/pkg/grpc",
            "go test -count=2 ./internal/pkg/grpc",
            "go test -count=1 -count=1 ./internal/pkg/grpc",
            "go test -count 1 ./internal/pkg/grpc",
            "go test -run=NoSuchTest -count=1 ./internal/pkg/grpc",
            "go test -run NoSuchTest -count=1 ./internal/pkg/grpc",
            "go test -count=1 -list=. ./internal/pkg/grpc",
            "go test -count=1 -list . ./internal/pkg/grpc",
            "go test -count=1 -skip=.* ./internal/pkg/grpc",
            "go test -count=1 -skip .* ./internal/pkg/grpc",
            "go test -count=1 -exec=/bin/true ./internal/pkg/grpc",
            "go test -count=1 -exec /bin/true ./internal/pkg/grpc",
            "go test -count=1 -c ./internal/pkg/grpc",
            "go test -count=1 ./internal/pkg/grpc -args -test.run=NoSuchTest",
            "go test -count=1 ./internal/pkg/grpc -- -test.run=.*",
        ):
            with self.subTest(command=command):
                manifest = self.valid_manifest()
                evidence = self.command_evidence()
                evidence["command"] = command
                manifest["infrastructure_signoff"]["topics"][0]["non_cached_tests"] = [evidence]
                kinds = {issue.kind for issue in self.validate(manifest)}
                self.assertIn("infrastructure-topic-non-cached-test-command", kinds)

    def test_operations_and_production_cannot_upgrade_without_real_evidence(self) -> None:
        manifest = self.valid_manifest()
        topic = manifest["infrastructure_signoff"]["topics"][0]
        for dimension_name in ("operations", "production"):
            dimension = topic["dimensions"][dimension_name]
            dimension["review_state"] = "ready"
            dimension["evidence"] = [self.selector_evidence()]
        kinds = {issue.kind for issue in self.validate(manifest)}
        self.assertIn("infrastructure-operations-upgrade-without-environment-test", kinds)
        self.assertIn("infrastructure-production-upgrade-without-current-evidence", kinds)

    def test_environment_not_run_must_reference_a_topic_gap(self) -> None:
        manifest = self.valid_manifest()
        environment_test = manifest["infrastructure_signoff"]["topics"][0]["environment_tests"][0]
        environment_test["gap_id"] = "missing-gap"
        self.assertIn("infrastructure-environment-test-gap", {issue.kind for issue in self.validate(manifest)})

    def test_passed_environment_test_can_support_operations_upgrade(self) -> None:
        manifest = self.valid_manifest()
        topic = manifest["infrastructure_signoff"]["topics"][0]
        topic["environment_tests"] = [
            {
                "kind": "environment_test",
                "environment": "staging",
                "command": "curl https://staging.example.test/readyz",
                "status": "passed",
                "result": "passed",
                "source_sha": self.head,
                "verified_on": self.today,
                "evidence": [{"kind": "repository_record", "ref": "docs/README.md"}],
            }
        ]
        operations = topic["dimensions"]["operations"]
        operations["review_state"] = "ready"
        operations["evidence"] = [self.selector_evidence()]
        self.assertEqual(self.validate(manifest), [])

    def test_passed_environment_test_requires_structured_reference_evidence(self) -> None:
        manifest = self.valid_manifest()
        topic = manifest["infrastructure_signoff"]["topics"][0]
        topic["environment_tests"] = [
            {
                "kind": "environment_test",
                "environment": "staging",
                "command": "curl https://staging.example.test/readyz",
                "status": "passed",
                "result": "passed",
                "source_sha": self.head,
                "verified_on": self.today,
            }
        ]
        kinds = {issue.kind for issue in self.validate(manifest)}
        self.assertIn("infrastructure-environment-test-schema", kinds)

        topic["environment_tests"][0]["evidence"] = [{"kind": "note", "ref": "trust me"}]
        kinds = {issue.kind for issue in self.validate(manifest)}
        self.assertIn("infrastructure-environment-test-evidence", kinds)

    def test_ready_dimension_rejects_fact_sources_newer_than_evidence(self) -> None:
        manifest = self.valid_manifest()
        dimension = manifest["infrastructure_signoff"]["topics"][0]["dimensions"]["runtime"]
        dimension["review_state"] = "ready"
        dimension["evidence"] = [self.selector_evidence()]
        kinds = {
            issue.kind
            for issue in self.validate(manifest, changed_paths={"scripts/check_docs_facts.py"})
        }
        self.assertIn("infrastructure-topic-fact-source-stale", kinds)

    def test_empty_test_and_unexplained_production_arrays_do_not_satisfy_topic_schema(self) -> None:
        manifest = self.valid_manifest()
        topic = manifest["infrastructure_signoff"]["topics"][0]
        topic["non_cached_tests"] = []
        topic["production_evidence"] = []
        kinds = {issue.kind for issue in self.validate(manifest)}
        self.assertIn("infrastructure-topic-non-cached-tests", kinds)
        self.assertIn("infrastructure-production-empty-evidence-without-gap", kinds)

    def test_empty_production_evidence_is_honest_only_with_unsigned_production_gap(self) -> None:
        manifest = self.valid_manifest()
        topic = manifest["infrastructure_signoff"]["topics"][0]
        topic["production_evidence"] = []
        production = topic["dimensions"]["production"]
        production["review_state"] = "blocked"
        production["gaps"] = [
            {
                "id": "no-production-evidence",
                "kind": "production",
                "severity": "high",
                "summary": "no relevant production record exists",
                "exit_criteria": "run exact-SHA production verification",
            }
        ]
        self.assertEqual(self.validate(manifest), [])

    def test_topic_cannot_borrow_unrelated_production_evidence(self) -> None:
        manifest = self.valid_manifest()
        first_topic = manifest["infrastructure_signoff"]["topics"][0]["topic"]
        production_entries = {
            "historical-prod": {
                "status": "historical",
                "topics": [next(topic for topic in check_docs_facts.INFRASTRUCTURE_TOPICS if topic != first_topic)],
            }
        }
        kinds = {issue.kind for issue in self.validate(manifest, production_entries)}
        self.assertIn("infrastructure-topic-production-evidence-unrelated", kinds)

    def test_production_can_upgrade_only_with_current_structured_evidence(self) -> None:
        manifest = self.valid_manifest()
        topic = manifest["infrastructure_signoff"]["topics"][0]
        topic["production_evidence"] = ["current-prod"]
        dimension = topic["dimensions"]["production"]
        dimension["review_state"] = "ready"
        dimension["evidence"] = [self.selector_evidence()]
        dimension["signoff"] = {
            "state": "signed",
            "by": "infra",
            "at": self.today,
            "source_sha": self.head,
        }
        production_entries = {
            "historical-prod": {
                "status": "historical",
                "topics": list(check_docs_facts.INFRASTRUCTURE_TOPICS),
            },
            "current-prod": self.current_production_entry(topic["topic"]),
        }
        self.assertEqual(self.validate(manifest, production_entries), [])

    def test_production_upgrade_rejects_current_evidence_shortcuts(self) -> None:
        def placeholder_hash(entry: dict[str, object]) -> None:
            entry["effective_config_hash"] = "sha256:" + "0" * 64

        def empty_measurements(entry: dict[str, object]) -> None:
            entry["result"]["measurements"] = {}

        def repository_only(entry: dict[str, object]) -> None:
            entry["evidence"] = [{"kind": "repository_record", "ref": "scripts/check_docs_facts.py"}]

        def future_observation(entry: dict[str, object]) -> None:
            entry["observed_at"] = (datetime.now().astimezone() + timedelta(days=1)).isoformat()

        for name, mutate in (
            ("placeholder_hash", placeholder_hash),
            ("empty_measurements", empty_measurements),
            ("repository_only", repository_only),
            ("future_observation", future_observation),
        ):
            with self.subTest(case=name):
                manifest = self.valid_manifest()
                topic = manifest["infrastructure_signoff"]["topics"][0]
                topic["production_evidence"] = ["current-prod"]
                production = topic["dimensions"]["production"]
                production["review_state"] = "ready"
                production["evidence"] = [self.selector_evidence()]
                current_entry = self.current_production_entry(topic["topic"])
                mutate(current_entry)
                kinds = {
                    issue.kind
                    for issue in self.validate(manifest, {"current-prod": current_entry})
                }
                self.assertIn("infrastructure-production-upgrade-without-current-evidence", kinds)

    def test_production_upgrade_rejects_entry_from_different_source_baseline(self) -> None:
        manifest = self.valid_manifest()
        previous = check_docs_facts.git_output("rev-parse", "HEAD^")
        manifest["source_baseline"]["sha"] = previous
        topic = manifest["infrastructure_signoff"]["topics"][0]
        topic["production_evidence"] = ["current-prod"]
        production = topic["dimensions"]["production"]
        production["review_state"] = "ready"
        production["evidence"] = [self.selector_evidence()]
        production_entries = {
            "historical-prod": {
                "status": "historical",
                "topics": list(check_docs_facts.INFRASTRUCTURE_TOPICS),
            },
            "current-prod": self.current_production_entry(topic["topic"]),
        }
        kinds = {issue.kind for issue in self.validate(manifest, production_entries)}
        self.assertIn("infrastructure-production-upgrade-without-current-evidence", kinds)

    def test_unsigned_signoff_rejects_claim_metadata(self) -> None:
        manifest = self.valid_manifest()
        signoff = manifest["infrastructure_signoff"]["topics"][0]["dimensions"]["runtime"]["signoff"]
        signoff["by"] = "reviewer"
        self.assertIn("infrastructure-dimension-unsigned-metadata", {issue.kind for issue in self.validate(manifest)})


class InfrastructureProductionLedgerTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.head = check_docs_facts.current_git_head()

    def valid_ledger(self) -> dict[str, object]:
        observed_at = datetime.now().astimezone() - timedelta(days=1)
        return {
            "schema_version": 1,
            "policy": {
                "current_evidence_requires_exact_deployed_sha": True,
                "current_evidence_max_validity_days": 30,
                "expires_on_scope": "current-state eligibility, not record retention",
                "effective_config_unknown_codes": ["unknown_not_recorded"],
            },
            "entries": [
                {
                    "id": "prod-current-1",
                    "status": "current",
                    "observed_at": observed_at.isoformat(),
                    "environment": "production",
                    "deployed_sha": self.head,
                    "source_baseline_sha": self.head,
                    "effective_config_hash": "sha256:" + "1" * 64,
                    "effective_config_hash_limitation": None,
                    "command": "recorded production probe",
                    "result": {
                        "status": "passed",
                        "summary": "probe passed",
                        "measurements": {"readyz_status": 200},
                    },
                    "owner": "infra",
                    "topics": ["runtime_lifecycle"],
                    "expires_on": (observed_at.date() + timedelta(days=30)).isoformat(),
                    "supersedes": [],
                    "evidence": [
                        {
                            "kind": "github_run",
                            "ref": "https://github.com/FangcunMount/qs-server/actions/runs/123456789",
                        }
                    ],
                    "limitations": ["point-in-time observation"],
                }
            ],
        }

    def validate(self, ledger: dict[str, object]) -> tuple[dict[str, dict[str, object]], list[check_docs_facts.Issue]]:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "production.json"
            path.write_text(json.dumps(ledger), encoding="utf-8")
            with mock.patch.object(check_docs_facts, "INFRA_PRODUCTION_EVIDENCE", path):
                return check_docs_facts.validate_infrastructure_production_evidence(self.head)

    def test_valid_structured_current_evidence_passes(self) -> None:
        entries, issues = self.validate(self.valid_ledger())
        self.assertEqual(issues, [])
        self.assertEqual(set(entries), {"prod-current-1"})

    def test_effective_config_xor_and_current_expiry_are_enforced(self) -> None:
        ledger = self.valid_ledger()
        entry = ledger["entries"][0]
        entry["effective_config_hash_limitation"] = {
            "code": "unknown_not_recorded",
            "detail": "hash was not retained",
        }
        entry["expires_on"] = "2000-01-01"
        kinds = {issue.kind for issue in self.validate(ledger)[1]}
        self.assertIn("infrastructure-production-effective-config", kinds)
        self.assertIn("infrastructure-production-entry-expired-current", kinds)

    def test_current_evidence_cannot_use_unknown_effective_config(self) -> None:
        ledger = self.valid_ledger()
        entry = ledger["entries"][0]
        entry["effective_config_hash"] = None
        entry["effective_config_hash_limitation"] = {
            "code": "unknown_not_recorded",
            "detail": "hash was not retained",
        }
        kinds = {issue.kind for issue in self.validate(ledger)[1]}
        self.assertIn("infrastructure-production-entry-current-effective-config", kinds)

    def test_current_evidence_rejects_placeholder_hash_and_empty_measurements(self) -> None:
        ledger = self.valid_ledger()
        entry = ledger["entries"][0]
        entry["effective_config_hash"] = "sha256:" + "0" * 64
        entry["result"]["measurements"] = {}
        kinds = {issue.kind for issue in self.validate(ledger)[1]}
        self.assertIn("infrastructure-production-effective-config", kinds)
        self.assertIn("infrastructure-production-entry-current-measurements", kinds)

    def test_current_evidence_rejects_future_observation_and_overlong_validity(self) -> None:
        ledger = self.valid_ledger()
        entry = ledger["entries"][0]
        future = datetime.now().astimezone() + timedelta(days=1)
        entry["observed_at"] = future.isoformat()
        entry["expires_on"] = (future.date() + timedelta(days=31)).isoformat()
        kinds = {issue.kind for issue in self.validate(ledger)[1]}
        self.assertIn("infrastructure-production-entry-observed-at-future", kinds)
        self.assertIn("infrastructure-production-entry-validity-window", kinds)

    def test_current_evidence_requires_immutable_github_run(self) -> None:
        ledger = self.valid_ledger()
        ledger["entries"][0]["evidence"] = [
            {"kind": "repository_record", "ref": "scripts/check_docs_facts.py"}
        ]
        kinds = {issue.kind for issue in self.validate(ledger)[1]}
        self.assertIn("infrastructure-production-entry-current-immutable-evidence", kinds)

    def test_repository_record_rejects_missing_anchor_and_archive(self) -> None:
        valid_ref = "docs/00-总览/09-当前版本定档验收台账.md#6-mongo-consistency-audit-当前真值"
        self.assertTrue(check_docs_facts.production_evidence_ref_valid("repository_record", valid_ref))
        self.assertFalse(
            check_docs_facts.production_evidence_ref_valid(
                "repository_record",
                "docs/00-总览/09-当前版本定档验收台账.md#definitely-not-an-anchor",
            )
        )
        self.assertFalse(
            check_docs_facts.production_evidence_ref_valid(
                "repository_record",
                "docs/_archive/README.md",
            )
        )

    def test_ledger_topics_must_be_nonempty_known_and_unique(self) -> None:
        ledger = self.valid_ledger()
        ledger["entries"][0]["topics"] = ["runtime_lifecycle", "runtime_lifecycle", "unknown"]
        kinds = {issue.kind for issue in self.validate(ledger)[1]}
        self.assertIn("infrastructure-production-entry-topics", kinds)

    def test_current_evidence_requires_exact_sha_and_valid_reference(self) -> None:
        ledger = self.valid_ledger()
        entry = ledger["entries"][0]
        entry["deployed_sha"] = "unknown"
        entry["evidence"] = [{"kind": "github_run", "ref": "https://example.test/actions/runs/1"}]
        kinds = {issue.kind for issue in self.validate(ledger)[1]}
        self.assertIn("infrastructure-production-entry-sha", kinds)
        self.assertIn("infrastructure-production-entry-evidence", kinds)

    def test_current_evidence_must_bind_exact_checkout_sha(self) -> None:
        ledger = self.valid_ledger()
        previous = check_docs_facts.git_output("rev-parse", "HEAD^")
        entry = ledger["entries"][0]
        entry["deployed_sha"] = previous
        entry["source_baseline_sha"] = previous
        kinds = {issue.kind for issue in self.validate(ledger)[1]}
        self.assertIn("infrastructure-production-entry-current-checkout-binding", kinds)


if __name__ == "__main__":
    unittest.main()

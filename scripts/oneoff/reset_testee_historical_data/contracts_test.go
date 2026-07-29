package resettesteedata

import (
	"os"
	"strings"
	"testing"
)

func TestQSMySQLResetContract(t *testing.T) {
	script := readArtifact(t, "01-reset-testee-facts-qs-mysql.sql")

	for _, token := range []string{
		"@qs_reset_confirm",
		"@qs_expected_database",
		"@qs_expected_testee_count",
		"@qs_expected_profile_count",
		"@qs_reset_resume",
		"_oneoff_delete_batches",
		"GET_LOCK",
		"RELEASE_LOCK",
		"answersheet.submitted",
		"evaluation.requested",
		"interpretation.report.generated",
		"task.completed",
	} {
		requireContains(t, script, token)
	}

	for _, table := range []string{
		"assessment_entry_resolve_log",
		"assessment_entry_intake_log",
		"clinician_relation",
		"assessment_task",
		"plan_enrollment",
		"assessment_score",
		"evaluation_outcome",
		"assessment",
		"testee",
		"statistics_access_fact",
		"statistics_assessment_fact",
		"statistics_plan_fact",
		"statistics_access_daily",
		"statistics_assessment_daily",
		"statistics_plan_activity_daily",
		"statistics_plan_fulfillment_daily",
		"statistics_org_snapshot",
		"statistics_sync_run",
		"runtime_checkpoint",
		"retry_event_hold",
		"event_delivery_dead_letter",
		"domain_event_outbox",
		"seed_backfill_stage_attempt",
		"seed_backfill_stage",
		"seed_backfill_rollback_phase_attempt",
		"seed_backfill_rollback_resource",
		"seed_backfill_rollback_operation",
	} {
		requireContains(t, script, "`"+table+"`")
	}

	for _, protected := range []string{"staff", "clinician", "assessment_entry", "assessment_plan", "system_governance_action_runs"} {
		assertNoDestructiveTableCall(t, script, protected)
	}
	if strings.Contains(strings.ToUpper(script), "FOREIGN_KEY_CHECKS") {
		t.Fatal("QS reset must not disable FOREIGN_KEY_CHECKS")
	}
	for _, contentEvent := range []string{"questionnaire.changed", "assessment_model.changed"} {
		if strings.Contains(script, contentEvent) {
			t.Fatalf("QS reset must preserve content event type %q", contentEvent)
		}
	}
}

func TestQSMongoResetContract(t *testing.T) {
	script := readArtifact(t, "02-reset-testee-facts-qs-mongo.js")

	for _, token := range []string{
		"QS_RESET_CONFIRM",
		"QS_RESET_EXPECTED_DATABASE",
		"QS_RESET_EXPECTED_ANSWERSHEET_COUNT",
		"QS_RESET_RESUME",
		"deleteMany",
		"answersheets",
		"answersheet_submit_idempotency",
		"report_generations",
		"interpretation_runs",
		"interpret_report_artifacts",
		"report_query_catalog",
		"archived_reports",
		"interpretation_admission_failures",
		"interpretation_attention_projections",
		"interpretation_catalog_repair_plans",
		"interpret_reports",
		"domain_event_outbox",
		"answersheet.submitted",
		"evaluation.requested",
		"interpretation.report.generated",
		"task.completed",
	} {
		requireContains(t, script, token)
	}

	for _, forbidden := range []string{"dropDatabase", ".drop(", "dropCollection"} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("Mongo reset contains forbidden destructive operation %q", forbidden)
		}
	}
	for _, contentEvent := range []string{"questionnaire.changed", "assessment_model.changed"} {
		if strings.Contains(script, contentEvent) {
			t.Fatalf("Mongo reset must preserve content event type %q", contentEvent)
		}
	}
	for _, protected := range []string{
		"questionnaires", "scales", "assessment_models", "published_assessment_models",
		"assessment_norms", "evaluation_rule_sets", "interpretation_report_templates", "schema_migrations",
	} {
		if strings.Contains(script, `deleteCollectionInBatches("`+protected+`"`) {
			t.Fatalf("Mongo reset deletes protected collection %q", protected)
		}
	}
}

func TestIAMMySQLResetContract(t *testing.T) {
	script := readArtifact(t, "03-reset-testee-profiles-iam-mysql.sql")

	for _, token := range []string{
		"@iam_reset_confirm",
		"@iam_expected_database",
		"@iam_expected_profile_count",
		"@iam_expected_healthcare_user_count",
		"@iam_reset_resume",
		"LOAD DATA LOCAL INFILE",
		"tmp_reset_testee_profile_ids",
		"tmp_reset_healthcare_user_ids",
		"tmp_reset_healthcare_profile_conflicts",
		"profile_links",
		"profiles",
		"guardianships",
		"children",
	} {
		requireContains(t, script, token)
	}

	for _, protected := range []string{
		"users", "auth_login_identities", "auth_credentials", "auth_token_audit",
		"domain_event_outbox", "identity_session_revocation_outbox",
	} {
		assertNoDestructiveTableCall(t, script, protected)
	}
	if strings.Contains(strings.ToUpper(script), "FOREIGN_KEY_CHECKS") {
		t.Fatal("IAM reset must not disable FOREIGN_KEY_CHECKS")
	}
}

func TestRunbookKeepsExportAndExecutionOrderExplicit(t *testing.T) {
	runbook := readArtifact(t, "README.md")
	for _, token := range []string{
		"iam-profile-ids.tsv",
		"iam-healthcare-user-ids.tsv",
		"01-reset-testee-facts-qs-mysql.sql",
		"02-reset-testee-facts-qs-mongo.js",
		"03-reset-testee-profiles-iam-mysql.sql",
		"--local-infile=1",
		"不要启动任何服务",
		"不适用于按 Org",
	} {
		requireContains(t, runbook, token)
	}
}

func readArtifact(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

func requireContains(t *testing.T, text, token string) {
	t.Helper()
	if !strings.Contains(text, token) {
		t.Fatalf("artifact is missing %q", token)
	}
}

func assertNoDestructiveTableCall(t *testing.T, script, table string) {
	t.Helper()
	lower := strings.ToLower(script)
	for _, token := range []string{
		"delete from `" + table + "`",
		"truncate table `" + table + "`",
		"update `" + table + "`",
		"_oneoff_delete_batches('" + table + "'",
		"_oneoff_delete_batches(\"" + table + "\"",
	} {
		if strings.Contains(lower, token) {
			t.Fatalf("script destructively targets protected table %q via %q", table, token)
		}
	}
}

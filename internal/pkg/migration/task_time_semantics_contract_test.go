package migration

import (
	"strings"
	"testing"
)

func TestTaskTimeSemanticsMigrationIsAdditiveAndIndexed(t *testing.T) {
	up := readMySQLMigration(t, "000065_add_task_time_semantics_and_assessment_summary_indexes.up.sql")
	for _, required := range []string{
		"`due_at` DATETIME(3) NULL",
		"`expiration_reason` VARCHAR(32) NULL",
		"idx_task_org_entry_expire_scan",
		"idx_assessment_org_testee_evaluated_summary",
		"`org_id`, `testee_id`, `status`, `deleted_at`, `evaluated_at`, `id`",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
	if strings.Contains(strings.ToUpper(up), "UPDATE `ASSESSMENT_TASK`") {
		t.Fatal("schema migration must not rewrite the large task table")
	}
}

func TestTaskScheduleRevisionMigrationIsAdditiveAndDoesNotRewriteFacts(t *testing.T) {
	up := readMySQLMigration(t, "000066_add_task_schedule_revision_and_statistics_facts.up.sql")
	for _, required := range []string{
		"`schedule_revision` INT UNSIGNED NOT NULL DEFAULT 1",
		"`schedule_defined_at` DATETIME(3) NULL",
		"idx_task_collect_schedule_defined",
		"`schedule_revision` INT UNSIGNED NULL",
		"`schedule_planned_at` DATETIME(3) NULL",
		"`schedule_due_at` DATETIME(3) NULL",
		"idx_statistics_plan_fact_schedule",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
	upper := strings.ToUpper(up)
	if strings.Contains(upper, "UPDATE `ASSESSMENT_TASK`") || strings.Contains(upper, "UPDATE `STATISTICS_PLAN_FACT`") {
		t.Fatal("schema migration must not rewrite source tasks or immutable facts")
	}
}

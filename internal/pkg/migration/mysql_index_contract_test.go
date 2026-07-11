package migration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOptimizeSecondaryMySQLIndexesMigrationContract(t *testing.T) {
	up := readMySQLMigration(t, "000033_optimize_secondary_mysql_indexes.up.sql")
	down := readMySQLMigration(t, "000033_optimize_secondary_mysql_indexes.down.sql")

	expectedAdds := []string{
		"ALTER TABLE `analytics_pending_event` ADD INDEX `idx_analytics_pending_event_deleted_due` (`deleted_at`, `next_attempt_at`, `event_id`)",
		"ALTER TABLE `domain_event_outbox` ADD INDEX `idx_outbox_status_due_created` (`status`, `next_attempt_at`, `created_at`, `id`)",
		"ALTER TABLE `domain_event_outbox` ADD INDEX `idx_outbox_status_updated_created` (`status`, `updated_at`, `created_at`, `id`)",
		"ALTER TABLE `domain_event_outbox` ADD INDEX `idx_outbox_status_created` (`status`, `created_at`, `id`)",
		"ALTER TABLE `clinician_relation` ADD INDEX `idx_relation_org_clinician_deleted_bound` (`org_id`, `clinician_id`, `deleted_at`, `bound_at`, `id`)",
		"ALTER TABLE `assessment_entry` ADD INDEX `idx_assessment_entry_org_deleted_id` (`org_id`, `deleted_at`, `id`)",
		"ALTER TABLE `assessment_entry` ADD INDEX `idx_assessment_entry_org_active_deleted_expire` (`org_id`, `is_active`, `deleted_at`, `expires_at`, `id`)",
		"ALTER TABLE `clinician` ADD INDEX `idx_clinician_org_deleted_id` (`org_id`, `deleted_at`, `id`)",
		"ALTER TABLE `clinician` ADD INDEX `idx_clinician_org_active_deleted_id` (`org_id`, `is_active`, `deleted_at`, `id`)",
		"ALTER TABLE `staff` ADD INDEX `idx_staff_org_deleted_id` (`org_id`, `deleted_at`, `id`)",
		"ALTER TABLE `staff` ADD INDEX `idx_staff_org_active_deleted_id` (`org_id`, `is_active`, `deleted_at`, `id`)",
		"ALTER TABLE `assessment_plan` ADD INDEX `idx_plan_org_deleted_id` (`org_id`, `deleted_at`, `id`)",
		"ALTER TABLE `assessment_plan` ADD INDEX `idx_plan_org_deleted_scale_status_id` (`org_id`, `deleted_at`, `scale_code`, `status`, `id`)",
	}
	for _, ddl := range expectedAdds {
		requireSQLContains(t, up, ddl)
		indexName := ddl[strings.Index(ddl, "ADD INDEX `")+len("ADD INDEX `"):]
		indexName = indexName[:strings.Index(indexName, "`")]
		requireSQLContains(t, up, "AND index_name = '"+indexName+"'")
		requireSQLContains(t, down, "DROP INDEX `"+indexName+"`")
	}
	requireSQLContains(t, up, "FROM information_schema.statistics")
	requireSQLContains(t, down, "FROM information_schema.statistics")
}

func TestPruneConfirmedRedundantMySQLIndexesMigrationContract(t *testing.T) {
	up := readMySQLMigration(t, "000034_prune_confirmed_redundant_mysql_indexes.up.sql")
	down := readMySQLMigration(t, "000034_prune_confirmed_redundant_mysql_indexes.down.sql")

	expectedDrops := []string{
		"ALTER TABLE `assessment` DROP INDEX `idx_assessment_org_code_created_deleted`",
		"ALTER TABLE `assessment` DROP INDEX `idx_assessment_org_code_status_created_deleted`",
		"ALTER TABLE `assessment_task` DROP INDEX `idx_assessment_id`",
		"ALTER TABLE `assessment_score` DROP INDEX `idx_risk_level`",
		"ALTER TABLE `analytics_projector_checkpoint` DROP INDEX `idx_analytics_projector_checkpoint_status`",
		"ALTER TABLE `analytics_projector_checkpoint` DROP INDEX `idx_analytics_projector_checkpoint_deleted_at`",
	}
	for _, ddl := range expectedDrops {
		requireSQLContains(t, up, ddl)
		indexName := ddl[strings.Index(ddl, "DROP INDEX `")+len("DROP INDEX `"):]
		indexName = indexName[:strings.Index(indexName, "`")]
		requireSQLContains(t, up, "AND index_name = '"+indexName+"'")
	}

	expectedRestores := []string{
		"ALTER TABLE `analytics_projector_checkpoint` ADD INDEX `idx_analytics_projector_checkpoint_deleted_at` (`deleted_at`)",
		"ALTER TABLE `analytics_projector_checkpoint` ADD INDEX `idx_analytics_projector_checkpoint_status` (`status`)",
		"ALTER TABLE `assessment_score` ADD INDEX `idx_risk_level` (`risk_level`)",
		"ALTER TABLE `assessment_task` ADD INDEX `idx_assessment_id` (`assessment_id`)",
		"ALTER TABLE `assessment` ADD INDEX `idx_assessment_org_code_status_created_deleted` (`org_id`, `questionnaire_code`, `status`, `created_at`, `deleted_at`)",
		"ALTER TABLE `assessment` ADD INDEX `idx_assessment_org_code_created_deleted` (`org_id`, `questionnaire_code`, `created_at`, `deleted_at`)",
	}
	for _, ddl := range expectedRestores {
		requireSQLContains(t, down, ddl)
	}

	for _, keptIndex := range []string{
		"DROP INDEX `idx_org_clinician_active_deleted`",
		"DROP INDEX `idx_org_testee_active_deleted`",
		"DROP INDEX `idx_plan_seq`",
		"DROP INDEX `idx_name`",
		"DROP INDEX `idx_is_active`",
		"DROP INDEX `idx_deleted_at`",
	} {
		if strings.Contains(up, keptIndex) {
			t.Fatalf("prune migration must not contain %q", keptIndex)
		}
	}
}

func TestOptimizeHighRiskLatestQueueIndexMigrationContract(t *testing.T) {
	up := readMySQLMigration(t, "000035_optimize_high_risk_latest_queue_index.up.sql")
	down := readMySQLMigration(t, "000035_optimize_high_risk_latest_queue_index.down.sql")

	requireSQLContains(t, up, "ALTER TABLE `assessment` ADD INDEX `idx_assessment_workbench_latest_id_risk_by_testee` (`org_id`, `status`, `deleted_at`, `testee_id`, `id`, `risk_level`)")
	requireSQLContains(t, up, "AND index_name = 'idx_assessment_workbench_latest_id_risk_by_testee'")
	requireSQLContains(t, down, "ALTER TABLE `assessment` DROP INDEX `idx_assessment_workbench_latest_id_risk_by_testee`")
	requireSQLContains(t, down, "AND index_name = 'idx_assessment_workbench_latest_id_risk_by_testee'")
}

func TestEvaluationCompatibilityRetirementMigrationContract(t *testing.T) {
	retire := readMySQLMigration(t, "000044_retire_assessment_interpreted_and_score_copy_fields.up.sql")
	for _, token := range []string{
		"ADD COLUMN `evaluated_at`",
		"SET `status` = 'evaluated'",
		"DROP COLUMN `interpreted_at`",
		"DROP COLUMN `conclusion`",
		"DROP COLUMN `suggestion`",
	} {
		requireSQLContains(t, retire, token)
	}

	linkOutcome := readMySQLMigration(t, "000045_link_assessment_score_to_evaluation_outcome.up.sql")
	for _, token := range []string{
		"ADD COLUMN `evaluation_outcome_id`",
		"ADD KEY `idx_assessment_score_outcome`",
		"INNER JOIN `evaluation_outcome` AS outcome",
	} {
		requireSQLContains(t, linkOutcome, token)
	}
}

func readMySQLMigration(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("migrations", "mysql", name))
	if err != nil {
		t.Fatalf("read migration %s: %v", name, err)
	}
	return string(data)
}

func requireSQLContains(t *testing.T, sql, token string) {
	t.Helper()
	if !strings.Contains(sql, token) {
		t.Fatalf("migration SQL does not contain %q", token)
	}
}

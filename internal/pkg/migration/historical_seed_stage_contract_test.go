package migration

import (
	"strings"
	"testing"
)

func TestHistoricalSeedStageMigrationContract(t *testing.T) {
	up := readMySQLMigration(t, "000059_add_seed_backfill_stage.up.sql")
	for _, required := range []string{"CREATE TABLE IF NOT EXISTS `seed_backfill_stage`", "`org_id` BIGINT NOT NULL", "uk_seed_backfill_stage_identity", "`payload_hash` CHAR(64)", "`business_at` DATETIME(6)", "`payload_json` JSON"} {
		if !strings.Contains(up, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
}

func TestHistoricalSeedStageAttemptMigrationContract(t *testing.T) {
	up := readMySQLMigration(t, "000061_add_seed_backfill_stage_attempt.up.sql")
	for _, required := range []string{"CREATE TABLE IF NOT EXISTS `seed_backfill_stage_attempt`", "`attempt_no` INT UNSIGNED NOT NULL", "`context_hash` CHAR(64)", "`status` VARCHAR(24)", "`error_text` VARCHAR(1000)", "uk_seed_backfill_stage_attempt"} {
		if !strings.Contains(up, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
}

func TestHistoricalSeedRollbackOperationMigrationContract(t *testing.T) {
	up := readMySQLMigration(t, "000062_add_seed_backfill_rollback_operation.up.sql")
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS `seed_backfill_rollback_operation`",
		"CREATE TABLE IF NOT EXISTS `seed_backfill_rollback_resource`",
		"CREATE TABLE IF NOT EXISTS `seed_backfill_rollback_phase_attempt`",
		"`manifest_hash` CHAR(64)", "`scope_hash` CHAR(64)",
		"uk_seed_backfill_rollback_batch_scope",
		"PRIMARY KEY (`operation_id`, `storage`, `resource_type`, `resource_id`)",
		"ON DELETE RESTRICT",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("migration missing %q", required)
		}
	}

	down := readMySQLMigration(t, "000062_add_seed_backfill_rollback_operation.down.sql")
	phase := strings.Index(down, "seed_backfill_rollback_phase_attempt")
	resource := strings.Index(down, "seed_backfill_rollback_resource")
	operation := strings.Index(down, "seed_backfill_rollback_operation")
	if phase < 0 || resource <= phase || operation <= resource {
		t.Fatalf("rollback tables must be dropped child-first: %s", down)
	}
}

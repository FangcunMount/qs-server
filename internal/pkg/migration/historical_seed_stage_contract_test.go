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

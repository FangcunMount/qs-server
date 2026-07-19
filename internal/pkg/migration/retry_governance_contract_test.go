package migration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMySQLRetryGovernanceMigrationContract(t *testing.T) {
	up := readMySQLMigration(t, "000049_add_retry_governance.up.sql")
	down := readMySQLMigration(t, "000049_add_retry_governance.down.sql")

	for _, token := range []string{
		"ADD COLUMN `attempt_origin`",
		"ADD COLUMN `retry_disposition`",
		"ADD COLUMN `next_attempt_at`",
		"ADD COLUMN `policy_max_attempts`",
		"ADD COLUMN `retry_event_id`",
		"ADD COLUMN `action_request_id`",
		"idx_runtime_checkpoint_retry_due",
		"ADD COLUMN `org_id`",
		"ADD COLUMN `last_error_kind`",
		"ADD COLUMN `manual_replay_request_id`",
		"idx_outbox_org_retry_due",
		"CREATE TABLE `event_delivery_dead_letter`",
		"uk_delivery_dead_letter_identity",
		"idx_delivery_dead_letter_org_disposition",
	} {
		requireSQLContains(t, up, token)
	}
	for _, token := range []string{
		"DROP TABLE IF EXISTS `event_delivery_dead_letter`",
		"DROP INDEX `idx_outbox_org_retry_due`",
		"DROP COLUMN `manual_replay_request_id`",
		"DROP INDEX `idx_runtime_checkpoint_retry_due`",
		"DROP COLUMN `attempt_origin`",
	} {
		requireSQLContains(t, down, token)
	}
	if strings.Index(down, "DROP TABLE IF EXISTS `event_delivery_dead_letter`") > strings.Index(down, "ALTER TABLE `runtime_checkpoint`") {
		t.Fatal("down migration must remove the dependent delivery audit table before retry columns")
	}
}

func TestMongoRetryGovernanceMigrationContract(t *testing.T) {
	up := readJSONMigration(t, "000012_add_retry_governance.up.json")
	down := readJSONMigration(t, "000012_add_retry_governance.down.json")

	for _, token := range []string{
		"idx_interpretation_run_retry_due",
		"idx_interpretation_run_action_request",
		"idx_outbox_org_retry_due",
		"retry_disposition",
		"next_attempt_at",
		"action_request_id",
		"org_id",
	} {
		if !strings.Contains(up, token) {
			t.Fatalf("mongo up migration does not contain %q", token)
		}
	}
	for _, index := range []string{
		"idx_interpretation_run_retry_due",
		"idx_interpretation_run_action_request",
		"idx_outbox_org_retry_due",
	} {
		if !strings.Contains(down, index) {
			t.Fatalf("mongo down migration does not remove %q", index)
		}
	}
}

func TestMySQLRetryEventHoldMigrationContract(t *testing.T) {
	up := readMySQLMigration(t, "000050_add_retry_event_hold.up.sql")
	down := readMySQLMigration(t, "000050_add_retry_event_hold.down.sql")
	for _, token := range []string{
		"CREATE TABLE `retry_event_hold`",
		"`original_delivery_attempt`",
		"`replay_attempt_count`",
		"`claim_token`",
		"`manual_replay_request_id`",
		"uk_retry_event_hold_delivery",
		"idx_retry_event_hold_due",
		"idx_retry_event_hold_org_governance",
	} {
		requireSQLContains(t, up, token)
	}
	requireSQLContains(t, down, "DROP TABLE IF EXISTS `retry_event_hold`")
}

func readJSONMigration(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("migrations", "mongodb", name))
	if err != nil {
		t.Fatalf("read migration %s: %v", name, err)
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("migration %s is not valid JSON: %v", name, err)
	}
	return string(data)
}

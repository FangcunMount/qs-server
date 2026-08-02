package migration

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRetireSeedBackfillControlMigrationContract(t *testing.T) {
	t.Parallel()

	up := readMySQLMigration(t, "000064_retire_seed_backfill_control.up.sql")
	down := readMySQLMigration(t, "000064_retire_seed_backfill_control.down.sql")
	wantDropOrder := []string{
		"seed_backfill_rollback_phase_attempt",
		"seed_backfill_rollback_resource",
		"seed_backfill_rollback_operation",
		"seed_backfill_stage_attempt",
		"seed_backfill_stage",
	}
	previous := -1
	for _, table := range wantDropOrder {
		token := "DROP TABLE IF EXISTS `" + table + "`"
		if strings.Count(up, token) != 1 {
			t.Fatalf("up migration must drop %s exactly once", table)
		}
		current := strings.Index(up, token)
		if current <= previous {
			t.Fatalf("up migration drop order is invalid for %s", table)
		}
		previous = current
		if !strings.Contains(down, "CREATE TABLE IF NOT EXISTS `"+table+"`") {
			t.Fatalf("down migration must recreate empty table %s", table)
		}
	}
	if strings.Contains(strings.ToLower(up), "business_created_at") {
		t.Fatal("retirement migration must not alter business_created_at")
	}
}

func TestRetireLegacyMongoCollectionsMigrationContract(t *testing.T) {
	t.Parallel()

	up := readJSONMigration(t, "000020_retire_legacy_collections.up.json")
	down := readJSONMigration(t, "000020_retire_legacy_collections.down.json")
	var upCommands []map[string]any
	if err := json.Unmarshal([]byte(up), &upCommands); err != nil {
		t.Fatalf("up migration is not valid JSON: %v", err)
	}
	want := []string{"published_assessment_models", "interpret_reports", "evaluation_rule_sets", "scales"}
	if len(upCommands) != len(want) {
		t.Fatalf("up command count = %d, want %d", len(upCommands), len(want))
	}
	for i, collection := range want {
		if got := upCommands[i]["drop"]; got != collection {
			t.Fatalf("up command %d drops %v, want %s", i, got, collection)
		}
		if !strings.Contains(down, `"create": "`+collection+`"`) {
			t.Fatalf("down migration must recreate empty collection %s", collection)
		}
	}
	for _, index := range []string{
		"idx_published_assessment_models_identity_version",
		"idx_reports_testee_active_created",
		"idx_evaluation_rule_sets_kind_code_version",
		"idx_scales_status_deleted",
		"idx_scales_status_published",
		"idx_scales_status_deleted_created",
		"idx_scales_questionnaire_deleted_created",
		"idx_scales_category_deleted_created",
		"idx_scales_code_version_deleted",
		"idx_scales_code_version_role_unique",
		"idx_scales_active_published_code",
		"idx_scales_questionnaire_version_role",
		"idx_scales_published_questionnaire_active",
	} {
		if !strings.Contains(down, index) {
			t.Fatalf("down migration must recreate legacy index %s", index)
		}
	}
	for _, retiredIndex := range []string{`"name": "idx_testee_id"`, `"name": "idx_status"`} {
		if strings.Contains(down, retiredIndex) {
			t.Fatalf("down migration must not recreate index %s removed before MongoDB v19", retiredIndex)
		}
	}
}

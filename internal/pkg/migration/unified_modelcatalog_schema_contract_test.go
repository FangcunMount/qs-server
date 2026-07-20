package migration

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMongoUnifiedModelCatalogSchemaMigrationContract(t *testing.T) {
	t.Parallel()

	up := readJSONMigration(t, "000013_unified_modelcatalog_schema.up.json")
	down := readJSONMigration(t, "000013_unified_modelcatalog_schema.down.json")

	for _, token := range []string{
		`"dropIndexes": "assessment_models"`,
		`"index": "idx_assessment_models_code"`,
		`"dropIndexes": "questionnaires"`,
		`"index": "idx_code_version"`,
		"idx_assessment_models_head_code",
		"idx_assessment_models_snapshot_identity_version",
		"idx_assessment_models_active_code",
		"idx_assessment_models_active_questionnaire",
		"idx_assessment_models_active_catalog",
		"idx_assessment_models_release_history",
		"idx_questionnaires_head_code",
		"idx_questionnaires_snapshot_version",
		"idx_questionnaires_active_code",
		"idx_questionnaires_release_history",
		"idx_assessment_norms_table_version",
		`"record_role": "head"`,
		`"record_role": "published_snapshot"`,
		`"release_status": "active"`,
		`"table_version": 1`,
		`"unique": true`,
	} {
		if !strings.Contains(up, token) {
			t.Fatalf("mongo up migration 000013 does not contain %q", token)
		}
	}

	// Ensure conflicting legacy unique indexes are not reintroduced by the up path.
	if strings.Count(up, `"name": "idx_assessment_models_code"`) != 0 {
		t.Fatal("up migration must not recreate legacy idx_assessment_models_code")
	}
	if strings.Count(up, `"name": "idx_code_version"`) != 0 {
		t.Fatal("up migration must not recreate legacy questionnaires idx_code_version")
	}

	for _, index := range []string{
		"idx_assessment_models_head_code",
		"idx_assessment_models_snapshot_identity_version",
		"idx_assessment_models_active_code",
		"idx_assessment_models_active_questionnaire",
		"idx_assessment_models_active_catalog",
		"idx_assessment_models_release_history",
		"idx_questionnaires_head_code",
		"idx_questionnaires_snapshot_version",
		"idx_questionnaires_active_code",
		"idx_questionnaires_release_history",
		"idx_assessment_norms_table_version",
	} {
		if !strings.Contains(down, index) {
			t.Fatalf("mongo down migration 000013 does not remove %q", index)
		}
	}
	for _, token := range []string{
		"idx_assessment_models_code",
		"idx_code_version",
	} {
		if !strings.Contains(down, token) {
			t.Fatalf("mongo down migration 000013 does not restore legacy %q", token)
		}
	}

	var commands []map[string]any
	if err := json.Unmarshal([]byte(up), &commands); err != nil {
		t.Fatalf("up migration is not a JSON array: %v", err)
	}
	if len(commands) < 4 {
		t.Fatalf("up migration command count = %d, want drop/create/index steps", len(commands))
	}
}

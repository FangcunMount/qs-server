package evaluation

import (
	"os"
	"strings"
	"testing"
)

func TestEvaluationModelRefMigrationBackfillsFromMedicalScaleFields(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../../pkg/migration/migrations/mysql/000030_add_evaluation_model_ref_to_assessment.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, token := range []string{
		"evaluation_model_kind",
		"evaluation_model_code",
		"evaluation_model_version",
		"evaluation_model_title",
		"`evaluation_model_kind` = 'scale'",
		"`evaluation_model_code` = `medical_scale_code`",
		"`evaluation_model_title` = `medical_scale_name`",
	} {
		if !strings.Contains(text, token) {
			t.Fatalf("migration does not contain %q", token)
		}
	}
}

func TestRetireMedicalScaleAssessmentFieldsMigrationUsesCanonicalModelReference(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../../pkg/migration/migrations/mysql/000041_retire_medical_scale_assessment_fields.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, token := range []string{
		"`evaluation_model_kind` = 'scale'",
		"`evaluation_model_code` = `medical_scale_code`",
		"DROP COLUMN `medical_scale_id`",
		"DROP COLUMN `medical_scale_code`",
		"DROP COLUMN `medical_scale_name`",
		"DROP INDEX `idx_score_testee_scale_deleted_id`",
	} {
		if !strings.Contains(text, token) {
			t.Fatalf("retirement migration does not contain %q", token)
		}
	}
}

func TestAssessmentOutcomeV2MigrationBackfillsInterpretedRows(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../../pkg/migration/migrations/mysql/000036_add_assessment_outcome_v2_fields.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, token := range []string{
		"evaluation_model_sub_kind",
		"evaluation_model_algorithm",
		"primary_score_kind",
		"primary_score_value",
		"level_code",
		"severity",
		"WHEN 'scale' THEN 'scale_default'",
		"WHEN 'mbti' THEN 'typology'",
		"WHEN 'mbti' THEN 'mbti'",
		"WHERE `status` = 'interpreted'",
	} {
		if !strings.Contains(text, token) {
			t.Fatalf("migration does not contain %q", token)
		}
	}
}

func TestRuntimeCheckpointMigrationMergesLegacyTables(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../../pkg/migration/migrations/mysql/000040_merge_runtime_checkpoint.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, token := range []string{
		"CREATE TABLE IF NOT EXISTS `runtime_checkpoint`",
		"`scope`",
		"`resource_id`",
		"`attempt_no`",
		"INSERT INTO `runtime_checkpoint`",
		"FROM `evaluation_run`",
		"FROM `analytics_projector_checkpoint`",
		"DROP TABLE IF EXISTS `analytics_projector_checkpoint`",
		"DROP TABLE IF EXISTS `evaluation_run`",
	} {
		if !strings.Contains(text, token) {
			t.Fatalf("migration does not contain %q", token)
		}
	}
}

func TestEvaluationOutcomeMigrationCreatesImmutableFactTable(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../../pkg/migration/migrations/mysql/000042_add_evaluation_outcome.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, token := range []string{
		"CREATE TABLE IF NOT EXISTS `evaluation_outcome`",
		"`assessment_id`",
		"`evaluation_run_id`",
		"`payload_json` longtext NOT NULL",
		"`schema_version`",
		"UNIQUE KEY `uk_evaluation_outcome_assessment_id`",
		"UNIQUE KEY `uk_evaluation_outcome_run_id`",
	} {
		if !strings.Contains(text, token) {
			t.Fatalf("evaluation outcome migration does not contain %q", token)
		}
	}
}

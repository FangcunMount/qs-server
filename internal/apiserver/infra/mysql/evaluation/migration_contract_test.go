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

func TestEvaluationRunMigrationDefinesRunTableAndAssessmentPointer(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../../pkg/migration/migrations/mysql/000038_add_evaluation_run.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, token := range []string{
		"CREATE TABLE IF NOT EXISTS `evaluation_run`",
		"`run_id`",
		"`attempt_no`",
		"`retryable`",
		"`current_run_id`",
		"idx_assessment_current_run_id",
	} {
		if !strings.Contains(text, token) {
			t.Fatalf("migration does not contain %q", token)
		}
	}
}

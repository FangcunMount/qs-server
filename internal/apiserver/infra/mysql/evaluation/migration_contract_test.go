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

package typology

import (
	"testing"

	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
)

func TestValidatePreviewAnswers(t *testing.T) {
	questionnaire := &questionnaireapp.QuestionnaireResult{
		Questions: []questionnaireapp.QuestionResult{
			{
				Code: "q1",
				Options: []questionnaireapp.OptionResult{
					{Value: "A"},
					{Value: "B"},
				},
			},
		},
	}

	t.Run("invalid option value", func(t *testing.T) {
		issues := validatePreviewAnswers([]PreviewAnswer{
			{QuestionCode: "q1", Value: "C"},
		}, questionnaire)
		if len(issues) == 0 || issues[0].Code != "answer.value.invalid_option" {
			t.Fatalf("issues = %+v, want answer.value.invalid_option", issues)
		}
	})

	t.Run("score zero counts as provided value", func(t *testing.T) {
		zero := 0.0
		issues := validatePreviewAnswers([]PreviewAnswer{
			{QuestionCode: "q1", Score: &zero},
		}, questionnaire)
		if len(issues) != 0 {
			t.Fatalf("issues = %+v, want none", issues)
		}
	})

	t.Run("missing value and score is rejected", func(t *testing.T) {
		issues := validatePreviewAnswers([]PreviewAnswer{
			{QuestionCode: "q1"},
		}, questionnaire)
		if len(issues) == 0 || issues[0].Code != "answer.value_or_score.required" {
			t.Fatalf("issues = %+v, want answer.value_or_score.required", issues)
		}
	})
}

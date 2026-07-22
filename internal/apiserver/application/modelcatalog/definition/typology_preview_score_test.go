package definition

import (
	"context"
	"encoding/json"
	"testing"

	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

func TestTypologyPreviewDerivesScoreFromQuestionnaireOption(t *testing.T) {
	questionnaire := previewQuestionnaireFixture()
	answers := []typologyPreviewAnswer{{QuestionCode: "q1", Value: "B"}}
	if issues := validateTypologyPreviewAnswers(answers, questionnaire); domain.HasValidationErrors(issues) {
		t.Fatalf("validateTypologyPreviewAnswers() issues = %#v", issues)
	}
	input := typologyPreviewExecutionInput(&domain.AssessmentModel{}, questionnaire, &modeltypology.Payload{}, answers)
	if got := input.AnswerSheet.Answers[0].Score; got != -2 {
		t.Fatalf("derived score = %v, want -2", got)
	}
}

func TestTypologyPreviewRejectsScoreThatDiffersFromOption(t *testing.T) {
	score := 99.0
	issues := validateTypologyPreviewAnswers([]typologyPreviewAnswer{{QuestionCode: "q1", Value: "A", Score: &score}}, previewQuestionnaireFixture())
	if !hasDomainIssueCode(issues, "answer.score.mismatch") {
		t.Fatalf("issues = %#v, want answer.score.mismatch", issues)
	}
}

func TestTypologyPreviewAllowsFiniteScoreForQuestionWithoutOptions(t *testing.T) {
	score := -1.5
	questionnaire := &questionnaireapp.QuestionnaireResult{Questions: []questionnaireapp.QuestionResult{{Code: "q2", Type: "Number"}}}
	issues := validateTypologyPreviewAnswers([]typologyPreviewAnswer{{QuestionCode: "q2", Score: &score}}, questionnaire)
	if domain.HasValidationErrors(issues) {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestTypologyPreviewUsesPublishValidationForReportMap(t *testing.T) {
	t.Parallel()

	service := TypologyPreviewService{ValidateForPublish: func(context.Context, *domain.AssessmentModel) []domain.DomainValidationIssue {
		return []domain.DomainValidationIssue{{
			Field: "report_map.sections.factors.source_refs", Code: "report_section.source_ref.not_found",
			Message: "factor source ref missing is not defined", Level: domain.ValidationLevelError,
		}}
	}}
	_, err := service.PreviewReport(context.Background(), &domain.AssessmentModel{}, json.RawMessage(`[{"question_code":"q1","value":"A"}]`))
	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("error = %T %v, want *ValidationError", err, err)
	}
	if len(validationErr.Issues) != 1 || validationErr.Issues[0].Code != "report_section.source_ref.not_found" {
		t.Fatalf("issues = %#v", validationErr.Issues)
	}
}

func previewQuestionnaireFixture() *questionnaireapp.QuestionnaireResult {
	return &questionnaireapp.QuestionnaireResult{Questions: []questionnaireapp.QuestionResult{{
		Code: "q1", Type: "Radio", Options: []questionnaireapp.OptionResult{{Value: "A", Score: 1}, {Value: "B", Score: -2}},
	}}}
}

func hasDomainIssueCode(issues []domain.DomainValidationIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

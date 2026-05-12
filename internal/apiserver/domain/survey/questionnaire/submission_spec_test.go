package questionnaire

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/validation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestBuildSubmissionSpecRequiresPublishedQuestionnaire(t *testing.T) {
	t.Parallel()

	qnr := mustSubmissionSpecQuestionnaire(t, WithStatus(STATUS_DRAFT))
	if _, err := qnr.BuildSubmissionSpec(); err == nil {
		t.Fatal("BuildSubmissionSpec() error = nil, want unpublished questionnaire error")
	}
}

func TestSubmissionSpecPrepareAnswersUsesQuestionnaireQuestionType(t *testing.T) {
	t.Parallel()

	spec := mustSubmissionSpec(t)
	prepared, err := spec.PrepareAnswers([]RawSubmissionAnswer{
		{QuestionCode: "Q1", QuestionType: TypeText.Value(), Value: "hello"},
	})
	if err != nil {
		t.Fatalf("PrepareAnswers() error = %v", err)
	}
	if len(prepared) != 1 {
		t.Fatalf("prepared count = %d, want 1", len(prepared))
	}
	answer := prepared[0]
	if answer.QuestionCode().Value() != "Q1" || answer.QuestionType() != TypeText || answer.Value() != "hello" {
		t.Fatalf("prepared answer = %+v", answer)
	}
	rules := answer.ValidationRules()
	if len(rules) != 1 || rules[0].GetRuleType() != validation.RuleTypeRequired {
		t.Fatalf("validation rules = %+v, want required rule", rules)
	}
}

func TestSubmissionSpecPrepareAnswersRejectsUnknownQuestion(t *testing.T) {
	t.Parallel()

	spec := mustSubmissionSpec(t)
	if _, err := spec.PrepareAnswers([]RawSubmissionAnswer{
		{QuestionCode: "missing", QuestionType: TypeText.Value(), Value: "hello"},
	}); err == nil {
		t.Fatal("PrepareAnswers() error = nil, want unknown question error")
	}
}

func TestSubmissionSpecPrepareAnswersRejectsQuestionTypeMismatch(t *testing.T) {
	t.Parallel()

	spec := mustSubmissionSpec(t)
	if _, err := spec.PrepareAnswers([]RawSubmissionAnswer{
		{QuestionCode: "Q1", QuestionType: TypeRadio.Value(), Value: "A"},
	}); err == nil {
		t.Fatal("PrepareAnswers() error = nil, want type mismatch error")
	}
}

func mustSubmissionSpec(t *testing.T) SubmissionSpec {
	t.Helper()
	qnr := mustSubmissionSpecQuestionnaire(t, WithStatus(STATUS_PUBLISHED))
	spec, err := qnr.BuildSubmissionSpec()
	if err != nil {
		t.Fatalf("BuildSubmissionSpec() error = %v", err)
	}
	return spec
}

func mustSubmissionSpecQuestionnaire(t *testing.T, opts ...QuestionnaireOption) *Questionnaire {
	t.Helper()
	options := []QuestionnaireOption{
		WithVersion(Version("1.0.0")),
	}
	options = append(options, opts...)
	qnr, err := NewQuestionnaire(meta.NewCode("QNR-1"), "Questionnaire", options...)
	if err != nil {
		t.Fatalf("NewQuestionnaire() error = %v", err)
	}
	question, err := NewQuestion(
		WithCode(meta.NewCode("Q1")),
		WithStem("Question 1"),
		WithQuestionType(TypeText),
		WithValidationRule(validation.RuleTypeRequired, ""),
	)
	if err != nil {
		t.Fatalf("NewQuestion() error = %v", err)
	}
	if err := qnr.AddQuestion(question); err != nil {
		t.Fatalf("AddQuestion() error = %v", err)
	}
	return qnr
}

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

func TestSubmissionSpecPrepareAnswersRejectsInvalidOptionCode(t *testing.T) {
	t.Parallel()

	qnr, err := NewQuestionnaire(meta.NewCode("QNR-2"), "Questionnaire", WithVersion(Version("1.0.0")), WithStatus(STATUS_PUBLISHED))
	if err != nil {
		t.Fatalf("NewQuestionnaire() error = %v", err)
	}
	optionA, err := NewOptionWithStringCode("A", "option A", 1)
	if err != nil {
		t.Fatalf("NewOption() error = %v", err)
	}
	question, err := NewQuestion(
		WithCode(meta.NewCode("Q1")),
		WithStem("Question 1"),
		WithQuestionType(TypeRadio),
		WithOptions([]Option{optionA}),
	)
	if err != nil {
		t.Fatalf("NewQuestion() error = %v", err)
	}
	if err := qnr.AddQuestion(question); err != nil {
		t.Fatalf("AddQuestion() error = %v", err)
	}
	spec, err := qnr.BuildSubmissionSpec()
	if err != nil {
		t.Fatalf("BuildSubmissionSpec() error = %v", err)
	}
	if _, err := spec.PrepareAnswers([]RawSubmissionAnswer{
		{QuestionCode: "Q1", QuestionType: TypeRadio.Value(), Value: "B"},
	}); err == nil {
		t.Fatal("PrepareAnswers() error = nil, want invalid option error")
	}
}

func TestSubmissionSpecPrepareAnswersRequiresVisibleRequiredQuestion(t *testing.T) {
	t.Parallel()

	qnr, err := NewQuestionnaire(meta.NewCode("QNR-3"), "Questionnaire", WithVersion(Version("1.0.0")), WithStatus(STATUS_PUBLISHED))
	if err != nil {
		t.Fatalf("NewQuestionnaire() error = %v", err)
	}
	triggerOption, err := NewOptionWithStringCode("YES", "yes", 1)
	if err != nil {
		t.Fatalf("NewOption() error = %v", err)
	}
	trigger, err := NewQuestion(
		WithCode(meta.NewCode("Q_TRIGGER")),
		WithStem("Trigger"),
		WithQuestionType(TypeRadio),
		WithOptions([]Option{triggerOption}),
	)
	if err != nil {
		t.Fatalf("NewQuestion() error = %v", err)
	}
	followUp, err := NewQuestion(
		WithCode(meta.NewCode("Q_FOLLOW")),
		WithStem("Follow up"),
		WithQuestionType(TypeText),
		WithValidationRule(validation.RuleTypeRequired, "true"),
		WithShowController(NewShowController("and", []ShowControllerCondition{
			NewShowControllerCondition(meta.NewCode("Q_TRIGGER"), []meta.Code{meta.NewCode("YES")}),
		})),
	)
	if err != nil {
		t.Fatalf("NewQuestion() error = %v", err)
	}
	for _, item := range []Question{trigger, followUp} {
		if err := qnr.AddQuestion(item); err != nil {
			t.Fatalf("AddQuestion() error = %v", err)
		}
	}
	spec, err := qnr.BuildSubmissionSpec()
	if err != nil {
		t.Fatalf("BuildSubmissionSpec() error = %v", err)
	}
	if _, err := spec.PrepareAnswers([]RawSubmissionAnswer{
		{QuestionCode: "Q_TRIGGER", QuestionType: TypeRadio.Value(), Value: "YES"},
	}); err == nil {
		t.Fatal("PrepareAnswers() error = nil, want missing required visible question")
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

package questionnaire

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/validation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestValidatorRejectsUnsupportedValidationRuleOnPublish(t *testing.T) {
	qnr, err := NewQuestionnaire(meta.NewCode("QNR"), "Questionnaire", WithVersion("1.0.0"))
	if err != nil {
		t.Fatalf("NewQuestionnaire() error = %v", err)
	}
	question, err := NewQuestion(
		WithCode(meta.NewCode("Q1")), WithStem("Question"), WithQuestionType(TypeText),
		WithValidationRule(validation.RuleType("custom"), "x"),
	)
	if err != nil {
		t.Fatalf("NewQuestion() error = %v", err)
	}
	if err := qnr.AddQuestion(question); err != nil {
		t.Fatalf("AddQuestion() error = %v", err)
	}
	for _, validationErr := range (Validator{}).ValidateForPublish(qnr) {
		if validationErr.Field == "validation_rules" {
			return
		}
	}
	t.Fatal("expected unsupported validation rule publish error")
}

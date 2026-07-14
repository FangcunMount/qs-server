package questionnaire

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/validation"
)

func TestBuildQuestionFromDTORejectsUnsupportedValidationRule(t *testing.T) {
	_, err := buildQuestionFromDTO("Q1", "Question", "Text", nil, false, "",
		[]validation.ValidationRule{validation.NewValidationRule(validation.RuleType("custom"), "x")}, nil, nil)
	if err == nil {
		t.Fatal("expected unsupported validation rule error")
	}
}

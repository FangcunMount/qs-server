package ruleengine

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/validation"
	ruleengineport "github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

type validatableStub struct {
	empty bool
	value string
}

func (v validatableStub) IsEmpty() bool { return v.empty }
func (v validatableStub) AsString() string {
	return v.value
}
func (v validatableStub) AsNumber() (float64, error) { return 0, nil }
func (v validatableStub) AsArray() []string          { return nil }

func TestAnswerValidatorMapsValidationResultsToPortDTO(t *testing.T) {
	t.Parallel()

	results, err := NewAnswerValidator(nil).ValidateAnswers(context.Background(), []ruleengineport.AnswerValidationTask{
		{
			ID:    "q1",
			Value: validatableStub{empty: true},
			Rules: []validation.ValidationRule{validation.NewValidationRule(validation.RuleTypeRequired, "")},
		},
	})
	if err != nil {
		t.Fatalf("ValidateAnswers returned error: %v", err)
	}
	if len(results) != 1 || results[0].ID != "q1" || results[0].Valid || len(results[0].Errors) != 1 {
		t.Fatalf("unexpected results: %+v", results)
	}
	if results[0].Errors[0].RuleType != string(validation.RuleTypeRequired) {
		t.Fatalf("rule type = %q, want required", results[0].Errors[0].RuleType)
	}
}

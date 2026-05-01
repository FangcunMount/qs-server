package ruleengine

import (
	"context"
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/validation"
	ruleengineport "github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

type validatableStub struct {
	empty     bool
	str       string
	number    float64
	numberErr error
	array     []string
}

func (v validatableStub) IsEmpty() bool { return v.empty }
func (v validatableStub) AsString() string {
	return v.str
}
func (v validatableStub) AsNumber() (float64, error) {
	if v.numberErr != nil {
		return 0, v.numberErr
	}
	return v.number, nil
}
func (v validatableStub) AsArray() []string { return v.array }

func TestAnswerValidatorMapsValidationResultsToPortDTO(t *testing.T) {
	t.Parallel()

	results, err := NewAnswerValidator().ValidateAnswers(context.Background(), []ruleengineport.AnswerValidationTask{
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

func TestAnswerValidatorAppliesBuiltInRules(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value validatableStub
		rule  validation.ValidationRule
		valid bool
	}{
		{name: "required fails for empty", value: validatableStub{empty: true}, rule: validation.NewValidationRule(validation.RuleTypeRequired, ""), valid: false},
		{name: "required passes for non empty", value: validatableStub{str: "x"}, rule: validation.NewValidationRule(validation.RuleTypeRequired, ""), valid: true},
		{name: "min length counts runes", value: validatableStub{str: "你好"}, rule: validation.NewValidationRule(validation.RuleTypeMinLength, "2"), valid: true},
		{name: "max length fails", value: validatableStub{str: "abcd"}, rule: validation.NewValidationRule(validation.RuleTypeMaxLength, "3"), valid: false},
		{name: "min value passes", value: validatableStub{number: 3}, rule: validation.NewValidationRule(validation.RuleTypeMinValue, "2"), valid: true},
		{name: "max value fails", value: validatableStub{number: 5}, rule: validation.NewValidationRule(validation.RuleTypeMaxValue, "4"), valid: false},
		{name: "min selections passes", value: validatableStub{array: []string{"a", "b"}}, rule: validation.NewValidationRule(validation.RuleTypeMinSelections, "2"), valid: true},
		{name: "max selections fails", value: validatableStub{array: []string{"a", "b", "c"}}, rule: validation.NewValidationRule(validation.RuleTypeMaxSelections, "2"), valid: false},
		{name: "pattern passes", value: validatableStub{str: "abc-123"}, rule: validation.NewValidationRule(validation.RuleTypePattern, `^[a-z]+-\d+$`), valid: true},
		{name: "pattern fails", value: validatableStub{str: "abc"}, rule: validation.NewValidationRule(validation.RuleTypePattern, `^\d+$`), valid: false},
	}

	validator := NewAnswerValidator()
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			results, err := validator.ValidateAnswers(context.Background(), []ruleengineport.AnswerValidationTask{
				{ID: "q", Value: tc.value, Rules: []validation.ValidationRule{tc.rule}},
			})
			if err != nil {
				t.Fatalf("ValidateAnswers returned error: %v", err)
			}
			if len(results) != 1 || results[0].Valid != tc.valid {
				t.Fatalf("Valid = %+v, want %v", results, tc.valid)
			}
		})
	}
}

func TestAnswerValidatorSkipsUnknownRuleType(t *testing.T) {
	t.Parallel()

	results, err := NewAnswerValidator().ValidateAnswers(context.Background(), []ruleengineport.AnswerValidationTask{
		{ID: "q", Value: validatableStub{empty: true}, Rules: []validation.ValidationRule{
			validation.NewValidationRule(validation.RuleType("unknown"), ""),
		}},
	})
	if err != nil {
		t.Fatalf("ValidateAnswers returned error: %v", err)
	}
	if len(results) != 1 || !results[0].Valid {
		t.Fatalf("unknown rule should be skipped, got %+v", results)
	}
}

func TestNumberRuleReturnsValidationErrorWhenValueCannotConvert(t *testing.T) {
	t.Parallel()

	results, err := NewAnswerValidator().ValidateAnswers(context.Background(), []ruleengineport.AnswerValidationTask{
		{ID: "q", Value: validatableStub{numberErr: errors.New("bad number")}, Rules: []validation.ValidationRule{
			validation.NewValidationRule(validation.RuleTypeMinValue, "1"),
		}},
	})
	if err != nil {
		t.Fatalf("ValidateAnswers returned error: %v", err)
	}
	if len(results) != 1 || results[0].Valid || len(results[0].Errors) != 1 {
		t.Fatalf("expected one validation error, got %+v", results)
	}
}

func TestAnswerValidatorKeepsTaskOrder(t *testing.T) {
	t.Parallel()

	results, err := NewAnswerValidator().ValidateAnswers(context.Background(), []ruleengineport.AnswerValidationTask{
		{ID: "a", Value: validatableStub{str: "ok"}, Rules: []validation.ValidationRule{validation.NewValidationRule(validation.RuleTypeRequired, "")}},
		{ID: "b", Value: validatableStub{empty: true}, Rules: []validation.ValidationRule{validation.NewValidationRule(validation.RuleTypeRequired, "")}},
	})
	if err != nil {
		t.Fatalf("ValidateAnswers returned error: %v", err)
	}
	if len(results) != 2 || results[0].ID != "a" || results[1].ID != "b" {
		t.Fatalf("results order = %+v", results)
	}
	if !results[0].Valid || results[1].Valid || len(results[1].Errors) != 1 {
		t.Fatalf("unexpected validation results = %+v", results)
	}
}

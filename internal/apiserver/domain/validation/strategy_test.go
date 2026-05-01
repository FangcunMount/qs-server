package validation

import (
	"context"
	"errors"
	"testing"
)

type validatableStub struct {
	empty     bool
	str       string
	number    float64
	numberErr error
	array     []string
}

func (v validatableStub) IsEmpty() bool { return v.empty }

func (v validatableStub) AsString() string { return v.str }

func (v validatableStub) AsNumber() (float64, error) {
	if v.numberErr != nil {
		return 0, v.numberErr
	}
	return v.number, nil
}

func (v validatableStub) AsArray() []string { return v.array }

func TestDefaultValidatorAppliesBuiltInRules(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value validatableStub
		rule  ValidationRule
		valid bool
	}{
		{name: "required fails for empty", value: validatableStub{empty: true}, rule: NewValidationRule(RuleTypeRequired, ""), valid: false},
		{name: "required passes for non empty", value: validatableStub{str: "x"}, rule: NewValidationRule(RuleTypeRequired, ""), valid: true},
		{name: "min length counts runes", value: validatableStub{str: "你好"}, rule: NewValidationRule(RuleTypeMinLength, "2"), valid: true},
		{name: "max length fails", value: validatableStub{str: "abcd"}, rule: NewValidationRule(RuleTypeMaxLength, "3"), valid: false},
		{name: "min value passes", value: validatableStub{number: 3}, rule: NewValidationRule(RuleTypeMinValue, "2"), valid: true},
		{name: "max value fails", value: validatableStub{number: 5}, rule: NewValidationRule(RuleTypeMaxValue, "4"), valid: false},
		{name: "min selections passes", value: validatableStub{array: []string{"a", "b"}}, rule: NewValidationRule(RuleTypeMinSelections, "2"), valid: true},
		{name: "max selections fails", value: validatableStub{array: []string{"a", "b", "c"}}, rule: NewValidationRule(RuleTypeMaxSelections, "2"), valid: false},
		{name: "pattern passes", value: validatableStub{str: "abc-123"}, rule: NewValidationRule(RuleTypePattern, `^[a-z]+-\d+$`), valid: true},
		{name: "pattern fails", value: validatableStub{str: "abc"}, rule: NewValidationRule(RuleTypePattern, `^\d+$`), valid: false},
	}

	validator := NewDefaultValidator()
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := validator.ValidateValue(tc.value, []ValidationRule{tc.rule})
			if result.IsValid() != tc.valid {
				t.Fatalf("IsValid() = %v, want %v; errors=%v", result.IsValid(), tc.valid, result.GetErrors())
			}
		})
	}
}

func TestValidatorSkipsUnknownRuleType(t *testing.T) {
	t.Parallel()

	result := NewDefaultValidator().ValidateValue(validatableStub{empty: true}, []ValidationRule{
		NewValidationRule(RuleType("unknown"), ""),
	})
	if !result.IsValid() {
		t.Fatalf("unknown rule should be skipped, got errors=%v", result.GetErrors())
	}
}

func TestNumberRuleReturnsValidationErrorWhenValueCannotConvert(t *testing.T) {
	t.Parallel()

	result := NewDefaultValidator().ValidateValue(validatableStub{numberErr: errors.New("bad number")}, []ValidationRule{
		NewValidationRule(RuleTypeMinValue, "1"),
	})
	if result.IsValid() || len(result.GetErrors()) != 1 {
		t.Fatalf("expected one validation error, got %+v", result)
	}
}

func TestBatchValidatorKeepsTaskOrderAndAggregatesFailures(t *testing.T) {
	t.Parallel()

	tasks := []ValidationTask{
		{ID: "a", Value: validatableStub{str: "ok"}, Rules: []ValidationRule{NewValidationRule(RuleTypeRequired, "")}},
		{ID: "b", Value: validatableStub{empty: true}, Rules: []ValidationRule{NewValidationRule(RuleTypeRequired, "")}},
	}

	results, err := NewBatchValidator().ValidateAllConcurrent(context.Background(), tasks, 2)
	if err != nil {
		t.Fatalf("ValidateAllConcurrent returned error: %v", err)
	}
	if results[0].ID != "a" || results[1].ID != "b" {
		t.Fatalf("results order = %+v", results)
	}

	agg := Aggregate(results)
	if agg.Valid || agg.PassedTasks != 1 || agg.FailedTasks != 1 || len(agg.Failures["b"]) != 1 {
		t.Fatalf("unexpected aggregate = %+v", agg)
	}
}

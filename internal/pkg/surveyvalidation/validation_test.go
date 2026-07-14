package surveyvalidation

import "testing"

func TestValidateAppliesPublishedSpecAndRules(t *testing.T) {
	spec := Spec{Questions: []Question{
		{Code: "trigger", Type: QuestionTypeRadio, OptionCodes: []string{"yes", "no"}},
		{Code: "follow", Type: QuestionTypeText, Rules: []Rule{{Type: "required"}, {Type: "min_length", TargetValue: "2"}}, ShowController: &ShowController{Rule: "and", Conditions: []ShowCondition{{QuestionCode: "trigger", OptionCodes: []string{"yes"}}}}},
	}}
	if _, err := spec.Validate([]Answer{{QuestionCode: "trigger", QuestionType: QuestionTypeRadio, Value: "yes"}}); err == nil {
		t.Fatal("expected visible required question error")
	}
	if _, err := spec.Validate([]Answer{{QuestionCode: "trigger", QuestionType: QuestionTypeRadio, Value: "yes"}, {QuestionCode: "follow", QuestionType: QuestionTypeText, Value: "a"}}); err == nil {
		t.Fatal("expected min length error")
	}
	if _, err := spec.Validate([]Answer{{QuestionCode: "trigger", QuestionType: QuestionTypeRadio, Value: "yes"}, {QuestionCode: "follow", QuestionType: QuestionTypeText, Value: "ok"}}); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsUnsupportedRule(t *testing.T) {
	_, err := (Spec{Questions: []Question{{Code: "q", Type: QuestionTypeText, Rules: []Rule{{Type: "custom"}}}}}).Validate(nil)
	if err == nil {
		t.Fatal("expected unsupported rule error")
	}
}

func TestValidateAppliesEveryBuiltInRule(t *testing.T) {
	cases := []struct {
		name     string
		question Question
		answer   Answer
	}{
		{"required", Question{Code: "q", Type: QuestionTypeText, Rules: []Rule{{Type: "required"}}}, Answer{QuestionCode: "q", QuestionType: QuestionTypeText, Value: ""}},
		{"min length", Question{Code: "q", Type: QuestionTypeText, Rules: []Rule{{Type: "min_length", TargetValue: "2"}}}, Answer{QuestionCode: "q", QuestionType: QuestionTypeText, Value: "a"}},
		{"max length", Question{Code: "q", Type: QuestionTypeText, Rules: []Rule{{Type: "max_length", TargetValue: "2"}}}, Answer{QuestionCode: "q", QuestionType: QuestionTypeText, Value: "abc"}},
		{"min value", Question{Code: "q", Type: QuestionTypeNumber, Rules: []Rule{{Type: "min_value", TargetValue: "2"}}}, Answer{QuestionCode: "q", QuestionType: QuestionTypeNumber, Value: float64(1)}},
		{"max value", Question{Code: "q", Type: QuestionTypeNumber, Rules: []Rule{{Type: "max_value", TargetValue: "2"}}}, Answer{QuestionCode: "q", QuestionType: QuestionTypeNumber, Value: float64(3)}},
		{"min selections", Question{Code: "q", Type: QuestionTypeCheckbox, Rules: []Rule{{Type: "min_selections", TargetValue: "2"}}}, Answer{QuestionCode: "q", QuestionType: QuestionTypeCheckbox, Value: []string{"a"}}},
		{"max selections", Question{Code: "q", Type: QuestionTypeCheckbox, Rules: []Rule{{Type: "max_selections", TargetValue: "1"}}}, Answer{QuestionCode: "q", QuestionType: QuestionTypeCheckbox, Value: []string{"a", "b"}}},
		{"pattern", Question{Code: "q", Type: QuestionTypeText, Rules: []Rule{{Type: "pattern", TargetValue: `^\\d+$`}}}, Answer{QuestionCode: "q", QuestionType: QuestionTypeText, Value: "abc"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := (Spec{Questions: []Question{tc.question}}).Validate([]Answer{tc.answer}); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestDecodeAnswerValuePreservesExistingWireForms(t *testing.T) {
	radio, err := DecodeAnswerValue(QuestionTypeRadio, `{"option":"A"}`)
	if err != nil || radio != "A" {
		t.Fatalf("radio = %#v, %v", radio, err)
	}
	checkbox, err := DecodeAnswerValue(QuestionTypeCheckbox, `["A","B"]`)
	if err != nil || len(checkbox.([]string)) != 2 {
		t.Fatalf("checkbox = %#v, %v", checkbox, err)
	}
	number, err := DecodeAnswerValue(QuestionTypeNumber, `"12"`)
	if err != nil || number != float64(12) {
		t.Fatalf("number = %#v, %v", number, err)
	}
}

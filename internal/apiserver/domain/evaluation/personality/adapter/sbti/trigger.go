package sbti

import (
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
)

func triggeredDrinkOutcome(model *modeltypology.SBTILegacyModel, answers []evaluationinput.Answer) (modeltypology.SBTILegacyOutcome, bool) {
	if model == nil || len(model.DrinkTrigger.QuestionCodes) == 0 || len(model.DrinkTrigger.OptionValues) == 0 {
		return modeltypology.SBTILegacyOutcome{}, false
	}
	questionCodes := evaluationinput.StringSet(model.DrinkTrigger.QuestionCodes)
	values := evaluationinput.StringSet(model.DrinkTrigger.OptionValues)
	for _, answer := range answers {
		if !questionCodes[answer.QuestionCode] {
			continue
		}
		if values[evaluationinput.AnswerValueKey(answer.Value)] {
			outcome, ok := findOutcome(model.SpecialOutcomes, "DRUNK")
			return outcome, ok
		}
	}
	return modeltypology.SBTILegacyOutcome{}, false
}

func findOutcome(outcomes []modeltypology.SBTILegacyOutcome, code string) (modeltypology.SBTILegacyOutcome, bool) {
	for _, outcome := range outcomes {
		if outcome.Code == code {
			return outcome, true
		}
	}
	return modeltypology.SBTILegacyOutcome{}, false
}

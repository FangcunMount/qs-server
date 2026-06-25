package evaluation

import (
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	scaleinterpretation "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale/interpretation"
	rulesetscale "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/scale"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func assembleScaleInterpretationInput(input ScaleEvaluateInput) scaleinterpretation.ScaleInterpretationInput {
	return scaleinterpretation.ScaleInterpretationInput{
		Scale:         scaleModelFromSnapshot(input.Scale),
		AnswerSheet:   scaleAnswerSheetFromSnapshot(input.AnswerSheet),
		Questionnaire: scaleQuestionnaireFromSnapshot(input.Questionnaire),
	}
}

func scaleModelFromSnapshot(snapshot *rulesetscale.ScaleSnapshot) scaleinterpretation.ScaleInterpretationModel {
	if snapshot == nil {
		return scaleinterpretation.ScaleInterpretationModel{}
	}
	factors := make([]domainScale.FactorSnapshot, 0, len(snapshot.Factors))
	for _, factor := range snapshot.Factors {
		factors = append(factors, scaleFactorFromSnapshot(factor))
	}
	return scaleinterpretation.ScaleInterpretationModel{
		Code:                 snapshot.Code,
		ScaleVersion:         snapshot.ScaleVersion,
		Title:                snapshot.Title,
		QuestionnaireCode:    snapshot.QuestionnaireCode,
		QuestionnaireVersion: snapshot.QuestionnaireVersion,
		Status:               domainScale.Status(snapshot.Status),
		Factors:              factors,
	}
}

func scaleFactorFromSnapshot(snapshot rulesetscale.FactorSnapshot) domainScale.FactorSnapshot {
	questionCodes := make([]meta.Code, 0, len(snapshot.QuestionCodes))
	for _, code := range snapshot.QuestionCodes {
		questionCodes = append(questionCodes, meta.NewCode(code))
	}
	rules := make([]domainScale.InterpretationRule, 0, len(snapshot.InterpretRules))
	for _, rule := range snapshot.InterpretRules {
		rules = append(rules, domainScale.NewInterpretationRule(
			domainScale.NewScoreRange(rule.Min, rule.Max),
			domainScale.RiskLevel(rule.RiskLevel),
			rule.Conclusion,
			rule.Suggestion,
		))
	}
	return domainScale.FactorSnapshot{
		Code:            domainScale.NewFactorCode(snapshot.Code),
		Title:           snapshot.Title,
		IsTotalScore:    snapshot.IsTotalScore,
		QuestionCodes:   questionCodes,
		ScoringStrategy: domainScale.ScoringStrategyCode(snapshot.ScoringStrategy),
		ScoringParams:   domainScale.NewScoringParams().WithCntOptionContents(snapshot.ScoringParams.CntOptionContents),
		MaxScore:        cloneFloat64Ptr(snapshot.MaxScore),
		InterpretRules:  rules,
	}
}

func scaleAnswerSheetFromSnapshot(snapshot *AnswerSheet) *scaleinterpretation.ScaleAnswerSheetSnapshot {
	if snapshot == nil {
		return nil
	}
	answers := make([]scaleinterpretation.ScaleAnswerSnapshot, 0, len(snapshot.Answers))
	for _, answer := range snapshot.Answers {
		answers = append(answers, scaleinterpretation.ScaleAnswerSnapshot{
			QuestionCode: meta.NewCode(answer.QuestionCode),
			Score:        answer.Score,
			Value:        answer.Value,
		})
	}
	return &scaleinterpretation.ScaleAnswerSheetSnapshot{
		QuestionnaireCode:    snapshot.QuestionnaireCode,
		QuestionnaireVersion: snapshot.QuestionnaireVersion,
		Answers:              answers,
	}
}

func scaleQuestionnaireFromSnapshot(snapshot *Questionnaire) *scaleinterpretation.ScaleQuestionnaireSnapshot {
	if snapshot == nil {
		return nil
	}
	questions := make([]scaleinterpretation.ScaleQuestionSnapshot, 0, len(snapshot.Questions))
	for _, question := range snapshot.Questions {
		options := make([]scaleinterpretation.ScaleOptionSnapshot, 0, len(question.Options))
		for _, option := range question.Options {
			options = append(options, scaleinterpretation.ScaleOptionSnapshot{
				Code:    option.Code,
				Content: option.Content,
				Score:   option.Score,
			})
		}
		questions = append(questions, scaleinterpretation.ScaleQuestionSnapshot{
			Code:    meta.NewCode(question.Code),
			Options: options,
		})
	}
	return &scaleinterpretation.ScaleQuestionnaireSnapshot{
		Code:      snapshot.Code,
		Version:   snapshot.Version,
		Questions: questions,
	}
}

func cloneFloat64Ptr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

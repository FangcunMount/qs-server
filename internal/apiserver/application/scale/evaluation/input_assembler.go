package evaluation

import (
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type InputAssembler interface {
	FromSnapshot(snapshot *evaluationinput.InputSnapshot) domainScale.ScaleEvaluationInput
}

type DefaultInputAssembler struct{}

func (DefaultInputAssembler) FromSnapshot(snapshot *evaluationinput.InputSnapshot) domainScale.ScaleEvaluationInput {
	if snapshot == nil {
		return domainScale.ScaleEvaluationInput{}
	}
	return domainScale.ScaleEvaluationInput{
		Scale:         modelFromSnapshot(snapshot.MedicalScale),
		AnswerSheet:   answerSheetFromSnapshot(snapshot.AnswerSheet),
		Questionnaire: questionnaireFromSnapshot(snapshot.Questionnaire),
	}
}

func modelFromSnapshot(snapshot *evaluationinput.ScaleSnapshot) domainScale.ScaleEvaluationModel {
	if snapshot == nil {
		return domainScale.ScaleEvaluationModel{}
	}
	factors := make([]domainScale.FactorSnapshot, 0, len(snapshot.Factors))
	for _, factor := range snapshot.Factors {
		factors = append(factors, factorFromSnapshot(factor))
	}
	return domainScale.ScaleEvaluationModel{
		Code:                 snapshot.Code,
		Title:                snapshot.Title,
		QuestionnaireCode:    snapshot.QuestionnaireCode,
		QuestionnaireVersion: snapshot.QuestionnaireVersion,
		Status:               domainScale.Status(snapshot.Status),
		Factors:              factors,
	}
}

func factorFromSnapshot(snapshot evaluationinput.FactorSnapshot) domainScale.FactorSnapshot {
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

func answerSheetFromSnapshot(snapshot *evaluationinput.AnswerSheetSnapshot) *domainScale.ScaleAnswerSheetSnapshot {
	if snapshot == nil {
		return nil
	}
	answers := make([]domainScale.ScaleAnswerSnapshot, 0, len(snapshot.Answers))
	for _, answer := range snapshot.Answers {
		answers = append(answers, domainScale.ScaleAnswerSnapshot{
			QuestionCode: meta.NewCode(answer.QuestionCode),
			Score:        answer.Score,
			Value:        answer.Value,
		})
	}
	return &domainScale.ScaleAnswerSheetSnapshot{
		ID:                   snapshot.ID,
		QuestionnaireCode:    snapshot.QuestionnaireCode,
		QuestionnaireVersion: snapshot.QuestionnaireVersion,
		Answers:              answers,
	}
}

func questionnaireFromSnapshot(snapshot *evaluationinput.QuestionnaireSnapshot) *domainScale.ScaleQuestionnaireSnapshot {
	if snapshot == nil {
		return nil
	}
	questions := make([]domainScale.ScaleQuestionSnapshot, 0, len(snapshot.Questions))
	for _, question := range snapshot.Questions {
		options := make([]domainScale.ScaleOptionSnapshot, 0, len(question.Options))
		for _, option := range question.Options {
			options = append(options, domainScale.ScaleOptionSnapshot{
				Code:    option.Code,
				Content: option.Content,
				Score:   option.Score,
			})
		}
		questions = append(questions, domainScale.ScaleQuestionSnapshot{
			Code:    meta.NewCode(question.Code),
			Options: options,
		})
	}
	return &domainScale.ScaleQuestionnaireSnapshot{
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

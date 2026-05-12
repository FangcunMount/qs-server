package pipeline

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func scaleDomainInputFromSnapshots(
	medicalScale *evaluationinput.ScaleSnapshot,
	sheet *evaluationinput.AnswerSheetSnapshot,
	qnr *evaluationinput.QuestionnaireSnapshot,
) domainScale.ScaleEvaluationInput {
	return domainScale.ScaleEvaluationInput{
		Scale:         scaleDomainModelFromSnapshot(medicalScale),
		AnswerSheet:   scaleDomainAnswerSheetFromSnapshot(sheet),
		Questionnaire: scaleDomainQuestionnaireFromSnapshot(qnr),
	}
}

func scaleDomainModelFromSnapshot(snapshot *evaluationinput.ScaleSnapshot) domainScale.ScaleEvaluationModel {
	if snapshot == nil {
		return domainScale.ScaleEvaluationModel{}
	}
	factors := make([]domainScale.FactorSnapshot, 0, len(snapshot.Factors))
	for _, factor := range snapshot.Factors {
		factors = append(factors, scaleDomainFactorFromSnapshot(factor))
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

func scaleDomainFactorFromSnapshot(snapshot evaluationinput.FactorSnapshot) domainScale.FactorSnapshot {
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
		MaxScore:        cloneFactorMaxScore(snapshot.MaxScore),
		InterpretRules:  rules,
	}
}

func scaleDomainAnswerSheetFromSnapshot(snapshot *evaluationinput.AnswerSheetSnapshot) *domainScale.ScaleAnswerSheetSnapshot {
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

func scaleDomainQuestionnaireFromSnapshot(snapshot *evaluationinput.QuestionnaireSnapshot) *domainScale.ScaleQuestionnaireSnapshot {
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

func scaleDomainScoresFromAssessment(scores []assessment.FactorScoreResult) []domainScale.ScaleFactorScore {
	out := make([]domainScale.ScaleFactorScore, 0, len(scores))
	for _, score := range scores {
		out = append(out, domainScale.ScaleFactorScore{
			FactorCode:   domainScale.NewFactorCode(string(score.FactorCode)),
			FactorName:   score.FactorName,
			RawScore:     score.RawScore,
			RiskLevel:    domainScale.RiskLevel(score.RiskLevel),
			Conclusion:   score.Conclusion,
			Suggestion:   score.Suggestion,
			IsTotalScore: score.IsTotalScore,
		})
	}
	return out
}

func assessmentScoresFromScaleDomain(scores []domainScale.ScaleFactorScore) []assessment.FactorScoreResult {
	out := make([]assessment.FactorScoreResult, 0, len(scores))
	for _, score := range scores {
		out = append(out, assessment.NewFactorScoreResult(
			assessment.NewFactorCode(string(score.FactorCode)),
			score.FactorName,
			score.RawScore,
			assessment.RiskLevel(score.RiskLevel),
			score.Conclusion,
			score.Suggestion,
			score.IsTotalScore,
		))
	}
	return out
}

func cloneFactorMaxScore(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

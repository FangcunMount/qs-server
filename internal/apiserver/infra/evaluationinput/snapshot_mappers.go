package evaluationinput

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func scaleToSnapshot(m *scale.MedicalScale) *port.ScaleSnapshot {
	if m == nil {
		return nil
	}
	factors := make([]port.FactorSnapshot, 0, len(m.GetFactors()))
	for _, f := range m.GetFactors() {
		factors = append(factors, factorToSnapshot(f))
	}
	return &port.ScaleSnapshot{
		ID:                   m.GetID().Uint64(),
		Code:                 m.GetCode().String(),
		Title:                m.GetTitle(),
		QuestionnaireCode:    m.GetQuestionnaireCode().String(),
		QuestionnaireVersion: m.GetQuestionnaireVersion(),
		Status:               m.GetStatus().String(),
		Factors:              factors,
	}
}

func factorToSnapshot(f *scale.Factor) port.FactorSnapshot {
	if f == nil {
		return port.FactorSnapshot{}
	}
	questionCodes := make([]string, 0, len(f.GetQuestionCodes()))
	for _, code := range f.GetQuestionCodes() {
		questionCodes = append(questionCodes, code.String())
	}
	rules := make([]port.InterpretRuleSnapshot, 0, len(f.GetInterpretRules()))
	for _, rule := range f.GetInterpretRules() {
		rules = append(rules, port.InterpretRuleSnapshot{
			Min:        rule.GetScoreRange().Min(),
			Max:        rule.GetScoreRange().Max(),
			RiskLevel:  string(rule.GetRiskLevel()),
			Conclusion: rule.GetConclusion(),
			Suggestion: rule.GetSuggestion(),
		})
	}
	return port.FactorSnapshot{
		Code:            f.GetCode().String(),
		Title:           f.GetTitle(),
		IsTotalScore:    f.IsTotalScore(),
		QuestionCodes:   questionCodes,
		ScoringStrategy: f.GetScoringStrategy().String(),
		ScoringParams: port.ScoringParamsSnapshot{
			CntOptionContents: append([]string(nil), f.GetScoringParams().GetCntOptionContents()...),
		},
		MaxScore:       f.GetMaxScore(),
		InterpretRules: rules,
	}
}

func answerSheetToSnapshot(sheet *answersheet.AnswerSheet) *port.AnswerSheetSnapshot {
	if sheet == nil {
		return nil
	}
	code, version, title := sheet.QuestionnaireInfo()
	answers := make([]port.AnswerSnapshot, 0, len(sheet.Answers()))
	for _, answer := range sheet.Answers() {
		var raw any
		if answer.Value() != nil {
			raw = answer.Value().Raw()
		}
		answers = append(answers, port.AnswerSnapshot{
			QuestionCode: answer.QuestionCode(),
			Score:        answer.Score(),
			Value:        raw,
		})
	}
	return &port.AnswerSheetSnapshot{
		ID:                   sheet.ID().Uint64(),
		QuestionnaireCode:    code,
		QuestionnaireVersion: version,
		QuestionnaireTitle:   title,
		Answers:              answers,
	}
}

func questionnaireToSnapshot(qnr *questionnaire.Questionnaire) *port.QuestionnaireSnapshot {
	if qnr == nil {
		return nil
	}
	questions := make([]port.QuestionSnapshot, 0, len(qnr.GetQuestions()))
	for _, q := range qnr.GetQuestions() {
		options := make([]port.OptionSnapshot, 0, len(q.GetOptions()))
		for _, opt := range q.GetOptions() {
			options = append(options, port.OptionSnapshot{
				Code:    opt.GetCode().String(),
				Content: opt.GetContent(),
				Score:   opt.GetScore(),
			})
		}
		questions = append(questions, port.QuestionSnapshot{
			Code:    q.GetCode().String(),
			Type:    q.GetType().Value(),
			Options: options,
		})
	}
	return &port.QuestionnaireSnapshot{
		Code:      qnr.GetCode().String(),
		Version:   qnr.GetVersion().String(),
		Title:     qnr.GetTitle(),
		Questions: questions,
	}
}

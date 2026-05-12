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
	domainSnapshots := m.FactorSnapshots()
	factors := make([]port.FactorSnapshot, 0, len(domainSnapshots))
	for _, snapshot := range domainSnapshots {
		factors = append(factors, factorSnapshotToPort(snapshot))
	}
	return &port.ScaleSnapshot{
		ID:                   m.GetID().Uint64(),
		Code:                 m.GetCode().String(),
		ScaleVersion:         m.GetScaleVersion(),
		Title:                m.GetTitle(),
		QuestionnaireCode:    m.GetQuestionnaireCode().String(),
		QuestionnaireVersion: m.GetQuestionnaireVersion(),
		Status:               m.GetStatus().String(),
		Factors:              factors,
	}
}

// factorSnapshotToPort 将领域因子快照映射为评估输入端口的因子快照。
// 输入是只读的 scale.FactorSnapshot，避免直接持有领域实体指针。
func factorSnapshotToPort(snapshot scale.FactorSnapshot) port.FactorSnapshot {
	questionCodes := make([]string, 0, len(snapshot.QuestionCodes))
	for _, code := range snapshot.QuestionCodes {
		questionCodes = append(questionCodes, code.String())
	}
	rules := make([]port.InterpretRuleSnapshot, 0, len(snapshot.InterpretRules))
	for _, rule := range snapshot.InterpretRules {
		rules = append(rules, port.InterpretRuleSnapshot{
			Min:        rule.GetScoreRange().Min(),
			Max:        rule.GetScoreRange().Max(),
			RiskLevel:  string(rule.GetRiskLevel()),
			Conclusion: rule.GetConclusion(),
			Suggestion: rule.GetSuggestion(),
		})
	}
	cntContents := []string(nil)
	if snapshot.ScoringParams != nil {
		cntContents = append([]string(nil), snapshot.ScoringParams.GetCntOptionContents()...)
	}
	return port.FactorSnapshot{
		Code:            snapshot.Code.String(),
		Title:           snapshot.Title,
		IsTotalScore:    snapshot.IsTotalScore,
		QuestionCodes:   questionCodes,
		ScoringStrategy: snapshot.ScoringStrategy.String(),
		ScoringParams: port.ScoringParamsSnapshot{
			CntOptionContents: cntContents,
		},
		MaxScore:       snapshot.MaxScore,
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

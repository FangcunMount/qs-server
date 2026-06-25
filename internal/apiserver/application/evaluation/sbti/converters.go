package sbti

import (
	evaluationdomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	rulesetsbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/sbti"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func answerSheetFromPort(sheet *port.AnswerSheetSnapshot) *evaluationdomain.AnswerSheet {
	if sheet == nil {
		return nil
	}
	answers := make([]evaluationdomain.Answer, 0, len(sheet.Answers))
	for _, answer := range sheet.Answers {
		answers = append(answers, evaluationdomain.Answer{
			QuestionCode: answer.QuestionCode,
			Score:        answer.Score,
			Value:        answer.Value,
		})
	}
	return &evaluationdomain.AnswerSheet{
		QuestionnaireCode:    sheet.QuestionnaireCode,
		QuestionnaireVersion: sheet.QuestionnaireVersion,
		Answers:              answers,
	}
}

func scoreSBTI(model *rulesetsbti.ModelSnapshot, sheet *port.AnswerSheetSnapshot) (evaluationdomain.SBTIResultDetail, error) {
	return evaluationdomain.ScoreSBTI(model, answerSheetFromPort(sheet))
}

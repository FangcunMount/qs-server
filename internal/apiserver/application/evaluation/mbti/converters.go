package mbti

import (
	evaluationdomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	rulesetmbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/mbti"
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

func scoreMBTI(model *rulesetmbti.ModelSnapshot, sheet *port.AnswerSheetSnapshot) (evaluationdomain.MBTIResultDetail, error) {
	return evaluationdomain.ScoreMBTI(model, answerSheetFromPort(sheet))
}

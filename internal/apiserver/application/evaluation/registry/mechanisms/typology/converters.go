package typology

import (
	evalinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/input"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func answerSheetFromPort(sheet *port.AnswerSheetSnapshot) *evalinput.AnswerSheet {
	if sheet == nil {
		return nil
	}
	answers := make([]evalinput.Answer, 0, len(sheet.Answers))
	for _, answer := range sheet.Answers {
		answers = append(answers, evalinput.Answer{
			QuestionCode: answer.QuestionCode,
			Score:        answer.Score,
			Value:        answer.Value,
		})
	}
	return &evalinput.AnswerSheet{
		QuestionnaireCode:    sheet.QuestionnaireCode,
		QuestionnaireVersion: sheet.QuestionnaireVersion,
		Answers:              answers,
	}
}

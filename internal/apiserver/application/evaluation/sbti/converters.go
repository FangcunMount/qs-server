package sbti

import (
	rulesetsbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/sbti"
	evaluationinputdomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	evaluationsbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/sbti"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func answerSheetFromPort(sheet *port.AnswerSheetSnapshot) *evaluationinputdomain.AnswerSheet {
	if sheet == nil {
		return nil
	}
	answers := make([]evaluationinputdomain.Answer, 0, len(sheet.Answers))
	for _, answer := range sheet.Answers {
		answers = append(answers, evaluationinputdomain.Answer{
			QuestionCode: answer.QuestionCode,
			Score:        answer.Score,
			Value:        answer.Value,
		})
	}
	return &evaluationinputdomain.AnswerSheet{
		QuestionnaireCode:    sheet.QuestionnaireCode,
		QuestionnaireVersion: sheet.QuestionnaireVersion,
		Answers:              answers,
	}
}

func scoreSBTI(model *rulesetsbti.ModelSnapshot, sheet *port.AnswerSheetSnapshot) (evaluationsbti.ResultDetail, error) {
	return evaluationsbti.Score(model, answerSheetFromPort(sheet))
}

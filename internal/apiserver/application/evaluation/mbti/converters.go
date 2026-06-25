package mbti

import (
	rulesetmbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/mbti"
	evaluationinputdomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	evaluationmbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/mbti"
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

func scoreMBTI(model *rulesetmbti.ModelSnapshot, sheet *port.AnswerSheetSnapshot) (evaluationmbti.ResultDetail, error) {
	return evaluationmbti.Score(model, answerSheetFromPort(sheet))
}

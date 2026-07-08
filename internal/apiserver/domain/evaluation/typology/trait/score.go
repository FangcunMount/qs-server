package trait

import (
	calcclassification "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/classification"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
)

type FactorScore = calcclassification.FactorScore
type ProfileVector = calcclassification.ProfileVector

func ScoreGraph(g FactorGraph, sheet *evaluationinput.AnswerSheet) (ProfileVector, error) {
	return calcclassification.ScoreGraph(g, answerSheetFromEvaluation(sheet))
}

func answerSheetFromEvaluation(sheet *evaluationinput.AnswerSheet) *calcclassification.AnswerSheet {
	if sheet == nil {
		return nil
	}
	answers := make([]calcclassification.Answer, 0, len(sheet.Answers))
	for _, answer := range sheet.Answers {
		answers = append(answers, calcclassification.Answer{
			QuestionCode: answer.QuestionCode,
			Score:        answer.Score,
			Value:        answer.Value,
		})
	}
	return &calcclassification.AnswerSheet{Answers: answers}
}

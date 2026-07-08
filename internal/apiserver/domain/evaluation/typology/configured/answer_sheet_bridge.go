package configured

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/classification"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
)

func classificationAnswers(sheet *evaluationinput.AnswerSheet) []classification.Answer {
	if sheet == nil {
		return nil
	}
	answers := make([]classification.Answer, 0, len(sheet.Answers))
	for _, answer := range sheet.Answers {
		answers = append(answers, classification.Answer{
			QuestionCode: answer.QuestionCode,
			Score:        answer.Score,
			Value:        answer.Value,
		})
	}
	return answers
}

func classificationAnswerSheet(sheet *evaluationinput.AnswerSheet) *classification.AnswerSheet {
	if sheet == nil {
		return nil
	}
	return &classification.AnswerSheet{Answers: classificationAnswers(sheet)}
}

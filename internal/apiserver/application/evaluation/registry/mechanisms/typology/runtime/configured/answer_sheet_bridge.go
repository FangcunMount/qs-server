package configured

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/classification"
	evalinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/input"
)

func classificationAnswers(sheet *evalinput.AnswerSheet) []classification.Answer {
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

func classificationAnswerSheet(sheet *evalinput.AnswerSheet) *classification.AnswerSheet {
	if sheet == nil {
		return nil
	}
	return &classification.AnswerSheet{Answers: classificationAnswers(sheet)}
}

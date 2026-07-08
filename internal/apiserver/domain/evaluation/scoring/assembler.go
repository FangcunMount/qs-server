package scoring

import (
	calcscoring "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/scoring"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func assembleInterpretationInput(input EvaluateInput) calcscoring.Input {
	return calcscoring.Input{
		Model:         modelFromSnapshot(input.Scale),
		AnswerSheet:   scaleAnswerSheetFromSnapshot(input.AnswerSheet),
		Questionnaire: scaleQuestionnaireFromSnapshot(input.Questionnaire),
	}
}

func scaleAnswerSheetFromSnapshot(snapshot *evaluationinput.AnswerSheet) *calcscoring.AnswerSheet {
	if snapshot == nil {
		return nil
	}
	answers := make([]calcscoring.Answer, 0, len(snapshot.Answers))
	for _, answer := range snapshot.Answers {
		answers = append(answers, calcscoring.Answer{
			QuestionCode: meta.NewCode(answer.QuestionCode),
			Score:        answer.Score,
			Value:        answer.Value,
		})
	}
	return &calcscoring.AnswerSheet{
		QuestionnaireCode:    snapshot.QuestionnaireCode,
		QuestionnaireVersion: snapshot.QuestionnaireVersion,
		Answers:              answers,
	}
}

func scaleQuestionnaireFromSnapshot(snapshot *evaluationinput.Questionnaire) *calcscoring.Questionnaire {
	if snapshot == nil {
		return nil
	}
	questions := make([]calcscoring.Question, 0, len(snapshot.Questions))
	for _, question := range snapshot.Questions {
		options := make([]calcscoring.Option, 0, len(question.Options))
		for _, option := range question.Options {
			options = append(options, calcscoring.Option{
				Code:    option.Code,
				Content: option.Content,
				Score:   option.Score,
			})
		}
		questions = append(questions, calcscoring.Question{
			Code:    meta.NewCode(question.Code),
			Options: options,
		})
	}
	return &calcscoring.Questionnaire{
		Code:      snapshot.Code,
		Version:   snapshot.Version,
		Questions: questions,
	}
}

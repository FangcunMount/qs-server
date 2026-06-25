package scale

import (
	evaluationdomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func scaleEvaluateInputFromSnapshot(snapshot *evaluationinput.InputSnapshot) evaluationdomain.ScaleEvaluateInput {
	scaleSnapshot, _ := evaluationinput.ScalePayload(snapshot)
	return evaluationdomain.ScaleEvaluateInput{
		Scale:         scaleSnapshot,
		AnswerSheet:   answerSheetFromPort(snapshot.AnswerSheet),
		Questionnaire: questionnaireFromPort(snapshot.Questionnaire),
	}
}

func answerSheetFromPort(snapshot *evaluationinput.AnswerSheetSnapshot) *evaluationdomain.AnswerSheet {
	if snapshot == nil {
		return nil
	}
	answers := make([]evaluationdomain.Answer, 0, len(snapshot.Answers))
	for _, answer := range snapshot.Answers {
		answers = append(answers, evaluationdomain.Answer{
			QuestionCode: answer.QuestionCode,
			Score:        answer.Score,
			Value:        answer.Value,
		})
	}
	return &evaluationdomain.AnswerSheet{
		QuestionnaireCode:    snapshot.QuestionnaireCode,
		QuestionnaireVersion: snapshot.QuestionnaireVersion,
		Answers:              answers,
	}
}

func questionnaireFromPort(snapshot *evaluationinput.QuestionnaireSnapshot) *evaluationdomain.Questionnaire {
	if snapshot == nil {
		return nil
	}
	questions := make([]evaluationdomain.Question, 0, len(snapshot.Questions))
	for _, question := range snapshot.Questions {
		options := make([]evaluationdomain.Option, 0, len(question.Options))
		for _, option := range question.Options {
			options = append(options, evaluationdomain.Option{
				Code:    option.Code,
				Content: option.Content,
				Score:   option.Score,
			})
		}
		questions = append(questions, evaluationdomain.Question{
			Code:    question.Code,
			Type:    question.Type,
			Options: options,
		})
	}
	return &evaluationdomain.Questionnaire{
		Code:      snapshot.Code,
		Version:   snapshot.Version,
		Title:     snapshot.Title,
		Questions: questions,
	}
}

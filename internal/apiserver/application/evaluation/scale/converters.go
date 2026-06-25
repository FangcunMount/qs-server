package scale

import (
	evaluationinputdomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	evaluationscale "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func scaleEvaluateInputFromSnapshot(snapshot *evaluationinput.InputSnapshot) evaluationscale.EvaluateInput {
	scaleSnapshot, _ := evaluationinput.ScalePayload(snapshot)
	return evaluationscale.EvaluateInput{
		Scale:         scaleSnapshot,
		AnswerSheet:   answerSheetFromPort(snapshot.AnswerSheet),
		Questionnaire: questionnaireFromPort(snapshot.Questionnaire),
	}
}

func answerSheetFromPort(snapshot *evaluationinput.AnswerSheetSnapshot) *evaluationinputdomain.AnswerSheet {
	if snapshot == nil {
		return nil
	}
	answers := make([]evaluationinputdomain.Answer, 0, len(snapshot.Answers))
	for _, answer := range snapshot.Answers {
		answers = append(answers, evaluationinputdomain.Answer{
			QuestionCode: answer.QuestionCode,
			Score:        answer.Score,
			Value:        answer.Value,
		})
	}
	return &evaluationinputdomain.AnswerSheet{
		QuestionnaireCode:    snapshot.QuestionnaireCode,
		QuestionnaireVersion: snapshot.QuestionnaireVersion,
		Answers:              answers,
	}
}

func questionnaireFromPort(snapshot *evaluationinput.QuestionnaireSnapshot) *evaluationinputdomain.Questionnaire {
	if snapshot == nil {
		return nil
	}
	questions := make([]evaluationinputdomain.Question, 0, len(snapshot.Questions))
	for _, question := range snapshot.Questions {
		options := make([]evaluationinputdomain.Option, 0, len(question.Options))
		for _, option := range question.Options {
			options = append(options, evaluationinputdomain.Option{
				Code:    option.Code,
				Content: option.Content,
				Score:   option.Score,
			})
		}
		questions = append(questions, evaluationinputdomain.Question{
			Code:    question.Code,
			Type:    question.Type,
			Options: options,
		})
	}
	return &evaluationinputdomain.Questionnaire{
		Code:      snapshot.Code,
		Version:   snapshot.Version,
		Title:     snapshot.Title,
		Questions: questions,
	}
}

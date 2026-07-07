package scoring

import (
	evaluationinputdomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainfactor_scoring "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/scoring"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/snapshot"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func evaluateInputFromSnapshot(snapshot *evaluationinput.InputSnapshot) domainfactor_scoring.EvaluateInput {
	scaleSnapshot, _ := evaluationinput.ScalePayload(snapshot)
	return domainfactor_scoring.EvaluateInput{
		Scale:         scaleSnapshot,
		AnswerSheet:   answerSheetFromPort(snapshot.AnswerSheet),
		Questionnaire: questionnaireFromPort(snapshot.Questionnaire),
	}
}

// CloneInputWithScaleSnapshot clones an input snapshot with a scale payload substituted.
func CloneInputWithScaleSnapshot(input *evaluationinput.InputSnapshot, scaleSnapshot *scalesnapshot.ScaleSnapshot) *evaluationinput.InputSnapshot {
	if input == nil {
		return nil
	}
	cloned := *input
	if scaleSnapshot != nil {
		cloned.ModelPayload = evaluationinput.ScaleModelPayload{Scale: scaleSnapshot}
		if cloned.Model != nil {
			model := *cloned.Model
			model.Payload = evaluationinput.ScaleModelPayload{Scale: scaleSnapshot}
			cloned.Model = &model
		}
	}
	return &cloned
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

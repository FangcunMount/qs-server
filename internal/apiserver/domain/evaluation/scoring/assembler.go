package scoring

import (
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/snapshot"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func assembleInterpretationInput(input EvaluateInput) ScaleInterpretationInput {
	return ScaleInterpretationInput{
		Scale:         scaleModelFromSnapshot(input.Scale),
		AnswerSheet:   scaleAnswerSheetFromSnapshot(input.AnswerSheet),
		Questionnaire: scaleQuestionnaireFromSnapshot(input.Questionnaire),
	}
}

func scaleModelFromSnapshot(snapshot *scalesnapshot.ScaleSnapshot) ScaleInterpretationModel {
	if snapshot == nil {
		return ScaleInterpretationModel{}
	}
	return ScaleInterpretationModel{
		Code:                 snapshot.Code,
		ScaleVersion:         snapshot.ScaleVersion,
		Title:                snapshot.Title,
		QuestionnaireCode:    snapshot.QuestionnaireCode,
		QuestionnaireVersion: snapshot.QuestionnaireVersion,
		Status:               snapshot.Status,
		Factors:              append([]scalesnapshot.FactorSnapshot(nil), snapshot.Factors...),
	}
}

func scaleAnswerSheetFromSnapshot(snapshot *evaluationinput.AnswerSheet) *ScaleAnswerSheetSnapshot {
	if snapshot == nil {
		return nil
	}
	answers := make([]ScaleAnswerSnapshot, 0, len(snapshot.Answers))
	for _, answer := range snapshot.Answers {
		answers = append(answers, ScaleAnswerSnapshot{
			QuestionCode: meta.NewCode(answer.QuestionCode),
			Score:        answer.Score,
			Value:        answer.Value,
		})
	}
	return &ScaleAnswerSheetSnapshot{
		QuestionnaireCode:    snapshot.QuestionnaireCode,
		QuestionnaireVersion: snapshot.QuestionnaireVersion,
		Answers:              answers,
	}
}

func scaleQuestionnaireFromSnapshot(snapshot *evaluationinput.Questionnaire) *ScaleQuestionnaireSnapshot {
	if snapshot == nil {
		return nil
	}
	questions := make([]ScaleQuestionSnapshot, 0, len(snapshot.Questions))
	for _, question := range snapshot.Questions {
		options := make([]ScaleOptionSnapshot, 0, len(question.Options))
		for _, option := range question.Options {
			options = append(options, ScaleOptionSnapshot{
				Code:    option.Code,
				Content: option.Content,
				Score:   option.Score,
			})
		}
		questions = append(questions, ScaleQuestionSnapshot{
			Code:    meta.NewCode(question.Code),
			Options: options,
		})
	}
	return &ScaleQuestionnaireSnapshot{
		Code:      snapshot.Code,
		Version:   snapshot.Version,
		Questions: questions,
	}
}

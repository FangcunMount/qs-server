package scale

import (
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	scaleinterpretation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/scale"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/scale/snapshot"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func assembleInterpretationInput(input EvaluateInput) scaleinterpretation.ScaleInterpretationInput {
	return scaleinterpretation.ScaleInterpretationInput{
		Scale:         scaleModelFromSnapshot(input.Scale),
		AnswerSheet:   scaleAnswerSheetFromSnapshot(input.AnswerSheet),
		Questionnaire: scaleQuestionnaireFromSnapshot(input.Questionnaire),
	}
}

func scaleModelFromSnapshot(snapshot *scalesnapshot.ScaleSnapshot) scaleinterpretation.ScaleInterpretationModel {
	if snapshot == nil {
		return scaleinterpretation.ScaleInterpretationModel{}
	}
	return scaleinterpretation.ScaleInterpretationModel{
		Code:                 snapshot.Code,
		ScaleVersion:         snapshot.ScaleVersion,
		Title:                snapshot.Title,
		QuestionnaireCode:    snapshot.QuestionnaireCode,
		QuestionnaireVersion: snapshot.QuestionnaireVersion,
		Status:               snapshot.Status,
		Factors:              append([]scalesnapshot.FactorSnapshot(nil), snapshot.Factors...),
	}
}

func scaleAnswerSheetFromSnapshot(snapshot *evaluationinput.AnswerSheet) *scaleinterpretation.ScaleAnswerSheetSnapshot {
	if snapshot == nil {
		return nil
	}
	answers := make([]scaleinterpretation.ScaleAnswerSnapshot, 0, len(snapshot.Answers))
	for _, answer := range snapshot.Answers {
		answers = append(answers, scaleinterpretation.ScaleAnswerSnapshot{
			QuestionCode: meta.NewCode(answer.QuestionCode),
			Score:        answer.Score,
			Value:        answer.Value,
		})
	}
	return &scaleinterpretation.ScaleAnswerSheetSnapshot{
		QuestionnaireCode:    snapshot.QuestionnaireCode,
		QuestionnaireVersion: snapshot.QuestionnaireVersion,
		Answers:              answers,
	}
}

func scaleQuestionnaireFromSnapshot(snapshot *evaluationinput.Questionnaire) *scaleinterpretation.ScaleQuestionnaireSnapshot {
	if snapshot == nil {
		return nil
	}
	questions := make([]scaleinterpretation.ScaleQuestionSnapshot, 0, len(snapshot.Questions))
	for _, question := range snapshot.Questions {
		options := make([]scaleinterpretation.ScaleOptionSnapshot, 0, len(question.Options))
		for _, option := range question.Options {
			options = append(options, scaleinterpretation.ScaleOptionSnapshot{
				Code:    option.Code,
				Content: option.Content,
				Score:   option.Score,
			})
		}
		questions = append(questions, scaleinterpretation.ScaleQuestionSnapshot{
			Code:    meta.NewCode(question.Code),
			Options: options,
		})
	}
	return &scaleinterpretation.ScaleQuestionnaireSnapshot{
		Code:      snapshot.Code,
		Version:   snapshot.Version,
		Questions: questions,
	}
}

package dailysim

import (
	"fmt"
	"math/rand"

	toolanswersheet "github.com/FangcunMount/qs-server/tools/seeddata-runner/internal/answersheet"
)

const (
	questionTypeRadio    = toolanswersheet.QuestionTypeRadio
	questionTypeCheckbox = toolanswersheet.QuestionTypeCheckbox
	questionTypeText     = toolanswersheet.QuestionTypeText
	questionTypeTextarea = toolanswersheet.QuestionTypeTextarea
	questionTypeNumber   = toolanswersheet.QuestionTypeNumber
	questionTypeSection  = toolanswersheet.QuestionTypeSection
)

func logBuiltAnswers(logger interface{ Infow(string, ...interface{}) }, answers []Answer, questionnaireCode, testeeID string) {
	answerDetails := make([]map[string]interface{}, 0, len(answers))
	for _, answer := range answers {
		answerDetails = append(answerDetails, map[string]interface{}{
			"question_code": answer.QuestionCode,
			"question_type": answer.QuestionType,
			"value":         formatAnswerValue(answer.Value),
			"value_type":    fmt.Sprintf("%T", answer.Value),
			"score":         answer.Score,
		})
	}

	logger.Infow("Built answers",
		"questionnaire_code", questionnaireCode,
		"testee_id", testeeID,
		"answer_count", len(answers),
		"answers", answerDetails,
	)
}

func logSubmitRequest(logger interface{ Infow(string, ...interface{}) }, req SubmitAnswerSheetRequest, testeeIDStr string) {
	answerDetails := make([]map[string]interface{}, 0, len(req.Answers))
	for _, answer := range req.Answers {
		answerDetails = append(answerDetails, map[string]interface{}{
			"question_code": answer.QuestionCode,
			"question_type": answer.QuestionType,
			"value":         formatAnswerValue(answer.Value),
			"value_type":    fmt.Sprintf("%T", answer.Value),
			"score":         answer.Score,
		})
	}

	logger.Infow("Submit answer sheet request",
		"testee_id", testeeIDStr,
		"testee_id_uint64", req.TesteeID,
		"questionnaire_code", req.QuestionnaireCode,
		"questionnaire_version", req.QuestionnaireVersion,
		"title", req.Title,
		"task_id", req.TaskID,
		"answer_count", len(req.Answers),
		"answers", answerDetails,
	)
}

func validateAnswers(detail *QuestionnaireDetailResponse, answers []Answer) []map[string]interface{} {
	return toolanswersheet.Validate(toToolQuestionnaire(detail), toToolAnswers(answers))
}

func formatAnswerValue(value interface{}) string {
	return toolanswersheet.FormatValue(value)
}

func buildAnswers(detail *QuestionnaireDetailResponse, rng *rand.Rand) []Answer {
	return fromToolAnswers(toolanswersheet.BuildAnswers(toToolQuestionnaire(detail), rng))
}

func normalizeQuestionType(raw string) string {
	return toolanswersheet.NormalizeQuestionType(raw)
}

func collectQuestionTypes(detail *QuestionnaireDetailResponse) []string {
	return toolanswersheet.CollectQuestionTypes(toToolQuestionnaire(detail))
}

func resolveQuestionType(question QuestionResponse) string {
	return toolanswersheet.ResolveQuestionType(toToolQuestion(question))
}

func previewAnswers(answers []Answer) []map[string]string {
	return toolanswersheet.PreviewAnswers(toToolAnswers(answers), 3)
}

func debugLogQuestionnaire(detail *QuestionnaireDetailResponse, logger interface{ Debugw(string, ...interface{}) }) {
	questionnaire := toToolQuestionnaire(detail)
	if len(questionnaire.Questions) == 0 {
		return
	}
	logger.Debugw("Questionnaire detail preview",
		"code", questionnaire.Code,
		"title", questionnaire.Title,
		"type", questionnaire.Type,
		"question_count", len(questionnaire.Questions),
		"questions", toolanswersheet.PreviewQuestionnaire(questionnaire, 3),
	)
}

func truncateString(value string, max int) string {
	return toolanswersheet.Truncate(value, max)
}

func toToolQuestionnaire(detail *QuestionnaireDetailResponse) toolanswersheet.Questionnaire {
	if detail == nil {
		return toolanswersheet.Questionnaire{}
	}
	questions := make([]toolanswersheet.Question, 0, len(detail.Questions))
	for _, question := range detail.Questions {
		questions = append(questions, toToolQuestion(question))
	}
	return toolanswersheet.Questionnaire{
		Code:      detail.Code,
		Title:     detail.Title,
		Version:   detail.Version,
		Type:      detail.Type,
		Questions: questions,
	}
}

func toToolQuestion(question QuestionResponse) toolanswersheet.Question {
	options := make([]toolanswersheet.Option, 0, len(question.Options))
	for _, option := range question.Options {
		options = append(options, toolanswersheet.Option{
			Code:    option.Code,
			Content: option.Content,
			Score:   option.Score,
		})
	}
	return toolanswersheet.Question{
		Code:    question.Code,
		Type:    question.Type,
		Title:   question.Title,
		Options: options,
	}
}

func toToolAnswers(answers []Answer) []toolanswersheet.Answer {
	out := make([]toolanswersheet.Answer, 0, len(answers))
	for _, answer := range answers {
		out = append(out, toolanswersheet.Answer{
			QuestionCode: answer.QuestionCode,
			QuestionType: answer.QuestionType,
			Score:        answer.Score,
			Value:        answer.Value,
		})
	}
	return out
}

func fromToolAnswers(answers []toolanswersheet.Answer) []Answer {
	out := make([]Answer, 0, len(answers))
	for _, answer := range answers {
		out = append(out, Answer{
			QuestionCode: answer.QuestionCode,
			QuestionType: answer.QuestionType,
			Score:        answer.Score,
			Value:        answer.Value,
		})
	}
	return out
}

func toToolSubmitRequest(req SubmitAnswerSheetRequest) toolanswersheet.SubmitRequest {
	return toolanswersheet.SubmitRequest{
		QuestionnaireCode:    req.QuestionnaireCode,
		QuestionnaireVersion: req.QuestionnaireVersion,
		Title:                req.Title,
		TesteeID:             req.TesteeID,
		TaskID:               req.TaskID,
		Answers:              toToolAnswers(req.Answers),
	}
}

func fromToolSubmitRequest(req toolanswersheet.SubmitRequest) SubmitAnswerSheetRequest {
	return SubmitAnswerSheetRequest{
		QuestionnaireCode:    req.QuestionnaireCode,
		QuestionnaireVersion: req.QuestionnaireVersion,
		Title:                req.Title,
		TesteeID:             req.TesteeID,
		TaskID:               req.TaskID,
		Answers:              fromToolAnswers(req.Answers),
	}
}

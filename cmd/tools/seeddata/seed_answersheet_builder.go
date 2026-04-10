package main

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
)

const (
	questionTypeRadio    = "Radio"
	questionTypeCheckbox = "Checkbox"
	questionTypeText     = "Text"
	questionTypeTextarea = "Textarea"
	questionTypeNumber   = "Number"
	questionTypeSection  = "Section"
)

func logBuiltAnswers(logger interface{ Infow(string, ...interface{}) }, answers []Answer, questionnaireCode, testeeID string) {
	answerDetails := make([]map[string]interface{}, 0, len(answers))
	for _, a := range answers {
		valueStr := formatAnswerValue(a.Value)
		answerDetails = append(answerDetails, map[string]interface{}{
			"question_code": a.QuestionCode,
			"question_type": a.QuestionType,
			"value":         valueStr,
			"value_type":    fmt.Sprintf("%T", a.Value),
			"score":         a.Score,
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
	for _, a := range req.Answers {
		valueStr := formatAnswerValue(a.Value)
		answerDetails = append(answerDetails, map[string]interface{}{
			"question_code": a.QuestionCode,
			"question_type": a.QuestionType,
			"value":         valueStr,
			"value_type":    fmt.Sprintf("%T", a.Value),
			"score":         a.Score,
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
	questionMap := make(map[string]map[string]bool)
	for _, q := range detail.Questions {
		optionSet := make(map[string]bool)
		for _, opt := range q.Options {
			if opt.Code != "" {
				optionSet[opt.Code] = true
			}
			if opt.Content != "" {
				optionSet[opt.Content] = true
			}
		}
		questionMap[q.Code] = optionSet
	}

	invalidAnswers := make([]map[string]interface{}, 0)
	for _, answer := range answers {
		optionSet, exists := questionMap[answer.QuestionCode]
		if !exists {
			invalidAnswers = append(invalidAnswers, map[string]interface{}{
				"question_code": answer.QuestionCode,
				"reason":        "question not found in questionnaire",
			})
			continue
		}

		var valueStr string
		switch v := answer.Value.(type) {
		case string:
			valueStr = v
		case []string:
			for _, val := range v {
				if !optionSet[val] {
					invalidAnswers = append(invalidAnswers, map[string]interface{}{
						"question_code": answer.QuestionCode,
						"value":         val,
						"reason":        "option not found in question",
					})
				}
			}
			continue
		default:
			valueStr = formatAnswerValue(v)
		}

		if !optionSet[valueStr] {
			invalidAnswers = append(invalidAnswers, map[string]interface{}{
				"question_code":     answer.QuestionCode,
				"value":             valueStr,
				"reason":            "option not found in question",
				"available_options": getQuestionOptions(detail, answer.QuestionCode),
			})
		}
	}

	return invalidAnswers
}

func getQuestionOptions(detail *QuestionnaireDetailResponse, questionCode string) []string {
	for _, q := range detail.Questions {
		if q.Code == questionCode {
			options := make([]string, 0, len(q.Options))
			for _, opt := range q.Options {
				if opt.Code != "" {
					options = append(options, opt.Code)
				} else if opt.Content != "" {
					options = append(options, opt.Content)
				}
			}
			return options
		}
	}
	return nil
}

func formatAnswerValue(value interface{}) string {
	if value == nil {
		return "<nil>"
	}
	switch v := value.(type) {
	case string:
		return v
	case []string:
		return fmt.Sprintf("%v", v)
	case float64:
		return fmt.Sprintf("%.0f", v)
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	default:
		return fmt.Sprintf("%v (type: %T)", v, v)
	}
}

func buildAnswers(q *QuestionnaireDetailResponse, rng *rand.Rand) []Answer {
	answers := make([]Answer, 0, len(q.Questions))
	for _, question := range q.Questions {
		answer, ok := buildAnswerForQuestion(question, rng)
		if !ok {
			continue
		}
		answers = append(answers, answer)
	}
	return answers
}

func buildAnswerForQuestion(question QuestionResponse, rng *rand.Rand) (Answer, bool) {
	resolvedType := resolveQuestionType(question)
	normalizedType := normalizeQuestionType(resolvedType)

	switch normalizedType {
	case strings.ToLower(questionTypeRadio):
		if len(question.Options) == 0 {
			return Answer{}, false
		}
		opt := question.Options[rng.Intn(len(question.Options))]
		value := opt.Code
		if value == "" {
			value = opt.Content
		}
		if value == "" {
			return Answer{}, false
		}
		return Answer{
			QuestionCode: question.Code,
			QuestionType: questionTypeRadio,
			Score:        0,
			Value:        value,
		}, true

	case strings.ToLower(questionTypeCheckbox):
		if len(question.Options) == 0 {
			return Answer{}, false
		}
		count := rng.Intn(3) + 1
		if count > len(question.Options) {
			count = len(question.Options)
		}

		selectedIndices := make(map[int]bool)
		selectedValues := make([]string, 0, count)
		for len(selectedValues) < count {
			idx := rng.Intn(len(question.Options))
			if !selectedIndices[idx] {
				selectedIndices[idx] = true
				opt := question.Options[idx]
				value := opt.Code
				if value == "" {
					value = opt.Content
				}
				if value != "" {
					selectedValues = append(selectedValues, value)
				}
			}
		}

		if len(selectedValues) == 0 {
			return Answer{}, false
		}

		return Answer{
			QuestionCode: question.Code,
			QuestionType: questionTypeCheckbox,
			Score:        0,
			Value:        selectedValues,
		}, true

	case strings.ToLower(questionTypeText), strings.ToLower(questionTypeTextarea):
		texts := []string{
			"正常",
			"良好",
			"一般",
			"需要关注",
			"测试答案",
		}
		value := texts[rng.Intn(len(texts))]
		return Answer{
			QuestionCode: question.Code,
			QuestionType: resolvedType,
			Score:        0,
			Value:        value,
		}, true

	case strings.ToLower(questionTypeNumber):
		value := float64(rng.Intn(100) + 1)
		return Answer{
			QuestionCode: question.Code,
			QuestionType: questionTypeNumber,
			Score:        0,
			Value:        value,
		}, true

	case strings.ToLower(questionTypeSection):
		return Answer{}, false

	default:
		return Answer{}, false
	}
}

func normalizeQuestionType(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func collectQuestionTypes(q *QuestionnaireDetailResponse) []string {
	if q == nil || len(q.Questions) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(q.Questions))
	out := make([]string, 0, len(q.Questions))
	for _, question := range q.Questions {
		typ := strings.TrimSpace(question.Type)
		if typ == "" {
			typ = fmt.Sprintf("<empty:%s>", resolveQuestionType(question))
		}
		if _, exists := seen[typ]; exists {
			continue
		}
		seen[typ] = struct{}{}
		out = append(out, typ)
	}
	return out
}

func resolveQuestionType(question QuestionResponse) string {
	raw := normalizeQuestionType(question.Type)
	switch raw {
	case strings.ToLower(questionTypeRadio):
		return questionTypeRadio
	case strings.ToLower(questionTypeCheckbox):
		return questionTypeCheckbox
	case strings.ToLower(questionTypeText):
		return questionTypeText
	case strings.ToLower(questionTypeTextarea):
		return questionTypeTextarea
	case strings.ToLower(questionTypeNumber):
		return questionTypeNumber
	case strings.ToLower(questionTypeSection):
		return questionTypeSection
	}
	if len(question.Options) > 0 {
		return questionTypeRadio
	}
	return questionTypeSection
}

func previewAnswers(answers []Answer) []map[string]string {
	const maxPreview = 3
	if len(answers) == 0 {
		return nil
	}
	n := len(answers)
	if n > maxPreview {
		n = maxPreview
	}
	out := make([]map[string]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, map[string]string{
			"question_code": answers[i].QuestionCode,
			"value":         formatAnswerValue(answers[i].Value),
		})
	}
	return out
}

func debugLogQuestionnaire(q *QuestionnaireDetailResponse, logger interface{ Debugw(string, ...interface{}) }) {
	if q == nil || len(q.Questions) == 0 {
		return
	}
	preview := make([]map[string]string, 0, 3)
	for i, question := range q.Questions {
		if i >= 3 {
			break
		}
		preview = append(preview, map[string]string{
			"code":          question.Code,
			"type":          question.Type,
			"resolved_type": resolveQuestionType(question),
			"option_count":  strconv.Itoa(len(question.Options)),
			"title_preview": truncateString(question.Title, 30),
		})
	}
	logger.Debugw("Questionnaire detail preview",
		"code", q.Code,
		"title", q.Title,
		"type", q.Type,
		"question_count", len(q.Questions),
		"questions", preview,
	)
}

func truncateString(value string, max int) string {
	if max <= 0 || value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max]) + "..."
}

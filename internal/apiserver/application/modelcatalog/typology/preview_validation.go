package typology

import (
	"fmt"
	"strings"

	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
)

func validatePreviewAnswers(
	answers []PreviewAnswer,
	questionnaire *questionnaireapp.QuestionnaireResult,
) []ValidationIssue {
	if len(answers) == 0 {
		return []ValidationIssue{{
			Field:   "answers",
			Message: "预览答卷 answers 不能为空",
			Code:    "answers.required",
			Level:   "error",
		}}
	}

	questionOptions := questionOptionIndex(questionnaire)
	seen := make(map[string]int, len(answers))
	issues := make([]ValidationIssue, 0)

	for i, answer := range answers {
		field := fmt.Sprintf("answers[%d]", i)
		code := strings.TrimSpace(answer.QuestionCode)
		if code == "" {
			issues = append(issues, ValidationIssue{
				Field:   field + ".question_code",
				Message: "question_code 不能为空",
				Code:    "question_code.required",
				Level:   "error",
			})
			continue
		}
		if prev, ok := seen[code]; ok {
			issues = append(issues, ValidationIssue{
				Field: field + ".question_code",
				Message: fmt.Sprintf(
					"question_code %q 重复（首次出现在 answers[%d]）",
					code,
					prev,
				),
				Code:  "question_code.duplicate",
				Level: "error",
			})
		}
		seen[code] = i

		options, exists := questionOptions[code]
		if !exists {
			issues = append(issues, ValidationIssue{
				Field:   field + ".question_code",
				Message: fmt.Sprintf("question_code %q 不存在于绑定问卷", code),
				Code:    "question_code.not_found",
				Level:   "error",
			})
			continue
		}

		if !previewAnswerHasValue(answer) {
			issues = append(issues, ValidationIssue{
				Field:   field,
				Message: "value 或 score 至少提供一个",
				Code:    "answer.value_or_score.required",
				Level:   "error",
			})
			continue
		}

		if strValue, ok := answer.Value.(string); ok && strings.TrimSpace(strValue) != "" {
			if !containsOptionValue(options, strValue) {
				issues = append(issues, ValidationIssue{
					Field: field + ".value",
					Message: fmt.Sprintf(
						"value %q 不是题目 %q 的有效选项",
						strValue,
						code,
					),
					Code:  "answer.value.invalid_option",
					Level: "error",
				})
			}
		}
	}

	return issues
}

func previewAnswerHasValue(answer PreviewAnswer) bool {
	if answer.Score != nil {
		return true
	}
	if answer.Value == nil {
		return false
	}
	switch v := answer.Value.(type) {
	case string:
		return strings.TrimSpace(v) != ""
	default:
		return true
	}
}

func questionOptionIndex(questionnaire *questionnaireapp.QuestionnaireResult) map[string][]string {
	index := make(map[string][]string)
	if questionnaire == nil {
		return index
	}
	for _, question := range questionnaire.Questions {
		values := make([]string, 0, len(question.Options))
		for _, option := range question.Options {
			values = append(values, option.Value)
		}
		index[question.Code] = values
	}
	return index
}

func containsOptionValue(options []string, value string) bool {
	for _, option := range options {
		if option == value {
			return true
		}
	}
	return false
}

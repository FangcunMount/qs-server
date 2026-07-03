package answersheet

import (
	"strings"

	"github.com/FangcunMount/qs-server/internal/pkg/answervalue"
)

// AnswerConverter 将 REST 答案转换为 application 保存输入。
type AnswerConverter struct{}

func (AnswerConverter) Convert(answers []Answer) []AnswerInput {
	result := make([]AnswerInput, len(answers))
	for i, a := range answers {
		result[i] = AnswerInput{
			QuestionCode: a.QuestionCode,
			QuestionType: a.QuestionType,
			Score:        a.Score,
			Value:        normalizeAnswerValueForGRPC(a.QuestionType, a.Value),
		}
	}
	return result
}

func normalizeAnswerValueForGRPC(questionType, value string) string {
	switch strings.TrimSpace(questionType) {
	case "Radio", "radio":
		if option, ok := answervalue.NormalizeSingleOption(value); ok {
			return option
		}
	}
	return value
}

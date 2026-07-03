package answersheet

import (
	"strings"

	"github.com/FangcunMount/qs-server/internal/collection-server/port/grpcbridge"
	"github.com/FangcunMount/qs-server/internal/pkg/answervalue"
)

// AnswerConverter 将 REST 答案转换为 gRPC 输入。
type AnswerConverter struct{}

func (AnswerConverter) Convert(answers []Answer) []grpcbridge.AnswerInput {
	result := make([]grpcbridge.AnswerInput, len(answers))
	for i, a := range answers {
		result[i] = grpcbridge.AnswerInput{
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

package evaluation

import (
	"fmt"
	"strings"

	"github.com/FangcunMount/qs-server/internal/pkg/answervalue"
)

type Answer struct {
	QuestionCode string
	Score        float64
	Value        any
}

type AnswerSheet struct {
	QuestionnaireCode    string
	QuestionnaireVersion string
	Answers              []Answer
}

type Option struct {
	Code    string
	Content string
	Score   float64
}

type Question struct {
	Code    string
	Type    string
	Options []Option
}

type Questionnaire struct {
	Code      string
	Version   string
	Title     string
	Questions []Question
}

func AnswerValueKey(raw any) string {
	switch value := raw.(type) {
	case []string:
		if len(value) == 0 {
			return ""
		}
		return AnswerValueKey(value[0])
	case []any:
		if len(value) == 0 {
			return ""
		}
		return AnswerValueKey(value[0])
	default:
		if option, ok := answervalue.NormalizeSingleOption(raw); ok {
			return option
		}
		if raw == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprint(raw))
	}
}

func StringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values)*2)
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		set[trimmed] = true
		set[strings.ToUpper(trimmed)] = true
	}
	return set
}

func AbsInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

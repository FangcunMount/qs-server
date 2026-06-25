package evaluation

import (
	"fmt"
	"strings"
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

func answerValueKey(raw any) string {
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	case fmt.Stringer:
		return strings.TrimSpace(value.String())
	case []string:
		if len(value) == 0 {
			return ""
		}
		return strings.TrimSpace(value[0])
	case []any:
		if len(value) == 0 {
			return ""
		}
		return answerValueKey(value[0])
	default:
		if raw == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprint(raw))
	}
}

func stringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values)*2)
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		set[trimmed] = true
		set[strings.ToUpper(trimmed)] = true
	}
	return set
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

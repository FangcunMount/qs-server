package input

import (
	"fmt"
	"strings"

	"github.com/FangcunMount/qs-server/internal/pkg/answervalue"
)

// AnswerValueKey normalizes a raw answer value into a stable option key.
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

// StringSet builds a case-insensitive lookup set from string values.
func StringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values)*2)
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		set[trimmed] = true
		set[strings.ToUpper(trimmed)] = true
	}
	return set
}

// AbsInt returns the absolute value of an integer.
func AbsInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

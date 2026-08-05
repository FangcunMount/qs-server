package input

import (
	"fmt"
	"strings"

	"github.com/FangcunMount/qs-server/internal/pkg/answervalue"
)

// AnswerValueKey 归一化原始 answer value 为 稳定 选项键。
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

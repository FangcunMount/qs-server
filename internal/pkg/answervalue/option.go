package answervalue

import (
	"encoding/json"
	"fmt"
	"strings"
)

type optionWrapper struct {
	Option string `json:"option"`
}

// NormalizeSingleOption unwraps documented single-choice payloads such as "5" or {"option":"5"}.
func NormalizeSingleOption(raw any) (string, bool) {
	switch value := raw.(type) {
	case string:
		return normalizeOptionString(value)
	case fmt.Stringer:
		return normalizeOptionString(value.String())
	case json.Number:
		return strings.TrimSpace(value.String()), true
	case map[string]any:
		return normalizeOptionMap(value)
	case map[string]string:
		if option, ok := value["option"]; ok {
			return strings.TrimSpace(option), strings.TrimSpace(option) != ""
		}
	}
	return "", false
}

func normalizeOptionString(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false
	}

	var asString string
	if err := json.Unmarshal([]byte(trimmed), &asString); err == nil {
		trimmed = strings.TrimSpace(asString)
		if trimmed == "" {
			return "", false
		}
	}

	var wrapped optionWrapper
	if err := json.Unmarshal([]byte(trimmed), &wrapped); err == nil {
		option := strings.TrimSpace(wrapped.Option)
		if option != "" {
			return option, true
		}
	}

	return trimmed, true
}

func normalizeOptionMap(values map[string]any) (string, bool) {
	option, ok := values["option"]
	if !ok {
		return "", false
	}
	return normalizeScalarOption(option)
}

func normalizeScalarOption(raw any) (string, bool) {
	switch value := raw.(type) {
	case string:
		trimmed := strings.TrimSpace(value)
		return trimmed, trimmed != ""
	case fmt.Stringer:
		return normalizeScalarOption(value.String())
	case json.Number:
		trimmed := strings.TrimSpace(value.String())
		return trimmed, trimmed != ""
	case float64:
		return strings.TrimSpace(fmt.Sprintf("%g", value)), true
	case int:
		return fmt.Sprintf("%d", value), true
	case int64:
		return fmt.Sprintf("%d", value), true
	default:
		if raw == nil {
			return "", false
		}
		trimmed := strings.TrimSpace(fmt.Sprint(raw))
		return trimmed, trimmed != ""
	}
}

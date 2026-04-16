package configmask

import (
	"encoding/json"
	"fmt"
	"strings"
)

var sensitiveFields = map[string]struct{}{
	"access_token":  {},
	"api_key":       {},
	"apikey":        {},
	"app_secret":    {},
	"authorization": {},
	"client_secret": {},
	"jwt_secret":    {},
	"key":           {},
	"password":      {},
	"private_key":   {},
	"refresh_token": {},
	"secret":        {},
	"secret_key":    {},
	"shared_secret": {},
	"token":         {},
}

// String marshals a config-like value to JSON after masking sensitive fields.
func String(v interface{}) string {
	masked := Sanitize(v)

	data, err := json.Marshal(masked)
	if err != nil {
		return fmt.Sprintf("%+v", masked)
	}

	return string(data)
}

// Sanitize masks sensitive values in arbitrarily nested config data.
func Sanitize(v interface{}) interface{} {
	data, err := marshalToGeneric(v)
	if err != nil {
		return v
	}

	return sanitizeRecursive(data)
}

// MaskEnvValue masks a single environment variable value when the key is sensitive.
func MaskEnvValue(key, value string) string {
	if value == "" || !isSensitiveField(key) {
		return value
	}

	return maskValue(value)
}

func marshalToGeneric(v interface{}) (interface{}, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	var generic interface{}
	if err := json.Unmarshal(data, &generic); err != nil {
		return nil, err
	}

	return generic, nil
}

func sanitizeRecursive(v interface{}) interface{} {
	switch value := v.(type) {
	case map[string]interface{}:
		for key, item := range value {
			if isSensitiveField(key) {
				value[key] = maskedPlaceholder(item)
				continue
			}

			value[key] = sanitizeRecursive(item)
		}
		return value
	case []interface{}:
		for i, item := range value {
			value[i] = sanitizeRecursive(item)
		}
		return value
	default:
		return value
	}
}

func maskedPlaceholder(value interface{}) interface{} {
	if str, ok := value.(string); ok {
		return maskValue(str)
	}
	if value == nil {
		return nil
	}
	return "***"
}

func maskValue(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "***"
	}
	return value[:4] + "***" + value[len(value)-4:]
}

func isSensitiveField(field string) bool {
	normalized := normalizeField(field)
	if _, ok := sensitiveFields[normalized]; ok {
		return true
	}

	return strings.HasSuffix(normalized, "_password") ||
		strings.HasSuffix(normalized, "_secret") ||
		strings.HasSuffix(normalized, "_token")
}

func normalizeField(field string) string {
	normalized := strings.ToLower(field)
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, ".", "_")
	return normalized
}

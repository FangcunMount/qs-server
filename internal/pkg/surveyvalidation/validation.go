// Package surveyvalidation owns the deterministic validation contract for a
// published questionnaire submission. It intentionally depends only on the
// published question projection so both the BFF and apiserver can apply the
// exact same policy.
package surveyvalidation

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/FangcunMount/qs-server/internal/pkg/answervalue"
)

const (
	QuestionTypeSection  = "Section"
	QuestionTypeRadio    = "Radio"
	QuestionTypeCheckbox = "Checkbox"
	QuestionTypeText     = "Text"
	QuestionTypeTextarea = "Textarea"
	QuestionTypeNumber   = "Number"
)

// Rule is a configured question validation rule.
type Rule struct {
	Type        string
	TargetValue string
}

// ShowCondition defines one question/option condition for conditional display.
type ShowCondition struct {
	QuestionCode string
	OptionCodes  []string
}

// ShowController controls whether a question is visible for a submission.
type ShowController struct {
	Rule       string
	Conditions []ShowCondition
}

// Question is the minimum published-question projection needed for validation.
type Question struct {
	Code           string
	Type           string
	OptionCodes    []string
	Rules          []Rule
	ShowController *ShowController
}

// Spec is the executable submission contract for one published questionnaire version.
type Spec struct {
	QuestionnaireCode    string
	QuestionnaireVersion string
	Questions            []Question
}

// Answer is a decoded answer submitted by a client.
type Answer struct {
	QuestionCode string
	QuestionType string
	Value        any
}

// PreparedAnswer is a normalized answer accepted by the published submission spec.
type PreparedAnswer struct {
	QuestionCode string
	QuestionType string
	Value        any
	Rules        []Rule
}

// ErrorKind lets transports distinguish invalid client input from invalid published configuration.
type ErrorKind string

const (
	ErrorInvalidInput    ErrorKind = "invalid_input"
	ErrorInvalidConfig   ErrorKind = "invalid_configuration"
	ErrorUnsupportedRule ErrorKind = "unsupported_rule"
)

// Error is a stable validation failure.
type Error struct {
	Kind    ErrorKind
	Message string
}

func (e *Error) Error() string { return e.Message }

func invalid(format string, args ...any) error {
	return &Error{Kind: ErrorInvalidInput, Message: fmt.Sprintf(format, args...)}
}

func invalidConfig(kind ErrorKind, format string, args ...any) error {
	return &Error{Kind: kind, Message: fmt.Sprintf(format, args...)}
}

// IsSupportedRule reports whether a rule can be executed by both submission layers.
func IsSupportedRule(ruleType string) bool {
	switch ruleType {
	case "required", "min_length", "max_length", "min_value", "max_value", "min_selections", "max_selections", "pattern":
		return true
	default:
		return false
	}
}

// DecodeAnswerValue decodes the existing gRPC answer wire format into the
// normalized runtime value used by validation and answer persistence.
func DecodeAnswerValue(questionType, raw string) (any, error) {
	switch questionType {
	case QuestionTypeCheckbox:
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return []string{}, nil
		}
		var values []string
		if err := json.Unmarshal([]byte(raw), &values); err == nil {
			return values, nil
		}
		return []string{raw}, nil
	case QuestionTypeNumber:
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return nil, fmt.Errorf("empty numeric answer")
		}
		var value float64
		if err := json.Unmarshal([]byte(raw), &value); err == nil {
			return value, nil
		}
		var encoded string
		if err := json.Unmarshal([]byte(raw), &encoded); err == nil {
			raw = encoded
		}
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, fmt.Errorf("expected numeric value, got %q", raw)
		}
		return value, nil
	default:
		var value string
		if err := json.Unmarshal([]byte(raw), &value); err == nil {
			if option, ok := answervalue.NormalizeSingleOption(value); ok {
				return option, nil
			}
			return value, nil
		}
		if option, ok := answervalue.NormalizeSingleOption(raw); ok {
			return option, nil
		}
		return raw, nil
	}
}

// Validate checks all schema and configured rule constraints, returning only
// normalized answers that may be persisted.
func (s Spec) Validate(rawAnswers []Answer) ([]PreparedAnswer, error) {
	questions := make(map[string]Question, len(s.Questions))
	for _, question := range s.Questions {
		if question.Code == "" {
			return nil, invalidConfig(ErrorInvalidConfig, "question code cannot be empty")
		}
		for _, rule := range question.Rules {
			if !IsSupportedRule(rule.Type) {
				return nil, invalidConfig(ErrorUnsupportedRule, "question %s uses unsupported validation rule %s", question.Code, rule.Type)
			}
		}
		questions[question.Code] = question
	}

	prepared := make([]PreparedAnswer, 0, len(rawAnswers))
	values := make(map[string]any, len(rawAnswers))
	for _, raw := range rawAnswers {
		code := strings.TrimSpace(raw.QuestionCode)
		if code == "" {
			return nil, invalid("question code cannot be empty")
		}
		question, ok := questions[code]
		if !ok {
			return nil, invalid("question %s is not in questionnaire", code)
		}
		if strings.TrimSpace(raw.QuestionType) == "" {
			return nil, invalid("question %s type cannot be empty", code)
		}
		if raw.QuestionType != question.Type {
			return nil, invalid("question %s type mismatch: got %s, want %s", code, raw.QuestionType, question.Type)
		}
		if err := validateOptionSelection(question, raw.Value); err != nil {
			return nil, err
		}
		values[code] = raw.Value
		prepared = append(prepared, PreparedAnswer{QuestionCode: question.Code, QuestionType: question.Type, Value: raw.Value, Rules: append([]Rule(nil), question.Rules...)})
	}

	for _, question := range questions {
		if question.Type == QuestionTypeSection || !isVisible(question, values) || !hasRequiredRule(question.Rules) {
			continue
		}
		value, ok := values[question.Code]
		if !ok {
			return nil, invalid("required question %s is missing", question.Code)
		}
		if isEmpty(value) {
			return nil, invalid("required question %s cannot be empty", question.Code)
		}
	}

	for _, answer := range prepared {
		for _, rule := range answer.Rules {
			if err := validateRule(answer.Value, rule); err != nil {
				return nil, invalid("答案验证失败: [%s: %s]", answer.QuestionCode, err.Error())
			}
		}
	}
	return prepared, nil
}

func validateOptionSelection(question Question, raw any) error {
	if len(question.OptionCodes) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(question.OptionCodes))
	for _, code := range question.OptionCodes {
		allowed[code] = struct{}{}
	}
	switch question.Type {
	case QuestionTypeRadio:
		option, ok := answervalue.NormalizeSingleOption(raw)
		if !ok {
			return invalid("question %s expects a single option value", question.Code)
		}
		if _, ok := allowed[option]; !ok {
			return invalid("question %s option %s is not allowed", question.Code, option)
		}
	case QuestionTypeCheckbox:
		options, ok := answervalue.NormalizeMultiOptions(raw)
		if !ok {
			return invalid("question %s expects option list value", question.Code)
		}
		for _, option := range options {
			if _, ok := allowed[option]; !ok {
				return invalid("question %s option %s is not allowed", question.Code, option)
			}
		}
	}
	return nil
}

func hasRequiredRule(rules []Rule) bool {
	for _, rule := range rules {
		if rule.Type == "required" {
			return true
		}
	}
	return false
}

func isVisible(question Question, values map[string]any) bool {
	controller := question.ShowController
	if controller == nil || controller.Rule == "" || len(controller.Conditions) == 0 {
		return true
	}
	matched := make([]bool, 0, len(controller.Conditions))
	for _, condition := range controller.Conditions {
		value, ok := values[condition.QuestionCode]
		matched = append(matched, ok && matchesCondition(value, condition.OptionCodes))
	}
	if strings.EqualFold(strings.TrimSpace(controller.Rule), "or") {
		for _, result := range matched {
			if result {
				return true
			}
		}
		return false
	}
	for _, result := range matched {
		if !result {
			return false
		}
	}
	return true
}

func matchesCondition(value any, expected []string) bool {
	if len(expected) == 0 {
		return false
	}
	if option, ok := answervalue.NormalizeSingleOption(value); ok {
		for _, code := range expected {
			if option == code {
				return true
			}
		}
		return false
	}
	if options, ok := answervalue.NormalizeMultiOptions(value); ok {
		for _, option := range options {
			for _, code := range expected {
				if option == code {
					return true
				}
			}
		}
	}
	return false
}

func isEmpty(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(v) == ""
	case []string:
		return len(v) == 0
	case []any:
		return len(v) == 0
	default:
		if option, ok := answervalue.NormalizeSingleOption(v); ok {
			return strings.TrimSpace(option) == ""
		}
		if options, ok := answervalue.NormalizeMultiOptions(v); ok {
			return len(options) == 0
		}
		return false
	}
}

func validateRule(value any, rule Rule) error {
	if rule.Type == "required" {
		if isEmpty(value) {
			return fmt.Errorf("该字段为必填项")
		}
		return nil
	}
	if isEmpty(value) {
		return nil
	}
	switch rule.Type {
	case "min_length", "max_length":
		limit, err := strconv.Atoi(rule.TargetValue)
		if err != nil {
			return fmt.Errorf("invalid %s rule value: %s", rule.Type, rule.TargetValue)
		}
		length := utf8.RuneCountInString(asString(value))
		if rule.Type == "min_length" && length < limit {
			return fmt.Errorf("字符数不得少于 %d 个", limit)
		}
		if rule.Type == "max_length" && length > limit {
			return fmt.Errorf("字符数不得超过 %d 个", limit)
		}
	case "min_value", "max_value":
		limit, err := strconv.ParseFloat(rule.TargetValue, 64)
		if err != nil {
			return fmt.Errorf("invalid %s rule value: %s", rule.Type, rule.TargetValue)
		}
		actual, err := asNumber(value)
		if err != nil {
			return fmt.Errorf("无法将值转换为数字: %v", err)
		}
		if rule.Type == "min_value" && actual < limit {
			return fmt.Errorf("值不得小于 %v", limit)
		}
		if rule.Type == "max_value" && actual > limit {
			return fmt.Errorf("值不得大于 %v", limit)
		}
	case "min_selections", "max_selections":
		limit, err := strconv.Atoi(rule.TargetValue)
		if err != nil {
			return fmt.Errorf("invalid %s rule value: %s", rule.Type, rule.TargetValue)
		}
		count := len(asArray(value))
		if rule.Type == "min_selections" && count < limit {
			return fmt.Errorf("至少需要选择 %d 项", limit)
		}
		if rule.Type == "max_selections" && count > limit {
			return fmt.Errorf("最多只能选择 %d 项", limit)
		}
	case "pattern":
		if rule.TargetValue == "" {
			return fmt.Errorf("pattern rule requires a non-empty pattern")
		}
		regex, err := regexp.Compile(rule.TargetValue)
		if err != nil {
			return fmt.Errorf("invalid pattern: %v", err)
		}
		if !regex.MatchString(asString(value)) {
			return fmt.Errorf("输入格式不正确")
		}
	}
	return nil
}

func asString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case []string:
		if len(v) > 0 {
			return v[0]
		}
	}
	return fmt.Sprintf("%v", value)
}

func asNumber(value any) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case string:
		value, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, fmt.Errorf("cannot convert string '%s' to number: %w", v, err)
		}
		return value, nil
	default:
		return 0, fmt.Errorf("cannot convert type %T to number", value)
	}
}

func asArray(value any) []string {
	switch v := value.(type) {
	case []string:
		return append([]string(nil), v...)
	case string:
		if v != "" {
			return []string{v}
		}
	}
	return []string{}
}

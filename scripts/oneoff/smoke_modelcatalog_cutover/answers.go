package main

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

func buildAnswers(questions []question) ([]answer, error) {
	answers := make([]answer, 0, len(questions))
	for index, item := range questions {
		built, include, err := buildAnswer(item, index)
		if err != nil {
			return nil, fmt.Errorf("question %s: %w", item.Code, err)
		}
		if include {
			answers = append(answers, built)
		}
	}
	if len(answers) == 0 {
		return nil, fmt.Errorf("questionnaire produced no answers")
	}
	return answers, nil
}

func buildAnswer(item question, index int) (answer, bool, error) {
	if strings.TrimSpace(item.Code) == "" {
		return answer{}, false, fmt.Errorf("question code is empty")
	}
	kind := normalizeQuestionType(item)
	switch kind {
	case "section":
		return answer{}, false, nil
	case "radio":
		if len(item.Options) == 0 {
			return answer{}, false, fmt.Errorf("radio question has no options")
		}
		value := optionValue(item.Options[index%len(item.Options)])
		if value == "" {
			return answer{}, false, fmt.Errorf("selected option has no code or content")
		}
		return newAnswer(item.Code, "Radio", value), true, nil
	case "checkbox":
		if len(item.Options) == 0 {
			return answer{}, false, fmt.Errorf("checkbox question has no options")
		}
		minimum := maxInt(1, intRule(item, "min_selections", 1))
		maximum := intRule(item, "max_selections", len(item.Options))
		if maximum <= 0 || maximum > len(item.Options) {
			maximum = len(item.Options)
		}
		if minimum > maximum || minimum > len(item.Options) {
			return answer{}, false, fmt.Errorf("invalid selection rules min=%d max=%d options=%d", minimum, maximum, len(item.Options))
		}
		values := make([]string, 0, minimum)
		for offset := 0; offset < len(item.Options) && len(values) < minimum; offset++ {
			value := optionValue(item.Options[(index+offset)%len(item.Options)])
			if value != "" {
				values = append(values, value)
			}
		}
		if len(values) != minimum {
			return answer{}, false, fmt.Errorf("not enough non-empty options")
		}
		encoded, _ := json.Marshal(values)
		return newAnswer(item.Code, "Checkbox", string(encoded)), true, nil
	case "text", "textarea":
		value, err := buildTextValue(item, index)
		if err != nil {
			return answer{}, false, err
		}
		questionType := "Text"
		if kind == "textarea" {
			questionType = "Textarea"
		}
		return newAnswer(item.Code, questionType, value), true, nil
	case "number":
		minimum := floatRule(item, "min_value", 1)
		maximum := floatRule(item, "max_value", 100)
		if maximum < minimum {
			return answer{}, false, fmt.Errorf("invalid number range %v..%v", minimum, maximum)
		}
		value := minimum
		if width := math.Floor(maximum-minimum) + 1; width > 1 {
			value += float64(index % int(width))
		}
		return newAnswer(item.Code, "Number", strconv.FormatFloat(value, 'f', -1, 64)), true, nil
	default:
		return answer{}, false, fmt.Errorf("unsupported question type %q", item.Type)
	}
}

func normalizeQuestionType(item question) string {
	value := strings.ToLower(strings.TrimSpace(item.Type))
	switch value {
	case "radio", "checkbox", "text", "textarea", "number", "section":
		return value
	}
	if len(item.Options) > 0 {
		return "radio"
	}
	return value
}

func buildTextValue(item question, index int) (string, error) {
	minimum := maxInt(2, intRule(item, "min_length", 2))
	maximum := intRule(item, "max_length", 0)
	if maximum > 0 && maximum < minimum {
		minimum = maximum
	}
	pattern := stringRule(item, "pattern", "")
	candidates := []string{"情况稳定", "状态良好", "需要关注", "测试填写", "学习正常", "睡眠正常", "情绪平稳", "测试123", "123456", "13812345678", "test@example.com"}
	for offset := range candidates {
		candidate := normalizeTextLength(candidates[(index+offset)%len(candidates)], minimum, maximum)
		if pattern == "" {
			return candidate, nil
		}
		matched, err := regexp.MatchString(pattern, candidate)
		if err == nil && matched {
			return candidate, nil
		}
	}
	if pattern != "" {
		return "", fmt.Errorf("cannot synthesize a value matching pattern %q", pattern)
	}
	return normalizeTextLength(strings.Repeat("测", minimum), minimum, maximum), nil
}

func normalizeTextLength(value string, minimum, maximum int) string {
	for utf8.RuneCountInString(value) < minimum {
		value += "测"
	}
	runes := []rune(value)
	if maximum > 0 && len(runes) > maximum {
		value = string(runes[:maximum])
	}
	return value
}

func newAnswer(questionCode, questionType, value string) answer {
	return answer{QuestionCode: questionCode, QuestionType: questionType, Value: value}
}

func optionValue(option questionOption) string {
	if value := strings.TrimSpace(option.Code); value != "" {
		return value
	}
	return strings.TrimSpace(option.Content)
}

func intRule(item question, name string, fallback int) int {
	value := stringRule(item, name, "")
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func floatRule(item question, name string, fallback float64) float64 {
	value := stringRule(item, name, "")
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func stringRule(item question, name, fallback string) string {
	for _, rule := range item.ValidationRules {
		if strings.TrimSpace(rule.RuleType) == name {
			return strings.TrimSpace(rule.TargetValue)
		}
	}
	return fallback
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

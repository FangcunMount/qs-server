package answersheet

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"
)

const (
	QuestionTypeRadio    = "Radio"
	QuestionTypeCheckbox = "Checkbox"
	QuestionTypeText     = "Text"
	QuestionTypeTextarea = "Textarea"
	QuestionTypeNumber   = "Number"
	QuestionTypeSection  = "Section"
)

type Questionnaire struct {
	Code      string
	Title     string
	Version   string
	Type      string
	Questions []Question
}

type Question struct {
	Code    string
	Type    string
	Title   string
	Options []Option
}

type Option struct {
	Code    string
	Content string
	Score   int32
}

type Answer struct {
	QuestionCode string
	QuestionType string
	Score        uint32
	Value        interface{}
}

type SubmitRequest struct {
	QuestionnaireCode    string
	QuestionnaireVersion string
	Title                string
	TesteeID             uint64
	TaskID               string
	Answers              []Answer
}

type SubmitPolicy struct {
	Timeout      time.Duration
	HTTPRetryMax int
	MaxAttempts  int
	RetryBackoff time.Duration
	Retryable    func(error) bool
}

func SubmitWithRetry[T any](
	ctx context.Context,
	req T,
	policy SubmitPolicy,
	submit func(context.Context, T, time.Duration, int) error,
) (int, error) {
	maxAttempts := policy.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	var lastErr error
	attempts := 0
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if ctx.Err() != nil {
			return attempts, ctx.Err()
		}

		attempts++
		if err := submit(ctx, req, policy.Timeout, policy.HTTPRetryMax); err == nil {
			return attempts, nil
		} else {
			lastErr = err
		}

		if attempt == maxAttempts-1 {
			break
		}
		if policy.Retryable != nil && !policy.Retryable(lastErr) {
			break
		}
		if policy.RetryBackoff <= 0 {
			continue
		}
		if err := sleepWithContext(ctx, policy.RetryBackoff*time.Duration(attempt+1)); err != nil {
			return attempts, err
		}
	}

	return attempts, lastErr
}

func BuildAnswers(q Questionnaire, rng *rand.Rand) []Answer {
	answers := make([]Answer, 0, len(q.Questions))
	for _, question := range q.Questions {
		answer, ok := BuildAnswerForQuestion(question, rng)
		if !ok {
			continue
		}
		answers = append(answers, answer)
	}
	return answers
}

func BuildAnswerForQuestion(question Question, rng *rand.Rand) (Answer, bool) {
	resolvedType := ResolveQuestionType(question)
	normalizedType := NormalizeQuestionType(resolvedType)

	switch normalizedType {
	case strings.ToLower(QuestionTypeRadio):
		if len(question.Options) == 0 {
			return Answer{}, false
		}
		opt := question.Options[rng.Intn(len(question.Options))]
		value := opt.Code
		if value == "" {
			value = opt.Content
		}
		if value == "" {
			return Answer{}, false
		}
		return Answer{
			QuestionCode: question.Code,
			QuestionType: QuestionTypeRadio,
			Score:        0,
			Value:        value,
		}, true

	case strings.ToLower(QuestionTypeCheckbox):
		if len(question.Options) == 0 {
			return Answer{}, false
		}
		count := rng.Intn(3) + 1
		if count > len(question.Options) {
			count = len(question.Options)
		}

		selectedIndices := make(map[int]bool)
		selectedValues := make([]string, 0, count)
		for len(selectedValues) < count {
			idx := rng.Intn(len(question.Options))
			if selectedIndices[idx] {
				continue
			}
			selectedIndices[idx] = true
			opt := question.Options[idx]
			value := opt.Code
			if value == "" {
				value = opt.Content
			}
			if value != "" {
				selectedValues = append(selectedValues, value)
			}
		}
		if len(selectedValues) == 0 {
			return Answer{}, false
		}
		return Answer{
			QuestionCode: question.Code,
			QuestionType: QuestionTypeCheckbox,
			Score:        0,
			Value:        selectedValues,
		}, true

	case strings.ToLower(QuestionTypeText), strings.ToLower(QuestionTypeTextarea):
		texts := []string{"正常", "良好", "一般", "需要关注", "测试答案"}
		return Answer{
			QuestionCode: question.Code,
			QuestionType: resolvedType,
			Score:        0,
			Value:        texts[rng.Intn(len(texts))],
		}, true

	case strings.ToLower(QuestionTypeNumber):
		return Answer{
			QuestionCode: question.Code,
			QuestionType: QuestionTypeNumber,
			Score:        0,
			Value:        float64(rng.Intn(100) + 1),
		}, true

	case strings.ToLower(QuestionTypeSection):
		return Answer{}, false
	default:
		return Answer{}, false
	}
}

func Validate(q Questionnaire, answers []Answer) []map[string]interface{} {
	questionMap := make(map[string]map[string]bool)
	for _, question := range q.Questions {
		optionSet := make(map[string]bool)
		for _, option := range question.Options {
			if option.Code != "" {
				optionSet[option.Code] = true
			}
			if option.Content != "" {
				optionSet[option.Content] = true
			}
		}
		questionMap[question.Code] = optionSet
	}

	invalidAnswers := make([]map[string]interface{}, 0)
	for _, answer := range answers {
		optionSet, exists := questionMap[answer.QuestionCode]
		if !exists {
			invalidAnswers = append(invalidAnswers, map[string]interface{}{
				"question_code": answer.QuestionCode,
				"reason":        "question not found in questionnaire",
			})
			continue
		}

		var valueStr string
		switch v := answer.Value.(type) {
		case string:
			valueStr = v
		case []string:
			for _, val := range v {
				if !optionSet[val] {
					invalidAnswers = append(invalidAnswers, map[string]interface{}{
						"question_code": answer.QuestionCode,
						"value":         val,
						"reason":        "option not found in question",
					})
				}
			}
			continue
		default:
			valueStr = FormatValue(v)
		}

		if !optionSet[valueStr] {
			invalidAnswers = append(invalidAnswers, map[string]interface{}{
				"question_code":     answer.QuestionCode,
				"value":             valueStr,
				"reason":            "option not found in question",
				"available_options": questionOptions(q, answer.QuestionCode),
			})
		}
	}
	return invalidAnswers
}

func FormatValue(value interface{}) string {
	if value == nil {
		return "<nil>"
	}
	switch v := value.(type) {
	case string:
		return v
	case []string:
		return fmt.Sprintf("%v", v)
	case float64:
		return fmt.Sprintf("%.0f", v)
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	default:
		return fmt.Sprintf("%v (type: %T)", v, v)
	}
}

func NormalizeQuestionType(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func ResolveQuestionType(question Question) string {
	switch NormalizeQuestionType(question.Type) {
	case strings.ToLower(QuestionTypeRadio):
		return QuestionTypeRadio
	case strings.ToLower(QuestionTypeCheckbox):
		return QuestionTypeCheckbox
	case strings.ToLower(QuestionTypeText):
		return QuestionTypeText
	case strings.ToLower(QuestionTypeTextarea):
		return QuestionTypeTextarea
	case strings.ToLower(QuestionTypeNumber):
		return QuestionTypeNumber
	case strings.ToLower(QuestionTypeSection):
		return QuestionTypeSection
	}
	if len(question.Options) > 0 {
		return QuestionTypeRadio
	}
	return QuestionTypeSection
}

func CollectQuestionTypes(q Questionnaire) []string {
	if len(q.Questions) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(q.Questions))
	out := make([]string, 0, len(q.Questions))
	for _, question := range q.Questions {
		typ := strings.TrimSpace(question.Type)
		if typ == "" {
			typ = fmt.Sprintf("<empty:%s>", ResolveQuestionType(question))
		}
		if _, exists := seen[typ]; exists {
			continue
		}
		seen[typ] = struct{}{}
		out = append(out, typ)
	}
	return out
}

func PreviewAnswers(answers []Answer, max int) []map[string]string {
	if len(answers) == 0 || max <= 0 {
		return nil
	}
	if len(answers) < max {
		max = len(answers)
	}
	out := make([]map[string]string, 0, max)
	for i := 0; i < max; i++ {
		out = append(out, map[string]string{
			"question_code": answers[i].QuestionCode,
			"value":         FormatValue(answers[i].Value),
		})
	}
	return out
}

func questionOptions(q Questionnaire, questionCode string) []string {
	for _, question := range q.Questions {
		if question.Code != questionCode {
			continue
		}
		options := make([]string, 0, len(question.Options))
		for _, option := range question.Options {
			if option.Code != "" {
				options = append(options, option.Code)
			} else if option.Content != "" {
				options = append(options, option.Content)
			}
		}
		return options
	}
	return nil
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func Truncate(value string, max int) string {
	if max <= 0 || value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

func PreviewQuestionnaire(q Questionnaire, max int) []map[string]string {
	if len(q.Questions) == 0 || max <= 0 {
		return nil
	}
	if len(q.Questions) < max {
		max = len(q.Questions)
	}
	out := make([]map[string]string, 0, max)
	for i := 0; i < max; i++ {
		question := q.Questions[i]
		out = append(out, map[string]string{
			"code":          question.Code,
			"type":          question.Type,
			"resolved_type": ResolveQuestionType(question),
			"option_count":  strconv.Itoa(len(question.Options)),
			"title_preview": Truncate(question.Title, 30),
		})
	}
	return out
}

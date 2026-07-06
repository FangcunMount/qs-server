package questionnaire

import (
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/validation"
	"github.com/FangcunMount/qs-server/internal/pkg/answervalue"
)

func optionCodesFromQuestion(question Question) map[string]struct{} {
	withOptions, ok := question.(HasOptions)
	if !ok {
		return nil
	}
	codes := make(map[string]struct{}, len(withOptions.GetOptions()))
	for _, option := range withOptions.GetOptions() {
		code := strings.TrimSpace(option.GetCode().Value())
		if code == "" {
			continue
		}
		codes[code] = struct{}{}
	}
	return codes
}

func validateOptionSelection(question submissionQuestionSpec, rawValue any) error {
	if len(question.optionCodes) == 0 {
		return nil
	}
	switch question.typ {
	case TypeRadio:
		option, ok := answervalue.NormalizeSingleOption(rawValue)
		if !ok {
			return newError(ErrorKindInvalidAnswer, "question %s expects a single option value", question.code.Value())
		}
		if _, ok := question.optionCodes[option]; !ok {
			return newError(ErrorKindInvalidAnswer, "question %s option %s is not allowed", question.code.Value(), option)
		}
	case TypeCheckbox:
		options, ok := answervalue.NormalizeMultiOptions(rawValue)
		if !ok {
			return newError(ErrorKindInvalidAnswer, "question %s expects option list value", question.code.Value())
		}
		for _, option := range options {
			if _, ok := question.optionCodes[option]; !ok {
				return newError(ErrorKindInvalidAnswer, "question %s option %s is not allowed", question.code.Value(), option)
			}
		}
	}
	return nil
}

func ensureVisibleRequiredQuestionsAnswered(
	questions map[string]submissionQuestionSpec,
	rawAnswers []RawSubmissionAnswer,
) error {
	answers := make(map[string]any, len(rawAnswers))
	for _, raw := range rawAnswers {
		code := strings.TrimSpace(raw.QuestionCode)
		if code == "" {
			continue
		}
		answers[code] = raw.Value
	}

	for code, question := range questions {
		if !question.isAnswerable() {
			continue
		}
		if !isQuestionVisible(question, answers) {
			continue
		}
		if !question.hasRequiredRule() {
			continue
		}
		value, ok := answers[code]
		if !ok {
			return newError(ErrorKindInvalidAnswer, "required question %s is missing", code)
		}
		if isEmptyAnswerValue(value) {
			return newError(ErrorKindInvalidAnswer, "required question %s cannot be empty", code)
		}
	}
	return nil
}

func (q submissionQuestionSpec) isAnswerable() bool {
	return q.typ != TypeSection
}

func (q submissionQuestionSpec) hasRequiredRule() bool {
	for _, rule := range q.validationRules {
		if rule.GetRuleType() == validation.RuleTypeRequired {
			return true
		}
	}
	return false
}

func isQuestionVisible(question submissionQuestionSpec, answers map[string]any) bool {
	if question.showController == nil || question.showController.IsEmpty() {
		return true
	}
	conditions := question.showController.GetQuestions()
	if len(conditions) == 0 {
		return true
	}
	results := make([]bool, 0, len(conditions))
	for _, condition := range conditions {
		results = append(results, matchesShowCondition(condition, answers))
	}
	rule := strings.ToLower(strings.TrimSpace(question.showController.GetRule()))
	if rule == "or" {
		for _, matched := range results {
			if matched {
				return true
			}
		}
		return false
	}
	for _, matched := range results {
		if !matched {
			return false
		}
	}
	return true
}

func matchesShowCondition(condition ShowControllerCondition, answers map[string]any) bool {
	if len(condition.SelectOptionCodes) == 0 {
		return false
	}
	raw, ok := answers[condition.Code.Value()]
	if !ok {
		return false
	}
	if option, ok := answervalue.NormalizeSingleOption(raw); ok {
		for _, expected := range condition.SelectOptionCodes {
			if option == expected.Value() {
				return true
			}
		}
		return false
	}
	if options, ok := answervalue.NormalizeMultiOptions(raw); ok {
		for _, expected := range condition.SelectOptionCodes {
			for _, selected := range options {
				if selected == expected.Value() {
					return true
				}
			}
		}
		return false
	}
	return false
}

func isEmptyAnswerValue(raw any) bool {
	if raw == nil {
		return true
	}
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value) == ""
	case []string:
		return len(value) == 0
	case []any:
		return len(value) == 0
	default:
		if option, ok := answervalue.NormalizeSingleOption(raw); ok {
			return strings.TrimSpace(option) == ""
		}
		if options, ok := answervalue.NormalizeMultiOptions(raw); ok {
			return len(options) == 0
		}
		return false
	}
}

func showControllerFromQuestion(question Question) *ShowController {
	if question == nil {
		return nil
	}
	return question.GetShowController()
}

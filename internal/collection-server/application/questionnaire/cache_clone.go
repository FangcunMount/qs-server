package questionnaire

import "strings"

func cacheKey(code, version string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	version = strings.TrimSpace(version)
	if version == "" {
		return "published:" + code
	}
	return "published:" + code + ":" + version
}

// cloneResponse 深拷贝问卷 REST DTO，避免缓存条目被调用方修改。
func cloneResponse(src *QuestionnaireResponse) *QuestionnaireResponse {
	if src == nil {
		return nil
	}

	dst := *src
	if len(src.Questions) > 0 {
		dst.Questions = make([]QuestionResponse, len(src.Questions))
		for i := range src.Questions {
			dst.Questions[i] = cloneQuestionResponse(src.Questions[i])
		}
	}
	return &dst
}

func cloneQuestionResponse(src QuestionResponse) QuestionResponse {
	dst := src
	if len(src.Options) > 0 {
		dst.Options = append([]OptionResponse(nil), src.Options...)
	}
	if len(src.ValidationRules) > 0 {
		dst.ValidationRules = append([]ValidationRuleResponse(nil), src.ValidationRules...)
	}
	if src.CalculationRule != nil {
		rule := *src.CalculationRule
		dst.CalculationRule = &rule
	}
	if src.ShowController != nil {
		controller := *src.ShowController
		if len(src.ShowController.Conditions) > 0 {
			controller.Conditions = make([]ShowControllerConditionResponse, len(src.ShowController.Conditions))
			for i, condition := range src.ShowController.Conditions {
				controller.Conditions[i] = ShowControllerConditionResponse{
					QuestionCode: condition.QuestionCode,
					OptionCodes:  append([]string(nil), condition.OptionCodes...),
				}
			}
		}
		dst.ShowController = &controller
	}
	return dst
}

package questionnaire

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/validation"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// buildQuestionFromDTO 从 DTO 构建问题领域对象。
func buildQuestionFromDTO(
	code, stem, qType string,
	options []OptionDTO,
	required bool,
	description string,
	validationRules []validation.ValidationRule,
	calculationRule *calculation.CalculationRule,
	showController *domainQuestionnaire.ShowController,
) (domainQuestionnaire.Question, error) {
	opts := make([]domainQuestionnaire.Option, 0, len(options))
	for i, optDTO := range options {
		optionCode := optDTO.Value
		if optionCode == "" {
			generatedCode, err := meta.GenerateCode()
			if err != nil {
				return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidQuestion, "生成选项编码失败")
			}
			optionCode = generatedCode.String()
		}

		opt, err := domainQuestionnaire.NewOptionWithStringCode(optionCode, optDTO.Label, float64(optDTO.Score))
		if err != nil {
			return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidQuestion, "第 %d 个选项创建失败: %v", i+1, err)
		}
		opts = append(opts, opt)
	}

	qOptions := []domainQuestionnaire.QuestionParamsOption{
		domainQuestionnaire.WithCode(meta.NewCode(code)),
		domainQuestionnaire.WithStem(stem),
		domainQuestionnaire.WithQuestionType(domainQuestionnaire.QuestionType(qType)),
		domainQuestionnaire.WithOptions(opts),
		domainQuestionnaire.WithTips(description),
	}
	if required {
		qOptions = append(qOptions, domainQuestionnaire.WithRequired())
	}
	if len(validationRules) > 0 {
		qOptions = append(qOptions, domainQuestionnaire.WithValidationRules(validationRules))
	}
	if calculationRule != nil {
		qOptions = append(qOptions, domainQuestionnaire.WithCalculationRule(calculationRule.GetFormula()))
	}
	if showController != nil {
		qOptions = append(qOptions, domainQuestionnaire.WithShowController(showController))
	}

	return domainQuestionnaire.NewQuestion(qOptions...)
}

func toDomainValidationRules(rules []ValidationRuleDTO) []validation.ValidationRule {
	result := make([]validation.ValidationRule, 0, len(rules))
	for _, rule := range rules {
		result = append(result, validation.NewValidationRule(validation.RuleType(rule.RuleType), rule.TargetValue))
	}
	return result
}

func toDomainCalculationRule(rule *CalculationRuleDTO) *calculation.CalculationRule {
	if rule == nil || rule.FormulaType == "" {
		return nil
	}
	return calculation.NewCalculationRule(calculation.FormulaType(rule.FormulaType), []string{})
}

func toDomainShowController(controller *ShowControllerDTO) *domainQuestionnaire.ShowController {
	if controller == nil {
		return nil
	}
	conditions := make([]domainQuestionnaire.ShowControllerCondition, 0, len(controller.Questions))
	for _, cond := range controller.Questions {
		optionCodes := make([]meta.Code, 0, len(cond.SelectOptionCodes))
		for _, code := range cond.SelectOptionCodes {
			optionCodes = append(optionCodes, meta.NewCode(code))
		}
		conditions = append(conditions, domainQuestionnaire.NewShowControllerCondition(
			meta.NewCode(cond.Code),
			optionCodes,
		))
	}
	return domainQuestionnaire.NewShowController(controller.Rule, conditions)
}

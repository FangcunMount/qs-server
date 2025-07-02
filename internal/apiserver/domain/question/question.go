package question

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/calculation"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/option"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/validation"
)

// Question 问题接口 - 统一所有题型的方法签名
type Question interface {
	// 基础方法
	GetCode() QuestionCode
	GetTitle() string
	GetType() QuestionType
	GetTips() string

	// 文本相关方法
	GetPlaceholder() string
	// 选项相关方法
	GetOptions() []option.Option
	// 校验相关方法
	GetValidationRules() []validation.ValidationRule
	// 计算相关方法
	GetCalculationRule() *calculation.CalculationRule
}

package question

import (
	"github.com/FangcunMount/qs-server/internal/pkg/calculation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/validation"
)

// Question 问题接口 - 统一所有题型的方法签名
type Question interface {
	// 基础方法
	GetCode() meta.Code
	GetTitle() string
	GetType() QuestionType
	GetTips() string

	// 文本相关方法
	GetPlaceholder() string
	// 选项相关方法
	GetOptions() []Option
	// 校验相关方法
	GetValidationRules() []validation.ValidationRule
	// 计算相关方法
	GetCalculationRule() *calculation.CalculationRule
}

// QuestionType 题型
type QuestionType string

func (t QuestionType) Value() string {
	return string(t)
}

const (
	QuestionTypeSection  QuestionType = "Section"  // 段落
	QuestionTypeRadio    QuestionType = "Radio"    // 单选
	QuestionTypeCheckbox QuestionType = "Checkbox" // 多选
	QuestionTypeText     QuestionType = "Text"     // 文本
	QuestionTypeTextarea QuestionType = "Textarea" // 文本域
	QuestionTypeNumber   QuestionType = "Number"   // 数字
)

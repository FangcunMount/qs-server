package question

import (
	"github.com/yshujie/questionnaire-scale/internal/pkg/calculation"
	"github.com/yshujie/questionnaire-scale/internal/pkg/validation"
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
	GetOptions() []Option
	// 校验相关方法
	GetValidationRules() []validation.ValidationRule
	// 计算相关方法
	GetCalculationRule() *calculation.CalculationRule
}

// QuestionCode 问题编码
type QuestionCode string

// NewQuestionCode 创建问题编码
func NewQuestionCode(value string) QuestionCode {
	return QuestionCode(value)
}

// Value 获取问题编码
func (c QuestionCode) Value() string {
	return string(c)
}

// Equals 判断问题编码是否相等
func (c QuestionCode) Equals(other QuestionCode) bool {
	return c.Value() == other.Value()
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

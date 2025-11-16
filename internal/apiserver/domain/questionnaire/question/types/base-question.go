package types

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/questionnaire/question"
	"github.com/FangcunMount/qs-server/internal/pkg/calculation"
	"github.com/FangcunMount/qs-server/internal/pkg/validation"
)

// BaseQuestion 基础问题
type BaseQuestion struct {
	code         question.QuestionCode
	questionType question.QuestionType
	title        string
	tips         string
}

// NewBaseQuestion
func NewBaseQuestion(code question.QuestionCode, title string, questionType question.QuestionType) BaseQuestion {
	return BaseQuestion{
		code:         code,
		title:        title,
		questionType: questionType,
	}
}

// GetCode 获取问题编码
func (q *BaseQuestion) GetCode() question.QuestionCode {
	return q.code
}

// GetTitle 获取问题标题
func (q *BaseQuestion) GetTitle() string {
	return q.title
}

// GetType 获取题型
func (q *BaseQuestion) GetType() question.QuestionType {
	return q.questionType
}

// GetTips 获取问题提示
func (q *BaseQuestion) GetTips() string {
	return q.tips
}

// 默认实现 - 返回零值
func (q *BaseQuestion) GetPlaceholder() string {
	return ""
}

// GetOptions 获取选项
func (q *BaseQuestion) GetOptions() []question.Option {
	return nil
}

// GetValidationRules 获取校验规则
func (q *BaseQuestion) GetValidationRules() []validation.ValidationRule {
	return nil
}

// GetCalculationRule 获取计算规则
func (q *BaseQuestion) GetCalculationRule() *calculation.CalculationRule {
	return nil
}

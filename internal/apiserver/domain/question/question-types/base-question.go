package question_types

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/calculation"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/option"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/validation"
)

// BaseQuestion 基础问题
type BaseQuestion struct {
	code         question.QuestionCode
	questionType question.QuestionType
	title        string
	tips         string
}

// NewBaseQuestion creates a new BaseQuestion with the given parameters
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

func (q *BaseQuestion) GetOptions() []option.Option {
	return nil
}

func (q *BaseQuestion) GetValidationRules() []validation.ValidationRule {
	return nil
}

func (q *BaseQuestion) GetCalculationRule() *calculation.CalculationRule {
	return nil
}

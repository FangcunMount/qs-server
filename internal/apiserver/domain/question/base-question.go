package question

import "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/vo"

// Question 问题接口 - 统一所有题型的方法签名
type Question interface {
	// 基础方法
	GetCode() string
	GetTitle() string
	GetType() QuestionType
	GetTips() string

	// 文本相关方法
	GetPlaceholder() string
	// 选项相关方法
	GetOptions() []vo.Option
	// 校验相关方法
	GetValidationRules() []vo.ValidationRule
	// 计算相关方法
	GetCalculationRule() *vo.CalculationRule
}

// BaseQuestion 基础问题
type BaseQuestion struct {
	code         string
	title        string
	questionType QuestionType
	tips         string
}

// GetCode 获取问题编码
func (q *BaseQuestion) GetCode() string {
	return q.code
}

// GetTitle 获取问题标题
func (q *BaseQuestion) GetTitle() string {
	return q.title
}

// GetType 获取题型
func (q *BaseQuestion) GetType() QuestionType {
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

func (q *BaseQuestion) GetOptions() []vo.Option {
	return nil
}

func (q *BaseQuestion) GetValidationRules() []vo.ValidationRule {
	return nil
}

func (q *BaseQuestion) GetCalculationRule() *vo.CalculationRule {
	return nil
}

func (q *BaseQuestion) SetCode(code string) {
	q.code = code
}

// SetTitle 设置问题标题
func (q *BaseQuestion) SetTitle(title string) {
	q.title = title
}

// SetQuestionType 设置题型
func (q *BaseQuestion) SetQuestionType(questionType QuestionType) {
	q.questionType = questionType
}

// SetTips 设置提醒
func (q *BaseQuestion) SetTips(tips string) {
	q.tips = tips
}

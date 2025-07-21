package questionnaire

import "github.com/yshujie/questionnaire-scale/internal/collection-server/domain/answersheet"

// QuestionnaireAdapter 问卷适配器，实现 answersheet.QuestionnaireInfo 接口
type QuestionnaireAdapter struct {
	questionnaire *Questionnaire
}

// NewQuestionnaireAdapter 创建问卷适配器
func NewQuestionnaireAdapter(questionnaire *Questionnaire) *QuestionnaireAdapter {
	return &QuestionnaireAdapter{
		questionnaire: questionnaire,
	}
}

// GetCode 获取问卷代码
func (a *QuestionnaireAdapter) GetCode() string {
	return a.questionnaire.Code
}

// GetQuestions 获取问题列表
func (a *QuestionnaireAdapter) GetQuestions() []answersheet.QuestionInfo {
	questions := make([]answersheet.QuestionInfo, 0, len(a.questionnaire.Questions))
	for _, q := range a.questionnaire.Questions {
		questions = append(questions, NewQuestionAdapter(q))
	}
	return questions
}

// QuestionAdapter 问题适配器，实现 answersheet.QuestionInfo 接口
type QuestionAdapter struct {
	question *Question
}

// NewQuestionAdapter 创建问题适配器
func NewQuestionAdapter(question *Question) *QuestionAdapter {
	return &QuestionAdapter{
		question: question,
	}
}

// GetCode 获取问题代码
func (a *QuestionAdapter) GetCode() string {
	return a.question.Code
}

// GetType 获取问题类型
func (a *QuestionAdapter) GetType() string {
	return a.question.Type
}

// GetOptions 获取问题选项
func (a *QuestionAdapter) GetOptions() []answersheet.QuestionOption {
	options := make([]answersheet.QuestionOption, 0, len(a.question.Options))
	for _, opt := range a.question.Options {
		options = append(options, NewQuestionOptionAdapter(opt))
	}
	return options
}

// GetValidationRules 获取验证规则
func (a *QuestionAdapter) GetValidationRules() []answersheet.QuestionValidationRule {
	rules := make([]answersheet.QuestionValidationRule, 0, len(a.question.ValidationRules))
	for _, rule := range a.question.ValidationRules {
		rules = append(rules, NewValidationRuleAdapter(rule))
	}
	return rules
}

// QuestionOptionAdapter 问题选项适配器，实现 answersheet.QuestionOption 接口
type QuestionOptionAdapter struct {
	option *QuestionOption
}

// NewQuestionOptionAdapter 创建问题选项适配器
func NewQuestionOptionAdapter(option *QuestionOption) *QuestionOptionAdapter {
	return &QuestionOptionAdapter{
		option: option,
	}
}

// GetCode 获取选项代码
func (a *QuestionOptionAdapter) GetCode() string {
	return a.option.Code
}

// GetContent 获取选项内容
func (a *QuestionOptionAdapter) GetContent() string {
	return a.option.Content
}

// GetScore 获取选项分数
func (a *QuestionOptionAdapter) GetScore() int32 {
	return a.option.Score
}

// ValidationRuleAdapter 验证规则适配器，实现 answersheet.QuestionValidationRule 接口
type ValidationRuleAdapter struct {
	rule *ValidationRule
}

// NewValidationRuleAdapter 创建验证规则适配器
func NewValidationRuleAdapter(rule *ValidationRule) *ValidationRuleAdapter {
	return &ValidationRuleAdapter{
		rule: rule,
	}
}

// GetRuleType 获取规则类型
func (a *ValidationRuleAdapter) GetRuleType() string {
	return a.rule.RuleType
}

// GetTargetValue 获取目标值
func (a *ValidationRuleAdapter) GetTargetValue() string {
	return a.rule.TargetValue
}

// GetMessage 获取错误消息
func (a *ValidationRuleAdapter) GetMessage() string {
	return a.rule.Message
}

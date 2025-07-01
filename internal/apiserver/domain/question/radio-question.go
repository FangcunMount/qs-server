package question

import "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/vo"

// RadioQuestion 单选问题
type RadioQuestion struct {
	BaseQuestion
	vo.ValidationAbility
	vo.CalculationAbility

	options []vo.Option
}

// NewRadioQuestion 创建单选问题
func NewRadioQuestion(code, title string) *RadioQuestion {
	return &RadioQuestion{
		BaseQuestion: BaseQuestion{
			code:         code,
			title:        title,
			questionType: QuestionTypeRadio,
		},
	}
}

// GetOptions 获取选项
func (q *RadioQuestion) GetOptions() []vo.Option {
	return q.options
}

// SetOptions 设置选项
func (q *RadioQuestion) SetOptions(options []vo.Option) {
	q.options = options
}

// AddOption 添加选项
func (q *RadioQuestion) AddOption(option vo.Option) {
	// 如果选项已存在，则不添加
	for _, o := range q.options {
		if o.GetCode() == option.GetCode() {
			return
		}
	}

	// 如果选项不存在，则添加
	q.options = append(q.options, option)
}

// ClearOptions 清空选项
func (q *RadioQuestion) ClearOptions() {
	q.options = []vo.Option{}
}

package question

import "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/vo"

// NumberQuestion 数字问题
type NumberQuestion struct {
	BaseQuestion
	vo.ValidationAbility

	placeholder string
}

// NewNumberQuestion 创建数字问题
func NewNumberQuestion(code, title string) *NumberQuestion {
	return &NumberQuestion{
		BaseQuestion: BaseQuestion{
			code:         code,
			title:        title,
			questionType: QuestionTypeNumber,
		},
		ValidationAbility: vo.ValidationAbility{},
	}
}

// GetPlaceholder 获取占位符
func (q *NumberQuestion) GetPlaceholder() string {
	return q.placeholder
}

// SetPlaceholder 设置占位符
func (q *NumberQuestion) SetPlaceholder(placeholder string) {
	q.placeholder = placeholder
}

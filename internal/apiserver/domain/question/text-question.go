package question

import "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/vo"

// TextQuestion 文本问题
type TextQuestion struct {
	BaseQuestion
	vo.ValidationAbility

	placeholder string
}

// NewTextQuestion 创建文本问题
func NewTextQuestion(code, title string) *TextQuestion {
	return &TextQuestion{
		BaseQuestion: BaseQuestion{
			code:         code,
			title:        title,
			questionType: QuestionTypeText,
		},
	}
}

// GetPlaceholder 获取占位符
func (q *TextQuestion) GetPlaceholder() string {
	return q.placeholder
}

func (q *TextQuestion) SetPlaceholder(placeholder string) {
	q.placeholder = placeholder
}

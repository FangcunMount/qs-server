package question

import "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/vo"

// SectionQuestion 段落问题
type SectionQuestion struct {
	BaseQuestion
}

// NewSectionQuestion 创建段落问题
func NewSectionQuestion(code vo.QuestionCode, title string) *SectionQuestion {
	return &SectionQuestion{
		BaseQuestion: BaseQuestion{
			code:         code,
			title:        title,
			questionType: QuestionTypeSection,
		},
	}
}

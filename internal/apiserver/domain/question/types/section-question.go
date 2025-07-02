package types

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question"
)

// SectionQuestion 段落问题
type SectionQuestion struct {
	BaseQuestion
}

// NewSectionQuestion 创建段落问题
func NewSectionQuestion(code question.QuestionCode, title string) *SectionQuestion {
	return &SectionQuestion{
		BaseQuestion: NewBaseQuestion(code, title, question.QuestionTypeSection),
	}
}

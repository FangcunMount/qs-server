package question_types

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question"
)

// SectionQuestion 段落问题
type SectionQuestion struct {
	BaseQuestion
}

// 注册段落问题
func init() {
	RegisterQuestionFactory(question.QuestionTypeSection, func(builder *QuestionBuilder) question.Question {
		return newSectionQuestion(builder.GetCode(), builder.GetTitle())
	})
}

// newSectionQuestion 创建段落问题
func newSectionQuestion(code question.QuestionCode, title string) *SectionQuestion {
	return &SectionQuestion{
		BaseQuestion: NewBaseQuestion(code, title, question.QuestionTypeSection),
	}
}

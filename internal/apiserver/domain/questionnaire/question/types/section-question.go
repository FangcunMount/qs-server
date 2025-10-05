package types

import (
	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/questionnaire/question"
)

// SectionQuestion 段落问题
type SectionQuestion struct {
	BaseQuestion
}

// 注册段落问题
func init() {
	question.RegisterQuestionFactory(question.QuestionTypeSection, func(builder *question.QuestionBuilder) question.Question {
		return newSectionQuestion(builder.GetCode(), builder.GetTitle())
	})
}

// newSectionQuestion 创建段落问题
func newSectionQuestion(code question.QuestionCode, title string) *SectionQuestion {
	return &SectionQuestion{
		BaseQuestion: NewBaseQuestion(code, title, question.QuestionTypeSection),
	}
}

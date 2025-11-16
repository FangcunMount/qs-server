package types

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/questionnaire/question"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
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
func newSectionQuestion(code meta.Code, title string) *SectionQuestion {
	return &SectionQuestion{
		BaseQuestion: NewBaseQuestion(code, title, question.QuestionTypeSection),
	}
}

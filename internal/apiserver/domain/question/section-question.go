package question

// SectionQuestion 段落问题
type SectionQuestion struct {
	BaseQuestion
}

// NewSectionQuestion 创建段落问题
func NewSectionQuestion(code, title string) *SectionQuestion {
	return &SectionQuestion{
		BaseQuestion: BaseQuestion{
			code:         code,
			title:        title,
			questionType: QuestionTypeSection,
		},
	}
}

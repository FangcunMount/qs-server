package question

// Question 问题
type Question interface {
	GetCode() string
	GetTitle() string
	GetType() QuestionType
	GetTips() string
}

// BaseQuestion 基础问题
type BaseQuestion struct {
	code         string
	title        string
	questionType QuestionType
	tips         string
}

// GetCode 获取问题编码
func (q *BaseQuestion) GetCode() string {
	return q.code
}

// GetTitle 获取问题标题
func (q *BaseQuestion) GetTitle() string {
	return q.title
}

// GetQuestionType 获取题型
func (q *BaseQuestion) GetQuestionType() QuestionType {
	return q.questionType
}

// GetTips 获取问题提示
func (q *BaseQuestion) GetTips() string {
	return q.tips
}

func (q *BaseQuestion) SetCode(code string) {
	q.code = code
}

// SetTitle 设置问题标题
func (q *BaseQuestion) SetTitle(title string) {
	q.title = title
}

// SetQuestionType 设置题型
func (q *BaseQuestion) SetQuestionType(questionType QuestionType) {
	q.questionType = questionType
}

// SetTips 设置提醒
func (q *BaseQuestion) SetTips(tips string) {
	q.tips = tips
}

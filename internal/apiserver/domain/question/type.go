package question

// 题型
type QuestionType string

const (
	QuestionTypeSection  QuestionType = "section"  // 段落
	QuestionTypeRadio    QuestionType = "radio"    // 单选
	QuestionTypeCheckbox QuestionType = "checkbox" // 多选
	QuestionTypeText     QuestionType = "text"     // 文本
	QuestionTypeTextarea QuestionType = "textarea" // 文本域
	QuestionTypeNumber   QuestionType = "number"   // 数字
)

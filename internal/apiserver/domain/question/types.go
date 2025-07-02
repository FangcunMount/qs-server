package question

// QuestionCode 问题编码
type QuestionCode string

// NewQuestionCode 创建问题编码
func NewQuestionCode(value string) QuestionCode {
	return QuestionCode(value)
}

// Value 获取问题编码
func (c QuestionCode) Value() string {
	return string(c)
}

// Equals 判断问题编码是否相等
func (c QuestionCode) Equals(other QuestionCode) bool {
	return c.Value() == other.Value()
}

// QuestionType 题型
type QuestionType string

const (
	QuestionTypeSection  QuestionType = "section"  // 段落
	QuestionTypeRadio    QuestionType = "radio"    // 单选
	QuestionTypeCheckbox QuestionType = "checkbox" // 多选
	QuestionTypeText     QuestionType = "text"     // 文本
	QuestionTypeTextarea QuestionType = "textarea" // 文本域
	QuestionTypeNumber   QuestionType = "number"   // 数字
)

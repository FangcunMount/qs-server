package answer

// AnswerValue 答案值
type AnswerValue interface {
	// Raw 原始值
	Raw() any
}

// Answer 基础答案
type Answer struct {
	questionCode string
	questionType string
	score        uint16
	value        AnswerValue
}

// NewAnswer 创建基础答案
func NewAnswer(questionCode string, questionType string, score uint16, value AnswerValue) Answer {
	return Answer{
		questionCode: questionCode,
		questionType: questionType,
		score:        score,
		value:        value,
	}
}

func (a *Answer) GetQuestionCode() string {
	return a.questionCode
}

func (a *Answer) GetQuestionType() string {
	return a.questionType
}

func (a *Answer) GetScore() uint16 {
	return a.score
}

func (a *Answer) GetValue() AnswerValue {
	// 如果 value 为 nil，返回一个简单的默认实现
	if a.value == nil {
		return &defaultAnswerValue{}
	}
	return a.value
}

// defaultAnswerValue 默认答案值实现
type defaultAnswerValue struct{}

func (d *defaultAnswerValue) Raw() any {
	return ""
}

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
func NewAnswer(questionCode string, questionType string, score uint16, value AnswerValue) *Answer {
	return &Answer{
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
	return a.value
}

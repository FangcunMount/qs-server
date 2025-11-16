package answer

import (
	"errors"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/questionnaire/question"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// Answer 基础答案
type Answer struct {
	questionCode meta.Code
	questionType question.QuestionType
	score        float64
	value        AnswerValue
}

// NewAnswer 创建基础答案
func NewAnswer(qCode meta.Code, qType question.QuestionType, score float64, v any) (Answer, error) {
	vType, err := transforAnswerValueType(qType)
	if err != nil {
		return Answer{}, err
	}

	return Answer{
		questionCode: qCode,
		questionType: qType,
		score:        score,
		value:        CreateAnswerValuer(vType, v),
	}, nil
}

func transforAnswerValueType(qType question.QuestionType) (AnswerValueType, error) {
	switch qType {
	case question.QuestionTypeRadio:
		return OptionValueType, nil
	case question.QuestionTypeCheckbox:
		return OptionsValueType, nil
	case question.QuestionTypeText, question.QuestionTypeTextarea:
		return StringValueType, nil
	case question.QuestionTypeNumber:
		return NumberValueType, nil
	default:
		return "", errors.New("no AnswerValueType")
	}
}

func (a *Answer) GetQuestionCode() string {
	return a.questionCode.Value()
}

func (a *Answer) GetQuestionType() string {
	return a.questionType.Value()
}

func (a *Answer) GetScore() float64 {
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

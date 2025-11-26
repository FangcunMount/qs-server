package answersheet

import (
	"errors"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// =========== 答案值接口定义 ============

// AnswerValue 答案值接口
type AnswerValue interface {
	Raw() any
}

// =========== 答案实体 ============

// Answer 答案实体（值对象）
// 表示对某个问题的回答，包含问题引用、答案值和得分
type Answer struct {
	questionCode meta.Code                  // 问题编码
	questionType questionnaire.QuestionType // 问题类型
	score        float64                    // 得分
	value        AnswerValue                // 答案值
}

// NewAnswer 创建答案
func NewAnswer(
	questionCode meta.Code,
	questionType questionnaire.QuestionType,
	value AnswerValue,
	score float64,
) (Answer, error) {
	// 验证
	if questionCode.IsEmpty() {
		return Answer{}, errors.New("question code cannot be empty")
	}
	if value == nil {
		return Answer{}, errors.New("answer value cannot be nil")
	}

	return Answer{
		questionCode: questionCode,
		questionType: questionType,
		score:        score,
		value:        value,
	}, nil
}

// =========== 答案方法 ============

// QuestionCode 获取问题编码
func (a Answer) QuestionCode() string {
	return a.questionCode.Value()
}

// QuestionType 获取问题类型
func (a Answer) QuestionType() string {
	return a.questionType.Value()
}

// Score 获取得分
func (a Answer) Score() float64 {
	return a.score
}

// Value 获取答案值
func (a Answer) Value() AnswerValue {
	return a.value
}

// WithScore 返回新的带有指定分数的答案（不可变性）
func (a Answer) WithScore(score float64) Answer {
	return Answer{
		questionCode: a.questionCode,
		questionType: a.questionType,
		score:        score,
		value:        a.value,
	}
}

// IsEmpty 检查答案值是否为空
func (a Answer) IsEmpty() bool {
	if a.value == nil {
		return true
	}
	rawValue := a.value.Raw()
	if rawValue == nil {
		return true
	}
	// 检查字符串类型是否为空
	if str, ok := rawValue.(string); ok {
		return str == ""
	}
	return false
}

// Validate 验证答案
func (a Answer) Validate() error {
	if a.questionCode.IsEmpty() {
		return errors.New("question code is required")
	}
	if a.value == nil {
		return errors.New("answer value is required")
	}
	return nil
}

// =========== 答案值具体实现 ============

// StringValue 字符串答案值
type StringValue struct {
	value string
}

// NewStringValue 创建字符串答案值
func NewStringValue(v string) AnswerValue {
	return StringValue{value: v}
}

func (s StringValue) Raw() any {
	return s.value
}

// NumberValue 数字答案值
type NumberValue struct {
	value float64
}

// NewNumberValue 创建数字答案值
func NewNumberValue(v float64) AnswerValue {
	return NumberValue{value: v}
}

func (n NumberValue) Raw() any {
	return n.value
}

// OptionValue 单选答案值
type OptionValue struct {
	value string
}

// NewOptionValue 创建单选答案值
func NewOptionValue(v string) AnswerValue {
	return OptionValue{value: v}
}

func (o OptionValue) Raw() any {
	return o.value
}

// OptionsValue 多选答案值
type OptionsValue struct {
	values []string
}

// NewOptionsValue 创建多选答案值
func NewOptionsValue(values []string) AnswerValue {
	if values == nil {
		values = []string{}
	}
	return OptionsValue{values: values}
}

func (o OptionsValue) Raw() any {
	return o.values
}

// =========== 工厂方法 ============

// CreateAnswerValueFromRaw 从原始值创建答案值（根据问题类型）
func CreateAnswerValueFromRaw(qType questionnaire.QuestionType, raw any) (AnswerValue, error) {
	if raw == nil {
		return nil, errors.New("raw value cannot be nil")
	}

	switch qType {
	case questionnaire.TypeRadio:
		str, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("radio answer expects string, got %T", raw)
		}
		return NewOptionValue(str), nil

	case questionnaire.TypeCheckbox:
		// 尝试多种类型
		switch v := raw.(type) {
		case []string:
			return NewOptionsValue(v), nil
		case []interface{}:
			strs := make([]string, len(v))
			for i, item := range v {
				if s, ok := item.(string); ok {
					strs[i] = s
				} else {
					return nil, fmt.Errorf("checkbox answer expects []string, got item %T at index %d", item, i)
				}
			}
			return NewOptionsValue(strs), nil
		default:
			return nil, fmt.Errorf("checkbox answer expects []string, got %T", raw)
		}

	case questionnaire.TypeText, questionnaire.TypeTextarea, questionnaire.TypeSection:
		str, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("text answer expects string, got %T", raw)
		}
		return NewStringValue(str), nil

	case questionnaire.TypeNumber:
		switch v := raw.(type) {
		case float64:
			return NewNumberValue(v), nil
		case int:
			return NewNumberValue(float64(v)), nil
		case int64:
			return NewNumberValue(float64(v)), nil
		default:
			return nil, fmt.Errorf("number answer expects numeric type, got %T", raw)
		}

	default:
		return nil, fmt.Errorf("unsupported question type: %s", qType.Value())
	}
}

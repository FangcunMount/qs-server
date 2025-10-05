package answer

import "github.com/fangcun-mount/qs-server/pkg/log"

// AnswerValue 答案值
type AnswerValue interface {
	// Raw 原始值
	Raw() any
}

// 值类型
type AnswerValueType string

func (t AnswerValueType) Value() string {
	return string(t)
}

const (
	StringValueType  AnswerValueType = "String"
	NumberValueType  AnswerValueType = "Number"
	OptionValueType  AnswerValueType = "Option"
	OptionsValueType AnswerValueType = "Options"
)

// 注册函数签名
type AnswerValueFactory func(v any) AnswerValue

// 注册表本体
var registry = make(map[AnswerValueType]AnswerValueFactory)

// 注册函数
func RegisterAnswerValueFactory(typ AnswerValueType, factory AnswerValueFactory) {
	if _, exists := registry[typ]; exists {
		log.Errorf("answer value type already registered: %s", typ)
	}
	registry[typ] = factory
}

// 创建统一入口
func CreateAnswerValuer(t AnswerValueType, v any) AnswerValue {
	factory, ok := registry[t]
	if !ok {
		log.Errorf("unknown answer value type: %s", t.Value())
		return nil
	}
	return factory(v)
}

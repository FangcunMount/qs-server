package question

import (
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// 注册函数签名
type QuestionFactory func(builder *QuestionBuilder) Question

// 注册表本体
var registry = make(map[QuestionType]QuestionFactory)

// 注册函数
func RegisterQuestionFactory(typ QuestionType, factory QuestionFactory) {
	if _, exists := registry[typ]; exists {
		log.Errorf("question type already registered: %s", typ)
	}
	registry[typ] = factory
}

// 创建统一入口
func CreateQuestionFromBuilder(builder *QuestionBuilder) Question {
	factory, ok := registry[builder.GetQuestionType()]
	if !ok {
		log.Errorf("unknown question type: %s", builder.GetQuestionType())
		return nil
	}
	return factory(builder)
}

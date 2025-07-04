package question_types

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// 注册函数签名
type QuestionFactory func(builder *QuestionBuilder) question.Question

// 注册表本体
var registry = make(map[question.QuestionType]QuestionFactory)

// 注册函数
func RegisterQuestionFactory(typ question.QuestionType, factory QuestionFactory) {
	if _, exists := registry[typ]; exists {
		log.Errorf("question type already registered: %s", typ)
	}
	registry[typ] = factory
}

// 创建统一入口
func CreateQuestionFromBuilder(builder *QuestionBuilder) question.Question {
	factory, ok := registry[builder.GetQuestionType()]
	if !ok {
		log.Errorf("unknown question type: %s", builder.GetQuestionType())
		return nil
	}
	return factory(builder)
}

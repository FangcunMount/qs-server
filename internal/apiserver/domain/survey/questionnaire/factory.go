package questionnaire

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// QuestionFactory 题型工厂函数签名
// 接收参数容器，返回具体的 Question 实例
type QuestionFactory func(*QuestionParams) (Question, error)

// questionRegistry 题型工厂注册表
var questionRegistry = make(map[QuestionType]QuestionFactory)

// RegisterQuestionFactory 注册题型工厂，便于扩展新题型
func RegisterQuestionFactory(typ QuestionType, factory QuestionFactory) {
	questionRegistry[typ] = factory
}

// NewQuestion 创建 Question 的统一入口
// 职责：
// 1. 创建参数容器并收集参数
// 2. 校验参数完整性
// 3. 根据题型选择对应的工厂函数创建实例
func NewQuestion(opts ...QuestionParamsOption) (Question, error) {
	// 1. 创建参数容器并收集参数
	params := NewQuestionParams(opts...)

	// 2. 校验参数
	if err := params.Validate(); err != nil {
		// 即便参数校验失败，也尝试构建问题实例以便后续验证时使用
		if factory, ok := questionRegistry[params.GetCore().typ]; ok {
			if q, buildErr := factory(params); buildErr == nil {
				return q, errors.WrapC(err, code.ErrQuestionnaireInvalidQuestion, "invalid question parameters")
			}
		}
		return nil, errors.WrapC(err, code.ErrQuestionnaireInvalidQuestion, "invalid question parameters")
	}

	// 3. 根据题型获取对应的工厂函数
	factory, ok := questionRegistry[params.GetCore().typ]
	if !ok {
		return nil, errors.WithCode(
			code.ErrQuestionnaireInvalidQuestion,
			"unknown question type: %s",
			string(params.GetCore().typ),
		)
	}

	// 4. 使用工厂函数创建 Question 实例
	return factory(params)
}

package execute

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

// mutableEvaluatorRegistry 可变评估模型评估器注册表。
type mutableEvaluatorRegistry struct {
	items map[assessment.EvaluationModelKind]Evaluator
}

// newEmptyEvaluatorRegistry 创建空的评估模型评估器注册表。
func newEmptyEvaluatorRegistry() *mutableEvaluatorRegistry {
	return &mutableEvaluatorRegistry{items: make(map[assessment.EvaluationModelKind]Evaluator)}
}

// NewEvaluatorRegistry 创建评估模型评估器注册表。
func NewEvaluatorRegistry(evaluators ...Evaluator) (*mutableEvaluatorRegistry, error) {
	registry := newEmptyEvaluatorRegistry()
	for _, evaluator := range evaluators {
		if err := registry.Register(evaluator); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

// Register 注册评估模型评估器。
func (r *mutableEvaluatorRegistry) Register(evaluator Evaluator) error {
	if evaluator == nil {
		return fmt.Errorf("evaluation evaluator is nil")
	}
	kind := evaluator.Kind()
	if kind == "" {
		return fmt.Errorf("evaluation evaluator kind is empty")
	}
	if _, exists := r.items[kind]; exists {
		return fmt.Errorf("evaluation evaluator already registered for kind %s", kind)
	}
	r.items[kind] = evaluator
	return nil
}

// Resolve 解析评估模型评估器。
func (r *mutableEvaluatorRegistry) Resolve(kind assessment.EvaluationModelKind) (Evaluator, error) {
	if r == nil {
		return nil, fmt.Errorf("evaluation evaluator registry is not configured")
	}
	evaluator, ok := r.items[kind]
	if !ok {
		return nil, fmt.Errorf("unsupported evaluation model kind: %s", kind)
	}
	return evaluator, nil
}

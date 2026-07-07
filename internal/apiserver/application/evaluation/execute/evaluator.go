package execute

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
)

// mutableEvaluator注册表 路由 execution 按 v2 Evaluator键。
type mutableEvaluatorRegistry struct {
	items map[evaluation.ExecutionIdentity]Evaluator
}

func newEmptyEvaluatorRegistry() *mutableEvaluatorRegistry {
	return &mutableEvaluatorRegistry{items: make(map[evaluation.ExecutionIdentity]Evaluator)}
}

// NewEvaluatorRegistry 创建evaluator 注册表 键ed 按 Evaluator键。
func NewEvaluatorRegistry(evaluators ...Evaluator) (*mutableEvaluatorRegistry, error) {
	registry := newEmptyEvaluatorRegistry()
	for _, evaluator := range evaluators {
		if err := registry.Register(evaluator); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

// Register registers evaluator 用于 its Evaluator键。
func (r *mutableEvaluatorRegistry) Register(evaluator Evaluator) error {
	if evaluator == nil {
		return fmt.Errorf("evaluation evaluator is nil")
	}
	key := evaluator.ExecutionIdentity()
	if key.IsZero() {
		return fmt.Errorf("evaluation evaluator key is empty")
	}
	if _, exists := r.items[key]; exists {
		return fmt.Errorf("evaluation evaluator already registered for key %s", key)
	}
	r.items[key] = evaluator
	return nil
}

// Resolve finds evaluator 按 v2 键。
func (r *mutableEvaluatorRegistry) Resolve(key evaluation.ExecutionIdentity) (Evaluator, error) {
	if r == nil {
		return nil, fmt.Errorf("evaluation evaluator registry is not configured")
	}
	if evaluator, ok := r.items[key]; ok {
		return evaluator, nil
	}
	if key.IsPersonalityTypologyLegacyIdentity() {
		if evaluator, ok := r.items[evaluation.ExecutionIdentityPersonalityTypology]; ok {
			return evaluator, nil
		}
	}
	if routed := evaluation.ResolveBehavioralRatingExecutorIdentity(key); routed != key {
		if evaluator, ok := r.items[routed]; ok {
			return evaluator, nil
		}
	}
	return nil, fmt.Errorf("unsupported evaluation model key: %s", key)
}

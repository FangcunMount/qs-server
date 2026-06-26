package execute

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
)

// mutableEvaluatorRegistry routes execution by v2 EvaluatorKey.
type mutableEvaluatorRegistry struct {
	items map[evaluation.EvaluatorKey]Evaluator
}

func newEmptyEvaluatorRegistry() *mutableEvaluatorRegistry {
	return &mutableEvaluatorRegistry{items: make(map[evaluation.EvaluatorKey]Evaluator)}
}

// NewEvaluatorRegistry creates an evaluator registry keyed by EvaluatorKey.
func NewEvaluatorRegistry(evaluators ...Evaluator) (*mutableEvaluatorRegistry, error) {
	registry := newEmptyEvaluatorRegistry()
	for _, evaluator := range evaluators {
		if err := registry.Register(evaluator); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

// Register registers an evaluator for its EvaluatorKey.
func (r *mutableEvaluatorRegistry) Register(evaluator Evaluator) error {
	if evaluator == nil {
		return fmt.Errorf("evaluation evaluator is nil")
	}
	key := evaluator.Key()
	if key.IsZero() {
		return fmt.Errorf("evaluation evaluator key is empty")
	}
	if _, exists := r.items[key]; exists {
		return fmt.Errorf("evaluation evaluator already registered for key %s", key)
	}
	r.items[key] = evaluator
	return nil
}

// Resolve finds an evaluator by v2 key.
func (r *mutableEvaluatorRegistry) Resolve(key evaluation.EvaluatorKey) (Evaluator, error) {
	if r == nil {
		return nil, fmt.Errorf("evaluation evaluator registry is not configured")
	}
	if evaluator, ok := r.items[key]; ok {
		return evaluator, nil
	}
	if key.IsPersonalityTypologyLegacyKey() {
		if evaluator, ok := r.items[evaluation.EvaluatorKeyPersonalityTypology]; ok {
			return evaluator, nil
		}
	}
	return nil, fmt.Errorf("unsupported evaluation model key: %s", key)
}

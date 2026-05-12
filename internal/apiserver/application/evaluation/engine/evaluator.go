package engine

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type ExecutionInput struct {
	Assessment *assessment.Assessment
	Input      *evaluationinput.InputSnapshot
}

// Evaluator 执行某一类解释模型的评估。
type Evaluator interface {
	Kind() assessment.EvaluationModelKind
	Evaluate(ctx context.Context, input ExecutionInput) error
}

type EvaluatorRegistry interface {
	Resolve(kind assessment.EvaluationModelKind) (Evaluator, error)
}

type mutableEvaluatorRegistry struct {
	items map[assessment.EvaluationModelKind]Evaluator
}

func NewEvaluatorRegistry(evaluators ...Evaluator) (*mutableEvaluatorRegistry, error) {
	registry := &mutableEvaluatorRegistry{items: make(map[assessment.EvaluationModelKind]Evaluator)}
	for _, evaluator := range evaluators {
		if err := registry.Register(evaluator); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

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

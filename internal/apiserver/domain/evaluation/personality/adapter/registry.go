package adapter

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

// ModelAdapter scores a typology payload and maps it to an assessment outcome.
type ModelAdapter interface {
	Algorithm() assessmentmodel.Algorithm
	BuildOutcome(
		modelRef assessment.EvaluationModelRef,
		payload *modeltypology.Payload,
		sheet *evaluationinput.AnswerSheet,
	) (*assessment.AssessmentOutcome, error)
}

// Registry resolves personality model adapters by algorithm.
type Registry struct {
	adapters map[assessmentmodel.Algorithm]ModelAdapter
}

func NewRegistry(adapters ...ModelAdapter) Registry {
	registry := Registry{adapters: make(map[assessmentmodel.Algorithm]ModelAdapter, len(adapters))}
	for _, adapter := range adapters {
		if adapter == nil {
			continue
		}
		registry.adapters[adapter.Algorithm()] = adapter
	}
	return registry
}

func DefaultRegistry() Registry {
	return NewRegistry(
		mbtiAdapter(),
		sbtiAdapter(),
	)
}

func (r Registry) Resolve(algorithm assessmentmodel.Algorithm) (ModelAdapter, error) {
	if adapter, ok := r.adapters[algorithm]; ok {
		return adapter, nil
	}
	return nil, fmt.Errorf("unsupported typology algorithm: %s", algorithm)
}

func (r Registry) Algorithms() []assessmentmodel.Algorithm {
	out := make([]assessmentmodel.Algorithm, 0, len(r.adapters))
	for algorithm := range r.adapters {
		out = append(out, algorithm)
	}
	return out
}

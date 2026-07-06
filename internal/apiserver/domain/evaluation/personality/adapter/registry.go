package adapter

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
)

// ModelAdapter scores a typology payload through the personality profile pipeline.
type ModelAdapter interface {
	Algorithm() modelcatalog.Algorithm
	Score(
		payload *modeltypology.Payload,
		sheet *evaluationinput.AnswerSheet,
	) (evaluationtypology.ScoringResult, error)
}

// Registry resolves personality model adapters by algorithm.
type Registry struct {
	adapters map[modelcatalog.Algorithm]ModelAdapter
}

func NewRegistry(adapters ...ModelAdapter) Registry {
	registry := Registry{adapters: make(map[modelcatalog.Algorithm]ModelAdapter, len(adapters))}
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
		bigfiveAdapter(),
	)
}

func (r Registry) Resolve(algorithm modelcatalog.Algorithm) (ModelAdapter, error) {
	if adapter, ok := r.adapters[algorithm]; ok {
		return adapter, nil
	}
	return nil, fmt.Errorf("unsupported typology algorithm: %s", algorithm)
}

func (r Registry) Algorithms() []modelcatalog.Algorithm {
	out := make([]modelcatalog.Algorithm, 0, len(r.adapters))
	for algorithm := range r.adapters {
		out = append(out, algorithm)
	}
	return out
}

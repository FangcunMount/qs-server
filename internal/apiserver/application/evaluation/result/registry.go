package result

import (
	"context"
	"fmt"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
)

type mutableScoreProjectorRegistry struct {
	items map[evaluation.EvaluatorKey]ScoreProjector
}

func NewScoreProjectorRegistry(projectors ...ScoreProjector) (ScoreProjectorRegistry, error) {
	registry := &mutableScoreProjectorRegistry{items: make(map[evaluation.EvaluatorKey]ScoreProjector)}
	for _, projector := range projectors {
		if err := registry.Register(projector); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func (r *mutableScoreProjectorRegistry) Register(projector ScoreProjector) error {
	if projector == nil {
		return fmt.Errorf("evaluation score projector is nil")
	}
	key := projector.Key()
	if key.IsZero() {
		return fmt.Errorf("evaluation score projector key is empty")
	}
	if _, exists := r.items[key]; exists {
		return fmt.Errorf("evaluation score projector already registered for key %s", key)
	}
	r.items[key] = projector
	return nil
}

func (r *mutableScoreProjectorRegistry) Resolve(key evaluation.EvaluatorKey) ScoreProjector {
	if r == nil {
		return noopScoreProjector{}
	}
	if projector, ok := r.items[key]; ok {
		return projector
	}
	return noopScoreProjector{}
}

func NewReportBuilderRegistry(builders ...ReportBuilder) (ReportBuilderRegistry, error) {
	return interpretationreporting.NewReportBuilderRegistry(builders...)
}

type noopScoreProjector struct{}

func (noopScoreProjector) Key() evaluation.EvaluatorKey {
	return evaluation.EvaluatorKey{}
}

func (noopScoreProjector) Project(context.Context, evaloutcome.Outcome) error {
	return nil
}

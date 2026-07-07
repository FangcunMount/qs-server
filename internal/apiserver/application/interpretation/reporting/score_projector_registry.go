package reporting

import (
	"context"
	"fmt"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

type mutableScoreProjectorRegistry struct {
	items          map[evaluation.EvaluatorKey]ScoreProjector
	mechanismItems map[MechanismReportBuilderKey]ScoreProjector
}

// NewScoreProjectorRegistry creates a score projector registry from the given projectors.
func NewScoreProjectorRegistry(projectors ...ScoreProjector) (ScoreProjectorRegistry, error) {
	registry := &mutableScoreProjectorRegistry{
		items:          make(map[evaluation.EvaluatorKey]ScoreProjector),
		mechanismItems: make(map[MechanismReportBuilderKey]ScoreProjector),
	}
	for _, projector := range projectors {
		if err := registry.Register(projector); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func (r *mutableScoreProjectorRegistry) Register(projector ScoreProjector) error {
	if projector == nil {
		return fmt.Errorf("interpretation score projector is nil")
	}
	key := projector.Key()
	if key.IsZero() {
		return fmt.Errorf("interpretation score projector key is empty")
	}
	if _, exists := r.items[key]; exists {
		return fmt.Errorf("interpretation score projector already registered for key %s", key)
	}
	r.items[key] = projector
	if keyed, ok := projector.(MechanismKeyedScoreProjector); ok {
		mechanismKeys := []MechanismReportBuilderKey{keyed.MechanismKey()}
		if multi, ok := projector.(MultiMechanismKeyedScoreProjector); ok {
			mechanismKeys = multi.MechanismKeys()
		}
		for _, mechanismKey := range mechanismKeys {
			if mechanismKey.ReportType == "" {
				mechanismKey.ReportType = domainReport.ReportTypeStandard
			}
			if _, exists := r.mechanismItems[mechanismKey]; exists {
				return fmt.Errorf("interpretation score projector already registered for mechanism %s", mechanismKey)
			}
			r.mechanismItems[mechanismKey] = projector
		}
	}
	return nil
}

func (r *mutableScoreProjectorRegistry) Resolve(key evaluation.EvaluatorKey) ScoreProjector {
	if r == nil {
		return noopScoreProjector{}
	}
	if projector, ok := r.items[key]; ok {
		return projector
	}
	if mechanismKey, ok := MechanismReportBuilderKeyFromEvaluatorKey(key, domainReport.ReportTypeStandard); ok {
		projector := r.ResolveByMechanism(mechanismKey)
		if _, ok := projector.(noopScoreProjector); !ok {
			return projector
		}
	}
	return noopScoreProjector{}
}

func (r *mutableScoreProjectorRegistry) ResolveByMechanism(key MechanismReportBuilderKey) ScoreProjector {
	if r == nil {
		return noopScoreProjector{}
	}
	if key.ReportType == "" {
		key.ReportType = domainReport.ReportTypeStandard
	}
	if projector, ok := r.mechanismItems[key]; ok {
		return projector
	}
	familyKey := MechanismReportBuilderKey{
		AlgorithmFamily: key.AlgorithmFamily,
		ReportType:      key.ReportType,
	}
	if projector, ok := r.mechanismItems[familyKey]; ok {
		return projector
	}
	return noopScoreProjector{}
}

type noopScoreProjector struct{}

func (noopScoreProjector) Key() evaluation.EvaluatorKey {
	return evaluation.EvaluatorKey{}
}

func (noopScoreProjector) Project(context.Context, evaloutcome.Outcome) error {
	return nil
}

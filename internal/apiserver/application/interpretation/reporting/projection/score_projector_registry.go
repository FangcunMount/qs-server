package projection

import (
	"context"
	"fmt"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/registry"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

type mutableScoreProjectorRegistry struct {
	items          map[evaluation.ExecutionIdentity]ScoreProjector
	mechanismItems map[registry.MechanismReportBuilderKey]ScoreProjector
}

// NewScoreProjectorRegistry 创建score 投影器 注册表 从 given 投影器。
func NewScoreProjectorRegistry(projectors ...ScoreProjector) (ScoreProjectorRegistry, error) {
	r := &mutableScoreProjectorRegistry{
		items:          make(map[evaluation.ExecutionIdentity]ScoreProjector),
		mechanismItems: make(map[registry.MechanismReportBuilderKey]ScoreProjector),
	}
	for _, projector := range projectors {
		if err := r.Register(projector); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *mutableScoreProjectorRegistry) Register(projector ScoreProjector) error {
	if projector == nil {
		return fmt.Errorf("interpretation score projector is nil")
	}
	keyed, ok := projector.(MechanismKeyedScoreProjector)
	if !ok {
		return fmt.Errorf("interpretation score projector must implement MechanismKeyedScoreProjector")
	}
	mechanismKeys := []registry.MechanismReportBuilderKey{keyed.MechanismKey()}
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
	if id := projector.ExecutionIdentity(); !id.IsZero() {
		if _, exists := r.items[id]; exists {
			return fmt.Errorf("interpretation score projector already registered for identity %s", id)
		}
		r.items[id] = projector
	}
	return nil
}

func (r *mutableScoreProjectorRegistry) Resolve(key evaluation.ExecutionIdentity) ScoreProjector {
	if r == nil {
		return noopScoreProjector{}
	}
	if projector, ok := r.items[key]; ok {
		return projector
	}
	if mechanismKey, ok := registry.MechanismReportBuilderKeyFromExecutionIdentity(key, domainReport.ReportTypeStandard); ok {
		projector := r.ResolveByMechanism(mechanismKey)
		if _, ok := projector.(noopScoreProjector); !ok {
			return projector
		}
	}
	if routed := evaluation.ResolvePersonalityTypologyExecutorIdentity(key); routed != key {
		if projector, ok := r.items[routed]; ok {
			return projector
		}
	}
	if routed := evaluation.ResolveBehavioralRatingExecutorIdentity(key); routed != key {
		if projector, ok := r.items[routed]; ok {
			return projector
		}
	}
	return noopScoreProjector{}
}

func (r *mutableScoreProjectorRegistry) ResolveByMechanism(key registry.MechanismReportBuilderKey) ScoreProjector {
	if r == nil {
		return noopScoreProjector{}
	}
	if key.ReportType == "" {
		key.ReportType = domainReport.ReportTypeStandard
	}
	for _, candidate := range registry.MechanismKeyFallbackCandidates(key) {
		if projector, ok := r.mechanismItems[candidate]; ok {
			return projector
		}
	}
	return noopScoreProjector{}
}

type noopScoreProjector struct{}

func (noopScoreProjector) ExecutionIdentity() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentity{}
}

func (noopScoreProjector) Key() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentity{}
}

func (noopScoreProjector) Project(context.Context, evaloutcome.Outcome) error {
	return nil
}

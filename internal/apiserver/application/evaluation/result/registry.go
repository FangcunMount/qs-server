package result

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

type ScoreProjectorRegistry interface {
	Resolve(key evaluation.EvaluatorKey) ScoreProjector
}

type mutableScoreProjectorRegistry struct {
	items map[evaluation.EvaluatorKey]ScoreProjector
}

func NewScoreProjectorRegistry(projectors ...ScoreProjector) (*mutableScoreProjectorRegistry, error) {
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

type reportBuilderKey struct {
	key        evaluation.EvaluatorKey
	reportType domainReport.ReportType
}

type ReportBuilderRegistry interface {
	Resolve(key evaluation.EvaluatorKey, reportType domainReport.ReportType) (ReportBuilder, error)
}

type mutableReportBuilderRegistry struct {
	items map[reportBuilderKey]ReportBuilder
}

func NewReportBuilderRegistry(builders ...ReportBuilder) (*mutableReportBuilderRegistry, error) {
	registry := &mutableReportBuilderRegistry{items: make(map[reportBuilderKey]ReportBuilder)}
	for _, builder := range builders {
		if err := registry.Register(builder); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func (r *mutableReportBuilderRegistry) Register(builder ReportBuilder) error {
	if builder == nil {
		return fmt.Errorf("evaluation report builder is nil")
	}
	key := builder.Key()
	if key.IsZero() {
		return fmt.Errorf("evaluation report builder key is empty")
	}
	reportType := builder.ReportType()
	if reportType == "" {
		return fmt.Errorf("evaluation report builder report type is empty")
	}
	registryKey := reportBuilderKey{key: key, reportType: reportType}
	if _, exists := r.items[registryKey]; exists {
		return fmt.Errorf("evaluation report builder already registered for key %s report type %s", key, reportType)
	}
	r.items[registryKey] = builder
	return nil
}

func (r *mutableReportBuilderRegistry) Resolve(key evaluation.EvaluatorKey, reportType domainReport.ReportType) (ReportBuilder, error) {
	if r == nil {
		return nil, fmt.Errorf("evaluation report builder registry is not configured")
	}
	if reportType == "" {
		reportType = domainReport.ReportTypeStandard
	}
	registryKey := reportBuilderKey{key: key, reportType: reportType}
	if builder, ok := r.items[registryKey]; ok {
		return builder, nil
	}
	if routed := evaluation.ResolvePersonalityTypologyExecutorKey(key); routed != key {
		registryKey.key = routed
		if builder, ok := r.items[registryKey]; ok {
			return builder, nil
		}
	}
	if mappedKind, subKind, algorithm, ok := modelcatalog.LegacyKindMapping(key.Kind); ok {
		registryKey.key = evaluation.EvaluatorKey{Kind: mappedKind, SubKind: subKind, Algorithm: algorithm}
		if builder, ok := r.items[registryKey]; ok {
			return builder, nil
		}
	}
	return nil, fmt.Errorf("unsupported evaluation report builder key: %s report type: %s", key, reportType)
}

type noopScoreProjector struct{}

func (noopScoreProjector) Key() evaluation.EvaluatorKey {
	return evaluation.EvaluatorKey{}
}

func (noopScoreProjector) Project(context.Context, Outcome) error {
	return nil
}

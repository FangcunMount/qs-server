package result

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
)

type ScoreProjectorRegistry interface {
	Resolve(kind assessment.EvaluationModelKind) ScoreProjector
}

type mutableScoreProjectorRegistry struct {
	items map[assessment.EvaluationModelKind]ScoreProjector
}

func NewScoreProjectorRegistry(projectors ...ScoreProjector) (*mutableScoreProjectorRegistry, error) {
	registry := &mutableScoreProjectorRegistry{items: make(map[assessment.EvaluationModelKind]ScoreProjector)}
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
	kind := projector.Kind()
	if kind == "" {
		return fmt.Errorf("evaluation score projector kind is empty")
	}
	if _, exists := r.items[kind]; exists {
		return fmt.Errorf("evaluation score projector already registered for kind %s", kind)
	}
	r.items[kind] = projector
	return nil
}

func (r *mutableScoreProjectorRegistry) Resolve(kind assessment.EvaluationModelKind) ScoreProjector {
	if r == nil {
		return noopScoreProjector{}
	}
	if projector, ok := r.items[kind]; ok {
		return projector
	}
	return noopScoreProjector{}
}

type reportBuilderKey struct {
	kind       assessment.EvaluationModelKind
	reportType domainReport.ReportType
}

type ReportBuilderRegistry interface {
	Resolve(kind assessment.EvaluationModelKind, reportType domainReport.ReportType) (ReportBuilder, error)
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
	kind := builder.Kind()
	if kind == "" {
		return fmt.Errorf("evaluation report builder kind is empty")
	}
	reportType := builder.ReportType()
	if reportType == "" {
		return fmt.Errorf("evaluation report builder report type is empty")
	}
	key := reportBuilderKey{kind: kind, reportType: reportType}
	if _, exists := r.items[key]; exists {
		return fmt.Errorf("evaluation report builder already registered for kind %s report type %s", kind, reportType)
	}
	r.items[key] = builder
	return nil
}

func (r *mutableReportBuilderRegistry) Resolve(kind assessment.EvaluationModelKind, reportType domainReport.ReportType) (ReportBuilder, error) {
	if r == nil {
		return nil, fmt.Errorf("evaluation report builder registry is not configured")
	}
	if reportType == "" {
		reportType = domainReport.ReportTypeStandard
	}
	key := reportBuilderKey{kind: kind, reportType: reportType}
	if builder, ok := r.items[key]; ok {
		return builder, nil
	}
	if mappedKind, _, _, ok := assessmentmodel.LegacyKindMapping(assessmentmodel.Kind(kind)); ok {
		key.kind = assessment.EvaluationModelKind(mappedKind)
		if builder, ok := r.items[key]; ok {
			return builder, nil
		}
	}
	return nil, fmt.Errorf("unsupported evaluation report builder kind: %s report type: %s", kind, reportType)
}

type noopScoreProjector struct{}

func (noopScoreProjector) Kind() assessment.EvaluationModelKind {
	return ""
}

func (noopScoreProjector) Project(context.Context, Outcome) error {
	return nil
}

package assessmentmodel

import (
	"fmt"

	typologyEvaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/personality/typology"
	evaluationResult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
)

// ReportWiringDeps groups dependencies for materializing report builders from descriptors.
type ReportWiringDeps struct {
	ScaleReportBuilder          domainreport.ReportBuilder
	TypologyRegistry            typologyEvaluation.ModuleRegistry
	sharedTypologyReportBuilder *typologyEvaluation.ReportBuilder
}

// MaterializeReportBuilders builds report builders from evaluation model descriptors.
func MaterializeReportBuilders(descs []evaldomain.ModelDescriptor, deps ReportWiringDeps) ([]evaluationResult.ReportBuilder, error) {
	var sharedConfigured typologyEvaluation.ReportBuilder
	deps.sharedTypologyReportBuilder = &sharedConfigured
	builders := make([]evaluationResult.ReportBuilder, 0, len(descs))
	for _, desc := range descs {
		builder, err := materializeReportBuilder(desc, deps)
		if err != nil {
			return nil, err
		}
		builders = append(builders, builder)
	}
	return builders, nil
}

func materializeReportBuilder(desc evaldomain.ModelDescriptor, deps ReportWiringDeps) (evaluationResult.ReportBuilder, error) {
	switch desc.Kind {
	case evaldomain.ModelKindScale:
		return evaluationResult.NewScaleReportBuilder(deps.ScaleReportBuilder), nil
	case evaldomain.ModelKindTypology:
		registry, err := requireTypologyRegistry(deps)
		if err != nil {
			return nil, err
		}
		return typologyEvaluation.MaterializeTypologyReportBuilder(desc, registry, deps.sharedTypologyReportBuilder)
	default:
		return nil, fmt.Errorf("unsupported evaluation model kind: %s", desc.Kind)
	}
}

func requireTypologyRegistry(deps ReportWiringDeps) (typologyEvaluation.ModuleRegistry, error) {
	if deps.TypologyRegistry.Len() == 0 {
		return typologyEvaluation.ModuleRegistry{}, fmt.Errorf("typology registry is required")
	}
	return deps.TypologyRegistry, nil
}

package materialize

import (
	"fmt"

	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	typologyreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/typology"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type factory func(domainreport.ReportBuilder) (interpretationreporting.ReportBuilder, error)

var factories = map[modelcatalog.ExecutionPath]factory{
	modelcatalog.ExecutionPathScaleDescriptor: func(composer domainreport.ReportBuilder) (interpretationreporting.ReportBuilder, error) {
		return interpretationreporting.NewFactorScoringReportBuilder(composer), nil
	},
	modelcatalog.ExecutionPathTypologyDescriptor: func(_ domainreport.ReportBuilder) (interpretationreporting.ReportBuilder, error) {
		return typologyreporting.NewConfiguredReportBuilder()
	},
	modelcatalog.ExecutionPathBehavioralRatingDescriptor: func(composer domainreport.ReportBuilder) (interpretationreporting.ReportBuilder, error) {
		return interpretationreporting.NewNormProfileReportBuilder(composer), nil
	},
	modelcatalog.ExecutionPathCognitiveDescriptor: func(composer domainreport.ReportBuilder) (interpretationreporting.ReportBuilder, error) {
		return interpretationreporting.NewTaskPerformanceReportBuilder(composer), nil
	},
}

// ReportBuilders builds Interpretation-owned report builders from model descriptors.
func ReportBuilders(descs []evaldomain.ModelDescriptor, composer domainreport.ReportBuilder) ([]interpretationreporting.ReportBuilder, error) {
	builders := make([]interpretationreporting.ReportBuilder, 0, len(descs))
	for _, desc := range descs {
		path, err := evaldomain.ExecutionPathForDescriptor(desc)
		if err != nil {
			return nil, err
		}
		build, ok := factories[path]
		if !ok {
			return nil, fmt.Errorf("unsupported interpretation execution path: %s", path)
		}
		builder, err := build(composer)
		if err != nil {
			return nil, err
		}
		builders = append(builders, builder)
	}
	return builders, nil
}

// RegisteredPaths returns Interpretation report paths in stable model-catalog order.
func RegisteredPaths() []modelcatalog.ExecutionPath {
	return []modelcatalog.ExecutionPath{
		modelcatalog.ExecutionPathScaleDescriptor,
		modelcatalog.ExecutionPathTypologyDescriptor,
		modelcatalog.ExecutionPathBehavioralRatingDescriptor,
		modelcatalog.ExecutionPathCognitiveDescriptor,
	}
}

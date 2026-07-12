package materialize

import (
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	typologyreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/typology"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type factory func(domainreport.DraftBuilder) (interpretationreporting.ReportBuilder, error)

var factories = map[modelcatalog.ExecutionPath]factory{
	modelcatalog.ExecutionPathScaleDescriptor: func(composer domainreport.DraftBuilder) (interpretationreporting.ReportBuilder, error) {
		return interpretationreporting.NewFactorScoringReportBuilder(composer), nil
	},
	modelcatalog.ExecutionPathTypologyDescriptor: func(_ domainreport.DraftBuilder) (interpretationreporting.ReportBuilder, error) {
		return typologyreporting.NewConfiguredReportBuilder()
	},
	modelcatalog.ExecutionPathBehavioralRatingDescriptor: func(composer domainreport.DraftBuilder) (interpretationreporting.ReportBuilder, error) {
		return interpretationreporting.NewNormProfileReportBuilder(composer), nil
	},
	modelcatalog.ExecutionPathCognitiveDescriptor: func(composer domainreport.DraftBuilder) (interpretationreporting.ReportBuilder, error) {
		return interpretationreporting.NewTaskPerformanceReportBuilder(composer), nil
	},
}

// ReportBuilders builds the complete Interpretation-owned report builder set.
// The module owns its supported report paths and no longer mirrors Evaluation's
// evaluator descriptor registry through an alias contract.
func ReportBuilders(composer domainreport.DraftBuilder) ([]interpretationreporting.ReportBuilder, error) {
	paths := RegisteredPaths()
	builders := make([]interpretationreporting.ReportBuilder, 0, len(paths))
	for _, path := range paths {
		build := factories[path]
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

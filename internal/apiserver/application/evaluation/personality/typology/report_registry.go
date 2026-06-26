package typology

import (
	"fmt"

	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
)

type reportBuilderFunc func(evaluationresult.Outcome) (*domainReport.InterpretReport, error)

// ReportAdapterRegistry resolves report builders by report adapter key.
type ReportAdapterRegistry struct {
	adapters map[modeltypology.ReportAdapterKey]reportBuilderFunc
}

// DefaultReportAdapterRegistry returns the built-in typology report adapters.
func DefaultReportAdapterRegistry() ReportAdapterRegistry {
	return ReportAdapterRegistry{
		adapters: map[modeltypology.ReportAdapterKey]reportBuilderFunc{
			modeltypology.ReportAdapterPersonalityType: buildPersonalityTypeReport,
			modeltypology.ReportAdapterTraitProfile:    buildTraitProfileReport,
			modeltypology.ReportAdapterMBTI:            buildMBTIReport,
			modeltypology.ReportAdapterSBTI:            buildSBTIReport,
			modeltypology.ReportAdapterBigFive:         buildBigFiveReport,
		},
	}
}

func (r ReportAdapterRegistry) build(
	spec modeltypology.ReportSpec,
	mapping modeltypology.OutcomeMappingSpec,
	decisionKind assessmentmodel.DecisionKind,
	outcome evaluationresult.Outcome,
) (*domainReport.InterpretReport, error) {
	adapterKey := spec.ResolvedAdapterKey(mapping, decisionKind)
	if adapterKey == "" {
		return nil, fmt.Errorf("report adapter key is required")
	}
	builder, ok := r.adapters[adapterKey]
	if !ok {
		return nil, fmt.Errorf("unsupported report adapter key: %s", adapterKey)
	}
	return builder(outcome)
}

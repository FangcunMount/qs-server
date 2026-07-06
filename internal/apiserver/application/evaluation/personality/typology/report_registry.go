package typology

import (
	"fmt"

	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
)

type reportBuilderFunc func(evaluationresult.Outcome) (*domainReport.InterpretReport, error)

// ReportAdapterRegistry resolves report builders by report adapter key.
type ReportAdapterRegistry struct {
	adapters map[modeltypology.ReportAdapterKey]reportBuilderFunc
}

// DefaultReportAdapterRegistry returns the built-in typology report adapters.
func DefaultReportAdapterRegistry() ReportAdapterRegistry {
	return NewReportAdapterRegistry()
}

// NewReportAdapterRegistry returns the built-in typology report adapters.
func NewReportAdapterRegistry() ReportAdapterRegistry {
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

// Len reports how many report builders are registered.
func (r ReportAdapterRegistry) Len() int {
	return len(r.adapters)
}

// Register returns a registry copy with an additional or overridden report builder.
func (r ReportAdapterRegistry) Register(key modeltypology.ReportAdapterKey, builder reportBuilderFunc) ReportAdapterRegistry {
	next := ReportAdapterRegistry{adapters: make(map[modeltypology.ReportAdapterKey]reportBuilderFunc, len(r.adapters)+1)}
	for k, v := range r.adapters {
		next.adapters[k] = v
	}
	next.adapters[key] = builder
	return next
}

func (r ReportAdapterRegistry) build(
	spec modeltypology.ReportSpec,
	mapping modeltypology.OutcomeMappingSpec,
	decisionKind modelcatalog.DecisionKind,
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

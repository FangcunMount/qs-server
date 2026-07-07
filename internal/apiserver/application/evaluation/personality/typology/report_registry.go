package typology

import (
	"fmt"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

type reportBuilderFunc func(evaloutcome.Outcome) (*domainReport.InterpretReport, error)

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
			modeltypology.ReportAdapterPersonalityType: buildTypologyReportAdapter(modeltypology.ReportAdapterPersonalityType),
			modeltypology.ReportAdapterTraitProfile:    buildTypologyReportAdapter(modeltypology.ReportAdapterTraitProfile),
			modeltypology.ReportAdapterMBTI:            buildTypologyReportAdapter(modeltypology.ReportAdapterMBTI),
			modeltypology.ReportAdapterSBTI:            buildTypologyReportAdapter(modeltypology.ReportAdapterSBTI),
			modeltypology.ReportAdapterBigFive:         buildTypologyReportAdapter(modeltypology.ReportAdapterBigFive),
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
	outcome evaloutcome.Outcome,
) (*domainReport.InterpretReport, error) {
	adapterKey := spec.ResolvedAdapterKey(mapping, decisionKind)
	return r.buildByAdapter(adapterKey, outcome)
}

func (r ReportAdapterRegistry) buildByAdapter(
	adapterKey modeltypology.ReportAdapterKey,
	outcome evaloutcome.Outcome,
) (*domainReport.InterpretReport, error) {
	if adapterKey == "" {
		return nil, fmt.Errorf("report adapter key is required")
	}
	builder, ok := r.adapters[adapterKey]
	if !ok {
		return nil, fmt.Errorf("unsupported report adapter key: %s", adapterKey)
	}
	return builder(outcome)
}

// buildTypologyReportAdapter returns a report builder for a fixed adapter key.
func buildTypologyReportAdapter(adapterKey modeltypology.ReportAdapterKey) reportBuilderFunc {
	return func(outcome evaloutcome.Outcome) (*domainReport.InterpretReport, error) {
		return buildTypologyReport(adapterKey, outcome)
	}
}

func buildTypologyReport(
	adapterKey modeltypology.ReportAdapterKey,
	outcome evaloutcome.Outcome,
) (*domainReport.InterpretReport, error) {
	switch adapterKey {
	case modeltypology.ReportAdapterPersonalityType, modeltypology.ReportAdapterMBTI, modeltypology.ReportAdapterSBTI:
		return buildPersonalityTypeReport(adapterKey, outcome)
	case modeltypology.ReportAdapterTraitProfile, modeltypology.ReportAdapterBigFive:
		return buildTraitProfileReport(adapterKey, outcome)
	default:
		return nil, fmt.Errorf("unsupported report adapter key: %s", adapterKey)
	}
}

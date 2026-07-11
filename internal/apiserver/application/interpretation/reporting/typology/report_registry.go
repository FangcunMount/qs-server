package typology

import modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"

// ReportAdapterRegistry records the configured Interpretation-owned typology
// adapters. Builders consume InterpretationInput directly; the registry no
// longer stores functions over Evaluation outcomes.
type ReportAdapterRegistry struct {
	adapters map[modeltypology.ReportAdapterKey]struct{}
}

func DefaultReportAdapterRegistry() ReportAdapterRegistry {
	return NewReportAdapterRegistry()
}

func NewReportAdapterRegistry() ReportAdapterRegistry {
	return ReportAdapterRegistry{adapters: map[modeltypology.ReportAdapterKey]struct{}{
		modeltypology.ReportAdapterPersonalityType: {},
		modeltypology.ReportAdapterTraitProfile:    {},
		modeltypology.ReportAdapterMBTI:            {},
		modeltypology.ReportAdapterSBTI:            {},
		modeltypology.ReportAdapterBigFive:         {},
	}}
}

func (r ReportAdapterRegistry) Len() int { return len(r.adapters) }

func (r ReportAdapterRegistry) Supports(key modeltypology.ReportAdapterKey) bool {
	_, ok := r.adapters[key]
	return ok
}

func (r ReportAdapterRegistry) Register(key modeltypology.ReportAdapterKey) ReportAdapterRegistry {
	next := ReportAdapterRegistry{adapters: make(map[modeltypology.ReportAdapterKey]struct{}, len(r.adapters)+1)}
	for current := range r.adapters {
		next.adapters[current] = struct{}{}
	}
	next.adapters[key] = struct{}{}
	return next
}

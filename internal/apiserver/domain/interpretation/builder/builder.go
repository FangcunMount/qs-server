// Package builder owns mechanism-oriented report builders and registries.
package builder

import (
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	typology "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/factor_classification/typology"
	reportscore "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/factor_scoring"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// ReportBuilder composes an InterpretReport from mechanism-neutral input.
type ReportBuilder = domainreport.ReportBuilder

// MechanismFamily identifies which report builder mechanism to use.
type MechanismFamily = modelcatalog.AlgorithmFamily

const (
	MechanismFactorScoring        = modelcatalog.AlgorithmFamilyFactorScoring
	MechanismFactorClassification = modelcatalog.AlgorithmFamilyFactorClassification
	MechanismFactorNorm           = modelcatalog.AlgorithmFamilyFactorNorm
	MechanismTaskPerformance      = modelcatalog.AlgorithmFamilyTaskPerformance
)

// FactorScoringBuilder builds score-range reports.
func FactorScoringBuilder(composer ReportBuilder, input reportscore.ScaleReportInput) (*domainreport.InterpretReport, error) {
	return reportscore.BuildScaleReport(composer, input)
}

// TypologyBuilder builds factor-classification reports via mechanism templates.
var (
	BuildPersonalityTypeReport = typology.BuildPersonalityTypeReport
	BuildTraitProfileReport    = typology.BuildTraitProfileReport
)

// Registry resolves mechanism builders by algorithm family.
type Registry struct {
	byFamily map[MechanismFamily]ReportBuilder
}

// NewRegistry creates an empty mechanism builder registry.
func NewRegistry() *Registry {
	return &Registry{byFamily: make(map[MechanismFamily]ReportBuilder)}
}

// Register adds a builder for a mechanism family.
func (r *Registry) Register(family MechanismFamily, builder ReportBuilder) {
	if r == nil {
		return
	}
	r.byFamily[family] = builder
}

// Resolve returns the builder for a mechanism family.
func (r *Registry) Resolve(family MechanismFamily) (ReportBuilder, bool) {
	if r == nil {
		return nil, false
	}
	builder, ok := r.byFamily[family]
	return builder, ok
}

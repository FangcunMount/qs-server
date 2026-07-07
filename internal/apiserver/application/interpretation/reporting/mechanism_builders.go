package reporting

import (
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// MechanismReportBuilderKey routes report builders by execution mechanism, not assessment code.
type MechanismReportBuilderKey struct {
	AlgorithmFamily modelcatalog.AlgorithmFamily
	DecisionKind    modelcatalog.DecisionKind
	ReportType      domainReport.ReportType
}

func (k MechanismReportBuilderKey) String() string {
	return k.AlgorithmFamily.String() + "/" + string(k.DecisionKind) + "/" + string(k.ReportType)
}

// MechanismKeyedReportBuilder exposes mechanism routing metadata for a report builder.
// MechanismKey is the primary routing key; Key remains for legacy characterization.
type MechanismKeyedReportBuilder interface {
	ReportBuilder
	MechanismKey() MechanismReportBuilderKey
}

// MultiMechanismKeyedReportBuilder registers additional decision-granularity mechanism keys.
type MultiMechanismKeyedReportBuilder interface {
	MechanismKeyedReportBuilder
	MechanismKeys() []MechanismReportBuilderKey
}

func (b FactorScoringReportBuilder) MechanismKey() MechanismReportBuilderKey {
	return MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		DecisionKind:    modelcatalog.DecisionKindScoreRange,
		ReportType:      b.ReportType(),
	}
}

func (b NormProfileReportBuilder) MechanismKey() MechanismReportBuilderKey {
	return MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorNorm,
		DecisionKind:    modelcatalog.DecisionKindNormLookup,
		ReportType:      b.ReportType(),
	}
}

func (b TaskPerformanceReportBuilder) MechanismKey() MechanismReportBuilderKey {
	return MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyTaskPerformance,
		DecisionKind:    modelcatalog.DecisionKindAbilityLevel,
		ReportType:      b.ReportType(),
	}
}

package reporting

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"

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

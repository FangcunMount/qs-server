package definition

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"

// ParseMeasureSpecFromDefinitionBody materializes target measure/calibration layers from shared legacy payload parts.
func ParseMeasureSpecFromDefinitionBody(dimensions []factor.DimensionRule, interpretRules []factor.InterpretRule) (MeasureSpec, Calibration) {
	return MeasureAndCalibrationFromLegacyFactors(factor.ParseLegacyFactorsFromDefinitionBody(dimensions, interpretRules))
}

// ValidateMeasureSpec checks measure-layer invariants.
func ValidateMeasureSpec(measure MeasureSpec) []factor.HierarchyIssue {
	return factor.ValidateMeasureSpecParts(measure.Factors, measure.FactorGraph, measure.Scoring)
}

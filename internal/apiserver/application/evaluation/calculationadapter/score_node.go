package calculationadapter

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

// ScoreNodesFromMeasureSpec translates catalog measure spec to calculation ScoreNodes.
func ScoreNodesFromMeasureSpec(measure definition.MeasureSpec) []calculation.ScoreNode {
	return factor.CalculationScoreNodesFromMeasureParts(measure.Factors, measure.FactorGraph, measure.Scoring)
}

// ScoreNodesFromSnapshots translates runtime snapshot factors at DTO boundaries.
func ScoreNodesFromSnapshots(factors []factor.FactorSnapshot) []calculation.ScoreNode {
	legacy := factor.LegacyFactorsFromSnapshots(factors)
	return factor.CalculationScoreNodesFromLegacyFactors(legacy)
}

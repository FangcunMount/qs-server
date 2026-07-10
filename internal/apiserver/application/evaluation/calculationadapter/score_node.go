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

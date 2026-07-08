package calculationadapter

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

// ScoreNodesFromFactors translates catalog factor snapshots to calculation ScoreNodes.
func ScoreNodesFromFactors(factors []factor.FactorSnapshot) []calculation.ScoreNode {
	return factor.CalculationScoreNodesFromSnapshots(factors)
}

package behavioralrating

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/projection"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	brief2norm "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/behavioral_rating/brief2"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/behavioral_rating/snapshot"
)

// ApplyFactorProjections enriches a raw scale outcome with composite rollup and Brief-2 norm projection.
func ApplyFactorProjections(
	outcome *assessment.AssessmentOutcome,
	snapshot *behavioralsnapshot.Snapshot,
	subject brief2norm.Subject,
) *assessment.AssessmentOutcome {
	if outcome == nil || snapshot == nil {
		return outcome
	}
	nodes := scoreNodesFromFactors(snapshot.Factors)
	calcResult := calcResultFromOutcome(outcome)
	calcResult = projection.CompositeProjection{Nodes: nodes}.Apply(calcResult)
	calcResult = enrichBrief2CalcResult(calcResult, snapshot, subject)
	calcResult = projection.HierarchyProjection{Nodes: nodes}.Apply(calcResult)
	return mergeCalcResultIntoOutcome(outcome, calcResult)
}

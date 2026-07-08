package norming

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/calculationadapter"
	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/projection"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norming/snapshot"
)

// ApplyFactorProjections 补充原始 scale 结果 使用 复合 rollup 和 常模 投影。
func ApplyFactorProjections(
	outcome *assessment.AssessmentOutcome,
	snapshot *behavioralsnapshot.Snapshot,
	subject calcnorm.Subject,
) *assessment.AssessmentOutcome {
	if outcome == nil || snapshot == nil {
		return outcome
	}
	nodes := calculationadapter.ScoreNodesFromFactors(snapshot.Factors)
	calcResult := calculationadapter.CalcResultFromOutcome(outcome)
	calcResult = projection.CompositeProjection{Nodes: nodes}.Apply(calcResult)
	calcResult = enrichNormCalcResult(calcResult, snapshot, subject)
	calcResult = projection.HierarchyProjection{Nodes: nodes}.Apply(calcResult)
	return calculationadapter.MergeCalcResultIntoOutcome(outcome, calcResult)
}

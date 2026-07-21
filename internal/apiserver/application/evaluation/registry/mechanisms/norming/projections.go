package norming

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/calculationadapter"
	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/projection"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	portevaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
)

// ApplyFactorProjectionsForInput requires canonical Definition.Measure.
func ApplyFactorProjectionsForInput(
	outcome *domainoutcome.Execution,
	input *portevaluationinput.InputSnapshot,
	snapshot *behavioralsnapshot.Snapshot,
	subject calcnorm.Subject,
) (*domainoutcome.Execution, error) {
	if outcome == nil {
		return outcome, nil
	}
	measure, _ := portevaluationinput.MeasureSpecFromSnapshot(input)
	if len(measure.Factors) == 0 {
		return outcome, nil
	}
	nodes := calculationadapter.ScoreNodesFromMeasureSpec(measure)
	calcResult := calculationadapter.CalcResultFromOutcome(outcome)
	calcResult = projection.CompositeProjection{Nodes: nodes}.Apply(calcResult)
	if snapshot != nil {
		var err error
		calcResult, err = enrichNormCalcResult(calcResult, snapshot, subject)
		if err != nil {
			return nil, err
		}
	}
	calcResult = projection.HierarchyProjection{Nodes: nodes}.Apply(calcResult)
	return calculationadapter.MergeCalcResultIntoOutcome(outcome, calcResult), nil
}

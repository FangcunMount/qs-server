package norming

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/calculationadapter"
	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/projection"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	portevaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
)

// ApplyFactorProjections 补充原始 scale 结果 使用 复合 rollup 和 常模 投影。
func ApplyFactorProjections(
	outcome *domainoutcome.Execution,
	snapshot *behavioralsnapshot.Snapshot,
	subject calcnorm.Subject,
) (*domainoutcome.Execution, error) {
	return ApplyFactorProjectionsForInput(outcome, nil, snapshot, subject)
}

// ApplyFactorProjectionsForInput prefers canonical Definition.Measure when present.
func ApplyFactorProjectionsForInput(
	outcome *domainoutcome.Execution,
	input *portevaluationinput.InputSnapshot,
	snapshot *behavioralsnapshot.Snapshot,
	subject calcnorm.Subject,
) (*domainoutcome.Execution, error) {
	if outcome == nil {
		return outcome, nil
	}
	var measure modeldefinition.MeasureSpec
	if spec, ok := portevaluationinput.MeasureSpecFromSnapshot(input); ok {
		measure = spec
	} else if snapshot != nil {
		measure = snapshot.MeasureSpec()
	}
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

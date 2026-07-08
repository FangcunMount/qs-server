package task_performance

import (
	"context"
	"fmt"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	factorscoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/scoring"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	portevaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

// Executor 运行task-performance 评估s via 共享 因子计分 engine。
type Executor struct {
	scoring *factorscoring.Executor
}

var _ evaluationexecute.Evaluator = (*Executor)(nil)

// NewExecutor 创建task-performance 评估 executor。
func NewExecutor(scorer ruleengine.ScaleFactorScorer) *Executor {
	return &Executor{scoring: factorscoring.NewExecutor(scorer)}
}

func (e *Executor) ExecutionIdentity() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentityCognitiveDefault
}

func (e *Executor) Key() evaluation.ExecutionIdentity {
	return e.ExecutionIdentity()
}

func (e *Executor) ExecutionPath() modelcatalog.ExecutionPath {
	return modelcatalog.ExecutionPathCognitiveDescriptor
}

func (e *Executor) Execute(ctx context.Context, input evaluationexecute.ExecutionInput) (*assessment.AssessmentOutcome, error) {
	if e == nil || e.scoring == nil {
		return nil, fmt.Errorf("task_performance evaluation executor is not configured")
	}
	scaleSnapshot, ok := portevaluationinput.CognitiveScaleSnapshot(input.Input)
	if !ok || scaleSnapshot == nil {
		return nil, fmt.Errorf("cognitive model payload is required")
	}
	outcome, err := e.scoring.Execute(ctx, evaluationexecute.ExecutionInput{
		Assessment: input.Assessment,
		Input:      factorscoring.CloneInputWithScaleSnapshot(input.Input, scaleSnapshot),
	})
	if err != nil {
		return nil, err
	}
	return NormalizeOutcome(outcome), nil
}

package task_performance

import (
	"context"
	"fmt"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	factorscoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/scoring"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	portevaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

// Executor runs task-performance evaluations via the shared factor-scoring engine.
type Executor struct {
	scoring *factorscoring.Executor
}

var _ evaluationexecute.Evaluator = (*Executor)(nil)

// NewExecutor creates a task-performance evaluation executor.
func NewExecutor(scorer ruleengine.ScaleFactorScorer) *Executor {
	return &Executor{scoring: factorscoring.NewExecutor(scorer)}
}

func (e *Executor) Key() evaluation.EvaluatorKey {
	return evaluation.EvaluatorKeyCognitiveDefault
}

func (e *Executor) Execute(ctx context.Context, input evaluationexecute.ExecutionInput) (*assessment.AssessmentOutcome, error) {
	if e == nil || e.scoring == nil {
		return nil, fmt.Errorf("task_performance evaluation executor is not configured")
	}
	scaleSnapshot, ok := portevaluationinput.CognitiveScaleSnapshot(input.Input)
	if !ok || scaleSnapshot == nil {
		return nil, fmt.Errorf("cognitive model payload is required")
	}
	return e.scoring.Execute(ctx, evaluationexecute.ExecutionInput{
		Assessment: input.Assessment,
		Input:      factorscoring.CloneInputWithScaleSnapshot(input.Input, scaleSnapshot),
	})
}

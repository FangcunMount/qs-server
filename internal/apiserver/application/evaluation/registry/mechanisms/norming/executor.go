package norming

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

// Executor runs factor-norm evaluations via the shared factor-scoring engine.
type Executor struct {
	scoring *factorscoring.Executor
}

var _ evaluationexecute.Evaluator = (*Executor)(nil)

// NewExecutor creates a factor-norm evaluation executor.
func NewExecutor(scorer ruleengine.ScaleFactorScorer) *Executor {
	return &Executor{scoring: factorscoring.NewExecutor(scorer)}
}

func (e *Executor) Key() evaluation.EvaluatorKey {
	return evaluation.EvaluatorKeyBehavioralRatingDefault
}

func (e *Executor) Execute(ctx context.Context, input evaluationexecute.ExecutionInput) (*assessment.AssessmentOutcome, error) {
	if e == nil || e.scoring == nil {
		return nil, fmt.Errorf("factor_norm evaluation executor is not configured")
	}
	scaleSnapshot, ok := portevaluationinput.BehavioralRatingScaleSnapshot(input.Input)
	if !ok || scaleSnapshot == nil {
		return nil, fmt.Errorf("behavioral_rating model payload is required")
	}
	outcome, err := e.scoring.Execute(ctx, evaluationexecute.ExecutionInput{
		Assessment: input.Assessment,
		Input:      factorscoring.CloneInputWithScaleSnapshot(input.Input, scaleSnapshot),
	})
	if err != nil {
		return nil, err
	}
	payload, ok := portevaluationinput.BehavioralRatingPayload(input.Input)
	if !ok || payload.Snapshot == nil {
		return outcome, nil
	}
	return ApplyFactorProjections(outcome, payload.Snapshot, NormSubjectFromInput(input.Input)), nil
}

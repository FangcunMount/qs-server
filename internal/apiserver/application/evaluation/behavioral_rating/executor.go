package behavioralrating

import (
	"context"
	"fmt"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	scaleEvaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/snapshot"
	portevaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/ruleengine"
)

// Executor runs behavioral_rating models via the shared scale scoring engine.
type Executor struct {
	scale *scaleEvaluation.Executor
}

var _ evaluationexecute.Evaluator = (*Executor)(nil)

func NewExecutor(scorer ruleengine.ScaleFactorScorer) *Executor {
	return &Executor{scale: scaleEvaluation.NewExecutor(scorer)}
}

func (e *Executor) Key() evaluation.EvaluatorKey {
	return evaluation.EvaluatorKeyBehavioralRatingDefault
}

func (e *Executor) Execute(ctx context.Context, input evaluationexecute.ExecutionInput) (*assessment.AssessmentOutcome, error) {
	if e == nil || e.scale == nil {
		return nil, fmt.Errorf("behavioral_rating evaluation executor is not configured")
	}
	scaleSnapshot, ok := portevaluationinput.BehavioralRatingScaleSnapshot(input.Input)
	if !ok || scaleSnapshot == nil {
		return nil, fmt.Errorf("behavioral_rating model payload is required")
	}
	outcome, err := e.scale.Execute(ctx, evaluationexecute.ExecutionInput{
		Assessment: input.Assessment,
		Input:      cloneInputWithScaleSnapshot(input.Input, scaleSnapshot),
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

func cloneInputWithScaleSnapshot(input *portevaluationinput.InputSnapshot, scaleSnapshot *scalesnapshot.ScaleSnapshot) *portevaluationinput.InputSnapshot {
	if input == nil {
		return nil
	}
	cloned := *input
	if scaleSnapshot != nil {
		cloned.ModelPayload = portevaluationinput.ScaleModelPayload{Scale: scaleSnapshot}
		if cloned.Model != nil {
			model := *cloned.Model
			model.Payload = portevaluationinput.ScaleModelPayload{Scale: scaleSnapshot}
			cloned.Model = &model
		}
	}
	return &cloned
}

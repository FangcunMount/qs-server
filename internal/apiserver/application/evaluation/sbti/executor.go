package sbti

import (
	"context"
	"fmt"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type Executor struct {
	scorer Scorer
}

var _ evaluationexecute.Evaluator = (*Executor)(nil)

func NewExecutor() *Executor {
	return &Executor{scorer: NewScorer()}
}

func (e *Executor) Kind() assessment.EvaluationModelKind {
	return assessment.EvaluationModelKindSBTI
}

func (e *Executor) Execute(_ context.Context, input evaluationexecute.ExecutionInput) (*assessment.EvaluationResult, error) {
	if e == nil {
		return nil, fmt.Errorf("sbti evaluator is not configured")
	}
	if input.Assessment == nil {
		return nil, fmt.Errorf("assessment is required")
	}
	if input.Input == nil {
		return nil, fmt.Errorf("evaluation input is required")
	}
	model, ok := port.SBTIPayload(input.Input)
	if !ok {
		return nil, fmt.Errorf("sbti model payload is required")
	}
	detail, err := e.scorer.Score(model, input.Input.AnswerSheet)
	if err != nil {
		return nil, err
	}

	modelRef := modelRefFromExecutionInput(input, model)
	score := detail.Similarity * 100
	summary := assessment.ResultSummary{
		PrimaryLabel: detail.TypeCode,
		Score:        &score,
		Tags:         []string{detail.TypeName, detail.OneLiner},
	}
	return assessment.NewModelEvaluationResult(modelRef, summary, assessment.EvaluationDetail{
		Kind:    assessment.EvaluationModelKindSBTI,
		Payload: detail,
	}), nil
}

func modelRefFromExecutionInput(input evaluationexecute.ExecutionInput, model *port.SBTIModelSnapshot) assessment.EvaluationModelRef {
	if input.Assessment != nil && input.Assessment.EvaluationModelRef() != nil {
		return *input.Assessment.EvaluationModelRef()
	}
	code := port.DefaultSBTIModelCode
	version := port.DefaultSBTIModelVersion
	title := port.DefaultSBTIModelTitle
	if model != nil {
		if model.Code != "" {
			code = model.Code
		}
		if model.Version != "" {
			version = model.Version
		}
		if model.Title != "" {
			title = model.Title
		}
	}
	return assessment.NewEvaluationModelRefByCode(assessment.EvaluationModelKindSBTI, meta.NewCode(code), version, title)
}

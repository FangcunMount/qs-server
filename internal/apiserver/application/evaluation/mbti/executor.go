package mbti

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
	return assessment.EvaluationModelKindMBTI
}

func (e *Executor) Execute(_ context.Context, input evaluationexecute.ExecutionInput) (*assessment.EvaluationResult, error) {
	if e == nil {
		return nil, fmt.Errorf("mbti evaluator is not configured")
	}
	if input.Assessment == nil {
		return nil, fmt.Errorf("assessment is required")
	}
	if input.Input == nil {
		return nil, fmt.Errorf("evaluation input is required")
	}
	model, ok := port.MBTIPayload(input.Input)
	if !ok {
		return nil, fmt.Errorf("mbti model payload is required")
	}
	detail, err := e.scorer.Score(model, input.Input.AnswerSheet)
	if err != nil {
		return nil, err
	}

	modelRef := modelRefFromExecutionInput(input, model)
	summary := assessment.ResultSummary{
		PrimaryLabel: detail.TypeCode,
		Tags:         []string{detail.TypeName, detail.OneLiner},
	}
	return assessment.NewModelEvaluationResult(modelRef, summary, assessment.EvaluationDetail{
		Kind:    assessment.EvaluationModelKindMBTI,
		Payload: detail,
	}), nil
}

func modelRefFromExecutionInput(input evaluationexecute.ExecutionInput, model *port.MBTIModelSnapshot) assessment.EvaluationModelRef {
	if input.Assessment != nil && input.Assessment.EvaluationModelRef() != nil {
		return *input.Assessment.EvaluationModelRef()
	}
	code := port.DefaultMBTIModelCode
	version := port.DefaultMBTIModelVersion
	title := port.DefaultMBTIModelTitle
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
	return assessment.NewEvaluationModelRefByCode(assessment.EvaluationModelKindMBTI, meta.NewCode(code), version, title)
}

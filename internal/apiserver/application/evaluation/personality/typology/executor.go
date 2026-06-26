package typology

import (
	"context"
	"fmt"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type Executor struct {
	algorithm assessmentmodel.Algorithm
}

var _ evaluationexecute.Evaluator = (*Executor)(nil)

func NewMBTIExecutor() *Executor {
	return &Executor{algorithm: assessmentmodel.AlgorithmMBTI}
}

func NewSBTIExecutor() *Executor {
	return &Executor{algorithm: assessmentmodel.AlgorithmSBTI}
}

func (e *Executor) Key() evaluation.EvaluatorKey {
	switch e.algorithm {
	case assessmentmodel.AlgorithmSBTI:
		return evaluation.EvaluatorKeySBTI
	default:
		return evaluation.EvaluatorKeyMBTI
	}
}

func (e *Executor) Execute(_ context.Context, input evaluationexecute.ExecutionInput) (*assessment.AssessmentOutcome, error) {
	if e == nil {
		return nil, fmt.Errorf("personality typology evaluator is not configured")
	}
	if input.Assessment == nil {
		return nil, fmt.Errorf("assessment is required")
	}
	if input.Input == nil {
		return nil, fmt.Errorf("evaluation input is required")
	}
	payload, ok := port.TypologyPayload(input.Input)
	if !ok {
		return nil, fmt.Errorf("personality typology payload is required")
	}
	if payload.Algorithm != e.algorithm {
		return nil, fmt.Errorf("typology algorithm %s does not match executor %s", payload.Algorithm, e.algorithm)
	}

	modelRef := modelRefFromExecutionInput(input, payload)
	switch e.algorithm {
	case assessmentmodel.AlgorithmSBTI:
		return buildSBTIOutcome(modelRef, payload, input.Input.AnswerSheet)
	default:
		return buildMBTIOutcome(modelRef, payload, input.Input.AnswerSheet)
	}
}

func buildMBTIOutcome(
	modelRef assessment.EvaluationModelRef,
	payload *modeltypology.Payload,
	sheet *port.AnswerSheetSnapshot,
) (*assessment.AssessmentOutcome, error) {
	model, err := modeltypology.ToMBTI(payload)
	if err != nil {
		return nil, err
	}
	detail, err := evaluationtypology.ScoreMBTI(model, answerSheetFromPort(sheet))
	if err != nil {
		return nil, err
	}
	outcome := assessment.NewAssessmentOutcome(modelRef, assessment.ResultSummary{
		PrimaryLabel: detail.TypeCode,
		Tags:         []string{detail.TypeName, detail.OneLiner},
	}, assessment.EvaluationDetail{
		Kind:    assessment.EvaluationModelKindPersonality,
		Payload: detail,
	})
	outcome.Primary = &assessment.OutcomeScoreValue{
		Kind:  assessment.OutcomeScoreKindMatchPercent,
		Value: detail.MatchPercent,
		Label: detail.TypeCode,
	}
	outcome.Level = &assessment.OutcomeResultLevel{
		Code:     detail.TypeCode,
		Label:    detail.TypeName,
		Severity: "none",
	}
	return outcome, nil
}

func buildSBTIOutcome(
	modelRef assessment.EvaluationModelRef,
	payload *modeltypology.Payload,
	sheet *port.AnswerSheetSnapshot,
) (*assessment.AssessmentOutcome, error) {
	model, err := modeltypology.ToSBTI(payload)
	if err != nil {
		return nil, err
	}
	detail, err := evaluationtypology.ScoreSBTI(model, answerSheetFromPort(sheet))
	if err != nil {
		return nil, err
	}
	score := detail.Similarity * 100
	outcome := assessment.NewAssessmentOutcome(modelRef, assessment.ResultSummary{
		PrimaryLabel: detail.TypeCode,
		Score:        &score,
		Tags:         []string{detail.TypeName, detail.OneLiner},
	}, assessment.EvaluationDetail{
		Kind:    assessment.EvaluationModelKindPersonality,
		Payload: detail,
	})
	outcome.Primary = &assessment.OutcomeScoreValue{
		Kind:  assessment.OutcomeScoreKindMatchPercent,
		Value: score,
		Label: detail.TypeCode,
	}
	outcome.Level = &assessment.OutcomeResultLevel{
		Code:     detail.TypeCode,
		Label:    detail.TypeName,
		Severity: "none",
	}
	return outcome, nil
}

func modelRefFromExecutionInput(input evaluationexecute.ExecutionInput, payload *modeltypology.Payload) assessment.EvaluationModelRef {
	if input.Assessment != nil && input.Assessment.EvaluationModelRef() != nil {
		return *input.Assessment.EvaluationModelRef()
	}
	code := payload.Code
	version := payload.Version
	title := payload.Title
	if code == "" {
		code = string(payload.Algorithm)
	}
	return assessment.NewEvaluationModelRefWithIdentity(
		assessment.EvaluationModelKindPersonality,
		assessmentmodel.SubKindTypology,
		payload.Algorithm,
		meta.ID(0),
		meta.NewCode(code),
		version,
		title,
	)
}

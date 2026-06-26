package typology

import (
	"context"
	"fmt"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type Executor struct {
	runner algorithmRunner
}

var _ evaluationexecute.Evaluator = (*Executor)(nil)

func NewTypologyExecutor(algorithm assessmentmodel.Algorithm) (*Executor, error) {
	return newExecutor(algorithm)
}

func NewMBTIExecutor() *Executor {
	executor, err := newExecutor(assessmentmodel.AlgorithmMBTI)
	if err != nil {
		panic(err)
	}
	return executor
}

func NewSBTIExecutor() *Executor {
	executor, err := newExecutor(assessmentmodel.AlgorithmSBTI)
	if err != nil {
		panic(err)
	}
	return executor
}

func newExecutor(algorithm assessmentmodel.Algorithm) (*Executor, error) {
	runner, err := algorithmRunnerFor(algorithm)
	if err != nil {
		return nil, err
	}
	return &Executor{runner: runner}, nil
}

func (e *Executor) Key() evaluation.EvaluatorKey {
	if e == nil || e.runner == nil {
		return evaluation.EvaluatorKey{}
	}
	return evaluation.PersonalityTypologyKey(e.runner.algorithm())
}

func (e *Executor) Execute(_ context.Context, input evaluationexecute.ExecutionInput) (*assessment.AssessmentOutcome, error) {
	if e == nil || e.runner == nil {
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
	if payload.Algorithm != e.runner.algorithm() {
		return nil, fmt.Errorf("typology algorithm %s does not match executor %s", payload.Algorithm, e.runner.algorithm())
	}

	modelRef := modelRefFromExecutionInput(input, payload)
	return e.runner.buildOutcome(modelRef, payload, input.Input.AnswerSheet)
}

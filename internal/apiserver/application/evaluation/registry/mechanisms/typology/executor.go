package typology

import (
	"context"
	"fmt"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type Executor struct {
	runner *algorithmRunner
	key    evaluation.ExecutionIdentity
}

func NewConfiguredTypologyExecutor() (*Executor, error) {
	return NewConfiguredTypologyExecutorWithRuntime(DefaultPersonalityRuntime())
}

func NewConfiguredTypologyExecutorWithRuntime(runtime PersonalityRuntime) (*Executor, error) {
	runner, err := runtime.runnerForIdentity(evaluation.ExecutionIdentityPersonalityTypology)
	if err != nil {
		return nil, err
	}
	return &Executor{
		runner: &runner,
		key:    evaluation.ExecutionIdentityPersonalityTypology,
	}, nil
}

func (e *Executor) ExecutionIdentity() evaluation.ExecutionIdentity {
	if e == nil {
		return evaluation.ExecutionIdentity{}
	}
	return e.key
}

func (e *Executor) Key() evaluation.ExecutionIdentity {
	return e.ExecutionIdentity()
}

func (e *Executor) ExecutionPath() modelcatalog.ExecutionPath {
	return modelcatalog.ExecutionPathTypologyDescriptor
}

func (e *Executor) Execute(_ context.Context, input evaluationexecute.ExecutionInput) (*domainoutcome.Execution, error) {
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
	modelRef := modelRefFromExecutionInput(input, payload)
	return e.runner.buildOutcome(modelRef, input.Input, payload, input.Input.AnswerSheet)
}

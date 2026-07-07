package typology

import (
	"context"
	"fmt"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type Executor struct {
	runner          *algorithmRunner
	key             evaluation.EvaluatorKey
	legacyAlgorithm modelcatalog.Algorithm
}

var _ evaluationexecute.Evaluator = (*Executor)(nil)

// NewTypologyExecutor constructs a legacy algorithm-scoped typology executor.
// Deprecated: use NewConfiguredTypologyExecutor for new wiring; legacy keys remain for compat resolve only.
func NewTypologyExecutor(algorithm modelcatalog.Algorithm) (*Executor, error) {
	return NewTypologyExecutorWithRegistry(mustDefaultModuleRegistry(), algorithm)
}

func NewConfiguredTypologyExecutor() (*Executor, error) {
	return NewConfiguredTypologyExecutorWithRegistry(mustDefaultModuleRegistry())
}

func NewConfiguredTypologyExecutorWithRegistry(registry ModuleRegistry) (*Executor, error) {
	runner, err := registry.runnerForKey(evaluation.EvaluatorKeyPersonalityTypology)
	if err != nil {
		return nil, err
	}
	return &Executor{
		runner: &runner,
		key:    evaluation.EvaluatorKeyPersonalityTypology,
	}, nil
}

func NewTypologyExecutorWithRegistry(registry ModuleRegistry, algorithm modelcatalog.Algorithm) (*Executor, error) {
	return newLegacyExecutor(registry, algorithm)
}

func newLegacyExecutor(registry ModuleRegistry, algorithm modelcatalog.Algorithm) (*Executor, error) {
	runner, err := algorithmRunnerFor(registry, algorithm)
	if err != nil {
		return nil, err
	}
	return &Executor{
		runner:          &runner,
		key:             evaluation.PersonalityTypologyKey(algorithm),
		legacyAlgorithm: algorithm,
	}, nil
}

func (e *Executor) Key() evaluation.EvaluatorKey {
	if e == nil {
		return evaluation.EvaluatorKey{}
	}
	return e.key
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
	if e.legacyAlgorithm != "" && payload.Algorithm != e.legacyAlgorithm {
		return nil, fmt.Errorf("typology algorithm %s does not match executor %s", payload.Algorithm, e.legacyAlgorithm)
	}

	modelRef := modelRefFromExecutionInput(input, payload)
	return e.runner.buildOutcome(modelRef, payload, input.Input.AnswerSheet)
}

package execute

import (
	"context"
	"fmt"

	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
)

type descriptorDrivenExecutor struct{}

func (descriptorDrivenExecutor) Execute(
	ctx context.Context,
	desc evalpipeline.RuntimeDescriptor,
	input evalpipeline.ExecutionInput,
) (*domainoutcome.Execution, error) {
	if desc.InputAssembler == nil || desc.Calculator == nil || desc.OutcomeAssembler == nil {
		return nil, fmt.Errorf("descriptor pipeline is incomplete for family %s", desc.AlgorithmFamily)
	}
	calcInput, err := desc.InputAssembler.Assemble(input)
	if err != nil {
		return nil, err
	}
	raw, err := desc.Calculator.Calculate(ctx, calcInput)
	if err != nil {
		return nil, err
	}
	assembled, err := desc.OutcomeAssembler.Assemble(raw)
	if err != nil {
		return nil, err
	}
	if outcome, ok := assembled.(*domainoutcome.Execution); ok && outcome != nil {
		return outcome, nil
	}
	return nil, fmt.Errorf("descriptor outcome assembler returned invalid type %T", assembled)
}

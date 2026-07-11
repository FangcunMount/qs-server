package execute

import (
	"context"
	"fmt"

	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type descriptorDrivenExecutor struct{}

func (descriptorDrivenExecutor) Execute(
	ctx context.Context,
	desc evalpipeline.RuntimeDescriptor,
	input ExecutionInput,
) (*domainoutcome.Execution, error) {
	if desc.InputAssembler == nil || desc.Calculator == nil || desc.OutcomeAssembler == nil {
		return nil, fmt.Errorf("descriptor pipeline is incomplete for family %s", desc.AlgorithmFamily)
	}
	route, ok := modelRouteFromInput(input.Input)
	if !ok {
		return nil, fmt.Errorf("descriptor pipeline requires model route")
	}
	calcInput, err := desc.InputAssembler.Assemble(route)
	if err != nil {
		return nil, err
	}
	raw, err := desc.Calculator.Calculate(ContextWithExecutionInput(ctx, input), calcInput)
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

func descriptorExecutorsFromFamilyEvaluators(
	familyEvaluators map[modelcatalog.AlgorithmFamily]Evaluator,
) map[modelcatalog.AlgorithmFamily]DescriptorExecutor {
	if len(familyEvaluators) == 0 {
		return nil
	}
	out := make(map[modelcatalog.AlgorithmFamily]DescriptorExecutor, len(familyEvaluators))
	for family := range familyEvaluators {
		out[family] = descriptorDrivenExecutor{}
	}
	return out
}

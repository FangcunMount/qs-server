package execute

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type descriptorDrivenExecutor struct {
	fallback Evaluator
}

func (e descriptorDrivenExecutor) Execute(
	ctx context.Context,
	desc evalpipeline.RuntimeDescriptor,
	input ExecutionInput,
) (*assessment.AssessmentOutcome, error) {
	if desc.InputAssembler != nil && desc.Calculator != nil && desc.OutcomeAssembler != nil {
		snapshot, ok := publishedSnapshotFromInput(input.Input)
		if !ok {
			return nil, fmt.Errorf("descriptor pipeline requires published model snapshot")
		}
		calcInput, err := desc.InputAssembler.Assemble(snapshot)
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
		outcome, ok := assembled.(*assessment.AssessmentOutcome)
		if !ok || outcome == nil {
			return nil, fmt.Errorf("descriptor outcome assembler returned invalid type %T", assembled)
		}
		return outcome, nil
	}
	if e.fallback == nil {
		return nil, fmt.Errorf("evaluation descriptor executor is not configured for family %s", desc.AlgorithmFamily)
	}
	return e.fallback.Execute(ctx, input)
}

func descriptorExecutorsFromFamilyEvaluators(
	familyEvaluators map[modelcatalog.AlgorithmFamily]Evaluator,
) map[modelcatalog.AlgorithmFamily]DescriptorExecutor {
	if familyEvaluators == nil {
		return nil
	}
	out := make(map[modelcatalog.AlgorithmFamily]DescriptorExecutor, len(familyEvaluators))
	for family, evaluator := range familyEvaluators {
		out[family] = descriptorDrivenExecutor{fallback: evaluator}
	}
	return out
}

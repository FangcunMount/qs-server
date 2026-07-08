package runtime

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// AttachEvaluatorPipelines wires descriptor pipeline triple from materialized family evaluators.
func AttachEvaluatorPipelines(
	registry *evalpipeline.RuntimeDescriptorRegistry,
	familyEvaluators map[modelcatalog.AlgorithmFamily]execute.Evaluator,
) {
	if registry == nil || len(familyEvaluators) == 0 {
		return
	}
	for family, evaluator := range familyEvaluators {
		if evaluator == nil {
			continue
		}
		desc, ok := registry.DescriptorForFamily(family)
		if !ok {
			continue
		}
		inputAsm, calculator, outcomeAsm := execute.EvaluatorPipelineComponents(evaluator)
		desc.InputAssembler = inputAsm
		desc.Calculator = calculator
		desc.OutcomeAssembler = outcomeAsm
		_ = registry.ReplaceFamilyDescriptor(family, desc)
	}
}

package runtime

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// AttachEvaluatorPipelines wires descriptor pipeline triple from materialized family evaluators.
// Families listed in skipFamilies keep any native pipeline already attached.
func AttachEvaluatorPipelines(
	registry *evalpipeline.RuntimeDescriptorRegistry,
	familyEvaluators map[modelcatalog.AlgorithmFamily]execute.Evaluator,
	skipFamilies ...modelcatalog.AlgorithmFamily,
) {
	if registry == nil || len(familyEvaluators) == 0 {
		return
	}
	skip := make(map[modelcatalog.AlgorithmFamily]struct{}, len(skipFamilies))
	for _, family := range skipFamilies {
		skip[family] = struct{}{}
	}
	for family, evaluator := range familyEvaluators {
		if _, omitted := skip[family]; omitted {
			continue
		}
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

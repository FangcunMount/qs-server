package runtime

import (
	mechanismscoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/scoring"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// NativePipelineDeps 分组 factor_scoring 原生 descriptor pipeline 依赖。
type NativePipelineDeps struct {
	ScaleScorer mechanismscoring.PipelineComponents
}

// AttachNativePipelines wires native descriptor pipeline triple for supported algorithm families.
func AttachNativePipelines(registry *evalpipeline.RuntimeDescriptorRegistry, deps NativePipelineDeps) {
	if registry == nil {
		return
	}
	attachFactorScoringNativePipeline(registry, deps)
}

func attachFactorScoringNativePipeline(registry *evalpipeline.RuntimeDescriptorRegistry, deps NativePipelineDeps) {
	desc, ok := registry.DescriptorForFamily(modelcatalog.AlgorithmFamilyFactorScoring)
	if !ok {
		return
	}
	components := deps.ScaleScorer
	if components.InputAssembler == nil && components.Calculator == nil && components.OutcomeAssembler == nil {
		components = mechanismscoring.NewPipelineComponents(nil)
	}
	desc.InputAssembler = components.InputAssembler
	desc.Calculator = components.Calculator
	desc.OutcomeAssembler = components.OutcomeAssembler
	_ = registry.ReplaceFamilyDescriptor(modelcatalog.AlgorithmFamilyFactorScoring, desc)
}

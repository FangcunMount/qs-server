package runtime

import (
	mechanismnorming "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/norming"
	mechanismscoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/scoring"
	mechanismtask "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/task_performance"
	mechanismtypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// NativePipelineDeps 分组已原生化的 RuntimeDescriptor pipeline 依赖。
type NativePipelineDeps struct {
	ScaleScorer          mechanismscoring.PipelineComponents
	FactorNorm           mechanismnorming.PipelineComponents
	TaskPerformance      mechanismtask.PipelineComponents
	FactorClassification mechanismtypology.PipelineComponents
}

// AttachNativePipelines wires native descriptor pipeline triple for supported algorithm families.
func AttachNativePipelines(registry *evalpipeline.RuntimeDescriptorRegistry, deps NativePipelineDeps) error {
	if registry == nil {
		return nil
	}
	for _, attach := range []func(*evalpipeline.RuntimeDescriptorRegistry, NativePipelineDeps) error{
		attachFactorScoringNativePipeline, attachFactorNormNativePipeline, attachTaskPerformanceNativePipeline, attachFactorClassificationNativePipeline,
	} {
		if err := attach(registry, deps); err != nil {
			return err
		}
	}
	return ValidateFamilyManifestCompleteness(registry)
}

func attachFactorScoringNativePipeline(registry *evalpipeline.RuntimeDescriptorRegistry, deps NativePipelineDeps) error {
	desc, ok := registry.DescriptorForFamily(modelcatalog.AlgorithmFamilyFactorScoring)
	if !ok {
		return nil
	}
	components := deps.ScaleScorer
	if components.InputAssembler == nil && components.Calculator == nil && components.OutcomeAssembler == nil {
		components = mechanismscoring.NewPipelineComponents(nil)
	}
	desc.InputAssembler = components.InputAssembler
	desc.Calculator = components.Calculator
	desc.OutcomeAssembler = components.OutcomeAssembler
	return registry.ReplaceFamilyDescriptor(modelcatalog.AlgorithmFamilyFactorScoring, desc)
}

func attachFactorNormNativePipeline(registry *evalpipeline.RuntimeDescriptorRegistry, deps NativePipelineDeps) error {
	desc, ok := registry.DescriptorForFamily(modelcatalog.AlgorithmFamilyFactorNorm)
	if !ok {
		return nil
	}
	components := deps.FactorNorm
	if components.InputAssembler == nil && components.Calculator == nil && components.OutcomeAssembler == nil {
		components = mechanismnorming.NewPipelineComponents(nil)
	}
	desc.InputAssembler = components.InputAssembler
	desc.Calculator = components.Calculator
	desc.OutcomeAssembler = components.OutcomeAssembler
	return registry.ReplaceFamilyDescriptor(modelcatalog.AlgorithmFamilyFactorNorm, desc)
}

func attachTaskPerformanceNativePipeline(registry *evalpipeline.RuntimeDescriptorRegistry, deps NativePipelineDeps) error {
	desc, ok := registry.DescriptorForFamily(modelcatalog.AlgorithmFamilyTaskPerformance)
	if !ok {
		return nil
	}
	components := deps.TaskPerformance
	if components.InputAssembler == nil && components.Calculator == nil && components.OutcomeAssembler == nil {
		components = mechanismtask.NewPipelineComponents(nil)
	}
	desc.InputAssembler = components.InputAssembler
	desc.Calculator = components.Calculator
	desc.OutcomeAssembler = components.OutcomeAssembler
	return registry.ReplaceFamilyDescriptor(modelcatalog.AlgorithmFamilyTaskPerformance, desc)
}

func attachFactorClassificationNativePipeline(registry *evalpipeline.RuntimeDescriptorRegistry, deps NativePipelineDeps) error {
	desc, ok := registry.DescriptorForFamily(modelcatalog.AlgorithmFamilyFactorClassification)
	if !ok {
		return nil
	}
	components := deps.FactorClassification
	if components.InputAssembler == nil && components.Calculator == nil && components.OutcomeAssembler == nil {
		components = mechanismtypology.NewPipelineComponents()
	}
	desc.InputAssembler = components.InputAssembler
	desc.Calculator = components.Calculator
	desc.OutcomeAssembler = components.OutcomeAssembler
	return registry.ReplaceFamilyDescriptor(modelcatalog.AlgorithmFamilyFactorClassification, desc)
}

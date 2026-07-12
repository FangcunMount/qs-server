package runtime_test

import (
	"reflect"
	"testing"

	factornorm "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/norming"
	factorscoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/scoring"
	taskperformance "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/task_performance"
	factorclassification "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology"
	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestAllNativeFamiliesUseNativePipelineComponents(t *testing.T) {
	t.Parallel()

	registry, err := evalruntime.DefaultRuntimeDescriptorRegistry()
	if err != nil {
		t.Fatalf("DefaultRuntimeDescriptorRegistry: %v", err)
	}
	if err := evalruntime.AttachNativePipelines(registry, evalruntime.NativePipelineDeps{
		ScaleScorer:          factorscoring.NewPipelineComponents(nil),
		FactorNorm:           factornorm.NewPipelineComponents(nil),
		TaskPerformance:      taskperformance.NewPipelineComponents(nil),
		FactorClassification: factorclassification.NewPipelineComponents(),
	}); err != nil {
		t.Fatal(err)
	}

	for _, family := range []modelcatalog.AlgorithmFamily{
		modelcatalog.AlgorithmFamilyFactorScoring,
		modelcatalog.AlgorithmFamilyFactorNorm,
		modelcatalog.AlgorithmFamilyTaskPerformance,
		modelcatalog.AlgorithmFamilyFactorClassification,
	} {
		desc, ok := registry.DescriptorForFamily(family)
		if !ok {
			t.Fatalf("%s descriptor is missing", family)
		}
		if desc.InputAssembler == nil || desc.Calculator == nil || desc.OutcomeAssembler == nil {
			t.Fatalf("%s descriptor pipeline is incomplete", family)
		}
		if reflect.TypeOf(desc.Calculator).Name() == "evaluatorCalculator" {
			t.Fatalf("%s calculator = %T, want native calculator", family, desc.Calculator)
		}
	}
}

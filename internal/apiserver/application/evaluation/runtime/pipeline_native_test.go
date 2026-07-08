package runtime_test

import (
	"reflect"
	"testing"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	factorscoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/scoring"
	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestFactorScoringDescriptorUsesNativePipelineComponents(t *testing.T) {
	t.Parallel()

	registry, err := evalruntime.DefaultRuntimeDescriptorRegistry()
	if err != nil {
		t.Fatalf("DefaultRuntimeDescriptorRegistry: %v", err)
	}
	evalruntime.AttachNativePipelines(registry, evalruntime.NativePipelineDeps{
		ScaleScorer: factorscoring.NewPipelineComponents(nil),
	})

	desc, ok := registry.DescriptorForFamily(modelcatalog.AlgorithmFamilyFactorScoring)
	if !ok {
		t.Fatal("factor_scoring descriptor is missing")
	}
	if desc.InputAssembler == nil || desc.Calculator == nil || desc.OutcomeAssembler == nil {
		t.Fatal("factor_scoring descriptor pipeline is incomplete")
	}
	if reflect.TypeOf(desc.Calculator).Name() == "evaluatorCalculator" {
		t.Fatalf("factor_scoring calculator = %T, want native factorScoringCalculator", desc.Calculator)
	}
}

func TestAttachEvaluatorPipelinesSkipsNativeFactorScoringFamily(t *testing.T) {
	t.Parallel()

	registry, err := evalruntime.DefaultRuntimeDescriptorRegistry()
	if err != nil {
		t.Fatalf("DefaultRuntimeDescriptorRegistry: %v", err)
	}
	evalruntime.AttachNativePipelines(registry, evalruntime.NativePipelineDeps{
		ScaleScorer: factorscoring.NewPipelineComponents(nil),
	})
	nativeDesc, ok := registry.DescriptorForFamily(modelcatalog.AlgorithmFamilyFactorScoring)
	if !ok {
		t.Fatal("factor_scoring descriptor is missing")
	}

	familyEvaluators := map[modelcatalog.AlgorithmFamily]evaluationexecute.Evaluator{
		modelcatalog.AlgorithmFamilyFactorScoring: factorscoring.NewExecutor(nil),
	}
	evalruntime.AttachEvaluatorPipelines(
		registry,
		familyEvaluators,
		modelcatalog.AlgorithmFamilyFactorScoring,
	)

	desc, ok := registry.DescriptorForFamily(modelcatalog.AlgorithmFamilyFactorScoring)
	if !ok {
		t.Fatal("factor_scoring descriptor is missing after skip attach")
	}
	if reflect.TypeOf(desc.Calculator) != reflect.TypeOf(nativeDesc.Calculator) {
		t.Fatalf("factor_scoring calculator changed to %T, want native %T", desc.Calculator, nativeDesc.Calculator)
	}
}

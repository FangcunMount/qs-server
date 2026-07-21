package runtime_test

import (
	"strings"
	"testing"

	factornorm "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/norming"
	factorscoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/scoring"
	taskperformance "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/task_performance"
	factorclassification "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology"
	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestValidateFamilyManifestCompletenessAfterAttach(t *testing.T) {
	t.Parallel()

	registry, err := evalruntime.DefaultRuntimeDescriptorRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if err := evalruntime.AttachNativePipelines(registry, evalruntime.NativePipelineDeps{
		ScaleScorer:          factorscoring.NewPipelineComponents(nil),
		FactorNorm:           factornorm.NewPipelineComponents(nil),
		TaskPerformance:      taskperformance.NewPipelineComponents(nil),
		FactorClassification: factorclassification.NewPipelineComponents(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := evalruntime.ValidateFamilyManifestCompleteness(registry); err != nil {
		t.Fatal(err)
	}
}

func TestValidateFamilyManifestCompletenessDetectsMissingPipeline(t *testing.T) {
	t.Parallel()

	registry, err := evalruntime.DefaultRuntimeDescriptorRegistry()
	if err != nil {
		t.Fatal(err)
	}
	err = evalruntime.ValidateFamilyManifestCompleteness(registry)
	if err == nil {
		t.Fatal("expected incomplete pipeline error before AttachNativePipelines")
	}
	if !strings.Contains(err.Error(), "incomplete native pipeline") {
		t.Fatalf("error = %v, want incomplete native pipeline", err)
	}
}

func TestRequiredFamilyManifestCoversFourStableFamilies(t *testing.T) {
	t.Parallel()

	manifest := evalruntime.RequiredFamilyManifest()
	if len(manifest) != 4 {
		t.Fatalf("manifest len = %d, want 4", len(manifest))
	}
	seen := map[modelcatalog.AlgorithmFamily]bool{}
	for _, entry := range manifest {
		if seen[entry.Family] {
			t.Fatalf("duplicate family %s", entry.Family)
		}
		seen[entry.Family] = true
		if entry.Path == "" {
			t.Fatalf("family %s missing path", entry.Family)
		}
	}
}

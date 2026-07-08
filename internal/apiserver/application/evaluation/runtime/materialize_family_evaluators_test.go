package runtime_test

import (
	"testing"

	factorclassification "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology"
	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestMaterializeFamilyEvaluators(t *testing.T) {
	t.Parallel()

	registry, err := factorclassification.DefaultModuleRegistry()
	if err != nil {
		t.Fatalf("DefaultModuleRegistry: %v", err)
	}
	families, err := evalruntime.MaterializeFamilyEvaluators(evalruntime.WiringDeps{
		TypologyRegistry: registry,
	})
	if err != nil {
		t.Fatalf("MaterializeFamilyEvaluators: %v", err)
	}
	if families[modelcatalog.AlgorithmFamilyFactorClassification] == nil {
		t.Fatal("typology family evaluator missing")
	}
}

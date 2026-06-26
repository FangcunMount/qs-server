package typology_test

import (
	"testing"

	typologyeval "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/personality/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
)

func TestDefaultModuleRegistryResolvesBuiltInModules(t *testing.T) {
	registry, err := typologyeval.DefaultModuleRegistry()
	if err != nil {
		t.Fatalf("DefaultModuleRegistry: %v", err)
	}
	for _, algorithm := range []assessmentmodel.Algorithm{
		assessmentmodel.AlgorithmMBTI,
		assessmentmodel.AlgorithmSBTI,
		assessmentmodel.AlgorithmBigFive,
	} {
		runner, err := typologyeval.NewTypologyExecutorWithRegistry(registry, algorithm)
		if err != nil {
			t.Fatalf("NewTypologyExecutorWithRegistry(%s): %v", algorithm, err)
		}
		if runner.Key().String() == "" {
			t.Fatalf("executor key for %s is empty", algorithm)
		}
	}
}

func TestModuleDescriptorsCoverDefaultModules(t *testing.T) {
	modules := typologyeval.DefaultModules()
	got := typologyeval.ModuleDescriptors(modules)
	if len(got) != len(modules) {
		t.Fatalf("descriptor count = %d, want %d", len(got), len(modules))
	}
	for i, module := range modules {
		if got[i].Algorithm != module.Algorithm {
			t.Fatalf("descriptor[%d] algorithm = %s, want %s", i, got[i].Algorithm, module.Algorithm)
		}
	}
}

func TestBigFiveModuleCanBeRegistered(t *testing.T) {
	registry, err := typologyeval.NewModuleRegistry(typologyeval.AllModules()...)
	if err != nil {
		t.Fatalf("NewModuleRegistry: %v", err)
	}
	executor, err := typologyeval.NewTypologyExecutorWithRegistry(registry, assessmentmodel.AlgorithmBigFive)
	if err != nil {
		t.Fatalf("NewTypologyExecutorWithRegistry: %v", err)
	}
	if executor.Key().String() != "personality/typology/bigfive" {
		t.Fatalf("key = %s", executor.Key().String())
	}
}

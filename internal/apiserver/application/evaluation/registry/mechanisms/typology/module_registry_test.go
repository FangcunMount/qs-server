package typology_test

import (
	"testing"

	factorclassification "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology"
)

func TestDefaultModuleRegistryResolvesBuiltInModules(t *testing.T) {
	registry, err := factorclassification.DefaultModuleRegistry()
	if err != nil {
		t.Fatalf("DefaultModuleRegistry: %v", err)
	}
	executor, err := factorclassification.NewConfiguredTypologyExecutorWithRegistry(registry)
	if err != nil {
		t.Fatalf("NewConfiguredTypologyExecutorWithRegistry: %v", err)
	}
	if executor.Key().String() != "personality/typology/personality_typology" {
		t.Fatalf("executor key = %s", executor.Key().String())
	}
}

func TestModuleDescriptorsCoverDefaultModules(t *testing.T) {
	modules := factorclassification.DefaultModules()
	got := factorclassification.ModuleDescriptors(modules)
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
	registry, err := factorclassification.NewModuleRegistry(factorclassification.AllModules()...)
	if err != nil {
		t.Fatalf("NewModuleRegistry: %v", err)
	}
	executor, err := factorclassification.NewConfiguredTypologyExecutorWithRegistry(registry)
	if err != nil {
		t.Fatalf("NewConfiguredTypologyExecutorWithRegistry: %v", err)
	}
	if executor.Key().String() != "personality/typology/personality_typology" {
		t.Fatalf("key = %s", executor.Key().String())
	}
}

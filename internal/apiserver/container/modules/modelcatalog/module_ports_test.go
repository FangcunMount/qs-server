package modelcatalog

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
)

func TestDescribeExposesAggregateAndLegacyRegisterNames(t *testing.T) {
	t.Parallel()

	desc := Describe()
	if desc.Name != modules.PackageModelCatalog {
		t.Fatalf("name = %q, want instrument", desc.Name)
	}
	want := []string{string(Name), "scale", "personalitymodel"}
	if len(desc.RegisterNames) != len(want) {
		t.Fatalf("register names = %#v, want %#v", desc.RegisterNames, want)
	}
	for i := range want {
		if desc.RegisterNames[i] != want[i] {
			t.Fatalf("register names[%d] = %q, want %q", i, desc.RegisterNames[i], want[i])
		}
	}
}

func TestExportEvaluationCatalogMatchesDefaultDescriptors(t *testing.T) {
	t.Parallel()

	catalog, err := ExportEvaluationCatalog()
	if err != nil {
		t.Fatalf("ExportEvaluationCatalog: %v", err)
	}
	descs := DefaultEvaluationDescriptors()
	if len(catalog.Descriptors) != len(descs) {
		t.Fatalf("descriptor count = %d, want %d", len(catalog.Descriptors), len(descs))
	}
	for i := range descs {
		if catalog.Descriptors[i] != descs[i] {
			t.Fatalf("descriptor[%d] = %#v, want %#v", i, catalog.Descriptors[i], descs[i])
		}
	}
	if catalog.TypologyRegistry.Len() != len(DefaultTypologyModules()) {
		t.Fatalf("registry len = %d, want %d", catalog.TypologyRegistry.Len(), len(DefaultTypologyModules()))
	}
}

func TestModuleExportEvaluationCatalogDelegatesToPackagePort(t *testing.T) {
	t.Parallel()

	module := &Module{}
	got, err := module.ExportEvaluationCatalog()
	if err != nil {
		t.Fatalf("Module.ExportEvaluationCatalog: %v", err)
	}
	want, err := ExportEvaluationCatalog()
	if err != nil {
		t.Fatalf("ExportEvaluationCatalog: %v", err)
	}
	if len(got.Descriptors) != len(want.Descriptors) {
		t.Fatalf("descriptor count = %d, want %d", len(got.Descriptors), len(want.Descriptors))
	}
	if got.TypologyRegistry.Len() != want.TypologyRegistry.Len() {
		t.Fatalf("registry len = %d, want %d", got.TypologyRegistry.Len(), want.TypologyRegistry.Len())
	}
	if got.Descriptors[0].Kind != evaldomain.ModelKindScale {
		t.Fatalf("first descriptor kind = %s, want scale", got.Descriptors[0].Kind)
	}
}

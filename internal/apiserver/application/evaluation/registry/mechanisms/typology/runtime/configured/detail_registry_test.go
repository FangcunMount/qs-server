package configured

import (
	"strings"
	"testing"

	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

func TestDetailAssemblerRegistryRegisterOverridesBuiltin(t *testing.T) {
	marker := modeltypology.DetailAdapterKey("contract_marker")
	registry := DefaultDetailAssemblerRegistry().Register(marker, func(_ DetailInput) (any, error) {
		return "injected-detail", nil
	})
	got, err := registry.Assemble(DetailInput{Adapter: marker})
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	if got != "injected-detail" {
		t.Fatalf("detail = %#v, want injected-detail", got)
	}
}

func TestDetailAssemblerRegistryRejectsUnknownAdapter(t *testing.T) {
	_, err := DefaultDetailAssemblerRegistry().Assemble(DetailInput{
		Adapter: modeltypology.DetailAdapterKey("custom_unknown"),
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported detail adapter key") {
		t.Fatalf("Assemble error = %v, want unsupported detail adapter key", err)
	}
}

func TestDefaultDetailAssemblerRegistryOnlyRegistersMechanismKeys(t *testing.T) {
	registry := DefaultDetailAssemblerRegistry()
	if registry.Len() != 2 {
		t.Fatalf("registry len = %d, want 2 mechanism adapters", registry.Len())
	}
	for _, key := range []modeltypology.DetailAdapterKey{
		modeltypology.DetailAdapterKey("mbti"),
		modeltypology.DetailAdapterKey("sbti"),
		modeltypology.DetailAdapterKey("bigfive"),
	} {
		if _, err := registry.Assemble(DetailInput{Adapter: key}); err == nil {
			t.Fatalf("default registry should not register legacy adapter %s", key)
		}
	}
}

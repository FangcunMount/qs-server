package configured

import (
	"strings"
	"testing"

	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
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

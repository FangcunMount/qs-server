package legacy_test

import (
	"testing"

	typologylegacy "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology/legacy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/typology/configured"
)

func TestRegisterLegacyDetailAssemblersRestoresLegacyKeys(t *testing.T) {
	registry := typologylegacy.RegisterLegacyDetailAssemblers(configured.DefaultDetailAssemblerRegistry())
	if registry.Len() != 5 {
		t.Fatalf("registry len = %d, want 5 with legacy adapters", registry.Len())
	}
}

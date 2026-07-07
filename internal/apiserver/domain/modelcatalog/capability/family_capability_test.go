package capability

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/routing"
)

func TestFamilyCapabilityByKindSeparatesDomainGuards(t *testing.T) {
	t.Parallel()

	family, ok := FamilyCapabilityByKind(identity.KindPersonality)
	if !ok || !family.RuntimeExecutable || family.ExecutionPath == "" {
		t.Fatalf("family capability = %#v", family)
	}
}

func TestCatalogOptionByKindSeparatesPresentation(t *testing.T) {
	t.Parallel()

	option, ok := CatalogOptionByKind(identity.KindPersonality)
	if !ok || option.APIKind != "personality" || option.DisplayName == "" {
		t.Fatalf("catalog option = %#v", option)
	}
}

func TestMergedKindCapabilityPreservesExistingMatrix(t *testing.T) {
	t.Parallel()

	cap, ok := CapabilityByKind(identity.KindScale)
	if !ok {
		t.Fatal("CapabilityByKind(scale) = false")
	}
	if cap.CreateSupported || cap.APIKind != "medical_scale" || cap.ExecutionPath != routing.ExecutionPathScaleDescriptor {
		t.Fatalf("merged capability = %#v", cap)
	}
}

package capability

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

func TestFamilyCapabilityByKindSeparatesDomainGuards(t *testing.T) {
	t.Parallel()

	family, ok := FamilyCapabilityByKind(identity.KindPersonality)
	if !ok || !family.CreateSupported || !family.RuntimeExecutable {
		t.Fatalf("personality family capability = %#v, %v", family, ok)
	}
}

func TestCatalogOptionByKindSeparatesPresentation(t *testing.T) {
	t.Parallel()

	option, ok := CatalogOptionByKind(identity.KindPersonality)
	if !ok || option.APIKind != "personality" || !option.PreviewSupported {
		t.Fatalf("personality catalog option = %#v, %v", option, ok)
	}
}

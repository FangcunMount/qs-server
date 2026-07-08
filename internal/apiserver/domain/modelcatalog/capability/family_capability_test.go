package capability

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

func TestCatalogOptionByKindSeparatesPresentation(t *testing.T) {
	t.Parallel()

	option, ok := CatalogOptionByKind(identity.KindPersonality)
	if !ok || option.APIKind != "personality" || !option.PreviewSupported {
		t.Fatalf("personality catalog option = %#v, %v", option, ok)
	}
}

package option_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/option"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/capability"
)

func TestRegistryAllowsMatchesDefaultCapabilities(t *testing.T) {
	t.Parallel()

	reg := option.NewRegistryFromDomain()
	ops := []capability.CatalogOperation{
		capability.CatalogOpCreate,
		capability.CatalogOpList,
		capability.CatalogOpUpdateBasicInfo,
		capability.CatalogOpDelete,
		capability.CatalogOpPublish,
		capability.CatalogOpUnpublish,
		capability.CatalogOpArchive,
		capability.CatalogOpBindQuestionnaire,
		capability.CatalogOpUpdateDefinition,
		capability.CatalogOpPreview,
		capability.CatalogOpQRCode,
	}
	for _, cap := range capability.DefaultCapabilities() {
		apiKind := cap.APIKind
		if apiKind == "" {
			continue
		}
		for _, op := range ops {
			if reg.Allows(apiKind, op) != cap.Allows(op) {
				t.Fatalf("Allows(%q, %q) mismatch with domain matrix", apiKind, op)
			}
		}
		entry, ok := reg.ByAPIKind(apiKind)
		if !ok {
			t.Fatalf("ByAPIKind(%q) = false", apiKind)
		}
		if entry.OptionsEnabled != cap.OptionsEnabled || entry.DisplayName != cap.DisplayName {
			t.Fatalf("presentation mismatch for %q: %#v vs %#v", apiKind, entry, cap)
		}
	}
}

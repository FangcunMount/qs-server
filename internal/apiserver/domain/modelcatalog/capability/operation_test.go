package capability

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

func TestKindCapabilityAllowsOperations(t *testing.T) {
	t.Parallel()

	personality, ok := CapabilityByKind(identity.KindPersonality)
	if !ok {
		t.Fatal("personality capability missing")
	}
	for _, op := range []CatalogOperation{
		CatalogOpCreate,
		CatalogOpList,
		CatalogOpUpdateBasicInfo,
		CatalogOpDelete,
		CatalogOpPublish,
		CatalogOpUnpublish,
		CatalogOpArchive,
		CatalogOpBindQuestionnaire,
		CatalogOpUpdateDefinition,
		CatalogOpPreview,
		CatalogOpQRCode,
	} {
		if !personality.Allows(op) {
			t.Fatalf("personality must allow %q", op)
		}
	}

	for _, kind := range []identity.Kind{identity.KindScale, identity.KindCustom} {
		cap, ok := CapabilityByKind(kind)
		if !ok {
			t.Fatalf("capability missing for %q", kind)
		}
		for _, op := range []CatalogOperation{
			CatalogOpCreate,
			CatalogOpUpdateDefinition,
			CatalogOpPublish,
			CatalogOpBindQuestionnaire,
		} {
			if cap.Allows(op) {
				t.Fatalf("%s must not allow %q", kind, op)
			}
		}
	}
}

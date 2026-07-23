package management

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestValidateLegacyIdentity(t *testing.T) {
	if err := validateLegacyIdentity(domain.KindTypology, "typology", "typology"); err != nil {
		t.Fatalf("matching legacy identity: %v", err)
	}
	if err := validateLegacyIdentity(domain.KindBehavioralRating, "", "medical_scale"); err == nil {
		t.Fatal("expected incompatible product_channel to be rejected")
	}
	if err := validateLegacyIdentity(domain.KindScale, "typology", ""); err == nil {
		t.Fatal("expected incompatible sub_kind to be rejected")
	}
}

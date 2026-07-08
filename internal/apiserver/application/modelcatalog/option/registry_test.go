package option_test

import (
	"os"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/option"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/capability"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

func TestRegistryUsesApplicationOwnedCatalogMatrix(t *testing.T) {
	t.Parallel()

	reg := option.NewRegistry()
	cases := []struct {
		apiKind        string
		kind           identity.Kind
		displayName    string
		optionsEnabled bool
		create         bool
		list           bool
		publish        bool
		preview        bool
		qrcode         bool
	}{
		{
			apiKind:        "personality",
			kind:           identity.KindPersonality,
			displayName:    "人格测评",
			optionsEnabled: true,
			create:         true,
			list:           true,
			publish:        true,
			preview:        true,
			qrcode:         true,
		},
		{
			apiKind:        "behavioral_rating",
			kind:           identity.KindBehavioralRating,
			displayName:    "行为评分",
			optionsEnabled: true,
			create:         true,
			list:           true,
			publish:        true,
			qrcode:         true,
		},
		{
			apiKind:        "medical_scale",
			kind:           identity.KindScale,
			displayName:    "医学量表",
			optionsEnabled: true,
		},
		{
			apiKind:        "cognitive",
			kind:           identity.KindCognitive,
			displayName:    "认知测评",
			optionsEnabled: true,
			create:         true,
			list:           true,
			publish:        true,
			qrcode:         true,
		},
		{
			apiKind:     "custom",
			kind:        identity.KindCustom,
			displayName: "自定义测评",
		},
	}
	for _, tc := range cases {
		entry, ok := reg.ByAPIKind(tc.apiKind)
		if !ok {
			t.Fatalf("ByAPIKind(%q) = false", tc.apiKind)
		}
		if entry.Kind != tc.kind || entry.DisplayName != tc.displayName || entry.OptionsEnabled != tc.optionsEnabled {
			t.Fatalf("entry(%q) = %#v", tc.apiKind, entry)
		}
		if reg.Allows(tc.apiKind, capability.CatalogOpCreate) != tc.create {
			t.Fatalf("create(%q) mismatch", tc.apiKind)
		}
		if reg.Allows(tc.apiKind, capability.CatalogOpList) != tc.list {
			t.Fatalf("list(%q) mismatch", tc.apiKind)
		}
		if reg.Allows(tc.apiKind, capability.CatalogOpPublish) != tc.publish {
			t.Fatalf("publish(%q) mismatch", tc.apiKind)
		}
		if reg.Allows(tc.apiKind, capability.CatalogOpPreview) != tc.preview {
			t.Fatalf("preview(%q) mismatch", tc.apiKind)
		}
		if reg.Allows(tc.apiKind, capability.CatalogOpQRCode) != tc.qrcode {
			t.Fatalf("qrcode(%q) mismatch", tc.apiKind)
		}
	}
}

func TestRegistryDoesNotSourcePresentationFromDomainDefaults(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("registry.go")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	for _, forbidden := range []string{"DefaultCatalogOptions", "DefaultFamilyCapabilities", "DefaultCapabilities"} {
		if strings.Contains(string(source), forbidden) {
			t.Fatalf("registry.go must not source application options from domain %s", forbidden)
		}
	}
}

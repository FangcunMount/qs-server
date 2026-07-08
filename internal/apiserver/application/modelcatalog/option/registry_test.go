package option_test

import (
	"os"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/option"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
)

func TestRegistryUsesApplicationOwnedCatalogMatrix(t *testing.T) {
	t.Parallel()

	reg := option.NewRegistry()
	cases := []struct {
		apiKind        string
		kind           binding.Kind
		displayName    string
		optionsEnabled bool
		create         bool
		list           bool
		publish        bool
		preview        bool
		qrcode         bool
	}{
		{
			apiKind:        "typology",
			kind:           binding.KindTypology,
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
			kind:           binding.KindBehavioralRating,
			displayName:    "行为评分",
			optionsEnabled: true,
			create:         true,
			list:           true,
			publish:        true,
			qrcode:         true,
		},
		{
			apiKind:        "medical_scale",
			kind:           binding.KindScale,
			displayName:    "医学量表",
			optionsEnabled: true,
		},
		{
			apiKind:        "cognitive",
			kind:           binding.KindCognitive,
			displayName:    "认知测评",
			optionsEnabled: true,
			create:         true,
			list:           true,
			publish:        true,
			qrcode:         true,
		},
		{
			apiKind:     "custom",
			kind:        binding.KindCustom,
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
		if reg.Allows(tc.apiKind, binding.CatalogOpCreate) != tc.create {
			t.Fatalf("create(%q) mismatch", tc.apiKind)
		}
		if reg.Allows(tc.apiKind, binding.CatalogOpList) != tc.list {
			t.Fatalf("list(%q) mismatch", tc.apiKind)
		}
		if reg.Allows(tc.apiKind, binding.CatalogOpPublish) != tc.publish {
			t.Fatalf("publish(%q) mismatch", tc.apiKind)
		}
		if reg.Allows(tc.apiKind, binding.CatalogOpPreview) != tc.preview {
			t.Fatalf("preview(%q) mismatch", tc.apiKind)
		}
		if reg.Allows(tc.apiKind, binding.CatalogOpQRCode) != tc.qrcode {
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

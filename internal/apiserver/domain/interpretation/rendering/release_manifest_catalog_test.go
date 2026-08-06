package rendering

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
)

func TestBuiltinReleaseManifestCatalogReturnsDefensiveCopies(t *testing.T) {
	t.Parallel()

	catalog, err := NewBuiltinReleaseManifestCatalog()
	if err != nil {
		t.Fatal(err)
	}
	first, ok := catalog.ResolveManifest("standard", policy.TemplateVersionV1)
	if !ok {
		t.Fatal("standard@legacy-v1 manifest is missing")
	}
	first.Routes[0].BuilderIdentity = "mutated"
	second, ok := catalog.ResolveManifest("standard", policy.TemplateVersionV1)
	if !ok || second.Routes[0].BuilderIdentity == "mutated" {
		t.Fatal("manifest catalog leaked mutable route state")
	}
	if _, ok := catalog.ResolveManifest("unknown", policy.TemplateVersionV1); ok {
		t.Fatal("unknown template release must not resolve")
	}
}

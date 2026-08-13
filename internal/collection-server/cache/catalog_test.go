package cache

import (
	"testing"

	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
)

func TestCollectionCatalogOwnsFixedL1TopologyMetadata(t *testing.T) {
	want := map[sharedcache.Capability]string{
		"catalog.questionnaire":        "questionnaire",
		"catalog.published_model":      "published-model",
		"evaluation.assessment_detail": "assessment-detail",
		"evaluation.assessment_access": "assessment-access",
	}
	for capability, group := range want {
		spec, ok := lookupCatalogSpec(capability)
		if !ok || spec.TopologyGroup != group || spec.TopologyOrder != 10 || spec.ReadModel == "" || spec.Layer != sharedcache.LayerL1 {
			t.Fatalf("L1 topology spec %q = %#v, found=%v", capability, spec, ok)
		}
	}
	typology, ok := lookupCatalogSpec("catalog.typology")
	if !ok || typology.TopologyGroup != "" {
		t.Fatalf("conditional typology path must remain outside fixed topology: %#v", typology)
	}
}

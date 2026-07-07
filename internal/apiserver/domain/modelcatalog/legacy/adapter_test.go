package legacy

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/routing"
)

func TestLegacyKindMapping(t *testing.T) {
	kind, subKind, algorithm, ok := LegacyKindMapping(identity.KindScale)
	if !ok || kind != identity.KindScale || subKind != identity.SubKindEmpty || algorithm != identity.AlgorithmScaleDefault {
		t.Fatalf("LegacyKindMapping(scale) = (%s,%s,%s,%v), want (scale,,scale_default,true)", kind, subKind, algorithm, ok)
	}
	for _, legacyKind := range []identity.Kind{identity.Kind(KindMBTIMigration), identity.Kind(KindSBTIMigration)} {
		if _, _, _, ok := LegacyKindMapping(legacyKind); ok {
			t.Fatalf("LegacyKindMapping(%s) should be removed from runtime read paths", legacyKind)
		}
	}
}

func TestPublishedLegacyEnvelopeRoundTrip(t *testing.T) {
	legacySnapshot := &Snapshot{
		SchemaVersion: catalog.SchemaVersionV1,
		PayloadFormat: routing.PayloadFormatAssessmentScaleV1,
		Definition: Definition{
			Kind:    identity.KindScale,
			Code:    "PHQ9",
			Version: "1.0.0",
			Title:   "PHQ-9",
			Status:  "published",
		},
		DecisionKind: identity.DecisionKindScoreRange,
		Payload:      []byte(`{"code":"PHQ9","version":"1.0.0","status":"published"}`),
	}
	published := PublishedFromLegacy(legacySnapshot)
	if published.Model.Kind != identity.KindScale || published.Model.Algorithm != identity.AlgorithmScaleDefault {
		t.Fatalf("published model = %#v", published.Model)
	}
	back := LegacyFromPublished(published)
	if back.Definition.Kind != identity.KindScale {
		t.Fatalf("legacy round trip = %#v", back)
	}
}

package legacy

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/publishing"
)

func TestLegacyKindMapping(t *testing.T) {
	kind, subKind, algorithm, ok := LegacyKindMapping(binding.KindScale)
	if !ok || kind != binding.KindScale || subKind != binding.SubKindEmpty || algorithm != binding.AlgorithmScaleDefault {
		t.Fatalf("LegacyKindMapping(scale) = (%s,%s,%s,%v), want (scale,,scale_default,true)", kind, subKind, algorithm, ok)
	}
	for _, legacyKind := range []binding.Kind{binding.Kind(KindMBTIMigration), binding.Kind(KindSBTIMigration)} {
		if _, _, _, ok := LegacyKindMapping(legacyKind); ok {
			t.Fatalf("LegacyKindMapping(%s) should be removed from runtime read paths", legacyKind)
		}
	}
}

func TestPublishedLegacyEnvelopeRoundTrip(t *testing.T) {
	legacySnapshot := &Snapshot{
		SchemaVersion: publishing.SchemaVersionV1,
		PayloadFormat: publishing.PayloadFormatAssessmentScaleV1,
		Definition: Definition{
			Kind:    binding.KindScale,
			Code:    "PHQ9",
			Version: "1.0.0",
			Title:   "PHQ-9",
			Status:  "published",
		},
		DecisionKind: binding.DecisionKindScoreRange,
		Payload:      []byte(`{"code":"PHQ9","version":"1.0.0","status":"published"}`),
	}
	published := PublishedFromLegacy(legacySnapshot)
	if published.Model.Kind != binding.KindScale || published.Model.Algorithm != binding.AlgorithmScaleDefault {
		t.Fatalf("published model = %#v", published.Model)
	}
	back := LegacyFromPublished(published)
	if back.Definition.Kind != binding.KindScale {
		t.Fatalf("legacy round trip = %#v", back)
	}
}

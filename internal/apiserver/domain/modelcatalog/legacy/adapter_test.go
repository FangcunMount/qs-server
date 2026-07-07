package legacy

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/routing"
)

func TestLegacyKindMapping(t *testing.T) {
	tests := []struct {
		legacy        identity.Kind
		wantKind      identity.Kind
		wantSubKind   identity.SubKind
		wantAlgorithm identity.Algorithm
	}{
		{identity.KindScale, identity.KindScale, identity.SubKindEmpty, identity.AlgorithmScaleDefault},
		{identity.Kind(KindMBTIMigration), identity.KindPersonality, identity.SubKindTypology, identity.AlgorithmMBTI},
		{identity.Kind(KindSBTIMigration), identity.KindPersonality, identity.SubKindTypology, identity.AlgorithmSBTI},
	}
	for _, tc := range tests {
		kind, subKind, algorithm, ok := LegacyKindMapping(tc.legacy)
		if !ok || kind != tc.wantKind || subKind != tc.wantSubKind || algorithm != tc.wantAlgorithm {
			t.Fatalf("LegacyKindMapping(%s) = (%s,%s,%s,%v), want (%s,%s,%s,true)",
				tc.legacy, kind, subKind, algorithm, ok, tc.wantKind, tc.wantSubKind, tc.wantAlgorithm)
		}
	}
}

func TestPublishedLegacyEnvelopeRoundTrip(t *testing.T) {
	legacySnapshot := &Snapshot{
		SchemaVersion: catalog.SchemaVersionV1,
		PayloadFormat: routing.PayloadFormatMBTIV1,
		Definition: Definition{
			Kind:    identity.Kind(KindMBTIMigration),
			Code:    "MBTI_OEJTS",
			Version: "1.0.0",
			Title:   "MBTI",
			Status:  "published",
		},
		DecisionKind: identity.DecisionKindPoleComposition,
		Payload:      []byte(`{"code":"MBTI_OEJTS","algorithm":"mbti","version":"1.0.0","status":"published"}`),
	}
	published := PublishedFromLegacy(legacySnapshot)
	if published.Model.Kind != identity.KindPersonality || published.Model.Algorithm != identity.AlgorithmMBTI {
		t.Fatalf("published model = %#v", published.Model)
	}
	back := LegacyFromPublished(published)
	if back.Definition.Kind != identity.KindPersonality || back.PayloadFormat != routing.PayloadFormatMBTIV1 {
		t.Fatalf("legacy round trip = %#v", back)
	}
}

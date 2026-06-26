package assessmentmodel

import "testing"

func TestLegacyKindMapping(t *testing.T) {
	tests := []struct {
		legacy        Kind
		wantKind      Kind
		wantSubKind   SubKind
		wantAlgorithm Algorithm
	}{
		{KindScale, KindScale, SubKindEmpty, AlgorithmScaleDefault},
		{KindMBTIMigration, KindPersonality, SubKindTypology, AlgorithmMBTI},
		{KindSBTIMigration, KindPersonality, SubKindTypology, AlgorithmSBTI},
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
	legacy := &Snapshot{
		SchemaVersion: SchemaVersionV1,
		PayloadFormat: PayloadFormatMBTIV1,
		Definition: Definition{
			Kind:    KindMBTIMigration,
			Code:    "MBTI_OEJTS",
			Version: "1.0.0",
			Title:   "MBTI",
			Status:  "published",
		},
		DecisionKind: DecisionKindPoleComposition,
		Payload:      []byte(`{"code":"MBTI_OEJTS"}`),
	}
	published := PublishedFromLegacy(legacy)
	if published.Model.Kind != KindPersonality || published.Model.Algorithm != AlgorithmMBTI {
		t.Fatalf("published model = %#v", published.Model)
	}
	back := LegacyFromPublished(published)
	if back.Definition.Kind != KindMBTIMigration || back.PayloadFormat != PayloadFormatMBTIV1 {
		t.Fatalf("legacy round trip = %#v", back)
	}
}

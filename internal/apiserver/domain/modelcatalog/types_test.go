package assessmentmodel

import (
	"testing"
)

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

func TestPayloadFormatHelpers(t *testing.T) {
	if IsMBTIPayloadFormat(PayloadFormatPersonalityTypologyV1) {
		t.Fatal("typology v1 must not be treated as legacy MBTI format")
	}
	if IsSBTIPayloadFormat(PayloadFormatPersonalityTypologyV1) {
		t.Fatal("typology v1 must not be treated as legacy SBTI format")
	}
	if !IsMBTIPayloadFormat(PayloadFormatMBTIV1) || !IsMBTIPayloadFormat(PayloadFormatMBTIV1Legacy) {
		t.Fatal("legacy MBTI formats must be recognized")
	}
	if !IsSBTIPayloadFormat(PayloadFormatSBTIV1) || !IsSBTIPayloadFormat(PayloadFormatSBTIV1Legacy) {
		t.Fatal("legacy SBTI formats must be recognized")
	}
	if !IsPersonalityTypologyPayloadFormat(PayloadFormatPersonalityTypologyV1) {
		t.Fatal("typology v1 format must be recognized")
	}
}

func TestAlgorithmFromTypologyPayload(t *testing.T) {
	algorithm, err := AlgorithmFromTypologyPayload([]byte(`{"algorithm":"mbti"}`))
	if err != nil || algorithm != AlgorithmMBTI {
		t.Fatalf("AlgorithmFromTypologyPayload() = (%q, %v), want (mbti, nil)", algorithm, err)
	}
	if _, err := AlgorithmFromTypologyPayload([]byte(`{}`)); err == nil {
		t.Fatal("empty typology payload algorithm must fail")
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

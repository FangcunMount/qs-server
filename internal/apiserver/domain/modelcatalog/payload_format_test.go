package modelcatalog

import "testing"

func TestPayloadFormatForBehavioralRating(t *testing.T) {
	t.Parallel()

	if got := PayloadFormatForBehavioralRating(AlgorithmBrief2); got != PayloadFormatBehavioralRatingBrief2V1 {
		t.Fatalf("brief2 format = %q, want %q", got, PayloadFormatBehavioralRatingBrief2V1)
	}
	if got := PayloadFormatForBehavioralRating(AlgorithmBehavioralRatingDefault); got != PayloadFormatBehavioralRatingDefaultV1 {
		t.Fatalf("default format = %q", got)
	}
	if PayloadFormatBehavioralRatingBrief2V1 == PayloadFormatBehavioralRatingDefaultV1 {
		t.Fatal("brief2 and default payload formats must differ")
	}
}

func TestPayloadFormatForCognitive(t *testing.T) {
	t.Parallel()

	if got := PayloadFormatForCognitive(AlgorithmSPM); got != PayloadFormatCognitiveSPMV1 {
		t.Fatalf("spm format = %q, want %q", got, PayloadFormatCognitiveSPMV1)
	}
	if got := PayloadFormatForCognitive(AlgorithmScaleDefault); got != PayloadFormatCognitiveDefaultV1 {
		t.Fatalf("unknown cognitive algorithm format = %q", got)
	}
	if PayloadFormatCognitiveSPMV1 == PayloadFormatCognitiveDefaultV1 {
		t.Fatal("spm and default cognitive payload formats must differ")
	}
}

func TestDraftPayloadFormatForModel(t *testing.T) {
	t.Parallel()

	if got := DraftPayloadFormatForModel(KindBehavioralRating, AlgorithmBrief2); got != PayloadFormatBehavioralRatingBrief2V1 {
		t.Fatalf("behavioral_rating draft format = %q", got)
	}
	if got := DraftPayloadFormatForModel(KindCognitive, AlgorithmSPM); got != PayloadFormatCognitiveSPMV1 {
		t.Fatalf("cognitive draft format = %q", got)
	}
}

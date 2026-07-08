package routing

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

func TestPayloadFormatForBehavioralRating(t *testing.T) {
	t.Parallel()

	if got := PayloadFormatForBehavioralRating(identity.AlgorithmBrief2); got != PayloadFormatBehavioralRatingDefaultV1 {
		t.Fatalf("brief2 algorithm format = %q, want family default %q", got, PayloadFormatBehavioralRatingDefaultV1)
	}
	if got := PayloadFormatForBehavioralRating(identity.AlgorithmBehavioralRatingDefault); got != PayloadFormatBehavioralRatingDefaultV1 {
		t.Fatalf("default format = %q", got)
	}
	if !IsBehavioralRatingPayloadFormat(PayloadFormatBehavioralRatingBrief2V1) {
		t.Fatal("legacy brief2 format must remain decodable")
	}
}

func TestPayloadFormatForCognitive(t *testing.T) {
	t.Parallel()

	if got := PayloadFormatForCognitive(identity.AlgorithmSPM); got != PayloadFormatCognitiveDefaultV1 {
		t.Fatalf("spm algorithm format = %q, want family default %q", got, PayloadFormatCognitiveDefaultV1)
	}
	if got := PayloadFormatForCognitive(identity.AlgorithmScaleDefault); got != PayloadFormatCognitiveDefaultV1 {
		t.Fatalf("unknown cognitive algorithm format = %q", got)
	}
	if !IsCognitivePayloadFormat(PayloadFormatCognitiveSPMV1) {
		t.Fatal("legacy spm format must remain decodable")
	}
}

func TestDraftPayloadFormatForModel(t *testing.T) {
	t.Parallel()

	if got := DraftPayloadFormatForModel(identity.KindBehavioralRating, identity.AlgorithmBrief2); got != PayloadFormatBehavioralRatingDefaultV1 {
		t.Fatalf("behavioral_rating draft format = %q", got)
	}
	if got := DraftPayloadFormatForModel(identity.KindCognitive, identity.AlgorithmSPM); got != PayloadFormatCognitiveDefaultV1 {
		t.Fatalf("cognitive draft format = %q", got)
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
	if err != nil || algorithm != identity.AlgorithmMBTI {
		t.Fatalf("AlgorithmFromTypologyPayload() = (%q, %v), want (mbti, nil)", algorithm, err)
	}
	if _, err := AlgorithmFromTypologyPayload([]byte(`{}`)); err == nil {
		t.Fatal("empty typology payload algorithm must fail")
	}
}

package publishing_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/publishing"
)

func TestPayloadFormatForBehavioralRating(t *testing.T) {
	t.Parallel()

	if got := publishing.PayloadFormatForBehavioralRating(binding.AlgorithmBrief2); got != publishing.PayloadFormatBehavioralRatingDefaultV1 {
		t.Fatalf("brief2 algorithm format = %q, want family default %q", got, publishing.PayloadFormatBehavioralRatingDefaultV1)
	}
	if got := publishing.PayloadFormatForBehavioralRating(binding.AlgorithmBehavioralRatingDefault); got != publishing.PayloadFormatBehavioralRatingDefaultV1 {
		t.Fatalf("default format = %q", got)
	}
	if !publishing.IsBehavioralRatingPayloadFormat(publishing.PayloadFormatBehavioralRatingBrief2V1) {
		t.Fatal("legacy brief2 format must remain decodable")
	}
}

func TestPayloadFormatForCognitive(t *testing.T) {
	t.Parallel()

	if got := publishing.PayloadFormatForCognitive(binding.AlgorithmSPM); got != publishing.PayloadFormatCognitiveDefaultV1 {
		t.Fatalf("spm algorithm format = %q, want family default %q", got, publishing.PayloadFormatCognitiveDefaultV1)
	}
	if got := publishing.PayloadFormatForCognitive(binding.AlgorithmScaleDefault); got != publishing.PayloadFormatCognitiveDefaultV1 {
		t.Fatalf("unknown cognitive algorithm format = %q", got)
	}
	if !publishing.IsCognitivePayloadFormat(publishing.PayloadFormatCognitiveSPMV1) {
		t.Fatal("legacy spm format must remain decodable")
	}
}

func TestDraftPayloadFormatForModel(t *testing.T) {
	t.Parallel()

	if got := publishing.DraftPayloadFormatForModel(binding.KindBehavioralRating, binding.AlgorithmBrief2); got != publishing.PayloadFormatBehavioralRatingDefaultV1 {
		t.Fatalf("behavioral_rating draft format = %q", got)
	}
	if got := publishing.DraftPayloadFormatForModel(binding.KindCognitive, binding.AlgorithmSPM); got != publishing.PayloadFormatCognitiveDefaultV1 {
		t.Fatalf("cognitive draft format = %q", got)
	}
}

func TestPayloadFormatHelpers(t *testing.T) {
	if publishing.IsMBTIPayloadFormat(publishing.PayloadFormatPersonalityTypologyV1) {
		t.Fatal("typology v1 must not be treated as legacy MBTI format")
	}
	if publishing.IsSBTIPayloadFormat(publishing.PayloadFormatPersonalityTypologyV1) {
		t.Fatal("typology v1 must not be treated as legacy SBTI format")
	}
	if !publishing.IsMBTIPayloadFormat(publishing.PayloadFormatMBTIV1) || !publishing.IsMBTIPayloadFormat(publishing.PayloadFormatMBTIV1Legacy) {
		t.Fatal("legacy MBTI formats must be recognized")
	}
	if !publishing.IsSBTIPayloadFormat(publishing.PayloadFormatSBTIV1) || !publishing.IsSBTIPayloadFormat(publishing.PayloadFormatSBTIV1Legacy) {
		t.Fatal("legacy SBTI formats must be recognized")
	}
	if !publishing.IsPersonalityTypologyPayloadFormat(publishing.PayloadFormatPersonalityTypologyV1) {
		t.Fatal("typology v1 format must be recognized")
	}
}

func TestAlgorithmFromTypologyPayload(t *testing.T) {
	algorithm, err := publishing.AlgorithmFromTypologyPayload([]byte(`{"algorithm":"mbti"}`))
	if err != nil || algorithm != binding.AlgorithmMBTI {
		t.Fatalf("AlgorithmFromTypologyPayload() = (%q, %v), want (mbti, nil)", algorithm, err)
	}
	if _, err := publishing.AlgorithmFromTypologyPayload([]byte(`{}`)); err == nil {
		t.Fatal("empty typology payload algorithm must fail")
	}
}

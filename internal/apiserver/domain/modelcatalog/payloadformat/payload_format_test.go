package payloadformat_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/payloadformat"
)

func TestPayloadFormatForBehavioralRating(t *testing.T) {
	t.Parallel()

	if got := payloadformat.PayloadFormatForBehavioralRating(binding.AlgorithmBrief2); got != payloadformat.PayloadFormatBehavioralRatingDefaultV1 {
		t.Fatalf("brief2 algorithm format = %q, want family default %q", got, payloadformat.PayloadFormatBehavioralRatingDefaultV1)
	}
	if got := payloadformat.PayloadFormatForBehavioralRating(binding.AlgorithmBehavioralRatingDefault); got != payloadformat.PayloadFormatBehavioralRatingDefaultV1 {
		t.Fatalf("default format = %q", got)
	}
	if !payloadformat.IsBehavioralRatingPayloadFormat(payloadformat.PayloadFormatBehavioralRatingBrief2V1) {
		t.Fatal("legacy brief2 format must remain decodable")
	}
}

func TestPayloadFormatForCognitive(t *testing.T) {
	t.Parallel()

	if got := payloadformat.PayloadFormatForCognitive(binding.AlgorithmSPM); got != payloadformat.PayloadFormatCognitiveDefaultV1 {
		t.Fatalf("spm algorithm format = %q, want family default %q", got, payloadformat.PayloadFormatCognitiveDefaultV1)
	}
	if got := payloadformat.PayloadFormatForCognitive(binding.AlgorithmScaleDefault); got != payloadformat.PayloadFormatCognitiveDefaultV1 {
		t.Fatalf("unknown cognitive algorithm format = %q", got)
	}
	if !payloadformat.IsCognitivePayloadFormat(payloadformat.PayloadFormatCognitiveSPMV1) {
		t.Fatal("legacy spm format must remain decodable")
	}
}

func TestDraftPayloadFormatForModel(t *testing.T) {
	t.Parallel()

	if got := payloadformat.DraftPayloadFormatForModel(binding.KindBehavioralRating, binding.AlgorithmBrief2); got != payloadformat.PayloadFormatBehavioralRatingDefaultV1 {
		t.Fatalf("behavioral_rating draft format = %q", got)
	}
	if got := payloadformat.DraftPayloadFormatForModel(binding.KindCognitive, binding.AlgorithmSPM); got != payloadformat.PayloadFormatCognitiveDefaultV1 {
		t.Fatalf("cognitive draft format = %q", got)
	}
}

func TestPayloadFormatHelpers(t *testing.T) {
	t.Parallel()

	if payloadformat.IsMBTIPayloadFormat(payloadformat.PayloadFormatPersonalityTypologyV1) {
		t.Fatal("typology v1 must not be treated as legacy MBTI format")
	}
	if payloadformat.IsSBTIPayloadFormat(payloadformat.PayloadFormatPersonalityTypologyV1) {
		t.Fatal("typology v1 must not be treated as legacy SBTI format")
	}
	if !payloadformat.IsMBTIPayloadFormat(payloadformat.PayloadFormatMBTIV1) || !payloadformat.IsMBTIPayloadFormat(payloadformat.PayloadFormatMBTIV1Legacy) {
		t.Fatal("legacy MBTI formats must be recognized")
	}
	if !payloadformat.IsSBTIPayloadFormat(payloadformat.PayloadFormatSBTIV1) || !payloadformat.IsSBTIPayloadFormat(payloadformat.PayloadFormatSBTIV1Legacy) {
		t.Fatal("legacy SBTI formats must be recognized")
	}
	if !payloadformat.IsPersonalityTypologyPayloadFormat(payloadformat.PayloadFormatPersonalityTypologyV1) {
		t.Fatal("typology v1 format must be recognized")
	}
}

func TestAlgorithmFromTypologyPayload(t *testing.T) {
	t.Parallel()

	algorithm, err := payloadformat.AlgorithmFromTypologyPayload([]byte(`{"algorithm":"mbti"}`))
	if err != nil || algorithm != binding.AlgorithmMBTI {
		t.Fatalf("AlgorithmFromTypologyPayload() = (%q, %v), want (mbti, nil)", algorithm, err)
	}
	if _, err := payloadformat.AlgorithmFromTypologyPayload([]byte(`{}`)); err == nil {
		t.Fatal("empty typology payload algorithm must fail")
	}
}

func TestSchemaVersionsRemainStable(t *testing.T) {
	t.Parallel()

	if payloadformat.SchemaVersionV1 != "1" || payloadformat.SchemaVersionV2 != "2" {
		t.Fatalf("schema versions changed: %q %q", payloadformat.SchemaVersionV1, payloadformat.SchemaVersionV2)
	}
}

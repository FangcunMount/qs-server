package modelcatalog

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestDraftMapperRoundTripProductChannel(t *testing.T) {
	original, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code:           "brief2_demo",
		Kind:           domain.KindBehavioralRating,
		Algorithm:      domain.AlgorithmBrief2,
		ProductChannel: domain.ProductChannelMedicalScale,
		Title:          "BRIEF-2 Demo",
	})
	if err != nil {
		t.Fatalf("NewAssessmentModel: %v", err)
	}

	mapper := NewDraftMapper()
	po := mapper.ToPO(original)
	if po.ProductChannel != string(domain.ProductChannelMedicalScale) {
		t.Fatalf("po.ProductChannel = %q", po.ProductChannel)
	}
	got := mapper.ToDomain(po)
	if got.ProductChannel != domain.ProductChannelMedicalScale {
		t.Fatalf("round trip product channel = %q", got.ProductChannel)
	}
}

func TestDraftMapperDerivesMissingProductChannel(t *testing.T) {
	po := &AssessmentModelPO{
		Code: "legacy_cognitive",
		Kind: string(domain.KindCognitive),
	}
	got := NewDraftMapper().ToDomain(po)
	if got.ProductChannel != domain.ProductChannelCognitive {
		t.Fatalf("derived product channel = %q, want cognitive", got.ProductChannel)
	}
}

func TestPublishedMapperRoundTripProductChannel(t *testing.T) {
	original := &domain.PublishedModelSnapshot{
		SchemaVersion: domain.SchemaVersionV2,
		PayloadFormat: domain.PayloadFormatBehavioralRatingBrief2V1,
		Model: domain.ModelDefinition{
			ProductChannel: domain.ProductChannelMedicalScale,
			Kind:           domain.KindBehavioralRating,
			Algorithm:      domain.AlgorithmBrief2,
			Code:           "brief2",
			Version:        "v1",
			Title:          "BRIEF-2",
			Status:         "published",
		},
		Decision: domain.DecisionSpec{Kind: domain.DecisionKindNormLookup},
		Payload:  []byte(`{}`),
	}

	mapper := NewMapper()
	po := mapper.ToPO(original)
	if po.ModelProductChannel != string(domain.ProductChannelMedicalScale) {
		t.Fatalf("po.ModelProductChannel = %q", po.ModelProductChannel)
	}
	got := mapper.ToPublished(po)
	if got.Model.ProductChannel != domain.ProductChannelMedicalScale {
		t.Fatalf("round trip product channel = %q", got.Model.ProductChannel)
	}
}

func TestPublishedMapperDerivesMissingProductChannel(t *testing.T) {
	po := &PublishedAssessmentModelPO{
		ModelKind:      string(domain.KindPersonality),
		ModelSubKind:   string(domain.SubKindTypology),
		ModelAlgorithm: string(domain.AlgorithmMBTI),
		ModelCode:      "mbti",
		ModelVersion:   "v1",
		Title:          "MBTI",
		Status:         "published",
		DecisionKind:   string(domain.DecisionKindPoleComposition),
		Payload:        []byte(`{}`),
	}
	got := NewMapper().ToPublished(po)
	if got.Model.ProductChannel != domain.ProductChannelPersonality {
		t.Fatalf("derived product channel = %q, want personality", got.Model.ProductChannel)
	}
}

func TestPublishedModelUpsertFilterExcludesProductChannel(t *testing.T) {
	po := &PublishedAssessmentModelPO{
		ModelProductChannel: string(domain.ProductChannelMedicalScale),
		ModelKind:           string(domain.KindBehavioralRating),
		ModelSubKind:        "",
		ModelAlgorithm:      string(domain.AlgorithmBrief2),
		ModelCode:           "brief2",
	}
	filter := publishedModelUpsertFilter(po)
	for key := range filter {
		if key == "model_product_channel" {
			t.Fatalf("upsert filter must not include model_product_channel: %#v", filter)
		}
	}
}

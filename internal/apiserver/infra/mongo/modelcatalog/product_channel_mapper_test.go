package modelcatalog

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

func TestDraftMapperOmitsProductChannelAndDerivesItOnRead(t *testing.T) {
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
	if data, err := po.ToBsonM(); err != nil || data["product_channel"] != nil || data["sub_kind"] != nil {
		t.Fatalf("new head must not persist legacy identity fields: %#v, %v", data, err)
	}
	got := mapper.ToDomain(po)
	if got.ProductChannel != domain.ProductChannelBehaviorAbility {
		t.Fatalf("round trip product channel = %q", got.ProductChannel)
	}
}

func TestDraftMapperDerivesMissingProductChannel(t *testing.T) {
	po := &AssessmentModelPO{
		Code: "legacy_cognitive",
		Kind: string(domain.KindCognitive),
	}
	got := NewDraftMapper().ToDomain(po)
	if got.ProductChannel != domain.ProductChannelBehaviorAbility {
		t.Fatalf("derived product channel = %q, want behavior_ability", got.ProductChannel)
	}
}

func TestPublishedMapperOmitsProductChannelAndDerivesItOnRead(t *testing.T) {
	original := &port.PublishedModel{
		SchemaVersion:  domain.SchemaVersionV2,
		ProductChannel: domain.ProductChannelMedicalScale,
		Kind:           domain.KindBehavioralRating,
		Algorithm:      domain.AlgorithmBrief2,
		Code:           "brief2",
		Version:        "v1",
		Title:          "BRIEF-2",
		Status:         "published",
		DecisionKind:   domain.DecisionKindNormLookup,
	}

	mapper := NewMapper()
	po := mapper.ToPO(original)
	if data, err := po.ToBsonM(); err != nil || data["product_channel"] != nil || data["sub_kind"] != nil {
		t.Fatalf("new snapshot must not persist legacy identity fields: %#v, %v", data, err)
	}
	got := mapper.ToPublished(po)
	if got.ProductChannel != domain.ProductChannelBehaviorAbility {
		t.Fatalf("round trip product channel = %q", got.ProductChannel)
	}
}

func TestPublishedMapperDerivesMissingProductChannel(t *testing.T) {
	po := &PublishedAssessmentModelPO{
		RecordRole:     recordRolePublishedSnapshot,
		Kind:           string(domain.KindTypology),
		Algorithm:      string(domain.AlgorithmPersonalityTypology),
		Code:           "mbti",
		ReleaseVersion: "v1",
		Title:          "MBTI",
		Status:         "published",
		DecisionKind:   string(domain.DecisionKindPoleComposition),
	}
	got := NewMapper().ToPublished(po)
	if got.ProductChannel != domain.ProductChannelTypology {
		t.Fatalf("derived product channel = %q, want typology", got.ProductChannel)
	}
}

func TestPublishedModelUpsertFilterUsesCanonicalIdentityOnly(t *testing.T) {
	po := &PublishedAssessmentModelPO{
		RecordRole: recordRolePublishedSnapshot,
		Kind:       string(domain.KindBehavioralRating),
		Algorithm:  string(domain.AlgorithmBrief2),
		Code:       "brief2",
	}
	filter := publishedModelUpsertFilter(po)
	for key := range filter {
		if key == "product_channel" {
			t.Fatalf("upsert filter must not include product_channel: %#v", filter)
		}
	}
	if _, ok := filter["sub_kind"]; ok {
		t.Fatalf("upsert filter must not include sub_kind: %#v", filter)
	}
}

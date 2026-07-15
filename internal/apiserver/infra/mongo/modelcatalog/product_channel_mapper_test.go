package modelcatalog

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
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
	if got.ProductChannel != domain.ProductChannelBehaviorAbility {
		t.Fatalf("derived product channel = %q, want behavior_ability", got.ProductChannel)
	}
}

func TestDraftMapperLeavesCanonicalProductChannelUntouched(t *testing.T) {
	po := &AssessmentModelPO{
		Code:           "legacy_cognitive",
		Kind:           string(domain.KindCognitive),
		ProductChannel: string(domain.ProductChannelBehaviorAbility),
	}
	got := NewDraftMapper().ToDomain(po)
	if got.ProductChannel != domain.ProductChannelBehaviorAbility {
		t.Fatalf("product channel = %q, want behavior_ability", got.ProductChannel)
	}
}

func TestPublishedMapperRoundTripProductChannel(t *testing.T) {
	original := &port.PublishedModel{
		SchemaVersion:  domain.SchemaVersionV2,
		PayloadFormat:  domain.PayloadFormatBehavioralRatingBrief2V1,
		ProductChannel: domain.ProductChannelMedicalScale,
		Kind:           domain.KindBehavioralRating,
		Algorithm:      domain.AlgorithmBrief2,
		Code:           "brief2",
		Version:        "v1",
		Title:          "BRIEF-2",
		Status:         "published",
		DecisionKind:   domain.DecisionKindNormLookup,
		Payload:        []byte(`{}`),
	}

	mapper := NewMapper()
	po := mapper.ToPO(original)
	if po.ProductChannel != string(domain.ProductChannelMedicalScale) {
		t.Fatalf("po.ProductChannel = %q", po.ProductChannel)
	}
	got := mapper.ToPublished(po)
	if got.ProductChannel != domain.ProductChannelMedicalScale {
		t.Fatalf("round trip product channel = %q", got.ProductChannel)
	}
}

func TestPublishedMapperDerivesMissingProductChannel(t *testing.T) {
	po := &PublishedAssessmentModelPO{
		RecordRole:     recordRolePublishedSnapshot,
		Kind:           string(domain.KindTypology),
		SubKind:        string(domain.SubKindTypology),
		Algorithm:      string(domain.AlgorithmMBTI),
		Code:           "mbti",
		ReleaseVersion: "v1",
		Title:          "MBTI",
		Status:         "published",
		DecisionKind:   string(domain.DecisionKindPoleComposition),
		Payload:        []byte(`{}`),
	}
	got := NewMapper().ToPublished(po)
	if got.ProductChannel != domain.ProductChannelTypology {
		t.Fatalf("derived product channel = %q, want typology", got.ProductChannel)
	}
}

func TestPublishedMapperLeavesCanonicalProductChannelUntouched(t *testing.T) {
	po := &PublishedAssessmentModelPO{
		ProductChannel: string(domain.ProductChannelBehaviorAbility),
		RecordRole:     recordRolePublishedSnapshot,
		Kind:           string(domain.KindCognitive),
		Algorithm:      string(domain.AlgorithmSPM),
		Code:           "spm",
		ReleaseVersion: "v1",
		Title:          "SPM",
		Status:         "published",
		DecisionKind:   string(domain.DecisionKindAbilityLevel),
		Payload:        []byte(`{}`),
	}
	got := NewMapper().ToPublished(po)
	if got.ProductChannel != domain.ProductChannelBehaviorAbility {
		t.Fatalf("product channel = %q, want behavior_ability", got.ProductChannel)
	}
}

func TestPublishedModelUpsertFilterExcludesProductChannel(t *testing.T) {
	po := &PublishedAssessmentModelPO{
		ProductChannel: string(domain.ProductChannelMedicalScale),
		RecordRole:     recordRolePublishedSnapshot,
		Kind:           string(domain.KindBehavioralRating),
		SubKind:        "",
		Algorithm:      string(domain.AlgorithmBrief2),
		Code:           "brief2",
	}
	filter := publishedModelUpsertFilter(po)
	for key := range filter {
		if key == "product_channel" {
			t.Fatalf("upsert filter must not include product_channel: %#v", filter)
		}
	}
	if _, ok := filter["sub_kind"]; ok {
		t.Fatalf("upsert filter must not include empty sub_kind: %#v", filter)
	}
}

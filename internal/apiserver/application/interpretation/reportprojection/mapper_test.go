package reportprojection

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
)

func TestMapperFromRowUsesStoredFrozenProfile(t *testing.T) {
	t.Parallel()

	row := interpretationreadmodel.ReportRow{
		AssessmentID: 42,
		Model:        interpretationreadmodel.ModelIdentityRow{Kind: "scale", Code: "scl-1", Title: "Scale"},
		Dimensions: []interpretationreadmodel.ReportDimensionRow{
			{FactorCode: "f1"},
			{FactorCode: "f2"},
			{FactorCode: "hidden"},
		},
		PresentationProfile: &interpretationreadmodel.PresentationProfileRow{
			VisibleFactorCodes: []string{"f1", "f2"},
			Source:             string(domainreport.PresentationProfileSourceFrozen),
		},
	}
	mapper := Mapper{}
	got, err := mapper.FromRow(context.Background(), row, policy.AudienceParticipant)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Dimensions) != 2 {
		t.Fatalf("dimensions = %d, want 2", len(got.Dimensions))
	}
	if got.PresentationSource != string(domainreport.PresentationProfileSourceFrozen) {
		t.Fatalf("presentation source = %q", got.PresentationSource)
	}
}

func TestMapperFromRowUsesGovernedLegacyArtifactProfile(t *testing.T) {
	t.Parallel()

	row := interpretationreadmodel.ReportRow{
		AssessmentID: 42,
		Model:        interpretationreadmodel.ModelIdentityRow{Kind: "scale", Code: "scl-1", Title: "Scale"},
		Dimensions: []interpretationreadmodel.ReportDimensionRow{
			{FactorCode: "f1"},
			{FactorCode: "hidden"},
		},
		PresentationProfile: &interpretationreadmodel.PresentationProfileRow{
			VisibleFactorCodes: []string{"f1"},
			Source:             string(domainreport.PresentationProfileSourceLegacyArtifact),
		},
	}
	got, err := Mapper{}.FromRow(context.Background(), row, policy.AudienceParticipant)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Dimensions) != 1 || got.Dimensions[0].FactorCode != "f1" {
		t.Fatalf("dimensions = %#v", got.Dimensions)
	}
	if got.PresentationSource != string(domainreport.PresentationProfileSourceLegacyArtifact) {
		t.Fatalf("presentation source = %q", got.PresentationSource)
	}
}

func TestMapperFromRowRejectsMissingFactorScorePresentationProfile(t *testing.T) {
	t.Parallel()

	row := interpretationreadmodel.ReportRow{
		AssessmentID: 42,
		Model:        interpretationreadmodel.ModelIdentityRow{Kind: "scale", Code: "scl-1", Title: "Scale"},
		Dimensions:   []interpretationreadmodel.ReportDimensionRow{{FactorCode: "f1"}},
	}
	if _, err := (Mapper{}).FromRow(context.Background(), row, policy.AudienceParticipant); err == nil {
		t.Fatal("missing factor-score presentation profile must fail closed")
	}
}

func TestMapperFromRowRejectsRetiredDynamicLegacySource(t *testing.T) {
	t.Parallel()

	row := interpretationreadmodel.ReportRow{
		AssessmentID: 42,
		Model:        interpretationreadmodel.ModelIdentityRow{Kind: "scale", Code: "scl-1", Title: "Scale"},
		PresentationProfile: &interpretationreadmodel.PresentationProfileRow{
			VisibleFactorCodes: []string{"f1"},
			Source:             "legacy",
		},
	}
	if _, err := (Mapper{}).FromRow(context.Background(), row, policy.AudienceParticipant); err == nil {
		t.Fatal("retired dynamic legacy source must fail closed")
	}
}

func TestMapperFromRowAppliesAudienceAfterFrozenVisibility(t *testing.T) {
	t.Parallel()

	row := interpretationreadmodel.ReportRow{
		AssessmentID: 42,
		Model:        interpretationreadmodel.ModelIdentityRow{Kind: "typology", Code: "MBTI", Title: "MBTI"},
		Dimensions:   []interpretationreadmodel.ReportDimensionRow{{FactorCode: "d1"}},
		ModelExtra:   &interpretationreadmodel.ReportModelExtraRow{TypeCode: "INTJ"},
	}
	got, err := Mapper{}.FromRow(context.Background(), row, policy.AudienceClinician)
	if err != nil {
		t.Fatal(err)
	}
	if got.ModelExtra != nil {
		t.Fatal("clinician audience must hide model extra after projection")
	}
}

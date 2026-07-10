package evaluation

import (
	"context"
	"testing"
)

func TestNormalizeAssessmentListRequestDefault(t *testing.T) {
	t.Parallel()

	req := &ListAssessmentsRequest{}
	NormalizeAssessmentListRequest(req, AssessmentListPageDefault)
	if req.Page != 1 || req.PageSize != 10 {
		t.Fatalf("page=(%d,%d), want (1,10)", req.Page, req.PageSize)
	}
}

func TestReportDimensionFilterKeepsVisibleFactors(t *testing.T) {
	t.Parallel()

	filter := NewReportDimensionFilter(stubFactorVisibility{factors: []string{"f1", "f2"}, configured: true})
	report := &AssessmentReportResponse{
		Model: ModelIdentityResponse{Kind: "scale", Code: "scl-1"},
		Dimensions: []DimensionInterpretResponse{
			{FactorCode: "f1"},
			{FactorCode: "hidden"},
			{FactorCode: "f2"},
		},
	}
	out, err := filter.Apply(context.Background(), report)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Dimensions) != 2 {
		t.Fatalf("dimensions = %d, want 2", len(out.Dimensions))
	}
}

func TestReportDimensionFilterSkipsPersonalityModels(t *testing.T) {
	t.Parallel()

	filter := NewReportDimensionFilter(stubFactorVisibility{factors: []string{"f1"}, configured: true})
	report := &AssessmentReportResponse{
		Model: ModelIdentityResponse{Kind: personalityModelKind, Code: "MBTI"},
		Dimensions: []DimensionInterpretResponse{
			{FactorCode: "d1"},
			{FactorCode: "d2"},
		},
	}
	out, err := filter.Apply(context.Background(), report)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Dimensions) != 2 {
		t.Fatalf("personality dimensions must be untouched, got %d", len(out.Dimensions))
	}
}

func TestReportDimensionFilterKeepsDimensionsWhenVisibilityIsNotConfigured(t *testing.T) {
	t.Parallel()

	filter := NewReportDimensionFilter(stubFactorVisibility{})
	report := &AssessmentReportResponse{
		Model: ModelIdentityResponse{Kind: "scale", Code: "scl-1"},
		Dimensions: []DimensionInterpretResponse{
			{FactorCode: "f1"},
			{FactorCode: "f2"},
		},
	}
	out, err := filter.Apply(context.Background(), report)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Dimensions) != 2 {
		t.Fatalf("unconfigured visibility must not filter dimensions: %#v", out.Dimensions)
	}
}

type stubFactorVisibility struct {
	factors    []string
	configured bool
}

func (s stubFactorVisibility) VisibleFactorCodes(context.Context, string) (map[string]bool, bool, error) {
	visible := make(map[string]bool, len(s.factors))
	for _, code := range s.factors {
		visible[code] = true
	}
	return visible, s.configured, nil
}

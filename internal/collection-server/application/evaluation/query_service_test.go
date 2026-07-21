package evaluation

import (
	"context"
	"testing"
)

type listAssessmentsReader struct {
	modelKind string
}

func (r *listAssessmentsReader) ListMyAssessments(_ context.Context, _ uint64, _, _, _, _, _, modelKind string, _, _ int32) (*ListAssessmentsResponse, error) {
	r.modelKind = modelKind
	return &ListAssessmentsResponse{}, nil
}

func (r *listAssessmentsReader) GetAssessmentScores(context.Context, uint64, uint64) ([]FactorScoreResponse, error) {
	return nil, nil
}
func (r *listAssessmentsReader) GetFactorTrend(context.Context, uint64, string, int32) ([]TrendPointResponse, error) {
	return nil, nil
}
func (r *listAssessmentsReader) GetHighRiskFactors(context.Context, uint64, uint64) ([]FactorScoreResponse, error) {
	return nil, nil
}
func (r *listAssessmentsReader) GetMyAssessment(context.Context, uint64, uint64) (*AssessmentDetailResponse, error) {
	return nil, nil
}
func (r *listAssessmentsReader) GetAssessmentReport(context.Context, uint64, uint64) (*AssessmentReportResponse, error) {
	return nil, nil
}
func (r *listAssessmentsReader) ResolveAssessmentByAnswerSheetID(context.Context, uint64) (uint64, uint64, error) {
	return 0, 0, nil
}

func TestQueryServiceListMyAssessmentsUsesScaleModelKind(t *testing.T) {
	t.Parallel()

	reader := &listAssessmentsReader{}
	svc := NewQueryService(reader)
	_, err := svc.ListMyAssessments(context.Background(), 1, &ListAssessmentsRequest{AssessmentKind: "medical"})
	if err != nil {
		t.Fatal(err)
	}
	if reader.modelKind != "scale" {
		t.Fatalf("modelKind = %q, want scale", reader.modelKind)
	}
}

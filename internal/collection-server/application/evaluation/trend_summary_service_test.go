package evaluation

import (
	"context"
	"testing"
)

type trendSummaryReader struct {
	listStatus string
	current    *AssessmentDetailResponse
	items      []AssessmentSummaryResponse
	reports    map[uint64]*AssessmentReportResponse
}

func (r *trendSummaryReader) GetAssessmentScores(context.Context, uint64, uint64) ([]FactorScoreResponse, error) {
	return nil, nil
}

func (r *trendSummaryReader) GetFactorTrend(context.Context, uint64, string, int32) ([]TrendPointResponse, error) {
	return nil, nil
}

func (r *trendSummaryReader) GetHighRiskFactors(context.Context, uint64, uint64) ([]FactorScoreResponse, error) {
	return nil, nil
}

func (r *trendSummaryReader) GetMyAssessment(context.Context, uint64, uint64) (*AssessmentDetailResponse, error) {
	return r.current, nil
}

func (r *trendSummaryReader) ListMyAssessments(_ context.Context, _ uint64, status, _, _, _, _, _ string, _ int32, _ int32) (*ListAssessmentsResponse, error) {
	r.listStatus = status
	return &ListAssessmentsResponse{Items: r.items, Total: int32(len(r.items)), Page: 1, PageSize: trendSummaryPageSize, TotalPages: 1}, nil
}

func (r *trendSummaryReader) GetAssessmentReport(_ context.Context, _ uint64, assessmentID uint64) (*AssessmentReportResponse, error) {
	return r.reports[assessmentID], nil
}

func (r *trendSummaryReader) ResolveAssessmentByAnswerSheetID(context.Context, uint64) (uint64, uint64, error) {
	return 0, 0, nil
}

func TestGetAssessmentTrendSummaryUsesCompletedAssessmentHistory(t *testing.T) {
	t.Parallel()

	const (
		testeeID       = uint64(42)
		previousID     = "100"
		currentID      = "101"
		questionnaire  = "3adyDE"
		questionnaireV = "33.0.1"
	)
	reader := &trendSummaryReader{
		current: &AssessmentDetailResponse{
			ID:                   currentID,
			QuestionnaireCode:    questionnaire,
			QuestionnaireVersion: questionnaireV,
			Model:                ModelIdentityResponse{Code: questionnaire, Title: "SNAP-IV"},
			PrimaryScore:         &ScoreValueResponse{Value: 20},
			Level:                &ResultLevelResponse{Code: "normal", Label: "正常"},
			Status:               "evaluated",
			SubmittedAt:          "2026-07-14T10:00:00Z",
		},
		items: []AssessmentSummaryResponse{
			{
				ID:                   previousID,
				QuestionnaireCode:    questionnaire,
				QuestionnaireVersion: questionnaireV,
				Model:                ModelIdentityResponse{Code: questionnaire, Title: "SNAP-IV"},
				PrimaryScore:         &ScoreValueResponse{Value: 18},
				Level:                &ResultLevelResponse{Code: "normal", Label: "正常"},
				Status:               "evaluated",
				SubmittedAt:          "2026-07-01T10:00:00Z",
			},
			{
				ID:                   currentID,
				QuestionnaireCode:    questionnaire,
				QuestionnaireVersion: questionnaireV,
				Model:                ModelIdentityResponse{Code: questionnaire, Title: "SNAP-IV"},
				PrimaryScore:         &ScoreValueResponse{Value: 20},
				Level:                &ResultLevelResponse{Code: "normal", Label: "正常"},
				Status:               "evaluated",
				SubmittedAt:          "2026-07-14T10:00:00Z",
			},
		},
		reports: map[uint64]*AssessmentReportResponse{
			100: {AssessmentID: previousID},
			101: {AssessmentID: currentID},
		},
	}

	summary, err := NewQueryService(reader, nil).GetAssessmentTrendSummary(context.Background(), testeeID, 101)
	if err != nil {
		t.Fatalf("GetAssessmentTrendSummary() error = %v", err)
	}
	if reader.listStatus != "done" {
		t.Fatalf("history status = %q, want done", reader.listStatus)
	}
	if summary.Meta.ComparableCount != 2 {
		t.Fatalf("comparable count = %d, want 2", summary.Meta.ComparableCount)
	}
	if len(summary.Timeline) != 2 {
		t.Fatalf("timeline length = %d, want 2", len(summary.Timeline))
	}
	if summary.Previous == nil || summary.Previous.AssessmentID != previousID {
		t.Fatalf("previous = %#v, want assessment %s", summary.Previous, previousID)
	}
}

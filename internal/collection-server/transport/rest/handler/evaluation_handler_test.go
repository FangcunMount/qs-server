package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	answersheetapp "github.com/FangcunMount/qs-server/internal/collection-server/application/answersheet"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
	"github.com/FangcunMount/qs-server/pkg/core"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeEvaluationQueryService struct {
	getLegacyMyAssessmentByAnswerSheetID func(ctx context.Context, answerSheetID uint64) (*evaluation.LegacyAssessmentDetailResponse, error)
}

func (f *fakeEvaluationQueryService) GetLegacyMyAssessment(context.Context, uint64, uint64) (*evaluation.LegacyAssessmentDetailResponse, error) {
	panic("unexpected GetLegacyMyAssessment call")
}

func (f *fakeEvaluationQueryService) GetLegacyMyAssessmentByAnswerSheetID(ctx context.Context, answerSheetID uint64) (*evaluation.LegacyAssessmentDetailResponse, error) {
	if f.getLegacyMyAssessmentByAnswerSheetID == nil {
		panic("unexpected GetLegacyMyAssessmentByAnswerSheetID call")
	}
	return f.getLegacyMyAssessmentByAnswerSheetID(ctx, answerSheetID)
}

func (f *fakeEvaluationQueryService) ListLegacyMyAssessments(context.Context, uint64, *evaluation.ListAssessmentsRequest) (*evaluation.LegacyListAssessmentsResponse, error) {
	panic("unexpected ListLegacyMyAssessments call")
}

func (f *fakeEvaluationQueryService) GetAssessmentScores(context.Context, uint64, uint64) ([]evaluation.FactorScoreResponse, error) {
	panic("unexpected GetAssessmentScores call")
}

func (f *fakeEvaluationQueryService) GetLegacyAssessmentReport(context.Context, uint64, uint64) (*evaluation.LegacyAssessmentReportResponse, error) {
	panic("unexpected GetLegacyAssessmentReport call")
}

func (f *fakeEvaluationQueryService) GetFactorTrend(context.Context, uint64, *evaluation.GetFactorTrendRequest) ([]evaluation.TrendPointResponse, error) {
	panic("unexpected GetFactorTrend call")
}

func (f *fakeEvaluationQueryService) GetAssessmentTrendSummary(context.Context, uint64, uint64) (*evaluation.AssessmentTrendSummaryResponse, error) {
	panic("unexpected GetAssessmentTrendSummary call")
}

func (f *fakeEvaluationQueryService) GetHighRiskFactors(context.Context, uint64, uint64) ([]evaluation.FactorScoreResponse, error) {
	panic("unexpected GetHighRiskFactors call")
}

func (f *fakeEvaluationQueryService) GetMyAssessment(context.Context, uint64, uint64) (*evaluation.AssessmentDetailResponse, error) {
	panic("unexpected GetMyAssessment call")
}

func (f *fakeEvaluationQueryService) ListMyAssessments(context.Context, uint64, *evaluation.ListAssessmentsRequest) (*evaluation.ListAssessmentsResponse, error) {
	panic("unexpected ListMyAssessments call")
}

func (f *fakeEvaluationQueryService) GetAssessmentReport(context.Context, uint64, uint64) (*evaluation.AssessmentReportResponse, error) {
	panic("unexpected GetAssessmentReport call")
}

type fakeAnswerSheetLookupService struct {
	get func(ctx context.Context, id uint64) (*answersheetapp.AnswerSheetResponse, error)
}

func (f *fakeAnswerSheetLookupService) Get(ctx context.Context, id uint64) (*answersheetapp.AnswerSheetResponse, error) {
	if f.get == nil {
		panic("unexpected answersheet Get call")
	}
	return f.get(ctx, id)
}

func TestEvaluationHandlerGetMyAssessmentByAnswerSheetIDPending(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewEvaluationHandler(
		&fakeEvaluationQueryService{
			getLegacyMyAssessmentByAnswerSheetID: func(context.Context, uint64) (*evaluation.LegacyAssessmentDetailResponse, error) {
				return nil, status.Error(codes.NotFound, "assessment not found")
			},
		},
		&fakeAnswerSheetLookupService{
			get: func(context.Context, uint64) (*answersheetapp.AnswerSheetResponse, error) {
				return &answersheetapp.AnswerSheetResponse{ID: "123"}, nil
			},
		},
		nil,
	)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/answersheets/123/assessment", nil)
	c.Params = gin.Params{{Key: "id", Value: "123"}}

	handler.GetLegacyMyAssessmentByAnswerSheetID(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}

	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Status    string `json:"status"`
			UpdatedAt int64  `json:"updated_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Code != 0 {
		t.Fatalf("expected business code 0, got %d", resp.Code)
	}
	if resp.Data.Status != "pending" {
		t.Fatalf("expected pending status, got %q", resp.Data.Status)
	}
	if resp.Data.UpdatedAt == 0 {
		t.Fatalf("expected updated_at to be set")
	}
}

func TestEvaluationHandlerGetMyAssessmentByAnswerSheetIDAnswerSheetNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewEvaluationHandler(
		&fakeEvaluationQueryService{
			getLegacyMyAssessmentByAnswerSheetID: func(context.Context, uint64) (*evaluation.LegacyAssessmentDetailResponse, error) {
				return nil, status.Error(codes.NotFound, "assessment not found")
			},
		},
		&fakeAnswerSheetLookupService{
			get: func(context.Context, uint64) (*answersheetapp.AnswerSheetResponse, error) {
				return nil, status.Error(codes.NotFound, "answer sheet not found")
			},
		},
		nil,
	)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/answersheets/123/assessment", nil)
	c.Params = gin.Params{{Key: "id", Value: "123"}}

	handler.GetLegacyMyAssessmentByAnswerSheetID(c)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", recorder.Code)
	}

	var resp core.ErrResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if resp.Code == 0 {
		t.Fatalf("expected non-zero error code")
	}
}

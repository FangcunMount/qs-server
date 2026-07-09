package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
	"github.com/gin-gonic/gin"
)

type fakeEvaluationQueryService struct {
	listMyAssessments func(ctx context.Context, testeeID uint64, req *evaluation.ListAssessmentsRequest) (*evaluation.ListAssessmentsResponse, error)
}

func (f *fakeEvaluationQueryService) ListMyAssessments(ctx context.Context, testeeID uint64, req *evaluation.ListAssessmentsRequest) (*evaluation.ListAssessmentsResponse, error) {
	if f.listMyAssessments == nil {
		panic("unexpected ListMyAssessments call")
	}
	return f.listMyAssessments(ctx, testeeID, req)
}

func (f *fakeEvaluationQueryService) GetAssessmentScores(context.Context, uint64, uint64) ([]evaluation.FactorScoreResponse, error) {
	return nil, nil
}
func (f *fakeEvaluationQueryService) GetFactorTrend(context.Context, uint64, *evaluation.GetFactorTrendRequest) ([]evaluation.TrendPointResponse, error) {
	return nil, nil
}
func (f *fakeEvaluationQueryService) GetAssessmentTrendSummary(context.Context, uint64, uint64) (*evaluation.AssessmentTrendSummaryResponse, error) {
	return nil, nil
}
func (f *fakeEvaluationQueryService) GetHighRiskFactors(context.Context, uint64, uint64) ([]evaluation.FactorScoreResponse, error) {
	return nil, nil
}
func (f *fakeEvaluationQueryService) GetMyAssessment(context.Context, uint64, uint64) (*evaluation.AssessmentDetailResponse, error) {
	return nil, nil
}

func TestEvaluationHandlerListAssessmentsReturnsMedicalList(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewEvaluationHandler(&fakeEvaluationQueryService{
		listMyAssessments: func(_ context.Context, testeeID uint64, req *evaluation.ListAssessmentsRequest) (*evaluation.ListAssessmentsResponse, error) {
			if testeeID != 7 {
				t.Fatalf("testeeID = %d, want 7", testeeID)
			}
			if req.AssessmentKind != "medical" {
				t.Fatalf("AssessmentKind = %q, want medical", req.AssessmentKind)
			}
			return &evaluation.ListAssessmentsResponse{
				Items: []evaluation.AssessmentSummaryResponse{{ID: "42"}},
				Total: 1,
			}, nil
		},
	}, &fakeWaitReportService{})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/assessments?testee_id=7", nil)
	handler.ListAssessments(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var resp struct {
		Data struct {
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data.Total != 1 {
		t.Fatalf("total = %d, want 1", resp.Data.Total)
	}
}

func TestEvaluationHandlerListAssessmentsRequiresTesteeID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewEvaluationHandler(&fakeEvaluationQueryService{}, &fakeWaitReportService{})
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/assessments", nil)
	handler.ListAssessments(c)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", recorder.Code)
	}
}

func TestEvaluationHandlerListAssessmentsRejectsPersonalityKind(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewEvaluationHandler(&fakeEvaluationQueryService{}, &fakeWaitReportService{})
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/assessments?testee_id=7&assessment_kind=personality", nil)
	handler.ListAssessments(c)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", recorder.Code)
	}
}

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
	listMyAssessments   func(ctx context.Context, testeeID uint64, req *evaluation.ListAssessmentsRequest) (*evaluation.ListAssessmentsResponse, error)
	getAssessmentReport func(ctx context.Context, testeeID, assessmentID uint64) (*evaluation.AssessmentReportResponse, error)
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
func (f *fakeEvaluationQueryService) GetAssessmentReport(ctx context.Context, testeeID, assessmentID uint64) (*evaluation.AssessmentReportResponse, error) {
	if f.getAssessmentReport == nil {
		panic("unexpected GetAssessmentReport call")
	}
	return f.getAssessmentReport(ctx, testeeID, assessmentID)
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

func TestEvaluationHandlerGetAssessmentReportReturnsInterpretation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewEvaluationHandler(&fakeEvaluationQueryService{
		getAssessmentReport: func(_ context.Context, testeeID, assessmentID uint64) (*evaluation.AssessmentReportResponse, error) {
			if testeeID != 7 || assessmentID != 42 {
				t.Fatalf("request = testee %d assessment %d, want testee 7 assessment 42", testeeID, assessmentID)
			}
			return &evaluation.AssessmentReportResponse{
				AssessmentID: "42",
				PrimaryScore: &evaluation.ScoreValueResponse{Value: 31},
				Conclusion:   "整体表现正常",
				Dimensions: []evaluation.DimensionInterpretResponse{{
					FactorCode:  "attention",
					FactorName:  "注意缺陷",
					RawScore:    6,
					Description: "注意力表现正常",
					Suggestion:  "保持规律作息",
				}},
			}, nil
		},
	}, &fakeWaitReportService{})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/assessments/42/report?testee_id=7", nil)
	c.Params = gin.Params{{Key: "id", Value: "42"}}
	handler.GetAssessmentReport(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var resp struct {
		Data evaluation.AssessmentReportResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data.PrimaryScore == nil || resp.Data.PrimaryScore.Value != 31 {
		t.Fatalf("primary score = %#v, want 31", resp.Data.PrimaryScore)
	}
	if len(resp.Data.Dimensions) != 1 || resp.Data.Dimensions[0].Description == "" || resp.Data.Dimensions[0].Suggestion == "" {
		t.Fatalf("dimensions = %#v, want interpretation and suggestion", resp.Data.Dimensions)
	}
}

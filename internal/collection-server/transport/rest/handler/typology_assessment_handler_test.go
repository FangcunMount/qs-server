package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	personalityassessment "github.com/FangcunMount/qs-server/internal/collection-server/application/typologyassessment"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeTypologyAssessmentQueryService struct {
	getReport func(ctx context.Context, testeeID, assessmentID uint64) (*personalityassessment.AssessmentReportResponse, error)
}

func (f *fakeTypologyAssessmentQueryService) List(context.Context, uint64, *personalityassessment.ListAssessmentsRequest) (*personalityassessment.ListAssessmentsResponse, error) {
	panic("unexpected List call")
}

func (f *fakeTypologyAssessmentQueryService) Get(context.Context, uint64, uint64) (*personalityassessment.AssessmentDetailResponse, error) {
	panic("unexpected Get call")
}

func (f *fakeTypologyAssessmentQueryService) GetReport(ctx context.Context, testeeID, assessmentID uint64) (*personalityassessment.AssessmentReportResponse, error) {
	if f.getReport == nil {
		panic("unexpected GetReport call")
	}
	return f.getReport(ctx, testeeID, assessmentID)
}

func (f *fakeTypologyAssessmentQueryService) GetReportStatus(context.Context, uint64, uint64) (*personalityassessment.AssessmentStatusResponse, error) {
	panic("unexpected GetReportStatus call")
}

func (f *fakeTypologyAssessmentQueryService) WaitReport(context.Context, uint64, uint64, time.Duration) (*personalityassessment.AssessmentStatusResponse, error) {
	panic("unexpected WaitReport call")
}

func TestTypologyAssessmentHandlerGetReportRequiresTesteeID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewTypologyAssessmentHandler(
		&fakeTypologyAssessmentQueryService{},
		nil,
	)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/typology-assessments/42/report", nil)
	c.Params = gin.Params{{Key: "id", Value: "42"}}

	handler.GetReport(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", recorder.Code)
	}
}

func TestTypologyAssessmentHandlerGetReportRejectsWrongTestee(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewTypologyAssessmentHandler(
		&fakeTypologyAssessmentQueryService{
			getReport: func(context.Context, uint64, uint64) (*personalityassessment.AssessmentReportResponse, error) {
				return nil, status.Error(codes.PermissionDenied, "forbidden")
			},
		},
		nil,
	)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/typology-assessments/42/report?testee_id=7", nil)
	c.Params = gin.Params{{Key: "id", Value: "42"}}

	handler.GetReport(c)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", recorder.Code)
	}
}

func TestTypologyAssessmentHandlerGetReportReturnsReportForOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewTypologyAssessmentHandler(
		&fakeTypologyAssessmentQueryService{
			getReport: func(_ context.Context, testeeID, assessmentID uint64) (*personalityassessment.AssessmentReportResponse, error) {
				if testeeID != 7 || assessmentID != 42 {
					t.Fatalf("unexpected ids: testee=%d assessment=%d", testeeID, assessmentID)
				}
				return &personalityassessment.AssessmentReportResponse{
					AssessmentID: "42",
					ModelExtra:   &personalityassessment.ModelExtraResponse{ImageURL: "https://qs.example/api/v1/assessment-assets/typology/MBTI/INTJ/portrait.png"},
				}, nil
			},
		},
		nil,
	)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/typology-assessments/42/report?testee_id=7", nil)
	c.Params = gin.Params{{Key: "id", Value: "42"}}

	handler.GetReport(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			AssessmentID string `json:"assessment_id"`
			ModelExtra   struct {
				ImageURL string `json:"image_url"`
			} `json:"model_extra"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Data.AssessmentID != "42" {
		t.Fatalf("expected assessment_id 42, got %q", resp.Data.AssessmentID)
	}
	if resp.Data.ModelExtra.ImageURL != "https://qs.example/api/v1/assessment-assets/typology/MBTI/INTJ/portrait.png" {
		t.Fatalf("model_extra.image_url = %q", resp.Data.ModelExtra.ImageURL)
	}
}

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportwait"
	personalityassessment "github.com/FangcunMount/qs-server/internal/collection-server/application/typologyassessment"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeTypologyAssessmentQueryService struct {
	getReport       func(ctx context.Context, testeeID, assessmentID uint64) (*personalityassessment.AssessmentReportResponse, error)
	getReportStatus func(ctx context.Context, testeeID, assessmentID uint64) (*personalityassessment.AssessmentStatusResponse, error)
	waitReport      func(ctx context.Context, testeeID, assessmentID uint64, timeout time.Duration) (*personalityassessment.AssessmentStatusResponse, error)
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

func (f *fakeTypologyAssessmentQueryService) GetReportStatus(ctx context.Context, testeeID, assessmentID uint64) (*personalityassessment.AssessmentStatusResponse, error) {
	if f.getReportStatus == nil {
		panic("unexpected GetReportStatus call")
	}
	return f.getReportStatus(ctx, testeeID, assessmentID)
}

func (f *fakeTypologyAssessmentQueryService) WaitReport(ctx context.Context, testeeID, assessmentID uint64, timeout time.Duration) (*personalityassessment.AssessmentStatusResponse, error) {
	if f.waitReport == nil {
		panic("unexpected WaitReport call")
	}
	return f.waitReport(ctx, testeeID, assessmentID, timeout)
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

func TestTypologyAssessmentHandlerReportAccessErrorContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	normalizer := reportwait.NewService(nil, nil, nil, nil, reportwait.DefaultConfig())
	accessErrors := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{name: "not found", err: status.Error(codes.NotFound, "missing secret"), wantStatus: http.StatusNotFound},
		{name: "permission denied", err: status.Error(codes.PermissionDenied, "owner secret"), wantStatus: http.StatusNotFound},
		{name: "dependency unavailable", err: status.Error(codes.Unavailable, "database endpoint"), wantStatus: http.StatusServiceUnavailable},
	}
	for _, endpoint := range []string{"status", "wait"} {
		var accessBodies []string
		for _, tt := range accessErrors {
			t.Run(endpoint+"/"+tt.name, func(t *testing.T) {
				query := &fakeTypologyAssessmentQueryService{
					getReportStatus: func(context.Context, uint64, uint64) (*personalityassessment.AssessmentStatusResponse, error) {
						return nil, tt.err
					},
					waitReport: func(context.Context, uint64, uint64, time.Duration) (*personalityassessment.AssessmentStatusResponse, error) {
						return nil, tt.err
					},
				}
				handler := NewTypologyAssessmentHandler(query, normalizer)
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/typology-assessments/42/report-status?testee_id=7", nil)
				c.Params = gin.Params{{Key: "id", Value: "42"}}
				if endpoint == "status" {
					handler.GetReportStatus(c)
				} else {
					handler.WaitReport(c)
				}
				if recorder.Code != tt.wantStatus {
					t.Fatalf("status = %d, want %d; body=%s", recorder.Code, tt.wantStatus, recorder.Body.String())
				}
				if tt.wantStatus == http.StatusNotFound {
					accessBodies = append(accessBodies, recorder.Body.String())
				}
				if strings.Contains(recorder.Body.String(), "secret") || strings.Contains(recorder.Body.String(), "endpoint") {
					t.Fatalf("response leaks dependency detail: %s", recorder.Body.String())
				}
			})
		}
		if len(accessBodies) != 2 || accessBodies[0] != accessBodies[1] {
			t.Fatalf("%s foreign/nonexistent bodies differ: %#v", endpoint, accessBodies)
		}
	}
}

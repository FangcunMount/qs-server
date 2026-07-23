package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	behaviorassessment "github.com/FangcunMount/qs-server/internal/collection-server/application/behaviorassessment"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportwait"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeBehaviorAssessmentQueryService struct {
	get             func(context.Context, uint64, uint64) (*behaviorassessment.AssessmentDetailResponse, error)
	getReportStatus func(context.Context, uint64, uint64) (*behaviorassessment.AssessmentStatusResponse, error)
	waitReport      func(context.Context, uint64, uint64, time.Duration) (*behaviorassessment.AssessmentStatusResponse, error)
}

func (f *fakeBehaviorAssessmentQueryService) List(context.Context, uint64, *behaviorassessment.ListAssessmentsRequest) (*behaviorassessment.ListAssessmentsResponse, error) {
	return nil, errors.New("unexpected List call")
}
func (f *fakeBehaviorAssessmentQueryService) Get(ctx context.Context, testeeID, assessmentID uint64) (*behaviorassessment.AssessmentDetailResponse, error) {
	return f.get(ctx, testeeID, assessmentID)
}
func (f *fakeBehaviorAssessmentQueryService) GetReport(context.Context, uint64, uint64) (*behaviorassessment.AssessmentReportResponse, error) {
	return nil, errors.New("unexpected GetReport call")
}
func (f *fakeBehaviorAssessmentQueryService) GetReportStatus(ctx context.Context, testeeID, assessmentID uint64) (*behaviorassessment.AssessmentStatusResponse, error) {
	if f.getReportStatus == nil {
		return nil, errors.New("unexpected GetReportStatus call")
	}
	return f.getReportStatus(ctx, testeeID, assessmentID)
}
func (f *fakeBehaviorAssessmentQueryService) WaitReport(ctx context.Context, testeeID, assessmentID uint64, timeout time.Duration) (*behaviorassessment.AssessmentStatusResponse, error) {
	if f.waitReport == nil {
		return nil, errors.New("unexpected WaitReport call")
	}
	return f.waitReport(ctx, testeeID, assessmentID, timeout)
}

func TestBehaviorAssessmentHandlerGetMapsOtherKindsToNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewBehaviorAssessmentHandler(&fakeBehaviorAssessmentQueryService{get: func(context.Context, uint64, uint64) (*behaviorassessment.AssessmentDetailResponse, error) {
		return nil, behaviorassessment.ErrNotBehaviorAssessment
	}}, nil)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/behavior-assessments/42?testee_id=7", nil)
	c.Params = gin.Params{{Key: "id", Value: "42"}}

	handler.Get(c)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestBehaviorAssessmentHandlerReportAccessErrorContract(t *testing.T) {
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
				query := &fakeBehaviorAssessmentQueryService{
					get: func(context.Context, uint64, uint64) (*behaviorassessment.AssessmentDetailResponse, error) {
						return nil, errors.New("unexpected Get call")
					},
					getReportStatus: func(context.Context, uint64, uint64) (*behaviorassessment.AssessmentStatusResponse, error) {
						return nil, tt.err
					},
					waitReport: func(context.Context, uint64, uint64, time.Duration) (*behaviorassessment.AssessmentStatusResponse, error) {
						return nil, tt.err
					},
				}
				handler := NewBehaviorAssessmentHandler(query, normalizer)
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/behavior-assessments/42/report-status?testee_id=7", nil)
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

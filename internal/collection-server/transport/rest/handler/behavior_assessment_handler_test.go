package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	behaviorassessment "github.com/FangcunMount/qs-server/internal/collection-server/application/behaviorassessment"
	"github.com/gin-gonic/gin"
)

type fakeBehaviorAssessmentQueryService struct {
	get func(context.Context, uint64, uint64) (*behaviorassessment.AssessmentDetailResponse, error)
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
func (f *fakeBehaviorAssessmentQueryService) GetReportStatus(context.Context, uint64, uint64) (*behaviorassessment.AssessmentStatusResponse, error) {
	return nil, errors.New("unexpected GetReportStatus call")
}
func (f *fakeBehaviorAssessmentQueryService) WaitReport(context.Context, uint64, uint64, time.Duration) (*behaviorassessment.AssessmentStatusResponse, error) {
	return nil, errors.New("unexpected WaitReport call")
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

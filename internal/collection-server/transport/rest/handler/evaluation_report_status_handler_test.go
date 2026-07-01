package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
	"github.com/gin-gonic/gin"
)

type fakeWaitReportService struct {
	getStatus func(ctx context.Context, testeeID, assessmentID uint64) (*evaluation.AssessmentStatusResponse, error)
	wait      func(ctx context.Context, testeeID, assessmentID uint64, timeout time.Duration) (*evaluation.AssessmentStatusResponse, error)
}

func (f *fakeWaitReportService) NormalizeTimeout(string) time.Duration {
	return 5 * time.Second
}

func (f *fakeWaitReportService) GetStatus(ctx context.Context, testeeID, assessmentID uint64) (*evaluation.AssessmentStatusResponse, error) {
	if f.getStatus == nil {
		panic("unexpected GetStatus call")
	}
	return f.getStatus(ctx, testeeID, assessmentID)
}

func (f *fakeWaitReportService) Wait(ctx context.Context, testeeID, assessmentID uint64, timeout time.Duration) (*evaluation.AssessmentStatusResponse, error) {
	if f.wait == nil {
		panic("unexpected Wait call")
	}
	return f.wait(ctx, testeeID, assessmentID, timeout)
}

func TestEvaluationHandlerGetReportStatusReturnsInterpreted(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewEvaluationHandler(nil, nil, &fakeWaitReportService{
		getStatus: func(context.Context, uint64, uint64) (*evaluation.AssessmentStatusResponse, error) {
			return &evaluation.AssessmentStatusResponse{
				Status:          "completed",
				Stage:           "completed",
				Message:         "报告已生成",
				NextPollAfterMs: 0,
				UpdatedAt:       1,
			}, nil
		},
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/assessments/42/report-status?testee_id=7", nil)
	c.Params = gin.Params{{Key: "id", Value: "42"}}

	handler.GetReportStatus(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	var resp struct {
		Data struct {
			Status          string `json:"status"`
			NextPollAfterMs int    `json:"next_poll_after_ms"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data.Status != "interpreted" {
		t.Fatalf("expected interpreted, got %s", resp.Data.Status)
	}
}

func TestEvaluationHandlerGetReportStatusRequiresTesteeID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewEvaluationHandler(nil, nil, &fakeWaitReportService{})
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/assessments/42/report-status", nil)
	c.Params = gin.Params{{Key: "id", Value: "42"}}
	handler.GetReportStatus(c)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", recorder.Code)
	}
}

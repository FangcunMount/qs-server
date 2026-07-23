package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

	handler := NewEvaluationHandler(nil, &fakeWaitReportService{
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
	handler := NewEvaluationHandler(nil, &fakeWaitReportService{})
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/assessments/42/report-status", nil)
	c.Params = gin.Params{{Key: "id", Value: "42"}}
	handler.GetReportStatus(c)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", recorder.Code)
	}
}

func TestEvaluationHandlerReportAccessErrorContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
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
				waitService := &fakeWaitReportService{
					getStatus: func(context.Context, uint64, uint64) (*evaluation.AssessmentStatusResponse, error) {
						return nil, tt.err
					},
					wait: func(context.Context, uint64, uint64, time.Duration) (*evaluation.AssessmentStatusResponse, error) {
						return nil, tt.err
					},
				}
				handler := NewEvaluationHandler(nil, waitService)
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/assessments/42/report-status?testee_id=7", nil)
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

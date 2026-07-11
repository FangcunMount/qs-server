package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	assessmentapp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	runquery "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runquery"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

type stubRunQueryService struct {
	lastAssessmentID uint64
	lastLimit        int
	lastOrgID        int64
	listResult       *runquery.RunListResult
	latestResult     *runquery.RunResult
	failedResult     *runquery.RetryableFailedListResult
}

func (s *stubRunQueryService) ListByAssessmentID(_ context.Context, assessmentID uint64, limit int) (*runquery.RunListResult, error) {
	s.lastAssessmentID = assessmentID
	s.lastLimit = limit
	return s.listResult, nil
}

func (s *stubRunQueryService) FindLatestByAssessmentID(_ context.Context, assessmentID uint64) (*runquery.RunResult, error) {
	s.lastAssessmentID = assessmentID
	return s.latestResult, nil
}

func (s *stubRunQueryService) ListRetryableFailed(_ context.Context, orgID int64, _ int, _ uint64) (*runquery.RetryableFailedListResult, error) {
	s.lastOrgID = orgID
	return s.failedResult, nil
}

func TestEvaluationHandlerListAssessmentRunsSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	management := &stubAssessmentManagementService{
		getByIDResult: &assessmentapp.AssessmentResult{ID: 301, OrgID: 12, TesteeID: 4001},
	}
	accessQuery := assessmentapp.NewAssessmentAccessQueryService(management, &stubTesteeAccessService{})
	runQuery := &stubRunQueryService{
		listResult: &runquery.RunListResult{
			Items: []*runquery.RunResult{{
				RunID:        "301:1",
				AssessmentID: 301,
				AttemptNo:    1,
				Status:       "succeeded",
				StartedAt:    time.Now(),
			}},
		},
	}
	handler := NewEvaluationHandler(
		management,
		nil,
		assessmentapp.NewProtectedQueryService(management, nil, nil, accessQuery, nil, runQuery),
		nil,
	)

	c, rec := newProtectedHandlerTestContext(http.MethodGet, "/api/v1/evaluations/assessments/301/runs?limit=5")
	c.Params = gin.Params{{Key: "id", Value: "301"}}
	handler.ListAssessmentRuns(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if runQuery.lastAssessmentID != 301 || runQuery.lastLimit != 5 {
		t.Fatalf("run query args assessment=%d limit=%d", runQuery.lastAssessmentID, runQuery.lastLimit)
	}
}

func TestEvaluationRunInternalHandlerListRetryableFailedSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	runQuery := &stubRunQueryService{
		failedResult: &runquery.RetryableFailedListResult{
			Items: []*runquery.RetryableFailedRunResult{{
				RunResult: runquery.RunResult{RunID: "88:1", AssessmentID: 88, AttemptNo: 1, Status: "failed", Retryable: true},
				OrgID:     12,
			}},
		},
	}
	handler := NewEvaluationRunInternalHandler(runQuery)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/internal/v1/evaluation-runs/failed?limit=10", nil)
	c.Set(middleware.OrgIDKey, uint64(12))
	c.Set(middleware.UserIDKey, uint64(34))

	handler.ListRetryableFailed(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if runQuery.lastOrgID != 12 {
		t.Fatalf("org id = %d, want 12", runQuery.lastOrgID)
	}
	var payload struct {
		Data struct {
			Items []struct {
				RunID string `json:"run_id"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Data.Items) != 1 || payload.Data.Items[0].RunID != "88:1" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

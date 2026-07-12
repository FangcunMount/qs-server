package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	evaluationoperator "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/operator"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

func TestEvaluationHandlerListAssessmentRunsSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	query := &operatorQueryStub{runList: &evaluationoperator.RunList{Items: []*evaluationoperator.Run{{RunID: "301:1", AssessmentID: 301, AttemptNo: 1, Status: "succeeded", StartedAt: time.Now()}}}}
	h := NewEvaluationOperatorHandler(nil, nil, query)
	c, rec := protectedContext(http.MethodGet, "/api/v1/evaluations/assessments/301/runs?limit=5")
	c.Params = gin.Params{{Key: "id", Value: "301"}}
	h.ListAssessmentRuns(c)
	if rec.Code != http.StatusOK || query.lastID != 301 || query.lastLimit != 5 {
		t.Fatalf("response/query=%d %#v", rec.Code, query)
	}
}

func TestEvaluationRunInternalHandlerListRetryableFailedSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	query := &operatorQueryStub{failedRuns: &evaluationoperator.RetryableFailedRunList{Items: []*evaluationoperator.RetryableFailedRun{{Run: evaluationoperator.Run{RunID: "88:1", AssessmentID: 88, AttemptNo: 1, Status: "failed", Retryable: true}, OrgID: 12}}}}
	h := NewEvaluationRunInternalHandler(query)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/internal/v1/evaluation-runs/failed?limit=10", nil)
	c.Set(middleware.OrgIDKey, uint64(12))
	c.Set(middleware.UserIDKey, uint64(34))
	h.ListRetryableFailed(c)
	var payload struct {
		Data struct {
			Items []struct {
				RunID string `json:"run_id"`
			} `json:"items"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &payload)
	if rec.Code != http.StatusOK || query.lastActor != (evaluationoperator.Actor{OrgID: 12, OperatorUserID: 34}) || len(payload.Data.Items) != 1 || payload.Data.Items[0].RunID != "88:1" {
		t.Fatalf("response=%d %s", rec.Code, rec.Body.String())
	}
}

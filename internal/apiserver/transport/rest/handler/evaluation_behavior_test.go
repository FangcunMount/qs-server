package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	evaluationoperator "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/operator"
	reportqueryjourney "github.com/FangcunMount/qs-server/internal/apiserver/application/journey/reportquery"
	reportwaitjourney "github.com/FangcunMount/qs-server/internal/apiserver/application/journey/reportwait"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

type operatorQueryStub struct {
	result     *evaluationoperator.Assessment
	err        error
	lastActor  evaluationoperator.Actor
	lastID     uint64
	lastLimit  int
	runList    *evaluationoperator.RunList
	failedRuns *evaluationoperator.RetryableFailedRunList
}

func (s *operatorQueryStub) GetAssessment(_ context.Context, actor evaluationoperator.Actor, id uint64) (*evaluationoperator.Assessment, error) {
	s.lastActor, s.lastID = actor, id
	return s.result, s.err
}
func (*operatorQueryStub) ValidateTesteeAccess(context.Context, evaluationoperator.Actor, uint64) error {
	return nil
}
func (*operatorQueryStub) ScopeTesteeList(context.Context, evaluationoperator.Actor, uint64) (evaluationoperator.TesteeListScope, error) {
	return evaluationoperator.TesteeListScope{}, nil
}
func (*operatorQueryStub) ListAssessments(context.Context, evaluationoperator.Actor, evaluationoperator.ListQuery) (*evaluationoperator.AssessmentList, error) {
	return &evaluationoperator.AssessmentList{}, nil
}
func (*operatorQueryStub) GetAssessmentOutcome(context.Context, evaluationoperator.Actor, uint64) (*evaluationoperator.OutcomeAssessment, error) {
	return nil, nil
}
func (*operatorQueryStub) ListAssessmentsOutcome(context.Context, evaluationoperator.Actor, evaluationoperator.ListQuery) (*evaluationoperator.OutcomeAssessmentList, error) {
	return &evaluationoperator.OutcomeAssessmentList{}, nil
}
func (*operatorQueryStub) GetScores(context.Context, evaluationoperator.Actor, uint64) (*evaluationoperator.Score, error) {
	return nil, nil
}
func (*operatorQueryStub) GetHighRiskFactors(context.Context, evaluationoperator.Actor, uint64) (*evaluationoperator.HighRiskFactors, error) {
	return nil, nil
}
func (*operatorQueryStub) GetFactorTrend(context.Context, evaluationoperator.Actor, evaluationoperator.TrendQuery) (*evaluationoperator.FactorTrend, error) {
	return nil, nil
}
func (s *operatorQueryStub) ListAssessmentRuns(_ context.Context, actor evaluationoperator.Actor, id uint64, limit int) (*evaluationoperator.RunList, error) {
	s.lastActor, s.lastID, s.lastLimit = actor, id, limit
	return s.runList, nil
}
func (*operatorQueryStub) GetLatestAssessmentRun(context.Context, evaluationoperator.Actor, uint64) (*evaluationoperator.Run, error) {
	return nil, nil
}
func (s *operatorQueryStub) ListRetryableFailedRuns(_ context.Context, actor evaluationoperator.Actor, limit int, _ uint64) (*evaluationoperator.RetryableFailedRunList, error) {
	s.lastActor, s.lastLimit = actor, limit
	return s.failedRuns, nil
}

func protectedContext(method, target string) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(method, target, nil)
	c.Set(middleware.OrgIDKey, uint64(12))
	c.Set(middleware.UserIDKey, uint64(34))
	return c, rec
}

func TestEvaluationHandlerGetAssessmentSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	query := &operatorQueryStub{result: &evaluationoperator.Assessment{ID: 301, OrgID: 12, TesteeID: 4001, QuestionnaireCode: "QNR-EVAL", Status: "submitted", AnswerSheetID: 8801}}
	h := NewAssessmentReportJourneyHandler(reportqueryjourney.NewAdministrationService(nil, nil, query), nil)
	c, rec := protectedContext(http.MethodGet, "/api/v1/evaluations/assessments/301")
	c.Params = gin.Params{{Key: "id", Value: "301"}}
	h.GetAssessment(c)
	if rec.Code != http.StatusOK || query.lastActor != (evaluationoperator.Actor{OrgID: 12, OperatorUserID: 34}) || query.lastID != 301 {
		t.Fatalf("response/query=%d %#v", rec.Code, query)
	}
}

func TestEvaluationHandlerWaitReportReturnsTerminalSummaryImmediately(t *testing.T) {
	gin.SetMode(gin.TestMode)
	total, risk := 18.5, "medium"
	query := &operatorQueryStub{result: &evaluationoperator.Assessment{ID: 302, OrgID: 12, TesteeID: 5001, Status: "interpreted", TotalScore: &total, RiskLevel: &risk}}
	h := NewAssessmentReportJourneyHandler(nil, reportwaitjourney.NewService(query, nil))
	c, rec := protectedContext(http.MethodGet, "/api/v1/assessments/302/wait-report?timeout=30")
	c.Params = gin.Params{{Key: "id", Value: "302"}}
	h.WaitReport(c)
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Status     string   `json:"status"`
			TotalScore *float64 `json:"total_score"`
			RiskLevel  *string  `json:"risk_level"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || payload.Data.Status != "interpreted" || payload.Data.TotalScore == nil || *payload.Data.TotalScore != total {
		t.Fatalf("payload=%#v", payload)
	}
}

func TestEvaluationHandlerWaitReportReturnsPendingWhenClientContextCanceled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	query := &operatorQueryStub{result: &evaluationoperator.Assessment{ID: 303, OrgID: 12, TesteeID: 5002, Status: "submitted"}}
	h := NewAssessmentReportJourneyHandler(nil, reportwaitjourney.NewService(query, nil))
	base, cancel := context.WithCancel(context.Background())
	cancel()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/assessments/303/wait-report?timeout=30", nil).WithContext(base)
	c.Params = gin.Params{{Key: "id", Value: "303"}}
	c.Set(middleware.OrgIDKey, uint64(12))
	c.Set(middleware.UserIDKey, uint64(34))
	h.WaitReport(c)
	var payload struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &payload)
	if rec.Code != http.StatusOK || payload.Data.Status != "pending" {
		t.Fatalf("response=%d %s", rec.Code, rec.Body.String())
	}
}

var _ evaluationoperator.QueryService = (*operatorQueryStub)(nil)

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	evaluationoperator "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/operator"
	reportqueryjourney "github.com/FangcunMount/qs-server/internal/apiserver/application/journey/reportquery"
	reportwaitjourney "github.com/FangcunMount/qs-server/internal/apiserver/application/journey/reportwait"
	systemgov "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	"github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
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
	latestRun  *evaluationoperator.Run
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
func (s *operatorQueryStub) GetLatestAssessmentRun(context.Context, evaluationoperator.Actor, uint64) (*evaluationoperator.Run, error) {
	return s.latestRun, s.err
}

type governanceActionRunnerStub struct {
	orgID    int64
	actionID string
	request  systemgov.ActionRunRequest
	calls    int
}

func (s *governanceActionRunnerStub) RunAction(_ context.Context, orgID int64, actionID string, request systemgov.ActionRunRequest) (*systemgov.ActionRunResult, error) {
	s.orgID, s.actionID, s.request = orgID, actionID, request
	s.calls++
	return &systemgov.ActionRunResult{RequestID: request.RequestID, ActionID: actionID, Status: "succeeded", StartedAt: time.Now(), FinishedAt: time.Now()}, nil
}

func TestLegacyEvaluationRetryUsesGovernanceAction(t *testing.T) {
	query := &operatorQueryStub{
		result:    &evaluationoperator.Assessment{ID: 301, OrgID: 12, Status: "failed"},
		latestRun: &evaluationoperator.Run{AssessmentID: 301, AttemptNo: 3, Status: "failed", RetryDisposition: "manual_required"},
	}
	actions := &governanceActionRunnerStub{}
	h := NewEvaluationOperatorHandler(nil, nil, query, actions)
	c, rec := protectedContext(http.MethodPost, "/api/v1/evaluations/assessments/301/retry")
	c.Params = gin.Params{{Key: "id", Value: "301"}}
	c.Request = c.Request.WithContext(pkgmiddleware.WithRequestID(c.Request.Context(), "request-legacy-retry"))
	h.RetryFailed(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("response=%d %s", rec.Code, rec.Body.String())
	}
	if actions.calls != 1 || actions.orgID != 12 || actions.actionID != "evaluation.retry" || !actions.request.Confirm || actions.request.RequestID != "request-legacy-retry" {
		t.Fatalf("governance action=%#v", actions)
	}
	if actions.request.Input["reason"] != "legacy evaluation retry endpoint" || actions.request.Input["expected_attempt"] != 3 {
		t.Fatalf("governance input=%#v", actions.request.Input)
	}
}

func TestLegacyEvaluationRetryRejectsNonManualLatestRun(t *testing.T) {
	query := &operatorQueryStub{latestRun: &evaluationoperator.Run{AssessmentID: 301, AttemptNo: 2, Status: "failed", RetryDisposition: "automatic"}}
	actions := &governanceActionRunnerStub{}
	h := NewEvaluationOperatorHandler(nil, nil, query, actions)
	c, rec := protectedContext(http.MethodPost, "/api/v1/evaluations/assessments/301/retry")
	c.Params = gin.Params{{Key: "id", Value: "301"}}
	h.RetryFailed(c)
	if rec.Code != http.StatusConflict || actions.calls != 0 {
		t.Fatalf("response=%d actions=%d body=%s", rec.Code, actions.calls, rec.Body.String())
	}
}

func TestLegacyEvaluationRetryReplaysAuditedRequestAfterDispositionChanged(t *testing.T) {
	query := &operatorQueryStub{
		result:    &evaluationoperator.Assessment{ID: 301, OrgID: 12, Status: "failed"},
		latestRun: &evaluationoperator.Run{AssessmentID: 301, AttemptNo: 3, Status: "failed", RetryDisposition: "automatic", ActionRequestID: "request-legacy-retry"},
	}
	actions := &governanceActionRunnerStub{}
	h := NewEvaluationOperatorHandler(nil, nil, query, actions)
	c, rec := protectedContext(http.MethodPost, "/api/v1/evaluations/assessments/301/retry")
	c.Params = gin.Params{{Key: "id", Value: "301"}}
	c.Request = c.Request.WithContext(pkgmiddleware.WithRequestID(c.Request.Context(), "request-legacy-retry"))
	h.RetryFailed(c)
	if rec.Code != http.StatusOK || actions.calls != 1 || actions.request.RequestID != "request-legacy-retry" {
		t.Fatalf("response=%d actions=%#v body=%s", rec.Code, actions, rec.Body.String())
	}
}
func (s *operatorQueryStub) ListRetryableFailedRuns(_ context.Context, actor evaluationoperator.Actor, limit int, _ uint64) (*evaluationoperator.RetryableFailedRunList, error) {
	s.lastActor, s.lastLimit = actor, limit
	return s.failedRuns, nil
}

type assessmentProjectionStub struct{}

func (*assessmentProjectionStub) ProjectAssessment(_ context.Context, result *evaluationoperator.Assessment) (*reportqueryjourney.AssessmentProjection, error) {
	return &reportqueryjourney.AssessmentProjection{Assessment: result, Status: "interpreted"}, nil
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
	h := NewAssessmentReportJourneyHandler(nil, reportwaitjourney.NewService(query, &assessmentProjectionStub{}))
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

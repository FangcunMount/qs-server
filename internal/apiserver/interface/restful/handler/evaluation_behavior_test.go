package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	actoraccess "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	assessmentapp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/middleware"
	"github.com/gin-gonic/gin"
)

type stubAssessmentManagementService struct {
	lastGetByID  uint64
	lastRetryID  uint64
	lastRetryOrg int64
	lastListDTO  assessmentapp.ListAssessmentsDTO

	getByIDResult *assessmentapp.AssessmentResult
	getByIDErr    error
	listResult    *assessmentapp.AssessmentListResult
	listErr       error
	retryResult   *assessmentapp.AssessmentResult
	retryErr      error
}

func (s *stubAssessmentManagementService) GetByID(_ context.Context, id uint64) (*assessmentapp.AssessmentResult, error) {
	s.lastGetByID = id
	return s.getByIDResult, s.getByIDErr
}

func (s *stubAssessmentManagementService) List(_ context.Context, dto assessmentapp.ListAssessmentsDTO) (*assessmentapp.AssessmentListResult, error) {
	s.lastListDTO = dto
	return s.listResult, s.listErr
}

func (s *stubAssessmentManagementService) Retry(_ context.Context, orgID int64, assessmentID uint64) (*assessmentapp.AssessmentResult, error) {
	s.lastRetryOrg = orgID
	s.lastRetryID = assessmentID
	return s.retryResult, s.retryErr
}

type stubTesteeAccessService struct {
	lastValidateOrgID    int64
	lastValidateUserID   int64
	lastValidateTesteeID uint64

	validateErr       error
	resolveScopeValue *actoraccess.TesteeAccessScope
	resolveScopeErr   error
	accessibleIDs     []uint64
	accessibleIDsErr  error
}

func (s *stubTesteeAccessService) ResolveAccessScope(context.Context, int64, int64) (*actoraccess.TesteeAccessScope, error) {
	if s.resolveScopeValue != nil || s.resolveScopeErr != nil {
		return s.resolveScopeValue, s.resolveScopeErr
	}
	return &actoraccess.TesteeAccessScope{IsAdmin: true}, nil
}

func (s *stubTesteeAccessService) ValidateTesteeAccess(_ context.Context, orgID int64, operatorUserID int64, testeeID uint64) error {
	s.lastValidateOrgID = orgID
	s.lastValidateUserID = operatorUserID
	s.lastValidateTesteeID = testeeID
	return s.validateErr
}

func (s *stubTesteeAccessService) ListAccessibleTesteeIDs(context.Context, int64, int64) ([]uint64, error) {
	return s.accessibleIDs, s.accessibleIDsErr
}

func newProtectedHandlerTestContext(method, target string) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(method, target, nil)
	c.Set(middleware.OrgIDKey, uint64(12))
	c.Set(middleware.UserIDKey, uint64(34))
	return c, rec
}

func TestEvaluationHandlerGetAssessmentSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	management := &stubAssessmentManagementService{
		getByIDResult: &assessmentapp.AssessmentResult{
			ID:                301,
			OrgID:             12,
			TesteeID:          4001,
			QuestionnaireCode: "QNR-EVAL",
			Status:            "submitted",
			AnswerSheetID:     8801,
		},
	}
	access := &stubTesteeAccessService{}
	handler := NewEvaluationHandler(management, nil, nil, nil)
	handler.SetTesteeAccessService(access)

	c, rec := newProtectedHandlerTestContext(http.MethodGet, "/api/v1/evaluations/assessments/301")
	c.Params = gin.Params{{Key: "id", Value: "301"}}

	handler.GetAssessment(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if management.lastGetByID != 301 {
		t.Fatalf("lastGetByID = %d, want 301", management.lastGetByID)
	}
	if access.lastValidateOrgID != 12 || access.lastValidateUserID != 34 || access.lastValidateTesteeID != 4001 {
		t.Fatalf("unexpected access validation call: %+v", access)
	}

	var payload struct {
		Code int `json:"code"`
		Data struct {
			ID                string `json:"id"`
			OrgID             string `json:"org_id"`
			TesteeID          string `json:"testee_id"`
			QuestionnaireCode string `json:"questionnaire_code"`
			Status            string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Code != 0 {
		t.Fatalf("code = %d, want 0", payload.Code)
	}
	if payload.Data.ID != "301" || payload.Data.OrgID != "12" || payload.Data.TesteeID != "4001" {
		t.Fatalf("unexpected response data: %+v", payload.Data)
	}
}

func TestEvaluationHandlerWaitReportReturnsTerminalSummaryImmediately(t *testing.T) {
	gin.SetMode(gin.TestMode)

	totalScore := 18.5
	riskLevel := "medium"
	management := &stubAssessmentManagementService{
		getByIDResult: &assessmentapp.AssessmentResult{
			ID:         302,
			OrgID:      12,
			TesteeID:   5001,
			Status:     "interpreted",
			TotalScore: &totalScore,
			RiskLevel:  &riskLevel,
		},
	}
	handler := NewEvaluationHandler(management, nil, nil, nil)
	handler.SetTesteeAccessService(&stubTesteeAccessService{})

	c, rec := newProtectedHandlerTestContext(http.MethodGet, "/api/v1/assessments/302/wait-report?timeout=30")
	c.Params = gin.Params{{Key: "id", Value: "302"}}

	handler.WaitReport(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var payload struct {
		Code int `json:"code"`
		Data struct {
			Status     string   `json:"status"`
			TotalScore *float64 `json:"total_score"`
			RiskLevel  *string  `json:"risk_level"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Code != 0 || payload.Data.Status != "interpreted" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.Data.TotalScore == nil || *payload.Data.TotalScore != totalScore {
		t.Fatalf("total_score = %v, want %v", payload.Data.TotalScore, totalScore)
	}
	if payload.Data.RiskLevel == nil || *payload.Data.RiskLevel != riskLevel {
		t.Fatalf("risk_level = %v, want %v", payload.Data.RiskLevel, riskLevel)
	}
}

func TestEvaluationHandlerWaitReportReturnsPendingWhenClientContextCanceled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	management := &stubAssessmentManagementService{
		getByIDResult: &assessmentapp.AssessmentResult{
			ID:       303,
			OrgID:    12,
			TesteeID: 5002,
			Status:   "submitted",
		},
	}
	handler := NewEvaluationHandler(management, nil, nil, nil)
	handler.SetTesteeAccessService(&stubTesteeAccessService{})

	baseCtx, cancel := context.WithCancel(context.Background())
	cancel()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/assessments/303/wait-report?timeout=30", nil).WithContext(baseCtx)
	c.Params = gin.Params{{Key: "id", Value: "303"}}
	c.Set(middleware.OrgIDKey, uint64(12))
	c.Set(middleware.UserIDKey, uint64(34))

	handler.WaitReport(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var payload struct {
		Code int `json:"code"`
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Code != 0 || payload.Data.Status != "pending" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

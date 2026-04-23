package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	actorAccessApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/access"
	assessmentEntryApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/assessmententry"
	clinicianApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/clinician"
	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/middleware"
	"github.com/gin-gonic/gin"
)

type stubActorTesteeQueryService struct {
	findByProfileResult *testeeApp.TesteeResult
	findByProfileErr    error
	lastFindOrgID       int64
	lastFindProfileID   uint64
}

func (*stubActorTesteeQueryService) GetByID(context.Context, uint64) (*testeeApp.TesteeResult, error) {
	return nil, nil
}

func (s *stubActorTesteeQueryService) FindByProfile(_ context.Context, orgID int64, profileID uint64) (*testeeApp.TesteeResult, error) {
	s.lastFindOrgID = orgID
	s.lastFindProfileID = profileID
	return s.findByProfileResult, s.findByProfileErr
}

func (*stubActorTesteeQueryService) ListTestees(context.Context, testeeApp.ListTesteeDTO) (*testeeApp.TesteeListResult, error) {
	return nil, nil
}

func (*stubActorTesteeQueryService) ListKeyFocus(context.Context, int64, int, int) (*testeeApp.TesteeListResult, error) {
	return nil, nil
}

func (*stubActorTesteeQueryService) ListByProfileIDs(context.Context, []uint64, int, int) (*testeeApp.TesteeListResult, error) {
	return nil, nil
}

type stubActorTesteeBackendQueryService struct {
	getByIDResult *testeeApp.TesteeBackendResult
	getByIDErr    error
	lastTesteeID  uint64
}

func (s *stubActorTesteeBackendQueryService) GetByIDWithGuardians(_ context.Context, testeeID uint64) (*testeeApp.TesteeBackendResult, error) {
	s.lastTesteeID = testeeID
	return s.getByIDResult, s.getByIDErr
}

func (*stubActorTesteeBackendQueryService) ListTesteesWithGuardians(context.Context, testeeApp.ListTesteeDTO) (*testeeApp.TesteeBackendListResult, error) {
	return nil, nil
}

type stubActorTesteeAccessService struct {
	lastValidateOrgID    int64
	lastValidateUserID   int64
	lastValidateTesteeID uint64
	validateErr          error
}

func (*stubActorTesteeAccessService) ResolveAccessScope(context.Context, int64, int64) (*actorAccessApp.TesteeAccessScope, error) {
	return &actorAccessApp.TesteeAccessScope{IsAdmin: true}, nil
}

func (s *stubActorTesteeAccessService) ValidateTesteeAccess(_ context.Context, orgID int64, operatorUserID int64, testeeID uint64) error {
	s.lastValidateOrgID = orgID
	s.lastValidateUserID = operatorUserID
	s.lastValidateTesteeID = testeeID
	return s.validateErr
}

func (*stubActorTesteeAccessService) ListAccessibleTesteeIDs(context.Context, int64, int64) ([]uint64, error) {
	return nil, nil
}

type stubActorClinicianQueryService struct {
	listResult     *clinicianApp.ClinicianListResult
	listErr        error
	lastListDTO    clinicianApp.ListClinicianDTO
	lastGetByID    uint64
	lastGetByOpOrg int64
	lastGetByOpID  uint64
}

func (s *stubActorClinicianQueryService) GetByID(_ context.Context, clinicianID uint64) (*clinicianApp.ClinicianResult, error) {
	s.lastGetByID = clinicianID
	return nil, nil
}

func (s *stubActorClinicianQueryService) GetByOperator(_ context.Context, orgID int64, operatorID uint64) (*clinicianApp.ClinicianResult, error) {
	s.lastGetByOpOrg = orgID
	s.lastGetByOpID = operatorID
	return nil, nil
}

func (s *stubActorClinicianQueryService) ListClinicians(_ context.Context, dto clinicianApp.ListClinicianDTO) (*clinicianApp.ClinicianListResult, error) {
	s.lastListDTO = dto
	return s.listResult, s.listErr
}

type stubActorAssessmentEntryService struct {
	resolveResult *assessmentEntryApp.ResolvedAssessmentEntryResult
	resolveErr    error
	intakeResult  *assessmentEntryApp.AssessmentEntryIntakeResult
	intakeErr     error
	lastToken     string
	lastIntakeDTO assessmentEntryApp.IntakeByAssessmentEntryDTO
}

func (*stubActorAssessmentEntryService) Create(context.Context, assessmentEntryApp.CreateAssessmentEntryDTO) (*assessmentEntryApp.AssessmentEntryResult, error) {
	return nil, nil
}

func (*stubActorAssessmentEntryService) GetByID(context.Context, uint64) (*assessmentEntryApp.AssessmentEntryResult, error) {
	return nil, nil
}

func (*stubActorAssessmentEntryService) Deactivate(context.Context, uint64) (*assessmentEntryApp.AssessmentEntryResult, error) {
	return nil, nil
}

func (*stubActorAssessmentEntryService) Reactivate(context.Context, uint64) (*assessmentEntryApp.AssessmentEntryResult, error) {
	return nil, nil
}

func (*stubActorAssessmentEntryService) ListByClinician(context.Context, assessmentEntryApp.ListAssessmentEntryDTO) (*assessmentEntryApp.AssessmentEntryListResult, error) {
	return nil, nil
}

func (s *stubActorAssessmentEntryService) Resolve(_ context.Context, token string) (*assessmentEntryApp.ResolvedAssessmentEntryResult, error) {
	s.lastToken = token
	return s.resolveResult, s.resolveErr
}

func (s *stubActorAssessmentEntryService) Intake(_ context.Context, token string, dto assessmentEntryApp.IntakeByAssessmentEntryDTO) (*assessmentEntryApp.AssessmentEntryIntakeResult, error) {
	s.lastToken = token
	s.lastIntakeDTO = dto
	return s.intakeResult, s.intakeErr
}

func newActorHandlerForTest() *ActorHandler {
	return &ActorHandler{BaseHandler: NewBaseHandler()}
}

func newActorTestContext(method, target string, body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.Request = req
	return c, rec
}

func TestActorHandlerGetTesteeUsesProtectedScopeAndBackendQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	backend := &stubActorTesteeBackendQueryService{
		getByIDResult: &testeeApp.TesteeBackendResult{
			TesteeResult: &testeeApp.TesteeResult{ID: 11, OrgID: 88, Name: "Alice"},
		},
	}
	access := &stubActorTesteeAccessService{}
	handler := newActorHandlerForTest()
	handler.testeeBackendQueryService = backend
	handler.testeeAccessService = access

	c, rec := newActorTestContext(http.MethodGet, "/api/v1/testees/11", nil)
	c.Params = gin.Params{{Key: "id", Value: "11"}}
	c.Set(restmiddleware.OrgIDKey, uint64(88))
	c.Set(restmiddleware.UserIDKey, uint64(701))

	handler.GetTestee(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if access.lastValidateOrgID != 88 || access.lastValidateUserID != 701 || access.lastValidateTesteeID != 11 {
		t.Fatalf("unexpected access validation: org=%d user=%d testee=%d", access.lastValidateOrgID, access.lastValidateUserID, access.lastValidateTesteeID)
	}
	if backend.lastTesteeID != 11 {
		t.Fatalf("backend testeeID = %d, want 11", backend.lastTesteeID)
	}

	var payload struct {
		Code int `json:"code"`
		Data struct {
			ID    string `json:"id"`
			OrgID string `json:"org_id"`
			Name  string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Code != 0 || payload.Data.ID != "11" || payload.Data.OrgID != "88" || payload.Data.Name != "Alice" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestActorHandlerGetTesteeByProfileIDUsesProtectedScopeAndAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	query := &stubActorTesteeQueryService{
		findByProfileResult: &testeeApp.TesteeResult{ID: 22, OrgID: 66, Name: "Bob"},
	}
	backend := &stubActorTesteeBackendQueryService{
		getByIDResult: &testeeApp.TesteeBackendResult{
			TesteeResult: &testeeApp.TesteeResult{ID: 22, OrgID: 66, Name: "Bob"},
		},
	}
	access := &stubActorTesteeAccessService{}
	handler := newActorHandlerForTest()
	handler.testeeQueryService = query
	handler.testeeBackendQueryService = backend
	handler.testeeAccessService = access

	c, rec := newActorTestContext(http.MethodGet, "/api/v1/testees/by-profile-id?profile_id=123&org_id=66", nil)
	c.Set(restmiddleware.OrgIDKey, uint64(66))
	c.Set(restmiddleware.UserIDKey, uint64(909))

	handler.GetTesteeByProfileID(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if query.lastFindOrgID != 66 || query.lastFindProfileID != 123 {
		t.Fatalf("unexpected query lookup: org=%d profile=%d", query.lastFindOrgID, query.lastFindProfileID)
	}
	if access.lastValidateOrgID != 66 || access.lastValidateUserID != 909 || access.lastValidateTesteeID != 22 {
		t.Fatalf("unexpected access validation: org=%d user=%d testee=%d", access.lastValidateOrgID, access.lastValidateUserID, access.lastValidateTesteeID)
	}
	if backend.lastTesteeID != 22 {
		t.Fatalf("backend testeeID = %d, want 22", backend.lastTesteeID)
	}
}

func TestActorHandlerListCliniciansDefaultsPaginationAndUsesProtectedOrgScope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	query := &stubActorClinicianQueryService{
		listResult: &clinicianApp.ClinicianListResult{
			Items: []*clinicianApp.ClinicianResult{{ID: 3, OrgID: 91, Name: "Dr. Chen", IsActive: true}},
		},
	}
	handler := newActorHandlerForTest()
	handler.clinicianQueryService = query

	c, rec := newActorTestContext(http.MethodGet, "/api/v1/clinicians?org_id=91", nil)
	c.Set(restmiddleware.OrgIDKey, uint64(91))

	handler.ListClinicians(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if query.lastListDTO.OrgID != 91 || query.lastListDTO.Offset != 0 || query.lastListDTO.Limit != 20 {
		t.Fatalf("unexpected dto: %+v", query.lastListDTO)
	}
}

func TestActorHandlerResolveAssessmentEntryReturnsResolvedPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	entryService := &stubActorAssessmentEntryService{
		resolveResult: &assessmentEntryApp.ResolvedAssessmentEntryResult{
			Entry: &assessmentEntryApp.AssessmentEntryResult{ID: 7, OrgID: 88, ClinicianID: 9, Token: "ae_token", TargetType: "questionnaire", TargetCode: "QNR-1", TargetVersion: "1.0.0", IsActive: true},
			Clinician: &assessmentEntryApp.ClinicianSummaryResult{ID: 9, Name: "Dr. Lin"},
		},
	}
	handler := newActorHandlerForTest()
	handler.assessmentEntryService = entryService

	c, rec := newActorTestContext(http.MethodGet, "/api/v1/public/assessment-entries/ae_token", nil)
	c.Params = gin.Params{{Key: "token", Value: "ae_token"}}

	handler.ResolveAssessmentEntry(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if entryService.lastToken != "ae_token" {
		t.Fatalf("token = %q, want ae_token", entryService.lastToken)
	}
}

func TestActorHandlerIntakeAssessmentEntryRejectsInvalidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := newActorHandlerForTest()
	handler.assessmentEntryService = &stubActorAssessmentEntryService{}

	c, rec := newActorTestContext(http.MethodPost, "/api/v1/public/assessment-entries/ae_token/intake", []byte(`{"name":`))
	c.Params = gin.Params{{Key: "token", Value: "ae_token"}}

	handler.IntakeAssessmentEntry(c)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

var _ testeeApp.TesteeQueryService = (*stubActorTesteeQueryService)(nil)
var _ testeeApp.TesteeBackendQueryService = (*stubActorTesteeBackendQueryService)(nil)
var _ actorAccessApp.TesteeAccessService = (*stubActorTesteeAccessService)(nil)
var _ clinicianApp.ClinicianQueryService = (*stubActorClinicianQueryService)(nil)
var _ assessmentEntryApp.AssessmentEntryService = (*stubActorAssessmentEntryService)(nil)

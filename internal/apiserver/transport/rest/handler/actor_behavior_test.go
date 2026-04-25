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
	operatorApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/operator"
	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	restmiddleware "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/middleware"
	"github.com/gin-gonic/gin"
)

type stubActorTesteeQueryService struct {
	findByProfileResult *testeeApp.TesteeResult
	findByProfileErr    error
	lastFindOrgID       int64
	lastFindProfileID   uint64
	listResult          *testeeApp.TesteeListResult
	listErr             error
	lastListDTO         testeeApp.ListTesteeDTO
}

func (*stubActorTesteeQueryService) GetByID(context.Context, uint64) (*testeeApp.TesteeResult, error) {
	return nil, nil
}

func (s *stubActorTesteeQueryService) FindByProfile(_ context.Context, orgID int64, profileID uint64) (*testeeApp.TesteeResult, error) {
	s.lastFindOrgID = orgID
	s.lastFindProfileID = profileID
	return s.findByProfileResult, s.findByProfileErr
}

func (s *stubActorTesteeQueryService) ListTestees(_ context.Context, dto testeeApp.ListTesteeDTO) (*testeeApp.TesteeListResult, error) {
	s.lastListDTO = dto
	return s.listResult, s.listErr
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
	lastResolveOrgID      int64
	lastResolveUserID     int64
	lastValidateOrgID     int64
	lastValidateUserID    int64
	lastValidateTesteeID  uint64
	validateErr           error
	accessibleTesteeIDs   []uint64
	accessibleTesteesErr  error
	resolveScope          *actorAccessApp.TesteeAccessScope
	resolveAccessScopeErr error
}

func (s *stubActorTesteeAccessService) ResolveAccessScope(_ context.Context, orgID int64, operatorUserID int64) (*actorAccessApp.TesteeAccessScope, error) {
	s.lastResolveOrgID = orgID
	s.lastResolveUserID = operatorUserID
	if s.resolveScope != nil || s.resolveAccessScopeErr != nil {
		return s.resolveScope, s.resolveAccessScopeErr
	}
	return &actorAccessApp.TesteeAccessScope{IsAdmin: true}, nil
}

func (s *stubActorTesteeAccessService) ValidateTesteeAccess(_ context.Context, orgID int64, operatorUserID int64, testeeID uint64) error {
	s.lastValidateOrgID = orgID
	s.lastValidateUserID = operatorUserID
	s.lastValidateTesteeID = testeeID
	return s.validateErr
}

func (s *stubActorTesteeAccessService) ListAccessibleTesteeIDs(context.Context, int64, int64) ([]uint64, error) {
	return s.accessibleTesteeIDs, s.accessibleTesteesErr
}

type stubActorClinicianQueryService struct {
	getByIDResult    *clinicianApp.ClinicianResult
	getByIDErr       error
	listResult       *clinicianApp.ClinicianListResult
	listErr          error
	lastListDTO      clinicianApp.ListClinicianDTO
	lastGetByID      uint64
	lastGetByOpOrg   int64
	lastGetByOpID    uint64
	getByOperator    *clinicianApp.ClinicianResult
	getByOperatorErr error
}

func (s *stubActorClinicianQueryService) GetByID(_ context.Context, clinicianID uint64) (*clinicianApp.ClinicianResult, error) {
	s.lastGetByID = clinicianID
	if s.getByIDResult != nil || s.getByIDErr != nil {
		return s.getByIDResult, s.getByIDErr
	}
	return nil, nil
}

func (s *stubActorClinicianQueryService) GetByOperator(_ context.Context, orgID int64, operatorID uint64) (*clinicianApp.ClinicianResult, error) {
	s.lastGetByOpOrg = orgID
	s.lastGetByOpID = operatorID
	return s.getByOperator, s.getByOperatorErr
}

func (s *stubActorClinicianQueryService) ListClinicians(_ context.Context, dto clinicianApp.ListClinicianDTO) (*clinicianApp.ClinicianListResult, error) {
	s.lastListDTO = dto
	return s.listResult, s.listErr
}

type stubActorOperatorQueryService struct {
	listResult  *operatorApp.OperatorListResult
	listErr     error
	lastListDTO operatorApp.ListOperatorDTO
}

func (*stubActorOperatorQueryService) GetByID(context.Context, uint64) (*operatorApp.OperatorResult, error) {
	return nil, nil
}

func (*stubActorOperatorQueryService) GetByUser(context.Context, int64, int64) (*operatorApp.OperatorResult, error) {
	return nil, nil
}

func (s *stubActorOperatorQueryService) ListOperators(_ context.Context, dto operatorApp.ListOperatorDTO) (*operatorApp.OperatorListResult, error) {
	s.lastListDTO = dto
	return s.listResult, s.listErr
}

type stubActorClinicianRelationshipService struct {
	testeeRelationsResult *clinicianApp.TesteeRelationListResult
	testeeRelationsErr    error
	lastTesteeRelationDTO clinicianApp.ListTesteeRelationDTO
}

func (*stubActorClinicianRelationshipService) AssignTestee(context.Context, clinicianApp.AssignTesteeDTO) (*clinicianApp.RelationResult, error) {
	return nil, nil
}

func (*stubActorClinicianRelationshipService) AssignPrimary(context.Context, clinicianApp.AssignTesteeDTO) (*clinicianApp.RelationResult, error) {
	return nil, nil
}

func (*stubActorClinicianRelationshipService) AssignAttending(context.Context, clinicianApp.AssignTesteeDTO) (*clinicianApp.RelationResult, error) {
	return nil, nil
}

func (*stubActorClinicianRelationshipService) AssignCollaborator(context.Context, clinicianApp.AssignTesteeDTO) (*clinicianApp.RelationResult, error) {
	return nil, nil
}

func (*stubActorClinicianRelationshipService) TransferPrimary(context.Context, clinicianApp.TransferPrimaryDTO) (*clinicianApp.RelationResult, error) {
	return nil, nil
}

func (*stubActorClinicianRelationshipService) UnbindRelation(context.Context, uint64) (*clinicianApp.RelationResult, error) {
	return nil, nil
}

func (*stubActorClinicianRelationshipService) ListAssignedTestees(context.Context, clinicianApp.ListAssignedTesteeDTO) (*clinicianApp.AssignedTesteeListResult, error) {
	return nil, nil
}

func (*stubActorClinicianRelationshipService) ListAssignedTesteeIDs(context.Context, int64, uint64) ([]uint64, error) {
	return nil, nil
}

func (s *stubActorClinicianRelationshipService) ListTesteeRelations(_ context.Context, dto clinicianApp.ListTesteeRelationDTO) (*clinicianApp.TesteeRelationListResult, error) {
	s.lastTesteeRelationDTO = dto
	return s.testeeRelationsResult, s.testeeRelationsErr
}

func (*stubActorClinicianRelationshipService) ListClinicianRelations(context.Context, clinicianApp.ListClinicianRelationDTO) (*clinicianApp.ClinicianRelationListResult, error) {
	return nil, nil
}

type stubActorAssessmentEntryService struct {
	resolveResult *assessmentEntryApp.ResolvedAssessmentEntryResult
	resolveErr    error
	intakeResult  *assessmentEntryApp.AssessmentEntryIntakeResult
	intakeErr     error
	listResult    *assessmentEntryApp.AssessmentEntryListResult
	listErr       error
	lastToken     string
	lastIntakeDTO assessmentEntryApp.IntakeByAssessmentEntryDTO
	lastListDTO   assessmentEntryApp.ListAssessmentEntryDTO
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

func (s *stubActorAssessmentEntryService) ListByClinician(_ context.Context, dto assessmentEntryApp.ListAssessmentEntryDTO) (*assessmentEntryApp.AssessmentEntryListResult, error) {
	s.lastListDTO = dto
	return s.listResult, s.listErr
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

func newTesteeHandlerForTest() *TesteeHandler {
	return &TesteeHandler{BaseHandler: NewBaseHandler()}
}

func newOperatorClinicianHandlerForTest() *OperatorClinicianHandler {
	return &OperatorClinicianHandler{BaseHandler: NewBaseHandler()}
}

func newAssessmentEntryHandlerForTest() *AssessmentEntryHandler {
	return &AssessmentEntryHandler{BaseHandler: NewBaseHandler()}
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

func TestTesteeHandlerGetTesteeUsesProtectedScopeAndBackendQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	backend := &stubActorTesteeBackendQueryService{
		getByIDResult: &testeeApp.TesteeBackendResult{
			TesteeResult: &testeeApp.TesteeResult{ID: 11, OrgID: 88, Name: "Alice"},
		},
	}
	access := &stubActorTesteeAccessService{}
	handler := newTesteeHandlerForTest()
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

func TestTesteeHandlerGetTesteeByProfileIDUsesProtectedScopeAndAccess(t *testing.T) {
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
	handler := newTesteeHandlerForTest()
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

func TestTesteeHandlerListTesteesDefaultsPaginationAndUsesProtectedScope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	query := &stubActorTesteeQueryService{
		listResult: &testeeApp.TesteeListResult{
			Items: []*testeeApp.TesteeResult{{ID: 5, OrgID: 91, Name: "Casey"}},
		},
	}
	access := &stubActorTesteeAccessService{}
	handler := newTesteeHandlerForTest()
	handler.testeeQueryService = query
	handler.testeeAccessService = access

	c, rec := newActorTestContext(http.MethodGet, "/api/v1/testees?org_id=91", nil)
	c.Set(restmiddleware.OrgIDKey, uint64(91))
	c.Set(restmiddleware.UserIDKey, uint64(702))

	handler.ListTestees(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if query.lastListDTO.OrgID != 91 || query.lastListDTO.Offset != 0 || query.lastListDTO.Limit != 20 {
		t.Fatalf("unexpected testee list dto: %+v", query.lastListDTO)
	}
	if access.lastResolveOrgID != 91 || access.lastResolveUserID != 702 {
		t.Fatalf("unexpected access scope lookup: org=%d user=%d", access.lastResolveOrgID, access.lastResolveUserID)
	}
}

func TestOperatorClinicianHandlerListStaffDefaultsPaginationAndUsesProtectedOrgScope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	query := &stubActorOperatorQueryService{
		listResult: &operatorApp.OperatorListResult{
			Items: []*operatorApp.OperatorResult{{ID: 3, OrgID: 91, Name: "Nurse Lee", IsActive: true}},
		},
	}
	handler := newOperatorClinicianHandlerForTest()
	handler.operatorQueryService = query

	c, rec := newActorTestContext(http.MethodGet, "/api/v1/staff?org_id=91", nil)
	c.Set(restmiddleware.OrgIDKey, uint64(91))

	handler.ListStaff(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if query.lastListDTO.OrgID != 91 || query.lastListDTO.Offset != 0 || query.lastListDTO.Limit != 20 {
		t.Fatalf("unexpected staff dto: %+v", query.lastListDTO)
	}
}

func TestOperatorClinicianHandlerListCliniciansDefaultsPaginationAndUsesProtectedOrgScope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	query := &stubActorClinicianQueryService{
		listResult: &clinicianApp.ClinicianListResult{
			Items: []*clinicianApp.ClinicianResult{{ID: 3, OrgID: 91, Name: "Dr. Chen", IsActive: true}},
		},
	}
	handler := newOperatorClinicianHandlerForTest()
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

func TestOperatorClinicianHandlerListTesteeClinicianRelationsUsesProtectedScope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	relations := &stubActorClinicianRelationshipService{
		testeeRelationsResult: &clinicianApp.TesteeRelationListResult{
			Items: []*clinicianApp.TesteeRelationResult{{
				Relation:  &clinicianApp.RelationResult{ID: 8, OrgID: 91, ClinicianID: 12, TesteeID: 11, RelationType: "primary", IsActive: true},
				Clinician: &clinicianApp.ClinicianResult{ID: 12, OrgID: 91, Name: "Dr. Ren", IsActive: true},
			}},
		},
	}
	access := &stubActorTesteeAccessService{}
	handler := newOperatorClinicianHandlerForTest()
	handler.clinicianRelationshipService = relations
	handler.testeeAccessService = access

	c, rec := newActorTestContext(http.MethodGet, "/api/v1/testees/11/clinician-relations", nil)
	c.Params = gin.Params{{Key: "id", Value: "11"}}
	c.Set(restmiddleware.OrgIDKey, uint64(91))
	c.Set(restmiddleware.UserIDKey, uint64(703))

	handler.ListTesteeClinicianRelations(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if access.lastValidateOrgID != 91 || access.lastValidateUserID != 703 || access.lastValidateTesteeID != 11 {
		t.Fatalf("unexpected access validation: org=%d user=%d testee=%d", access.lastValidateOrgID, access.lastValidateUserID, access.lastValidateTesteeID)
	}
	if relations.lastTesteeRelationDTO.OrgID != 91 || relations.lastTesteeRelationDTO.TesteeID != 11 || relations.lastTesteeRelationDTO.ActiveOnly {
		t.Fatalf("unexpected relation dto: %+v", relations.lastTesteeRelationDTO)
	}
}

func TestAssessmentEntryHandlerResolveAssessmentEntryReturnsResolvedPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)

	entryService := &stubActorAssessmentEntryService{
		resolveResult: &assessmentEntryApp.ResolvedAssessmentEntryResult{
			Entry:     &assessmentEntryApp.AssessmentEntryResult{ID: 7, OrgID: 88, ClinicianID: 9, Token: "ae_token", TargetType: "questionnaire", TargetCode: "QNR-1", TargetVersion: "1.0.0", IsActive: true},
			Clinician: &assessmentEntryApp.ClinicianSummaryResult{ID: 9, Name: "Dr. Lin"},
		},
	}
	handler := newAssessmentEntryHandlerForTest()
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

func TestAssessmentEntryHandlerIntakeAssessmentEntryRejectsInvalidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := newAssessmentEntryHandlerForTest()
	handler.assessmentEntryService = &stubActorAssessmentEntryService{}

	c, rec := newActorTestContext(http.MethodPost, "/api/v1/public/assessment-entries/ae_token/intake", []byte(`{"name":`))
	c.Params = gin.Params{{Key: "token", Value: "ae_token"}}

	handler.IntakeAssessmentEntry(c)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestAssessmentEntryHandlerListClinicianAssessmentEntriesDefaultsPaginationAndOrgScope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	query := &stubActorClinicianQueryService{
		getByIDResult: &clinicianApp.ClinicianResult{ID: 12, OrgID: 91, Name: "Dr. Fang", IsActive: true},
	}
	entryService := &stubActorAssessmentEntryService{
		listResult: &assessmentEntryApp.AssessmentEntryListResult{
			Items: []*assessmentEntryApp.AssessmentEntryResult{{ID: 7, OrgID: 91, ClinicianID: 12, Token: "tok-1", TargetType: "questionnaire", TargetCode: "Q1", TargetVersion: "1.0.0", IsActive: true}},
		},
	}
	handler := newAssessmentEntryHandlerForTest()
	handler.clinicianQueryService = query
	handler.assessmentEntryService = entryService

	c, rec := newActorTestContext(http.MethodGet, "/api/v1/clinicians/12/assessment-entries", nil)
	c.Params = gin.Params{{Key: "id", Value: "12"}}
	c.Set(restmiddleware.OrgIDKey, uint64(91))

	handler.ListClinicianAssessmentEntries(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if query.lastGetByID != 12 {
		t.Fatalf("clinician id = %d, want 12", query.lastGetByID)
	}
	if entryService.lastListDTO.OrgID != 91 || entryService.lastListDTO.ClinicianID != 12 || entryService.lastListDTO.Offset != 0 || entryService.lastListDTO.Limit != 20 {
		t.Fatalf("unexpected assessment-entry dto: %+v", entryService.lastListDTO)
	}
}

var _ testeeApp.TesteeQueryService = (*stubActorTesteeQueryService)(nil)
var _ testeeApp.TesteeBackendQueryService = (*stubActorTesteeBackendQueryService)(nil)
var _ actorAccessApp.TesteeAccessService = (*stubActorTesteeAccessService)(nil)
var _ clinicianApp.ClinicianQueryService = (*stubActorClinicianQueryService)(nil)
var _ clinicianApp.ClinicianRelationshipService = (*stubActorClinicianRelationshipService)(nil)
var _ operatorApp.OperatorQueryService = (*stubActorOperatorQueryService)(nil)
var _ assessmentEntryApp.AssessmentEntryService = (*stubActorAssessmentEntryService)(nil)

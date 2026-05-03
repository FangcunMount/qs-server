package assessment

import (
	"context"
	"testing"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	evaluationwaiter "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationwaiter"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

func TestProtectedQueryServiceListAssessmentsAdminKeepsWideScopeAndDefaults(t *testing.T) {
	management := &protectedManagementStub{}
	checker := &protectedAccessCheckerStub{scope: &TesteeAccessScope{IsAdmin: true}}
	svc := NewProtectedQueryService(
		management,
		nil,
		nil,
		nil,
		NewAssessmentAccessQueryService(management, checker),
	)

	_, err := svc.ListAssessments(context.Background(), ProtectedQueryScope{OrgID: 12, OperatorUserID: 34}, ListAssessmentsDTO{})
	if err != nil {
		t.Fatalf("ListAssessments returned error: %v", err)
	}

	if management.listCalls != 1 {
		t.Fatalf("management list calls = %d, want 1", management.listCalls)
	}
	if management.lastListDTO.OrgID != 12 || management.lastListDTO.Page != 1 || management.lastListDTO.PageSize != 10 {
		t.Fatalf("list dto = %#v, want default page and org scope", management.lastListDTO)
	}
	if management.lastListDTO.RestrictToAccessScope || len(management.lastListDTO.AccessibleTesteeIDs) != 0 {
		t.Fatalf("admin list dto should not be access-restricted: %#v", management.lastListDTO)
	}
	if checker.resolveCalls != 1 || checker.listAccessibleCalls != 0 || checker.validateCalls != 0 {
		t.Fatalf("access calls = resolve:%d list:%d validate:%d, want admin resolve only", checker.resolveCalls, checker.listAccessibleCalls, checker.validateCalls)
	}
}

func TestProtectedQueryServiceListAssessmentsClinicianScopeRestrictsToAccessibleIDs(t *testing.T) {
	clinicianID := uint64(7001)
	management := &protectedManagementStub{}
	checker := &protectedAccessCheckerStub{
		scope:         &TesteeAccessScope{ClinicianID: &clinicianID},
		accessibleIDs: []uint64{101, 102},
	}
	svc := NewProtectedQueryService(
		management,
		nil,
		nil,
		nil,
		NewAssessmentAccessQueryService(management, checker),
	)

	_, err := svc.ListAssessments(context.Background(), ProtectedQueryScope{OrgID: 12, OperatorUserID: 34}, ListAssessmentsDTO{Page: 2, PageSize: 20})
	if err != nil {
		t.Fatalf("ListAssessments returned error: %v", err)
	}

	if !management.lastListDTO.RestrictToAccessScope {
		t.Fatalf("list dto should be access-restricted: %#v", management.lastListDTO)
	}
	if got := management.lastListDTO.AccessibleTesteeIDs; len(got) != 2 || got[0] != 101 || got[1] != 102 {
		t.Fatalf("accessible IDs = %#v, want [101 102]", got)
	}
	if checker.resolveCalls != 1 || checker.listAccessibleCalls != 1 || checker.validateCalls != 0 {
		t.Fatalf("access calls = resolve:%d list:%d validate:%d, want clinician resolve+list only", checker.resolveCalls, checker.listAccessibleCalls, checker.validateCalls)
	}
}

func TestProtectedQueryServiceSpecifiedTesteeValidatesDirectly(t *testing.T) {
	t.Run("list assessments", func(t *testing.T) {
		testeeID := uint64(101)
		management := &protectedManagementStub{}
		checker := &protectedAccessCheckerStub{}
		svc := NewProtectedQueryService(
			management,
			nil,
			nil,
			nil,
			NewAssessmentAccessQueryService(management, checker),
		)

		_, err := svc.ListAssessments(context.Background(), ProtectedQueryScope{OrgID: 12, OperatorUserID: 34}, ListAssessmentsDTO{TesteeID: &testeeID})
		if err != nil {
			t.Fatalf("ListAssessments returned error: %v", err)
		}
		assertValidatedOnly(t, checker, testeeID)
	})

	t.Run("list reports", func(t *testing.T) {
		reportQuery := &protectedReportQueryStub{}
		checker := &protectedAccessCheckerStub{}
		svc := NewProtectedQueryService(
			&protectedManagementStub{},
			reportQuery,
			nil,
			nil,
			NewAssessmentAccessQueryService(&protectedManagementStub{}, checker),
		)

		_, err := svc.ListReports(context.Background(), ProtectedQueryScope{OrgID: 12, OperatorUserID: 34}, ListReportsDTO{TesteeID: 102})
		if err != nil {
			t.Fatalf("ListReports returned error: %v", err)
		}
		assertValidatedOnly(t, checker, 102)
		if reportQuery.lastListDTO.TesteeID != 102 || reportQuery.lastListDTO.RestrictToAccessScope {
			t.Fatalf("report list dto = %#v, want direct testee query without scope restriction", reportQuery.lastListDTO)
		}
	})

	t.Run("factor trend", func(t *testing.T) {
		scoreQuery := &protectedScoreQueryStub{}
		checker := &protectedAccessCheckerStub{}
		svc := NewProtectedQueryService(
			&protectedManagementStub{},
			nil,
			scoreQuery,
			nil,
			NewAssessmentAccessQueryService(&protectedManagementStub{}, checker),
		)

		_, err := svc.GetFactorTrend(context.Background(), ProtectedQueryScope{OrgID: 12, OperatorUserID: 34}, GetFactorTrendDTO{TesteeID: 103, FactorCode: "sleep"})
		if err != nil {
			t.Fatalf("GetFactorTrend returned error: %v", err)
		}
		assertValidatedOnly(t, checker, 103)
		if scoreQuery.lastTrendDTO.TesteeID != 103 || scoreQuery.lastTrendDTO.Limit != 10 {
			t.Fatalf("trend dto = %#v, want direct testee query with default limit", scoreQuery.lastTrendDTO)
		}
	})
}

func TestProtectedQueryServiceEmptyAccessibleIDsKeepRestrictedEmptyScope(t *testing.T) {
	management := &protectedManagementStub{}
	checker := &protectedAccessCheckerStub{scope: &TesteeAccessScope{}, accessibleIDs: []uint64{}}
	svc := NewProtectedQueryService(
		management,
		nil,
		nil,
		nil,
		NewAssessmentAccessQueryService(management, checker),
	)

	_, err := svc.ListAssessments(context.Background(), ProtectedQueryScope{OrgID: 12, OperatorUserID: 34}, ListAssessmentsDTO{})
	if err != nil {
		t.Fatalf("ListAssessments returned error: %v", err)
	}

	if !management.lastListDTO.RestrictToAccessScope {
		t.Fatalf("list dto should retain restricted empty access scope: %#v", management.lastListDTO)
	}
	if management.lastListDTO.AccessibleTesteeIDs == nil || len(management.lastListDTO.AccessibleTesteeIDs) != 0 {
		t.Fatalf("accessible IDs = %#v, want explicit empty slice", management.lastListDTO.AccessibleTesteeIDs)
	}
}

func TestProtectedQueryServiceWaitReportValidatesAccessBeforeWaiting(t *testing.T) {
	management := &protectedManagementStub{
		getByIDResult: &AssessmentResult{ID: 901, TesteeID: 401, Status: "submitted"},
	}
	checker := &protectedAccessCheckerStub{}
	wait := &protectedWaitStub{summary: evaluationwaiter.StatusSummary{Status: "interpreted", UpdatedAt: 123}}
	svc := NewProtectedQueryService(
		management,
		nil,
		nil,
		wait,
		NewAssessmentAccessQueryService(management, checker),
	)

	summary, err := svc.WaitReport(context.Background(), ProtectedQueryScope{OrgID: 12, OperatorUserID: 34}, 901)
	if err != nil {
		t.Fatalf("WaitReport returned error: %v", err)
	}

	if summary.Status != "interpreted" || wait.lastAssessmentID != 901 {
		t.Fatalf("summary = %#v wait id=%d, want interpreted via wait service", summary, wait.lastAssessmentID)
	}
	if checker.validateCalls != 1 || checker.lastValidateTesteeID != 401 {
		t.Fatalf("validate calls = %d testee=%d, want assessment access validation first", checker.validateCalls, checker.lastValidateTesteeID)
	}
}

func TestProtectedQueryServiceWaitReportWithoutWaitServiceReturnsPendingAfterAccess(t *testing.T) {
	management := &protectedManagementStub{
		getByIDResult: &AssessmentResult{ID: 902, TesteeID: 402, Status: "submitted"},
	}
	checker := &protectedAccessCheckerStub{}
	svc := NewProtectedQueryService(
		management,
		nil,
		nil,
		nil,
		NewAssessmentAccessQueryService(management, checker),
	)

	summary, err := svc.WaitReport(context.Background(), ProtectedQueryScope{OrgID: 12, OperatorUserID: 34}, 902)
	if err != nil {
		t.Fatalf("WaitReport returned error: %v", err)
	}

	if summary.Status != "pending" || summary.UpdatedAt <= 0 {
		t.Fatalf("summary = %#v, want pending fallback", summary)
	}
	if checker.validateCalls != 1 || checker.lastValidateTesteeID != 402 {
		t.Fatalf("validate calls = %d testee=%d, want assessment access validation first", checker.validateCalls, checker.lastValidateTesteeID)
	}
}

func TestProtectedQueryServiceMissingDependenciesReturnModuleNotConfigured(t *testing.T) {
	_, err := NewProtectedQueryService(nil, nil, nil, nil, nil).
		ListAssessments(context.Background(), ProtectedQueryScope{OrgID: 12, OperatorUserID: 34}, ListAssessmentsDTO{})
	assertCode(t, err, errorCode.ErrModuleInitializationFailed)

	_, err = NewProtectedQueryService(&protectedManagementStub{}, nil, nil, nil, nil).
		GetScores(context.Background(), ProtectedQueryScope{OrgID: 12, OperatorUserID: 34}, 901)
	assertCode(t, err, errorCode.ErrModuleInitializationFailed)

	_, err = NewProtectedQueryService(&protectedManagementStub{}, nil, nil, nil, nil).
		GetReport(context.Background(), ProtectedQueryScope{OrgID: 12, OperatorUserID: 34}, 901)
	assertCode(t, err, errorCode.ErrModuleInitializationFailed)
}

func assertValidatedOnly(t *testing.T, checker *protectedAccessCheckerStub, testeeID uint64) {
	t.Helper()
	if checker.validateCalls != 1 || checker.lastValidateTesteeID != testeeID {
		t.Fatalf("validate calls = %d testee=%d, want validate testee %d", checker.validateCalls, checker.lastValidateTesteeID, testeeID)
	}
	if checker.resolveCalls != 0 || checker.listAccessibleCalls != 0 {
		t.Fatalf("resolve/list calls = %d/%d, want direct validation only", checker.resolveCalls, checker.listAccessibleCalls)
	}
}

func assertCode(t *testing.T, err error, want int) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want code %d", want)
	}
	if got := cberrors.ParseCoder(err).Code(); got != want {
		t.Fatalf("error code = %d, want %d: %v", got, want, err)
	}
}

type protectedManagementStub struct {
	getByIDResult *AssessmentResult
	getByIDErr    error
	listResult    *AssessmentListResult
	listErr       error
	retryResult   *AssessmentResult
	retryErr      error
	listCalls     int
	lastListDTO   ListAssessmentsDTO
}

func (s *protectedManagementStub) GetByID(_ context.Context, id uint64) (*AssessmentResult, error) {
	if s.getByIDErr != nil {
		return nil, s.getByIDErr
	}
	if s.getByIDResult != nil {
		return s.getByIDResult, nil
	}
	return &AssessmentResult{ID: id, TesteeID: 9001, Status: "submitted"}, nil
}

func (s *protectedManagementStub) List(_ context.Context, dto ListAssessmentsDTO) (*AssessmentListResult, error) {
	s.listCalls++
	s.lastListDTO = dto
	if s.listErr != nil {
		return nil, s.listErr
	}
	if s.listResult != nil {
		return s.listResult, nil
	}
	return &AssessmentListResult{Items: []*AssessmentResult{}, Page: dto.Page, PageSize: dto.PageSize}, nil
}

func (s *protectedManagementStub) Retry(context.Context, int64, uint64) (*AssessmentResult, error) {
	if s.retryErr != nil {
		return nil, s.retryErr
	}
	return s.retryResult, nil
}

type protectedReportQueryStub struct {
	lastGetAssessmentID uint64
	lastListDTO         ListReportsDTO
	getErr              error
	listErr             error
}

func (s *protectedReportQueryStub) GetByAssessmentID(_ context.Context, assessmentID uint64) (*ReportResult, error) {
	s.lastGetAssessmentID = assessmentID
	if s.getErr != nil {
		return nil, s.getErr
	}
	return &ReportResult{AssessmentID: assessmentID}, nil
}

func (s *protectedReportQueryStub) ListByTesteeID(_ context.Context, dto ListReportsDTO) (*ReportListResult, error) {
	s.lastListDTO = dto
	if s.listErr != nil {
		return nil, s.listErr
	}
	return &ReportListResult{Items: []*ReportResult{}, Page: dto.Page, PageSize: dto.PageSize}, nil
}

type protectedScoreQueryStub struct {
	lastGetAssessmentID      uint64
	lastHighRiskAssessmentID uint64
	lastTrendDTO             GetFactorTrendDTO
	getErr                   error
	highRiskErr              error
	trendErr                 error
}

func (s *protectedScoreQueryStub) GetByAssessmentID(_ context.Context, assessmentID uint64) (*ScoreResult, error) {
	s.lastGetAssessmentID = assessmentID
	if s.getErr != nil {
		return nil, s.getErr
	}
	return &ScoreResult{AssessmentID: assessmentID}, nil
}

func (s *protectedScoreQueryStub) GetFactorTrend(_ context.Context, dto GetFactorTrendDTO) (*FactorTrendResult, error) {
	s.lastTrendDTO = dto
	if s.trendErr != nil {
		return nil, s.trendErr
	}
	return &FactorTrendResult{TesteeID: dto.TesteeID, FactorCode: dto.FactorCode}, nil
}

func (s *protectedScoreQueryStub) GetHighRiskFactors(_ context.Context, assessmentID uint64) (*HighRiskFactorsResult, error) {
	s.lastHighRiskAssessmentID = assessmentID
	if s.highRiskErr != nil {
		return nil, s.highRiskErr
	}
	return &HighRiskFactorsResult{AssessmentID: assessmentID}, nil
}

type protectedWaitStub struct {
	summary          evaluationwaiter.StatusSummary
	lastAssessmentID uint64
}

func (s *protectedWaitStub) WaitReport(_ context.Context, assessmentID uint64) evaluationwaiter.StatusSummary {
	s.lastAssessmentID = assessmentID
	return s.summary
}

type protectedAccessCheckerStub struct {
	scope                *TesteeAccessScope
	resolveErr           error
	validateErr          error
	accessibleIDs        []uint64
	accessibleIDsErr     error
	resolveCalls         int
	validateCalls        int
	listAccessibleCalls  int
	lastValidateOrgID    int64
	lastValidateUserID   int64
	lastValidateTesteeID uint64
}

func (s *protectedAccessCheckerStub) ResolveAccessScope(context.Context, int64, int64) (*TesteeAccessScope, error) {
	s.resolveCalls++
	if s.resolveErr != nil {
		return nil, s.resolveErr
	}
	if s.scope != nil {
		return s.scope, nil
	}
	return &TesteeAccessScope{IsAdmin: true}, nil
}

func (s *protectedAccessCheckerStub) ValidateTesteeAccess(_ context.Context, orgID int64, operatorUserID int64, testeeID uint64) error {
	s.validateCalls++
	s.lastValidateOrgID = orgID
	s.lastValidateUserID = operatorUserID
	s.lastValidateTesteeID = testeeID
	if s.validateErr != nil {
		return s.validateErr
	}
	return nil
}

func (s *protectedAccessCheckerStub) ListAccessibleTesteeIDs(context.Context, int64, int64) ([]uint64, error) {
	s.listAccessibleCalls++
	if s.accessibleIDsErr != nil {
		return nil, s.accessibleIDsErr
	}
	if s.accessibleIDs != nil {
		return s.accessibleIDs, nil
	}
	return []uint64{}, nil
}

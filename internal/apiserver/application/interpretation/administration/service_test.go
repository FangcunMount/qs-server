package administration

import (
	"context"
	"errors"
	authztest "github.com/FangcunMount/qs-server/internal/apiserver/application/authz/testutil"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reportprojection"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
)

func TestGetAuthorizesBeforeRead(t *testing.T) {
	denied := errors.New("denied")
	r := &adminReader{}
	s := NewService(r, adminAccess{err: denied}, reportprojection.Mapper{})
	_, err := s.GetReport(authztest.WithPermission(context.Background(), "qs:evaluation:collection:reports", "read"), Actor{OrgID: 1, OperatorUserID: 2}, GetQuery{AssessmentID: 3})
	if !errors.Is(err, denied) {
		t.Fatal(err)
	}
	if r.calls != 0 {
		t.Fatal("read before authorization")
	}
}

func TestListUsesResolvedScope(t *testing.T) {
	r := &adminReader{}
	s := NewService(r, adminAccess{scope: ListScope{
		AccessibleTesteeIDs: []uint64{7, 8},
		Restricted:          true,
		Audience:            policy.AudienceClinician,
		IsAdmin:             false,
		DecisionSource:      "test",
	}}, reportprojection.Mapper{})
	_, err := s.ListReports(authztest.WithPermission(context.Background(), "qs:evaluation:collection:reports", "list"), Actor{OrgID: 1, OperatorUserID: 2}, ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.filter.TesteeIDs) != 2 {
		t.Fatalf("filter=%#v", r.filter)
	}
}

func TestListUsesOrganizationScopeForAdministrator(t *testing.T) {
	r := &adminReader{}
	s := NewService(r, adminAccess{scope: ListScope{
		OrgID:          9,
		Audience:       policy.AudienceAdmin,
		IsAdmin:        true,
		DecisionSource: "test",
	}}, reportprojection.Mapper{})
	if _, err := s.ListReports(authztest.WithPermission(context.Background(), "qs:evaluation:collection:reports", "list"), Actor{OrgID: 9, OperatorUserID: 2}, ListQuery{}); err != nil {
		t.Fatal(err)
	}
	if r.filter.OrgID == nil || *r.filter.OrgID != 9 {
		t.Fatalf("filter=%#v", r.filter)
	}
}

func TestRestrictedClinicianAdministrationHidesModelExtra(t *testing.T) {
	r := &adminReader{row: interpretationreadmodel.ReportRow{
		ModelExtra: &interpretationreadmodel.ReportModelExtraRow{TypeCode: "secret"},
	}}
	s := NewService(r, adminAccess{decision: ReportAccessDecision{
		Audience:       policy.AudienceClinician,
		IsAdmin:        false,
		Restricted:     true,
		DecisionSource: "test",
	}}, reportprojection.Mapper{})
	result, err := s.GetReport(authztest.WithPermission(context.Background(), "qs:evaluation:collection:reports", "read"), Actor{OrgID: 1, OperatorUserID: 2}, GetQuery{AssessmentID: 3})
	if err != nil {
		t.Fatal(err)
	}
	if result.ModelExtra != nil {
		t.Fatal("restricted clinician administration view exposed model extra")
	}
}

func TestAdminAdministrationKeepsModelExtra(t *testing.T) {
	r := &adminReader{row: interpretationreadmodel.ReportRow{
		ModelExtra: &interpretationreadmodel.ReportModelExtraRow{TypeCode: "secret"},
	}}
	s := NewService(r, adminAccess{decision: ReportAccessDecision{
		Audience:       policy.AudienceAdmin,
		IsAdmin:        true,
		Restricted:     false,
		DecisionSource: "test",
	}}, reportprojection.Mapper{})
	result, err := s.GetReport(authztest.WithPermission(context.Background(), "qs:evaluation:collection:reports", "read"), Actor{OrgID: 1, OperatorUserID: 2}, GetQuery{AssessmentID: 3})
	if err != nil {
		t.Fatal(err)
	}
	if result.ModelExtra == nil || result.ModelExtra.TypeCode != "secret" {
		t.Fatalf("admin administration view lost model extra: %#v", result.ModelExtra)
	}
}

type adminAccess struct {
	err      error
	scope    ListScope
	decision ReportAccessDecision
}

func (a adminAccess) AuthorizeAssessment(context.Context, Actor, uint64) (ReportAccessDecision, error) {
	if a.err != nil {
		return ReportAccessDecision{}, a.err
	}
	if a.decision.Audience != "" {
		return a.decision, nil
	}
	return ReportAccessDecision{
		Audience:       policy.AudienceAdmin,
		IsAdmin:        true,
		Restricted:     false,
		DecisionSource: "test",
	}, nil
}

func (a adminAccess) ScopeReports(context.Context, Actor, uint64) (ListScope, error) {
	scope := a.scope
	if scope.Audience == "" {
		scope.Audience = policy.AudienceAdmin
		scope.IsAdmin = true
		scope.DecisionSource = "test"
	}
	return scope, a.err
}

type adminReader struct {
	calls  int
	filter interpretationreadmodel.ReportFilter
	row    interpretationreadmodel.ReportRow
}

func (r *adminReader) GetReportByAssessmentID(context.Context, uint64) (*interpretationreadmodel.ReportRow, error) {
	r.calls++
	row := r.row
	return &row, nil
}
func (r *adminReader) ListReports(_ context.Context, f interpretationreadmodel.ReportFilter, _ interpretationreadmodel.PageRequest) ([]interpretationreadmodel.ReportRow, int64, error) {
	r.filter = f
	return nil, 0, nil
}

func TestReportListForwardsExplicitStoreRangeIncludingEmpty(t *testing.T) {
	for _, ids := range [][]uint64{{7, 8}, nil} {
		reader := &adminReader{}
		svc := NewService(reader, adminAccess{scope: ListScope{OrgID: 1, RestrictToStoreScope: true, StoreScopedTesteeIDs: ids, Audience: policy.AudienceOperator, DecisionSource: "store_scope"}})
		ctx := authztest.WithPermission(context.Background(), "qs:evaluation:collection:reports", "list")
		if _, err := svc.ListReports(ctx, Actor{OrgID: 1, OperatorUserID: 2}, ListQuery{}); err != nil {
			t.Fatal(err)
		}
		if !reader.filter.RestrictToStoreScope || reader.filter.OrgID == nil || *reader.filter.OrgID != 1 || len(reader.filter.StoreScopedTesteeIDs) != len(ids) {
			t.Fatalf("lost explicit scope: %+v", reader.filter)
		}
	}
}

type movingReportReader struct{ adminReader }

func (r *movingReportReader) ListReports(_ context.Context, f interpretationreadmodel.ReportFilter, _ interpretationreadmodel.PageRequest) ([]interpretationreadmodel.ReportRow, int64, error) {
	r.calls++
	r.filter = f
	return []interpretationreadmodel.ReportRow{{}}, 1, nil
}

type changingReportAccess struct {
	calls int
}

func (a *changingReportAccess) ScopeReports(ctx context.Context, actor Actor, id uint64) (ListScope, error) {
	a.calls++
	result := ListScope{OrgID: actor.OrgID, Audience: policy.AudienceOperator, DecisionSource: "scope", RestrictToStoreScope: true, StoreScopedTesteeIDs: []uint64{7}}
	if a.calls > 1 {
		result.StoreScopedTesteeIDs = []uint64{8}
	}
	return result, nil
}
func (a *changingReportAccess) AuthorizeAssessment(context.Context, Actor, uint64) (ReportAccessDecision, error) {
	a.calls++
	if a.calls > 1 {
		return ReportAccessDecision{}, errors.New("testee transferred")
	}
	return ReportAccessDecision{Audience: policy.AudienceOperator, Restricted: true, DecisionSource: "scope"}, nil
}
func TestReportListDoesNotReturnPageAfterOwnershipChanges(t *testing.T) {
	access := &changingReportAccess{}
	reader := &movingReportReader{}
	service := NewService(reader, access)
	result, err := service.ListReports(authztest.WithPermission(context.Background(), "qs:evaluation:collection:reports", "list"), Actor{OrgID: 1, OperatorUserID: 2}, ListQuery{})
	if err == nil || result != nil || access.calls != 2 || reader.calls != 1 {
		t.Fatalf("stale report page returned: %+v %v calls=%d", result, err, access.calls)
	}
}
func TestReportDetailRechecksOwnershipBeforeReturningProjection(t *testing.T) {
	access := &changingReportAccess{}
	service := NewService(&adminReader{}, access)
	result, err := service.GetReport(authztest.WithPermission(context.Background(), "qs:evaluation:collection:reports", "read"), Actor{OrgID: 1, OperatorUserID: 2}, GetQuery{AssessmentID: 3})
	if err == nil || result != nil || access.calls != 2 {
		t.Fatalf("transferred report returned: %+v %v", result, err)
	}
}
func TestReportScopeComparisonIgnoresOnlySetOrder(t *testing.T) {
	a := ListScope{OrgID: 1, RestrictToStoreScope: true, StoreScopedTesteeIDs: []uint64{8, 7, 7}}
	b := ListScope{OrgID: 1, RestrictToStoreScope: true, StoreScopedTesteeIDs: []uint64{7, 8}}
	if !sameListScope(a, b) {
		t.Fatal("same scope rejected")
	}
	b.OrgID = 2
	if sameListScope(a, b) {
		t.Fatal("company boundary ignored")
	}
}

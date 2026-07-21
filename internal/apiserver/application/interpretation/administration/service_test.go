package administration

import (
	"context"
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reportprojection"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
)

func TestGetAuthorizesBeforeRead(t *testing.T) {
	denied := errors.New("denied")
	r := &adminReader{}
	s := NewService(r, adminAccess{err: denied}, reportprojection.Mapper{})
	_, err := s.GetReport(context.Background(), Actor{OrgID: 1, OperatorUserID: 2}, GetQuery{AssessmentID: 3})
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
	_, err := s.ListReports(context.Background(), Actor{OrgID: 1, OperatorUserID: 2}, ListQuery{})
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
	if _, err := s.ListReports(context.Background(), Actor{OrgID: 9, OperatorUserID: 2}, ListQuery{}); err != nil {
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
	result, err := s.GetReport(context.Background(), Actor{OrgID: 1, OperatorUserID: 2}, GetQuery{AssessmentID: 3})
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
	result, err := s.GetReport(context.Background(), Actor{OrgID: 1, OperatorUserID: 2}, GetQuery{AssessmentID: 3})
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

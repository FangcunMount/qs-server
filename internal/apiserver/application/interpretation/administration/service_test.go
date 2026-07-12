package administration

import (
	"context"
	"errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
	"testing"
)

func TestGetAuthorizesBeforeRead(t *testing.T) {
	denied := errors.New("denied")
	r := &adminReader{}
	s := NewService(r, adminAccess{err: denied})
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
	s := NewService(r, adminAccess{scope: ListScope{AccessibleTesteeIDs: []uint64{7, 8}, Restricted: true}})
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
	s := NewService(r, adminAccess{scope: ListScope{OrgID: 9}})
	if _, err := s.ListReports(context.Background(), Actor{OrgID: 9, OperatorUserID: 2}, ListQuery{}); err != nil {
		t.Fatal(err)
	}
	if r.filter.OrgID == nil || *r.filter.OrgID != 9 {
		t.Fatalf("filter=%#v", r.filter)
	}
}

type adminAccess struct {
	err   error
	scope ListScope
}

func (a adminAccess) AuthorizeAssessment(context.Context, Actor, uint64) error { return a.err }
func (a adminAccess) ScopeReports(context.Context, Actor, uint64) (ListScope, error) {
	return a.scope, a.err
}

type adminReader struct {
	calls  int
	filter interpretationreadmodel.ReportFilter
}

func (r *adminReader) GetReportByAssessmentID(context.Context, uint64) (*interpretationreadmodel.ReportRow, error) {
	r.calls++
	return &interpretationreadmodel.ReportRow{}, nil
}
func (r *adminReader) ListReports(_ context.Context, f interpretationreadmodel.ReportFilter, _ interpretationreadmodel.PageRequest) ([]interpretationreadmodel.ReportRow, int64, error) {
	r.filter = f
	return nil, 0, nil
}

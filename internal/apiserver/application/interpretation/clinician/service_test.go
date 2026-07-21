package clinician

import (
	"context"
	"errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reportprojection"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
	"testing"
)

func TestClinicianAuthorizationPrecedesRead(t *testing.T) {
	denied := errors.New("denied")
	r := &reader{}
	s := NewService(r, access{err: denied}, reportprojection.Mapper{})
	_, err := s.GetParticipantReport(context.Background(), Actor{OrgID: 1, OperatorUserID: 2}, GetQuery{TesteeID: 3, AssessmentID: 4})
	if !errors.Is(err, denied) {
		t.Fatal(err)
	}
	if r.calls != 0 {
		t.Fatal("read before authorization")
	}
}
func TestClinicianViewHidesModelExtra(t *testing.T) {
	r := &reader{row: interpretationreadmodel.ReportRow{ModelExtra: &interpretationreadmodel.ReportModelExtraRow{TypeCode: "secret"}}}
	s := NewService(r, access{}, reportprojection.Mapper{})
	result, err := s.GetParticipantReport(context.Background(), Actor{OrgID: 1, OperatorUserID: 2}, GetQuery{TesteeID: 3, AssessmentID: 4})
	if err != nil {
		t.Fatal(err)
	}
	if result.ModelExtra != nil {
		t.Fatal("clinician view exposed model extra")
	}
}

type access struct{ err error }

func (a access) AuthorizeParticipant(context.Context, Actor, uint64) error { return a.err }
func (a access) AuthorizeParticipantAssessment(context.Context, Actor, uint64, uint64) error {
	return a.err
}

type reader struct {
	calls int
	row   interpretationreadmodel.ReportRow
}

func (r *reader) GetReportByAssessmentID(context.Context, uint64) (*interpretationreadmodel.ReportRow, error) {
	r.calls++
	return &r.row, nil
}
func (r *reader) ListReports(context.Context, interpretationreadmodel.ReportFilter, interpretationreadmodel.PageRequest) ([]interpretationreadmodel.ReportRow, int64, error) {
	return nil, 0, nil
}

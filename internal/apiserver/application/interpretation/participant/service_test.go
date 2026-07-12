package participant

import (
	"context"
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
)

func TestGetMyReportAuthorizesBeforeReading(t *testing.T) {
	denied := errors.New("denied")
	reader := &readerStub{}
	service := NewService(reader, accessStub{err: denied})
	_, err := service.GetMyReport(context.Background(), Actor{TesteeID: 7}, GetQuery{AssessmentID: 42})
	if !errors.Is(err, denied) {
		t.Fatalf("error = %v, want denied", err)
	}
	if reader.getCalls != 0 {
		t.Fatalf("reader called before authorization")
	}
}

func TestListMyReportsScopesToActor(t *testing.T) {
	reader := &readerStub{}
	service := NewService(reader, accessStub{})
	if _, err := service.ListMyReports(context.Background(), Actor{TesteeID: 7}, ListQuery{}); err != nil {
		t.Fatal(err)
	}
	if reader.testeeID == nil || *reader.testeeID != 7 {
		t.Fatalf("filter testee = %v, want 7", reader.testeeID)
	}
}

type accessStub struct{ err error }

func (a accessStub) AuthorizeOwnAssessment(context.Context, uint64, uint64) error { return a.err }

type readerStub struct {
	getCalls int
	testeeID *uint64
}

func (r *readerStub) GetReportByID(context.Context, uint64) (*interpretationreadmodel.ReportRow, error) {
	panic("unexpected")
}
func (r *readerStub) GetReportByAssessmentID(context.Context, uint64) (*interpretationreadmodel.ReportRow, error) {
	r.getCalls++
	return &interpretationreadmodel.ReportRow{}, nil
}
func (r *readerStub) ListReports(_ context.Context, filter interpretationreadmodel.ReportFilter, _ interpretationreadmodel.PageRequest) ([]interpretationreadmodel.ReportRow, int64, error) {
	r.testeeID = filter.TesteeID
	return nil, 0, nil
}

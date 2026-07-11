package interpretation

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
)

type reportQueryReaderStub struct {
	row        *evaluationreadmodel.ReportRow
	rows       []evaluationreadmodel.ReportRow
	total      int64
	lastFilter evaluationreadmodel.ReportFilter
	lastPage   evaluationreadmodel.PageRequest
}

func (s *reportQueryReaderStub) GetReportByID(context.Context, uint64) (*evaluationreadmodel.ReportRow, error) {
	return s.row, nil
}

func (s *reportQueryReaderStub) GetReportByAssessmentID(context.Context, uint64) (*evaluationreadmodel.ReportRow, error) {
	return s.row, nil
}

func (s *reportQueryReaderStub) ListReports(_ context.Context, filter evaluationreadmodel.ReportFilter, page evaluationreadmodel.PageRequest) ([]evaluationreadmodel.ReportRow, int64, error) {
	s.lastFilter = filter
	s.lastPage = page
	return s.rows, s.total, nil
}

func TestReportQueryServiceReturnsInterpretationProjectionForAssessment(t *testing.T) {
	reader := &reportQueryReaderStub{row: &evaluationreadmodel.ReportRow{
		AssessmentID: 42,
		ModelName:    "压力评估",
		ModelCode:    "stress",
		Conclusion:   "稳定",
		CreatedAt:    time.Unix(123, 0),
	}}

	result, err := NewReportQueryService(reader).GetByAssessmentID(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetByAssessmentID() error = %v", err)
	}
	if result.AssessmentID != 42 || result.ModelCode != "stress" || result.Conclusion != "稳定" {
		t.Fatalf("result = %#v", result)
	}
}

func TestReportQueryServiceListsRestrictedAccessibleTesteeScope(t *testing.T) {
	reader := &reportQueryReaderStub{
		rows:  []evaluationreadmodel.ReportRow{{AssessmentID: 43}},
		total: 1,
	}

	result, err := NewReportQueryService(reader).ListByTesteeID(context.Background(), ListReportsDTO{
		Page:                  0,
		PageSize:              0,
		AccessibleTesteeIDs:   []uint64{7, 8},
		RestrictToAccessScope: true,
	})
	if err != nil {
		t.Fatalf("ListByTesteeID() error = %v", err)
	}
	if result.Page != 1 || result.PageSize != 10 || result.Total != 1 || len(result.Items) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if got := reader.lastFilter.TesteeIDs; len(got) != 2 || got[0] != 7 || got[1] != 8 {
		t.Fatalf("filter testee ids = %#v, want [7 8]", got)
	}
}

var _ evaluationreadmodel.ReportReader = (*reportQueryReaderStub)(nil)

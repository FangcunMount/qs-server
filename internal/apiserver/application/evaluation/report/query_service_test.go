package report

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
)

type queryReportReaderStub struct {
	rows   []evaluationreadmodel.ReportRow
	total  int64
	filter evaluationreadmodel.ReportFilter
	page   evaluationreadmodel.PageRequest
}

func (r *queryReportReaderStub) GetReportByID(context.Context, uint64) (*evaluationreadmodel.ReportRow, error) {
	return nil, nil
}

func (r *queryReportReaderStub) GetReportByAssessmentID(context.Context, uint64) (*evaluationreadmodel.ReportRow, error) {
	return nil, nil
}

func (r *queryReportReaderStub) ListReports(_ context.Context, filter evaluationreadmodel.ReportFilter, page evaluationreadmodel.PageRequest) ([]evaluationreadmodel.ReportRow, int64, error) {
	r.filter = filter
	r.page = page
	return r.rows, r.total, nil
}

func TestReportQueryServiceListByTesteeIDBuildsPaginationResult(t *testing.T) {
	createdAt := time.Date(2026, time.April, 22, 10, 30, 0, 0, time.Local)
	reader := &queryReportReaderStub{
		rows: []evaluationreadmodel.ReportRow{
			{
				AssessmentID: 1001,
				ScaleName:    "Scale",
				ScaleCode:    "scale-code",
				TotalScore:   88,
				RiskLevel:    "high",
				Conclusion:   "high risk",
				CreatedAt:    createdAt,
			},
		},
		total: 1,
	}

	svc := NewReportQueryServiceWithReadModel(nil, reader)
	result, err := svc.ListByTesteeID(context.Background(), ListReportsDTO{
		TesteeID: 2001,
		Page:     1,
		PageSize: 10,
	})
	if err != nil {
		t.Fatalf("ListByTesteeID returned error: %v", err)
	}
	if result.Total != 1 || result.TotalPages != 1 {
		t.Fatalf("unexpected pagination result: %+v", result)
	}
	if len(result.Items) != 1 || result.Items[0].ScaleCode != "scale-code" {
		t.Fatalf("unexpected items: %+v", result.Items)
	}
	if reader.filter.TesteeID == nil || *reader.filter.TesteeID != 2001 {
		t.Fatalf("reader did not receive testee filter: %+v", reader.filter)
	}
	if reader.page.Page != 1 || reader.page.PageSize != 10 {
		t.Fatalf("reader did not receive pagination: %+v", reader.page)
	}
}

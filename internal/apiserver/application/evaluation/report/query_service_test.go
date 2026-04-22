package report

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type queryReportRepoStub struct {
	items []*domainReport.InterpretReport
	total int64
}

func (r *queryReportRepoStub) Save(context.Context, *domainReport.InterpretReport) error { return nil }

func (r *queryReportRepoStub) SaveWithTesteeAndEvents(context.Context, *domainReport.InterpretReport, testee.ID, []event.DomainEvent) error {
	return nil
}

func (r *queryReportRepoStub) FindByID(context.Context, domainReport.ID) (*domainReport.InterpretReport, error) {
	return nil, nil
}

func (r *queryReportRepoStub) FindByAssessmentID(context.Context, domainReport.AssessmentID) (*domainReport.InterpretReport, error) {
	return nil, nil
}

func (r *queryReportRepoStub) FindByTesteeID(context.Context, testee.ID, domainReport.Pagination) ([]*domainReport.InterpretReport, int64, error) {
	return r.items, r.total, nil
}

func (r *queryReportRepoStub) FindByTesteeIDs(context.Context, []testee.ID, domainReport.Pagination) ([]*domainReport.InterpretReport, int64, error) {
	return nil, 0, nil
}

func (r *queryReportRepoStub) Update(context.Context, *domainReport.InterpretReport) error {
	return nil
}

func (r *queryReportRepoStub) Delete(context.Context, domainReport.ID) error { return nil }

func (r *queryReportRepoStub) ExistsByID(context.Context, domainReport.ID) (bool, error) {
	return false, nil
}

func TestReportQueryServiceListByTesteeIDBuildsPaginationResult(t *testing.T) {
	createdAt := time.Date(2026, time.April, 22, 10, 30, 0, 0, time.Local)
	repo := &queryReportRepoStub{
		items: []*domainReport.InterpretReport{
			domainReport.ReconstructInterpretReport(
				domainReport.NewID(1001),
				"Scale",
				"scale-code",
				88,
				domainReport.RiskLevelHigh,
				"high risk",
				nil,
				nil,
				createdAt,
				nil,
			),
		},
		total: 1,
	}

	svc := NewReportQueryService(repo)
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
}

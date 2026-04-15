package report

import (
	"context"
	"testing"

	cbErrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type reportRepoStub struct {
	report *domainReport.InterpretReport
}

func (r *reportRepoStub) Save(context.Context, *domainReport.InterpretReport) error { return nil }
func (r *reportRepoStub) SaveWithTesteeAndEvents(context.Context, *domainReport.InterpretReport, testee.ID, []event.DomainEvent) error {
	return nil
}
func (r *reportRepoStub) Update(context.Context, *domainReport.InterpretReport) error {
	return nil
}
func (r *reportRepoStub) Delete(context.Context, domainReport.ID) error { return nil }
func (r *reportRepoStub) ExistsByID(context.Context, domainReport.ID) (bool, error) {
	return r.report != nil, nil
}
func (r *reportRepoStub) FindByID(context.Context, domainReport.ID) (*domainReport.InterpretReport, error) {
	return r.report, nil
}
func (r *reportRepoStub) FindByAssessmentID(context.Context, domainReport.AssessmentID) (*domainReport.InterpretReport, error) {
	return r.report, nil
}
func (r *reportRepoStub) FindByTesteeID(context.Context, testee.ID, domainReport.Pagination) ([]*domainReport.InterpretReport, int64, error) {
	return nil, 0, nil
}
func (r *reportRepoStub) FindByTesteeIDs(context.Context, []testee.ID, domainReport.Pagination) ([]*domainReport.InterpretReport, int64, error) {
	return nil, 0, nil
}

func TestReportExportServiceWithoutExporterReturnsUnsupported(t *testing.T) {
	repo := &reportRepoStub{
		report: domainReport.NewInterpretReport(
			domainReport.NewID(1001),
			"Scale",
			"scale-code",
			80,
			domainReport.RiskLevelLow,
			"ok",
			nil,
			nil,
		),
	}
	svc := NewReportExportService(repo, nil)

	if got := svc.GetSupportedFormats(); len(got) != 0 {
		t.Fatalf("expected no supported formats when exporter is unsupported, got %v", got)
	}

	_, err := svc.ExportPDF(context.Background(), 1001, ExportOptionsDTO{})
	if err == nil {
		t.Fatalf("expected unsupported export error")
	}
	if !cbErrors.IsCode(err, code.ErrUnsupportedOperation) {
		t.Fatalf("expected ErrUnsupportedOperation, got %v", err)
	}
}

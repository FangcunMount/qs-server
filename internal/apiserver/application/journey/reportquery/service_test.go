package reportquery

import (
	"context"
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	interpretationAdmin "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/administration"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
	"testing"
	"time"
)

func TestProjectAssessmentMapsGeneratedReportWithoutMutatingEvaluationResult(t *testing.T) {
	created := time.Unix(123, 0)
	reader := &journeyReader{row: &interpretationreadmodel.ReportRow{CreatedAt: created}}
	original := &assessmentApp.AssessmentResult{ID: 42, Status: "evaluated"}
	projected, err := NewAdministrationService(reader, adminStub{}).ProjectAssessment(context.Background(), original)
	if err != nil {
		t.Fatal(err)
	}
	if projected.Status != "interpreted" || projected.InterpretedAt == nil || !projected.InterpretedAt.Equal(created) {
		t.Fatalf("projection=%#v", projected)
	}
	if original.Status != "evaluated" {
		t.Fatal("evaluation result mutated")
	}
}

type adminStub struct{}

func (adminStub) GetReport(context.Context, interpretationAdmin.Actor, interpretationAdmin.GetQuery) (*interpretationAdmin.Report, error) {
	return &interpretationAdmin.Report{}, nil
}
func (adminStub) ListReports(context.Context, interpretationAdmin.Actor, interpretationAdmin.ListQuery) (*interpretationAdmin.ListResult, error) {
	return &interpretationAdmin.ListResult{}, nil
}

type journeyReader struct {
	row *interpretationreadmodel.ReportRow
}

func (j *journeyReader) GetReportByID(context.Context, uint64) (*interpretationreadmodel.ReportRow, error) {
	return j.row, nil
}
func (j *journeyReader) GetReportByAssessmentID(context.Context, uint64) (*interpretationreadmodel.ReportRow, error) {
	return j.row, nil
}
func (j *journeyReader) ListReports(context.Context, interpretationreadmodel.ReportFilter, interpretationreadmodel.PageRequest) ([]interpretationreadmodel.ReportRow, int64, error) {
	return nil, 0, nil
}

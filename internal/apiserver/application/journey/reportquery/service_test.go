package reportquery

import (
	"context"
	"testing"
	"time"

	evaluationoperator "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/operator"
	interpretationAdmin "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/administration"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
)

func TestProjectAssessmentMapsGeneratedReportWithoutMutatingEvaluationResult(t *testing.T) {
	created := time.Unix(123, 0)
	reader := &journeyReader{row: &interpretationreadmodel.ReportRow{CreatedAt: created}}
	original := &evaluationoperator.Assessment{ID: 42, Status: "evaluated"}
	projected, err := NewAdministrationService(reader, adminStub{}, nil).ProjectAssessment(context.Background(), original)
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

func TestProjectAssessmentKeepsEvaluatedWhenReportDoesNotExist(t *testing.T) {
	original := &evaluationoperator.Assessment{ID: 42, Status: "evaluated"}
	projected, err := NewAdministrationService(&journeyReader{err: interpretationreadmodel.ErrReportNotFound}, adminStub{}, nil).ProjectAssessment(context.Background(), original)
	if err != nil {
		t.Fatal(err)
	}
	if projected.Status != "evaluated" || projected.InterpretedAt != nil {
		t.Fatalf("projection=%#v", projected)
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
	err error
}

func (j *journeyReader) GetReportByAssessmentID(context.Context, uint64) (*interpretationreadmodel.ReportRow, error) {
	return j.row, j.err
}
func (j *journeyReader) ListReports(context.Context, interpretationreadmodel.ReportFilter, interpretationreadmodel.PageRequest) ([]interpretationreadmodel.ReportRow, int64, error) {
	return nil, 0, nil
}

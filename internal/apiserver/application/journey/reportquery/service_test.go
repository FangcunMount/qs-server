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

func TestListAssessmentProjectionUsesSingleBatchMetadataRead(t *testing.T) {
	created := time.Unix(456, 0)
	reader := &batchJourneyReader{
		journeyReader: journeyReader{},
		metadata: map[uint64]interpretationreadmodel.CurrentReportMetadata{
			1: {AssessmentID: 1, Status: interpretationreadmodel.CurrentReportMetadataFound, CreatedAt: created},
			2: {AssessmentID: 2, Status: interpretationreadmodel.CurrentReportMetadataMissing},
		},
	}
	operator := &assessmentQueryStub{result: &evaluationoperator.AssessmentList{
		Items: []*evaluationoperator.Assessment{
			{ID: 1, Status: "evaluated"},
			{ID: 2, Status: "evaluated"},
		},
	}}

	result, err := NewAdministrationService(reader, adminStub{}, operator).ListAssessmentProjection(context.Background(), Scope{OrgID: 8, OperatorUserID: 9}, evaluationoperator.ListQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if reader.batchCalls != 1 || reader.singleCalls != 0 {
		t.Fatalf("batch/single calls = %d/%d, want 1/0", reader.batchCalls, reader.singleCalls)
	}
	if result.Items[0].Status != "interpreted" || result.Items[0].InterpretedAt == nil || result.Items[1].Status != "evaluated" {
		t.Fatalf("projection = %#v", result.Items)
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
	row         *interpretationreadmodel.ReportRow
	err         error
	singleCalls int
}

func (j *journeyReader) GetReportByAssessmentID(context.Context, uint64) (*interpretationreadmodel.ReportRow, error) {
	j.singleCalls++
	return j.row, j.err
}
func (j *journeyReader) ListReports(context.Context, interpretationreadmodel.ReportFilter, interpretationreadmodel.PageRequest) ([]interpretationreadmodel.ReportRow, int64, error) {
	return nil, 0, nil
}

type batchJourneyReader struct {
	journeyReader
	metadata   map[uint64]interpretationreadmodel.CurrentReportMetadata
	batchCalls int
}

func (r *batchJourneyReader) GetCurrentReportMetadataByAssessmentIDs(context.Context, []uint64) (map[uint64]interpretationreadmodel.CurrentReportMetadata, error) {
	r.batchCalls++
	return r.metadata, nil
}

type assessmentQueryStub struct {
	result *evaluationoperator.AssessmentList
}

func (*assessmentQueryStub) GetAssessment(context.Context, evaluationoperator.Actor, uint64) (*evaluationoperator.Assessment, error) {
	return nil, nil
}

func (s *assessmentQueryStub) ListAssessments(context.Context, evaluationoperator.Actor, evaluationoperator.ListQuery) (*evaluationoperator.AssessmentList, error) {
	return s.result, nil
}

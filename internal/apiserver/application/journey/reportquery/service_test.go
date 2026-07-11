package reportquery

import (
	"context"
	"errors"
	"testing"
	"time"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	interpretationApp "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

type accessStub struct {
	loadErr    error
	loaded     uint64
	listScope  assessmentApp.TesteeListAccessScope
	listErr    error
	listTestee uint64
}

func (s *accessStub) LoadAccessibleAssessment(_ context.Context, _, _ int64, assessmentID uint64) (*assessmentApp.AccessibleAssessmentContext, error) {
	s.loaded = assessmentID
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	return &assessmentApp.AccessibleAssessmentContext{AssessmentID: assessmentID}, nil
}

func (s *accessStub) ScopeTesteeList(_ context.Context, _, _ int64, testeeID uint64) (assessmentApp.TesteeListAccessScope, error) {
	s.listTestee = testeeID
	return s.listScope, s.listErr
}

type reportStub struct {
	getResult      *interpretationApp.ReportResult
	getErr         error
	getCalls       int
	lastAssessment uint64
	lastList       interpretationApp.ListReportsDTO
}

func (s *reportStub) GetByAssessmentID(_ context.Context, assessmentID uint64) (*interpretationApp.ReportResult, error) {
	s.getCalls++
	s.lastAssessment = assessmentID
	return s.getResult, s.getErr
}

func (s *reportStub) ListByTesteeID(_ context.Context, dto interpretationApp.ListReportsDTO) (*interpretationApp.ReportListResult, error) {
	s.lastList = dto
	return &interpretationApp.ReportListResult{}, nil
}

func (s *reportStub) GetOutcomeByAssessmentID(context.Context, uint64) (*interpretationApp.ReportOutcomeResult, error) {
	return &interpretationApp.ReportOutcomeResult{}, nil
}

func (s *reportStub) ListOutcomeByTesteeID(context.Context, interpretationApp.ListReportsDTO) (*interpretationApp.ReportOutcomeListResult, error) {
	return &interpretationApp.ReportOutcomeListResult{}, nil
}

func TestProjectAssessmentMapsGeneratedReportWithoutMutatingEvaluationResult(t *testing.T) {
	createdAt := time.Unix(123, 0)
	original := &assessmentApp.AssessmentResult{ID: 42, Status: "evaluated"}
	reports := &reportStub{getResult: &interpretationApp.ReportResult{AssessmentID: 42, CreatedAt: createdAt}}

	projected, err := NewService(&accessStub{}, reports).ProjectAssessment(context.Background(), original)
	if err != nil {
		t.Fatal(err)
	}
	if projected.Assessment != original || projected.Status != "interpreted" || projected.InterpretedAt == nil || !projected.InterpretedAt.Equal(createdAt) {
		t.Fatalf("projected = %#v, want copied interpreted projection", projected)
	}
	if original.Status != "evaluated" {
		t.Fatalf("original was mutated: %#v", original)
	}
}

func TestProjectAssessmentKeepsEvaluatedWhenReportDoesNotExist(t *testing.T) {
	original := &assessmentApp.AssessmentResult{ID: 42, Status: "evaluated"}
	reports := &reportStub{getErr: cberrors.WithCode(errorCode.ErrInterpretReportNotFound, "report not found")}

	projected, err := NewService(&accessStub{}, reports).ProjectAssessment(context.Background(), original)
	if err != nil {
		t.Fatal(err)
	}
	if projected.Assessment != original || projected.Status != "evaluated" || projected.InterpretedAt != nil {
		t.Fatalf("projected = %#v, want unchanged evaluated result", projected)
	}
}

func TestGetReportAuthorizesBeforeReadingInterpretation(t *testing.T) {
	accessErr := errors.New("forbidden")
	access := &accessStub{loadErr: accessErr}
	reports := &reportStub{}

	_, err := NewService(access, reports).GetReport(context.Background(), Scope{OrgID: 1, OperatorUserID: 2}, 42)
	if !errors.Is(err, accessErr) {
		t.Fatalf("GetReport() error = %v, want %v", err, accessErr)
	}
	if reports.getCalls != 0 {
		t.Fatalf("report query calls = %d, want 0 before access succeeds", reports.getCalls)
	}
}

func TestListReportsPassesJourneyAccessScopeToInterpretation(t *testing.T) {
	access := &accessStub{listScope: assessmentApp.TesteeListAccessScope{
		AccessibleTesteeIDs:   []uint64{7, 8},
		RestrictToAccessScope: true,
	}}
	reports := &reportStub{}

	_, err := NewService(access, reports).ListReports(context.Background(), Scope{OrgID: 1, OperatorUserID: 2}, interpretationApp.ListReportsDTO{Page: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !reports.lastList.RestrictToAccessScope || len(reports.lastList.AccessibleTesteeIDs) != 2 || reports.lastList.Page != 2 {
		t.Fatalf("scoped report query = %#v", reports.lastList)
	}
}

var _ AssessmentAccess = (*accessStub)(nil)
var _ interpretationApp.ReportQueryService = (*reportStub)(nil)

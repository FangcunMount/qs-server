package typologyassessment

import (
	"context"
	"errors"
	"testing"
	"time"

	evaluationapp "github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
	appreportstatus "github.com/FangcunMount/qs-server/internal/collection-server/application/reportstatus"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportwait"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
)

type fakeEvaluationReader struct {
	detail        *evaluationapp.AssessmentDetailResponse
	report        *evaluationapp.AssessmentReportResponse
	list          *evaluationapp.ListAssessmentsResponse
	err           error
	listModelKind string
}

func (f *fakeEvaluationReader) GetMyAssessment(context.Context, uint64, uint64) (*evaluationapp.AssessmentDetailResponse, error) {
	return f.detail, f.err
}

func (f *fakeEvaluationReader) GetAssessmentScores(context.Context, uint64, uint64) ([]evaluationapp.FactorScoreResponse, error) {
	return nil, nil
}
func (f *fakeEvaluationReader) GetFactorTrend(context.Context, uint64, string, int32) ([]evaluationapp.TrendPointResponse, error) {
	return nil, nil
}
func (f *fakeEvaluationReader) GetHighRiskFactors(context.Context, uint64, uint64) ([]evaluationapp.FactorScoreResponse, error) {
	return nil, nil
}
func (f *fakeEvaluationReader) ListMyAssessments(_ context.Context, _ uint64, _ string, _ string, _ string, _ string, _ string, modelKind string, _ int32, _ int32) (*evaluationapp.ListAssessmentsResponse, error) {
	f.listModelKind = modelKind
	return f.list, f.err
}
func (f *fakeEvaluationReader) GetAssessmentReport(context.Context, uint64, uint64) (*evaluationapp.AssessmentReportResponse, error) {
	return f.report, f.err
}
func (f *fakeEvaluationReader) ResolveAssessmentByAnswerSheetID(context.Context, uint64) (uint64, uint64, error) {
	return 0, 0, nil
}

type fakeStatusCache struct {
	snapshots map[string]*reportstatus.Snapshot
}

func (f *fakeStatusCache) Get(_ context.Context, assessmentID string) (*reportstatus.Snapshot, error) {
	if f.snapshots == nil {
		return nil, nil
	}
	return f.snapshots[assessmentID], nil
}

func (f *fakeStatusCache) Set(context.Context, *reportstatus.Snapshot, time.Duration) error {
	return nil
}
func (f *fakeStatusCache) SetIfHigherPriority(context.Context, *reportstatus.Snapshot, time.Duration) error {
	return nil
}

func TestQueryServiceGetRejectsNonTypologyModel(t *testing.T) {
	t.Parallel()

	svc := NewQueryService(&fakeEvaluationReader{
		detail: &evaluationapp.AssessmentDetailResponse{
			Model: evaluationapp.ModelIdentityResponse{Kind: "scale"},
		},
	}, nil)
	_, err := svc.Get(context.Background(), 1, 2)
	if !IsNotTypologyAssessment(err) {
		t.Fatalf("expected personality guard error, got %v", err)
	}
}

func TestQueryServiceGetAcceptsCanonicalTypologyModel(t *testing.T) {
	t.Parallel()

	svc := NewQueryService(&fakeEvaluationReader{
		detail: &evaluationapp.AssessmentDetailResponse{
			Model: evaluationapp.ModelIdentityResponse{Kind: typologyModelKind, Code: "mbti"},
		},
	}, nil)
	got, err := svc.Get(context.Background(), 1, 2)
	if err != nil || got == nil {
		t.Fatalf("Get() = %#v, %v; want canonical typology model", got, err)
	}
}

func TestQueryServiceGetReportAcceptsCanonicalTypologyModel(t *testing.T) {
	t.Parallel()

	svc := NewQueryService(&fakeEvaluationReader{
		report: &evaluationapp.AssessmentReportResponse{
			AssessmentID: "42",
			Model:        evaluationapp.ModelIdentityResponse{Kind: typologyModelKind, Code: "SBTI_FUN"},
		},
	}, nil)
	got, err := svc.GetReport(context.Background(), 1, 42)
	if err != nil || got == nil || got.AssessmentID != "42" {
		t.Fatalf("GetReport() = %#v, %v; want canonical typology report", got, err)
	}
}

func TestQueryServiceListUsesCanonicalTypologyKind(t *testing.T) {
	t.Parallel()

	reader := &fakeEvaluationReader{list: &evaluationapp.ListAssessmentsResponse{}}
	svc := NewQueryService(reader, nil)
	if _, err := svc.List(context.Background(), 1, &ListAssessmentsRequest{}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if reader.listModelKind != typologyModelKind {
		t.Fatalf("model kind = %q, want %q", reader.listModelKind, typologyModelKind)
	}
}

func TestQueryServiceGetReportStatus(t *testing.T) {
	t.Parallel()

	reader := &fakeEvaluationReader{
		detail: &evaluationapp.AssessmentDetailResponse{
			ID:    "2",
			Model: evaluationapp.ModelIdentityResponse{Kind: typologyModelKind},
		},
	}
	wait := reportwait.NewService(reader, &fakeStatusCache{
		snapshots: map[string]*reportstatus.Snapshot{
			"2": {Status: "processing", Stage: "scoring", UpdatedAt: time.Unix(99, 0).UTC()},
		},
	}, nil, nil, reportwait.DefaultConfig())
	svc := NewQueryService(reader, wait)

	status, err := svc.GetReportStatus(context.Background(), 1, 2)
	if err != nil {
		t.Fatalf("GetReportStatus: %v", err)
	}
	if status.Status != "processing" || status.Stage != "scoring" {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestQueryServiceGetReportStatusDeniesForeignAssessment(t *testing.T) {
	t.Parallel()

	wait := reportwait.NewService(&fakeEvaluationReader{}, &fakeStatusCache{
		snapshots: map[string]*reportstatus.Snapshot{
			"2": {Status: "completed", Stage: "completed", UpdatedAt: time.Unix(99, 0).UTC()},
		},
	}, nil, nil, reportwait.DefaultConfig())
	svc := NewQueryService(&fakeEvaluationReader{}, wait)

	_, err := svc.GetReportStatus(context.Background(), 1, 2)
	if !errors.Is(err, appreportstatus.ErrAssessmentAccess) {
		t.Fatalf("GetReportStatus error = %v, want %v", err, appreportstatus.ErrAssessmentAccess)
	}
}

func TestQueryServiceGetReturnsDetail(t *testing.T) {
	t.Parallel()

	svc := NewQueryService(&fakeEvaluationReader{
		detail: &evaluationapp.AssessmentDetailResponse{
			ID:    "42",
			Model: evaluationapp.ModelIdentityResponse{Kind: typologyModelKind, Code: "mbti"},
		},
	}, nil)
	got, err := svc.Get(context.Background(), 1, 42)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.Model.Code != "mbti" {
		t.Fatalf("unexpected detail: %+v", got)
	}
}

func TestQueryServiceGetPropagatesError(t *testing.T) {
	t.Parallel()

	want := errors.New("boom")
	svc := NewQueryService(&fakeEvaluationReader{err: want}, nil)
	_, err := svc.Get(context.Background(), 1, 2)
	if !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
	}
}

func TestQueryServiceGetReportStatusInterpretedEnrichesModel(t *testing.T) {
	t.Parallel()

	reader := &fakeEvaluationReader{
		detail: &evaluationapp.AssessmentDetailResponse{
			ID:    "42",
			Model: evaluationapp.ModelIdentityResponse{Kind: typologyModelKind, Code: "sbti"},
			Level: &evaluationapp.ResultLevelResponse{Code: "INTJ"},
		},
	}
	wait := reportwait.NewService(reader, &fakeStatusCache{
		snapshots: map[string]*reportstatus.Snapshot{
			"42": {Status: "interpreted", Stage: "done", UpdatedAt: time.Unix(1, 0).UTC()},
		},
	}, nil, nil, reportwait.DefaultConfig())
	svc := NewQueryService(reader, wait)

	status, err := svc.GetReportStatus(context.Background(), 1, 42)
	if err != nil {
		t.Fatalf("GetReportStatus: %v", err)
	}
	if status.Status != "interpreted" {
		t.Fatalf("status = %q", status.Status)
	}
	if status.Model == nil || status.Model.Code != "sbti" {
		t.Fatalf("expected enriched model, got %+v", status.Model)
	}
}

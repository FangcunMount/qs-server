package personalityassessment

import (
	"context"
	"errors"
	"testing"
	"time"

	evaluationapp "github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportwait"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
)

type fakeEvaluationReader struct {
	detail *evaluationapp.AssessmentDetailV2Response
	err    error
}

func (f *fakeEvaluationReader) GetMyAssessmentV2(context.Context, uint64, uint64) (*evaluationapp.AssessmentDetailV2Response, error) {
	return f.detail, f.err
}

func (f *fakeEvaluationReader) GetMyAssessment(context.Context, uint64, uint64) (*evaluationapp.AssessmentDetailResponse, error) {
	return nil, nil
}
func (f *fakeEvaluationReader) GetMyAssessmentByAnswerSheetID(context.Context, uint64) (*evaluationapp.AssessmentDetailResponse, error) {
	return nil, nil
}
func (f *fakeEvaluationReader) ListMyAssessments(context.Context, uint64, string, string, string, string, string, string, int32, int32) (*evaluationapp.ListAssessmentsResponse, error) {
	return nil, nil
}
func (f *fakeEvaluationReader) GetAssessmentScores(context.Context, uint64, uint64) ([]evaluationapp.FactorScoreResponse, error) {
	return nil, nil
}
func (f *fakeEvaluationReader) GetAssessmentReport(context.Context, uint64) (*evaluationapp.AssessmentReportResponse, error) {
	return nil, nil
}
func (f *fakeEvaluationReader) GetFactorTrend(context.Context, uint64, string, int32) ([]evaluationapp.TrendPointResponse, error) {
	return nil, nil
}
func (f *fakeEvaluationReader) GetHighRiskFactors(context.Context, uint64, uint64) ([]evaluationapp.FactorScoreResponse, error) {
	return nil, nil
}
func (f *fakeEvaluationReader) ListMyAssessmentsV2(context.Context, uint64, string, string, string, string, string, int32, int32) (*evaluationapp.ListAssessmentsV2Response, error) {
	return nil, nil
}
func (f *fakeEvaluationReader) GetAssessmentReportV2(context.Context, uint64, uint64) (*evaluationapp.AssessmentReportV2Response, error) {
	return nil, nil
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

func TestQueryServiceGetRejectsNonPersonalityModel(t *testing.T) {
	t.Parallel()

	svc := NewQueryService(&fakeEvaluationReader{
		detail: &evaluationapp.AssessmentDetailV2Response{
			Model: evaluationapp.ModelIdentityResponse{Kind: "scale"},
		},
	}, nil)
	_, err := svc.Get(context.Background(), 1, 2)
	if !IsNotPersonalityAssessment(err) {
		t.Fatalf("expected personality guard error, got %v", err)
	}
}

func TestQueryServiceGetReportStatus(t *testing.T) {
	t.Parallel()

	wait := reportwait.NewService(nil, &fakeStatusCache{
		snapshots: map[string]*reportstatus.Snapshot{
			"2": {Status: "processing", Stage: "scoring", UpdatedAt: time.Unix(99, 0).UTC()},
		},
	}, nil, nil, reportwait.DefaultConfig())
	svc := NewQueryService(&fakeEvaluationReader{}, wait)

	status, err := svc.GetReportStatus(context.Background(), 1, 2)
	if err != nil {
		t.Fatalf("GetReportStatus: %v", err)
	}
	if status.Status != "processing" || status.Stage != "scoring" {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestQueryServiceGetReturnsDetail(t *testing.T) {
	t.Parallel()

	svc := NewQueryService(&fakeEvaluationReader{
		detail: &evaluationapp.AssessmentDetailV2Response{
			ID:    "42",
			Model: evaluationapp.ModelIdentityResponse{Kind: personalityModelKind, Code: "mbti"},
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

	wait := reportwait.NewService(nil, &fakeStatusCache{
		snapshots: map[string]*reportstatus.Snapshot{
			"42": {Status: "interpreted", Stage: "done", UpdatedAt: time.Unix(1, 0).UTC()},
		},
	}, nil, nil, reportwait.DefaultConfig())
	svc := NewQueryService(&fakeEvaluationReader{
		detail: &evaluationapp.AssessmentDetailV2Response{
			ID:    "42",
			Model: evaluationapp.ModelIdentityResponse{Kind: personalityModelKind, Code: "sbti"},
			Level: &evaluationapp.ResultLevelResponse{Code: "INTJ"},
		},
	}, wait)

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

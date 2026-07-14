package reportwait

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
	appreportstatus "github.com/FangcunMount/qs-server/internal/collection-server/application/reportstatus"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeStatusCache struct {
	snapshots map[string]*reportstatus.Snapshot
	getErr    error
}

func (f *fakeStatusCache) Get(_ context.Context, assessmentID string) (*reportstatus.Snapshot, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
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

type fakeAssessmentQuery struct {
	result    *evaluation.AssessmentDetailResponse
	err       error
	report    *evaluation.AssessmentReportResponse
	reportErr error
}

func (f *fakeAssessmentQuery) GetMyAssessment(context.Context, uint64, uint64) (*evaluation.AssessmentDetailResponse, error) {
	return f.result, f.err
}

func (f *fakeAssessmentQuery) GetAssessmentReport(context.Context, uint64, uint64) (*evaluation.AssessmentReportResponse, error) {
	return f.report, f.reportErr
}

func TestToPublicAssessmentStatusMapsCompletedToInterpreted(t *testing.T) {
	got := appreportstatus.ToPublicAssessmentStatus(&evaluation.AssessmentStatusResponse{Status: "completed"})
	if got.Status != "interpreted" {
		t.Fatalf("expected interpreted, got %s", got.Status)
	}
}

func TestGetStatusRedisHitTerminal(t *testing.T) {
	svc := NewService(nil, &fakeStatusCache{
		snapshots: map[string]*reportstatus.Snapshot{
			"42": {
				AssessmentID: "42",
				Status:       "completed",
				Stage:        "completed",
				Message:      "报告已生成",
				UpdatedAt:    time.Now().UTC(),
			},
		},
	}, nil, nil, DefaultConfig())

	resp, err := svc.GetStatus(context.Background(), 1, 42)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if resp.Status != "completed" {
		t.Fatalf("expected internal completed, got %s", resp.Status)
	}
}

func TestGetStatusRedisMissDBFallbackInterpreted(t *testing.T) {
	svc := NewService(&fakeAssessmentQuery{
		result: &evaluation.AssessmentDetailResponse{Status: "interpreted"},
	}, &fakeStatusCache{snapshots: map[string]*reportstatus.Snapshot{}}, nil, nil, DefaultConfig())

	resp, err := svc.GetStatus(context.Background(), 1, 99)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if resp.Status != "completed" {
		t.Fatalf("expected completed, got %s", resp.Status)
	}
}

func TestGetStatusRedisMissPendingIncludesNextPoll(t *testing.T) {
	svc := NewService(&fakeAssessmentQuery{
		result: &evaluation.AssessmentDetailResponse{Status: "submitted"},
	}, &fakeStatusCache{snapshots: map[string]*reportstatus.Snapshot{}}, nil, nil, DefaultConfig())

	resp, err := svc.GetStatus(context.Background(), 1, 99)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if resp.NextPollAfterMs == 0 {
		t.Fatal("expected next_poll_after_ms for non-terminal status")
	}
}

func TestGetStatusRedisMissEvaluatedWithReportReturnsCompleted(t *testing.T) {
	svc := NewService(&fakeAssessmentQuery{
		result: &evaluation.AssessmentDetailResponse{Status: "evaluated"},
		report: &evaluation.AssessmentReportResponse{AssessmentID: "99"},
	}, &fakeStatusCache{snapshots: map[string]*reportstatus.Snapshot{}}, nil, nil, DefaultConfig())

	resp, err := svc.GetStatus(context.Background(), 1, 99)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if resp.Status != "completed" || resp.Stage != "completed" || resp.NextPollAfterMs != 0 {
		t.Fatalf("response = %#v", resp)
	}
}

func TestGetStatusRedisMissEvaluatedWithoutReportStaysInterpreting(t *testing.T) {
	svc := NewService(&fakeAssessmentQuery{
		result:    &evaluation.AssessmentDetailResponse{Status: "evaluated"},
		reportErr: status.Error(codes.NotFound, "report not found"),
	}, &fakeStatusCache{snapshots: map[string]*reportstatus.Snapshot{}}, nil, nil, DefaultConfig())

	resp, err := svc.GetStatus(context.Background(), 1, 99)
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if resp.Status != "processing" || resp.Stage != "interpreting" || resp.NextPollAfterMs != 2000 {
		t.Fatalf("response = %#v", resp)
	}
}

func TestGetStatusPropagatesQueryError(t *testing.T) {
	svc := NewService(&fakeAssessmentQuery{err: errors.New("db down")}, nil, nil, nil, DefaultConfig())
	if _, err := svc.GetStatus(context.Background(), 1, 1); err == nil {
		t.Fatal("expected error")
	}
}

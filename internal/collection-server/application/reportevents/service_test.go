package reportevents

import (
	"context"
	"errors"
	"testing"

	evaluationapp "github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
)

type fakeMedicalReader struct {
	result *evaluationapp.AssessmentDetailResponse
	err    error
}

func (f *fakeMedicalReader) GetMyAssessment(context.Context, uint64, uint64) (*evaluationapp.AssessmentDetailResponse, error) {
	return f.result, f.err
}

type fakeWaitReport struct {
	status *evaluationapp.AssessmentStatusResponse
	err    error
}

func (f *fakeWaitReport) GetStatus(context.Context, uint64, uint64) (*evaluationapp.AssessmentStatusResponse, error) {
	return f.status, f.err
}

func TestServiceAuthorizeMedical(t *testing.T) {
	svc := NewService(
		&fakeWaitReport{status: &evaluationapp.AssessmentStatusResponse{Status: "processing"}},
		&fakeMedicalReader{result: &evaluationapp.AssessmentDetailResponse{ID: "1"}},
		nil,
	)
	if err := svc.Authorize(context.Background(), KindMedical, 1, 2); err != nil {
		t.Fatalf("authorize: %v", err)
	}
}

func TestServiceAuthorizeMedicalDenied(t *testing.T) {
	svc := NewService(nil, &fakeMedicalReader{result: nil}, nil)
	if err := svc.Authorize(context.Background(), KindMedical, 1, 2); !errors.Is(err, ErrAssessmentAccess) {
		t.Fatalf("expected access denied, got %v", err)
	}
}

func TestServiceCurrentStatusMedical(t *testing.T) {
	svc := NewService(
		&fakeWaitReport{status: &evaluationapp.AssessmentStatusResponse{
			Status:          "completed",
			Stage:           "completed",
			NextPollAfterMs: 0,
			UpdatedAt:       1,
		}},
		&fakeMedicalReader{result: &evaluationapp.AssessmentDetailResponse{ID: "1"}},
		nil,
	)
	payload, err := svc.CurrentStatus(context.Background(), KindMedical, 1, 2)
	if err != nil {
		t.Fatalf("current status: %v", err)
	}
	if payload.Status != "interpreted" {
		t.Fatalf("expected interpreted, got %q", payload.Status)
	}
}

func TestServiceInvalidKind(t *testing.T) {
	svc := NewService(&fakeWaitReport{}, &fakeMedicalReader{}, nil)
	if err := svc.Authorize(context.Background(), "unknown", 1, 2); !errors.Is(err, ErrInvalidKind) {
		t.Fatalf("expected invalid kind, got %v", err)
	}
}

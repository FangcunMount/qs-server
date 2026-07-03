package reportevents

import (
	"context"
	"errors"
	"testing"

	evaluationapp "github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportstatus"
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

func newTestResolver(wait *fakeWaitReport, medical *fakeMedicalReader) *reportstatus.Resolver {
	return reportstatus.NewResolver(map[string]reportstatus.KindReader{
		reportstatus.KindMedical: medicalKindReaderTest{
			medical:    medical,
			waitReport: wait,
		},
	})
}

type medicalKindReaderTest struct {
	medical    *fakeMedicalReader
	waitReport *fakeWaitReport
}

func (m medicalKindReaderTest) Authorize(ctx context.Context, testeeID, assessmentID uint64) error {
	if m.medical == nil {
		return errors.New("medical query service is not configured")
	}
	result, err := m.medical.GetMyAssessment(ctx, testeeID, assessmentID)
	if err != nil {
		return err
	}
	if result == nil {
		return reportstatus.ErrAssessmentAccess
	}
	return nil
}

func (m medicalKindReaderTest) CurrentStatus(ctx context.Context, testeeID, assessmentID uint64) (*reportstatus.View, error) {
	if m.waitReport == nil {
		return nil, errors.New("wait-report service is not configured")
	}
	status, err := m.waitReport.GetStatus(ctx, testeeID, assessmentID)
	if err != nil {
		return nil, err
	}
	return reportstatus.MedicalView(reportstatus.ToPublicAssessmentStatus(status)), nil
}

func newTestService(wait *fakeWaitReport, medical *fakeMedicalReader) *Service {
	return NewService(newTestResolver(wait, medical))
}

func TestServiceAuthorizeMedical(t *testing.T) {
	svc := newTestService(
		&fakeWaitReport{status: &evaluationapp.AssessmentStatusResponse{Status: "processing"}},
		&fakeMedicalReader{result: &evaluationapp.AssessmentDetailResponse{ID: "1"}},
	)
	if err := svc.Authorize(context.Background(), reportstatus.KindMedical, 1, 2); err != nil {
		t.Fatalf("authorize: %v", err)
	}
}

func TestServiceAuthorizeMedicalDenied(t *testing.T) {
	svc := newTestService(nil, &fakeMedicalReader{result: nil})
	if err := svc.Authorize(context.Background(), reportstatus.KindMedical, 1, 2); !errors.Is(err, reportstatus.ErrAssessmentAccess) {
		t.Fatalf("expected access denied, got %v", err)
	}
}

func TestServiceCurrentStatusMedical(t *testing.T) {
	svc := newTestService(
		&fakeWaitReport{status: &evaluationapp.AssessmentStatusResponse{
			Status:          "completed",
			Stage:           "completed",
			NextPollAfterMs: 0,
			UpdatedAt:       1,
		}},
		&fakeMedicalReader{result: &evaluationapp.AssessmentDetailResponse{ID: "1"}},
	)
	payload, err := svc.CurrentStatus(context.Background(), reportstatus.KindMedical, 1, 2)
	if err != nil {
		t.Fatalf("current status: %v", err)
	}
	if payload.Status != "interpreted" {
		t.Fatalf("expected interpreted, got %q", payload.Status)
	}
}

func TestServiceInvalidKind(t *testing.T) {
	svc := newTestService(&fakeWaitReport{}, &fakeMedicalReader{})
	if err := svc.Authorize(context.Background(), "unknown", 1, 2); !errors.Is(err, reportstatus.ErrInvalidKind) {
		t.Fatalf("expected invalid kind, got %v", err)
	}
}

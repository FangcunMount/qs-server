package answersheet

import (
	"context"
	"errors"
	"testing"
)

type stubSubmitAssessmentResolver struct {
	testeeID     uint64
	assessmentID uint64
	err          error
	calls        int
}

func (s *stubSubmitAssessmentResolver) ResolveAssessmentByAnswerSheetID(context.Context, uint64) (uint64, uint64, error) {
	s.calls++
	return s.testeeID, s.assessmentID, s.err
}

func TestGetSubmitStatusEnrichesAssessmentIDWhenReady(t *testing.T) {
	t.Parallel()

	resolver := &stubSubmitAssessmentResolver{assessmentID: 9001}
	service := &SubmissionService{
		queue: &SubmitQueue{
			statuses: newSubmitQueueStatusStore(0),
		},
		assessmentResolver: resolver,
	}
	service.queue.setStatus("req-1", SubmitStatusDone, "42")

	status, ok := service.GetSubmitStatus(context.Background(), "req-1")
	if !ok || status == nil {
		t.Fatal("expected submit status")
	}
	if status.AssessmentID != "9001" {
		t.Fatalf("assessment_id = %q, want 9001", status.AssessmentID)
	}
	if resolver.calls != 1 {
		t.Fatalf("resolver calls = %d, want 1", resolver.calls)
	}
}

func TestGetSubmitStatusSkipsResolverUntilDone(t *testing.T) {
	t.Parallel()

	resolver := &stubSubmitAssessmentResolver{assessmentID: 9001}
	service := &SubmissionService{
		queue: &SubmitQueue{
			statuses: newSubmitQueueStatusStore(0),
		},
		assessmentResolver: resolver,
	}
	service.queue.setStatus("req-1", SubmitStatusProcessing, "42")

	status, ok := service.GetSubmitStatus(context.Background(), "req-1")
	if !ok || status == nil || status.AssessmentID != "" {
		t.Fatalf("unexpected status: %+v, ok=%v", status, ok)
	}
	if resolver.calls != 0 {
		t.Fatalf("resolver calls = %d, want 0", resolver.calls)
	}
}

func TestGetSubmitStatusOmitsAssessmentIDWhenNotReady(t *testing.T) {
	t.Parallel()

	resolver := &stubSubmitAssessmentResolver{err: errors.New("not found")}
	service := &SubmissionService{
		queue: &SubmitQueue{
			statuses: newSubmitQueueStatusStore(0),
		},
		assessmentResolver: resolver,
	}
	service.queue.setStatus("req-1", SubmitStatusDone, "42")

	status, ok := service.GetSubmitStatus(context.Background(), "req-1")
	if !ok || status == nil || status.AssessmentID != "" {
		t.Fatalf("expected done without assessment_id, got %+v", status)
	}
}

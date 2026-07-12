package answersheet

import (
	"context"
	"testing"
)

func TestSubmitStatusDonePreservesAssessmentID(t *testing.T) {
	t.Parallel()
	queue := &SubmitQueue{statuses: newSubmitQueueStatusStore(0)}
	queue.setStatus("req-1", SubmitStatusProcessing, "")
	queue.setAssessmentID("req-1", "9001")
	queue.setStatus("req-1", SubmitStatusDone, "42")
	service := &SubmissionService{queue: queue}
	status, ok := service.GetSubmitStatus(context.Background(), "req-1")
	if !ok || status == nil {
		t.Fatal("expected submit status")
	}
	if status.AnswerSheetID != "42" || status.AssessmentID != "9001" {
		t.Fatalf("done status = %+v, want both ids", status)
	}
}

func TestProcessingStatusDoesNotInventAssessmentID(t *testing.T) {
	t.Parallel()
	queue := &SubmitQueue{statuses: newSubmitQueueStatusStore(0)}
	queue.setStatus("req-1", SubmitStatusProcessing, "")
	status, ok := (&SubmissionService{queue: queue}).GetSubmitStatus(context.Background(), "req-1")
	if !ok || status == nil || status.AssessmentID != "" {
		t.Fatalf("unexpected status: %+v", status)
	}
}

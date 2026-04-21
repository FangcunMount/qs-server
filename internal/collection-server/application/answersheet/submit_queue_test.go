package answersheet

import (
	"context"
	"testing"
	"time"
)

func TestSubmitQueueEnqueueReturnsImmediately(t *testing.T) {
	release := make(chan struct{})
	q := NewSubmitQueue(1, 1, func(context.Context, string, uint64, *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error) {
		<-release
		return &SubmitAnswerSheetResponse{ID: "42"}, nil
	})
	if q == nil {
		t.Fatal("expected queue to be initialized")
	}

	start := time.Now()
	if err := q.Enqueue(context.Background(), "req-1", 1, &SubmitAnswerSheetRequest{}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("enqueue should return immediately, took %s", elapsed)
	}

	status, ok := q.GetStatus("req-1")
	if !ok {
		t.Fatal("expected status to be recorded")
	}
	if status.Status != SubmitStatusQueued && status.Status != SubmitStatusProcessing {
		t.Fatalf("expected queued or processing status, got %q", status.Status)
	}

	close(release)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		status, ok = q.GetStatus("req-1")
		if ok && status.Status == SubmitStatusDone && status.AnswerSheetID == "42" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("expected done status, got %+v", status)
}

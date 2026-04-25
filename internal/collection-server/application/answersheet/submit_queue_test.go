package answersheet

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
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

func TestSubmitQueueReturnsFullWhenBufferIsExhausted(t *testing.T) {
	release := make(chan struct{})
	q := NewSubmitQueue(1, 1, func(context.Context, string, uint64, *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error) {
		<-release
		return &SubmitAnswerSheetResponse{ID: "42"}, nil
	})
	t.Cleanup(func() { close(release) })

	if err := q.Enqueue(context.Background(), "req-1", 1, &SubmitAnswerSheetRequest{}); err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		status, ok := q.GetStatus("req-1")
		if ok && status.Status == SubmitStatusProcessing {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("req-1 did not enter processing")
		}
		time.Sleep(time.Millisecond)
	}
	if err := q.Enqueue(context.Background(), "req-2", 1, &SubmitAnswerSheetRequest{}); err != nil {
		t.Fatalf("second enqueue should fill buffer: %v", err)
	}
	if err := q.Enqueue(context.Background(), "req-3", 1, &SubmitAnswerSheetRequest{}); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("third enqueue error = %v, want ErrQueueFull", err)
	}
}

func TestSubmitQueueDuplicateRequestReusesInFlightStatus(t *testing.T) {
	release := make(chan struct{})
	q := NewSubmitQueue(1, 1, func(context.Context, string, uint64, *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error) {
		<-release
		return &SubmitAnswerSheetResponse{ID: "42"}, nil
	})
	t.Cleanup(func() { close(release) })

	if err := q.Enqueue(context.Background(), "req-duplicate", 1, &SubmitAnswerSheetRequest{}); err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if err := q.Enqueue(context.Background(), "req-duplicate", 1, &SubmitAnswerSheetRequest{}); err != nil {
		t.Fatalf("duplicate enqueue should reuse status: %v", err)
	}
}

func TestSubmitQueueFailedRequestRequiresNewRequestID(t *testing.T) {
	q := NewSubmitQueue(1, 1, func(context.Context, string, uint64, *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error) {
		return nil, errors.New("boom")
	})

	if err := q.Enqueue(context.Background(), "req-failed", 1, &SubmitAnswerSheetRequest{}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		status, ok := q.GetStatus("req-failed")
		if ok && status.Status == SubmitStatusFailed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("req-failed did not fail")
		}
		time.Sleep(time.Millisecond)
	}
	if err := q.Enqueue(context.Background(), "req-failed", 1, &SubmitAnswerSheetRequest{}); err == nil {
		t.Fatal("expected failed request id reuse to be rejected")
	}
}

func TestSubmitQueueStatusExpiresAfterTTL(t *testing.T) {
	q := NewSubmitQueue(1, 1, func(context.Context, string, uint64, *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error) {
		return &SubmitAnswerSheetResponse{ID: "42"}, nil
	})
	q.statuses.statusTTL = time.Millisecond

	q.statuses.mu.Lock()
	q.statuses.statuses["expired"] = SubmitStatusResponse{
		Status:    SubmitStatusDone,
		UpdatedAt: time.Now().Add(-time.Second).Unix(),
	}
	q.statuses.mu.Unlock()

	if _, ok := q.GetStatus("expired"); ok {
		t.Fatal("expected expired status to be removed")
	}
}

func TestSubmitQueueReportsOutcomes(t *testing.T) {
	observer := &submitQueueRecordingObserver{}
	q := NewSubmitQueueWithOptions(1, 1, func(context.Context, string, uint64, *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error) {
		return &SubmitAnswerSheetResponse{ID: "42"}, nil
	}, SubmitQueueRuntimeOptions{Observer: observer})

	if err := q.Enqueue(context.Background(), "req-observed", 1, &SubmitAnswerSheetRequest{}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	deadline := time.Now().Add(time.Second)
	for {
		status, ok := q.GetStatus("req-observed")
		if ok && status.Status == SubmitStatusDone {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("req-observed did not finish")
		}
		time.Sleep(time.Millisecond)
	}

	for _, outcome := range []resilienceplane.Outcome{
		resilienceplane.OutcomeQueueAccepted,
		resilienceplane.OutcomeQueueProcessing,
		resilienceplane.OutcomeQueueDone,
	} {
		if !observer.has(outcome) {
			t.Fatalf("expected outcome %s", outcome)
		}
	}
}

type submitQueueRecordingObserver struct {
	mu        sync.Mutex
	decisions []resilienceplane.Decision
}

func (r *submitQueueRecordingObserver) ObserveDecision(_ context.Context, decision resilienceplane.Decision) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.decisions = append(r.decisions, decision)
}

func (r *submitQueueRecordingObserver) has(outcome resilienceplane.Outcome) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, decision := range r.decisions {
		if decision.Outcome == outcome {
			return true
		}
	}
	return false
}

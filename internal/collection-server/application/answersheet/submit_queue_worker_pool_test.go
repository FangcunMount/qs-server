package answersheet

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSubmitQueueWorkerPoolWritesSuccessStatusInOrder(t *testing.T) {
	jobs := make(chan submitJob, 1)
	t.Cleanup(func() { close(jobs) })

	events := make(chan submitQueueStatusEvent, 2)
	pool := newSubmitQueueWorkerPool(
		1,
		jobs,
		func(context.Context, string, uint64, *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error) {
			return &SubmitAnswerSheetResponse{ID: "42"}, nil
		},
		func(requestID, status, answerSheetID string) {
			events <- submitQueueStatusEvent{requestID: requestID, status: status, answerSheetID: answerSheetID}
		},
	)

	pool.Start()
	jobs <- submitJob{ctx: context.Background(), requestID: "req-1"}

	first := waitSubmitQueueStatusEvent(t, events)
	second := waitSubmitQueueStatusEvent(t, events)
	if first.status != SubmitStatusProcessing {
		t.Fatalf("first status = %q, want %q", first.status, SubmitStatusProcessing)
	}
	if second.status != SubmitStatusDone || second.answerSheetID != "42" {
		t.Fatalf("second event = %+v, want done with answerSheetID 42", second)
	}
}

func TestSubmitQueueWorkerPoolWritesFailedStatusOnSubmitError(t *testing.T) {
	jobs := make(chan submitJob, 1)
	t.Cleanup(func() { close(jobs) })

	events := make(chan submitQueueStatusEvent, 2)
	pool := newSubmitQueueWorkerPool(
		1,
		jobs,
		func(context.Context, string, uint64, *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error) {
			return nil, errors.New("boom")
		},
		func(requestID, status, answerSheetID string) {
			events <- submitQueueStatusEvent{requestID: requestID, status: status, answerSheetID: answerSheetID}
		},
	)

	pool.Start()
	jobs <- submitJob{ctx: context.Background(), requestID: "req-1"}

	first := waitSubmitQueueStatusEvent(t, events)
	second := waitSubmitQueueStatusEvent(t, events)
	if first.status != SubmitStatusProcessing {
		t.Fatalf("first status = %q, want %q", first.status, SubmitStatusProcessing)
	}
	if second.status != SubmitStatusFailed {
		t.Fatalf("second status = %q, want %q", second.status, SubmitStatusFailed)
	}
}

func TestSubmitQueueWorkerPoolRejectsInvalidInput(t *testing.T) {
	if got := newSubmitQueueWorkerPool(0, make(chan submitJob), func(context.Context, string, uint64, *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error) {
		return nil, nil
	}, func(string, string, string) {}); got != nil {
		t.Fatalf("pool = %#v, want nil", got)
	}
	if got := newSubmitQueueWorkerPool(1, nil, func(context.Context, string, uint64, *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error) {
		return nil, nil
	}, func(string, string, string) {}); got != nil {
		t.Fatalf("pool = %#v, want nil", got)
	}
	if got := newSubmitQueueWorkerPool(1, make(chan submitJob), nil, func(string, string, string) {}); got != nil {
		t.Fatalf("pool = %#v, want nil", got)
	}
	if got := newSubmitQueueWorkerPool(1, make(chan submitJob), func(context.Context, string, uint64, *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error) {
		return nil, nil
	}, nil); got != nil {
		t.Fatalf("pool = %#v, want nil", got)
	}
}

type submitQueueStatusEvent struct {
	requestID     string
	status        string
	answerSheetID string
}

func waitSubmitQueueStatusEvent(t *testing.T, events <-chan submitQueueStatusEvent) submitQueueStatusEvent {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for status event")
		return submitQueueStatusEvent{}
	}
}

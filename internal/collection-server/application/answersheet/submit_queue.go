package answersheet

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrQueueFull indicates the submit queue is full.
var ErrQueueFull = errors.New("submit queue full")

type submitJob struct {
	ctx       context.Context
	requestID string
	writerID  uint64
	req       *SubmitAnswerSheetRequest
}

type submitFunc func(context.Context, string, uint64, *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error)

// SubmitQueue queues submit requests for asynchronous processing.
type SubmitQueue struct {
	jobs        chan submitJob
	submit      submitFunc
	statusTTL   time.Duration
	mu          sync.Mutex
	statuses    map[string]SubmitStatusResponse
	lastCleanup time.Time
}

// NewSubmitQueue creates a submit queue with worker goroutines.
func NewSubmitQueue(workerCount, queueSize int, submit submitFunc) *SubmitQueue {
	if workerCount <= 0 || queueSize <= 0 || submit == nil {
		return nil
	}

	q := &SubmitQueue{
		jobs:      make(chan submitJob, queueSize),
		submit:    submit,
		statusTTL: 10 * time.Minute,
		statuses:  make(map[string]SubmitStatusResponse),
	}

	for i := 0; i < workerCount; i++ {
		go q.worker()
	}

	return q
}

// Enqueue accepts a submit request for asynchronous processing.
func (q *SubmitQueue) Enqueue(ctx context.Context, requestID string, writerID uint64, req *SubmitAnswerSheetRequest) error {
	if q == nil {
		return errors.New("submit queue disabled")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if requestID == "" {
		return errors.New("request id is required")
	}

	// 幂等：若已有状态，直接复用现有状态。
	if status, ok := q.getStatus(requestID); ok {
		switch status.Status {
		case SubmitStatusDone:
			return nil
		case SubmitStatusQueued, SubmitStatusProcessing:
			return nil
		case SubmitStatusFailed:
			return errors.New("previous request failed, please retry with a new request_id")
		}
	}

	job := submitJob{
		ctx:       context.WithoutCancel(ctx),
		requestID: requestID,
		writerID:  writerID,
		req:       req,
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	select {
	case q.jobs <- job:
		q.setStatus(requestID, SubmitStatusQueued, "")
	default:
		return ErrQueueFull
	}

	return nil
}

func (q *SubmitQueue) worker() {
	for job := range q.jobs {
		q.setStatus(job.requestID, SubmitStatusProcessing, "")
		resp, err := q.submit(job.ctx, job.requestID, job.writerID, job.req)
		if err != nil {
			q.setStatus(job.requestID, SubmitStatusFailed, "")
		} else if resp != nil {
			q.setStatus(job.requestID, SubmitStatusDone, resp.ID)
		}
	}
}

// GetStatus returns submit status for a request ID.
func (q *SubmitQueue) GetStatus(requestID string) (SubmitStatusResponse, bool) {
	if q == nil || requestID == "" {
		return SubmitStatusResponse{}, false
	}

	q.cleanup()
	q.mu.Lock()
	defer q.mu.Unlock()
	status, ok := q.statuses[requestID]
	return status, ok
}

const (
	SubmitStatusQueued     = "queued"
	SubmitStatusProcessing = "processing"
	SubmitStatusDone       = "done"
	SubmitStatusFailed     = "failed"
)

func (q *SubmitQueue) setStatus(requestID, status, answerSheetID string) {
	if requestID == "" {
		return
	}

	q.cleanup()
	q.mu.Lock()
	q.statuses[requestID] = SubmitStatusResponse{
		Status:        status,
		AnswerSheetID: answerSheetID,
		UpdatedAt:     time.Now().Unix(),
	}
	q.mu.Unlock()
}

func (q *SubmitQueue) cleanup() {
	now := time.Now()
	q.mu.Lock()
	defer q.mu.Unlock()
	if now.Sub(q.lastCleanup) < time.Minute {
		return
	}
	for key, status := range q.statuses {
		if now.Sub(time.Unix(status.UpdatedAt, 0)) > q.statusTTL {
			delete(q.statuses, key)
		}
	}
	q.lastCleanup = now
}

func (q *SubmitQueue) getStatus(requestID string) (SubmitStatusResponse, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	status, ok := q.statuses[requestID]
	return status, ok
}

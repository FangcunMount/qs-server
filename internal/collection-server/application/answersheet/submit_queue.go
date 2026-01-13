package answersheet

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrQueueFull indicates the submit queue is full.
var ErrQueueFull = errors.New("submit queue full")

type submitResult struct {
	resp *SubmitAnswerSheetResponse
	err  error
}

type submitJob struct {
	ctx       context.Context
	requestID string
	writerID  uint64
	req       *SubmitAnswerSheetRequest
	respCh    chan submitResult
}

type submitFunc func(context.Context, uint64, *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error)

// SubmitQueue queues submit requests for asynchronous processing.
type SubmitQueue struct {
	jobs        chan submitJob
	waitTimeout time.Duration
	submit      submitFunc
	statusTTL   time.Duration
	mu          sync.Mutex
	statuses    map[string]SubmitStatusResponse
	lastCleanup time.Time
}

// NewSubmitQueue creates a submit queue with worker goroutines.
func NewSubmitQueue(workerCount, queueSize int, waitTimeout time.Duration, submit submitFunc) *SubmitQueue {
	if workerCount <= 0 || queueSize <= 0 || submit == nil {
		return nil
	}

	q := &SubmitQueue{
		jobs:        make(chan submitJob, queueSize),
		waitTimeout: waitTimeout,
		submit:      submit,
		statusTTL:   10 * time.Minute,
		statuses:    make(map[string]SubmitStatusResponse),
	}

	for i := 0; i < workerCount; i++ {
		go q.worker()
	}

	return q
}

// Enqueue submits a job and waits for a short time for the result.
// It returns queued=true when the job is accepted but not finished in time.
func (q *SubmitQueue) Enqueue(ctx context.Context, requestID string, writerID uint64, req *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, bool, error) {
	if q == nil {
		return nil, false, errors.New("submit queue disabled")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if requestID == "" {
		return nil, false, errors.New("request id is required")
	}

	// 幂等：若已有状态，直接返回或告知排队中
	if status, ok := q.getStatus(requestID); ok {
		switch status.Status {
		case SubmitStatusDone:
			// 已完成，直接返回结果
			return &SubmitAnswerSheetResponse{
				ID:      status.AnswerSheetID,
				Message: "already submitted",
			}, false, nil
		case SubmitStatusQueued, SubmitStatusProcessing:
			// 还在队列/处理中，提示客户端等待
			return nil, true, nil
		case SubmitStatusFailed:
			// 已失败，不重复入队，由客户端决定是否换 request_id 重试
			return nil, false, errors.New("previous request failed, please retry with a new request_id")
		}
	}

	respCh := make(chan submitResult, 1)
	job := submitJob{
		ctx:       context.WithoutCancel(ctx),
		requestID: requestID,
		writerID:  writerID,
		req:       req,
		respCh:    respCh,
	}

	select {
	case q.jobs <- job:
		q.setStatus(requestID, SubmitStatusQueued, "")
	default:
		return nil, false, ErrQueueFull
	}

	if q.waitTimeout <= 0 {
		return nil, true, nil
	}

	timer := time.NewTimer(q.waitTimeout)
	defer timer.Stop()

	select {
	case result := <-respCh:
		return result.resp, false, result.err
	case <-timer.C:
		return nil, true, nil
	case <-ctx.Done():
		return nil, false, ctx.Err()
	}
}

func (q *SubmitQueue) worker() {
	for job := range q.jobs {
		q.setStatus(job.requestID, SubmitStatusProcessing, "")
		resp, err := q.submit(job.ctx, job.writerID, job.req)
		if err != nil {
			q.setStatus(job.requestID, SubmitStatusFailed, "")
		} else if resp != nil {
			q.setStatus(job.requestID, SubmitStatusDone, resp.ID)
		}
		job.respCh <- submitResult{resp: resp, err: err}
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

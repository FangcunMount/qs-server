package answersheet

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
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
	jobs     chan submitJob
	submit   submitFunc
	statuses *submitQueueStatusStore
	observer resilienceplane.Observer
	subject  resilienceplane.Subject
}

type SubmitQueueRuntimeOptions struct {
	Component string
	Name      string
	Observer  resilienceplane.Observer
}

// NewSubmitQueue creates a submit queue with worker goroutines.
func NewSubmitQueue(workerCount, queueSize int, submit submitFunc) *SubmitQueue {
	return NewSubmitQueueWithOptions(workerCount, queueSize, submit, SubmitQueueRuntimeOptions{
		Component: "collection-server",
		Name:      "answersheet_submit",
	})
}

func NewSubmitQueueWithOptions(workerCount, queueSize int, submit submitFunc, opts SubmitQueueRuntimeOptions) *SubmitQueue {
	if workerCount <= 0 || queueSize <= 0 || submit == nil {
		return nil
	}

	q := &SubmitQueue{
		jobs:     make(chan submitJob, queueSize),
		submit:   submit,
		statuses: newSubmitQueueStatusStore(10 * time.Minute),
		observer: defaultSubmitQueueObserver(opts.Observer),
		subject: resilienceplane.Subject{
			Component: opts.Component,
			Scope:     opts.Name,
			Resource:  "submit_queue",
			Strategy:  "memory_channel",
		},
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
			q.observe(ctx, resilienceplane.OutcomeQueueDuplicate)
			return nil
		case SubmitStatusQueued, SubmitStatusProcessing:
			q.observe(ctx, resilienceplane.OutcomeQueueDuplicate)
			return nil
		case SubmitStatusFailed:
			q.observe(ctx, resilienceplane.OutcomeQueueFailed)
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
		q.observe(ctx, resilienceplane.OutcomeQueueAccepted)
	default:
		q.observe(ctx, resilienceplane.OutcomeQueueFull)
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

	return q.statuses.Get(requestID)
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

	q.statuses.Set(requestID, SubmitStatusResponse{
		Status:        status,
		AnswerSheetID: answerSheetID,
		UpdatedAt:     time.Now().Unix(),
	})

	switch status {
	case SubmitStatusProcessing:
		q.observe(context.Background(), resilienceplane.OutcomeQueueProcessing)
	case SubmitStatusDone:
		q.observe(context.Background(), resilienceplane.OutcomeQueueDone)
	case SubmitStatusFailed:
		q.observe(context.Background(), resilienceplane.OutcomeQueueFailed)
	}
}

func (q *SubmitQueue) getStatus(requestID string) (SubmitStatusResponse, bool) {
	if q == nil || q.statuses == nil {
		return SubmitStatusResponse{}, false
	}
	return q.statuses.GetFresh(requestID)
}

func (q *SubmitQueue) observe(ctx context.Context, outcome resilienceplane.Outcome) {
	if q == nil {
		return
	}
	resilienceplane.Observe(ctx, q.observer, resilienceplane.ProtectionQueue, q.subject, outcome)
}

func defaultSubmitQueueObserver(observer resilienceplane.Observer) resilienceplane.Observer {
	if observer != nil {
		return observer
	}
	return resilienceplane.DefaultObserver()
}

type submitQueueStatusStore struct {
	statusTTL   time.Duration
	mu          sync.Mutex
	statuses    map[string]SubmitStatusResponse
	lastCleanup time.Time
}

func newSubmitQueueStatusStore(statusTTL time.Duration) *submitQueueStatusStore {
	return &submitQueueStatusStore{
		statusTTL: statusTTL,
		statuses:  make(map[string]SubmitStatusResponse),
	}
}

func (s *submitQueueStatusStore) Set(requestID string, status SubmitStatusResponse) {
	if s == nil || requestID == "" {
		return
	}
	s.cleanup()
	s.mu.Lock()
	s.statuses[requestID] = status
	s.mu.Unlock()
}

func (s *submitQueueStatusStore) Get(requestID string) (SubmitStatusResponse, bool) {
	if s == nil || requestID == "" {
		return SubmitStatusResponse{}, false
	}
	s.cleanup()
	s.mu.Lock()
	defer s.mu.Unlock()
	status, ok := s.statuses[requestID]
	return status, ok
}

func (s *submitQueueStatusStore) GetFresh(requestID string) (SubmitStatusResponse, bool) {
	if s == nil || requestID == "" {
		return SubmitStatusResponse{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	status, ok := s.statuses[requestID]
	return status, ok
}

func (s *submitQueueStatusStore) cleanup() {
	if s == nil {
		return
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if now.Sub(s.lastCleanup) < time.Minute {
		return
	}
	for key, status := range s.statuses {
		if now.Sub(time.Unix(status.UpdatedAt, 0)) > s.statusTTL {
			delete(s.statuses, key)
		}
	}
	s.lastCleanup = now
}

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
	jobs       chan submitJob
	statuses   *submitQueueStatusStore
	workerPool *submitQueueWorkerPool
	observer   resilienceplane.Observer
	subject    resilienceplane.Subject
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
		statuses: newSubmitQueueStatusStore(10 * time.Minute),
		observer: defaultSubmitQueueObserver(opts.Observer),
		subject: resilienceplane.Subject{
			Component: opts.Component,
			Scope:     opts.Name,
			Resource:  "submit_queue",
			Strategy:  "memory_channel",
		},
	}
	q.workerPool = newSubmitQueueWorkerPool(workerCount, q.jobs, submit, q.setStatus)
	q.workerPool.Start()

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
		q.observeQueueDepth()
	default:
		q.observe(ctx, resilienceplane.OutcomeQueueFull)
		q.observeQueueDepth()
		return ErrQueueFull
	}

	return nil
}

// GetStatus returns submit status for a request ID.
func (q *SubmitQueue) GetStatus(requestID string) (SubmitStatusResponse, bool) {
	if q == nil || requestID == "" {
		return SubmitStatusResponse{}, false
	}

	status, ok, cleaned := q.statuses.Get(requestID)
	q.observeCleaned(context.Background(), cleaned)
	q.observeQueueStatusCounts()
	return status, ok
}

func (q *SubmitQueue) StatusSnapshot(now time.Time) resilienceplane.QueueSnapshot {
	if now.IsZero() {
		now = time.Now()
	}
	if q == nil {
		return resilienceplane.QueueSnapshot{
			GeneratedAt:       now,
			Component:         "collection-server",
			Name:              "answersheet_submit",
			Strategy:          "memory_channel",
			LifecycleBoundary: "process_memory_no_drain",
		}
	}
	counts, cleaned := q.statuses.Snapshot(now)
	q.observeCleaned(context.Background(), cleaned)
	snapshot := resilienceplane.QueueSnapshot{
		GeneratedAt:       now,
		Component:         q.subject.Component,
		Name:              q.subject.Scope,
		Strategy:          q.subject.Strategy,
		Depth:             len(q.jobs),
		Capacity:          cap(q.jobs),
		StatusTTLSeconds:  int64(q.statuses.statusTTL.Seconds()),
		StatusCounts:      counts,
		LifecycleBoundary: "process_memory_no_drain",
	}
	q.observeQueueSnapshot(snapshot)
	return snapshot
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

	cleaned := q.statuses.Set(requestID, SubmitStatusResponse{
		Status:        status,
		AnswerSheetID: answerSheetID,
		UpdatedAt:     time.Now().Unix(),
	})
	q.observeCleaned(context.Background(), cleaned)
	q.observeQueueDepth()
	q.observeQueueStatusCounts()

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

func (q *SubmitQueue) observeCleaned(ctx context.Context, count int) {
	if q == nil || count <= 0 {
		return
	}
	resilienceplane.Observe(ctx, q.observer, resilienceplane.ProtectionQueue, q.subject, resilienceplane.OutcomeQueueStatusCleaned)
}

func (q *SubmitQueue) observeQueueDepth() {
	if q == nil {
		return
	}
	resilienceplane.ObserveQueueDepth(q.subject, len(q.jobs))
}

func (q *SubmitQueue) observeQueueStatusCounts() {
	if q == nil || q.statuses == nil {
		return
	}
	for status, count := range q.statuses.Counts() {
		resilienceplane.ObserveQueueStatus(q.subject, status, count)
	}
}

func (q *SubmitQueue) observeQueueSnapshot(snapshot resilienceplane.QueueSnapshot) {
	if q == nil {
		return
	}
	resilienceplane.ObserveQueueDepth(q.subject, snapshot.Depth)
	for status, count := range snapshot.StatusCounts {
		resilienceplane.ObserveQueueStatus(q.subject, status, count)
	}
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

func (s *submitQueueStatusStore) Set(requestID string, status SubmitStatusResponse) int {
	if s == nil || requestID == "" {
		return 0
	}
	cleaned := s.cleanup()
	s.mu.Lock()
	s.statuses[requestID] = status
	s.mu.Unlock()
	return cleaned
}

func (s *submitQueueStatusStore) Get(requestID string) (SubmitStatusResponse, bool, int) {
	if s == nil || requestID == "" {
		return SubmitStatusResponse{}, false, 0
	}
	cleaned := s.cleanup()
	s.mu.Lock()
	defer s.mu.Unlock()
	status, ok := s.statuses[requestID]
	return status, ok, cleaned
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

func (s *submitQueueStatusStore) Snapshot(now time.Time) (map[string]int, int) {
	if s == nil {
		return map[string]int{}, 0
	}
	cleaned := s.cleanupAt(now)
	return s.Counts(), cleaned
}

func (s *submitQueueStatusStore) Counts() map[string]int {
	counts := map[string]int{
		SubmitStatusQueued:     0,
		SubmitStatusProcessing: 0,
		SubmitStatusDone:       0,
		SubmitStatusFailed:     0,
	}
	if s == nil {
		return counts
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, status := range s.statuses {
		counts[status.Status]++
	}
	return counts
}

func (s *submitQueueStatusStore) cleanup() int {
	return s.cleanupAt(time.Now())
}

func (s *submitQueueStatusStore) cleanupAt(now time.Time) int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if now.Sub(s.lastCleanup) < time.Minute {
		return 0
	}
	cleaned := 0
	for key, status := range s.statuses {
		if now.Sub(time.Unix(status.UpdatedAt, 0)) > s.statusTTL {
			delete(s.statuses, key)
			cleaned++
		}
	}
	s.lastCleanup = now
	return cleaned
}

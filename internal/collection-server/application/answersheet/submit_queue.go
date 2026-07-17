package answersheet

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/resiliencecontrol"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
)

// ErrQueueFull indicates the submit queue is full.
var ErrQueueFull = errors.New("submit queue full")

// ErrQueueDraining indicates governance has closed queue admission.
var ErrQueueDraining = errors.New("submit queue draining")

type submitJob struct {
	ctx       context.Context
	requestID string
	writerID  uint64
	req       *SubmitAnswerSheetRequest
}

type submitFunc func(context.Context, string, uint64, *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error)

// SubmitQueue queues submit requests for asynchronous processing.
type SubmitQueue struct {
	jobs         chan submitJob
	statuses     *submitQueueStatusStore
	workerPool   *submitQueueWorkerPool
	observer     resilienceplane.Observer
	subject      resilienceplane.Subject
	lifecycleMu  sync.Mutex
	state        resiliencecontrol.QueueState
	stateVersion uint64
	inFlight     atomic.Int64
	outstanding  atomic.Int64
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
		state:        resiliencecontrol.QueueStateActive,
		stateVersion: 1,
	}
	q.workerPool = newSubmitQueueWorkerPool(workerCount, q.jobs, submit, q.setStatus, q.setFailed)
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

	q.lifecycleMu.Lock()
	admission, cleaned := q.admit(job, q.state != "" && q.state != resiliencecontrol.QueueStateActive)
	q.lifecycleMu.Unlock()
	q.observeCleaned(ctx, cleaned)
	switch admission {
	case submitQueueAdmissionAccepted:
		q.observe(ctx, resilienceplane.OutcomeQueueAccepted)
		q.observeQueueDepth()
		logger.L(ctx).Infow("答卷提交请求已进入处理队列",
			"action", "enqueue_answersheet_submit",
			"request_id", requestID,
			"writer_id", writerID,
			"testee_id", req.TesteeID,
			"questionnaire_code", req.QuestionnaireCode,
			"queue_depth", len(q.jobs),
			"queue_capacity", cap(q.jobs),
		)
	case submitQueueAdmissionDuplicate:
		q.observe(ctx, resilienceplane.OutcomeQueueDuplicate)
		return nil
	case submitQueueAdmissionRejected:
		q.observe(ctx, resilienceplane.OutcomeQueueFailed)
		return errors.New("previous request failed, please retry with a new request_id")
	case submitQueueAdmissionFull:
		q.observe(ctx, resilienceplane.OutcomeQueueFull)
		q.observeQueueDepth()
		return ErrQueueFull
	case submitQueueAdmissionClosed:
		q.observe(ctx, resilienceplane.OutcomeQueueAdmissionClosed)
		return ErrQueueDraining
	}

	return nil
}

type submitQueueAdmission uint8

const (
	submitQueueAdmissionAccepted submitQueueAdmission = iota
	submitQueueAdmissionDuplicate
	submitQueueAdmissionRejected
	submitQueueAdmissionFull
	submitQueueAdmissionClosed
)

func (q *SubmitQueue) admit(job submitJob, admissionClosed bool) (submitQueueAdmission, int) {
	if q == nil || q.statuses == nil {
		return submitQueueAdmissionRejected, 0
	}
	store := q.statuses
	store.mu.Lock()
	defer store.mu.Unlock()

	cleaned := store.cleanupLocked(time.Now())
	previous, exists := store.statuses[job.requestID]
	if exists {
		switch previous.Response.Status {
		case SubmitStatusDone, SubmitStatusQueued, SubmitStatusProcessing:
			return submitQueueAdmissionDuplicate, cleaned
		case SubmitStatusFailed:
			if !previous.RetryableLeaseFailure {
				return submitQueueAdmissionRejected, cleaned
			}
		}
	}
	if admissionClosed {
		return submitQueueAdmissionClosed, cleaned
	}

	queued := SubmitStatusResponse{Status: SubmitStatusQueued, UpdatedAt: time.Now().Unix()}
	if exists {
		queued.AssessmentID = previous.Response.AssessmentID
	}
	store.statuses[job.requestID] = submitQueueStatusEntry{Response: queued}
	q.outstanding.Add(1)
	select {
	case q.jobs <- job:
		return submitQueueAdmissionAccepted, cleaned
	default:
		q.outstanding.Add(-1)
		if exists {
			store.statuses[job.requestID] = previous
		} else {
			delete(store.statuses, job.requestID)
		}
		return submitQueueAdmissionFull, cleaned
	}
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
			LifecycleBoundary: "process_memory_drainable",
			State:             string(resiliencecontrol.QueueStateActive),
		}
	}
	q.lifecycleMu.Lock()
	state := q.state
	stateVersion := q.stateVersion
	if state == "" {
		state = resiliencecontrol.QueueStateActive
	}
	q.lifecycleMu.Unlock()
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
		LifecycleBoundary: "process_memory_drainable",
		State:             string(state),
		StateVersion:      stateVersion,
		InFlight:          int(q.inFlight.Load()),
		AdmissionClosed:   state != resiliencecontrol.QueueStateActive,
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
	cleaned := q.statuses.SetStatus(requestID, status, answerSheetID, false)
	if status == SubmitStatusProcessing {
		q.inFlight.Add(1)
	} else if status == SubmitStatusDone {
		q.inFlight.Add(-1)
		q.outstanding.Add(-1)
	}
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

func (q *SubmitQueue) setFailed(requestID string, err error) {
	if q == nil || requestID == "" || err == nil {
		return
	}
	retryable := errors.Is(err, locklease.ErrLeaseLost) || errors.Is(err, locklease.ErrLeaseRenewFailed)
	cleaned := q.statuses.SetStatus(requestID, SubmitStatusFailed, "", retryable)
	q.inFlight.Add(-1)
	q.outstanding.Add(-1)
	q.observeCleaned(context.Background(), cleaned)
	q.observeQueueDepth()
	q.observeQueueStatusCounts()
	q.observe(context.Background(), resilienceplane.OutcomeQueueFailed)
}

// Drain closes admission and waits until queued and processing work reaches zero.
// A timeout intentionally leaves the queue in draining state.
func (q *SubmitQueue) Drain(ctx context.Context, opts resiliencecontrol.DrainOptions) (resiliencecontrol.DrainResult, error) {
	if q == nil {
		return resiliencecontrol.DrainResult{}, errors.New("submit queue disabled")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	q.lifecycleMu.Lock()
	if q.state == "" || q.state == resiliencecontrol.QueueStateActive {
		q.state = resiliencecontrol.QueueStateDraining
		q.stateVersion++
	}
	q.lifecycleMu.Unlock()

	waitCtx := ctx
	var cancel context.CancelFunc
	if opts.Timeout > 0 {
		waitCtx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if result, done := q.finishDrainIfEmpty(); done {
			return result, nil
		}
		select {
		case <-waitCtx.Done():
			return q.drainResult(), waitCtx.Err()
		case <-ticker.C:
		}
	}
}

// Resume reopens admission only after a completed drain.
func (q *SubmitQueue) Resume(_ context.Context) error {
	if q == nil {
		return errors.New("submit queue disabled")
	}
	q.lifecycleMu.Lock()
	defer q.lifecycleMu.Unlock()
	if q.state != resiliencecontrol.QueueStatePaused || q.outstanding.Load() != 0 {
		return resiliencecontrol.ErrInvalidState
	}
	q.state = resiliencecontrol.QueueStateActive
	q.stateVersion++
	return nil
}

func (q *SubmitQueue) finishDrainIfEmpty() (resiliencecontrol.DrainResult, bool) {
	q.lifecycleMu.Lock()
	defer q.lifecycleMu.Unlock()
	if q.state == resiliencecontrol.QueueStatePaused {
		return q.drainResultLocked(), true
	}
	if q.outstanding.Load() != 0 {
		return q.drainResultLocked(), false
	}
	q.state = resiliencecontrol.QueueStatePaused
	q.stateVersion++
	return q.drainResultLocked(), true
}

func (q *SubmitQueue) drainResult() resiliencecontrol.DrainResult {
	q.lifecycleMu.Lock()
	defer q.lifecycleMu.Unlock()
	return q.drainResultLocked()
}

func (q *SubmitQueue) drainResultLocked() resiliencecontrol.DrainResult {
	return resiliencecontrol.DrainResult{
		State:      q.state,
		Version:    q.stateVersion,
		Depth:      len(q.jobs),
		InFlight:   int(q.inFlight.Load()),
		FinishedAt: time.Now(),
	}
}

var _ resiliencecontrol.QueueController = (*SubmitQueue)(nil)

func (q *SubmitQueue) setAssessmentID(requestID, assessmentID string) {
	if q == nil || requestID == "" || assessmentID == "" {
		return
	}
	q.statuses.SetAssessmentID(requestID, assessmentID)
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
	statuses    map[string]submitQueueStatusEntry
	lastCleanup time.Time
}

type submitQueueStatusEntry struct {
	Response              SubmitStatusResponse
	RetryableLeaseFailure bool
}

func newSubmitQueueStatusStore(statusTTL time.Duration) *submitQueueStatusStore {
	return &submitQueueStatusStore{
		statusTTL: statusTTL,
		statuses:  make(map[string]submitQueueStatusEntry),
	}
}

func (s *submitQueueStatusStore) Set(requestID string, status SubmitStatusResponse) int {
	return s.SetEntry(requestID, submitQueueStatusEntry{Response: status})
}

func (s *submitQueueStatusStore) SetEntry(requestID string, entry submitQueueStatusEntry) int {
	if s == nil || requestID == "" {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cleaned := s.cleanupLocked(time.Now())
	s.statuses[requestID] = entry
	return cleaned
}

func (s *submitQueueStatusStore) SetStatus(requestID, status, answerSheetID string, retryable bool) int {
	if s == nil || requestID == "" {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cleaned := s.cleanupLocked(time.Now())
	entry := s.statuses[requestID]
	entry.Response.Status = status
	entry.Response.AnswerSheetID = answerSheetID
	entry.Response.UpdatedAt = time.Now().Unix()
	entry.RetryableLeaseFailure = retryable
	s.statuses[requestID] = entry
	return cleaned
}

func (s *submitQueueStatusStore) SetAssessmentID(requestID, assessmentID string) {
	if s == nil || requestID == "" || assessmentID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.statuses[requestID]
	entry.Response.AssessmentID = assessmentID
	entry.Response.UpdatedAt = time.Now().Unix()
	s.statuses[requestID] = entry
}

func (s *submitQueueStatusStore) Get(requestID string) (SubmitStatusResponse, bool, int) {
	if s == nil || requestID == "" {
		return SubmitStatusResponse{}, false, 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cleaned := s.cleanupLocked(time.Now())
	entry, ok := s.statuses[requestID]
	return entry.Response, ok, cleaned
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
	for _, entry := range s.statuses {
		counts[entry.Response.Status]++
	}
	return counts
}

func (s *submitQueueStatusStore) cleanupAt(now time.Time) int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cleanupLocked(now)
}

func (s *submitQueueStatusStore) cleanupLocked(now time.Time) int {
	if now.Sub(s.lastCleanup) < time.Minute {
		return 0
	}
	cleaned := 0
	for key, entry := range s.statuses {
		if now.Sub(time.Unix(entry.Response.UpdatedAt, 0)) > s.statusTTL {
			delete(s.statuses, key)
			cleaned++
		}
	}
	s.lastCleanup = now
	return cleaned
}

package answersheet

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
)

func TestSubmitQueueEnqueueReturnsImmediately(t *testing.T) {
	release := make(chan struct{})
	q := NewSubmitQueue(1, 1, func(context.Context, string, uint64, *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error) {
		<-release
		return &SubmitAnswerSheetResponse{ID: "42", AssessmentID: "9001"}, nil
	})
	if q == nil {
		t.Fatal("expected queue to be initialized")
	}

	start := time.Now()
	if err := q.Enqueue(context.Background(), "req-1", 1, &SubmitAnswerSheetRequest{
		TesteeID:          7,
		QuestionnaireCode: "QNR-001",
	}); err != nil {
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
	processingStarted := make(chan struct{}, 1)
	q := NewSubmitQueue(1, 1, func(context.Context, string, uint64, *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error) {
		processingStarted <- struct{}{}
		<-release
		return &SubmitAnswerSheetResponse{ID: "42"}, nil
	})
	t.Cleanup(func() { close(release) })

	if err := q.Enqueue(context.Background(), "req-1", 1, &SubmitAnswerSheetRequest{}); err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	select {
	case <-processingStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("req-1 did not enter processing")
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

func TestSubmitQueueConcurrentDuplicateRequestCreatesOneJob(t *testing.T) {
	const concurrentRequests = 128

	release := make(chan struct{})
	var calls atomic.Int32
	q := NewSubmitQueue(1, concurrentRequests, func(context.Context, string, uint64, *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error) {
		calls.Add(1)
		<-release
		return &SubmitAnswerSheetResponse{ID: "42"}, nil
	})
	t.Cleanup(func() { close(release) })

	start := make(chan struct{})
	errs := make(chan error, concurrentRequests)
	var wg sync.WaitGroup
	for i := 0; i < concurrentRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- q.Enqueue(context.Background(), "req-concurrent-duplicate", 1, &SubmitAnswerSheetRequest{})
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent duplicate enqueue error = %v, want status reuse", err)
		}
	}

	deadline := time.Now().Add(time.Second)
	for calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("submit calls = %d, want exactly one queued job", got)
	}
}

func TestSubmitQueueFailedRequestRequiresNewRequestID(t *testing.T) {
	q := NewSubmitQueue(1, 1, func(context.Context, string, uint64, *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error) {
		return nil, errors.New("boom")
	})

	if err := q.Enqueue(context.Background(), "req-failed", 1, &SubmitAnswerSheetRequest{}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		status, ok := q.GetStatus("req-failed")
		if ok && status.Status == SubmitStatusFailed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("req-failed did not fail within 5s")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := q.Enqueue(context.Background(), "req-failed", 1, &SubmitAnswerSheetRequest{}); err == nil {
		t.Fatal("expected failed request id reuse to be rejected")
	}
}

func TestSubmitQueueAllowsSameRequestIDAfterRetryableLeaseFailure(t *testing.T) {
	var mu sync.Mutex
	attempts := 0
	q := NewSubmitQueue(1, 1, func(context.Context, string, uint64, *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error) {
		mu.Lock()
		defer mu.Unlock()
		attempts++
		if attempts == 1 {
			return nil, locklease.ErrLeaseLost
		}
		return &SubmitAnswerSheetResponse{ID: "42"}, nil
	})

	if err := q.Enqueue(context.Background(), "req-retry", 1, &SubmitAnswerSheetRequest{}); err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	waitForSubmitStatus(t, q, "req-retry", SubmitStatusFailed)
	if err := q.Enqueue(context.Background(), "req-retry", 1, &SubmitAnswerSheetRequest{}); err != nil {
		t.Fatalf("retry enqueue with same request id: %v", err)
	}
	waitForSubmitStatus(t, q, "req-retry", SubmitStatusDone)
}

func TestSubmitQueueConcurrentRetryableRequeueCreatesOneJob(t *testing.T) {
	const concurrentRetries = 64

	var calls atomic.Int32
	secondStarted := make(chan struct{})
	releaseSecond := make(chan struct{})
	q := NewSubmitQueue(8, concurrentRetries, func(context.Context, string, uint64, *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error) {
		switch calls.Add(1) {
		case 1:
			return nil, locklease.ErrLeaseRenewFailed
		case 2:
			close(secondStarted)
			<-releaseSecond
			return &SubmitAnswerSheetResponse{ID: "42"}, nil
		default:
			return nil, errors.New("duplicate retry job")
		}
	})

	if err := q.Enqueue(context.Background(), "req-concurrent-retry", 1, &SubmitAnswerSheetRequest{}); err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	waitForSubmitStatus(t, q, "req-concurrent-retry", SubmitStatusFailed)

	start := make(chan struct{})
	errs := make(chan error, concurrentRetries)
	var wg sync.WaitGroup
	for range concurrentRetries {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- q.Enqueue(context.Background(), "req-concurrent-retry", 1, &SubmitAnswerSheetRequest{})
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent retry enqueue error = %v", err)
		}
	}

	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("retry job did not start")
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("submit calls = %d, want original plus exactly one retry", got)
	}
	close(releaseSecond)
	waitForSubmitStatus(t, q, "req-concurrent-retry", SubmitStatusDone)
}

func TestSubmitQueueFullRestoresRetryableFailedEntry(t *testing.T) {
	q := &SubmitQueue{
		jobs:     make(chan submitJob, 1),
		statuses: newSubmitQueueStatusStore(10 * time.Minute),
		observer: defaultSubmitQueueObserver(nil),
		subject: resilienceplane.Subject{
			Component: "collection-server",
			Scope:     "answersheet_submit",
			Resource:  "submit_queue",
			Strategy:  "memory_channel",
		},
	}
	q.jobs <- submitJob{requestID: "buffer-full"}
	q.setFailed("req-retry-full", locklease.ErrLeaseLost)

	if err := q.Enqueue(context.Background(), "req-retry-full", 1, &SubmitAnswerSheetRequest{}); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("Enqueue() error = %v, want ErrQueueFull", err)
	}
	q.statuses.mu.Lock()
	entry := q.statuses.statuses["req-retry-full"]
	q.statuses.mu.Unlock()
	if entry.Response.Status != SubmitStatusFailed || !entry.RetryableLeaseFailure {
		t.Fatalf("entry after queue full = %+v", entry)
	}

	<-q.jobs
	if err := q.Enqueue(context.Background(), "req-retry-full", 1, &SubmitAnswerSheetRequest{}); err != nil {
		t.Fatalf("retry after capacity recovery: %v", err)
	}
}

func TestSubmitQueueCleanupRemovesRetryMetadata(t *testing.T) {
	store := newSubmitQueueStatusStore(time.Millisecond)
	store.SetStatus("expired-retry", SubmitStatusFailed, "", true)
	store.mu.Lock()
	entry := store.statuses["expired-retry"]
	entry.Response.UpdatedAt = time.Now().Add(-time.Second).Unix()
	store.statuses["expired-retry"] = entry
	store.lastCleanup = time.Now().Add(-time.Hour)
	store.mu.Unlock()

	if _, cleaned := store.Snapshot(time.Now()); cleaned != 1 {
		t.Fatalf("cleaned = %d, want 1", cleaned)
	}
	store.mu.Lock()
	_, exists := store.statuses["expired-retry"]
	store.mu.Unlock()
	if exists {
		t.Fatal("expired retry metadata was not removed")
	}
}

func waitForSubmitStatus(t *testing.T, q *SubmitQueue, requestID, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, ok := q.GetStatus(requestID)
		if ok && status.Status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("request %q did not reach status %q", requestID, want)
}

func TestSubmitQueueStatusExpiresAfterTTL(t *testing.T) {
	q := NewSubmitQueue(1, 1, func(context.Context, string, uint64, *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error) {
		return &SubmitAnswerSheetResponse{ID: "42"}, nil
	})
	q.statuses.statusTTL = time.Millisecond

	q.statuses.mu.Lock()
	q.statuses.statuses["expired"] = submitQueueStatusEntry{Response: SubmitStatusResponse{
		Status:    SubmitStatusDone,
		UpdatedAt: time.Now().Add(-time.Second).Unix(),
	}}
	q.statuses.mu.Unlock()

	if _, ok := q.GetStatus("expired"); ok {
		t.Fatal("expected expired status to be removed")
	}
}

func TestSubmitQueueStatusSnapshotReportsDepthCountsAndTTL(t *testing.T) {
	release := make(chan struct{})
	q := NewSubmitQueue(1, 2, func(context.Context, string, uint64, *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error) {
		<-release
		return &SubmitAnswerSheetResponse{ID: "42"}, nil
	})
	t.Cleanup(func() { close(release) })

	if err := q.Enqueue(context.Background(), "req-1", 1, &SubmitAnswerSheetRequest{}); err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	if err := q.Enqueue(context.Background(), "req-2", 1, &SubmitAnswerSheetRequest{}); err != nil {
		t.Fatalf("second enqueue: %v", err)
	}

	snapshot := q.StatusSnapshot(time.Now())
	if snapshot.Name != "answersheet_submit" || snapshot.Capacity != 2 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	if snapshot.StatusTTLSeconds != int64((10 * time.Minute).Seconds()) {
		t.Fatalf("ttl seconds = %d", snapshot.StatusTTLSeconds)
	}
	if snapshot.StatusCounts[SubmitStatusQueued]+snapshot.StatusCounts[SubmitStatusProcessing] != 2 {
		t.Fatalf("status counts = %+v, want two in-flight statuses", snapshot.StatusCounts)
	}
}

func TestSubmitQueueStatusSnapshotReportsCleanupOutcome(t *testing.T) {
	observer := &submitQueueRecordingObserver{}
	q := NewSubmitQueueWithOptions(1, 1, func(context.Context, string, uint64, *SubmitAnswerSheetRequest) (*SubmitAnswerSheetResponse, error) {
		return &SubmitAnswerSheetResponse{ID: "42"}, nil
	}, SubmitQueueRuntimeOptions{Observer: observer})
	q.statuses.statusTTL = time.Millisecond
	q.statuses.mu.Lock()
	q.statuses.statuses["expired"] = submitQueueStatusEntry{Response: SubmitStatusResponse{
		Status:    SubmitStatusDone,
		UpdatedAt: time.Now().Add(-time.Second).Unix(),
	}}
	q.statuses.lastCleanup = time.Now().Add(-time.Hour)
	q.statuses.mu.Unlock()

	snapshot := q.StatusSnapshot(time.Now())
	if snapshot.StatusCounts[SubmitStatusDone] != 0 {
		t.Fatalf("status counts = %+v, want expired done status removed", snapshot.StatusCounts)
	}
	if !observer.has(resilienceplane.OutcomeQueueStatusCleaned) {
		t.Fatal("expected queue_status_cleaned outcome")
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

	deadline := time.Now().Add(5 * time.Second)
	for {
		status, ok := q.GetStatus("req-observed")
		if ok && status.Status == SubmitStatusDone {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("req-observed did not finish within 5s, last status=%+v ok=%v", status, ok)
		}
		time.Sleep(10 * time.Millisecond)
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

func TestSubmitQueueHasNoLifecycleControlSurface(t *testing.T) {
	queueType := reflect.TypeOf(&SubmitQueue{})
	for _, method := range []string{"Stop", "Drain", "Close"} {
		if _, ok := queueType.MethodByName(method); ok {
			t.Fatalf("SubmitQueue exposes %s; lifecycle drain/shutdown is intentionally not supported", method)
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

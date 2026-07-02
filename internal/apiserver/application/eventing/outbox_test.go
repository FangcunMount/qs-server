package eventing

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventobservability"
	"github.com/FangcunMount/qs-server/internal/pkg/outboxpriority"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type outboxObserver struct {
	events       []eventobservability.OutboxEvent
	status       []eventobservability.OutboxStatusEvent
	statusScrape []eventobservability.OutboxStatusScrapeEvent
}

func (o *outboxObserver) ObservePublish(context.Context, eventobservability.PublishEvent) {}
func (o *outboxObserver) ObserveConsume(context.Context, eventobservability.ConsumeEvent) {}

func (o *outboxObserver) ObserveOutbox(_ context.Context, evt eventobservability.OutboxEvent) {
	o.events = append(o.events, evt)
}

func (o *outboxObserver) ObserveOutboxStatus(_ context.Context, evt eventobservability.OutboxStatusEvent) {
	o.status = append(o.status, evt)
}

func (o *outboxObserver) ObserveOutboxStatusScrape(_ context.Context, evt eventobservability.OutboxStatusScrapeEvent) {
	o.statusScrape = append(o.statusScrape, evt)
}

type fakeOutboxStore struct {
	pending          []PendingOutboxEvent
	claimByIDs       map[string]PendingOutboxEvent
	claimByIDsCalls  [][]string
	claimErr         error
	markPublishedErr error
	markFailedErr    error
	statusSnapshot   outboxport.StatusSnapshot
	statusErr        error
	statusCalls      int
	published        []string
	failed           []string
	lastLimit        int
}

type fakeReadyIndex struct {
	buckets  map[string][]string
	enqueues []readyIndexEnqueue
}

type readyIndexEnqueue struct {
	eventType     string
	eventID       string
	nextAttemptAt time.Time
	createdAt     time.Time
}

func (f *fakeReadyIndex) Enqueue(_ context.Context, eventType, eventID string, nextAttemptAt, createdAt time.Time) error {
	f.enqueues = append(f.enqueues, readyIndexEnqueue{
		eventType:     eventType,
		eventID:       eventID,
		nextAttemptAt: nextAttemptAt,
		createdAt:     createdAt,
	})
	return nil
}

func (f *fakeReadyIndex) Remove(context.Context, string, string) error {
	return nil
}

func (f *fakeReadyIndex) RemoveByEventID(_ context.Context, eventID string) error {
	for bucket, ids := range f.buckets {
		filtered := ids[:0]
		for _, id := range ids {
			if id != eventID {
				filtered = append(filtered, id)
			}
		}
		if len(filtered) == 0 {
			delete(f.buckets, bucket)
			continue
		}
		f.buckets[bucket] = filtered
	}
	return nil
}

func (f *fakeReadyIndex) ClaimDueIDs(_ context.Context, bucket string, limit int, _ time.Time) ([]string, error) {
	ids := f.buckets[bucket]
	if len(ids) > limit {
		ids = ids[:limit]
	}
	f.buckets[bucket] = f.buckets[bucket][len(ids):]
	return ids, nil
}

type durableFakePublisher struct {
	fakePublisher
	mqBacked bool
}

type fakeBeforePublishHook struct {
	calls    []string
	failWith error
}

func (h *fakeBeforePublishHook) BeforePublish(_ context.Context, pending PendingOutboxEvent) error {
	h.calls = append(h.calls, pending.EventID)
	return h.failWith
}

func (p *durableFakePublisher) IsMQBacked() bool {
	return p.mqBacked
}

func (s *fakeOutboxStore) ClaimDueEvents(_ context.Context, limit int, _ time.Time) ([]PendingOutboxEvent, error) {
	s.lastLimit = limit
	if s.claimErr != nil {
		return nil, s.claimErr
	}
	return s.pending, nil
}

func (s *fakeOutboxStore) ClaimEventsByIDs(_ context.Context, eventIDs []string, _ time.Time) ([]PendingOutboxEvent, error) {
	s.claimByIDsCalls = append(s.claimByIDsCalls, append([]string(nil), eventIDs...))
	claimed := make([]PendingOutboxEvent, 0, len(eventIDs))
	for _, eventID := range eventIDs {
		if pending, ok := s.claimByIDs[eventID]; ok {
			claimed = append(claimed, pending)
		}
	}
	return claimed, nil
}

func (s *fakeOutboxStore) MarkEventPublished(_ context.Context, eventID string, _ time.Time) error {
	s.published = append(s.published, eventID)
	return s.markPublishedErr
}

func (s *fakeOutboxStore) MarkEventFailed(_ context.Context, eventID, _ string, _ time.Time) error {
	s.failed = append(s.failed, eventID)
	return s.markFailedErr
}

func (s *fakeOutboxStore) OutboxStatusSnapshot(context.Context, time.Time) (outboxport.StatusSnapshot, error) {
	s.statusCalls++
	if s.statusErr != nil {
		return outboxport.StatusSnapshot{}, s.statusErr
	}
	return s.statusSnapshot, nil
}

func TestOutboxRelayObservesClaimFailed(t *testing.T) {
	wantErr := errors.New("claim failed")
	observer := &outboxObserver{}
	relay := NewOutboxRelayWithOptions(OutboxRelayOptions{
		Name:      "test-relay",
		Store:     &fakeOutboxStore{claimErr: wantErr},
		Publisher: &fakePublisher{},
		Observer:  observer,
	})

	err := relay.DispatchDue(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("DispatchDue error = %v, want %v", err, wantErr)
	}
	assertOutboxOutcome(t, observer, eventobservability.OutboxOutcomeClaimFailed)
}

func TestOutboxRelayObservesPublished(t *testing.T) {
	observer := &outboxObserver{}
	store := &fakeOutboxStore{
		pending: []PendingOutboxEvent{pendingEvent("evt-1", eventcatalog.AssessmentSubmitted)},
		statusSnapshot: outboxport.StatusSnapshot{
			Store: "test-relay",
			Buckets: []outboxport.StatusBucket{
				{Status: "pending", Count: 0},
			},
		},
	}
	relay := NewOutboxRelayWithOptions(OutboxRelayOptions{
		Name:      "test-relay",
		Store:     store,
		Publisher: &fakePublisher{},
		Observer:  observer,
	})

	if err := relay.DispatchDue(context.Background()); err != nil {
		t.Fatalf("DispatchDue: %v", err)
	}
	assertOutboxOutcome(t, observer, eventobservability.OutboxOutcomePublished)
	if len(store.published) != 1 || store.published[0] != "evt-1" {
		t.Fatalf("published markers = %#v, want evt-1", store.published)
	}
	assertOutboxStatusScrape(t, observer, eventobservability.OutboxStatusScrapeOutcomeSuccess)
}

func TestOutboxRelayUsesConfiguredBatchSize(t *testing.T) {
	store := &fakeOutboxStore{}
	relay := NewOutboxRelayWithOptions(OutboxRelayOptions{
		Name:      "test-relay",
		Store:     store,
		Publisher: &fakePublisher{},
		BatchSize: 300,
	})

	if err := relay.DispatchDue(context.Background()); err != nil {
		t.Fatalf("DispatchDue: %v", err)
	}
	if store.lastLimit != 300 {
		t.Fatalf("claim limit = %d, want 300", store.lastLimit)
	}
}

func TestOutboxRelayUsesConfiguredPublishWorkers(t *testing.T) {
	store := &fakeOutboxStore{
		pending: []PendingOutboxEvent{
			pendingEvent("evt-1", eventcatalog.AssessmentSubmitted),
			pendingEvent("evt-2", eventcatalog.ReportGenerated),
			pendingEvent("evt-3", eventcatalog.AnswerSheetSubmitted),
			pendingEvent("evt-4", eventcatalog.ReportGeneratedV2),
		},
	}
	publisher := &trackingPublisher{}
	relay := NewOutboxRelayWithOptions(OutboxRelayOptions{
		Name:           "test-relay",
		Store:          store,
		Publisher:      publisher,
		PublishWorkers: 2,
	})

	if err := relay.DispatchDue(context.Background()); err != nil {
		t.Fatalf("DispatchDue: %v", err)
	}
	if publisher.maxInflight > 2 {
		t.Fatalf("max inflight publishes = %d, want <= 2", publisher.maxInflight)
	}
	if len(store.published) != 4 {
		t.Fatalf("published markers = %#v, want 4 events", store.published)
	}
}

func TestOutboxRelayMarksPublishedBeforeWholeClaimedBatchCompletes(t *testing.T) {
	store := newBlockingMarkStore([]PendingOutboxEvent{
		pendingEvent("evt-1", eventcatalog.AssessmentSubmitted),
		pendingEvent("evt-2", eventcatalog.ReportGenerated),
		pendingEvent("evt-3", eventcatalog.AnswerSheetSubmitted),
	})
	publisher := &blockingThirdPublisher{
		blockEventType: eventcatalog.AnswerSheetSubmitted,
		blocked:        make(chan struct{}),
		release:        make(chan struct{}),
	}
	relay := NewOutboxRelayWithOptions(OutboxRelayOptions{
		Name:           "test-relay",
		Store:          store,
		Publisher:      publisher,
		PublishWorkers: 2,
	})

	done := make(chan error, 1)
	go func() {
		done <- relay.DispatchDue(context.Background())
	}()

	select {
	case <-publisher.blocked:
	case <-time.After(time.Second):
		t.Fatal("third publish did not block")
	}
	select {
	case <-store.marked:
	case <-time.After(time.Second):
		t.Fatal("expected first published batch to be marked while later publish is still blocked")
	}

	store.mu.Lock()
	batches := append([][]string(nil), store.publishedBatches...)
	store.mu.Unlock()
	if len(batches) == 0 || len(batches[0]) != 2 {
		t.Fatalf("published batches = %#v, want first batch of two", batches)
	}

	close(publisher.release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("DispatchDue: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("DispatchDue did not finish after releasing blocked publish")
	}
}

func TestOutboxRelayObservesBatchMarkPublishedFailed(t *testing.T) {
	observer := &outboxObserver{}
	store := newBlockingMarkStore([]PendingOutboxEvent{
		pendingEvent("evt-1", eventcatalog.AssessmentSubmitted),
	})
	store.markPublishedErr = errors.New("batch mark failed")
	relay := NewOutboxRelayWithOptions(OutboxRelayOptions{
		Name:      "test-relay",
		Store:     store,
		Publisher: &fakePublisher{},
		Observer:  observer,
	})

	if err := relay.DispatchDue(context.Background()); err != nil {
		t.Fatalf("DispatchDue: %v", err)
	}
	assertOutboxOutcome(t, observer, eventobservability.OutboxOutcomeMarkPublishedFailed)
	if len(store.failed) != 0 {
		t.Fatalf("failed markers = %#v, want none", store.failed)
	}
}

func TestOutboxRelayObservesPublishFailureAndContinues(t *testing.T) {
	observer := &outboxObserver{}
	store := &fakeOutboxStore{
		pending: []PendingOutboxEvent{
			pendingEvent("evt-1", eventcatalog.AssessmentSubmitted),
			pendingEvent("evt-2", eventcatalog.ReportGenerated),
		},
	}
	publisher := &fakePublisher{failAt: map[string]error{eventcatalog.AssessmentSubmitted: errors.New("publish failed")}}
	relay := NewOutboxRelayWithOptions(OutboxRelayOptions{
		Name:      "test-relay",
		Store:     store,
		Publisher: publisher,
		Observer:  observer,
	})

	if err := relay.DispatchDue(context.Background()); err != nil {
		t.Fatalf("DispatchDue: %v", err)
	}
	assertOutboxContainsOutcome(t, observer, eventobservability.OutboxOutcomePublishFailed)
	assertOutboxContainsOutcome(t, observer, eventobservability.OutboxOutcomePublished)
	if len(publisher.published) != 2 {
		t.Fatalf("publish attempts = %d, want 2", len(publisher.published))
	}
	if len(store.failed) != 1 || store.failed[0] != "evt-1" {
		t.Fatalf("failed markers = %#v, want evt-1", store.failed)
	}
	if len(store.published) != 1 || store.published[0] != "evt-2" {
		t.Fatalf("published markers = %#v, want evt-2", store.published)
	}
}

func TestOutboxRelayRunsBeforePublishHook(t *testing.T) {
	hook := &fakeBeforePublishHook{}
	store := &fakeOutboxStore{
		pending: []PendingOutboxEvent{pendingEvent("evt-1", eventcatalog.AnswerSheetSubmitted)},
	}
	publisher := &fakePublisher{}
	relay := NewOutboxRelayWithOptions(OutboxRelayOptions{
		Name:               "test-relay",
		Store:              store,
		Publisher:          publisher,
		BeforePublishHooks: []OutboxBeforePublishHook{hook},
	})

	if err := relay.DispatchDue(context.Background()); err != nil {
		t.Fatalf("DispatchDue: %v", err)
	}
	if len(hook.calls) != 1 || hook.calls[0] != "evt-1" {
		t.Fatalf("hook calls = %#v, want evt-1", hook.calls)
	}
	if len(publisher.published) != 1 || publisher.published[0] != eventcatalog.AnswerSheetSubmitted {
		t.Fatalf("published events = %#v, want %q", publisher.published, eventcatalog.AnswerSheetSubmitted)
	}
	if len(store.published) != 1 || store.published[0] != "evt-1" {
		t.Fatalf("published markers = %#v, want evt-1", store.published)
	}
}

func TestOutboxRelayBeforePublishFailureMarksFailedAndSkipsPublish(t *testing.T) {
	observer := &outboxObserver{}
	hookErr := errors.New("projection failed")
	hook := &fakeBeforePublishHook{failWith: hookErr}
	store := &fakeOutboxStore{
		pending: []PendingOutboxEvent{pendingEvent("evt-1", eventcatalog.AnswerSheetSubmitted)},
	}
	publisher := &fakePublisher{}
	relay := NewOutboxRelayWithOptions(OutboxRelayOptions{
		Name:               "test-relay",
		Store:              store,
		Publisher:          publisher,
		Observer:           observer,
		BeforePublishHooks: []OutboxBeforePublishHook{hook},
	})

	if err := relay.DispatchDue(context.Background()); err != nil {
		t.Fatalf("DispatchDue: %v", err)
	}
	if len(publisher.published) != 0 {
		t.Fatalf("publish attempts = %d, want 0", len(publisher.published))
	}
	if len(store.failed) != 1 || store.failed[0] != "evt-1" {
		t.Fatalf("failed markers = %#v, want evt-1", store.failed)
	}
	if len(store.published) != 0 {
		t.Fatalf("published markers = %#v, want none", store.published)
	}
	assertOutboxContainsOutcome(t, observer, eventobservability.OutboxOutcomePublishFailed)
}

func TestOutboxRelayObservesMarkFailedFailed(t *testing.T) {
	observer := &outboxObserver{}
	store := &fakeOutboxStore{
		pending:       []PendingOutboxEvent{pendingEvent("evt-1", eventcatalog.AssessmentSubmitted)},
		markFailedErr: errors.New("mark failed"),
	}
	relay := NewOutboxRelayWithOptions(OutboxRelayOptions{
		Name:      "test-relay",
		Store:     store,
		Publisher: &fakePublisher{failAt: map[string]error{eventcatalog.AssessmentSubmitted: errors.New("publish failed")}},
		Observer:  observer,
	})

	if err := relay.DispatchDue(context.Background()); err != nil {
		t.Fatalf("DispatchDue: %v", err)
	}
	assertOutboxContainsOutcome(t, observer, eventobservability.OutboxOutcomeMarkFailedFailed)
	assertOutboxContainsOutcome(t, observer, eventobservability.OutboxOutcomePublishFailed)
}

func TestOutboxRelayObservesMarkPublishedFailed(t *testing.T) {
	observer := &outboxObserver{}
	relay := NewOutboxRelayWithOptions(OutboxRelayOptions{
		Name: "test-relay",
		Store: &fakeOutboxStore{
			pending:          []PendingOutboxEvent{pendingEvent("evt-1", eventcatalog.AssessmentSubmitted)},
			markPublishedErr: errors.New("mark published failed"),
		},
		Publisher: &fakePublisher{},
		Observer:  observer,
	})

	if err := relay.DispatchDue(context.Background()); err != nil {
		t.Fatalf("DispatchDue: %v", err)
	}
	assertOutboxOutcome(t, observer, eventobservability.OutboxOutcomeMarkPublishedFailed)
}

func TestOutboxRelayStatusReporterFailureDoesNotChangeDispatchResult(t *testing.T) {
	observer := &outboxObserver{}
	store := &fakeOutboxStore{
		pending:   []PendingOutboxEvent{pendingEvent("evt-1", eventcatalog.AssessmentSubmitted)},
		statusErr: errors.New("status failed"),
	}
	relay := NewOutboxRelayWithOptions(OutboxRelayOptions{
		Name:      "test-relay",
		Store:     store,
		Publisher: &fakePublisher{},
		Observer:  observer,
	})

	if err := relay.DispatchDue(context.Background()); err != nil {
		t.Fatalf("DispatchDue: %v", err)
	}
	assertOutboxStatusScrape(t, observer, eventobservability.OutboxStatusScrapeOutcomeFailure)
	if len(store.published) != 1 || store.published[0] != "evt-1" {
		t.Fatalf("published markers = %#v, want evt-1", store.published)
	}
}

func TestOutboxStatusReporterThrottlesStatusScrapes(t *testing.T) {
	current := time.Date(2026, 6, 17, 16, 0, 0, 0, time.UTC)
	observer := &outboxObserver{}
	store := &fakeOutboxStore{
		statusSnapshot: outboxport.StatusSnapshot{
			Store: "test-relay",
			Buckets: []outboxport.StatusBucket{
				{Status: "pending", Count: 10},
			},
		},
	}
	reporter := newOutboxStatusReporterWithInterval(
		"test-relay",
		store,
		observer,
		func() time.Time { return current },
		30*time.Second,
	)

	reporter.ReportOutboxStatus(context.Background())
	reporter.ReportOutboxStatus(context.Background())

	if store.statusCalls != 1 {
		t.Fatalf("status calls before interval = %d, want 1", store.statusCalls)
	}
	if len(observer.statusScrape) != 1 {
		t.Fatalf("status scrapes before interval = %d, want 1", len(observer.statusScrape))
	}

	current = current.Add(31 * time.Second)
	reporter.ReportOutboxStatus(context.Background())

	if store.statusCalls != 2 {
		t.Fatalf("status calls after interval = %d, want 2", store.statusCalls)
	}
	if len(observer.statusScrape) != 2 {
		t.Fatalf("status scrapes after interval = %d, want 2", len(observer.statusScrape))
	}
}

func TestDurableOutboxRelayRejectsNonMQBackedPublisher(t *testing.T) {
	store := &fakeOutboxStore{
		pending: []PendingOutboxEvent{pendingEvent("evt-1", eventcatalog.AssessmentSubmitted)},
	}
	relay := NewOutboxRelayWithOptions(OutboxRelayOptions{
		Name:                    "durable-relay",
		Store:                   store,
		Publisher:               &fakePublisher{},
		RequireDurablePublisher: true,
	})
	if relay != nil {
		t.Fatal("expected durable relay construction to reject generic publisher")
	}
	if len(store.published) != 0 || len(store.failed) != 0 {
		t.Fatalf("store should not be touched, published=%#v failed=%#v", store.published, store.failed)
	}
}

func TestDurableOutboxRelayAcceptsMQBackedPublisher(t *testing.T) {
	store := &fakeOutboxStore{
		pending: []PendingOutboxEvent{pendingEvent("evt-1", eventcatalog.AssessmentSubmitted)},
	}
	relay := NewOutboxRelayWithOptions(OutboxRelayOptions{
		Name:                    "durable-relay",
		Store:                   store,
		Publisher:               &durableFakePublisher{mqBacked: true},
		RequireDurablePublisher: true,
	})
	if relay == nil {
		t.Fatal("expected durable relay construction to accept MQ-backed publisher")
	}
	if err := relay.DispatchDue(context.Background()); err != nil {
		t.Fatalf("DispatchDue: %v", err)
	}
	if len(store.published) != 1 || store.published[0] != "evt-1" {
		t.Fatalf("published markers = %#v, want evt-1", store.published)
	}
}

func TestOutboxRelayClaimsFromReadyIndexFirst(t *testing.T) {
	ready := &fakeReadyIndex{
		buckets: map[string][]string{
			outboxpriority.BucketP0: {"evt-zset"},
		},
	}
	store := &fakeOutboxStore{
		claimByIDs: map[string]PendingOutboxEvent{
			"evt-zset": pendingEvent("evt-zset", eventcatalog.AnswerSheetSubmitted),
		},
	}
	relay := NewOutboxRelayWithOptions(OutboxRelayOptions{
		Name:       "test-relay",
		Store:      store,
		Publisher:  &fakePublisher{},
		ReadyIndex: ready,
	})

	if err := relay.DispatchDue(context.Background()); err != nil {
		t.Fatalf("DispatchDue: %v", err)
	}
	if len(store.claimByIDsCalls) != 1 || len(store.claimByIDsCalls[0]) != 1 || store.claimByIDsCalls[0][0] != "evt-zset" {
		t.Fatalf("claim by ids calls = %#v, want evt-zset", store.claimByIDsCalls)
	}
	if len(store.published) != 1 || store.published[0] != "evt-zset" {
		t.Fatalf("published markers = %#v, want evt-zset", store.published)
	}
	if store.lastLimit != 0 {
		t.Fatalf("db fallback claim limit = %d, want 0 when ready index hit", store.lastLimit)
	}
}

func TestOutboxRelayFailureReenqueuesCorrectReadyBucket(t *testing.T) {
	ready := &fakeReadyIndex{buckets: map[string][]string{}}
	store := &fakeOutboxStore{
		pending: []PendingOutboxEvent{pendingEvent("evt-1", eventcatalog.AssessmentSubmitted)},
	}
	relay := NewOutboxRelayWithOptions(OutboxRelayOptions{
		Name:       "test-relay",
		Store:      store,
		Publisher:  &fakePublisher{failAt: map[string]error{eventcatalog.AssessmentSubmitted: errors.New("publish failed")}},
		ReadyIndex: ready,
	})

	if err := relay.DispatchDue(context.Background()); err != nil {
		t.Fatalf("DispatchDue: %v", err)
	}
	if len(ready.enqueues) != 1 {
		t.Fatalf("ready index enqueues = %#v, want one", ready.enqueues)
	}
	if ready.enqueues[0].eventType != eventcatalog.AssessmentSubmitted || ready.enqueues[0].eventID != "evt-1" {
		t.Fatalf("ready index enqueue = %#v, want assessment.submitted evt-1", ready.enqueues[0])
	}
	if outboxpriority.Bucket(ready.enqueues[0].eventType) != outboxpriority.BucketP0 {
		t.Fatalf("bucket = %q, want %q", outboxpriority.Bucket(ready.enqueues[0].eventType), outboxpriority.BucketP0)
	}
}

type trackingPublisher struct {
	mu          sync.Mutex
	inflight    int
	maxInflight int
}

func (p *trackingPublisher) Publish(context.Context, event.DomainEvent) error {
	p.mu.Lock()
	p.inflight++
	if p.inflight > p.maxInflight {
		p.maxInflight = p.inflight
	}
	p.mu.Unlock()

	time.Sleep(10 * time.Millisecond)

	p.mu.Lock()
	p.inflight--
	p.mu.Unlock()
	return nil
}

func (p *trackingPublisher) PublishAll(ctx context.Context, events []event.DomainEvent) error {
	for _, evt := range events {
		if err := p.Publish(ctx, evt); err != nil {
			return err
		}
	}
	return nil
}

type blockingThirdPublisher struct {
	mu             sync.Mutex
	blockEventType string
	blocked        chan struct{}
	release        chan struct{}
	blockedOnce    bool
}

func (p *blockingThirdPublisher) Publish(ctx context.Context, evt event.DomainEvent) error {
	p.mu.Lock()
	shouldBlock := evt.EventType() == p.blockEventType && !p.blockedOnce
	if shouldBlock {
		p.blockedOnce = true
	}
	p.mu.Unlock()
	if shouldBlock {
		close(p.blocked)
		select {
		case <-p.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (p *blockingThirdPublisher) PublishAll(ctx context.Context, events []event.DomainEvent) error {
	for _, evt := range events {
		if err := p.Publish(ctx, evt); err != nil {
			return err
		}
	}
	return nil
}

type blockingMarkStore struct {
	fakeOutboxStore
	mu               sync.Mutex
	marked           chan struct{}
	publishedBatches [][]string
}

func newBlockingMarkStore(pending []PendingOutboxEvent) *blockingMarkStore {
	return &blockingMarkStore{
		fakeOutboxStore: fakeOutboxStore{pending: pending},
		marked:          make(chan struct{}, 1),
	}
}

func (s *blockingMarkStore) MarkEventsPublished(_ context.Context, eventIDs []string, _ time.Time) error {
	s.mu.Lock()
	s.publishedBatches = append(s.publishedBatches, append([]string(nil), eventIDs...))
	s.mu.Unlock()
	select {
	case s.marked <- struct{}{}:
	default:
	}
	return s.markPublishedErr
}

func (s *blockingMarkStore) MarkEventsFailed(_ context.Context, failures []outboxport.FailedMark, _ time.Time) error {
	for _, failure := range failures {
		s.failed = append(s.failed, failure.EventID)
	}
	return s.markFailedErr
}

func pendingEvent(eventID, eventType string) PendingOutboxEvent {
	return PendingOutboxEvent{
		EventID: eventID,
		Event:   event.New(eventType, "Sample", eventID, struct{}{}),
	}
}

func assertOutboxOutcome(t *testing.T, observer *outboxObserver, outcome eventobservability.OutboxOutcome) {
	t.Helper()
	if len(observer.events) != 1 {
		t.Fatalf("observed outbox events = %#v, want one", observer.events)
	}
	if observer.events[0].Outcome != outcome {
		t.Fatalf("outcome = %q, want %q", observer.events[0].Outcome, outcome)
	}
}

func assertOutboxContainsOutcome(t *testing.T, observer *outboxObserver, outcome eventobservability.OutboxOutcome) {
	t.Helper()
	for _, evt := range observer.events {
		if evt.Outcome == outcome {
			return
		}
	}
	t.Fatalf("observed outbox events = %#v, want outcome %q", observer.events, outcome)
}

func assertOutboxStatusScrape(t *testing.T, observer *outboxObserver, outcome eventobservability.OutboxStatusScrapeOutcome) {
	t.Helper()
	if len(observer.statusScrape) != 1 {
		t.Fatalf("observed status scrape events = %#v, want one", observer.statusScrape)
	}
	if observer.statusScrape[0].Outcome != outcome {
		t.Fatalf("scrape outcome = %q, want %q", observer.statusScrape[0].Outcome, outcome)
	}
}

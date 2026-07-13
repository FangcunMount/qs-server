package eventing

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/observe"
)

type outboxObserver struct {
	mu           sync.Mutex
	events       []eventobservability.OutboxEvent
	status       []eventobservability.OutboxStatusEvent
	statusScrape []eventobservability.OutboxStatusScrapeEvent
}

func (o *outboxObserver) ObservePublish(context.Context, eventobservability.PublishEvent) {}
func (o *outboxObserver) ObserveConsume(context.Context, eventobservability.ConsumeEvent) {}

func (o *outboxObserver) ObserveOutbox(_ context.Context, evt eventobservability.OutboxEvent) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, evt)
}

func (o *outboxObserver) ObserveOutboxStatus(_ context.Context, evt eventobservability.OutboxStatusEvent) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.status = append(o.status, evt)
}

func (o *outboxObserver) ObserveOutboxStatusScrape(_ context.Context, evt eventobservability.OutboxStatusScrapeEvent) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.statusScrape = append(o.statusScrape, evt)
}

func (o *outboxObserver) hasOutcome(outcome eventobservability.OutboxOutcome) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, evt := range o.events {
		if evt.Outcome == outcome {
			return true
		}
	}
	return false
}

func (o *outboxObserver) statusScrapeCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.statusScrape)
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
		pending: []PendingOutboxEvent{pendingEvent("evt-1", eventcatalog.EvaluationRequested)},
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

func TestOutboxRelayPerEventGoroutineBaseline(t *testing.T) {
	store := &fakeOutboxStore{
		pending: []PendingOutboxEvent{
			pendingEvent("evt-1", eventcatalog.EvaluationRequested),
			pendingEvent("evt-2", eventcatalog.InterpretationReportGenerated),
			pendingEvent("evt-3", eventcatalog.AnswerSheetSubmitted),
			pendingEvent("evt-4", eventcatalog.InterpretationReportGenerated),
			pendingEvent("evt-5", eventcatalog.EvaluationRequested),
			pendingEvent("evt-6", eventcatalog.InterpretationReportGenerated),
		},
	}
	publisher := &trackingPublisher{}
	relay := NewOutboxRelayWithOptions(OutboxRelayOptions{
		Name:           "test-relay",
		Store:          store,
		Publisher:      publisher,
		PublishWorkers: 3,
	})

	if err := relay.DispatchDue(context.Background()); err != nil {
		t.Fatalf("DispatchDue: %v", err)
	}
	// Characterization 基线 (pre worker-pool re因子): 一个goroutine per 待处理。
	// event, 使用 信号量 限制 并发 发布。
	if publisher.maxInflight > 3 {
		t.Fatalf("max inflight publishes = %d, want <= 3", publisher.maxInflight)
	}
	if len(store.published) != 6 {
		t.Fatalf("published markers = %#v, want 6 events", store.published)
	}
}

func TestOutboxRelayUsesConfiguredPublishWorkers(t *testing.T) {
	store := &fakeOutboxStore{
		pending: []PendingOutboxEvent{
			pendingEvent("evt-1", eventcatalog.EvaluationRequested),
			pendingEvent("evt-2", eventcatalog.InterpretationReportGenerated),
			pendingEvent("evt-3", eventcatalog.AnswerSheetSubmitted),
			pendingEvent("evt-4", eventcatalog.InterpretationReportGenerated),
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
		pendingEvent("evt-1", eventcatalog.EvaluationRequested),
		pendingEvent("evt-2", eventcatalog.InterpretationReportGenerated),
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
		pendingEvent("evt-1", eventcatalog.EvaluationRequested),
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
			pendingEvent("evt-1", eventcatalog.EvaluationRequested),
			pendingEvent("evt-2", eventcatalog.InterpretationReportGenerated),
		},
	}
	publisher := &fakePublisher{failAt: map[string]error{eventcatalog.EvaluationRequested: errors.New("publish failed")}}
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

func TestOutboxRelayObservesMarkFailedFailed(t *testing.T) {
	observer := &outboxObserver{}
	store := &fakeOutboxStore{
		pending:       []PendingOutboxEvent{pendingEvent("evt-1", eventcatalog.EvaluationRequested)},
		markFailedErr: errors.New("mark failed"),
	}
	relay := NewOutboxRelayWithOptions(OutboxRelayOptions{
		Name:      "test-relay",
		Store:     store,
		Publisher: &fakePublisher{failAt: map[string]error{eventcatalog.EvaluationRequested: errors.New("publish failed")}},
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
			pending:          []PendingOutboxEvent{pendingEvent("evt-1", eventcatalog.EvaluationRequested)},
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
		pending:   []PendingOutboxEvent{pendingEvent("evt-1", eventcatalog.EvaluationRequested)},
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
	if observer.statusScrapeCount() != 1 {
		t.Fatalf("status scrapes before interval = %d, want 1", observer.statusScrapeCount())
	}

	current = current.Add(31 * time.Second)
	reporter.ReportOutboxStatus(context.Background())

	if store.statusCalls != 2 {
		t.Fatalf("status calls after interval = %d, want 2", store.statusCalls)
	}
	if observer.statusScrapeCount() != 2 {
		t.Fatalf("status scrapes after interval = %d, want 2", observer.statusScrapeCount())
	}
}

func TestDurableOutboxRelayRejectsNonMQBackedPublisher(t *testing.T) {
	store := &fakeOutboxStore{
		pending: []PendingOutboxEvent{pendingEvent("evt-1", eventcatalog.EvaluationRequested)},
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
		pending: []PendingOutboxEvent{pendingEvent("evt-1", eventcatalog.EvaluationRequested)},
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
			string(eventcatalog.PriorityP0): {"evt-zset"},
		},
	}
	store := &fakeOutboxStore{
		claimByIDs: map[string]PendingOutboxEvent{
			"evt-zset": pendingEvent("evt-zset", eventcatalog.AnswerSheetSubmitted),
		},
	}
	relay := NewOutboxRelayWithOptions(OutboxRelayOptions{
		Name:         "test-relay",
		Store:        store,
		Publisher:    &fakePublisher{},
		ReadyIndex:   ready,
		ReadyBuckets: []string{string(eventcatalog.PriorityP0), string(eventcatalog.PriorityP1), string(eventcatalog.PriorityP2)},
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
		pending: []PendingOutboxEvent{pendingEvent("evt-1", eventcatalog.EvaluationRequested)},
	}
	relay := NewOutboxRelayWithOptions(OutboxRelayOptions{
		Name:       "test-relay",
		Store:      store,
		Publisher:  &fakePublisher{failAt: map[string]error{eventcatalog.EvaluationRequested: errors.New("publish failed")}},
		ReadyIndex: ready,
	})

	if err := relay.DispatchDue(context.Background()); err != nil {
		t.Fatalf("DispatchDue: %v", err)
	}
	if len(ready.enqueues) != 1 {
		t.Fatalf("ready index enqueues = %#v, want one", ready.enqueues)
	}
	if ready.enqueues[0].eventType != eventcatalog.EvaluationRequested || ready.enqueues[0].eventID != "evt-1" {
		t.Fatalf("ready index enqueue = %#v, want assessment.submitted evt-1", ready.enqueues[0])
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
	observer.mu.Lock()
	defer observer.mu.Unlock()
	if len(observer.events) != 1 {
		t.Fatalf("observed outbox events = %#v, want one", observer.events)
	}
	if observer.events[0].Outcome != outcome {
		t.Fatalf("outcome = %q, want %q", observer.events[0].Outcome, outcome)
	}
}

func assertOutboxContainsOutcome(t *testing.T, observer *outboxObserver, outcome eventobservability.OutboxOutcome) {
	t.Helper()
	observer.mu.Lock()
	defer observer.mu.Unlock()
	for _, evt := range observer.events {
		if evt.Outcome == outcome {
			return
		}
	}
	t.Fatalf("observed outbox events = %#v, want outcome %q", observer.events, outcome)
}

func assertOutboxStatusScrape(t *testing.T, observer *outboxObserver, outcome eventobservability.OutboxStatusScrapeOutcome) {
	t.Helper()
	observer.mu.Lock()
	defer observer.mu.Unlock()
	if len(observer.statusScrape) != 1 {
		t.Fatalf("observed status scrape events = %#v, want one", observer.statusScrape)
	}
	if observer.statusScrape[0].Outcome != outcome {
		t.Fatalf("scrape outcome = %q, want %q", observer.statusScrape[0].Outcome, outcome)
	}
}

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

type immediateTestStore struct {
	fakeOutboxStore
	claimBlock chan struct{}
	claimCalls int
	claimed    map[string]bool
	mu         sync.Mutex
}

func (s *immediateTestStore) ClaimEventsByIDs(ctx context.Context, eventIDs []string, _ time.Time) ([]outboxport.PendingEvent, error) {
	s.mu.Lock()
	s.claimCalls++
	s.mu.Unlock()
	if s.claimBlock != nil {
		select {
		case <-s.claimBlock:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claimed == nil {
		s.claimed = make(map[string]bool)
	}
	claimed := make([]outboxport.PendingEvent, 0, len(eventIDs))
	for _, eventID := range eventIDs {
		if s.claimed[eventID] {
			continue
		}
		s.claimed[eventID] = true
		claimed = append(claimed, pendingEvent(eventID, eventcatalog.AnswerSheetSubmitted))
	}
	return claimed, nil
}

func TestImmediateDispatcherUsesExplicitEventTypes(t *testing.T) {
	dispatcher := NewImmediateDispatcher(ImmediateDispatcherOptions{
		ImmediateEventTypes: []string{eventcatalog.AnswerSheetSubmitted, eventcatalog.EvaluationRequested},
	})
	if _, ok := dispatcher.immediateEventTypes[eventcatalog.AnswerSheetSubmitted]; !ok {
		t.Fatal("answersheet.submitted should be immediate")
	}
	if _, ok := dispatcher.immediateEventTypes[eventcatalog.EvaluationRequested]; !ok {
		t.Fatal("assessment.submitted should be immediate for MySQL assessment outbox")
	}
}

func TestImmediateDispatcherRespectsMaxConcurrent(t *testing.T) {
	store := &immediateTestStore{claimBlock: make(chan struct{})}
	publisher := &fakePublisher{}
	observer := &outboxObserver{}
	dispatcher := NewImmediateDispatcher(ImmediateDispatcherOptions{
		Name:                "test-immediate",
		Store:               store,
		Publisher:           publisher,
		Observer:            observer,
		Enabled:             true,
		MaxConcurrent:       1,
		Timeout:             time.Second,
		ImmediateEventTypes: []string{eventcatalog.AnswerSheetSubmitted},
	})

	submitted := event.New(eventcatalog.AnswerSheetSubmitted, "Sample", "evt-1", struct{}{})
	dispatcher.TryDispatchAfterCommit(context.Background(), []event.DomainEvent{submitted})

	waitFor(t, func() bool {
		store.mu.Lock()
		defer store.mu.Unlock()
		return store.claimCalls == 1
	})

	deferred := event.New(eventcatalog.AnswerSheetSubmitted, "Sample", "evt-2", struct{}{})
	dispatcher.TryDispatchAfterCommit(context.Background(), []event.DomainEvent{deferred})

	waitFor(t, func() bool {
		store.mu.Lock()
		calls := store.claimCalls
		store.mu.Unlock()
		return calls == 1 && observer.hasOutcome(eventobservability.OutboxOutcomeImmediateSkipped)
	})
	assertOutboxContainsOutcome(t, observer, eventobservability.OutboxOutcomeImmediateSkipped)

	close(store.claimBlock)

	waitFor(t, func() bool {
		publisher.mu.Lock()
		defer publisher.mu.Unlock()
		return len(publisher.published) == 1
	})
	dispatcher.Close()

	publisher.mu.Lock()
	published := append([]string(nil), publisher.published...)
	publisher.mu.Unlock()
	if len(published) != 1 || published[0] != eventcatalog.AnswerSheetSubmitted {
		t.Fatalf("published = %#v, want one answersheet.submitted", published)
	}
}

func TestImmediateDispatcherRequiresMQBackedPublisherForDurableEvents(t *testing.T) {
	store := &immediateTestStore{}
	dispatcher := NewImmediateDispatcher(ImmediateDispatcherOptions{
		Name:                    "test-durable-immediate",
		Store:                   store,
		Publisher:               &durableFakePublisher{mqBacked: false},
		Enabled:                 true,
		RequireDurablePublisher: true,
		ImmediateEventTypes:     []string{eventcatalog.AnswerSheetSubmitted},
	})

	submitted := event.New(eventcatalog.AnswerSheetSubmitted, "Sample", "evt-non-mq", struct{}{})
	dispatcher.TryDispatchAfterCommit(context.Background(), []event.DomainEvent{submitted})
	time.Sleep(20 * time.Millisecond)

	store.mu.Lock()
	claimCalls := store.claimCalls
	store.mu.Unlock()
	if claimCalls != 0 {
		t.Fatalf("ClaimEventsByIDs calls = %d, want 0", claimCalls)
	}
	if len(store.published) != 0 {
		t.Fatalf("published = %#v, want durable event to remain pending", store.published)
	}
}

func TestImmediateDispatcherAllowsMQBackedPublisherForDurableEvents(t *testing.T) {
	store := &immediateTestStore{}
	publisher := &durableFakePublisher{mqBacked: true}
	dispatcher := NewImmediateDispatcher(ImmediateDispatcherOptions{
		Name:                    "test-durable-immediate",
		Store:                   store,
		Publisher:               publisher,
		Enabled:                 true,
		RequireDurablePublisher: true,
		ImmediateEventTypes:     []string{eventcatalog.AnswerSheetSubmitted},
	})

	submitted := event.New(eventcatalog.AnswerSheetSubmitted, "Sample", "evt-mq", struct{}{})
	dispatcher.TryDispatchAfterCommit(context.Background(), []event.DomainEvent{submitted})
	dispatcher.Close()
	if len(store.published) != 1 {
		t.Fatalf("published = %#v, want one event", store.published)
	}
}

func TestImmediateDispatcherAtomicallyClaimsEventBeforePublish(t *testing.T) {
	store := &immediateTestStore{}
	publisher := &durableFakePublisher{mqBacked: true}
	dispatcher := NewImmediateDispatcher(ImmediateDispatcherOptions{
		Name:                    "test-claim-immediate",
		Store:                   store,
		Publisher:               publisher,
		Enabled:                 true,
		RequireDurablePublisher: true,
		ImmediateEventTypes:     []string{eventcatalog.AnswerSheetSubmitted},
	})

	evt := event.New(eventcatalog.AnswerSheetSubmitted, "Sample", "evt-claimed-once", struct{}{})
	dispatcher.TryDispatchAfterCommit(context.Background(), []event.DomainEvent{evt, evt})
	dispatcher.Close()

	publisher.mu.Lock()
	published := append([]string(nil), publisher.published...)
	publisher.mu.Unlock()
	if len(published) != 1 {
		t.Fatalf("published = %#v, want one atomic-claim winner", published)
	}
	store.mu.Lock()
	claimCalls := store.claimCalls
	store.mu.Unlock()
	if claimCalls != 2 {
		t.Fatalf("ClaimEventsByIDs calls = %d, want 2 competing claims", claimCalls)
	}
}

func TestImmediateDispatcherPersistsPublishFailureAfterClaim(t *testing.T) {
	store := &immediateTestStore{}
	wantErr := errors.New("nsq unavailable")
	publisher := &fakePublisher{failAt: map[string]error{
		eventcatalog.AnswerSheetSubmitted: wantErr,
	}}
	dispatcher := NewImmediateDispatcher(ImmediateDispatcherOptions{
		Name:                "test-failed-immediate",
		Store:               store,
		Publisher:           publisher,
		Enabled:             true,
		ImmediateEventTypes: []string{eventcatalog.AnswerSheetSubmitted},
	})

	evt := event.New(eventcatalog.AnswerSheetSubmitted, "Sample", "evt-failed-claim", struct{}{})
	eventID := evt.EventID()
	dispatcher.TryDispatchAfterCommit(context.Background(), []event.DomainEvent{evt})
	dispatcher.Close()

	if len(store.failed) != 1 || store.failed[0] != eventID {
		t.Fatalf("failed marks = %#v, want claimed event persisted for retry", store.failed)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

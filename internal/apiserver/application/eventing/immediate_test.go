package eventing

import (
	"context"
	"sync"
	"testing"
	"time"

	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventobservability"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type immediateTestStore struct {
	fakeOutboxStore
	getBlock chan struct{}
	getCalls int
	mu       sync.Mutex
}

func (s *immediateTestStore) GetPublishableEvent(ctx context.Context, eventID string, _ time.Time) (outboxport.PendingEvent, bool, error) {
	s.mu.Lock()
	s.getCalls++
	s.mu.Unlock()
	if s.getBlock != nil {
		select {
		case <-s.getBlock:
		case <-ctx.Done():
			return outboxport.PendingEvent{}, false, ctx.Err()
		}
	}
	return pendingEvent(eventID, eventcatalog.AnswerSheetSubmitted), true, nil
}

func TestImmediateDispatcherUsesExplicitEventTypes(t *testing.T) {
	dispatcher := NewImmediateDispatcher(ImmediateDispatcherOptions{
		ImmediateEventTypes: []string{eventcatalog.AnswerSheetSubmitted, eventcatalog.AssessmentSubmitted},
	})
	if _, ok := dispatcher.immediateEventTypes[eventcatalog.AnswerSheetSubmitted]; !ok {
		t.Fatal("answersheet.submitted should be immediate")
	}
	if _, ok := dispatcher.immediateEventTypes[eventcatalog.AssessmentSubmitted]; !ok {
		t.Fatal("assessment.submitted should be immediate for MySQL assessment outbox")
	}
}

func TestImmediateDispatcherRespectsMaxConcurrent(t *testing.T) {
	store := &immediateTestStore{getBlock: make(chan struct{})}
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
		return store.getCalls == 1
	})

	deferred := event.New(eventcatalog.AnswerSheetSubmitted, "Sample", "evt-2", struct{}{})
	dispatcher.TryDispatchAfterCommit(context.Background(), []event.DomainEvent{deferred})

	waitFor(t, func() bool {
		store.mu.Lock()
		calls := store.getCalls
		store.mu.Unlock()
		return calls == 1 && observer.hasOutcome(eventobservability.OutboxOutcomeImmediateSkipped)
	})
	assertOutboxContainsOutcome(t, observer, eventobservability.OutboxOutcomeImmediateSkipped)

	close(store.getBlock)

	waitFor(t, func() bool {
		publisher.mu.Lock()
		defer publisher.mu.Unlock()
		return len(publisher.published) == 1
	})

	publisher.mu.Lock()
	published := append([]string(nil), publisher.published...)
	publisher.mu.Unlock()
	if len(published) != 1 || published[0] != eventcatalog.AnswerSheetSubmitted {
		t.Fatalf("published = %#v, want one answersheet.submitted", published)
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

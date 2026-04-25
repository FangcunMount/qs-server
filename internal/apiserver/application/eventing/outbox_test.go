package eventing

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventobservability"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type outboxObserver struct {
	events []eventobservability.OutboxEvent
}

func (o *outboxObserver) ObservePublish(context.Context, eventobservability.PublishEvent) {}
func (o *outboxObserver) ObserveConsume(context.Context, eventobservability.ConsumeEvent) {}

func (o *outboxObserver) ObserveOutbox(_ context.Context, evt eventobservability.OutboxEvent) {
	o.events = append(o.events, evt)
}

type fakeOutboxStore struct {
	pending          []PendingOutboxEvent
	claimErr         error
	markPublishedErr error
	markFailedErr    error
	published        []string
	failed           []string
}

func (s *fakeOutboxStore) ClaimDueEvents(context.Context, int, time.Time) ([]PendingOutboxEvent, error) {
	if s.claimErr != nil {
		return nil, s.claimErr
	}
	return s.pending, nil
}

func (s *fakeOutboxStore) MarkEventPublished(_ context.Context, eventID string, _ time.Time) error {
	s.published = append(s.published, eventID)
	return s.markPublishedErr
}

func (s *fakeOutboxStore) MarkEventFailed(_ context.Context, eventID, _ string, _ time.Time) error {
	s.failed = append(s.failed, eventID)
	return s.markFailedErr
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
	store := &fakeOutboxStore{pending: []PendingOutboxEvent{pendingEvent("evt-1", eventcatalog.AssessmentSubmitted)}}
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

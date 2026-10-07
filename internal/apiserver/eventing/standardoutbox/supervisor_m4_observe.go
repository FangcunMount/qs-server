package standardoutbox

import (
	"context"
	"time"

	eventobservability "github.com/FangcunMount/qs-server/internal/pkg/eventing/observe"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/outbox"
	"github.com/FangcunMount/reliable-messaging/transport"
)

// ObserveRelayDelivery preserves QS's delivery counters at the SDK seams that
// carry the original message. The SDK Relay observer has no message identity;
// using it to invent topic or event-type labels would misrepresent the evidence.
// These wrappers own no resources and leave every result and error unchanged.
func ObserveRelayDelivery(store outbox.Store, publisher transport.Publisher, name string, observer eventobservability.Observer) (outbox.Store, transport.Publisher) {
	if observer == nil {
		observer = eventobservability.DefaultObserver()
	}
	// Preserve absent dependencies so SDK construction keeps its own validation.
	if store != nil {
		store = observedStore{Store: store, name: name, observer: observer}
	}
	if publisher != nil {
		publisher = observedPublisher{Publisher: publisher, name: name, observer: observer}
	}
	return store, publisher
}

type observedStore struct {
	outbox.Store
	name     string
	observer eventobservability.Observer
}

func (s observedStore) ClaimDue(ctx context.Context, limit int, lease time.Duration) ([]outbox.Claim, error) {
	claims, err := s.Store.ClaimDue(ctx, limit, lease)
	if err != nil {
		// A failed scan does not identify an event or destination.
		s.observer.ObserveOutbox(ctx, eventobservability.OutboxEvent{Relay: s.name, Outcome: eventobservability.OutboxOutcomeClaimFailed})
	}
	return claims, err
}

func (s observedStore) Confirm(ctx context.Context, claim outbox.Claim) error {
	err := s.Store.Confirm(ctx, claim)
	outcome := eventobservability.OutboxOutcomePublished
	if err != nil {
		outcome = eventobservability.OutboxOutcomeMarkPublishedFailed
	}
	s.observe(ctx, claim, outcome)
	return err
}

func (s observedStore) Retry(ctx context.Context, claim outbox.Claim, delay time.Duration, code string) error {
	err := s.Store.Retry(ctx, claim, delay, code)
	if err != nil {
		s.observe(ctx, claim, eventobservability.OutboxOutcomeMarkFailedFailed)
	}
	return err
}

func (s observedStore) Quarantine(ctx context.Context, claim outbox.Claim, code string) error {
	err := s.Store.Quarantine(ctx, claim, code)
	if err != nil {
		s.observe(ctx, claim, eventobservability.OutboxOutcomeMarkFailedFailed)
	}
	return err
}

func (s observedStore) observe(ctx context.Context, claim outbox.Claim, outcome eventobservability.OutboxOutcome) {
	input := claim.Message.Input()
	s.observer.ObserveOutbox(ctx, eventobservability.OutboxEvent{Relay: s.name, Topic: input.Destination, EventType: input.EventType, Outcome: outcome})
}

type observedPublisher struct {
	transport.Publisher
	name     string
	observer eventobservability.Observer
}

func (p observedPublisher) Publish(ctx context.Context, msg message.Message) transport.Result {
	result := p.Publisher.Publish(ctx, msg)
	if result.Outcome != transport.Confirmed {
		// Unknown remains Unknown. A non-confirmed transport result neither
		// proves that nothing was sent nor that the consumer's business failed.
		input := msg.Input()
		p.observer.ObserveOutbox(ctx, eventobservability.OutboxEvent{Relay: p.name, Topic: input.Destination, EventType: input.EventType, Outcome: eventobservability.OutboxOutcomePublishFailed})
	}
	// A confirmed PUB is not counted as published until Store.Confirm succeeds.
	return result
}

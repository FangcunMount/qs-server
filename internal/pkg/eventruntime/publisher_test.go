package eventruntime

import (
	"context"
	"errors"
	"testing"

	"github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventobservability"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type capturedPublisher struct {
	topic string
	msg   *messaging.Message
	err   error
}

func (p *capturedPublisher) Publish(_ context.Context, _ string, _ []byte) error { return nil }

func (p *capturedPublisher) PublishMessage(_ context.Context, topic string, msg *messaging.Message) error {
	p.topic = topic
	p.msg = msg
	return p.err
}

func (p *capturedPublisher) Close() error { return nil }

type publishObserver struct {
	events []eventobservability.PublishEvent
}

func (o *publishObserver) ObservePublish(_ context.Context, evt eventobservability.PublishEvent) {
	o.events = append(o.events, evt)
}

func (o *publishObserver) ObserveOutbox(context.Context, eventobservability.OutboxEvent)   {}
func (o *publishObserver) ObserveConsume(context.Context, eventobservability.ConsumeEvent) {}

func TestRoutingPublisherUsesExplicitCatalogAndMetadata(t *testing.T) {
	cfg, err := eventcatalog.Load("../../../configs/events.yaml")
	if err != nil {
		t.Fatalf("Load events.yaml: %v", err)
	}
	mq := &capturedPublisher{}
	publisher := NewRoutingPublisher(RoutingPublisherOptions{
		Catalog:     eventcatalog.NewCatalog(cfg),
		MQPublisher: mq,
		Observer:    &publishObserver{},
		Source:      "unit-test",
		Mode:        PublishModeMQ,
	})
	evt := event.New(eventcatalog.AnswerSheetSubmitted, "AnswerSheet", "sheet-1", map[string]string{"id": "sheet-1"})

	if err := publisher.Publish(context.Background(), evt); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if mq.topic == "" {
		t.Fatalf("topic was not captured")
	}
	if mq.msg == nil {
		t.Fatalf("message was not captured")
	}
	if mq.msg.Metadata["event_type"] != eventcatalog.AnswerSheetSubmitted {
		t.Fatalf("event_type metadata = %q", mq.msg.Metadata["event_type"])
	}
	if mq.msg.Metadata["source"] != "unit-test" {
		t.Fatalf("source metadata = %q", mq.msg.Metadata["source"])
	}
	if len(mq.msg.Payload) == 0 {
		t.Fatalf("payload is empty")
	}
}

func TestRoutingPublisherAllowsDurableOutboxEventForRelayPublish(t *testing.T) {
	catalog := loadEventCatalog(t)
	if !catalog.IsDurableOutbox(eventcatalog.AnswerSheetSubmitted) {
		t.Fatalf("%q must be configured as durable_outbox for this contract test", eventcatalog.AnswerSheetSubmitted)
	}
	mq := &capturedPublisher{}
	publisher := NewRoutingPublisher(RoutingPublisherOptions{
		Catalog:     catalog,
		MQPublisher: mq,
		Source:      "outbox-relay",
		Mode:        PublishModeMQ,
	})
	evt := event.New(eventcatalog.AnswerSheetSubmitted, "AnswerSheet", "sheet-1", map[string]string{"id": "sheet-1"})

	if err := publisher.Publish(context.Background(), evt); err != nil {
		t.Fatalf("Publish durable outbox event from relay path: %v", err)
	}
	if mq.topic == "" {
		t.Fatalf("durable outbox event was not routed to MQ")
	}
	if mq.msg == nil || mq.msg.Metadata["event_type"] != eventcatalog.AnswerSheetSubmitted {
		t.Fatalf("published message metadata = %#v", mq.msg)
	}
}

func TestRoutingPublisherObservesMQPublished(t *testing.T) {
	observer := &publishObserver{}
	publisher := NewRoutingPublisher(RoutingPublisherOptions{
		Catalog:     loadEventCatalog(t),
		MQPublisher: &capturedPublisher{},
		Observer:    observer,
		Source:      "unit-test",
		Mode:        PublishModeMQ,
	})

	err := publisher.Publish(context.Background(), event.New(eventcatalog.AnswerSheetSubmitted, "AnswerSheet", "sheet-1", struct{}{}))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	assertPublishOutcome(t, observer, eventobservability.PublishOutcomeMQPublished)
}

func TestRoutingPublisherObservesNilPublisherFallback(t *testing.T) {
	observer := &publishObserver{}
	publisher := NewRoutingPublisher(RoutingPublisherOptions{
		Catalog:  loadEventCatalog(t),
		Observer: observer,
		Source:   "unit-test",
		Mode:     PublishModeMQ,
	})

	err := publisher.Publish(context.Background(), event.New(eventcatalog.AnswerSheetSubmitted, "AnswerSheet", "sheet-1", struct{}{}))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	assertPublishOutcome(t, observer, eventobservability.PublishOutcomeFallbackLogged)
}

func TestRoutingPublisherObservesLoggingAndNopModes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mode    PublishMode
		outcome eventobservability.PublishOutcome
	}{
		{name: "logging", mode: PublishModeLogging, outcome: eventobservability.PublishOutcomeLogged},
		{name: "nop", mode: PublishModeNop, outcome: eventobservability.PublishOutcomeNop},
	} {
		t.Run(tc.name, func(t *testing.T) {
			observer := &publishObserver{}
			publisher := NewRoutingPublisher(RoutingPublisherOptions{
				Catalog:  loadEventCatalog(t),
				Observer: observer,
				Source:   "unit-test",
				Mode:     tc.mode,
			})

			err := publisher.Publish(context.Background(), event.New(eventcatalog.AnswerSheetSubmitted, "AnswerSheet", "sheet-1", struct{}{}))
			if err != nil {
				t.Fatalf("Publish: %v", err)
			}
			assertPublishOutcome(t, observer, tc.outcome)
		})
	}
}

func TestRoutingPublisherObservesUnknownEvent(t *testing.T) {
	observer := &publishObserver{}
	publisher := NewRoutingPublisher(RoutingPublisherOptions{
		Catalog: eventcatalog.NewCatalog(&eventcatalog.Config{
			Topics: map[string]eventcatalog.TopicConfig{},
			Events: map[string]eventcatalog.EventConfig{},
		}),
		Observer: observer,
		Mode:     PublishModeNop,
	})

	err := publisher.Publish(context.Background(), event.New("unknown.event", "Unknown", "1", map[string]string{}))
	if err == nil {
		t.Fatalf("Publish should reject unknown event")
	}
	assertPublishOutcome(t, observer, eventobservability.PublishOutcomeUnknownEvent)
}

func TestRoutingPublisherObservesEncodeFailed(t *testing.T) {
	observer := &publishObserver{}
	publisher := NewRoutingPublisher(RoutingPublisherOptions{
		Catalog:     loadEventCatalog(t),
		MQPublisher: &capturedPublisher{},
		Observer:    observer,
		Mode:        PublishModeMQ,
	})

	err := publisher.Publish(context.Background(), event.New(eventcatalog.AnswerSheetSubmitted, "AnswerSheet", "sheet-1", map[string]any{
		"bad": func() {},
	}))
	if err == nil {
		t.Fatalf("Publish should fail on non-json payload")
	}
	assertPublishOutcome(t, observer, eventobservability.PublishOutcomeEncodeFailed)
}

func TestRoutingPublisherObservesMQFailed(t *testing.T) {
	wantErr := errors.New("mq failed")
	observer := &publishObserver{}
	publisher := NewRoutingPublisher(RoutingPublisherOptions{
		Catalog:     loadEventCatalog(t),
		MQPublisher: &capturedPublisher{err: wantErr},
		Observer:    observer,
		Mode:        PublishModeMQ,
	})

	err := publisher.Publish(context.Background(), event.New(eventcatalog.AnswerSheetSubmitted, "AnswerSheet", "sheet-1", struct{}{}))
	if !errors.Is(err, wantErr) {
		t.Fatalf("Publish error = %v, want %v", err, wantErr)
	}
	assertPublishOutcome(t, observer, eventobservability.PublishOutcomeMQFailed)
}

func TestRoutingPublisherRejectsUnknownEvent(t *testing.T) {
	publisher := NewRoutingPublisher(RoutingPublisherOptions{
		Catalog: eventcatalog.NewCatalog(&eventcatalog.Config{
			Topics: map[string]eventcatalog.TopicConfig{},
			Events: map[string]eventcatalog.EventConfig{},
		}),
		Mode: PublishModeNop,
	})
	evt := event.New("unknown.event", "Unknown", "1", map[string]string{})

	if err := publisher.Publish(context.Background(), evt); err == nil {
		t.Fatalf("Publish should reject unknown event")
	}
}

func loadEventCatalog(t *testing.T) *eventcatalog.Catalog {
	t.Helper()
	cfg, err := eventcatalog.Load("../../../configs/events.yaml")
	if err != nil {
		t.Fatalf("Load events.yaml: %v", err)
	}
	return eventcatalog.NewCatalog(cfg)
}

func assertPublishOutcome(t *testing.T, observer *publishObserver, outcome eventobservability.PublishOutcome) {
	t.Helper()
	if len(observer.events) != 1 {
		t.Fatalf("observed publish events = %#v, want one", observer.events)
	}
	if observer.events[0].Outcome != outcome {
		t.Fatalf("outcome = %q, want %q", observer.events[0].Outcome, outcome)
	}
	if observer.events[0].EventType == "" && outcome != eventobservability.PublishOutcomeUnknownEvent {
		t.Fatalf("event type was not captured")
	}
}
